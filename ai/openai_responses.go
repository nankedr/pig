package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

func streamOpenAIResponses(ctx context.Context, model Model, input Context, options OpenAIResponsesOptions) *AssistantMessageEventStream {
	if model.Provider != ProviderIDDeepSeek {
		return failedProviderStream(newNotImplemented("OpenAIResponses.Provider." + string(model.Provider)))
	}
	if model.API != APIOpenAIResponses {
		return failedProviderStream(fmt.Errorf("%w: model API %q is not %q", ErrEventStreamInvariant, model.API, APIOpenAIResponses))
	}
	if options.ReasoningSummary.IsSet() || options.ServiceTier.IsSet() {
		return failedProviderStream(newNotImplemented("OpenAIResponses.AdvancedOptions"))
	}
	cacheDisabled := options.CacheRetention != nil && *options.CacheRetention == CacheRetentionNone
	if options.Temperature != nil || options.SamplingParams != nil || model.SamplingParams != nil || options.Metadata != nil || options.Env != nil || options.WebSocketConnectTimeoutMS != nil || options.Transport != nil && *options.Transport != TransportSSE && *options.Transport != TransportAuto || !cacheDisabled && (options.CacheRetention != nil || options.SessionID != nil) {
		return failedProviderStream(newNotImplemented("OpenAIResponses.AdvancedOptions"))
	}
	if model.Compat.IsSet() {
		return failedProviderStream(newNotImplemented("OpenAIResponses.Compat"))
	}
	items, err := responsesInput(model, input)
	if err != nil {
		return failedProviderStream(err)
	}
	tools, err := responsesTools(input.Tools)
	if err != nil {
		return failedProviderStream(err)
	}
	payload := map[string]any{"model": model.ID, "input": items, "stream": true, "store": false}
	if len(tools) > 0 {
		payload["tools"] = tools
	}
	if !isNilRuntimeValue(options.ToolChoice) {
		switch options.ToolChoice.(type) {
		case OpenAIResponsesToolChoiceMode, *OpenAIResponsesToolChoiceMode, OpenAIResponsesToolChoiceFunction, *OpenAIResponsesToolChoiceFunction:
			payload["tool_choice"] = options.ToolChoice
		default:
			return failedProviderStream(newNotImplemented("OpenAIResponses.ToolChoice"))
		}
	}
	if instructions, ok := input.SystemPrompt.Value(); ok {
		payload["instructions"] = sanitizeOpenAIText(instructions)
	}
	if options.MaxTokens != nil {
		payload["max_output_tokens"] = *options.MaxTokens
	}
	if options.ReasoningEffort != nil {
		if !options.ReasoningEffort.valid() {
			return failedProviderStream(fmt.Errorf("invalid reasoning effort %q", *options.ReasoningEffort))
		}
		payload["reasoning"] = map[string]any{"effort": *options.ReasoningEffort}
	} else if model.Reasoning {
		payload["reasoning"] = map[string]any{"effort": "none"}
	}
	stream := NewAssistantMessageEventStream()
	go runOpenAIResponses(nonNilContext(ctx), stream, model, options, payload)
	return stream
}

func streamSimpleOpenAIResponses(ctx context.Context, model Model, input Context, options SimpleStreamOptions) *AssistantMessageEventStream {
	base := options.StreamOptions
	tokens := model.MaxTokens
	if base.MaxTokens != nil {
		tokens = *base.MaxTokens
	}
	tokens = clampOpenAIMaxTokens(model, input, tokens)
	base.MaxTokens = &tokens
	var effort *OpenAIReasoningEffort
	if options.Reasoning != nil {
		value := OpenAIReasoningEffort(ClampThinkingLevel(model, ModelThinkingLevel(*options.Reasoning)))
		if value != "off" {
			effort = &value
		}
	}
	return streamOpenAIResponses(ctx, model, input, OpenAIResponsesOptions{StreamOptions: base, ReasoningEffort: effort})
}

func responsesInput(model Model, input Context) ([]any, error) {
	if input.SystemPrompt.IsNull() {
		return nil, fmt.Errorf("system prompt cannot be null")
	}
	messages, err := TransformMessages(input.Messages, model)
	if err != nil {
		return nil, err
	}
	if err := ValidateUserImages(input); err != nil {
		return nil, err
	}
	items := make([]any, 0, len(messages))
	calls, results := map[string]bool{}, map[string]bool{}
	for _, message := range messages {
		switch m := message.(type) {
		case UserMessage:
			parts := []map[string]any{}
			if text, ok := m.Content.Text(); ok {
				parts = append(parts, map[string]any{"type": "input_text", "text": sanitizeOpenAIText(text)})
			} else {
				blocks, _ := m.Content.Blocks()
				for _, block := range blocks {
					switch block := replayContentValue(block).(type) {
					case TextContent:
						parts = append(parts, map[string]any{"type": "input_text", "text": sanitizeOpenAIText(block.Text)})
					case ImageContent:
						parts = append(parts, map[string]any{"type": "input_image", "detail": "auto", "image_url": "data:" + block.MIMEType + ";base64," + block.Data})
					default:
						return nil, fmt.Errorf("invalid user content")
					}
				}
			}
			if len(parts) > 0 {
				items = append(items, map[string]any{"role": "user", "content": parts})
			}
		case AssistantMessage:
			for _, block := range m.Content {
				switch block := replayContentValue(block).(type) {
				case TextContent:
					items = append(items, map[string]any{"type": "message", "role": "assistant", "status": "completed", "content": []any{map[string]any{"type": "output_text", "text": sanitizeOpenAIText(block.Text), "annotations": []any{}}}})
				case ThinkingContent:
					if redacted, _ := block.Redacted.Value(); redacted {
						return nil, newNotImplemented("OpenAIResponses.Input.EncryptedReasoning")
					}
					item := map[string]any{"type": "reasoning", "content": []any{map[string]any{"type": "reasoning_text", "text": sanitizeOpenAIText(block.Thinking)}}}
					if signature, ok := block.ThinkingSignature.Value(); ok {
						var metadata struct {
							ID string `json:"id"`
						}
						if json.Unmarshal([]byte(signature), &metadata) == nil && metadata.ID != "" {
							item["id"] = metadata.ID
						}
					}
					items = append(items, item)
				case ToolCall:
					callID, itemID, _ := strings.Cut(block.ID, "|")
					if callID == "" || calls[callID] {
						return nil, errors.New("Responses history has empty or duplicate call_id")
					}
					calls[callID] = true
					args, err := json.Marshal(block.Arguments)
					if err != nil {
						return nil, err
					}
					item := map[string]any{"type": "function_call", "call_id": callID, "name": block.Name, "arguments": string(args)}
					if itemID != "" {
						item["id"] = itemID
					}
					items = append(items, item)
				}
			}
		case ToolResultMessage:
			callID, _, _ := strings.Cut(m.ToolCallID, "|")
			if !calls[callID] || results[callID] {
				return nil, errors.New("Responses history has orphan or duplicate function_call_output")
			}
			results[callID] = true
			var texts []string
			for _, block := range m.Content {
				text, ok := replayContentValue(block).(TextContent)
				if !ok {
					return nil, newNotImplemented("OpenAIResponses.Input.ToolResultImage")
				}
				texts = append(texts, sanitizeOpenAIText(text.Text))
			}
			items = append(items, map[string]any{"type": "function_call_output", "call_id": callID, "output": strings.Join(texts, "\n")})
		default:
			return nil, newNotImplemented("OpenAIResponses.Input.ToolResult")
		}
	}
	return items, nil
}

func responsesTools(tools []Tool) ([]any, error) {
	out := make([]any, 0, len(tools))
	seen := map[string]bool{}
	for _, tool := range tools {
		if tool.Name == "" || len(tool.Name) > 128 || sanitizeOpenAIToolCallIDPart(tool.Name) != tool.Name || seen[tool.Name] {
			return nil, fmt.Errorf("invalid or duplicate Responses function name %q", tool.Name)
		}
		seen[tool.Name] = true
		if !isNilRuntimeValue(tool.ConstrainedSampling) {
			switch tool.ConstrainedSampling.(type) {
			case ConstrainedSamplingDisabled, *ConstrainedSamplingDisabled:
			default:
				return nil, newNotImplemented("OpenAIResponses.Tools.ConstrainedSampling")
			}
		}
		item := map[string]any{"type": "function", "name": tool.Name, "description": tool.Description}
		if len(tool.Parameters) > 0 {
			item["parameters"] = tool.Parameters
		}
		out = append(out, item)
	}
	return out, nil
}

func runOpenAIResponses(ctx context.Context, stream *AssistantMessageEventStream, model Model, options OpenAIResponsesOptions, payload map[string]any) {
	output := AssistantMessage{Role: MessageRoleAssistant, Content: []AssistantContent{}, API: model.API, Provider: model.Provider, Model: model.ID, StopReason: StopReasonPending, Timestamp: time.Now().UnixMilli()}
	if options.TimeoutMS != nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(*options.TimeoutMS)*time.Millisecond)
		defer cancel()
	}
	err := executeOpenAIResponses(ctx, stream, model, options, payload, &output)
	if err != nil {
		output.StopReason = StopReasonError
		if ctx.Err() != nil {
			output.StopReason = StopReasonAborted
			err = context.Cause(ctx)
		}
		output.ErrorMessage = Some(redactOpenAIRequestSecrets(openAIErrorMessage(err), model, OpenAICompletionsOptions{StreamOptions: options.StreamOptions}))
		stream.Push(AssistantMessageErrorEvent{Type: AssistantMessageEventTypeError, Reason: output.StopReason, Error: output})
		return
	}
	stream.Push(AssistantMessageDoneEvent{Type: AssistantMessageEventTypeDone, Reason: output.StopReason, Message: output})
}

type responsesPart struct {
	Type    string `json:"type"`
	Text    string `json:"text"`
	Refusal string `json:"refusal"`
}

type responsesItem struct {
	ID        string          `json:"id"`
	CallID    string          `json:"call_id"`
	Name      string          `json:"name"`
	Arguments string          `json:"arguments"`
	Type      string          `json:"type"`
	Status    string          `json:"status"`
	Content   []responsesPart `json:"content"`
}

type responsesResponse struct {
	ID     string          `json:"id"`
	Model  string          `json:"model"`
	Status string          `json:"status"`
	Output []responsesItem `json:"output"`
	Usage  *struct {
		Input        int64 `json:"input_tokens"`
		Output       int64 `json:"output_tokens"`
		Total        int64 `json:"total_tokens"`
		InputDetails struct {
			Cached int64 `json:"cached_tokens"`
			Write  int64 `json:"cache_write_tokens"`
		} `json:"input_tokens_details"`
		OutputDetails struct {
			Reasoning *int64 `json:"reasoning_tokens"`
		} `json:"output_tokens_details"`
	} `json:"usage"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
	Incomplete *struct {
		Reason string `json:"reason"`
	} `json:"incomplete_details"`
}

type responsesEvent struct {
	Type         string             `json:"type"`
	Sequence     *int64             `json:"sequence_number"`
	OutputIndex  int                `json:"output_index"`
	ContentIndex int                `json:"content_index"`
	Delta        string             `json:"delta"`
	Text         string             `json:"text"`
	Arguments    string             `json:"arguments"`
	ItemID       string             `json:"item_id"`
	Code         string             `json:"code"`
	Message      string             `json:"message"`
	Item         responsesItem      `json:"item"`
	Part         responsesPart      `json:"part"`
	Response     *responsesResponse `json:"response"`
}

type responsesContent struct {
	index int
	ended bool
}

type responsesToolState struct {
	index int
	item  responsesItem
	raw   string
	ended bool
}

func executeOpenAIResponses(ctx context.Context, stream *AssistantMessageEventStream, model Model, options OpenAIResponsesOptions, payload map[string]any, output *AssistantMessage) error {
	key, err := openAIClientAPIKey(model.Provider, options.APIKey, options.Headers)
	if err != nil {
		return err
	}
	var value any = payload
	if options.OnPayload != nil {
		result, err := options.OnPayload(ctx, payload, model)
		if err != nil {
			return err
		}
		if result.Replace {
			value = result.Value
		}
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	fetch := options.Fetch
	if fetch == nil {
		fetch = defaultOpenAIFetch
	}
	response, body, err := fetchOpenAICompletions(ctx, fetch, FetchRequest{URL: strings.TrimRight(model.BaseURL, "/") + "/responses", Method: http.MethodPost, Headers: openAIRequestHeaders(model.Headers, options.Headers, key), Body: encoded}, options.MaxRetries, options.MaxRetryDelayMS)
	if err != nil {
		return err
	}
	var once sync.Once
	closeBody := func() { once.Do(func() { _ = body.Close() }) }
	defer closeBody()
	stopClose := context.AfterFunc(ctx, closeBody)
	defer stopClose()
	if options.OnResponse != nil {
		if err := options.OnResponse(ctx, ProviderResponse{Status: response.Status, Headers: cloneOpenAIHeaders(response.Headers)}, model); err != nil {
			return err
		}
	}
	stream.Push(AssistantMessageStartEvent{Type: AssistantMessageEventTypeStart, Partial: *output})
	parts := map[[3]int]*responsesContent{}
	reasoningIDs := map[int]string{}
	var partOrder [][3]int
	appendPart := func(item, part, kind int, text string, full, end bool) error {
		key := [3]int{item, part, kind}
		state := parts[key]
		if state == nil {
			state = &responsesContent{index: len(output.Content)}
			parts[key] = state
			partOrder = append(partOrder, key)
			if kind == 0 {
				output.Content = append(output.Content, TextContent{Type: ContentTypeText})
				stream.Push(AssistantMessageTextStartEvent{Type: AssistantMessageEventTypeTextStart, ContentIndex: state.index, Partial: *output})
			} else {
				block := ThinkingContent{Type: ContentTypeThinking}
				if id := reasoningIDs[item]; id != "" {
					metadata, _ := json.Marshal(map[string]string{"id": id, "type": "reasoning"})
					block.ThinkingSignature = Some(string(metadata))
				}
				output.Content = append(output.Content, block)
				stream.Push(AssistantMessageThinkingStartEvent{Type: AssistantMessageEventTypeThinkingStart, ContentIndex: state.index, Partial: *output})
			}
		}
		previous := ""
		if kind == 0 {
			previous = output.Content[state.index].(TextContent).Text
		} else {
			previous = output.Content[state.index].(ThinkingContent).Thinking
		}
		if full {
			if !strings.HasPrefix(text, previous) {
				return errors.New("Responses final content disagrees with streamed content")
			}
			text = strings.TrimPrefix(text, previous)
		}
		if state.ended {
			if text != "" {
				return errors.New("Responses content changed after done")
			}
			return nil
		}
		if text != "" {
			if kind == 0 {
				block := output.Content[state.index].(TextContent)
				block.Text += text
				output.Content[state.index] = block
				stream.Push(AssistantMessageTextDeltaEvent{Type: AssistantMessageEventTypeTextDelta, ContentIndex: state.index, Delta: text, Partial: *output})
			} else {
				block := output.Content[state.index].(ThinkingContent)
				block.Thinking += text
				output.Content[state.index] = block
				stream.Push(AssistantMessageThinkingDeltaEvent{Type: AssistantMessageEventTypeThinkingDelta, ContentIndex: state.index, Delta: text, Partial: *output})
			}
		}
		if end {
			state.ended = true
			if kind == 0 {
				stream.Push(AssistantMessageTextEndEvent{Type: AssistantMessageEventTypeTextEnd, ContentIndex: state.index, Content: previous + text, Partial: *output})
			} else {
				stream.Push(AssistantMessageThinkingEndEvent{Type: AssistantMessageEventTypeThinkingEnd, ContentIndex: state.index, Content: previous + text, Partial: *output})
			}
		}
		return nil
	}
	tools := map[int]*responsesToolState{}
	var toolOrder []int
	callIDs := map[string]int{}
	if items, ok := payload["input"].([]any); ok {
		for _, raw := range items {
			if item, ok := raw.(map[string]any); ok && item["type"] == "function_call" {
				if id, ok := item["call_id"].(string); ok {
					callIDs[id] = -1
				}
			}
		}
	}
	appendTool := func(index int, item responsesItem, text string, full, end bool) error {
		state := tools[index]
		if state == nil {
			if item.CallID == "" || item.Name == "" {
				return errors.New("Responses function call missing call_id or name")
			}
			if _, exists := callIDs[item.CallID]; exists {
				return errors.New("Responses duplicate function call_id")
			}
			callIDs[item.CallID] = index
			state = &responsesToolState{index: len(output.Content), item: item}
			tools[index] = state
			toolOrder = append(toolOrder, index)
			output.Content = append(output.Content, ToolCall{Type: ContentTypeToolCall, ID: item.CallID + "|" + item.ID, Name: item.Name, Arguments: map[string]any{}})
			stream.Push(AssistantMessageToolCallStartEvent{Type: AssistantMessageEventTypeToolCallStart, ContentIndex: state.index, Partial: *output})
		}
		if item.CallID != "" && item.CallID != state.item.CallID || item.Name != "" && item.Name != state.item.Name || item.ID != "" && state.item.ID != "" && item.ID != state.item.ID {
			return errors.New("Responses function call identity changed")
		}
		if full {
			if !strings.HasPrefix(text, state.raw) {
				return errors.New("Responses final arguments disagree with deltas")
			}
			text = strings.TrimPrefix(text, state.raw)
		}
		if state.ended {
			if text != "" {
				return errors.New("Responses arguments changed after done")
			}
			return nil
		}
		state.raw += text
		call := output.Content[state.index].(ToolCall)
		call.Arguments = ParseStreamingJSONObject(state.raw)
		output.Content[state.index] = call
		if text != "" {
			stream.Push(AssistantMessageToolCallDeltaEvent{Type: AssistantMessageEventTypeToolCallDelta, ContentIndex: state.index, Delta: text, Partial: *output})
		}
		if end {
			var args map[string]any
			if item.Status != "incomplete" {
				if err := json.Unmarshal([]byte(state.raw), &args); err != nil || args == nil {
					return errors.New("Responses function arguments are not a complete JSON object")
				}
				call.Arguments = args
			}
			output.Content[state.index] = call
			state.ended = true
			stream.Push(AssistantMessageToolCallEndEvent{Type: AssistantMessageEventTypeToolCallEnd, ContentIndex: state.index, ToolCall: call, Partial: *output})
		}
		return nil
	}
	applyItem := func(index int, item responsesItem, end bool) error {
		if item.Type == "function_call" {
			return appendTool(index, item, item.Arguments, true, end)
		}
		if item.Type == "reasoning" && item.ID != "" {
			reasoningIDs[index] = item.ID
		}

		if item.Type != "message" && item.Type != "reasoning" {
			return newNotImplemented("OpenAIResponses.Output." + item.Type)
		}
		for p, part := range item.Content {
			kind := 0
			if item.Type == "reasoning" {
				kind = 1
			}
			text := part.Text
			if part.Type == "refusal" {
				text = part.Refusal
			}
			if err := appendPart(index, p, kind, text, true, end); err != nil {
				return err
			}
		}
		if item.Type == "reasoning" && item.ID != "" {
			metadata, _ := json.Marshal(map[string]string{"id": item.ID, "type": "reasoning"})
			for p := range item.Content {
				if state := parts[[3]int{index, p, 1}]; state != nil {
					block := output.Content[state.index].(ThinkingContent)
					block.ThinkingSignature = Some(string(metadata))
					output.Content[state.index] = block
				}
			}
		}
		return nil
	}
	terminal := false
	lastSequence := int64(-1)
	err = consumeOpenAISSE(body, func(data string) (bool, error) {
		var event responsesEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			return false, fmt.Errorf("Responses malformed SSE: %w", err)
		}
		if event.Type == "" {
			return false, errors.New("Responses SSE event has no type")
		}
		if event.Sequence != nil {
			if *event.Sequence <= lastSequence {
				return false, errors.New("Responses sequence_number is not increasing")
			}
			lastSequence = *event.Sequence
		}
		if event.Response != nil {
			r := event.Response
			if r.ID != "" {
				output.ResponseID = Some(r.ID)
			}
			if r.Model != "" && r.Model != model.ID {
				output.ResponseModel = Some(r.Model)
			}
			if r.Usage != nil {
				u := r.Usage
				output.Usage = Usage{Input: max(0, u.Input-u.InputDetails.Cached-u.InputDetails.Write), Output: u.Output, CacheRead: u.InputDetails.Cached, CacheWrite: u.InputDetails.Write, TotalTokens: u.Total}
				if u.OutputDetails.Reasoning != nil {
					output.Usage.Reasoning = Some(*u.OutputDetails.Reasoning)
				}
				CalculateCost(model, &output.Usage)
			}
		}
		switch event.Type {
		case "response.function_call_arguments.delta", "response.function_call_arguments.done":
			state := tools[event.OutputIndex]
			if state == nil {
				return false, errors.New("Responses arguments before function call item")
			}
			if event.ItemID != "" && event.ItemID != state.item.ID {
				return false, errors.New("Responses argument item_id mismatch")
			}
			text, done := event.Delta, event.Type == "response.function_call_arguments.done"
			if done {
				text = event.Arguments
			}
			return false, appendTool(event.OutputIndex, responsesItem{}, text, done, false)
		case "response.reasoning_summary_part.added", "response.reasoning_summary_part.done", "response.reasoning_summary_text.delta", "response.reasoning_summary_text.done":
			return false, nil
		case "response.output_item.added", "response.output_item.done":
			return false, applyItem(event.OutputIndex, event.Item, event.Type == "response.output_item.done")
		case "response.content_part.added", "response.content_part.done":
			kind, index := 0, event.ContentIndex
			if event.Part.Type == "reasoning_text" {
				kind = 1
			}
			text := event.Part.Text
			if event.Part.Type == "refusal" {
				text = event.Part.Refusal
			}
			return false, appendPart(event.OutputIndex, index, kind, text, true, strings.HasSuffix(event.Type, ".done"))
		case "response.output_text.delta", "response.refusal.delta", "response.reasoning_text.delta":
			kind, index := 0, event.ContentIndex
			if event.Type == "response.reasoning_text.delta" {
				kind = 1
			}
			return false, appendPart(event.OutputIndex, index, kind, event.Delta, false, false)
		case "response.output_text.done", "response.reasoning_text.done":
			kind, index := 0, event.ContentIndex
			if event.Type == "response.reasoning_text.done" {
				kind = 1
			}
			return false, appendPart(event.OutputIndex, index, kind, event.Text, true, true)
		case "response.completed", "response.incomplete", "response.failed":
			if event.Response == nil {
				return false, errors.New("Responses terminal event has no response")
			}
			r := event.Response
			status := strings.TrimPrefix(event.Type, "response.")
			if r.Status != status {
				return false, errors.New("Responses terminal status disagrees with event")
			}
			for i, item := range r.Output {
				if status != "completed" {
					item.Status = "incomplete"
				}
				if err := applyItem(i, item, true); err != nil {
					return false, err
				}
			}
			terminal = true
			output.RawStopReason = Some(status)
			output.StopReason = StopReasonStop
			if status == "completed" && len(tools) > 0 {
				for _, index := range toolOrder {
					state := tools[index]
					var args map[string]any
					if err := json.Unmarshal([]byte(state.raw), &args); err != nil || args == nil {
						return false, errors.New("Responses function arguments are not a complete JSON object")
					}
					if err := appendTool(index, responsesItem{}, state.raw, true, true); err != nil {
						return false, err
					}
				}
				output.StopReason = StopReasonToolUse
			}
			if status == "incomplete" {
				if r.Incomplete == nil {
					return true, errors.New("Response incomplete without a provider reason")
				}
				output.RawStopReason = Some(redactOpenAIRequestSecrets(status+"."+r.Incomplete.Reason, model, OpenAICompletionsOptions{StreamOptions: options.StreamOptions}))
				if r.Incomplete.Reason != "max_output_tokens" {
					return true, fmt.Errorf("Response incomplete: %s", r.Incomplete.Reason)
				}
				output.StopReason = StopReasonLength
			}
			if status == "failed" {
				if r.Error != nil {
					return true, fmt.Errorf("%s: %s", r.Error.Code, r.Error.Message)
				}
				return true, errors.New("Responses failed without error details")
			}
			return true, nil
		case "error":
			return true, fmt.Errorf("Error Code %s: %s", event.Code, event.Message)
		}
		return false, nil
	})
	for _, key := range partOrder {
		if endErr := appendPart(key[0], key[1], key[2], "", false, true); err == nil {
			err = endErr
		}
	}
	if ctx.Err() != nil {
		return context.Cause(ctx)
	}
	if err != nil {
		return err
	}
	if !terminal {
		return errors.New("OpenAI Responses stream ended before a terminal response event")
	}
	return nil
}

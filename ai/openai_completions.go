package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand/v2"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf16"
)

// SSE protocol errors cross Result as Go errors instead of business outcomes.
var (
	ErrOpenAISSEProtocol  = errors.New("OpenAI Completions SSE protocol error")
	ErrOpenAISSEMalformed = errors.New("OpenAI Completions SSE contains malformed JSON")
	ErrOpenAISSETruncated = errors.New("OpenAI Completions SSE ended with a truncated frame")
	ErrOpenAISSEEmpty     = errors.New("OpenAI Completions SSE response body is empty")
)

type openAICompletionsCompat struct {
	supportsReasoningEffort                     bool
	supportsUsageInStreaming                    bool
	supportsFinishReason                        bool
	requiresThinkingAsText                      bool
	requiresReasoningContentOnAssistantMessages bool
	thinkingFormat                              ThinkingFormat
	supportsThinkingTokenBudget                 bool
}

type openAICompletionChunk struct {
	ID      string                   `json:"id"`
	Model   string                   `json:"model"`
	Choices []openAICompletionChoice `json:"choices"`
	Usage   *openAICompletionUsage   `json:"usage"`
}

type openAICompletionChoice struct {
	Delta        *openAICompletionDelta `json:"delta"`
	FinishReason *string                `json:"finish_reason"`
	Usage        *openAICompletionUsage `json:"usage"`
}

type openAICompletionDelta struct {
	Content          *string                        `json:"content"`
	ReasoningContent *string                        `json:"reasoning_content"`
	Reasoning        *string                        `json:"reasoning"`
	ReasoningText    *string                        `json:"reasoning_text"`
	ReasoningDetails json.RawMessage                `json:"reasoning_details"`
	ToolCalls        []openAIStreamingToolCallDelta `json:"tool_calls"`
}

type openAIStreamingToolCallDelta struct {
	Index    *int                          `json:"index"`
	ID       string                        `json:"id"`
	Function *openAIStreamingFunctionDelta `json:"function"`
}

type openAIStreamingFunctionDelta struct {
	Name      *string                `json:"name"`
	Arguments *openAIStreamingString `json:"arguments"`
}

type openAIStreamingString struct{ value string }

type openAIToolCallState struct {
	contentIndex int
	call         ToolCall
	rawArguments string
	streamIndex  *int
}

type openAICompletionUsage struct {
	PromptTokens         int64 `json:"prompt_tokens"`
	CompletionTokens     int64 `json:"completion_tokens"`
	PromptCacheHitTokens int64 `json:"prompt_cache_hit_tokens"`
	PromptTokensDetails  *struct {
		CachedTokens     *int64 `json:"cached_tokens"`
		CacheWriteTokens int64  `json:"cache_write_tokens"`
	} `json:"prompt_tokens_details"`
	CompletionTokensDetails *struct {
		ReasoningTokens *int64 `json:"reasoning_tokens"`
	} `json:"completion_tokens_details"`
}

func streamOpenAICompletions(ctx context.Context, model Model, input Context, options OpenAICompletionsOptions) *AssistantMessageEventStream {
	ctx = nonNilContext(ctx)
	if err := validateOpenAICompletions(model, input, options); err != nil {
		return failedProviderStream(err)
	}
	stream := NewAssistantMessageEventStream()
	go runOpenAICompletions(ctx, stream, model, input, options)
	return stream
}

func streamSimpleOpenAICompletions(ctx context.Context, model Model, input Context, options SimpleStreamOptions) *AssistantMessageEventStream {
	streamOptions := options.StreamOptions
	maxTokens := model.MaxTokens
	if streamOptions.MaxTokens != nil {
		maxTokens = *streamOptions.MaxTokens
	}
	maxTokens = clampOpenAIMaxTokens(model, input, maxTokens)
	streamOptions.MaxTokens = &maxTokens
	var reasoningEffort *OpenAIReasoningEffort
	if options.Reasoning != nil {
		level := ClampThinkingLevel(model, ModelThinkingLevel(*options.Reasoning))
		if level != ModelThinkingLevelOff {
			effort := OpenAIReasoningEffort(level)
			reasoningEffort = &effort
		}
	}
	return streamOpenAICompletions(ctx, model, input, OpenAICompletionsOptions{
		StreamOptions: streamOptions, ReasoningEffort: reasoningEffort, ThinkingBudgets: options.ThinkingBudgets,
	})
}

func ConvertOpenAICompletionsMessages(model Model, input Context, compat OpenAICompletionsCompat, _ ...ConvertOpenAICompletionsMessagesOptions) ([]json.RawMessage, error) {
	if err := validateOpenAICompletionsCompat(compat); err != nil {
		return nil, err
	}
	if input.SystemPrompt.IsNull() {
		return nil, fmt.Errorf("system prompt cannot be null")
	}
	messages := make([]json.RawMessage, 0, len(input.Messages)+1)
	appendMessage := func(message any) error {
		raw, err := json.Marshal(message)
		if err == nil {
			messages = append(messages, raw)
		}
		return err
	}
	if system, ok := input.SystemPrompt.Value(); ok {
		if err := appendMessage(map[string]any{"role": "system", "content": sanitizeOpenAIText(system)}); err != nil {
			return nil, err
		}
	}
	toolCallIDs := map[string]string{}
	for _, message := range input.Messages {
		switch value := message.(type) {
		case UserMessage:
			if err := appendOpenAIUserMessage(&messages, value); err != nil {
				return nil, err
			}
		case *UserMessage:
			if value == nil {
				return nil, fmt.Errorf("nil user message")
			}
			if err := appendOpenAIUserMessage(&messages, *value); err != nil {
				return nil, err
			}
		case AssistantMessage:
			if err := appendOpenAIAssistantMessage(&messages, model, compat, value, toolCallIDs); err != nil {
				return nil, err
			}
		case *AssistantMessage:
			if value == nil {
				return nil, fmt.Errorf("nil assistant message")
			}
			if err := appendOpenAIAssistantMessage(&messages, model, compat, *value, toolCallIDs); err != nil {
				return nil, err
			}
		case ToolResultMessage:
			if err := appendOpenAIToolResultMessage(&messages, value, toolCallIDs); err != nil {
				return nil, err
			}
		case *ToolResultMessage:
			if value == nil {
				return nil, fmt.Errorf("nil tool result message")
			}
			if err := appendOpenAIToolResultMessage(&messages, *value, toolCallIDs); err != nil {
				return nil, err
			}
		default:
			return nil, fmt.Errorf("unsupported message %T", message)
		}
	}
	return messages, nil
}

func appendOpenAIUserMessage(messages *[]json.RawMessage, message UserMessage) error {
	var content any
	if text, ok := message.Content.Text(); ok {
		content = sanitizeOpenAIText(text)
	} else {
		blocks, _ := message.Content.Blocks()
		parts := make([]map[string]any, 0, len(blocks))
		for _, block := range blocks {
			switch value := block.(type) {
			case TextContent:
				parts = append(parts, map[string]any{"type": "text", "text": sanitizeOpenAIText(value.Text)})
			case *TextContent:
				if value != nil {
					parts = append(parts, map[string]any{"type": "text", "text": sanitizeOpenAIText(value.Text)})
				}
			case ImageContent, *ImageContent:
				return newNotImplemented("OpenAICompletions.ConvertMessages.Image")
			default:
				return fmt.Errorf("unsupported user content %T", block)
			}
		}
		if len(parts) == 0 {
			return nil
		}
		content = parts
	}
	raw, err := json.Marshal(map[string]any{"role": "user", "content": content})
	if err == nil {
		*messages = append(*messages, raw)
	}
	return err
}

func appendOpenAIAssistantMessage(messages *[]json.RawMessage, model Model, compat OpenAICompletionsCompat, message AssistantMessage, toolCallIDs map[string]string) error {
	if message.StopReason == StopReasonError || message.StopReason == StopReasonAborted {
		return nil
	}
	sameModel := message.Provider == model.Provider && message.API == model.API && message.Model == model.ID
	var textParts []string
	var thinking []ThinkingContent
	var toolCalls []map[string]any
	var reasoningDetails []any
	appendToolCall := func(call ToolCall) error {
		arguments, err := json.Marshal(call.Arguments)
		if err != nil {
			return err
		}
		id := call.ID
		if !sameModel {
			id = normalizeOpenAIToolCallID(model, id)
			if id != call.ID {
				toolCallIDs[call.ID] = id
			}
		}
		toolCalls = append(toolCalls, map[string]any{
			"id": id, "type": "function",
			"function": map[string]any{"name": call.Name, "arguments": string(arguments)},
		})
		if sameModel {
			if signature, ok := call.ThoughtSignature.Value(); ok {
				var detail any
				if json.Unmarshal([]byte(signature), &detail) == nil && truthyJSONValue(detail) {
					reasoningDetails = append(reasoningDetails, detail)
				}
			}
		}
		return nil
	}
	for _, block := range message.Content {
		switch value := block.(type) {
		case TextContent:
			if strings.TrimSpace(value.Text) != "" {
				textParts = append(textParts, sanitizeOpenAIText(value.Text))
			}
		case *TextContent:
			if value != nil && strings.TrimSpace(value.Text) != "" {
				textParts = append(textParts, sanitizeOpenAIText(value.Text))
			}
		case ThinkingContent:
			appendOpenAIThinking(&thinking, &textParts, value, sameModel)
		case *ThinkingContent:
			if value != nil {
				appendOpenAIThinking(&thinking, &textParts, *value, sameModel)
			}
		case ToolCall:
			if err := appendToolCall(value); err != nil {
				return err
			}
		case *ToolCall:
			if value != nil {
				if err := appendToolCall(*value); err != nil {
					return err
				}
			}
		default:
			return fmt.Errorf("unsupported assistant content %T", block)
		}
	}
	if len(textParts) == 0 && len(thinking) == 0 && len(toolCalls) == 0 {
		return nil
	}
	converted := map[string]any{"role": "assistant", "content": nil}
	requiresThinkingAsText, _ := compat.RequiresThinkingAsText.Value()
	if requiresThinkingAsText && len(thinking) > 0 {
		parts := make([]map[string]any, 0, len(textParts)+1)
		thinkingText := make([]string, len(thinking))
		for i, block := range thinking {
			thinkingText[i] = sanitizeOpenAIText(block.Thinking)
		}
		parts = append(parts, map[string]any{"type": "text", "text": strings.Join(thinkingText, "\n\n")})
		for _, text := range textParts {
			parts = append(parts, map[string]any{"type": "text", "text": text})
		}
		converted["content"] = parts
	} else {
		if len(textParts) > 0 {
			converted["content"] = strings.Join(textParts, "")
		}
		if len(thinking) > 0 {
			if signature, ok := thinking[0].ThinkingSignature.Value(); ok && signature != "" {
				if model.Provider == ProviderIDOpenCodeGo && signature == "reasoning" {
					signature = "reasoning_content"
				}
				values := make([]string, len(thinking))
				for i, block := range thinking {
					values[i] = sanitizeOpenAIText(block.Thinking)
				}
				converted[signature] = strings.Join(values, "\n")
			}
		}
	}
	if len(toolCalls) > 0 {
		converted["tool_calls"] = toolCalls
	}
	if len(reasoningDetails) > 0 {
		converted["reasoning_details"] = reasoningDetails
	}
	if required, _ := compat.RequiresReasoningContentOnAssistantMessages.Value(); required && model.Reasoning {
		if _, ok := converted["reasoning_content"]; !ok {
			converted["reasoning_content"] = ""
		}
	}
	raw, err := json.Marshal(converted)
	if err == nil {
		*messages = append(*messages, raw)
	}
	return err
}

func appendOpenAIThinking(thinking *[]ThinkingContent, textParts *[]string, block ThinkingContent, sameModel bool) {
	if !sameModel {
		if redacted, _ := block.Redacted.Value(); redacted || strings.TrimSpace(block.Thinking) == "" {
			return
		}
		*textParts = append(*textParts, sanitizeOpenAIText(block.Thinking))
		return
	}
	if strings.TrimSpace(block.Thinking) != "" {
		*thinking = append(*thinking, block)
	}
}

func truthyJSONValue(value any) bool {
	switch value := value.(type) {
	case nil:
		return false
	case bool:
		return value
	case float64:
		return value != 0 && !math.IsNaN(value)
	case string:
		return value != ""
	default:
		return true
	}
}

func appendOpenAIToolResultMessage(messages *[]json.RawMessage, message ToolResultMessage, toolCallIDs map[string]string) error {
	if message.AddedToolNames.IsSet() {
		return newNotImplemented("OpenAICompletions.ConvertMessages.ToolResultAddedToolNames")
	}
	var text []string
	for _, block := range message.Content {
		switch value := block.(type) {
		case TextContent:
			text = append(text, value.Text)
		case *TextContent:
			if value != nil {
				text = append(text, value.Text)
			}
		case ImageContent, *ImageContent:
			return newNotImplemented("OpenAICompletions.ConvertMessages.ToolResultImage")
		default:
			return fmt.Errorf("unsupported tool result content %T", block)
		}
	}
	content := strings.Join(text, "\n")
	if content == "" {
		content = "(no tool output)"
	}
	id := message.ToolCallID
	if normalized, ok := toolCallIDs[id]; ok {
		id = normalized
	}
	raw, err := json.Marshal(map[string]any{
		"role": "tool", "content": sanitizeOpenAIText(content), "tool_call_id": id,
	})
	if err == nil {
		*messages = append(*messages, raw)
	}
	return err
}

func normalizeOpenAIToolCallID(model Model, id string) string {
	if separator := strings.IndexByte(id, '|'); separator >= 0 {
		callID := sanitizeOpenAIToolCallIDPart(id[:separator])
		itemID := sanitizeOpenAIToolCallIDPart(id[separator+1:])
		combined := callID
		if itemID != "" {
			combined += "_" + itemID
		}
		if len(combined) <= 40 {
			return combined
		}
		hash := openAIShortHash(id)
		if len(hash) > 8 {
			hash = hash[:8]
		}
		prefixLength := max(1, 40-len(hash)-1)
		if len(callID) > prefixLength {
			callID = callID[:prefixLength]
		}
		return callID + "_" + hash
	}
	if model.Provider == ProviderIDOpenAI && len(id) > 40 {
		return id[:40]
	}
	return id
}

func sanitizeOpenAIToolCallIDPart(value string) string {
	var sanitized strings.Builder
	for _, unit := range utf16.Encode([]rune(value)) {
		if unit >= 'a' && unit <= 'z' || unit >= 'A' && unit <= 'Z' || unit >= '0' && unit <= '9' || unit == '_' || unit == '-' {
			sanitized.WriteByte(byte(unit))
		} else {
			sanitized.WriteByte('_')
		}
	}
	return sanitized.String()
}

func openAIShortHash(value string) string {
	h1, h2 := uint32(0xdeadbeef), uint32(0x41c6ce57)
	for _, unit := range utf16.Encode([]rune(value)) {
		h1 = (h1 ^ uint32(unit)) * 2654435761
		h2 = (h2 ^ uint32(unit)) * 1597334677
	}
	h1 = (h1^(h1>>16))*2246822507 ^ (h2^(h2>>13))*3266489909
	h2 = (h2^(h2>>16))*2246822507 ^ (h1^(h1>>13))*3266489909
	return strconv.FormatUint(uint64(h2), 36) + strconv.FormatUint(uint64(h1), 36)
}

func runOpenAICompletions(ctx context.Context, stream *AssistantMessageEventStream, model Model, input Context, options OpenAICompletionsOptions) {
	output := AssistantMessage{
		Role: MessageRoleAssistant, Content: []AssistantContent{}, API: model.API, Provider: model.Provider,
		Model: model.ID, StopReason: StopReasonPending, Timestamp: time.Now().UnixMilli(),
	}
	requestCtx := ctx
	cancel := func() {}
	if options.TimeoutMS != nil {
		requestCtx, cancel = context.WithTimeout(ctx, time.Duration(*options.TimeoutMS)*time.Millisecond)
	}
	defer cancel()

	if err := executeOpenAICompletions(requestCtx, stream, model, input, options, &output); err != nil {
		if errors.Is(err, ErrNotImplemented) || errors.Is(err, ErrOpenAISSEProtocol) {
			stream.stream.endWithError(err)
			return
		}
		output.StopReason = StopReasonError
		if requestCtx.Err() != nil {
			output.StopReason = StopReasonAborted
		}
		output.ErrorMessage = Some(openAIErrorMessage(err))
		stream.Push(AssistantMessageErrorEvent{
			Type: AssistantMessageEventTypeError, Reason: output.StopReason, Error: output,
		})
	}
}

func executeOpenAICompletions(ctx context.Context, stream *AssistantMessageEventStream, model Model, input Context, options OpenAICompletionsOptions, output *AssistantMessage) error {
	apiKey, err := openAIClientAPIKey(model.Provider, options.APIKey, options.Headers)
	if err != nil {
		return err
	}
	compat, err := resolveOpenAICompletionsCompat(model)
	if err != nil {
		return err
	}
	messages, err := ConvertOpenAICompletionsMessages(model, input, openAICompatPublic(compat))
	if err != nil {
		return err
	}
	payload := map[string]any{"model": model.ID, "messages": messages, "store": false, "stream": true}
	if compat.supportsUsageInStreaming {
		payload["stream_options"] = map[string]any{"include_usage": true}
	}
	if options.MaxTokens != nil {
		payload["max_completion_tokens"] = *options.MaxTokens
	}
	if options.Temperature != nil {
		payload["temperature"] = *options.Temperature
	}
	applyOpenAIReasoningOptions(payload, model, compat, options)
	if len(input.Tools) > 0 {
		tools, err := convertOpenAIFunctionTools(input.Tools)
		if err != nil {
			return err
		}
		payload["tools"] = tools
	}
	if !isNilRuntimeValue(options.ToolChoice) {
		payload["tool_choice"] = options.ToolChoice
	}
	if options.OnPayload != nil {
		result, hookErr := options.OnPayload(ctx, payload, model)
		if hookErr != nil {
			return hookErr
		}
		if result.Replace {
			encoded, marshalErr := json.Marshal(result.Value)
			if marshalErr != nil {
				return marshalErr
			}
			return sendOpenAICompletions(ctx, stream, model, options, apiKey, compat, encoded, output)
		}
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return sendOpenAICompletions(ctx, stream, model, options, apiKey, compat, encoded, output)
}

func applyOpenAIReasoningOptions(payload map[string]any, model Model, compat openAICompletionsCompat, options OpenAICompletionsOptions) {
	effort, enabled := mappedOpenAIReasoningEffort(model, options.ReasoningEffort)
	switch compat.thinkingFormat {
	case "zai":
		if model.Reasoning {
			thinking := map[string]any{"type": "disabled"}
			if enabled {
				thinking = map[string]any{"type": "enabled", "clear_thinking": false}
			}
			payload["thinking"] = thinking
			if enabled && compat.supportsReasoningEffort {
				payload["reasoning_effort"] = effort
			}
		}
	case "qwen":
		if model.Reasoning {
			payload["enable_thinking"] = enabled
			if enabled && compat.supportsReasoningEffort {
				payload["reasoning_effort"] = effort
			}
		}
	case "deepseek":
		if model.Reasoning {
			if enabled {
				payload["thinking"] = map[string]any{"type": "enabled"}
			} else if !openAIThinkingOffIsNull(model) {
				payload["thinking"] = map[string]any{"type": "disabled"}
			}
			if enabled && compat.supportsReasoningEffort {
				payload["reasoning_effort"] = effort
			}
		}
	case "openrouter":
		if model.Reasoning {
			if enabled {
				payload["reasoning"] = map[string]any{"effort": effort}
			} else if !openAIThinkingOffIsNull(model) {
				payload["reasoning"] = map[string]any{"effort": openAIThinkingOffValue(model, "none")}
			}
		}
	case "ant-ling":
		if model.Reasoning && enabled {
			if mapped, ok := openAIThinkingLevelValue(model, ModelThinkingLevel(*options.ReasoningEffort)); ok {
				payload["reasoning"] = map[string]any{"effort": mapped}
			}
		}
	case "together":
		if model.Reasoning {
			payload["reasoning"] = map[string]any{"enabled": enabled}
			if enabled && compat.supportsReasoningEffort {
				payload["reasoning_effort"] = effort
			}
		}
	case "string-thinking":
		if model.Reasoning {
			if enabled {
				payload["thinking"] = effort
			} else if !openAIThinkingOffIsNull(model) {
				payload["thinking"] = openAIThinkingOffValue(model, "none")
			}
		}
	default:
		if model.Reasoning && compat.supportsReasoningEffort {
			if enabled {
				payload["reasoning_effort"] = effort
			} else if off, ok := openAIThinkingLevelValue(model, ModelThinkingLevelOff); ok {
				payload["reasoning_effort"] = off
			}
		}
	}
	if compat.supportsThinkingTokenBudget && enabled && model.Reasoning {
		level := ModelThinkingLevel(*options.ReasoningEffort)
		if level == ModelThinkingLevelXHigh || level == ModelThinkingLevelMax {
			level = ModelThinkingLevelHigh
		}
		budget := openAIThinkingBudget(options.ThinkingBudgets, level)
		ceiling := model.MaxTokens
		if options.MaxTokens != nil {
			ceiling = *options.MaxTokens
		}
		budget = min(budget, max(int64(0), ceiling-1024))
		if budget > 0 {
			payload["thinking_token_budget"] = budget
		}
	}
}

func mappedOpenAIReasoningEffort(model Model, requested *OpenAIReasoningEffort) (string, bool) {
	if requested == nil {
		return "", false
	}
	if mapped, ok := openAIThinkingLevelValue(model, ModelThinkingLevel(*requested)); ok {
		return mapped, true
	}
	return string(*requested), true
}

func openAIThinkingLevelValue(model Model, level ModelThinkingLevel) (string, bool) {
	value, present := model.ThinkingLevelMap[level]
	if !present || value.IsNull() {
		return "", false
	}
	return value.Value()
}

func openAIThinkingOffIsNull(model Model) bool {
	value, present := model.ThinkingLevelMap[ModelThinkingLevelOff]
	return present && value.IsNull()
}

func openAIThinkingOffValue(model Model, fallback string) string {
	if value, ok := openAIThinkingLevelValue(model, ModelThinkingLevelOff); ok {
		return value
	}
	return fallback
}

func openAIThinkingBudget(budgets *ThinkingBudgets, level ModelThinkingLevel) int64 {
	defaults := map[ModelThinkingLevel]int64{
		ModelThinkingLevelMinimal: 1024, ModelThinkingLevelLow: 2048,
		ModelThinkingLevelMedium: 8192, ModelThinkingLevelHigh: 16384,
	}
	if budgets == nil {
		return defaults[level]
	}
	configured := map[ModelThinkingLevel]*int64{
		ModelThinkingLevelMinimal: budgets.Minimal, ModelThinkingLevelLow: budgets.Low,
		ModelThinkingLevelMedium: budgets.Medium, ModelThinkingLevelHigh: budgets.High,
	}
	if value := configured[level]; value != nil {
		return *value
	}
	return defaults[level]
}

func convertOpenAIFunctionTools(tools []Tool) ([]any, error) {
	converted := make([]any, len(tools))
	for i, tool := range tools {
		if tool.ConstrainedSampling != nil {
			return nil, newNotImplemented("OpenAICompletions.Tool.ConstrainedSampling")
		}
		if err := validateJSONSchemaRoot(tool.Parameters); err != nil {
			return nil, newCodecError("tool parameters", "", err)
		}
		converted[i] = map[string]any{
			"type": "function",
			"function": map[string]any{
				"name": tool.Name, "description": tool.Description,
				"parameters": append(json.RawMessage(nil), tool.Parameters...), "strict": false,
			},
		}
	}
	return converted, nil
}

func sendOpenAICompletions(ctx context.Context, stream *AssistantMessageEventStream, model Model, options OpenAICompletionsOptions, apiKey string, compat openAICompletionsCompat, payload []byte, output *AssistantMessage) error {
	headers := openAIRequestHeaders(model.Headers, options.Headers, apiKey)
	request := FetchRequest{
		URL: strings.TrimRight(model.BaseURL, "/") + "/chat/completions", Method: http.MethodPost,
		Headers: headers, Body: payload,
	}
	fetch := options.Fetch
	if fetch == nil {
		fetch = defaultOpenAIFetch
	}
	response, body, err := fetchOpenAICompletions(ctx, fetch, request, options.MaxRetries, options.MaxRetryDelayMS)
	if err != nil {
		return err
	}
	var closeOnce sync.Once
	closeBody := func() { closeOnce.Do(func() { _ = body.Close() }) }
	defer closeBody()
	stopClose := context.AfterFunc(ctx, closeBody)
	defer stopClose()
	if options.OnResponse != nil {
		if err := options.OnResponse(ctx, ProviderResponse{Status: response.Status, Headers: cloneOpenAIHeaders(response.Headers)}, model); err != nil {
			return err
		}
	}
	stream.Push(AssistantMessageStartEvent{Type: AssistantMessageEventTypeStart, Partial: *output})
	hasFinishReason := false
	sawFrame := false
	textIndex := -1
	thinkingIndex := -1
	toolCallsByIndex := map[int]*openAIToolCallState{}
	toolCallsByID := map[string]*openAIToolCallState{}
	pendingReasoningDetails := map[string]string{}
	var toolCalls []*openAIToolCallState
	err = consumeOpenAISSE(body, func(data string) (bool, error) {
		sawFrame = true
		if strings.HasPrefix(data, "[DONE]") {
			return true, nil
		}
		trimmed := bytes.TrimSpace([]byte(data))
		if len(trimmed) == 0 {
			return false, nil
		}
		if trimmed[0] != '{' {
			return false, errors.Join(ErrOpenAISSEProtocol, ErrOpenAISSEMalformed, errors.New("SSE data is not a JSON object"))
		}
		var chunk openAICompletionChunk
		if err := json.Unmarshal(trimmed, &chunk); err != nil {
			return false, errors.Join(ErrOpenAISSEProtocol, ErrOpenAISSEMalformed, err)
		}
		if chunk.ID != "" && !output.ResponseID.IsSet() {
			output.ResponseID = Some(chunk.ID)
		}
		if chunk.Model != "" && chunk.Model != model.ID && !output.ResponseModel.IsSet() {
			output.ResponseModel = Some(chunk.Model)
		}
		if chunk.Usage != nil {
			output.Usage = mapOpenAIUsage(*chunk.Usage, model)
		}
		if len(chunk.Choices) == 0 {
			return false, nil
		}
		choice := chunk.Choices[0]
		if chunk.Usage == nil && choice.Usage != nil {
			output.Usage = mapOpenAIUsage(*choice.Usage, model)
		}
		if choice.FinishReason != nil && *choice.FinishReason != "" {
			output.RawStopReason = Some(*choice.FinishReason)
			output.StopReason, err = mapOpenAIStopReason(*choice.FinishReason)
			if err != nil {
				output.ErrorMessage = Some(err.Error())
			}
			hasFinishReason = true
		}
		if choice.Delta != nil && choice.Delta.Content != nil && *choice.Delta.Content != "" {
			if textIndex == -1 {
				textIndex = len(output.Content)
				output.Content = append(output.Content, TextContent{Type: ContentTypeText, Text: *choice.Delta.Content})
				stream.Push(AssistantMessageTextStartEvent{Type: AssistantMessageEventTypeTextStart, ContentIndex: textIndex, Partial: *output})
			} else {
				block := output.Content[textIndex].(TextContent)
				block.Text += *choice.Delta.Content
				output.Content[textIndex] = block
			}
			stream.Push(AssistantMessageTextDeltaEvent{
				Type: AssistantMessageEventTypeTextDelta, ContentIndex: textIndex, Delta: *choice.Delta.Content, Partial: *output,
			})
		}
		if choice.Delta != nil {
			if field, delta := openAIReasoningDelta(choice.Delta); delta != "" {
				if model.Provider == ProviderIDOpenCodeGo && field == "reasoning" {
					field = "reasoning_content"
				}
				if thinkingIndex == -1 {
					thinkingIndex = len(output.Content)
					output.Content = append(output.Content, ThinkingContent{
						Type: ContentTypeThinking, Thinking: delta, ThinkingSignature: Some(field),
					})
					stream.Push(AssistantMessageThinkingStartEvent{
						Type: AssistantMessageEventTypeThinkingStart, ContentIndex: thinkingIndex, Partial: *output,
					})
				} else {
					block := output.Content[thinkingIndex].(ThinkingContent)
					block.Thinking += delta
					output.Content[thinkingIndex] = block
				}
				stream.Push(AssistantMessageThinkingDeltaEvent{
					Type: AssistantMessageEventTypeThinkingDelta, ContentIndex: thinkingIndex, Delta: delta, Partial: *output,
				})
			}
			for _, delta := range choice.Delta.ToolCalls {
				state, created := applyOpenAIToolCallDelta(stream, output, delta, toolCallsByIndex, toolCallsByID)
				if created {
					toolCalls = append(toolCalls, state)
				}
				applyOpenAIPendingReasoningDetail(output, state, pendingReasoningDetails)
			}
			applyOpenAIReasoningDetails(output, choice.Delta.ReasoningDetails, toolCallsByID, pendingReasoningDetails)
		}
		return false, nil
	})
	if ctx.Err() != nil {
		cause := context.Cause(ctx)
		output.StopReason = StopReasonAborted
		output.ErrorMessage = Some(openAIErrorMessage(cause))
		finishOpenAIContent(stream, output)
		return cause
	}
	if err != nil {
		return err
	}
	if !sawFrame {
		return errors.Join(ErrOpenAISSEProtocol, ErrOpenAISSEEmpty)
	}
	finalArguments := make([]map[string]any, len(toolCalls))
	for i, state := range toolCalls {
		arguments := map[string]any{}
		if err := json.Unmarshal([]byte(state.rawArguments), &arguments); err != nil {
			return fmt.Errorf("parse tool call %q arguments: %w", state.call.Name, err)
		}
		finalArguments[i] = arguments
	}
	for i, state := range toolCalls {
		state.call.Arguments = finalArguments[i]
		output.Content[state.contentIndex] = state.call
	}
	finishOpenAIContent(stream, output)
	if !hasFinishReason && !compat.supportsFinishReason {
		output.StopReason = StopReasonStop
		if len(toolCalls) > 0 {
			output.StopReason = StopReasonToolUse
		}
	}
	if output.StopReason == StopReasonError {
		if message, ok := output.ErrorMessage.Value(); ok {
			return errors.New(message)
		}
		return errors.New("provider returned an error stop reason")
	}
	if (compat.supportsFinishReason && !hasFinishReason) || output.StopReason == StopReasonPending {
		return errors.Join(ErrOpenAISSEProtocol, ErrOpenAISSETruncated, errors.New("stream ended without finish_reason"))
	}
	stream.Push(AssistantMessageDoneEvent{Type: AssistantMessageEventTypeDone, Reason: output.StopReason, Message: *output})
	return nil
}

func finishOpenAIContent(stream *AssistantMessageEventStream, output *AssistantMessage) {
	for index, content := range output.Content {
		switch block := content.(type) {
		case TextContent:
			stream.Push(AssistantMessageTextEndEvent{Type: AssistantMessageEventTypeTextEnd, ContentIndex: index, Content: block.Text, Partial: *output})
		case ThinkingContent:
			stream.Push(AssistantMessageThinkingEndEvent{Type: AssistantMessageEventTypeThinkingEnd, ContentIndex: index, Content: block.Thinking, Partial: *output})
		case ToolCall:
			stream.Push(AssistantMessageToolCallEndEvent{Type: AssistantMessageEventTypeToolCallEnd, ContentIndex: index, ToolCall: block, Partial: *output})
		}
	}
}

func applyOpenAIToolCallDelta(
	stream *AssistantMessageEventStream,
	output *AssistantMessage,
	delta openAIStreamingToolCallDelta,
	byIndex map[int]*openAIToolCallState,
	byID map[string]*openAIToolCallState,
) (*openAIToolCallState, bool) {
	var state *openAIToolCallState
	if delta.Index != nil {
		state = byIndex[*delta.Index]
	}
	if state == nil && delta.ID != "" {
		state = byID[delta.ID]
	}
	name := ""
	if delta.Function != nil && delta.Function.Name != nil {
		name = *delta.Function.Name
	}
	created := state == nil
	if state == nil {
		state = &openAIToolCallState{
			contentIndex: len(output.Content),
			call:         ToolCall{Type: ContentTypeToolCall, ID: delta.ID, Name: name, Arguments: map[string]any{}},
		}
		output.Content = append(output.Content, state.call)
		stream.Push(AssistantMessageToolCallStartEvent{
			Type: AssistantMessageEventTypeToolCallStart, ContentIndex: state.contentIndex, Partial: *output,
		})
	}
	if delta.Index != nil && state.streamIndex == nil {
		index := *delta.Index
		state.streamIndex = &index
		byIndex[index] = state
	}
	if delta.ID != "" {
		byID[delta.ID] = state
		if state.call.ID == "" {
			state.call.ID = delta.ID
		}
	}
	if state.call.Name == "" && name != "" {
		state.call.Name = name
	}
	fragment := ""
	if delta.Function != nil && delta.Function.Arguments != nil {
		fragment = delta.Function.Arguments.value
		if fragment != "" {
			state.rawArguments += encodeOpenAIArgumentFragment(fragment)
			state.call.Arguments = parseOpenAIPartialObject(state.rawArguments)
		}
	}
	output.Content[state.contentIndex] = state.call
	stream.Push(AssistantMessageToolCallDeltaEvent{
		Type: AssistantMessageEventTypeToolCallDelta, ContentIndex: state.contentIndex, Delta: fragment, Partial: *output,
	})
	return state, created
}

func openAIReasoningDelta(delta *openAICompletionDelta) (string, string) {
	for _, field := range []struct {
		name  string
		value *string
	}{
		{name: "reasoning_content", value: delta.ReasoningContent},
		{name: "reasoning", value: delta.Reasoning},
		{name: "reasoning_text", value: delta.ReasoningText},
	} {
		if field.value != nil && *field.value != "" {
			return field.name, *field.value
		}
	}
	return "", ""
}

func applyOpenAIReasoningDetails(
	output *AssistantMessage,
	raw json.RawMessage,
	toolCallsByID map[string]*openAIToolCallState,
	pending map[string]string,
) {
	if len(raw) == 0 || raw[0] != '[' {
		return
	}
	var details []json.RawMessage
	if json.Unmarshal(raw, &details) != nil {
		return
	}
	for _, detail := range details {
		var fields struct {
			Type string `json:"type"`
			ID   string `json:"id"`
			Data string `json:"data"`
		}
		if json.Unmarshal(detail, &fields) != nil || fields.Type != "reasoning.encrypted" || fields.ID == "" || fields.Data == "" {
			continue
		}
		var serialized bytes.Buffer
		if json.Compact(&serialized, detail) != nil {
			continue
		}
		if state := toolCallsByID[fields.ID]; state != nil {
			state.call.ThoughtSignature = Some(serialized.String())
			output.Content[state.contentIndex] = state.call
		} else {
			pending[fields.ID] = serialized.String()
		}
	}
}

func applyOpenAIPendingReasoningDetail(output *AssistantMessage, state *openAIToolCallState, pending map[string]string) {
	if state.call.ID == "" {
		return
	}
	signature, ok := pending[state.call.ID]
	if !ok {
		return
	}
	state.call.ThoughtSignature = Some(signature)
	output.Content[state.contentIndex] = state.call
	delete(pending, state.call.ID)
}

type openAIProviderError struct {
	status  *int
	headers map[string]string
	message string
	cause   error
}

func (e *openAIProviderError) Error() string { return e.message }
func (e *openAIProviderError) Unwrap() error { return e.cause }

type openAIRequestConstructionError struct{ cause error }

func (e *openAIRequestConstructionError) Error() string { return e.cause.Error() }
func (e *openAIRequestConstructionError) Unwrap() error { return e.cause }

func fetchOpenAICompletions(ctx context.Context, fetch FetchFunction, request FetchRequest, maxRetries *int, maxRetryDelayMS *int64) (FetchResponse, io.ReadCloser, error) {
	retries := 0
	if maxRetries != nil && *maxRetries > 0 {
		retries = *maxRetries
	}
	retryIndex := 0
	for {
		response, err := fetch(ctx, request)
		var providerErr *openAIProviderError
		if err != nil {
			if response.BodyReader != nil {
				_ = response.BodyReader.Close()
			}
			if ctx.Err() != nil {
				return FetchResponse{}, nil, context.Cause(ctx)
			}
			var constructionErr *openAIRequestConstructionError
			if errors.As(err, &constructionErr) {
				return FetchResponse{}, nil, constructionErr.cause
			}
			providerErr = &openAIProviderError{message: err.Error(), cause: err}
		} else {
			body := response.BodyReader
			if body == nil {
				body = io.NopCloser(bytes.NewReader(response.Body))
			}
			if response.Status >= 200 && response.Status < 300 {
				return response, body, nil
			}
			var closeOnce sync.Once
			closeBody := func() { closeOnce.Do(func() { _ = body.Close() }) }
			stopClose := context.AfterFunc(ctx, closeBody)
			detail, _ := io.ReadAll(io.LimitReader(body, 4096))
			stopClose()
			closeBody()
			if ctx.Err() != nil {
				return FetchResponse{}, nil, context.Cause(ctx)
			}
			providerErr = &openAIProviderError{
				status: &response.Status, headers: cloneOpenAIHeaders(response.Headers),
				message: fmt.Sprintf("provider response %d: %s", response.Status, strings.TrimSpace(string(detail))),
			}
		}
		if retries == 0 || !isRetryableOpenAIProviderError(providerErr) {
			return FetchResponse{}, nil, providerErr
		}
		delay, delayErr := openAIRetryDelay(providerErr, retryIndex, maxRetryDelayMS, time.Now(), rand.Float64())
		if delayErr != nil {
			return FetchResponse{}, nil, delayErr
		}
		retries--
		retryIndex++
		if err := waitOpenAIRetry(ctx, delay); err != nil {
			return FetchResponse{}, nil, err
		}
	}
}

func isRetryableOpenAIProviderError(err *openAIProviderError) bool {
	if shouldRetry, ok := openAIHeader(err.headers, "x-should-retry"); ok {
		if shouldRetry == "true" {
			return true
		}
		if shouldRetry == "false" {
			return false
		}
	}
	return err.status == nil || *err.status == http.StatusRequestTimeout || *err.status == http.StatusConflict || *err.status == http.StatusTooManyRequests || *err.status >= 500
}

func openAIHeader(headers map[string]string, name string) (string, bool) {
	for key, value := range headers {
		if equalFoldASCII(key, name) {
			return value, true
		}
	}
	return "", false
}

func openAIRetryDelay(err *openAIProviderError, retryIndex int, maxRetryDelayMS *int64, now time.Time, random float64) (time.Duration, error) {
	if value, ok := openAIHeader(err.headers, "retry-after-ms"); ok && value != "" {
		if milliseconds, parseErr := strconv.ParseFloat(strings.TrimSpace(value), 64); parseErr == nil && !math.IsNaN(milliseconds) {
			return validateOpenAIServerRetryDelay(milliseconds, maxRetryDelayMS, err.message)
		}
	}
	if value, ok := openAIHeader(err.headers, "retry-after"); ok && value != "" {
		if seconds, parseErr := strconv.ParseFloat(strings.TrimSpace(value), 64); parseErr == nil {
			return validateOpenAIServerRetryDelay(seconds*1_000, maxRetryDelayMS, err.message)
		}
		retryAt, parseErr := http.ParseTime(value)
		if parseErr != nil {
			return 0, nil
		}
		milliseconds := float64(retryAt.UnixMilli() - now.UnixMilli())
		return validateOpenAIServerRetryDelay(milliseconds, maxRetryDelayMS, err.message)
	}
	base := math.Min(500*math.Pow(2, float64(retryIndex)), 8_000)
	return time.Duration(base * (1 - random*.25) * float64(time.Millisecond)), nil
}

func validateOpenAIServerRetryDelay(milliseconds float64, maxDelayMS *int64, providerError string) (time.Duration, error) {
	maxDelay := int64(60_000)
	if maxDelayMS != nil {
		maxDelay = *maxDelayMS
	}
	if maxDelay > 0 && milliseconds > float64(maxDelay) {
		return 0, fmt.Errorf("Server requested %.0fs retry delay (max: %.0fs). %s", math.Ceil(milliseconds/1_000), math.Ceil(float64(maxDelay)/1_000), providerError)
	}
	if milliseconds <= 0 {
		return 0, nil
	}
	return time.Duration(milliseconds * float64(time.Millisecond)), nil
}

func waitOpenAIRetry(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return context.Cause(ctx)
	}
}

func consumeOpenAISSE(reader io.Reader, handle func(string) (bool, error)) error {
	buffered := bufio.NewReader(reader)
	var data []string
	dispatch := func() (bool, error) {
		if len(data) == 0 {
			return false, nil
		}
		joined := strings.Join(data, "\n")
		data = data[:0]
		return handle(joined)
	}
	var line []byte
	skipLF := false
	consumeLine := func() (bool, error) {
		value := string(line)
		line = line[:0]
		if value == "" {
			return dispatch()
		}
		if strings.HasPrefix(value, ":") {
			return false, nil
		}
		field, fieldValue, _ := strings.Cut(value, ":")
		if field == "data" {
			data = append(data, strings.TrimPrefix(fieldValue, " "))
		}
		return false, nil
	}
	for {
		value, err := buffered.ReadByte()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				return err
			}
			if len(line) > 0 {
				if done, lineErr := consumeLine(); lineErr != nil || done {
					return lineErr
				}
			}
			if len(data) > 0 {
				return errors.Join(ErrOpenAISSEProtocol, ErrOpenAISSETruncated)
			}
			return nil
		}
		if skipLF {
			skipLF = false
			if value == '\n' {
				continue
			}
		}
		if value == '\r' || value == '\n' {
			done, lineErr := consumeLine()
			if lineErr != nil || done {
				return lineErr
			}
			skipLF = value == '\r'
			continue
		}
		line = append(line, value)
	}
}

func defaultOpenAIFetch(ctx context.Context, request FetchRequest) (FetchResponse, error) {
	httpRequest, err := http.NewRequestWithContext(ctx, request.Method, request.URL, bytes.NewReader(request.Body))
	if err != nil {
		return FetchResponse{}, &openAIRequestConstructionError{cause: err}
	}
	for name, value := range request.Headers {
		httpRequest.Header[name] = []string{value}
	}
	response, err := http.DefaultClient.Do(httpRequest)
	if err != nil {
		return FetchResponse{}, err
	}
	headers := make(map[string]string, len(response.Header))
	for name, values := range response.Header {
		headers[name] = strings.Join(values, ", ")
	}
	return FetchResponse{Status: response.StatusCode, Headers: headers, BodyReader: response.Body}, nil
}

func openAIRequestHeaders(modelHeaders map[string]string, overrides ProviderHeaders, apiKey string) map[string]string {
	headers := map[string]string{
		"Accept": "application/json", "Content-Type": "application/json", "Authorization": "Bearer " + apiKey,
	}
	for _, name := range sortedStringKeys(modelHeaders) {
		value := modelHeaders[name]
		setOpenAIHeader(headers, name, &value)
	}
	for _, name := range sortedStringKeys(overrides) {
		value := overrides[name]
		setOpenAIHeader(headers, name, value)
	}
	return headers
}

func sortedStringKeys[T any](values map[string]T) []string {
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func setOpenAIHeader(headers map[string]string, name string, value *string) {
	for existing := range headers {
		if equalFoldASCII(existing, name) {
			delete(headers, existing)
		}
	}
	if value != nil {
		headers[name] = *value
	}
}

func openAIClientAPIKey(provider ProviderID, apiKey *string, headers ProviderHeaders) (string, error) {
	if apiKey != nil && *apiKey != "" {
		return *apiKey, nil
	}
	for name, value := range headers {
		if (equalFoldASCII(name, "authorization") || equalFoldASCII(name, "cf-aig-authorization")) && value != nil && strings.TrimSpace(*value) != "" {
			return "unused", nil
		}
	}
	return "", fmt.Errorf("no API key for provider: %s", provider)
}

func mapOpenAIUsage(raw openAICompletionUsage, model Model) Usage {
	cacheRead := raw.PromptCacheHitTokens
	cacheWrite := int64(0)
	if raw.PromptTokensDetails != nil {
		if raw.PromptTokensDetails.CachedTokens != nil {
			cacheRead = *raw.PromptTokensDetails.CachedTokens
		}
		cacheWrite = raw.PromptTokensDetails.CacheWriteTokens
	}
	input := raw.PromptTokens - cacheRead - cacheWrite
	if input < 0 {
		input = 0
	}
	usage := Usage{
		Input: input, Output: raw.CompletionTokens, CacheRead: cacheRead, CacheWrite: cacheWrite,
		TotalTokens: input + raw.CompletionTokens + cacheRead + cacheWrite,
	}
	if raw.CompletionTokensDetails != nil && raw.CompletionTokensDetails.ReasoningTokens != nil {
		usage.Reasoning = Some(*raw.CompletionTokensDetails.ReasoningTokens)
	}
	CalculateCost(model, &usage)
	return usage
}

func mapOpenAIStopReason(reason string) (StopReason, error) {
	switch reason {
	case "stop", "end":
		return StopReasonStop, nil
	case "length":
		return StopReasonLength, nil
	case "function_call", "tool_calls":
		return StopReasonToolUse, nil
	default:
		return StopReasonError, fmt.Errorf("Provider finish_reason: %s", reason)
	}
}

func resolveOpenAICompletionsCompat(model Model) (openAICompletionsCompat, error) {
	compat := OpenAICompletionsCompat{}
	if model.Compat.IsNull() {
		return openAICompletionsCompat{}, fmt.Errorf("OpenAI Completions compat cannot be null")
	}
	if raw, ok := model.Compat.Value(); ok {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil {
			return openAICompletionsCompat{}, err
		}
		supported := make(map[string]json.RawMessage, 7)
		for _, name := range []string{
			"supportsReasoningEffort", "supportsUsageInStreaming", "supportsFinishReason",
			"requiresThinkingAsText", "requiresReasoningContentOnAssistantMessages", "thinkingFormat",
			"supportsThinkingTokenBudget",
		} {
			if value, present := fields[name]; present {
				supported[name] = value
				delete(fields, name)
			}
		}
		if model.Provider == ProviderIDDeepSeek {
			for _, field := range []struct {
				name string
				want string
			}{
				{name: "supportsStore", want: "false"},
				{name: "supportsDeveloperRole", want: "false"},
				{name: "requiresReasoningContentOnAssistantMessages", want: "true"},
				{name: "thinkingFormat", want: `"deepseek"`},
			} {
				if value, present := fields[field.name]; present {
					if string(bytes.TrimSpace(value)) != field.want {
						return openAICompletionsCompat{}, newNotImplemented("OpenAICompletions.Compat." + field.name)
					}
					delete(fields, field.name)
				}
			}
		}
		if names := sortedStringKeys(fields); len(names) > 0 {
			return openAICompletionsCompat{}, newNotImplemented("OpenAICompletions.Compat." + names[0])
		}
		raw, err := json.Marshal(supported)
		if err != nil {
			return openAICompletionsCompat{}, err
		}
		if err := json.Unmarshal(raw, &compat); err != nil {
			return openAICompletionsCompat{}, err
		}
	}
	if err := validateOpenAICompletionsCompat(compat); err != nil {
		return openAICompletionsCompat{}, err
	}
	resolved := detectOpenAICompletionsCompat(model)
	if err := applyOpenAICompatBool(&resolved.supportsReasoningEffort, compat.SupportsReasoningEffort, "supportsReasoningEffort"); err != nil {
		return openAICompletionsCompat{}, err
	}
	if value, ok := compat.SupportsUsageInStreaming.Value(); ok {
		resolved.supportsUsageInStreaming = value
	} else if compat.SupportsUsageInStreaming.IsNull() {
		return openAICompletionsCompat{}, fmt.Errorf("supportsUsageInStreaming cannot be null")
	}
	if value, ok := compat.SupportsFinishReason.Value(); ok {
		resolved.supportsFinishReason = value
	} else if compat.SupportsFinishReason.IsNull() {
		return openAICompletionsCompat{}, fmt.Errorf("supportsFinishReason cannot be null")
	}
	if err := applyOpenAICompatBool(&resolved.requiresThinkingAsText, compat.RequiresThinkingAsText, "requiresThinkingAsText"); err != nil {
		return openAICompletionsCompat{}, err
	}
	if err := applyOpenAICompatBool(&resolved.requiresReasoningContentOnAssistantMessages, compat.RequiresReasoningContentOnAssistantMessages, "requiresReasoningContentOnAssistantMessages"); err != nil {
		return openAICompletionsCompat{}, err
	}
	if value, ok := compat.ThinkingFormat.Value(); ok {
		resolved.thinkingFormat = value
	} else if compat.ThinkingFormat.IsNull() {
		return openAICompletionsCompat{}, fmt.Errorf("thinkingFormat cannot be null")
	}
	if err := applyOpenAICompatBool(&resolved.supportsThinkingTokenBudget, compat.SupportsThinkingTokenBudget, "supportsThinkingTokenBudget"); err != nil {
		return openAICompletionsCompat{}, err
	}
	return resolved, nil
}

func applyOpenAICompatBool(target *bool, value Optional[bool], name string) error {
	if resolved, ok := value.Value(); ok {
		*target = resolved
	} else if value.IsNull() {
		return fmt.Errorf("%s cannot be null", name)
	}
	return nil
}

func detectOpenAICompletionsCompat(model Model) openAICompletionsCompat {
	baseURL := model.BaseURL
	isZAI := model.Provider == ProviderIDZAI || model.Provider == ProviderIDZAICodingCN ||
		strings.Contains(baseURL, "api.z.ai") || strings.Contains(baseURL, "open.bigmodel.cn")
	isTogether := model.Provider == ProviderIDTogether || strings.Contains(baseURL, "api.together.ai") || strings.Contains(baseURL, "api.together.xyz")
	isMoonshot := model.Provider == ProviderIDMoonshotAI || model.Provider == ProviderIDMoonshotAICN || strings.Contains(baseURL, "api.moonshot.")
	isOpenRouter := model.Provider == ProviderIDOpenRouter || strings.Contains(baseURL, "openrouter.ai")
	isCloudflareGateway := model.Provider == ProviderIDCloudflareAIGateway || strings.Contains(baseURL, "gateway.ai.cloudflare.com")
	isNVIDIA := model.Provider == ProviderIDNVIDIA || strings.Contains(baseURL, "integrate.api.nvidia.com")
	isAntLing := model.Provider == ProviderIDAntLing || strings.Contains(baseURL, "api.ant-ling.com")
	isGrok := model.Provider == ProviderIDXAI || strings.Contains(baseURL, "api.x.ai")
	isDeepSeek := model.Provider == ProviderIDDeepSeek || strings.Contains(baseURL, "deepseek.com")
	format := ThinkingFormat("openai")
	switch {
	case isDeepSeek:
		format = "deepseek"
	case isZAI:
		format = "zai"
	case isTogether:
		format = "together"
	case isAntLing:
		format = "ant-ling"
	case isOpenRouter:
		format = "openrouter"
	}
	return openAICompletionsCompat{
		supportsReasoningEffort:                     !isGrok && !isZAI && !isMoonshot && !isTogether && !isCloudflareGateway && !isNVIDIA && !isAntLing,
		supportsUsageInStreaming:                    true,
		supportsFinishReason:                        true,
		requiresReasoningContentOnAssistantMessages: isDeepSeek,
		thinkingFormat:                              format,
	}
}

func openAICompatPublic(compat openAICompletionsCompat) OpenAICompletionsCompat {
	return OpenAICompletionsCompat{
		SupportsReasoningEffort:                     Some(compat.supportsReasoningEffort),
		SupportsUsageInStreaming:                    Some(compat.supportsUsageInStreaming),
		SupportsFinishReason:                        Some(compat.supportsFinishReason),
		RequiresThinkingAsText:                      Some(compat.requiresThinkingAsText),
		RequiresReasoningContentOnAssistantMessages: Some(compat.requiresReasoningContentOnAssistantMessages),
		ThinkingFormat:                              Some(compat.thinkingFormat),
		SupportsThinkingTokenBudget:                 Some(compat.supportsThinkingTokenBudget),
	}
}

func validateOpenAICompletionsCompat(compat OpenAICompletionsCompat) error {
	if compat.ChatTemplateKwargs != nil || compat.ChatTemplateArgs != nil {
		return newNotImplemented("OpenAICompletions.Compat.ChatTemplate")
	}
	if format, ok := compat.ThinkingFormat.Value(); ok {
		switch format {
		case "chat-template", "qwen-chat-template", "baseten":
			return newNotImplemented("OpenAICompletions.Compat.ThinkingFormat." + string(format))
		}
	}
	raw, err := json.Marshal(compat)
	if err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return err
	}
	delete(fields, "supportsUsageInStreaming")
	delete(fields, "supportsFinishReason")
	delete(fields, "supportsReasoningEffort")
	delete(fields, "requiresThinkingAsText")
	delete(fields, "requiresReasoningContentOnAssistantMessages")
	delete(fields, "thinkingFormat")
	delete(fields, "supportsThinkingTokenBudget")
	if names := sortedStringKeys(fields); len(names) > 0 {
		return newNotImplemented("OpenAICompletions.Compat." + names[0])
	}
	return nil
}

func validateOpenAICompletions(model Model, input Context, options OpenAICompletionsOptions) error {
	if model.API != APIOpenAICompletions {
		return fmt.Errorf("%w: model API %q is not %q", ErrEventStreamInvariant, model.API, APIOpenAICompletions)
	}
	if model.SamplingParams != nil || options.SamplingParams != nil || options.CacheRetention != nil || options.SessionID != nil || options.Env != nil {
		return newNotImplemented("OpenAICompletions.AdvancedOptions")
	}
	_, err := resolveOpenAICompletionsCompat(model)
	return err
}

func clampOpenAIMaxTokens(model Model, input Context, maxTokens int64) int64 {
	if maxTokens < 1 {
		maxTokens = 1
	}
	if model.ContextWindow <= 0 {
		return maxTokens
	}
	available := model.ContextWindow - estimateOpenAITextTokens(input) - 4096
	if available < 1 {
		available = 1
	}
	if maxTokens > available {
		return available
	}
	return maxTokens
}

func estimateOpenAITextTokens(input Context) int64 {
	usageIndex, usageTokens := lastOpenAIAssistantUsage(input.Messages)
	start := 0
	if usageIndex >= 0 {
		start = usageIndex + 1
	}
	tokens := usageTokens
	for _, message := range input.Messages[start:] {
		tokens += estimateOpenAIMessageTokens(message)
	}
	if usageIndex < 0 {
		if system, ok := input.SystemPrompt.Value(); ok {
			tokens += estimateOpenAICharacters(jsStringLength(system))
		}
	}
	return tokens
}

func lastOpenAIAssistantUsage(messages []Message) (int, int64) {
	latestTimestamp := int64(-1 << 63)
	index, tokens := -1, int64(0)
	for i, message := range messages {
		var assistant *AssistantMessage
		switch value := message.(type) {
		case AssistantMessage:
			assistant = &value
		case *AssistantMessage:
			assistant = value
		}
		if assistant != nil && assistant.Timestamp >= latestTimestamp && assistant.StopReason != StopReasonAborted && assistant.StopReason != StopReasonError {
			usageTokens := assistant.Usage.TotalTokens
			if usageTokens == 0 {
				usageTokens = assistant.Usage.Input + assistant.Usage.Output + assistant.Usage.CacheRead + assistant.Usage.CacheWrite
			}
			if usageTokens > 0 {
				index, tokens = i, usageTokens
			}
		}
		switch value := message.(type) {
		case UserMessage:
			latestTimestamp = max(latestTimestamp, value.Timestamp)
		case *UserMessage:
			if value != nil {
				latestTimestamp = max(latestTimestamp, value.Timestamp)
			}
		case AssistantMessage:
			latestTimestamp = max(latestTimestamp, value.Timestamp)
		case *AssistantMessage:
			if value != nil {
				latestTimestamp = max(latestTimestamp, value.Timestamp)
			}
		case ToolResultMessage:
			latestTimestamp = max(latestTimestamp, value.Timestamp)
		case *ToolResultMessage:
			if value != nil {
				latestTimestamp = max(latestTimestamp, value.Timestamp)
			}
		}
	}
	return index, tokens
}

func estimateOpenAIMessageTokens(message Message) int64 {
	switch value := message.(type) {
	case UserMessage:
		return estimateOpenAICharacters(userMessageCharacters(value))
	case *UserMessage:
		if value != nil {
			return estimateOpenAICharacters(userMessageCharacters(*value))
		}
	case AssistantMessage:
		return estimateOpenAICharacters(assistantMessageCharacters(value))
	case *AssistantMessage:
		if value != nil {
			return estimateOpenAICharacters(assistantMessageCharacters(*value))
		}
	}
	return 0
}

func estimateOpenAICharacters(characters int) int64 {
	return int64((characters + 3) / 4)
}

func userMessageCharacters(message UserMessage) int {
	if text, ok := message.Content.Text(); ok {
		return jsStringLength(text)
	}
	blocks, _ := message.Content.Blocks()
	total := 0
	for _, block := range blocks {
		if text, ok := block.(TextContent); ok {
			total += jsStringLength(text.Text)
		} else if text, ok := block.(*TextContent); ok && text != nil {
			total += jsStringLength(text.Text)
		}
	}
	return total
}

func assistantMessageCharacters(message AssistantMessage) int {
	total := 0
	for _, block := range message.Content {
		if text, ok := block.(TextContent); ok {
			total += jsStringLength(text.Text)
		} else if text, ok := block.(*TextContent); ok && text != nil {
			total += jsStringLength(text.Text)
		}
	}
	return total
}

func jsStringLength(value string) int {
	return len(utf16.Encode([]rune(value)))
}

func sanitizeOpenAIText(text string) string {
	return strings.ToValidUTF8(text, "")
}

func cloneOpenAIHeaders(values map[string]string) map[string]string {
	if values == nil {
		return map[string]string{}
	}
	clone := make(map[string]string, len(values))
	for name, value := range values {
		clone[name] = value
	}
	return clone
}

func openAIErrorMessage(err error) string {
	if errors.Is(err, context.Canceled) {
		return "Request was aborted"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "Request timed out"
	}
	return err.Error()
}

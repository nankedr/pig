package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
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
	supportsUsageInStreaming bool
	supportsFinishReason     bool
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
	Content *string `json:"content"`
}

type openAICompletionUsage struct {
	PromptTokens         int64 `json:"prompt_tokens"`
	CompletionTokens     int64 `json:"completion_tokens"`
	PromptCacheHitTokens int64 `json:"prompt_cache_hit_tokens"`
	PromptTokensDetails  *struct {
		CachedTokens     *int64 `json:"cached_tokens"`
		CacheWriteTokens int64  `json:"cache_write_tokens"`
	} `json:"prompt_tokens_details"`
}

func StreamOpenAICompletions(ctx context.Context, model Model, input Context, options OpenAICompletionsOptions) *AssistantMessageEventStream {
	ctx = nonNilContext(ctx)
	if err := validateOpenAICompletionsM1(model, input, options); err != nil {
		return failedProviderStream(err)
	}
	stream := NewAssistantMessageEventStream()
	go runOpenAICompletions(ctx, stream, model, input, options)
	return stream
}

func StreamSimpleOpenAICompletions(ctx context.Context, model Model, input Context, options SimpleStreamOptions) *AssistantMessageEventStream {
	if options.Reasoning != nil || options.ThinkingBudgets != nil {
		return failedProviderStream(newNotImplemented("OpenAICompletions.StreamSimple.Reasoning"))
	}
	streamOptions := options.StreamOptions
	maxTokens := model.MaxTokens
	if streamOptions.MaxTokens != nil {
		maxTokens = *streamOptions.MaxTokens
	}
	maxTokens = clampOpenAIMaxTokens(model, input, maxTokens)
	streamOptions.MaxTokens = &maxTokens
	return StreamOpenAICompletions(ctx, model, input, OpenAICompletionsOptions{StreamOptions: streamOptions})
}

func ConvertOpenAICompletionsMessages(model Model, input Context, compat OpenAICompletionsCompat, _ ...ConvertOpenAICompletionsMessagesOptions) ([]json.RawMessage, error) {
	if model.Reasoning {
		return nil, newNotImplemented("OpenAICompletions.Reasoning")
	}
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
			if err := appendOpenAIAssistantMessage(&messages, value); err != nil {
				return nil, err
			}
		case *AssistantMessage:
			if value == nil {
				return nil, fmt.Errorf("nil assistant message")
			}
			if err := appendOpenAIAssistantMessage(&messages, *value); err != nil {
				return nil, err
			}
		case ToolResultMessage, *ToolResultMessage:
			return nil, newNotImplemented("OpenAICompletions.ConvertMessages.ToolResult")
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

func appendOpenAIAssistantMessage(messages *[]json.RawMessage, message AssistantMessage) error {
	if message.StopReason == StopReasonError || message.StopReason == StopReasonAborted {
		return nil
	}
	var text strings.Builder
	for _, block := range message.Content {
		switch value := block.(type) {
		case TextContent:
			if strings.TrimSpace(value.Text) != "" {
				text.WriteString(sanitizeOpenAIText(value.Text))
			}
		case *TextContent:
			if value != nil && strings.TrimSpace(value.Text) != "" {
				text.WriteString(sanitizeOpenAIText(value.Text))
			}
		case ThinkingContent, *ThinkingContent:
			return newNotImplemented("OpenAICompletions.ConvertMessages.Thinking")
		case ToolCall, *ToolCall:
			return newNotImplemented("OpenAICompletions.ConvertMessages.ToolCall")
		default:
			return fmt.Errorf("unsupported assistant content %T", block)
		}
	}
	if text.Len() == 0 {
		return nil
	}
	raw, err := json.Marshal(map[string]any{"role": "assistant", "content": text.String()})
	if err == nil {
		*messages = append(*messages, raw)
	}
	return err
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
	response, err := fetch(ctx, request)
	if err != nil {
		return err
	}
	body := response.BodyReader
	if body == nil {
		body = io.NopCloser(bytes.NewReader(response.Body))
	}
	var closeOnce sync.Once
	closeBody := func() { closeOnce.Do(func() { _ = body.Close() }) }
	defer closeBody()
	stopClose := context.AfterFunc(ctx, closeBody)
	defer stopClose()
	if response.Status < 200 || response.Status >= 300 {
		detail, _ := io.ReadAll(io.LimitReader(body, 4096))
		return fmt.Errorf("provider response %d: %s", response.Status, strings.TrimSpace(string(detail)))
	}
	if options.OnResponse != nil {
		if err := options.OnResponse(ctx, ProviderResponse{Status: response.Status, Headers: cloneOpenAIHeaders(response.Headers)}, model); err != nil {
			return err
		}
	}
	stream.Push(AssistantMessageStartEvent{Type: AssistantMessageEventTypeStart, Partial: *output})
	hasFinishReason := false
	sawFrame := false
	textIndex := -1
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
		return false, nil
	})
	if ctx.Err() != nil {
		cause := context.Cause(ctx)
		output.StopReason = StopReasonAborted
		output.ErrorMessage = Some(openAIErrorMessage(cause))
		if textIndex != -1 {
			text := output.Content[textIndex].(TextContent).Text
			stream.Push(AssistantMessageTextEndEvent{Type: AssistantMessageEventTypeTextEnd, ContentIndex: textIndex, Content: text, Partial: *output})
		}
		return cause
	}
	if err != nil {
		return err
	}
	if !sawFrame {
		return errors.Join(ErrOpenAISSEProtocol, ErrOpenAISSEEmpty)
	}
	if textIndex != -1 {
		text := output.Content[textIndex].(TextContent).Text
		stream.Push(AssistantMessageTextEndEvent{Type: AssistantMessageEventTypeTextEnd, ContentIndex: textIndex, Content: text, Partial: *output})
	}
	if !hasFinishReason && !compat.supportsFinishReason {
		output.StopReason = StopReasonStop
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
		return FetchResponse{}, err
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
		delete(fields, "supportsUsageInStreaming")
		delete(fields, "supportsFinishReason")
		if names := sortedStringKeys(fields); len(names) > 0 {
			return openAICompletionsCompat{}, newNotImplemented("OpenAICompletions.Compat." + names[0])
		}
		if err := json.Unmarshal(raw, &compat); err != nil {
			return openAICompletionsCompat{}, err
		}
	}
	if err := validateOpenAICompletionsCompat(compat); err != nil {
		return openAICompletionsCompat{}, err
	}
	resolved := openAICompletionsCompat{supportsUsageInStreaming: true, supportsFinishReason: true}
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
	return resolved, nil
}

func openAICompatPublic(compat openAICompletionsCompat) OpenAICompletionsCompat {
	return OpenAICompletionsCompat{
		SupportsUsageInStreaming: Some(compat.supportsUsageInStreaming),
		SupportsFinishReason:     Some(compat.supportsFinishReason),
	}
}

func validateOpenAICompletionsCompat(compat OpenAICompletionsCompat) error {
	if compat.ChatTemplateKwargs != nil || compat.ChatTemplateArgs != nil {
		return newNotImplemented("OpenAICompletions.Compat.ChatTemplate")
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
	if names := sortedStringKeys(fields); len(names) > 0 {
		return newNotImplemented("OpenAICompletions.Compat." + names[0])
	}
	return nil
}

func validateOpenAICompletionsM1(model Model, input Context, options OpenAICompletionsOptions) error {
	if model.API != APIOpenAICompletions {
		return fmt.Errorf("%w: model API %q is not %q", ErrEventStreamInvariant, model.API, APIOpenAICompletions)
	}
	if model.Reasoning || options.ReasoningEffort != nil || options.ThinkingBudgets != nil {
		return newNotImplemented("OpenAICompletions.Reasoning")
	}
	if !isNilRuntimeValue(options.ToolChoice) {
		return newNotImplemented("OpenAICompletions.ToolChoice")
	}
	if model.SamplingParams != nil || options.SamplingParams != nil || options.CacheRetention != nil || options.SessionID != nil || options.Env != nil {
		return newNotImplemented("OpenAICompletions.AdvancedOptions")
	}
	if (options.MaxRetries != nil && *options.MaxRetries > 0) || options.MaxRetryDelayMS != nil {
		return newNotImplemented("OpenAICompletions.TransportRetry")
	}
	if len(input.Tools) > 0 {
		return newNotImplemented("OpenAICompletions.Tools")
	}
	if openAIRequiresProviderCompat(model) {
		return newNotImplemented("OpenAICompletions.Compat.ProviderDetection")
	}
	_, err := resolveOpenAICompletionsCompat(model)
	return err
}

func openAIRequiresProviderCompat(model Model) bool {
	baseURL := model.BaseURL
	switch model.Provider {
	case ProviderIDAntLing, ProviderIDCerebras, ProviderIDCloudflareAIGateway, ProviderIDCloudflareWorkersAI,
		ProviderIDDeepSeek, ProviderIDMoonshotAI, ProviderIDMoonshotAICN, ProviderIDNVIDIA, ProviderIDOpenCode,
		ProviderIDOpenRouter, ProviderIDTogether, ProviderIDXAI, ProviderIDZAI, ProviderIDZAICodingCN:
		return true
	}
	for _, fragment := range []string{
		"api.ant-ling.com", "api.cloudflare.com", "api.moonshot.", "api.together.", "api.x.ai", "api.z.ai",
		"cerebras.ai", "chutes.ai", "deepseek.com", "gateway.ai.cloudflare.com", "integrate.api.nvidia.com",
		"opencode.ai", "open.bigmodel.cn", "openrouter.ai",
	} {
		if strings.Contains(baseURL, fragment) {
			return true
		}
	}
	return false
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

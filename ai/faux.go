package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

const (
	defaultFauxProvider     ProviderID = "faux"
	defaultFauxModelID                 = "faux-1"
	defaultFauxModelName               = "Faux Model"
	defaultFauxBaseURL                 = "http://localhost:0"
	defaultFauxMinTokenSize            = 3
	defaultFauxMaxTokenSize            = 5
)

var fauxID atomic.Uint64

type fauxRuntime struct {
	mu              sync.Mutex
	responses       []FauxResponseStep
	state           *FauxProviderState
	api             API
	provider        ProviderID
	minTokenSize    int
	tokensPerSecond *float64
}

func newFauxToolCall(name string, arguments map[string]any, options ...FauxToolCallOptions) (ToolCall, error) {
	id := ""
	configured := false
	if len(options) != 0 {
		if options[0].ID.IsNull() {
			return ToolCall{}, fmt.Errorf("Faux ToolCall id cannot be null")
		}
		id, configured = options[0].ID.Value()
	}
	if !configured {
		id = nextFauxID("tool")
	}
	return ToolCall{Type: ContentTypeToolCall, ID: id, Name: name, Arguments: cloneStringAnyMap(arguments)}, nil
}

func newFauxAssistantMessage(content FauxAssistantMessageContent, options ...FauxAssistantMessageOptions) (AssistantMessage, error) {
	configured := FauxAssistantMessageOptions{}
	if len(options) != 0 {
		configured = options[0]
	}
	if configured.Deferred.IsSet() {
		return AssistantMessage{}, newNotImplemented("Faux.AssistantMessage.Deferred")
	}
	stopReason := StopReasonStop
	if configured.StopReason.IsNull() {
		return AssistantMessage{}, fmt.Errorf("Faux AssistantMessage stop reason cannot be null")
	}
	if value, ok := configured.StopReason.Value(); ok {
		stopReason = value
	}
	if stopReason == StopReasonDeferred {
		return AssistantMessage{}, newNotImplemented("Faux.AssistantMessage.Deferred")
	}
	if stopReason != StopReasonPending && !validDoneReason(stopReason) && !validErrorReason(stopReason) {
		return AssistantMessage{}, fmt.Errorf("invalid Faux stop reason %q", stopReason)
	}

	blocks := content.blocks
	if content.isText {
		blocks = []FauxContentBlock{FauxText(content.text)}
	}
	cloned := make([]AssistantContent, len(blocks))
	for i, block := range blocks {
		if block == nil {
			return AssistantMessage{}, fmt.Errorf("Faux AssistantMessage content %d is nil", i)
		}
		cloned[i] = cloneAssistantContent(block)
	}
	timestamp := time.Now().UnixMilli()
	if configured.Timestamp.IsNull() {
		return AssistantMessage{}, fmt.Errorf("Faux AssistantMessage timestamp cannot be null")
	}
	if value, ok := configured.Timestamp.Value(); ok {
		timestamp = value
	}
	message := AssistantMessage{
		Role:       MessageRoleAssistant,
		Content:    cloned,
		API:        "faux",
		Provider:   defaultFauxProvider,
		Model:      defaultFauxModelID,
		StopReason: stopReason,
		Timestamp:  timestamp,
	}
	message.ErrorMessage = configured.ErrorMessage
	message.ResponseID = configured.ResponseID
	return message, nil
}

func createFauxCore(options RegisterFauxProviderOptions) (*FauxCore, error) {
	if options.Deferred != nil {
		return nil, newNotImplemented("Faux.Deferred")
	}
	api := options.API
	if api == "" {
		api = API(nextFauxID("faux"))
	}
	provider := options.Provider
	if provider == "" {
		provider = defaultFauxProvider
	}
	models, err := makeFauxModels(api, provider, options.Models)
	if err != nil {
		return nil, err
	}
	minTokenSize := normalizeFauxTokenSize(options.TokenSize)
	var tokensPerSecond *float64
	if options.TokensPerSecond != nil {
		value := *options.TokensPerSecond
		tokensPerSecond = &value
	}
	runtime := &fauxRuntime{
		state:           &FauxProviderState{},
		api:             api,
		provider:        provider,
		minTokenSize:    minTokenSize,
		tokensPerSecond: tokensPerSecond,
	}
	start := func(ctx context.Context, model Model, input Context, options *SimpleStreamOptions) *AssistantMessageEventStream {
		if err := unsupportedFauxOptions(options); err != nil {
			return failedProviderStream(err)
		}
		stream := NewAssistantMessageEventStream()
		step, state := runtime.takeResponse()
		go runtime.run(ctx, stream, step, state, model, input, options)
		return stream
	}
	getModel := func(ids ...string) (Model, bool) {
		if len(ids) == 0 {
			return cloneModel(models[0]), true
		}
		for _, model := range models {
			if model.ID == ids[0] {
				return cloneModel(model), true
			}
		}
		return Model{}, false
	}
	return &FauxCore{
		API:      api,
		Provider: provider,
		Models:   cloneModels(models),
		Stream: func(ctx context.Context, model Model, input Context, options StreamOptions) *AssistantMessageEventStream {
			simple := SimpleStreamOptions{StreamOptions: options}
			return start(ctx, model, input, &simple)
		},
		StreamSimple: func(ctx context.Context, model Model, input Context, options SimpleStreamOptions) *AssistantMessageEventStream {
			return start(ctx, model, input, &options)
		},
		FetchDeferred: func(context.Context, Model, DeferredHandle, DeferredFetchOptions) (*AssistantMessageEventStream, error) {
			return nil, newNotImplemented("Faux.FetchDeferred")
		},
		CancelDeferred: func(context.Context, Model, DeferredHandle, DeferredCancelOptions) error {
			return newNotImplemented("Faux.CancelDeferred")
		},
		GetModel: getModel,
		State:    runtime.state,
		SetResponses: func(responses []FauxResponseStep) {
			runtime.setResponses(responses)
		},
		AppendResponses: func(responses []FauxResponseStep) {
			runtime.appendResponses(responses)
		},
		GetPendingResponseCount: runtime.pendingResponseCount,
	}, nil
}

func newFauxProvider(options ...RegisterFauxProviderOptions) (*FauxProviderHandle, error) {
	configured := RegisterFauxProviderOptions{}
	if len(options) != 0 {
		configured = options[0]
	}
	core, err := createFauxCore(configured)
	if err != nil {
		return nil, err
	}
	auth := APIKeyAuth{
		Name: "Faux",
		Check: func(context.Context, APIKeyCheckInput) (Optional[AuthCheck], error) {
			return Some(AuthCheck{Source: Some("Faux"), Type: AuthTypeAPIKey}), nil
		},
		Resolve: func(ctx context.Context, _ APIKeyResolveInput) (Optional[AuthResult], error) {
			if err := ctx.Err(); err != nil {
				return Absent[AuthResult](), err
			}
			return Some(AuthResult{Source: Some("Faux")}), nil
		},
	}
	provider := CreateProvider(CreateProviderOptions{
		ID:     core.Provider,
		Name:   "Faux",
		Auth:   ProviderAuth{APIKey: &auth},
		Models: core.Models,
		API: SingleProviderAPI(ProviderStreams{
			Stream:       core.Stream,
			StreamSimple: core.StreamSimple,
		}),
	})
	return &FauxProviderHandle{
		Provider:                provider,
		API:                     core.API,
		Models:                  cloneModels(core.Models),
		State:                   core.State,
		GetModel:                core.GetModel,
		SetResponses:            core.SetResponses,
		AppendResponses:         core.AppendResponses,
		GetPendingResponseCount: core.GetPendingResponseCount,
	}, nil
}

func registerFauxProvider(options ...RegisterFauxProviderOptions) (*FauxProviderRegistration, error) {
	configured := RegisterFauxProviderOptions{}
	if len(options) != 0 {
		configured = options[0]
	}
	core, err := createFauxCore(configured)
	if err != nil {
		return nil, err
	}
	sourceID := nextFauxID("faux-provider")
	err = RegisterAPIProvider(APIProvider{
		API: core.API,
		Stream: func(ctx context.Context, model Model, input Context, options ProviderStreamOptions) *AssistantMessageEventStream {
			if isNilRuntimeValue(options) {
				return failedProviderStream(fmt.Errorf("%w: Faux stream options must not be nil", ErrEventStreamInvariant))
			}
			return core.Stream(ctx, model, input, options.streamOptions())
		},
		StreamSimple: func(ctx context.Context, model Model, input Context, options SimpleStreamOptions) *AssistantMessageEventStream {
			return core.StreamSimple(ctx, model, input, options)
		},
	}, sourceID)
	if err != nil {
		return nil, err
	}
	var once sync.Once
	return &FauxProviderRegistration{
		API:                     core.API,
		Models:                  cloneModels(core.Models),
		State:                   core.State,
		GetModel:                core.GetModel,
		SetResponses:            core.SetResponses,
		AppendResponses:         core.AppendResponses,
		GetPendingResponseCount: core.GetPendingResponseCount,
		Unregister: func() {
			once.Do(func() { UnregisterAPIProviders(sourceID) })
		},
	}, nil
}

func (r *fauxRuntime) takeResponse() (FauxResponseStep, *FauxProviderState) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.state.CallCount++
	state := cloneFauxProviderState(r.state)
	if len(r.responses) == 0 {
		return nil, state
	}
	step := r.responses[0]
	r.responses[0] = nil
	r.responses = r.responses[1:]
	return cloneFauxResponseStep(step), state
}

func (r *fauxRuntime) setResponses(responses []FauxResponseStep) {
	r.mu.Lock()
	r.responses = cloneFauxResponseSteps(responses)
	r.mu.Unlock()
}

func (r *fauxRuntime) appendResponses(responses []FauxResponseStep) {
	r.mu.Lock()
	r.responses = append(r.responses, cloneFauxResponseSteps(responses)...)
	r.mu.Unlock()
}

func (r *fauxRuntime) pendingResponseCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.responses)
}

func (r *fauxRuntime) run(
	ctx context.Context,
	stream *AssistantMessageEventStream,
	step FauxResponseStep,
	state *FauxProviderState,
	model Model,
	input Context,
	options *SimpleStreamOptions,
) {
	ctx = nonNilContext(ctx)
	if options != nil && options.OnResponse != nil {
		if err := options.OnResponse(ctx, ProviderResponse{Status: 200, Headers: map[string]string{}}, model); err != nil {
			r.pushError(stream, model, err)
			return
		}
	}
	if step == nil {
		r.pushError(stream, model, fmt.Errorf("No more faux responses queued"))
		return
	}
	message, err := resolveFauxResponse(step, input, options, state, model)
	if err != nil {
		r.pushError(stream, model, err)
		return
	}
	message = CloneAssistantMessage(message)
	message.Role = MessageRoleAssistant
	message.API = r.api
	message.Provider = r.provider
	message.Model = model.ID
	if err := validateFauxM1Message(message); err != nil {
		stream.stream.endWithError(err)
		return
	}
	r.streamMessage(ctx, stream, message)
}

func cloneFauxProviderState(state *FauxProviderState) *FauxProviderState {
	clone := &FauxProviderState{
		CallCount:          state.CallCount,
		DeferredFetchCount: state.DeferredFetchCount,
		CancelledDeferred:  append([]DeferredHandle(nil), state.CancelledDeferred...),
	}
	for i := range clone.CancelledDeferred {
		clone.CancelledDeferred[i].Data = cloneOptional(clone.CancelledDeferred[i].Data, cloneJSONValue)
	}
	return clone
}

func (r *fauxRuntime) streamMessage(ctx context.Context, stream *AssistantMessageEventStream, message AssistantMessage) {
	partial := CloneAssistantMessage(message)
	partial.Content = []AssistantContent{}
	partial.StopReason = StopReasonPending
	if ctx.Err() != nil {
		r.pushAborted(stream, partial)
		return
	}
	stream.Push(AssistantMessageStartEvent{Type: AssistantMessageEventTypeStart, Partial: partial})
	for i, content := range message.Content {
		if ctx.Err() != nil {
			r.pushAborted(stream, partial)
			return
		}
		if text, ok := fauxTextContent(content); ok {
			partial.Content = append(partial.Content, TextContent{Type: ContentTypeText})
			stream.Push(AssistantMessageTextStartEvent{Type: AssistantMessageEventTypeTextStart, ContentIndex: i, Partial: partial})
			for _, chunk := range splitFauxText(text.Text, r.minTokenSize) {
				if !r.waitChunk(ctx, chunk) {
					r.pushAborted(stream, partial)
					return
				}
				current := partial.Content[i].(TextContent)
				current.Text += chunk
				partial.Content[i] = current
				stream.Push(AssistantMessageTextDeltaEvent{
					Type: AssistantMessageEventTypeTextDelta, ContentIndex: i, Delta: chunk, Partial: partial,
				})
			}
			stream.Push(AssistantMessageTextEndEvent{
				Type: AssistantMessageEventTypeTextEnd, ContentIndex: i, Content: text.Text, Partial: partial,
			})
			continue
		}
		if thinking, ok := fauxThinkingContent(content); ok {
			partial.Content = append(partial.Content, ThinkingContent{Type: ContentTypeThinking})
			stream.Push(AssistantMessageThinkingStartEvent{
				Type: AssistantMessageEventTypeThinkingStart, ContentIndex: i, Partial: partial,
			})
			for _, chunk := range splitFauxText(thinking.Thinking, r.minTokenSize) {
				if !r.waitChunk(ctx, chunk) {
					r.pushAborted(stream, partial)
					return
				}
				current := partial.Content[i].(ThinkingContent)
				current.Thinking += chunk
				partial.Content[i] = current
				stream.Push(AssistantMessageThinkingDeltaEvent{
					Type: AssistantMessageEventTypeThinkingDelta, ContentIndex: i, Delta: chunk, Partial: partial,
				})
			}
			stream.Push(AssistantMessageThinkingEndEvent{
				Type: AssistantMessageEventTypeThinkingEnd, ContentIndex: i, Content: thinking.Thinking, Partial: partial,
			})
			continue
		}
		toolCall, _ := fauxToolCallContent(content)
		partial.Content = append(partial.Content, ToolCall{
			Type: ContentTypeToolCall, ID: toolCall.ID, Name: toolCall.Name, Arguments: map[string]any{},
		})
		stream.Push(AssistantMessageToolCallStartEvent{
			Type: AssistantMessageEventTypeToolCallStart, ContentIndex: i, Partial: partial,
		})
		arguments, err := json.Marshal(toolCall.Arguments)
		if err != nil {
			r.pushError(stream, messageToModel(message), fmt.Errorf("encode Faux ToolCall arguments: %w", err))
			return
		}
		for _, chunk := range splitFauxText(string(arguments), r.minTokenSize) {
			if !r.waitChunk(ctx, chunk) {
				r.pushAborted(stream, partial)
				return
			}
			stream.Push(AssistantMessageToolCallDeltaEvent{
				Type: AssistantMessageEventTypeToolCallDelta, ContentIndex: i, Delta: chunk, Partial: partial,
			})
		}
		partial.Content[i] = cloneToolCall(toolCall)
		stream.Push(AssistantMessageToolCallEndEvent{
			Type: AssistantMessageEventTypeToolCallEnd, ContentIndex: i, ToolCall: toolCall, Partial: partial,
		})
	}
	switch message.StopReason {
	case StopReasonError, StopReasonAborted:
		stream.Push(AssistantMessageErrorEvent{Type: AssistantMessageEventTypeError, Reason: message.StopReason, Error: message})
	case StopReasonPending:
		r.pushError(stream, messageToModel(message), fmt.Errorf("Faux response ended without a stop reason"))
	default:
		stream.Push(AssistantMessageDoneEvent{Type: AssistantMessageEventTypeDone, Reason: message.StopReason, Message: message})
	}
}

func (r *fauxRuntime) waitChunk(ctx context.Context, chunk string) bool {
	if ctx.Err() != nil {
		return false
	}
	if r.tokensPerSecond == nil || *r.tokensPerSecond <= 0 || chunk == "" {
		return true
	}
	tokens := (len([]rune(chunk)) + 3) / 4
	delay := time.Duration(float64(time.Second) * float64(tokens) / *r.tokensPerSecond)
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return ctx.Err() == nil
	}
}

func (r *fauxRuntime) pushError(stream *AssistantMessageEventStream, model Model, err error) {
	stream.Push(AssistantMessageErrorEvent{
		Type:   AssistantMessageEventTypeError,
		Reason: StopReasonError,
		Error: AssistantMessage{
			Role:         MessageRoleAssistant,
			Content:      []AssistantContent{},
			API:          r.api,
			Provider:     r.provider,
			Model:        model.ID,
			StopReason:   StopReasonError,
			ErrorMessage: Some(err.Error()),
			Timestamp:    time.Now().UnixMilli(),
		},
	})
}

func (r *fauxRuntime) pushAborted(stream *AssistantMessageEventStream, partial AssistantMessage) {
	partial = CloneAssistantMessage(partial)
	partial.StopReason = StopReasonAborted
	partial.ErrorMessage = Some("Request was aborted")
	partial.Timestamp = time.Now().UnixMilli()
	stream.Push(AssistantMessageErrorEvent{Type: AssistantMessageEventTypeError, Reason: StopReasonAborted, Error: partial})
}

func unsupportedFauxOptions(options *SimpleStreamOptions) error {
	if options == nil {
		return nil
	}
	if !isNilRuntimeValue(options.Deferred) {
		switch deferred := options.Deferred.(type) {
		case DeferredBoolean:
			if deferred.Enabled {
				return newNotImplemented("Faux.Deferred")
			}
		case *DeferredBoolean:
			if deferred.Enabled {
				return newNotImplemented("Faux.Deferred")
			}
		default:
			return newNotImplemented("Faux.Deferred")
		}
	}
	if options.SessionID != nil && *options.SessionID != "" &&
		(options.CacheRetention == nil || *options.CacheRetention != CacheRetentionNone) {
		return newNotImplemented("Faux.Cache")
	}
	return nil
}

func validateFauxM1Message(message AssistantMessage) error {
	if message.StopReason == StopReasonDeferred || message.Deferred.IsSet() {
		return newNotImplemented("Faux.Deferred")
	}
	for _, content := range message.Content {
		if _, ok := fauxTextContent(content); ok {
			continue
		}
		if _, ok := fauxThinkingContent(content); ok {
			continue
		}
		if toolCall, ok := fauxToolCallContent(content); ok {
			if _, err := json.Marshal(toolCall.Arguments); err != nil {
				return fmt.Errorf("encode Faux ToolCall arguments: %w", err)
			}
			continue
		}
		return fmt.Errorf("unsupported Faux content %T", content)
	}
	return nil
}

func fauxToolCallContent(content AssistantContent) (ToolCall, bool) {
	switch content := content.(type) {
	case ToolCall:
		return cloneToolCall(content), true
	case *ToolCall:
		if content != nil {
			return cloneToolCall(*content), true
		}
	}
	return ToolCall{}, false
}

func fauxTextContent(content AssistantContent) (TextContent, bool) {
	switch content := content.(type) {
	case TextContent:
		return content, true
	case *TextContent:
		if content != nil {
			return *content, true
		}
	}
	return TextContent{}, false
}

func fauxThinkingContent(content AssistantContent) (ThinkingContent, bool) {
	switch content := content.(type) {
	case ThinkingContent:
		return content, true
	case *ThinkingContent:
		if content != nil {
			return *content, true
		}
	}
	return ThinkingContent{}, false
}

func resolveFauxResponse(
	step FauxResponseStep,
	input Context,
	options *SimpleStreamOptions,
	state *FauxProviderState,
	model Model,
) (AssistantMessage, error) {
	switch step := step.(type) {
	case AssistantMessage:
		return CloneAssistantMessage(step), nil
	case *AssistantMessage:
		if step != nil {
			return CloneAssistantMessage(*step), nil
		}
	case FauxResponseFactory:
		if step != nil {
			return step(input, options, state, cloneModel(model))
		}
	}
	return AssistantMessage{}, fmt.Errorf("unsupported Faux response step %T", step)
}

func cloneFauxResponseSteps(steps []FauxResponseStep) []FauxResponseStep {
	cloned := make([]FauxResponseStep, len(steps))
	for i, step := range steps {
		cloned[i] = cloneFauxResponseStep(step)
	}
	return cloned
}

func cloneFauxResponseStep(step FauxResponseStep) FauxResponseStep {
	switch step := step.(type) {
	case AssistantMessage:
		return CloneAssistantMessage(step)
	case *AssistantMessage:
		if step != nil {
			clone := CloneAssistantMessage(*step)
			return &clone
		}
	}
	return step
}

func makeFauxModels(api API, provider ProviderID, definitions []FauxModelDefinition) ([]Model, error) {
	defaulted := len(definitions) == 0
	if len(definitions) == 0 {
		definitions = []FauxModelDefinition{{ID: defaultFauxModelID}}
	}
	models := make([]Model, len(definitions))
	for i, definition := range definitions {
		if definition.ID == "" {
			return nil, fmt.Errorf("Faux model id must not be empty")
		}
		name := definition.ID
		if defaulted && i == 0 {
			name = defaultFauxModelName
		}
		if value, ok := definition.Name.Value(); ok {
			name = value
		}
		reasoning, _ := definition.Reasoning.Value()
		input := []ModelInput{ModelInputText, ModelInputImage}
		if value, ok := definition.Input.Value(); ok {
			input = append([]ModelInput(nil), value...)
		}
		cost := ModelCost{}
		if value, ok := definition.Cost.Value(); ok {
			cost.ModelCostRates = value
		}
		contextWindow := int64(128000)
		if value, ok := definition.ContextWindow.Value(); ok {
			contextWindow = int64(value)
		}
		maxTokens := int64(16384)
		if value, ok := definition.MaxTokens.Value(); ok {
			maxTokens = int64(value)
		}
		models[i] = Model{
			ID: definition.ID, Name: name, API: api, Provider: provider, BaseURL: defaultFauxBaseURL,
			Reasoning: reasoning, Input: input, Cost: cost, ContextWindow: contextWindow, MaxTokens: maxTokens,
		}
	}
	return models, nil
}

func normalizeFauxTokenSize(size *FauxTokenSize) int {
	minSize, maxSize := defaultFauxMinTokenSize, defaultFauxMaxTokenSize
	if size != nil {
		if size.Min != nil {
			minSize = *size.Min
		}
		if size.Max != nil {
			maxSize = *size.Max
		}
	}
	return max(1, min(minSize, maxSize))
}

func splitFauxText(text string, tokenSize int) []string {
	if text == "" {
		return []string{""}
	}
	runes := []rune(text)
	chunkSize := tokenSize * 4
	chunks := make([]string, 0, (len(runes)+chunkSize-1)/chunkSize)
	for len(runes) != 0 {
		n := min(chunkSize, len(runes))
		chunks = append(chunks, string(runes[:n]))
		runes = runes[n:]
	}
	return chunks
}

func nextFauxID(prefix string) string {
	return fmt.Sprintf("%s:%d", prefix, fauxID.Add(1))
}

func messageToModel(message AssistantMessage) Model {
	return Model{ID: message.Model, API: message.API, Provider: message.Provider}
}

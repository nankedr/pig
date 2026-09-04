package ai

import (
	"context"
	"fmt"
	"time"
)

type fauxDeferredResponse struct {
	handle         DeferredHandle
	step           FauxResponseStep
	input          Context
	options        SimpleStreamOptions
	model          Model
	state          *FauxProviderState
	pendingFetches int
	cancelled      bool
	ready          chan struct{}
	ctx            context.Context
	cancel         context.CancelFunc
	final          AssistantMessage
}

func fauxDeferredEnabled(option DeferredOption) bool {
	if isNilRuntimeValue(option) {
		return false
	}
	switch option := option.(type) {
	case DeferredBoolean:
		return option.Enabled
	case *DeferredBoolean:
		return option.Enabled
	default:
		return true
	}
}

func fauxDeferredMessage(model Model, handle DeferredHandle) AssistantMessage {
	return AssistantMessage{
		Role: MessageRoleAssistant, Content: []AssistantContent{},
		API: model.API, Provider: model.Provider, Model: model.ID,
		StopReason: StopReasonDeferred, Deferred: Some(handle), Timestamp: time.Now().UnixMilli(),
	}
}

func (r *fauxRuntime) submitDeferred(ctx context.Context, stream *AssistantMessageEventStream, step FauxResponseStep, state *FauxProviderState, model Model, input Context, options *SimpleStreamOptions) {
	if ctx.Err() != nil {
		r.pushAborted(stream, modelsTerminalMessage(model, ctx.Err()))
		return
	}
	handle := DeferredHandle{Provider: model.Provider, ModelID: model.ID, API: model.API, ID: nextFauxID("deferred"), PollAfterMS: r.pollAfterMS}
	entry := &fauxDeferredResponse{handle: handle, step: step, input: cloneFauxContext(input), options: cloneFauxOptions(*options), model: cloneModel(model), state: state, pendingFetches: r.pendingFetches}
	entry.ctx, entry.cancel = context.WithCancel(context.Background())
	entry.options.Deferred = nil
	entry.options.OnResponse = nil
	r.mu.Lock()
	r.deferred[handle.ID] = entry
	r.mu.Unlock()
	r.streamMessage(ctx, stream, fauxDeferredMessage(model, handle), model)
}

func cloneFauxContext(input Context) Context {
	input.Messages = append([]Message(nil), input.Messages...)
	for i, message := range input.Messages {
		input.Messages[i] = cloneFauxMessage(message)
	}
	input.Tools = append([]Tool(nil), input.Tools...)
	for i := range input.Tools {
		input.Tools[i].Parameters = append([]byte(nil), input.Tools[i].Parameters...)
		switch sampling := input.Tools[i].ConstrainedSampling.(type) {
		case *JSONSchemaConstrainedSampling:
			input.Tools[i].ConstrainedSampling = cloneFauxPointer(sampling)
		case *GrammarConstrainedSampling:
			input.Tools[i].ConstrainedSampling = cloneFauxPointer(sampling)
		}
	}
	return input
}

func cloneFauxMessage(message Message) Message {
	switch value := message.(type) {
	case UserMessage:
		value.Content.blocks = cloneUserContentSlice(value.Content.blocks)
		return value
	case AssistantMessage:
		return CloneAssistantMessage(value)
	case ToolResultMessage:
		content := make([]UserContent, len(value.Content))
		for i, block := range value.Content {
			if block != nil {
				content[i] = block.(UserContent)
			}
		}
		value.Content = make([]ToolResultContent, len(content))
		for i, block := range cloneUserContentSlice(content) {
			if block != nil {
				value.Content[i] = block.(ToolResultContent)
			}
		}
		value.Details = cloneOptional(value.Details, cloneJSONValue)
		value.AddedToolNames = cloneOptional(value.AddedToolNames, func(names []string) []string { return append([]string(nil), names...) })
		return value
	case *UserMessage:
		if value != nil {
			clone := cloneFauxMessage(*value).(UserMessage)
			return &clone
		}
	case *AssistantMessage:
		if value != nil {
			clone := CloneAssistantMessage(*value)
			return &clone
		}
	case *ToolResultMessage:
		if value != nil {
			clone := cloneFauxMessage(*value).(ToolResultMessage)
			return &clone
		}
	}
	return message
}

func cloneFauxOptions(options SimpleStreamOptions) SimpleStreamOptions {
	options.APIKey = cloneFauxPointer(options.APIKey)
	options.Headers = cloneProviderHeaders(options.Headers)
	options.Env = cloneProviderEnv(options.Env)
	options.TimeoutMS = cloneFauxPointer(options.TimeoutMS)
	options.MaxRetries = cloneFauxPointer(options.MaxRetries)
	options.MaxRetryDelayMS = cloneFauxPointer(options.MaxRetryDelayMS)
	options.Temperature = cloneFauxPointer(options.Temperature)
	options.SamplingParams = cloneRawMessageMap(options.SamplingParams)
	options.MaxTokens = cloneFauxPointer(options.MaxTokens)
	options.Transport = cloneFauxPointer(options.Transport)
	options.CacheRetention = cloneFauxPointer(options.CacheRetention)
	options.SessionID = cloneFauxPointer(options.SessionID)
	options.WebSocketConnectTimeoutMS = cloneFauxPointer(options.WebSocketConnectTimeoutMS)
	options.Metadata = cloneRawMessageMap(options.Metadata)
	options.Reasoning = cloneFauxPointer(options.Reasoning)
	options.ThinkingBudgets = cloneFauxPointer(options.ThinkingBudgets)
	if budget := options.ThinkingBudgets; budget != nil {
		budget.Minimal = cloneFauxPointer(budget.Minimal)
		budget.Low = cloneFauxPointer(budget.Low)
		budget.Medium = cloneFauxPointer(budget.Medium)
		budget.High = cloneFauxPointer(budget.High)
	}
	return options
}

func cloneFauxPointer[T any](value *T) *T {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

func (r *fauxRuntime) fetchDeferred(ctx context.Context, model Model, handle DeferredHandle, options DeferredFetchOptions) (*AssistantMessageEventStream, error) {
	ctx = nonNilContext(ctx)
	stream := NewAssistantMessageEventStream()
	r.mu.Lock()
	r.state.DeferredFetchCount++
	r.mu.Unlock()
	go func() {
		if ctx.Err() != nil {
			r.pushAborted(stream, fauxDeferredMessage(model, handle))
			return
		}
		if options.OnResponse != nil {
			if err := options.OnResponse(ctx, ProviderResponse{Status: 200, Headers: map[string]string{}}, model); err != nil {
				if ctx.Err() != nil {
					r.pushAborted(stream, fauxDeferredMessage(model, handle))
				} else {
					r.pushError(stream, model, err)
				}
				return
			}
		}
		if ctx.Err() != nil {
			r.pushAborted(stream, fauxDeferredMessage(model, handle))
			return
		}
		r.mu.Lock()
		entry, err := r.deferredEntry(model, handle)
		if err != nil {
			r.mu.Unlock()
			r.pushError(stream, model, err)
			return
		}
		pending := entry.pendingFetches > 0
		if pending {
			entry.pendingFetches--
		} else if entry.ready == nil {
			entry.ready = make(chan struct{})
			go r.resolveDeferred(entry)
		}
		r.mu.Unlock()
		fetchCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		stop := context.AfterFunc(entry.ctx, cancel)
		defer stop()
		if pending {
			r.streamDeferred(fetchCtx, stream, fauxDeferredMessage(model, entry.handle), model, entry)
			return
		}
		select {
		case <-entry.ready:
			r.streamDeferred(fetchCtx, stream, entry.final, model, entry)
		case <-entry.ctx.Done():
			r.pushError(stream, model, fmt.Errorf("Faux deferred response was cancelled: %s", handle.ID))
		case <-ctx.Done():
			r.pushAborted(stream, fauxDeferredMessage(model, handle))
		}
	}()
	return stream, nil
}

func (r *fauxRuntime) resolveDeferred(entry *fauxDeferredResponse) {
	defer close(entry.ready)
	message, err := resolveFauxResponse(entry.step, entry.input, &entry.options, entry.state, entry.model)
	if err == nil {
		message = CloneAssistantMessage(message)
		message.Role, message.API, message.Provider, message.Model = MessageRoleAssistant, r.api, r.provider, entry.model.ID
		err = validateFauxM1Message(message)
	}
	if err != nil {
		message = AssistantMessage{
			Role: MessageRoleAssistant, Content: []AssistantContent{}, API: r.api, Provider: r.provider, Model: entry.model.ID,
			StopReason: StopReasonError, ErrorMessage: Some(err.Error()), Timestamp: time.Now().UnixMilli(),
		}
	} else {
		message.Usage = r.estimateUsage(entry.input, &entry.options)
		updateFauxUsage(&message.Usage, entry.model, assistantMessageText(message))
	}
	entry.final = message
}

func (r *fauxRuntime) streamDeferred(ctx context.Context, stream *AssistantMessageEventStream, message AssistantMessage, model Model, entry *fauxDeferredResponse) {
	r.streamMessage(ctx, stream, message, model, func(event AssistantMessageEvent) {
		r.mu.Lock()
		defer r.mu.Unlock()
		if entry.cancelled {
			r.pushAborted(stream, message)
		} else if ctx.Err() != nil {
			r.pushAborted(stream, message)
		} else {
			stream.Push(event)
		}
	})
}

func (r *fauxRuntime) deferredEntry(model Model, handle DeferredHandle) (*fauxDeferredResponse, error) {
	entry := r.deferred[handle.ID]
	if entry == nil || entry.handle.Provider != handle.Provider || entry.handle.ModelID != handle.ModelID || entry.handle.API != handle.API ||
		model.Provider != handle.Provider || model.ID != handle.ModelID || model.API != handle.API {
		return nil, fmt.Errorf("Unknown faux deferred response: %s", handle.ID)
	}
	if entry.cancelled {
		return nil, fmt.Errorf("Faux deferred response was cancelled: %s", handle.ID)
	}
	return entry, nil
}

func (r *fauxRuntime) cancelDeferred(ctx context.Context, model Model, handle DeferredHandle, options DeferredCancelOptions) error {
	ctx = nonNilContext(ctx)
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	entry, err := r.deferredEntry(model, handle)
	if err == nil {
		entry.cancelled = true
		entry.cancel()
		handle.Data = cloneOptional(handle.Data, cloneJSONValue)
		r.state.CancelledDeferred = append(r.state.CancelledDeferred, handle)
	}
	r.mu.Unlock()
	if err != nil {
		return err
	}
	if options.OnResponse != nil {
		return options.OnResponse(ctx, ProviderResponse{Status: 200, Headers: map[string]string{}}, model)
	}
	return nil
}

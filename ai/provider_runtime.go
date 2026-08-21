package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// Provider is a named model source. It owns model discovery, authentication,
// and dispatch to one or more API protocol implementations; it is not itself
// an API Adapter.
type Provider interface {
	ID() ProviderID
	Name() string
	BaseURL() Optional[string]
	Headers() ProviderHeaders
	Auth() ProviderAuth
	GetModels() []Model
	FilterModels([]Model, Credential) []Model
	Stream(context.Context, Model, Context, ProviderStreamOptions) *AssistantMessageEventStream
	StreamSimple(context.Context, Model, Context, SimpleStreamOptions) *AssistantMessageEventStream

	SupportsRefreshModels() bool
	RefreshModels(RefreshModelsContext) error
	SupportsFetchDeferred() bool
	FetchDeferred(context.Context, Model, DeferredHandle, DeferredFetchOptions) (*AssistantMessageEventStream, error)
	SupportsCancelDeferred() bool
	CancelDeferred(context.Context, Model, DeferredHandle, DeferredCancelOptions) error
}

// ModelsPublication describes a provider-selected, ordered persistence
// mutation followed by a generation-checked in-memory update. An absent
// Persist leaves storage alone; a present nil pointer deletes the entry; a
// present non-nil pointer writes it. Persistence cannot be rolled back if the
// refresh is superseded while a replaceable store operation is in flight. An
// Update is a synchronous generation-checked commit; it must not call Refresh
// again for the same provider before returning. Such reentrancy is rejected so
// an older callback cannot overwrite a newer generation.
type ModelsPublication struct {
	Persist Optional[*ModelsStoreEntry]
	Update  func()
}

// RefreshModelsContext is the complete injected environment for one provider
// refresh phase. Publish is generation checked by the owning Models runtime.
// Implementations must observe Context and return promptly after cancellation;
// Go cannot forcibly stop an injected callback that ignores its context.
type RefreshModelsContext struct {
	Context      context.Context
	Credential   Credential
	Stored       *ModelsStoreEntry
	AllowNetwork bool
	Force        Optional[bool]
	Publish      func(ModelsPublication) (bool, error)
}

// FetchModelsFunction supplies a dynamic model overlay without prescribing a
// transport. Network access is controlled by RefreshModelsContext.
type FetchModelsFunction func(RefreshModelsContext) ([]Model, error)

// FilterModelsFunction applies provider policy to an immutable model snapshot.
// A nil Credential means that no provider credential is stored.
type FilterModelsFunction func([]Model, Credential) []Model

// ProviderAPIConfig is a discriminated runtime dispatch configuration. Build
// one with SingleProviderAPI or ProviderAPIs. Its zero value has no adapters.
type ProviderAPIConfig struct {
	single *ProviderStreams
	byAPI  map[API]ProviderStreams
}

// SingleProviderAPI applies one stream bundle to every model owned by the
// provider.
func SingleProviderAPI(streams ProviderStreams) ProviderAPIConfig {
	copy := streams
	return ProviderAPIConfig{single: &copy}
}

// ProviderAPIs dispatches each model strictly by Model.API. The input map is
// copied so later caller mutations cannot alter dispatch.
func ProviderAPIs(streams map[API]ProviderStreams) ProviderAPIConfig {
	copy := make(map[API]ProviderStreams, len(streams))
	for apiID, implementation := range streams {
		if implementation.registrationErr == nil && implementation.api != "" && implementation.api != apiID {
			implementation.registrationErr = fmt.Errorf(
				"%w: Provider API map key %q does not match adapter API %q for options %v",
				ErrEventStreamInvariant, apiID, implementation.api, implementation.optionsType,
			)
		}
		copy[apiID] = implementation
	}
	return ProviderAPIConfig{byAPI: copy}
}

// CreateProviderOptions supplies Provider-owned metadata, model catalogs, and
// API implementations. Name defaults to ID.
type CreateProviderOptions struct {
	ID           ProviderID
	Name         string
	BaseURL      Optional[string]
	Headers      ProviderHeaders
	Auth         ProviderAuth
	Models       []Model
	FetchModels  FetchModelsFunction
	FilterModels FilterModelsFunction
	API          ProviderAPIConfig
}

// CreatedProvider is the concrete Provider returned by CreateProvider. Its
// mutable dynamic catalog and dispatch configuration stay private.
type CreatedProvider struct {
	mu sync.RWMutex

	id             ProviderID
	name           string
	baseURL        Optional[string]
	headers        ProviderHeaders
	auth           ProviderAuth
	baselineModels []Model
	dynamicModels  []Model
	fetchModels    FetchModelsFunction
	filterModels   FilterModelsFunction
	api            ProviderAPIConfig
}

var _ Provider = (*CreatedProvider)(nil)

// CreateProvider constructs a runtime Provider while taking immutable
// snapshots of all caller-owned model and metadata inputs.
func CreateProvider(options CreateProviderOptions) *CreatedProvider {
	name := options.Name
	if name == "" {
		name = string(options.ID)
	}
	return &CreatedProvider{
		id:             options.ID,
		name:           name,
		baseURL:        options.BaseURL,
		headers:        cloneProviderHeaders(options.Headers),
		auth:           cloneProviderAuth(options.Auth),
		baselineModels: cloneModels(options.Models),
		fetchModels:    options.FetchModels,
		filterModels:   options.FilterModels,
		api:            cloneProviderAPIConfig(options.API),
	}
}

func (p *CreatedProvider) ID() ProviderID              { return p.id }
func (p *CreatedProvider) Name() string                { return p.name }
func (p *CreatedProvider) BaseURL() Optional[string]   { return p.baseURL }
func (p *CreatedProvider) Headers() ProviderHeaders    { return cloneProviderHeaders(p.headers) }
func (p *CreatedProvider) Auth() ProviderAuth          { return cloneProviderAuth(p.auth) }
func (p *CreatedProvider) SupportsRefreshModels() bool { return p.fetchModels != nil }
func (p *CreatedProvider) SupportsFetchDeferred() bool {
	return p.anyStreams(func(s ProviderStreams) bool {
		return s.FetchDeferred != nil && !s.fetchDeferredStub
	})
}
func (p *CreatedProvider) SupportsCancelDeferred() bool {
	return p.anyStreams(func(s ProviderStreams) bool {
		return s.CancelDeferred != nil && !s.cancelDeferredStub
	})
}

// GetModels returns the baseline with the current dynamic overlay. Equal IDs
// replace in place; new IDs append. Every returned model is a deep snapshot.
func (p *CreatedProvider) GetModels() []Model {
	p.mu.RLock()
	defer p.mu.RUnlock()
	merged := cloneModels(p.baselineModels)
	for _, dynamic := range p.dynamicModels {
		replacement := cloneModel(dynamic)
		replaced := false
		for i := range merged {
			if merged[i].ID == dynamic.ID {
				merged[i] = replacement
				replaced = true
				break
			}
		}
		if !replaced {
			merged = append(merged, replacement)
		}
	}
	return merged
}

// FilterModels applies the optional provider filter without exposing either
// the caller's or provider's model storage to mutation.
func (p *CreatedProvider) FilterModels(models []Model, credential Credential) []Model {
	input := cloneModels(models)
	if p.filterModels == nil {
		return input
	}
	return cloneModels(p.filterModels(input, credential))
}

// Stream dispatches with the full typed Go options. A missing implementation
// is represented by a terminal stream outcome, never a synchronous Go error.
func (p *CreatedProvider) Stream(ctx context.Context, model Model, input Context, options ProviderStreamOptions) *AssistantMessageEventStream {
	streams, ok := p.streamsFor(model)
	if !ok || (streams.streamTyped == nil && streams.Stream == nil) {
		return missingProviderAPIStream(p.id, model)
	}
	if isNilRuntimeValue(options) {
		return terminalProviderErrorStream(model, newModelsError(ModelsErrorCodeStream, "Provider stream options must not be nil", ErrEventStreamInvariant))
	}
	if streams.registrationErr != nil {
		return failedProviderStream(streams.registrationErr)
	}
	if streams.api != "" && streams.api != model.API {
		return failedProviderStream(fmt.Errorf(
			"%w: Provider adapter API %q does not match model API %q",
			ErrEventStreamInvariant, streams.api, model.API,
		))
	}
	var stream *AssistantMessageEventStream
	if streams.streamTyped != nil {
		stream = streams.streamTyped(ctx, model, input, options)
	} else {
		stream = streams.Stream(ctx, model, input, options.streamOptions())
	}
	if stream == nil {
		return terminalProviderErrorStream(model, newModelsError(ModelsErrorCodeStream, fmt.Sprintf("Provider %s returned a nil stream for %q", p.id, model.API), ErrEventStreamInvariant))
	}
	return stream
}

// StreamSimple is the simplified counterpart of Stream.
func (p *CreatedProvider) StreamSimple(ctx context.Context, model Model, input Context, options SimpleStreamOptions) *AssistantMessageEventStream {
	streams, ok := p.streamsFor(model)
	if !ok || streams.StreamSimple == nil {
		return missingProviderAPIStream(p.id, model)
	}
	stream := streams.StreamSimple(ctx, model, input, options)
	if stream == nil {
		return terminalProviderErrorStream(model, newModelsError(ModelsErrorCodeStream, fmt.Sprintf("Provider %s returned a nil simple stream for %q", p.id, model.API), ErrEventStreamInvariant))
	}
	return stream
}

// RefreshModels restores a stored overlay, then optionally fetches and
// publishes a replacement. Unsupported refresh is a structured provider error.
func (p *CreatedProvider) RefreshModels(refresh RefreshModelsContext) error {
	if p.fetchModels == nil {
		return newModelsError(
			ModelsErrorCodeProvider,
			fmt.Sprintf("Provider %s does not support model refresh", p.id),
			newNotImplemented("Provider.RefreshModels"),
		)
	}
	if refresh.Context == nil {
		refresh.Context = context.Background()
	}
	if refresh.Stored != nil {
		storedSnapshot := *refresh.Stored
		storedSnapshot.Models = cloneModels(refresh.Stored.Models)
		refresh.Stored = &storedSnapshot
		restored := make([]Model, 0, len(refresh.Stored.Models))
		for _, model := range refresh.Stored.Models {
			if model.Provider == p.id {
				restored = append(restored, cloneModel(model))
			}
		}
		published, err := publishModels(refresh.Publish, ModelsPublication{Update: func() { p.setDynamicModels(restored) }})
		if err != nil || !published {
			return err
		}
	}
	if !refresh.AllowNetwork || refresh.Context.Err() != nil {
		return nil
	}
	models, err := p.fetchModels(refresh)
	if err != nil {
		return err
	}
	if refresh.Context.Err() != nil {
		return nil
	}
	models = cloneModels(models)
	entry := &ModelsStoreEntry{Models: cloneModels(models), CheckedAt: Some(time.Now().UnixMilli())}
	_, err = publishModels(refresh.Publish, ModelsPublication{
		Persist: Some(entry),
		Update:  func() { p.setDynamicModels(models) },
	})
	return err
}

// FetchDeferred dispatches an optional deferred capability by Model.API.
func (p *CreatedProvider) FetchDeferred(ctx context.Context, model Model, handle DeferredHandle, options DeferredFetchOptions) (*AssistantMessageEventStream, error) {
	streams, ok := p.streamsFor(model)
	if !ok || streams.FetchDeferred == nil {
		return nil, newModelsError(
			ModelsErrorCodeProvider,
			fmt.Sprintf("Provider %s does not support deferred responses for %q", p.id, model.API),
			newNotImplemented("Provider.FetchDeferred"),
		)
	}
	stream, err := streams.FetchDeferred(ctx, model, handle, options)
	if err != nil {
		return nil, err
	}
	if stream == nil {
		return nil, newModelsError(
			ModelsErrorCodeStream,
			fmt.Sprintf("Provider %s returned a nil deferred stream for %q", p.id, model.API),
			ErrEventStreamInvariant,
		)
	}
	return stream, nil
}

// CancelDeferred dispatches an optional cancellation capability by Model.API.
func (p *CreatedProvider) CancelDeferred(ctx context.Context, model Model, handle DeferredHandle, options DeferredCancelOptions) error {
	streams, ok := p.streamsFor(model)
	if !ok || streams.CancelDeferred == nil {
		return newModelsError(
			ModelsErrorCodeProvider,
			fmt.Sprintf("Provider %s cannot cancel deferred responses for %q", p.id, model.API),
			newNotImplemented("Provider.CancelDeferred"),
		)
	}
	return streams.CancelDeferred(ctx, model, handle, options)
}

func (p *CreatedProvider) streamsFor(model Model) (ProviderStreams, bool) {
	if p.api.single != nil {
		return *p.api.single, true
	}
	streams, ok := p.api.byAPI[model.API]
	return streams, ok
}

func (p *CreatedProvider) anyStreams(predicate func(ProviderStreams) bool) bool {
	if p.api.single != nil {
		return predicate(*p.api.single)
	}
	for _, streams := range p.api.byAPI {
		if predicate(streams) {
			return true
		}
	}
	return false
}

func (p *CreatedProvider) setDynamicModels(models []Model) {
	p.mu.Lock()
	p.dynamicModels = cloneModels(models)
	p.mu.Unlock()
}

func publishModels(publish func(ModelsPublication) (bool, error), publication ModelsPublication) (bool, error) {
	if publish == nil {
		return false, newModelsError(ModelsErrorCodeProvider, "model refresh has no publication function", nil)
	}
	return publish(publication)
}

func missingProviderAPIStream(providerID ProviderID, model Model) *AssistantMessageEventStream {
	return terminalProviderErrorStream(model, newModelsError(
		ModelsErrorCodeStream,
		fmt.Sprintf("Provider %s has no API implementation for %q", providerID, model.API),
		nil,
	))
}

func terminalProviderErrorStream(model Model, err error) *AssistantMessageEventStream {
	message := AssistantMessage{
		Role:         MessageRoleAssistant,
		Content:      []AssistantContent{},
		API:          model.API,
		Provider:     model.Provider,
		Model:        model.ID,
		StopReason:   StopReasonError,
		ErrorMessage: Some(terminalErrorMessage(err)),
		Timestamp:    time.Now().UnixMilli(),
	}
	stream := NewAssistantMessageEventStream()
	stream.Push(AssistantMessageErrorEvent{
		Type:   AssistantMessageEventTypeError,
		Reason: StopReasonError,
		Error:  message,
	})
	return stream
}

func cloneProviderAPIConfig(config ProviderAPIConfig) ProviderAPIConfig {
	if config.single != nil {
		return SingleProviderAPI(*config.single)
	}
	return ProviderAPIs(config.byAPI)
}

func cloneProviderHeaders(headers ProviderHeaders) ProviderHeaders {
	if headers == nil {
		return nil
	}
	clone := make(ProviderHeaders, len(headers))
	for name, value := range headers {
		if value == nil {
			clone[name] = nil
			continue
		}
		copy := *value
		clone[name] = &copy
	}
	return clone
}

func cloneProviderAuth(auth ProviderAuth) ProviderAuth {
	clone := ProviderAuth{}
	if auth.APIKey != nil {
		apiKey := *auth.APIKey
		clone.APIKey = &apiKey
	}
	if auth.OAuth != nil {
		oauth := *auth.OAuth
		clone.OAuth = &oauth
	}
	return clone
}

func cloneModels(models []Model) []Model {
	if models == nil {
		return nil
	}
	clone := make([]Model, len(models))
	for i, model := range models {
		clone[i] = cloneModel(model)
	}
	return clone
}

func cloneModel(model Model) Model {
	clone := model
	if model.Input != nil {
		clone.Input = append([]ModelInput{}, model.Input...)
	}
	if model.Cost.Tiers != nil {
		clone.Cost.Tiers = append([]ModelCostTier{}, model.Cost.Tiers...)
	}
	if model.ThinkingLevelMap != nil {
		clone.ThinkingLevelMap = make(ThinkingLevelMap, len(model.ThinkingLevelMap))
		for level, value := range model.ThinkingLevelMap {
			clone.ThinkingLevelMap[level] = value
		}
	}
	if model.SamplingParams != nil {
		clone.SamplingParams = make(map[string]json.RawMessage, len(model.SamplingParams))
		for name, raw := range model.SamplingParams {
			clone.SamplingParams[name] = append([]byte(nil), raw...)
		}
	}
	if model.Headers != nil {
		clone.Headers = make(map[string]string, len(model.Headers))
		for name, value := range model.Headers {
			clone.Headers[name] = value
		}
	}
	clone.Compat = cloneOptional(model.Compat, func(raw json.RawMessage) json.RawMessage {
		return append(json.RawMessage(nil), raw...)
	})
	return clone
}

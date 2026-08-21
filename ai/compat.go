package ai

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

type CompatAPIStreamFunction func(context.Context, Model, Context, ProviderStreamOptions) *AssistantMessageEventStream

type CompatAPISimpleStreamFunction func(context.Context, Model, Context, SimpleStreamOptions) *AssistantMessageEventStream

type APIProvider struct {
	API          API
	Stream       CompatAPIStreamFunction
	StreamSimple CompatAPISimpleStreamFunction
}

type registeredAPIProvider struct {
	provider APIProvider
	sourceID Optional[string]
}

type compatRegistry struct {
	sync.RWMutex
	providers map[API]registeredAPIProvider
	order     []API
}

var compatAPIRegistry = newCompatAPIRegistry()

func newCompatAPIRegistry() *compatRegistry {
	registry := &compatRegistry{providers: make(map[API]registeredAPIProvider)}
	registry.order = builtinCompatAPIs()
	for _, api := range registry.order {
		registry.providers[api] = registeredAPIProvider{provider: guardedCompatAPIProvider(stubCompatAPIProvider(api))}
	}
	return registry
}

func RegisterAPIProvider(provider APIProvider, sourceID ...string) error {
	if provider.API == "" {
		return errors.New("API provider id must not be empty")
	}
	if provider.Stream == nil || provider.StreamSimple == nil {
		return errors.New("API provider stream functions must not be nil")
	}
	source := Absent[string]()
	if len(sourceID) != 0 {
		source = Some(sourceID[0])
	}
	provider = guardedCompatAPIProvider(provider)
	compatAPIRegistry.Lock()
	if _, exists := compatAPIRegistry.providers[provider.API]; !exists {
		compatAPIRegistry.order = append(compatAPIRegistry.order, provider.API)
	}
	compatAPIRegistry.providers[provider.API] = registeredAPIProvider{provider: provider, sourceID: source}
	compatAPIRegistry.Unlock()
	return nil
}

func GetAPIProvider(api API) (APIProvider, bool) {
	compatAPIRegistry.RLock()
	entry, ok := compatAPIRegistry.providers[api]
	compatAPIRegistry.RUnlock()
	return entry.provider, ok
}

func GetAPIProviders() []APIProvider {
	compatAPIRegistry.RLock()
	providers := make([]APIProvider, 0, len(compatAPIRegistry.order))
	for _, api := range compatAPIRegistry.order {
		providers = append(providers, compatAPIRegistry.providers[api].provider)
	}
	compatAPIRegistry.RUnlock()
	return providers
}

func UnregisterAPIProviders(sourceID string) {
	compatAPIRegistry.Lock()
	order := compatAPIRegistry.order[:0]
	for _, api := range compatAPIRegistry.order {
		entry := compatAPIRegistry.providers[api]
		if source, ok := entry.sourceID.Value(); ok && source == sourceID {
			delete(compatAPIRegistry.providers, api)
			continue
		}
		order = append(order, api)
	}
	compatAPIRegistry.order = order
	compatAPIRegistry.Unlock()
}

func RegisterBuiltinAPIProviders() error {
	compatAPIRegistry.Lock()
	defer compatAPIRegistry.Unlock()
	for _, api := range builtinCompatAPIs() {
		if _, exists := compatAPIRegistry.providers[api]; exists {
			continue
		}
		compatAPIRegistry.providers[api] = registeredAPIProvider{provider: guardedCompatAPIProvider(stubCompatAPIProvider(api))}
		compatAPIRegistry.order = append(compatAPIRegistry.order, api)
	}
	return nil
}

func builtinCompatAPIs() []API {
	return []API{
		APIAnthropicMessages,
		APIOpenAICompletions,
		APIOpenAIResponses,
		APIOpenAICodexResponses,
		APIAzureOpenAIResponses,
		APIGoogleGenerativeAI,
		APIGoogleVertex,
		APIMistralConversations,
		APIBedrockConverseStream,
		APIPiMessages,
	}
}

func ResetAPIProviders() error {
	providers := make(map[API]registeredAPIProvider)
	order := builtinCompatAPIs()
	for _, api := range order {
		providers[api] = registeredAPIProvider{provider: guardedCompatAPIProvider(stubCompatAPIProvider(api))}
	}
	compatAPIRegistry.Lock()
	compatAPIRegistry.providers = providers
	compatAPIRegistry.order = order
	compatAPIRegistry.Unlock()
	return nil
}

func guardedCompatAPIProvider(provider APIProvider) APIProvider {
	registered := provider
	stream := provider.Stream
	registered.Stream = func(ctx context.Context, model Model, input Context, options ProviderStreamOptions) *AssistantMessageEventStream {
		if model.API != provider.API {
			return failedProviderStream(fmt.Errorf("%w: model API %q does not match registered API %q", ErrEventStreamInvariant, model.API, provider.API))
		}
		return stream(ctx, model, input, options)
	}
	streamSimple := provider.StreamSimple
	registered.StreamSimple = func(ctx context.Context, model Model, input Context, options SimpleStreamOptions) *AssistantMessageEventStream {
		if model.API != provider.API {
			return failedProviderStream(fmt.Errorf("%w: model API %q does not match registered API %q", ErrEventStreamInvariant, model.API, provider.API))
		}
		return streamSimple(ctx, model, input, options)
	}
	return registered
}

func stubCompatAPIProvider(api API) APIProvider {
	return APIProvider{
		API: api,
		Stream: func(context.Context, Model, Context, ProviderStreamOptions) *AssistantMessageEventStream {
			return failedProviderStream(newNotImplemented("Compat.APIProvider.Stream"))
		},
		StreamSimple: func(context.Context, Model, Context, SimpleStreamOptions) *AssistantMessageEventStream {
			return failedProviderStream(newNotImplemented("Compat.APIProvider.StreamSimple"))
		},
	}
}

func Stream(context.Context, Model, Context, ...ProviderStreamOptions) *AssistantMessageEventStream {
	return failedProviderStream(newNotImplemented("Compat.Stream"))
}

func Complete(context.Context, Model, Context, ...ProviderStreamOptions) (AssistantMessage, error) {
	return AssistantMessage{}, newNotImplemented("Compat.Complete")
}

func StreamSimple(context.Context, Model, Context, ...SimpleStreamOptions) *AssistantMessageEventStream {
	return failedProviderStream(newNotImplemented("Compat.StreamSimple"))
}

func CompleteSimple(context.Context, Model, Context, ...SimpleStreamOptions) (AssistantMessage, error) {
	return AssistantMessage{}, newNotImplemented("Compat.CompleteSimple")
}

func GetModel(provider BuiltinProvider, modelID string) (Model, bool) {
	return GetBuiltinModel(provider, modelID)
}

func GetModels(provider BuiltinProvider) []Model { return GetBuiltinModels(provider) }

func GetProviders() []BuiltinProvider { return GetBuiltinProviders() }

type FauxModelDefinition struct {
	ID            string
	Name          Optional[string]
	Reasoning     Optional[bool]
	Input         Optional[[]ModelInput]
	Cost          Optional[ModelCostRates]
	ContextWindow Optional[int]
	MaxTokens     Optional[int]
}

type FauxContentBlock = AssistantContent

type FauxProviderState struct {
	CallCount          int
	DeferredFetchCount int
	CancelledDeferred  []DeferredHandle
}

type FauxResponseFactory func(Context, *SimpleStreamOptions, *FauxProviderState, Model) (AssistantMessage, error)

type FauxResponseStep interface {
	fauxResponseStep()
}

func (AssistantMessage) fauxResponseStep()    {}
func (FauxResponseFactory) fauxResponseStep() {}

type FauxAssistantMessageContent struct {
	text   string
	blocks []FauxContentBlock
	isText bool
}

func FauxAssistantText(text string) FauxAssistantMessageContent {
	return FauxAssistantMessageContent{text: text, isText: true}
}

func FauxAssistantBlocks(blocks ...FauxContentBlock) FauxAssistantMessageContent {
	return FauxAssistantMessageContent{blocks: append([]FauxContentBlock(nil), blocks...)}
}

type FauxDeferredOptions struct {
	PendingFetches *int
	PollAfterMS    *int64
}

type FauxTokenSize struct {
	Min *int
	Max *int
}

type RegisterFauxProviderOptions struct {
	API             API
	Provider        ProviderID
	Models          []FauxModelDefinition
	Deferred        *FauxDeferredOptions
	TokensPerSecond *float64
	TokenSize       *FauxTokenSize
}

type FauxProviderRegistration struct {
	API                     API
	Models                  []Model
	State                   *FauxProviderState
	GetModel                func(...string) (Model, bool)
	SetResponses            func([]FauxResponseStep)
	AppendResponses         func([]FauxResponseStep)
	GetPendingResponseCount func() int
	Unregister              func()
}

type FauxProviderHandle struct {
	Provider                Provider
	API                     API
	Models                  []Model
	State                   *FauxProviderState
	GetModel                func(...string) (Model, bool)
	SetResponses            func([]FauxResponseStep)
	AppendResponses         func([]FauxResponseStep)
	GetPendingResponseCount func() int
}

type FauxCore struct {
	API                     API
	Provider                ProviderID
	Models                  []Model
	Stream                  ProviderStreamFunction
	StreamSimple            ProviderSimpleStreamFunction
	FetchDeferred           func(context.Context, Model, DeferredHandle, DeferredFetchOptions) (*AssistantMessageEventStream, error)
	CancelDeferred          func(context.Context, Model, DeferredHandle, DeferredCancelOptions) error
	GetModel                func(...string) (Model, bool)
	State                   *FauxProviderState
	SetResponses            func([]FauxResponseStep)
	AppendResponses         func([]FauxResponseStep)
	GetPendingResponseCount func() int
}

type FauxToolCallOptions struct {
	ID Optional[string]
}

type FauxAssistantMessageOptions struct {
	StopReason   Optional[StopReason]
	Deferred     Optional[DeferredHandle]
	ErrorMessage Optional[string]
	ResponseID   Optional[string]
	Timestamp    Optional[int64]
}

func FauxText(text string) TextContent {
	return TextContent{Type: ContentTypeText, Text: text}
}

func FauxThinking(thinking string) ThinkingContent {
	return ThinkingContent{Type: ContentTypeThinking, Thinking: thinking}
}

func FauxToolCall(string, map[string]any, ...FauxToolCallOptions) (ToolCall, error) {
	return ToolCall{}, newNotImplemented("Faux.ToolCall")
}

func FauxAssistantMessage(FauxAssistantMessageContent, ...FauxAssistantMessageOptions) (AssistantMessage, error) {
	return AssistantMessage{}, newNotImplemented("Faux.AssistantMessage")
}

func CreateFauxCore(RegisterFauxProviderOptions) (*FauxCore, error) {
	return nil, newNotImplemented("Faux.CreateCore")
}

func NewFauxProvider(...RegisterFauxProviderOptions) (*FauxProviderHandle, error) {
	return nil, newNotImplemented("Faux.Provider")
}

func RegisterFauxProvider(...RegisterFauxProviderOptions) (*FauxProviderRegistration, error) {
	return nil, newNotImplemented("Compat.RegisterFauxProvider")
}

func StreamAnthropic(ctx context.Context, model Model, input Context, options AnthropicOptions) *AssistantMessageEventStream {
	return StreamAnthropicMessages(ctx, model, input, options)
}

func StreamGoogle(ctx context.Context, model Model, input Context, options GoogleOptions) *AssistantMessageEventStream {
	return StreamGoogleGenerativeAI(ctx, model, input, options)
}

func StreamMistral(ctx context.Context, model Model, input Context, options MistralOptions) *AssistantMessageEventStream {
	return StreamMistralConversations(ctx, model, input, options)
}

func StreamSimpleAnthropic(ctx context.Context, model Model, input Context, options SimpleStreamOptions) *AssistantMessageEventStream {
	return StreamSimpleAnthropicMessages(ctx, model, input, options)
}

func StreamSimpleGoogle(ctx context.Context, model Model, input Context, options SimpleStreamOptions) *AssistantMessageEventStream {
	return StreamSimpleGoogleGenerativeAI(ctx, model, input, options)
}

func StreamSimpleMistral(ctx context.Context, model Model, input Context, options SimpleStreamOptions) *AssistantMessageEventStream {
	return StreamSimpleMistralConversations(ctx, model, input, options)
}

type SessionResourceCleanup func(...string)

func RegisterSessionResourceCleanup(SessionResourceCleanup) (func(), error) {
	return nil, newNotImplemented("RegisterSessionResourceCleanup")
}

func CleanupSessionResources(...string) error {
	return newNotImplemented("CleanupSessionResources")
}

package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"reflect"
)

// APIStreamFunction is the strongly typed API-adapter authoring seam. It has
// no synchronous error channel: after a stream is created, Provider, model,
// request, and cancellation failures are terminal stream outcomes.
type APIStreamFunction[TOptions any] func(context.Context, Model, Context, TOptions) *AssistantMessageEventStream

// StreamFunction is the default StreamOptions specialization exported by the
// core contract.
type StreamFunction = APIStreamFunction[StreamOptions]

// APIAdapterDescriptor binds an API ID to its strongly typed options. Prefer
// the API-specific constructors below for known APIs and NewCustomAPIAdapter
// for extension APIs. EraseAPIAdapter rejects a known API paired with the
// wrong options type, including descriptors assembled with a struct literal.
//
// The zero value is only a declaration value; its Stream field is nil and must
// not be called. Constructors return descriptors with a non-nil Stream field;
// passing nil to a constructor installs a side-effect-free Capability Stub.
type APIAdapterDescriptor[TOptions any] struct {
	API    API
	Stream APIStreamFunction[TOptions]
	stub   bool
}

// NewAnthropicAPIAdapter binds the Anthropic Messages API to AnthropicOptions.
func NewAnthropicAPIAdapter(stream APIStreamFunction[AnthropicOptions]) APIAdapterDescriptor[AnthropicOptions] {
	return newAPIAdapterDescriptor(APIAnthropicMessages, stream)
}

// NewAzureOpenAIResponsesAPIAdapter binds the Azure OpenAI Responses API to
// AzureOpenAIResponsesOptions.
func NewAzureOpenAIResponsesAPIAdapter(stream APIStreamFunction[AzureOpenAIResponsesOptions]) APIAdapterDescriptor[AzureOpenAIResponsesOptions] {
	return newAPIAdapterDescriptor(APIAzureOpenAIResponses, stream)
}

// NewBedrockAPIAdapter binds the Bedrock Converse Stream API to BedrockOptions.
func NewBedrockAPIAdapter(stream APIStreamFunction[BedrockOptions]) APIAdapterDescriptor[BedrockOptions] {
	return newAPIAdapterDescriptor(APIBedrockConverseStream, stream)
}

// NewGoogleAPIAdapter binds the Google Generative AI API to GoogleOptions.
func NewGoogleAPIAdapter(stream APIStreamFunction[GoogleOptions]) APIAdapterDescriptor[GoogleOptions] {
	return newAPIAdapterDescriptor(APIGoogleGenerativeAI, stream)
}

// NewGoogleVertexAPIAdapter binds the Google Vertex API to GoogleVertexOptions.
func NewGoogleVertexAPIAdapter(stream APIStreamFunction[GoogleVertexOptions]) APIAdapterDescriptor[GoogleVertexOptions] {
	return newAPIAdapterDescriptor(APIGoogleVertex, stream)
}

// NewMistralAPIAdapter binds the Mistral Conversations API to MistralOptions.
func NewMistralAPIAdapter(stream APIStreamFunction[MistralOptions]) APIAdapterDescriptor[MistralOptions] {
	return newAPIAdapterDescriptor(APIMistralConversations, stream)
}

// NewOpenAICodexResponsesAPIAdapter binds the OpenAI Codex Responses API to
// OpenAICodexResponsesOptions.
func NewOpenAICodexResponsesAPIAdapter(stream APIStreamFunction[OpenAICodexResponsesOptions]) APIAdapterDescriptor[OpenAICodexResponsesOptions] {
	return newAPIAdapterDescriptor(APIOpenAICodexResponses, stream)
}

// NewOpenAICompletionsAPIAdapter binds the OpenAI Chat Completions API to
// OpenAICompletionsOptions.
func NewOpenAICompletionsAPIAdapter(stream APIStreamFunction[OpenAICompletionsOptions]) APIAdapterDescriptor[OpenAICompletionsOptions] {
	return newAPIAdapterDescriptor(APIOpenAICompletions, stream)
}

// NewOpenAIResponsesAPIAdapter binds the OpenAI Responses API to
// OpenAIResponsesOptions.
func NewOpenAIResponsesAPIAdapter(stream APIStreamFunction[OpenAIResponsesOptions]) APIAdapterDescriptor[OpenAIResponsesOptions] {
	return newAPIAdapterDescriptor(APIOpenAIResponses, stream)
}

// NewPiMessagesAPIAdapter binds the Pi Messages API to PiMessagesOptions.
func NewPiMessagesAPIAdapter(stream APIStreamFunction[PiMessagesOptions]) APIAdapterDescriptor[PiMessagesOptions] {
	return newAPIAdapterDescriptor(APIPiMessages, stream)
}

// NewCustomAPIAdapter constructs the raw extension-API authoring seam. It
// retains the original fixed signature so an untyped nil still creates a
// Capability Stub. Use NewCustomProviderAPIAdapter when composing through a
// Provider or Models runtime.
func NewCustomAPIAdapter(api API, stream APIStreamFunction[json.RawMessage]) APIAdapterDescriptor[json.RawMessage] {
	return newAPIAdapterDescriptor(api, stream)
}

// NewCustomProviderAPIAdapter constructs an extension adapter for Provider and
// Models composition. CustomAPIOptions carries both exact raw JSON and generic
// request options such as transport and lifecycle hooks.
func NewCustomProviderAPIAdapter(api API, stream APIStreamFunction[CustomAPIOptions]) APIAdapterDescriptor[CustomAPIOptions] {
	return newAPIAdapterDescriptor(api, stream)
}

func newAPIAdapterDescriptor[TOptions any](api API, stream APIStreamFunction[TOptions]) APIAdapterDescriptor[TOptions] {
	if stream == nil {
		stream = func(context.Context, Model, Context, TOptions) *AssistantMessageEventStream {
			failed := NewAssistantMessageEventStream()
			failed.stream.endWithError(newNotImplemented("APIAdapter.Stream"))
			return failed
		}
		return APIAdapterDescriptor[TOptions]{API: api, Stream: stream, stub: true}
	}
	return APIAdapterDescriptor[TOptions]{API: api, Stream: stream}
}

// ErasedAPIStreamFunction is the runtime dispatch seam for heterogeneous API
// adapters. Its error is limited to options decoding, Capability Stubs, or an
// internal invariant. Provider/runtime failures belong in the returned stream.
type ErasedAPIStreamFunction func(context.Context, Model, Context, json.RawMessage) (*AssistantMessageEventStream, error)

// ErasedAPIAdapter retains adapter metadata while erasing its options type.
type ErasedAPIAdapter struct {
	API    API
	Stream ErasedAPIStreamFunction

	providerStream  providerTypedStreamFunction
	optionsType     reflect.Type
	registrationErr error
}

// EraseAPIAdapter validates and decodes dynamic JSON options before invoking
// the typed authoring function. Invalid options never reach the typed stream.
// A json.RawMessage option type receives an exact copy of the source bytes.
func EraseAPIAdapter[TOptions any](descriptor APIAdapterDescriptor[TOptions]) ErasedAPIAdapter {
	optionsTypeErr := validateKnownAPIOptionsType[TOptions](descriptor.API)
	providerOptionsTypeErr := validateProviderOptionsType(descriptor.API, reflect.TypeFor[TOptions]())
	return ErasedAPIAdapter{
		API:             descriptor.API,
		optionsType:     reflect.TypeFor[TOptions](),
		registrationErr: providerOptionsTypeErr,
		Stream: func(ctx context.Context, model Model, input Context, raw json.RawMessage) (*AssistantMessageEventStream, error) {
			if descriptor.stub || descriptor.Stream == nil {
				return nil, newNotImplemented("APIAdapter.Stream")
			}
			if optionsTypeErr != nil {
				return nil, optionsTypeErr
			}
			var options TOptions
			if rawTarget, ok := any(&options).(*json.RawMessage); ok {
				if !json.Valid(raw) {
					return nil, fmt.Errorf("decode %s API options: invalid JSON", descriptor.API)
				}
				*rawTarget = append(json.RawMessage(nil), raw...)
			} else if customTarget, ok := any(&options).(*CustomAPIOptions); ok {
				if !json.Valid(raw) {
					return nil, fmt.Errorf("decode %s API options: invalid JSON", descriptor.API)
				}
				customTarget.Raw = append(json.RawMessage(nil), raw...)
			} else if err := json.Unmarshal(raw, &options); err != nil {
				return nil, fmt.Errorf("decode %s API options: %w", descriptor.API, err)
			}
			stream := descriptor.Stream(ctx, model, input, options)
			if stream == nil {
				return nil, fmt.Errorf("%w: adapter %q returned a nil stream", ErrEventStreamInvariant, descriptor.API)
			}
			return stream, nil
		},
		providerStream: func(ctx context.Context, model Model, input Context, options ProviderStreamOptions) *AssistantMessageEventStream {
			if descriptor.stub || descriptor.Stream == nil {
				return failedProviderStream(newNotImplemented("APIAdapter.Stream"))
			}
			if optionsTypeErr != nil {
				return failedProviderStream(optionsTypeErr)
			}
			typed, ok := any(options).(TOptions)
			if !ok {
				if generic, isGeneric := options.(StreamOptions); isGeneric && reflect.TypeFor[TOptions]().Kind() != reflect.Pointer {
					var zero TOptions
					if carrier, isCarrier := any(zero).(ProviderStreamOptions); isCarrier {
						typed, ok = any(carrier.withStreamOptions(generic)).(TOptions)
					}
				}
			}
			if !ok {
				return failedProviderStream(fmt.Errorf(
					"%w: Provider stream options type %T does not match %v for API %q",
					ErrEventStreamInvariant, options, reflect.TypeFor[TOptions](), model.API,
				))
			}
			if custom, isCustom := any(typed).(CustomAPIOptions); isCustom {
				if len(bytes.TrimSpace(custom.Raw)) == 0 {
					custom.Raw = json.RawMessage(`{}`)
				}
				if !json.Valid(custom.Raw) {
					return failedProviderStream(fmt.Errorf(
						"%w: invalid JSON options for custom API %q",
						ErrEventStreamInvariant, model.API,
					))
				}
				custom.Raw = append(json.RawMessage(nil), custom.Raw...)
				typed = any(custom).(TOptions)
			}
			stream := descriptor.Stream(ctx, model, input, typed)
			if stream == nil {
				return failedProviderStream(fmt.Errorf(
					"%w: adapter %q returned a nil stream", ErrEventStreamInvariant, descriptor.API,
				))
			}
			return stream
		},
	}
}

func validateKnownAPIOptionsType[TOptions any](api API) error {
	return validateKnownAPIOptionsReflect(api, reflect.TypeFor[TOptions]())
}

func validateKnownAPIOptionsReflect(api API, got reflect.Type) error {
	want, known := knownAPIOptionsType(api)
	if !known {
		return nil
	}
	if got == want {
		return nil
	}
	return fmt.Errorf(
		"%w: API adapter %q requires options %s, got %s",
		ErrEventStreamInvariant, api, want, got,
	)
}

func validateProviderOptionsType(api API, got reflect.Type) error {
	if err := validateKnownAPIOptionsReflect(api, got); err != nil {
		return err
	}
	if got == nil || !got.Implements(reflect.TypeFor[ProviderStreamOptions]()) {
		return fmt.Errorf(
			"%w: API adapter %q options %v cannot enter Provider runtime; use CustomAPIOptions for extension APIs",
			ErrEventStreamInvariant, api, got,
		)
	}
	return nil
}

func knownAPIOptionsType(api API) (reflect.Type, bool) {
	switch api {
	case APIAnthropicMessages:
		return reflect.TypeFor[AnthropicOptions](), true
	case APIAzureOpenAIResponses:
		return reflect.TypeFor[AzureOpenAIResponsesOptions](), true
	case APIBedrockConverseStream:
		return reflect.TypeFor[BedrockOptions](), true
	case APIGoogleGenerativeAI:
		return reflect.TypeFor[GoogleOptions](), true
	case APIGoogleVertex:
		return reflect.TypeFor[GoogleVertexOptions](), true
	case APIMistralConversations:
		return reflect.TypeFor[MistralOptions](), true
	case APIOpenAICodexResponses:
		return reflect.TypeFor[OpenAICodexResponsesOptions](), true
	case APIOpenAICompletions:
		return reflect.TypeFor[OpenAICompletionsOptions](), true
	case APIOpenAIResponses:
		return reflect.TypeFor[OpenAIResponsesOptions](), true
	case APIPiMessages:
		return reflect.TypeFor[PiMessagesOptions](), true
	default:
		return nil, false
	}
}

func knownAPIForOptionsType(optionsType reflect.Type) (API, bool) {
	for _, api := range []API{
		APIAnthropicMessages,
		APIAzureOpenAIResponses,
		APIBedrockConverseStream,
		APIGoogleGenerativeAI,
		APIGoogleVertex,
		APIMistralConversations,
		APIOpenAICodexResponses,
		APIOpenAICompletions,
		APIOpenAIResponses,
		APIPiMessages,
	} {
		want, _ := knownAPIOptionsType(api)
		if optionsType == want {
			return api, true
		}
	}
	return "", false
}

// NewStubAPIAdapter declares an API boundary that is not implemented in the
// current milestone. It returns before decoding options or invoking hooks.
func NewStubAPIAdapter(api API) ErasedAPIAdapter {
	return ErasedAPIAdapter{
		API: api,
		Stream: func(context.Context, Model, Context, json.RawMessage) (*AssistantMessageEventStream, error) {
			return nil, newNotImplemented("APIAdapter.Stream")
		},
		providerStream: func(context.Context, Model, Context, ProviderStreamOptions) *AssistantMessageEventStream {
			return failedProviderStream(newNotImplemented("APIAdapter.Stream"))
		},
	}
}

// ProviderDescriptor records Provider-owned metadata without conflating it
// with an API Adapter. Registry, authentication, model catalog, refresh, and
// network behavior belong to later issues.
type ProviderDescriptor struct {
	ID              ProviderID
	Name            string
	DefaultEndpoint Optional[string]
	DefaultHeaders  map[string]string
	AdapterAPIs     []API
}

// ProviderStreamFunction is the runtime Provider stream seam. Unlike an
// ErasedAPIAdapter, it preserves the complete Go request options, including
// transport and lifecycle hooks.
type ProviderStreamFunction func(context.Context, Model, Context, StreamOptions) *AssistantMessageEventStream

// providerTypedStreamFunction is the heterogeneous runtime wrapper installed
// by NewTypedProviderStreams. It stays private so API adapter implementations
// continue to author against one concrete option type.
type providerTypedStreamFunction func(context.Context, Model, Context, ProviderStreamOptions) *AssistantMessageEventStream

// ProviderSimpleStreamFunction is the simplified runtime Provider stream
// seam. It likewise preserves the complete Go request options.
type ProviderSimpleStreamFunction func(context.Context, Model, Context, SimpleStreamOptions) *AssistantMessageEventStream

// ProviderStreams is the runtime API capability bundle used by Provider
// composition. It is intentionally distinct from the raw-JSON API Adapter
// seam. Optional deferred operations remain explicit Capability Stubs. Its
// zero value has nil function fields and is not callable; construct a complete
// unimplemented bundle with NewStubProviderStreams.
type ProviderStreams struct {
	Stream         ProviderStreamFunction
	StreamSimple   ProviderSimpleStreamFunction
	FetchDeferred  func(context.Context, Model, DeferredHandle, DeferredFetchOptions) (*AssistantMessageEventStream, error)
	CancelDeferred func(context.Context, Model, DeferredHandle, DeferredCancelOptions) error

	streamTyped        providerTypedStreamFunction
	fetchDeferredStub  bool
	cancelDeferredStub bool
	api                API
	optionsType        reflect.Type
	registrationErr    error
}

// NewProviderStreams composes a validated, type-erased API adapter into a
// Provider runtime bundle while retaining its direct Go dispatch path. Known
// options remain concrete; CustomAPIOptions keeps raw extension JSON and
// generic request hooks without a JSON round trip.
func NewProviderStreams(adapter ErasedAPIAdapter) ProviderStreams {
	registrationErr := adapter.registrationErr
	if adapter.optionsType != nil {
		registrationErr = validateProviderOptionsType(adapter.API, adapter.optionsType)
	}
	if adapter.providerStream == nil && registrationErr == nil {
		registrationErr = fmt.Errorf(
			"%w: erased API adapter %q has no Provider dispatch path",
			ErrEventStreamInvariant, adapter.API,
		)
	}
	return ProviderStreams{
		api:             adapter.API,
		optionsType:     adapter.optionsType,
		registrationErr: registrationErr,
		streamTyped:     adapter.providerStream,
	}
}

// NewTypedProviderStreams adapts one concrete API option type to the
// heterogeneous Provider runtime without JSON conversion. Passing a different
// concrete option type produces a terminal stream invariant instead of a
// panic. Optional simple and deferred capabilities may be assigned separately.
func NewTypedProviderStreams[T ProviderStreamOptions](stream APIStreamFunction[T]) ProviderStreams {
	api, _ := knownAPIForOptionsType(reflect.TypeFor[T]())
	return NewProviderStreams(EraseAPIAdapter(newAPIAdapterDescriptor(api, stream)))
}

// NewStubProviderStreams returns a complete, side-effect-free Provider stream
// capability bundle. Every function field is non-nil and returns immediately
// without decoding options or invoking transports and lifecycle hooks.
func NewStubProviderStreams() ProviderStreams {
	return ProviderStreams{
		Stream: func(context.Context, Model, Context, StreamOptions) *AssistantMessageEventStream {
			return failedProviderStream(newNotImplemented("ProviderStreams.Stream"))
		},
		StreamSimple: func(context.Context, Model, Context, SimpleStreamOptions) *AssistantMessageEventStream {
			return failedProviderStream(newNotImplemented("ProviderStreams.StreamSimple"))
		},
		FetchDeferred: func(context.Context, Model, DeferredHandle, DeferredFetchOptions) (*AssistantMessageEventStream, error) {
			return nil, newNotImplemented("ProviderStreams.FetchDeferred")
		},
		CancelDeferred: func(context.Context, Model, DeferredHandle, DeferredCancelOptions) error {
			return newNotImplemented("ProviderStreams.CancelDeferred")
		},
		fetchDeferredStub:  true,
		cancelDeferredStub: true,
	}
}

func failedProviderStream(err error) *AssistantMessageEventStream {
	failed := NewAssistantMessageEventStream()
	failed.stream.endWithError(err)
	return failed
}

// ImagesFunction is the image-generation callable contract. Provider failures
// are represented by AssistantImages with an error/aborted stop reason; the Go
// error channel is reserved for waiter cancellation or internal invariants.
type ImagesFunction func(context.Context, ImagesModel, ImagesContext, ImagesOptions) (AssistantImages, error)

// ProviderImages is the uniform image API-adapter seam. Implementations remain
// deferred; this interface performs no registration or network behavior.
type ProviderImages interface {
	GenerateImages(context.Context, ImagesModel, ImagesContext, ImagesOptions) (AssistantImages, error)
}

type stubProviderImages struct{}

// NewStubProviderImages returns a non-nil, side-effect-free image capability
// stub. GenerateImages returns immediately without invoking transports or
// lifecycle hooks carried by ImagesOptions.
func NewStubProviderImages() ProviderImages {
	return stubProviderImages{}
}

func (stubProviderImages) GenerateImages(context.Context, ImagesModel, ImagesContext, ImagesOptions) (AssistantImages, error) {
	return AssistantImages{}, newNotImplemented("ProviderImages.GenerateImages")
}

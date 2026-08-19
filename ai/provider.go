package ai

import (
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

// NewCustomAPIAdapter constructs the separate raw-options authoring seam for
// extension APIs. The raw JSON is validated and copied before Stream is called.
func NewCustomAPIAdapter(api API, stream APIStreamFunction[json.RawMessage]) APIAdapterDescriptor[json.RawMessage] {
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
}

// EraseAPIAdapter validates and decodes dynamic JSON options before invoking
// the typed authoring function. Invalid options never reach the typed stream.
// A json.RawMessage option type receives an exact copy of the source bytes.
func EraseAPIAdapter[TOptions any](descriptor APIAdapterDescriptor[TOptions]) ErasedAPIAdapter {
	optionsTypeErr := validateKnownAPIOptionsType[TOptions](descriptor.API)
	return ErasedAPIAdapter{
		API: descriptor.API,
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
			} else if err := json.Unmarshal(raw, &options); err != nil {
				return nil, fmt.Errorf("decode %s API options: %w", descriptor.API, err)
			}
			stream := descriptor.Stream(ctx, model, input, options)
			if stream == nil {
				return nil, fmt.Errorf("%w: adapter %q returned a nil stream", ErrEventStreamInvariant, descriptor.API)
			}
			return stream, nil
		},
	}
}

func validateKnownAPIOptionsType[TOptions any](api API) error {
	want, known := knownAPIOptionsType(api)
	if !known {
		return nil
	}
	got := reflect.TypeFor[TOptions]()
	if got == want {
		return nil
	}
	return fmt.Errorf(
		"%w: API adapter %q requires options %s, got %s",
		ErrEventStreamInvariant, api, want, got,
	)
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

// NewStubAPIAdapter declares an API boundary that is not implemented in the
// current milestone. It returns before decoding options or invoking hooks.
func NewStubAPIAdapter(api API) ErasedAPIAdapter {
	return ErasedAPIAdapter{
		API: api,
		Stream: func(context.Context, Model, Context, json.RawMessage) (*AssistantMessageEventStream, error) {
			return nil, newNotImplemented("APIAdapter.Stream")
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

// ProviderStreams is the erased API capability bundle used by future Provider
// composition. Optional deferred operations remain explicit Capability Stubs.
// Its zero value has nil function fields and is not callable; construct a
// complete unimplemented bundle with NewStubProviderStreams.
type ProviderStreams struct {
	Stream         ErasedAPIStreamFunction
	StreamSimple   ErasedAPIStreamFunction
	FetchDeferred  func(context.Context, Model, DeferredHandle, DeferredFetchOptions) (*AssistantMessageEventStream, error)
	CancelDeferred func(context.Context, Model, DeferredHandle, DeferredCancelOptions) error
}

// NewStubProviderStreams returns a complete, side-effect-free Provider stream
// capability bundle. Every function field is non-nil and returns immediately
// without decoding options or invoking transports and lifecycle hooks.
func NewStubProviderStreams() ProviderStreams {
	return ProviderStreams{
		Stream: func(context.Context, Model, Context, json.RawMessage) (*AssistantMessageEventStream, error) {
			return nil, newNotImplemented("ProviderStreams.Stream")
		},
		StreamSimple: func(context.Context, Model, Context, json.RawMessage) (*AssistantMessageEventStream, error) {
			return nil, newNotImplemented("ProviderStreams.StreamSimple")
		},
		FetchDeferred: func(context.Context, Model, DeferredHandle, DeferredFetchOptions) (*AssistantMessageEventStream, error) {
			return nil, newNotImplemented("ProviderStreams.FetchDeferred")
		},
		CancelDeferred: func(context.Context, Model, DeferredHandle, DeferredCancelOptions) error {
			return newNotImplemented("ProviderStreams.CancelDeferred")
		},
	}
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

package ai

import (
	"context"
	"encoding/json"
	"reflect"
)

type LazyAPICapabilities struct {
	FetchDeferred  bool
	CancelDeferred bool
}

type LazyAPILoader func(context.Context) (ProviderStreams, error)

type LazyStreamSetup func(context.Context) (*AssistantMessageEventStream, error)

var BedrockProviderModule = BedrockConverseStreamAPI()

func LazyAPI(_ LazyAPILoader, capabilities ...LazyAPICapabilities) ProviderStreams {
	streams := NewStubProviderStreams()
	streams.Stream = func(context.Context, Model, Context, StreamOptions) *AssistantMessageEventStream {
		return failedProviderStream(newNotImplemented("LazyAPI.Stream"))
	}
	streams.StreamSimple = func(context.Context, Model, Context, SimpleStreamOptions) *AssistantMessageEventStream {
		return failedProviderStream(newNotImplemented("LazyAPI.StreamSimple"))
	}
	configured := LazyAPICapabilities{}
	if len(capabilities) != 0 {
		configured = capabilities[0]
	}
	if !configured.FetchDeferred {
		streams.FetchDeferred = nil
	}
	if !configured.CancelDeferred {
		streams.CancelDeferred = nil
	}
	return streams
}

func LazyStream(context.Context, Model, LazyStreamSetup) *AssistantMessageEventStream {
	return failedProviderStream(newNotImplemented("LazyStream"))
}

func AnthropicMessagesAPI() ProviderStreams {
	return stubAPIEntry(APIAnthropicMessages)
}

func AzureOpenAIResponsesAPI() ProviderStreams {
	return stubAPIEntry(APIAzureOpenAIResponses)
}

func BedrockConverseStreamAPI() ProviderStreams {
	return stubAPIEntry(APIBedrockConverseStream)
}

func GoogleGenerativeAIAPI() ProviderStreams {
	return stubAPIEntry(APIGoogleGenerativeAI)
}

func GoogleVertexAPI() ProviderStreams {
	return stubAPIEntry(APIGoogleVertex)
}

func MistralConversationsAPI() ProviderStreams {
	return stubAPIEntry(APIMistralConversations)
}

func OpenAICodexResponsesAPI() ProviderStreams {
	return stubAPIEntry(APIOpenAICodexResponses)
}

func OpenAICompletionsAPI() ProviderStreams {
	streams := stubAPIEntry(APIOpenAICompletions)
	streams.Stream = func(ctx context.Context, model Model, input Context, options StreamOptions) *AssistantMessageEventStream {
		return StreamOpenAICompletions(ctx, model, input, OpenAICompletionsOptions{StreamOptions: options})
	}
	streams.StreamSimple = StreamSimpleOpenAICompletions
	return streams
}

func OpenAIResponsesAPI() ProviderStreams {
	return stubAPIEntry(APIOpenAIResponses)
}

func PiMessagesAPI() ProviderStreams {
	return stubAPIEntry(APIPiMessages)
}

func stubAPIEntry(api API) ProviderStreams {
	streams := NewStubProviderStreams()
	streams.FetchDeferred = nil
	streams.CancelDeferred = nil
	streams.api = api
	streams.optionsType, _ = knownAPIOptionsType(api)
	return streams
}

func StreamAnthropicMessages(context.Context, Model, Context, AnthropicOptions) *AssistantMessageEventStream {
	return failedAPIEntry("AnthropicMessages.Stream")
}

func StreamAzureOpenAIResponses(context.Context, Model, Context, AzureOpenAIResponsesOptions) *AssistantMessageEventStream {
	return failedAPIEntry("AzureOpenAIResponses.Stream")
}

func StreamBedrockConverse(context.Context, Model, Context, BedrockOptions) *AssistantMessageEventStream {
	return failedAPIEntry("BedrockConverseStream.Stream")
}

func StreamGoogleGenerativeAI(context.Context, Model, Context, GoogleOptions) *AssistantMessageEventStream {
	return failedAPIEntry("GoogleGenerativeAI.Stream")
}

func StreamGoogleVertex(context.Context, Model, Context, GoogleVertexOptions) *AssistantMessageEventStream {
	return failedAPIEntry("GoogleVertex.Stream")
}

func StreamMistralConversations(context.Context, Model, Context, MistralOptions) *AssistantMessageEventStream {
	return failedAPIEntry("MistralConversations.Stream")
}

func StreamOpenAICodexResponses(context.Context, Model, Context, OpenAICodexResponsesOptions) *AssistantMessageEventStream {
	return failedAPIEntry("OpenAICodexResponses.Stream")
}

func StreamOpenAICompletions(_ context.Context, _ Model, _ Context, options OpenAICompletionsOptions) *AssistantMessageEventStream {
	return failedProviderStream(openAICompletionsStubError("OpenAICompletions.Stream", options))
}

// ConvertOpenAICompletionsMessagesOptions configures the published message
// conversion helper. GrammarToolInputProperties maps a grammar-constrained
// Tool name to the wire input-property name used while replaying ToolCall
// arguments.
type ConvertOpenAICompletionsMessagesOptions struct {
	GrammarToolInputProperties map[string]string
}

// ConvertOpenAICompletionsMessages maps model-context messages to OpenAI Chat
// Completions request-message objects. The result remains raw JSON so Pig does
// not expose third-party SDK types as part of its interface. Message conversion
// becomes live with the staged Chat Completions adapter; until then it is an
// explicit, side-effect-free Capability Stub.
func ConvertOpenAICompletionsMessages(Model, Context, OpenAICompletionsCompat, ...ConvertOpenAICompletionsMessagesOptions) ([]json.RawMessage, error) {
	return nil, newNotImplemented("OpenAICompletions.ConvertMessages")
}

func StreamOpenAIResponses(context.Context, Model, Context, OpenAIResponsesOptions) *AssistantMessageEventStream {
	return failedAPIEntry("OpenAIResponses.Stream")
}

func StreamPiMessages(context.Context, Model, Context, PiMessagesOptions) *AssistantMessageEventStream {
	return failedAPIEntry("PiMessages.Stream")
}

func StreamSimpleAnthropicMessages(context.Context, Model, Context, SimpleStreamOptions) *AssistantMessageEventStream {
	return failedAPIEntry("AnthropicMessages.StreamSimple")
}

func StreamSimpleAzureOpenAIResponses(context.Context, Model, Context, SimpleStreamOptions) *AssistantMessageEventStream {
	return failedAPIEntry("AzureOpenAIResponses.StreamSimple")
}

func StreamSimpleBedrockConverse(context.Context, Model, Context, SimpleStreamOptions) *AssistantMessageEventStream {
	return failedAPIEntry("BedrockConverseStream.StreamSimple")
}

func StreamSimpleGoogleGenerativeAI(context.Context, Model, Context, SimpleStreamOptions) *AssistantMessageEventStream {
	return failedAPIEntry("GoogleGenerativeAI.StreamSimple")
}

func StreamSimpleGoogleVertex(context.Context, Model, Context, SimpleStreamOptions) *AssistantMessageEventStream {
	return failedAPIEntry("GoogleVertex.StreamSimple")
}

func StreamSimpleMistralConversations(context.Context, Model, Context, SimpleStreamOptions) *AssistantMessageEventStream {
	return failedAPIEntry("MistralConversations.StreamSimple")
}

func StreamSimpleOpenAICodexResponses(context.Context, Model, Context, SimpleStreamOptions) *AssistantMessageEventStream {
	return failedAPIEntry("OpenAICodexResponses.StreamSimple")
}

func StreamSimpleOpenAICompletions(_ context.Context, _ Model, _ Context, options SimpleStreamOptions) *AssistantMessageEventStream {
	return failedProviderStream(openAICompletionsSimpleStubError(options))
}

func StreamSimpleOpenAIResponses(context.Context, Model, Context, SimpleStreamOptions) *AssistantMessageEventStream {
	return failedAPIEntry("OpenAIResponses.StreamSimple")
}

func StreamSimplePiMessages(context.Context, Model, Context, SimpleStreamOptions) *AssistantMessageEventStream {
	return failedAPIEntry("PiMessages.StreamSimple")
}

func failedAPIEntry(operation string) *AssistantMessageEventStream {
	return failedProviderStream(newNotImplemented(operation))
}

// openAICompletionsStubError distinguishes the adapter's baseline no-op
// options from unsupported options while the M1 request path remains a
// Capability Stub. The no-op fields are deliberately erased before checking
// whether a more specific unsupported-options error is required.
func openAICompletionsStubError(operation string, options OpenAICompletionsOptions) error {
	normalizeOpenAICompletionsNoOpOptions(&options.StreamOptions)
	if !reflect.ValueOf(options).IsZero() {
		return newNotImplemented(operation + ".Options")
	}
	return newNotImplemented(operation)
}

func openAICompletionsSimpleStubError(options SimpleStreamOptions) error {
	normalizeOpenAICompletionsNoOpOptions(&options.StreamOptions)
	options.Deferred = nil
	if !reflect.ValueOf(options).IsZero() {
		return newNotImplemented("OpenAICompletions.StreamSimple.Options")
	}
	return newNotImplemented("OpenAICompletions.StreamSimple")
}

func normalizeOpenAICompletionsNoOpOptions(options *StreamOptions) {
	options.TelemetryContext = nil
	options.Transport = nil
	options.WebSocketConnectTimeoutMS = nil
	options.Metadata = nil
}

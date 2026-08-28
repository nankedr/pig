package ai

import (
	"context"
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
	streams := NewTypedProviderStreams(StreamOpenAICompletions)
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

// ConvertOpenAICompletionsMessagesOptions configures the published message
// conversion helper. GrammarToolInputProperties maps a grammar-constrained
// Tool name to the wire input-property name used while replaying ToolCall
// arguments.
type ConvertOpenAICompletionsMessagesOptions struct {
	GrammarToolInputProperties map[string]string
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

func StreamSimpleOpenAIResponses(context.Context, Model, Context, SimpleStreamOptions) *AssistantMessageEventStream {
	return failedAPIEntry("OpenAIResponses.StreamSimple")
}

func StreamSimplePiMessages(context.Context, Model, Context, SimpleStreamOptions) *AssistantMessageEventStream {
	return failedAPIEntry("PiMessages.StreamSimple")
}

func failedAPIEntry(operation string) *AssistantMessageEventStream {
	return failedProviderStream(newNotImplemented(operation))
}

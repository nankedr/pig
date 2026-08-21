package ai_test

import (
	"context"
	"errors"
	"testing"

	"github.com/nankedr/pig/ai"
)

var (
	_ ai.APIStreamFunction[ai.AnthropicOptions]            = ai.StreamAnthropicMessages    // upstream: ./api/anthropic-messages#stream
	_ ai.APIStreamFunction[ai.AzureOpenAIResponsesOptions] = ai.StreamAzureOpenAIResponses // upstream: ./api/azure-openai-responses#stream
	_ ai.APIStreamFunction[ai.BedrockOptions]              = ai.StreamBedrockConverse      // upstream: ./api/bedrock-converse-stream#stream
	_ ai.APIStreamFunction[ai.GoogleOptions]               = ai.StreamGoogleGenerativeAI   // upstream: ./api/google-generative-ai#stream
	_ ai.APIStreamFunction[ai.GoogleVertexOptions]         = ai.StreamGoogleVertex         // upstream: ./api/google-vertex#stream
	_ ai.APIStreamFunction[ai.MistralOptions]              = ai.StreamMistralConversations // upstream: ./api/mistral-conversations#stream
	_ ai.APIStreamFunction[ai.OpenAICodexResponsesOptions] = ai.StreamOpenAICodexResponses // upstream: ./api/openai-codex-responses#stream
	_ ai.APIStreamFunction[ai.OpenAICompletionsOptions]    = ai.StreamOpenAICompletions    // upstream: ./api/openai-completions#stream
	_ ai.APIStreamFunction[ai.OpenAIResponsesOptions]      = ai.StreamOpenAIResponses      // upstream: ./api/openai-responses#stream
	_ ai.APIStreamFunction[ai.PiMessagesOptions]           = ai.StreamPiMessages           // upstream: ./api/pi-messages#stream

	_ ai.ProviderSimpleStreamFunction = ai.StreamSimpleAnthropicMessages    // upstream: ./api/anthropic-messages#streamSimple
	_ ai.ProviderSimpleStreamFunction = ai.StreamSimpleAzureOpenAIResponses // upstream: ./api/azure-openai-responses#streamSimple
	_ ai.ProviderSimpleStreamFunction = ai.StreamSimpleBedrockConverse      // upstream: ./api/bedrock-converse-stream#streamSimple
	_ ai.ProviderSimpleStreamFunction = ai.StreamSimpleGoogleGenerativeAI   // upstream: ./api/google-generative-ai#streamSimple
	_ ai.ProviderSimpleStreamFunction = ai.StreamSimpleGoogleVertex         // upstream: ./api/google-vertex#streamSimple
	_ ai.ProviderSimpleStreamFunction = ai.StreamSimpleMistralConversations // upstream: ./api/mistral-conversations#streamSimple
	_ ai.ProviderSimpleStreamFunction = ai.StreamSimpleOpenAICodexResponses // upstream: ./api/openai-codex-responses#streamSimple
	_ ai.ProviderSimpleStreamFunction = ai.StreamSimpleOpenAICompletions    // upstream: ./api/openai-completions#streamSimple
	_ ai.ProviderSimpleStreamFunction = ai.StreamSimpleOpenAIResponses      // upstream: ./api/openai-responses#streamSimple
	_ ai.ProviderSimpleStreamFunction = ai.StreamSimplePiMessages           // upstream: ./api/pi-messages#streamSimple

	_ = ai.LazyAPICapabilities{}    // upstream: LazyApiCapabilities
	_ = ai.LazyAPI                  // upstream: lazyApi
	_ = ai.LazyStream               // upstream: lazyStream
	_ = ai.SetBedrockProviderModule // upstream: setBedrockProviderModule
	_ = ai.BedrockProviderModule    // upstream: bedrockProviderModule
)

func TestBuiltinAPIEntriesAreCompleteSideEffectFreeStubs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		api     ai.API
		factory func() ai.ProviderStreams
	}{
		{ai.APIAnthropicMessages, ai.AnthropicMessagesAPI},
		{ai.APIAzureOpenAIResponses, ai.AzureOpenAIResponsesAPI},
		{ai.APIBedrockConverseStream, ai.BedrockConverseStreamAPI},
		{ai.APIGoogleGenerativeAI, ai.GoogleGenerativeAIAPI},
		{ai.APIGoogleVertex, ai.GoogleVertexAPI},
		{ai.APIMistralConversations, ai.MistralConversationsAPI},
		{ai.APIOpenAICodexResponses, ai.OpenAICodexResponsesAPI},
		{ai.APIOpenAICompletions, ai.OpenAICompletionsAPI},
		{ai.APIOpenAIResponses, ai.OpenAIResponsesAPI},
		{ai.APIPiMessages, ai.PiMessagesAPI},
	}

	for _, test := range tests {
		test := test
		t.Run(string(test.api), func(t *testing.T) {
			t.Parallel()

			entry := test.factory()
			if entry.Stream == nil || entry.StreamSimple == nil {
				t.Fatalf("entry has nil stream capabilities: %#v", entry)
			}
			if entry.FetchDeferred != nil || entry.CancelDeferred != nil {
				t.Fatalf("entry advertises deferred capabilities: %#v", entry)
			}
			invoked := 0
			options := ai.StreamOptions{ProviderRequestOptions: poisonedRequestOptions(&invoked)}
			stream := entry.Stream(context.Background(), ai.Model{API: test.api}, ai.Context{}, options)
			if stream == nil {
				t.Fatal("Stream returned nil")
			}
			if _, err := stream.Result(context.Background()); !errors.Is(err, ai.ErrNotImplemented) {
				t.Fatalf("Stream error = %v, want ErrNotImplemented", err)
			}
			stream = entry.StreamSimple(context.Background(), ai.Model{API: test.api}, ai.Context{}, ai.SimpleStreamOptions{StreamOptions: options})
			if _, err := stream.Result(context.Background()); !errors.Is(err, ai.ErrNotImplemented) {
				t.Fatalf("StreamSimple error = %v, want ErrNotImplemented", err)
			}
			if invoked != 0 {
				t.Fatalf("entry invoked request hooks %d times, want zero", invoked)
			}
		})
	}
}

func TestLazyAPIStubDoesNotInvokeLoader(t *testing.T) {
	t.Parallel()

	loads := 0
	entry := ai.LazyAPI(func(context.Context) (ai.ProviderStreams, error) {
		loads++
		return ai.NewStubProviderStreams(), nil
	}, ai.LazyAPICapabilities{FetchDeferred: true, CancelDeferred: true})

	stream := entry.Stream(context.Background(), ai.Model{}, ai.Context{}, ai.StreamOptions{})
	if _, err := stream.Result(context.Background()); !errors.Is(err, ai.ErrNotImplemented) {
		t.Fatalf("lazy Stream error = %v, want ErrNotImplemented", err)
	}
	if _, err := entry.FetchDeferred(context.Background(), ai.Model{}, ai.DeferredHandle{}, ai.DeferredFetchOptions{}); !errors.Is(err, ai.ErrNotImplemented) {
		t.Fatalf("lazy FetchDeferred error = %v, want ErrNotImplemented", err)
	}
	if err := entry.CancelDeferred(context.Background(), ai.Model{}, ai.DeferredHandle{}, ai.DeferredCancelOptions{}); !errors.Is(err, ai.ErrNotImplemented) {
		t.Fatalf("lazy CancelDeferred error = %v, want ErrNotImplemented", err)
	}
	if loads != 0 {
		t.Fatalf("lazy stub invoked loader %d times, want zero", loads)
	}
}

func TestLazyAPIOmitsUnrequestedCapabilities(t *testing.T) {
	t.Parallel()

	entry := ai.LazyAPI(func(context.Context) (ai.ProviderStreams, error) {
		t.Fatal("stub invoked loader")
		return ai.ProviderStreams{}, nil
	})
	if entry.Stream == nil || entry.StreamSimple == nil {
		t.Fatal("lazy API omitted required stream capabilities")
	}
	if entry.FetchDeferred != nil || entry.CancelDeferred != nil {
		t.Fatalf("lazy API exposed unrequested deferred capabilities: %#v", entry)
	}
}

func TestLazyStreamStubDoesNotInvokeSetup(t *testing.T) {
	t.Parallel()

	setups := 0
	stream := ai.LazyStream(context.Background(), ai.Model{}, func(context.Context) (*ai.AssistantMessageEventStream, error) {
		setups++
		return ai.NewAssistantMessageEventStream(), nil
	})
	if _, err := stream.Result(context.Background()); !errors.Is(err, ai.ErrNotImplemented) {
		t.Fatalf("LazyStream error = %v, want ErrNotImplemented", err)
	}
	if setups != 0 {
		t.Fatalf("LazyStream invoked setup %d times, want zero", setups)
	}
}

func poisonedRequestOptions(invoked *int) ai.ProviderRequestOptions {
	return ai.ProviderRequestOptions{
		Fetch: func(context.Context, ai.FetchRequest) (ai.FetchResponse, error) {
			*invoked++
			return ai.FetchResponse{}, nil
		},
		OnPayload: func(context.Context, ai.JSONValue, ai.Model) (ai.PayloadHookResult, error) {
			*invoked++
			return ai.PayloadHookResult{}, nil
		},
		OnResponse: func(context.Context, ai.ProviderResponse, ai.Model) error {
			*invoked++
			return nil
		},
	}
}

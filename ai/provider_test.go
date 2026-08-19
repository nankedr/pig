package ai_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/nankedr/pig/ai"
)

func TestKnownAPIAdapterConstructorsBindConcreteOptions(t *testing.T) {
	t.Parallel()

	exerciseKnownAPIAdapter(t, ai.APIAnthropicMessages, ai.NewAnthropicAPIAdapter)
	exerciseKnownAPIAdapter(t, ai.APIAzureOpenAIResponses, ai.NewAzureOpenAIResponsesAPIAdapter)
	exerciseKnownAPIAdapter(t, ai.APIBedrockConverseStream, ai.NewBedrockAPIAdapter)
	exerciseKnownAPIAdapter(t, ai.APIGoogleGenerativeAI, ai.NewGoogleAPIAdapter)
	exerciseKnownAPIAdapter(t, ai.APIGoogleVertex, ai.NewGoogleVertexAPIAdapter)
	exerciseKnownAPIAdapter(t, ai.APIMistralConversations, ai.NewMistralAPIAdapter)
	exerciseKnownAPIAdapter(t, ai.APIOpenAICodexResponses, ai.NewOpenAICodexResponsesAPIAdapter)
	exerciseKnownAPIAdapter(t, ai.APIOpenAICompletions, ai.NewOpenAICompletionsAPIAdapter)
	exerciseKnownAPIAdapter(t, ai.APIOpenAIResponses, ai.NewOpenAIResponsesAPIAdapter)
	exerciseKnownAPIAdapter(t, ai.APIPiMessages, ai.NewPiMessagesAPIAdapter)
}

func exerciseKnownAPIAdapter[TOptions any](
	t *testing.T,
	wantAPI ai.API,
	constructor func(ai.APIStreamFunction[TOptions]) ai.APIAdapterDescriptor[TOptions],
) {
	t.Helper()

	calls := 0
	descriptor := constructor(func(context.Context, ai.Model, ai.Context, TOptions) *ai.AssistantMessageEventStream {
		calls++
		return ai.NewAssistantMessageEventStream()
	})
	if descriptor.API != wantAPI {
		t.Fatalf("descriptor API = %q, want %q", descriptor.API, wantAPI)
	}
	if descriptor.Stream == nil {
		t.Fatal("descriptor Stream is nil")
	}
	if descriptorWithNilInput := constructor(nil); descriptorWithNilInput.Stream == nil {
		t.Fatal("descriptor constructed from a nil Stream has a nil function field")
	}

	erased := ai.EraseAPIAdapter(descriptor)
	stream, err := erased.Stream(context.Background(), ai.Model{}, ai.Context{}, json.RawMessage(`{}`))
	if err != nil || stream == nil || calls != 1 {
		t.Fatalf("erased Stream() = (%v, %v), calls=%d; want non-nil stream, nil error, one call", stream, err, calls)
	}
}

func TestEraseAPIAdapterRejectsEveryKnownAPIOptionsMismatch(t *testing.T) {
	t.Parallel()

	knownAPIs := []ai.API{
		ai.APIOpenAICompletions,
		ai.APIMistralConversations,
		ai.APIOpenAIResponses,
		ai.APIAzureOpenAIResponses,
		ai.APIOpenAICodexResponses,
		ai.APIAnthropicMessages,
		ai.APIBedrockConverseStream,
		ai.APIGoogleGenerativeAI,
		ai.APIGoogleVertex,
		ai.APIPiMessages,
	}
	for _, apiID := range knownAPIs {
		apiID := apiID
		t.Run(string(apiID), func(t *testing.T) {
			t.Parallel()

			calls := 0
			descriptor := ai.APIAdapterDescriptor[json.RawMessage]{
				API: apiID,
				Stream: func(context.Context, ai.Model, ai.Context, json.RawMessage) *ai.AssistantMessageEventStream {
					calls++
					return ai.NewAssistantMessageEventStream()
				},
			}

			stream, err := ai.EraseAPIAdapter(descriptor).Stream(
				context.Background(), ai.Model{}, ai.Context{}, json.RawMessage(`{`),
			)
			if stream != nil || !errors.Is(err, ai.ErrEventStreamInvariant) {
				t.Fatalf("mismatched descriptor Stream() = (%v, %v), want nil and ErrEventStreamInvariant", stream, err)
			}
			if calls != 0 {
				t.Fatalf("mismatched descriptor called typed Stream %d times, want zero", calls)
			}
		})
	}
}

func TestEraseAPIAdapterRejectsConcreteKnownAPIOptionsMismatch(t *testing.T) {
	t.Parallel()

	calls := 0
	descriptor := ai.APIAdapterDescriptor[ai.AnthropicOptions]{
		API: ai.APIOpenAIResponses,
		Stream: func(context.Context, ai.Model, ai.Context, ai.AnthropicOptions) *ai.AssistantMessageEventStream {
			calls++
			return ai.NewAssistantMessageEventStream()
		},
	}

	stream, err := ai.EraseAPIAdapter(descriptor).Stream(
		context.Background(), ai.Model{}, ai.Context{}, json.RawMessage(`{}`),
	)
	if stream != nil || !errors.Is(err, ai.ErrEventStreamInvariant) {
		t.Fatalf("mismatched descriptor Stream() = (%v, %v), want nil and ErrEventStreamInvariant", stream, err)
	}
	if calls != 0 {
		t.Fatalf("mismatched descriptor called typed Stream %d times, want zero", calls)
	}
}

func TestCustomAPIAdapterPreservesRawOptions(t *testing.T) {
	t.Parallel()

	raw := json.RawMessage(`{ "vendor_flag": false, "limit": 0 }`)
	var received json.RawMessage
	descriptor := ai.NewCustomAPIAdapter(
		ai.API("acme-wire-v1"),
		func(_ context.Context, _ ai.Model, _ ai.Context, options json.RawMessage) *ai.AssistantMessageEventStream {
			received = append(json.RawMessage(nil), options...)
			return ai.NewAssistantMessageEventStream()
		},
	)

	stream, err := ai.EraseAPIAdapter(descriptor).Stream(context.Background(), ai.Model{}, ai.Context{}, raw)
	if err != nil || stream == nil {
		t.Fatalf("custom Stream() = (%v, %v), want non-nil stream and nil error", stream, err)
	}
	if string(received) != string(raw) {
		t.Fatalf("custom options = %s, want exact bytes %s", received, raw)
	}
}

func TestStubProviderCapabilitiesAreCompleteAndSideEffectFree(t *testing.T) {
	t.Parallel()

	streams := ai.NewStubProviderStreams()
	if streams.Stream == nil || streams.StreamSimple == nil || streams.FetchDeferred == nil || streams.CancelDeferred == nil {
		t.Fatalf("stub ProviderStreams contains a nil operation: %#v", streams)
	}

	stream, err := streams.Stream(context.Background(), ai.Model{}, ai.Context{}, json.RawMessage(`{`))
	if stream != nil {
		t.Fatalf("stub Stream() returned %v, want nil", stream)
	}
	assertNotImplementedOperation(t, err, "ProviderStreams.Stream")

	stream, err = streams.StreamSimple(context.Background(), ai.Model{}, ai.Context{}, json.RawMessage(`{`))
	if stream != nil {
		t.Fatalf("stub StreamSimple() returned %v, want nil", stream)
	}
	assertNotImplementedOperation(t, err, "ProviderStreams.StreamSimple")

	sideEffects := 0
	requestOptions := ai.ProviderRequestOptions{
		Fetch: func(context.Context, ai.FetchRequest) (ai.FetchResponse, error) {
			sideEffects++
			return ai.FetchResponse{}, nil
		},
		OnPayload: func(context.Context, ai.JSONValue, ai.Model) (ai.PayloadHookResult, error) {
			sideEffects++
			return ai.PayloadHookResult{}, nil
		},
		OnResponse: func(context.Context, ai.ProviderResponse, ai.Model) error {
			sideEffects++
			return nil
		},
	}

	stream, err = streams.FetchDeferred(
		context.Background(),
		ai.Model{},
		ai.DeferredHandle{},
		ai.DeferredFetchOptions{ProviderRequestOptions: requestOptions},
	)
	if stream != nil {
		t.Fatalf("stub FetchDeferred() returned %v, want nil", stream)
	}
	assertNotImplementedOperation(t, err, "ProviderStreams.FetchDeferred")

	err = streams.CancelDeferred(context.Background(), ai.Model{}, ai.DeferredHandle{}, requestOptions)
	assertNotImplementedOperation(t, err, "ProviderStreams.CancelDeferred")

	images := ai.NewStubProviderImages()
	if images == nil {
		t.Fatal("stub ProviderImages is nil")
	}
	_, err = images.GenerateImages(
		context.Background(), ai.ImagesModel{}, ai.ImagesContext{},
		ai.ImagesOptions{ProviderRequestOptions: requestOptions},
	)
	assertNotImplementedOperation(t, err, "ProviderImages.GenerateImages")

	if sideEffects != 0 {
		t.Fatalf("stub capabilities invoked input hooks or transport %d times, want zero", sideEffects)
	}
}

func assertNotImplementedOperation(t *testing.T, err error, wantOperation string) {
	t.Helper()

	if !errors.Is(err, ai.ErrNotImplemented) {
		t.Fatalf("error = %v, want ErrNotImplemented", err)
	}
	var target *ai.NotImplementedError
	if !errors.As(err, &target) {
		t.Fatalf("error = %v, want *NotImplementedError", err)
	}
	if target.Module != "ai" || target.Operation != wantOperation {
		t.Fatalf("NotImplementedError = %#v, want module ai and operation %q", target, wantOperation)
	}
}

package ai_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nankedr/pig/ai"
)

func TestCreateProviderExposesMetadataAndCloneSafeBaselineModels(t *testing.T) {
	t.Parallel()

	headerValue := "provider-header"
	input := modelForProviderRuntime("baseline", ai.API("api-a"))
	input.Input = []ai.ModelInput{ai.ModelInputText}
	input.Headers = map[string]string{"X-Model": "original"}
	provider := ai.CreateProvider(ai.CreateProviderOptions{
		ID:      ai.ProviderID("mixed"),
		BaseURL: ai.Some("https://example.test/v1"),
		Headers: ai.ProviderHeaders{"X-Provider": &headerValue},
		Auth:    ai.ProviderAuth{APIKey: ptr(ai.NewStubAPIKeyAuth("Test key"))},
		Models:  []ai.Model{input},
		API:     ai.SingleProviderAPI(recordingProviderStreams(nil, nil)),
	})

	if provider.ID() != "mixed" || provider.Name() != "mixed" {
		t.Fatalf("provider identity = (%q, %q), want mixed default name", provider.ID(), provider.Name())
	}
	if baseURL, ok := provider.BaseURL().Value(); !ok || baseURL != "https://example.test/v1" {
		t.Fatalf("BaseURL() = (%q, %t), want configured URL", baseURL, ok)
	}

	input.Name = "mutated input"
	input.Input[0] = ai.ModelInputImage
	input.Headers["X-Model"] = "mutated input"
	headerValue = "mutated input"
	first := provider.GetModels()
	if len(first) != 1 || first[0].Name != "baseline" || first[0].Input[0] != ai.ModelInputText || first[0].Headers["X-Model"] != "original" {
		t.Fatalf("GetModels() observed mutated constructor input: %#v", first)
	}
	headers := provider.Headers()
	if got := headers["X-Provider"]; got == nil || *got != "provider-header" {
		t.Fatalf("Headers() = %#v, want cloned provider header", headers)
	}

	first[0].Name = "mutated output"
	first[0].Input[0] = ai.ModelInputImage
	first[0].Headers["X-Model"] = "mutated output"
	*headers["X-Provider"] = "mutated output"
	second := provider.GetModels()
	if second[0].Name != "baseline" || second[0].Input[0] != ai.ModelInputText || second[0].Headers["X-Model"] != "original" {
		t.Fatalf("GetModels() leaked returned snapshot mutation: %#v", second)
	}
	if got := provider.Headers()["X-Provider"]; got == nil || *got != "provider-header" {
		t.Fatalf("Headers() leaked returned snapshot mutation: %#v", provider.Headers())
	}
}

func TestCreateProviderPreservesBaseURLPresence(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		baseURL ai.Optional[string]
		wantSet bool
		want    string
	}{
		{name: "omitted", baseURL: ai.Absent[string]()},
		{name: "explicit empty", baseURL: ai.Some(""), wantSet: true},
		{name: "configured", baseURL: ai.Some("https://example.test/v1"), wantSet: true, want: "https://example.test/v1"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			provider := ai.CreateProvider(ai.CreateProviderOptions{ID: "base-url", BaseURL: test.baseURL})
			got, ok := provider.BaseURL().Value()
			if ok != test.wantSet || got != test.want {
				t.Fatalf("BaseURL() = (%q, %t), want (%q, %t)", got, ok, test.want, test.wantSet)
			}
		})
	}
}

func TestCreateProviderDispatchesSingleAndMappedAPIsWithFullOptions(t *testing.T) {
	t.Parallel()

	apiA := ai.API("api-a")
	apiB := ai.API("api-b")
	modelA := modelForProviderRuntime("model-a", apiA)
	modelB := modelForProviderRuntime("model-b", apiB)
	fetchCalls := 0
	payloadCalls := 0
	responseCalls := 0
	streamOptions := ai.StreamOptions{ProviderRequestOptions: ai.ProviderRequestOptions{
		Fetch: func(context.Context, ai.FetchRequest) (ai.FetchResponse, error) {
			fetchCalls++
			return ai.FetchResponse{Status: 204}, nil
		},
		OnPayload: func(context.Context, ai.JSONValue, ai.Model) (ai.PayloadHookResult, error) {
			payloadCalls++
			return ai.PayloadHookResult{}, nil
		},
		OnResponse: func(context.Context, ai.ProviderResponse, ai.Model) error {
			responseCalls++
			return nil
		},
	}}

	singleCalls := []string{}
	singleStream := ai.NewAssistantMessageEventStream()
	single := ai.CreateProvider(ai.CreateProviderOptions{
		ID:     "single",
		Auth:   ai.ProviderAuth{APIKey: ptr(ai.NewStubAPIKeyAuth("Test key"))},
		Models: []ai.Model{modelA, modelB},
		API: ai.SingleProviderAPI(recordingProviderStreams(
			func(ctx context.Context, model ai.Model, _ ai.Context, options ai.StreamOptions) *ai.AssistantMessageEventStream {
				singleCalls = append(singleCalls, model.ID)
				if response, err := options.Fetch(ctx, ai.FetchRequest{}); err != nil || response.Status != 204 {
					t.Fatalf("preserved Fetch() = (%#v, %v), want status 204", response, err)
				}
				if _, err := options.OnPayload(ctx, nil, model); err != nil {
					t.Fatalf("preserved OnPayload() error = %v", err)
				}
				if err := options.OnResponse(ctx, ai.ProviderResponse{}, model); err != nil {
					t.Fatalf("preserved OnResponse() error = %v", err)
				}
				return singleStream
			},
			nil,
		)),
	})
	if got := single.Stream(context.Background(), modelB, ai.Context{}, streamOptions); got != singleStream {
		t.Fatalf("single Stream() = %p, want %p", got, singleStream)
	}
	if len(singleCalls) != 1 || singleCalls[0] != "model-b" || fetchCalls != 1 || payloadCalls != 1 || responseCalls != 1 {
		t.Fatalf("single dispatch calls=%v fetch=%d payload=%d response=%d", singleCalls, fetchCalls, payloadCalls, responseCalls)
	}

	mappedCalls := []string{}
	simpleA := ai.NewAssistantMessageEventStream()
	simpleB := ai.NewAssistantMessageEventStream()
	mapped := ai.CreateProvider(ai.CreateProviderOptions{
		ID:   "mixed",
		Auth: ai.ProviderAuth{APIKey: ptr(ai.NewStubAPIKeyAuth("Test key"))},
		API: ai.ProviderAPIs(map[ai.API]ai.ProviderStreams{
			apiA: recordingProviderStreams(nil, func(context.Context, ai.Model, ai.Context, ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
				mappedCalls = append(mappedCalls, "a")
				return simpleA
			}),
			apiB: recordingProviderStreams(nil, func(context.Context, ai.Model, ai.Context, ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
				mappedCalls = append(mappedCalls, "b")
				return simpleB
			}),
		}),
	})
	if got := mapped.StreamSimple(context.Background(), modelB, ai.Context{}, ai.SimpleStreamOptions{}); got != simpleB {
		t.Fatalf("mapped StreamSimple(api-b) = %p, want %p", got, simpleB)
	}
	if got := mapped.StreamSimple(context.Background(), modelA, ai.Context{}, ai.SimpleStreamOptions{}); got != simpleA {
		t.Fatalf("mapped StreamSimple(api-a) = %p, want %p", got, simpleA)
	}
	if len(mappedCalls) != 2 || mappedCalls[0] != "b" || mappedCalls[1] != "a" {
		t.Fatalf("mapped dispatch calls = %v, want [b a]", mappedCalls)
	}
}

func TestCreateProviderPreservesConcreteTypedStreamOptions(t *testing.T) {
	t.Parallel()

	model := modelForProviderRuntime("typed-model", ai.APIOpenAIResponses)
	returned := ai.NewAssistantMessageEventStream()
	var received ai.OpenAIResponsesOptions
	provider := ai.CreateProvider(ai.CreateProviderOptions{
		ID: "mixed",
		API: ai.SingleProviderAPI(ai.NewTypedProviderStreams(
			func(_ context.Context, _ ai.Model, _ ai.Context, options ai.OpenAIResponsesOptions) *ai.AssistantMessageEventStream {
				received = options
				return returned
			},
		)),
	})

	want := ai.OpenAIResponsesOptions{
		StreamOptions: ai.StreamOptions{SessionID: ptr("typed-session")},
		ServiceTier:   ai.Some(ai.OpenAIServiceTierPriority),
	}
	if got := provider.Stream(context.Background(), model, ai.Context{}, want); got != returned {
		t.Fatalf("Stream() = %p, want typed implementation stream %p", got, returned)
	}
	if tier, ok := received.ServiceTier.Value(); !ok || tier != ai.OpenAIServiceTierPriority {
		t.Fatalf("received ServiceTier = (%q, %t), want priority", tier, ok)
	}
	if received.SessionID != want.SessionID {
		t.Fatalf("received SessionID pointer = %p, want %p", received.SessionID, want.SessionID)
	}
}

func TestCreateProviderRejectsMappedKnownAPIOptionsTypeMismatch(t *testing.T) {
	t.Parallel()

	model := modelForProviderRuntime("mismatched-options", ai.APIOpenAIResponses)
	calls := 0
	provider := ai.CreateProvider(ai.CreateProviderOptions{
		ID: model.Provider,
		API: ai.ProviderAPIs(map[ai.API]ai.ProviderStreams{
			ai.APIOpenAIResponses: ai.NewTypedProviderStreams(
				func(context.Context, ai.Model, ai.Context, ai.AnthropicOptions) *ai.AssistantMessageEventStream {
					calls++
					return ai.NewAssistantMessageEventStream()
				},
			),
		}),
	})

	stream := provider.Stream(context.Background(), model, ai.Context{}, ai.AnthropicOptions{})
	if _, err := stream.Result(context.Background()); !errors.Is(err, ai.ErrEventStreamInvariant) {
		t.Fatalf("mismatched ProviderAPIs registration error = %v, want ErrEventStreamInvariant", err)
	}
	if calls != 0 {
		t.Fatalf("mismatched ProviderAPIs registration invoked callback %d times, want zero", calls)
	}
}

func TestCustomAPIAdapterFlowsThroughProviderAndModels(t *testing.T) {
	t.Parallel()

	apiID := ai.API("acme-wire-v1")
	model := modelForProviderRuntime("custom-model", apiID)
	raw := json.RawMessage(`{ "vendor_flag": false, "limit": 0 }`)
	type observation struct {
		raw         json.RawMessage
		hasFetch    bool
		hasPayload  bool
		hasResponse bool
		apiKey      string
	}
	observed := make(chan observation, 2)
	adapter := ai.NewCustomProviderAPIAdapter(
		apiID,
		func(_ context.Context, got ai.Model, _ ai.Context, options ai.CustomAPIOptions) *ai.AssistantMessageEventStream {
			apiKey := ""
			if options.APIKey != nil {
				apiKey = *options.APIKey
			}
			observed <- observation{
				raw:         append(json.RawMessage(nil), options.Raw...),
				hasFetch:    options.Fetch != nil,
				hasPayload:  options.OnPayload != nil,
				hasResponse: options.OnResponse != nil,
				apiKey:      apiKey,
			}
			return completedOutcomeRegressionStream(got)
		},
	)
	provider := ai.CreateProvider(ai.CreateProviderOptions{
		ID:     model.Provider,
		Models: []ai.Model{model},
		Auth: ai.ProviderAuth{APIKey: &ai.APIKeyAuth{
			Name: "Configured",
			Resolve: func(context.Context, ai.APIKeyResolveInput) (ai.Optional[ai.AuthResult], error) {
				return ai.Some(ai.AuthResult{Auth: ai.ModelAuth{APIKey: ai.Some("resolved-key")}}), nil
			},
		}},
		API: ai.ProviderAPIs(map[ai.API]ai.ProviderStreams{apiID: ai.NewProviderStreams(ai.EraseAPIAdapter(adapter))}),
	})

	var hookCalls atomic.Int32
	options := ai.CustomAPIOptions{
		StreamOptions: ai.StreamOptions{ProviderRequestOptions: ai.ProviderRequestOptions{
			Fetch: func(context.Context, ai.FetchRequest) (ai.FetchResponse, error) {
				hookCalls.Add(1)
				return ai.FetchResponse{}, nil
			},
			OnPayload: func(context.Context, ai.JSONValue, ai.Model) (ai.PayloadHookResult, error) {
				hookCalls.Add(1)
				return ai.PayloadHookResult{}, nil
			},
			OnResponse: func(context.Context, ai.ProviderResponse, ai.Model) error {
				hookCalls.Add(1)
				return nil
			},
		}},
		Raw: raw,
	}
	direct := provider.Stream(context.Background(), model, ai.Context{}, options)
	if _, err := direct.Result(context.Background()); err != nil {
		t.Fatalf("direct custom stream error = %v", err)
	}

	models := ai.CreateModels()
	models.SetProvider(provider)
	stream := models.Stream(context.Background(), model, ai.Context{}, ai.ModelsAPIStreamOptions[ai.CustomAPIOptions]{Options: options})
	if _, err := stream.Result(context.Background()); err != nil {
		t.Fatalf("Models custom stream error = %v", err)
	}

	for index := 0; index < 2; index++ {
		got := <-observed
		if !bytes.Equal(got.raw, raw) {
			t.Fatalf("custom raw options = %s, want exact bytes %s", got.raw, raw)
		}
		if !got.hasFetch || !got.hasPayload || !got.hasResponse {
			t.Fatalf("custom generic hooks = fetch %t/payload %t/response %t, want all preserved", got.hasFetch, got.hasPayload, got.hasResponse)
		}
		if index == 1 && got.apiKey != "resolved-key" {
			t.Fatalf("Models custom API key = %q, want auth overlay", got.apiKey)
		}
	}
	if hookCalls.Load() != 0 {
		t.Fatalf("custom adapter registration invoked hooks %d times, want callbacks preserved but not invoked by runtime", hookCalls.Load())
	}
}

func TestCustomAPIAdapterRejectsInvalidRawOptionsBeforeCallback(t *testing.T) {
	t.Parallel()

	apiID := ai.API("invalid-custom-options")
	model := modelForProviderRuntime("invalid-custom", apiID)
	calls := 0
	provider := ai.CreateProvider(ai.CreateProviderOptions{
		ID: model.Provider,
		API: ai.ProviderAPIs(map[ai.API]ai.ProviderStreams{
			apiID: ai.NewProviderStreams(ai.EraseAPIAdapter(ai.NewCustomProviderAPIAdapter(
				apiID,
				func(context.Context, ai.Model, ai.Context, ai.CustomAPIOptions) *ai.AssistantMessageEventStream {
					calls++
					return ai.NewAssistantMessageEventStream()
				},
			))),
		}),
	})

	stream := provider.Stream(context.Background(), model, ai.Context{}, ai.CustomAPIOptions{Raw: json.RawMessage(`{`)})
	if _, err := stream.Result(context.Background()); !errors.Is(err, ai.ErrEventStreamInvariant) {
		t.Fatalf("invalid custom options error = %v, want ErrEventStreamInvariant", err)
	}
	if calls != 0 {
		t.Fatalf("invalid custom options invoked callback %d times, want zero", calls)
	}
}

func TestNewProviderStreamsRejectsRawOnlyAdapterAtRuntimeBoundary(t *testing.T) {
	t.Parallel()

	apiID := ai.API("raw-only-custom")
	model := modelForProviderRuntime("raw-only-model", apiID)
	calls := 0
	provider := ai.CreateProvider(ai.CreateProviderOptions{
		ID: model.Provider,
		API: ai.ProviderAPIs(map[ai.API]ai.ProviderStreams{
			apiID: ai.NewProviderStreams(ai.EraseAPIAdapter(ai.NewCustomAPIAdapter(
				apiID,
				func(context.Context, ai.Model, ai.Context, json.RawMessage) *ai.AssistantMessageEventStream {
					calls++
					return ai.NewAssistantMessageEventStream()
				},
			))),
		}),
	})

	stream := provider.Stream(context.Background(), model, ai.Context{}, ai.CustomAPIOptions{Raw: json.RawMessage(`{}`)})
	if _, err := stream.Result(context.Background()); !errors.Is(err, ai.ErrEventStreamInvariant) {
		t.Fatalf("raw-only Provider adapter error = %v, want ErrEventStreamInvariant", err)
	}
	if calls != 0 {
		t.Fatalf("raw-only Provider adapter invoked callback %d times, want zero", calls)
	}
}

func TestNewProviderStreamsPreservesStubAdapterIdentity(t *testing.T) {
	t.Parallel()

	apiID := ai.API("future-custom-api")
	model := modelForProviderRuntime("stub-adapter-model", apiID)
	provider := ai.CreateProvider(ai.CreateProviderOptions{
		ID: model.Provider,
		Auth: ai.ProviderAuth{APIKey: &ai.APIKeyAuth{
			Name: "Configured",
			Resolve: func(context.Context, ai.APIKeyResolveInput) (ai.Optional[ai.AuthResult], error) {
				return ai.Some(ai.AuthResult{Auth: ai.ModelAuth{APIKey: ai.Some("key")}}), nil
			},
		}},
		API: ai.ProviderAPIs(map[ai.API]ai.ProviderStreams{
			apiID: ai.NewProviderStreams(ai.NewStubAPIAdapter(apiID)),
		}),
	})

	stream := provider.Stream(context.Background(), model, ai.Context{}, ai.CustomAPIOptions{Raw: json.RawMessage(`{}`)})
	_, err := stream.Result(context.Background())
	assertStubAdapterOperation(t, err)

	models := ai.CreateModels()
	models.SetProvider(provider)
	stream = models.Stream(
		context.Background(),
		model,
		ai.Context{},
		ai.ModelsAPIStreamOptions[ai.CustomAPIOptions]{Options: ai.CustomAPIOptions{Raw: json.RawMessage(`{}`)}},
	)
	_, err = stream.Result(context.Background())
	assertStubAdapterOperation(t, err)
}

func TestNewProviderStreamsPreservesTypedConstructorStubIdentity(t *testing.T) {
	t.Parallel()

	model := modelForProviderRuntime("typed-stub-model", ai.APIOpenAIResponses)
	provider := ai.CreateProvider(ai.CreateProviderOptions{
		ID: model.Provider,
		API: ai.ProviderAPIs(map[ai.API]ai.ProviderStreams{
			ai.APIOpenAIResponses: ai.NewProviderStreams(ai.EraseAPIAdapter(ai.NewOpenAIResponsesAPIAdapter(nil))),
		}),
	})

	stream := provider.Stream(context.Background(), model, ai.Context{}, ai.OpenAIResponsesOptions{})
	_, err := stream.Result(context.Background())
	assertStubAdapterOperation(t, err)
}

func assertStubAdapterOperation(t *testing.T, err error) {
	t.Helper()
	if !errors.Is(err, ai.ErrNotImplemented) {
		t.Fatalf("stub adapter stream error = %v, want ErrNotImplemented", err)
	}
	var notImplemented *ai.NotImplementedError
	if !errors.As(err, &notImplemented) || notImplemented.Module != "ai" || notImplemented.Operation != "APIAdapter.Stream" {
		t.Fatalf("stub adapter stream error = %#v, want ai/APIAdapter.Stream NotImplementedError", err)
	}
}

func TestCreateProviderMissingAPIReturnsTerminalStreamError(t *testing.T) {
	t.Parallel()

	model := modelForProviderRuntime("model-ghost", ai.API("api-ghost"))
	provider := ai.CreateProvider(ai.CreateProviderOptions{
		ID:   "mixed",
		Auth: ai.ProviderAuth{APIKey: ptr(ai.NewStubAPIKeyAuth("Test key"))},
		API: ai.ProviderAPIs(map[ai.API]ai.ProviderStreams{
			ai.API("api-a"): recordingProviderStreams(nil, nil),
		}),
	})

	streams := []*ai.AssistantMessageEventStream{
		provider.Stream(context.Background(), model, ai.Context{}, ai.StreamOptions{}),
		provider.StreamSimple(context.Background(), model, ai.Context{}, ai.SimpleStreamOptions{}),
	}
	for index, stream := range streams {
		wait, cancel := context.WithTimeout(context.Background(), time.Second)
		event, ok, err := stream.Next(wait)
		cancel()
		if err != nil || !ok {
			t.Fatalf("stream %d Next() = (%#v, %t, %v), want terminal error event", index, event, ok, err)
		}
		failed, ok := event.(ai.AssistantMessageErrorEvent)
		if !ok || failed.Type != ai.AssistantMessageEventTypeError || failed.Reason != ai.StopReasonError {
			t.Fatalf("stream %d event = %#v, want AssistantMessageErrorEvent/error", index, event)
		}

		wait, cancel = context.WithTimeout(context.Background(), time.Second)
		result, err := stream.Result(wait)
		cancel()
		if err != nil {
			t.Fatalf("stream %d Result() error = %v, want terminal outcome", index, err)
		}
		errorMessage, ok := result.ErrorMessage.Value()
		if !ok || result.StopReason != ai.StopReasonError || result.Role != ai.MessageRoleAssistant ||
			result.API != model.API || result.Provider != model.Provider || result.Model != model.ID ||
			!strings.Contains(errorMessage, string(ai.ModelsErrorCodeStream)) ||
			!strings.Contains(errorMessage, `no API implementation for "api-ghost"`) || result.Timestamp <= 0 {
			t.Fatalf("stream %d Result() = %#v, want classified terminal model error", index, result)
		}
	}
}

func TestCreateProviderExposesAndDispatchesOnlyDeclaredDeferredCapabilities(t *testing.T) {
	t.Parallel()

	apiA := ai.API("api-a")
	apiB := ai.API("api-b")
	modelA := modelForProviderRuntime("model-a", apiA)
	modelB := modelForProviderRuntime("model-b", apiB)
	handle := ai.DeferredHandle{Provider: modelA.Provider, ModelID: modelA.ID, API: modelA.API, ID: "response-1"}
	fetched := ai.NewAssistantMessageEventStream()
	fetchCalls := 0
	cancelCalls := 0
	streamsA := recordingProviderStreams(nil, nil)
	streamsA.FetchDeferred = func(_ context.Context, model ai.Model, got ai.DeferredHandle, _ ai.DeferredFetchOptions) (*ai.AssistantMessageEventStream, error) {
		fetchCalls++
		if model.ID != modelA.ID || got.ID != handle.ID {
			t.Fatalf("FetchDeferred(model, handle) = (%#v, %#v), want model-a/response-1", model, got)
		}
		return fetched, nil
	}
	streamsA.CancelDeferred = func(_ context.Context, model ai.Model, got ai.DeferredHandle, _ ai.DeferredCancelOptions) error {
		cancelCalls++
		if model.ID != modelA.ID || got.ID != handle.ID {
			t.Fatalf("CancelDeferred(model, handle) = (%#v, %#v), want model-a/response-1", model, got)
		}
		return nil
	}
	provider := ai.CreateProvider(ai.CreateProviderOptions{
		ID:   "mixed",
		Auth: ai.ProviderAuth{APIKey: ptr(ai.NewStubAPIKeyAuth("Test key"))},
		API: ai.ProviderAPIs(map[ai.API]ai.ProviderStreams{
			apiA: streamsA,
			apiB: recordingProviderStreams(nil, nil),
		}),
	})
	if !provider.SupportsFetchDeferred() || !provider.SupportsCancelDeferred() {
		t.Fatalf("Supports deferred = (%t, %t), want both true", provider.SupportsFetchDeferred(), provider.SupportsCancelDeferred())
	}
	if got, err := provider.FetchDeferred(context.Background(), modelA, handle, ai.DeferredFetchOptions{}); err != nil || got != fetched {
		t.Fatalf("FetchDeferred(api-a) = (%p, %v), want (%p, nil)", got, err, fetched)
	}
	if err := provider.CancelDeferred(context.Background(), modelA, handle, ai.DeferredCancelOptions{}); err != nil {
		t.Fatalf("CancelDeferred(api-a) error = %v", err)
	}
	if fetchCalls != 1 || cancelCalls != 1 {
		t.Fatalf("deferred calls = fetch %d/cancel %d, want one each", fetchCalls, cancelCalls)
	}

	if stream, err := provider.FetchDeferred(context.Background(), modelB, handle, ai.DeferredFetchOptions{}); stream != nil {
		t.Fatalf("FetchDeferred(api-b) stream = %p, want nil", stream)
	} else {
		assertProviderRuntimeModelsError(t, err, ai.ModelsErrorCodeProvider)
	}
	assertProviderRuntimeModelsError(
		t,
		provider.CancelDeferred(context.Background(), modelB, handle, ai.DeferredCancelOptions{}),
		ai.ModelsErrorCodeProvider,
	)

	undeclared := ai.CreateProvider(ai.CreateProviderOptions{
		ID:   "static",
		Auth: ai.ProviderAuth{APIKey: ptr(ai.NewStubAPIKeyAuth("Test key"))},
		API:  ai.SingleProviderAPI(recordingProviderStreams(nil, nil)),
	})
	if undeclared.SupportsFetchDeferred() || undeclared.SupportsCancelDeferred() {
		t.Fatal("provider without deferred functions reports deferred support")
	}

	stubbed := ai.CreateProvider(ai.CreateProviderOptions{
		ID:   "stubbed",
		Auth: ai.ProviderAuth{APIKey: ptr(ai.NewStubAPIKeyAuth("Test key"))},
		API:  ai.SingleProviderAPI(ai.NewStubProviderStreams()),
	})
	if stubbed.SupportsFetchDeferred() || stubbed.SupportsCancelDeferred() {
		t.Fatal("stub bundle must not advertise deferred capabilities")
	}
	if stream, err := stubbed.FetchDeferred(context.Background(), modelA, handle, ai.DeferredFetchOptions{}); stream != nil || !errors.Is(err, ai.ErrNotImplemented) {
		t.Fatalf("stub FetchDeferred() = (%p, %v), want nil and ErrNotImplemented", stream, err)
	}
	if err := stubbed.CancelDeferred(context.Background(), modelA, handle, ai.DeferredCancelOptions{}); !errors.Is(err, ai.ErrNotImplemented) {
		t.Fatalf("stub CancelDeferred() error = %v, want ErrNotImplemented", err)
	}

	invalidStreams := recordingProviderStreams(nil, nil)
	invalidStreams.FetchDeferred = func(context.Context, ai.Model, ai.DeferredHandle, ai.DeferredFetchOptions) (*ai.AssistantMessageEventStream, error) {
		return nil, nil
	}
	invalid := ai.CreateProvider(ai.CreateProviderOptions{
		ID:   "invalid",
		Auth: ai.ProviderAuth{APIKey: ptr(ai.NewStubAPIKeyAuth("Test key"))},
		API:  ai.SingleProviderAPI(invalidStreams),
	})
	if stream, err := invalid.FetchDeferred(context.Background(), modelA, handle, ai.DeferredFetchOptions{}); stream != nil {
		t.Fatalf("invalid FetchDeferred() stream = %p, want nil", stream)
	} else {
		assertProviderRuntimeModelsError(t, err, ai.ModelsErrorCodeStream)
		if !errors.Is(err, ai.ErrEventStreamInvariant) {
			t.Fatalf("invalid FetchDeferred() error = %v, want ErrEventStreamInvariant", err)
		}
	}
}

func TestCreateProviderRefreshRestoresThenPublishesCloneSafeDynamicOverlay(t *testing.T) {
	t.Parallel()

	apiID := ai.API("api-a")
	baselineA := modelForProviderRuntime("model-a", apiID)
	baselineA.Name = "baseline-a"
	baselineB := modelForProviderRuntime("model-b", apiID)
	baselineB.Name = "baseline-b"
	storedA := modelForProviderRuntime("model-a", apiID)
	storedA.Name = "stored-a"
	storedExtra := modelForProviderRuntime("stored-extra", apiID)
	foreign := modelForProviderRuntime("foreign", apiID)
	foreign.Provider = "another-provider"
	stored := &ai.ModelsStoreEntry{Models: []ai.Model{storedA, storedExtra, foreign}}
	fetchedA := modelForProviderRuntime("model-a", apiID)
	fetchedA.Name = "fetched-a"
	fetchedExtra := modelForProviderRuntime("fetched-extra", apiID)
	fetched := []ai.Model{fetchedA, fetchedExtra}
	fetchCalls := 0
	var provider *ai.CreatedProvider
	provider = ai.CreateProvider(ai.CreateProviderOptions{
		ID:     "mixed",
		Auth:   ai.ProviderAuth{APIKey: ptr(ai.NewStubAPIKeyAuth("Test key"))},
		Models: []ai.Model{baselineA, baselineB},
		FetchModels: func(refresh ai.RefreshModelsContext) ([]ai.Model, error) {
			fetchCalls++
			force, forced := refresh.Force.Value()
			if !refresh.AllowNetwork || !forced || !force || refresh.Context == nil {
				t.Fatalf("FetchModels context = %#v, want online forced concrete context", refresh)
			}
			if refresh.Stored == nil || len(refresh.Stored.Models) == 0 {
				t.Fatal("FetchModels did not receive stored snapshot")
			}
			restored := provider.GetModels()
			if len(restored) != 3 || restored[0].Name != "stored-a" || restored[1].ID != "model-b" || restored[2].ID != "stored-extra" {
				t.Fatalf("models before network fetch = %#v, want restored provider-only overlay", restored)
			}
			refresh.Stored.Models[0].Name = "mutated by fetch"
			return fetched, nil
		},
		API: ai.SingleProviderAPI(recordingProviderStreams(nil, nil)),
	})

	publications := 0
	var persisted *ai.ModelsStoreEntry
	before := time.Now().UnixMilli()
	err := provider.RefreshModels(ai.RefreshModelsContext{
		Stored:       stored,
		AllowNetwork: true,
		Force:        ai.Some(true),
		Publish: func(publication ai.ModelsPublication) (bool, error) {
			publications++
			if publications == 1 {
				if publication.Persist.IsSet() {
					t.Fatal("stored restoration unexpectedly requested persistence")
				}
			} else {
				var ok bool
				persisted, ok = publication.Persist.Value()
				if !ok || persisted == nil {
					t.Fatalf("network publication Persist = %#v, want write entry", publication.Persist)
				}
			}
			if publication.Update != nil {
				publication.Update()
			}
			return true, nil
		},
	})
	after := time.Now().UnixMilli()
	if err != nil {
		t.Fatalf("RefreshModels() error = %v", err)
	}
	if fetchCalls != 1 || publications != 2 {
		t.Fatalf("RefreshModels calls = fetch %d/publication %d, want 1/2", fetchCalls, publications)
	}
	if stored.Models[0].Name != "stored-a" {
		t.Fatalf("FetchModels mutated caller's stored snapshot: %#v", stored.Models[0])
	}
	checkedAt, ok := persisted.CheckedAt.Value()
	if !ok || checkedAt < before || checkedAt > after {
		t.Fatalf("persisted CheckedAt = (%d, %t), want UnixMilli in [%d,%d]", checkedAt, ok, before, after)
	}
	if len(persisted.Models) != 2 || persisted.Models[0].Name != "fetched-a" {
		t.Fatalf("persisted models = %#v, want fetched snapshot", persisted.Models)
	}

	fetched[0].Name = "mutated fetch result"
	persisted.Models[0].Name = "mutated publication"
	models := provider.GetModels()
	if len(models) != 3 || models[0].ID != "model-a" || models[0].Name != "fetched-a" ||
		models[1].ID != "model-b" || models[2].ID != "fetched-extra" {
		t.Fatalf("GetModels() after refresh = %#v, want replacement in place plus appended dynamic model", models)
	}
}

func TestCreateProviderRefreshSkipsFetchOfflineCanceledOrRejectedRestore(t *testing.T) {
	t.Parallel()

	fetchCalls := 0
	newProvider := func() *ai.CreatedProvider {
		return ai.CreateProvider(ai.CreateProviderOptions{
			ID:   "mixed",
			Auth: ai.ProviderAuth{APIKey: ptr(ai.NewStubAPIKeyAuth("Test key"))},
			FetchModels: func(ai.RefreshModelsContext) ([]ai.Model, error) {
				fetchCalls++
				return []ai.Model{modelForProviderRuntime("network", "api-a")}, nil
			},
			API: ai.SingleProviderAPI(recordingProviderStreams(nil, nil)),
		})
	}
	publish := func(publication ai.ModelsPublication) (bool, error) {
		if publication.Update != nil {
			publication.Update()
		}
		return true, nil
	}
	offline := ai.RefreshModelsContext{AllowNetwork: false, Publish: publish}
	if offline.Force.IsSet() {
		t.Fatal("offline refresh Force is set, want absent")
	}
	if err := newProvider().RefreshModels(offline); err != nil {
		t.Fatalf("offline RefreshModels() error = %v", err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := newProvider().RefreshModels(ai.RefreshModelsContext{Context: canceled, AllowNetwork: true, Publish: publish}); err != nil {
		t.Fatalf("canceled RefreshModels() error = %v", err)
	}
	stored := &ai.ModelsStoreEntry{Models: []ai.Model{modelForProviderRuntime("stored", "api-a")}}
	if err := newProvider().RefreshModels(ai.RefreshModelsContext{
		Stored:       stored,
		AllowNetwork: true,
		Publish:      func(ai.ModelsPublication) (bool, error) { return false, nil },
	}); err != nil {
		t.Fatalf("rejected-restore RefreshModels() error = %v", err)
	}
	if fetchCalls != 0 {
		t.Fatalf("offline/canceled/rejected refresh called FetchModels %d times, want zero", fetchCalls)
	}
}

func TestCreateProviderUnsupportedRefreshIsStructuredNotImplemented(t *testing.T) {
	t.Parallel()

	provider := ai.CreateProvider(ai.CreateProviderOptions{
		ID:   "static",
		Auth: ai.ProviderAuth{APIKey: ptr(ai.NewStubAPIKeyAuth("Test key"))},
		API:  ai.SingleProviderAPI(ai.NewStubProviderStreams()),
	})
	if provider.SupportsRefreshModels() {
		t.Fatal("static provider unexpectedly reports refresh support")
	}

	err := provider.RefreshModels(ai.RefreshModelsContext{})
	assertProviderRuntimeModelsError(t, err, ai.ModelsErrorCodeProvider)
	if !errors.Is(err, ai.ErrNotImplemented) {
		t.Fatalf("RefreshModels() error = %v, want ErrNotImplemented", err)
	}
	var notImplemented *ai.NotImplementedError
	if !errors.As(err, &notImplemented) || notImplemented.Operation != "Provider.RefreshModels" {
		t.Fatalf("RefreshModels() error = %#v, want Provider.RefreshModels NotImplementedError", err)
	}
}

func assertProviderRuntimeModelsError(t *testing.T, err error, code ai.ModelsErrorCode) {
	t.Helper()
	var modelsError *ai.ModelsError
	if !errors.As(err, &modelsError) || modelsError.Code != code {
		t.Fatalf("error = %v, want ModelsError code %q", err, code)
	}
}

func modelForProviderRuntime(id string, apiID ai.API) ai.Model {
	return ai.Model{
		ID:       id,
		Name:     id,
		API:      apiID,
		Provider: ai.ProviderID("mixed"),
		BaseURL:  "https://example.test/v1",
	}
}

func recordingProviderStreams(
	stream func(context.Context, ai.Model, ai.Context, ai.StreamOptions) *ai.AssistantMessageEventStream,
	simple func(context.Context, ai.Model, ai.Context, ai.SimpleStreamOptions) *ai.AssistantMessageEventStream,
) ai.ProviderStreams {
	if stream == nil {
		stream = func(context.Context, ai.Model, ai.Context, ai.StreamOptions) *ai.AssistantMessageEventStream {
			return ai.NewAssistantMessageEventStream()
		}
	}
	if simple == nil {
		simple = func(context.Context, ai.Model, ai.Context, ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
			return ai.NewAssistantMessageEventStream()
		}
	}
	return ai.ProviderStreams{Stream: stream, StreamSimple: simple}
}

func ptr[T any](value T) *T { return &value }

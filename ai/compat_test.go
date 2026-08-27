package ai_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nankedr/pig/ai"
)

var (
	_ ai.APIStreamFunction[ai.AnthropicOptions]            = ai.StreamAnthropic            // upstream: streamAnthropic
	_ ai.APIStreamFunction[ai.AzureOpenAIResponsesOptions] = ai.StreamAzureOpenAIResponses // upstream: streamAzureOpenAIResponses
	_ ai.APIStreamFunction[ai.GoogleOptions]               = ai.StreamGoogle               // upstream: streamGoogle
	_ ai.APIStreamFunction[ai.GoogleVertexOptions]         = ai.StreamGoogleVertex         // upstream: streamGoogleVertex
	_ ai.APIStreamFunction[ai.MistralOptions]              = ai.StreamMistral              // upstream: streamMistral
	_ ai.APIStreamFunction[ai.OpenAICodexResponsesOptions] = ai.StreamOpenAICodexResponses // upstream: streamOpenAICodexResponses
	_ ai.APIStreamFunction[ai.OpenAICompletionsOptions]    = ai.StreamOpenAICompletions    // upstream: streamOpenAICompletions
	_ ai.APIStreamFunction[ai.OpenAIResponsesOptions]      = ai.StreamOpenAIResponses      // upstream: streamOpenAIResponses

	_ ai.ProviderSimpleStreamFunction = ai.StreamSimpleAnthropic            // upstream: streamSimpleAnthropic
	_ ai.ProviderSimpleStreamFunction = ai.StreamSimpleAzureOpenAIResponses // upstream: streamSimpleAzureOpenAIResponses
	_ ai.ProviderSimpleStreamFunction = ai.StreamSimpleGoogle               // upstream: streamSimpleGoogle
	_ ai.ProviderSimpleStreamFunction = ai.StreamSimpleGoogleVertex         // upstream: streamSimpleGoogleVertex
	_ ai.ProviderSimpleStreamFunction = ai.StreamSimpleMistral              // upstream: streamSimpleMistral
	_ ai.ProviderSimpleStreamFunction = ai.StreamSimpleOpenAICodexResponses // upstream: streamSimpleOpenAICodexResponses
	_ ai.ProviderSimpleStreamFunction = ai.StreamSimpleOpenAICompletions    // upstream: streamSimpleOpenAICompletions
	_ ai.ProviderSimpleStreamFunction = ai.StreamSimpleOpenAIResponses      // upstream: streamSimpleOpenAIResponses

	_ ai.CompatAPIStreamFunction       = nil                               // upstream: ApiStreamFunction
	_ ai.CompatAPISimpleStreamFunction = nil                               // upstream: ApiStreamSimpleFunction
	_                                  = ai.APIProvider{}                  // upstream: ApiProvider
	_                                  = ai.RegisterAPIProvider            // upstream: registerApiProvider
	_                                  = ai.GetAPIProvider                 // upstream: getApiProvider
	_                                  = ai.GetAPIProviders                // upstream: getApiProviders
	_                                  = ai.UnregisterAPIProviders         // upstream: unregisterApiProviders
	_                                  = ai.RegisterBuiltinAPIProviders    // upstream: registerBuiltInApiProviders
	_                                  = ai.ResetAPIProviders              // upstream: resetApiProviders
	_                                  = ai.Stream                         // upstream: stream
	_                                  = ai.Complete                       // upstream: complete
	_                                  = ai.StreamSimple                   // upstream: streamSimple
	_                                  = ai.CompleteSimple                 // upstream: completeSimple
	_                                  = ai.GetModel                       // upstream: getModel
	_                                  = ai.GetModels                      // upstream: getModels
	_                                  = ai.GetProviders                   // upstream: getProviders
	_                                  = ai.RegisterFauxProvider           // upstream: registerFauxProvider
	_ ai.SessionResourceCleanup        = nil                               // upstream: SessionResourceCleanup
	_                                  = ai.RegisterSessionResourceCleanup // upstream: registerSessionResourceCleanup
	_                                  = ai.CleanupSessionResources        // upstream: cleanupSessionResources
)

func TestCompatAPIRegistrySupportsPureInMemoryLifecycle(t *testing.T) {
	if err := ai.ResetAPIProviders(); err != nil {
		t.Fatalf("ResetAPIProviders error = %v", err)
	}
	t.Cleanup(func() { _ = ai.ResetAPIProviders() })

	if providers := ai.GetAPIProviders(); len(providers) != 10 {
		t.Fatalf("builtin API provider count = %d, want 10", len(providers))
	}
	if err := ai.RegisterBuiltinAPIProviders(); err != nil {
		t.Fatalf("RegisterBuiltinAPIProviders error = %v", err)
	}
	builtin, ok := ai.GetAPIProvider(ai.APIOpenAIResponses)
	if !ok {
		t.Fatal("builtin OpenAI Responses API is not registered")
	}
	builtinMismatch := builtin.Stream(context.Background(), ai.Model{API: "different-api"}, ai.Context{}, ai.StreamOptions{})
	if _, err := builtinMismatch.Result(context.Background()); !errors.Is(err, ai.ErrEventStreamInvariant) {
		t.Fatalf("builtin API mismatch error = %v, want ErrEventStreamInvariant", err)
	}
	ai.UnregisterAPIProviders("")
	if providers := ai.GetAPIProviders(); len(providers) != 10 {
		t.Fatalf("empty source removed %d builtin providers", 10-len(providers))
	}
	emptySource := customCompatProvider("empty-source-api")
	if err := ai.RegisterAPIProvider(emptySource, ""); err != nil {
		t.Fatalf("RegisterAPIProvider(empty source) error = %v", err)
	}
	ai.UnregisterAPIProviders("")
	if _, ok := ai.GetAPIProvider(emptySource.API); ok {
		t.Fatal("explicit empty source provider was not removed")
	}
	custom := ai.APIProvider{
		API: "custom-api",
		Stream: func(context.Context, ai.Model, ai.Context, ai.ProviderStreamOptions) *ai.AssistantMessageEventStream {
			return ai.NewAssistantMessageEventStream()
		},
		StreamSimple: func(context.Context, ai.Model, ai.Context, ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
			return ai.NewAssistantMessageEventStream()
		},
	}
	if err := ai.RegisterAPIProvider(custom, "extension-one"); err != nil {
		t.Fatalf("RegisterAPIProvider error = %v", err)
	}
	got, ok := ai.GetAPIProvider(custom.API)
	if !ok || got.API != custom.API || got.Stream == nil || got.StreamSimple == nil {
		t.Fatalf("GetAPIProvider() = (%#v, %t)", got, ok)
	}
	mismatched := got.Stream(context.Background(), ai.Model{API: "different-api"}, ai.Context{}, ai.StreamOptions{})
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := mismatched.Result(ctx); !errors.Is(err, ai.ErrEventStreamInvariant) {
		t.Fatalf("registered API mismatch error = %v, want ErrEventStreamInvariant", err)
	}
	ai.UnregisterAPIProviders("extension-one")
	if _, ok := ai.GetAPIProvider(custom.API); ok {
		t.Fatal("UnregisterAPIProviders retained custom provider")
	}
}

func customCompatProvider(api ai.API) ai.APIProvider {
	return ai.APIProvider{
		API: api,
		Stream: func(context.Context, ai.Model, ai.Context, ai.ProviderStreamOptions) *ai.AssistantMessageEventStream {
			return ai.NewAssistantMessageEventStream()
		},
		StreamSimple: func(context.Context, ai.Model, ai.Context, ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
			return ai.NewAssistantMessageEventStream()
		},
	}
}

func TestCompatExecutionStubsDoNotConsultRegistryEnvironmentOrHooks(t *testing.T) {
	apiCalls := 0
	if err := ai.RegisterAPIProvider(ai.APIProvider{
		API: "poison-api",
		Stream: func(context.Context, ai.Model, ai.Context, ai.ProviderStreamOptions) *ai.AssistantMessageEventStream {
			apiCalls++
			return ai.NewAssistantMessageEventStream()
		},
		StreamSimple: func(context.Context, ai.Model, ai.Context, ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
			apiCalls++
			return ai.NewAssistantMessageEventStream()
		},
	}, "poison"); err != nil {
		t.Fatalf("RegisterAPIProvider error = %v", err)
	}
	t.Cleanup(func() {
		ai.UnregisterAPIProviders("poison")
	})

	hooks := 0
	model := ai.Model{API: "poison-api", Provider: ai.ProviderIDOpenAI}
	stream := ai.Stream(context.Background(), model, ai.Context{}, ai.StreamOptions{
		ProviderRequestOptions: poisonedRequestOptions(&hooks),
	})
	if _, err := stream.Result(context.Background()); !errors.Is(err, ai.ErrNotImplemented) {
		t.Fatalf("Stream error = %v, want ErrNotImplemented", err)
	}
	stream = ai.StreamSimple(context.Background(), model, ai.Context{})
	if _, err := stream.Result(context.Background()); !errors.Is(err, ai.ErrNotImplemented) {
		t.Fatalf("StreamSimple error = %v, want ErrNotImplemented", err)
	}
	if _, err := ai.Complete(context.Background(), model, ai.Context{}); !errors.Is(err, ai.ErrNotImplemented) {
		t.Fatalf("Complete error = %v, want ErrNotImplemented", err)
	}
	if _, err := ai.CompleteSimple(context.Background(), model, ai.Context{}); !errors.Is(err, ai.ErrNotImplemented) {
		t.Fatalf("CompleteSimple error = %v, want ErrNotImplemented", err)
	}
	if apiCalls != 0 || hooks != 0 {
		t.Fatalf("compat stub side effects = API %d/hooks %d, want zero", apiCalls, hooks)
	}
}

func TestCompatCatalogAliasesAndDeferredEntriesAreExplicit(t *testing.T) {
	t.Parallel()

	if providers := ai.GetProviders(); len(providers) != 39 {
		t.Fatalf("GetProviders count = %d, want 39 static providers", len(providers))
	}
	if models := ai.GetModels(ai.ProviderIDOpenAI); len(models) != 0 {
		t.Fatalf("GetModels = %#v, want M0-unloaded catalog", models)
	}
	if _, ok := ai.GetModel(ai.ProviderIDOpenAI, "missing"); ok {
		t.Fatal("GetModel found a model before runtime catalog loading")
	}
	if _, err := ai.RegisterFauxProvider(); !errors.Is(err, ai.ErrNotImplemented) {
		t.Fatalf("RegisterFauxProvider error = %v, want ErrNotImplemented", err)
	}

	cleanups := 0
	unregister, err := ai.RegisterSessionResourceCleanup(func(...string) { cleanups++ })
	if unregister != nil || !errors.Is(err, ai.ErrNotImplemented) {
		t.Fatalf("RegisterSessionResourceCleanup returned unregister=%t, error=%v; want false/ErrNotImplemented", unregister != nil, err)
	}
	if err := ai.CleanupSessionResources("session"); !errors.Is(err, ai.ErrNotImplemented) {
		t.Fatalf("CleanupSessionResources error = %v, want ErrNotImplemented", err)
	}
	if cleanups != 0 {
		t.Fatalf("cleanup callback invoked %d times, want zero", cleanups)
	}
}

func TestDeprecatedStreamAliasesRemainExplicitStubs(t *testing.T) {
	t.Parallel()

	stream := ai.StreamAnthropic(context.Background(), ai.Model{}, ai.Context{}, ai.AnthropicOptions{})
	if _, err := stream.Result(context.Background()); !errors.Is(err, ai.ErrNotImplemented) {
		t.Fatalf("StreamAnthropic error = %v, want ErrNotImplemented", err)
	}
	stream = ai.StreamSimpleOpenAIResponses(context.Background(), ai.Model{}, ai.Context{}, ai.SimpleStreamOptions{})
	if _, err := stream.Result(context.Background()); !errors.Is(err, ai.ErrNotImplemented) {
		t.Fatalf("StreamSimpleOpenAIResponses error = %v, want ErrNotImplemented", err)
	}
}

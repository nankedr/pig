package ai_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/telemetry"
)

func TestDeferredModelsPreservesAuthenticatedRequestOptions(t *testing.T) {
	core, _ := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{API: "faux:dispatch", Provider: "dispatch"})
	model, _ := core.GetModel()
	key, header, endpoint, env := "credential", "transformed", "https://provider.invalid", "override"
	requestCtx := context.WithValue(context.Background(), struct{}{}, "request")
	trace := telemetry.NOOPTelemetryContext
	hooks := 0
	request := ai.ProviderRequestOptions{
		TelemetryContext: trace, Env: ai.ProviderEnv{"ENV": env},
		OnResponse: func(ctx context.Context, response ai.ProviderResponse, got ai.Model) error {
			hooks++
			if ctx != requestCtx || response.Status != 200 || got.BaseURL != endpoint {
				t.Error("response hook lost its request context or resolved model")
			}
			return nil
		},
	}
	check := func(ctx context.Context, got ai.Model, options ai.ProviderRequestOptions) {
		if ctx != requestCtx || got.BaseURL != endpoint || options.APIKey == nil || *options.APIKey != key ||
			options.Headers["test"] == nil || *options.Headers["test"] != header || options.Env["ENV"] != env || options.TelemetryContext != trace || options.OnResponse == nil {
			t.Errorf("request options lost in dispatch: model=%#v options=%#v", got, options)
		}
	}
	provider := ai.CreateProvider(ai.CreateProviderOptions{
		ID: model.Provider, Models: []ai.Model{model},
		Auth: ai.ProviderAuth{APIKey: &ai.APIKeyAuth{Resolve: func(context.Context, ai.APIKeyResolveInput) (ai.Optional[ai.AuthResult], error) {
			return ai.Some(ai.AuthResult{Auth: ai.ModelAuth{APIKey: ai.Some(key), BaseURL: ai.Some(endpoint)}}), nil
		}}},
		API: ai.ProviderAPIs(map[ai.API]ai.ProviderStreams{model.API: {
			StreamSimple: func(ctx context.Context, got ai.Model, input ai.Context, options ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
				check(ctx, got, options.ProviderRequestOptions)
				return core.StreamSimple(ctx, got, input, options)
			},
			FetchDeferred: func(ctx context.Context, got ai.Model, handle ai.DeferredHandle, options ai.DeferredFetchOptions) (*ai.AssistantMessageEventStream, error) {
				check(ctx, got, options.ProviderRequestOptions)
				if options.WaitMS == nil || *options.WaitMS != 0 {
					t.Error("explicit zero wait was lost")
				}
				return core.FetchDeferred(ctx, got, handle, options)
			},
			CancelDeferred: func(ctx context.Context, got ai.Model, handle ai.DeferredHandle, options ai.DeferredCancelOptions) error {
				check(ctx, got, options)
				return core.CancelDeferred(ctx, got, handle, options)
			},
		}}),
	})
	models := ai.CreateModels()
	models.SetProvider(provider)
	response, _ := ai.FauxAssistantMessage(ai.FauxAssistantText("ready"))
	core.SetResponses([]ai.FauxResponseStep{response})
	transforms := ai.ModelsRequestTransforms{TransformHeaders: func(ctx context.Context, headers ai.ProviderHeaders) (ai.ProviderHeaders, error) {
		if ctx != requestCtx {
			t.Error("transform lost request context")
		}
		headers["test"] = &header
		return headers, nil
	}}
	submitted, err := models.CompleteSimple(requestCtx, model, ai.Context{}, ai.ModelsSimpleStreamOptions{
		SimpleStreamOptions:     ai.SimpleStreamOptions{StreamOptions: ai.StreamOptions{ProviderRequestOptions: request}, Deferred: ai.DeferredBoolean{Enabled: true}},
		ModelsRequestTransforms: transforms,
	})
	if err != nil || submitted.StopReason != ai.StopReasonDeferred {
		t.Fatalf("submission = %#v, %v", submitted, err)
	}
	handle, _ := submitted.Deferred.Value()
	wait := int64(0)
	final, err := models.FetchDeferred(requestCtx, model, handle, ai.ModelsDeferredFetchOptions{
		DeferredFetchOptions: ai.DeferredFetchOptions{ProviderRequestOptions: request, WaitMS: &wait}, ModelsRequestTransforms: transforms,
	})
	if err != nil || final.StopReason != ai.StopReasonStop {
		t.Fatalf("fetch = %#v, %v", final, err)
	}
	if err := models.CancelDeferred(requestCtx, model, handle, ai.ModelsDeferredCancelOptions{DeferredCancelOptions: request, ModelsRequestTransforms: transforms}); err != nil || hooks != 3 {
		t.Fatalf("cancel = %v, response hooks = %d", err, hooks)
	}
	unknownAPI := model
	unknownAPI.API = "missing"
	if _, err := models.FetchDeferred(requestCtx, unknownAPI, handle); !errors.Is(err, ai.ErrNotImplemented) {
		t.Fatalf("unsupported API fetch = %v", err)
	}
	if err := models.CancelDeferred(requestCtx, unknownAPI, handle); !errors.Is(err, ai.ErrNotImplemented) {
		t.Fatalf("unsupported API cancel = %v", err)
	}
}

func TestDeferredCancelDuringStreamingPreservesPartialAndTerminalOrder(t *testing.T) {
	rate := 100.0
	core, _ := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{TokensPerSecond: &rate})
	model, _ := core.GetModel()
	response, _ := ai.FauxAssistantMessage(ai.FauxAssistantText(strings.Repeat("content ", 100)))
	core.SetResponses([]ai.FauxResponseStep{response})
	ctx := context.Background()
	submitted, _ := core.StreamSimple(ctx, model, ai.Context{}, ai.SimpleStreamOptions{Deferred: ai.DeferredBoolean{Enabled: true}}).Result(ctx)
	handle, _ := submitted.Deferred.Value()
	stream, _ := core.FetchDeferred(ctx, model, handle, ai.DeferredFetchOptions{})
	seen, terminals := false, 0
	for {
		event, ok, err := stream.Next(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			break
		}
		if terminals > 0 {
			t.Fatal("event published after terminal")
		}
		switch event.(type) {
		case ai.AssistantMessageTextDeltaEvent:
			if !seen {
				seen = true
				if err := core.CancelDeferred(ctx, model, handle, ai.DeferredCancelOptions{}); err != nil {
					t.Fatal(err)
				}
			}
		case ai.AssistantMessageDoneEvent:
			t.Fatal("success published after cancellation")
		case ai.AssistantMessageErrorEvent:
			terminals++
		}
	}
	result, _ := stream.Result(ctx)
	repeated, _ := stream.Result(ctx)
	if !seen || terminals != 1 || result.StopReason != ai.StopReasonAborted || len(result.Content) == 0 || !reflect.DeepEqual(result, repeated) {
		t.Fatalf("cancelled stream = %#v, terminal count = %d", result, terminals)
	}
}

func TestDeferredConcurrentFetchCancelKeepsEmittedContent(t *testing.T) {
	ctx := context.Background()
	for range 256 {
		core, _ := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
		model, _ := core.GetModel()
		response, _ := ai.FauxAssistantMessage(ai.FauxAssistantText("ready"))
		core.SetResponses([]ai.FauxResponseStep{response})
		submitted, _ := core.StreamSimple(ctx, model, ai.Context{}, ai.SimpleStreamOptions{Deferred: ai.DeferredBoolean{Enabled: true}}).Result(ctx)
		handle, _ := submitted.Deferred.Value()
		stream, _ := core.FetchDeferred(ctx, model, handle, ai.DeferredFetchOptions{})
		for {
			event, ok, err := stream.Next(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if !ok {
				break
			}
			if _, ok := event.(ai.AssistantMessageTextEndEvent); ok {
				if err := core.CancelDeferred(ctx, model, handle, ai.DeferredCancelOptions{}); err != nil {
					t.Fatal(err)
				}
			}
		}
		result, err := stream.Result(ctx)
		if err != nil || len(result.Content) != 1 || result.Content[0].(ai.TextContent).Text != "ready" {
			t.Fatalf("terminal race lost emitted text: %#v, %v", result, err)
		}
	}
}

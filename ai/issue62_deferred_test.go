package ai_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nankedr/pig/ai"
)

func TestDeferredLifecycleThroughModels(t *testing.T) {
	pending, poll := 2, int64(0)
	faux, err := ai.NewFauxProvider(ai.RegisterFauxProviderOptions{Deferred: &ai.FauxDeferredOptions{PendingFetches: &pending, PollAfterMS: &poll}})
	if err != nil {
		t.Fatal(err)
	}
	if !faux.Provider.SupportsFetchDeferred() || !faux.Provider.SupportsCancelDeferred() {
		t.Fatal("Faux must advertise its implemented deferred capabilities")
	}
	models := ai.CreateModels()
	models.SetProvider(faux.Provider)
	model, _ := faux.GetModel()
	response, _ := ai.FauxAssistantMessage(ai.FauxAssistantText("ready"), ai.FauxAssistantMessageOptions{Timestamp: ai.Some(int64(42))})
	calls := 0
	faux.SetResponses([]ai.FauxResponseStep{ai.FauxResponseFactory(func(_ ai.Context, options *ai.SimpleStreamOptions, _ *ai.FauxProviderState, _ ai.Model) (ai.AssistantMessage, error) {
		calls++
		if options.Deferred != nil || options.OnResponse != nil {
			t.Error("final resolution retained submission-only options")
		}
		return response, nil
	}), response})
	ctx := context.Background()
	submitted, err := models.CompleteSimple(ctx, model, ai.Context{}, ai.ModelsSimpleStreamOptions{SimpleStreamOptions: ai.SimpleStreamOptions{Deferred: ai.DeferredBoolean{Enabled: true}}})
	if err != nil || submitted.StopReason != ai.StopReasonDeferred {
		t.Fatalf("submission = %#v, %v", submitted, err)
	}
	handle, ok := submitted.Deferred.Value()
	if !ok || handle.ID == "" || handle.Provider != model.Provider || handle.ModelID != model.ID || handle.API != model.API || handle.PollAfterMS != ai.Some(int64(0)) {
		t.Fatalf("incomplete handle: %#v", handle)
	}
	if calls != 0 || faux.GetPendingResponseCount() != 1 {
		t.Fatal("submission must reserve exactly one response without resolving it")
	}
	for range pending {
		result, err := models.FetchDeferred(ctx, model, handle)
		if err != nil || result.StopReason != ai.StopReasonDeferred || result.Deferred != submitted.Deferred || calls != 0 {
			t.Fatalf("pending = %#v, %v; factory calls = %d", result, err, calls)
		}
	}
	final, err := models.FetchDeferred(ctx, model, handle)
	if err != nil || final.StopReason != ai.StopReasonStop || final.Content[0].(ai.TextContent).Text != "ready" || calls != 1 {
		t.Fatalf("final = %#v, %v; factory calls = %d", final, err, calls)
	}
	again, err := models.FetchDeferred(ctx, model, handle)
	if err != nil || !reflect.DeepEqual(final, again) || calls != 1 || faux.GetPendingResponseCount() != 1 {
		t.Fatalf("repeated final = %#v, %v; factory calls = %d", again, err, calls)
	}
	syncResult, err := models.CompleteSimple(ctx, model, ai.Context{})
	if err != nil || syncResult.StopReason != ai.StopReasonStop || syncResult.Deferred.IsSet() {
		t.Fatalf("synchronous request = %#v, %v", syncResult, err)
	}
}

func TestDeferredRejectsForeignAndMismatchedHandles(t *testing.T) {
	core, _ := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{API: "faux:handles", Provider: "owner"})
	other, _ := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{API: core.API, Provider: core.Provider})
	model, _ := core.GetModel()
	response, _ := ai.FauxAssistantMessage(ai.FauxAssistantText("untouched"))
	core.SetResponses([]ai.FauxResponseStep{response})
	ctx := context.Background()
	submitted, _ := core.StreamSimple(ctx, model, ai.Context{}, ai.SimpleStreamOptions{Deferred: ai.DeferredBoolean{Enabled: true}}).Result(ctx)
	handle, _ := submitted.Deferred.Value()
	for _, field := range []string{"id", "provider", "model", "api", "request-model", "other-core"} {
		t.Run(field, func(t *testing.T) {
			bad, request, target := handle, model, core
			switch field {
			case "id":
				bad.ID = "unknown"
			case "provider":
				bad.Provider = "other"
			case "model":
				bad.ModelID = "other"
			case "api":
				bad.API = "other"
			case "request-model":
				request.ID = "other"
			case "other-core":
				target = other
			}
			if err := target.CancelDeferred(ctx, request, bad, ai.DeferredCancelOptions{}); err == nil || !strings.Contains(err.Error(), "Unknown faux deferred response") {
				t.Fatalf("invalid cancellation = %v", err)
			}
			stream, _ := target.FetchDeferred(ctx, request, bad, ai.DeferredFetchOptions{})
			result, err := stream.Result(ctx)
			if err != nil || result.StopReason != ai.StopReasonError {
				t.Fatalf("invalid fetch = %#v, %v", result, err)
			}
		})
	}
	stream, _ := core.FetchDeferred(ctx, model, handle, ai.DeferredFetchOptions{})
	final, _ := stream.Result(ctx)
	if final.StopReason != ai.StopReasonStop || len(core.State.CancelledDeferred) != 0 {
		t.Fatal("invalid cancellation affected the valid handle")
	}
	if err := core.CancelDeferred(ctx, model, handle, ai.DeferredCancelOptions{}); err != nil {
		t.Fatal(err)
	}
	stream, _ = core.FetchDeferred(ctx, model, handle, ai.DeferredFetchOptions{})
	result, _ := stream.Result(ctx)
	if result.StopReason != ai.StopReasonError || len(core.State.CancelledDeferred) != 1 {
		t.Fatalf("fetch after final cancellation = %#v", result)
	}
}

func TestDeferredHookAndFactoryFailuresRemainDistinct(t *testing.T) {
	core, _ := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
	model, _ := core.GetModel()
	var calls atomic.Int32
	core.SetResponses([]ai.FauxResponseStep{ai.FauxResponseFactory(func(ai.Context, *ai.SimpleStreamOptions, *ai.FauxProviderState, ai.Model) (ai.AssistantMessage, error) {
		calls.Add(1)
		return ai.AssistantMessage{}, errors.New("script failure")
	})})
	ctx := context.Background()
	submitted, _ := core.StreamSimple(ctx, model, ai.Context{}, ai.SimpleStreamOptions{Deferred: ai.DeferredBoolean{Enabled: true}}).Result(ctx)
	handle, _ := submitted.Deferred.Value()
	hookError := errors.New("hook failure")
	stream, _ := core.FetchDeferred(ctx, model, handle, ai.DeferredFetchOptions{ProviderRequestOptions: ai.ProviderRequestOptions{
		OnResponse: func(context.Context, ai.ProviderResponse, ai.Model) error { return hookError },
	}})
	result, _ := stream.Result(ctx)
	if text, _ := result.ErrorMessage.Value(); text != "hook failure" || calls.Load() != 0 {
		t.Fatalf("hook failure resolved the script: %#v", result)
	}
	stream, _ = core.FetchDeferred(ctx, model, handle, ai.DeferredFetchOptions{})
	first, _ := stream.Result(ctx)
	stream, _ = core.FetchDeferred(ctx, model, handle, ai.DeferredFetchOptions{})
	second, _ := stream.Result(ctx)
	if text, _ := first.ErrorMessage.Value(); text != "script failure" || !reflect.DeepEqual(first, second) || calls.Load() != 1 {
		t.Fatalf("factory error not cached: %#v, %#v", first, second)
	}
	if err := core.CancelDeferred(ctx, model, handle, ai.DeferredCancelOptions{
		OnResponse: func(context.Context, ai.ProviderResponse, ai.Model) error { return hookError },
	}); !errors.Is(err, hookError) || len(core.State.CancelledDeferred) != 1 {
		t.Fatalf("cancel hook failure = %v", err)
	}
}

func TestDeferredCancelInterruptsConcurrentFetches(t *testing.T) {
	core, err := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	model, _ := core.GetModel()
	entered, release := make(chan struct{}), make(chan struct{})
	defer close(release)
	var calls atomic.Int32
	core.SetResponses([]ai.FauxResponseStep{ai.FauxResponseFactory(func(ai.Context, *ai.SimpleStreamOptions, *ai.FauxProviderState, ai.Model) (ai.AssistantMessage, error) {
		if calls.Add(1) == 1 {
			close(entered)
		}
		<-release
		return ai.FauxAssistantMessage(ai.FauxAssistantText("late success"))
	})})
	ctx := context.Background()
	submitted, _ := core.StreamSimple(ctx, model, ai.Context{}, ai.SimpleStreamOptions{Deferred: ai.DeferredWindowOptions{}}).Result(ctx)
	handle, _ := submitted.Deferred.Value()
	first, _ := core.FetchDeferred(ctx, model, handle, ai.DeferredFetchOptions{})
	<-entered
	second, _ := core.FetchDeferred(ctx, model, handle, ai.DeferredFetchOptions{})
	if err := core.CancelDeferred(ctx, model, handle, ai.DeferredCancelOptions{}); err != nil {
		t.Fatal(err)
	}
	deadline, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	for _, stream := range []*ai.AssistantMessageEventStream{first, second} {
		result, err := stream.Result(deadline)
		errorMessage, _ := result.ErrorMessage.Value()
		if err != nil || result.StopReason != ai.StopReasonError || !strings.Contains(errorMessage, "was cancelled") {
			t.Fatalf("cancelled in-flight fetch = %#v, %v", result, err)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("factory calls = %d", calls.Load())
	}
}

func TestDeferredConcurrentFinalReadsResolveOnce(t *testing.T) {
	core, _ := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
	model, _ := core.GetModel()
	var calls atomic.Int32
	core.SetResponses([]ai.FauxResponseStep{ai.FauxResponseFactory(func(ai.Context, *ai.SimpleStreamOptions, *ai.FauxProviderState, ai.Model) (ai.AssistantMessage, error) {
		calls.Add(1)
		return ai.FauxAssistantMessage(ai.FauxAssistantText("stable"), ai.FauxAssistantMessageOptions{Timestamp: ai.Some(int64(42))})
	})})
	ctx := context.Background()
	submitted, _ := core.StreamSimple(ctx, model, ai.Context{}, ai.SimpleStreamOptions{Deferred: ai.DeferredBoolean{Enabled: true}}).Result(ctx)
	handle, _ := submitted.Deferred.Value()
	var workers sync.WaitGroup
	for range 20 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			stream, _ := core.FetchDeferred(ctx, model, handle, ai.DeferredFetchOptions{})
			result, err := stream.Result(ctx)
			if err != nil || result.StopReason != ai.StopReasonStop || result.Timestamp != 42 || result.Content[0].(ai.TextContent).Text != "stable" {
				t.Errorf("concurrent final = %#v, %v", result, err)
			}
		}()
	}
	workers.Wait()
	if calls.Load() != 1 || core.State.DeferredFetchCount != 20 {
		t.Fatalf("factory calls = %d, fetches = %d", calls.Load(), core.State.DeferredFetchCount)
	}
}

func TestDeferredSubmissionSnapshotsAndFetchCancellation(t *testing.T) {
	core, _ := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
	model, _ := core.GetModel()
	entered, release := make(chan struct{}), make(chan struct{})
	var unblock sync.Once
	defer unblock.Do(func() { close(release) })
	core.SetResponses([]ai.FauxResponseStep{ai.FauxResponseFactory(func(input ai.Context, options *ai.SimpleStreamOptions, _ *ai.FauxProviderState, _ ai.Model) (ai.AssistantMessage, error) {
		close(entered)
		<-release
		text, _ := input.Messages[0].(ai.UserMessage).Content.Text()
		return ai.FauxAssistantMessage(ai.FauxAssistantText(text + ":" + *options.SessionID + ":" + *options.Headers["test"]))
	})})
	ctx, cancel := context.WithCancel(context.Background())
	input := ai.Context{Messages: []ai.Message{ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText("original")}}}
	session, header := "session", "header"
	options := ai.SimpleStreamOptions{Deferred: ai.DeferredBoolean{Enabled: true}, StreamOptions: ai.StreamOptions{
		SessionID: &session, ProviderRequestOptions: ai.ProviderRequestOptions{Headers: ai.ProviderHeaders{"test": &header}},
	}}
	submitted, _ := core.StreamSimple(ctx, model, input, options).Result(context.Background())
	handle, _ := submitted.Deferred.Value()
	cancel()
	input.Messages[0] = ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText("changed")}
	session, header = "changed", "changed"
	fetchCtx, cancelFetch := context.WithCancel(context.Background())
	stream, _ := core.FetchDeferred(fetchCtx, model, handle, ai.DeferredFetchOptions{})
	<-entered
	cancelFetch()
	deadline, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()
	result, err := stream.Result(deadline)
	if err != nil || result.StopReason != ai.StopReasonAborted {
		t.Fatalf("cancelled poll = %#v, %v", result, err)
	}
	unblock.Do(func() { close(release) })
	stream, _ = core.FetchDeferred(context.Background(), model, handle, ai.DeferredFetchOptions{})
	final, err := stream.Result(deadline)
	if err != nil || final.StopReason != ai.StopReasonStop || final.Content[0].(ai.TextContent).Text != "original:session:header" {
		t.Fatalf("retained submission = %#v, %v", final, err)
	}
}

func TestDeferredRequestCancellationDuringResponseHook(t *testing.T) {
	for _, operation := range []string{"submit", "fetch"} {
		t.Run(operation, func(t *testing.T) {
			core, _ := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
			model, _ := core.GetModel()
			response, _ := ai.FauxAssistantMessage(ai.FauxAssistantText("ready"))
			core.SetResponses([]ai.FauxResponseStep{response})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			options := ai.ProviderRequestOptions{OnResponse: func(context.Context, ai.ProviderResponse, ai.Model) error {
				cancel()
				return context.Canceled
			}}
			var stream *ai.AssistantMessageEventStream
			if operation == "submit" {
				stream = core.StreamSimple(ctx, model, ai.Context{}, ai.SimpleStreamOptions{StreamOptions: ai.StreamOptions{ProviderRequestOptions: options}, Deferred: ai.DeferredBoolean{Enabled: true}})
			} else {
				submitted, _ := core.StreamSimple(ctx, model, ai.Context{}, ai.SimpleStreamOptions{Deferred: ai.DeferredBoolean{Enabled: true}}).Result(context.Background())
				handle, _ := submitted.Deferred.Value()
				stream, _ = core.FetchDeferred(ctx, model, handle, ai.DeferredFetchOptions{ProviderRequestOptions: options})
			}
			result, err := stream.Result(context.Background())
			if err != nil || result.StopReason != ai.StopReasonAborted {
				t.Fatalf("request cancellation = %#v, %v", result, err)
			}
		})
	}
}

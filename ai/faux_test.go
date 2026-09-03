package ai_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nankedr/pig/ai"
)

func TestFauxTextStreamAndCompleteShareOutcome(t *testing.T) {
	one := 1
	message, err := ai.FauxAssistantMessage(
		ai.FauxAssistantText("hello faux"),
		ai.FauxAssistantMessageOptions{Timestamp: ai.Some[int64](1)},
	)
	if err != nil {
		t.Fatalf("FauxAssistantMessage() error = %v", err)
	}
	handle, err := ai.NewFauxProvider(ai.RegisterFauxProviderOptions{
		API:      "faux:test",
		Provider: "faux-test",
		Models:   []ai.FauxModelDefinition{{ID: "faux-1"}},
		TokenSize: &ai.FauxTokenSize{
			Min: &one,
			Max: &one,
		},
	})
	if err != nil {
		t.Fatalf("NewFauxProvider() error = %v", err)
	}
	handle.SetResponses([]ai.FauxResponseStep{message, message})
	models := ai.CreateModels()
	models.SetProvider(handle.Provider)
	model, ok := handle.GetModel("faux-1")
	if !ok {
		t.Fatal("GetModel(faux-1) did not find the configured model")
	}
	input := ai.Context{Messages: []ai.Message{ai.UserMessage{
		Role: ai.MessageRoleUser, Content: ai.UserText("hi"), Timestamp: 1,
	}}}

	stream := models.Stream(context.Background(), model, input)
	var events []ai.AssistantMessageEvent
	for {
		event, ok, err := stream.Next(context.Background())
		if err != nil {
			t.Fatalf("Next() error = %v", err)
		}
		if !ok {
			break
		}
		events = append(events, event)
	}
	streamed, err := stream.Result(context.Background())
	if err != nil {
		t.Fatalf("Result() error = %v", err)
	}
	terminal := events[len(events)-1].(ai.AssistantMessageDoneEvent).Message
	if !reflect.DeepEqual(streamed, terminal) {
		t.Fatalf("Result() = %#v, terminal event = %#v", streamed, terminal)
	}
	completed, err := models.Complete(context.Background(), model, input)
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if !reflect.DeepEqual(completed, streamed) {
		t.Fatalf("Complete() = %#v, Stream outcome = %#v", completed, streamed)
	}
	if handle.State.CallCount != 2 || handle.GetPendingResponseCount() != 0 {
		t.Fatalf("Faux state = %#v, pending = %d", handle.State, handle.GetPendingResponseCount())
	}
}

func TestFauxConcurrentFactoriesSeePerCallStateSnapshots(t *testing.T) {
	const calls = 8
	handle, err := ai.NewFauxProvider(ai.RegisterFauxProviderOptions{
		API: "faux:concurrent", Provider: "faux-concurrent", Models: []ai.FauxModelDefinition{{ID: "concurrent-model"}},
	})
	if err != nil {
		t.Fatalf("NewFauxProvider() error = %v", err)
	}
	release := make(chan struct{})
	steps := make([]ai.FauxResponseStep, calls)
	for i := range steps {
		steps[i] = ai.FauxResponseFactory(func(_ ai.Context, _ *ai.SimpleStreamOptions, state *ai.FauxProviderState, _ ai.Model) (ai.AssistantMessage, error) {
			<-release
			state.DeferredFetchCount++
			return ai.FauxAssistantMessage(ai.FauxAssistantText(strconv.Itoa(state.CallCount)))
		})
	}
	handle.SetResponses(steps)
	model, _ := handle.GetModel()
	streams := make([]*ai.AssistantMessageEventStream, calls)
	for i := range streams {
		streams[i] = handle.Provider.Stream(context.Background(), model, ai.Context{}, ai.StreamOptions{})
	}
	close(release)

	values := make([]int, calls)
	errorsByCall := make([]error, calls)
	var wait sync.WaitGroup
	for i, stream := range streams {
		wait.Add(1)
		go func() {
			defer wait.Done()
			result, err := stream.Result(context.Background())
			errorsByCall[i] = err
			if err == nil {
				values[i], err = strconv.Atoi(result.Content[0].(ai.TextContent).Text)
				errorsByCall[i] = err
			}
		}()
	}
	wait.Wait()
	for i, err := range errorsByCall {
		if err != nil {
			t.Fatalf("Result(%d) error = %v", i, err)
		}
	}
	sort.Ints(values)
	want := []int{1, 2, 3, 4, 5, 6, 7, 8}
	if !reflect.DeepEqual(values, want) {
		t.Fatalf("factory CallCount snapshots = %v, want %v", values, want)
	}
	if handle.State.CallCount != calls || handle.State.DeferredFetchCount != 0 || handle.GetPendingResponseCount() != 0 {
		t.Fatalf("state=%#v pending=%d", handle.State, handle.GetPendingResponseCount())
	}
}

func TestFauxCancellationRetainsStreamedPartialContent(t *testing.T) {
	one := 1
	rate := float64(100)
	message, err := ai.FauxAssistantMessage(
		ai.FauxAssistantText("abcdefghijklmnop"),
		ai.FauxAssistantMessageOptions{Timestamp: ai.Some[int64](1)},
	)
	if err != nil {
		t.Fatalf("FauxAssistantMessage() error = %v", err)
	}
	handle, err := ai.NewFauxProvider(ai.RegisterFauxProviderOptions{
		API: "faux:cancel", Provider: "faux-cancel", Models: []ai.FauxModelDefinition{{ID: "cancel-model"}},
		TokenSize: &ai.FauxTokenSize{Min: &one, Max: &one}, TokensPerSecond: &rate,
	})
	if err != nil {
		t.Fatalf("NewFauxProvider() error = %v", err)
	}
	handle.SetResponses([]ai.FauxResponseStep{message})
	model, _ := handle.GetModel()
	ctx, cancel := context.WithCancel(context.Background())
	stream := handle.Provider.Stream(ctx, model, ai.Context{}, ai.StreamOptions{})
	for {
		event, ok, err := stream.Next(context.Background())
		if err != nil || !ok {
			t.Fatalf("Next() before cancellation = (%#v, %t, %v)", event, ok, err)
		}
		if event.AssistantMessageEventType() == ai.AssistantMessageEventTypeTextDelta {
			cancel()
			break
		}
	}

	wait, stopWaiting := context.WithTimeout(context.Background(), time.Second)
	defer stopWaiting()
	result, err := stream.Result(wait)
	if err != nil {
		t.Fatalf("Result() error = %v", err)
	}
	if result.StopReason != ai.StopReasonAborted {
		t.Fatalf("StopReason = %q, want aborted", result.StopReason)
	}
	if len(result.Content) != 1 || result.Content[0].(ai.TextContent).Text != "abcd" {
		t.Fatalf("aborted content = %#v, want first streamed chunk", result.Content)
	}
	second, err := stream.Result(context.Background())
	if err != nil || !reflect.DeepEqual(second, result) {
		t.Fatalf("repeated Result() = (%#v, %v), want %#v", second, err, result)
	}
}

func TestFauxPreCanceledContextReturnsOnlyAbortedOutcome(t *testing.T) {
	message, _ := ai.FauxAssistantMessage(ai.FauxAssistantText("not streamed"))
	handle, err := ai.NewFauxProvider(ai.RegisterFauxProviderOptions{
		API: "faux:pre-cancel", Provider: "faux-pre-cancel", Models: []ai.FauxModelDefinition{{ID: "pre-cancel-model"}},
	})
	if err != nil {
		t.Fatalf("NewFauxProvider() error = %v", err)
	}
	handle.SetResponses([]ai.FauxResponseStep{message})
	model, _ := handle.GetModel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	stream := handle.Provider.Stream(ctx, model, ai.Context{}, ai.StreamOptions{})
	event, ok, err := stream.Next(context.Background())
	if err != nil || !ok {
		t.Fatalf("Next() = (%#v, %t, %v)", event, ok, err)
	}
	errorEvent, ok := event.(ai.AssistantMessageErrorEvent)
	if !ok || errorEvent.Reason != ai.StopReasonAborted || len(errorEvent.Error.Content) != 0 {
		t.Fatalf("terminal event = %#v, want empty aborted outcome", event)
	}
	if event, ok, err = stream.Next(context.Background()); event != nil || ok || err != nil {
		t.Fatalf("second Next() = (%#v, %t, %v)", event, ok, err)
	}
	result, err := stream.Result(context.Background())
	if err != nil || !reflect.DeepEqual(result, errorEvent.Error) {
		t.Fatalf("Result() = (%#v, %v), terminal = %#v", result, err, errorEvent.Error)
	}
}

func TestFauxProviderErrorRetainsScriptedPartialContent(t *testing.T) {
	message, err := ai.FauxAssistantMessage(
		ai.FauxAssistantText("partial response"),
		ai.FauxAssistantMessageOptions{
			StopReason: ai.Some(ai.StopReasonError), ErrorMessage: ai.Some("provider failed"), Timestamp: ai.Some[int64](1),
		},
	)
	if err != nil {
		t.Fatalf("FauxAssistantMessage() error = %v", err)
	}
	handle, err := ai.NewFauxProvider(ai.RegisterFauxProviderOptions{
		API: "faux:error", Provider: "faux-error", Models: []ai.FauxModelDefinition{{ID: "error-model"}},
	})
	if err != nil {
		t.Fatalf("NewFauxProvider() error = %v", err)
	}
	handle.SetResponses([]ai.FauxResponseStep{message})
	model, _ := handle.GetModel()
	stream := handle.Provider.Stream(context.Background(), model, ai.Context{}, ai.StreamOptions{})
	var terminal ai.AssistantMessageEvent
	for {
		event, ok, err := stream.Next(context.Background())
		if err != nil {
			t.Fatalf("Next() error = %v", err)
		}
		if !ok {
			break
		}
		terminal = event
	}
	errorEvent, ok := terminal.(ai.AssistantMessageErrorEvent)
	if !ok || errorEvent.Reason != ai.StopReasonError {
		t.Fatalf("terminal event = %#v, want provider error", terminal)
	}
	result, err := stream.Result(context.Background())
	if err != nil {
		t.Fatalf("Result() error = %v", err)
	}
	if !reflect.DeepEqual(result, errorEvent.Error) || len(result.Content) != 1 {
		t.Fatalf("Result() = %#v, terminal = %#v", result, errorEvent.Error)
	}
	if text := result.Content[0].(ai.TextContent).Text; text != "partial response" {
		t.Fatalf("partial text = %q", text)
	}
	if message, ok := result.ErrorMessage.Value(); !ok || message != "provider failed" {
		t.Fatalf("ErrorMessage = %#v", result.ErrorMessage)
	}
}

func TestFauxResponseQueueIsFIFOAndFailuresAreOutcomes(t *testing.T) {
	first, _ := ai.FauxAssistantMessage(ai.FauxAssistantText("first"), ai.FauxAssistantMessageOptions{Timestamp: ai.Some[int64](1)})
	handle, err := ai.NewFauxProvider(ai.RegisterFauxProviderOptions{
		API: "faux:queue", Provider: "faux-queue", Models: []ai.FauxModelDefinition{{ID: "queue-model"}},
	})
	if err != nil {
		t.Fatalf("NewFauxProvider() error = %v", err)
	}
	handle.SetResponses([]ai.FauxResponseStep{first})
	handle.AppendResponses([]ai.FauxResponseStep{ai.FauxResponseFactory(func(ai.Context, *ai.SimpleStreamOptions, *ai.FauxProviderState, ai.Model) (ai.AssistantMessage, error) {
		return ai.AssistantMessage{}, errors.New("factory failed")
	})})
	model, _ := handle.GetModel()

	result, err := handle.Provider.Stream(context.Background(), model, ai.Context{}, ai.StreamOptions{}).Result(context.Background())
	if err != nil || result.Content[0].(ai.TextContent).Text != "first" {
		t.Fatalf("first Result() = (%#v, %v)", result, err)
	}
	result, err = handle.Provider.Stream(context.Background(), model, ai.Context{}, ai.StreamOptions{}).Result(context.Background())
	if err != nil || result.StopReason != ai.StopReasonError {
		t.Fatalf("factory Result() = (%#v, %v)", result, err)
	}
	if message, ok := result.ErrorMessage.Value(); !ok || message != "factory failed" {
		t.Fatalf("factory ErrorMessage = %#v", result.ErrorMessage)
	}
	result, err = handle.Provider.Stream(context.Background(), model, ai.Context{}, ai.StreamOptions{}).Result(context.Background())
	if err != nil || result.StopReason != ai.StopReasonError {
		t.Fatalf("exhausted Result() = (%#v, %v)", result, err)
	}
	if message, ok := result.ErrorMessage.Value(); !ok || message != "No more faux responses queued" {
		t.Fatalf("exhausted ErrorMessage = %#v", result.ErrorMessage)
	}
	if handle.State.CallCount != 3 || handle.GetPendingResponseCount() != 0 {
		t.Fatalf("state calls=%d pending=%d", handle.State.CallCount, handle.GetPendingResponseCount())
	}
}

func TestFauxNeverInvokesConfiguredTransport(t *testing.T) {
	message, _ := ai.FauxAssistantMessage(ai.FauxAssistantText("offline"))
	handle, err := ai.NewFauxProvider(ai.RegisterFauxProviderOptions{
		API: "faux:offline", Provider: "faux-offline", Models: []ai.FauxModelDefinition{{ID: "offline-model"}},
	})
	if err != nil {
		t.Fatalf("NewFauxProvider() error = %v", err)
	}
	handle.SetResponses([]ai.FauxResponseStep{message})
	model, _ := handle.GetModel()
	transportCalls := 0
	result, err := handle.Provider.Stream(context.Background(), model, ai.Context{}, ai.StreamOptions{
		ProviderRequestOptions: ai.ProviderRequestOptions{
			Fetch: func(context.Context, ai.FetchRequest) (ai.FetchResponse, error) {
				transportCalls++
				return ai.FetchResponse{}, errors.New("transport must not be used")
			},
		},
	}).Result(context.Background())
	if err != nil || result.StopReason != ai.StopReasonStop {
		t.Fatalf("Result() = (%#v, %v)", result, err)
	}
	if transportCalls != 0 {
		t.Fatalf("configured transport called %d times, want zero", transportCalls)
	}
}

func TestCompatFauxRegistrationDrivesStreamAndComplete(t *testing.T) {
	registration, err := ai.RegisterFauxProvider(ai.RegisterFauxProviderOptions{
		API: "faux:compat", Provider: "faux-compat", Models: []ai.FauxModelDefinition{{ID: "compat-model"}},
	})
	if err != nil {
		t.Fatalf("RegisterFauxProvider() error = %v", err)
	}
	t.Cleanup(registration.Unregister)
	first, _ := ai.FauxAssistantMessage(ai.FauxAssistantText("streamed"), ai.FauxAssistantMessageOptions{Timestamp: ai.Some[int64](1)})
	second, _ := ai.FauxAssistantMessage(ai.FauxAssistantText("completed"), ai.FauxAssistantMessageOptions{Timestamp: ai.Some[int64](2)})
	registration.SetResponses([]ai.FauxResponseStep{first, second})
	model, _ := registration.GetModel()

	streamed, err := ai.Stream(context.Background(), model, ai.Context{}).Result(context.Background())
	if err != nil || streamed.Content[0].(ai.TextContent).Text != "streamed" {
		t.Fatalf("compat Stream().Result() = (%#v, %v)", streamed, err)
	}
	completed, err := ai.Complete(context.Background(), model, ai.Context{})
	if err != nil || completed.Content[0].(ai.TextContent).Text != "completed" {
		t.Fatalf("compat Complete() = (%#v, %v)", completed, err)
	}
	if registration.State.CallCount != 2 {
		t.Fatalf("CallCount = %d, want 2", registration.State.CallCount)
	}
	registration.Unregister()
	registration.Unregister()
	if _, err := ai.Complete(context.Background(), model, ai.Context{}); err == nil || err.Error() != "No API provider registered for api: faux:compat" {
		t.Fatalf("Complete() after Unregister error = %v", err)
	}
}

func TestFauxUnsupportedOptionsFailWithoutConsumingScript(t *testing.T) {
	message, _ := ai.FauxAssistantMessage(ai.FauxAssistantText("still queued"))
	handle, err := ai.NewFauxProvider(ai.RegisterFauxProviderOptions{
		API: "faux:m2", Provider: "faux-m2", Models: []ai.FauxModelDefinition{{ID: "m2-model"}},
	})
	if err != nil {
		t.Fatalf("NewFauxProvider() error = %v", err)
	}
	handle.SetResponses([]ai.FauxResponseStep{message})
	model, _ := handle.GetModel()
	hookCalls := 0
	requestOptions := ai.ProviderRequestOptions{OnResponse: func(context.Context, ai.ProviderResponse, ai.Model) error {
		hookCalls++
		return nil
	}}
	if handle.Provider.SupportsFetchDeferred() || handle.Provider.SupportsCancelDeferred() {
		t.Fatal("Faux must not advertise deferred support")
	}

	sessionID := "cache-session"
	shortCache := ai.CacheRetentionShort
	longCache := ai.CacheRetentionLong
	for _, test := range []struct {
		name      string
		options   ai.SimpleStreamOptions
		operation string
	}{
		{name: "enabled deferred", options: ai.SimpleStreamOptions{Deferred: ai.DeferredBoolean{Enabled: true}}, operation: "Faux.Deferred"},
		{name: "deferred window", options: ai.SimpleStreamOptions{Deferred: ai.DeferredWindowOptions{}}, operation: "Faux.Deferred"},
		{name: "default cache", options: ai.SimpleStreamOptions{StreamOptions: ai.StreamOptions{ProviderRequestOptions: requestOptions, SessionID: &sessionID}}, operation: "Faux.Cache"},
		{name: "short cache", options: ai.SimpleStreamOptions{StreamOptions: ai.StreamOptions{ProviderRequestOptions: requestOptions, SessionID: &sessionID, CacheRetention: &shortCache}}, operation: "Faux.Cache"},
		{name: "long cache", options: ai.SimpleStreamOptions{StreamOptions: ai.StreamOptions{ProviderRequestOptions: requestOptions, SessionID: &sessionID, CacheRetention: &longCache}}, operation: "Faux.Cache"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := handle.Provider.StreamSimple(context.Background(), model, ai.Context{}, test.options).Result(context.Background())
			if !errors.Is(err, ai.ErrNotImplemented) {
				t.Fatalf("Result() error = %v, want ErrNotImplemented", err)
			}
			var notImplemented *ai.NotImplementedError
			if !errors.As(err, &notImplemented) || notImplemented.Operation != test.operation {
				t.Fatalf("Result() error = %#v, want %s", err, test.operation)
			}
		})
	}
	if hookCalls != 0 || handle.State.CallCount != 0 || handle.GetPendingResponseCount() != 1 {
		t.Fatalf("unsupported option hooks=%d calls=%d pending=%d, want 0/0/1", hookCalls, handle.State.CallCount, handle.GetPendingResponseCount())
	}
}

func TestFauxCoreDeferredMethodsFailExplicitly(t *testing.T) {
	core, err := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
	if err != nil {
		t.Fatalf("CreateFauxCore() error = %v", err)
	}
	model, _ := core.GetModel()
	stream, err := core.FetchDeferred(context.Background(), model, ai.DeferredHandle{}, ai.DeferredFetchOptions{})
	if stream != nil || !errors.Is(err, ai.ErrNotImplemented) {
		t.Fatalf("FetchDeferred() = (%#v, %v), want nil/ErrNotImplemented", stream, err)
	}
	var notImplemented *ai.NotImplementedError
	if !errors.As(err, &notImplemented) || notImplemented.Operation != "Faux.FetchDeferred" {
		t.Fatalf("FetchDeferred() error = %#v", err)
	}
	err = core.CancelDeferred(context.Background(), model, ai.DeferredHandle{}, ai.DeferredCancelOptions{})
	if !errors.Is(err, ai.ErrNotImplemented) {
		t.Fatalf("CancelDeferred() error = %v, want ErrNotImplemented", err)
	}
	notImplemented = nil
	if !errors.As(err, &notImplemented) || notImplemented.Operation != "Faux.CancelDeferred" {
		t.Fatalf("CancelDeferred() error = %#v", err)
	}
}

func TestFauxChecksCancellationBetweenContentBlocks(t *testing.T) {
	message, _ := ai.FauxAssistantMessage(ai.FauxAssistantBlocks(ai.FauxText("one"), ai.FauxText("two")))
	handle, err := ai.NewFauxProvider(ai.RegisterFauxProviderOptions{
		API: "faux:block-cancel", Provider: "faux-block-cancel", Models: []ai.FauxModelDefinition{{ID: "block-cancel-model"}},
	})
	if err != nil {
		t.Fatalf("NewFauxProvider() error = %v", err)
	}
	handle.SetResponses([]ai.FauxResponseStep{message})
	model, _ := handle.GetModel()
	ctx := &cancelAfterErrChecks{Context: context.Background(), done: make(chan struct{}), cancelAt: 4}
	stream := handle.Provider.Stream(ctx, model, ai.Context{}, ai.StreamOptions{})
	var types []ai.AssistantMessageEventType
	for {
		event, ok, err := stream.Next(context.Background())
		if err != nil {
			t.Fatalf("Next() error = %v", err)
		}
		if !ok {
			break
		}
		types = append(types, event.AssistantMessageEventType())
	}
	want := []ai.AssistantMessageEventType{
		ai.AssistantMessageEventTypeStart,
		ai.AssistantMessageEventTypeTextStart,
		ai.AssistantMessageEventTypeTextDelta,
		ai.AssistantMessageEventTypeTextEnd,
		ai.AssistantMessageEventTypeError,
	}
	if !reflect.DeepEqual(types, want) {
		t.Fatalf("event types = %v, want %v", types, want)
	}
	result, err := stream.Result(context.Background())
	if err != nil || result.StopReason != ai.StopReasonAborted || len(result.Content) != 1 {
		t.Fatalf("Result() = (%#v, %v), want first block plus aborted", result, err)
	}
}

type cancelAfterErrChecks struct {
	context.Context
	checks   atomic.Int32
	done     chan struct{}
	once     sync.Once
	cancelAt int32
}

func (c *cancelAfterErrChecks) Done() <-chan struct{} { return c.done }

func (c *cancelAfterErrChecks) Err() error {
	if c.checks.Add(1) >= c.cancelAt {
		c.once.Do(func() { close(c.done) })
		return context.Canceled
	}
	select {
	case <-c.done:
		return context.Canceled
	default:
	}
	return nil
}

func TestFauxM2NoOpOptionsDoNotSelectUnsupportedBranches(t *testing.T) {
	message, _ := ai.FauxAssistantMessage(ai.FauxAssistantText("not deferred"))
	handle, err := ai.NewFauxProvider(ai.RegisterFauxProviderOptions{
		API: "faux:no-deferred", Provider: "faux-no-deferred", Models: []ai.FauxModelDefinition{{ID: "no-deferred-model"}},
	})
	if err != nil {
		t.Fatalf("NewFauxProvider() error = %v", err)
	}
	emptySession := ""
	sessionID := "cache-session"
	noCache := ai.CacheRetentionNone
	shortCache := ai.CacheRetentionShort
	tests := []ai.SimpleStreamOptions{
		{Deferred: ai.DeferredBoolean{Enabled: false}},
		{StreamOptions: ai.StreamOptions{SessionID: &emptySession}},
		{StreamOptions: ai.StreamOptions{SessionID: &sessionID, CacheRetention: &noCache}},
		{StreamOptions: ai.StreamOptions{CacheRetention: &shortCache}},
		{StreamOptions: ai.StreamOptions{SessionID: &emptySession, CacheRetention: &shortCache}},
	}
	responses := make([]ai.FauxResponseStep, len(tests))
	for i := range responses {
		responses[i] = message
	}
	handle.SetResponses(responses)
	model, _ := handle.GetModel()
	for i, options := range tests {
		result, err := handle.Provider.StreamSimple(context.Background(), model, ai.Context{}, options).Result(context.Background())
		if err != nil || result.StopReason != ai.StopReasonStop {
			t.Fatalf("no-op option %d Result() = (%#v, %v)", i, result, err)
		}
	}
}

func TestFauxStreamsToolCallDeltasAndToolUseOutcome(t *testing.T) {
	one := 1
	toolCall, err := ai.FauxToolCall("echo", map[string]any{"text": "hi", "count": float64(12)}, ai.FauxToolCallOptions{ID: ai.Some("tool-1")})
	if err != nil {
		t.Fatalf("FauxToolCall() error = %v", err)
	}
	message, err := ai.FauxAssistantMessage(
		ai.FauxAssistantBlocks(ai.FauxText("answer"), toolCall),
		ai.FauxAssistantMessageOptions{StopReason: ai.Some(ai.StopReasonToolUse), Timestamp: ai.Some[int64](1)},
	)
	if err != nil {
		t.Fatalf("FauxAssistantMessage() error = %v", err)
	}
	handle, err := ai.NewFauxProvider(ai.RegisterFauxProviderOptions{
		API: "faux:tool", Provider: "faux-tool", Models: []ai.FauxModelDefinition{{ID: "tool-model"}},
		TokenSize: &ai.FauxTokenSize{Min: &one, Max: &one},
	})
	if err != nil {
		t.Fatalf("NewFauxProvider() error = %v", err)
	}
	handle.SetResponses([]ai.FauxResponseStep{message})
	models := ai.CreateModels()
	models.SetProvider(handle.Provider)
	model, _ := handle.GetModel()
	stream := models.Stream(context.Background(), model, ai.Context{})

	var eventTypes []ai.AssistantMessageEventType
	var deltas string
	for {
		event, ok, err := stream.Next(context.Background())
		if err != nil {
			t.Fatalf("Next() error = %v", err)
		}
		if !ok {
			break
		}
		eventTypes = append(eventTypes, event.AssistantMessageEventType())
		if delta, ok := event.(ai.AssistantMessageToolCallDeltaEvent); ok {
			deltas += delta.Delta
		}
	}
	wantTypes := []ai.AssistantMessageEventType{
		ai.AssistantMessageEventTypeStart,
		ai.AssistantMessageEventTypeTextStart,
		ai.AssistantMessageEventTypeTextDelta,
		ai.AssistantMessageEventTypeTextDelta,
		ai.AssistantMessageEventTypeTextEnd,
		ai.AssistantMessageEventTypeToolCallStart,
		ai.AssistantMessageEventTypeToolCallDelta,
		ai.AssistantMessageEventTypeToolCallDelta,
		ai.AssistantMessageEventTypeToolCallDelta,
		ai.AssistantMessageEventTypeToolCallDelta,
		ai.AssistantMessageEventTypeToolCallDelta,
		ai.AssistantMessageEventTypeToolCallDelta,
		ai.AssistantMessageEventTypeToolCallEnd,
		ai.AssistantMessageEventTypeDone,
	}
	if !reflect.DeepEqual(eventTypes, wantTypes) {
		t.Fatalf("event types = %#v, want %#v", eventTypes, wantTypes)
	}
	var arguments map[string]any
	if err := json.Unmarshal([]byte(deltas), &arguments); err != nil {
		t.Fatalf("tool deltas %q are not JSON: %v", deltas, err)
	}
	if !reflect.DeepEqual(arguments, map[string]any{"count": float64(12), "text": "hi"}) {
		t.Fatalf("tool delta arguments = %#v", arguments)
	}
	result, err := stream.Result(context.Background())
	if err != nil || result.StopReason != ai.StopReasonToolUse {
		t.Fatalf("Result() = (%#v, %v), want toolUse outcome", result, err)
	}
}

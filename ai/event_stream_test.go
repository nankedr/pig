package ai_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/nankedr/pig/ai"
)

type streamEvent struct {
	Value    int
	Terminal bool
}

func TestAssistantMessageEventVariantsExposeExactDiscriminators(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		event ai.AssistantMessageEvent
		want  ai.AssistantMessageEventType
	}{
		{name: "start", event: ai.AssistantMessageStartEvent{Type: ai.AssistantMessageEventTypeStart}, want: "start"},
		{name: "text start", event: ai.AssistantMessageTextStartEvent{Type: ai.AssistantMessageEventTypeTextStart}, want: "text_start"},
		{name: "text delta", event: ai.AssistantMessageTextDeltaEvent{Type: ai.AssistantMessageEventTypeTextDelta}, want: "text_delta"},
		{name: "text end", event: ai.AssistantMessageTextEndEvent{Type: ai.AssistantMessageEventTypeTextEnd}, want: "text_end"},
		{name: "thinking start", event: ai.AssistantMessageThinkingStartEvent{Type: ai.AssistantMessageEventTypeThinkingStart}, want: "thinking_start"},
		{name: "thinking delta", event: ai.AssistantMessageThinkingDeltaEvent{Type: ai.AssistantMessageEventTypeThinkingDelta}, want: "thinking_delta"},
		{name: "thinking end", event: ai.AssistantMessageThinkingEndEvent{Type: ai.AssistantMessageEventTypeThinkingEnd}, want: "thinking_end"},
		{name: "tool call start", event: ai.AssistantMessageToolCallStartEvent{Type: ai.AssistantMessageEventTypeToolCallStart}, want: "toolcall_start"},
		{name: "tool call delta", event: ai.AssistantMessageToolCallDeltaEvent{Type: ai.AssistantMessageEventTypeToolCallDelta}, want: "toolcall_delta"},
		{name: "tool call end", event: ai.AssistantMessageToolCallEndEvent{Type: ai.AssistantMessageEventTypeToolCallEnd}, want: "toolcall_end"},
		{name: "done", event: ai.AssistantMessageDoneEvent{Type: ai.AssistantMessageEventTypeDone}, want: "done"},
		{name: "error", event: ai.AssistantMessageErrorEvent{Type: ai.AssistantMessageEventTypeError}, want: "error"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := test.event.AssistantMessageEventType(); got != test.want {
				t.Fatalf("AssistantMessageEventType() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestAssistantMessageEventVariantsExposeCompletePayloads(t *testing.T) {
	t.Parallel()

	partial := ai.AssistantMessage{Model: "partial-model"}
	toolCall := ai.ToolCall{ID: "call-1", Name: "read"}

	start := ai.AssistantMessageStartEvent{
		Type:    ai.AssistantMessageEventTypeStart,
		Partial: partial,
	}
	textStart := ai.AssistantMessageTextStartEvent{
		Type:         ai.AssistantMessageEventTypeTextStart,
		ContentIndex: 1,
		Partial:      partial,
	}
	textDelta := ai.AssistantMessageTextDeltaEvent{
		Type:         ai.AssistantMessageEventTypeTextDelta,
		ContentIndex: 2,
		Delta:        "delta",
		Partial:      partial,
	}
	textEnd := ai.AssistantMessageTextEndEvent{
		Type:         ai.AssistantMessageEventTypeTextEnd,
		ContentIndex: 3,
		Content:      "text",
		Partial:      partial,
	}
	thinkingStart := ai.AssistantMessageThinkingStartEvent{
		Type:         ai.AssistantMessageEventTypeThinkingStart,
		ContentIndex: 4,
		Partial:      partial,
	}
	thinkingDelta := ai.AssistantMessageThinkingDeltaEvent{
		Type:         ai.AssistantMessageEventTypeThinkingDelta,
		ContentIndex: 5,
		Delta:        "thought delta",
		Partial:      partial,
	}
	thinkingEnd := ai.AssistantMessageThinkingEndEvent{
		Type:         ai.AssistantMessageEventTypeThinkingEnd,
		ContentIndex: 6,
		Content:      "thought",
		Partial:      partial,
	}
	toolCallStart := ai.AssistantMessageToolCallStartEvent{
		Type:         ai.AssistantMessageEventTypeToolCallStart,
		ContentIndex: 7,
		Partial:      partial,
	}
	toolCallDelta := ai.AssistantMessageToolCallDeltaEvent{
		Type:         ai.AssistantMessageEventTypeToolCallDelta,
		ContentIndex: 8,
		Delta:        "{\"path\":",
		Partial:      partial,
	}
	toolCallEnd := ai.AssistantMessageToolCallEndEvent{
		Type:         ai.AssistantMessageEventTypeToolCallEnd,
		ContentIndex: 9,
		ToolCall:     toolCall,
		Partial:      partial,
	}
	done := ai.AssistantMessageDoneEvent{
		Type:    ai.AssistantMessageEventTypeDone,
		Reason:  ai.StopReasonStop,
		Message: partial,
	}
	failed := ai.AssistantMessageErrorEvent{
		Type:   ai.AssistantMessageEventTypeError,
		Reason: ai.StopReasonError,
		Error:  partial,
	}

	if start.Partial.Model != "partial-model" || textStart.ContentIndex != 1 || textStart.Partial.Model != "partial-model" {
		t.Fatal("start payloads were not preserved")
	}
	if textDelta.ContentIndex != 2 || textDelta.Delta != "delta" || textDelta.Partial.Model != "partial-model" {
		t.Fatal("text_delta payload was not preserved")
	}
	if textEnd.ContentIndex != 3 || textEnd.Content != "text" || textEnd.Partial.Model != "partial-model" {
		t.Fatal("text_end payload was not preserved")
	}
	if thinkingStart.ContentIndex != 4 || thinkingStart.Partial.Model != "partial-model" {
		t.Fatal("thinking_start payload was not preserved")
	}
	if thinkingDelta.ContentIndex != 5 || thinkingDelta.Delta != "thought delta" || thinkingDelta.Partial.Model != "partial-model" {
		t.Fatal("thinking_delta payload was not preserved")
	}
	if thinkingEnd.ContentIndex != 6 || thinkingEnd.Content != "thought" || thinkingEnd.Partial.Model != "partial-model" {
		t.Fatal("thinking_end payload was not preserved")
	}
	if toolCallStart.ContentIndex != 7 || toolCallStart.Partial.Model != "partial-model" {
		t.Fatal("toolcall_start payload was not preserved")
	}
	if toolCallDelta.ContentIndex != 8 || toolCallDelta.Delta != `{"path":` || toolCallDelta.Partial.Model != "partial-model" {
		t.Fatal("toolcall_delta payload was not preserved")
	}
	if toolCallEnd.ContentIndex != 9 || toolCallEnd.ToolCall.ID != "call-1" || toolCallEnd.Partial.Model != "partial-model" {
		t.Fatal("toolcall_end payload was not preserved")
	}
	if done.Reason != ai.StopReasonStop || done.Message.Model != "partial-model" {
		t.Fatal("done payload was not preserved")
	}
	if failed.Reason != ai.StopReasonError || failed.Error.Model != "partial-model" {
		t.Fatal("error payload was not preserved")
	}
}

func TestEventStreamPreservesFIFOAndTerminalOutcome(t *testing.T) {
	t.Parallel()

	stream := ai.NewEventStream(
		func(event streamEvent) bool { return event.Terminal },
		func(event streamEvent) int { return event.Value },
	)
	stream.Push(streamEvent{Value: 1})
	stream.Push(streamEvent{Value: 2})
	stream.Push(streamEvent{Value: 3, Terminal: true})
	stream.Push(streamEvent{Value: 4})

	for _, want := range []int{1, 2, 3} {
		event, ok, err := stream.Next(context.Background())
		if err != nil {
			t.Fatalf("Next() error = %v", err)
		}
		if !ok || event.Value != want {
			t.Fatalf("Next() = (%#v, %t), want value %d and ok", event, ok, want)
		}
	}

	if event, ok, err := stream.Next(context.Background()); err != nil || ok {
		t.Fatalf("Next() after terminal = (%#v, %t, %v), want zero, false, nil", event, ok, err)
	}
	for i := 0; i < 2; i++ {
		result, err := stream.Result(context.Background())
		if err != nil || result != 3 {
			t.Fatalf("Result() call %d = (%d, %v), want (3, nil)", i+1, result, err)
		}
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := stream.Result(canceled)
	if err != nil || result != 3 {
		t.Fatalf("completed Result(canceled) = (%d, %v), want (3, nil)", result, err)
	}
	if errors.Is(err, context.Canceled) {
		t.Fatal("completed Result returned waiter cancellation")
	}
}

func TestEventStreamCancellationOnlyReleasesTheCanceledWaiter(t *testing.T) {
	t.Parallel()

	t.Run("Next", func(t *testing.T) {
		stream := ai.NewEventStream(
			func(event streamEvent) bool { return event.Terminal },
			func(event streamEvent) int { return event.Value },
		)
		waitCanceled := errors.New("stop waiting for the next event")
		ctx, cancel := context.WithCancelCause(context.Background())
		cancel(waitCanceled)

		event, ok, err := stream.Next(ctx)
		if event != (streamEvent{}) || ok || !errors.Is(err, waitCanceled) {
			t.Fatalf("canceled Next() = (%#v, %t, %v), want zero, false, cancellation cause", event, ok, err)
		}

		stream.Push(streamEvent{Value: 42, Terminal: true})
		event, ok, err = stream.Next(context.Background())
		if err != nil || !ok || event.Value != 42 {
			t.Fatalf("Next() after another waiter canceled = (%#v, %t, %v), want terminal event", event, ok, err)
		}
	})

	t.Run("Result", func(t *testing.T) {
		stream := ai.NewEventStream(
			func(event streamEvent) bool { return event.Terminal },
			func(event streamEvent) int { return event.Value },
		)
		waitCanceled := errors.New("stop waiting for the result")
		ctx, cancel := context.WithCancelCause(context.Background())
		cancel(waitCanceled)

		result, err := stream.Result(ctx)
		if result != 0 || !errors.Is(err, waitCanceled) {
			t.Fatalf("canceled Result() = (%d, %v), want zero and cancellation cause", result, err)
		}

		stream.Push(streamEvent{Value: 99, Terminal: true})
		result, err = stream.Result(context.Background())
		if err != nil || result != 99 {
			t.Fatalf("Result() after another waiter canceled = (%d, %v), want (99, nil)", result, err)
		}
	})
}

func TestEventStreamEndSeparatesExplicitOutcomeFromInvariantFailure(t *testing.T) {
	t.Parallel()

	t.Run("explicit outcome", func(t *testing.T) {
		stream := ai.NewEventStream(
			func(streamEvent) bool { return false },
			func(event streamEvent) int { return event.Value },
		)
		stream.Push(streamEvent{Value: 1})
		stream.End(7)
		stream.Push(streamEvent{Value: 2})

		event, ok, err := stream.Next(context.Background())
		if err != nil || !ok || event.Value != 1 {
			t.Fatalf("first Next() = (%#v, %t, %v), want queued event", event, ok, err)
		}
		if event, ok, err = stream.Next(context.Background()); err != nil || ok {
			t.Fatalf("Next() after End = (%#v, %t, %v), want zero, false, nil", event, ok, err)
		}
		result, err := stream.Result(context.Background())
		if err != nil || result != 7 {
			t.Fatalf("Result() = (%d, %v), want (7, nil)", result, err)
		}
	})

	t.Run("missing outcome", func(t *testing.T) {
		stream := ai.NewEventStream(
			func(streamEvent) bool { return false },
			func(event streamEvent) int { return event.Value },
		)
		stream.End()

		if event, ok, err := stream.Next(context.Background()); err != nil || ok {
			t.Fatalf("Next() after End() = (%#v, %t, %v), want zero, false, nil", event, ok, err)
		}
		var firstErr error
		for i := 0; i < 2; i++ {
			result, err := stream.Result(context.Background())
			if err == nil || result != 0 {
				t.Fatalf("Result() call %d = (%d, %v), want zero and invariant error", i+1, result, err)
			}
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("Result() invariant error = %v, want non-context Go error", err)
			}
			if i == 0 {
				firstErr = err
			} else if err != firstErr {
				t.Fatalf("Result() errors are not stable: first %p, second %p", firstErr, err)
			}
		}
	})
}

func TestEventStreamConcurrentProducersAndResultWaiters(t *testing.T) {
	t.Parallel()

	const (
		producerCount     = 8
		eventsPerProducer = 128
		resultWaiters     = 8
	)
	total := producerCount * eventsPerProducer
	stream := ai.NewEventStream(
		func(event streamEvent) bool { return event.Terminal },
		func(event streamEvent) int { return event.Value },
	)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resultErrors := make(chan error, resultWaiters)
	var waiters sync.WaitGroup
	for i := 0; i < resultWaiters; i++ {
		waiters.Add(1)
		go func() {
			defer waiters.Done()
			result, err := stream.Result(ctx)
			if err != nil {
				resultErrors <- err
				return
			}
			if result != total {
				resultErrors <- fmt.Errorf("Result() = %d, want %d", result, total)
			}
		}()
	}

	consumed := make(chan streamEvent, total+1)
	consumerError := make(chan error, 1)
	go func() {
		for {
			event, ok, err := stream.Next(ctx)
			if err != nil {
				consumerError <- err
				return
			}
			if !ok {
				close(consumed)
				return
			}
			consumed <- event
		}
	}()

	start := make(chan struct{})
	var producers sync.WaitGroup
	for producer := 0; producer < producerCount; producer++ {
		producer := producer
		producers.Add(1)
		go func() {
			defer producers.Done()
			<-start
			for sequence := 0; sequence < eventsPerProducer; sequence++ {
				stream.Push(streamEvent{Value: producer*eventsPerProducer + sequence})
			}
		}()
	}
	close(start)
	producers.Wait()
	stream.Push(streamEvent{Value: total, Terminal: true})
	waiters.Wait()
	close(resultErrors)

	for err := range resultErrors {
		t.Errorf("concurrent Result waiter: %v", err)
	}

	seen := make(map[int]int, total)
	terminalCount := 0
	for {
		select {
		case err := <-consumerError:
			t.Fatalf("consumer error = %v", err)
		case event, ok := <-consumed:
			if !ok {
				if len(seen) != total {
					t.Fatalf("unique non-terminal events = %d, want %d", len(seen), total)
				}
				if terminalCount != 1 {
					t.Fatalf("terminal events = %d, want 1", terminalCount)
				}
				for value, count := range seen {
					if count != 1 {
						t.Fatalf("event %d consumed %d times, want once", value, count)
					}
				}
				return
			}
			if event.Terminal {
				terminalCount++
			} else {
				seen[event.Value]++
			}
		}
	}
}

func TestAssistantMessageEventStreamExtractsDoneAndErrorOutcomes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		terminal  ai.AssistantMessageEvent
		wantModel string
	}{
		{
			name: "done",
			terminal: ai.AssistantMessageDoneEvent{
				Type:    ai.AssistantMessageEventTypeDone,
				Reason:  ai.StopReasonStop,
				Message: ai.AssistantMessage{Model: "successful-model"},
			},
			wantModel: "successful-model",
		},
		{
			name: "provider error remains an outcome",
			terminal: ai.AssistantMessageErrorEvent{
				Type:   ai.AssistantMessageEventTypeError,
				Reason: ai.StopReasonError,
				Error: ai.AssistantMessage{
					Model:        "failed-model",
					StopReason:   ai.StopReasonError,
					ErrorMessage: ai.Some("provider unavailable"),
				},
			},
			wantModel: "failed-model",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			stream := ai.NewAssistantMessageEventStream()
			stream.Push(test.terminal)

			event, ok, err := stream.Next(context.Background())
			if err != nil || !ok || event.AssistantMessageEventType() != test.terminal.AssistantMessageEventType() {
				t.Fatalf("Next() = (%#v, %t, %v), want terminal %q", event, ok, err, test.terminal.AssistantMessageEventType())
			}
			result, err := stream.Result(context.Background())
			if err != nil || result.Model != test.wantModel {
				t.Fatalf("Result() = (%#v, %v), want model %q and nil Go error", result, err, test.wantModel)
			}
		})
	}
}

func TestAssistantMessageEventStreamPushPublishesImmutableSnapshots(t *testing.T) {
	t.Parallel()

	t.Run("partial message", func(t *testing.T) {
		stream := ai.NewAssistantMessageEventStream()
		message, arguments := assistantMessageWithToolCall("partial")
		stream.Push(ai.AssistantMessageTextDeltaEvent{
			Type:         ai.AssistantMessageEventTypeTextDelta,
			ContentIndex: 0,
			Delta:        "chunk",
			Partial:      message,
		})

		arguments["path"] = "after-push"
		arguments["segments"].([]any)[0] = "after-push"
		message.Content[0] = ai.TextContent{Type: ai.ContentTypeText, Text: "after-push"}

		event, ok, err := stream.Next(context.Background())
		if err != nil || !ok {
			t.Fatalf("Next() = (%#v, %t, %v), want event", event, ok, err)
		}
		received := event.(ai.AssistantMessageTextDeltaEvent)
		assertToolCallSnapshot(t, received.Partial, "partial")
	})

	t.Run("tool call payload", func(t *testing.T) {
		stream := ai.NewAssistantMessageEventStream()
		message, _ := assistantMessageWithToolCall("partial")
		arguments := map[string]any{"path": "tool-call", "segments": []any{"tool-call"}}
		stream.Push(ai.AssistantMessageToolCallEndEvent{
			Type:         ai.AssistantMessageEventTypeToolCallEnd,
			ContentIndex: 0,
			ToolCall: ai.ToolCall{
				Type:      ai.ContentTypeToolCall,
				ID:        "call-2",
				Name:      "read",
				Arguments: arguments,
			},
			Partial: message,
		})

		arguments["path"] = "after-push"
		arguments["segments"].([]any)[0] = "after-push"

		event, ok, err := stream.Next(context.Background())
		if err != nil || !ok {
			t.Fatalf("Next() = (%#v, %t, %v), want event", event, ok, err)
		}
		received := event.(ai.AssistantMessageToolCallEndEvent)
		assertToolCallArguments(t, received.ToolCall, "tool-call")
	})

	for _, test := range []struct {
		name     string
		terminal func(ai.AssistantMessage) ai.AssistantMessageEvent
	}{
		{
			name: "done outcome",
			terminal: func(message ai.AssistantMessage) ai.AssistantMessageEvent {
				return ai.AssistantMessageDoneEvent{
					Type: ai.AssistantMessageEventTypeDone, Reason: ai.StopReasonStop, Message: message,
				}
			},
		},
		{
			name: "error outcome",
			terminal: func(message ai.AssistantMessage) ai.AssistantMessageEvent {
				return ai.AssistantMessageErrorEvent{
					Type: ai.AssistantMessageEventTypeError, Reason: ai.StopReasonError, Error: message,
				}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			stream := ai.NewAssistantMessageEventStream()
			message, arguments := assistantMessageWithToolCall("terminal")
			stream.Push(test.terminal(message))

			arguments["path"] = "after-push"
			arguments["segments"].([]any)[0] = "after-push"
			message.Content[0] = ai.TextContent{Type: ai.ContentTypeText, Text: "after-push"}

			event, ok, err := stream.Next(context.Background())
			if err != nil || !ok {
				t.Fatalf("Next() = (%#v, %t, %v), want terminal event", event, ok, err)
			}
			mutateEventMessage(t, event)

			first, err := stream.Result(context.Background())
			if err != nil {
				t.Fatalf("Result() error = %v", err)
			}
			assertToolCallSnapshot(t, first, "terminal")
			first.Content[0].(ai.ToolCall).Arguments["path"] = "after-result"

			second, err := stream.Result(context.Background())
			if err != nil {
				t.Fatalf("second Result() error = %v", err)
			}
			assertToolCallSnapshot(t, second, "terminal")
		})
	}
}

func assistantMessageWithToolCall(value string) (ai.AssistantMessage, map[string]any) {
	arguments := map[string]any{
		"path":     value,
		"segments": []any{value},
	}
	return ai.AssistantMessage{
		Model: value,
		Content: []ai.AssistantContent{ai.ToolCall{
			Type: ai.ContentTypeToolCall, ID: "call-1", Name: "read", Arguments: arguments,
		}},
	}, arguments
}

func assertToolCallSnapshot(t *testing.T, message ai.AssistantMessage, want string) {
	t.Helper()
	if message.Model != want || len(message.Content) != 1 {
		t.Fatalf("AssistantMessage snapshot = %#v, want model %q and one content block", message, want)
	}
	toolCall, ok := message.Content[0].(ai.ToolCall)
	if !ok {
		t.Fatalf("AssistantMessage content = %T, want ai.ToolCall", message.Content[0])
	}
	assertToolCallArguments(t, toolCall, want)
}

func assertToolCallArguments(t *testing.T, toolCall ai.ToolCall, want string) {
	t.Helper()
	if got := toolCall.Arguments["path"]; got != want {
		t.Fatalf("ToolCall path = %#v, want %q", got, want)
	}
	segments, ok := toolCall.Arguments["segments"].([]any)
	if !ok || len(segments) != 1 || segments[0] != want {
		t.Fatalf("ToolCall segments = %#v, want [%q]", toolCall.Arguments["segments"], want)
	}
}

func mutateEventMessage(t *testing.T, event ai.AssistantMessageEvent) {
	t.Helper()
	switch event := event.(type) {
	case ai.AssistantMessageDoneEvent:
		event.Message.Content[0].(ai.ToolCall).Arguments["path"] = "after-next"
	case ai.AssistantMessageErrorEvent:
		event.Error.Content[0].(ai.ToolCall).Arguments["path"] = "after-next"
	default:
		t.Fatalf("terminal event = %T", event)
	}
}

func TestEventStreamWaiterCancellationDoesNotEndTheStream(t *testing.T) {
	t.Parallel()

	stream := ai.NewEventStream(
		func(event streamEvent) bool { return event.Terminal },
		func(event streamEvent) int { return event.Value },
	)
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, ok, err := stream.Next(canceled); !errors.Is(err, context.Canceled) || ok {
		t.Fatalf("Next(canceled) = (_, %t, %v), want (_, false, context.Canceled)", ok, err)
	}
	if _, err := stream.Result(canceled); !errors.Is(err, context.Canceled) {
		t.Fatalf("Result(canceled) error = %v, want context.Canceled", err)
	}

	stream.Push(streamEvent{Value: 9, Terminal: true})
	if result, err := stream.Result(context.Background()); err != nil || result != 9 {
		t.Fatalf("Result() = (%d, %v), want (9, nil)", result, err)
	}
}

func TestEventStreamEndWithoutOutcomeReturnsInternalError(t *testing.T) {
	t.Parallel()

	stream := ai.NewEventStream(
		func(event streamEvent) bool { return event.Terminal },
		func(event streamEvent) int { return event.Value },
	)
	stream.End()
	if _, err := stream.Result(context.Background()); !errors.Is(err, ai.ErrEventStreamInvariant) {
		t.Fatalf("Result() error = %v, want ErrEventStreamInvariant", err)
	}
	if errors.Is(streamError(t, stream), ai.ErrNotImplemented) {
		t.Fatal("internal stream error must not be ErrNotImplemented")
	}
}

func streamError(t *testing.T, stream *ai.EventStream[streamEvent, int]) error {
	t.Helper()
	_, err := stream.Result(context.Background())
	return err
}

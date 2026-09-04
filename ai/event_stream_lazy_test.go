package ai_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nankedr/pig/ai"
)

func TestAssistantStreamBuildsLazySnapshotsOnlyOnNext(t *testing.T) {
	stream := ai.NewAssistantMessageEventStream()
	var calls atomic.Int32
	stream.PushLazy(func() ai.AssistantMessageEvent {
		calls.Add(1)
		return ai.AssistantMessageStartEvent{Type: ai.AssistantMessageEventTypeStart}
	})
	stream.Push(ai.AssistantMessageDoneEvent{Type: ai.AssistantMessageEventTypeDone, Reason: ai.StopReasonStop, Message: ai.AssistantMessage{StopReason: ai.StopReasonStop}})
	if _, err := stream.Result(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 0 {
		t.Fatal("Result materialized an unconsumed partial")
	}
	if _, ok, err := stream.Next(context.Background()); !ok || err != nil || calls.Load() != 1 {
		t.Fatalf("Next = %v %v; calls=%d", ok, err, calls.Load())
	}
}

func TestAssistantStreamConcurrentNextWaitCanCancel(t *testing.T) {
	stream := ai.NewAssistantMessageEventStream()
	entered, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	stream.PushLazy(func() ai.AssistantMessageEvent {
		close(entered)
		<-release
		return ai.AssistantMessageStartEvent{Type: ai.AssistantMessageEventTypeStart}
	})
	go func() { defer close(finished); stream.Next(context.Background()) }()
	<-entered
	defer func() { close(release); <-finished }()
	ctx, cancel := context.WithCancelCause(context.Background())
	cause := errors.New("reader canceled")
	cancel(cause)
	returned := make(chan error, 1)
	go func() { _, _, err := stream.Next(ctx); returned <- err }()
	select {
	case err := <-returned:
		if !errors.Is(err, cause) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled reader blocked behind another Next")
	}
}

package ai_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/nankedr/pig/ai"
)

func TestFauxSessionCacheUsageIsDeterministicAndIsolated(t *testing.T) {
	message, err := ai.FauxAssistantMessage(
		ai.FauxAssistantText("done"),
		ai.FauxAssistantMessageOptions{Timestamp: ai.Some[int64](1)},
	)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := ai.NewFauxProvider(ai.RegisterFauxProviderOptions{
		API: "faux:usage", Provider: "faux-usage", Models: []ai.FauxModelDefinition{{ID: "usage-model"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	handle.SetResponses([]ai.FauxResponseStep{message, message, message, message})
	model, _ := handle.GetModel()
	models := ai.CreateModels()
	models.SetProvider(handle.Provider)
	input := ai.Context{
		SystemPrompt: ai.Some("rules"),
		Messages: []ai.Message{ai.UserMessage{
			Role: ai.MessageRoleUser, Content: ai.UserText("hello"), Timestamp: 1,
		}},
	}
	sessionA, sessionB := "session-a", "session-b"
	noCache := ai.CacheRetentionNone

	first := fauxUsageResult(t, handle.Provider.Stream(context.Background(), model, input, ai.StreamOptions{SessionID: &sessionA}))
	second, err := models.Complete(context.Background(), model, input, ai.ModelsStreamOptions{StreamOptions: ai.StreamOptions{SessionID: &sessionA}})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	isolated := fauxUsageResult(t, handle.Provider.Stream(context.Background(), model, input, ai.StreamOptions{SessionID: &sessionB}))
	disabled := fauxUsageResult(t, handle.Provider.Stream(context.Background(), model, input, ai.StreamOptions{
		SessionID: &sessionA, CacheRetention: &noCache,
	}))

	wantWrite := ai.Usage{Input: 6, Output: 1, CacheWrite: 6, TotalTokens: 13}
	wantRead := ai.Usage{Output: 1, CacheRead: 6, TotalTokens: 7}
	wantDisabled := ai.Usage{Input: 6, Output: 1, TotalTokens: 7}
	if !reflect.DeepEqual(first, wantWrite) || !reflect.DeepEqual(isolated, wantWrite) {
		t.Fatalf("first/isolated usage = (%#v, %#v), want %#v", first, isolated, wantWrite)
	}
	if !reflect.DeepEqual(second.Usage, wantRead) {
		t.Fatalf("same-session Complete usage = %#v, want %#v", second.Usage, wantRead)
	}
	if !reflect.DeepEqual(disabled, wantDisabled) {
		t.Fatalf("cache-disabled usage = %#v, want %#v", disabled, wantDisabled)
	}
}

func TestFauxCancellationReportsOnlyStreamedOutputUsage(t *testing.T) {
	one := 1
	rate := float64(100)
	message, _ := ai.FauxAssistantMessage(
		ai.FauxAssistantText("abcdefghijklmnop"),
		ai.FauxAssistantMessageOptions{Timestamp: ai.Some[int64](1)},
	)
	handle, err := ai.NewFauxProvider(ai.RegisterFauxProviderOptions{
		API: "faux:usage-cancel", Provider: "faux-usage-cancel", Models: []ai.FauxModelDefinition{{ID: "usage-model"}},
		TokenSize: &ai.FauxTokenSize{Min: &one, Max: &one}, TokensPerSecond: &rate,
	})
	if err != nil {
		t.Fatal(err)
	}
	handle.SetResponses([]ai.FauxResponseStep{message})
	model, _ := handle.GetModel()
	ctx, cancel := context.WithCancel(context.Background())
	stream := handle.Provider.Stream(ctx, model, ai.Context{}, ai.StreamOptions{})
	for {
		event, ok, err := stream.Next(context.Background())
		if err != nil || !ok {
			t.Fatalf("Next() = (%#v, %t, %v)", event, ok, err)
		}
		if event.AssistantMessageEventType() == ai.AssistantMessageEventTypeTextDelta {
			cancel()
			break
		}
	}
	wait, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()
	result, err := stream.Result(wait)
	if err != nil {
		t.Fatal(err)
	}
	if result.StopReason != ai.StopReasonAborted || result.Usage.Output != 1 || result.Usage.TotalTokens != 1 {
		t.Fatalf("aborted usage = %#v, want one streamed output token", result.Usage)
	}
}

func TestFauxPreCancelledRequestDoesNotPopulateCache(t *testing.T) {
	message, _ := ai.FauxAssistantMessage(ai.FauxAssistantText("done"))
	handle, err := ai.NewFauxProvider(ai.RegisterFauxProviderOptions{
		API: "faux:usage-pre-cancel", Provider: "faux-usage-pre-cancel", Models: []ai.FauxModelDefinition{{ID: "usage-model"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	handle.SetResponses([]ai.FauxResponseStep{message, message})
	model, _ := handle.GetModel()
	input := ai.Context{Messages: []ai.Message{ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText("hello")}}}
	session := "session"
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	aborted, err := handle.Provider.Stream(ctx, model, input, ai.StreamOptions{SessionID: &session}).Result(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	first := fauxUsageResult(t, handle.Provider.Stream(context.Background(), model, input, ai.StreamOptions{SessionID: &session}))
	if aborted.StopReason != ai.StopReasonAborted || aborted.Usage != (ai.Usage{}) {
		t.Fatalf("pre-cancelled result = %#v, want aborted with zero usage", aborted)
	}
	if first.CacheRead != 0 || first.CacheWrite == 0 {
		t.Fatalf("first live usage = %#v, want cache write without a hit", first)
	}
}

func fauxUsageResult(t *testing.T, stream *ai.AssistantMessageEventStream) ai.Usage {
	t.Helper()
	var terminal ai.AssistantMessage
	for {
		event, ok, err := stream.Next(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			break
		}
		if done, ok := event.(ai.AssistantMessageDoneEvent); ok {
			terminal = done.Message
		}
	}
	result, err := stream.Result(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(terminal.Usage, result.Usage) {
		t.Fatalf("terminal usage = %#v, Result usage = %#v", terminal.Usage, result.Usage)
	}
	return result.Usage
}

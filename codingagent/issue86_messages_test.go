package codingagent_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
)

func TestTurnRetryPreservesQueuedMessages(t *testing.T) {
	var session *codingagent.AgentSession
	var requests [][]string
	session = newMessage85Session(t, func(ctx context.Context, model ai.Model, input ai.Context, options ai.SimpleStreamOptions, core *ai.FauxCore) *ai.AssistantMessageEventStream {
		messages := make([]agent.AgentMessage, len(input.Messages))
		for i, message := range input.Messages {
			messages[i] = message
		}
		requests = append(requests, message85History(messages))
		if len(requests) == 1 {
			if err := session.Steer("steer"); err != nil {
				t.Error(err)
			}
			if err := session.SendUserMessage(ai.UserText("follow"), codingagent.SendUserMessageOptions{DeliverAs: codingagent.UserMessageDeliveryFollowUp}); err != nil {
				t.Error(err)
			}
			failed, _ := ai.FauxAssistantMessage(ai.FauxAssistantText("partial"), ai.FauxAssistantMessageOptions{StopReason: ai.Some(ai.StopReasonError), ErrorMessage: ai.Some("503")})
			reply, _ := ai.FauxAssistantMessage(ai.FauxAssistantText("reply"))
			core.SetResponses([]ai.FauxResponseStep{failed, reply, reply})
		}
		return core.StreamSimple(ctx, model, input, options)
	})
	delay := int64(1)
	if err := session.SettingsManager().ApplyOverrides(codingagent.Settings{Retry: &codingagent.RetrySettings{BaseDelayMS: &delay}}); err != nil {
		t.Fatal(err)
	}
	var starts, settled int
	_, err := session.Subscribe(func(event codingagent.AgentSessionEvent) {
		switch event.(type) {
		case codingagent.AgentSessionAutoRetryStartEvent:
			starts++
			if count, err := session.PendingMessageCount(); err != nil || count != 2 {
				t.Errorf("pending at retry: %d, %v", count, err)
			}
			if err := session.Steer("during backoff"); err == nil {
				t.Error("backoff must retain Legacy Agent idle admission")
			}
		case codingagent.AgentSessionAgentSettledEvent:
			settled++
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := session.SendUserMessage(ai.UserText("prompt")); err != nil {
		t.Fatal(err)
	}
	want := [][]string{{"user:prompt"}, {"user:prompt", "user:steer"}, {"user:prompt", "user:steer", "assistant:reply", "user:follow"}}
	if !reflect.DeepEqual(requests, want) {
		t.Fatalf("model inputs: %v, want %v", requests, want)
	}
	wantHistory := []string{"user:prompt", "assistant:partial", "user:steer", "assistant:reply", "user:follow", "assistant:reply"}
	if got := message85History(session.SessionManager().BuildSessionContext().Messages); !reflect.DeepEqual(got, wantHistory) {
		t.Fatalf("persistent history: %v, want %v", got, wantHistory)
	}
	if count, err := session.PendingMessageCount(); err != nil || count != 0 || starts != 1 || settled != 1 {
		t.Fatalf("settlement: pending=%d, starts=%d, settled=%d, err=%v", count, starts, settled, err)
	}
}

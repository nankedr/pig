package codingagent_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
)

func TestAutoCompactionSessionBashSettlement(t *testing.T) {
	generations := 0
	s := auto90Session(t, func(_ context.Context, model ai.Model, input ai.Context, _ ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
		system, _ := input.SystemPrompt.Value()
		if strings.HasPrefix(system, "You are a context summarization assistant.") {
			return auto90Reply(model, "checkpoint", auto90Response{})
		}
		generations++
		data, err := json.Marshal(input.Messages)
		if err != nil {
			t.Fatal(err)
		}
		if hasBash := strings.Contains(string(data), "merge-output"); hasBash != (generations == 3) {
			t.Fatalf("generation %d: unexpected Bash context %s", generations, data)
		}
		if generations == 1 {
			return auto90Reply(model, "overflow", auto90Response{Input: pointerTo(int64(0)), Error: "prompt is too long"})
		}
		return auto90Reply(model, "recovered", auto90Response{})
	}, nil)
	s.Subscribe(func(e codingagent.AgentSessionEvent) {
		if e.AgentSessionEventType() == codingagent.AgentSessionEventTypeCompactionStart {
			if err := s.RecordBashResult("external", codingagent.BashResult{Output: "merge-output", ExitCode: pointerTo(0)}); err != nil {
				t.Error(err)
			}
			if pending, _ := s.HasPendingBashMessages(); !pending {
				t.Error("Bash was not deferred during compaction")
			}
		}
		if e.AgentSessionEventType() == codingagent.AgentSessionEventTypeAgentSettled {
			if pending, _ := s.HasPendingBashMessages(); pending {
				t.Error("settled before Bash flush")
			}
		}
	})
	if outcome, err := auto90Run(context.Background(), s); err != nil || strings.Join(outcome.Text, "") != "recovered" {
		t.Fatalf("outcome=%+v err=%v", outcome, err)
	}
	if err := s.Prompt(context.Background(), "next"); err != nil {
		t.Fatal(err)
	}
	reopened, err := codingagent.OpenSessionManager(*s.SessionManager().GetSessionFile(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, entry := range reopened.GetEntries() {
		if entry.Type == "message" && entry.Message.MessageRole() == "bashExecution" {
			count++
		}
	}
	if count != 1 || generations != 3 {
		t.Fatalf("Bash records=%d generations=%d", count, generations)
	}
}

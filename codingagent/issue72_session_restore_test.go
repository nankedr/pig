package codingagent_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
)

func TestHistoricalSummariesReachResumedProvider(t *testing.T) {
	data, err := os.ReadFile("../parity/oracle/fixtures/session-interop.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Case struct {
			Input struct {
				Sessions []struct{ Name, Content string }
			}
		}
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	for _, input := range fixture.Case.Input.Sessions {
		if input.Name != "v1" && input.Name != "v2" && input.Name != "pi-writer-v3" {
			continue
		}
		t.Run(input.Name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "history.jsonl")
			if err := os.WriteFile(path, []byte(input.Content), 0600); err != nil {
				t.Fatal(err)
			}
			manager, err := codingagent.OpenSessionManager(path, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			provider, err := ai.NewFauxProvider()
			if err != nil {
				t.Fatal(err)
			}
			reply, err := ai.FauxAssistantMessage(ai.FauxAssistantText("after summary"))
			if err != nil {
				t.Fatal(err)
			}
			called := false
			provider.SetResponses([]ai.FauxResponseStep{ai.FauxResponseFactory(func(ctx ai.Context, _ *ai.SimpleStreamOptions, _ *ai.FauxProviderState, _ ai.Model) (ai.AssistantMessage, error) {
				called = true
				encoded, err := json.Marshal(ctx.Messages)
				if err != nil {
					t.Fatal(err)
				}
				for _, want := range []string{"old summary", "branch summary", "custom text", "hello", "continue after summary"} {
					if !strings.Contains(string(encoded), want) {
						t.Errorf("Provider context lost %q: %s", want, encoded)
					}
				}
				return reply, nil
			})})
			model, _ := provider.GetModel()
			created, err := codingagent.CreateAgentSession(context.Background(), codingagent.CreateAgentSessionOptions{CWD: t.TempDir(), Model: &model, Provider: provider.Provider, SessionManager: manager})
			if err != nil {
				t.Fatal(err)
			}
			if err := created.Session.Prompt(context.Background(), "continue after summary"); err != nil {
				t.Fatal(err)
			}
			if err := created.Session.Dispose(); err != nil {
				t.Fatal(err)
			}
			if !called {
				t.Fatal("Provider was not called")
			}
			reopened, err := codingagent.OpenSessionManager(path, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			msgs := reopened.BuildSessionContext().Messages
			if msgs[len(msgs)-1].MessageRole() != ai.MessageRoleAssistant {
				t.Fatal("continued Assistant was not persisted")
			}
		})
	}
}

func TestSessionWriterPreservesTypedMessagesForPi(t *testing.T) {
	path := filepath.Join(t.TempDir(), "typed.jsonl")
	if err := os.WriteFile(path, []byte("{\"type\":\"session\",\"version\":3,\"id\":\"typed\",\"cwd\":\"/project\"}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	manager, err := codingagent.OpenSessionManager(path, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, message := range []agent.AgentMessage{
		agent.CreateCustomMessage("absent", ai.UserText("custom"), true, ai.Absent[ai.JSONValue](), 1),
		agent.CreateCustomMessage("null", ai.UserText("custom null"), false, ai.Null[ai.JSONValue](), 2),
		agent.CreateBranchSummaryMessage("branch", "source", 3),
		agent.CreateCompactionSummaryMessage("compaction", 4, 4),
	} {
		if _, err := manager.AppendMessage(message); err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	records := codingagent.ParseSessionEntries(string(data))
	if len(records) != 5 {
		t.Fatalf("records = %d", len(records))
	}
	for i, wantRole := range []string{"custom", "custom", "branchSummary", "compactionSummary"} {
		var record struct{ Message map[string]json.RawMessage }
		if err := json.Unmarshal(records[i+1].Raw, &record); err != nil {
			t.Fatal(err)
		}
		if string(record.Message["role"]) != `"`+wantRole+`"` || record.Message["timestamp"] == nil {
			t.Fatalf("Pi wire message: %s", records[i+1].Raw)
		}
		if i == 0 && record.Message["details"] != nil {
			t.Fatal("absent details was materialized")
		}
		if i == 1 && string(record.Message["details"]) != "null" {
			t.Fatal("explicit null details was lost")
		}
		if i == 2 && string(record.Message["fromId"]) != `"source"` {
			t.Fatal("branch reference was lost")
		}
		if i == 3 && string(record.Message["tokensBefore"]) != "4" {
			t.Fatal("compaction tokens were lost")
		}
	}
	reopened, err := codingagent.OpenSessionManager(path, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(codingagent.ConvertToLLM(reopened.BuildSessionContext().Messages)); got != 4 {
		t.Fatalf("restored LLM messages = %d", got)
	}
}

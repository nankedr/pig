package parity_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fmt"
	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/codingagent"
	"github.com/nankedr/pig/internal/baseline"
	"github.com/nankedr/pig/internal/parity"
)

func TestSessionInteropParity(t *testing.T) {
	root := parityRepoRoot(t)
	lock, _, err := baseline.Load(filepath.Join(root, "parity/baseline"))
	if err != nil {
		t.Fatal(err)
	}
	locked := parity.Baseline{ID: lock.BaselineID, Commit: lock.Upstream.Commit, Repository: lock.Upstream.Repository}
	fixture, err := parity.LoadFixture(filepath.Join(root, "parity/oracle/fixtures/session-interop.json"), locked)
	if err != nil {
		t.Fatal(err)
	}
	oracle, err := parity.NewFixtureDriver(fixture, locked)
	if err != nil {
		t.Fatal(err)
	}
	result, err := parity.RunCase(context.Background(), fixture.Case, oracle, parity.DriverFunc{SurfaceName: parity.SurfaceGoSDK, ObserveFunc: func(_ context.Context, c parity.Case) (parity.Observation, error) {
		var input struct {
			Sessions    []struct{ Name, Content string }
			Append      json.RawMessage
			LabelTarget string
		}
		if err := json.Unmarshal(c.Input, &input); err != nil {
			return parity.Observation{}, err
		}
		message, err := agent.UnmarshalAgentMessage(input.Append)
		if err != nil {
			return parity.Observation{}, err
		}
		outcomes := []any{}
		for _, item := range input.Sessions {
			path := filepath.Join(t.TempDir(), item.Name+".jsonl")
			if err := os.WriteFile(path, []byte(item.Content), 0600); err != nil {
				return parity.Observation{}, err
			}
			manager, err := codingagent.OpenSessionManager(path, nil, nil)
			if err != nil {
				after, _ := os.ReadFile(path)
				outcomes = append(outcomes, map[string]any{"name": item.Name, "error": strings.Contains(err.Error(), "not a valid pi session"), "preserved": string(after) == item.Content})
				continue
			}
			ctx := manager.BuildSessionContext()
			messages := []any{}
			for _, msg := range ctx.Messages {
				b, err := agent.MarshalAgentMessage(msg)
				if err != nil {
					return parity.Observation{}, err
				}
				var v any
				if err := json.Unmarshal(b, &v); err != nil {
					return parity.Observation{}, err
				}
				switch msg := msg.(type) {
				case agent.CompactionSummaryMessage:
					v = map[string]any{"role": msg.Role, "summary": msg.Summary, "tokensBefore": msg.TokensBefore, "timestamp": msg.Timestamp}
				case agent.BranchSummaryMessage:
					v = map[string]any{"role": msg.Role, "summary": msg.Summary, "fromId": msg.FromID, "timestamp": msg.Timestamp}
				case agent.CustomMessage:
					v = map[string]any{"role": msg.Role, "customType": msg.CustomType, "content": msg.Content, "display": msg.Display, "details": msg.Details, "timestamp": msg.Timestamp}
				}
				messages = append(messages, v)
			}
			var model any
			if ctx.Model != nil {
				model = map[string]any{"provider": ctx.Model.Provider, "modelId": ctx.Model.ModelID}
			}
			projected := map[string]any{"messages": messages, "thinkingLevel": ctx.ThinkingLevel, "model": model}
			if _, err := manager.AppendMessage(message); err != nil {
				return parity.Observation{}, err
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return parity.Observation{}, err
			}
			records := []map[string]any{}
			for _, e := range codingagent.ParseSessionEntries(string(data)) {
				var record map[string]any
				if err := json.Unmarshal(e.Raw, &record); err != nil {
					return parity.Observation{}, err
				}
				records = append(records, record)
			}
			last := records[len(records)-1]
			if msg, ok := last["message"].(map[string]any); ok && msg["content"] == "continued" {
				last["id"] = "appended"
				last["timestamp"] = "2025-01-01T00:00:00.000Z"
			}
			if strings.HasPrefix(item.Name, "v1") {
				ids := map[string]string{}
				for i, e := range records[1:] {
					ids[e["id"].(string)] = fmt.Sprintf("entry-%d", i)
				}
				for _, e := range records[1:] {
					for _, key := range []string{"id", "parentId", "firstKeptEntryId", "fromId", "targetId"} {
						if id, ok := e[key].(string); ok && ids[id] != "" {
							e[key] = ids[id]
						}
					}
				}
				for _, msg := range messages {
					m := msg.(map[string]any)
					if id, ok := m["fromId"].(string); ok && ids[id] != "" {
						m["fromId"] = ids[id]
					}
				}
			}
			reopened, err := codingagent.OpenSessionManager(path, nil, nil)
			if err != nil {
				return parity.Observation{}, err
			}
			roles := []string{}
			for _, m := range reopened.BuildSessionContext().Messages {
				roles = append(roles, string(m.MessageRole()))
			}
			outcomes = append(outcomes, map[string]any{"name": item.Name, "llmContext": codingagent.ConvertToLLM(ctx.Messages), "label": manager.GetLabel(input.LabelTarget), "sessionName": manager.GetSessionName(), "records": records, "context": projected, "reopenedRoles": roles})
		}
		outcome, err := json.Marshal(outcomes)
		effects := []parity.SideEffect{}
		return parity.Observation{Outcome: outcome, SideEffects: &effects}, err
	}})
	if err != nil || !result.Match {
		t.Fatalf("session interop parity: %v; differences: %+v; got: %s", err, result.Differences, result.Pig.Outcome)
	}
}

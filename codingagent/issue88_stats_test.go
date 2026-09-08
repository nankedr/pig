package codingagent_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
	"github.com/nankedr/pig/internal/baseline"
	"github.com/nankedr/pig/internal/parity"
)

func stats88Session(t *testing.T, manager *codingagent.SessionManager, window int64, stream agent.StreamFunction) *codingagent.AgentSession {
	t.Helper()
	core, err := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	model, _ := core.GetModel()
	model.ContextWindow = window
	if stream == nil {
		stream = func(context.Context, ai.Model, ai.Context, ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
			t.Error("query triggered a provider request")
			return nil
		}
	}
	disabled := false
	settings, err := codingagent.NewInMemorySettingsManager(codingagent.Settings{Retry: &codingagent.RetrySettings{Enabled: &disabled}, Compaction: &codingagent.CompactionSettings{Enabled: false}})
	if err != nil {
		t.Fatal(err)
	}
	created, err := codingagent.CreateAgentSession(context.Background(), codingagent.CreateAgentSessionOptions{CWD: manager.GetCWD(), Model: &model, StreamFunction: stream, SessionManager: manager, SettingsManager: settings, Tools: []string{"read"}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = created.Session.Dispose() })
	return created.Session
}

func stats88Observe(t *testing.T, s *codingagent.AgentSession) map[string]any {
	t.Helper()
	stats, err := s.GetSessionStats()
	if err != nil {
		t.Fatal(err)
	}
	usage, err := s.GetContextUsage()
	if err != nil {
		t.Fatal(err)
	}
	last, err := s.GetLastAssistantText()
	if err != nil {
		t.Fatal(err)
	}
	contextValue := func(u *codingagent.ContextUsage) any {
		if u == nil {
			return nil
		}
		return map[string]any{"contextWindow": u.ContextWindow, "tokens": u.Tokens, "percent": u.Percent}
	}
	roles := []ai.MessageRole{}
	for _, m := range s.Messages() {
		roles = append(roles, m.MessageRole())
	}
	return map[string]any{"stats": map[string]any{
		"sessionFile": stats.SessionFile != nil && s.SessionFile() != nil && *stats.SessionFile == *s.SessionFile(), "sessionId": stats.SessionID,
		"userMessages": stats.UserMessages, "assistantMessages": stats.AssistantMessages, "toolResults": stats.ToolResults, "toolCalls": stats.ToolCalls, "totalMessages": stats.TotalMessages,
		"tokens": map[string]any{"input": stats.Tokens.Input, "output": stats.Tokens.Output, "cacheRead": stats.Tokens.CacheRead, "cacheWrite": stats.Tokens.CacheWrite, "total": stats.Tokens.TotalTokens}, "cost": stats.Cost, "contextUsage": contextValue(stats.ContextUsage),
	}, "context": contextValue(usage), "last": last, "roles": roles}
}

func TestSessionStatsParity(t *testing.T) {
	root := issue32RepoRoot(t)
	lock, _, err := baseline.Load(filepath.Join(root, "parity/baseline"))
	if err != nil {
		t.Fatal(err)
	}
	locked := parity.Baseline{ID: lock.BaselineID, Commit: lock.Upstream.Commit, Repository: lock.Upstream.Repository}
	fixture, err := parity.LoadFixture(filepath.Join(root, "parity/oracle/fixtures/session-stats.json"), locked)
	if err != nil {
		t.Fatal(err)
	}
	oracle, err := parity.NewFixtureDriver(fixture, locked)
	if err != nil {
		t.Fatal(err)
	}
	result, err := parity.RunCase(context.Background(), fixture.Case, oracle, parity.DriverFunc{SurfaceName: parity.SurfaceGoSDK, ObserveFunc: func(ctx context.Context, c parity.Case) (parity.Observation, error) {
		var input struct {
			Scenarios []struct {
				Name          string
				Records       []json.RawMessage
				ContextWindow int64
			}
		}
		if err := json.Unmarshal(c.Input, &input); err != nil {
			return parity.Observation{}, err
		}
		observations := []map[string]any{}
		for _, scenario := range input.Scenarios {
			dir := t.TempDir()
			file := filepath.Join(dir, "session.jsonl")
			var data []byte
			for _, record := range scenario.Records {
				line, err := json.Marshal(record)
				if err != nil {
					return parity.Observation{}, err
				}
				data = append(data, line...)
				data = append(data, '\n')
			}
			if err := os.WriteFile(file, data, 0600); err != nil {
				return parity.Observation{}, err
			}
			manager, err := codingagent.OpenSessionManager(file, &dir, &dir)
			if err != nil {
				return parity.Observation{}, err
			}
			s := stats88Session(t, manager, scenario.ContextWindow, nil)
			beforeFile, _ := os.ReadFile(file)
			beforeEntries, beforeMessages := manager.GetEntries(), s.Messages()
			got := stats88Observe(t, s)
			if scenario.Name == "paid-compaction-unknown" || scenario.Name == "branch-without-compaction" {
				stats88CheckProcess(t, s)
			}
			got["name"] = scenario.Name
			observations = append(observations, got)
			afterFile, _ := os.ReadFile(file)
			if string(beforeFile) != string(afterFile) || !reflect.DeepEqual(beforeEntries, manager.GetEntries()) || !reflect.DeepEqual(beforeMessages, s.Messages()) {
				t.Fatalf("%s query mutated session", scenario.Name)
			}
		}
		outcome, err := json.Marshal(observations)
		return parity.Observation{Outcome: outcome, SideEffects: &[]parity.SideEffect{}}, err
	}})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Match {
		t.Fatalf("stats parity: %+v\nwant: %s\ngot: %s", result, fixture.Observation.Outcome, result.Pig.Outcome)
	}
}

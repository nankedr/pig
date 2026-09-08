package codingagent_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
	"github.com/nankedr/pig/internal/baseline"
	"github.com/nankedr/pig/internal/parity"
)

func tree91Messages(messages []agent.AgentMessage) []map[string]string {
	result := []map[string]string{}
	for _, message := range messages {
		data, _ := agent.MarshalAgentMessage(message)
		var m struct {
			Role    string
			Content json.RawMessage
		}
		_ = json.Unmarshal(data, &m)
		var text string
		if json.Unmarshal(m.Content, &text) != nil {
			var blocks []struct{ Type, Text string }
			_ = json.Unmarshal(m.Content, &blocks)
			for _, b := range blocks {
				if b.Type == "text" {
					text += b.Text
				}
			}
		}
		result = append(result, map[string]string{"role": m.Role, "text": text})
	}
	return result
}

func TestSessionTreeNavigationParity(t *testing.T) {
	root := issue32RepoRoot(t)
	lock, _, err := baseline.Load(filepath.Join(root, "parity/baseline"))
	if err != nil {
		t.Fatal(err)
	}
	locked := parity.Baseline{ID: lock.BaselineID, Commit: lock.Upstream.Commit, Repository: lock.Upstream.Repository}
	fixture, err := parity.LoadFixture(filepath.Join(root, "parity/oracle/fixtures/session-tree-navigation.json"), locked)
	if err != nil {
		t.Fatal(err)
	}
	oracle, err := parity.NewFixtureDriver(fixture, locked)
	if err != nil {
		t.Fatal(err)
	}
	result, err := parity.RunCase(context.Background(), fixture.Case, oracle, parity.DriverFunc{SurfaceName: parity.SurfaceGoSDK, ObserveFunc: func(ctx context.Context, c parity.Case) (parity.Observation, error) {
		var input struct {
			Sequence []string
			History  []json.RawMessage
		}
		if err := json.Unmarshal(c.Input, &input); err != nil {
			t.Fatal(err)
		}
		runtime, models, _ := config87Runtime(t)
		models[1].Reasoning = true
		dir := t.TempDir()
		manager, err := codingagent.NewSessionManager(dir, &dir)
		if err != nil {
			t.Fatal(err)
		}
		core, _ := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
		settings, _ := codingagent.NewInMemorySettingsManager(codingagent.Settings{})
		created, err := codingagent.CreateAgentSession(ctx, codingagent.CreateAgentSessionOptions{CWD: dir, Model: &models[0], ModelRuntime: runtime, SessionManager: manager, SettingsManager: settings, ThinkingLevel: "off", StreamFunction: agent.StreamFunction(core.StreamSimple), Tools: []string{}})
		if err != nil {
			t.Fatal(err)
		}
		s := created.Session
		defer s.Dispose()
		names := map[string]string{}
		states, requests := []map[string]any{}, []map[string]any{}
		request := func(prompt string) {
			core.SetResponses([]ai.FauxResponseStep{ai.FauxResponseFactory(func(c ai.Context, o *ai.SimpleStreamOptions, _ *ai.FauxProviderState, m ai.Model) (ai.AssistantMessage, error) {
				messages := []agent.AgentMessage{}
				for _, message := range c.Messages {
					messages = append(messages, message)
				}
				thinking := "off"
				if o != nil && o.Reasoning != nil {
					thinking = string(*o.Reasoning)
				}
				requests = append(requests, map[string]any{"messages": tree91Messages(messages), "model": m.ID, "thinking": thinking})
				return ai.FauxAssistantMessage(ai.FauxAssistantText("reply:" + prompt))
			})})
			if err := s.Prompt(ctx, prompt); err != nil {
				t.Fatal(err)
			}
		}
		capture := func(name string, r codingagent.NavigateTreeResult) {
			c := manager.BuildSessionContext()
			var leaf, model any
			if id := manager.GetLeafID(); id != nil {
				leaf = names[*id]
				if leaf == "" {
					e := manager.GetEntry(*id)
					leaf = e.Type
				}
			}
			if c.Model != nil {
				model = c.Model.ModelID
			}
			labels := []*string{}
			for _, n := range []string{"u2", "a1"} {
				for id, v := range names {
					if v == n {
						labels = append(labels, manager.GetLabel(id))
					}
				}
			}
			summaries := 0
			for _, e := range manager.GetEntries() {
				if e.Type == "branch_summary" {
					summaries++
				}
			}
			states = append(states, map[string]any{"name": name, "editorText": r.EditorText, "cancelled": r.Cancelled, "leaf": leaf, "messages": tree91Messages(s.Messages()), "model": s.Model().ID, "thinking": s.ThinkingLevel(), "pathModel": model, "pathThinking": c.ThinkingLevel, "labels": labels, "summaryCount": summaries})
		}
		navigate := func(name, id string, options ...codingagent.NavigateTreeOptions) {
			r, err := s.NavigateTree(ctx, id, options...)
			if err != nil {
				t.Fatal(err)
			}
			capture(name, r)
		}
		request(input.Sequence[0])
		a1 := *manager.GetLeafID()
		names[a1] = "a1"
		for _, e := range manager.GetEntries() {
			if e.Message != nil && e.Message.MessageRole() == ai.MessageRoleUser {
				names[e.ID] = "u1"
			}
		}
		if err := s.SetModel(models[1]); err != nil {
			t.Fatal(err)
		}
		if err := s.SetThinkingLevel("high"); err != nil {
			t.Fatal(err)
		}
		request(input.Sequence[1])
		a2 := *manager.GetLeafID()
		names[a2] = "a2"
		u2 := ""
		for _, e := range manager.GetEntries() {
			if e.Message != nil && e.Message.MessageRole() == ai.MessageRoleUser && names[e.ID] == "" {
				u2 = e.ID
				names[e.ID] = "u2"
			}
		}
		count := len(manager.GetEntries())
		navigate("same", a2, codingagent.NavigateTreeOptions{Summarize: true, Label: "ignored"})
		sameCount := len(manager.GetEntries()) == count
		navigate("user", u2, codingagent.NavigateTreeOptions{Label: "retry", CustomInstructions: "ignored", ReplaceInstructions: true})
		navigate("ancestor", a1)
		request(input.Sequence[2])
		branch := *manager.GetLeafID()
		names[branch] = "branch"
		navigate("other", a2)
		request(input.Sequence[3])
		navigate("return", branch)
		before := *manager.GetLeafID()
		_, err = s.NavigateTree(ctx, "missing")
		invalid := err != nil && *manager.GetLeafID() == before
		reopened, err := codingagent.OpenSessionManager(*manager.GetSessionFile(), nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		original := reopened.GetEntry(a2) != nil
		hasBranch := reopened.GetEntry(branch) != nil
		cxt := reopened.BuildSessionContext()
		persisted := map[string]any{"original": original, "branch": hasBranch, "label": reopened.GetLabel(u2), "messages": tree91Messages(cxt.Messages), "thinking": cxt.ThinkingLevel, "model": cxt.Model.ModelID}
		if _, err = s.NavigateTree(ctx, a1); err != nil {
			t.Fatal(err)
		}
		if _, err = s.NavigateTree(ctx, branch, codingagent.NavigateTreeOptions{Label: "branch end"}); err != nil {
			t.Fatal(err)
		}
		reopenedBranch, err := codingagent.OpenSessionManager(*s.SessionFile(), nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		restored, err := codingagent.CreateAgentSession(ctx, codingagent.CreateAgentSessionOptions{CWD: dir, ModelRuntime: runtime, SessionManager: reopenedBranch, SettingsManager: settings, Tools: []string{}})
		if err != nil {
			t.Fatal(err)
		}
		restoredBranch := map[string]any{"model": restored.Session.Model().ID, "thinking": restored.Session.ThinkingLevel(), "messages": tree91Messages(restored.Session.Messages())}
		if err := restored.Session.Dispose(); err != nil {
			t.Fatal(err)
		}
		historical := []map[string]any{}
		for _, target := range []string{"root", "custom", "tool", "compact", "summary", "label", "model", "thinking"} {
			file := filepath.Join(dir, "history.jsonl")
			header, _ := json.Marshal(map[string]any{"type": "session", "version": 3, "id": "history", "cwd": dir, "timestamp": "2026-01-01T00:00:00Z"})
			data := append(header, '\n')
			for _, entry := range input.History {
				var compact bytes.Buffer
				if err := json.Compact(&compact, entry); err != nil {
					t.Fatal(err)
				}
				data = append(data, compact.Bytes()...)
				data = append(data, '\n')
			}
			if err := os.WriteFile(file, data, 0600); err != nil {
				t.Fatal(err)
			}
			m, err := codingagent.OpenSessionManager(file, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			created, err := codingagent.CreateAgentSession(ctx, codingagent.CreateAgentSessionOptions{CWD: dir, Model: &models[1], ModelRuntime: runtime, SessionManager: m, SettingsManager: settings, ThinkingLevel: "high", StreamFunction: agent.StreamFunction(core.StreamSimple), Tools: []string{}})
			if err != nil {
				t.Fatal(err)
			}
			h := created.Session
			r, err := h.NavigateTree(ctx, target)
			if err != nil {
				t.Fatal(err)
			}
			historical = append(historical, map[string]any{"target": target, "leaf": m.GetLeafID(), "editorText": r.EditorText, "messages": tree91Messages(h.Messages()), "model": h.Model().ID, "thinking": h.ThinkingLevel()})
			if err := h.Dispose(); err != nil {
				t.Fatal(err)
			}
		}
		outcome, _ := json.Marshal(map[string]any{"states": states, "requests": requests, "sameCount": sameCount, "invalid": invalid, "persisted": persisted, "historical": historical, "restoredBranch": restoredBranch})
		return parity.Observation{Outcome: outcome, SideEffects: &[]parity.SideEffect{}}, nil
	}})
	if err != nil || !result.Match {
		t.Fatalf("tree navigation parity: %v\n%+v\nwant %s\ngot %s", err, result, fixture.Observation.Outcome, result.Pig.Outcome)
	}
}

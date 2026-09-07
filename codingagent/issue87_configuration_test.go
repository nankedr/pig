package codingagent_test

import (
	"context"
	"encoding/json"

	"path/filepath"
	"strings"
	"testing"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
	"github.com/nankedr/pig/internal/baseline"
	"github.com/nankedr/pig/internal/parity"
)

func config87Runtime(t *testing.T) (*codingagent.ModelRuntime, []ai.Model, *codingagent.AuthStorage) {
	t.Helper()
	dir := t.TempDir()
	store, err := codingagent.NewAuthStorage(filepath.Join(dir, "auth.json"))
	if err != nil {
		t.Fatal(err)
	}
	credential76Set(t, store, "deepseek", "fixture")
	credential76Set(t, store, "openai", "fixture")
	runtime, err := codingagent.NewModelRuntime(context.Background(), codingagent.CreateModelRuntimeOptions{Offline: true, Credentials: store, ModelsPath: ai.Null[string]()})
	if err != nil {
		t.Fatal(err)
	}
	models := []ai.Model{}
	for i, id := range []string{"deepseek-v4-flash", "deepseek-v4-pro", "deepseek-v4-flash"} {
		m, ok, e := runtime.GetModel("deepseek", id)
		if e != nil || !ok {
			t.Fatal(e)
		}
		m.Reasoning = i != 1
		m.ThinkingLevelMap = nil
		if i == 2 {
			m.ThinkingLevelMap = ai.ThinkingLevelMap{"xhigh": ai.Some("xhigh"), "max": ai.Some("max")}
		}
		models = append(models, m)
	}
	if _, err = runtime.GetAvailable(context.Background()); err != nil {
		t.Fatal(err)
	}
	return runtime, models, store
}

func TestSessionConfigurationParity(t *testing.T) {
	root := issue32RepoRoot(t)
	lock, _, err := baseline.Load(filepath.Join(root, "parity/baseline"))
	if err != nil {
		t.Fatal(err)
	}
	locked := parity.Baseline{ID: lock.BaselineID, Commit: lock.Upstream.Commit, Repository: lock.Upstream.Repository}
	fixture, err := parity.LoadFixture(filepath.Join(root, "parity/oracle/fixtures/session-configuration.json"), locked)
	if err != nil {
		t.Fatal(err)
	}
	oracle, err := parity.NewFixtureDriver(fixture, locked)
	if err != nil {
		t.Fatal(err)
	}
	result, err := parity.RunCase(context.Background(), fixture.Case, oracle, parity.DriverFunc{SurfaceName: parity.SurfaceGoSDK, ObserveFunc: func(ctx context.Context, _ parity.Case) (parity.Observation, error) {
		runtime, models, _ := config87Runtime(t)
		core, _ := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
		manager := codingagent.NewInMemorySessionManager(t.TempDir())
		settings, _ := codingagent.NewInMemorySettingsManager(codingagent.Settings{})
		created, err := codingagent.CreateAgentSession(ctx, codingagent.CreateAgentSessionOptions{CWD: manager.GetCWD(), Model: &models[0], ModelRuntime: runtime, StreamFunction: agent.StreamFunction(core.StreamSimple), SessionManager: manager, SettingsManager: settings, ThinkingLevel: ai.ModelThinkingLevelOff, Tools: []string{"read", "bash", "edit", "write"}})
		if err != nil {
			return parity.Observation{}, err
		}
		s := created.Session
		defer s.Dispose()
		states, requests, entries := []map[string]any{}, []map[string]any{}, []map[string]any{}
		events := []agent.ThinkingLevel{}
		_, err = s.Subscribe(func(e codingagent.AgentSessionEvent) {
			if v, ok := e.(codingagent.AgentSessionThinkingLevelChangedEvent); ok {
				events = append(events, v.Level)
			}
		})
		if err != nil {
			return parity.Observation{}, err
		}
		capture := func(label string) {
			levels, e := s.GetAvailableThinkingLevels()
			if e != nil {
				t.Fatal(e)
			}
			supports, e := s.SupportsThinking()
			if e != nil {
				t.Fatal(e)
			}
			states = append(states, map[string]any{"label": label, "model": s.Model().ID, "thinking": s.ThinkingLevel(), "levels": levels, "supports": supports, "tools": s.GetActiveToolNames()})
		}
		request := func() {
			message, _ := ai.FauxAssistantMessage(ai.FauxAssistantText("ok"))
			core.SetResponses([]ai.FauxResponseStep{ai.FauxResponseFactory(func(c ai.Context, o *ai.SimpleStreamOptions, _ *ai.FauxProviderState, m ai.Model) (ai.AssistantMessage, error) {
				prompt, _ := c.SystemPrompt.Value()
				names, promptNames := []string{}, []string{}
				for _, tool := range c.Tools {
					names = append(names, tool.Name)
				}
				for _, n := range []string{"read", "bash", "edit", "write"} {
					if strings.Contains(prompt, "- "+n+":") {
						promptNames = append(promptNames, n)
					}
				}
				thinking := ai.ModelThinkingLevelOff
				if o != nil && o.Reasoning != nil {
					thinking = ai.ModelThinkingLevel(*o.Reasoning)
				}
				requests = append(requests, map[string]any{"model": m.ID, "thinking": thinking, "tools": names, "promptTools": promptNames})
				return message, nil
			})})
			if e := s.Prompt(ctx, "go"); e != nil {
				t.Fatal(e)
			}
		}
		must := func(e error) {
			if e != nil {
				t.Fatal(e)
			}
		}
		must(s.SetThinkingLevel("high"))
		capture("high")
		request()
		missing := models[0]
		missing.ID = "missing"
		must(s.SetScopedModels([]codingagent.ScopedModel{{Model: models[0]}, {Model: missing}, {Model: models[1]}}))
		_, err = s.CycleModel(ctx)
		must(err)
		capture("nonreasoning")
		must(s.SetScopedModels([]codingagent.ScopedModel{{Model: models[1]}, {Model: models[2], ThinkingLevel: "max"}}))
		_, err = s.CycleModel(ctx)
		must(err)
		capture("scoped-max")
		_, err = s.CycleThinkingLevel()
		must(err)
		capture("cycle-off")
		must(s.SetThinkingLevel("high"))
		_, err = s.CycleThinkingLevel()
		must(err)
		capture("cycle-xhigh")
		_, err = s.CycleModel(ctx, codingagent.ModelCycleBackward)
		must(err)
		capture("backward")
		must(s.SetModel(models[0]))
		capture("restore-preference")
		must(s.SetActiveToolsByName([]string{"write", "read"}))
		capture("tools")
		request()
		must(s.SetActiveToolsByName([]string{}))
		capture("empty-tools")
		request()
		must(s.SetScopedModels([]codingagent.ScopedModel{{Model: models[0]}, {Model: missing}}))
		single, err := s.CycleModel(ctx)
		must(err)
		for _, e := range manager.GetEntries() {
			if e.Type == "model_change" {
				entries = append(entries, map[string]any{"type": e.Type, "model": e.ModelID})
			}
			if e.Type == "thinking_level_change" {
				entries = append(entries, map[string]any{"type": e.Type, "thinking": e.ThinkingLevel})
			}
		}
		level, _ := settings.GetDefaultThinkingLevel()
		model, _ := settings.GetDefaultModel()
		outcome, _ := json.Marshal(map[string]any{"states": states, "requests": requests, "events": events, "entries": entries, "single": single, "defaultThinking": level, "defaultModel": model})
		return parity.Observation{Outcome: outcome, SideEffects: &[]parity.SideEffect{}}, nil
	}})
	if err != nil {
		t.Fatalf("%v\nwant: %s\ngot: %s", err, fixture.Observation.Outcome, result.Pig.Outcome)
	}
	if !result.Match {
		t.Fatalf("configuration parity: %+v", result)
	}
}

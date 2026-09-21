package codingagent_test

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
	"github.com/nankedr/pig/internal/baseline"
	"github.com/nankedr/pig/internal/parity"
	"path/filepath"
	"reflect"
	"testing"
)

func TestModelScopeAtomic117(t *testing.T) {
	runtime, models, _ := config87Runtime(t)
	storage := &config87Storage{}
	settings, err := codingagent.NewSettingsManagerFromStorage(storage)
	if err != nil {
		t.Fatal(err)
	}
	core, _ := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
	created, err := codingagent.CreateAgentSession(context.Background(), codingagent.CreateAgentSessionOptions{CWD: t.TempDir(), AgentDir: t.TempDir(), Model: &models[0], ModelRuntime: runtime, SettingsManager: settings, StreamFunction: agent.StreamFunction(core.StreamSimple)})
	if err != nil {
		t.Fatal(err)
	}
	defer created.Session.Dispose()
	s := created.Session
	ids := []string{"deepseek/deepseek-v4-pro", "missing/unavailable"}
	if err = s.SetEnabledModels(ids, false); err != nil {
		t.Fatal(err)
	}
	if len(s.ScopedModels()) != 1 || s.ScopedModels()[0].Model.ID != "deepseek-v4-pro" {
		t.Fatal(s.ScopedModels())
	}
	if got, _ := settings.GetEnabledModels(); len(got) != 0 {
		t.Fatal(got)
	}
	if err = s.SetEnabledModels(ids, true); err != nil {
		t.Fatal(err)
	}
	if got, _ := settings.GetEnabledModels(); !reflect.DeepEqual(got, ids) {
		t.Fatal(got)
	}
	storage.fail = true
	if err = s.SetEnabledModels([]string{"deepseek/deepseek-v4-flash"}, true); err == nil {
		t.Fatal("save must fail")
	}
	if got, _ := settings.GetEnabledModels(); !reflect.DeepEqual(got, ids) || s.ScopedModels()[0].Model.ID != "deepseek-v4-pro" {
		t.Fatal("partial update", got, s.ScopedModels())
	}
	storage.fail = false
	unavailable := []string{"missing/one", "missing/two"}
	if err = s.SetEnabledModels(unavailable, true); err != nil {
		t.Fatal(err)
	}
	if got, _ := settings.GetEnabledModels(); !reflect.DeepEqual(got, unavailable) {
		t.Fatal("unavailable IDs lost", got)
	}
	changes := [][]string{}
	selector := codingagent.NewScopedModelsSelectorComponent(codingagent.ModelsConfig{AllModels: models[:2]}, codingagent.ModelsCallbacks{OnChange: func(ids []string) error { changes = append(changes, ids); return nil }})
	if err = selector.HandleInput("\r"); err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 || !reflect.DeepEqual(changes[0], []string{"deepseek/deepseek-v4-flash"}) {
		t.Fatal(changes)
	}
	failed := codingagent.NewScopedModelsSelectorComponent(codingagent.ModelsConfig{AllModels: models[:2]}, codingagent.ModelsCallbacks{OnChange: func([]string) error { return errors.New("busy") }})
	if err = failed.HandleInput("\r"); err == nil {
		t.Fatal("must propagate failed mutation")
	}
}

func TestModelScopeParity117(t *testing.T) {
	root := issue32RepoRoot(t)
	lock, _, err := baseline.Load(filepath.Join(root, "parity/baseline"))
	if err != nil {
		t.Fatal(err)
	}
	locked := parity.Baseline{ID: lock.BaselineID, Commit: lock.Upstream.Commit, Repository: lock.Upstream.Repository}
	fixture, err := parity.LoadFixture(filepath.Join(root, "parity/oracle/fixtures/model-scope.json"), locked)
	if err != nil {
		t.Fatal(err)
	}
	oracle, err := parity.NewFixtureDriver(fixture, locked)
	if err != nil {
		t.Fatal(err)
	}
	result, err := parity.RunCase(context.Background(), fixture.Case, oracle, parity.DriverFunc{SurfaceName: parity.SurfaceGoSDK, ObserveFunc: func(ctx context.Context, c parity.Case) (parity.Observation, error) {
		var input struct {
			Models []ai.Model
			Cases  []struct {
				ID            string
				Keys, Enabled []string
			}
		}
		data, _ := json.Marshal(c.Input)
		if err := json.Unmarshal(data, &input); err != nil {
			return parity.Observation{}, err
		}
		outcome := []map[string]any{}
		for _, tc := range input.Cases {
			changes, saved := [][]string{}, [][]string{}
			cancelled := false
			s := codingagent.NewScopedModelsSelectorComponent(codingagent.ModelsConfig{AllModels: input.Models, EnabledModelIDs: tc.Enabled}, codingagent.ModelsCallbacks{OnChange: func(ids []string) error { changes = append(changes, ids); return nil }, OnPersist: func(ids []string) error { saved = append(saved, ids); return nil }, OnCancel: func() { cancelled = true }})
			for _, key := range tc.Keys {
				if err := s.HandleInput(key); err != nil {
					return parity.Observation{}, err
				}
			}
			outcome = append(outcome, map[string]any{"id": tc.ID, "changes": changes, "saved": saved, "cancelled": cancelled})
		}
		raw, _ := json.Marshal(outcome)
		return parity.Observation{Outcome: raw, SideEffects: &[]parity.SideEffect{}}, nil
	}})
	if err != nil || !result.Match {
		t.Fatalf("%v\nwant %s\ngot %s", err, fixture.Observation.Outcome, result.Pig.Outcome)
	}
}

package codingagent_test

import (
	"context"
	"encoding/json"
	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
	"github.com/nankedr/pig/internal/baseline"
	"github.com/nankedr/pig/internal/parity"
	"path/filepath"
	"testing"
)

func TestModelSelectorParity117(t *testing.T) {
	root := issue32RepoRoot(t)
	lock, _, err := baseline.Load(filepath.Join(root, "parity/baseline"))
	if err != nil {
		t.Fatal(err)
	}
	locked := parity.Baseline{ID: lock.BaselineID, Commit: lock.Upstream.Commit, Repository: lock.Upstream.Repository}
	fixture, err := parity.LoadFixture(filepath.Join(root, "parity/oracle/fixtures/model-selector.json"), locked)
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
				ID    string
				Keys  []string
				Scope []int
				Empty bool
			}
		}
		data, _ := json.Marshal(c.Input)
		if err := json.Unmarshal(data, &input); err != nil {
			return parity.Observation{}, err
		}
		outcome := []map[string]any{}
		for _, tc := range input.Cases {
			models := input.Models
			if tc.Empty {
				models = nil
			}
			scope := []codingagent.ScopedModel{}
			for _, i := range tc.Scope {
				scope = append(scope, codingagent.ScopedModel{Model: input.Models[i]})
			}
			selected := ""
			cancelled := false
			selector := codingagent.NewModelSelectorComponent(models, &input.Models[1], scope, func(m ai.Model) { selected = string(m.Provider) + "/" + m.ID }, func() { cancelled = true })
			for _, key := range tc.Keys {
				if err := selector.HandleInput(key); err != nil {
					return parity.Observation{}, err
				}
			}
			_ = selector.Dispose()
			outcome = append(outcome, map[string]any{"id": tc.ID, "selected": selected, "cancelled": cancelled})
		}
		raw, _ := json.Marshal(outcome)
		return parity.Observation{Outcome: raw, SideEffects: &[]parity.SideEffect{}}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Match {
		t.Fatalf("%+v", result)
	}
}

func TestThinkingSelector117(t *testing.T) {
	selected := ""
	selector := codingagent.NewThinkingSelectorComponent("high", []agent.ThinkingLevel{"off", "high", "max"}, func(level agent.ThinkingLevel) { selected = string(level) }, nil)
	if err := selector.HandleInput("\x1b[B"); err != nil {
		t.Fatal(err)
	}
	if err := selector.HandleInput("\r"); err != nil {
		t.Fatal(err)
	}
	if selected != "max" {
		t.Fatal(selected)
	}
}

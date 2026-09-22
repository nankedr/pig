package codingagent_test

import (
	"context"
	"encoding/json"

	"github.com/nankedr/pig/codingagent"
	"github.com/nankedr/pig/internal/baseline"
	"github.com/nankedr/pig/internal/parity"
	"path/filepath"
	"testing"
)

func TestSessionSelectorParity118(t *testing.T) {
	root := issue32RepoRoot(t)
	lock, _, err := baseline.Load(filepath.Join(root, "parity/baseline"))
	if err != nil {
		t.Fatal(err)
	}
	locked := parity.Baseline{ID: lock.BaselineID, Commit: lock.Upstream.Commit, Repository: lock.Upstream.Repository}
	fixture, err := parity.LoadFixture(filepath.Join(root, "parity/oracle/fixtures/session-selector.json"), locked)
	if err != nil {
		t.Fatal(err)
	}
	oracle, err := parity.NewFixtureDriver(fixture, locked)
	if err != nil {
		t.Fatal(err)
	}
	result, err := parity.RunCase(context.Background(), fixture.Case, oracle, parity.DriverFunc{SurfaceName: parity.SurfaceGoSDK, ObserveFunc: func(ctx context.Context, c parity.Case) (parity.Observation, error) {
		var input struct {
			Sessions []codingagent.SessionInfo
			Cases    []struct {
				ID   string
				Keys []string
			}
		}
		data, _ := json.Marshal(c.Input)
		if err := json.Unmarshal(data, &input); err != nil {
			return parity.Observation{}, err
		}
		outcome := []map[string]any{}
		for _, tc := range input.Cases {
			selected := ""
			cancelled := false
			current := func(context.Context) ([]codingagent.SessionInfo, error) {
				var entries []codingagent.SessionInfo
				for _, s := range input.Sessions {
					if s.CWD == "/project" {
						entries = append(entries, s)
					}
				}
				return entries, nil
			}
			all := func(context.Context) ([]codingagent.SessionInfo, error) { return input.Sessions, nil }
			selector, err := codingagent.NewSessionSelectorComponent(ctx, current, all, func(path string) { selected = path }, func() { cancelled = true })
			if err != nil {
				return parity.Observation{}, err
			}

			for _, key := range tc.Keys {
				if err := selector.HandleInput(key); err != nil {
					return parity.Observation{}, err
				}
			}
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

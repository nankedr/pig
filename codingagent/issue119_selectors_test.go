package codingagent_test

import (
	"context"
	"encoding/json"
	"github.com/nankedr/pig/codingagent"
	"github.com/nankedr/pig/internal/baseline"
	"github.com/nankedr/pig/internal/parity"
	"os"
	"path/filepath"
	"testing"
)

func TestForkSelectorParity119(t *testing.T) {
	root := issue32RepoRoot(t)
	lock, _, err := baseline.Load(filepath.Join(root, "parity/baseline"))
	if err != nil {
		t.Fatal(err)
	}
	locked := parity.Baseline{ID: lock.BaselineID, Commit: lock.Upstream.Commit, Repository: lock.Upstream.Repository}
	fixture, err := parity.LoadFixture(filepath.Join(root, "parity/oracle/fixtures/fork-selector.json"), locked)
	if err != nil {
		t.Fatal(err)
	}
	oracle, err := parity.NewFixtureDriver(fixture, locked)
	if err != nil {
		t.Fatal(err)
	}
	result, err := parity.RunCase(context.Background(), fixture.Case, oracle, parity.DriverFunc{SurfaceName: parity.SurfaceGoSDK, ObserveFunc: func(ctx context.Context, c parity.Case) (parity.Observation, error) {
		var input struct {
			Messages []struct{ ID, Text string }
			Cases    []struct {
				ID, Initial string
				Keys        []string
			}
		}
		data, _ := json.Marshal(c.Input)
		if err := json.Unmarshal(data, &input); err != nil {
			return parity.Observation{}, err
		}
		messages := []codingagent.ForkMessage{}
		for _, m := range input.Messages {
			messages = append(messages, codingagent.ForkMessage{EntryID: m.ID, Text: m.Text})
		}
		outcome := []map[string]any{}
		for _, tc := range input.Cases {
			selected := ""
			cancelled := false
			s := codingagent.NewUserMessageSelectorComponent(messages, func(id string) { selected = id }, func() { cancelled = true }, tc.Initial)
			for _, key := range tc.Keys {
				if err := s.HandleInput(key); err != nil {
					return parity.Observation{}, err
				}
			}
			outcome = append(outcome, map[string]any{"id": tc.ID, "selected": selected, "cancelled": cancelled})
		}
		raw, _ := json.Marshal(outcome)
		return parity.Observation{Outcome: raw, SideEffects: &[]parity.SideEffect{}}, nil
	}})
	if err != nil || !result.Match {
		t.Fatalf("%v\nwant %s\ngot %s", err, result.Oracle.Outcome, result.Pig.Outcome)
	}
}

func TestTreeSelectorParity119(t *testing.T) {
	root := issue32RepoRoot(t)
	lock, _, err := baseline.Load(filepath.Join(root, "parity/baseline"))
	if err != nil {
		t.Fatal(err)
	}
	locked := parity.Baseline{ID: lock.BaselineID, Commit: lock.Upstream.Commit, Repository: lock.Upstream.Repository}
	fixture, err := parity.LoadFixture(filepath.Join(root, "parity/oracle/fixtures/tree-selector.json"), locked)
	if err != nil {
		t.Fatal(err)
	}
	oracle, err := parity.NewFixtureDriver(fixture, locked)
	if err != nil {
		t.Fatal(err)
	}
	result, err := parity.RunCase(context.Background(), fixture.Case, oracle, parity.DriverFunc{SurfaceName: parity.SurfaceGoSDK, ObserveFunc: func(ctx context.Context, c parity.Case) (parity.Observation, error) {
		var input struct {
			Entries []json.RawMessage
			Leaf    string
			Cases   []struct {
				ID, Initial, Mode string
				Keys              []string
			}
		}
		data, _ := json.Marshal(c.Input)
		if err := json.Unmarshal(data, &input); err != nil {
			return parity.Observation{}, err
		}
		path := filepath.Join(t.TempDir(), "tree.jsonl")
		data = []byte(`{"type":"session","version":3,"id":"fixture","cwd":"/tmp","timestamp":"2026-01-01T00:00:00.000Z"}` + "\n")
		for _, entry := range input.Entries {
			data = append(data, entry...)
			data = append(data, '\n')
		}
		if err := os.WriteFile(path, data, 0600); err != nil {
			return parity.Observation{}, err
		}
		manager, err := codingagent.OpenSessionManager(path, nil, nil)
		if err != nil {
			return parity.Observation{}, err
		}
		outcome := []map[string]any{}
		for _, tc := range input.Cases {
			tree, err := manager.GetTree()
			if err != nil {
				return parity.Observation{}, err
			}
			selected := ""
			cancelled := false
			labels := []map[string]any{}
			copies := []string{}
			s := codingagent.NewTreeSelectorComponent(tree, &input.Leaf, 20, func(id string) { selected = id }, func() { cancelled = true }, codingagent.TreeSelectorOptions{InitialSelectedID: tc.Initial, InitialFilterMode: tc.Mode, OnLabelChange: func(id string, label *string) error {
				labels = append(labels, map[string]any{"id": id, "label": label})
				return nil
			}})
			s.OnCopy(func(text string) { copies = append(copies, text) })
			for _, key := range tc.Keys {
				if err := s.HandleInput(key); err != nil {
					return parity.Observation{}, err
				}
			}
			outcome = append(outcome, map[string]any{"id": tc.ID, "selected": selected, "cancelled": cancelled, "labels": labels, "copies": copies})
		}
		raw, _ := json.Marshal(outcome)
		return parity.Observation{Outcome: raw, SideEffects: &[]parity.SideEffect{}}, nil
	}})
	if err != nil || !result.Match {
		t.Fatalf("%v\nwant %s\ngot %s", err, result.Oracle.Outcome, result.Pig.Outcome)
	}
}

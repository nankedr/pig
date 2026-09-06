package parity_test

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

func TestProjectTrustParity(t *testing.T) {
	root := parityRepoRoot(t)
	lock, _, err := baseline.Load(filepath.Join(root, "parity/baseline"))
	if err != nil {
		t.Fatal(err)
	}
	locked := parity.Baseline{ID: lock.BaselineID, Commit: lock.Upstream.Commit, Repository: lock.Upstream.Repository}
	fixture, err := parity.LoadFixture(filepath.Join(root, "parity/oracle/fixtures/project-trust.json"), locked)
	if err != nil {
		t.Fatal(err)
	}
	oracle, err := parity.NewFixtureDriver(fixture, locked)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	result, err := parity.RunCase(context.Background(), fixture.Case, oracle, parity.DriverFunc{SurfaceName: parity.SurfaceGoSDK, ObserveFunc: func(ctx context.Context, c parity.Case) (parity.Observation, error) {
		cwd := filepath.Join(dir, "project")
		child := filepath.Join(cwd, "child")
		must := func(err error) {
			t.Helper()
			if err != nil {
				t.Fatal(err)
			}
		}
		must(os.MkdirAll(child, 0700))
		must(os.MkdirAll(filepath.Join(dir, ".agents", "skills"), 0700))
		resources := []bool{}
		probe := func(path string) {
			value, err := codingagent.HasTrustRequiringProjectResources(ctx, path)
			must(err)
			resources = append(resources, value)
		}
		probe(dir)
		probe(child)
		must(os.Mkdir(filepath.Join(child, ".pig"), 0700))
		probe(child)
		must(os.WriteFile(filepath.Join(child, ".pig", "settings.json"), []byte("{}"), 0600))
		probe(child)
		store := codingagent.NewProjectTrustStore(filepath.Join(dir, "agent"))
		decisions := []*bool{}
		get := func() { d, err := store.Get(ctx, child); must(err); decisions = append(decisions, d) }
		get()
		must(store.Set(ctx, cwd, codingagent.ProjectTrustDecisionTrusted()))
		get()
		must(store.Set(ctx, child, codingagent.ProjectTrustDecisionUntrusted()))
		get()
		must(store.SetMany(ctx, []codingagent.ProjectTrustUpdate{{Path: cwd, Decision: codingagent.ProjectTrustDecisionTrusted()}, {Path: child}}))
		get()
		entry, err := store.GetEntry(ctx, child)
		must(err)
		canonical, err := filepath.EvalSymlinks(cwd)
		must(err)
		outcome, err := json.Marshal(map[string]any{"resources": resources, "decisions": decisions, "nearestParent": entry != nil && entry.Path == canonical})
		effects := []parity.SideEffect{}
		return parity.Observation{Outcome: outcome, SideEffects: &effects}, err
	}})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Match {
		t.Fatalf("trust parity: %+v", result)
	}
}

package parity_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/internal/baseline"
	"github.com/nankedr/pig/internal/parity"
)

func TestDeferredToolsParity(t *testing.T) {
	fixture, locked := loadDeferredToolsFixture(t, "deferred-tools.json")
	oracle, err := parity.NewFixtureDriver(fixture, locked)
	if err != nil {
		t.Fatal(err)
	}
	pig := parity.DriverFunc{SurfaceName: parity.SurfaceGoSDK, ObserveFunc: observeDeferredTools}
	result, err := parity.RunCase(context.Background(), fixture.Case, oracle, pig)
	if err != nil || !result.Match || len(result.Normalizations) != 0 {
		t.Fatalf("deferred tools parity = %+v, %v", result, err)
	}
}

func TestDeferredToolsUsedAfterMarkerDeviation(t *testing.T) {
	fixture, _ := loadDeferredToolsFixture(t, "deferred-tools-used-deviation.json")
	pig, err := observeDeferredTools(context.Background(), fixture.Case)
	if err != nil {
		t.Fatal(err)
	}
	for _, check := range []struct {
		name      string
		outcome   json.RawMessage
		immediate []string
		deferred  []string
	}{
		{"Pi", fixture.Observation.Outcome, []string{"discover", "spare"}, []string{"read", "write"}},
		{"Pig", pig.Outcome, []string{"discover", "read", "spare"}, []string{"write"}},
	} {
		var outcomes []deferredToolsOutcome
		if err := json.Unmarshal(check.outcome, &outcomes); err != nil || len(outcomes) != 1 {
			t.Fatalf("%s outcome = %s, %v", check.name, check.outcome, err)
		}
		names := func(tools []ai.Tool) []string {
			result := make([]string, len(tools))
			for i, tool := range tools {
				result[i] = tool.Name
			}
			return result
		}
		if !slices.Equal(names(outcomes[0].Immediate), check.immediate) || !slices.Equal(names(outcomes[0].Deferred), check.deferred) {
			t.Errorf("%s partition = %s; want immediate %v, deferred %v (ADR-0016)", check.name, check.outcome, check.immediate, check.deferred)
		}
	}
}

func loadDeferredToolsFixture(t *testing.T, name string) (parity.Fixture, parity.Baseline) {
	t.Helper()
	root := parityRepoRoot(t)
	lock, _, err := baseline.Load(filepath.Join(root, "parity", "baseline"))
	if err != nil {
		t.Fatal(err)
	}
	locked := parity.Baseline{ID: lock.BaselineID, Commit: lock.Upstream.Commit, Repository: lock.Upstream.Repository}
	fixture, err := parity.LoadFixture(filepath.Join(root, "parity", "oracle", "fixtures", name), locked)
	if err != nil {
		t.Fatal(err)
	}
	return fixture, locked
}

type deferredToolsOutcome struct {
	ID        string    `json:"id"`
	Immediate []ai.Tool `json:"immediate"`
	Deferred  []ai.Tool `json:"deferred"`
}

func observeDeferredTools(_ context.Context, declaration parity.Case) (parity.Observation, error) {
	var input struct {
		Scenarios []struct {
			ID        string     `json:"id"`
			Enabled   bool       `json:"enabled"`
			Normalize bool       `json:"normalize"`
			Context   ai.Context `json:"context"`
		} `json:"scenarios"`
	}
	if err := json.Unmarshal(declaration.Input, &input); err != nil {
		return parity.Observation{}, err
	}
	outcomes := make([]deferredToolsOutcome, 0, len(input.Scenarios))
	for _, scenario := range input.Scenarios {
		var normalize func(string) string
		if scenario.Normalize {
			normalize = strings.ToLower
		}
		immediate, deferred := ai.SplitDeferredTools(scenario.Context, scenario.Enabled, normalize)
		outcomes = append(outcomes, deferredToolsOutcome{scenario.ID, immediate, deferred})
	}
	outcome, err := json.Marshal(outcomes)
	sideEffects := []parity.SideEffect{}
	return parity.Observation{Outcome: outcome, SideEffects: &sideEffects}, err
}

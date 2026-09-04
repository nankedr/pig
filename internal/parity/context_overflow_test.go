package parity_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/internal/parity"
)

func TestContextOverflowParity(t *testing.T) {
	fixture, locked := loadDeferredToolsFixture(t, "context-overflow.json")
	oracle, err := parity.NewFixtureDriver(fixture, locked)
	if err != nil {
		t.Fatal(err)
	}
	pig := parity.DriverFunc{SurfaceName: parity.SurfaceGoSDK, ObserveFunc: observeContextOverflow}
	result, err := parity.RunCase(context.Background(), fixture.Case, oracle, pig)
	if err != nil || !result.Match || len(result.Normalizations) != 0 {
		t.Fatalf("context overflow parity differences = %+v, %v; got %s", result.Differences, err, result.Pig.Outcome)
	}
}

func observeContextOverflow(_ context.Context, declaration parity.Case) (parity.Observation, error) {
	var input struct {
		Estimates []struct {
			ID      string     `json:"id"`
			Context ai.Context `json:"context"`
		} `json:"estimates"`
		Overflows []struct {
			ID               string          `json:"id"`
			Message          json.RawMessage `json:"message"`
			ContextWindow    *int64          `json:"contextWindow"`
			DesiredMaxOutput int64           `json:"desiredMaxOutput"`
		} `json:"overflows"`
	}
	if err := json.Unmarshal(declaration.Input, &input); err != nil {
		return parity.Observation{}, err
	}
	type estimate struct {
		ID string `json:"id"`
		ai.ContextUsageEstimate
	}
	type overflow struct {
		ID          string `json:"id"`
		Overflow    bool   `json:"overflow"`
		Recoverable bool   `json:"recoverable"`
		Matches     []int  `json:"matches"`
	}
	outcomes := struct {
		Estimates []estimate `json:"estimates"`
		Overflows []overflow `json:"overflows"`
	}{Estimates: []estimate{}, Overflows: []overflow{}}
	for _, scenario := range input.Estimates {
		outcomes.Estimates = append(outcomes.Estimates, estimate{scenario.ID, ai.EstimateContextTokens(scenario.Context)})
	}
	for _, scenario := range input.Overflows {
		decoded, err := ai.UnmarshalMessage(scenario.Message)
		if err != nil {
			return parity.Observation{}, err
		}
		message := decoded.(ai.AssistantMessage)
		windows := []int64{}
		if scenario.ContextWindow != nil {
			windows = append(windows, *scenario.ContextWindow)
		}
		matches := []int{}
		text, _ := message.ErrorMessage.Value()
		for i, pattern := range ai.GetOverflowPatterns() {
			if pattern.MatchString(text) {
				matches = append(matches, i)
			}
		}
		outcomes.Overflows = append(outcomes.Overflows, overflow{scenario.ID, ai.IsContextOverflow(message, windows...), ai.IsRecoverableLength(message, scenario.DesiredMaxOutput), matches})
	}
	outcome, err := json.Marshal(outcomes)
	sideEffects := []parity.SideEffect{}
	return parity.Observation{Outcome: outcome, SideEffects: &sideEffects}, err
}

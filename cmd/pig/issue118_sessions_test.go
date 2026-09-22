//go:build darwin || linux

package main

import (
	"context"
	"encoding/json"
	"github.com/nankedr/pig/internal/baseline"
	"github.com/nankedr/pig/internal/parity"
	"os"
	"os/exec"
	"testing"
)

func TestPigSessionSelectionParity118(t *testing.T) {
	binary := buildPigBinary(t)
	if os.Getenv("PIG_TEST_RACE") == "1" {
		cmd := exec.Command("go", "build", "-race", "-o", binary, ".")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s: %v", output, err)
		}
	}
	cmd := exec.Command("python3", "../../parity/terminal/session-selection.py", binary, "--pig")
	data, err := cmd.Output()
	if err != nil {
		if e, ok := err.(*exec.ExitError); ok {
			t.Fatalf("%s: %v", e.Stderr, err)
		}
		t.Fatal(err)
	}
	lock, _, err := baseline.Load("../../parity/baseline")
	if err != nil {
		t.Fatal(err)
	}
	locked := parity.Baseline{ID: lock.BaselineID, Commit: lock.Upstream.Commit, Repository: lock.Upstream.Repository}
	fixture, err := parity.LoadFixture("../../parity/oracle/fixtures/session-selection-cli.json", locked)
	if err != nil {
		t.Fatal(err)
	}
	oracle, err := parity.NewFixtureDriver(fixture, locked)
	if err != nil {
		t.Fatal(err)
	}
	result, err := parity.RunCase(context.Background(), fixture.Case, oracle, parity.DriverFunc{SurfaceName: parity.SurfaceCLI, ObserveFunc: func(context.Context, parity.Case) (parity.Observation, error) {
		return parity.Observation{Outcome: json.RawMessage(data), SideEffects: &[]parity.SideEffect{}}, nil
	}})
	if err != nil || !result.Match {
		t.Fatalf("%v\nwant %s\ngot %s", err, fixture.Observation.Outcome, data)
	}
}

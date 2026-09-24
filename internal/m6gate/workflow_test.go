//go:build darwin || linux

package m6gate_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/nankedr/pig/internal/baseline"
	"github.com/nankedr/pig/internal/parity"
)

func TestPigM6Workflow(t *testing.T) { runM6Workflow(t, false) }

func TestM6SDKWorkflow(t *testing.T) { runM6Workflow(t, true) }

func runM6Workflow(t *testing.T, sdk bool) {
	binary := os.Getenv("PIG_BINARY")
	if sdk {
		binary = os.Getenv("PIG_M6_SDK_BINARY")
	}
	if binary == "" {
		binary = filepath.Join(t.TempDir(), "pig")
		target := "../../cmd/pig"
		if sdk {
			target = "../../examples/m6-workflow"
		}
		args := []string{"build", "-o", binary, target}
		if os.Getenv("PIG_TEST_RACE") == "1" {
			args = append([]string{"build", "-race"}, args[1:]...)
		}
		if out, err := exec.Command("go", args...).CombinedOutput(); err != nil {
			t.Fatalf("build: %v\n%s", err, out)
		}
	}
	lock, _, err := baseline.Load("../../parity/baseline")
	if err != nil {
		t.Fatal(err)
	}
	locked := parity.Baseline{ID: lock.BaselineID, Commit: lock.Upstream.Commit, Repository: lock.Upstream.Repository}
	for _, mode := range []string{"regular", "fullscreen"} {
		t.Run(mode, func(t *testing.T) {
			fixture, err := parity.LoadFixture("../../parity/oracle/fixtures/m6-workflow-"+mode+".json", locked)
			if err != nil {
				t.Fatal(err)
			}
			oracle, err := parity.NewFixtureDriver(fixture, locked)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()
			flag := "--pig"
			if sdk {
				flag = "--sdk"
			}
			args := []string{"../../parity/terminal/m6-workflow.py", binary, flag}
			if mode == "fullscreen" {
				args = append(args, "--fullscreen")
			}
			cmd := exec.CommandContext(ctx, "python3", args...)
			if dir := os.Getenv("PIG_M6_ARTIFACTS"); dir != "" {
				surface := "cli"
				if sdk {
					surface = "sdk"
				}
				cmd.Env = append(os.Environ(), "PIG_M6_ARTIFACTS="+filepath.Join(dir, surface))
			}
			data, err := cmd.Output()
			if err != nil {
				if exit, ok := err.(*exec.ExitError); ok {
					t.Fatalf("PTY: %v\n%s", err, exit.Stderr)
				}
				t.Fatal(err)
			}
			result, err := parity.RunCase(ctx, fixture.Case, oracle, parity.DriverFunc{SurfaceName: parity.SurfaceCLI, ObserveFunc: func(context.Context, parity.Case) (parity.Observation, error) {
				return parity.Observation{Outcome: json.RawMessage(data), SideEffects: &[]parity.SideEffect{}}, nil
			}})
			if err != nil || !result.Match {
				t.Fatalf("%v\nwant %s\ngot %s", err, fixture.Observation.Outcome, data)
			}
		})
	}
}

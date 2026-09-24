package m6gate_test

import (
	"github.com/nankedr/pig/codingagent"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestM6FreezePlan(t *testing.T) {
	cmd := exec.Command("make", "-n", "m6-freeze", "PIG_PI_ORACLE_CHECKOUT=/prepared/pi", "PIG_PI_SOURCE_CHECKOUT=/pristine/pi", "PIG_M6_MANUAL_EVIDENCE=/manual.json")
	cmd.Dir = "../.."
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("freeze plan: %v\n%s", err, output)
	}
	for _, step := range []string{"M6 freeze requires a clean Pig checkout", "go test ./... -count=1", "go test -race ./... -count=1", "go vet ./...", "CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build ./...", "-count=20 -shuffle=on", "PIG_TEST_RACE=1", "parity/oracle/m6-workflow.mjs", "parity/oracle/interactive.mjs", "parity/oracle/editor.mjs", "parity/oracle/editor-cli.mjs", "parity/oracle/layout-scrolling.mjs", "parity/oracle/layout-scrolling-cli.mjs", "parity/oracle/trust-dialog.mjs", "parity/oracle/maintenance-cli.mjs", "parity/oracle/external-editor-cli.mjs", "parity/oracle/session-selection-cli.mjs", "parity/oracle/settings-cli.mjs", "parity/oracle/themes-cli.mjs", "parity/oracle/interactive-branches-cli.mjs", "parity/oracle/queues.mjs", "parity/oracle/bash-cli.mjs", "parity/oracle/rpc.mjs", "parity/extract/surface.mjs", "scripts/m6-evidence.py", "PIG_REQUIRE_LIVE=1"} {
		if !strings.Contains(string(output), step) {
			t.Errorf("freeze omits %s", step)
		}
	}
}

func TestM6CatalogAudit(t *testing.T) {
	cmd := exec.Command("python3", "../../scripts/m6-audit.py")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("audit: %v\n%s", err, output)
	}
}

func TestM6ReleaseVersion(t *testing.T) {
	if codingagent.Version != "0.6.0" {
		t.Fatalf("SDK version = %s", codingagent.Version)
	}
}

func TestM6ManualEvidenceGate(t *testing.T) {
	cmd := exec.Command("python3", "../../scripts/m6-evidence-test.py")
	cmd.Env = append(os.Environ(), "PYTHONDONTWRITEBYTECODE=1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("manual evidence validation: %v\n%s", err, out)
	}
	cmd = exec.Command("python3", "../../scripts/m6-evidence.py", "")
	if out, err := cmd.CombinedOutput(); err == nil || !strings.Contains(string(out), "PIG_M6_MANUAL_EVIDENCE") {
		t.Fatalf("missing human evidence accepted: %v: %s", err, out)
	}
}

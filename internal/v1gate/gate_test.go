package v1gate_test

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/nankedr/pig/codingagent"
)

func TestV1CatalogEvidence(t *testing.T) {
	if output, err := exec.Command("python3", "../../scripts/v1-audit.py").CombinedOutput(); err != nil {
		t.Fatalf("V1 evidence: %v\n%s", err, output)
	}
}

func TestV1RejectsMissingOrSkippedLiveEvidence(t *testing.T) {
	command := exec.Command("python3", "../../scripts/v1-evidence-test.py")
	command.Env = append(os.Environ(), "PYTHONDONTWRITEBYTECODE=1")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("live evidence validation: %v\n%s", err, output)
	}
	command = exec.Command("python3", "../../scripts/v1-live.py")
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, "DEEPSEEK_API_KEY=") {
			command.Env = append(command.Env, value)
		}
	}
	output, err := command.CombinedOutput()
	if err == nil || (!strings.Contains(string(output), "requires DEEPSEEK_API_KEY") && !strings.Contains(string(output), "requires darwin/arm64")) {
		t.Fatalf("missing credential or unsupported platform accepted: %v\n%s", err, output)
	}
}

func TestV1ReleaseVersion(t *testing.T) {
	if codingagent.Version != "1.0.0" {
		t.Fatalf("SDK version = %s", codingagent.Version)
	}
}

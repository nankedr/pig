package m5gate_test

import (
	"context"
	"github.com/nankedr/pig/codingagent"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestM5ReleaseVersion(t *testing.T) {
	if codingagent.Version != "0.5.0" {
		t.Fatalf("SDK version = %s", codingagent.Version)
	}
	for _, flag := range []string{"--version", "-v"} {
		result, err := codingagent.RunCLI(context.Background(), codingagent.CLIInvocation{Arguments: []string{flag}})
		if err != nil || result.Stdout != codingagent.Version+"\n" || result.Stderr != "" {
			t.Fatalf("%s: result=%+v, err=%v", flag, result, err)
		}
	}
}

func TestM5FreezePlan(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	cmd := exec.Command("make", "-n", "m5-freeze", "PIG_PI_ORACLE_CHECKOUT=/prepared/pi", "PIG_PI_SOURCE_CHECKOUT=/pristine/pi")
	cmd.Dir = filepath.Join(filepath.Dir(file), "../..")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("freeze plan: %v\n%s", err, output)
	}
	plan := string(output)
	for _, step := range []string{
		"M5 freeze requires a clean Pig checkout", "Unicode 16.0",
		"go test ./... -count=1", "go test -race ./... -count=1", "go vet ./...",
		"CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build ./...",
		"-count=20 -shuffle=on", "-count=5 -shuffle=on",
		"parity/oracle/session-interop.mjs", "parity/oracle/rpc.mjs", "parity/oracle/rpc-control.mjs", "parity/oracle/rpc-lifecycle.mjs", "parity/oracle/export-html.mjs",
		"parity/export-html/check.mjs", "TestInventoryDriftAgainstUpstream", "parity/extract/surface.mjs",
		"PIG_REQUIRE_LIVE=1 go test ./codingagent -run '^TestDeepSeekLiveHeadlessReadContinuation$'",
		"go run ./examples/m5-workflow", "go run ./examples/find-ls-read",
		"parity/oracle/local-extensions.mjs", "parity/oracle/local-resources.mjs", "parity/oracle/session-reload.mjs", "parity/oracle/context-files.mjs", "parity/oracle/system-prompts.mjs", "parity/oracle/prompt-templates.mjs", "parity/oracle/skills.mjs", "parity/oracle/themes.mjs", "Test(SessionReload", "PigM5",
	} {
		if !strings.Contains(plan, step) {
			t.Errorf("freeze omits %s", step)
		}
	}
	if strings.Index(plan, "M5 freeze requires") > strings.Index(plan, "go test ./...") {
		t.Error("clean check must precede tests")
	}
}
func TestM5CleanRejectsDirtyCheckout(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	makefile := filepath.Join(filepath.Dir(file), "../../Makefile")
	dir := t.TempDir()
	git := exec.Command("git", "init", "--quiet", dir)
	if output, err := git.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}
	run := func() ([]byte, error) {
		cmd := exec.Command("make", "-f", makefile, "m5-clean")
		cmd.Dir = dir
		return cmd.CombinedOutput()
	}
	if output, err := run(); err != nil {
		t.Fatalf("clean checkout rejected: %v: %s", err, output)
	}
	if err := os.WriteFile(filepath.Join(dir, "untracked.txt"), []byte("dirty"), 0600); err != nil {
		t.Fatal(err)
	}
	if output, err := run(); err == nil || !strings.Contains(string(output), "M5 freeze requires a clean Pig checkout") {
		t.Fatalf("dirty checkout accepted or wrong failure: %v: %s", err, output)
	}
}

package m5gate_test

import (
	"context"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestM5Workflow(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "run", "-race", "./examples/m5-workflow")
	cmd.Dir = filepath.Join(filepath.Dir(file), "../..")
	output, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(output), "PASS: local resources, template, skill, reload, persistence, theme and HTML export") {
		t.Fatalf("SDK workflow: %v\n%s", err, output)
	}
}

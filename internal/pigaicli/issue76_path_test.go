package pigaicli_test

import (
	"bytes"
	"errors"
	"path/filepath"
	"testing"

	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
	"github.com/nankedr/pig/internal/pigaicli"
)

func TestCredential76PigAISharedPathAndStubs(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PIG_CODING_AGENT_DIR", "")
	for _, agentDir := range []string{"", "~/custom"} {
		t.Setenv("PIG_CODING_AGENT_DIR", agentDir)
		for _, path := range []string{"", "~/explicit/auth.json", filepath.Join(t.TempDir(), "auth.json"), "relative-auth.json"} {
			pig, err := codingagent.ResolveAuthPath(path)
			if err != nil {
				t.Fatal(err)
			}
			pigAI, err := pigaicli.ResolveAuthPath(path)
			if err != nil || pig != pigAI {
				t.Fatalf("path mismatch %q %q %v", pig, pigAI, err)
			}
		}
	}
	for _, args := range [][]string{{"list"}, {"login", "anthropic"}, {"list", "--auth-path", "~/explicit.json"}} {
		var out, diagnostics bytes.Buffer
		if err := pigaicli.Run(args, &out, &diagnostics); !errors.Is(err, ai.ErrNotImplemented) {
			t.Fatalf("M11 operation: %v", err)
		}
		if out.Len() != 0 || diagnostics.Len() != 0 {
			t.Fatal("stub emitted success or credentials")
		}
	}
}

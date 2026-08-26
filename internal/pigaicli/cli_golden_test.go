package pigaicli

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var updateIssue33CLIGolden = flag.Bool("update-issue33-cli-golden", false, "regenerate the Issue #33 pig-ai CLI snapshot")

func TestIssue33StaticCLIHelpSnapshot(t *testing.T) {
	path := filepath.Join("testdata", "pig_ai_help.golden.txt")
	want, err := os.ReadFile(path)
	if err != nil && !*updateIssue33CLIGolden {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name string
		args []string
	}{
		{name: "no arguments"},
		{name: "help command", args: []string{"help"}},
		{name: "help long", args: []string{"--help"}},
		{name: "help short", args: []string{"-h"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if err := Run(test.args, &stdout, &stderr); err != nil {
				t.Fatal(err)
			}
			if stderr.Len() != 0 {
				t.Fatalf("stderr = %q, want empty", stderr.String())
			}
			if *updateIssue33CLIGolden {
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, stdout.Bytes(), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			if !bytes.Equal(stdout.Bytes(), want) {
				t.Fatalf("pig-ai CLI snapshot drifted; regenerate with -update-issue33-cli-golden\n--- got ---\n%s\n--- want ---\n%s", stdout.Bytes(), want)
			}
		})
	}
}

func TestIssue33CLIRoutingSnapshot(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "list", args: []string{"list"}},
		{name: "list trailing argument", args: []string{"list", "ignored"}},
		{name: "list trailing option", args: []string{"list", "--unknown"}},
		{name: "login chooser", args: []string{"login"}},
		{name: "login anthropic", args: []string{"login", "anthropic"}},
		{name: "login github copilot", args: []string{"login", "github-copilot"}},
		{name: "login kimi coding", args: []string{"login", "kimi-coding"}},
		{name: "login openai codex", args: []string{"login", "openai-codex"}},
		{name: "login openrouter", args: []string{"login", "openrouter"}},
		{name: "login radius", args: []string{"login", "radius"}},
		{name: "login xai", args: []string{"login", "xai"}},
		{name: "login trailing argument", args: []string{"login", "anthropic", "ignored"}},
		{name: "login trailing option", args: []string{"login", "anthropic", "--unknown"}},
		{name: "auth path before list", args: []string{"--auth-path", "/tmp/auth.json", "list"}},
		{name: "auth path after list", args: []string{"list", "--auth-path", "/tmp/auth.json"}},
		{name: "auth path before login", args: []string{"--auth-path", "/tmp/auth.json", "login", "anthropic"}},
		{name: "auth path after login", args: []string{"login", "--auth-path", "/tmp/auth.json", "anthropic"}},
		{name: "auth path after provider", args: []string{"login", "anthropic", "--auth-path", "/tmp/auth.json"}},
		{name: "missing auth path only", args: []string{"--auth-path"}},
		{name: "missing auth path after list", args: []string{"list", "--auth-path"}},
		{name: "missing auth path after provider", args: []string{"login", "anthropic", "--auth-path"}},
		{name: "option cannot be auth path", args: []string{"list", "--auth-path", "--unknown"}},
		{name: "duplicate auth path", args: []string{"--auth-path", "one.json", "list", "--auth-path", "two.json"}},
		{name: "unknown provider", args: []string{"login", "unknown"}},
		{name: "version absent", args: []string{"--version"}},
		{name: "logout absent", args: []string{"logout"}},
		{name: "unknown command", args: []string{"unknown"}},
	}

	var snapshot strings.Builder
	for _, test := range tests {
		var stdout, stderr bytes.Buffer
		err := Run(test.args, &stdout, &stderr)
		exitCode := 0
		if err != nil {
			exitCode = 1
			stderr.WriteString(err.Error())
			stderr.WriteByte('\n')
		}
		fmt.Fprintf(&snapshot, "%s\n  args: %q\n  exit: %d\n  stdout: %q\n  stderr: %q\n",
			test.name, test.args, exitCode, stdout.String(), stderr.String())
	}

	path := filepath.Join("testdata", "pig_ai_routing.golden.txt")
	if *updateIssue33CLIGolden {
		if err := os.WriteFile(path, []byte(snapshot.String()), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.String() != string(want) {
		t.Fatalf("pig-ai CLI routing snapshot drifted; regenerate with -update-issue33-cli-golden\n--- got ---\n%s\n--- want ---\n%s", snapshot.String(), want)
	}
}

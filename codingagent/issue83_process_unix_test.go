//go:build darwin || linux

package codingagent_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
)

func installGrepScript(t *testing.T, script string, local bool) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("PIG_CODING_AGENT_DIR", dir)
	bin := filepath.Join(dir, "bin")
	if err := os.Mkdir(bin, 0700); err != nil {
		t.Fatal(err)
	}
	if !local {
		bin = t.TempDir()
		t.Setenv("PATH", bin)
	}
	path := filepath.Join(bin, "rg")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script), 0700); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestGrepToolExternalProcessContract(t *testing.T) {
	cwd := t.TempDir()
	argsFile := filepath.Join(cwd, "args")
	t.Setenv("GREP_ARGS", argsFile)
	t.Setenv("GREP_STATUS", "1")
	t.Setenv("GREP_STDERR", "")
	installGrepScript(t, `if [ "$1" = --version ]; then printf probe >> "$GREP_ARGS"; exit 9; fi
printf '%s\n' "$@" >> "$GREP_ARGS"
printf '%s' "$GREP_STDERR" >&2
exit "$GREP_STATUS"
`, false)
	tool, err := codingagent.CreateGrepTool(cwd)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(argsFile); !os.IsNotExist(err) {
		t.Fatal("constructor spawned rg")
	}
	for _, tc := range []struct {
		code, stderr, want string
		failure            bool
	}{{"0", "", "No matches found", false}, {"1", "", "No matches found", false}, {"2", "  fixture error\n", "fixture error", true}, {"7", "", "ripgrep exited with code 7", true}} {
		t.Setenv("GREP_STATUS", tc.code)
		t.Setenv("GREP_STDERR", tc.stderr)
		results := runFileToolSession(t, context.Background(), cwd, []agent.ErasedAgentTool{tool}, []ai.ToolCall{{Type: "toolCall", ID: "grep", Name: "grep", Arguments: map[string]any{"pattern": "--pre=bad; echo bad", "glob": "**/*.go", "ignoreCase": true, "literal": true}}})
		if results[0].IsError != tc.failure || readToolResultText(t, results[0]) != tc.want {
			t.Fatalf("exit %s: %+v", tc.code, results)
		}
	}
	data, err := os.ReadFile(argsFile)
	want := "probe--json\n--line-number\n--color=never\n--hidden\n--ignore-case\n--fixed-strings\n--glob\n**/*.go\n--\n--pre=bad; echo bad\n" + cwd + "\n"
	if err != nil || string(data) != strings.Repeat(want, 4) {
		t.Fatalf("process arguments: %q %v", data, err)
	}
	local := installGrepScript(t, "exit 1\n", true)
	t.Setenv("PATH", t.TempDir())
	result := runFileToolSession(t, context.Background(), cwd, []agent.ErasedAgentTool{tool}, []ai.ToolCall{{Type: "toolCall", ID: "local", Name: "grep", Arguments: map[string]any{"pattern": "x"}}})
	if result[0].IsError {
		t.Fatalf("local binary not preferred: %+v", result)
	}
	if err := os.Chmod(local, 0600); err != nil {
		t.Fatal(err)
	}
	result = runFileToolSession(t, context.Background(), cwd, []agent.ErasedAgentTool{tool}, []ai.ToolCall{{Type: "toolCall", ID: "spawn", Name: "grep", Arguments: map[string]any{"pattern": "x"}}})
	if !result[0].IsError || !strings.HasPrefix(readToolResultText(t, result[0]), "Failed to run ripgrep:") {
		t.Fatalf("spawn error: %+v", result)
	}
	if err := os.Remove(local); err != nil {
		t.Fatal(err)
	}
	for _, offline := range []string{"1", "true", "yes", "0"} {
		t.Setenv("PIG_OFFLINE", offline)
		result = runFileToolSession(t, context.Background(), cwd, []agent.ErasedAgentTool{tool}, []ai.ToolCall{{Type: "toolCall", ID: "missing", Name: "grep", Arguments: map[string]any{"pattern": "x"}}})
		text := readToolResultText(t, result[0])
		if !result[0].IsError || !strings.Contains(text, "Install ripgrep") || strings.Contains(text, "Offline mode") != (offline != "0") {
			t.Fatalf("missing binary: %s", text)
		}
		if _, err := os.Stat(local); !os.IsNotExist(err) {
			t.Fatal("missing tool downloaded")
		}
	}
}

func TestGrepToolSessionProcessCleanup(t *testing.T) {
	for _, phase := range []string{"search", "probe", "limit", "orphan"} {
		t.Run(phase, func(t *testing.T) {
			cwd := t.TempDir()
			pids := filepath.Join(cwd, "pids")
			t.Setenv("GREP_PIDS", pids)
			t.Setenv("GREP_PHASE", phase)
			installGrepScript(t, `if [ "$1" = --version ] && [ "$GREP_PHASE" != probe ]; then exit 0; fi
/bin/sleep 30 & child=$!
printf '%s %s\n' $$ "$child" > "$GREP_PIDS"
if [ "$GREP_PHASE" = limit ]; then printf '%s\n' '{"type":"match","data":{"path":{"text":"/fixture.txt"},"line_number":1,"lines":{"text":"match\n"}}}'; fi
if [ "$GREP_PHASE" = orphan ]; then exit 0; fi
wait
`, phase != "probe")
			core, _ := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
			response, _ := ai.FauxAssistantMessage(ai.FauxAssistantBlocks(ai.ToolCall{Type: "toolCall", ID: "grep", Name: "grep", Arguments: map[string]any{"pattern": "match", "limit": 1}}), ai.FauxAssistantMessageOptions{StopReason: ai.Some(ai.StopReasonToolUse)})
			final, _ := ai.FauxAssistantMessage(ai.FauxAssistantText("done"))
			core.SetResponses([]ai.FauxResponseStep{response, final})
			model, _ := core.GetModel()
			created, err := codingagent.CreateAgentSession(context.Background(), codingagent.CreateAgentSessionOptions{CWD: cwd, Model: &model, Tools: []string{"grep"}, StreamFunction: agent.StreamFunction(core.StreamSimple)})
			if err != nil {
				t.Fatal(err)
			}
			defer created.Session.Dispose()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- created.Session.Prompt(ctx, "search") }()
			var parent, child int
			deadline := time.Now().Add(3 * time.Second)
			for time.Now().Before(deadline) {
				data, _ := os.ReadFile(pids)
				if n, _ := fmt.Sscanf(string(data), "%d %d", &parent, &child); n == 2 {
					break
				}
				time.Sleep(5 * time.Millisecond)
			}
			if parent <= 0 || child <= 0 {
				t.Fatal("search process did not start")
			}
			t.Cleanup(func() { syscall.Kill(parent, syscall.SIGKILL); syscall.Kill(child, syscall.SIGKILL) })
			if phase == "orphan" {
				deadline := time.Now().Add(2 * time.Second)
				for syscall.Kill(parent, 0) == nil && time.Now().Before(deadline) {
					time.Sleep(5 * time.Millisecond)
				}
				if err := syscall.Kill(parent, 0); err != syscall.ESRCH {
					t.Fatalf("parent did not exit before cancellation: %v", err)
				}
				if err := syscall.Kill(child, 0); err != nil {
					t.Fatalf("child did not retain output pipe: %v", err)
				}
			}
			if phase != "limit" {
				if err := created.Session.Abort(); err != nil {
					t.Fatal(err)
				}
			}
			select {
			case err := <-done:
				if err != nil && !errors.Is(err, context.Canceled) {
					t.Fatal(err)
				}
			case <-ctx.Done():
				t.Fatal("grep did not settle")
			}
			for _, pid := range []int{parent, child} {
				deadline := time.Now().Add(2 * time.Second)
				for syscall.Kill(pid, 0) == nil && time.Now().Before(deadline) {
					time.Sleep(5 * time.Millisecond)
				}
				if err := syscall.Kill(pid, 0); err != syscall.ESRCH {
					t.Fatalf("process %d survived: %v", pid, err)
				}
			}
			found := false
			for _, m := range created.Session.Messages() {
				if result, ok := m.(ai.ToolResultMessage); ok {
					found = true
					if result.IsError != (phase != "limit") {
						t.Fatalf("wrong outcome: %+v", result)
					}
					if phase != "limit" && readToolResultText(t, result) != "Operation aborted" {
						t.Fatal(result)
					}
				}
			}
			if !found {
				t.Fatal("missing settled ToolResult")
			}
		})
	}
}

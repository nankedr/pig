//go:build !windows

package codingagent_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
)

func fd84Script(t *testing.T, dir, name, body string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0700); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestFindLsFDPlatformContract(t *testing.T) {
	cwd, bin, state := t.TempDir(), t.TempDir(), t.TempDir()
	t.Setenv("PATH", bin)
	t.Setenv("PIG_CODING_AGENT_DIR", state)
	def, err := codingagent.CreateFindToolDefinition(cwd)
	if err != nil {
		t.Fatal(err)
	}
	run := func(pattern string) (string, error) {
		got, err := def.Execute(context.Background(), "find", map[string]any{"pattern": pattern}, nil)
		if err != nil {
			return "", err
		}
		return got.Content[0].(ai.TextContent).Text, nil
	}
	if _, err := run("*"); err == nil || !strings.Contains(err.Error(), "automatic downloads are disabled") {
		t.Fatalf("missing fd: %v", err)
	}
	entries, err := os.ReadDir(state)
	if err != nil || len(entries) != 0 {
		t.Fatalf("missing fd wrote state: %v %v", entries, err)
	}
	fd84Script(t, bin, "fdfind", "printf 'fallback.txt\\n'\n")
	if got, err := run("*"); err != nil || got != "fallback.txt" {
		t.Fatalf("fallback: %q %v", got, err)
	}
	fd := fd84Script(t, bin, "fd", "printf '%s\\n' \"$@\"\n")
	for _, test := range []struct{ pattern, want string }{{"--help", "--glob\n--color=never\n--hidden\n--no-require-git\n--max-results\n1000\n--\n--help\n."}, {"src/**/*.spec.ts", "--glob\n--color=never\n--hidden\n--no-require-git\n--max-results\n1000\n--full-path\n--\n**/src/**/*.spec.ts\n."}} {
		got, err := run(test.pattern)
		if err != nil || got != test.want {
			t.Fatalf("arguments %q: %q %v", test.pattern, got, err)
		}
	}
	if err := os.Mkdir(filepath.Join(cwd, ".git"), 0700); err != nil {
		t.Fatal(err)
	}
	if got, err := run("*"); err != nil || strings.Contains(got, "--no-require-git") {
		t.Fatalf("git boundary: %q %v", got, err)
	}
	local := fd84Script(t, filepath.Join(state, "bin"), "fd", "printf 'cached.txt\\n'\n")
	if got, err := run("*"); err != nil || got != "cached.txt" {
		t.Fatalf("local priority: %q %v", got, err)
	}
	if err := os.Remove(local); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		body, want string
		failure    bool
	}{{"echo 'bad glob' >&2; exit 2", "bad glob", true}, {"exit 7", "fd exited with code 7", true}, {"printf 'file.txt\\n'; echo ignored >&2; exit 2", "file.txt", false}, {"exit 0", "No files found matching pattern", false}, {"printf '  space name.txt  \\r\\nfile\\\\\\n'", "space name.txt\nfile\\", false}} {
		fd84Script(t, bin, "fd", test.body)
		got, err := run("*")
		if test.failure {
			if err == nil || err.Error() != test.want {
				t.Fatalf("exit: %v", err)
			}
		} else if err != nil || got != test.want {
			t.Fatalf("output: %q %v want %q", got, err, test.want)
		}
	}
	if err := os.WriteFile(fd, []byte("not executable format"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := run("*"); err == nil || !strings.HasPrefix(err.Error(), "Failed to run fd:") {
		t.Fatalf("spawn: %v", err)
	}
}

func TestFindLsFDCancellationWaitsForExit(t *testing.T) {
	cwd, bin := t.TempDir(), t.TempDir()
	t.Setenv("PATH", bin)
	t.Setenv("PIG_CODING_AGENT_DIR", t.TempDir())
	ready := filepath.Join(cwd, "ready")
	t.Setenv("FD_READY", ready)
	fd84Script(t, bin, "fd", "echo $$ > \"$FD_READY\"\nexec /bin/sleep 30\n")
	def, err := codingagent.CreateFindToolDefinition(cwd)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := def.Execute(ctx, "find", map[string]any{"pattern": "*"}, nil); done <- err }()
	deadline := time.After(5 * time.Second)
	var pid int
	for pid == 0 {
		select {
		case err := <-done:
			t.Fatalf("early exit: %v", err)
		case <-deadline:
			t.Fatal("fd did not start")
		case <-time.After(time.Millisecond):
			data, _ := os.ReadFile(ready)
			pid, _ = strconv.Atoi(strings.TrimSpace(string(data)))
		}
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancel: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("fd cancellation did not settle")
	}
	if err := syscall.Kill(pid, 0); !errors.Is(err, syscall.ESRCH) {
		t.Fatalf("fd remains after execution settled: pid=%d err=%v", pid, err)
	}
}

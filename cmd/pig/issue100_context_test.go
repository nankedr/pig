//go:build !windows

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPigContextFilesResumeAndDiagnostics(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("requires non-root permissions")
	}
	binary := buildPigBinary(t)
	root := t.TempDir()
	cwd := filepath.Join(root, "repo", "child")
	agentDir := filepath.Join(root, "agent")
	sessions := filepath.Join(root, "sessions")
	write := func(path, content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(agentDir, "AGENTS.md"), "GLOBAL_100")
	write(filepath.Join(root, "AGENTS.md"), "ANCESTOR_100")
	write(filepath.Join(root, "repo", ".git", "HEAD"), "ref: refs/heads/main")
	write(filepath.Join(cwd, "AGENTS.override.md"), "UNREADABLE_100")
	write(filepath.Join(cwd, "CLAUDE.md"), "LOCAL_100")
	write(filepath.Join(cwd, ".pig", "settings.json"), `{"defaultModel":"must-not-load","packages":["npm:must-not-install"]}`)
	if err := os.Chmod(filepath.Join(cwd, "AGENTS.override.md"), 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Join(cwd, "AGENTS.override.md"), 0600) })
	requests := make(chan string, 8)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []struct {
				Role    string
				Content json.RawMessage
			}
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		prompt := ""
		for _, m := range body.Messages {
			if m.Role == "system" {
				_ = json.Unmarshal(m.Content, &prompt)
			}
		}
		requests <- prompt
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"context reply\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	run := func(flags ...string) (string, string) {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		args := []string{"-p", "hello", "--provider", "deepseek", "--model", "deepseek-v4-flash", "--no-tools", "--no-approve", "--offline", "--session-dir", sessions}
		command := exec.CommandContext(ctx, binary, append(args, flags...)...)
		command.Dir = cwd
		command.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + root, "PIG_CODING_AGENT_DIR=" + agentDir, "DEEPSEEK_API_KEY=fixture", "PIG_DEEPSEEK_BASE_URL=" + server.URL}
		var out, stderr bytes.Buffer
		command.Stdout = &out
		command.Stderr = &stderr
		if err := command.Run(); err != nil || out.String() != "context reply\n" {
			t.Fatalf("CLI: %v %s %s", err, out.String(), stderr.String())
		}
		select {
		case prompt := <-requests:
			return prompt, stderr.String()
		default:
			t.Fatal("no provider request")
			return "", ""
		}
	}
	prompt, stderr := run()
	last := -1
	for _, content := range []string{"GLOBAL_100", "ANCESTOR_100", "LOCAL_100"} {
		index := strings.Index(prompt, content)
		if index <= last {
			t.Fatalf("context order: %s", prompt)
		}
		last = index
	}
	if strings.Contains(prompt, "UNREADABLE_100") || !strings.Contains(stderr, "Could not read") || !strings.Contains(stderr, "AGENTS.override.md") {
		t.Fatalf("read diagnosis: %s %s", prompt, stderr)
	}
	files, err := filepath.Glob(filepath.Join(sessions, "*.jsonl"))
	if err != nil || len(files) != 1 {
		t.Fatalf("session: %v %v", files, err)
	}
	write(filepath.Join(cwd, "CLAUDE.md"), "RESUMED_100")
	prompt, _ = run("--session", files[0])
	if !strings.Contains(prompt, "RESUMED_100") || strings.Contains(prompt, "LOCAL_100") {
		t.Fatalf("stale resumed context: %s", prompt)
	}
	for _, flag := range []string{"--no-context-files", "-nc"} {
		prompt, stderr = run("--session", files[0], flag)
		if strings.Contains(prompt, "<project_context>") || strings.Contains(stderr, "Could not read") {
			t.Fatalf("disabled: %s %s", prompt, stderr)
		}
	}
}

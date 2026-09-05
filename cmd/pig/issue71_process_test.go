//go:build !windows

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestPigProcessesPersistAndReopenExplicitSessionPath(t *testing.T) {
	var mu sync.Mutex
	requests := []map[string]any{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		mu.Lock()
		requests = append(requests, body)
		count := len(requests)
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprintf(w, "data: {\"id\":\"chatcmpl-session\",\"model\":\"deepseek-v4-flash\",\"choices\":[{\"delta\":{\"content\":\"reply %d\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":2,\"completion_tokens\":2}}\n\ndata: [DONE]\n\n", count)
	}))
	defer server.Close()

	binary := buildPigBinary(t)
	cwd := t.TempDir()
	sessionDir := filepath.Join(t.TempDir(), "sessions")
	run := func(arguments ...string) string {
		t.Helper()
		command := exec.Command(binary, arguments...)
		command.Dir = cwd
		command.Env = append(filteredEnvironment(os.Environ(), "DEEPSEEK_API_KEY", "PIG_DEEPSEEK_BASE_URL", "PIG_CODING_AGENT_DIR", "PIG_CODING_AGENT_SESSION_DIR"), "PIG_DEEPSEEK_BASE_URL="+server.URL)
		var stdout, stderr bytes.Buffer
		command.Stdout, command.Stderr = &stdout, &stderr
		if err := command.Run(); err != nil {
			t.Fatalf("pig %v: %v; stderr=%q", arguments, err, stderr.String())
		}
		if stderr.Len() != 0 {
			t.Fatalf("pig %v stderr=%q", arguments, stderr.String())
		}
		return stdout.String()
	}

	if got := run("--provider", "deepseek", "--model", "deepseek-v4-flash", "--api-key", "key", "--session-dir", sessionDir, "--session-id", "process-session", "-p", "first prompt"); got != "reply 1\n" {
		t.Fatalf("first stdout = %q", got)
	}
	files, err := filepath.Glob(filepath.Join(sessionDir, "*.jsonl"))
	if err != nil || len(files) != 1 {
		t.Fatalf("session files = %v, err=%v", files, err)
	}
	if got := run("--provider", "deepseek", "--model", "deepseek-v4-flash", "--api-key", "key", "--session", files[0], "-p", "second prompt"); got != "reply 2\n" {
		t.Fatalf("second stdout = %q", got)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(requests) != 2 {
		t.Fatalf("Provider requests = %d, want 2", len(requests))
	}
	messages, _ := requests[1]["messages"].([]any)
	roles := make([]string, 0, len(messages))
	for _, raw := range messages {
		message, _ := raw.(map[string]any)
		roles = append(roles, message["role"].(string))
	}
	if got, want := strings.Join(roles, ","), "system,user,assistant,user"; got != want {
		t.Fatalf("reopened Provider roles = %q, want %q", got, want)
	}
	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	if lines := strings.Count(strings.TrimSpace(string(data)), "\n") + 1; lines != 7 {
		t.Fatalf("persisted JSONL lines = %d, want 7", lines)
	}
}

func TestPigNoSessionDoesNotCreatePigState(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"id\":\"chatcmpl-memory\",\"model\":\"deepseek-v4-flash\",\"choices\":[{\"delta\":{\"content\":\"memory\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()

	stateDir := filepath.Join(t.TempDir(), "agent")
	command := exec.Command(buildPigBinary(t), "--provider", "deepseek", "--model", "deepseek-v4-flash", "--api-key", "key", "--no-session", "-p", "hello")
	command.Env = append(filteredEnvironment(os.Environ(), "DEEPSEEK_API_KEY", "PIG_DEEPSEEK_BASE_URL", "PIG_CODING_AGENT_DIR"), "PIG_DEEPSEEK_BASE_URL="+server.URL, "PIG_CODING_AGENT_DIR="+stateDir)
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	if err := command.Run(); err != nil {
		t.Fatalf("pig --no-session: %v; stderr=%q", err, stderr.String())
	}
	if _, err := os.Stat(stateDir); !os.IsNotExist(err) {
		t.Fatalf("--no-session created Pig state: %v", err)
	}
}

func TestPigExplicitPiSessionDoesNotMigrateAdjacentPiState(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"id\":\"chatcmpl-pi-path\",\"model\":\"deepseek-v4-flash\",\"choices\":[{\"delta\":{\"content\":\"continued\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()

	home := t.TempDir()
	cwd := t.TempDir()
	piDir := filepath.Join(home, ".pi", "agent")
	if err := os.MkdirAll(filepath.Join(piDir, "sessions"), 0o700); err != nil {
		t.Fatal(err)
	}
	authPath := filepath.Join(piDir, "auth.json")
	trustPath := filepath.Join(piDir, "trust.json")
	if err := os.WriteFile(authPath, []byte(`{"secret":"unchanged"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(trustPath, []byte(`{"trusted":false}`), 0o600); err != nil {
		t.Fatal(err)
	}
	header, err := json.Marshal(map[string]any{"type": "session", "version": 3, "id": "pi-session", "timestamp": "2026-01-01T00:00:00.000Z", "cwd": cwd})
	if err != nil {
		t.Fatal(err)
	}
	sessionPath := filepath.Join(piDir, "sessions", "pi-session.jsonl")
	if err := os.WriteFile(sessionPath, append(header, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}

	command := exec.Command(buildPigBinary(t), "--provider", "deepseek", "--model", "deepseek-v4-flash", "--api-key", "key", "--session", sessionPath, "-p", "continue")
	command.Dir = cwd
	command.Env = append(filteredEnvironment(os.Environ(), "HOME", "DEEPSEEK_API_KEY", "PIG_DEEPSEEK_BASE_URL", "PIG_CODING_AGENT_DIR", "PIG_CODING_AGENT_SESSION_DIR"), "HOME="+home, "PIG_DEEPSEEK_BASE_URL="+server.URL)
	if output, err := command.CombinedOutput(); err != nil || string(output) != "continued\n" {
		t.Fatalf("pig explicit Pi session = output %q, err %v", output, err)
	}
	for path, want := range map[string]string{authPath: `{"secret":"unchanged"}`, trustPath: `{"trusted":false}`} {
		got, err := os.ReadFile(path)
		if err != nil || string(got) != want {
			t.Fatalf("adjacent Pi state %s = %q, err %v", path, got, err)
		}
	}
	if _, err := os.Stat(filepath.Join(home, ".pig")); !os.IsNotExist(err) {
		t.Fatalf("explicit Pi session created Pig migration state: %v", err)
	}
}

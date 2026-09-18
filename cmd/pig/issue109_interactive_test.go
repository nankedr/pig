//go:build darwin || linux

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/nankedr/pig/codingagent"
	"github.com/nankedr/pig/internal/baseline"
	"github.com/nankedr/pig/internal/parity"
	"github.com/nankedr/pig/internal/terminaltest"
)

func TestPigInteractiveConversation(t *testing.T) {
	var mu sync.Mutex
	var requests []map[string]any
	release := make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(release) })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			return
		}
		mu.Lock()
		requests = append(requests, request)
		turn := len(requests)
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		if turn == 1 {
			fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"FIRST_STREAM_TOKEN\"},\"finish_reason\":null}]}\n\n")
			w.(http.Flusher).Flush()
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
			fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\" FIRST_DONE\"},\"finish_reason\":\"stop\"}]}\n\n")
		} else {
			fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"SECOND_DONE\"},\"finish_reason\":\"stop\"}]}\n\n")
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()
	tty := terminaltest.Open(t)
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sessions := filepath.Join(root, "sessions")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, buildPigBinary(t), "--provider", "deepseek", "--model", "deepseek-v4-flash", "--api-key", "synthetic", "--no-tools", "--no-extensions", "--no-skills", "--session-dir", sessions)
	cmd.Dir = root
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + root, "TERM=xterm-256color", "PIG_DEEPSEEK_BASE_URL=" + server.URL}
	cmd.Stdin = tty.Slave
	cmd.Stdout = tty.Slave
	cmd.Stderr = tty.Slave
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	defer func() { cancel(); <-ctx.Done() }()
	tty.Wait(t, "> ")
	tty.Send(t, "first question\r")
	tty.Wait(t, "FIRST_STREAM_TOKEN")
	streamedBeforeCompletion := !strings.Contains(tty.Output(), "FIRST_DONE")
	once.Do(func() { close(release) })
	tty.Wait(t, "FIRST_DONE")
	tty.Send(t, "second question\r")
	tty.Wait(t, "SECOND_DONE")
	tty.Send(t, "\x04")
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("exit: %v; %q", err, tty.Output())
		}
	case <-ctx.Done():
		t.Fatal("interactive exit timed out")
	}
	tty.Wait(t, "\x1b[?2004l")
	if !tty.Restored(t) {
		t.Fatal("terminal mode was not restored")
	}
	if !tty.CursorVisible() {
		t.Fatal("cursor was not restored")
	}
	entries, err := codingagent.ListSessions(context.Background(), root, codingagent.SessionListOptions{SessionDir: &sessions})
	if err != nil || len(entries) != 1 {
		t.Fatalf("sessions: %v %v", entries, err)
	}
	manager, err := codingagent.OpenSessionManager(entries[0].Path, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	var roles []string
	for _, message := range manager.BuildSessionContext().Messages {
		roles = append(roles, string(message.MessageRole()))
	}
	oracle := interactiveOracle(t)
	delete(oracle, "exits")
	delete(oracle, "sigint")
	mu.Lock()
	defer mu.Unlock()
	actual := map[string]any{"exit_code": cmd.ProcessState.ExitCode(), "stream_before_completion": streamedBeforeCompletion, "first_reply_visible": strings.Contains(tty.Output(), "FIRST_DONE"), "second_reply_visible": strings.Contains(tty.Output(), "SECOND_DONE"), "request_count": len(requests), "session_roles": roles, "terminal_restored": tty.Restored(t), "cursor_restored": tty.CursorVisible(), "paste_disabled": tty.PasteDisabled()}
	got, _ := json.Marshal(actual)
	want, _ := json.Marshal(oracle)
	if string(got) != string(want) {
		t.Fatalf("PTY parity: got %s, want %s", got, want)
	}
	if messages := requests[1]["messages"].([]any); len(messages) < 4 {
		t.Fatalf("second request lost conversation: %v", messages)
	}
}

func TestPigInteractiveExitPaths(t *testing.T) {
	binary := buildPigBinary(t)
	for _, value := range interactiveOracle(t)["exits"].([]any) {
		expected := value.(map[string]any)
		action := expected["action"].(string)
		t.Run(action, func(t *testing.T) {
			tty := terminaltest.Open(t)
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			root := t.TempDir()
			cmd := exec.CommandContext(ctx, binary, "--provider", "deepseek", "--model", "deepseek-v4-flash", "--api-key", "synthetic", "--no-session", "--no-tools")
			cmd.Dir = root
			cmd.Env = []string{"HOME=" + root, "PATH=" + os.Getenv("PATH")}
			cmd.Stdin = tty.Slave
			cmd.Stdout = tty.Slave
			cmd.Stderr = tty.Slave
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() { done <- cmd.Wait() }()
			tty.Wait(t, "> ")
			switch action {
			case "SIGTERM":
				_ = cmd.Process.Signal(syscall.SIGTERM)
			case "SIGHUP":
				_ = cmd.Process.Signal(syscall.SIGHUP)
			case "quit":
				tty.Send(t, "/quit\r")
			case "ctrl-c-twice":
				tty.Send(t, "draft\x03\x03")
			default:
				tty.Send(t, "\x04")
			}
			select {
			case err := <-done:
				if err != nil {
					t.Fatalf("exit: %v %q", err, tty.Output())
				}
			case <-ctx.Done():
				t.Fatal("exit timed out")
			}
			tty.Wait(t, "\x1b[?2004l")
			actual := map[string]any{"action": action, "exit_code": cmd.ProcessState.ExitCode(), "terminal_restored": tty.Restored(t), "cursor_restored": tty.CursorVisible(), "paste_disabled": tty.PasteDisabled()}
			got, _ := json.Marshal(actual)
			want, _ := json.Marshal(expected)
			if string(got) != string(want) {
				t.Fatalf("exit parity: got %s want %s", got, want)
			}
		})
	}
}

func TestPigInteractiveProviderErrorIsVisible(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "controlled provider failure", http.StatusUnauthorized)
	}))
	defer server.Close()
	tty := terminaltest.Open(t)
	root := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, buildPigBinary(t), "--provider", "deepseek", "--model", "deepseek-v4-flash", "--api-key", "synthetic", "--no-session", "--no-tools")
	cmd.Dir = root
	cmd.Env = []string{"HOME=" + root, "PATH=" + os.Getenv("PATH"), "PIG_DEEPSEEK_BASE_URL=" + server.URL}
	cmd.Stdin = tty.Slave
	cmd.Stdout = tty.Slave
	cmd.Stderr = tty.Slave
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	tty.Wait(t, "> ")
	tty.Send(t, "question\r")
	tty.Wait(t, "Error:")
	tty.Send(t, "\x04")
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("exit timed out")
	}
	if !tty.Restored(t) {
		t.Fatal("terminal not restored")
	}
}

func interactiveOracle(t *testing.T) map[string]any {
	t.Helper()
	lock, _, err := baseline.Load("../../parity/baseline")
	if err != nil {
		t.Fatal(err)
	}
	fixture, err := parity.LoadFixture("../../parity/oracle/fixtures/interactive.json", parity.Baseline{ID: lock.BaselineID, Commit: lock.Upstream.Commit, Repository: lock.Upstream.Repository})
	if err != nil {
		t.Fatal(err)
	}
	var outcome map[string]any
	if err := json.Unmarshal(fixture.Observation.Outcome, &outcome); err != nil {
		t.Fatal(err)
	}
	return outcome
}

func TestPigInteractiveRoutingKeepsExplicitModes(t *testing.T) {
	binary := buildPigBinary(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ROUTED_REPLY\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	for _, mode := range []string{"print", "json", "rpc"} {
		t.Run(mode, func(t *testing.T) {
			tty := terminaltest.Open(t)
			root := t.TempDir()
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			args := []string{"--provider", "deepseek", "--model", "deepseek-v4-flash", "--api-key", "synthetic", "--no-tools", "--no-session"}
			if mode == "print" {
				args = append(args, "--print", "question")
			} else {
				args = append(args, "--mode", mode)
				if mode == "json" {
					args = append(args, "question")
				}
			}
			cmd := exec.CommandContext(ctx, binary, args...)
			cmd.Dir = root
			cmd.Env = []string{"HOME=" + root, "PATH=" + os.Getenv("PATH"), "PIG_DEEPSEEK_BASE_URL=" + server.URL}
			cmd.Stdin = tty.Slave
			cmd.Stdout = tty.Slave
			cmd.Stderr = tty.Slave
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() { done <- cmd.Wait() }()
			if mode == "rpc" {
				tty.Send(t, "{\"type\":\"get_state\",\"id\":\"route\"}\n")
				tty.Wait(t, "\"success\":true")
				tty.Send(t, "\x04")
			} else {
				tty.Wait(t, "ROUTED_REPLY")
			}
			select {
			case err := <-done:
				if err != nil {
					t.Fatalf("%v %q", err, tty.Output())
				}
			case <-ctx.Done():
				t.Fatal("explicit mode exit timed out")
			}
			if strings.Contains(tty.Output(), "\x1b[?2004h") || !tty.Restored(t) {
				t.Fatal("explicit mode started Interactive terminal")
			}
			if mode == "json" && !strings.Contains(tty.Output(), "\"type\":\"session\"") {
				t.Fatalf("not JSONL: %q", tty.Output())
			}
		})
	}
}

func TestPigInteractiveSIGINTPreservesSignalAfterCleanup(t *testing.T) {
	binary := buildPigBinary(t)
	for _, active := range []bool{false, true} {
		t.Run(fmt.Sprint(active), func(t *testing.T) {
			canceled := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"PARTIAL_SIGINT\"},\"finish_reason\":null}]}\n\n")
				w.(http.Flusher).Flush()
				<-r.Context().Done()
				close(canceled)
			}))
			defer server.Close()
			tty := terminaltest.Open(t)
			root := t.TempDir()
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, binary, "--provider", "deepseek", "--model", "deepseek-v4-flash", "--api-key", "synthetic", "--no-session", "--no-tools")
			cmd.Dir = root
			cmd.Env = []string{"HOME=" + root, "PATH=" + os.Getenv("PATH"), "PIG_DEEPSEEK_BASE_URL=" + server.URL}
			cmd.Stdin = tty.Slave
			cmd.Stdout = tty.Slave
			cmd.Stderr = tty.Slave
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() { done <- cmd.Wait() }()
			tty.Wait(t, "> ")
			if active {
				tty.Send(t, "hold\r")
				tty.Wait(t, "PARTIAL_SIGINT")
			}
			if err := cmd.Process.Signal(os.Interrupt); err != nil {
				t.Fatal(err)
			}
			select {
			case <-done:
			case <-ctx.Done():
				t.Fatal("SIGINT did not terminate")
			}
			status := cmd.ProcessState.Sys().(syscall.WaitStatus)
			expected := interactiveOracle(t)["sigint"].(map[string]any)
			if !status.Signaled() || int(status.Signal()) != -int(expected["exit_code"].(float64)) {
				t.Fatalf("wait status: %v", status)
			}
			tty.Wait(t, "\x1b[?2004l")
			if !tty.Restored(t) || !tty.CursorVisible() || !tty.PasteDisabled() {
				t.Fatal("SIGINT left terminal state active")
			}
			if active {
				select {
				case <-canceled:
				case <-ctx.Done():
					t.Fatal("Provider request survived SIGINT")
				}
			}
		})
	}
}

func TestPigInteractiveFullscreen113(t *testing.T) {
	binary := buildPigBinary(t)
	for _, fromSettings := range []bool{false, true} {
		t.Run(fmt.Sprint(fromSettings), func(t *testing.T) {
			tty := terminaltest.Open(t)
			root := t.TempDir()
			args := []string{"--provider", "deepseek", "--model", "deepseek-v4-flash", "--api-key", "synthetic", "--no-session", "--no-tools"}
			if fromSettings {
				dir := filepath.Join(root, ".pig", "agent")
				if err := os.MkdirAll(dir, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{"tuiMode":"fullscreen"}`), 0600); err != nil {
					t.Fatal(err)
				}
			} else {
				args = append(args, "--tui-mode", "fullscreen")
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, binary, args...)
			cmd.Dir = root
			cmd.Env = []string{"HOME=" + root, "PATH=" + os.Getenv("PATH")}
			cmd.Stdin = tty.Slave
			cmd.Stdout = tty.Slave
			cmd.Stderr = tty.Slave
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			tty.Wait(t, "\x1b[?1049h")
			tty.Wait(t, "> ")
			tty.Send(t, "\x04")
			if err := cmd.Wait(); err != nil {
				t.Fatal(err)
			}
			tty.Wait(t, "\x1b[?1049l")
			if !tty.Restored(t) {
				t.Fatal("fullscreen did not restore terminal")
			}

		})
	}
}

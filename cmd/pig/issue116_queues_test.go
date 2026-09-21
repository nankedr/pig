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
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
	"github.com/nankedr/pig/internal/baseline"
	"github.com/nankedr/pig/internal/parity"
	"github.com/nankedr/pig/internal/terminaltest"
)

func startQueues116(t *testing.T, url, settings string, tool ...string) (*terminaltest.Terminal, string, <-chan error) {
	t.Helper()
	tty := terminaltest.Open(t)
	root := t.TempDir()
	dir := filepath.Join(root, ".pig", "agent")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(settings), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	binary := buildPigBinary(t)
	if os.Getenv("PIG_TEST_RACE") == "1" {
		build := exec.Command("go", "build", "-race", "-o", binary, ".")
		if output, err := build.CombinedOutput(); err != nil {
			t.Fatalf("race build: %v %s", err, output)
		}
	}
	cmd := exec.CommandContext(ctx, binary, "--provider", "deepseek", "--model", "deepseek-v4-flash", "--api-key", "synthetic", "--no-tools", "--no-extensions", "--no-skills", "--session-dir", filepath.Join(root, "sessions"))
	if len(tool) > 0 {
		args := []string{}
		for _, arg := range cmd.Args {
			if arg != "--no-tools" {
				args = append(args, arg)
			}
		}
		cmd.Args = append(args, "--tools", strings.Join(tool, ","))
	}
	cmd.Dir = root
	cmd.Env = []string{"HOME=" + root, "PATH=" + os.Getenv("PATH"), "TERM=xterm-256color", "PIG_DEEPSEEK_BASE_URL=" + url}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = tty.Slave, tty.Slave, tty.Slave
	if err := cmd.Start(); err != nil {
		cancel()
		t.Fatal(err)
	}
	done := make(chan error, 1)
	reaped := make(chan struct{})
	go func() { done <- cmd.Wait(); close(reaped) }()
	t.Cleanup(func() { cancel(); <-reaped })
	tty.Wait(t, "> ")
	return tty, root, done
}
func exitQueues116(t *testing.T, tty *terminaltest.Terminal, done <-chan error) {
	t.Helper()
	tty.Send(t, "\x04")
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("exit: %v: %s", err, tty.Output())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("exit timeout")
	}
	if !tty.Restored(t) {
		t.Fatal("terminal not restored")
	}
}
func requestUsers116(t *testing.T, r *http.Request) []string {
	t.Helper()
	var req struct {
		Messages []struct {
			Role    string
			Content json.RawMessage
		}
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		t.Error(err)
		return nil
	}
	var users []string
	for _, m := range req.Messages {
		if m.Role != "user" {
			continue
		}
		var text string
		if json.Unmarshal(m.Content, &text) != nil {
			var blocks []struct{ Text string }
			if err := json.Unmarshal(m.Content, &blocks); err != nil {
				t.Error(err)
			}
			for _, b := range blocks {
				text += b.Text
			}
		}
		users = append(users, text)
	}
	return users
}
func TestPigInteractiveQueues116(t *testing.T) {
	var mu sync.Mutex
	var requests [][]string
	release := make(chan struct{})
	var once sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		users := requestUsers116(t, r)
		mu.Lock()
		requests = append(requests, users)
		turn := len(requests)
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"PARTIAL_%d\"},\"finish_reason\":null}]}\n\n", turn)
		w.(http.Flusher).Flush()
		if turn == 1 {
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
		}
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\" DONE_%d\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n", turn)
	}))
	defer server.Close()
	defer once.Do(func() { close(release) })
	tty, root, done := startQueues116(t, server.URL, `{"compaction":{"enabled":false},"retry":{"enabled":false}}`)
	tty.Send(t, "first question\r")
	tty.Wait(t, "PARTIAL_1")
	tty.Send(t, "follow question\x1b\r")
	tty.Wait(t, "Follow-up: follow question")
	tty.Send(t, "steer question\r")
	tty.Wait(t, "Steering: steer question")
	once.Do(func() { close(release) })
	tty.Wait(t, "DONE_3")
	tty.Send(t, "after question\r")
	tty.Wait(t, "DONE_4")
	exitQueues116(t, tty, done)
	files, err := filepath.Glob(filepath.Join(root, "sessions", "*.jsonl"))
	if err != nil || len(files) != 1 {
		t.Fatalf("sessions %v %v", files, err)
	}
	manager, err := codingagent.OpenSessionManager(files[0], nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	var roles []string
	for _, m := range manager.BuildSessionContext().Messages {
		roles = append(roles, string(m.MessageRole()))
	}
	lock, _, err := baseline.Load("../../parity/baseline")
	if err != nil {
		t.Fatal(err)
	}
	fixture, err := parity.LoadFixture("../../parity/oracle/fixtures/queues.json", parity.Baseline{ID: lock.BaselineID, Commit: lock.Upstream.Commit, Repository: lock.Upstream.Repository})
	if err != nil {
		t.Fatal(err)
	}
	var want struct {
		Prompts          [][]string
		Roles            []string
		SteeringVisible  bool `json:"steering_visible"`
		FollowUpVisible  bool `json:"follow_up_visible"`
		ExitCode         int  `json:"exit_code"`
		TerminalRestored bool `json:"terminal_restored"`
	}
	if err := json.Unmarshal(fixture.Observation.Outcome, &want); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if !reflect.DeepEqual(requests, want.Prompts) || !reflect.DeepEqual(roles, want.Roles) || !want.SteeringVisible || !want.FollowUpVisible || want.ExitCode != 0 || !want.TerminalRestored {
		t.Fatalf("got requests=%v roles=%v; want %+v", requests, roles, want)
	}
}

func TestPigInteractiveRestoreAbortRetry116(t *testing.T) {
	for _, scenario := range []string{"restore", "abort", "retry", "retry-success"} {
		t.Run(scenario, func(t *testing.T) {
			var mu sync.Mutex
			var requests [][]string
			release := make(chan struct{})
			var once sync.Once
			cancelled := make(chan struct{}, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				users := requestUsers116(t, r)
				mu.Lock()
				requests = append(requests, users)
				turn := len(requests)
				mu.Unlock()
				if strings.HasPrefix(scenario, "retry") && turn == 1 {
					http.Error(w, "controlled overloaded", 503)
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"PARTIAL_%d\"},\"finish_reason\":null}]}\n\n", turn)
				w.(http.Flusher).Flush()
				if turn == 1 {
					select {
					case <-release:
					case <-r.Context().Done():
						cancelled <- struct{}{}
						return
					}
				}
				fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\" DONE_%d\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n", turn)
			}))
			defer server.Close()
			defer once.Do(func() { close(release) })
			delay := 30000
			if scenario == "retry-success" {
				delay = 1000
			}
			settings := fmt.Sprintf(`{"compaction":{"enabled":false},"retry":{"enabled":%t,"maxRetries":2,"baseDelayMs":%d,"provider":{"maxRetries":0}}}`, strings.HasPrefix(scenario, "retry"), delay)
			tty, root, done := startQueues116(t, server.URL, settings)
			tty.Send(t, "first question\r")
			if strings.HasPrefix(scenario, "retry") {
				tty.Wait(t, "Retrying (1/2)")
				if scenario == "retry" {
					tty.Send(t, "\x1b")
					tty.Wait(t, "Retry cancelled")
				} else {
					tty.Wait(t, "DONE_2")
				}
				time.Sleep(30 * time.Millisecond)
				tty.Send(t, "after question\r")
				if scenario == "retry" {
					tty.Wait(t, "DONE_2")
				} else {
					tty.Wait(t, "DONE_3")
				}
			} else {
				tty.Wait(t, "PARTIAL_1")
				tty.Send(t, "follow question\x1b\r")
				tty.Wait(t, "Follow-up: follow question")
				tty.Send(t, "steer question\r")
				tty.Wait(t, "Steering: steer question")
				tty.Send(t, "draft")
				if scenario == "restore" {
					tty.Send(t, "\x1b[1;3A")
					waitScreen116(t, tty, func(s string) bool {
						return strings.Contains(s, "> steer question") && strings.Contains(s, "follow question") && !strings.Contains(s, "Steering:") && !strings.Contains(s, "Follow-up:")
					})
					tty.Send(t, "\x03")
					once.Do(func() { close(release) })
					tty.Wait(t, "DONE_1")
					time.Sleep(30 * time.Millisecond)
					tty.Send(t, "after question\r")
				} else {
					tty.Send(t, "\x1b")
					select {
					case <-cancelled:
					case <-time.After(5 * time.Second):
						t.Fatal("request survived abort")
					}
					waitScreen116(t, tty, func(s string) bool {
						return strings.Contains(s, "> steer question") && strings.Contains(s, "draft") && strings.Contains(s, "canceled")
					})
					time.Sleep(30 * time.Millisecond)
					tty.Send(t, " edited\r")
				}
				tty.Wait(t, "DONE_2")
			}
			exitQueues116(t, tty, done)
			lock, _, err := baseline.Load("../../parity/baseline")
			if err != nil {
				t.Fatal(err)
			}
			fixture, err := parity.LoadFixture("../../parity/oracle/fixtures/queues-"+scenario+".json", parity.Baseline{ID: lock.BaselineID, Commit: lock.Upstream.Commit, Repository: lock.Upstream.Repository})
			if err != nil {
				t.Fatal(err)
			}
			var want struct {
				Prompts       [][]string
				Roles, Stops  []string
				AssistantText []string `json:"assistant_text"`
			}
			if err := json.Unmarshal(fixture.Observation.Outcome, &want); err != nil {
				t.Fatal(err)
			}
			files, _ := filepath.Glob(filepath.Join(root, "sessions", "*.jsonl"))
			if len(files) != 1 {
				t.Fatal(files)
			}
			manager, err := codingagent.OpenSessionManager(files[0], nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			var roles, stops, texts []string
			for _, m := range manager.BuildSessionContext().Messages {
				roles = append(roles, string(m.MessageRole()))
				if a, ok := m.(ai.AssistantMessage); ok {
					stops = append(stops, string(a.StopReason))
					text := ""
					for _, block := range a.Content {
						if b, ok := block.(ai.TextContent); ok {
							text += b.Text
						}
					}
					texts = append(texts, text)
				}
			}
			mu.Lock()
			defer mu.Unlock()
			if !reflect.DeepEqual(requests, want.Prompts) || !reflect.DeepEqual(roles, want.Roles) || !reflect.DeepEqual(stops, want.Stops) || !reflect.DeepEqual(texts, want.AssistantText) {
				t.Fatalf("requests=%v roles=%v stops=%v texts=%v; want %+v", requests, roles, stops, texts, want)
			}
		})
	}
}
func waitScreen116(t *testing.T, tty *terminaltest.Terminal, ready func(string) bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if ready(tty.ScreenText()) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("unexpected screen: %s", tty.ScreenText())
}
func TestPigInteractiveExitActive116(t *testing.T) {
	for _, exit := range []string{"/quit\r", "\x04", "\x03\x03"} {
		t.Run(fmt.Sprintf("%q", exit), func(t *testing.T) {
			var calls atomic.Int32
			cancelled := make(chan struct{}, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"HOLD_116\"},\"finish_reason\":null}]}\n\n")
				w.(http.Flusher).Flush()
				<-r.Context().Done()
				cancelled <- struct{}{}
			}))
			defer server.Close()
			tty, _, done := startQueues116(t, server.URL, `{"retry":{"enabled":false}}`)
			tty.Send(t, "hold\r")
			tty.Wait(t, "HOLD_116")
			tty.Send(t, "queued\x1b\r")
			tty.Wait(t, "Follow-up: queued")
			tty.Send(t, exit)
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("exit blocked")
			}
			select {
			case <-cancelled:
			case <-time.After(5 * time.Second):
				t.Fatal("provider survived exit")
			}
			if calls.Load() != 1 || !tty.Restored(t) {
				t.Fatalf("calls=%d restored=%t", calls.Load(), tty.Restored(t))
			}
		})
	}
}

func TestPigInteractiveToolAbort116(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		if calls.Add(1) == 1 {
			payload := map[string]any{"choices": []any{map[string]any{"delta": map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": "hold116", "type": "function", "function": map[string]any{"name": "bash", "arguments": `{"command":"printf TOOL_; printf PARTIAL_116; sleep 30"}`}}}}, "finish_reason": "tool_calls"}}}
			data, _ := json.Marshal(payload)
			fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", data)
		} else {
			fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"AFTER_TOOL_ABORT\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
		}
	}))
	defer server.Close()
	tty, root, done := startQueues116(t, server.URL, `{"retry":{"enabled":false}}`, "bash")
	tty.Send(t, "tool question\r")
	tty.Wait(t, "TOOL_PARTIAL_116")
	tty.Send(t, "\x1b")
	tty.Wait(t, "Command aborted")
	time.Sleep(30 * time.Millisecond)
	tty.Send(t, "after question\r")
	tty.Wait(t, "AFTER_TOOL_ABORT")
	exitQueues116(t, tty, done)
	files, _ := filepath.Glob(filepath.Join(root, "sessions", "*.jsonl"))
	if len(files) != 1 {
		t.Fatal(files)
	}
	manager, err := codingagent.OpenSessionManager(files[0], nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	var output string
	for _, message := range manager.BuildSessionContext().Messages {
		if m, ok := message.(ai.ToolResultMessage); ok {
			for _, c := range m.Content {
				if text, ok := c.(ai.TextContent); ok {
					output += text.Text
				}
			}
			if !m.IsError {
				t.Fatal("cancelled tool marked successful")
			}
		}
	}
	if !strings.Contains(output, "TOOL_PARTIAL_116") || !strings.Contains(output, "Command aborted") || calls.Load() != 2 {
		t.Fatalf("tool result=%q requests=%d", output, calls.Load())
	}
}

func TestPigInteractiveFollowUpCommands116(t *testing.T) {
	var mu sync.Mutex
	var requests [][]string
	release := make(chan struct{})
	var once sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		users := requestUsers116(t, r)
		mu.Lock()
		requests = append(requests, users)
		turn := len(requests)
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"PARTIAL_%d\"},\"finish_reason\":null}]}\n\n", turn)
		w.(http.Flusher).Flush()
		if turn == 1 {
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
		}
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\" DONE_%d\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n", turn)
	}))
	defer server.Close()
	defer once.Do(func() { close(release) })
	tty, _, done := startQueues116(t, server.URL, `{"compaction":{"enabled":false},"retry":{"enabled":false}}`)
	tty.Send(t, "first question\r")
	tty.Wait(t, "PARTIAL_1")
	tty.Send(t, "/quit\x1b\r")
	tty.Wait(t, "Follow-up: /quit")
	tty.Send(t, "/new\x1b\r")
	tty.Wait(t, "Follow-up: /new")
	once.Do(func() { close(release) })
	tty.Wait(t, "DONE_3")
	tty.Send(t, "after question\r")
	tty.Wait(t, "DONE_4")
	exitQueues116(t, tty, done)
	lock, _, err := baseline.Load("../../parity/baseline")
	if err != nil {
		t.Fatal(err)
	}
	fixture, err := parity.LoadFixture("../../parity/oracle/fixtures/queues-commands.json", parity.Baseline{ID: lock.BaselineID, Commit: lock.Upstream.Commit, Repository: lock.Upstream.Repository})
	if err != nil {
		t.Fatal(err)
	}
	var want struct{ Prompts [][]string }
	if err := json.Unmarshal(fixture.Observation.Outcome, &want); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if !reflect.DeepEqual(requests, want.Prompts) {
		t.Fatalf("requests=%v want=%v", requests, want.Prompts)
	}
}

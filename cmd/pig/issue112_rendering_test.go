//go:build darwin || linux

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/creack/pty"
	"github.com/nankedr/pig/codingagent"
	"github.com/nankedr/pig/internal/terminaltest"
	"github.com/nankedr/pig/tui"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestPigTextRendering112(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sessions := filepath.Join(root, "sessions")
	os.WriteFile(filepath.Join(root, "progress.txt"), []byte("PROGRESS_VISIBLE\n"), 0600)
	os.WriteFile(filepath.Join(root, "final.txt"), []byte(strings.Repeat("output line\n", 15)+"FINAL_TOOL_OUTPUT\n"), 0600)
	thinking, args := make(chan struct{}), make(chan struct{})
	var thinkOnce, argsOnce sync.Once
	defer thinkOnce.Do(func() { close(thinking) })
	defer argsOnce.Do(func() { close(args) })
	var mu sync.Mutex
	var requests []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)
		mu.Lock()
		requests = append(requests, req)
		turn := len(requests)
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		chunk := func(delta any, finish any) {
			b, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": finish}}})
			fmt.Fprintf(w, "data: %s\n\n", b)
			w.(http.Flusher).Flush()
		}
		if turn == 1 {
			chunk(map[string]any{"reasoning_content": "THINKING_VISIBLE"}, nil)
			select {
			case <-thinking:
			case <-r.Context().Done():
				return
			}
			chunk(map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": "tool-112", "type": "function", "function": map[string]any{"name": "bash", "arguments": "{\"command\":\"cat progress.txt"}}}}, nil)
			select {
			case <-args:
			case <-r.Context().Done():
				return
			}
			chunk(map[string]any{"tool_calls": []any{map[string]any{"index": 0, "function": map[string]any{"arguments": "; while [ ! -f release ]; do sleep 0.05; done; cat final.txt\"}"}}}}, "tool_calls")
		} else {
			chunk(map[string]any{"content": "**ANSWER_DONE**\n\n```go\nprintln(\"ok\")\n```"}, "stop")
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	binary := buildPigBinary(t)
	launch := func(extra ...string) (*terminaltest.Terminal, <-chan error) {
		tty := terminaltest.Open(t)
		if err := pty.Setsize(tty.Slave, &pty.Winsize{Rows: 40, Cols: 44}); err != nil {
			t.Fatal(err)
		}
		argv := []string{"--provider", "deepseek", "--model", "deepseek-v4-flash", "--api-key", "synthetic", "--no-extensions", "--no-skills", "--session-dir", sessions}
		argv = append(argv, extra...)
		cmd := exec.CommandContext(ctx, binary, argv...)
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
		return tty, done
	}
	tty, done := launch()
	tty.Wait(t, "> ")
	tty.Send(t, "run the tool\r")
	tty.Wait(t, "THINKING_VISIBLE")
	thinkOnce.Do(func() { close(thinking) })
	tty.Wait(t, "cat progress.txt")
	argsOnce.Do(func() { close(args) })
	tty.Wait(t, "PROGRESS_VISIBLE")
	os.WriteFile(filepath.Join(root, "release"), nil, 0600)
	tty.Wait(t, "ANSWER_DONE")
	tty.Send(t, "\x0f")
	tty.Wait(t, "FINAL_TOOL_OUTPUT")
	waitFrame := func(tty *terminaltest.Terminal, contains string) string {
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			s := latestFrame112(tty.Output())
			if strings.Contains(s, contains) {
				return s
			}
			time.Sleep(5 * time.Millisecond)
		}
		t.Fatalf("missing final frame %q: %s", contains, tty.Output())
		return ""
	}
	live := waitFrame(tty, "FINAL_TOOL_OUTPUT")
	tty.Send(t, "\x14")
	waitFrame(tty, "Thinking...")
	tty.Send(t, "\x14")
	waitFrame(tty, "THINKING_VISIBLE")
	tty.Send(t, "\x04")
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	wire, _ := json.Marshal(requests[len(requests)-1]["messages"])
	mu.Unlock()
	data, err := os.ReadFile("../../parity/oracle/fixtures/text-rendering-cli.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Observation struct{ Outcome map[string]any }
	}
	json.Unmarshal(data, &fixture)
	actual := map[string]any{"thinking_before_tool": true, "arguments_before_execution": true, "progress_before_final": true, "expanded_output": true, "answer_visible": strings.Contains(live, "ANSWER_DONE"), "model_context_clean": !strings.Contains(string(wire), "[running]") && !strings.Contains(string(wire), "ctrl+o"), "exit_code": float64(0), "terminal_restored": tty.Restored(t)}
	if !reflect.DeepEqual(actual, fixture.Observation.Outcome) {
		t.Fatalf("CLI parity: %v want %v", actual, fixture.Observation.Outcome)
	}
	entries, err := codingagent.ListSessions(context.Background(), root, codingagent.SessionListOptions{SessionDir: &sessions})
	if err != nil || len(entries) != 1 {
		t.Fatalf("sessions %v %v", entries, err)
	}
	restored, finished := launch("--session", entries[0].Path)
	restored.Wait(t, "ANSWER_DONE")
	restored.Send(t, "\x0f")
	history := waitFrame(restored, "FINAL_TOOL_OUTPUT")
	if live != history {
		t.Fatalf("live/history mismatch:\n%s\n---\n%s", live, history)
	}
	restored.Send(t, "\x04")
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(history, "\n") {
		width, _ := tui.VisibleWidth(line)
		if width > 44 {
			t.Fatalf("line exceeds width: %q", line)
		}
	}
}
func latestFrame112(output string) string {
	parts := strings.Split(output, "\x1b[H\x1b[2J")
	s := parts[len(parts)-1]
	s, _ = tui.StripTerminalSequences(s)
	s = strings.ReplaceAll(s, "\r", "")
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " ")
	}
	return strings.Join(lines, "\n")
}

func TestPigPartialFailureAndCancel112(t *testing.T) {
	for _, mode := range []string{"failure", "cancel", "length"} {
		t.Run(mode, func(t *testing.T) {
			release := make(chan struct{})
			var once sync.Once
			defer once.Do(func() { close(release) })
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"PARTIAL_112\"},\"finish_reason\":null}]}\n\n")
				w.(http.Flusher).Flush()
				select {
				case <-release:
				case <-r.Context().Done():
					return
				}
				if mode == "failure" {
					fmt.Fprint(w, "data: {\"error\":{\"message\":\"REMOTE_FAILURE\"}}\n\n")
				} else {
					fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"length\"}]}\n\ndata: [DONE]\n\n")
				}
			}))
			defer server.Close()
			tty := terminaltest.Open(t)
			pty.Setsize(tty.Slave, &pty.Winsize{Rows: 14, Cols: 22})
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			root := t.TempDir()
			cmd := exec.CommandContext(ctx, buildPigBinary(t), "--provider", "deepseek", "--model", "deepseek-v4-flash", "--api-key", "synthetic", "--no-tools", "--no-session", "--no-extensions", "--no-skills")
			cmd.Dir = root
			cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + root, "PIG_DEEPSEEK_BASE_URL=" + server.URL}
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
			tty.Wait(t, "PARTIAL_112")
			expected := "Stream ended"
			if mode == "cancel" {
				tty.Send(t, "\x1b")
				expected = "canceled"
			} else {
				once.Do(func() { close(release) })
				if mode == "length" {
					expected = "Response was"
				}
			}
			deadline := time.Now().Add(8 * time.Second)
			frame := ""
			for time.Now().Before(deadline) {
				frame = latestFrame112(tty.Output())
				if strings.Contains(frame, expected) {
					break
				}
				time.Sleep(5 * time.Millisecond)
			}
			if !strings.Contains(frame, expected) || !strings.Contains(frame, "PARTIAL_112") {
				t.Fatalf("partial/failure not preserved: %s", frame)
			}
			tty.Send(t, "\x04")
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			if !tty.Restored(t) {
				t.Fatal("raw mode not restored")
			}
		})
	}
}

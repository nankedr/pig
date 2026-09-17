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
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/creack/pty"
	"github.com/nankedr/pig/internal/baseline"
	"github.com/nankedr/pig/internal/parity"
	"github.com/nankedr/pig/internal/terminaltest"
)

func TestPigMultilineEditor110(t *testing.T) {
	lock, _, err := baseline.Load("../../parity/baseline")
	if err != nil {
		t.Fatal(err)
	}
	fixture, err := parity.LoadFixture("../../parity/oracle/fixtures/editor-cli.json", parity.Baseline{ID: lock.BaselineID, Commit: lock.Upstream.Commit, Repository: lock.Upstream.Repository})
	if err != nil {
		t.Fatal(err)
	}
	var input struct {
		Turns []struct {
			Chunks  []string
			Columns uint16
		}
	}
	var want struct {
		Prompts  []string
		ExitCode int `json:"exit_code"`
	}
	if err := json.Unmarshal(fixture.Case.Input, &input); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(fixture.Observation.Outcome, &want); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var prompts []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Messages []struct {
				Role    string
				Content json.RawMessage
			}
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			return
		}
		prompt := ""
		for _, m := range request.Messages {
			if m.Role == "user" {
				if json.Unmarshal(m.Content, &prompt) != nil {
					var blocks []struct{ Text string }
					if err := json.Unmarshal(m.Content, &blocks); err != nil {
						t.Error(err)
						return
					}
					prompt = ""
					for _, b := range blocks {
						prompt += b.Text
					}
				}
			}
		}
		mu.Lock()
		prompts = append(prompts, prompt)
		turn := len(prompts)
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"EDITOR_DONE_%d\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n", turn)
	}))
	defer server.Close()
	tty := terminaltest.Open(t)
	root := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, buildPigBinary(t), "--provider", "deepseek", "--model", "deepseek-v4-flash", "--api-key", "synthetic", "--no-session", "--no-tools", "--no-extensions", "--no-skills")
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
	tty.Wait(t, "> ")
	tty.Send(t, "\r   \rdiscard\x03")
	for index, turn := range input.Turns {
		for j, chunk := range turn.Chunks {
			tty.Send(t, chunk)
			time.Sleep(10 * time.Millisecond)
			if j == 0 && turn.Columns > 0 {
				if err := pty.Setsize(tty.Slave, &pty.Winsize{Rows: 32, Cols: turn.Columns}); err != nil {
					t.Fatal(err)
				}
			}
		}
		tty.Wait(t, fmt.Sprintf("EDITOR_DONE_%d", index+1))
	}
	tty.Send(t, "\x04")
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("exit: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("exit timed out")
	}
	mu.Lock()
	defer mu.Unlock()
	if !reflect.DeepEqual(prompts, want.Prompts) {
		t.Fatalf("Provider prompts got %#v want %#v", prompts, want.Prompts)
	}
	if cmd.ProcessState.ExitCode() != want.ExitCode {
		t.Fatal("exit mismatch")
	}
	tty.Wait(t, "\x1b[?2004l")
	if !tty.Restored(t) {
		t.Fatal("terminal not restored")
	}
}

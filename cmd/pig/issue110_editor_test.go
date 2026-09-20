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
	"testing"
	"time"

	"github.com/creack/pty"
	"github.com/nankedr/pig/internal/baseline"
	"github.com/nankedr/pig/internal/parity"
	"github.com/nankedr/pig/internal/terminaltest"
)

func TestPigMultilineEditor110(t *testing.T) { runEditorCLI(t, "editor-cli.json") }
func TestPigTerminalKeys111(t *testing.T)    { runEditorCLI(t, "keys-cli.json") }
func runEditorCLI(t *testing.T, fixtureName string) {
	lock, _, err := baseline.Load("../../parity/baseline")
	if err != nil {
		t.Fatal(err)
	}
	fixture, err := parity.LoadFixture("../../parity/oracle/fixtures/"+fixtureName, parity.Baseline{ID: lock.BaselineID, Commit: lock.Upstream.Commit, Repository: lock.Upstream.Repository})
	if err != nil {
		t.Fatal(err)
	}
	var input struct {
		Files       map[string]string
		Args        []string
		Executables []string
		DelayMS     int
		Config      json.RawMessage
		Negotiation []string
		Exit        string
		Clear       string
		Turns       []struct {
			Chunks  []string
			Waits   []string
			Columns uint16
		}
	}
	var want struct {
		Prompts    []string
		UserCounts []int `json:"user_counts"`
		ExitCode   int   `json:"exit_code"`
	}
	if err := json.Unmarshal(fixture.Case.Input, &input); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(fixture.Observation.Outcome, &want); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var prompts []string
	var userCounts []int
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
		userCount := 0
		for _, m := range request.Messages {
			if m.Role == "user" {
				userCount++
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
		userCounts = append(userCounts, userCount)
		turn := len(prompts)
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"EDITOR_DONE_%d\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n", turn)
	}))
	defer server.Close()
	tty := terminaltest.Open(t)
	root := t.TempDir()
	for name, content := range input.Files {
		path := filepath.Join(root, strings.ReplaceAll(name, ".pi/", ".pig/"))
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range input.Executables {
		if err := os.Chmod(filepath.Join(root, strings.ReplaceAll(name, ".pi/", ".pig/")), 0755); err != nil {
			t.Fatal(err)
		}
	}
	if len(input.Config) > 0 {
		dir := filepath.Join(root, ".pig", "agent")
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "keybindings.json"), input.Config, 0600); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, buildPigBinary(t), "--provider", "deepseek", "--model", "deepseek-v4-flash", "--api-key", "synthetic", "--no-session", "--no-tools", "--no-extensions", "--no-skills")
	for _, arg := range input.Args {
		cmd.Args = append(cmd.Args, strings.ReplaceAll(strings.ReplaceAll(arg, "$ROOT", root), "/.pi/", "/.pig/"))
	}
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
	if fixtureName == "keys-cli.json" {
		tty.Wait(t, "Keybinding conflict ctrl+g")
	}
	if input.Clear == "" {
		tty.Send(t, "\r   \rdiscard\x03")
	} else {
		tty.Send(t, "discard"+input.Clear)
	}
	for _, chunk := range input.Negotiation {
		tty.Send(t, chunk)
		time.Sleep(30 * time.Millisecond)
	}
	for index, turn := range input.Turns {
		for j, chunk := range turn.Chunks {
			offset := len(tty.Output())
			tty.Send(t, strings.ReplaceAll(chunk, "$ROOT", root))
			if j < len(turn.Waits) && turn.Waits[j] != "" {
				deadline := time.Now().Add(10 * time.Second)
				for !strings.Contains(tty.Output()[offset:], turn.Waits[j]) {
					if time.Now().After(deadline) {
						t.Fatalf("completion never displayed %q: %q", turn.Waits[j], tty.Output()[offset:])
					}
					time.Sleep(5 * time.Millisecond)
				}
			}
			time.Sleep(time.Duration(max(10, input.DelayMS)) * time.Millisecond)
			if j == 0 && turn.Columns > 0 {
				if err := pty.Setsize(tty.Slave, &pty.Winsize{Rows: 32, Cols: turn.Columns}); err != nil {
					t.Fatal(err)
				}
			}
		}
		tty.Wait(t, fmt.Sprintf("EDITOR_DONE_%d", index+1))
	}
	if input.Exit == "" {
		input.Exit = "\x04"
	}
	tty.Send(t, input.Exit)
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
	for i := range prompts {
		prompts[i] = strings.ReplaceAll(strings.ReplaceAll(prompts[i], root, "$ROOT"), "/.pig/", "/.pi/")
	}
	if !reflect.DeepEqual(prompts, want.Prompts) {
		t.Fatalf("Provider prompts got %#v want %#v", prompts, want.Prompts)
	}
	if want.UserCounts != nil && !reflect.DeepEqual(userCounts, want.UserCounts) {
		t.Fatalf("session user counts %v want %v", userCounts, want.UserCounts)
	}
	if cmd.ProcessState.ExitCode() != want.ExitCode {
		t.Fatal("exit mismatch")
	}
	tty.Wait(t, "\x1b[?2004l")
	if !tty.Restored(t) {
		t.Fatal("terminal not restored")
	}
}

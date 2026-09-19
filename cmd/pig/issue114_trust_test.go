//go:build darwin || linux

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/nankedr/pig/codingagent"
	"github.com/nankedr/pig/internal/terminaltest"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestPigTrustDialog114(t *testing.T) {
	data, err := os.ReadFile("../../parity/oracle/fixtures/trust-dialog.json")
	if err != nil {
		t.Fatal(err)
	}
	type run struct {
		Prompt, Project, Context, Restored bool
		Exit                               int
	}
	type scenario struct {
		Name   string
		Runs   []run
		Saved  *bool
		Parent bool
	}
	var fixture struct{ Observation struct{ Outcome []scenario } }
	if err = json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	binary := buildPigBinary(t)
	for _, want := range fixture.Observation.Outcome {
		t.Run(want.Name, func(t *testing.T) {
			root, _ := filepath.EvalSymlinks(t.TempDir())
			cwd := filepath.Join(root, "project")
			dir := filepath.Join(root, "agent")
			for _, path := range []string{filepath.Join(cwd, ".pig"), dir} {
				if err := os.MkdirAll(path, 0700); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(filepath.Join(cwd, ".pig", "SYSTEM.md"), []byte("PROJECT_TRUST_SYSTEM"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(cwd, "AGENTS.md"), []byte("CONTEXT_TRUST_EXCEPTION"), 0600); err != nil {
				t.Fatal(err)
			}
			requests := make(chan string, 2)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request map[string]any
				_ = json.NewDecoder(r.Body).Decode(&request)
				encoded, _ := json.Marshal(request["messages"])
				requests <- string(encoded)
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"TRUST_REPLY\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
			}))
			defer server.Close()
			got := scenario{Name: want.Name}
			for attempt, expected := range want.Runs {
				tty := terminaltest.Open(t)
				ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer cancel()
				cmd := exec.CommandContext(ctx, binary, "--provider", "deepseek", "--model", "deepseek-v4-flash", "--api-key", "synthetic", "--no-tools", "--no-extensions", "--no-skills", "--no-session")
				cmd.Dir = cwd
				cmd.Env = []string{"HOME=" + root, "PATH=" + os.Getenv("PATH"), "PIG_CODING_AGENT_DIR=" + dir, "PIG_DEEPSEEK_BASE_URL=" + server.URL}
				cmd.Stdin = tty.Slave
				cmd.Stdout = tty.Slave
				cmd.Stderr = tty.Slave
				if err := cmd.Start(); err != nil {
					t.Fatal(err)
				}
				if expected.Prompt {
					tty.Wait(t, "Trust project folder?")
					keys := map[string]string{"trust": "\r", "parent": "j\r", "session": "jj\r", "deny": "jjj\r", "deny-session": "jjjj\r", "cancel": "\x1b"}[want.Name]
					if attempt > 0 {
						keys = "\x1b"
					}
					tty.Send(t, keys)
				}
				tty.Wait(t, "> ")
				tty.Send(t, "hello\r")
				tty.Wait(t, "TRUST_REPLY")
				message := <-requests
				tty.Send(t, "\x04")
				if err := cmd.Wait(); err != nil {
					t.Fatalf("%v %s", err, tty.Output())
				}
				got.Runs = append(got.Runs, run{Prompt: strings.Contains(tty.Output(), "Trust project folder?"), Project: strings.Contains(message, "PROJECT_TRUST_SYSTEM"), Context: strings.Contains(message, "CONTEXT_TRUST_EXCEPTION"), Exit: cmd.ProcessState.ExitCode(), Restored: tty.Restored(t)})
			}
			entry, err := codingagent.NewProjectTrustStore(dir).GetEntry(context.Background(), cwd)
			if err != nil {
				t.Fatal(err)
			}
			if entry != nil {
				got.Saved = &entry.Decision
				got.Parent = entry.Path == root
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("got %+v want %+v", got, want)
			}
		})
	}
}

func TestPigTrustFailureAndNoSensitiveReads114(t *testing.T) {
	binary := buildPigBinary(t)
	for _, name := range []string{"deny", "cancel", "save-failure", "signal"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			cwd := filepath.Join(root, "project")
			dir := filepath.Join(root, "agent")
			config := filepath.Join(cwd, ".pig")
			for _, p := range []string{config, dir} {
				if err := os.MkdirAll(p, 0700); err != nil {
					t.Fatal(err)
				}
			}
			for _, p := range []string{"settings.json", "SYSTEM.md"} {
				if err := syscall.Mkfifo(filepath.Join(config, p), 0600); err != nil {
					t.Fatal(err)
				}
			}
			tty := terminaltest.Open(t)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, binary, "--provider", "deepseek", "--model", "deepseek-v4-flash", "--api-key", "synthetic", "--no-tools", "--no-session")
			cmd.Dir = cwd
			cmd.Env = []string{"HOME=" + root, "PATH=" + os.Getenv("PATH"), "PIG_CODING_AGENT_DIR=" + dir}
			cmd.Stdin = tty.Slave
			cmd.Stdout = tty.Slave
			cmd.Stderr = tty.Slave
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			tty.Wait(t, "Trust project folder?")
			if !strings.Contains(tty.Output(), "host user permissions") || !strings.Contains(tty.Output(), "not Tool approval") {
				t.Fatal("trust scope missing")
			}
			switch name {
			case "save-failure":
				if err := os.Mkdir(filepath.Join(dir, "trust.json"), 0700); err != nil {
					t.Fatal(err)
				}
				tty.Send(t, "\r")
			case "signal":
				_ = cmd.Process.Signal(syscall.SIGTERM)
			case "deny":
				tty.Send(t, "jjj\r")
			default:
				tty.Send(t, "\x1b")
			}
			if name == "deny" || name == "cancel" {
				tty.Wait(t, "> ")
				tty.Send(t, "\x04")
			}
			err := cmd.Wait()
			if name == "save-failure" {
				if err == nil || !strings.Contains(tty.Output(), "save project trust") {
					t.Fatalf("save failure %v %s", err, tty.Output())
				}
				if strings.Contains(tty.Output(), "> ") {
					t.Fatal("session started after save failure")
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if !tty.Restored(t) {
				t.Fatal("terminal not restored")
			}
		})
	}
}

//go:build darwin || linux

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
	"github.com/nankedr/pig/internal/terminaltest"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestImageWorkflowCLI136(t *testing.T) {
	binary := binary133(t)
	for _, mode := range []string{"regular", "fullscreen"} {
		t.Run(mode, func(t *testing.T) {
			root, dir := taskRoot133(t)
			image, err := ai.LoadImageFile("../../parity/services/user-image.png")
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(root, "screenshot.png")
			data, _ := os.ReadFile("../../parity/services/user-image.png")
			os.WriteFile(path, data, 0600)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request map[string]any
				json.NewDecoder(r.Body).Decode(&request)
				raw, _ := json.Marshal(request)
				if !bytes.Contains(raw, []byte("data:image/png;base64,"+image.Data)) {
					t.Error("CLI/restored image missing")
				}
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, frame133(map[string]any{"type": "response.output_text.delta", "delta": "CLI_IMAGE_DONE"})+frame133(map[string]any{"type": "response.completed", "response": map[string]any{"status": "completed"}}))
			}))
			defer server.Close()
			run := func(extra ...string) {
				t.Helper()
				tty := terminaltest.Open(t)
				ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
				defer cancel()
				args := append([]string{"--no-tools", "--no-context-files", "--model", "deepseek-flash", "--api", "openai-responses", "--tui-mode", mode}, extra...)
				cmd := exec.CommandContext(ctx, binary, args...)
				cmd.Dir = root
				cmd.Env = append(taskEnv133(root, dir, server.URL), "TERM=xterm-256color")
				cmd.Stdin = tty.Slave
				cmd.Stdout = tty.Slave
				cmd.Stderr = tty.Slave
				if err := cmd.Start(); err != nil {
					t.Fatal(err)
				}
				done := make(chan error, 1)
				go func() { done <- cmd.Wait() }()
				tty.Wait(t, "CLI_IMAGE_DONE")
				tty.Send(t, "/image list\r")
				tty.Wait(t, "Session image 1")
				tty.Send(t, "\x04")
				select {
				case err := <-done:
					if err != nil {
						t.Fatalf("CLI exit: %v", err)
					}
				case <-ctx.Done():
					t.Fatal("CLI exit timeout")
				}
				if !tty.Restored(t) {
					t.Fatal("CLI left terminal in raw mode")
				}
			}
			run("@"+path, "inspect screenshot")
			sessionDir := filepath.Join(dir, "sessions")
			sessions, err := codingagent.ListAllSessions(t.Context(), codingagent.SessionListOptions{SessionDir: &sessionDir})
			if err != nil || len(sessions) != 1 {
				t.Fatal("session missing", err)
			}
			os.Remove(path)
			run("--session", sessions[0].Path, "continue screenshot")
			output := filepath.Join(root, "restored.html")
			cmd := exec.Command(binary, "--export", sessions[0].Path, output)
			cmd.Env = taskEnv133(root, dir, server.URL)
			cmd.Dir = root
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("restored export: %v %s", err, out)
			}
		})
	}
}

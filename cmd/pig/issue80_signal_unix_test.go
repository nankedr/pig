//go:build darwin || linux

package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestPigBashShutdownSignalsKillProcessTree(t *testing.T) {
	binary := buildPigBinary(t)
	for _, signal := range []os.Signal{os.Interrupt, syscall.SIGTERM, syscall.SIGHUP} {
		t.Run(signal.String(), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				args, _ := json.Marshal(map[string]any{"command": `sleep 30 & child=$!; printf '%s %s\n' $$ $child; wait`})
				data, _ := json.Marshal(map[string]any{"id": "tree", "choices": []any{map[string]any{"delta": map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": "bash", "type": "function", "function": map[string]any{"name": "bash", "arguments": string(args)}}}}, "finish_reason": "tool_calls"}}})
				fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", data)
			}))
			defer server.Close()
			cmd := exec.Command(binary, "--provider", "deepseek", "--model", "deepseek-v4-flash", "--no-session", "--tools", "bash", "--mode", "json", "-p", "run")
			cmd.Dir = t.TempDir()
			home := t.TempDir()
			cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + home, "PIG_CODING_AGENT_DIR=" + home, "DEEPSEEK_API_KEY=fixture", "PIG_DEEPSEEK_BASE_URL=" + server.URL}
			reader, err := cmd.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { cmd.Process.Kill() })
			ready := make(chan [2]int, 1)
			done := make(chan string, 1)
			go func() {
				var lines strings.Builder
				scanner := bufio.NewScanner(reader)
				for scanner.Scan() {
					line := scanner.Text()
					lines.WriteString(line + "\n")
					var event struct {
						Type          string
						PartialResult struct{ Content []struct{ Text string } }
					}
					if json.Unmarshal([]byte(line), &event) == nil && event.Type == "tool_execution_update" && len(event.PartialResult.Content) > 0 {
						var pids [2]int
						if n, _ := fmt.Sscanf(event.PartialResult.Content[0].Text, "%d %d", &pids[0], &pids[1]); n == 2 {
							select {
							case ready <- pids:
							default:
							}
						}
					}
				}
				done <- lines.String()
			}()
			var pids [2]int
			select {
			case pids = <-ready:
			case <-time.After(5 * time.Second):
				t.Fatal("bash did not start")
			}
			t.Cleanup(func() {
				for _, pid := range pids {
					syscall.Kill(pid, syscall.SIGKILL)
				}
			})
			if err := cmd.Process.Signal(signal); err != nil {
				t.Fatal(err)
			}
			var output string
			select {
			case output = <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("signal did not settle")
			}
			err = cmd.Wait()
			if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 130 {
				t.Fatalf("exit=%v stderr=%s", err, &stderr)
			}
			if !strings.Contains(output, "Command aborted") || !strings.Contains(output, `"type":"tool_execution_end"`) {
				t.Fatalf("missing final error: %s", output)
			}
			for _, pid := range pids {
				deadline := time.Now().Add(2 * time.Second)
				for syscall.Kill(pid, 0) == nil && time.Now().Before(deadline) {
					time.Sleep(10 * time.Millisecond)
				}
				if err := syscall.Kill(pid, 0); err != syscall.ESRCH {
					t.Fatalf("process %d survived: %v", pid, err)
				}
			}
			io.Copy(io.Discard, reader)
		})
	}
}

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
	"sync/atomic"
	"testing"
	"time"

	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
)

func TestPigTurnRetryLayers(t *testing.T) {
	binary := buildPigBinary(t)
	for _, mode := range []string{"text", "json"} {
		for _, scenario := range []struct {
			name                                                      string
			failures, status, transport, outer, wantCalls, wantStarts int
			success                                                   bool
		}{
			{"transport-only", 1, 503, 1, 2, 2, 0, true},
			{"stream-drop", 1, 200, 1, 2, 2, 1, true},
			{"layered", 3, 503, 1, 2, 4, 1, true},
			{"exhausted", 9, 503, 1, 1, 4, 1, false},
			{"permanent", 9, 401, 1, 2, 1, 0, false},
			{"quota", 9, 429, 0, 2, 1, 0, false},
			{"disabled", 9, 503, 1, 0, 2, 0, false},
		} {
			t.Run(mode+"/"+scenario.name, func(t *testing.T) {
				cwd, home, dir := t.TempDir(), t.TempDir(), t.TempDir()
				settings := fmt.Sprintf(`{"retry":{"enabled":%t,"maxRetries":%d,"baseDelayMs":1,"provider":{"maxRetries":%d}}}`, scenario.name != "disabled", scenario.outer, scenario.transport)
				if err := os.WriteFile(filepath.Join(home, "settings.json"), []byte(settings), 0600); err != nil {
					t.Fatal(err)
				}
				var calls atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if int(calls.Add(1)) <= scenario.failures {
						if scenario.name == "stream-drop" {
							w.Header().Set("Content-Type", "text/event-stream")
							fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"partial\"},\"finish_reason\":null}]}\n\n")
							return
						}
						w.Header().Set("retry-after-ms", "1")
						w.WriteHeader(scenario.status)
						if scenario.name == "quota" {
							fmt.Fprint(w, `{"error":{"message":"insufficient_quota"}}`)
						} else {
							fmt.Fprint(w, `{"error":{"message":"fixture failure"}}`)
						}
						return
					}
					w.Header().Set("Content-Type", "text/event-stream")
					fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"recovered\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
				}))
				defer server.Close()
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				command := exec.CommandContext(ctx, binary, "--provider", "deepseek", "--model", "deepseek-v4-flash", "--thinking", "off", "--no-tools", "--session-dir", dir, "--session-id", "retry-test", "--mode", mode, "-p", "retry please")
				command.Dir = cwd
				command.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + home, "PIG_CODING_AGENT_DIR=" + home, "DEEPSEEK_API_KEY=fixture", "PIG_DEEPSEEK_BASE_URL=" + server.URL}
				var out, stderr bytes.Buffer
				command.Stdout = &out
				command.Stderr = &stderr
				err := command.Run()
				if ctx.Err() != nil {
					t.Fatal("retry did not finish")
				}
				if scenario.success || mode == "json" {
					if err != nil {
						t.Fatalf("process: %v stderr=%s", err, &stderr)
					}
				} else if err == nil {
					t.Fatal("text failure exited successfully")
				}
				if int(calls.Load()) != scenario.wantCalls {
					t.Fatalf("requests=%d want=%d", calls.Load(), scenario.wantCalls)
				}
				if mode == "text" && scenario.success && out.String() != "recovered\n" {
					t.Fatalf("text=%q", out.String())
				}
				if mode == "json" {
					starts, settled := 0, 0
					for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
						var event struct {
							Type      string
							Success   bool
							WillRetry bool
						}
						if err := json.Unmarshal([]byte(line), &event); err != nil {
							t.Fatal(err)
						}
						if event.Type == "auto_retry_start" {
							starts++
						}
						if event.Type == "auto_retry_end" && event.Success != scenario.success {
							t.Fatalf("retry result=%+v", event)
						}
						if event.Type == "agent_settled" {
							settled++
						}
					}
					if starts != scenario.wantStarts || settled != 1 {
						t.Fatalf("retry events=%d settled=%d", starts, settled)
					}
				}
				files, err := filepath.Glob(filepath.Join(dir, "*retry-test.jsonl"))
				if err != nil || len(files) != 1 {
					t.Fatalf("session files=%v err=%v", files, err)
				}
				manager, err := codingagent.OpenSessionManager(files[0], &dir, nil)
				if err != nil {
					t.Fatal(err)
				}
				var failures int
				for _, m := range manager.BuildSessionContext().Messages {
					if a, ok := m.(ai.AssistantMessage); ok && a.StopReason == ai.StopReasonError {
						failures++
						if scenario.name == "stream-drop" && (len(a.Content) != 1 || a.Content[0].(ai.TextContent).Text != "partial") {
							t.Fatalf("lost persisted partial: %+v", a)
						}
					}
				}
				wantFailures := scenario.wantStarts
				if !scenario.success {
					wantFailures++
				}
				if failures != wantFailures {
					t.Fatalf("persisted failed assistants=%d want=%d", failures, wantFailures)
				}
			})
		}
	}
}

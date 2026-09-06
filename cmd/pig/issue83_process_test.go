//go:build darwin || linux

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
)

func TestPigGrepEditContinuation(t *testing.T) {
	binary := buildPigBinary(t)
	for _, mode := range []string{"text", "json"} {
		t.Run(mode, func(t *testing.T) {
			var turns atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request struct {
					Tools    []struct{ Function struct{ Name string } }
					Messages []struct {
						Role    string
						Content any
					}
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
					return
				}
				var names []string
				for _, tool := range request.Tools {
					names = append(names, tool.Function.Name)
				}
				if strings.Join(names, ",") != "grep,edit" {
					t.Errorf("selected tools: %v", names)
				}
				if !strings.Contains(fmt.Sprint(request.Messages[0].Content), "- grep: Search file contents for patterns") {
					t.Error("missing grep system prompt")
				}
				turn := turns.Add(1)
				tool, args := "grep", `{"pattern":"[","path":"code.txt"}`
				last := request.Messages[len(request.Messages)-1]
				switch turn {
				case 2:
					if last.Role != "tool" || !strings.Contains(fmt.Sprint(last.Content), "regex parse error") {
						t.Errorf("error continuation: %+v", last)
					}
					args = `{"pattern":"OLD","path":"code.txt","ignoreCase":true,"context":1}`
				case 3:
					if last.Role != "tool" || last.Content != "code.txt-1- before\ncode.txt:2: old value\ncode.txt-3- after" {
						t.Errorf("search continuation: %+v", last)
					}
					tool, args = "edit", `{"path":"code.txt","edits":[{"oldText":"old value","newText":"new value"}]}`
				case 4:
					if last.Role != "tool" || !strings.Contains(fmt.Sprint(last.Content), "Successfully replaced 1 block(s) in code.txt.") {
						t.Errorf("edit continuation: %+v", last)
					}
				}
				delta := map[string]any{"content": "searched and edited"}
				reason := "stop"
				if turn < 4 {
					delta = map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": fmt.Sprint(turn), "type": "function", "function": map[string]any{"name": tool, "arguments": args}}}}
					reason = "tool_calls"
				}
				w.Header().Set("Content-Type", "text/event-stream")
				data, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": delta, "finish_reason": reason}}})
				fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", data)
			}))
			defer server.Close()
			cwd, home := t.TempDir(), t.TempDir()
			if err := os.WriteFile(filepath.Join(cwd, "code.txt"), []byte("before\nold value\nafter\n"), 0600); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			command := exec.CommandContext(ctx, binary, "--provider", "deepseek", "--model", "deepseek-v4-flash", "--no-session", "--no-approve", "--offline", "--tools", "grep,edit", "--mode", mode, "-p", "search and edit")
			command.Dir = cwd
			command.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + home, "PIG_CODING_AGENT_DIR=" + home, "DEEPSEEK_API_KEY=fixture", "PIG_DEEPSEEK_BASE_URL=" + server.URL}
			var stdout, stderr bytes.Buffer
			command.Stdout, command.Stderr = &stdout, &stderr
			if err := command.Run(); err != nil {
				t.Fatalf("CLI: %v stderr=%s stdout=%s", err, &stderr, &stdout)
			}
			if stderr.Len() != 0 || turns.Load() != 4 {
				t.Fatalf("turns=%d stderr=%s", turns.Load(), &stderr)
			}
			if mode == "text" {
				if stdout.String() != "searched and edited\n" {
					t.Fatal(stdout.String())
				}
			} else {
				for _, line := range strings.Split(strings.TrimSpace(stdout.String()), "\n") {
					if !json.Valid([]byte(line)) {
						t.Fatalf("invalid JSONL: %s", line)
					}
				}
				if !strings.Contains(stdout.String(), `"isError":true`) || !strings.Contains(stdout.String(), "code.txt:2: old value") || !strings.Contains(stdout.String(), "searched and edited") {
					t.Fatal(stdout.String())
				}
			}
			data, err := os.ReadFile(filepath.Join(cwd, "code.txt"))
			if err != nil || string(data) != "before\nnew value\nafter\n" {
				t.Fatalf("edited file: %q %v", data, err)
			}
		})
	}
}

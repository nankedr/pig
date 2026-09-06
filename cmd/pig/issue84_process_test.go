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

func TestPigFindLsReadContinuation(t *testing.T) {
	if _, err := exec.LookPath("fd"); err != nil {
		t.Skip("real fd is required on PATH for CLI parity")
	}
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
				}
				var names []string
				for _, tool := range request.Tools {
					names = append(names, tool.Function.Name)
				}
				if strings.Join(names, ",") != "read,find,ls" {
					t.Errorf("selected tools: %v", names)
				}
				turn := turns.Add(1)
				delta := map[string]any{}
				reason := "tool_calls"
				switch turn {
				case 1:
					delta["tool_calls"] = []any{map[string]any{"index": 0, "id": "ls", "type": "function", "function": map[string]any{"name": "ls", "arguments": `{}`}}}
				case 2:
					if last := request.Messages[len(request.Messages)-1]; last.Role != "tool" || !strings.Contains(fmt.Sprint(last.Content), "nested/") {
						t.Errorf("ls continuation: %#v", last)
					}
					delta["tool_calls"] = []any{map[string]any{"index": 0, "id": "find", "type": "function", "function": map[string]any{"name": "find", "arguments": `{"pattern":"nested/*.txt"}`}}}
				case 3:
					if last := request.Messages[len(request.Messages)-1]; last.Role != "tool" || last.Content != "nested/result.txt" {
						t.Errorf("find continuation: %#v", last)
					}
					delta["tool_calls"] = []any{map[string]any{"index": 0, "id": "read", "type": "function", "function": map[string]any{"name": "read", "arguments": `{"path":"nested/result.txt"}`}}}
				default:
					if last := request.Messages[len(request.Messages)-1]; last.Role != "tool" || last.Content != "你好🙂" {
						t.Errorf("read continuation: %#v", last)
					}
					delta["content"] = "located and read back"
					reason = "stop"
				}
				w.Header().Set("Content-Type", "text/event-stream")
				data, _ := json.Marshal(map[string]any{"id": "fixture", "choices": []any{map[string]any{"delta": delta, "finish_reason": reason}}})
				fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", data)
			}))
			defer server.Close()
			cwd, home := t.TempDir(), t.TempDir()
			if err := os.Mkdir(filepath.Join(cwd, "nested"), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(cwd, "nested/result.txt"), []byte("你好🙂"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(filepath.Join(cwd, ".pig"), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(cwd, ".pig", "settings.json"), []byte(`{"packages":["npm:must-not-load"]}`), 0600); err != nil {
				t.Fatal(err)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			command := exec.CommandContext(ctx, binary, "--provider", "deepseek", "--model", "deepseek-v4-flash", "--no-session", "--tools", "read,find,ls", "--mode", mode, "-p", "save and read")
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
				if stdout.String() != "located and read back\n" {
					t.Fatal(stdout.String())
				}
			} else {
				for _, line := range strings.Split(strings.TrimSpace(stdout.String()), "\n") {
					if !json.Valid([]byte(line)) {
						t.Fatalf("invalid JSONL: %s", line)
					}
				}
				if !strings.Contains(stdout.String(), "nested/result.txt") || !strings.Contains(stdout.String(), "located and read back") {
					t.Fatal(stdout.String())
				}
			}
			data, err := os.ReadFile(filepath.Join(cwd, "nested/result.txt"))
			if err != nil || string(data) != "你好🙂" {
				t.Fatalf("file=%q err=%v", data, err)
			}
		})
	}
}

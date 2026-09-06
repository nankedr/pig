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

func TestPigEditReadContinuation(t *testing.T) {
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
				if strings.Join(names, ",") != "read,edit" {
					t.Errorf("selected tools: %v", names)
				}
				turn := turns.Add(1)
				delta := map[string]any{}
				reason := "tool_calls"
				switch turn {
				case 1:
					delta["tool_calls"] = []any{map[string]any{"index": 0, "id": "edit", "type": "function", "function": map[string]any{"name": "edit", "arguments": `{"path":"target.txt","edits":[{"oldText":"before","newText":"你好🙂"}]}`}}}
				case 2:
					if last := request.Messages[len(request.Messages)-1]; last.Role != "tool" || last.Content != "Successfully replaced 1 block(s) in target.txt." {
						t.Errorf("edit continuation: %#v", last)
					}
					delta["tool_calls"] = []any{map[string]any{"index": 0, "id": "read", "type": "function", "function": map[string]any{"name": "read", "arguments": `{"path":"target.txt"}`}}}
				default:
					if last := request.Messages[len(request.Messages)-1]; last.Role != "tool" || last.Content != "你好🙂" {
						t.Errorf("read continuation: %#v", last)
					}
					delta["content"] = "edited and read back"
					reason = "stop"
				}
				w.Header().Set("Content-Type", "text/event-stream")
				data, _ := json.Marshal(map[string]any{"id": "fixture", "choices": []any{map[string]any{"delta": delta, "finish_reason": reason}}})
				fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", data)
			}))
			defer server.Close()
			cwd, home := t.TempDir(), t.TempDir()
			if err := os.WriteFile(filepath.Join(cwd, "target.txt"), []byte("before"), 0600); err != nil {
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
			command := exec.CommandContext(ctx, binary, "--provider", "deepseek", "--model", "deepseek-v4-flash", "--no-session", "--tools", "read,edit", "--mode", mode, "-p", "edit and read")
			command.Dir = cwd
			command.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + home, "PIG_CODING_AGENT_DIR=" + home, "DEEPSEEK_API_KEY=fixture", "PIG_DEEPSEEK_BASE_URL=" + server.URL}
			var stdout, stderr bytes.Buffer
			command.Stdout, command.Stderr = &stdout, &stderr
			if err := command.Run(); err != nil {
				t.Fatalf("CLI: %v stderr=%s stdout=%s", err, &stderr, &stdout)
			}
			if stderr.Len() != 0 || turns.Load() != 3 {
				t.Fatalf("turns=%d stderr=%s", turns.Load(), &stderr)
			}
			if mode == "text" {
				if stdout.String() != "edited and read back\n" {
					t.Fatal(stdout.String())
				}
			} else {
				for _, line := range strings.Split(strings.TrimSpace(stdout.String()), "\n") {
					if !json.Valid([]byte(line)) {
						t.Fatalf("invalid JSONL: %s", line)
					}
				}
				if !strings.Contains(stdout.String(), `"firstChangedLine":1`) || !strings.Contains(stdout.String(), `"diff":"-1 before\n+1 你好🙂"`) || !strings.Contains(stdout.String(), `"patch":"--- target.txt\n+++ target.txt\n@@ -1,1 +1,1 @@\n-before\n\\ No newline at end of file\n+你好🙂\n\\ No newline at end of file\n"`) || !strings.Contains(stdout.String(), "Successfully replaced 1 block(s)") || !strings.Contains(stdout.String(), "edited and read back") {
					t.Fatal(stdout.String())
				}
			}
			data, err := os.ReadFile(filepath.Join(cwd, "target.txt"))
			if err != nil || string(data) != "你好🙂" {
				t.Fatalf("file=%q err=%v", data, err)
			}
		})
	}
}

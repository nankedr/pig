package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
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

func TestIssue131ResponsesHeadlessProcessMatchesSDKAndKeepsPartialFailure(t *testing.T) {
	binary := buildPigBinary(t)
	for _, failure := range []bool{false, true} {
		t.Run(fmt.Sprint(failure), func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				instructions, _ := body["instructions"].(string)
				if r.URL.Path != "/responses" || body["model"] != "deepseek-v4-pro" || !strings.HasPrefix(instructions, "test system") || r.Header.Get("Authorization") != "Bearer explicit-key" {
					t.Errorf("request = %s, %#v", r.URL, body)
				}
				if body["tools"] != nil {
					t.Error("text path sent tools")
				}
				w.Header().Set("Content-Type", "text/event-stream")
				io.WriteString(w, "data: {\"type\":\"response.output_text.delta\",\"output_index\":0,\"content_index\":0,\"delta\":\"你好🙂\"}\n\n")
				if !failure {
					io.WriteString(w, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp-cli\",\"status\":\"completed\",\"usage\":{\"input_tokens\":10,\"output_tokens\":5,\"total_tokens\":15,\"input_tokens_details\":{\"cached_tokens\":3,\"cache_write_tokens\":1},\"output_tokens_details\":{\"reasoning_tokens\":2}}}}\n\n")
				}
			}))
			defer server.Close()
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{"retry":{"enabled":true,"baseDelayMs":1,"maxRetries":1}}`), 0600); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			command := exec.CommandContext(ctx, binary, "--api", "openai-responses", "--provider", "deepseek", "--model", "deepseek-v4-pro", "--api-key", "explicit-key", "--no-tools", "--no-session", "--no-context-files", "--offline", "--system-prompt", "test system", "--mode", "json", "-p", "你好")
			command.Dir = dir
			command.Env = append(filteredEnvironment(os.Environ(), "DEEPSEEK_API_KEY", "PIG_DEEPSEEK_BASE_URL", "PIG_CODING_AGENT_DIR"), "PIG_DEEPSEEK_BASE_URL="+server.URL, "PIG_CODING_AGENT_DIR="+dir)
			var stdout, stderr bytes.Buffer
			command.Stdout = &stdout
			command.Stderr = &stderr
			err := command.Run()
			if err != nil {
				t.Fatalf("process = %v, stderr = %s", err, stderr.String())
			}
			if requests.Load() != 1 {
				t.Fatalf("partial stream was retried: %d requests", requests.Load())
			}
			var final map[string]any
			for _, record := range decodeJSONLines(t, stdout.String()) {
				if record["type"] == "message_end" {
					message, _ := record["message"].(map[string]any)
					if message["role"] == "assistant" {
						final = message
					}
				}
			}
			wantStop := "stop"
			if failure {
				wantStop = "error"
			}
			if final == nil || final["stopReason"] != wantStop || final["api"] != "openai-responses" {
				t.Fatalf("final = %#v; stderr = %s", final, stderr.String())
			}
			content := final["content"].([]any)
			if len(content) != 1 || content[0].(map[string]any)["text"] != "你好🙂" {
				t.Fatalf("content = %#v", content)
			}
			if !failure {
				u := final["usage"].(map[string]any)
				if u["input"] != float64(6) || u["cacheRead"] != float64(3) || u["reasoning"] != float64(2) {
					t.Fatalf("usage = %#v", u)
				}
			}
			if strings.Contains(stdout.String()+stderr.String(), "explicit-key") {
				t.Fatal("process exposed credential")
			}
		})
	}
}

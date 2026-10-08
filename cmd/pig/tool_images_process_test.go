package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
)

func TestToolImagesCLIAndRPC(t *testing.T) {
	binary := binary133(t)
	for _, mode := range []string{"cli-responses", "cli-chat", "rpc"} {
		t.Run(mode, func(t *testing.T) {
			root, dir := taskRoot133(t)
			data, err := os.ReadFile("../../parity/services/user-image.png")
			if err != nil {
				t.Fatal(err)
			}
			os.WriteFile(filepath.Join(root, "screen.png"), data, 0600)
			imageURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(data)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
					return
				}
				raw, _ := json.Marshal(body)
				imageSeen := bytes.Contains(raw, []byte(imageURL))
				finished := bytes.Contains(raw, []byte("Successfully wrote"))
				name, args, id := "read", map[string]any{"path": "screen.png"}, "read-image"
				if imageSeen {
					name, args, id = "write", map[string]any{"path": "task.txt", "content": "screenshot understood"}, "write-code"
				}
				if finished {
					name = ""
				}
				if (imageSeen || finished) && !bytes.Contains(raw, []byte(imageURL)) {
					t.Error("tool image disappeared")
				}
				w.Header().Set("Content-Type", "text/event-stream")
				if mode == "cli-chat" {
					delta := map[string]any{"content": "IMAGE_TASK_DONE"}
					reason := "stop"
					if name != "" {
						encoded, _ := json.Marshal(args)
						delta = map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": id, "type": "function", "function": map[string]any{"name": name, "arguments": string(encoded)}}}}
						reason = "tool_calls"
					}
					fmt.Fprint(w, frame133(map[string]any{"choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": reason}}})+"data: [DONE]\n\n")
					return
				}
				if name != "" {
					encoded, _ := json.Marshal(args)
					item := map[string]any{"type": "function_call", "id": "fc-" + id, "call_id": id, "name": name, "arguments": string(encoded)}
					fmt.Fprint(w, frame133(map[string]any{"type": "response.output_item.added", "output_index": 0, "item": item})+frame133(map[string]any{"type": "response.output_item.done", "output_index": 0, "item": item}))
				} else {
					fmt.Fprint(w, frame133(map[string]any{"type": "response.output_text.delta", "delta": "IMAGE_TASK_DONE"}))
				}
				fmt.Fprint(w, frame133(map[string]any{"type": "response.completed", "response": map[string]any{"status": "completed"}}))
			}))
			defer server.Close()
			api := ai.APIOpenAIResponses
			if mode == "cli-chat" {
				api = ai.APIOpenAICompletions
			}
			args := []string{"--no-context-files", "--model", "deepseek-flash", "--api", string(api), "--thinking", "off"}
			if mode == "rpc" {
				ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
				defer cancel()
				client := codingagent.NewRPCClient(codingagent.RPCClientOptions{CLIPath: &binary, CWD: &root, Args: args, Env: map[string]string{"HOME": root, "PIG_CODING_AGENT_DIR": dir, "PIG_DEEPSEEK_BASE_URL": server.URL}})
				if err := client.Start(ctx); err != nil {
					t.Fatal(err)
				}
				defer client.Stop(context.Background())
				if _, err := client.PromptAndWait(ctx, "read screenshot and implement"); err != nil {
					t.Fatal(err)
				}
				messages, err := client.GetMessages(ctx)
				if err != nil {
					t.Fatal(err)
				}
				raw, _ := json.Marshal(messages)
				if !bytes.Contains(raw, []byte(base64.StdEncoding.EncodeToString(data))) {
					t.Fatal("RPC history lost tool image")
				}
			} else {
				run := func(extra ...string) {
					t.Helper()
					cmd := exec.Command(binary, append(append([]string{"--print"}, args...), extra...)...)
					cmd.Dir, cmd.Env = root, taskEnv133(root, dir, server.URL)
					out, err := cmd.CombinedOutput()
					if err != nil || !strings.Contains(string(out), "IMAGE_TASK_DONE") {
						t.Fatalf("CLI=%s %v", out, err)
					}
				}
				run("read screenshot and implement")
				sessionsDir := filepath.Join(dir, "sessions")
				sessions, err := codingagent.ListAllSessions(t.Context(), codingagent.SessionListOptions{SessionDir: &sessionsDir})
				if err != nil || len(sessions) != 1 {
					t.Fatal("session missing")
				}
				saved := sessions[0].Path
				os.Remove(filepath.Join(root, "screen.png"))
				run("--session", saved, "continue same screenshot")
				source, _ := os.ReadFile(saved)
				run("--fork", saved, "continue fork screenshot")
				after, _ := os.ReadFile(saved)
				if !bytes.Equal(source, after) {
					t.Fatal("fork source mutated")
				}
			}
			result, err := os.ReadFile(filepath.Join(root, "task.txt"))
			if err != nil || string(result) != "screenshot understood" {
				t.Fatalf("task=%s %v", result, err)
			}
		})
	}
}

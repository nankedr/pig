//go:build !windows

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
	"testing"
	"time"

	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
)

func TestPigDefaultCodingTaskResumeAndFork(t *testing.T) { pigCodingTaskResumeAndFork(t, false) }

func TestPigM4SevenToolsResumeAndFork(t *testing.T) {
	for _, tool := range []string{"fd", "rg"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s must be preinstalled; make m4-gate requires it", tool)
		}
	}
	pigCodingTaskResumeAndFork(t, true)
}

func pigCodingTaskResumeAndFork(t *testing.T, all bool) {
	binary := buildPigBinary(t)
	for _, mode := range []string{"text", "json"} {
		t.Run(mode, func(t *testing.T) {
			cwd, home, dir := t.TempDir(), t.TempDir(), t.TempDir()
			shell := filepath.Join(home, "shell")
			if err := os.WriteFile(shell, []byte("#!/bin/sh\nexport SHELL_MARKER=custom\nexec /bin/sh \"$@\"\n"), 0700); err != nil {
				t.Fatal(err)
			}
			settings, _ := json.Marshal(map[string]any{"shellPath": shell, "shellCommandPrefix": "export PREFIX_MARKER=trusted"})
			if err := os.WriteFile(filepath.Join(home, "settings.json"), settings, 0600); err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request struct {
					Tools    []struct{ Function struct{ Name string } }
					Messages []struct {
						Role       string
						Content    any
						ToolCallID string `json:"tool_call_id"`
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
				wantTools := "read,bash,edit,write"
				if all {
					wantTools += ",grep,find,ls"
				}
				if strings.Join(names, ",") != wantTools {
					t.Errorf("default tools: %v", names)
				}
				phase, users, prior, step := "", 0, 0, 0
				seen := map[string]bool{}
				for _, m := range request.Messages {
					if m.Role == "system" {
						for _, name := range names {
							if !strings.Contains(fmt.Sprint(m.Content), "- "+name+":") {
								t.Errorf("prompt missing %s", name)
							}
						}
					}
					if m.Role == "user" {
						phase = codingTaskText(m.Content)
						users++
						prior += step
						step = 0
					}
					if m.Role == "tool" {
						if seen[m.ToolCallID] {
							t.Errorf("duplicate history: %s", m.ToolCallID)
						}
						seen[m.ToolCallID] = true
						step++
						if strings.HasSuffix(m.ToolCallID, "-bash") {
							id := "original"
							if strings.HasPrefix(m.ToolCallID, "fork-") {
								id = "forked"
							}
							text := fmt.Sprint(m.Content)
							if !strings.HasPrefix(text, "custom|trusted|"+id+"|") || !strings.Contains(text, id+".jsonl|deepseek|deepseek-v4-flash|off\nv2\n") {
								t.Errorf("bash session metadata: %q", text)
							}
						}
						if all && (strings.HasSuffix(m.ToolCallID, "-ls") || strings.HasSuffix(m.ToolCallID, "-find")) && !strings.Contains(fmt.Sprint(m.Content), "task.txt") {
							t.Errorf("search/list result: %v", m.Content)
						}
						if all && strings.HasSuffix(m.ToolCallID, "-grep") && !strings.Contains(fmt.Sprint(m.Content), "1: v2") {
							t.Errorf("grep result: %v", m.Content)
						}
						if strings.HasSuffix(m.ToolCallID, "-read") && fmt.Sprint(m.Content) != "v1\n" && fmt.Sprint(m.Content) != "v2\n" {
							t.Errorf("read result: %v", m.Content)
						}
					}
				}
				wantUsers, wantPrior := 1, 0
				if phase == "resume" {
					wantUsers, wantPrior = 2, 5
					if all {
						wantPrior += 3
					}
				}
				if phase == "fork" {
					wantUsers, wantPrior = 3, 7
					if all {
						wantPrior += 3
					}
				}
				if users != wantUsers || prior != wantPrior {
					t.Errorf("%s history users=%d tools=%d", phase, users, prior)
				}
				commands := []struct {
					name string
					args any
				}{
					{"bash", map[string]any{"command": `printf '%s|%s|%s|%s|%s|%s|%s\n' "$SHELL_MARKER" "$PREFIX_MARKER" "$PIG_SESSION_ID" "$PIG_SESSION_FILE" "$PIG_PROVIDER" "$PIG_MODEL" "$PIG_REASONING_LEVEL"; cat task.txt`}},
					{"read", map[string]any{"path": "task.txt"}},
				}
				if phase == "create" {
					commands = []struct {
						name string
						args any
					}{
						{"write", map[string]any{"path": "task.txt", "content": "v1\n"}},
						{"read", map[string]any{"path": "task.txt"}},
						{"edit", map[string]any{"path": "task.txt", "edits": []any{map[string]any{"oldText": "v1", "newText": "v2"}}}},
						commands[0],
						{"bash", map[string]any{"command": "printf 'expected failure'; exit 7"}},
					}
				}
				if all && phase == "create" {
					commands = append(commands, []struct {
						name string
						args any
					}{
						{"ls", map[string]any{}},
						{"find", map[string]any{"pattern": "*.txt"}},
						{"grep", map[string]any{"pattern": "v2", "path": "task.txt"}},
					}...)
				}
				delta := map[string]any{"content": phase + " complete"}
				reason := "stop"
				if step < len(commands) {
					c := commands[step]
					args, _ := json.Marshal(c.args)
					id := phase + "-" + c.name
					if phase == "create" && step == 4 {
						id = phase + "-failure"
					}
					delta = map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": id, "type": "function", "function": map[string]any{"name": c.name, "arguments": string(args)}}}}
					reason = "tool_calls"
				}
				w.Header().Set("Content-Type", "text/event-stream")
				data, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": delta, "finish_reason": reason}}})
				fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", data)
			}))
			defer server.Close()
			var source string
			var saved []byte
			for _, phase := range []string{"create", "resume", "fork"} {
				args := []string{"--provider", "deepseek", "--model", "deepseek-v4-flash", "--thinking", "off", "--session-dir", dir, "--mode", mode, "-p", phase}
				if all {
					args = append(args, "--tools", "read,bash,edit,write,grep,find,ls")
				}
				id := "original"
				switch phase {
				case "create":
					args = append(args, "--session-id", id)
				case "resume":
					args = append(args, "--continue")
				case "fork":
					id = "forked"
					args = append(args, "--fork", "original", "--session-id", id)
				}
				ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
				command := exec.CommandContext(ctx, binary, args...)
				command.Dir = cwd
				command.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + home, "PIG_CODING_AGENT_DIR=" + home, "DEEPSEEK_API_KEY=fixture", "PIG_DEEPSEEK_BASE_URL=" + server.URL}
				var out, stderr bytes.Buffer
				command.Stdout = &out
				command.Stderr = &stderr
				err := command.Run()
				cancel()
				if err != nil || stderr.Len() != 0 {
					t.Fatalf("%s: %v stderr=%s stdout=%s", phase, err, &stderr, &out)
				}
				if mode == "text" {
					if out.String() != phase+" complete\n" {
						t.Fatal(out.String())
					}
				} else {
					lines := strings.Split(strings.TrimSpace(out.String()), "\n")
					var header struct{ Type, ID string }
					if err := json.Unmarshal([]byte(lines[0]), &header); err != nil || header.Type != "session" || header.ID != id {
						t.Fatalf("session-first header: %s", lines[0])
					}
					for _, line := range lines[1:] {
						if !json.Valid([]byte(line)) {
							t.Fatalf("invalid event: %s", line)
						}
					}
					if phase != "create" && strings.Contains(out.String(), `"toolCallId":"create-write"`) {
						t.Fatal("JSON replayed historical event")
					}
				}
				files, _ := filepath.Glob(filepath.Join(dir, "*"+id+".jsonl"))
				if len(files) != 1 {
					t.Fatal(files)
				}
				manager, err := codingagent.OpenSessionManager(files[0], nil, nil)
				if err != nil {
					t.Fatal(err)
				}
				history := manager.BuildSessionContext().Messages
				want := 12
				if phase == "resume" {
					want = 18
				}
				if phase == "fork" {
					want = 24
				}
				if all {
					want += 6
				}
				if len(history) != want {
					t.Fatalf("%s persisted messages=%d want=%d", phase, len(history), want)
				}
				failures := 0
				for _, m := range history {
					if result, ok := m.(ai.ToolResultMessage); ok && result.IsError {
						failures++
						if result.ToolCallID != "create-failure" {
							t.Fatalf("unexpected failure: %+v", result)
						}
					}
				}
				if failures != 1 {
					t.Fatalf("lost persisted Tool failure: %d", failures)
				}
				if phase == "resume" {
					source = files[0]
					saved, err = os.ReadFile(source)
					if err != nil {
						t.Fatal(err)
					}
				}
				if phase == "fork" {
					after, err := os.ReadFile(source)
					if err != nil || !bytes.Equal(saved, after) {
						t.Fatal("fork modified source")
					}
				}
			}
			data, err := os.ReadFile(filepath.Join(cwd, "task.txt"))
			if err != nil || string(data) != "v2\n" {
				t.Fatalf("task file: %q %v", data, err)
			}
		})
	}
}

func codingTaskText(content any) string {
	if text, ok := content.(string); ok {
		return text
	}
	var text string
	if blocks, ok := content.([]any); ok {
		for _, block := range blocks {
			if item, ok := block.(map[string]any); ok {
				if value, ok := item["text"].(string); ok {
					text += value
				}
			}
		}
	}
	return text
}

func TestPigDefaultCodingTrustAndStartupFailures(t *testing.T) {
	binary := buildPigBinary(t)
	for _, name := range []string{"trusted", "untrusted", "credentials", "model", "storage"} {
		t.Run(name, func(t *testing.T) {
			cwd, home := t.TempDir(), t.TempDir()
			if err := os.Mkdir(filepath.Join(cwd, ".pig"), 0700); err != nil {
				t.Fatal(err)
			}
			global := `{"shellCommandPrefix":"printf global > prefix.txt","retry":{"provider":{"maxRetries":0}}}`
			project := `{"shellCommandPrefix":"printf project > prefix.txt"}`
			if err := os.WriteFile(filepath.Join(home, "settings.json"), []byte(global), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(cwd, ".pig", "settings.json"), []byte(project), 0600); err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if name == "credentials" {
					http.Error(w, "invalid fixture credentials", http.StatusUnauthorized)
					return
				}
				var body struct{ Messages []struct{ Role string } }
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				if body.Messages[len(body.Messages)-1].Role == "tool" {
					fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"done\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
					return
				}
				fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"bash\",\"type\":\"function\",\"function\":{\"name\":\"bash\",\"arguments\":\"{\\\"command\\\":\\\"printf ok\\\"}\"}}]},\"finish_reason\":\"tool_calls\"}]}\n\ndata: [DONE]\n\n")
			}))
			defer server.Close()
			args := []string{"--provider", "deepseek", "--model", "deepseek-v4-flash", "-p", "run"}
			if name == "trusted" {
				args = append(args, "--approve")
			} else {
				args = append(args, "--no-approve")
			}
			if name == "model" {
				args[1], args[3] = "anthropic", "claude-sonnet-4-5"
			}
			if name == "storage" {
				block := filepath.Join(home, "blocked")
				if err := os.WriteFile(block, []byte("sentinel"), 0600); err != nil {
					t.Fatal(err)
				}
				args = append(args, "--session-dir", filepath.Join(block, "sessions"))
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			command := exec.CommandContext(ctx, binary, args...)
			command.Dir = cwd
			command.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + home, "PIG_CODING_AGENT_DIR=" + home, "DEEPSEEK_API_KEY=fixture", "PIG_DEEPSEEK_BASE_URL=" + server.URL}
			var out, stderr bytes.Buffer
			command.Stdout = &out
			command.Stderr = &stderr
			err := command.Run()
			success := name == "trusted" || name == "untrusted"
			if success {
				want := "global"
				if name == "trusted" {
					want = "project"
				}
				data, readErr := os.ReadFile(filepath.Join(cwd, "prefix.txt"))
				if err != nil || out.String() != "done\n" || string(data) != want || readErr != nil {
					t.Fatalf("%s: err=%v out=%s stderr=%s prefix=%q/%v", name, err, &out, &stderr, data, readErr)
				}
			} else if err == nil || stderr.Len() == 0 || strings.Contains(out.String(), "done") {
				t.Fatalf("failure claimed success: %v stdout=%s stderr=%s", err, &out, &stderr)
			}
		})
	}
}

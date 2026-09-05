//go:build !windows

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nankedr/pig/codingagent"
)

func TestPigProcessesContinueHistoricalSessions(t *testing.T) {
	binary := buildPigBinary(t)
	for _, version := range []int{1, 2, 3} {
		t.Run(fmt.Sprintf("v%d", version), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body struct {
					Messages []struct {
						Role    string
						Content any
					}
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				roles := []string{}
				for _, m := range body.Messages {
					roles = append(roles, m.Role)
				}
				if got := strings.Join(roles, ","); got != "system,user,assistant,user" {
					t.Errorf("Provider history=%s", got)
				}
				if len(body.Messages) > 1 && body.Messages[1].Content != "historic prompt" {
					t.Errorf("historic text=%v", body.Messages[1].Content)
				}
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, "data: {\"id\":\"history\",\"choices\":[{\"delta\":{\"content\":\"continued\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
			}))
			defer server.Close()
			cwd := t.TempDir()
			path := filepath.Join(cwd, "history.jsonl")
			header, _ := json.Marshal(map[string]any{"type": "session", "version": version, "id": "legacy", "cwd": cwd, "timestamp": "2025-01-01T00:00:00.000Z", "extra": "keep"})
			content := "bad line\n" + string(header) + "\n" + `{"type":"message","id":"u","parentId":null,"timestamp":"2025-01-01T00:00:00.000Z","message":{"role":"user","content":"historic prompt","timestamp":1}}` + "\n" + `{"type":"message","id":"a","parentId":"u","timestamp":"2025-01-01T00:00:00.000Z","message":{"role":"assistant","content":[{"type":"text","text":"historic reply"}],"provider":"deepseek","api":"openai-completions","model":"deepseek-v4-flash","usage":{"input":1,"output":1,"cacheRead":0,"cacheWrite":0,"totalTokens":2,"cost":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0,"total":0}},"stopReason":"stop","timestamp":2}}` + "\n"
			if err := os.WriteFile(path, []byte(content), 0600); err != nil {
				t.Fatal(err)
			}
			command := exec.Command(binary, "--provider", "deepseek", "--model", "deepseek-v4-flash", "--api-key", "key", "--session", path, "-p", "continue")
			command.Dir = cwd
			command.Env = append(filteredEnvironment(os.Environ(), "DEEPSEEK_API_KEY", "PIG_DEEPSEEK_BASE_URL", "PIG_CODING_AGENT_DIR", "PIG_CODING_AGENT_SESSION_DIR"), "PIG_DEEPSEEK_BASE_URL="+server.URL)
			if output, err := command.CombinedOutput(); err != nil || string(output) != "continued\n" {
				t.Fatalf("pig=%q, %v", output, err)
			}
			manager, err := codingagent.OpenSessionManager(path, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			if *manager.GetHeader().Version != 3 || len(manager.BuildSessionContext().Messages) != 4 {
				t.Fatalf("restored header=%+v context=%+v", manager.GetHeader(), manager.BuildSessionContext())
			}
			entries := manager.GetEntries()
			for i, e := range entries {
				if i > 0 && (e.ParentID == nil || *e.ParentID != entries[i-1].ID) {
					t.Fatalf("broken parent chain at %d", i)
				}
			}
			data, err := os.ReadFile(path)
			if err != nil || !strings.Contains(string(data), `"extra":"keep"`) {
				t.Fatalf("lost header: %v", err)
			}
		})
	}
}

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
	"sync"
	"testing"
	"time"

	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
)

func frame133(value any) string {
	data, _ := json.Marshal(value)
	return "data: " + string(data) + "\n\n"
}

func binary133(t *testing.T) string {
	t.Helper()
	if os.Getenv("PIG_TEST_RACE") != "1" {
		return buildPigBinary(t)
	}
	binary := filepath.Join(t.TempDir(), "pig")
	if out, err := exec.Command("go", "build", "-race", "-o", binary, ".").CombinedOutput(); err != nil {
		t.Fatalf("race build: %v %s", err, out)
	}
	return binary
}

func taskServer133(t *testing.T) (*httptest.Server, func() []map[string]any) {
	t.Helper()
	data, err := os.ReadFile("../../parity/services/responses-codingagent.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Output    []json.RawMessage
		Completed json.RawMessage
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var requests []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		if r.URL.Path != "/responses" || r.Header.Get("Authorization") != "Bearer stored-key" || body["store"] != false || body["previous_response_id"] != nil || body["conversation"] != nil {
			t.Errorf("target/auth/state: %s %s %#v", r.URL.Path, r.Header.Get("Authorization"), body)
		}
		mu.Lock()
		requests = append(requests, body)
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		input, _ := json.Marshal(body["input"])
		text := "TASK_DONE"
		if body["tools"] == nil {
			text = "CHECKPOINT"
		} else if strings.Contains(string(input), "continue") {
			text = "CONTINUED"
		} else if !strings.Contains(string(input), "function_call_output") {
			for i, item := range fixture.Output {
				fmt.Fprint(w, frame133(map[string]any{"type": "response.output_item.added", "output_index": i, "item": item}))
				fmt.Fprint(w, frame133(map[string]any{"type": "response.output_item.done", "output_index": i, "item": item}))
			}
			text = ""
		}
		if text != "" {
			fmt.Fprint(w, frame133(map[string]any{"type": "response.output_text.delta", "delta": text}))
		}
		fmt.Fprint(w, frame133(map[string]any{"type": "response.completed", "response": fixture.Completed}))
	}))
	t.Cleanup(server.Close)
	return server, func() []map[string]any { mu.Lock(); defer mu.Unlock(); return append([]map[string]any{}, requests...) }
}

func taskRoot133(t *testing.T) (string, string) {
	t.Helper()
	root, dir := t.TempDir(), t.TempDir()
	for path, data := range map[string]string{
		filepath.Join(root, "sentinel.txt"): "before\n",
		filepath.Join(dir, "auth.json"):     `{"deepseek":{"type":"api_key","key":"stored-key"}}`,
		filepath.Join(dir, "settings.json"): `{"defaultProvider":"deepseek","defaultModel":"deepseek-v4-pro","defaultAPI":"openai-responses","retry":{"enabled":false},"compaction":{"enabled":false,"keepRecentTokens":1}}`,
	} {
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return root, dir
}

func taskEnv133(root, dir, endpoint string) []string {
	return []string{"PATH=" + os.Getenv("PATH"), "HOME=" + root, "PIG_CODING_AGENT_DIR=" + dir, "PIG_CODING_AGENT_SESSION_DIR=" + filepath.Join(dir, "sessions"), "PIG_DEEPSEEK_BASE_URL=" + endpoint, "TERM=xterm-256color"}
}

func assertTask133(t *testing.T, root, path string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, "sentinel.txt"))
	if err != nil || string(data) != "after\n" {
		t.Fatalf("coding task: %q %v", data, err)
	}
	manager, err := codingagent.OpenSessionManager(path, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	context := manager.BuildSessionContext()
	if context.Model == nil || context.Model.API != ai.APIOpenAIResponses {
		t.Fatalf("persisted model: %+v", context.Model)
	}
	var calls, results int
	var reasoning bool
	for _, message := range context.Messages {
		switch message := message.(type) {
		case ai.AssistantMessage:
			for _, content := range message.Content {
				switch content := content.(type) {
				case ai.ToolCall:
					calls++
					if content.ID != "edit-once|fc-task" {
						t.Errorf("call ID: %s", content.ID)
					}
				case ai.ThinkingContent:
					reasoning = content.Thinking == "replace once"
				}
			}
		case ai.ToolResultMessage:
			results++
			if message.IsError {
				t.Fatal("edit failed")
			}
		}
	}
	if calls != 1 || results != 1 || !reasoning {
		t.Fatalf("history: calls=%d results=%d reasoning=%v", calls, results, reasoning)
	}
}

func TestResponsesCLICodingTaskResumeAndFork(t *testing.T) {
	server, requests := taskServer133(t)
	root, dir := taskRoot133(t)
	binary := binary133(t)
	run := func(args ...string) string {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, binary, append([]string{"--print", "--tools", "edit", "--no-context-files"}, args...)...)
		cmd.Dir, cmd.Env = root, taskEnv133(root, dir, server.URL)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("CLI: %v %s", err, out)
		}
		return string(out)
	}
	if !strings.Contains(run("change sentinel.txt"), "TASK_DONE") {
		t.Fatal("task did not finish")
	}
	sessionsDir := filepath.Join(dir, "sessions")
	sessions, err := codingagent.ListAllSessions(context.Background(), codingagent.SessionListOptions{SessionDir: &sessionsDir})
	if err != nil || len(sessions) != 1 {
		t.Fatalf("sessions: %v %v", sessions, err)
	}
	source := sessions[0].Path
	assertTask133(t, root, source)
	if !strings.Contains(run("--session", source, "continue"), "CONTINUED") {
		t.Fatal("resume failed")
	}
	before, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(run("--fork", source, "continue fork"), "CONTINUED") {
		t.Fatal("fork failed")
	}
	after, err := os.ReadFile(source)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("fork changed source")
	}
	for _, request := range requests()[2:] {
		input, _ := json.Marshal(request["input"])
		if !bytes.Contains(input, []byte("rs-task")) || !bytes.Contains(input, []byte("fc-task")) || !bytes.Contains(input, []byte("function_call_output")) {
			t.Fatalf("resume lost replay: %s", input)
		}
	}
}

func TestResponsesRPCCodingTaskLifecycle(t *testing.T) {
	server, requests := taskServer133(t)
	root, dir := taskRoot133(t)
	binary := binary133(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client := codingagent.NewRPCClient(codingagent.RPCClientOptions{CLIPath: &binary, CWD: &root, Args: []string{"--tools", "edit", "--no-context-files", "--session-dir", filepath.Join(dir, "sessions")}, Env: map[string]string{"HOME": root, "PIG_CODING_AGENT_DIR": dir, "PIG_DEEPSEEK_BASE_URL": server.URL}})
	if err := client.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer client.Stop(context.Background())
	if _, err := client.PromptAndWait(ctx, "change sentinel.txt"); err != nil {
		t.Fatal(err)
	}
	state, err := client.GetState(ctx)
	if err != nil || state.SessionFile == nil {
		t.Fatalf("state: %+v %v", state, err)
	}
	source := *state.SessionFile
	assertTask133(t, root, source)
	stats, err := client.GetSessionStats(ctx)
	if err != nil || stats.ToolCalls != 1 || stats.ToolResults != 1 || stats.Tokens.TotalTokens != 30 {
		t.Fatalf("stats: %+v %v", stats, err)
	}
	selected, err := client.SetModel(ctx, "deepseek", "deepseek-v4-flash")
	if err != nil || selected.ID != "deepseek-v4-flash" {
		t.Fatalf("selected: %+v %v", selected, err)
	}
	models, err := client.GetAvailableModels(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, model := range models {
		if model.Provider != "deepseek" {
			t.Fatalf("available model: %+v", model)
		}
	}
	if _, err := client.PromptAndWait(ctx, "continue second"); err != nil {
		t.Fatal(err)
	}
	users, err := client.GetForkMessages(ctx)
	if err != nil || len(users) != 2 {
		t.Fatalf("fork messages: %+v %v", users, err)
	}
	before, _ := os.ReadFile(source)
	if _, cancelled, err := client.Fork(ctx, users[1].EntryID); err != nil || cancelled {
		t.Fatalf("fork: %v %v", cancelled, err)
	}
	if _, err := client.PromptAndWait(ctx, "continue fork"); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(source)
	if !bytes.Equal(before, after) {
		t.Fatal("fork changed source")
	}
	if cancelled, err := client.SwitchSession(ctx, source); err != nil || cancelled {
		t.Fatalf("switch: %v %v", cancelled, err)
	}
	result, err := client.Compact(ctx)
	if err != nil || !strings.Contains(result.Summary, "CHECKPOINT") {
		t.Fatalf("compact: %+v %v", result, err)
	}
	if _, err := client.PromptAndWait(ctx, "continue compact"); err != nil {
		t.Fatal(err)
	}
	input, _ := json.Marshal(requests()[len(requests())-1]["input"])
	if !bytes.Contains(input, []byte("CHECKPOINT")) {
		t.Fatalf("compacted history: %s", input)
	}
	if err := client.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	client = codingagent.NewRPCClient(codingagent.RPCClientOptions{CLIPath: &binary, CWD: &root, Args: []string{"--session", source, "--tools", "edit", "--no-context-files"}, Env: map[string]string{"HOME": root, "PIG_CODING_AGENT_DIR": dir, "PIG_DEEPSEEK_BASE_URL": server.URL}})
	if err := client.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer client.Stop(context.Background())
	if _, err := client.PromptAndWait(ctx, "continue cross process"); err != nil {
		t.Fatal(err)
	}
}

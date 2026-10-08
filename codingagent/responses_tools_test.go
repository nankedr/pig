package codingagent_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
)

func responseFrame132(v any) string { b, _ := json.Marshal(v); return "data: " + string(b) + "\n\n" }

func TestResponsesSessionRetryPersistsAndReplaysWithoutRepeatingTools(t *testing.T) {
	var requests, effects, retries atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		if requests.Add(1) == 1 {
			reasoning := map[string]any{"type": "reasoning", "id": "rs-persist", "content": []any{map[string]any{"type": "reasoning_text", "text": "先查询"}}}
			call := map[string]any{"type": "function_call", "id": "fc-once", "call_id": "once", "name": "lookup", "arguments": "{}"}
			for i, item := range []any{reasoning, call} {
				io.WriteString(w, responseFrame132(map[string]any{"type": "response.output_item.added", "output_index": i, "item": item}))
				io.WriteString(w, responseFrame132(map[string]any{"type": "response.output_item.done", "output_index": i, "item": item}))
			}
			io.WriteString(w, responseFrame132(map[string]any{"type": "response.completed", "response": map[string]any{"status": "completed"}}))
			return
		}
		calls, outputs, reasoning := 0, 0, false
		for _, raw := range body["input"].([]any) {
			item := raw.(map[string]any)
			switch item["type"] {
			case "function_call":
				calls++
				if item["call_id"] != "once" || item["id"] != "fc-once" {
					t.Errorf("call replay=%#v", item)
				}
			case "function_call_output":
				outputs++
				if item["call_id"] != "once" || item["output"] != "result-once" {
					t.Errorf("output replay=%#v", item)
				}
			case "reasoning":
				reasoning = item["id"] == "rs-persist" && item["content"].([]any)[0].(map[string]any)["text"] == "先查询"
			}
		}
		if calls != 1 || outputs != 1 || !reasoning || effects.Load() != 1 {
			t.Errorf("history calls=%d outputs=%d reasoning=%v effects=%d", calls, outputs, reasoning, effects.Load())
		}
		if requests.Load() == 2 {
			io.WriteString(w, responseFrame132(map[string]any{"type": "response.failed", "response": map[string]any{"status": "failed", "error": map[string]any{"code": "503", "message": "service unavailable"}}}))
			return
		}
		io.WriteString(w, responseFrame132(map[string]any{"type": "response.output_text.delta", "delta": "完成"})+responseFrame132(map[string]any{"type": "response.completed", "response": map[string]any{"status": "completed"}}))
	}))
	defer server.Close()
	tool, err := agent.EraseAgentTool(agent.AgentTool[map[string]any, any]{Tool: ai.Tool{Name: "lookup", Parameters: json.RawMessage(`{"type":"object"}`)}, DecodeValidated: func(v ai.JSONValue) map[string]any { return v.(map[string]any) }, Execute: func(context.Context, string, map[string]any, agent.AgentToolUpdateCallback[any]) (agent.AgentToolResult[any], error) {
		effects.Add(1)
		return agent.AgentToolResult[any]{Content: []ai.ToolResultContent{ai.TextContent{Type: ai.ContentTypeText, Text: "result-once"}}}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	model := ai.Model{ID: "fixture", Provider: ai.ProviderIDDeepSeek, API: ai.APIOpenAIResponses, BaseURL: server.URL, MaxTokens: 512, Input: []ai.ModelInput{ai.ModelInputText}}
	key := "offline"
	zero := 0
	cache := ai.CacheRetentionNone
	stream := func(ctx context.Context, m ai.Model, c ai.Context, o ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
		o.APIKey = &key
		o.MaxRetries = &zero
		o.CacheRetention = &cache
		return ai.StreamSimpleOpenAIResponses(ctx, m, c, o)
	}
	enabled, maxRetries, delay := true, 1, int64(1)
	settings, _ := codingagent.NewInMemorySettingsManager(codingagent.Settings{Retry: &codingagent.RetrySettings{Enabled: &enabled, MaxRetries: &maxRetries, BaseDelayMS: &delay}})
	dir := t.TempDir()
	manager, err := codingagent.NewSessionManager(dir, &dir)
	if err != nil {
		t.Fatal(err)
	}
	create := func(manager *codingagent.SessionManager) *codingagent.AgentSession {
		t.Helper()
		created, err := codingagent.CreateAgentSession(context.Background(), codingagent.CreateAgentSessionOptions{CWD: dir, Model: &model, StreamFunction: stream, SessionManager: manager, SettingsManager: settings, NoTools: codingagent.NoToolsBuiltin, AgentTools: []agent.ErasedAgentTool{tool}, NoContextFiles: true})
		if err != nil {
			t.Fatal(err)
		}
		return created.Session
	}
	session := create(manager)
	session.Subscribe(func(e codingagent.AgentSessionEvent) {
		if e.AgentSessionEventType() == codingagent.AgentSessionEventTypeAutoRetryStart {
			retries.Add(1)
		}
	})
	if err := session.Prompt(context.Background(), "查询"); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 3 || effects.Load() != 1 || retries.Load() != 1 {
		t.Fatalf("requests=%d effects=%d retries=%d", requests.Load(), effects.Load(), retries.Load())
	}
	path := *session.SessionFile()
	if err := session.Dispose(); err != nil {
		t.Fatal(err)
	}
	reopened, err := codingagent.OpenSessionManager(path, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	failures := 0
	for _, entry := range reopened.GetEntries() {
		if m, ok := entry.Message.(ai.AssistantMessage); ok && m.StopReason == ai.StopReasonError {
			failures++
		}
	}
	if failures != 1 {
		t.Fatalf("persisted failures=%d", failures)
	}
	restored := create(reopened)
	defer restored.Dispose()
	if err := restored.Prompt(context.Background(), "继续"); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 4 || effects.Load() != 1 {
		t.Fatalf("restored requests=%d effects=%d", requests.Load(), effects.Load())
	}
}

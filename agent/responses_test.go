package agent_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
)

func responsesFrame(v any) string { b, _ := json.Marshal(v); return "data: " + string(b) + "\n\n" }
func responsesToolItem(id, args string) map[string]any {
	return map[string]any{"type": "function_call", "id": "item-" + id, "call_id": id, "name": "lookup", "arguments": args}
}

func TestResponsesAgentToolContinuationAndMultiTurn(t *testing.T) {
	var requests atomic.Int32
	var effects []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		for _, key := range []string{"previous_response_id", "conversation", "background"} {
			if _, ok := body[key]; ok {
				t.Errorf("server state requested: %s", key)
			}
		}
		if body["store"] != false {
			t.Error("store must be false")
		}
		tools, _ := body["tools"].([]any)
		if len(tools) != 1 || tools[0].(map[string]any)["name"] != "lookup" {
			t.Errorf("tools = %#v", tools)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		switch requests.Add(1) {
		case 1:
			io.WriteString(w, responsesFrame(map[string]any{"type": "response.output_item.added", "output_index": 0, "item": map[string]any{"type": "reasoning", "id": "rs-1", "content": []any{}}}))
			io.WriteString(w, responsesFrame(map[string]any{"type": "response.reasoning_text.delta", "output_index": 0, "delta": "先查询"}))
			for i, id := range []string{"call-a", "call-b"} {
				io.WriteString(w, responsesFrame(map[string]any{"type": "response.output_item.added", "output_index": i + 1, "item": responsesToolItem(id, "")}))
				for _, delta := range []string{`{"value":`, `"` + id + `"}`} {
					io.WriteString(w, responsesFrame(map[string]any{"type": "response.function_call_arguments.delta", "output_index": i + 1, "delta": delta}))
				}
				io.WriteString(w, responsesFrame(map[string]any{"type": "response.output_item.done", "output_index": i + 1, "item": responsesToolItem(id, `{"value":"`+id+`"}`)}))
			}
			io.WriteString(w, responsesFrame(map[string]any{"type": "response.completed", "response": map[string]any{"status": "completed"}}))
		default:
			input := body["input"].([]any)
			var calls, outputs []string
			reasoning := false
			for _, raw := range input {
				item := raw.(map[string]any)
				switch item["type"] {
				case "reasoning":
					reasoning = item["content"].([]any)[0].(map[string]any)["text"] == "先查询"
					if item["summary"] != nil || item["encrypted_content"] != nil {
						t.Error("unsupported reasoning replay")
					}
				case "function_call":
					calls = append(calls, item["call_id"].(string))
				case "function_call_output":
					outputs = append(outputs, item["call_id"].(string))
					if item["output"] != "result-"+item["call_id"].(string) {
						t.Errorf("result = %#v", item)
					}
				}
			}
			if !reasoning || !reflect.DeepEqual(calls, []string{"call-a", "call-b"}) || !reflect.DeepEqual(outputs, calls) {
				t.Errorf("replay = %#v", input)
			}
			if !reflect.DeepEqual(effects, []string{"call-a|item-call-a", "call-b|item-call-b"}) {
				t.Errorf("execution order = %v", effects)
			}
			io.WriteString(w, responsesFrame(map[string]any{"type": "response.output_text.delta", "delta": "完成"}))
			io.WriteString(w, responsesFrame(map[string]any{"type": "response.completed", "response": map[string]any{"status": "completed"}}))
		}
	}))
	defer server.Close()
	tool, err := agent.EraseAgentTool(agent.AgentTool[map[string]any, any]{Tool: ai.Tool{Name: "lookup", Parameters: json.RawMessage(`{"type":"object","properties":{"value":{"type":"string"}},"required":["value"]}`)}, DecodeValidated: func(v ai.JSONValue) map[string]any { return v.(map[string]any) }, Execute: func(_ context.Context, id string, p map[string]any, update agent.AgentToolUpdateCallback[any]) (agent.AgentToolResult[any], error) {
		if p["value"] != strings.Split(id, "|")[0] {
			t.Errorf("args = %#v", p)
		}
		effects = append(effects, id)
		update(agent.AgentToolResult[any]{Content: []ai.ToolResultContent{ai.TextContent{Type: ai.ContentTypeText, Text: "progress"}}})
		return agent.AgentToolResult[any]{Content: []ai.ToolResultContent{ai.TextContent{Type: ai.ContentTypeText, Text: "result-" + strings.Split(id, "|")[0]}}}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	model := ai.Model{ID: "fixture", Provider: ai.ProviderIDDeepSeek, API: ai.APIOpenAIResponses, BaseURL: server.URL, Input: []ai.ModelInput{ai.ModelInputText}, MaxTokens: 512}
	key := "offline"
	a, err := agent.NewAgent(agent.AgentOptions{ToolExecution: agent.ToolExecutionSequential, InitialState: &agent.AgentInitialState{Model: model, Tools: []agent.ErasedAgentTool{tool}}, StreamFunction: func(ctx context.Context, m ai.Model, c ai.Context, o ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
		o.APIKey = &key
		return ai.StreamSimpleOpenAIResponses(ctx, m, c, o)
	}})
	if err != nil {
		t.Fatal(err)
	}
	var argumentSnapshots []map[string]any
	a.Subscribe(func(_ context.Context, event agent.AgentEvent) error {
		if update, ok := event.(agent.MessageUpdateEvent); ok {
			if delta, ok := update.AssistantMessageEvent.(ai.AssistantMessageToolCallDeltaEvent); ok {
				call := delta.Partial.Content[delta.ContentIndex].(ai.ToolCall)
				argumentSnapshots = append(argumentSnapshots, call.Arguments)
			}
		}
		return nil
	})
	for _, text := range []string{"查询", "再回答"} {
		if err := a.Prompt(context.Background(), ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText(text)}); err != nil {
			t.Fatal(err)
		}
	}
	if want := []map[string]any{{}, {"value": "call-a"}, {}, {"value": "call-b"}}; !reflect.DeepEqual(argumentSnapshots, want) {
		t.Fatalf("public Agent argument prefixes=%#v, want %#v", argumentSnapshots, want)
	}
	if requests.Load() != 3 || len(effects) != 2 {
		t.Fatalf("requests=%d effects=%v", requests.Load(), effects)
	}
	messages := a.State().Messages
	final := messages[len(messages)-1].(ai.AssistantMessage)
	if final.StopReason != ai.StopReasonStop || final.Content[0].(ai.TextContent).Text != "完成" {
		t.Fatalf("final=%#v", final)
	}
}

func TestResponsesAgentDoesNotExecuteFailedPartialCallsAndCanContinue(t *testing.T) {
	for _, mode := range []string{"cancel", "truncated", "invalid-json", "mismatched-id", "duplicate-id", "failed"} {
		t.Run(mode, func(t *testing.T) {
			var requests, effects atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				if requests.Add(1) > 1 {
					var body map[string]any
					json.NewDecoder(r.Body).Decode(&body)
					for _, raw := range body["input"].([]any) {
						if raw.(map[string]any)["type"] == "function_call" {
							t.Error("failed call replayed")
						}
					}
					io.WriteString(w, responsesFrame(map[string]any{"type": "response.output_text.delta", "delta": "恢复"})+responsesFrame(map[string]any{"type": "response.completed", "response": map[string]any{"status": "completed"}}))
					return
				}
				io.WriteString(w, responsesFrame(map[string]any{"type": "response.output_item.added", "item": responsesToolItem("a", "")}))
				io.WriteString(w, responsesFrame(map[string]any{"type": "response.function_call_arguments.delta", "delta": `{"value":"part`}))
				w.(http.Flusher).Flush()
				switch mode {
				case "cancel":
					<-r.Context().Done()
				case "truncated":
					return
				case "invalid-json":
					io.WriteString(w, responsesFrame(map[string]any{"type": "response.output_item.done", "item": responsesToolItem("a", `{"value":"part`)}))
				case "mismatched-id":
					io.WriteString(w, responsesFrame(map[string]any{"type": "response.function_call_arguments.delta", "item_id": "wrong", "delta": "x"}))
				case "duplicate-id":
					io.WriteString(w, responsesFrame(map[string]any{"type": "response.output_item.added", "output_index": 1, "item": responsesToolItem("a", "")}))
				case "failed":
					io.WriteString(w, responsesFrame(map[string]any{"type": "response.failed", "response": map[string]any{"status": "failed", "error": map[string]any{"code": "503", "message": "unavailable"}}}))
				}
			}))
			defer server.Close()
			tool, err := agent.EraseAgentTool(agent.AgentTool[map[string]any, any]{Tool: ai.Tool{Name: "lookup", Parameters: json.RawMessage(`{"type":"object"}`)}, DecodeValidated: func(v ai.JSONValue) map[string]any { return v.(map[string]any) }, Execute: func(context.Context, string, map[string]any, agent.AgentToolUpdateCallback[any]) (agent.AgentToolResult[any], error) {
				effects.Add(1)
				return agent.AgentToolResult[any]{}, nil
			}})
			if err != nil {
				t.Fatal(err)
			}
			key := "offline"
			model := ai.Model{ID: "fixture", Provider: ai.ProviderIDDeepSeek, API: ai.APIOpenAIResponses, BaseURL: server.URL, MaxTokens: 512}
			a, err := agent.NewAgent(agent.AgentOptions{InitialState: &agent.AgentInitialState{Model: model, Tools: []agent.ErasedAgentTool{tool}}, StreamFunction: func(ctx context.Context, m ai.Model, c ai.Context, o ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
				o.APIKey = &key
				return ai.StreamSimpleOpenAIResponses(ctx, m, c, o)
			}})
			if err != nil {
				t.Fatal(err)
			}
			a.Subscribe(func(_ context.Context, e agent.AgentEvent) error {
				if update, ok := e.(agent.MessageUpdateEvent); ok && mode == "cancel" {
					if _, ok := update.AssistantMessageEvent.(ai.AssistantMessageToolCallDeltaEvent); ok {
						a.Abort()
					}
				}
				return nil
			})
			_ = a.Prompt(context.Background(), ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText("查询")})
			messages := a.State().Messages
			partial := messages[len(messages)-1].(ai.AssistantMessage)
			if len(partial.Content) != 1 || effects.Load() != 0 || partial.StopReason != ai.StopReasonError && partial.StopReason != ai.StopReasonAborted {
				t.Fatalf("partial=%#v effects=%d", partial, effects.Load())
			}
			if err := a.Prompt(context.Background(), ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText("继续")}); err != nil {
				t.Fatal(err)
			}
			if requests.Load() != 2 || effects.Load() != 0 {
				t.Fatalf("requests=%d effects=%d", requests.Load(), effects.Load())
			}
		})
	}
}

func TestResponsesAgentPreflightFailuresAndBarriers(t *testing.T) {
	for _, mode := range []string{"sequential", "parallel", "validation", "tool-failure", "listener-failure"} {
		t.Run(mode, func(t *testing.T) {
			var requests, prepared, executed, updates, ended atomic.Int32
			both := make(chan struct{})
			var a *agent.Agent
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				if requests.Add(1) == 1 {
					for i, id := range []string{"a", "b"} {
						io.WriteString(w, responsesFrame(map[string]any{"type": "response.output_item.added", "output_index": i, "item": responsesToolItem(id, `{}`)}))
						io.WriteString(w, responsesFrame(map[string]any{"type": "response.output_item.done", "output_index": i, "item": responsesToolItem(id, `{}`)}))
					}
					io.WriteString(w, responsesFrame(map[string]any{"type": "response.completed", "response": map[string]any{"status": "completed"}}))
					return
				}
				var body map[string]any
				json.NewDecoder(r.Body).Decode(&body)
				results := 0
				for _, raw := range body["input"].([]any) {
					item := raw.(map[string]any)
					if item["type"] == "function_call_output" {
						results++
						if mode == "tool-failure" && !strings.Contains(item["output"].(string), "lookup failed") {
							t.Errorf("failure not returned: %#v", item)
						}
					}
				}
				expected := int32(2)
				if mode == "validation" {
					expected = 0
				}
				if results != 2 || executed.Load() != expected || updates.Load() != expected || ended.Load() != 2 {
					t.Errorf("barrier: results=%d executed=%d updates=%d ended=%d", results, executed.Load(), updates.Load(), ended.Load())
				}
				io.WriteString(w, responsesFrame(map[string]any{"type": "response.completed", "response": map[string]any{"status": "completed"}}))
			}))
			defer server.Close()
			tool, err := agent.EraseAgentTool(agent.AgentTool[map[string]any, any]{Tool: ai.Tool{Name: "lookup", Parameters: json.RawMessage(`{"type":"object","properties":{"value":{"type":"string"}},"required":["value"]}`)}, PrepareArguments: func(v ai.JSONValue) (ai.JSONValue, error) {
				prepared.Add(1)
				if mode != "validation" {
					v.(map[string]any)["value"] = "prepared"
				}
				return v, nil
			}, DecodeValidated: func(v ai.JSONValue) map[string]any { return v.(map[string]any) }, Execute: func(ctx context.Context, _ string, p map[string]any, update agent.AgentToolUpdateCallback[any]) (agent.AgentToolResult[any], error) {
				if p["value"] != "prepared" {
					t.Errorf("prepare bypassed: %#v", p)
				}
				count := executed.Add(1)
				if mode == "parallel" {
					if prepared.Load() != 2 {
						t.Error("execution before all preflight")
					}
					if count == 2 {
						close(both)
					}
					select {
					case <-both:
					case <-ctx.Done():
						return agent.AgentToolResult[any]{}, ctx.Err()
					}
				}
				update(agent.AgentToolResult[any]{Content: []ai.ToolResultContent{ai.TextContent{Type: ai.ContentTypeText, Text: "progress"}}})
				if mode == "tool-failure" {
					return agent.AgentToolResult[any]{}, errors.New("lookup failed")
				}
				return agent.AgentToolResult[any]{Content: []ai.ToolResultContent{ai.TextContent{Type: ai.ContentTypeText, Text: "done"}}}, nil
			}})
			if err != nil {
				t.Fatal(err)
			}
			key := "offline"
			model := ai.Model{ID: "fixture", Provider: ai.ProviderIDDeepSeek, API: ai.APIOpenAIResponses, BaseURL: server.URL, MaxTokens: 512}
			execution := agent.ToolExecutionSequential
			if mode == "parallel" {
				execution = agent.ToolExecutionParallel
			}
			a, err = agent.NewAgent(agent.AgentOptions{ToolExecution: execution, InitialState: &agent.AgentInitialState{Model: model, Tools: []agent.ErasedAgentTool{tool}}, StreamFunction: func(ctx context.Context, m ai.Model, c ai.Context, o ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
				o.APIKey = &key
				return ai.StreamSimpleOpenAIResponses(ctx, m, c, o)
			}})
			if err != nil {
				t.Fatal(err)
			}
			a.Subscribe(func(_ context.Context, e agent.AgentEvent) error {
				switch event := e.(type) {
				case agent.ToolExecutionUpdateEvent:
					updates.Add(1)
				case agent.ToolExecutionEndEvent:
					ended.Add(1)
				case agent.MessageEndEvent:
					if _, ok := event.Message.(ai.AssistantMessage); ok && mode == "listener-failure" {
						return errors.New("listener failed")
					}
				}
				return nil
			})
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			err = a.Prompt(ctx, ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText("查询")})
			if mode == "listener-failure" {
				if err == nil || executed.Load() != 0 || requests.Load() != 1 {
					t.Fatalf("listener barrier err=%v effects=%d requests=%d", err, executed.Load(), requests.Load())
				}
				return
			}
			if err != nil || requests.Load() != 2 || prepared.Load() != 2 {
				t.Fatalf("err=%v requests=%d preflight=%d", err, requests.Load(), prepared.Load())
			}
		})
	}
}

func TestResponsesAgentCancelAfterToolAndResumePreservesSideEffects(t *testing.T) {
	var requests, effects atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		switch requests.Add(1) {
		case 1:
			io.WriteString(w, responsesFrame(map[string]any{"type": "response.output_item.done", "item": responsesToolItem("once", "{}")})+responsesFrame(map[string]any{"type": "response.completed", "response": map[string]any{"status": "completed"}}))
		case 2:
			io.WriteString(w, responsesFrame(map[string]any{"type": "response.output_text.delta", "delta": "partial"}))
			w.(http.Flusher).Flush()
			<-r.Context().Done()
		default:
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			calls, results := 0, 0
			for _, raw := range body["input"].([]any) {
				item := raw.(map[string]any)
				switch item["type"] {
				case "function_call":
					calls++
				case "function_call_output":
					results++
				}
			}
			if calls != 1 || results != 1 || effects.Load() != 1 {
				t.Errorf("resume calls=%d results=%d effects=%d", calls, results, effects.Load())
			}
			io.WriteString(w, responsesFrame(map[string]any{"type": "response.completed", "response": map[string]any{"status": "completed"}}))
		}
	}))
	defer server.Close()
	tool, err := agent.EraseAgentTool(agent.AgentTool[map[string]any, any]{Tool: ai.Tool{Name: "lookup", Parameters: json.RawMessage(`{"type":"object"}`)}, DecodeValidated: func(v ai.JSONValue) map[string]any { return v.(map[string]any) }, Execute: func(context.Context, string, map[string]any, agent.AgentToolUpdateCallback[any]) (agent.AgentToolResult[any], error) {
		effects.Add(1)
		return agent.AgentToolResult[any]{Content: []ai.ToolResultContent{ai.TextContent{Type: ai.ContentTypeText, Text: "once"}}}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	key := "offline"
	model := ai.Model{ID: "fixture", Provider: ai.ProviderIDDeepSeek, API: ai.APIOpenAIResponses, BaseURL: server.URL, MaxTokens: 512}
	a, err := agent.NewAgent(agent.AgentOptions{InitialState: &agent.AgentInitialState{Model: model, Tools: []agent.ErasedAgentTool{tool}}, StreamFunction: func(ctx context.Context, m ai.Model, c ai.Context, o ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
		o.APIKey = &key
		return ai.StreamSimpleOpenAIResponses(ctx, m, c, o)
	}})
	if err != nil {
		t.Fatal(err)
	}
	a.Subscribe(func(_ context.Context, e agent.AgentEvent) error {
		if update, ok := e.(agent.MessageUpdateEvent); ok {
			if _, ok := update.AssistantMessageEvent.(ai.AssistantMessageTextDeltaEvent); ok {
				a.Abort()
			}
		}
		return nil
	})
	_ = a.Prompt(context.Background(), ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText("查询")})
	if err := a.Prompt(context.Background(), ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText("继续")}); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 3 || effects.Load() != 1 {
		t.Fatalf("requests=%d effects=%d", requests.Load(), effects.Load())
	}
}

func TestResponsesAgentRejectsAlreadyExecutedCallID(t *testing.T) {
	var requests, effects atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, responsesFrame(map[string]any{"type": "response.output_item.done", "item": responsesToolItem("once", "{}")})+responsesFrame(map[string]any{"type": "response.completed", "response": map[string]any{"status": "completed"}}))
	}))
	defer server.Close()
	tool, err := agent.EraseAgentTool(agent.AgentTool[map[string]any, any]{Tool: ai.Tool{Name: "lookup", Parameters: json.RawMessage(`{"type":"object"}`)}, DecodeValidated: func(v ai.JSONValue) map[string]any { return v.(map[string]any) }, Execute: func(context.Context, string, map[string]any, agent.AgentToolUpdateCallback[any]) (agent.AgentToolResult[any], error) {
		effects.Add(1)
		return agent.AgentToolResult[any]{Content: []ai.ToolResultContent{ai.TextContent{Type: ai.ContentTypeText, Text: "once"}}}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	key := "offline"
	model := ai.Model{ID: "fixture", Provider: ai.ProviderIDDeepSeek, API: ai.APIOpenAIResponses, BaseURL: server.URL, MaxTokens: 512}
	a, err := agent.NewAgent(agent.AgentOptions{InitialState: &agent.AgentInitialState{Model: model, Tools: []agent.ErasedAgentTool{tool}}, StreamFunction: func(ctx context.Context, m ai.Model, c ai.Context, o ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
		o.APIKey = &key
		return ai.StreamSimpleOpenAIResponses(ctx, m, c, o)
	}})
	if err != nil {
		t.Fatal(err)
	}
	_ = a.Prompt(context.Background(), ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText("查询")})
	if requests.Load() != 2 || effects.Load() != 1 {
		t.Fatalf("requests=%d effects=%d; reused call_id must fail before repeating side effects", requests.Load(), effects.Load())
	}
	final := a.State().Messages[len(a.State().Messages)-1].(ai.AssistantMessage)
	if final.StopReason != ai.StopReasonError {
		t.Fatalf("final=%#v", final)
	}
}

func TestResponsesAgentIncompleteToolCreatesFailureWithoutExecuting(t *testing.T) {
	var requests, effects atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		if requests.Add(1) == 1 {
			item := responsesToolItem("partial", `{"value":"part`)
			item["status"] = "incomplete"
			io.WriteString(w, responsesFrame(map[string]any{"type": "response.output_item.added", "item": responsesToolItem("partial", "")})+responsesFrame(map[string]any{"type": "response.function_call_arguments.delta", "delta": `{"value":"part`})+responsesFrame(map[string]any{"type": "response.incomplete", "response": map[string]any{"status": "incomplete", "output": []any{item}, "incomplete_details": map[string]any{"reason": "max_output_tokens"}}}))
			return
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		results := 0
		for _, raw := range body["input"].([]any) {
			item := raw.(map[string]any)
			if item["type"] == "function_call_output" {
				results++
				if !strings.Contains(item["output"].(string), "truncated") {
					t.Errorf("partial failure=%#v", item)
				}
			}
		}
		if results != 1 || effects.Load() != 0 {
			t.Errorf("results=%d effects=%d", results, effects.Load())
		}
		io.WriteString(w, responsesFrame(map[string]any{"type": "response.completed", "response": map[string]any{"status": "completed"}}))
	}))
	defer server.Close()
	tool, err := agent.EraseAgentTool(agent.AgentTool[map[string]any, any]{Tool: ai.Tool{Name: "lookup", Parameters: json.RawMessage(`{"type":"object"}`)}, DecodeValidated: func(v ai.JSONValue) map[string]any { return v.(map[string]any) }, Execute: func(context.Context, string, map[string]any, agent.AgentToolUpdateCallback[any]) (agent.AgentToolResult[any], error) {
		effects.Add(1)
		return agent.AgentToolResult[any]{}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	key := "offline"
	model := ai.Model{ID: "fixture", Provider: ai.ProviderIDDeepSeek, API: ai.APIOpenAIResponses, BaseURL: server.URL, MaxTokens: 512}
	a, err := agent.NewAgent(agent.AgentOptions{InitialState: &agent.AgentInitialState{Model: model, Tools: []agent.ErasedAgentTool{tool}}, StreamFunction: func(ctx context.Context, m ai.Model, c ai.Context, o ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
		o.APIKey = &key
		return ai.StreamSimpleOpenAIResponses(ctx, m, c, o)
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Prompt(context.Background(), ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText("查询")}); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 2 || effects.Load() != 0 {
		t.Fatalf("requests=%d effects=%d", requests.Load(), effects.Load())
	}
	partial := a.State().Messages[1].(ai.AssistantMessage)
	raw, _ := partial.RawStopReason.Value()
	if partial.StopReason != ai.StopReasonLength || raw != "incomplete.max_output_tokens" {
		t.Fatalf("partial=%#v", partial)
	}
}

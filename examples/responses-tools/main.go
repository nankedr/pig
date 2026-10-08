package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
)

func main() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Input []map[string]any `json:"input"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "text/event-stream")
		for _, item := range body.Input {
			if item["type"] == "function_call_output" {
				io.WriteString(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"工具返回了 42。\"}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\n\n")
				return
			}
		}
		io.WriteString(w, "data: {\"type\":\"response.output_item.added\",\"item\":{\"type\":\"function_call\",\"id\":\"fc_lookup\",\"call_id\":\"lookup-1\",\"name\":\"lookup\",\"arguments\":\"\"}}\n\ndata: {\"type\":\"response.function_call_arguments.delta\",\"delta\":\"{}\"}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\n\n")
	}))
	defer server.Close()
	tool, err := agent.EraseAgentTool(agent.AgentTool[map[string]any, any]{Tool: ai.Tool{Name: "lookup", Description: "查询本地数值", Parameters: json.RawMessage(`{"type":"object"}`)}, DecodeValidated: func(v ai.JSONValue) map[string]any { return v.(map[string]any) }, Execute: func(context.Context, string, map[string]any, agent.AgentToolUpdateCallback[any]) (agent.AgentToolResult[any], error) {
		return agent.AgentToolResult[any]{Content: []ai.ToolResultContent{ai.TextContent{Type: ai.ContentTypeText, Text: "42"}}}, nil
	}})
	if err != nil {
		panic(err)
	}
	model := ai.Model{ID: "offline-model", Provider: ai.ProviderIDDeepSeek, API: ai.APIOpenAIResponses, BaseURL: server.URL, MaxTokens: 512, Input: []ai.ModelInput{ai.ModelInputText}}
	key := "offline-key"
	a, err := agent.NewAgent(agent.AgentOptions{InitialState: &agent.AgentInitialState{Model: model, Tools: []agent.ErasedAgentTool{tool}}, StreamFunction: func(ctx context.Context, m ai.Model, c ai.Context, o ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
		o.APIKey = &key
		return ai.StreamSimpleOpenAIResponses(ctx, m, c, o)
	}})
	if err != nil {
		panic(err)
	}
	if err := a.Prompt(context.Background(), ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText("查询本地数值")}); err != nil {
		panic(err)
	}
	messages := a.State().Messages
	out := messages[len(messages)-1].(ai.AssistantMessage)
	for _, block := range out.Content {
		if text, ok := block.(ai.TextContent); ok {
			fmt.Println(text.Text)
		}
	}
	fmt.Println("历史消息数:", len(messages), "stop:", out.StopReason)
}

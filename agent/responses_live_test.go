package agent_test

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
)

func TestResponsesAgentDeepSeekLiveToolContinuation(t *testing.T) {
	if os.Getenv("PIG_REQUIRE_RESPONSES_TOOLS_LIVE") != "1" {
		t.Skip("protected smoke requires PIG_REQUIRE_RESPONSES_TOOLS_LIVE=1")
	}
	key := strings.TrimSpace(os.Getenv("DEEPSEEK_API_KEY"))
	if key == "" {
		t.Fatal("protected Responses tool smoke requires DEEPSEEK_API_KEY")
	}
	id := os.Getenv("PIG_RESPONSES_SMOKE_MODEL")
	if id == "" {
		id = "deepseek-v4-pro"
	}
	var calls, requests, reasoningReplays atomic.Int32
	tool, err := agent.EraseAgentTool(agent.AgentTool[map[string]any, any]{Tool: ai.Tool{Name: "lookup", Description: "Return the requested local value", Parameters: json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)}, DecodeValidated: func(v ai.JSONValue) map[string]any { return v.(map[string]any) }, Execute: func(context.Context, string, map[string]any, agent.AgentToolUpdateCallback[any]) (agent.AgentToolResult[any], error) {
		calls.Add(1)
		return agent.AgentToolResult[any]{Content: []ai.ToolResultContent{ai.TextContent{Type: ai.ContentTypeText, Text: "local value: 132"}}}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	model := ai.Model{ID: id, Provider: ai.ProviderIDDeepSeek, API: ai.APIOpenAIResponses, BaseURL: "https://api.deepseek.com", Reasoning: true, MaxTokens: 4096, ContextWindow: 1000000, Input: []ai.ModelInput{ai.ModelInputText}}
	effort := ai.OpenAIReasoningEffortLow
	a, err := agent.NewAgent(agent.AgentOptions{InitialState: &agent.AgentInitialState{Model: model, Tools: []agent.ErasedAgentTool{tool}}, StreamFunction: func(ctx context.Context, m ai.Model, c ai.Context, o ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
		requests.Add(1)
		o.APIKey = &key
		o.OnPayload = func(_ context.Context, value ai.JSONValue, _ ai.Model) (ai.PayloadHookResult, error) {
			body := value.(map[string]any)
			for _, raw := range body["input"].([]any) {
				item := raw.(map[string]any)
				if item["type"] == "reasoning" {
					if item["encrypted_content"] != nil || item["summary"] != nil {
						t.Error("unsupported reasoning replay")
					}
					for _, raw := range item["content"].([]any) {
						if raw.(map[string]any)["text"] != "" {
							reasoningReplays.Add(1)
						}
					}
				}
			}
			return ai.PayloadHookResult{}, nil
		}
		return ai.StreamOpenAIResponses(ctx, m, c, ai.OpenAIResponsesOptions{StreamOptions: o.StreamOptions, ReasoningEffort: &effort})
	}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	if err := a.Prompt(ctx, ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText("Call lookup once, then briefly report the local value returned by the tool.")}); err != nil {
		t.Fatal("protected DeepSeek tool continuation did not complete")
	}
	messages := a.State().Messages
	final, ok := messages[len(messages)-1].(ai.AssistantMessage)
	text := ""
	for _, block := range final.Content {
		if c, ok := block.(ai.TextContent); ok {
			text += c.Text
		}
	}
	if !ok || final.StopReason != ai.StopReasonStop || strings.TrimSpace(text) == "" || calls.Load() != 1 || requests.Load() != 2 || reasoningReplays.Load() == 0 {
		message, _ := final.ErrorMessage.Value()
		t.Fatalf("protected DeepSeek tool smoke: stop=%s calls=%d requests=%d reasoningReplays=%d error=%s", final.StopReason, calls.Load(), requests.Load(), reasoningReplays.Load(), message)
	}
	t.Logf("PASS DeepSeek %s: one local tool execution, reasoning enabled, stateless continuation", id)
}

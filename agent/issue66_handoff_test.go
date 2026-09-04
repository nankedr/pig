package agent_test

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
)

func TestAgentModelHandoffReplaysHistoryWithoutMutatingIt(t *testing.T) {
	source := ai.Model{ID: "source", API: ai.APIOpenAICompletions, Provider: "source", BaseURL: "https://example.test/v1", MaxTokens: 1024}
	target := source
	target.ID, target.Provider = "target", ai.ProviderIDOpenAI
	history := ai.AssistantMessage{Role: ai.MessageRoleAssistant, API: source.API, Provider: source.Provider, Model: source.ID, StopReason: ai.StopReasonToolUse, Content: []ai.AssistantContent{
		ai.ThinkingContent{Type: ai.ContentTypeThinking, Thinking: "plan", ThinkingSignature: ai.Some("reasoning_content")},
		ai.ToolCall{Type: ai.ContentTypeToolCall, ID: "call|item", Name: "read", Arguments: map[string]any{"path": "README.md"}},
	}}
	var requests []map[string]any
	created, err := agent.NewAgent(agent.AgentOptions{
		InitialState: &agent.AgentInitialState{Model: source, Messages: []agent.AgentMessage{history}},
		StreamFunction: func(ctx context.Context, model ai.Model, input ai.Context, options ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
			key := "fixture-key"
			options.APIKey = &key
			options.Fetch = func(_ context.Context, request ai.FetchRequest) (ai.FetchResponse, error) {
				var body map[string]any
				if err := json.Unmarshal(request.Body, &body); err != nil {
					return ai.FetchResponse{}, err
				}
				requests = append(requests, body)
				return ai.FetchResponse{Status: 200, Body: []byte("data: {\"choices\":[{\"delta\":{\"content\":\"continued\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")}, nil
			}
			return ai.StreamSimpleOpenAICompletions(ctx, model, input, options)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, model := range []ai.Model{source, target, source} {
		created.SetModel(model)
		if err := created.PromptText(context.Background(), "continue"); err != nil {
			t.Fatal(err)
		}
		state := created.State()
		if state.ErrorMessage != nil || !reflect.DeepEqual(state.Messages[0], history) {
			t.Fatalf("handoff changed history or failed: %+v", state)
		}
	}
	if len(requests) != 3 {
		t.Fatalf("requests = %d, want 3", len(requests))
	}
	for i, request := range requests {
		messages := request["messages"].([]any)
		if messages[0].(map[string]any)["role"] == "system" {
			messages = messages[1:]
		}
		assistant := messages[0].(map[string]any)
		id := "call|item"
		if i == 1 {
			id = "call_item"
			if assistant["content"] != "plan" || assistant["reasoning_content"] != nil {
				t.Fatalf("cross-model reasoning = %#v", assistant)
			}
		} else if assistant["reasoning_content"] != "plan" {
			t.Fatalf("same-model reasoning = %#v", assistant)
		}
		if assistant["tool_calls"].([]any)[0].(map[string]any)["id"] != id {
			t.Fatalf("tool call = %#v", assistant)
		}
		result := messages[1].(map[string]any)
		if result["role"] != "tool" || result["tool_call_id"] != id || result["content"] != "No result provided" {
			t.Fatalf("missing repaired tool result: %#v", result)
		}
	}
}

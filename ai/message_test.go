package ai_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/nankedr/pig/ai"
)

func TestMessageCodecRoundTripsEveryRole(t *testing.T) {
	t.Parallel()

	usage := ai.Usage{
		Input: 1, Output: 2, CacheRead: 3, CacheWrite: 4,
		CacheWrite1H: ai.Some[int64](0), Reasoning: ai.Null[int64](), TotalTokens: 10,
		Cost: ai.UsageCost{Input: .1, Output: .2, CacheRead: .3, CacheWrite: .4, Total: 1},
	}
	tests := []struct {
		name    string
		message ai.Message
		want    ai.MessageRole
	}{
		{name: "user text", message: ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText("hello"), Timestamp: 1}, want: ai.MessageRoleUser},
		{name: "user blocks", message: ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserBlocks(ai.TextContent{Type: ai.ContentTypeText, Text: "hello"}, ai.ImageContent{Type: ai.ContentTypeImage, Data: "aGk=", MIMEType: "image/png"}), Timestamp: 2}, want: ai.MessageRoleUser},
		{name: "assistant", message: ai.AssistantMessage{Role: ai.MessageRoleAssistant, Content: []ai.AssistantContent{ai.ThinkingContent{Type: ai.ContentTypeThinking, Thinking: "hmm"}, ai.TextContent{Type: ai.ContentTypeText, Text: "answer"}}, API: ai.API("custom-api"), Provider: ai.ProviderID("custom-provider"), Model: "model-1", Usage: usage, StopReason: ai.StopReasonStop, EndTurn: ai.Some(false), Timestamp: 3}, want: ai.MessageRoleAssistant},
		{name: "tool result", message: ai.ToolResultMessage{Role: ai.MessageRoleToolResult, ToolCallID: "call-1", ToolName: "read", Content: []ai.ToolResultContent{ai.TextContent{Type: ai.ContentTypeText, Text: "done"}}, Details: ai.Null[ai.JSONValue](), Usage: ai.Some(usage), AddedToolNames: ai.Some([]string{}), IsError: false, Timestamp: 4}, want: ai.MessageRoleToolResult},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			encoded, err := ai.MarshalMessage(test.message)
			if err != nil {
				t.Fatalf("MarshalMessage() error = %v", err)
			}
			decoded, err := ai.UnmarshalMessage(encoded)
			if err != nil {
				t.Fatalf("UnmarshalMessage() error = %v", err)
			}
			if got := decoded.MessageRole(); got != test.want {
				t.Fatalf("MessageRole() = %q, want %q", got, test.want)
			}
			if !reflect.DeepEqual(decoded, test.message) {
				t.Fatalf("round trip = %#v, want %#v\nJSON: %s", decoded, test.message, encoded)
			}
		})
	}
}

func TestMessageCodecRejectsUnknownRoleAndInvalidRoleContent(t *testing.T) {
	t.Parallel()

	if _, err := ai.UnmarshalMessage([]byte(`{"role":"system","content":"x","timestamp":1}`)); !errors.Is(err, ai.ErrCodec) {
		t.Fatalf("unknown role error = %v, want ErrCodec", err)
	}

	_, err := ai.UnmarshalMessage([]byte(`{"role":"user","content":[{"type":"thinking","thinking":"private"}],"timestamp":1}`))
	if !errors.Is(err, ai.ErrCodec) {
		t.Fatalf("invalid user content error = %v, want ErrCodec", err)
	}
}

func TestCloneAssistantMessageDeepCopiesPointerContent(t *testing.T) {
	t.Parallel()

	arguments := map[string]any{"query": map[string]any{"term": "before"}}
	sourceCall := &ai.ToolCall{
		Type:      ai.ContentTypeToolCall,
		ID:        "call-1",
		Name:      "search",
		Arguments: arguments,
	}
	snapshot := ai.CloneAssistantMessage(ai.AssistantMessage{
		Role:    ai.MessageRoleAssistant,
		Content: []ai.AssistantContent{sourceCall},
	})

	arguments["query"].(map[string]any)["term"] = "after"
	sourceCall.Name = "mutated"

	cloned, ok := snapshot.Content[0].(*ai.ToolCall)
	if !ok {
		t.Fatalf("snapshot content type = %T, want *ai.ToolCall", snapshot.Content[0])
	}
	if cloned.Name != "search" {
		t.Fatalf("snapshot tool name = %q, want search", cloned.Name)
	}
	query := cloned.Arguments["query"].(map[string]any)
	if query["term"] != "before" {
		t.Fatalf("snapshot argument term = %v, want before", query["term"])
	}
}

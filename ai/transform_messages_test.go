package ai_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/nankedr/pig/ai"
)

func TestTransformMessagesPreservesOnlyReplayableSignatures(t *testing.T) {
	model := ai.Model{ID: "source", API: ai.APIOpenAIResponses, Provider: "source"}
	content := []ai.AssistantContent{
		ai.ThinkingContent{Type: ai.ContentTypeThinking, Thinking: "plan", ThinkingSignature: ai.Some("reasoning")},
		ai.ThinkingContent{Type: ai.ContentTypeThinking, ThinkingSignature: ai.Some("encrypted")},
		ai.ThinkingContent{Type: ai.ContentTypeThinking, Redacted: ai.Some(true), ThinkingSignature: ai.Some("opaque")},
		ai.ThinkingContent{Type: ai.ContentTypeThinking, Thinking: " \n"},
		ai.TextContent{Type: ai.ContentTypeText, Text: "answer", TextSignature: ai.Some("text-id")},
		ai.ToolCall{Type: ai.ContentTypeToolCall, ID: "call|item", Name: "read", Arguments: map[string]any{"path": "README.md"}, ThoughtSignature: ai.Some("tool-signature")},
	}
	message := ai.AssistantMessage{Role: ai.MessageRoleAssistant, API: model.API, Provider: model.Provider, Model: model.ID, Content: content, StopReason: ai.StopReasonToolUse}
	history := []ai.Message{message, ai.ToolResultMessage{Role: ai.MessageRoleToolResult, ToolCallID: "call|item", ToolName: "read"}}
	for _, field := range []string{"same", "model", "provider", "api"} {
		t.Run(field, func(t *testing.T) {
			target := model
			switch field {
			case "model":
				target.ID = "target"
			case "provider":
				target.Provider = "target"
			case "api":
				target.API = ai.APIOpenAICompletions
			}
			got, err := ai.TransformMessages(history, target)
			if err != nil {
				t.Fatal(err)
			}
			want := []ai.AssistantContent{content[0], content[1], content[2], content[4], content[5]}
			if field != "same" {
				want = []ai.AssistantContent{
					ai.TextContent{Type: ai.ContentTypeText, Text: "plan"},
					ai.TextContent{Type: ai.ContentTypeText, Text: "answer"},
					ai.ToolCall{Type: ai.ContentTypeToolCall, ID: "call|item", Name: "read", Arguments: map[string]any{"path": "README.md"}},
				}
			}
			if len(got) != 2 || !reflect.DeepEqual(got[0].(ai.AssistantMessage).Content, want) {
				t.Fatalf("replay = %#v, want content %#v", got, want)
			}
		})
	}
	if !message.Content[5].(ai.ToolCall).ThoughtSignature.IsSet() || !reflect.DeepEqual(message.Content, content) {
		t.Fatal("handoff mutated source history")
	}
}

func TestTransformMessagesKeepsImageDowngradeExplicitlyDeferred(t *testing.T) {
	image := ai.ImageContent{Type: ai.ContentTypeImage, Data: "aQ==", MIMEType: "image/png"}
	for _, message := range []ai.Message{
		ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserBlocks(image)},
		ai.ToolResultMessage{Role: ai.MessageRoleToolResult, Content: []ai.ToolResultContent{&image}},
	} {
		if _, err := ai.TransformMessages([]ai.Message{message}, ai.Model{Input: []ai.ModelInput{ai.ModelInputText}}); !errors.Is(err, ai.ErrNotImplemented) {
			t.Fatalf("image downgrade error = %v, want ErrNotImplemented", err)
		}
		if got, err := ai.TransformMessages([]ai.Message{message}, ai.Model{Input: []ai.ModelInput{ai.ModelInputImage}}); err != nil || len(got) != 1 {
			t.Fatalf("vision passthrough = %#v, %v", got, err)
		}
	}
}

func TestTransformMessagesRepairsOrphansWithMappedIDs(t *testing.T) {
	model := ai.Model{ID: "target", API: ai.APIAnthropicMessages, Provider: "github-copilot"}
	for _, boundary := range []string{"end", "user", "assistant", "error", "aborted"} {
		t.Run(boundary, func(t *testing.T) {
			history := []ai.Message{
				ai.AssistantMessage{Role: ai.MessageRoleAssistant, Model: "source", API: ai.APIOpenAIResponses, Provider: model.Provider, StopReason: ai.StopReasonToolUse, Content: []ai.AssistantContent{
					ai.ToolCall{Type: ai.ContentTypeToolCall, ID: "call|done", Name: "read", Arguments: map[string]any{}},
					ai.ToolCall{Type: ai.ContentTypeToolCall, ID: "call|missing", Name: "write", Arguments: map[string]any{}},
				}},
				ai.ToolResultMessage{Role: ai.MessageRoleToolResult, ToolCallID: "call|done", ToolName: "read"},
			}
			switch boundary {
			case "user":
				history = append(history, ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText("continue")})
			case "assistant", "error", "aborted":
				reason := ai.StopReasonStop
				if boundary != "assistant" {
					reason = ai.StopReason(boundary)
				}
				history = append(history, ai.AssistantMessage{Role: ai.MessageRoleAssistant, StopReason: reason, Content: []ai.AssistantContent{
					ai.TextContent{Type: ai.ContentTypeText, Text: "next"},
				}})
			}
			before := time.Now().UnixMilli()
			got, err := ai.TransformMessages(history, model, func(id string, target ai.Model, source ai.AssistantMessage) string {
				if target.ID != model.ID || source.Model != "source" {
					t.Fatalf("normalizer metadata = %s, %s", target.ID, source.Model)
				}
				return strings.ReplaceAll(id, "|", "_")
			})
			if err != nil {
				t.Fatal(err)
			}
			wantLen := 3
			if boundary == "user" || boundary == "assistant" {
				wantLen++
			}
			if len(got) != wantLen {
				t.Fatalf("replay length = %d, want %d: %#v", len(got), wantLen, got)
			}
			calls := got[0].(ai.AssistantMessage).Content
			if calls[0].(ai.ToolCall).ID != "call_done" || calls[1].(ai.ToolCall).ID != "call_missing" || got[1].(ai.ToolResultMessage).ToolCallID != "call_done" {
				t.Fatalf("ID mapping = %#v", got)
			}
			result := got[2].(ai.ToolResultMessage)
			if !result.IsError || result.ToolCallID != "call_missing" || result.ToolName != "write" || result.Content[0].(ai.TextContent).Text != "No result provided" || result.Timestamp < before || result.Timestamp > time.Now().UnixMilli() {
				t.Fatalf("synthetic result = %+v", result)
			}
		})
	}
}

func TestTransformMessagesOwnsReplaySnapshots(t *testing.T) {
	call := &ai.ToolCall{Type: ai.ContentTypeToolCall, ID: "call|item", Name: "read", Arguments: map[string]any{"nested": map[string]any{"path": "original"}}}
	text := &ai.TextContent{Type: ai.ContentTypeText, Text: "original"}
	source := &ai.AssistantMessage{Role: ai.MessageRoleAssistant, Model: "source", Content: []ai.AssistantContent{call}}
	toolResult := &ai.ToolResultMessage{Role: ai.MessageRoleToolResult, ToolCallID: call.ID, Content: []ai.ToolResultContent{text}, Details: ai.Some[any](map[string]any{"value": "original"})}
	model := ai.Model{ID: "target", Headers: map[string]string{"x-test": "original"}}
	got, err := ai.TransformMessages([]ai.Message{&ai.UserMessage{Role: ai.MessageRoleUser}, source, toolResult}, model, func(id string, target ai.Model, source ai.AssistantMessage) string {
		source.Content[0].(*ai.ToolCall).Arguments["nested"].(map[string]any)["path"] = "callback"
		target.Headers["x-test"] = "callback"
		return "call_item"
	})
	if err != nil {
		t.Fatal(err)
	}
	user := got[0].(ai.UserMessage)
	if blocks, _ := user.Content.Blocks(); blocks == nil || len(blocks) != 0 {
		t.Fatalf("nil user content was not normalized: %#v", blocks)
	}
	replayed := got[1].(ai.AssistantMessage).Content[0].(ai.ToolCall)
	if replayed.Arguments["nested"].(map[string]any)["path"] != "original" {
		t.Fatal("normalizer mutated replay content")
	}
	replayed.Arguments["nested"].(map[string]any)["path"] = "replay"
	details, _ := got[2].(ai.ToolResultMessage).Details.Value()
	details.(map[string]any)["value"] = "replay"
	originalDetails, _ := toolResult.Details.Value()
	if call.Arguments["nested"].(map[string]any)["path"] != "original" || originalDetails.(map[string]any)["value"] != "original" || model.Headers["x-test"] != "original" || call.ID != "call|item" {
		t.Fatal("replay or normalizer changed caller-owned history/model")
	}
}

func TestIssue66LockedGoAPISnapshot(t *testing.T) {
	want := "func([]ai.Message, ai.Model, ...ai.ToolCallIDNormalizer) ([]ai.Message, error)"
	if got := reflect.TypeOf(ai.TransformMessages).String(); got != want {
		t.Fatalf("TransformMessages API = %s, want %s", got, want)
	}
	want = "func(string, ai.Model, ai.AssistantMessage) string"
	if got := issue60FuncSignature(reflect.TypeOf((*ai.ToolCallIDNormalizer)(nil)).Elem()); got != want {
		t.Fatalf("ToolCallIDNormalizer API = %s, want %s", got, want)
	}
}

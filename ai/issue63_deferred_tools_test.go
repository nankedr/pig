package ai_test

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/nankedr/pig/ai"
)

func TestIssue63LockedGoAPISnapshot(t *testing.T) {
	want, err := os.ReadFile("testdata/issue63_surface_golden.txt")
	if err != nil {
		t.Fatal(err)
	}
	got := "func ai.SplitDeferredTools " + issue60FuncSignature(reflect.TypeOf(ai.SplitDeferredTools)) + "\n"
	if got != string(want) {
		t.Fatalf("deferred tools API changed: %s; want %s", got, want)
	}
}

func TestSplitDeferredToolsPointerTranscript(t *testing.T) {
	input := ai.Context{
		Tools: []ai.Tool{
			{Name: "read", Description: "old", Parameters: json.RawMessage(`{"type":"object"}`)},
			{Name: "write", Parameters: json.RawMessage(`{"type":"object"}`)},
			{Name: "Read", Description: "current", Parameters: json.RawMessage(`{"type":"object","properties":{}}`)},
		},
		Messages: []ai.Message{
			&ai.ToolResultMessage{Role: ai.MessageRoleToolResult, ToolCallID: "discover-1", ToolName: "discover", Content: []ai.ToolResultContent{}, AddedToolNames: ai.Some([]string{"READ", "write", "write", "missing"})},
			&ai.AssistantMessage{Role: ai.MessageRoleAssistant, Content: []ai.AssistantContent{&ai.ToolCall{Type: ai.ContentTypeToolCall, ID: "read-1", Name: "READ", Arguments: map[string]any{}}}, API: "faux", Provider: "faux", Model: "faux-1", StopReason: ai.StopReasonToolUse},
			&ai.ToolResultMessage{Role: ai.MessageRoleToolResult, ToolCallID: "read-1", ToolName: "READ", Content: []ai.ToolResultContent{}, AddedToolNames: ai.Null[[]string]()},
		},
	}
	before, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	immediate, deferred := ai.SplitDeferredTools(input, true, strings.ToLower)
	if !reflect.DeepEqual(immediate, []ai.Tool{input.Tools[2]}) || !reflect.DeepEqual(deferred, []ai.Tool{input.Tools[1]}) {
		t.Fatalf("pointer transcript partition = %#v / %#v", immediate, deferred)
	}
	after, err := json.Marshal(input)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("partition rewrote input: %s, %v", after, err)
	}
}

func TestSplitDeferredToolsIgnoresNilVariants(t *testing.T) {
	tool := ai.Tool{Name: "read"}
	immediate, deferred := ai.SplitDeferredTools(ai.Context{
		Tools: []ai.Tool{tool},
		Messages: []ai.Message{nil, (*ai.AssistantMessage)(nil), (*ai.ToolResultMessage)(nil),
			ai.ToolResultMessage{AddedToolNames: ai.Some([]string{"read"})},
			ai.AssistantMessage{Content: []ai.AssistantContent{nil, (*ai.ToolCall)(nil)}},
		},
	}, true, nil)
	if len(immediate) != 0 || !reflect.DeepEqual(deferred, []ai.Tool{tool}) {
		t.Fatalf("nil variants changed partition: %#v / %#v", immediate, deferred)
	}
}

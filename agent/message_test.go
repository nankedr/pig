package agent_test

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
)

func TestAgentMessageDecodesBuiltinsAndPreservesUnknownMessages(t *testing.T) {
	t.Parallel()

	builtin, err := agent.UnmarshalAgentMessage([]byte(`{"role":"user","content":"hello","timestamp":7}`))
	if err != nil {
		t.Fatalf("UnmarshalAgentMessage(builtin) error = %v", err)
	}
	if _, ok := builtin.(ai.UserMessage); !ok {
		t.Fatalf("builtin type = %T, want ai.UserMessage", builtin)
	}

	rawJSON := []byte(`{ "role" : "notification", "count" : 0, "enabled" : false }`)
	custom, err := agent.UnmarshalAgentMessage(rawJSON)
	if err != nil {
		t.Fatalf("UnmarshalAgentMessage(custom) error = %v", err)
	}
	raw, ok := custom.(agent.RawAgentMessage)
	if !ok {
		t.Fatalf("custom type = %T, want agent.RawAgentMessage", custom)
	}
	if raw.MessageRole() != "notification" {
		t.Fatalf("MessageRole() = %q, want notification", raw.MessageRole())
	}
	encoded, err := agent.MarshalAgentMessage(raw)
	if err != nil {
		t.Fatalf("MarshalAgentMessage(custom) error = %v", err)
	}
	if !bytes.Equal(encoded, rawJSON) {
		t.Fatalf("custom round trip = %s, want exact bytes %s", encoded, rawJSON)
	}

	copyOfRaw := raw.RawJSON()
	copyOfRaw[0] = '['
	reencoded, err := agent.MarshalAgentMessage(raw)
	if err != nil {
		t.Fatalf("MarshalAgentMessage(after mutation) error = %v", err)
	}
	if !bytes.Equal(reencoded, rawJSON) {
		t.Fatalf("RawJSON exposed mutable storage: got %s", reencoded)
	}
}

func TestAgentMessageRejectsInvalidOpenBoundaryValues(t *testing.T) {
	t.Parallel()

	for _, input := range []string{``, `null`, `[]`, `{}`, `{"role":null}`, `{"role":""}`} {
		if _, err := agent.UnmarshalAgentMessage([]byte(input)); err == nil {
			t.Errorf("UnmarshalAgentMessage(%q) error = nil", input)
		}
	}

	if _, err := agent.NewRawAgentMessage(json.RawMessage(`{"role":"assistant","future":true}`)); err == nil {
		t.Fatal("NewRawAgentMessage accepted a closed built-in role")
	}
}

func TestDefaultConvertToLLMFiltersCustomMessagesWithoutTransformingContext(t *testing.T) {
	t.Parallel()

	custom, err := agent.NewRawAgentMessage(json.RawMessage(`{"role":"notification","text":"hidden"}`))
	if err != nil {
		t.Fatalf("NewRawAgentMessage error = %v", err)
	}
	user := ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText("visible"), Timestamp: 3}
	messages := []agent.AgentMessage{custom, user}

	converted, err := agent.DefaultConvertToLLM(context.Background(), messages)
	if err != nil {
		t.Fatalf("DefaultConvertToLLM error = %v", err)
	}
	if !reflect.DeepEqual(converted, []ai.Message{user}) {
		t.Fatalf("converted = %#v, want only builtin user message", converted)
	}
	if len(messages) != 2 {
		t.Fatalf("input mutated: len = %d, want 2", len(messages))
	}
}

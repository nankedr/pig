package ai_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/nankedr/pig/ai"
)

func TestContextCodecRoundTripsClosedMessages(t *testing.T) {
	t.Parallel()

	contextValue := ai.Context{
		SystemPrompt: ai.Some("Be concise"),
		Messages: []ai.Message{
			ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText("hello"), Timestamp: 1},
			ai.AssistantMessage{
				Role:       ai.MessageRoleAssistant,
				Content:    []ai.AssistantContent{ai.TextContent{Type: ai.ContentTypeText, Text: "hi"}},
				API:        ai.APIOpenAIResponses,
				Provider:   ai.ProviderIDOpenAI,
				Model:      "model-1",
				Usage:      ai.Usage{},
				StopReason: ai.StopReasonStop,
				Timestamp:  2,
			},
		},
		Tools: []ai.Tool{{Name: "search", Description: "Search", Parameters: []byte("{\"type\":\"object\"}")}},
	}

	encoded, err := ai.MarshalContext(contextValue)
	if err != nil {
		t.Fatalf("MarshalContext() error = %v", err)
	}
	decoded, err := ai.UnmarshalContext(encoded)
	if err != nil {
		t.Fatalf("UnmarshalContext() error = %v", err)
	}
	if !reflect.DeepEqual(decoded, contextValue) {
		t.Fatalf("Context round trip = %#v, want %#v\nJSON: %s", decoded, contextValue, encoded)
	}
}

func TestContextCodecRejectsMissingOrUnknownMessages(t *testing.T) {
	t.Parallel()

	for _, input := range []string{
		"{}",
		"{\"messages\":null}",
		"{\"messages\":[{\"role\":\"future\"}]}",
	} {
		if _, err := ai.UnmarshalContext([]byte(input)); !errors.Is(err, ai.ErrCodec) {
			t.Errorf("UnmarshalContext(%s) error = %v, want ErrCodec", input, err)
		}
	}
}

package ai_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/nankedr/pig/ai"
)

func TestAssistantMessageEventCodecRoundTripsClosedVariants(t *testing.T) {
	t.Parallel()

	partial := eventCodecAssistantMessage("partial")
	final := eventCodecAssistantMessage("final")
	tests := []struct {
		name  string
		event ai.AssistantMessageEvent
	}{
		{name: "start", event: ai.AssistantMessageStartEvent{Type: ai.AssistantMessageEventTypeStart, Partial: partial}},
		{name: "text start", event: ai.AssistantMessageTextStartEvent{Type: ai.AssistantMessageEventTypeTextStart, ContentIndex: 1, Partial: partial}},
		{name: "text delta", event: ai.AssistantMessageTextDeltaEvent{Type: ai.AssistantMessageEventTypeTextDelta, ContentIndex: 1, Delta: "chunk", Partial: partial}},
		{name: "text end", event: ai.AssistantMessageTextEndEvent{Type: ai.AssistantMessageEventTypeTextEnd, ContentIndex: 1, Content: "complete", Partial: partial}},
		{name: "thinking start", event: ai.AssistantMessageThinkingStartEvent{Type: ai.AssistantMessageEventTypeThinkingStart, ContentIndex: 2, Partial: partial}},
		{name: "thinking delta", event: ai.AssistantMessageThinkingDeltaEvent{Type: ai.AssistantMessageEventTypeThinkingDelta, ContentIndex: 2, Delta: "hmm", Partial: partial}},
		{name: "thinking end", event: ai.AssistantMessageThinkingEndEvent{Type: ai.AssistantMessageEventTypeThinkingEnd, ContentIndex: 2, Content: "reasoned", Partial: partial}},
		{name: "tool call start", event: ai.AssistantMessageToolCallStartEvent{Type: ai.AssistantMessageEventTypeToolCallStart, ContentIndex: 3, Partial: partial}},
		{name: "tool call delta", event: ai.AssistantMessageToolCallDeltaEvent{Type: ai.AssistantMessageEventTypeToolCallDelta, ContentIndex: 3, Delta: `{"query":`, Partial: partial}},
		{name: "tool call end", event: ai.AssistantMessageToolCallEndEvent{Type: ai.AssistantMessageEventTypeToolCallEnd, ContentIndex: 3, ToolCall: ai.ToolCall{Type: ai.ContentTypeToolCall, ID: "call-1", Name: "search", Arguments: map[string]any{"query": "pig"}}, Partial: partial}},
		{name: "done", event: ai.AssistantMessageDoneEvent{Type: ai.AssistantMessageEventTypeDone, Reason: ai.StopReasonToolUse, Message: final}},
		{name: "error", event: ai.AssistantMessageErrorEvent{Type: ai.AssistantMessageEventTypeError, Reason: ai.StopReasonError, Error: final}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			encoded, err := ai.MarshalAssistantMessageEvent(test.event)
			if err != nil {
				t.Fatalf("MarshalAssistantMessageEvent() error = %v", err)
			}
			decoded, err := ai.UnmarshalAssistantMessageEvent(encoded)
			if err != nil {
				t.Fatalf("UnmarshalAssistantMessageEvent() error = %v", err)
			}
			if !reflect.DeepEqual(decoded, test.event) {
				t.Fatalf("event round trip = %#v, want %#v", decoded, test.event)
			}
		})
	}
}

func TestAssistantMessageEventCodecRejectsUnknownMismatchedAndInvalidTerminalValues(t *testing.T) {
	t.Parallel()

	partial := eventCodecAssistantMessage("partial")
	invalidMarshal := []ai.AssistantMessageEvent{
		ai.AssistantMessageTextDeltaEvent{Type: ai.AssistantMessageEventTypeTextEnd, Partial: partial},
		ai.AssistantMessageDoneEvent{Type: ai.AssistantMessageEventTypeDone, Reason: ai.StopReasonError, Message: partial},
		ai.AssistantMessageErrorEvent{Type: ai.AssistantMessageEventTypeError, Reason: ai.StopReasonStop, Error: partial},
	}
	for _, event := range invalidMarshal {
		if _, err := ai.MarshalAssistantMessageEvent(event); !errors.Is(err, ai.ErrCodec) {
			t.Errorf("MarshalAssistantMessageEvent(%T) error = %v, want ErrCodec", event, err)
		}
	}

	invalidJSON := []string{
		`{"type":"future_event"}`,
		`{"type":"done","reason":"error","message":{}}`,
		`{"type":"error","reason":"stop","error":{}}`,
		`{"type":"toolcall_end","contentIndex":0,"toolCall":{"type":"text","text":"wrong"},"partial":{}}`,
	}
	for _, input := range invalidJSON {
		if _, err := ai.UnmarshalAssistantMessageEvent([]byte(input)); !errors.Is(err, ai.ErrCodec) {
			t.Errorf("UnmarshalAssistantMessageEvent(%s) error = %v, want ErrCodec", input, err)
		}
	}
}

func eventCodecAssistantMessage(model string) ai.AssistantMessage {
	return ai.AssistantMessage{
		Role:       ai.MessageRoleAssistant,
		Model:      model,
		Content:    []ai.AssistantContent{ai.TextContent{Type: ai.ContentTypeText, Text: model}},
		Usage:      ai.Usage{},
		StopReason: ai.StopReasonPending,
	}
}

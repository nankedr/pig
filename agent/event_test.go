package agent_test

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
)

func TestAgentEventCodecRoundTripsEveryClosedVariant(t *testing.T) {
	t.Parallel()

	custom, err := agent.NewRawAgentMessage(json.RawMessage(`{"role":"notification","text":"queued"}`))
	if err != nil {
		t.Fatal(err)
	}
	assistant := ai.AssistantMessage{
		Role: ai.MessageRoleAssistant, API: ai.API("test"), Provider: ai.ProviderID("test"), Model: "test",
		Content:    []ai.AssistantContent{ai.TextContent{Type: ai.ContentTypeText, Text: "working"}},
		StopReason: ai.StopReasonPending, Timestamp: 9,
	}
	toolResult := agent.ErasedAgentToolResult{
		Content: []ai.ToolResultContent{ai.TextContent{Type: ai.ContentTypeText, Text: "one"}},
		Details: map[string]any{"count": float64(1)}, Terminate: ai.Some(false),
	}
	tests := []struct {
		name         string
		event        agent.AgentEvent
		wantType     agent.AgentEventType
		wantConcrete any
	}{
		{"agent start", agent.AgentStartEvent{Type: agent.AgentEventTypeAgentStart}, agent.AgentEventTypeAgentStart, agent.AgentStartEvent{}},
		{"agent end", agent.AgentEndEvent{Type: agent.AgentEventTypeAgentEnd, Messages: []agent.AgentMessage{custom}}, agent.AgentEventTypeAgentEnd, agent.AgentEndEvent{}},
		{"turn start", agent.TurnStartEvent{Type: agent.AgentEventTypeTurnStart}, agent.AgentEventTypeTurnStart, agent.TurnStartEvent{}},
		{"turn end", agent.TurnEndEvent{Type: agent.AgentEventTypeTurnEnd, Message: assistant, ToolResults: []ai.ToolResultMessage{}}, agent.AgentEventTypeTurnEnd, agent.TurnEndEvent{}},
		{"message start", agent.MessageStartEvent{Type: agent.AgentEventTypeMessageStart, Message: custom}, agent.AgentEventTypeMessageStart, agent.MessageStartEvent{}},
		{"message update", agent.MessageUpdateEvent{Type: agent.AgentEventTypeMessageUpdate, Message: assistant, AssistantMessageEvent: ai.AssistantMessageTextDeltaEvent{Type: ai.AssistantMessageEventTypeTextDelta, ContentIndex: 0, Delta: "x", Partial: assistant}}, agent.AgentEventTypeMessageUpdate, agent.MessageUpdateEvent{}},
		{"message end", agent.MessageEndEvent{Type: agent.AgentEventTypeMessageEnd, Message: custom}, agent.AgentEventTypeMessageEnd, agent.MessageEndEvent{}},
		{"tool start", agent.ToolExecutionStartEvent{Type: agent.AgentEventTypeToolExecutionStart, ToolCallID: "call-1", ToolName: "search", Arguments: map[string]any{"q": "pig"}}, agent.AgentEventTypeToolExecutionStart, agent.ToolExecutionStartEvent{}},
		{"tool update", agent.ToolExecutionUpdateEvent{Type: agent.AgentEventTypeToolExecutionUpdate, ToolCallID: "call-1", ToolName: "search", Arguments: map[string]any{"q": "pig"}, PartialResult: toolResult}, agent.AgentEventTypeToolExecutionUpdate, agent.ToolExecutionUpdateEvent{}},
		{"tool end", agent.ToolExecutionEndEvent{Type: agent.AgentEventTypeToolExecutionEnd, ToolCallID: "call-1", ToolName: "search", Result: toolResult, IsError: false}, agent.AgentEventTypeToolExecutionEnd, agent.ToolExecutionEndEvent{}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			encoded, err := agent.MarshalAgentEvent(test.event)
			if err != nil {
				t.Fatalf("MarshalAgentEvent error = %v", err)
			}
			if !bytes.Contains(encoded, []byte(`"type":"`+string(test.wantType)+`"`)) {
				t.Fatalf("encoded = %s, want type %q", encoded, test.wantType)
			}
			decoded, err := agent.UnmarshalAgentEvent(encoded)
			if err != nil {
				t.Fatalf("UnmarshalAgentEvent error = %v", err)
			}
			if decoded.AgentEventType() != test.wantType || reflect.TypeOf(decoded) != reflect.TypeOf(test.wantConcrete) {
				t.Fatalf("decoded = %T/%q, want %T/%q", decoded, decoded.AgentEventType(), test.wantConcrete, test.wantType)
			}
		})
	}
}

func TestAgentEventCodecRejectsUnknownMismatchedAndTerminalUpdateEvents(t *testing.T) {
	t.Parallel()

	if _, err := agent.UnmarshalAgentEvent([]byte(`{"type":"future"}`)); err == nil {
		t.Fatal("UnmarshalAgentEvent accepted unknown type")
	}
	if _, err := agent.MarshalAgentEvent(agent.AgentStartEvent{Type: agent.AgentEventTypeTurnStart}); err == nil {
		t.Fatal("MarshalAgentEvent accepted mismatched concrete type and discriminator")
	}
	assistant := ai.AssistantMessage{Role: ai.MessageRoleAssistant, API: "test", Provider: "test", Model: "test", StopReason: ai.StopReasonStop, Timestamp: 1}
	terminal := agent.MessageUpdateEvent{
		Type: agent.AgentEventTypeMessageUpdate, Message: assistant,
		AssistantMessageEvent: ai.AssistantMessageDoneEvent{Type: ai.AssistantMessageEventTypeDone, Reason: ai.StopReasonStop, Message: assistant},
	}
	if _, err := agent.MarshalAgentEvent(terminal); err == nil {
		t.Fatal("MarshalAgentEvent accepted terminal Assistant event as message_update")
	}
}

func TestUnmarshalAgentEventRejectsUnknownFieldsThroughoutClosedWireShape(t *testing.T) {
	t.Parallel()

	t.Run("event envelope", func(t *testing.T) {
		data := []byte(`{"type":"agent_start","future":true}`)
		if _, err := agent.UnmarshalAgentEvent(data); err == nil {
			t.Fatalf("UnmarshalAgentEvent(%s) accepted unknown event field", data)
		}
	})

	t.Run("Assistant message event", func(t *testing.T) {
		data := []byte(`{
			"type":"message_update",
			"message":{"role":"assistant","content":[],"api":"test","provider":"test","model":"test","usage":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0,"totalTokens":0,"cost":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0,"total":0}},"stopReason":"pending","timestamp":1},
			"assistantMessageEvent":{"type":"text_delta","contentIndex":0,"delta":"x","partial":{"role":"assistant","content":[],"api":"test","provider":"test","model":"test","usage":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0,"totalTokens":0,"cost":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0,"total":0}},"stopReason":"pending","timestamp":1},"future":true}
		}`)
		if _, err := agent.UnmarshalAgentEvent(data); err == nil {
			t.Fatalf("UnmarshalAgentEvent(%s) accepted unknown Assistant event field", data)
		}
	})

	t.Run("built-in Agent message", func(t *testing.T) {
		data := []byte(`{
			"type":"message_start",
			"message":{"role":"assistant","content":[],"api":"test","provider":"test","model":"test","usage":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0,"totalTokens":0,"cost":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0,"total":0}},"stopReason":"pending","timestamp":1,"future":true}
		}`)
		if _, err := agent.UnmarshalAgentEvent(data); err == nil {
			t.Fatalf("UnmarshalAgentEvent(%s) accepted unknown built-in Agent message field", data)
		}
	})

	t.Run("Assistant event partial message", func(t *testing.T) {
		data := []byte(`{
			"type":"message_update",
			"message":{"role":"assistant","content":[],"api":"test","provider":"test","model":"test","usage":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0,"totalTokens":0,"cost":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0,"total":0}},"stopReason":"pending","timestamp":1},
			"assistantMessageEvent":{"type":"text_delta","contentIndex":0,"delta":"x","partial":{"role":"assistant","content":[],"api":"test","provider":"test","model":"test","usage":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0,"totalTokens":0,"cost":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0,"total":0}},"stopReason":"pending","timestamp":1,"future":true}}
		}`)
		if _, err := agent.UnmarshalAgentEvent(data); err == nil {
			t.Fatalf("UnmarshalAgentEvent(%s) accepted unknown Assistant partial message field", data)
		}
	})

	t.Run("Tool execution result", func(t *testing.T) {
		data := []byte(`{
			"type":"tool_execution_end",
			"toolCallId":"call-1",
			"toolName":"search",
			"result":{"content":[],"details":null,"future":true},
			"isError":false
		}`)
		if _, err := agent.UnmarshalAgentEvent(data); err == nil {
			t.Fatalf("UnmarshalAgentEvent(%s) accepted unknown Tool result field", data)
		}
	})

	t.Run("Tool result content", func(t *testing.T) {
		data := []byte(`{
			"type":"tool_execution_end",
			"toolCallId":"call-1",
			"toolName":"search",
			"result":{"content":[{"type":"text","text":"ok","future":true}],"details":null},
			"isError":false
		}`)
		if _, err := agent.UnmarshalAgentEvent(data); err == nil {
			t.Fatalf("UnmarshalAgentEvent(%s) accepted unknown Tool-result content field", data)
		}
	})

	t.Run("turn Tool-result message", func(t *testing.T) {
		data := []byte(`{
			"type":"turn_end",
			"message":{"role":"notification","text":"done"},
			"toolResults":[{"role":"toolResult","toolCallId":"call-1","toolName":"search","content":[],"isError":false,"timestamp":1,"future":true}]
		}`)
		if _, err := agent.UnmarshalAgentEvent(data); err == nil {
			t.Fatalf("UnmarshalAgentEvent(%s) accepted unknown Tool-result message field", data)
		}
	})

	t.Run("Assistant event Tool call", func(t *testing.T) {
		data := []byte(`{
			"type":"message_update",
			"message":{"role":"assistant","content":[],"api":"test","provider":"test","model":"test","usage":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0,"totalTokens":0,"cost":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0,"total":0}},"stopReason":"pending","timestamp":1},
			"assistantMessageEvent":{"type":"toolcall_end","contentIndex":0,"toolCall":{"type":"toolCall","id":"call-1","name":"search","arguments":{},"future":true},"partial":{"role":"assistant","content":[],"api":"test","provider":"test","model":"test","usage":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0,"totalTokens":0,"cost":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0,"total":0}},"stopReason":"pending","timestamp":1}}
		}`)
		if _, err := agent.UnmarshalAgentEvent(data); err == nil {
			t.Fatalf("UnmarshalAgentEvent(%s) accepted unknown Assistant Tool-call field", data)
		}
	})

	fixedRecords := []struct {
		name string
		data string
	}{
		{
			name: "usage",
			data: `{"type":"message_start","message":{"role":"assistant","content":[],"api":"test","provider":"test","model":"test","usage":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0,"totalTokens":0,"cost":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0,"total":0},"future":true},"stopReason":"pending","timestamp":1}}`,
		},
		{
			name: "usage cost",
			data: `{"type":"message_start","message":{"role":"assistant","content":[],"api":"test","provider":"test","model":"test","usage":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0,"totalTokens":0,"cost":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0,"total":0,"future":true}},"stopReason":"pending","timestamp":1}}`,
		},
		{
			name: "diagnostic",
			data: `{"type":"message_start","message":{"role":"assistant","content":[],"api":"test","provider":"test","model":"test","diagnostics":[{"type":"provider","timestamp":1,"future":true}],"usage":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0,"totalTokens":0,"cost":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0,"total":0}},"stopReason":"pending","timestamp":1}}`,
		},
		{
			name: "diagnostic error",
			data: `{"type":"message_start","message":{"role":"assistant","content":[],"api":"test","provider":"test","model":"test","diagnostics":[{"type":"provider","timestamp":1,"error":{"message":"bad","future":true}}],"usage":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0,"totalTokens":0,"cost":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0,"total":0}},"stopReason":"pending","timestamp":1}}`,
		},
		{
			name: "deferred handle",
			data: `{"type":"message_start","message":{"role":"assistant","content":[],"api":"test","provider":"test","model":"test","usage":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0,"totalTokens":0,"cost":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0,"total":0}},"stopReason":"deferred","deferred":{"provider":"test","modelId":"test","api":"test","id":"job-1","future":true},"timestamp":1}}`,
		},
		{
			name: "Tool execution result usage",
			data: `{"type":"tool_execution_end","toolCallId":"call-1","toolName":"search","result":{"content":[],"details":null,"usage":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0,"totalTokens":0,"cost":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0,"total":0},"future":true}},"isError":false}`,
		},
	}
	for _, test := range fixedRecords {
		t.Run(test.name, func(t *testing.T) {
			if _, err := agent.UnmarshalAgentEvent([]byte(test.data)); err == nil {
				t.Fatalf("UnmarshalAgentEvent(%s) accepted unknown %s field", test.data, test.name)
			}
		})
	}
}

func TestUnmarshalAgentEventRejectsMissingRequiredFieldsInNestedClosedWireShape(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		data string
	}{
		{
			name: "assistant usage input",
			data: `{"type":"message_start","message":{"role":"assistant","content":[],"api":"test","provider":"test","model":"test","usage":{"output":0,"cacheRead":0,"cacheWrite":0,"totalTokens":0,"cost":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0,"total":0}},"stopReason":"pending","timestamp":1}}`,
		},
		{
			name: "assistant usage cost total",
			data: `{"type":"message_start","message":{"role":"assistant","content":[],"api":"test","provider":"test","model":"test","usage":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0,"totalTokens":0,"cost":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0}},"stopReason":"pending","timestamp":1}}`,
		},
		{
			name: "assistant diagnostic type",
			data: `{"type":"message_start","message":{"role":"assistant","content":[],"api":"test","provider":"test","model":"test","diagnostics":[{"timestamp":1}],"usage":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0,"totalTokens":0,"cost":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0,"total":0}},"stopReason":"pending","timestamp":1}}`,
		},
		{
			name: "assistant diagnostic timestamp",
			data: `{"type":"message_start","message":{"role":"assistant","content":[],"api":"test","provider":"test","model":"test","diagnostics":[{"type":"provider"}],"usage":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0,"totalTokens":0,"cost":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0,"total":0}},"stopReason":"pending","timestamp":1}}`,
		},
		{
			name: "assistant diagnostic error message",
			data: `{"type":"message_start","message":{"role":"assistant","content":[],"api":"test","provider":"test","model":"test","diagnostics":[{"type":"provider","timestamp":1,"error":{"name":"boom"}}],"usage":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0,"totalTokens":0,"cost":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0,"total":0}},"stopReason":"pending","timestamp":1}}`,
		},
		{
			name: "assistant deferred provider",
			data: `{"type":"message_start","message":{"role":"assistant","content":[],"api":"test","provider":"test","model":"test","usage":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0,"totalTokens":0,"cost":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0,"total":0}},"stopReason":"deferred","deferred":{"modelId":"test","api":"test","id":"job-1"},"timestamp":1}}`,
		},
		{
			name: "assistant deferred id",
			data: `{"type":"message_start","message":{"role":"assistant","content":[],"api":"test","provider":"test","model":"test","usage":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0,"totalTokens":0,"cost":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0,"total":0}},"stopReason":"deferred","deferred":{"provider":"test","modelId":"test","api":"test"},"timestamp":1}}`,
		},
		{
			name: "assistant event partial usage output",
			data: `{"type":"message_update","message":{"role":"assistant","content":[],"api":"test","provider":"test","model":"test","usage":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0,"totalTokens":0,"cost":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0,"total":0}},"stopReason":"pending","timestamp":1},"assistantMessageEvent":{"type":"text_delta","contentIndex":0,"delta":"x","partial":{"role":"assistant","content":[],"api":"test","provider":"test","model":"test","usage":{"input":0,"cacheRead":0,"cacheWrite":0,"totalTokens":0,"cost":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0,"total":0}},"stopReason":"pending","timestamp":1}}}`,
		},
		{
			name: "tool execution result usage cache write",
			data: `{"type":"tool_execution_end","toolCallId":"call-1","toolName":"search","result":{"content":[],"details":null,"usage":{"input":0,"output":0,"cacheRead":0,"totalTokens":0,"cost":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0,"total":0}}},"isError":false}`,
		},
		{
			name: "turn tool result usage cost input",
			data: `{"type":"turn_end","message":{"role":"notification","text":"done"},"toolResults":[{"role":"toolResult","toolCallId":"call-1","toolName":"search","content":[],"usage":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0,"totalTokens":0,"cost":{"output":0,"cacheRead":0,"cacheWrite":0,"total":0}},"isError":false,"timestamp":1}]}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := agent.UnmarshalAgentEvent([]byte(test.data)); err == nil {
				t.Fatalf("UnmarshalAgentEvent(%s) accepted missing required nested field for %s", test.data, test.name)
			}
		})
	}
}

func TestUnmarshalAgentEventPreservesExplicitOpenNestedValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		data string
	}{
		{
			name: "custom Agent message",
			data: `{"type":"message_start","message":{"role":"notification","future":true}}`,
		},
		{
			name: "Tool arguments",
			data: `{"type":"tool_execution_start","toolCallId":"call-1","toolName":"search","args":{"future":true}}`,
		},
		{
			name: "Tool result details",
			data: `{"type":"tool_execution_end","toolCallId":"call-1","toolName":"search","result":{"content":[],"details":{"future":true}},"isError":false}`,
		},
		{
			name: "Tool-call arguments",
			data: `{"type":"message_update","message":{"role":"assistant","content":[],"api":"test","provider":"test","model":"test","usage":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0,"totalTokens":0,"cost":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0,"total":0}},"stopReason":"pending","timestamp":1},"assistantMessageEvent":{"type":"toolcall_end","contentIndex":0,"toolCall":{"type":"toolCall","id":"call-1","name":"search","arguments":{"future":true}},"partial":{"role":"assistant","content":[],"api":"test","provider":"test","model":"test","usage":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0,"totalTokens":0,"cost":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0,"total":0}},"stopReason":"pending","timestamp":1}}}`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := agent.UnmarshalAgentEvent([]byte(test.data)); err != nil {
				t.Fatalf("UnmarshalAgentEvent(%s) rejected open %s: %v", test.data, test.name, err)
			}
		})
	}
}

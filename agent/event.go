package agent

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/nankedr/pig/ai"
)

// AgentEventType is the published discriminator for the closed Agent event
// union. The string values are compatibility-sensitive wire identifiers.
type AgentEventType string

const (
	AgentEventTypeAgentStart          AgentEventType = "agent_start"
	AgentEventTypeAgentEnd            AgentEventType = "agent_end"
	AgentEventTypeTurnStart           AgentEventType = "turn_start"
	AgentEventTypeTurnEnd             AgentEventType = "turn_end"
	AgentEventTypeMessageStart        AgentEventType = "message_start"
	AgentEventTypeMessageUpdate       AgentEventType = "message_update"
	AgentEventTypeMessageEnd          AgentEventType = "message_end"
	AgentEventTypeToolExecutionStart  AgentEventType = "tool_execution_start"
	AgentEventTypeToolExecutionUpdate AgentEventType = "tool_execution_update"
	AgentEventTypeToolExecutionEnd    AgentEventType = "tool_execution_end"
)

// AgentEvent is the closed set of events emitted by the legacy Agent.
//
// A normal Tool-free run emits agent_start, turn_start, the input message
// start/end pairs, the Assistant message start/update/end events, turn_end,
// and finally agent_end. Tool execution events occur after the Assistant
// message_end barrier and before Tool-result message pairs. Agent listeners
// are awaited in registration order; agent_end is the last event, but the run
// is not idle until its listeners have settled. Published message snapshots
// must not be mutated by later deltas.
type AgentEvent interface {
	agentEvent()
	AgentEventType() AgentEventType
}

// AgentStartEvent begins one Agent run.
type AgentStartEvent struct {
	Type AgentEventType `json:"type"`
}

func (AgentStartEvent) agentEvent() {}
func (e AgentStartEvent) AgentEventType() AgentEventType {
	return e.Type
}

// AgentEndEvent is the final loop event and carries all messages produced by
// this loop invocation. Listener settlement still follows this event.
type AgentEndEvent struct {
	Type     AgentEventType `json:"type"`
	Messages []AgentMessage `json:"-"`
}

func (AgentEndEvent) agentEvent() {}
func (e AgentEndEvent) AgentEventType() AgentEventType {
	return e.Type
}

// TurnStartEvent begins one Assistant response and its optional Tool batch.
type TurnStartEvent struct {
	Type AgentEventType `json:"type"`
}

func (TurnStartEvent) agentEvent() {}
func (e TurnStartEvent) AgentEventType() AgentEventType {
	return e.Type
}

// TurnEndEvent ends one Assistant response and carries Tool results in the
// Assistant source order, regardless of parallel completion order.
type TurnEndEvent struct {
	Type        AgentEventType         `json:"type"`
	Message     AgentMessage           `json:"-"`
	ToolResults []ai.ToolResultMessage `json:"-"`
}

func (TurnEndEvent) agentEvent() {}
func (e TurnEndEvent) AgentEventType() AgentEventType {
	return e.Type
}

// MessageStartEvent begins the lifecycle of one transcript message.
type MessageStartEvent struct {
	Type    AgentEventType `json:"type"`
	Message AgentMessage   `json:"-"`
}

func (MessageStartEvent) agentEvent() {}
func (e MessageStartEvent) AgentEventType() AgentEventType {
	return e.Type
}

// MessageUpdateEvent publishes an immutable Assistant-message snapshot and
// the nonterminal Assistant delta that produced it. Assistant start, done, and
// error events are represented by message_start/message_end instead.
type MessageUpdateEvent struct {
	Type                  AgentEventType           `json:"type"`
	Message               AgentMessage             `json:"-"`
	AssistantMessageEvent ai.AssistantMessageEvent `json:"-"`
}

func (MessageUpdateEvent) agentEvent() {}
func (e MessageUpdateEvent) AgentEventType() AgentEventType {
	return e.Type
}

// MessageEndEvent ends the lifecycle of one transcript message.
type MessageEndEvent struct {
	Type    AgentEventType `json:"type"`
	Message AgentMessage   `json:"-"`
}

func (MessageEndEvent) agentEvent() {}
func (e MessageEndEvent) AgentEventType() AgentEventType {
	return e.Type
}

// ToolExecutionStartEvent is emitted in Assistant source order after preflight
// begins for a Tool call.
type ToolExecutionStartEvent struct {
	Type       AgentEventType `json:"type"`
	ToolCallID string         `json:"toolCallId"`
	ToolName   string         `json:"toolName"`
	Arguments  ai.JSONValue   `json:"args"`
}

func (ToolExecutionStartEvent) agentEvent() {}
func (e ToolExecutionStartEvent) AgentEventType() AgentEventType {
	return e.Type
}

// ToolExecutionUpdateEvent carries one accepted partial Tool result. Accepted
// updates form a barrier before the corresponding ToolExecutionEndEvent.
type ToolExecutionUpdateEvent struct {
	Type          AgentEventType        `json:"type"`
	ToolCallID    string                `json:"toolCallId"`
	ToolName      string                `json:"toolName"`
	Arguments     ai.JSONValue          `json:"args"`
	PartialResult ErasedAgentToolResult `json:"-"`
}

func (ToolExecutionUpdateEvent) agentEvent() {}
func (e ToolExecutionUpdateEvent) AgentEventType() AgentEventType {
	return e.Type
}

// ToolExecutionEndEvent carries the finalized Tool result. In a parallel batch
// these events use completion order, while transcript Tool results retain
// Assistant source order.
type ToolExecutionEndEvent struct {
	Type       AgentEventType        `json:"type"`
	ToolCallID string                `json:"toolCallId"`
	ToolName   string                `json:"toolName"`
	Result     ErasedAgentToolResult `json:"-"`
	IsError    bool                  `json:"isError"`
}

func (ToolExecutionEndEvent) agentEvent() {}
func (e ToolExecutionEndEvent) AgentEventType() AgentEventType {
	return e.Type
}

// MarshalAgentEvent encodes one member of the closed AgentEvent union. It
// validates that the concrete variant agrees with its discriminator and routes
// nested open and closed unions through their respective codecs.
func MarshalAgentEvent(event AgentEvent) ([]byte, error) {
	want, ok := agentEventVariantType(event)
	if !ok {
		return nil, fmt.Errorf("marshal Agent event: unsupported concrete type %T", event)
	}
	if got := event.AgentEventType(); got != want {
		return nil, fmt.Errorf("marshal Agent event: %T requires discriminator %q, got %q", event, want, got)
	}
	encoded, err := marshalAgentEventVariant(event)
	if err != nil {
		return nil, fmt.Errorf("marshal Agent event %q: %w", want, err)
	}
	return encoded, nil
}

// UnmarshalAgentEvent decodes one member of the closed AgentEvent union. An
// unknown discriminator fails rather than becoming an open raw event.
func UnmarshalAgentEvent(data []byte) (AgentEvent, error) {
	fields, err := decodeAgentEventObject(data)
	if err != nil {
		return nil, fmt.Errorf("unmarshal Agent event: %w", err)
	}
	rawType, err := requireAgentEventField(fields, "type", false)
	if err != nil {
		return nil, fmt.Errorf("unmarshal Agent event: %w", err)
	}
	var eventType AgentEventType
	if err := json.Unmarshal(rawType, &eventType); err != nil || eventType == "" {
		if err == nil {
			err = fmt.Errorf("must not be empty")
		}
		return nil, fmt.Errorf("unmarshal Agent event: field %q must be a non-empty string: %w", "type", err)
	}

	event, err := unmarshalAgentEventVariant(eventType, fields)
	if err != nil {
		return nil, fmt.Errorf("unmarshal Agent event %q: %w", eventType, err)
	}
	return event, nil
}

func agentEventVariantType(event AgentEvent) (AgentEventType, bool) {
	switch event := event.(type) {
	case AgentStartEvent:
		return AgentEventTypeAgentStart, true
	case *AgentStartEvent:
		return AgentEventTypeAgentStart, event != nil
	case AgentEndEvent:
		return AgentEventTypeAgentEnd, true
	case *AgentEndEvent:
		return AgentEventTypeAgentEnd, event != nil
	case TurnStartEvent:
		return AgentEventTypeTurnStart, true
	case *TurnStartEvent:
		return AgentEventTypeTurnStart, event != nil
	case TurnEndEvent:
		return AgentEventTypeTurnEnd, true
	case *TurnEndEvent:
		return AgentEventTypeTurnEnd, event != nil
	case MessageStartEvent:
		return AgentEventTypeMessageStart, true
	case *MessageStartEvent:
		return AgentEventTypeMessageStart, event != nil
	case MessageUpdateEvent:
		return AgentEventTypeMessageUpdate, true
	case *MessageUpdateEvent:
		return AgentEventTypeMessageUpdate, event != nil
	case MessageEndEvent:
		return AgentEventTypeMessageEnd, true
	case *MessageEndEvent:
		return AgentEventTypeMessageEnd, event != nil
	case ToolExecutionStartEvent:
		return AgentEventTypeToolExecutionStart, true
	case *ToolExecutionStartEvent:
		return AgentEventTypeToolExecutionStart, event != nil
	case ToolExecutionUpdateEvent:
		return AgentEventTypeToolExecutionUpdate, true
	case *ToolExecutionUpdateEvent:
		return AgentEventTypeToolExecutionUpdate, event != nil
	case ToolExecutionEndEvent:
		return AgentEventTypeToolExecutionEnd, true
	case *ToolExecutionEndEvent:
		return AgentEventTypeToolExecutionEnd, event != nil
	default:
		return "", false
	}
}

func marshalAgentEventVariant(event AgentEvent) ([]byte, error) {
	switch event := event.(type) {
	case AgentStartEvent:
		return json.Marshal(struct {
			Type AgentEventType `json:"type"`
		}{Type: event.Type})
	case *AgentStartEvent:
		return marshalAgentEventVariant(*event)
	case AgentEndEvent:
		messages, err := marshalAgentMessageSlice(event.Messages)
		if err != nil {
			return nil, err
		}
		return json.Marshal(struct {
			Type     AgentEventType    `json:"type"`
			Messages []json.RawMessage `json:"messages"`
		}{Type: event.Type, Messages: messages})
	case *AgentEndEvent:
		return marshalAgentEventVariant(*event)
	case TurnStartEvent:
		return json.Marshal(struct {
			Type AgentEventType `json:"type"`
		}{Type: event.Type})
	case *TurnStartEvent:
		return marshalAgentEventVariant(*event)
	case TurnEndEvent:
		message, err := marshalNestedAgentMessage(event.Message)
		if err != nil {
			return nil, fmt.Errorf("message: %w", err)
		}
		toolResults, err := marshalToolResultMessageSlice(event.ToolResults)
		if err != nil {
			return nil, err
		}
		return json.Marshal(struct {
			Type        AgentEventType    `json:"type"`
			Message     json.RawMessage   `json:"message"`
			ToolResults []json.RawMessage `json:"toolResults"`
		}{Type: event.Type, Message: message, ToolResults: toolResults})
	case *TurnEndEvent:
		return marshalAgentEventVariant(*event)
	case MessageStartEvent:
		return marshalMessageLifecycleEvent(event.Type, event.Message)
	case *MessageStartEvent:
		return marshalAgentEventVariant(*event)
	case MessageUpdateEvent:
		message, err := marshalNestedAgentMessage(event.Message)
		if err != nil {
			return nil, fmt.Errorf("message: %w", err)
		}
		assistantEvent, err := ai.MarshalAssistantMessageEvent(event.AssistantMessageEvent)
		if err != nil {
			return nil, fmt.Errorf("assistantMessageEvent: %w", err)
		}
		if err := validateMessageUpdateAssistantEvent(event.AssistantMessageEvent); err != nil {
			return nil, err
		}
		return json.Marshal(struct {
			Type                  AgentEventType  `json:"type"`
			Message               json.RawMessage `json:"message"`
			AssistantMessageEvent json.RawMessage `json:"assistantMessageEvent"`
		}{Type: event.Type, Message: message, AssistantMessageEvent: assistantEvent})
	case *MessageUpdateEvent:
		return marshalAgentEventVariant(*event)
	case MessageEndEvent:
		return marshalMessageLifecycleEvent(event.Type, event.Message)
	case *MessageEndEvent:
		return marshalAgentEventVariant(*event)
	case ToolExecutionStartEvent:
		arguments, err := marshalAgentJSONValue(event.Arguments)
		if err != nil {
			return nil, fmt.Errorf("args: %w", err)
		}
		return json.Marshal(struct {
			Type       AgentEventType  `json:"type"`
			ToolCallID string          `json:"toolCallId"`
			ToolName   string          `json:"toolName"`
			Arguments  json.RawMessage `json:"args"`
		}{Type: event.Type, ToolCallID: event.ToolCallID, ToolName: event.ToolName, Arguments: arguments})
	case *ToolExecutionStartEvent:
		return marshalAgentEventVariant(*event)
	case ToolExecutionUpdateEvent:
		arguments, err := marshalAgentJSONValue(event.Arguments)
		if err != nil {
			return nil, fmt.Errorf("args: %w", err)
		}
		partialResult, err := marshalErasedAgentToolResult(event.PartialResult)
		if err != nil {
			return nil, fmt.Errorf("partialResult: %w", err)
		}
		return json.Marshal(struct {
			Type          AgentEventType  `json:"type"`
			ToolCallID    string          `json:"toolCallId"`
			ToolName      string          `json:"toolName"`
			Arguments     json.RawMessage `json:"args"`
			PartialResult json.RawMessage `json:"partialResult"`
		}{
			Type: event.Type, ToolCallID: event.ToolCallID, ToolName: event.ToolName,
			Arguments: arguments, PartialResult: partialResult,
		})
	case *ToolExecutionUpdateEvent:
		return marshalAgentEventVariant(*event)
	case ToolExecutionEndEvent:
		result, err := marshalErasedAgentToolResult(event.Result)
		if err != nil {
			return nil, fmt.Errorf("result: %w", err)
		}
		return json.Marshal(struct {
			Type       AgentEventType  `json:"type"`
			ToolCallID string          `json:"toolCallId"`
			ToolName   string          `json:"toolName"`
			Result     json.RawMessage `json:"result"`
			IsError    bool            `json:"isError"`
		}{Type: event.Type, ToolCallID: event.ToolCallID, ToolName: event.ToolName, Result: result, IsError: event.IsError})
	case *ToolExecutionEndEvent:
		return marshalAgentEventVariant(*event)
	default:
		return nil, fmt.Errorf("unsupported concrete type %T", event)
	}
}

func unmarshalAgentEventVariant(eventType AgentEventType, fields map[string]json.RawMessage) (AgentEvent, error) {
	if err := validateAgentEventFields(eventType, fields); err != nil {
		return nil, err
	}
	switch eventType {
	case AgentEventTypeAgentStart:
		return AgentStartEvent{Type: eventType}, nil
	case AgentEventTypeAgentEnd:
		messages, err := unmarshalAgentMessageSliceField(fields, "messages")
		if err != nil {
			return nil, err
		}
		return AgentEndEvent{Type: eventType, Messages: messages}, nil
	case AgentEventTypeTurnStart:
		return TurnStartEvent{Type: eventType}, nil
	case AgentEventTypeTurnEnd:
		message, err := unmarshalAgentMessageField(fields, "message")
		if err != nil {
			return nil, err
		}
		toolResults, err := unmarshalToolResultMessageSliceField(fields, "toolResults")
		if err != nil {
			return nil, err
		}
		return TurnEndEvent{Type: eventType, Message: message, ToolResults: toolResults}, nil
	case AgentEventTypeMessageStart:
		message, err := unmarshalAgentMessageField(fields, "message")
		if err != nil {
			return nil, err
		}
		return MessageStartEvent{Type: eventType, Message: message}, nil
	case AgentEventTypeMessageUpdate:
		message, err := unmarshalAgentMessageField(fields, "message")
		if err != nil {
			return nil, err
		}
		rawAssistantEvent, err := requireAgentEventField(fields, "assistantMessageEvent", false)
		if err != nil {
			return nil, err
		}
		if !agentEventJSONObject(rawAssistantEvent) {
			return nil, fmt.Errorf("field %q must be an object", "assistantMessageEvent")
		}
		if err := validateAssistantMessageEventFields(rawAssistantEvent); err != nil {
			return nil, fmt.Errorf("assistantMessageEvent: %w", err)
		}
		assistantEvent, err := ai.UnmarshalAssistantMessageEvent(rawAssistantEvent)
		if err != nil {
			return nil, fmt.Errorf("assistantMessageEvent: %w", err)
		}
		if err := validateMessageUpdateAssistantEvent(assistantEvent); err != nil {
			return nil, err
		}
		return MessageUpdateEvent{Type: eventType, Message: message, AssistantMessageEvent: assistantEvent}, nil
	case AgentEventTypeMessageEnd:
		message, err := unmarshalAgentMessageField(fields, "message")
		if err != nil {
			return nil, err
		}
		return MessageEndEvent{Type: eventType, Message: message}, nil
	case AgentEventTypeToolExecutionStart:
		toolCallID, toolName, arguments, err := unmarshalToolExecutionFields(fields)
		if err != nil {
			return nil, err
		}
		return ToolExecutionStartEvent{
			Type: eventType, ToolCallID: toolCallID, ToolName: toolName, Arguments: arguments,
		}, nil
	case AgentEventTypeToolExecutionUpdate:
		toolCallID, toolName, arguments, err := unmarshalToolExecutionFields(fields)
		if err != nil {
			return nil, err
		}
		rawResult, err := requireAgentEventField(fields, "partialResult", false)
		if err != nil {
			return nil, err
		}
		partialResult, err := unmarshalErasedAgentToolResult(rawResult)
		if err != nil {
			return nil, fmt.Errorf("partialResult: %w", err)
		}
		return ToolExecutionUpdateEvent{
			Type: eventType, ToolCallID: toolCallID, ToolName: toolName,
			Arguments: arguments, PartialResult: partialResult,
		}, nil
	case AgentEventTypeToolExecutionEnd:
		toolCallID, err := unmarshalRequiredStringField(fields, "toolCallId")
		if err != nil {
			return nil, err
		}
		toolName, err := unmarshalRequiredStringField(fields, "toolName")
		if err != nil {
			return nil, err
		}
		rawResult, err := requireAgentEventField(fields, "result", false)
		if err != nil {
			return nil, err
		}
		result, err := unmarshalErasedAgentToolResult(rawResult)
		if err != nil {
			return nil, fmt.Errorf("result: %w", err)
		}
		isError, err := unmarshalRequiredBoolField(fields, "isError")
		if err != nil {
			return nil, err
		}
		return ToolExecutionEndEvent{
			Type: eventType, ToolCallID: toolCallID, ToolName: toolName, Result: result, IsError: isError,
		}, nil
	default:
		return nil, fmt.Errorf("unknown discriminator")
	}
}

func validateAgentEventFields(eventType AgentEventType, fields map[string]json.RawMessage) error {
	var allowed []string
	switch eventType {
	case AgentEventTypeAgentStart, AgentEventTypeTurnStart:
		allowed = []string{"type"}
	case AgentEventTypeAgentEnd:
		allowed = []string{"type", "messages"}
	case AgentEventTypeTurnEnd:
		allowed = []string{"type", "message", "toolResults"}
	case AgentEventTypeMessageStart, AgentEventTypeMessageEnd:
		allowed = []string{"type", "message"}
	case AgentEventTypeMessageUpdate:
		allowed = []string{"type", "message", "assistantMessageEvent"}
	case AgentEventTypeToolExecutionStart:
		allowed = []string{"type", "toolCallId", "toolName", "args"}
	case AgentEventTypeToolExecutionUpdate:
		allowed = []string{"type", "toolCallId", "toolName", "args", "partialResult"}
	case AgentEventTypeToolExecutionEnd:
		allowed = []string{"type", "toolCallId", "toolName", "result", "isError"}
	default:
		return fmt.Errorf("unknown discriminator")
	}
	return rejectUnknownAgentEventFields(fields, allowed...)
}

func marshalMessageLifecycleEvent(eventType AgentEventType, message AgentMessage) ([]byte, error) {
	encoded, err := marshalNestedAgentMessage(message)
	if err != nil {
		return nil, fmt.Errorf("message: %w", err)
	}
	return json.Marshal(struct {
		Type    AgentEventType  `json:"type"`
		Message json.RawMessage `json:"message"`
	}{Type: eventType, Message: encoded})
}

func marshalNestedAgentMessage(message AgentMessage) (json.RawMessage, error) {
	encoded, err := MarshalAgentMessage(message)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(encoded), nil
}

func marshalAgentMessageSlice(messages []AgentMessage) ([]json.RawMessage, error) {
	encoded := make([]json.RawMessage, len(messages))
	for i, message := range messages {
		raw, err := marshalNestedAgentMessage(message)
		if err != nil {
			return nil, fmt.Errorf("messages[%d]: %w", i, err)
		}
		encoded[i] = raw
	}
	if encoded == nil {
		encoded = []json.RawMessage{}
	}
	return encoded, nil
}

func marshalToolResultMessageSlice(messages []ai.ToolResultMessage) ([]json.RawMessage, error) {
	encoded := make([]json.RawMessage, len(messages))
	for i, message := range messages {
		raw, err := ai.MarshalMessage(message)
		if err != nil {
			return nil, fmt.Errorf("toolResults[%d]: %w", i, err)
		}
		encoded[i] = json.RawMessage(raw)
	}
	if encoded == nil {
		encoded = []json.RawMessage{}
	}
	return encoded, nil
}

func validateMessageUpdateAssistantEvent(event ai.AssistantMessageEvent) error {
	eventType := event.AssistantMessageEventType()
	switch eventType {
	case ai.AssistantMessageEventTypeTextStart,
		ai.AssistantMessageEventTypeTextDelta,
		ai.AssistantMessageEventTypeTextEnd,
		ai.AssistantMessageEventTypeThinkingStart,
		ai.AssistantMessageEventTypeThinkingDelta,
		ai.AssistantMessageEventTypeThinkingEnd,
		ai.AssistantMessageEventTypeToolCallStart,
		ai.AssistantMessageEventTypeToolCallDelta,
		ai.AssistantMessageEventTypeToolCallEnd:
		return nil
	case ai.AssistantMessageEventTypeStart, ai.AssistantMessageEventTypeDone, ai.AssistantMessageEventTypeError:
		return fmt.Errorf("assistantMessageEvent %q is not valid in message_update", eventType)
	default:
		return fmt.Errorf("unknown assistantMessageEvent discriminator %q", eventType)
	}
}

func validateAssistantMessageEventFields(data []byte) error {
	fields, err := decodeAgentEventObject(data)
	if err != nil {
		return err
	}
	rawType, err := requireAgentEventField(fields, "type", false)
	if err != nil {
		return err
	}
	var eventType ai.AssistantMessageEventType
	if err := json.Unmarshal(rawType, &eventType); err != nil {
		return fmt.Errorf("field %q must be a string: %w", "type", err)
	}

	var allowed []string
	switch eventType {
	case ai.AssistantMessageEventTypeStart:
		allowed = []string{"type", "partial"}
	case ai.AssistantMessageEventTypeTextStart,
		ai.AssistantMessageEventTypeThinkingStart,
		ai.AssistantMessageEventTypeToolCallStart:
		allowed = []string{"type", "contentIndex", "partial"}
	case ai.AssistantMessageEventTypeTextDelta,
		ai.AssistantMessageEventTypeThinkingDelta,
		ai.AssistantMessageEventTypeToolCallDelta:
		allowed = []string{"type", "contentIndex", "delta", "partial"}
	case ai.AssistantMessageEventTypeTextEnd, ai.AssistantMessageEventTypeThinkingEnd:
		allowed = []string{"type", "contentIndex", "content", "partial"}
	case ai.AssistantMessageEventTypeToolCallEnd:
		allowed = []string{"type", "contentIndex", "toolCall", "partial"}
	case ai.AssistantMessageEventTypeDone:
		allowed = []string{"type", "reason", "message"}
	case ai.AssistantMessageEventTypeError:
		allowed = []string{"type", "reason", "error"}
	default:
		return nil
	}
	if err := rejectUnknownAgentEventFields(fields, allowed...); err != nil {
		return err
	}

	var messageField string
	switch eventType {
	case ai.AssistantMessageEventTypeStart,
		ai.AssistantMessageEventTypeTextStart,
		ai.AssistantMessageEventTypeTextDelta,
		ai.AssistantMessageEventTypeTextEnd,
		ai.AssistantMessageEventTypeThinkingStart,
		ai.AssistantMessageEventTypeThinkingDelta,
		ai.AssistantMessageEventTypeThinkingEnd,
		ai.AssistantMessageEventTypeToolCallStart,
		ai.AssistantMessageEventTypeToolCallDelta,
		ai.AssistantMessageEventTypeToolCallEnd:
		messageField = "partial"
	case ai.AssistantMessageEventTypeDone:
		messageField = "message"
	case ai.AssistantMessageEventTypeError:
		messageField = "error"
	}
	if messageField != "" {
		rawMessage, err := requireAgentEventField(fields, messageField, false)
		if err != nil {
			return err
		}
		if err := validateAgentMessageFields(rawMessage); err != nil {
			return fmt.Errorf("%s: %w", messageField, err)
		}
	}
	if eventType == ai.AssistantMessageEventTypeToolCallEnd {
		rawToolCall, err := requireAgentEventField(fields, "toolCall", false)
		if err != nil {
			return err
		}
		if err := validateAgentEventContentFields(rawToolCall); err != nil {
			return fmt.Errorf("toolCall: %w", err)
		}
	}
	return nil
}

func marshalAgentJSONValue(value ai.JSONValue) (json.RawMessage, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(encoded), nil
}

func marshalErasedAgentToolResult(result ErasedAgentToolResult) (json.RawMessage, error) {
	content := make([]json.RawMessage, len(result.Content))
	for i, item := range result.Content {
		encoded, err := ai.MarshalContent(item)
		if err != nil {
			return nil, fmt.Errorf("content[%d]: %w", i, err)
		}
		content[i] = json.RawMessage(encoded)
	}
	if content == nil {
		content = []json.RawMessage{}
	}
	details, err := marshalAgentJSONValue(result.Details)
	if err != nil {
		return nil, fmt.Errorf("details: %w", err)
	}
	encoded, err := json.Marshal(struct {
		Content        []json.RawMessage     `json:"content"`
		Details        json.RawMessage       `json:"details"`
		Usage          ai.Optional[ai.Usage] `json:"usage,omitzero"`
		AddedToolNames ai.Optional[[]string] `json:"addedToolNames,omitzero"`
		Terminate      ai.Optional[bool]     `json:"terminate,omitzero"`
	}{
		Content: content, Details: details, Usage: result.Usage,
		AddedToolNames: result.AddedToolNames, Terminate: result.Terminate,
	})
	return json.RawMessage(encoded), err
}

func decodeAgentEventObject(data []byte) (map[string]json.RawMessage, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return nil, fmt.Errorf("expected JSON object")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &fields); err != nil {
		return nil, err
	}
	if fields == nil {
		return nil, fmt.Errorf("expected JSON object")
	}
	return fields, nil
}

func requireAgentEventField(fields map[string]json.RawMessage, name string, allowNull bool) (json.RawMessage, error) {
	raw, ok := fields[name]
	trimmed := bytes.TrimSpace(raw)
	if !ok || len(trimmed) == 0 || (!allowNull && bytes.Equal(trimmed, []byte("null"))) {
		return nil, fmt.Errorf("required field %q is missing or null", name)
	}
	return trimmed, nil
}

func rejectUnknownAgentEventFields(fields map[string]json.RawMessage, allowed ...string) error {
	for name := range fields {
		known := false
		for _, allowedName := range allowed {
			if name == allowedName {
				known = true
				break
			}
		}
		if !known {
			return fmt.Errorf("unknown field %q", name)
		}
	}
	return nil
}

func agentEventJSONObject(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) != 0 && trimmed[0] == '{'
}

func agentEventJSONArray(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) != 0 && trimmed[0] == '['
}

func unmarshalRequiredStringField(fields map[string]json.RawMessage, name string) (string, error) {
	raw, err := requireAgentEventField(fields, name, false)
	if err != nil {
		return "", err
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", fmt.Errorf("field %q must be a string: %w", name, err)
	}
	return value, nil
}

func unmarshalRequiredBoolField(fields map[string]json.RawMessage, name string) (bool, error) {
	raw, err := requireAgentEventField(fields, name, false)
	if err != nil {
		return false, err
	}
	var value bool
	if err := json.Unmarshal(raw, &value); err != nil {
		return false, fmt.Errorf("field %q must be a boolean: %w", name, err)
	}
	return value, nil
}

func unmarshalAgentMessageField(fields map[string]json.RawMessage, name string) (AgentMessage, error) {
	raw, err := requireAgentEventField(fields, name, false)
	if err != nil {
		return nil, err
	}
	if !agentEventJSONObject(raw) {
		return nil, fmt.Errorf("field %q must be an object", name)
	}
	if err := validateAgentMessageFields(raw); err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	message, err := UnmarshalAgentMessage(raw)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	return message, nil
}

func unmarshalAgentMessageSliceField(fields map[string]json.RawMessage, name string) ([]AgentMessage, error) {
	raw, err := requireAgentEventField(fields, name, false)
	if err != nil {
		return nil, err
	}
	if !agentEventJSONArray(raw) {
		return nil, fmt.Errorf("field %q must be an array", name)
	}
	var values []json.RawMessage
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, fmt.Errorf("field %q must be an array: %w", name, err)
	}
	messages := make([]AgentMessage, len(values))
	for i, value := range values {
		if err := validateAgentMessageFields(value); err != nil {
			return nil, fmt.Errorf("%s[%d]: %w", name, i, err)
		}
		message, err := UnmarshalAgentMessage(value)
		if err != nil {
			return nil, fmt.Errorf("%s[%d]: %w", name, i, err)
		}
		messages[i] = message
	}
	return messages, nil
}

func validateAgentMessageFields(data []byte) error {
	fields, err := decodeAgentEventObject(data)
	if err != nil {
		return err
	}
	rawRole, err := requireAgentEventField(fields, "role", false)
	if err != nil {
		return err
	}
	var role ai.MessageRole
	if err := json.Unmarshal(rawRole, &role); err != nil {
		return fmt.Errorf("field %q must be a string: %w", "role", err)
	}

	switch role {
	case ai.MessageRoleUser:
		if err := rejectUnknownAgentEventFields(fields, "role", "content", "timestamp"); err != nil {
			return err
		}
	case ai.MessageRoleAssistant:
		if err := rejectUnknownAgentEventFields(fields,
			"role", "content", "api", "provider", "model",
			"responseModel", "responseId", "diagnostics", "usage",
			"stopReason", "deferred", "errorMessage", "rawStopReason",
			"endTurn", "timestamp",
		); err != nil {
			return err
		}
	case ai.MessageRoleToolResult:
		if err := rejectUnknownAgentEventFields(fields,
			"role", "toolCallId", "toolName", "content", "details",
			"usage", "addedToolNames", "isError", "timestamp",
		); err != nil {
			return err
		}
	default:
		return nil
	}
	if err := validateAgentMessageContentFields(fields); err != nil {
		return err
	}
	switch role {
	case ai.MessageRoleAssistant:
		if err := validateUsageField(fields, "usage", false); err != nil {
			return err
		}
		if err := validateAssistantDiagnosticsField(fields); err != nil {
			return err
		}
		return validateDeferredHandleField(fields)
	case ai.MessageRoleToolResult:
		return validateUsageField(fields, "usage", true)
	default:
		return nil
	}
}

func validateAgentMessageContentFields(fields map[string]json.RawMessage) error {
	rawContent, err := requireAgentEventField(fields, "content", false)
	if err != nil {
		return err
	}
	trimmed := bytes.TrimSpace(rawContent)
	if len(trimmed) == 0 || trimmed[0] != '[' {
		return nil
	}
	var items []json.RawMessage
	if err := json.Unmarshal(trimmed, &items); err != nil {
		return fmt.Errorf("content: %w", err)
	}
	for i, item := range items {
		if err := validateAgentEventContentFields(item); err != nil {
			return fmt.Errorf("content[%d]: %w", i, err)
		}
	}
	return nil
}

func validateUsageField(fields map[string]json.RawMessage, name string, optional bool) error {
	raw, ok := fields[name]
	if !ok {
		if optional {
			return nil
		}
		return fmt.Errorf("required field %q is missing or null", name)
	}
	if optional && bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil
	}
	usageFields, err := decodeAgentEventObject(raw)
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	if err := rejectUnknownAgentEventFields(usageFields,
		"input", "output", "cacheRead", "cacheWrite", "cacheWrite1h",
		"reasoning", "totalTokens", "cost",
	); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	for _, required := range []string{"input", "output", "cacheRead", "cacheWrite", "totalTokens"} {
		if err := requireAgentEventNumberField(usageFields, required); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	rawCost, err := requireAgentEventField(usageFields, "cost", false)
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	costFields, err := decodeAgentEventObject(rawCost)
	if err != nil {
		return fmt.Errorf("%s.cost: %w", name, err)
	}
	if err := rejectUnknownAgentEventFields(costFields, "input", "output", "cacheRead", "cacheWrite", "total"); err != nil {
		return fmt.Errorf("%s.cost: %w", name, err)
	}
	for _, required := range []string{"input", "output", "cacheRead", "cacheWrite", "total"} {
		if err := requireAgentEventNumberField(costFields, required); err != nil {
			return fmt.Errorf("%s.cost: %w", name, err)
		}
	}
	return nil
}

func validateAssistantDiagnosticsField(fields map[string]json.RawMessage) error {
	raw, ok := fields["diagnostics"]
	if !ok || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil
	}
	var diagnostics []json.RawMessage
	if err := json.Unmarshal(raw, &diagnostics); err != nil {
		return fmt.Errorf("diagnostics: %w", err)
	}
	for i, rawDiagnostic := range diagnostics {
		diagnostic, err := decodeAgentEventObject(rawDiagnostic)
		if err != nil {
			return fmt.Errorf("diagnostics[%d]: %w", i, err)
		}
		if err := rejectUnknownAgentEventFields(diagnostic, "type", "timestamp", "error", "details"); err != nil {
			return fmt.Errorf("diagnostics[%d]: %w", i, err)
		}
		if err := requireAgentEventStringField(diagnostic, "type"); err != nil {
			return fmt.Errorf("diagnostics[%d]: %w", i, err)
		}
		if err := requireAgentEventNumberField(diagnostic, "timestamp"); err != nil {
			return fmt.Errorf("diagnostics[%d]: %w", i, err)
		}
		rawError, ok := diagnostic["error"]
		if !ok || bytes.Equal(bytes.TrimSpace(rawError), []byte("null")) {
			continue
		}
		errorFields, err := decodeAgentEventObject(rawError)
		if err != nil {
			return fmt.Errorf("diagnostics[%d].error: %w", i, err)
		}
		if err := rejectUnknownAgentEventFields(errorFields, "name", "message", "stack", "code"); err != nil {
			return fmt.Errorf("diagnostics[%d].error: %w", i, err)
		}
		if err := requireAgentEventStringField(errorFields, "message"); err != nil {
			return fmt.Errorf("diagnostics[%d].error: %w", i, err)
		}
	}
	return nil
}

func validateDeferredHandleField(fields map[string]json.RawMessage) error {
	raw, ok := fields["deferred"]
	if !ok || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil
	}
	deferred, err := decodeAgentEventObject(raw)
	if err != nil {
		return fmt.Errorf("deferred: %w", err)
	}
	if err := rejectUnknownAgentEventFields(deferred,
		"provider", "modelId", "api", "id", "expiresAt", "pollAfterMs", "data",
	); err != nil {
		return fmt.Errorf("deferred: %w", err)
	}
	for _, required := range []string{"provider", "modelId", "api", "id"} {
		if err := requireAgentEventStringField(deferred, required); err != nil {
			return fmt.Errorf("deferred: %w", err)
		}
	}
	return nil
}

func requireAgentEventStringField(fields map[string]json.RawMessage, name string) error {
	raw, err := requireAgentEventField(fields, name, false)
	if err != nil {
		return err
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return fmt.Errorf("field %q must be a string: %w", name, err)
	}
	return nil
}

func requireAgentEventNumberField(fields map[string]json.RawMessage, name string) error {
	raw, err := requireAgentEventField(fields, name, false)
	if err != nil {
		return err
	}
	var value float64
	if err := json.Unmarshal(raw, &value); err != nil {
		return fmt.Errorf("field %q must be a number: %w", name, err)
	}
	return nil
}

func unmarshalToolResultMessageSliceField(fields map[string]json.RawMessage, name string) ([]ai.ToolResultMessage, error) {
	raw, err := requireAgentEventField(fields, name, false)
	if err != nil {
		return nil, err
	}
	if !agentEventJSONArray(raw) {
		return nil, fmt.Errorf("field %q must be an array", name)
	}
	var values []json.RawMessage
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, fmt.Errorf("field %q must be an array: %w", name, err)
	}
	messages := make([]ai.ToolResultMessage, len(values))
	for i, value := range values {
		if err := validateAgentMessageFields(value); err != nil {
			return nil, fmt.Errorf("%s[%d]: %w", name, i, err)
		}
		message, err := ai.UnmarshalMessage(value)
		if err != nil {
			return nil, fmt.Errorf("%s[%d]: %w", name, i, err)
		}
		toolResult, ok := message.(ai.ToolResultMessage)
		if !ok {
			return nil, fmt.Errorf("%s[%d] must be a ToolResultMessage, got %T", name, i, message)
		}
		messages[i] = toolResult
	}
	return messages, nil
}

func unmarshalToolExecutionFields(fields map[string]json.RawMessage) (string, string, ai.JSONValue, error) {
	toolCallID, err := unmarshalRequiredStringField(fields, "toolCallId")
	if err != nil {
		return "", "", nil, err
	}
	toolName, err := unmarshalRequiredStringField(fields, "toolName")
	if err != nil {
		return "", "", nil, err
	}
	rawArguments, err := requireAgentEventField(fields, "args", true)
	if err != nil {
		return "", "", nil, err
	}
	var arguments ai.JSONValue
	if err := json.Unmarshal(rawArguments, &arguments); err != nil {
		return "", "", nil, fmt.Errorf("args: %w", err)
	}
	return toolCallID, toolName, arguments, nil
}

func unmarshalErasedAgentToolResult(data []byte) (ErasedAgentToolResult, error) {
	fields, err := decodeAgentEventObject(data)
	if err != nil {
		return ErasedAgentToolResult{}, err
	}
	if err := rejectUnknownAgentEventFields(fields,
		"content", "details", "usage", "addedToolNames", "terminate",
	); err != nil {
		return ErasedAgentToolResult{}, err
	}
	rawContent, err := requireAgentEventField(fields, "content", false)
	if err != nil {
		return ErasedAgentToolResult{}, err
	}
	if !agentEventJSONArray(rawContent) {
		return ErasedAgentToolResult{}, fmt.Errorf("field %q must be an array", "content")
	}
	var rawItems []json.RawMessage
	if err := json.Unmarshal(rawContent, &rawItems); err != nil {
		return ErasedAgentToolResult{}, fmt.Errorf("content: %w", err)
	}
	content := make([]ai.ToolResultContent, len(rawItems))
	for i, rawItem := range rawItems {
		if err := validateAgentEventContentFields(rawItem); err != nil {
			return ErasedAgentToolResult{}, fmt.Errorf("content[%d]: %w", i, err)
		}
		item, err := ai.UnmarshalContent(rawItem)
		if err != nil {
			return ErasedAgentToolResult{}, fmt.Errorf("content[%d]: %w", i, err)
		}
		switch item := item.(type) {
		case ai.TextContent:
			content[i] = item
		case ai.ImageContent:
			content[i] = item
		default:
			return ErasedAgentToolResult{}, fmt.Errorf("content[%d] has invalid Tool-result variant %T", i, item)
		}
	}
	rawDetails, err := requireAgentEventField(fields, "details", true)
	if err != nil {
		return ErasedAgentToolResult{}, err
	}
	var details ai.JSONValue
	if err := json.Unmarshal(rawDetails, &details); err != nil {
		return ErasedAgentToolResult{}, fmt.Errorf("details: %w", err)
	}
	if err := validateUsageField(fields, "usage", true); err != nil {
		return ErasedAgentToolResult{}, err
	}

	var optionals struct {
		Usage          ai.Optional[ai.Usage] `json:"usage"`
		AddedToolNames ai.Optional[[]string] `json:"addedToolNames"`
		Terminate      ai.Optional[bool]     `json:"terminate"`
	}
	if err := json.Unmarshal(data, &optionals); err != nil {
		return ErasedAgentToolResult{}, err
	}
	return ErasedAgentToolResult{
		Content: content, Details: details, Usage: optionals.Usage,
		AddedToolNames: optionals.AddedToolNames, Terminate: optionals.Terminate,
	}, nil
}

func validateAgentEventContentFields(data []byte) error {
	fields, err := decodeAgentEventObject(data)
	if err != nil {
		return err
	}
	rawType, err := requireAgentEventField(fields, "type", false)
	if err != nil {
		return err
	}
	var contentType ai.ContentType
	if err := json.Unmarshal(rawType, &contentType); err != nil {
		return fmt.Errorf("field %q must be a string: %w", "type", err)
	}

	switch contentType {
	case ai.ContentTypeText:
		return rejectUnknownAgentEventFields(fields, "type", "text", "textSignature")
	case ai.ContentTypeThinking:
		return rejectUnknownAgentEventFields(fields, "type", "thinking", "thinkingSignature", "redacted")
	case ai.ContentTypeImage:
		return rejectUnknownAgentEventFields(fields, "type", "data", "mimeType")
	case ai.ContentTypeToolCall:
		return rejectUnknownAgentEventFields(fields,
			"type", "id", "name", "arguments", "thoughtSignature", "namespace",
		)
	default:
		return nil
	}
}

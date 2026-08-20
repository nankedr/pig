package ai

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
)

// ErrCodec identifies an invalid closed-union discriminator or representation.
var ErrCodec = errors.New("invalid AI codec value")

// CodecError identifies which closed surface and discriminator failed.
type CodecError struct {
	Surface       string
	Discriminator string
	Cause         error
}

func (e *CodecError) Error() string {
	message := fmt.Sprintf("%s discriminator %q: %s", e.Surface, e.Discriminator, ErrCodec)
	if e.Cause != nil {
		message += ": " + e.Cause.Error()
	}
	return message
}

func (e *CodecError) Unwrap() []error {
	if e.Cause == nil {
		return []error{ErrCodec}
	}
	return []error{ErrCodec, e.Cause}
}

func newCodecError(surface, discriminator string, cause error) *CodecError {
	return &CodecError{Surface: surface, Discriminator: discriminator, Cause: cause}
}

// MarshalContent encodes one member of the closed content union after checking
// that its concrete variant and published discriminator agree.
func MarshalContent(content Content) ([]byte, error) {
	want, ok := contentVariantType(content)
	if !ok || isNilClosedUnion(content) {
		return nil, newCodecError("content", "", fmt.Errorf("unsupported concrete type %T", content))
	}
	if got := content.ContentType(); got != want {
		return nil, newCodecError("content", string(got), fmt.Errorf("%T requires %q", content, want))
	}
	encoded, err := json.Marshal(content)
	if err != nil {
		return nil, newCodecError("content", string(want), err)
	}
	return encoded, nil
}

// UnmarshalContent decodes one member of the closed content union. Unknown
// discriminators fail instead of being retained as an open raw value.
func UnmarshalContent(data []byte) (Content, error) {
	var envelope struct {
		Type ContentType `json:"type"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, newCodecError("content", "", err)
	}
	if err := validateContentWire(envelope.Type, data); err != nil {
		return nil, newCodecError("content", string(envelope.Type), err)
	}

	var content Content
	switch envelope.Type {
	case ContentTypeText:
		content = &TextContent{}
	case ContentTypeThinking:
		content = &ThinkingContent{}
	case ContentTypeImage:
		content = &ImageContent{}
	case ContentTypeToolCall:
		content = &ToolCall{}
	default:
		return nil, newCodecError("content", string(envelope.Type), nil)
	}
	if err := json.Unmarshal(data, content); err != nil {
		return nil, newCodecError("content", string(envelope.Type), err)
	}

	switch value := content.(type) {
	case *TextContent:
		return *value, nil
	case *ThinkingContent:
		return *value, nil
	case *ImageContent:
		return *value, nil
	case *ToolCall:
		return *value, nil
	default:
		panic("unreachable content variant")
	}
}

func contentVariantType(content Content) (ContentType, bool) {
	switch content.(type) {
	case TextContent, *TextContent:
		return ContentTypeText, true
	case ThinkingContent, *ThinkingContent:
		return ContentTypeThinking, true
	case ImageContent, *ImageContent:
		return ContentTypeImage, true
	case ToolCall, *ToolCall:
		return ContentTypeToolCall, true
	default:
		return "", false
	}
}

// MarshalMessage encodes one member of the closed model-message union after
// checking that its concrete variant and role discriminator agree.
func MarshalMessage(message Message) ([]byte, error) {
	want, ok := messageVariantRole(message)
	if !ok || isNilClosedUnion(message) {
		return nil, newCodecError("message", "", fmt.Errorf("unsupported concrete type %T", message))
	}
	if got := message.MessageRole(); got != want {
		return nil, newCodecError("message", string(got), fmt.Errorf("%T requires %q", message, want))
	}
	encoded, err := marshalMessageVariant(message)
	if err != nil {
		if errors.Is(err, ErrCodec) {
			return nil, err
		}
		return nil, newCodecError("message", string(want), err)
	}
	return encoded, nil
}

// UnmarshalMessage decodes one member of the closed model-message union.
// Unknown roles fail rather than leaking into the separate open Agent seam.
func UnmarshalMessage(data []byte) (Message, error) {
	var envelope struct {
		Role MessageRole `json:"role"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, newCodecError("message", "", err)
	}
	if err := validateMessageWire(envelope.Role, data); err != nil {
		return nil, newCodecError("message", string(envelope.Role), err)
	}

	switch envelope.Role {
	case MessageRoleUser:
		var message UserMessage
		if err := json.Unmarshal(data, &message); err != nil {
			return nil, wrapCodecError("message", string(envelope.Role), err)
		}
		return message, nil
	case MessageRoleAssistant:
		message, err := unmarshalAssistantMessage(data)
		if err != nil {
			return nil, wrapCodecError("message", string(envelope.Role), err)
		}
		return message, nil
	case MessageRoleToolResult:
		message, err := unmarshalToolResultMessage(data)
		if err != nil {
			return nil, wrapCodecError("message", string(envelope.Role), err)
		}
		return message, nil
	default:
		return nil, newCodecError("message", string(envelope.Role), nil)
	}
}

func marshalMessageVariant(message Message) ([]byte, error) {
	switch value := message.(type) {
	case UserMessage:
		return json.Marshal(value)
	case *UserMessage:
		return json.Marshal(value)
	case AssistantMessage:
		return marshalAssistantMessage(value)
	case *AssistantMessage:
		return marshalAssistantMessage(*value)
	case ToolResultMessage:
		return marshalToolResultMessage(value)
	case *ToolResultMessage:
		return marshalToolResultMessage(*value)
	default:
		return nil, fmt.Errorf("unsupported concrete type %T", message)
	}
}

func marshalAssistantMessage(message AssistantMessage) ([]byte, error) {
	raw, err := marshalContentSlice(message.Content)
	if err != nil {
		return nil, err
	}
	type wire AssistantMessage
	return json.Marshal(struct {
		wire
		Content []json.RawMessage `json:"content"`
	}{wire: wire(message), Content: raw})
}

func unmarshalAssistantMessage(data []byte) (AssistantMessage, error) {
	type wire AssistantMessage
	var decoded struct {
		wire
		Content []json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return AssistantMessage{}, err
	}
	message := AssistantMessage(decoded.wire)
	message.Content = make([]AssistantContent, 0, len(decoded.Content))
	for _, raw := range decoded.Content {
		content, err := UnmarshalContent(raw)
		if err != nil {
			return AssistantMessage{}, err
		}
		switch value := content.(type) {
		case TextContent:
			message.Content = append(message.Content, value)
		case ThinkingContent:
			message.Content = append(message.Content, value)
		case ToolCall:
			message.Content = append(message.Content, value)
		default:
			return AssistantMessage{}, newCodecError("assistant content", string(content.ContentType()), nil)
		}
	}
	return message, nil
}

func marshalToolResultMessage(message ToolResultMessage) ([]byte, error) {
	raw, err := marshalContentSlice(message.Content)
	if err != nil {
		return nil, err
	}
	type wire ToolResultMessage
	return json.Marshal(struct {
		wire
		Content []json.RawMessage `json:"content"`
	}{wire: wire(message), Content: raw})
}

func unmarshalToolResultMessage(data []byte) (ToolResultMessage, error) {
	type wire ToolResultMessage
	var decoded struct {
		wire
		Content []json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return ToolResultMessage{}, err
	}
	message := ToolResultMessage(decoded.wire)
	message.Content = make([]ToolResultContent, 0, len(decoded.Content))
	for _, raw := range decoded.Content {
		content, err := UnmarshalContent(raw)
		if err != nil {
			return ToolResultMessage{}, err
		}
		switch value := content.(type) {
		case TextContent:
			message.Content = append(message.Content, value)
		case ImageContent:
			message.Content = append(message.Content, value)
		default:
			return ToolResultMessage{}, newCodecError("tool-result content", string(content.ContentType()), nil)
		}
	}
	return message, nil
}

func marshalContentSlice[T interface{ Content }](content []T) ([]json.RawMessage, error) {
	raw := make([]json.RawMessage, len(content))
	for i, item := range content {
		encoded, err := MarshalContent(item)
		if err != nil {
			return nil, err
		}
		raw[i] = encoded
	}
	return raw, nil
}

func messageVariantRole(message Message) (MessageRole, bool) {
	switch message.(type) {
	case UserMessage, *UserMessage:
		return MessageRoleUser, true
	case AssistantMessage, *AssistantMessage:
		return MessageRoleAssistant, true
	case ToolResultMessage, *ToolResultMessage:
		return MessageRoleToolResult, true
	default:
		return "", false
	}
}

func wrapCodecError(surface, discriminator string, err error) error {
	if errors.Is(err, ErrCodec) {
		return err
	}
	return newCodecError(surface, discriminator, err)
}

// MarshalAssistantMessageEvent encodes one member of the closed assistant
// event union. The concrete variant, its discriminator, and terminal reason
// must agree. Nested AssistantMessage and ToolCall values use their own closed
// codecs rather than encoding interface fields loosely.
func MarshalAssistantMessageEvent(event AssistantMessageEvent) ([]byte, error) {
	want, ok := assistantMessageEventVariantType(event)
	if !ok || isNilClosedUnion(event) {
		return nil, newCodecError("assistant message event", "", fmt.Errorf("unsupported concrete type %T", event))
	}
	if got := event.AssistantMessageEventType(); got != want {
		return nil, newCodecError("assistant message event", string(got), fmt.Errorf("%T requires %q", event, want))
	}

	encoded, err := marshalAssistantMessageEventVariant(event)
	if err != nil {
		return nil, wrapCodecError("assistant message event", string(want), err)
	}
	return encoded, nil
}

// UnmarshalAssistantMessageEvent decodes one member of the closed assistant
// event union. Unknown event types and invalid terminal reasons are rejected.
func UnmarshalAssistantMessageEvent(data []byte) (AssistantMessageEvent, error) {
	var envelope struct {
		Type AssistantMessageEventType `json:"type"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, newCodecError("assistant message event", "", err)
	}
	if err := validateAssistantMessageEventWire(envelope.Type, data); err != nil {
		return nil, newCodecError("assistant message event", string(envelope.Type), err)
	}

	event, err := unmarshalAssistantMessageEventVariant(envelope.Type, data)
	if err != nil {
		return nil, wrapCodecError("assistant message event", string(envelope.Type), err)
	}
	if event == nil {
		return nil, newCodecError("assistant message event", string(envelope.Type), nil)
	}
	return event, nil
}

func assistantMessageEventVariantType(event AssistantMessageEvent) (AssistantMessageEventType, bool) {
	switch event.(type) {
	case AssistantMessageStartEvent, *AssistantMessageStartEvent:
		return AssistantMessageEventTypeStart, true
	case AssistantMessageTextStartEvent, *AssistantMessageTextStartEvent:
		return AssistantMessageEventTypeTextStart, true
	case AssistantMessageTextDeltaEvent, *AssistantMessageTextDeltaEvent:
		return AssistantMessageEventTypeTextDelta, true
	case AssistantMessageTextEndEvent, *AssistantMessageTextEndEvent:
		return AssistantMessageEventTypeTextEnd, true
	case AssistantMessageThinkingStartEvent, *AssistantMessageThinkingStartEvent:
		return AssistantMessageEventTypeThinkingStart, true
	case AssistantMessageThinkingDeltaEvent, *AssistantMessageThinkingDeltaEvent:
		return AssistantMessageEventTypeThinkingDelta, true
	case AssistantMessageThinkingEndEvent, *AssistantMessageThinkingEndEvent:
		return AssistantMessageEventTypeThinkingEnd, true
	case AssistantMessageToolCallStartEvent, *AssistantMessageToolCallStartEvent:
		return AssistantMessageEventTypeToolCallStart, true
	case AssistantMessageToolCallDeltaEvent, *AssistantMessageToolCallDeltaEvent:
		return AssistantMessageEventTypeToolCallDelta, true
	case AssistantMessageToolCallEndEvent, *AssistantMessageToolCallEndEvent:
		return AssistantMessageEventTypeToolCallEnd, true
	case AssistantMessageDoneEvent, *AssistantMessageDoneEvent:
		return AssistantMessageEventTypeDone, true
	case AssistantMessageErrorEvent, *AssistantMessageErrorEvent:
		return AssistantMessageEventTypeError, true
	default:
		return "", false
	}
}

func marshalAssistantMessageEventVariant(event AssistantMessageEvent) ([]byte, error) {
	switch value := event.(type) {
	case AssistantMessageStartEvent:
		return marshalPartialEvent(value.Type, value.Partial)
	case *AssistantMessageStartEvent:
		if value == nil {
			return nil, fmt.Errorf("nil event")
		}
		return marshalPartialEvent(value.Type, value.Partial)
	case AssistantMessageTextStartEvent:
		return marshalIndexedPartialEvent(value.Type, value.ContentIndex, value.Partial)
	case *AssistantMessageTextStartEvent:
		if value == nil {
			return nil, fmt.Errorf("nil event")
		}
		return marshalIndexedPartialEvent(value.Type, value.ContentIndex, value.Partial)
	case AssistantMessageTextDeltaEvent:
		return marshalDeltaPartialEvent(value.Type, value.ContentIndex, value.Delta, value.Partial)
	case *AssistantMessageTextDeltaEvent:
		if value == nil {
			return nil, fmt.Errorf("nil event")
		}
		return marshalDeltaPartialEvent(value.Type, value.ContentIndex, value.Delta, value.Partial)
	case AssistantMessageTextEndEvent:
		return marshalContentPartialEvent(value.Type, value.ContentIndex, value.Content, value.Partial)
	case *AssistantMessageTextEndEvent:
		if value == nil {
			return nil, fmt.Errorf("nil event")
		}
		return marshalContentPartialEvent(value.Type, value.ContentIndex, value.Content, value.Partial)
	case AssistantMessageThinkingStartEvent:
		return marshalIndexedPartialEvent(value.Type, value.ContentIndex, value.Partial)
	case *AssistantMessageThinkingStartEvent:
		if value == nil {
			return nil, fmt.Errorf("nil event")
		}
		return marshalIndexedPartialEvent(value.Type, value.ContentIndex, value.Partial)
	case AssistantMessageThinkingDeltaEvent:
		return marshalDeltaPartialEvent(value.Type, value.ContentIndex, value.Delta, value.Partial)
	case *AssistantMessageThinkingDeltaEvent:
		if value == nil {
			return nil, fmt.Errorf("nil event")
		}
		return marshalDeltaPartialEvent(value.Type, value.ContentIndex, value.Delta, value.Partial)
	case AssistantMessageThinkingEndEvent:
		return marshalContentPartialEvent(value.Type, value.ContentIndex, value.Content, value.Partial)
	case *AssistantMessageThinkingEndEvent:
		if value == nil {
			return nil, fmt.Errorf("nil event")
		}
		return marshalContentPartialEvent(value.Type, value.ContentIndex, value.Content, value.Partial)
	case AssistantMessageToolCallStartEvent:
		return marshalIndexedPartialEvent(value.Type, value.ContentIndex, value.Partial)
	case *AssistantMessageToolCallStartEvent:
		if value == nil {
			return nil, fmt.Errorf("nil event")
		}
		return marshalIndexedPartialEvent(value.Type, value.ContentIndex, value.Partial)
	case AssistantMessageToolCallDeltaEvent:
		return marshalDeltaPartialEvent(value.Type, value.ContentIndex, value.Delta, value.Partial)
	case *AssistantMessageToolCallDeltaEvent:
		if value == nil {
			return nil, fmt.Errorf("nil event")
		}
		return marshalDeltaPartialEvent(value.Type, value.ContentIndex, value.Delta, value.Partial)
	case AssistantMessageToolCallEndEvent:
		return marshalToolCallPartialEvent(value)
	case *AssistantMessageToolCallEndEvent:
		if value == nil {
			return nil, fmt.Errorf("nil event")
		}
		return marshalToolCallPartialEvent(*value)
	case AssistantMessageDoneEvent:
		return marshalDoneEvent(value)
	case *AssistantMessageDoneEvent:
		if value == nil {
			return nil, fmt.Errorf("nil event")
		}
		return marshalDoneEvent(*value)
	case AssistantMessageErrorEvent:
		return marshalErrorEvent(value)
	case *AssistantMessageErrorEvent:
		if value == nil {
			return nil, fmt.Errorf("nil event")
		}
		return marshalErrorEvent(*value)
	default:
		return nil, fmt.Errorf("unsupported concrete type %T", event)
	}
}

func marshalPartialEvent(eventType AssistantMessageEventType, partial AssistantMessage) ([]byte, error) {
	raw, err := marshalEventAssistantMessage(partial)
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Type    AssistantMessageEventType `json:"type"`
		Partial json.RawMessage           `json:"partial"`
	}{Type: eventType, Partial: raw})
}

func marshalIndexedPartialEvent(eventType AssistantMessageEventType, contentIndex int, partial AssistantMessage) ([]byte, error) {
	raw, err := marshalEventAssistantMessage(partial)
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Type         AssistantMessageEventType `json:"type"`
		ContentIndex int                       `json:"contentIndex"`
		Partial      json.RawMessage           `json:"partial"`
	}{Type: eventType, ContentIndex: contentIndex, Partial: raw})
}

func marshalDeltaPartialEvent(eventType AssistantMessageEventType, contentIndex int, delta string, partial AssistantMessage) ([]byte, error) {
	raw, err := marshalEventAssistantMessage(partial)
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Type         AssistantMessageEventType `json:"type"`
		ContentIndex int                       `json:"contentIndex"`
		Delta        string                    `json:"delta"`
		Partial      json.RawMessage           `json:"partial"`
	}{Type: eventType, ContentIndex: contentIndex, Delta: delta, Partial: raw})
}

func marshalContentPartialEvent(eventType AssistantMessageEventType, contentIndex int, content string, partial AssistantMessage) ([]byte, error) {
	raw, err := marshalEventAssistantMessage(partial)
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Type         AssistantMessageEventType `json:"type"`
		ContentIndex int                       `json:"contentIndex"`
		Content      string                    `json:"content"`
		Partial      json.RawMessage           `json:"partial"`
	}{Type: eventType, ContentIndex: contentIndex, Content: content, Partial: raw})
}

func marshalToolCallPartialEvent(event AssistantMessageToolCallEndEvent) ([]byte, error) {
	toolCall, err := MarshalContent(event.ToolCall)
	if err != nil {
		return nil, err
	}
	partial, err := marshalEventAssistantMessage(event.Partial)
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Type         AssistantMessageEventType `json:"type"`
		ContentIndex int                       `json:"contentIndex"`
		ToolCall     json.RawMessage           `json:"toolCall"`
		Partial      json.RawMessage           `json:"partial"`
	}{Type: event.Type, ContentIndex: event.ContentIndex, ToolCall: toolCall, Partial: partial})
}

func marshalDoneEvent(event AssistantMessageDoneEvent) ([]byte, error) {
	if !validDoneReason(event.Reason) {
		return nil, fmt.Errorf("invalid done reason %q", event.Reason)
	}
	message, err := marshalEventAssistantMessage(event.Message)
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Type    AssistantMessageEventType `json:"type"`
		Reason  StopReason                `json:"reason"`
		Message json.RawMessage           `json:"message"`
	}{Type: event.Type, Reason: event.Reason, Message: message})
}

func marshalErrorEvent(event AssistantMessageErrorEvent) ([]byte, error) {
	if !validErrorReason(event.Reason) {
		return nil, fmt.Errorf("invalid error reason %q", event.Reason)
	}
	message, err := marshalEventAssistantMessage(event.Error)
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Type   AssistantMessageEventType `json:"type"`
		Reason StopReason                `json:"reason"`
		Error  json.RawMessage           `json:"error"`
	}{Type: event.Type, Reason: event.Reason, Error: message})
}

func marshalEventAssistantMessage(message AssistantMessage) (json.RawMessage, error) {
	encoded, err := MarshalMessage(message)
	return json.RawMessage(encoded), err
}

func unmarshalAssistantMessageEventVariant(eventType AssistantMessageEventType, data []byte) (AssistantMessageEvent, error) {
	switch eventType {
	case AssistantMessageEventTypeStart:
		var wire struct {
			Type    AssistantMessageEventType `json:"type"`
			Partial json.RawMessage           `json:"partial"`
		}
		if err := json.Unmarshal(data, &wire); err != nil {
			return nil, err
		}
		partial, err := unmarshalEventAssistantMessage(wire.Partial)
		return AssistantMessageStartEvent{Type: wire.Type, Partial: partial}, err
	case AssistantMessageEventTypeTextStart, AssistantMessageEventTypeThinkingStart, AssistantMessageEventTypeToolCallStart:
		var wire struct {
			Type         AssistantMessageEventType `json:"type"`
			ContentIndex int                       `json:"contentIndex"`
			Partial      json.RawMessage           `json:"partial"`
		}
		if err := json.Unmarshal(data, &wire); err != nil {
			return nil, err
		}
		partial, err := unmarshalEventAssistantMessage(wire.Partial)
		if err != nil {
			return nil, err
		}
		switch eventType {
		case AssistantMessageEventTypeTextStart:
			return AssistantMessageTextStartEvent{Type: wire.Type, ContentIndex: wire.ContentIndex, Partial: partial}, nil
		case AssistantMessageEventTypeThinkingStart:
			return AssistantMessageThinkingStartEvent{Type: wire.Type, ContentIndex: wire.ContentIndex, Partial: partial}, nil
		default:
			return AssistantMessageToolCallStartEvent{Type: wire.Type, ContentIndex: wire.ContentIndex, Partial: partial}, nil
		}
	case AssistantMessageEventTypeTextDelta, AssistantMessageEventTypeThinkingDelta, AssistantMessageEventTypeToolCallDelta:
		var wire struct {
			Type         AssistantMessageEventType `json:"type"`
			ContentIndex int                       `json:"contentIndex"`
			Delta        string                    `json:"delta"`
			Partial      json.RawMessage           `json:"partial"`
		}
		if err := json.Unmarshal(data, &wire); err != nil {
			return nil, err
		}
		partial, err := unmarshalEventAssistantMessage(wire.Partial)
		if err != nil {
			return nil, err
		}
		switch eventType {
		case AssistantMessageEventTypeTextDelta:
			return AssistantMessageTextDeltaEvent{Type: wire.Type, ContentIndex: wire.ContentIndex, Delta: wire.Delta, Partial: partial}, nil
		case AssistantMessageEventTypeThinkingDelta:
			return AssistantMessageThinkingDeltaEvent{Type: wire.Type, ContentIndex: wire.ContentIndex, Delta: wire.Delta, Partial: partial}, nil
		default:
			return AssistantMessageToolCallDeltaEvent{Type: wire.Type, ContentIndex: wire.ContentIndex, Delta: wire.Delta, Partial: partial}, nil
		}
	case AssistantMessageEventTypeTextEnd, AssistantMessageEventTypeThinkingEnd:
		var wire struct {
			Type         AssistantMessageEventType `json:"type"`
			ContentIndex int                       `json:"contentIndex"`
			Content      string                    `json:"content"`
			Partial      json.RawMessage           `json:"partial"`
		}
		if err := json.Unmarshal(data, &wire); err != nil {
			return nil, err
		}
		partial, err := unmarshalEventAssistantMessage(wire.Partial)
		if err != nil {
			return nil, err
		}
		if eventType == AssistantMessageEventTypeTextEnd {
			return AssistantMessageTextEndEvent{Type: wire.Type, ContentIndex: wire.ContentIndex, Content: wire.Content, Partial: partial}, nil
		}
		return AssistantMessageThinkingEndEvent{Type: wire.Type, ContentIndex: wire.ContentIndex, Content: wire.Content, Partial: partial}, nil
	case AssistantMessageEventTypeToolCallEnd:
		var wire struct {
			Type         AssistantMessageEventType `json:"type"`
			ContentIndex int                       `json:"contentIndex"`
			ToolCall     json.RawMessage           `json:"toolCall"`
			Partial      json.RawMessage           `json:"partial"`
		}
		if err := json.Unmarshal(data, &wire); err != nil {
			return nil, err
		}
		content, err := UnmarshalContent(wire.ToolCall)
		if err != nil {
			return nil, err
		}
		toolCall, ok := content.(ToolCall)
		if !ok {
			return nil, newCodecError("tool call event content", string(content.ContentType()), nil)
		}
		partial, err := unmarshalEventAssistantMessage(wire.Partial)
		if err != nil {
			return nil, err
		}
		return AssistantMessageToolCallEndEvent{Type: wire.Type, ContentIndex: wire.ContentIndex, ToolCall: toolCall, Partial: partial}, nil
	case AssistantMessageEventTypeDone:
		var wire struct {
			Type    AssistantMessageEventType `json:"type"`
			Reason  StopReason                `json:"reason"`
			Message json.RawMessage           `json:"message"`
		}
		if err := json.Unmarshal(data, &wire); err != nil {
			return nil, err
		}
		if !validDoneReason(wire.Reason) {
			return nil, fmt.Errorf("invalid done reason %q", wire.Reason)
		}
		message, err := unmarshalEventAssistantMessage(wire.Message)
		return AssistantMessageDoneEvent{Type: wire.Type, Reason: wire.Reason, Message: message}, err
	case AssistantMessageEventTypeError:
		var wire struct {
			Type   AssistantMessageEventType `json:"type"`
			Reason StopReason                `json:"reason"`
			Error  json.RawMessage           `json:"error"`
		}
		if err := json.Unmarshal(data, &wire); err != nil {
			return nil, err
		}
		if !validErrorReason(wire.Reason) {
			return nil, fmt.Errorf("invalid error reason %q", wire.Reason)
		}
		message, err := unmarshalEventAssistantMessage(wire.Error)
		return AssistantMessageErrorEvent{Type: wire.Type, Reason: wire.Reason, Error: message}, err
	default:
		return nil, nil
	}
}

func unmarshalEventAssistantMessage(data json.RawMessage) (AssistantMessage, error) {
	message, err := UnmarshalMessage(data)
	if err != nil {
		return AssistantMessage{}, err
	}
	assistant, ok := message.(AssistantMessage)
	if !ok {
		return AssistantMessage{}, newCodecError("event assistant message", string(message.MessageRole()), nil)
	}
	return assistant, nil
}

func validDoneReason(reason StopReason) bool {
	switch reason {
	case StopReasonStop, StopReasonLength, StopReasonToolUse, StopReasonDeferred:
		return true
	default:
		return false
	}
}

func validErrorReason(reason StopReason) bool {
	return reason == StopReasonAborted || reason == StopReasonError
}

func isNilClosedUnion(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

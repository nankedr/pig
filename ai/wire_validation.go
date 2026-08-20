package ai

import (
	"bytes"
	"encoding/json"
	"fmt"
)

func validateContentWire(contentType ContentType, data []byte) error {
	fields, err := decodeWireObject(data)
	if err != nil {
		return err
	}
	switch contentType {
	case ContentTypeText:
		return requireWireString(fields, "text")
	case ContentTypeThinking:
		return requireWireString(fields, "thinking")
	case ContentTypeImage:
		if err := requireWireString(fields, "data"); err != nil {
			return err
		}
		return requireWireString(fields, "mimeType")
	case ContentTypeToolCall:
		if err := requireWireString(fields, "id"); err != nil {
			return err
		}
		if err := requireWireString(fields, "name"); err != nil {
			return err
		}
		return requireWireObject(fields, "arguments")
	default:
		return nil
	}
}

func validateMessageWire(role MessageRole, data []byte) error {
	fields, err := decodeWireObject(data)
	if err != nil {
		return err
	}
	switch role {
	case MessageRoleUser:
		if err := requireUserContent(fields, "content"); err != nil {
			return err
		}
		return requireWireNumber(fields, "timestamp")
	case MessageRoleAssistant:
		for _, name := range []string{"api", "provider", "model", "stopReason"} {
			if err := requireWireString(fields, name); err != nil {
				return err
			}
		}
		if err := requireWireArray(fields, "content"); err != nil {
			return err
		}
		if err := requireWireObject(fields, "usage"); err != nil {
			return err
		}
		return requireWireNumber(fields, "timestamp")
	case MessageRoleToolResult:
		for _, name := range []string{"toolCallId", "toolName"} {
			if err := requireWireString(fields, name); err != nil {
				return err
			}
		}
		if err := requireWireArray(fields, "content"); err != nil {
			return err
		}
		if err := requireWireBool(fields, "isError"); err != nil {
			return err
		}
		return requireWireNumber(fields, "timestamp")
	default:
		return nil
	}
}

func validateAssistantMessageEventWire(eventType AssistantMessageEventType, data []byte) error {
	fields, err := decodeWireObject(data)
	if err != nil {
		return err
	}
	switch eventType {
	case AssistantMessageEventTypeStart:
		return requireWireObject(fields, "partial")
	case AssistantMessageEventTypeTextStart, AssistantMessageEventTypeThinkingStart, AssistantMessageEventTypeToolCallStart:
		if err := requireWireNumber(fields, "contentIndex"); err != nil {
			return err
		}
		return requireWireObject(fields, "partial")
	case AssistantMessageEventTypeTextDelta, AssistantMessageEventTypeThinkingDelta, AssistantMessageEventTypeToolCallDelta:
		if err := requireWireNumber(fields, "contentIndex"); err != nil {
			return err
		}
		if err := requireWireString(fields, "delta"); err != nil {
			return err
		}
		return requireWireObject(fields, "partial")
	case AssistantMessageEventTypeTextEnd, AssistantMessageEventTypeThinkingEnd:
		if err := requireWireNumber(fields, "contentIndex"); err != nil {
			return err
		}
		if err := requireWireString(fields, "content"); err != nil {
			return err
		}
		return requireWireObject(fields, "partial")
	case AssistantMessageEventTypeToolCallEnd:
		if err := requireWireNumber(fields, "contentIndex"); err != nil {
			return err
		}
		if err := requireWireObject(fields, "toolCall"); err != nil {
			return err
		}
		return requireWireObject(fields, "partial")
	case AssistantMessageEventTypeDone:
		if err := requireWireString(fields, "reason"); err != nil {
			return err
		}
		return requireWireObject(fields, "message")
	case AssistantMessageEventTypeError:
		if err := requireWireString(fields, "reason"); err != nil {
			return err
		}
		return requireWireObject(fields, "error")
	default:
		return nil
	}
}

func decodeWireObject(data []byte) (map[string]json.RawMessage, error) {
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

func requiredWireField(fields map[string]json.RawMessage, name string) (json.RawMessage, error) {
	raw, ok := fields[name]
	trimmed := bytes.TrimSpace(raw)
	if !ok || len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil, fmt.Errorf("required field %q is missing or null", name)
	}
	return trimmed, nil
}

func requireWireString(fields map[string]json.RawMessage, name string) error {
	raw, err := requiredWireField(fields, name)
	if err != nil {
		return err
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return fmt.Errorf("field %q must be a string: %w", name, err)
	}
	return nil
}

func requireWireNumber(fields map[string]json.RawMessage, name string) error {
	raw, err := requiredWireField(fields, name)
	if err != nil {
		return err
	}
	var value float64
	if err := json.Unmarshal(raw, &value); err != nil {
		return fmt.Errorf("field %q must be a number: %w", name, err)
	}
	return nil
}

func requireWireBool(fields map[string]json.RawMessage, name string) error {
	raw, err := requiredWireField(fields, name)
	if err != nil {
		return err
	}
	var value bool
	if err := json.Unmarshal(raw, &value); err != nil {
		return fmt.Errorf("field %q must be a boolean: %w", name, err)
	}
	return nil
}

func requireWireObject(fields map[string]json.RawMessage, name string) error {
	raw, err := requiredWireField(fields, name)
	if err != nil {
		return err
	}
	if raw[0] != '{' {
		return fmt.Errorf("field %q must be an object", name)
	}
	return nil
}

func requireWireArray(fields map[string]json.RawMessage, name string) error {
	raw, err := requiredWireField(fields, name)
	if err != nil {
		return err
	}
	if raw[0] != '[' {
		return fmt.Errorf("field %q must be an array", name)
	}
	return nil
}

func requireUserContent(fields map[string]json.RawMessage, name string) error {
	raw, err := requiredWireField(fields, name)
	if err != nil {
		return err
	}
	if raw[0] != '"' && raw[0] != '[' {
		return fmt.Errorf("field %q must be a string or array", name)
	}
	return nil
}

func validateJSONSchemaRoot(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if !json.Valid(trimmed) {
		return fmt.Errorf("invalid JSON Schema")
	}
	if len(trimmed) == 0 || (trimmed[0] != '{' && !bytes.Equal(trimmed, []byte("true")) && !bytes.Equal(trimmed, []byte("false"))) {
		return fmt.Errorf("JSON Schema root must be an object or boolean")
	}
	return nil
}

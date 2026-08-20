package ai

import (
	"encoding/json"
	"fmt"
)

// MarshalContext encodes model input while preserving the closed Message
// union. A nil Messages slice is emitted as an empty required array.
func MarshalContext(value Context) ([]byte, error) {
	if value.SystemPrompt.IsNull() {
		return nil, newCodecError("context", "systemPrompt", fmt.Errorf("systemPrompt cannot be null"))
	}
	messages := make([]json.RawMessage, len(value.Messages))
	for i, message := range value.Messages {
		encoded, err := MarshalMessage(message)
		if err != nil {
			return nil, newCodecError("context", "messages", fmt.Errorf("message %d: %w", i, err))
		}
		messages[i] = encoded
	}
	if messages == nil {
		messages = []json.RawMessage{}
	}
	type wire struct {
		SystemPrompt Optional[string]  `json:"systemPrompt,omitzero"`
		Messages     []json.RawMessage `json:"messages"`
		Tools        []Tool            `json:"tools,omitempty"`
	}
	encoded, err := json.Marshal(wire{SystemPrompt: value.SystemPrompt, Messages: messages, Tools: value.Tools})
	if err != nil {
		return nil, newCodecError("context", "", err)
	}
	return encoded, nil
}

// UnmarshalContext decodes model input and rejects missing, null, or unknown
// members of the closed Message union.
func UnmarshalContext(data []byte) (Context, error) {
	fields, err := decodeWireObject(data)
	if err != nil {
		return Context{}, newCodecError("context", "", err)
	}
	if err := requireWireArray(fields, "messages"); err != nil {
		return Context{}, newCodecError("context", "messages", err)
	}
	if raw, ok := fields["systemPrompt"]; ok {
		var nullable *string
		if err := json.Unmarshal(raw, &nullable); err != nil {
			return Context{}, newCodecError("context", "systemPrompt", err)
		}
		if nullable == nil {
			return Context{}, newCodecError("context", "systemPrompt", fmt.Errorf("systemPrompt cannot be null"))
		}
	}

	var decoded struct {
		SystemPrompt Optional[string]  `json:"systemPrompt"`
		Messages     []json.RawMessage `json:"messages"`
		Tools        []Tool            `json:"tools"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return Context{}, newCodecError("context", "", err)
	}
	messages := make([]Message, len(decoded.Messages))
	for i, raw := range decoded.Messages {
		message, err := UnmarshalMessage(raw)
		if err != nil {
			return Context{}, newCodecError("context", "messages", fmt.Errorf("message %d: %w", i, err))
		}
		messages[i] = message
	}
	return Context{SystemPrompt: decoded.SystemPrompt, Messages: messages, Tools: decoded.Tools}, nil
}

func (c Context) MarshalJSON() ([]byte, error) {
	return MarshalContext(c)
}

func (c *Context) UnmarshalJSON(data []byte) error {
	decoded, err := UnmarshalContext(data)
	if err != nil {
		return err
	}
	*c = decoded
	return nil
}

package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/nankedr/pig/ai"
)

// AgentMessage is the open transcript value accepted by the Agent module. The
// three model-visible roles use ai.Message concrete values; applications may
// implement this interface with their own JSON object messages. A custom value
// stored by Agent must also provide CloneAgentMessage() AgentMessage so Agent
// can preserve its concrete type without retaining caller-owned mutable state.
type AgentMessage interface {
	MessageRole() ai.MessageRole
}

type agentMessageCloner interface {
	CloneAgentMessage() AgentMessage
}

// CustomAgentMessages names the application extension seam represented by
// AgentMessage in Go. It mirrors Pi's declaration-merge interface without
// introducing a runtime extension registry.
type CustomAgentMessages = AgentMessage

// RawAgentMessage is the lossless fallback produced when an Agent message has
// an application-defined role. Its bytes are copied on construction and read.
type RawAgentMessage struct {
	role string
	raw  json.RawMessage
}

// NewRawAgentMessage validates and copies one application-defined Agent
// message. Built-in roles must use their corresponding ai.Message variants.
func NewRawAgentMessage(raw json.RawMessage) (RawAgentMessage, error) {
	role, err := agentMessageRole(raw)
	if err != nil {
		return RawAgentMessage{}, err
	}
	if isModelMessageRole(role) {
		return RawAgentMessage{}, fmt.Errorf("agent message role %q is closed; use an ai.Message variant", role)
	}
	return RawAgentMessage{role: role, raw: append(json.RawMessage(nil), raw...)}, nil
}

// AgentMessageRole returns the application-defined role discriminator.
func (m RawAgentMessage) MessageRole() ai.MessageRole {
	return ai.MessageRole(m.role)
}

// RawJSON returns an independent copy of the original JSON representation.
func (m RawAgentMessage) RawJSON() json.RawMessage {
	return append(json.RawMessage(nil), m.raw...)
}

// MarshalJSON preserves the exact bytes supplied to NewRawAgentMessage.
func (m RawAgentMessage) MarshalJSON() ([]byte, error) {
	if _, err := agentMessageRole(m.raw); err != nil {
		return nil, err
	}
	return append([]byte(nil), m.raw...), nil
}

// UnmarshalJSON accepts only application-defined roles.
func (m *RawAgentMessage) UnmarshalJSON(data []byte) error {
	decoded, err := NewRawAgentMessage(data)
	if err != nil {
		return err
	}
	*m = decoded
	return nil
}

// MarshalAgentMessage encodes a model message through the closed ai codec and
// an application message through the open JSON-object boundary.
func MarshalAgentMessage(message AgentMessage) ([]byte, error) {
	switch message := message.(type) {
	case RawAgentMessage:
		return message.MarshalJSON()
	case *RawAgentMessage:
		if message == nil {
			return nil, fmt.Errorf("unsupported nil Agent message")
		}
		return message.MarshalJSON()
	case ai.Message:
		return ai.MarshalMessage(message)
	case nil:
		return nil, fmt.Errorf("unsupported nil Agent message")
	default:
		encoded, err := json.Marshal(message)
		if err != nil {
			return nil, fmt.Errorf("marshal Agent message: %w", err)
		}
		role, err := agentMessageRole(encoded)
		if err != nil {
			return nil, err
		}
		if ai.MessageRole(role) != message.MessageRole() {
			return nil, fmt.Errorf("custom Agent message role %q does not match encoded role %q", message.MessageRole(), role)
		}
		if isModelMessageRole(role) {
			return nil, fmt.Errorf("custom Agent message cannot claim closed role %q", role)
		}
		return encoded, nil
	}
}

// UnmarshalAgentMessage decodes built-in roles with ai's strict codec and
// retains unknown roles as RawAgentMessage without normalizing their JSON.
func UnmarshalAgentMessage(data []byte) (AgentMessage, error) {
	role, err := agentMessageRole(data)
	if err != nil {
		return nil, err
	}
	if isModelMessageRole(role) {
		message, err := ai.UnmarshalMessage(data)
		if err != nil {
			return nil, err
		}
		return message, nil
	}
	return RawAgentMessage{role: role, raw: append(json.RawMessage(nil), data...)}, nil
}

// DefaultConvertToLLM keeps the three built-in ai.Message variants and
// filters application-only messages. It does not transform Agent context.
func DefaultConvertToLLM(_ context.Context, messages []AgentMessage) ([]ai.Message, error) {
	converted := make([]ai.Message, 0, len(messages))
	for _, message := range messages {
		if modelMessage, ok := message.(ai.Message); ok {
			converted = append(converted, modelMessage)
		}
	}
	return converted, nil
}

func agentMessageRole(data []byte) (string, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || trimmed[0] != '{' || trimmed[len(trimmed)-1] != '}' {
		return "", fmt.Errorf("Agent message must be a JSON object")
	}
	var envelope struct {
		Role *string `json:"role"`
	}
	if err := json.Unmarshal(trimmed, &envelope); err != nil {
		return "", fmt.Errorf("decode Agent message: %w", err)
	}
	if envelope.Role == nil || *envelope.Role == "" {
		return "", fmt.Errorf("Agent message requires a non-empty string role")
	}
	return *envelope.Role, nil
}

func isModelMessageRole(role string) bool {
	switch ai.MessageRole(role) {
	case ai.MessageRoleUser, ai.MessageRoleAssistant, ai.MessageRoleToolResult:
		return true
	default:
		return false
	}
}

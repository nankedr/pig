package ai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
)

// MessageRole is the published message discriminator.
type MessageRole string

const (
	MessageRoleUser       MessageRole = "user"
	MessageRoleAssistant  MessageRole = "assistant"
	MessageRoleToolResult MessageRole = "toolResult"
)

// Message is the closed set of model-context messages. Application-specific
// Agent messages form a separate open extension seam in package agent.
type Message interface {
	message()
	MessageRole() MessageRole
}

// UserMessageContent preserves Pi's string-or-content-block union without
// exposing an untyped any to callers.
type UserMessageContent struct {
	text   string
	blocks []UserContent
	isText bool
}

// UserText constructs user content in its compact string form.
func UserText(text string) UserMessageContent {
	return UserMessageContent{text: text, isText: true}
}

// UserBlocks constructs user content from the closed Text/Image block set.
func UserBlocks(blocks ...UserContent) UserMessageContent {
	return UserMessageContent{blocks: cloneUserContentSlice(blocks)}
}

// Text returns compact text content and whether this is the string variant.
func (c UserMessageContent) Text() (string, bool) {
	return c.text, c.isText
}

// Blocks returns a defensive copy and whether this is the block-list variant.
func (c UserMessageContent) Blocks() ([]UserContent, bool) {
	if c.isText {
		return nil, false
	}
	return cloneUserContentSlice(c.blocks), true
}

func (c UserMessageContent) MarshalJSON() ([]byte, error) {
	if c.isText {
		return json.Marshal(c.text)
	}
	raw := make([]json.RawMessage, len(c.blocks))
	for i, block := range c.blocks {
		encoded, err := MarshalContent(block)
		if err != nil {
			return nil, err
		}
		raw[i] = encoded
	}
	return json.Marshal(raw)
}

func (c *UserMessageContent) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return newCodecError("user content", "", fmt.Errorf("empty JSON"))
	}
	if trimmed[0] == '"' {
		var text string
		if err := json.Unmarshal(trimmed, &text); err != nil {
			return newCodecError("user content", "string", err)
		}
		*c = UserText(text)
		return nil
	}
	if trimmed[0] != '[' {
		return newCodecError("user content", string(trimmed), nil)
	}
	var raw []json.RawMessage
	if err := json.Unmarshal(trimmed, &raw); err != nil {
		return newCodecError("user content", "blocks", err)
	}
	blocks := make([]UserContent, 0, len(raw))
	for _, item := range raw {
		content, err := UnmarshalContent(item)
		if err != nil {
			return err
		}
		switch block := content.(type) {
		case TextContent:
			blocks = append(blocks, block)
		case ImageContent:
			blocks = append(blocks, block)
		default:
			return newCodecError("user content", string(content.ContentType()), fmt.Errorf("variant is not valid for a user message"))
		}
	}
	*c = UserBlocks(blocks...)
	return nil
}

// UserMessage is a user turn in model context.
type UserMessage struct {
	Role      MessageRole        `json:"role"`
	Content   UserMessageContent `json:"content"`
	Timestamp int64              `json:"timestamp"`
}

func (UserMessage) message()                   {}
func (m UserMessage) MessageRole() MessageRole { return m.Role }

// DiagnosticErrorInfo is the redacted error portion of an assistant diagnostic.
type DiagnosticErrorInfo struct {
	Name    Optional[string]    `json:"name,omitzero"`
	Message string              `json:"message"`
	Stack   Optional[string]    `json:"stack,omitzero"`
	Code    Optional[JSONValue] `json:"code,omitzero"`
}

// AssistantMessageDiagnostic records redacted provider/runtime diagnostics.
type AssistantMessageDiagnostic struct {
	Type      string                        `json:"type"`
	Timestamp int64                         `json:"timestamp"`
	Error     Optional[DiagnosticErrorInfo] `json:"error,omitzero"`
	Details   Optional[map[string]any]      `json:"details,omitzero"`
}

// AssistantMessage is both the accumulated stream snapshot and final outcome.
type AssistantMessage struct {
	Role          MessageRole                            `json:"role"`
	Content       []AssistantContent                     `json:"content"`
	API           API                                    `json:"api"`
	Provider      ProviderID                             `json:"provider"`
	Model         string                                 `json:"model"`
	ResponseModel Optional[string]                       `json:"responseModel,omitzero"`
	ResponseID    Optional[string]                       `json:"responseId,omitzero"`
	Diagnostics   Optional[[]AssistantMessageDiagnostic] `json:"diagnostics,omitzero"`
	Usage         Usage                                  `json:"usage"`
	StopReason    StopReason                             `json:"stopReason"`
	Deferred      Optional[DeferredHandle]               `json:"deferred,omitzero"`
	ErrorMessage  Optional[string]                       `json:"errorMessage,omitzero"`
	RawStopReason Optional[string]                       `json:"rawStopReason,omitzero"`
	EndTurn       Optional[bool]                         `json:"endTurn,omitzero"`
	Timestamp     int64                                  `json:"timestamp"`
}

func (AssistantMessage) message()                   {}
func (m AssistantMessage) MessageRole() MessageRole { return m.Role }

// ToolResultMessage reports the result of a prior ToolCall. Details remain a
// JSON-compatible open value because each Tool owns its details schema.
type ToolResultMessage struct {
	Role           MessageRole         `json:"role"`
	ToolCallID     string              `json:"toolCallId"`
	ToolName       string              `json:"toolName"`
	Content        []ToolResultContent `json:"content"`
	Details        Optional[JSONValue] `json:"details,omitzero"`
	Usage          Optional[Usage]     `json:"usage,omitzero"`
	AddedToolNames Optional[[]string]  `json:"addedToolNames,omitzero"`
	IsError        bool                `json:"isError"`
	Timestamp      int64               `json:"timestamp"`
}

func (ToolResultMessage) message()                   {}
func (m ToolResultMessage) MessageRole() MessageRole { return m.Role }

// CloneAssistantMessage returns a deep-enough immutable snapshot of all slices,
// maps, optional records, and content blocks exposed by AssistantMessage.
func CloneAssistantMessage(message AssistantMessage) AssistantMessage {
	clone := message
	clone.Content = make([]AssistantContent, len(message.Content))
	for i, content := range message.Content {
		clone.Content[i] = cloneAssistantContent(content)
	}
	clone.Diagnostics = cloneOptional(message.Diagnostics, func(values []AssistantMessageDiagnostic) []AssistantMessageDiagnostic {
		result := append([]AssistantMessageDiagnostic(nil), values...)
		for i := range result {
			result[i].Details = cloneOptional(result[i].Details, cloneStringAnyMap)
			result[i].Error = cloneOptional(result[i].Error, func(info DiagnosticErrorInfo) DiagnosticErrorInfo {
				info.Code = cloneOptional(info.Code, cloneJSONValue)
				return info
			})
		}
		return result
	})
	clone.Deferred = cloneOptional(message.Deferred, func(handle DeferredHandle) DeferredHandle {
		handle.Data = cloneOptional(handle.Data, cloneJSONValue)
		return handle
	})
	return clone
}

func cloneAssistantContent(content AssistantContent) AssistantContent {
	switch value := content.(type) {
	case TextContent:
		return value
	case *TextContent:
		if value == nil {
			return nil
		}
		clone := *value
		return &clone
	case ThinkingContent:
		return value
	case *ThinkingContent:
		if value == nil {
			return nil
		}
		clone := *value
		return &clone
	case ToolCall:
		value.Arguments = cloneStringAnyMap(value.Arguments)
		return value
	case *ToolCall:
		if value == nil {
			return nil
		}
		clone := *value
		clone.Arguments = cloneStringAnyMap(value.Arguments)
		return &clone
	default:
		return content
	}
}

func cloneOptional[T any](value Optional[T], clone func(T) T) Optional[T] {
	if !value.IsSet() {
		return Absent[T]()
	}
	if value.IsNull() {
		return Null[T]()
	}
	inner, _ := value.Value()
	return Some(clone(inner))
}

func cloneStringAnyMap(value map[string]any) map[string]any {
	if value == nil {
		return nil
	}
	clone := make(map[string]any, len(value))
	for key, item := range value {
		clone[key] = cloneJSONValue(item)
	}
	return clone
}

func cloneJSONValue(value JSONValue) JSONValue {
	if value == nil {
		return nil
	}
	if raw, ok := value.(json.RawMessage); ok {
		return append(json.RawMessage(nil), raw...)
	}
	return cloneDynamicValue(reflect.ValueOf(value)).Interface()
}

func cloneDynamicValue(value reflect.Value) reflect.Value {
	if !value.IsValid() {
		return value
	}
	switch value.Kind() {
	case reflect.Interface:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		cloned := cloneDynamicValue(value.Elem())
		result := reflect.New(value.Type()).Elem()
		result.Set(cloned)
		return result
	case reflect.Pointer:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		result := reflect.New(value.Type().Elem())
		result.Elem().Set(cloneDynamicValue(value.Elem()))
		return result
	case reflect.Map:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		result := reflect.MakeMapWithSize(value.Type(), value.Len())
		iter := value.MapRange()
		for iter.Next() {
			result.SetMapIndex(cloneDynamicValue(iter.Key()), cloneDynamicValue(iter.Value()))
		}
		return result
	case reflect.Slice:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		result := reflect.MakeSlice(value.Type(), value.Len(), value.Len())
		for i := 0; i < value.Len(); i++ {
			result.Index(i).Set(cloneDynamicValue(value.Index(i)))
		}
		return result
	case reflect.Array:
		result := reflect.New(value.Type()).Elem()
		for i := 0; i < value.Len(); i++ {
			result.Index(i).Set(cloneDynamicValue(value.Index(i)))
		}
		return result
	default:
		return value
	}
}

func cloneUserContentSlice(blocks []UserContent) []UserContent {
	result := make([]UserContent, len(blocks))
	for i, block := range blocks {
		switch value := block.(type) {
		case TextContent:
			result[i] = value
		case *TextContent:
			if value != nil {
				clone := *value
				result[i] = &clone
			}
		case ImageContent:
			result[i] = value
		case *ImageContent:
			if value != nil {
				clone := *value
				result[i] = &clone
			}
		default:
			result[i] = block
		}
	}
	return result
}

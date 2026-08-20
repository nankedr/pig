package ai

// JSONValue is a JSON-compatible value. Runtime schema validation remains the
// authority for values such as tool-call arguments.
type JSONValue = any

// ContentType is the published content-block discriminator.
type ContentType string

const (
	ContentTypeText     ContentType = "text"
	ContentTypeThinking ContentType = "thinking"
	ContentTypeImage    ContentType = "image"
	ContentTypeToolCall ContentType = "toolCall"
)

// Content is the closed set of AI content blocks. More specific message roles
// expose narrower closed sets below.
type Content interface {
	content()
	ContentType() ContentType
}

// UserContent is the closed set accepted by a user message.
type UserContent interface {
	Content
	userContent()
}

// AssistantContent is the closed set produced by an assistant message.
type AssistantContent interface {
	Content
	assistantContent()
}

// ToolResultContent is the closed set accepted by a tool-result message.
type ToolResultContent interface {
	Content
	toolResultContent()
}

// TextContent is a text block. Type must be ContentTypeText on the wire.
type TextContent struct {
	Type          ContentType      `json:"type"`
	Text          string           `json:"text"`
	TextSignature Optional[string] `json:"textSignature,omitzero"`
}

func (TextContent) content()           {}
func (TextContent) userContent()       {}
func (TextContent) assistantContent()  {}
func (TextContent) toolResultContent() {}
func (c TextContent) ContentType() ContentType {
	return c.Type
}

// ThinkingContent is an assistant reasoning block.
type ThinkingContent struct {
	Type              ContentType      `json:"type"`
	Thinking          string           `json:"thinking"`
	ThinkingSignature Optional[string] `json:"thinkingSignature,omitzero"`
	Redacted          Optional[bool]   `json:"redacted,omitzero"`
}

func (ThinkingContent) content()          {}
func (ThinkingContent) assistantContent() {}
func (c ThinkingContent) ContentType() ContentType {
	return c.Type
}

// ImageContent is a base64-encoded image block.
type ImageContent struct {
	Type     ContentType `json:"type"`
	Data     string      `json:"data"`
	MIMEType string      `json:"mimeType"`
}

func (ImageContent) content()           {}
func (ImageContent) userContent()       {}
func (ImageContent) toolResultContent() {}
func (c ImageContent) ContentType() ContentType {
	return c.Type
}

// ToolCall requests a named tool invocation. Arguments retain their dynamic
// JSON shape; the Tool's original JSON Schema remains the validation authority.
type ToolCall struct {
	Type             ContentType      `json:"type"`
	ID               string           `json:"id"`
	Name             string           `json:"name"`
	Arguments        map[string]any   `json:"arguments"`
	ThoughtSignature Optional[string] `json:"thoughtSignature,omitzero"`
	Namespace        Optional[string] `json:"namespace,omitzero"`
}

func (ToolCall) content()          {}
func (ToolCall) assistantContent() {}
func (c ToolCall) ContentType() ContentType {
	return c.Type
}

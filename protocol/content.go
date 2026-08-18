package protocol

// JSONValue is a JSON-compatible wire value. Runtime validation is deferred
// to the corresponding schema descriptor.
type JSONValue = any

// ContentType is a content-block wire discriminator.
type ContentType string

const (
	ContentTypeText     ContentType = "text"
	ContentTypeThinking ContentType = "thinking"
	ContentTypeImage    ContentType = "image"
	ContentTypeToolCall ContentType = "toolCall"
)

// UserContent is a closed union of user content blocks.
type UserContent interface {
	userContent()
}

// AssistantContent is a closed union of assistant content blocks.
type AssistantContent interface {
	assistantContent()
}

// ToolContent is a closed union of tool-result content blocks.
type ToolContent interface {
	toolContent()
}

// TextContent is a text content block.
type TextContent struct {
	Type ContentType `json:"type"`
	Text string      `json:"text"`
}

func (TextContent) userContent()      {}
func (TextContent) assistantContent() {}
func (TextContent) toolContent()      {}

// ThinkingContent is a reasoning content block.
type ThinkingContent struct {
	Type     ContentType    `json:"type"`
	Thinking string         `json:"thinking"`
	Redacted Optional[bool] `json:"redacted"`
}

func (ThinkingContent) assistantContent() {}

// ImageContent is a base64-encoded image content block.
type ImageContent struct {
	Type     ContentType `json:"type"`
	Data     string      `json:"data"`
	MIMEType string      `json:"mimeType"`
}

func (ImageContent) userContent() {}
func (ImageContent) toolContent() {}

// ToolCallContent requests a named tool invocation.
type ToolCallContent struct {
	Type       ContentType `json:"type"`
	ToolCallID string      `json:"toolCallId"`
	ToolName   string      `json:"toolName"`
	Input      JSONValue   `json:"input"`
}

func (ToolCallContent) assistantContent() {}

// UsageCost contains the aggregate cost of one assistant response or tool
// invocation.
type UsageCost struct {
	Input      float64 `json:"input"`
	Output     float64 `json:"output"`
	CacheRead  float64 `json:"cacheRead"`
	CacheWrite float64 `json:"cacheWrite"`
	Total      float64 `json:"total"`
}

// Usage contains token counts and cost for one response or tool invocation.
type Usage struct {
	Input       int64           `json:"input"`
	Output      int64           `json:"output"`
	CacheRead   int64           `json:"cacheRead"`
	CacheWrite  int64           `json:"cacheWrite"`
	Reasoning   Optional[int64] `json:"reasoning"`
	TotalTokens int64           `json:"totalTokens"`
	Cost        UsageCost       `json:"cost"`
}

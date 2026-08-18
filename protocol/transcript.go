package protocol

// TranscriptRole is a transcript-item wire discriminator.
type TranscriptRole string

const (
	TranscriptRoleUser      TranscriptRole = "user"
	TranscriptRoleAssistant TranscriptRole = "assistant"
	TranscriptRoleTool      TranscriptRole = "tool"
)

// TranscriptItem is a closed union of transcript item variants.
type TranscriptItem interface {
	transcriptItem()
}

// AssistantTranscriptItem is a closed union of assistant transcript states.
type AssistantTranscriptItem interface {
	TranscriptItem
	assistantTranscriptItem()
}

// ToolTranscriptItem is a closed union of tool transcript states.
type ToolTranscriptItem interface {
	TranscriptItem
	toolTranscriptItem()
}

// UpdatedTranscriptItem is a transcript item permitted in item_updated
// progress.
type UpdatedTranscriptItem interface {
	TranscriptItem
	updatedTranscriptItem()
}

// FinishedTranscriptItem is a terminal transcript item permitted in
// item_finished progress.
type FinishedTranscriptItem interface {
	TranscriptItem
	finishedTranscriptItem()
}

// UserTranscriptItem records one user turn.
type UserTranscriptItem struct {
	ID        string         `json:"id"`
	Role      TranscriptRole `json:"role"`
	Content   []UserContent  `json:"content"`
	Timestamp int64          `json:"timestamp"`
}

func (UserTranscriptItem) transcriptItem() {}

// AssistantTranscriptStatus is the status discriminator for assistant items.
type AssistantTranscriptStatus string

const (
	AssistantTranscriptStatusStreaming AssistantTranscriptStatus = "streaming"
	AssistantTranscriptStatusComplete  AssistantTranscriptStatus = "complete"
	AssistantTranscriptStatusError     AssistantTranscriptStatus = "error"
	AssistantTranscriptStatusAborted   AssistantTranscriptStatus = "aborted"
)

// AssistantStopReason is the terminal reason for an assistant item.
type AssistantStopReason string

const (
	AssistantStopReasonStop    AssistantStopReason = "stop"
	AssistantStopReasonLength  AssistantStopReason = "length"
	AssistantStopReasonToolUse AssistantStopReason = "toolUse"
	AssistantStopReasonError   AssistantStopReason = "error"
	AssistantStopReasonAborted AssistantStopReason = "aborted"
)

// StreamingAssistantTranscriptItem is an assistant item still being emitted.
type StreamingAssistantTranscriptItem struct {
	ID            string                    `json:"id"`
	Role          TranscriptRole            `json:"role"`
	Content       []AssistantContent        `json:"content"`
	Model         ModelRef                  `json:"model"`
	ResponseModel Optional[string]          `json:"responseModel"`
	Usage         Optional[Usage]           `json:"usage"`
	Timestamp     int64                     `json:"timestamp"`
	Status        AssistantTranscriptStatus `json:"status"`
}

func (StreamingAssistantTranscriptItem) transcriptItem()          {}
func (StreamingAssistantTranscriptItem) assistantTranscriptItem() {}
func (StreamingAssistantTranscriptItem) updatedTranscriptItem()   {}

// CompleteAssistantTranscriptItem is a successfully completed assistant item.
type CompleteAssistantTranscriptItem struct {
	ID            string                    `json:"id"`
	Role          TranscriptRole            `json:"role"`
	Content       []AssistantContent        `json:"content"`
	Model         ModelRef                  `json:"model"`
	ResponseModel Optional[string]          `json:"responseModel"`
	Usage         Optional[Usage]           `json:"usage"`
	Timestamp     int64                     `json:"timestamp"`
	Status        AssistantTranscriptStatus `json:"status"`
	StopReason    AssistantStopReason       `json:"stopReason"`
}

func (CompleteAssistantTranscriptItem) transcriptItem()          {}
func (CompleteAssistantTranscriptItem) assistantTranscriptItem() {}
func (CompleteAssistantTranscriptItem) updatedTranscriptItem()   {}
func (CompleteAssistantTranscriptItem) finishedTranscriptItem()  {}

// ErrorAssistantTranscriptItem is an assistant item that failed.
type ErrorAssistantTranscriptItem struct {
	ID            string                    `json:"id"`
	Role          TranscriptRole            `json:"role"`
	Content       []AssistantContent        `json:"content"`
	Model         ModelRef                  `json:"model"`
	ResponseModel Optional[string]          `json:"responseModel"`
	Usage         Optional[Usage]           `json:"usage"`
	Timestamp     int64                     `json:"timestamp"`
	Status        AssistantTranscriptStatus `json:"status"`
	StopReason    AssistantStopReason       `json:"stopReason"`
	ErrorMessage  Optional[string]          `json:"errorMessage"`
}

func (ErrorAssistantTranscriptItem) transcriptItem()          {}
func (ErrorAssistantTranscriptItem) assistantTranscriptItem() {}
func (ErrorAssistantTranscriptItem) updatedTranscriptItem()   {}
func (ErrorAssistantTranscriptItem) finishedTranscriptItem()  {}

// AbortedAssistantTranscriptItem is an assistant item stopped by a caller.
type AbortedAssistantTranscriptItem struct {
	ID            string                    `json:"id"`
	Role          TranscriptRole            `json:"role"`
	Content       []AssistantContent        `json:"content"`
	Model         ModelRef                  `json:"model"`
	ResponseModel Optional[string]          `json:"responseModel"`
	Usage         Optional[Usage]           `json:"usage"`
	Timestamp     int64                     `json:"timestamp"`
	Status        AssistantTranscriptStatus `json:"status"`
	StopReason    AssistantStopReason       `json:"stopReason"`
	ErrorMessage  Optional[string]          `json:"errorMessage"`
}

func (AbortedAssistantTranscriptItem) transcriptItem()          {}
func (AbortedAssistantTranscriptItem) assistantTranscriptItem() {}
func (AbortedAssistantTranscriptItem) updatedTranscriptItem()   {}
func (AbortedAssistantTranscriptItem) finishedTranscriptItem()  {}

// ToolTranscriptStatus is the status discriminator for tool items.
type ToolTranscriptStatus string

const (
	ToolTranscriptStatusRunning  ToolTranscriptStatus = "running"
	ToolTranscriptStatusComplete ToolTranscriptStatus = "complete"
	ToolTranscriptStatusError    ToolTranscriptStatus = "error"
)

// RunningToolTranscriptItem is a tool item still being evaluated.
type RunningToolTranscriptItem struct {
	ID         string               `json:"id"`
	Role       TranscriptRole       `json:"role"`
	ToolCallID string               `json:"toolCallId"`
	ToolName   string               `json:"toolName"`
	Input      JSONValue            `json:"input"`
	Content    []ToolContent        `json:"content"`
	Details    Optional[JSONValue]  `json:"details"`
	Usage      Optional[Usage]      `json:"usage"`
	Timestamp  int64                `json:"timestamp"`
	Status     ToolTranscriptStatus `json:"status"`
	IsError    bool                 `json:"isError"`
}

func (RunningToolTranscriptItem) transcriptItem()        {}
func (RunningToolTranscriptItem) toolTranscriptItem()    {}
func (RunningToolTranscriptItem) updatedTranscriptItem() {}

// CompleteToolTranscriptItem is a successfully completed tool item.
type CompleteToolTranscriptItem struct {
	ID         string               `json:"id"`
	Role       TranscriptRole       `json:"role"`
	ToolCallID string               `json:"toolCallId"`
	ToolName   string               `json:"toolName"`
	Input      JSONValue            `json:"input"`
	Content    []ToolContent        `json:"content"`
	Details    Optional[JSONValue]  `json:"details"`
	Usage      Optional[Usage]      `json:"usage"`
	Timestamp  int64                `json:"timestamp"`
	Status     ToolTranscriptStatus `json:"status"`
	IsError    bool                 `json:"isError"`
}

func (CompleteToolTranscriptItem) transcriptItem()         {}
func (CompleteToolTranscriptItem) toolTranscriptItem()     {}
func (CompleteToolTranscriptItem) updatedTranscriptItem()  {}
func (CompleteToolTranscriptItem) finishedTranscriptItem() {}

// ErrorToolTranscriptItem is a failed tool item.
type ErrorToolTranscriptItem struct {
	ID         string               `json:"id"`
	Role       TranscriptRole       `json:"role"`
	ToolCallID string               `json:"toolCallId"`
	ToolName   string               `json:"toolName"`
	Input      JSONValue            `json:"input"`
	Content    []ToolContent        `json:"content"`
	Details    Optional[JSONValue]  `json:"details"`
	Usage      Optional[Usage]      `json:"usage"`
	Timestamp  int64                `json:"timestamp"`
	Status     ToolTranscriptStatus `json:"status"`
	IsError    bool                 `json:"isError"`
}

func (ErrorToolTranscriptItem) transcriptItem()         {}
func (ErrorToolTranscriptItem) toolTranscriptItem()     {}
func (ErrorToolTranscriptItem) updatedTranscriptItem()  {}
func (ErrorToolTranscriptItem) finishedTranscriptItem() {}

// TranscriptProgressType is an incremental transcript activity discriminator.
type TranscriptProgressType string

const (
	TranscriptProgressTypeItemStarted    TranscriptProgressType = "item_started"
	TranscriptProgressTypeAssistantDelta TranscriptProgressType = "assistant_delta"
	TranscriptProgressTypeItemUpdated    TranscriptProgressType = "item_updated"
	TranscriptProgressTypeItemFinished   TranscriptProgressType = "item_finished"
)

// AssistantDeltaKind identifies the assistant block receiving a delta.
type AssistantDeltaKind string

const (
	AssistantDeltaKindText     AssistantDeltaKind = "text"
	AssistantDeltaKindThinking AssistantDeltaKind = "thinking"
	AssistantDeltaKindToolCall AssistantDeltaKind = "toolCall"
)

// TranscriptProgress is a closed union of incremental transcript activity.
type TranscriptProgress interface {
	transcriptProgress()
	TranscriptProgressType() TranscriptProgressType
}

// ItemStartedProgress reports a newly started transcript item.
type ItemStartedProgress struct {
	Type TranscriptProgressType `json:"type"`
	Item TranscriptItem         `json:"item"`
}

func (ItemStartedProgress) transcriptProgress()                              {}
func (p ItemStartedProgress) TranscriptProgressType() TranscriptProgressType { return p.Type }

// AssistantDeltaProgress reports an incremental assistant content delta.
type AssistantDeltaProgress struct {
	Type         TranscriptProgressType `json:"type"`
	MessageID    string                 `json:"messageId"`
	ContentIndex int64                  `json:"contentIndex"`
	Kind         AssistantDeltaKind     `json:"kind"`
	Delta        string                 `json:"delta"`
}

func (AssistantDeltaProgress) transcriptProgress()                              {}
func (p AssistantDeltaProgress) TranscriptProgressType() TranscriptProgressType { return p.Type }

// ItemUpdatedProgress replaces a streaming assistant or tool item.
type ItemUpdatedProgress struct {
	Type TranscriptProgressType `json:"type"`
	Item UpdatedTranscriptItem  `json:"item"`
}

func (ItemUpdatedProgress) transcriptProgress()                              {}
func (p ItemUpdatedProgress) TranscriptProgressType() TranscriptProgressType { return p.Type }

// ItemFinishedProgress reports a terminal assistant or tool item.
type ItemFinishedProgress struct {
	Type TranscriptProgressType `json:"type"`
	Item FinishedTranscriptItem `json:"item"`
}

func (ItemFinishedProgress) transcriptProgress()                              {}
func (p ItemFinishedProgress) TranscriptProgressType() TranscriptProgressType { return p.Type }

package ai

type PiMessagesRewriteImpact struct {
	PolicyID            string `json:"policyId"`
	PolicyVersion       int    `json:"policyVersion"`
	Changed             bool   `json:"changed"`
	TokenCountChange    int    `json:"tokenCountChange"`
	MessageCountChange  int    `json:"messageCountChange"`
	SystemPromptChanged bool   `json:"systemPromptChanged"`
}

type PiMessagesEvent interface {
	piMessagesEvent()
	PiMessagesEventType() AssistantMessageEventType
}

type PiMessagesStartEvent struct {
	Type AssistantMessageEventType `json:"type"`
}

func (PiMessagesStartEvent) piMessagesEvent()                                 {}
func (e PiMessagesStartEvent) PiMessagesEventType() AssistantMessageEventType { return e.Type }

type PiMessagesTextStartEvent struct {
	Type         AssistantMessageEventType `json:"type"`
	ContentIndex int                       `json:"contentIndex"`
}

func (PiMessagesTextStartEvent) piMessagesEvent()                                 {}
func (e PiMessagesTextStartEvent) PiMessagesEventType() AssistantMessageEventType { return e.Type }

type PiMessagesTextDeltaEvent struct {
	Type         AssistantMessageEventType `json:"type"`
	ContentIndex int                       `json:"contentIndex"`
	Delta        string                    `json:"delta"`
}

func (PiMessagesTextDeltaEvent) piMessagesEvent()                                 {}
func (e PiMessagesTextDeltaEvent) PiMessagesEventType() AssistantMessageEventType { return e.Type }

type PiMessagesTextEndEvent struct {
	Type             AssistantMessageEventType `json:"type"`
	ContentIndex     int                       `json:"contentIndex"`
	Content          string                    `json:"content"`
	ContentSignature Optional[string]          `json:"contentSignature,omitzero"`
}

func (PiMessagesTextEndEvent) piMessagesEvent()                                 {}
func (e PiMessagesTextEndEvent) PiMessagesEventType() AssistantMessageEventType { return e.Type }

type PiMessagesThinkingStartEvent struct {
	Type         AssistantMessageEventType `json:"type"`
	ContentIndex int                       `json:"contentIndex"`
}

func (PiMessagesThinkingStartEvent) piMessagesEvent()                                 {}
func (e PiMessagesThinkingStartEvent) PiMessagesEventType() AssistantMessageEventType { return e.Type }

type PiMessagesThinkingDeltaEvent struct {
	Type         AssistantMessageEventType `json:"type"`
	ContentIndex int                       `json:"contentIndex"`
	Delta        string                    `json:"delta"`
}

func (PiMessagesThinkingDeltaEvent) piMessagesEvent()                                 {}
func (e PiMessagesThinkingDeltaEvent) PiMessagesEventType() AssistantMessageEventType { return e.Type }

type PiMessagesThinkingEndEvent struct {
	Type             AssistantMessageEventType `json:"type"`
	ContentIndex     int                       `json:"contentIndex"`
	Content          string                    `json:"content"`
	ContentSignature Optional[string]          `json:"contentSignature,omitzero"`
	Redacted         Optional[bool]            `json:"redacted,omitzero"`
}

func (PiMessagesThinkingEndEvent) piMessagesEvent()                                 {}
func (e PiMessagesThinkingEndEvent) PiMessagesEventType() AssistantMessageEventType { return e.Type }

type PiMessagesToolCallStartEvent struct {
	Type         AssistantMessageEventType `json:"type"`
	ContentIndex int                       `json:"contentIndex"`
	ID           string                    `json:"id"`
	ToolName     string                    `json:"toolName"`
}

func (PiMessagesToolCallStartEvent) piMessagesEvent()                                 {}
func (e PiMessagesToolCallStartEvent) PiMessagesEventType() AssistantMessageEventType { return e.Type }

type PiMessagesToolCallDeltaEvent struct {
	Type         AssistantMessageEventType `json:"type"`
	ContentIndex int                       `json:"contentIndex"`
	Delta        string                    `json:"delta"`
}

func (PiMessagesToolCallDeltaEvent) piMessagesEvent()                                 {}
func (e PiMessagesToolCallDeltaEvent) PiMessagesEventType() AssistantMessageEventType { return e.Type }

type PiMessagesToolCallEndEvent struct {
	Type         AssistantMessageEventType `json:"type"`
	ContentIndex int                       `json:"contentIndex"`
	ToolCall     ToolCall                  `json:"toolCall"`
}

func (PiMessagesToolCallEndEvent) piMessagesEvent()                                 {}
func (e PiMessagesToolCallEndEvent) PiMessagesEventType() AssistantMessageEventType { return e.Type }

type PiMessagesDoneReason string

const (
	PiMessagesDoneReasonStop    PiMessagesDoneReason = "stop"
	PiMessagesDoneReasonLength  PiMessagesDoneReason = "length"
	PiMessagesDoneReasonToolUse PiMessagesDoneReason = "toolUse"
)

type PiMessagesErrorReason string

const (
	PiMessagesErrorReasonAborted PiMessagesErrorReason = "aborted"
	PiMessagesErrorReasonError   PiMessagesErrorReason = "error"
)

type PiMessagesDoneEvent struct {
	Type       AssistantMessageEventType         `json:"type"`
	Reason     PiMessagesDoneReason              `json:"reason"`
	Usage      Usage                             `json:"usage"`
	ResponseID Optional[string]                  `json:"responseId,omitzero"`
	Rewrite    Optional[PiMessagesRewriteImpact] `json:"rewrite,omitzero"`
}

func (PiMessagesDoneEvent) piMessagesEvent()                                 {}
func (e PiMessagesDoneEvent) PiMessagesEventType() AssistantMessageEventType { return e.Type }

type PiMessagesErrorEvent struct {
	Type         AssistantMessageEventType         `json:"type"`
	Reason       PiMessagesErrorReason             `json:"reason"`
	Usage        Usage                             `json:"usage"`
	ErrorMessage Optional[string]                  `json:"errorMessage,omitzero"`
	ResponseID   Optional[string]                  `json:"responseId,omitzero"`
	Rewrite      Optional[PiMessagesRewriteImpact] `json:"rewrite,omitzero"`
}

func (PiMessagesErrorEvent) piMessagesEvent()                                 {}
func (e PiMessagesErrorEvent) PiMessagesEventType() AssistantMessageEventType { return e.Type }

type OpenAICodexWebSocketDebugStats struct {
	Requests                int
	ConnectionsCreated      int
	ConnectionsReused       int
	CachedContextRequests   int
	StoreTrueRequests       int
	FullContextRequests     int
	DeltaRequests           int
	LastInputItems          int
	LastDeltaInputItems     Optional[int]
	LastPreviousResponseID  Optional[string]
	WebSocketFailures       int
	SSEFallbacks            int
	WebSocketFallbackActive Optional[bool]
	LastWebSocketError      Optional[string]
}

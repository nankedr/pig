package ai

// AssistantMessageEventType is the wire discriminator for an
// AssistantMessageEvent.
type AssistantMessageEventType string

const (
	AssistantMessageEventTypeStart         AssistantMessageEventType = "start"
	AssistantMessageEventTypeTextStart     AssistantMessageEventType = "text_start"
	AssistantMessageEventTypeTextDelta     AssistantMessageEventType = "text_delta"
	AssistantMessageEventTypeTextEnd       AssistantMessageEventType = "text_end"
	AssistantMessageEventTypeThinkingStart AssistantMessageEventType = "thinking_start"
	AssistantMessageEventTypeThinkingDelta AssistantMessageEventType = "thinking_delta"
	AssistantMessageEventTypeThinkingEnd   AssistantMessageEventType = "thinking_end"
	AssistantMessageEventTypeToolCallStart AssistantMessageEventType = "toolcall_start"
	AssistantMessageEventTypeToolCallDelta AssistantMessageEventType = "toolcall_delta"
	AssistantMessageEventTypeToolCallEnd   AssistantMessageEventType = "toolcall_end"
	AssistantMessageEventTypeDone          AssistantMessageEventType = "done"
	AssistantMessageEventTypeError         AssistantMessageEventType = "error"
)

// AssistantMessageEvent is the closed set of events emitted while producing
// an AssistantMessage.
type AssistantMessageEvent interface {
	assistantMessageEvent()
	AssistantMessageEventType() AssistantMessageEventType
}

type AssistantMessageStartEvent struct {
	Type    AssistantMessageEventType `json:"type"`
	Partial AssistantMessage          `json:"partial"`
}

func (AssistantMessageStartEvent) assistantMessageEvent() {}
func (e AssistantMessageStartEvent) AssistantMessageEventType() AssistantMessageEventType {
	return e.Type
}

type AssistantMessageTextStartEvent struct {
	Type         AssistantMessageEventType `json:"type"`
	ContentIndex int                       `json:"contentIndex"`
	Partial      AssistantMessage          `json:"partial"`
}

func (AssistantMessageTextStartEvent) assistantMessageEvent() {}
func (e AssistantMessageTextStartEvent) AssistantMessageEventType() AssistantMessageEventType {
	return e.Type
}

type AssistantMessageTextDeltaEvent struct {
	Type         AssistantMessageEventType `json:"type"`
	ContentIndex int                       `json:"contentIndex"`
	Delta        string                    `json:"delta"`
	Partial      AssistantMessage          `json:"partial"`
}

func (AssistantMessageTextDeltaEvent) assistantMessageEvent() {}
func (e AssistantMessageTextDeltaEvent) AssistantMessageEventType() AssistantMessageEventType {
	return e.Type
}

type AssistantMessageTextEndEvent struct {
	Type         AssistantMessageEventType `json:"type"`
	ContentIndex int                       `json:"contentIndex"`
	Content      string                    `json:"content"`
	Partial      AssistantMessage          `json:"partial"`
}

func (AssistantMessageTextEndEvent) assistantMessageEvent() {}
func (e AssistantMessageTextEndEvent) AssistantMessageEventType() AssistantMessageEventType {
	return e.Type
}

type AssistantMessageThinkingStartEvent struct {
	Type         AssistantMessageEventType `json:"type"`
	ContentIndex int                       `json:"contentIndex"`
	Partial      AssistantMessage          `json:"partial"`
}

func (AssistantMessageThinkingStartEvent) assistantMessageEvent() {}
func (e AssistantMessageThinkingStartEvent) AssistantMessageEventType() AssistantMessageEventType {
	return e.Type
}

type AssistantMessageThinkingDeltaEvent struct {
	Type         AssistantMessageEventType `json:"type"`
	ContentIndex int                       `json:"contentIndex"`
	Delta        string                    `json:"delta"`
	Partial      AssistantMessage          `json:"partial"`
}

func (AssistantMessageThinkingDeltaEvent) assistantMessageEvent() {}
func (e AssistantMessageThinkingDeltaEvent) AssistantMessageEventType() AssistantMessageEventType {
	return e.Type
}

type AssistantMessageThinkingEndEvent struct {
	Type         AssistantMessageEventType `json:"type"`
	ContentIndex int                       `json:"contentIndex"`
	Content      string                    `json:"content"`
	Partial      AssistantMessage          `json:"partial"`
}

func (AssistantMessageThinkingEndEvent) assistantMessageEvent() {}
func (e AssistantMessageThinkingEndEvent) AssistantMessageEventType() AssistantMessageEventType {
	return e.Type
}

type AssistantMessageToolCallStartEvent struct {
	Type         AssistantMessageEventType `json:"type"`
	ContentIndex int                       `json:"contentIndex"`
	Partial      AssistantMessage          `json:"partial"`
}

func (AssistantMessageToolCallStartEvent) assistantMessageEvent() {}
func (e AssistantMessageToolCallStartEvent) AssistantMessageEventType() AssistantMessageEventType {
	return e.Type
}

type AssistantMessageToolCallDeltaEvent struct {
	Type         AssistantMessageEventType `json:"type"`
	ContentIndex int                       `json:"contentIndex"`
	Delta        string                    `json:"delta"`
	Partial      AssistantMessage          `json:"partial"`
}

func (AssistantMessageToolCallDeltaEvent) assistantMessageEvent() {}
func (e AssistantMessageToolCallDeltaEvent) AssistantMessageEventType() AssistantMessageEventType {
	return e.Type
}

type AssistantMessageToolCallEndEvent struct {
	Type         AssistantMessageEventType `json:"type"`
	ContentIndex int                       `json:"contentIndex"`
	ToolCall     ToolCall                  `json:"toolCall"`
	Partial      AssistantMessage          `json:"partial"`
}

func (AssistantMessageToolCallEndEvent) assistantMessageEvent() {}
func (e AssistantMessageToolCallEndEvent) AssistantMessageEventType() AssistantMessageEventType {
	return e.Type
}

type AssistantMessageDoneEvent struct {
	Type    AssistantMessageEventType `json:"type"`
	Reason  StopReason                `json:"reason"`
	Message AssistantMessage          `json:"message"`
}

func (AssistantMessageDoneEvent) assistantMessageEvent() {}
func (e AssistantMessageDoneEvent) AssistantMessageEventType() AssistantMessageEventType {
	return e.Type
}

type AssistantMessageErrorEvent struct {
	Type   AssistantMessageEventType `json:"type"`
	Reason StopReason                `json:"reason"`
	Error  AssistantMessage          `json:"error"`
}

func (AssistantMessageErrorEvent) assistantMessageEvent() {}
func (e AssistantMessageErrorEvent) AssistantMessageEventType() AssistantMessageEventType {
	return e.Type
}

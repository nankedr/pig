package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/nankedr/pig/ai"
)

// ProxyStreamOptions contains the serializable simple-stream options plus the
// local transport credentials used by the legacy proxy adapter.
type ProxyStreamOptions struct {
	AuthToken       string                     `json:"-"`
	CacheRetention  *ai.CacheRetention         `json:"cacheRetention,omitempty"`
	Headers         ai.ProviderHeaders         `json:"headers,omitempty"`
	MaxRetryDelayMS *int64                     `json:"maxRetryDelayMs,omitempty"`
	MaxTokens       *int64                     `json:"maxTokens,omitempty"`
	Metadata        map[string]json.RawMessage `json:"metadata,omitempty"`
	ProxyURL        string                     `json:"-"`
	Reasoning       *ai.ThinkingLevel          `json:"reasoning,omitempty"`
	SamplingParams  map[string]json.RawMessage `json:"samplingParams,omitempty"`
	SessionID       *string                    `json:"sessionId,omitempty"`
	Signal          context.Context            `json:"-"`
	Temperature     *float64                   `json:"temperature,omitempty"`
	ThinkingBudgets *ai.ThinkingBudgets        `json:"thinkingBudgets,omitempty"`
	Transport       *ai.Transport              `json:"transport,omitempty"`
}

type ProxyAssistantMessageEvent interface {
	proxyAssistantMessageEvent()
	ProxyAssistantMessageEventType() ai.AssistantMessageEventType
}

type ProxyStartEvent struct {
	Type ai.AssistantMessageEventType `json:"type"`
}
type ProxyTextStartEvent struct {
	Type         ai.AssistantMessageEventType `json:"type"`
	ContentIndex int                          `json:"contentIndex"`
}
type ProxyTextDeltaEvent struct {
	Type         ai.AssistantMessageEventType `json:"type"`
	ContentIndex int                          `json:"contentIndex"`
	Delta        string                       `json:"delta"`
}
type ProxyTextEndEvent struct {
	Type             ai.AssistantMessageEventType `json:"type"`
	ContentIndex     int                          `json:"contentIndex"`
	ContentSignature *string                      `json:"contentSignature,omitempty"`
}
type ProxyThinkingStartEvent struct {
	Type         ai.AssistantMessageEventType `json:"type"`
	ContentIndex int                          `json:"contentIndex"`
}
type ProxyThinkingDeltaEvent struct {
	Type         ai.AssistantMessageEventType `json:"type"`
	ContentIndex int                          `json:"contentIndex"`
	Delta        string                       `json:"delta"`
}
type ProxyThinkingEndEvent struct {
	Type             ai.AssistantMessageEventType `json:"type"`
	ContentIndex     int                          `json:"contentIndex"`
	ContentSignature *string                      `json:"contentSignature,omitempty"`
}
type ProxyToolCallStartEvent struct {
	Type         ai.AssistantMessageEventType `json:"type"`
	ContentIndex int                          `json:"contentIndex"`
	ID           string                       `json:"id"`
	ToolName     string                       `json:"toolName"`
}
type ProxyToolCallDeltaEvent struct {
	Type         ai.AssistantMessageEventType `json:"type"`
	ContentIndex int                          `json:"contentIndex"`
	Delta        string                       `json:"delta"`
}
type ProxyToolCallEndEvent struct {
	Type         ai.AssistantMessageEventType `json:"type"`
	ContentIndex int                          `json:"contentIndex"`
	ToolCall     ai.ToolCall                  `json:"toolCall"`
}
type ProxyDoneEvent struct {
	Type   ai.AssistantMessageEventType `json:"type"`
	Reason ai.StopReason                `json:"reason"`
	Usage  ai.Usage                     `json:"usage"`
}
type ProxyErrorEvent struct {
	Type         ai.AssistantMessageEventType `json:"type"`
	Reason       ai.StopReason                `json:"reason"`
	ErrorMessage *string                      `json:"errorMessage,omitempty"`
	Usage        ai.Usage                     `json:"usage"`
}

func (ProxyStartEvent) proxyAssistantMessageEvent()         {}
func (ProxyTextStartEvent) proxyAssistantMessageEvent()     {}
func (ProxyTextDeltaEvent) proxyAssistantMessageEvent()     {}
func (ProxyTextEndEvent) proxyAssistantMessageEvent()       {}
func (ProxyThinkingStartEvent) proxyAssistantMessageEvent() {}
func (ProxyThinkingDeltaEvent) proxyAssistantMessageEvent() {}
func (ProxyThinkingEndEvent) proxyAssistantMessageEvent()   {}
func (ProxyToolCallStartEvent) proxyAssistantMessageEvent() {}
func (ProxyToolCallDeltaEvent) proxyAssistantMessageEvent() {}
func (ProxyToolCallEndEvent) proxyAssistantMessageEvent()   {}
func (ProxyDoneEvent) proxyAssistantMessageEvent()          {}
func (ProxyErrorEvent) proxyAssistantMessageEvent()         {}

func (e ProxyStartEvent) ProxyAssistantMessageEventType() ai.AssistantMessageEventType { return e.Type }
func (e ProxyTextStartEvent) ProxyAssistantMessageEventType() ai.AssistantMessageEventType {
	return e.Type
}
func (e ProxyTextDeltaEvent) ProxyAssistantMessageEventType() ai.AssistantMessageEventType {
	return e.Type
}
func (e ProxyTextEndEvent) ProxyAssistantMessageEventType() ai.AssistantMessageEventType {
	return e.Type
}
func (e ProxyThinkingStartEvent) ProxyAssistantMessageEventType() ai.AssistantMessageEventType {
	return e.Type
}
func (e ProxyThinkingDeltaEvent) ProxyAssistantMessageEventType() ai.AssistantMessageEventType {
	return e.Type
}
func (e ProxyThinkingEndEvent) ProxyAssistantMessageEventType() ai.AssistantMessageEventType {
	return e.Type
}
func (e ProxyToolCallStartEvent) ProxyAssistantMessageEventType() ai.AssistantMessageEventType {
	return e.Type
}
func (e ProxyToolCallDeltaEvent) ProxyAssistantMessageEventType() ai.AssistantMessageEventType {
	return e.Type
}
func (e ProxyToolCallEndEvent) ProxyAssistantMessageEventType() ai.AssistantMessageEventType {
	return e.Type
}
func (e ProxyDoneEvent) ProxyAssistantMessageEventType() ai.AssistantMessageEventType  { return e.Type }
func (e ProxyErrorEvent) ProxyAssistantMessageEventType() ai.AssistantMessageEventType { return e.Type }

func proxyEventExpectedType(event ProxyAssistantMessageEvent) (ai.AssistantMessageEventType, bool) {
	switch event.(type) {
	case ProxyStartEvent:
		return ai.AssistantMessageEventTypeStart, true
	case ProxyTextStartEvent:
		return ai.AssistantMessageEventTypeTextStart, true
	case ProxyTextDeltaEvent:
		return ai.AssistantMessageEventTypeTextDelta, true
	case ProxyTextEndEvent:
		return ai.AssistantMessageEventTypeTextEnd, true
	case ProxyThinkingStartEvent:
		return ai.AssistantMessageEventTypeThinkingStart, true
	case ProxyThinkingDeltaEvent:
		return ai.AssistantMessageEventTypeThinkingDelta, true
	case ProxyThinkingEndEvent:
		return ai.AssistantMessageEventTypeThinkingEnd, true
	case ProxyToolCallStartEvent:
		return ai.AssistantMessageEventTypeToolCallStart, true
	case ProxyToolCallDeltaEvent:
		return ai.AssistantMessageEventTypeToolCallDelta, true
	case ProxyToolCallEndEvent:
		return ai.AssistantMessageEventTypeToolCallEnd, true
	case ProxyDoneEvent:
		return ai.AssistantMessageEventTypeDone, true
	case ProxyErrorEvent:
		return ai.AssistantMessageEventTypeError, true
	default:
		return "", false
	}
}

func MarshalProxyAssistantMessageEvent(event ProxyAssistantMessageEvent) ([]byte, error) {
	if event == nil {
		return nil, fmt.Errorf("unsupported proxy event %T", event)
	}
	value := reflect.ValueOf(event)
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return nil, fmt.Errorf("unsupported proxy event %T", event)
		}
		event = value.Elem().Interface().(ProxyAssistantMessageEvent)
	}
	want, ok := proxyEventExpectedType(event)
	if !ok || event.ProxyAssistantMessageEventType() != want {
		return nil, fmt.Errorf("proxy event %T requires discriminator %q", event, want)
	}
	if err := validateProxyTerminal(event); err != nil {
		return nil, err
	}
	if toolEnd, ok := event.(ProxyToolCallEndEvent); ok {
		if _, err := ai.MarshalContent(toolEnd.ToolCall); err != nil {
			return nil, fmt.Errorf("invalid proxy toolCall: %w", err)
		}
	}
	return json.Marshal(event)
}

func UnmarshalProxyAssistantMessageEvent(data []byte) (ProxyAssistantMessageEvent, error) {
	var envelope struct {
		Type ai.AssistantMessageEventType `json:"type"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, err
	}
	if err := validateProxyEventFields(envelope.Type, data); err != nil {
		return nil, err
	}
	var event ProxyAssistantMessageEvent
	switch envelope.Type {
	case ai.AssistantMessageEventTypeStart:
		event = &ProxyStartEvent{}
	case ai.AssistantMessageEventTypeTextStart:
		event = &ProxyTextStartEvent{}
	case ai.AssistantMessageEventTypeTextDelta:
		event = &ProxyTextDeltaEvent{}
	case ai.AssistantMessageEventTypeTextEnd:
		event = &ProxyTextEndEvent{}
	case ai.AssistantMessageEventTypeThinkingStart:
		event = &ProxyThinkingStartEvent{}
	case ai.AssistantMessageEventTypeThinkingDelta:
		event = &ProxyThinkingDeltaEvent{}
	case ai.AssistantMessageEventTypeThinkingEnd:
		event = &ProxyThinkingEndEvent{}
	case ai.AssistantMessageEventTypeToolCallStart:
		event = &ProxyToolCallStartEvent{}
	case ai.AssistantMessageEventTypeToolCallDelta:
		event = &ProxyToolCallDeltaEvent{}
	case ai.AssistantMessageEventTypeToolCallEnd:
		event = &ProxyToolCallEndEvent{}
	case ai.AssistantMessageEventTypeDone:
		event = &ProxyDoneEvent{}
	case ai.AssistantMessageEventTypeError:
		event = &ProxyErrorEvent{}
	default:
		return nil, fmt.Errorf("unknown proxy event type %q", envelope.Type)
	}
	if err := json.Unmarshal(data, event); err != nil {
		return nil, err
	}
	if toolEnd, ok := event.(*ProxyToolCallEndEvent); ok {
		fields, _ := proxyObjectFields(data)
		content, err := ai.UnmarshalContent(fields["toolCall"])
		if err != nil {
			return nil, fmt.Errorf("invalid proxy toolCall: %w", err)
		}
		toolCall, ok := content.(ai.ToolCall)
		if !ok {
			return nil, fmt.Errorf("proxy toolCall requires toolCall content, got %T", content)
		}
		toolEnd.ToolCall = toolCall
	}
	if err := validateProxyTerminal(event); err != nil {
		return nil, err
	}
	return event, nil
}

func validateProxyEventFields(eventType ai.AssistantMessageEventType, data []byte) error {
	fields, err := proxyObjectFields(data)
	if err != nil {
		return err
	}
	requiredStrings := []string{"type"}
	requiredNumbers := []string{}
	requiredObjects := []string{}
	switch eventType {
	case ai.AssistantMessageEventTypeStart:
	case ai.AssistantMessageEventTypeTextStart, ai.AssistantMessageEventTypeThinkingStart:
		requiredNumbers = append(requiredNumbers, "contentIndex")
	case ai.AssistantMessageEventTypeTextDelta, ai.AssistantMessageEventTypeThinkingDelta, ai.AssistantMessageEventTypeToolCallDelta:
		requiredNumbers = append(requiredNumbers, "contentIndex")
		requiredStrings = append(requiredStrings, "delta")
	case ai.AssistantMessageEventTypeTextEnd, ai.AssistantMessageEventTypeThinkingEnd:
		requiredNumbers = append(requiredNumbers, "contentIndex")
	case ai.AssistantMessageEventTypeToolCallStart:
		requiredNumbers = append(requiredNumbers, "contentIndex")
		requiredStrings = append(requiredStrings, "id", "toolName")
	case ai.AssistantMessageEventTypeToolCallEnd:
		requiredNumbers = append(requiredNumbers, "contentIndex")
		requiredObjects = append(requiredObjects, "toolCall")
	case ai.AssistantMessageEventTypeDone, ai.AssistantMessageEventTypeError:
		requiredStrings = append(requiredStrings, "reason")
		requiredObjects = append(requiredObjects, "usage")
	default:
		return fmt.Errorf("unknown proxy event type %q", eventType)
	}
	for _, name := range requiredStrings {
		if err := proxyRequireKind(fields, name, "string"); err != nil {
			return err
		}
	}
	for _, name := range requiredNumbers {
		if err := proxyRequireKind(fields, name, "number"); err != nil {
			return err
		}
	}
	for _, name := range requiredObjects {
		if err := proxyRequireKind(fields, name, "object"); err != nil {
			return err
		}
	}
	if eventType == ai.AssistantMessageEventTypeDone || eventType == ai.AssistantMessageEventTypeError {
		if err := validateProxyUsage(fields["usage"]); err != nil {
			return err
		}
	}
	if eventType == ai.AssistantMessageEventTypeTextEnd || eventType == ai.AssistantMessageEventTypeThinkingEnd {
		if err := proxyOptionalString(fields, "contentSignature"); err != nil {
			return err
		}
	}
	if eventType == ai.AssistantMessageEventTypeError {
		if err := proxyOptionalString(fields, "errorMessage"); err != nil {
			return err
		}
	}
	return nil
}

func proxyObjectFields(data []byte) (map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, err
	}
	if fields == nil {
		return nil, fmt.Errorf("proxy event must be an object")
	}
	return fields, nil
}

func proxyRequireKind(fields map[string]json.RawMessage, name, kind string) error {
	raw, ok := fields[name]
	if !ok {
		return fmt.Errorf("proxy event requires field %q", name)
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return err
	}
	valid := false
	switch kind {
	case "string":
		_, valid = value.(string)
	case "number":
		_, valid = value.(float64)
	case "object":
		_, valid = value.(map[string]any)
	}
	if !valid {
		return fmt.Errorf("proxy event field %q must be a %s", name, kind)
	}
	return nil
}

func proxyOptionalString(fields map[string]json.RawMessage, name string) error {
	raw, ok := fields[name]
	if !ok {
		return nil
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return err
	}
	if _, ok := value.(string); !ok {
		return fmt.Errorf("proxy event field %q must be a string when present", name)
	}
	return nil
}

func validateProxyUsage(data []byte) error {
	fields, err := proxyObjectFields(data)
	if err != nil {
		return fmt.Errorf("invalid proxy usage: %w", err)
	}
	for _, name := range []string{"input", "output", "cacheRead", "cacheWrite", "totalTokens"} {
		if err := proxyRequireKind(fields, name, "number"); err != nil {
			return fmt.Errorf("invalid proxy usage: %w", err)
		}
	}
	if err := proxyRequireKind(fields, "cost", "object"); err != nil {
		return fmt.Errorf("invalid proxy usage: %w", err)
	}
	cost, err := proxyObjectFields(fields["cost"])
	if err != nil {
		return err
	}
	for _, name := range []string{"input", "output", "cacheRead", "cacheWrite", "total"} {
		if err := proxyRequireKind(cost, name, "number"); err != nil {
			return fmt.Errorf("invalid proxy usage cost: %w", err)
		}
	}
	return nil
}

func validateProxyTerminal(event ProxyAssistantMessageEvent) error {
	value := reflect.ValueOf(event)
	if value.IsValid() && value.Kind() == reflect.Pointer && !value.IsNil() {
		event = value.Elem().Interface().(ProxyAssistantMessageEvent)
	}
	switch event := event.(type) {
	case ProxyDoneEvent:
		if event.Reason != ai.StopReasonStop && event.Reason != ai.StopReasonLength && event.Reason != ai.StopReasonToolUse {
			return fmt.Errorf("invalid proxy done reason %q", event.Reason)
		}
	case ProxyErrorEvent:
		if event.Reason != ai.StopReasonError && event.Reason != ai.StopReasonAborted {
			return fmt.Errorf("invalid proxy error reason %q", event.Reason)
		}
	}
	return nil
}

// ProxyMessageEventStream is the consumer-only result of StreamProxy.
type ProxyMessageEventStream struct{ err error }

func (s *ProxyMessageEventStream) Next(context.Context) (ai.AssistantMessageEvent, bool, error) {
	return nil, false, nil
}

func (s *ProxyMessageEventStream) Result(context.Context) (ai.AssistantMessage, error) {
	return ai.AssistantMessage{}, s.err
}

// StreamProxy is an immediate, network-free M0 Capability Stub.
func StreamProxy(context.Context, ai.Model, ai.Context, ProxyStreamOptions) *ProxyMessageEventStream {
	return &ProxyMessageEventStream{err: newNotImplemented("StreamProxy")}
}

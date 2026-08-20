package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/nankedr/pig/telemetry"
)

// ProviderEnv contains provider-scoped environment overrides.
type ProviderEnv map[string]string

// ProviderHeaders preserves all three header override states: a missing key
// leaves a default unchanged, a nil value deletes it, and a non-nil pointer
// supplies the value (including an explicit empty string).
type ProviderHeaders map[string]*string

// FetchRequest and FetchResponse keep the injectable transport seam independent
// of net/http. Real HTTP behavior belongs to later API-adapter milestones.
type FetchRequest struct {
	URL     string
	Method  string
	Headers map[string]string
	Body    []byte
}

type FetchResponse struct {
	Status  int
	Headers map[string]string
	Body    []byte
}

// FetchFunction is an injectable provider transport boundary.
type FetchFunction func(context.Context, FetchRequest) (FetchResponse, error)

// ProviderResponse is the response metadata exposed to lifecycle hooks.
type ProviderResponse struct {
	Status  int               `json:"status"`
	Headers map[string]string `json:"headers"`
}

// PayloadHookResult explicitly distinguishes retaining the old payload from
// replacing it with any value, including nil, false, an empty object, or zero.
type PayloadHookResult struct {
	Value   JSONValue
	Replace bool
}

type PayloadHook func(context.Context, JSONValue, Model) (PayloadHookResult, error)
type ResponseHook func(context.Context, ProviderResponse, Model) error

// ProviderRequestOptions contains authentication, transport and lifecycle
// options common to all model API requests. Optional non-null scalar fields
// are pointers so omission never collapses into an explicit zero value.
type ProviderRequestOptions struct {
	TelemetryContext telemetry.TelemetryContext `json:"-"`
	APIKey           *string                    `json:"apiKey,omitempty"`
	Fetch            FetchFunction              `json:"-"`
	Env              ProviderEnv                `json:"env,omitempty"`
	OnPayload        PayloadHook                `json:"-"`
	OnResponse       ResponseHook               `json:"-"`
	Headers          ProviderHeaders            `json:"headers,omitempty"`
	TimeoutMS        *int64                     `json:"timeoutMs,omitempty"`
	MaxRetries       *int                       `json:"maxRetries,omitempty"`
	MaxRetryDelayMS  *int64                     `json:"maxRetryDelayMs,omitempty"`
}

// StreamOptions adds sampling, cache and session preferences. Raw JSON maps
// retain extension keys and exact JSON values for custom APIs.
type StreamOptions struct {
	ProviderRequestOptions
	Temperature               *float64                   `json:"temperature,omitempty"`
	SamplingParams            map[string]json.RawMessage `json:"samplingParams,omitempty"`
	MaxTokens                 *int64                     `json:"maxTokens,omitempty"`
	Transport                 *Transport                 `json:"transport,omitempty"`
	CacheRetention            *CacheRetention            `json:"cacheRetention,omitempty"`
	SessionID                 *string                    `json:"sessionId,omitempty"`
	WebSocketConnectTimeoutMS *int64                     `json:"websocketConnectTimeoutMs,omitempty"`
	Metadata                  map[string]json.RawMessage `json:"metadata,omitempty"`
}

type APIStreamOptions = StreamOptions
type ProviderStreamOptions = StreamOptions
type APIOptionsMap map[API]json.RawMessage

type ThinkingBudgets struct {
	Minimal *int64 `json:"minimal,omitempty"`
	Low     *int64 `json:"low,omitempty"`
	Medium  *int64 `json:"medium,omitempty"`
	High    *int64 `json:"high,omitempty"`
}

type DeferredWindow string

const (
	DeferredWindow15Minutes DeferredWindow = "15m"
	DeferredWindow1Hour     DeferredWindow = "1h"
	DeferredWindow24Hours   DeferredWindow = "24h"
)

func (w DeferredWindow) MarshalJSON() ([]byte, error) {
	if !w.valid() {
		return nil, fmt.Errorf("invalid deferred window %q", w)
	}
	return json.Marshal(string(w))
}

func (w *DeferredWindow) UnmarshalJSON(data []byte) error {
	return unmarshalStringEnum(data, w, "deferred window", func(value DeferredWindow) bool {
		return value.valid()
	})
}

func (w DeferredWindow) valid() bool {
	switch w {
	case DeferredWindow15Minutes, DeferredWindow1Hour, DeferredWindow24Hours:
		return true
	default:
		return false
	}
}

type DeferredOption interface{ deferredOption() }
type DeferredBoolean struct{ Enabled bool }
type DeferredWindowOptions struct {
	Window *DeferredWindow `json:"window,omitempty"`
}

func (DeferredBoolean) deferredOption()       {}
func (DeferredWindowOptions) deferredOption() {}

type SimpleStreamOptions struct {
	StreamOptions
	Reasoning       *ThinkingLevel   `json:"reasoning,omitempty"`
	Deferred        DeferredOption   `json:"-"`
	ThinkingBudgets *ThinkingBudgets `json:"thinkingBudgets,omitempty"`
}

func (o SimpleStreamOptions) MarshalJSON() ([]byte, error) {
	type plain SimpleStreamOptions
	wire := struct {
		plain
		Deferred json.RawMessage `json:"deferred,omitempty"`
	}{plain: plain(o)}

	var err error
	if o.Deferred != nil {
		wire.Deferred, err = marshalDeferredOption(o.Deferred)
		if err != nil {
			return nil, err
		}
	}
	return json.Marshal(wire)
}

func (o *SimpleStreamOptions) UnmarshalJSON(data []byte) error {
	type plain SimpleStreamOptions
	var wire struct {
		*plain
		Deferred json.RawMessage `json:"deferred"`
	}
	decoded := plain{}
	wire.plain = &decoded
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	if wire.Deferred != nil {
		deferred, err := unmarshalDeferredOption(wire.Deferred)
		if err != nil {
			return err
		}
		decoded.Deferred = deferred
	}
	*o = SimpleStreamOptions(decoded)
	return nil
}

func marshalDeferredOption(option DeferredOption) ([]byte, error) {
	switch value := option.(type) {
	case DeferredBoolean:
		return json.Marshal(value.Enabled)
	case *DeferredBoolean:
		if value == nil {
			return nil, fmt.Errorf("invalid nil deferred boolean")
		}
		return json.Marshal(value.Enabled)
	case DeferredWindowOptions:
		return json.Marshal(value)
	case *DeferredWindowOptions:
		if value == nil {
			return nil, fmt.Errorf("invalid nil deferred window options")
		}
		return json.Marshal(value)
	default:
		return nil, fmt.Errorf("unsupported deferred option %T", option)
	}
}

func unmarshalDeferredOption(data []byte) (DeferredOption, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil, fmt.Errorf("deferred must be a boolean or window object")
	}
	if trimmed[0] != '{' {
		var enabled bool
		if err := json.Unmarshal(trimmed, &enabled); err != nil {
			return nil, fmt.Errorf("deferred must be a boolean or window object: %w", err)
		}
		return DeferredBoolean{Enabled: enabled}, nil
	}

	fields, err := decodeStrictObject(trimmed, "deferred", "window")
	if err != nil {
		return nil, err
	}
	windowJSON, ok := fields["window"]
	if !ok {
		return DeferredWindowOptions{}, nil
	}
	if bytes.Equal(bytes.TrimSpace(windowJSON), []byte("null")) {
		return nil, fmt.Errorf("deferred window must not be null")
	}
	var window DeferredWindow
	if err := json.Unmarshal(windowJSON, &window); err != nil {
		return nil, err
	}
	return DeferredWindowOptions{Window: &window}, nil
}

func unmarshalStringEnum[T ~string](data []byte, target *T, surface string, valid func(T) bool) error {
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return fmt.Errorf("%s must be a string: %w", surface, err)
	}
	typed := T(value)
	if !valid(typed) {
		return fmt.Errorf("invalid %s %q", surface, value)
	}
	*target = typed
	return nil
}

func decodeStrictObject(data []byte, surface string, allowed ...string) (map[string]json.RawMessage, error) {
	allowedFields := make(map[string]struct{}, len(allowed))
	for _, field := range allowed {
		allowedFields[field] = struct{}{}
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	opening, err := decoder.Token()
	if err != nil {
		return nil, fmt.Errorf("invalid %s object: %w", surface, err)
	}
	if delimiter, ok := opening.(json.Delim); !ok || delimiter != '{' {
		return nil, fmt.Errorf("%s must be an object", surface)
	}

	fields := make(map[string]json.RawMessage, len(allowed))
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return nil, fmt.Errorf("invalid %s object: %w", surface, err)
		}
		key, ok := keyToken.(string)
		if !ok {
			return nil, fmt.Errorf("invalid %s object key", surface)
		}
		if _, ok := allowedFields[key]; !ok {
			return nil, fmt.Errorf("unknown %s field %q", surface, key)
		}
		if _, duplicate := fields[key]; duplicate {
			return nil, fmt.Errorf("duplicate %s field %q", surface, key)
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, fmt.Errorf("invalid %s field %q: %w", surface, key, err)
		}
		fields[key] = value
	}
	if _, err := decoder.Token(); err != nil {
		return nil, fmt.Errorf("invalid %s object: %w", surface, err)
	}
	if decoder.More() {
		return nil, fmt.Errorf("invalid trailing %s data", surface)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err == nil {
		return nil, fmt.Errorf("invalid trailing %s data", surface)
	}
	return fields, nil
}

type DeferredFetchOptions struct {
	ProviderRequestOptions
	WaitMS *int64 `json:"wait,omitempty"`
}

type DeferredCancelOptions = ProviderRequestOptions

// ImagesOptions is the image-generation counterpart of request options.
type ImagesOptions struct {
	ProviderRequestOptions
	Metadata map[string]json.RawMessage `json:"metadata,omitempty"`
}

type ProviderImagesOptions = ImagesOptions

// Provider-specific enums and full option records exported from Pi's root.
type AnthropicEffort string
type AnthropicThinkingDisplay string
type BedrockThinkingDisplay string
type OpenAIReasoningEffort string
type OpenAIReasoningSummary string
type CodexReasoningEffort string
type CodexReasoningSummary string
type OpenAIServiceTier string
type TextVerbosity string
type GoogleToolChoice string
type GoogleThinkingLevel string
type MistralPromptMode string
type MistralReasoningEffort string
type CodexToolChoice string

const (
	AnthropicEffortLow               AnthropicEffort          = "low"
	AnthropicEffortMedium            AnthropicEffort          = "medium"
	AnthropicEffortHigh              AnthropicEffort          = "high"
	AnthropicEffortXHigh             AnthropicEffort          = "xhigh"
	AnthropicEffortMax               AnthropicEffort          = "max"
	ThinkingDisplaySummarized        AnthropicThinkingDisplay = "summarized"
	ThinkingDisplayOmitted           AnthropicThinkingDisplay = "omitted"
	BedrockThinkingDisplaySummarized BedrockThinkingDisplay   = "summarized"
	BedrockThinkingDisplayOmitted    BedrockThinkingDisplay   = "omitted"
	GoogleToolChoiceAuto             GoogleToolChoice         = "auto"
	GoogleToolChoiceNone             GoogleToolChoice         = "none"
	GoogleToolChoiceAny              GoogleToolChoice         = "any"

	AnthropicThinkingDisplaySummarized AnthropicThinkingDisplay = "summarized"
	AnthropicThinkingDisplayOmitted    AnthropicThinkingDisplay = "omitted"

	OpenAIReasoningEffortMinimal OpenAIReasoningEffort = "minimal"
	OpenAIReasoningEffortLow     OpenAIReasoningEffort = "low"
	OpenAIReasoningEffortMedium  OpenAIReasoningEffort = "medium"
	OpenAIReasoningEffortHigh    OpenAIReasoningEffort = "high"
	OpenAIReasoningEffortXHigh   OpenAIReasoningEffort = "xhigh"
	OpenAIReasoningEffortMax     OpenAIReasoningEffort = "max"

	OpenAIReasoningSummaryAuto     OpenAIReasoningSummary = "auto"
	OpenAIReasoningSummaryDetailed OpenAIReasoningSummary = "detailed"
	OpenAIReasoningSummaryConcise  OpenAIReasoningSummary = "concise"

	CodexReasoningEffortNone    CodexReasoningEffort = "none"
	CodexReasoningEffortMinimal CodexReasoningEffort = "minimal"
	CodexReasoningEffortLow     CodexReasoningEffort = "low"
	CodexReasoningEffortMedium  CodexReasoningEffort = "medium"
	CodexReasoningEffortHigh    CodexReasoningEffort = "high"
	CodexReasoningEffortXHigh   CodexReasoningEffort = "xhigh"
	CodexReasoningEffortMax     CodexReasoningEffort = "max"

	CodexReasoningSummaryAuto     CodexReasoningSummary = "auto"
	CodexReasoningSummaryConcise  CodexReasoningSummary = "concise"
	CodexReasoningSummaryDetailed CodexReasoningSummary = "detailed"
	CodexReasoningSummaryOff      CodexReasoningSummary = "off"
	CodexReasoningSummaryOn       CodexReasoningSummary = "on"

	OpenAIServiceTierAuto     OpenAIServiceTier = "auto"
	OpenAIServiceTierDefault  OpenAIServiceTier = "default"
	OpenAIServiceTierFlex     OpenAIServiceTier = "flex"
	OpenAIServiceTierScale    OpenAIServiceTier = "scale"
	OpenAIServiceTierPriority OpenAIServiceTier = "priority"

	TextVerbosityLow    TextVerbosity = "low"
	TextVerbosityMedium TextVerbosity = "medium"
	TextVerbosityHigh   TextVerbosity = "high"

	GoogleThinkingLevelUnspecified GoogleThinkingLevel = "THINKING_LEVEL_UNSPECIFIED"
	GoogleThinkingLevelMinimal     GoogleThinkingLevel = "MINIMAL"
	GoogleThinkingLevelLow         GoogleThinkingLevel = "LOW"
	GoogleThinkingLevelMedium      GoogleThinkingLevel = "MEDIUM"
	GoogleThinkingLevelHigh        GoogleThinkingLevel = "HIGH"

	MistralPromptModeReasoning MistralPromptMode = "reasoning"

	MistralReasoningEffortNone MistralReasoningEffort = "none"
	MistralReasoningEffortHigh MistralReasoningEffort = "high"

	CodexToolChoiceAuto     CodexToolChoice = "auto"
	CodexToolChoiceNone     CodexToolChoice = "none"
	CodexToolChoiceRequired CodexToolChoice = "required"
)

func (value AnthropicEffort) MarshalJSON() ([]byte, error) {
	return marshalStringEnum(value, "Anthropic effort", value.valid())
}
func (value *AnthropicEffort) UnmarshalJSON(data []byte) error {
	return unmarshalStringEnum(data, value, "Anthropic effort", func(candidate AnthropicEffort) bool { return candidate.valid() })
}
func (value AnthropicEffort) valid() bool {
	switch value {
	case AnthropicEffortLow, AnthropicEffortMedium, AnthropicEffortHigh, AnthropicEffortXHigh, AnthropicEffortMax:
		return true
	default:
		return false
	}
}

func (value AnthropicThinkingDisplay) MarshalJSON() ([]byte, error) {
	return marshalStringEnum(value, "Anthropic thinking display", value.valid())
}
func (value *AnthropicThinkingDisplay) UnmarshalJSON(data []byte) error {
	return unmarshalStringEnum(data, value, "Anthropic thinking display", func(candidate AnthropicThinkingDisplay) bool { return candidate.valid() })
}
func (value AnthropicThinkingDisplay) valid() bool {
	return value == AnthropicThinkingDisplaySummarized || value == AnthropicThinkingDisplayOmitted
}

func (value BedrockThinkingDisplay) MarshalJSON() ([]byte, error) {
	return marshalStringEnum(value, "Bedrock thinking display", value.valid())
}
func (value *BedrockThinkingDisplay) UnmarshalJSON(data []byte) error {
	return unmarshalStringEnum(data, value, "Bedrock thinking display", func(candidate BedrockThinkingDisplay) bool { return candidate.valid() })
}
func (value BedrockThinkingDisplay) valid() bool {
	return value == BedrockThinkingDisplaySummarized || value == BedrockThinkingDisplayOmitted
}

func (value OpenAIReasoningEffort) MarshalJSON() ([]byte, error) {
	return marshalStringEnum(value, "OpenAI reasoning effort", value.valid())
}
func (value *OpenAIReasoningEffort) UnmarshalJSON(data []byte) error {
	return unmarshalStringEnum(data, value, "OpenAI reasoning effort", func(candidate OpenAIReasoningEffort) bool { return candidate.valid() })
}
func (value OpenAIReasoningEffort) valid() bool {
	switch value {
	case OpenAIReasoningEffortMinimal, OpenAIReasoningEffortLow, OpenAIReasoningEffortMedium, OpenAIReasoningEffortHigh, OpenAIReasoningEffortXHigh, OpenAIReasoningEffortMax:
		return true
	default:
		return false
	}
}

func (value OpenAIReasoningSummary) MarshalJSON() ([]byte, error) {
	return marshalStringEnum(value, "OpenAI reasoning summary", value.valid())
}
func (value *OpenAIReasoningSummary) UnmarshalJSON(data []byte) error {
	return unmarshalStringEnum(data, value, "OpenAI reasoning summary", func(candidate OpenAIReasoningSummary) bool { return candidate.valid() })
}
func (value OpenAIReasoningSummary) valid() bool {
	switch value {
	case OpenAIReasoningSummaryAuto, OpenAIReasoningSummaryDetailed, OpenAIReasoningSummaryConcise:
		return true
	default:
		return false
	}
}

func (value CodexReasoningEffort) MarshalJSON() ([]byte, error) {
	return marshalStringEnum(value, "Codex reasoning effort", value.valid())
}
func (value *CodexReasoningEffort) UnmarshalJSON(data []byte) error {
	return unmarshalStringEnum(data, value, "Codex reasoning effort", func(candidate CodexReasoningEffort) bool { return candidate.valid() })
}
func (value CodexReasoningEffort) valid() bool {
	switch value {
	case CodexReasoningEffortNone, CodexReasoningEffortMinimal, CodexReasoningEffortLow, CodexReasoningEffortMedium, CodexReasoningEffortHigh, CodexReasoningEffortXHigh, CodexReasoningEffortMax:
		return true
	default:
		return false
	}
}

func (value CodexReasoningSummary) MarshalJSON() ([]byte, error) {
	return marshalStringEnum(value, "Codex reasoning summary", value.valid())
}
func (value *CodexReasoningSummary) UnmarshalJSON(data []byte) error {
	return unmarshalStringEnum(data, value, "Codex reasoning summary", func(candidate CodexReasoningSummary) bool { return candidate.valid() })
}
func (value CodexReasoningSummary) valid() bool {
	switch value {
	case CodexReasoningSummaryAuto, CodexReasoningSummaryConcise, CodexReasoningSummaryDetailed, CodexReasoningSummaryOff, CodexReasoningSummaryOn:
		return true
	default:
		return false
	}
}

func (value OpenAIServiceTier) MarshalJSON() ([]byte, error) {
	return marshalStringEnum(value, "OpenAI service tier", value.valid())
}
func (value *OpenAIServiceTier) UnmarshalJSON(data []byte) error {
	return unmarshalStringEnum(data, value, "OpenAI service tier", func(candidate OpenAIServiceTier) bool { return candidate.valid() })
}
func (value OpenAIServiceTier) valid() bool {
	switch value {
	case OpenAIServiceTierAuto, OpenAIServiceTierDefault, OpenAIServiceTierFlex, OpenAIServiceTierScale, OpenAIServiceTierPriority:
		return true
	default:
		return false
	}
}

func (value TextVerbosity) MarshalJSON() ([]byte, error) {
	return marshalStringEnum(value, "text verbosity", value.valid())
}
func (value *TextVerbosity) UnmarshalJSON(data []byte) error {
	return unmarshalStringEnum(data, value, "text verbosity", func(candidate TextVerbosity) bool { return candidate.valid() })
}
func (value TextVerbosity) valid() bool {
	switch value {
	case TextVerbosityLow, TextVerbosityMedium, TextVerbosityHigh:
		return true
	default:
		return false
	}
}

func (value GoogleToolChoice) MarshalJSON() ([]byte, error) {
	return marshalStringEnum(value, "Google tool choice", value.valid())
}
func (value *GoogleToolChoice) UnmarshalJSON(data []byte) error {
	return unmarshalStringEnum(data, value, "Google tool choice", func(candidate GoogleToolChoice) bool { return candidate.valid() })
}
func (value GoogleToolChoice) valid() bool {
	switch value {
	case GoogleToolChoiceAuto, GoogleToolChoiceNone, GoogleToolChoiceAny:
		return true
	default:
		return false
	}
}

func (value GoogleThinkingLevel) MarshalJSON() ([]byte, error) {
	return marshalStringEnum(value, "Google thinking level", value.valid())
}
func (value *GoogleThinkingLevel) UnmarshalJSON(data []byte) error {
	return unmarshalStringEnum(data, value, "Google thinking level", func(candidate GoogleThinkingLevel) bool { return candidate.valid() })
}
func (value GoogleThinkingLevel) valid() bool {
	switch value {
	case GoogleThinkingLevelUnspecified, GoogleThinkingLevelMinimal, GoogleThinkingLevelLow, GoogleThinkingLevelMedium, GoogleThinkingLevelHigh:
		return true
	default:
		return false
	}
}

func (value MistralPromptMode) MarshalJSON() ([]byte, error) {
	return marshalStringEnum(value, "Mistral prompt mode", value.valid())
}
func (value *MistralPromptMode) UnmarshalJSON(data []byte) error {
	return unmarshalStringEnum(data, value, "Mistral prompt mode", func(candidate MistralPromptMode) bool { return candidate.valid() })
}
func (value MistralPromptMode) valid() bool {
	return value == MistralPromptModeReasoning
}

func (value MistralReasoningEffort) MarshalJSON() ([]byte, error) {
	return marshalStringEnum(value, "Mistral reasoning effort", value.valid())
}
func (value *MistralReasoningEffort) UnmarshalJSON(data []byte) error {
	return unmarshalStringEnum(data, value, "Mistral reasoning effort", func(candidate MistralReasoningEffort) bool { return candidate.valid() })
}
func (value MistralReasoningEffort) valid() bool {
	return value == MistralReasoningEffortNone || value == MistralReasoningEffortHigh
}

func (value CodexToolChoice) MarshalJSON() ([]byte, error) {
	return marshalStringEnum(value, "Codex tool choice", value.valid())
}
func (value *CodexToolChoice) UnmarshalJSON(data []byte) error {
	return unmarshalStringEnum(data, value, "Codex tool choice", func(candidate CodexToolChoice) bool { return candidate.valid() })
}
func (value CodexToolChoice) valid() bool {
	switch value {
	case CodexToolChoiceAuto, CodexToolChoiceNone, CodexToolChoiceRequired:
		return true
	default:
		return false
	}
}

// Tool-choice unions are sealed to the variants exported by the fixed Pi
// baseline. Provider SDKs are not part of the core module's public surface.
type AnthropicToolChoice interface{ anthropicToolChoice() }

type AnthropicToolChoiceMode string

const (
	AnthropicToolChoiceAuto AnthropicToolChoiceMode = "auto"
	AnthropicToolChoiceAny  AnthropicToolChoiceMode = "any"
	AnthropicToolChoiceNone AnthropicToolChoiceMode = "none"
)

func (AnthropicToolChoiceMode) anthropicToolChoice() {}

func (choice AnthropicToolChoiceMode) MarshalJSON() ([]byte, error) {
	return marshalStringEnum(choice, "anthropic tool choice", choice.valid())
}

func (choice AnthropicToolChoiceMode) valid() bool {
	switch choice {
	case AnthropicToolChoiceAuto, AnthropicToolChoiceAny, AnthropicToolChoiceNone:
		return true
	default:
		return false
	}
}

type AnthropicToolChoiceTool struct {
	Name string
}

func (AnthropicToolChoiceTool) anthropicToolChoice() {}

func (choice AnthropicToolChoiceTool) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type string `json:"type"`
		Name string `json:"name"`
	}{Type: "tool", Name: choice.Name})
}

func NewAnthropicToolChoiceTool(name string) AnthropicToolChoiceTool {
	return AnthropicToolChoiceTool{Name: name}
}

type BedrockToolChoice interface{ bedrockToolChoice() }

type BedrockToolChoiceMode string

const (
	BedrockToolChoiceAuto BedrockToolChoiceMode = "auto"
	BedrockToolChoiceAny  BedrockToolChoiceMode = "any"
	BedrockToolChoiceNone BedrockToolChoiceMode = "none"
)

func (BedrockToolChoiceMode) bedrockToolChoice() {}

func (choice BedrockToolChoiceMode) MarshalJSON() ([]byte, error) {
	return marshalStringEnum(choice, "bedrock tool choice", choice.valid())
}

func (choice BedrockToolChoiceMode) valid() bool {
	switch choice {
	case BedrockToolChoiceAuto, BedrockToolChoiceAny, BedrockToolChoiceNone:
		return true
	default:
		return false
	}
}

type BedrockToolChoiceTool struct {
	Name string
}

func (BedrockToolChoiceTool) bedrockToolChoice() {}

func (choice BedrockToolChoiceTool) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type string `json:"type"`
		Name string `json:"name"`
	}{Type: "tool", Name: choice.Name})
}

func NewBedrockToolChoiceTool(name string) BedrockToolChoiceTool {
	return BedrockToolChoiceTool{Name: name}
}

type MistralToolChoice interface{ mistralToolChoice() }

type MistralToolChoiceMode string

const (
	MistralToolChoiceAuto     MistralToolChoiceMode = "auto"
	MistralToolChoiceNone     MistralToolChoiceMode = "none"
	MistralToolChoiceAny      MistralToolChoiceMode = "any"
	MistralToolChoiceRequired MistralToolChoiceMode = "required"
)

func (MistralToolChoiceMode) mistralToolChoice() {}

func (choice MistralToolChoiceMode) MarshalJSON() ([]byte, error) {
	return marshalStringEnum(choice, "mistral tool choice", choice.valid())
}

func (choice MistralToolChoiceMode) valid() bool {
	switch choice {
	case MistralToolChoiceAuto, MistralToolChoiceNone, MistralToolChoiceAny, MistralToolChoiceRequired:
		return true
	default:
		return false
	}
}

type MistralToolChoiceFunction struct {
	Name string
}

func (MistralToolChoiceFunction) mistralToolChoice() {}

func (choice MistralToolChoiceFunction) MarshalJSON() ([]byte, error) {
	return marshalFunctionToolChoice(choice.Name)
}

func NewMistralToolChoiceFunction(name string) MistralToolChoiceFunction {
	return MistralToolChoiceFunction{Name: name}
}

type OpenAIChatToolChoice interface{ openAIChatToolChoice() }

type OpenAIChatToolChoiceMode string

const (
	OpenAIChatToolChoiceNone     OpenAIChatToolChoiceMode = "none"
	OpenAIChatToolChoiceAuto     OpenAIChatToolChoiceMode = "auto"
	OpenAIChatToolChoiceRequired OpenAIChatToolChoiceMode = "required"
)

func (OpenAIChatToolChoiceMode) openAIChatToolChoice() {}

func (choice OpenAIChatToolChoiceMode) MarshalJSON() ([]byte, error) {
	return marshalStringEnum(choice, "OpenAI Chat tool choice", choice.valid())
}

func (choice OpenAIChatToolChoiceMode) valid() bool {
	switch choice {
	case OpenAIChatToolChoiceNone, OpenAIChatToolChoiceAuto, OpenAIChatToolChoiceRequired:
		return true
	default:
		return false
	}
}

type OpenAIChatToolChoiceFunction struct{ Name string }

func (OpenAIChatToolChoiceFunction) openAIChatToolChoice() {}

func (choice OpenAIChatToolChoiceFunction) MarshalJSON() ([]byte, error) {
	return marshalFunctionToolChoice(choice.Name)
}

func NewOpenAIChatToolChoiceFunction(name string) OpenAIChatToolChoiceFunction {
	return OpenAIChatToolChoiceFunction{Name: name}
}

type OpenAIChatToolChoiceCustom struct{ Name string }

func (OpenAIChatToolChoiceCustom) openAIChatToolChoice() {}

func (choice OpenAIChatToolChoiceCustom) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type   string `json:"type"`
		Custom struct {
			Name string `json:"name"`
		} `json:"custom"`
	}{Type: "custom", Custom: struct {
		Name string `json:"name"`
	}{Name: choice.Name}})
}

func NewOpenAIChatToolChoiceCustom(name string) OpenAIChatToolChoiceCustom {
	return OpenAIChatToolChoiceCustom{Name: name}
}

type ToolChoiceAllowedMode string

const (
	ToolChoiceAllowedModeAuto     ToolChoiceAllowedMode = "auto"
	ToolChoiceAllowedModeRequired ToolChoiceAllowedMode = "required"
)

func (mode ToolChoiceAllowedMode) MarshalJSON() ([]byte, error) {
	return marshalStringEnum(mode, "allowed-tools mode", mode.valid())
}

func (mode *ToolChoiceAllowedMode) UnmarshalJSON(data []byte) error {
	return unmarshalStringEnum(data, mode, "allowed-tools mode", func(value ToolChoiceAllowedMode) bool {
		return value.valid()
	})
}

func (mode ToolChoiceAllowedMode) valid() bool {
	return mode == ToolChoiceAllowedModeAuto || mode == ToolChoiceAllowedModeRequired
}

type OpenAIChatToolChoiceAllowed struct {
	Mode  ToolChoiceAllowedMode
	Tools []map[string]JSONValue
}

func (OpenAIChatToolChoiceAllowed) openAIChatToolChoice() {}

func (choice OpenAIChatToolChoiceAllowed) MarshalJSON() ([]byte, error) {
	if !choice.Mode.valid() {
		return nil, fmt.Errorf("invalid allowed-tools mode %q", choice.Mode)
	}
	if err := validateToolChoiceTools(choice.Tools); err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Type         string `json:"type"`
		AllowedTools struct {
			Mode  ToolChoiceAllowedMode  `json:"mode"`
			Tools []map[string]JSONValue `json:"tools"`
		} `json:"allowed_tools"`
	}{Type: "allowed_tools", AllowedTools: struct {
		Mode  ToolChoiceAllowedMode  `json:"mode"`
		Tools []map[string]JSONValue `json:"tools"`
	}{Mode: choice.Mode, Tools: choice.Tools}})
}

func NewOpenAIChatToolChoiceAllowed(mode ToolChoiceAllowedMode, tools []map[string]JSONValue) OpenAIChatToolChoiceAllowed {
	return OpenAIChatToolChoiceAllowed{Mode: mode, Tools: tools}
}

type OpenAIResponsesToolChoice interface{ openAIResponsesToolChoice() }

type OpenAIResponsesToolChoiceMode string

const (
	OpenAIResponsesToolChoiceNone     OpenAIResponsesToolChoiceMode = "none"
	OpenAIResponsesToolChoiceAuto     OpenAIResponsesToolChoiceMode = "auto"
	OpenAIResponsesToolChoiceRequired OpenAIResponsesToolChoiceMode = "required"
)

func (OpenAIResponsesToolChoiceMode) openAIResponsesToolChoice() {}

func (choice OpenAIResponsesToolChoiceMode) MarshalJSON() ([]byte, error) {
	return marshalStringEnum(choice, "OpenAI Responses tool choice", choice.valid())
}

func (choice OpenAIResponsesToolChoiceMode) valid() bool {
	switch choice {
	case OpenAIResponsesToolChoiceNone, OpenAIResponsesToolChoiceAuto, OpenAIResponsesToolChoiceRequired:
		return true
	default:
		return false
	}
}

type OpenAIResponsesToolChoiceFunction struct{ Name string }

func (OpenAIResponsesToolChoiceFunction) openAIResponsesToolChoice() {}

func (choice OpenAIResponsesToolChoiceFunction) MarshalJSON() ([]byte, error) {
	return marshalNamedToolChoice("function", choice.Name)
}

func NewOpenAIResponsesToolChoiceFunction(name string) OpenAIResponsesToolChoiceFunction {
	return OpenAIResponsesToolChoiceFunction{Name: name}
}

type OpenAIResponsesToolChoiceCustom struct{ Name string }

func (OpenAIResponsesToolChoiceCustom) openAIResponsesToolChoice() {}

func (choice OpenAIResponsesToolChoiceCustom) MarshalJSON() ([]byte, error) {
	return marshalNamedToolChoice("custom", choice.Name)
}

func NewOpenAIResponsesToolChoiceCustom(name string) OpenAIResponsesToolChoiceCustom {
	return OpenAIResponsesToolChoiceCustom{Name: name}
}

type OpenAIResponsesToolChoiceMCP struct {
	ServerLabel string
	Name        Optional[string]
}

func (OpenAIResponsesToolChoiceMCP) openAIResponsesToolChoice() {}

func (choice OpenAIResponsesToolChoiceMCP) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type        string           `json:"type"`
		ServerLabel string           `json:"server_label"`
		Name        Optional[string] `json:"name,omitzero"`
	}{Type: "mcp", ServerLabel: choice.ServerLabel, Name: choice.Name})
}

func NewOpenAIResponsesToolChoiceMCP(serverLabel string) OpenAIResponsesToolChoiceMCP {
	return OpenAIResponsesToolChoiceMCP{ServerLabel: serverLabel}
}

func NewOpenAIResponsesToolChoiceNamedMCP(serverLabel, name string) OpenAIResponsesToolChoiceMCP {
	return OpenAIResponsesToolChoiceMCP{ServerLabel: serverLabel, Name: Some(name)}
}

type OpenAIResponsesToolChoiceAllowed struct {
	Mode  ToolChoiceAllowedMode
	Tools []map[string]JSONValue
}

func (OpenAIResponsesToolChoiceAllowed) openAIResponsesToolChoice() {}

func (choice OpenAIResponsesToolChoiceAllowed) MarshalJSON() ([]byte, error) {
	if !choice.Mode.valid() {
		return nil, fmt.Errorf("invalid allowed-tools mode %q", choice.Mode)
	}
	if err := validateToolChoiceTools(choice.Tools); err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Type  string                 `json:"type"`
		Mode  ToolChoiceAllowedMode  `json:"mode"`
		Tools []map[string]JSONValue `json:"tools"`
	}{Type: "allowed_tools", Mode: choice.Mode, Tools: choice.Tools})
}

func NewOpenAIResponsesToolChoiceAllowed(mode ToolChoiceAllowedMode, tools []map[string]JSONValue) OpenAIResponsesToolChoiceAllowed {
	return OpenAIResponsesToolChoiceAllowed{Mode: mode, Tools: tools}
}

type OpenAIResponsesHostedTool string

const (
	OpenAIResponsesHostedToolFileSearch               OpenAIResponsesHostedTool = "file_search"
	OpenAIResponsesHostedToolWebSearchPreview         OpenAIResponsesHostedTool = "web_search_preview"
	OpenAIResponsesHostedToolComputer                 OpenAIResponsesHostedTool = "computer"
	OpenAIResponsesHostedToolComputerUsePreview       OpenAIResponsesHostedTool = "computer_use_preview"
	OpenAIResponsesHostedToolComputerUse              OpenAIResponsesHostedTool = "computer_use"
	OpenAIResponsesHostedToolWebSearchPreview20250311 OpenAIResponsesHostedTool = "web_search_preview_2025_03_11"
	OpenAIResponsesHostedToolImageGeneration          OpenAIResponsesHostedTool = "image_generation"
	OpenAIResponsesHostedToolCodeInterpreter          OpenAIResponsesHostedTool = "code_interpreter"
	OpenAIResponsesHostedToolMCP                      OpenAIResponsesHostedTool = "mcp"
)

func (tool OpenAIResponsesHostedTool) valid() bool {
	switch tool {
	case OpenAIResponsesHostedToolFileSearch, OpenAIResponsesHostedToolWebSearchPreview,
		OpenAIResponsesHostedToolComputer, OpenAIResponsesHostedToolComputerUsePreview,
		OpenAIResponsesHostedToolComputerUse, OpenAIResponsesHostedToolWebSearchPreview20250311,
		OpenAIResponsesHostedToolImageGeneration, OpenAIResponsesHostedToolCodeInterpreter,
		OpenAIResponsesHostedToolMCP:
		return true
	default:
		return false
	}
}

type OpenAIResponsesToolChoiceHosted struct{ Type OpenAIResponsesHostedTool }

func (OpenAIResponsesToolChoiceHosted) openAIResponsesToolChoice() {}

func (choice OpenAIResponsesToolChoiceHosted) MarshalJSON() ([]byte, error) {
	if !choice.Type.valid() {
		return nil, fmt.Errorf("invalid OpenAI Responses hosted tool %q", choice.Type)
	}
	return json.Marshal(struct {
		Type OpenAIResponsesHostedTool `json:"type"`
	}{Type: choice.Type})
}

func NewOpenAIResponsesToolChoiceHosted(tool OpenAIResponsesHostedTool) OpenAIResponsesToolChoiceHosted {
	return OpenAIResponsesToolChoiceHosted{Type: tool}
}

type OpenAIResponsesToolChoiceApplyPatch struct{}

func (OpenAIResponsesToolChoiceApplyPatch) openAIResponsesToolChoice() {}
func (OpenAIResponsesToolChoiceApplyPatch) MarshalJSON() ([]byte, error) {
	return []byte(`{"type":"apply_patch"}`), nil
}

type OpenAIResponsesToolChoiceShell struct{}

func (OpenAIResponsesToolChoiceShell) openAIResponsesToolChoice() {}
func (OpenAIResponsesToolChoiceShell) MarshalJSON() ([]byte, error) {
	return []byte(`{"type":"shell"}`), nil
}

type PiMessagesToolChoice interface{ piMessagesToolChoice() }

type PiMessagesToolChoiceMode string

const (
	PiMessagesToolChoiceAuto     PiMessagesToolChoiceMode = "auto"
	PiMessagesToolChoiceNone     PiMessagesToolChoiceMode = "none"
	PiMessagesToolChoiceRequired PiMessagesToolChoiceMode = "required"
)

func (PiMessagesToolChoiceMode) piMessagesToolChoice() {}

func (choice PiMessagesToolChoiceMode) MarshalJSON() ([]byte, error) {
	return marshalStringEnum(choice, "Pi Messages tool choice", choice.valid())
}

func (choice PiMessagesToolChoiceMode) valid() bool {
	switch choice {
	case PiMessagesToolChoiceAuto, PiMessagesToolChoiceNone, PiMessagesToolChoiceRequired:
		return true
	default:
		return false
	}
}

type PiMessagesToolChoiceFunction struct{ Name string }

func (PiMessagesToolChoiceFunction) piMessagesToolChoice() {}

func (choice PiMessagesToolChoiceFunction) MarshalJSON() ([]byte, error) {
	return marshalFunctionToolChoice(choice.Name)
}

func NewPiMessagesToolChoiceFunction(name string) PiMessagesToolChoiceFunction {
	return PiMessagesToolChoiceFunction{Name: name}
}

func marshalStringEnum[T ~string](value T, surface string, valid bool) ([]byte, error) {
	if !valid {
		return nil, fmt.Errorf("invalid %s %q", surface, value)
	}
	return json.Marshal(string(value))
}

func marshalFunctionToolChoice(name string) ([]byte, error) {
	return json.Marshal(struct {
		Type     string `json:"type"`
		Function struct {
			Name string `json:"name"`
		} `json:"function"`
	}{Type: "function", Function: struct {
		Name string `json:"name"`
	}{Name: name}})
}

func marshalNamedToolChoice(kind, name string) ([]byte, error) {
	return json.Marshal(struct {
		Type string `json:"type"`
		Name string `json:"name"`
	}{Type: kind, Name: name})
}

func validateToolChoiceTools(tools []map[string]JSONValue) error {
	if tools == nil {
		return fmt.Errorf("allowed-tools tools must be an array")
	}
	for index, tool := range tools {
		if tool == nil {
			return fmt.Errorf("allowed-tools tool %d must be an object", index)
		}
	}
	return nil
}

func toolChoiceIsObject(data []byte, surface string) (bool, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return false, fmt.Errorf("%s must not be null", surface)
	}
	switch trimmed[0] {
	case '{':
		return true, nil
	case '"':
		return false, nil
	default:
		return false, fmt.Errorf("%s must be a string or object", surface)
	}
}

func unmarshalToolChoiceMode[T ~string](data []byte, surface string, valid func(T) bool) (T, error) {
	var choice T
	if err := unmarshalStringEnum(data, &choice, surface, valid); err != nil {
		return choice, err
	}
	return choice, nil
}

func unmarshalNamedToolChoice(data []byte, surface, kind string) (string, error) {
	fields, err := decodeStrictObject(data, surface, "type", "name")
	if err != nil {
		return "", err
	}
	if err := requireDiscriminator(fields, surface, kind); err != nil {
		return "", err
	}
	return requireStringField(fields, surface, "name")
}

func unmarshalFunctionToolChoice(data []byte, surface string) (string, error) {
	fields, err := decodeStrictObject(data, surface, "type", "function")
	if err != nil {
		return "", err
	}
	if err := requireDiscriminator(fields, surface, "function"); err != nil {
		return "", err
	}
	functionJSON, ok := fields["function"]
	if !ok {
		return "", fmt.Errorf("missing %s field %q", surface, "function")
	}
	functionFields, err := decodeStrictObject(functionJSON, surface+" function", "name")
	if err != nil {
		return "", err
	}
	return requireStringField(functionFields, surface+" function", "name")
}

func requireDiscriminator(fields map[string]json.RawMessage, surface, want string) error {
	discriminator, err := requireStringField(fields, surface, "type")
	if err != nil {
		return err
	}
	if discriminator != want {
		return fmt.Errorf("invalid %s discriminator %q", surface, discriminator)
	}
	return nil
}

func requireStringField(fields map[string]json.RawMessage, surface, field string) (string, error) {
	raw, ok := fields[field]
	if !ok {
		return "", fmt.Errorf("missing %s field %q", surface, field)
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", fmt.Errorf("%s field %q must be a string: %w", surface, field, err)
	}
	return value, nil
}

func unmarshalAnthropicToolChoice(data []byte) (AnthropicToolChoice, error) {
	isObject, err := toolChoiceIsObject(data, "anthropic tool choice")
	if err != nil {
		return nil, err
	}
	if !isObject {
		return unmarshalToolChoiceMode(data, "anthropic tool choice", func(value AnthropicToolChoiceMode) bool { return value.valid() })
	}
	name, err := unmarshalNamedToolChoice(data, "anthropic tool choice", "tool")
	if err != nil {
		return nil, err
	}
	return NewAnthropicToolChoiceTool(name), nil
}

func unmarshalBedrockToolChoice(data []byte) (BedrockToolChoice, error) {
	isObject, err := toolChoiceIsObject(data, "bedrock tool choice")
	if err != nil {
		return nil, err
	}
	if !isObject {
		return unmarshalToolChoiceMode(data, "bedrock tool choice", func(value BedrockToolChoiceMode) bool { return value.valid() })
	}
	name, err := unmarshalNamedToolChoice(data, "bedrock tool choice", "tool")
	if err != nil {
		return nil, err
	}
	return NewBedrockToolChoiceTool(name), nil
}

func unmarshalMistralToolChoice(data []byte) (MistralToolChoice, error) {
	isObject, err := toolChoiceIsObject(data, "mistral tool choice")
	if err != nil {
		return nil, err
	}
	if !isObject {
		return unmarshalToolChoiceMode(data, "mistral tool choice", func(value MistralToolChoiceMode) bool { return value.valid() })
	}
	name, err := unmarshalFunctionToolChoice(data, "mistral tool choice")
	if err != nil {
		return nil, err
	}
	return NewMistralToolChoiceFunction(name), nil
}

func unmarshalPiMessagesToolChoice(data []byte) (PiMessagesToolChoice, error) {
	isObject, err := toolChoiceIsObject(data, "Pi Messages tool choice")
	if err != nil {
		return nil, err
	}
	if !isObject {
		return unmarshalToolChoiceMode(data, "Pi Messages tool choice", func(value PiMessagesToolChoiceMode) bool { return value.valid() })
	}
	name, err := unmarshalFunctionToolChoice(data, "Pi Messages tool choice")
	if err != nil {
		return nil, err
	}
	return NewPiMessagesToolChoiceFunction(name), nil
}

func unmarshalOpenAIChatToolChoice(data []byte) (OpenAIChatToolChoice, error) {
	isObject, err := toolChoiceIsObject(data, "OpenAI Chat tool choice")
	if err != nil {
		return nil, err
	}
	if !isObject {
		return unmarshalToolChoiceMode(data, "OpenAI Chat tool choice", func(value OpenAIChatToolChoiceMode) bool { return value.valid() })
	}
	fields, err := decodeStrictObject(data, "OpenAI Chat tool choice", "type", "function", "custom", "allowed_tools")
	if err != nil {
		return nil, err
	}
	discriminator, err := requireStringField(fields, "OpenAI Chat tool choice", "type")
	if err != nil {
		return nil, err
	}
	switch discriminator {
	case "function":
		if _, ok := fields["custom"]; ok {
			return nil, fmt.Errorf("unexpected OpenAI Chat custom field")
		}
		if _, ok := fields["allowed_tools"]; ok {
			return nil, fmt.Errorf("unexpected OpenAI Chat allowed_tools field")
		}
		name, err := unmarshalNestedName(fields, "OpenAI Chat tool choice", "function")
		if err != nil {
			return nil, err
		}
		return NewOpenAIChatToolChoiceFunction(name), nil
	case "custom":
		if _, ok := fields["function"]; ok {
			return nil, fmt.Errorf("unexpected OpenAI Chat function field")
		}
		if _, ok := fields["allowed_tools"]; ok {
			return nil, fmt.Errorf("unexpected OpenAI Chat allowed_tools field")
		}
		name, err := unmarshalNestedName(fields, "OpenAI Chat tool choice", "custom")
		if err != nil {
			return nil, err
		}
		return NewOpenAIChatToolChoiceCustom(name), nil
	case "allowed_tools":
		if _, ok := fields["function"]; ok {
			return nil, fmt.Errorf("unexpected OpenAI Chat function field")
		}
		if _, ok := fields["custom"]; ok {
			return nil, fmt.Errorf("unexpected OpenAI Chat custom field")
		}
		allowedJSON, ok := fields["allowed_tools"]
		if !ok {
			return nil, fmt.Errorf("missing OpenAI Chat allowed_tools field")
		}
		mode, tools, err := unmarshalAllowedTools(allowedJSON, "OpenAI Chat allowed tools")
		if err != nil {
			return nil, err
		}
		return NewOpenAIChatToolChoiceAllowed(mode, tools), nil
	default:
		return nil, fmt.Errorf("invalid OpenAI Chat tool choice discriminator %q", discriminator)
	}
}

func unmarshalNestedName(fields map[string]json.RawMessage, surface, field string) (string, error) {
	raw, ok := fields[field]
	if !ok {
		return "", fmt.Errorf("missing %s field %q", surface, field)
	}
	nested, err := decodeStrictObject(raw, surface+" "+field, "name")
	if err != nil {
		return "", err
	}
	return requireStringField(nested, surface+" "+field, "name")
}

func unmarshalAllowedTools(data []byte, surface string) (ToolChoiceAllowedMode, []map[string]JSONValue, error) {
	fields, err := decodeStrictObject(data, surface, "mode", "tools")
	if err != nil {
		return "", nil, err
	}
	modeJSON, ok := fields["mode"]
	if !ok {
		return "", nil, fmt.Errorf("missing %s mode", surface)
	}
	var mode ToolChoiceAllowedMode
	if err := json.Unmarshal(modeJSON, &mode); err != nil {
		return "", nil, err
	}
	toolsJSON, ok := fields["tools"]
	if !ok || bytes.Equal(bytes.TrimSpace(toolsJSON), []byte("null")) {
		return "", nil, fmt.Errorf("missing %s tools array", surface)
	}
	var tools []map[string]JSONValue
	if err := json.Unmarshal(toolsJSON, &tools); err != nil {
		return "", nil, fmt.Errorf("invalid %s tools: %w", surface, err)
	}
	if err := validateToolChoiceTools(tools); err != nil {
		return "", nil, err
	}
	return mode, tools, nil
}

func unmarshalOpenAIResponsesToolChoice(data []byte) (OpenAIResponsesToolChoice, error) {
	isObject, err := toolChoiceIsObject(data, "OpenAI Responses tool choice")
	if err != nil {
		return nil, err
	}
	if !isObject {
		return unmarshalToolChoiceMode(data, "OpenAI Responses tool choice", func(value OpenAIResponsesToolChoiceMode) bool { return value.valid() })
	}
	fields, err := decodeStrictObject(data, "OpenAI Responses tool choice", "type", "name", "server_label", "mode", "tools")
	if err != nil {
		return nil, err
	}
	discriminator, err := requireStringField(fields, "OpenAI Responses tool choice", "type")
	if err != nil {
		return nil, err
	}
	switch discriminator {
	case "function", "custom":
		if len(fields) != 2 {
			return nil, fmt.Errorf("unexpected %s tool choice fields", discriminator)
		}
		name, err := requireStringField(fields, "OpenAI Responses tool choice", "name")
		if err != nil {
			return nil, err
		}
		if discriminator == "function" {
			return NewOpenAIResponsesToolChoiceFunction(name), nil
		}
		return NewOpenAIResponsesToolChoiceCustom(name), nil
	case "mcp":
		if len(fields) == 1 {
			return NewOpenAIResponsesToolChoiceHosted(OpenAIResponsesHostedToolMCP), nil
		}
		if _, ok := fields["mode"]; ok {
			return nil, fmt.Errorf("unexpected MCP mode field")
		}
		if _, ok := fields["tools"]; ok {
			return nil, fmt.Errorf("unexpected MCP tools field")
		}
		serverLabel, err := requireStringField(fields, "OpenAI Responses MCP tool choice", "server_label")
		if err != nil {
			return nil, err
		}
		nameJSON, hasName := fields["name"]
		if !hasName {
			return NewOpenAIResponsesToolChoiceMCP(serverLabel), nil
		}
		if bytes.Equal(bytes.TrimSpace(nameJSON), []byte("null")) {
			return OpenAIResponsesToolChoiceMCP{ServerLabel: serverLabel, Name: Null[string]()}, nil
		}
		name, err := requireStringField(fields, "OpenAI Responses MCP tool choice", "name")
		if err != nil {
			return nil, err
		}
		return NewOpenAIResponsesToolChoiceNamedMCP(serverLabel, name), nil
	case "allowed_tools":
		allowed := make(map[string]json.RawMessage, 2)
		for _, field := range []string{"mode", "tools"} {
			if raw, ok := fields[field]; ok {
				allowed[field] = raw
			}
		}
		if len(fields) != 3 {
			return nil, fmt.Errorf("unexpected OpenAI Responses allowed-tools fields")
		}
		encodedAllowed, err := json.Marshal(allowed)
		if err != nil {
			return nil, err
		}
		mode, tools, err := unmarshalAllowedTools(encodedAllowed, "OpenAI Responses allowed tools")
		if err != nil {
			return nil, err
		}
		return NewOpenAIResponsesToolChoiceAllowed(mode, tools), nil
	case "apply_patch":
		if len(fields) != 1 {
			return nil, fmt.Errorf("unexpected apply_patch tool choice fields")
		}
		return OpenAIResponsesToolChoiceApplyPatch{}, nil
	case "shell":
		if len(fields) != 1 {
			return nil, fmt.Errorf("unexpected shell tool choice fields")
		}
		return OpenAIResponsesToolChoiceShell{}, nil
	default:
		hosted := OpenAIResponsesHostedTool(discriminator)
		if !hosted.valid() || len(fields) != 1 {
			return nil, fmt.Errorf("invalid OpenAI Responses tool choice discriminator %q", discriminator)
		}
		return NewOpenAIResponsesToolChoiceHosted(hosted), nil
	}
}

type AnthropicOptions struct {
	StreamOptions
	ThinkingEnabled      *bool                     `json:"thinkingEnabled,omitempty"`
	ThinkingBudgetTokens *int64                    `json:"thinkingBudgetTokens,omitempty"`
	Effort               *AnthropicEffort          `json:"effort,omitempty"`
	ThinkingDisplay      *AnthropicThinkingDisplay `json:"thinkingDisplay,omitempty"`
	InterleavedThinking  *bool                     `json:"interleavedThinking,omitempty"`
	ToolChoice           AnthropicToolChoice       `json:"toolChoice,omitempty"`
	Client               any                       `json:"-"`
}

func (o *AnthropicOptions) UnmarshalJSON(data []byte) error {
	type plain AnthropicOptions
	var wire struct {
		*plain
		ToolChoice json.RawMessage `json:"toolChoice"`
	}
	decoded := plain{}
	wire.plain = &decoded
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	if wire.ToolChoice != nil {
		choice, err := unmarshalAnthropicToolChoice(wire.ToolChoice)
		if err != nil {
			return err
		}
		decoded.ToolChoice = choice
	}
	*o = AnthropicOptions(decoded)
	return nil
}

type AzureOpenAIResponsesOptions struct {
	StreamOptions
	ReasoningEffort     *OpenAIReasoningEffort           `json:"reasoningEffort,omitempty"`
	ReasoningSummary    Optional[OpenAIReasoningSummary] `json:"reasoningSummary,omitzero"`
	AzureAPIVersion     *string                          `json:"azureApiVersion,omitempty"`
	AzureResourceName   *string                          `json:"azureResourceName,omitempty"`
	AzureBaseURL        *string                          `json:"azureBaseUrl,omitempty"`
	AzureDeploymentName *string                          `json:"azureDeploymentName,omitempty"`
}

type BedrockOptions struct {
	StreamOptions
	Region              *string                 `json:"region,omitempty"`
	Profile             *string                 `json:"profile,omitempty"`
	ToolChoice          BedrockToolChoice       `json:"toolChoice,omitempty"`
	Reasoning           *ThinkingLevel          `json:"reasoning,omitempty"`
	ThinkingBudgets     *ThinkingBudgets        `json:"thinkingBudgets,omitempty"`
	InterleavedThinking *bool                   `json:"interleavedThinking,omitempty"`
	ThinkingDisplay     *BedrockThinkingDisplay `json:"thinkingDisplay,omitempty"`
	RequestMetadata     map[string]string       `json:"requestMetadata,omitempty"`
	BearerToken         *string                 `json:"bearerToken,omitempty"`
}

func (o *BedrockOptions) UnmarshalJSON(data []byte) error {
	type plain BedrockOptions
	var wire struct {
		*plain
		ToolChoice json.RawMessage `json:"toolChoice"`
	}
	decoded := plain{}
	wire.plain = &decoded
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	if wire.ToolChoice != nil {
		choice, err := unmarshalBedrockToolChoice(wire.ToolChoice)
		if err != nil {
			return err
		}
		decoded.ToolChoice = choice
	}
	*o = BedrockOptions(decoded)
	return nil
}

type GoogleThinkingOptions struct {
	Enabled      bool                 `json:"enabled"`
	BudgetTokens *int64               `json:"budgetTokens,omitempty"`
	Level        *GoogleThinkingLevel `json:"level,omitempty"`
}

type GoogleOptions struct {
	StreamOptions
	ToolChoice *GoogleToolChoice      `json:"toolChoice,omitempty"`
	Thinking   *GoogleThinkingOptions `json:"thinking,omitempty"`
}

type GoogleVertexOptions struct {
	StreamOptions
	ToolChoice *GoogleToolChoice      `json:"toolChoice,omitempty"`
	Thinking   *GoogleThinkingOptions `json:"thinking,omitempty"`
	Project    *string                `json:"project,omitempty"`
	Location   *string                `json:"location,omitempty"`
}

type MistralOptions struct {
	StreamOptions
	ToolChoice      MistralToolChoice       `json:"toolChoice,omitempty"`
	PromptMode      *MistralPromptMode      `json:"promptMode,omitempty"`
	ReasoningEffort *MistralReasoningEffort `json:"reasoningEffort,omitempty"`
}

func (o *MistralOptions) UnmarshalJSON(data []byte) error {
	type plain MistralOptions
	var wire struct {
		*plain
		ToolChoice json.RawMessage `json:"toolChoice"`
	}
	decoded := plain{}
	wire.plain = &decoded
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	if wire.ToolChoice != nil {
		choice, err := unmarshalMistralToolChoice(wire.ToolChoice)
		if err != nil {
			return err
		}
		decoded.ToolChoice = choice
	}
	*o = MistralOptions(decoded)
	return nil
}

type OpenAICodexResponsesOptions struct {
	StreamOptions
	ReasoningEffort  *CodexReasoningEffort           `json:"reasoningEffort,omitempty"`
	ReasoningSummary Optional[CodexReasoningSummary] `json:"reasoningSummary,omitzero"`
	ServiceTier      Optional[OpenAIServiceTier]     `json:"serviceTier,omitzero"`
	TextVerbosity    *TextVerbosity                  `json:"textVerbosity,omitempty"`
	ToolChoice       *CodexToolChoice                `json:"toolChoice,omitempty"`
}

type OpenAICompletionsOptions struct {
	StreamOptions
	ToolChoice      OpenAIChatToolChoice   `json:"toolChoice,omitempty"`
	ReasoningEffort *OpenAIReasoningEffort `json:"reasoningEffort,omitempty"`
	ThinkingBudgets *ThinkingBudgets       `json:"thinkingBudgets,omitempty"`
}

func (o *OpenAICompletionsOptions) UnmarshalJSON(data []byte) error {
	type plain OpenAICompletionsOptions
	var wire struct {
		*plain
		ToolChoice json.RawMessage `json:"toolChoice"`
	}
	decoded := plain{}
	wire.plain = &decoded
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	if wire.ToolChoice != nil {
		choice, err := unmarshalOpenAIChatToolChoice(wire.ToolChoice)
		if err != nil {
			return err
		}
		decoded.ToolChoice = choice
	}
	*o = OpenAICompletionsOptions(decoded)
	return nil
}

type OpenAIResponsesOptions struct {
	StreamOptions
	ReasoningEffort  *OpenAIReasoningEffort           `json:"reasoningEffort,omitempty"`
	ReasoningSummary Optional[OpenAIReasoningSummary] `json:"reasoningSummary,omitzero"`
	ServiceTier      Optional[OpenAIServiceTier]      `json:"serviceTier,omitzero"`
	ToolChoice       OpenAIResponsesToolChoice        `json:"toolChoice,omitempty"`
}

func (o *OpenAIResponsesOptions) UnmarshalJSON(data []byte) error {
	type plain OpenAIResponsesOptions
	var wire struct {
		*plain
		ToolChoice json.RawMessage `json:"toolChoice"`
	}
	decoded := plain{}
	wire.plain = &decoded
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	if wire.ToolChoice != nil {
		choice, err := unmarshalOpenAIResponsesToolChoice(wire.ToolChoice)
		if err != nil {
			return err
		}
		decoded.ToolChoice = choice
	}
	*o = OpenAIResponsesOptions(decoded)
	return nil
}

type PiMessagesOptions struct {
	StreamOptions
	Reasoning  *ThinkingLevel       `json:"reasoning,omitempty"`
	ToolChoice PiMessagesToolChoice `json:"toolChoice,omitempty"`
	Debug      *bool                `json:"debug,omitempty"`
}

func (o *PiMessagesOptions) UnmarshalJSON(data []byte) error {
	type plain PiMessagesOptions
	var wire struct {
		*plain
		ToolChoice json.RawMessage `json:"toolChoice"`
	}
	decoded := plain{}
	wire.plain = &decoded
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	if wire.ToolChoice != nil {
		choice, err := unmarshalPiMessagesToolChoice(wire.ToolChoice)
		if err != nil {
			return err
		}
		decoded.ToolChoice = choice
	}
	*o = PiMessagesOptions(decoded)
	return nil
}

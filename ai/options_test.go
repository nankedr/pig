package ai_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/nankedr/pig/ai"
)

func TestSimpleStreamOptionsDeferredRoundTripsAllVariants(t *testing.T) {
	t.Parallel()

	window := ai.DeferredWindow1Hour
	tests := []struct {
		name    string
		options ai.SimpleStreamOptions
		want    string
		check   func(*testing.T, ai.DeferredOption)
	}{
		{
			name: "absent",
			want: `{}`,
			check: func(t *testing.T, got ai.DeferredOption) {
				t.Helper()
				if got != nil {
					t.Fatalf("Deferred = %#v, want nil", got)
				}
			},
		},
		{
			name:    "false",
			options: ai.SimpleStreamOptions{Deferred: ai.DeferredBoolean{Enabled: false}},
			want:    `{"deferred":false}`,
			check: func(t *testing.T, got ai.DeferredOption) {
				t.Helper()
				value, ok := got.(ai.DeferredBoolean)
				if !ok || value.Enabled {
					t.Fatalf("Deferred = %#v, want DeferredBoolean(false)", got)
				}
			},
		},
		{
			name:    "true",
			options: ai.SimpleStreamOptions{Deferred: ai.DeferredBoolean{Enabled: true}},
			want:    `{"deferred":true}`,
			check: func(t *testing.T, got ai.DeferredOption) {
				t.Helper()
				value, ok := got.(ai.DeferredBoolean)
				if !ok || !value.Enabled {
					t.Fatalf("Deferred = %#v, want DeferredBoolean(true)", got)
				}
			},
		},
		{
			name:    "default window",
			options: ai.SimpleStreamOptions{Deferred: ai.DeferredWindowOptions{}},
			want:    `{"deferred":{}}`,
			check: func(t *testing.T, got ai.DeferredOption) {
				t.Helper()
				value, ok := got.(ai.DeferredWindowOptions)
				if !ok || value.Window != nil {
					t.Fatalf("Deferred = %#v, want DeferredWindowOptions with no window", got)
				}
			},
		},
		{
			name:    "explicit window",
			options: ai.SimpleStreamOptions{Deferred: ai.DeferredWindowOptions{Window: &window}},
			want:    `{"deferred":{"window":"1h"}}`,
			check: func(t *testing.T, got ai.DeferredOption) {
				t.Helper()
				value, ok := got.(ai.DeferredWindowOptions)
				if !ok || value.Window == nil || *value.Window != ai.DeferredWindow1Hour {
					t.Fatalf("Deferred = %#v, want DeferredWindowOptions(1h)", got)
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			encoded, err := json.Marshal(test.options)
			if err != nil {
				t.Fatalf("json.Marshal(SimpleStreamOptions) error = %v", err)
			}
			if string(encoded) != test.want {
				t.Fatalf("json.Marshal(SimpleStreamOptions) = %s, want %s", encoded, test.want)
			}

			var decoded ai.SimpleStreamOptions
			if err := json.Unmarshal(encoded, &decoded); err != nil {
				t.Fatalf("json.Unmarshal(SimpleStreamOptions) error = %v", err)
			}
			test.check(t, decoded.Deferred)
		})
	}
}

func TestProviderToolChoicesRoundTripAllSealedVariants(t *testing.T) {
	t.Parallel()

	chatTools := []map[string]ai.JSONValue{{"type": "function", "function": map[string]ai.JSONValue{"name": "lookup"}}}
	responseTools := []map[string]ai.JSONValue{{"type": "function", "name": "lookup"}}
	tests := []struct {
		name    string
		options any
		want    string
		decode  func() any
	}{
		{name: "anthropic auto", options: ai.AnthropicOptions{ToolChoice: ai.AnthropicToolChoiceAuto}, want: `{"toolChoice":"auto"}`, decode: func() any { return &ai.AnthropicOptions{} }},
		{name: "anthropic named tool", options: ai.AnthropicOptions{ToolChoice: ai.NewAnthropicToolChoiceTool("lookup")}, want: `{"toolChoice":{"type":"tool","name":"lookup"}}`, decode: func() any { return &ai.AnthropicOptions{} }},
		{name: "bedrock none", options: ai.BedrockOptions{ToolChoice: ai.BedrockToolChoiceNone}, want: `{"toolChoice":"none"}`, decode: func() any { return &ai.BedrockOptions{} }},
		{name: "bedrock named tool", options: ai.BedrockOptions{ToolChoice: ai.NewBedrockToolChoiceTool("lookup")}, want: `{"toolChoice":{"type":"tool","name":"lookup"}}`, decode: func() any { return &ai.BedrockOptions{} }},
		{name: "mistral required", options: ai.MistralOptions{ToolChoice: ai.MistralToolChoiceRequired}, want: `{"toolChoice":"required"}`, decode: func() any { return &ai.MistralOptions{} }},
		{name: "mistral named function", options: ai.MistralOptions{ToolChoice: ai.NewMistralToolChoiceFunction("lookup")}, want: `{"toolChoice":{"type":"function","function":{"name":"lookup"}}}`, decode: func() any { return &ai.MistralOptions{} }},
		{name: "openai chat auto", options: ai.OpenAICompletionsOptions{ToolChoice: ai.OpenAIChatToolChoiceAuto}, want: `{"toolChoice":"auto"}`, decode: func() any { return &ai.OpenAICompletionsOptions{} }},
		{name: "openai chat function", options: ai.OpenAICompletionsOptions{ToolChoice: ai.NewOpenAIChatToolChoiceFunction("lookup")}, want: `{"toolChoice":{"type":"function","function":{"name":"lookup"}}}`, decode: func() any { return &ai.OpenAICompletionsOptions{} }},
		{name: "openai chat custom", options: ai.OpenAICompletionsOptions{ToolChoice: ai.NewOpenAIChatToolChoiceCustom("lookup")}, want: `{"toolChoice":{"type":"custom","custom":{"name":"lookup"}}}`, decode: func() any { return &ai.OpenAICompletionsOptions{} }},
		{name: "openai chat allowed tools", options: ai.OpenAICompletionsOptions{ToolChoice: ai.NewOpenAIChatToolChoiceAllowed(ai.ToolChoiceAllowedModeRequired, chatTools)}, want: `{"toolChoice":{"type":"allowed_tools","allowed_tools":{"mode":"required","tools":[{"function":{"name":"lookup"},"type":"function"}]}}}`, decode: func() any { return &ai.OpenAICompletionsOptions{} }},
		{name: "openai responses auto", options: ai.OpenAIResponsesOptions{ToolChoice: ai.OpenAIResponsesToolChoiceAuto}, want: `{"toolChoice":"auto"}`, decode: func() any { return &ai.OpenAIResponsesOptions{} }},
		{name: "openai responses function", options: ai.OpenAIResponsesOptions{ToolChoice: ai.NewOpenAIResponsesToolChoiceFunction("lookup")}, want: `{"toolChoice":{"type":"function","name":"lookup"}}`, decode: func() any { return &ai.OpenAIResponsesOptions{} }},
		{name: "openai responses custom", options: ai.OpenAIResponsesOptions{ToolChoice: ai.NewOpenAIResponsesToolChoiceCustom("lookup")}, want: `{"toolChoice":{"type":"custom","name":"lookup"}}`, decode: func() any { return &ai.OpenAIResponsesOptions{} }},
		{name: "openai responses mcp", options: ai.OpenAIResponsesOptions{ToolChoice: ai.NewOpenAIResponsesToolChoiceMCP("server")}, want: `{"toolChoice":{"type":"mcp","server_label":"server"}}`, decode: func() any { return &ai.OpenAIResponsesOptions{} }},
		{name: "openai responses named mcp", options: ai.OpenAIResponsesOptions{ToolChoice: ai.NewOpenAIResponsesToolChoiceNamedMCP("server", "lookup")}, want: `{"toolChoice":{"type":"mcp","server_label":"server","name":"lookup"}}`, decode: func() any { return &ai.OpenAIResponsesOptions{} }},
		{name: "openai responses allowed tools", options: ai.OpenAIResponsesOptions{ToolChoice: ai.NewOpenAIResponsesToolChoiceAllowed(ai.ToolChoiceAllowedModeAuto, responseTools)}, want: `{"toolChoice":{"type":"allowed_tools","mode":"auto","tools":[{"name":"lookup","type":"function"}]}}`, decode: func() any { return &ai.OpenAIResponsesOptions{} }},
		{name: "openai responses hosted", options: ai.OpenAIResponsesOptions{ToolChoice: ai.NewOpenAIResponsesToolChoiceHosted(ai.OpenAIResponsesHostedToolFileSearch)}, want: `{"toolChoice":{"type":"file_search"}}`, decode: func() any { return &ai.OpenAIResponsesOptions{} }},
		{name: "openai responses hosted mcp", options: ai.OpenAIResponsesOptions{ToolChoice: ai.NewOpenAIResponsesToolChoiceHosted(ai.OpenAIResponsesHostedToolMCP)}, want: `{"toolChoice":{"type":"mcp"}}`, decode: func() any { return &ai.OpenAIResponsesOptions{} }},
		{name: "openai responses apply patch", options: ai.OpenAIResponsesOptions{ToolChoice: ai.OpenAIResponsesToolChoiceApplyPatch{}}, want: `{"toolChoice":{"type":"apply_patch"}}`, decode: func() any { return &ai.OpenAIResponsesOptions{} }},
		{name: "openai responses shell", options: ai.OpenAIResponsesOptions{ToolChoice: ai.OpenAIResponsesToolChoiceShell{}}, want: `{"toolChoice":{"type":"shell"}}`, decode: func() any { return &ai.OpenAIResponsesOptions{} }},
		{name: "pi messages required", options: ai.PiMessagesOptions{ToolChoice: ai.PiMessagesToolChoiceRequired}, want: `{"toolChoice":"required"}`, decode: func() any { return &ai.PiMessagesOptions{} }},
		{name: "pi messages named function", options: ai.PiMessagesOptions{ToolChoice: ai.NewPiMessagesToolChoiceFunction("lookup")}, want: `{"toolChoice":{"type":"function","function":{"name":"lookup"}}}`, decode: func() any { return &ai.PiMessagesOptions{} }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			encoded, err := json.Marshal(test.options)
			if err != nil {
				t.Fatalf("json.Marshal(%T) error = %v", test.options, err)
			}
			if string(encoded) != test.want {
				t.Fatalf("json.Marshal(%T) = %s, want %s", test.options, encoded, test.want)
			}

			decoded := test.decode()
			if err := json.Unmarshal(encoded, decoded); err != nil {
				t.Fatalf("json.Unmarshal(%T) error = %v", decoded, err)
			}
			roundTrip, err := json.Marshal(decoded)
			if err != nil {
				t.Fatalf("json.Marshal(round trip %T) error = %v", decoded, err)
			}
			if !reflect.DeepEqual(roundTrip, encoded) {
				t.Fatalf("round trip = %s, want %s", roundTrip, encoded)
			}
		})
	}
}

func TestProviderToolChoicesRejectValuesOutsideSealedUnions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		input  string
		decode func() any
	}{
		{name: "anthropic null", input: `{"toolChoice":null}`, decode: func() any { return &ai.AnthropicOptions{} }},
		{name: "anthropic unknown string", input: `{"toolChoice":"required"}`, decode: func() any { return &ai.AnthropicOptions{} }},
		{name: "anthropic unknown field", input: `{"toolChoice":{"type":"tool","name":"lookup","future":true}}`, decode: func() any { return &ai.AnthropicOptions{} }},
		{name: "bedrock missing name", input: `{"toolChoice":{"type":"tool"}}`, decode: func() any { return &ai.BedrockOptions{} }},
		{name: "mistral wrong discriminator", input: `{"toolChoice":{"type":"tool","name":"lookup"}}`, decode: func() any { return &ai.MistralOptions{} }},
		{name: "mistral nested unknown field", input: `{"toolChoice":{"type":"function","function":{"name":"lookup","future":true}}}`, decode: func() any { return &ai.MistralOptions{} }},
		{name: "openai chat number", input: `{"toolChoice":1}`, decode: func() any { return &ai.OpenAICompletionsOptions{} }},
		{name: "openai chat bad allowed mode", input: `{"toolChoice":{"type":"allowed_tools","allowed_tools":{"mode":"none","tools":[]}}}`, decode: func() any { return &ai.OpenAICompletionsOptions{} }},
		{name: "openai chat non-object tool", input: `{"toolChoice":{"type":"allowed_tools","allowed_tools":{"mode":"auto","tools":[1]}}}`, decode: func() any { return &ai.OpenAICompletionsOptions{} }},
		{name: "openai responses missing server label", input: `{"toolChoice":{"type":"mcp","name":"lookup"}}`, decode: func() any { return &ai.OpenAIResponsesOptions{} }},
		{name: "openai responses unknown hosted type", input: `{"toolChoice":{"type":"future_search"}}`, decode: func() any { return &ai.OpenAIResponsesOptions{} }},
		{name: "openai responses tools null", input: `{"toolChoice":{"type":"allowed_tools","mode":"auto","tools":null}}`, decode: func() any { return &ai.OpenAIResponsesOptions{} }},
		{name: "pi messages unknown string", input: `{"toolChoice":"any"}`, decode: func() any { return &ai.PiMessagesOptions{} }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if err := json.Unmarshal([]byte(test.input), test.decode()); err == nil {
				t.Fatalf("json.Unmarshal(%s) error = nil", test.input)
			}
		})
	}
}

func TestProviderStringEnumsRoundTripEveryFixedValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		value  any
		decode func() any
	}{
		{name: "anthropic effort low", value: ai.AnthropicEffortLow, decode: func() any { return new(ai.AnthropicEffort) }},
		{name: "anthropic effort medium", value: ai.AnthropicEffortMedium, decode: func() any { return new(ai.AnthropicEffort) }},
		{name: "anthropic effort high", value: ai.AnthropicEffortHigh, decode: func() any { return new(ai.AnthropicEffort) }},
		{name: "anthropic effort xhigh", value: ai.AnthropicEffortXHigh, decode: func() any { return new(ai.AnthropicEffort) }},
		{name: "anthropic effort max", value: ai.AnthropicEffortMax, decode: func() any { return new(ai.AnthropicEffort) }},
		{name: "anthropic display summarized", value: ai.AnthropicThinkingDisplaySummarized, decode: func() any { return new(ai.AnthropicThinkingDisplay) }},
		{name: "anthropic display omitted", value: ai.AnthropicThinkingDisplayOmitted, decode: func() any { return new(ai.AnthropicThinkingDisplay) }},
		{name: "bedrock display summarized", value: ai.BedrockThinkingDisplaySummarized, decode: func() any { return new(ai.BedrockThinkingDisplay) }},
		{name: "bedrock display omitted", value: ai.BedrockThinkingDisplayOmitted, decode: func() any { return new(ai.BedrockThinkingDisplay) }},
		{name: "openai effort minimal", value: ai.OpenAIReasoningEffortMinimal, decode: func() any { return new(ai.OpenAIReasoningEffort) }},
		{name: "openai effort low", value: ai.OpenAIReasoningEffortLow, decode: func() any { return new(ai.OpenAIReasoningEffort) }},
		{name: "openai effort medium", value: ai.OpenAIReasoningEffortMedium, decode: func() any { return new(ai.OpenAIReasoningEffort) }},
		{name: "openai effort high", value: ai.OpenAIReasoningEffortHigh, decode: func() any { return new(ai.OpenAIReasoningEffort) }},
		{name: "openai effort xhigh", value: ai.OpenAIReasoningEffortXHigh, decode: func() any { return new(ai.OpenAIReasoningEffort) }},
		{name: "openai effort max", value: ai.OpenAIReasoningEffortMax, decode: func() any { return new(ai.OpenAIReasoningEffort) }},
		{name: "openai summary auto", value: ai.OpenAIReasoningSummaryAuto, decode: func() any { return new(ai.OpenAIReasoningSummary) }},
		{name: "openai summary detailed", value: ai.OpenAIReasoningSummaryDetailed, decode: func() any { return new(ai.OpenAIReasoningSummary) }},
		{name: "openai summary concise", value: ai.OpenAIReasoningSummaryConcise, decode: func() any { return new(ai.OpenAIReasoningSummary) }},
		{name: "codex effort none", value: ai.CodexReasoningEffortNone, decode: func() any { return new(ai.CodexReasoningEffort) }},
		{name: "codex effort minimal", value: ai.CodexReasoningEffortMinimal, decode: func() any { return new(ai.CodexReasoningEffort) }},
		{name: "codex effort low", value: ai.CodexReasoningEffortLow, decode: func() any { return new(ai.CodexReasoningEffort) }},
		{name: "codex effort medium", value: ai.CodexReasoningEffortMedium, decode: func() any { return new(ai.CodexReasoningEffort) }},
		{name: "codex effort high", value: ai.CodexReasoningEffortHigh, decode: func() any { return new(ai.CodexReasoningEffort) }},
		{name: "codex effort xhigh", value: ai.CodexReasoningEffortXHigh, decode: func() any { return new(ai.CodexReasoningEffort) }},
		{name: "codex effort max", value: ai.CodexReasoningEffortMax, decode: func() any { return new(ai.CodexReasoningEffort) }},
		{name: "codex summary auto", value: ai.CodexReasoningSummaryAuto, decode: func() any { return new(ai.CodexReasoningSummary) }},
		{name: "codex summary concise", value: ai.CodexReasoningSummaryConcise, decode: func() any { return new(ai.CodexReasoningSummary) }},
		{name: "codex summary detailed", value: ai.CodexReasoningSummaryDetailed, decode: func() any { return new(ai.CodexReasoningSummary) }},
		{name: "codex summary off", value: ai.CodexReasoningSummaryOff, decode: func() any { return new(ai.CodexReasoningSummary) }},
		{name: "codex summary on", value: ai.CodexReasoningSummaryOn, decode: func() any { return new(ai.CodexReasoningSummary) }},
		{name: "service tier auto", value: ai.OpenAIServiceTierAuto, decode: func() any { return new(ai.OpenAIServiceTier) }},
		{name: "service tier default", value: ai.OpenAIServiceTierDefault, decode: func() any { return new(ai.OpenAIServiceTier) }},
		{name: "service tier flex", value: ai.OpenAIServiceTierFlex, decode: func() any { return new(ai.OpenAIServiceTier) }},
		{name: "service tier scale", value: ai.OpenAIServiceTierScale, decode: func() any { return new(ai.OpenAIServiceTier) }},
		{name: "service tier priority", value: ai.OpenAIServiceTierPriority, decode: func() any { return new(ai.OpenAIServiceTier) }},
		{name: "text verbosity low", value: ai.TextVerbosityLow, decode: func() any { return new(ai.TextVerbosity) }},
		{name: "text verbosity medium", value: ai.TextVerbosityMedium, decode: func() any { return new(ai.TextVerbosity) }},
		{name: "text verbosity high", value: ai.TextVerbosityHigh, decode: func() any { return new(ai.TextVerbosity) }},
		{name: "google tool choice auto", value: ai.GoogleToolChoiceAuto, decode: func() any { return new(ai.GoogleToolChoice) }},
		{name: "google tool choice none", value: ai.GoogleToolChoiceNone, decode: func() any { return new(ai.GoogleToolChoice) }},
		{name: "google tool choice any", value: ai.GoogleToolChoiceAny, decode: func() any { return new(ai.GoogleToolChoice) }},
		{name: "google thinking unspecified", value: ai.GoogleThinkingLevelUnspecified, decode: func() any { return new(ai.GoogleThinkingLevel) }},
		{name: "google thinking minimal", value: ai.GoogleThinkingLevelMinimal, decode: func() any { return new(ai.GoogleThinkingLevel) }},
		{name: "google thinking low", value: ai.GoogleThinkingLevelLow, decode: func() any { return new(ai.GoogleThinkingLevel) }},
		{name: "google thinking medium", value: ai.GoogleThinkingLevelMedium, decode: func() any { return new(ai.GoogleThinkingLevel) }},
		{name: "google thinking high", value: ai.GoogleThinkingLevelHigh, decode: func() any { return new(ai.GoogleThinkingLevel) }},
		{name: "mistral prompt reasoning", value: ai.MistralPromptModeReasoning, decode: func() any { return new(ai.MistralPromptMode) }},
		{name: "mistral effort none", value: ai.MistralReasoningEffortNone, decode: func() any { return new(ai.MistralReasoningEffort) }},
		{name: "mistral effort high", value: ai.MistralReasoningEffortHigh, decode: func() any { return new(ai.MistralReasoningEffort) }},
		{name: "codex tool choice auto", value: ai.CodexToolChoiceAuto, decode: func() any { return new(ai.CodexToolChoice) }},
		{name: "codex tool choice none", value: ai.CodexToolChoiceNone, decode: func() any { return new(ai.CodexToolChoice) }},
		{name: "codex tool choice required", value: ai.CodexToolChoiceRequired, decode: func() any { return new(ai.CodexToolChoice) }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			encoded, err := json.Marshal(test.value)
			if err != nil {
				t.Fatalf("json.Marshal(%T(%v)) error = %v", test.value, test.value, err)
			}
			decoded := test.decode()
			if err := json.Unmarshal(encoded, decoded); err != nil {
				t.Fatalf("json.Unmarshal(%s, %T) error = %v", encoded, decoded, err)
			}
			roundTrip, err := json.Marshal(decoded)
			if err != nil {
				t.Fatalf("json.Marshal(round trip %T) error = %v", decoded, err)
			}
			if !reflect.DeepEqual(roundTrip, encoded) {
				t.Fatalf("round trip = %s, want %s", roundTrip, encoded)
			}
		})
	}
}

func TestProviderStringEnumsRejectInvalidValues(t *testing.T) {
	t.Parallel()

	decoders := []struct {
		name   string
		decode func() any
	}{
		{name: "anthropic effort", decode: func() any { return new(ai.AnthropicEffort) }},
		{name: "anthropic thinking display", decode: func() any { return new(ai.AnthropicThinkingDisplay) }},
		{name: "bedrock thinking display", decode: func() any { return new(ai.BedrockThinkingDisplay) }},
		{name: "openai reasoning effort", decode: func() any { return new(ai.OpenAIReasoningEffort) }},
		{name: "openai reasoning summary", decode: func() any { return new(ai.OpenAIReasoningSummary) }},
		{name: "codex reasoning effort", decode: func() any { return new(ai.CodexReasoningEffort) }},
		{name: "codex reasoning summary", decode: func() any { return new(ai.CodexReasoningSummary) }},
		{name: "openai service tier", decode: func() any { return new(ai.OpenAIServiceTier) }},
		{name: "text verbosity", decode: func() any { return new(ai.TextVerbosity) }},
		{name: "google tool choice", decode: func() any { return new(ai.GoogleToolChoice) }},
		{name: "google thinking level", decode: func() any { return new(ai.GoogleThinkingLevel) }},
		{name: "mistral prompt mode", decode: func() any { return new(ai.MistralPromptMode) }},
		{name: "mistral reasoning effort", decode: func() any { return new(ai.MistralReasoningEffort) }},
		{name: "codex tool choice", decode: func() any { return new(ai.CodexToolChoice) }},
	}
	for _, decoder := range decoders {
		t.Run(decoder.name, func(t *testing.T) {
			t.Parallel()
			for _, input := range []string{`"future"`, `null`} {
				if err := json.Unmarshal([]byte(input), decoder.decode()); err == nil {
					t.Errorf("json.Unmarshal(%s, %s) error = nil", input, decoder.name)
				}
			}
		})
	}
}

func TestNullableProviderEnumsKeepExplicitNullInOptions(t *testing.T) {
	t.Parallel()

	input := []byte(`{"reasoningSummary":null,"serviceTier":null}`)
	var options ai.OpenAIResponsesOptions
	if err := json.Unmarshal(input, &options); err != nil {
		t.Fatalf("json.Unmarshal(OpenAIResponsesOptions) error = %v", err)
	}
	if !options.ReasoningSummary.IsNull() || !options.ServiceTier.IsNull() {
		t.Fatalf("nullable enums = (%#v, %#v), want explicit nulls", options.ReasoningSummary, options.ServiceTier)
	}
	encoded, err := json.Marshal(options)
	if err != nil {
		t.Fatalf("json.Marshal(OpenAIResponsesOptions) error = %v", err)
	}
	if !reflect.DeepEqual(encoded, input) {
		t.Fatalf("json.Marshal(OpenAIResponsesOptions) = %s, want %s", encoded, input)
	}
}

func TestSimpleStreamOptionsDeferredRejectsInvalidJSON(t *testing.T) {
	t.Parallel()

	invalid := []string{
		`{"deferred":null}`,
		`{"deferred":"true"}`,
		`{"deferred":{"window":null}}`,
		`{"deferred":{"window":"7d"}}`,
		`{"deferred":{"window":"1h","future":true}}`,
	}
	for _, input := range invalid {
		var options ai.SimpleStreamOptions
		if err := json.Unmarshal([]byte(input), &options); err == nil {
			t.Errorf("json.Unmarshal(%s) error = nil", input)
		}
	}
}

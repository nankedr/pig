package ai_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/nankedr/pig/ai"
)

func TestAPIAndProviderIDsExposeKnownValuesWithoutClosingExtensions(t *testing.T) {
	t.Parallel()

	apis := map[ai.API]string{
		ai.APIOpenAICompletions:     "openai-completions",
		ai.APIMistralConversations:  "mistral-conversations",
		ai.APIOpenAIResponses:       "openai-responses",
		ai.APIAzureOpenAIResponses:  "azure-openai-responses",
		ai.APIOpenAICodexResponses:  "openai-codex-responses",
		ai.APIAnthropicMessages:     "anthropic-messages",
		ai.APIBedrockConverseStream: "bedrock-converse-stream",
		ai.APIGoogleGenerativeAI:    "google-generative-ai",
		ai.APIGoogleVertex:          "google-vertex",
		ai.APIPiMessages:            "pi-messages",
	}
	for got, want := range apis {
		if string(got) != want {
			t.Fatalf("API constant = %q, want %q", got, want)
		}
	}

	providers := map[ai.ProviderID]string{
		ai.ProviderIDAmazonBedrock:           "amazon-bedrock",
		ai.ProviderIDAntLing:                 "ant-ling",
		ai.ProviderIDAnthropic:               "anthropic",
		ai.ProviderIDGoogle:                  "google",
		ai.ProviderIDGoogleVertex:            "google-vertex",
		ai.ProviderIDOpenAI:                  "openai",
		ai.ProviderIDAzureOpenAIResponses:    "azure-openai-responses",
		ai.ProviderIDOpenAICodex:             "openai-codex",
		ai.ProviderIDRadius:                  "radius",
		ai.ProviderIDNVIDIA:                  "nvidia",
		ai.ProviderIDDeepSeek:                "deepseek",
		ai.ProviderIDGitHubCopilot:           "github-copilot",
		ai.ProviderIDXAI:                     "xai",
		ai.ProviderIDGroq:                    "groq",
		ai.ProviderIDCerebras:                "cerebras",
		ai.ProviderIDOpenRouter:              "openrouter",
		ai.ProviderIDVercelAIGateway:         "vercel-ai-gateway",
		ai.ProviderIDZAI:                     "zai",
		ai.ProviderIDZAICodingCN:             "zai-coding-cn",
		ai.ProviderIDMistral:                 "mistral",
		ai.ProviderIDMiniMax:                 "minimax",
		ai.ProviderIDMiniMaxCN:               "minimax-cn",
		ai.ProviderIDMoonshotAI:              "moonshotai",
		ai.ProviderIDMoonshotAICN:            "moonshotai-cn",
		ai.ProviderIDHuggingFace:             "huggingface",
		ai.ProviderIDFireworks:               "fireworks",
		ai.ProviderIDTogether:                "together",
		ai.ProviderIDBaseten:                 "baseten",
		ai.ProviderIDOpenCode:                "opencode",
		ai.ProviderIDOpenCodeGo:              "opencode-go",
		ai.ProviderIDKimiCoding:              "kimi-coding",
		ai.ProviderIDCloudflareWorkersAI:     "cloudflare-workers-ai",
		ai.ProviderIDCloudflareAIGateway:     "cloudflare-ai-gateway",
		ai.ProviderIDQwenTokenPlan:           "qwen-token-plan",
		ai.ProviderIDQwenTokenPlanCN:         "qwen-token-plan-cn",
		ai.ProviderIDQwenTokenPlanIndividual: "qwen-token-plan-individual",
		ai.ProviderIDXiaomi:                  "xiaomi",
		ai.ProviderIDXiaomiTokenPlanCN:       "xiaomi-token-plan-cn",
		ai.ProviderIDXiaomiTokenPlanAMS:      "xiaomi-token-plan-ams",
		ai.ProviderIDXiaomiTokenPlanSGP:      "xiaomi-token-plan-sgp",
	}
	for got, want := range providers {
		if string(got) != want {
			t.Fatalf("ProviderID constant = %q, want %q", got, want)
		}
	}

	if got := ai.API("acme-wire-v1"); got != "acme-wire-v1" {
		t.Fatalf("custom API = %q", got)
	}
	if got := ai.ProviderID("acme-cloud"); got != "acme-cloud" {
		t.Fatalf("custom ProviderID = %q", got)
	}
}

func TestCoreValueContractsPreserveEnumsAndOptionalBreakdowns(t *testing.T) {
	t.Parallel()

	values := []struct {
		name string
		got  string
		want string
	}{
		{name: "thinking minimal", got: string(ai.ThinkingLevelMinimal), want: "minimal"},
		{name: "thinking low", got: string(ai.ThinkingLevelLow), want: "low"},
		{name: "thinking medium", got: string(ai.ThinkingLevelMedium), want: "medium"},
		{name: "thinking high", got: string(ai.ThinkingLevelHigh), want: "high"},
		{name: "thinking xhigh", got: string(ai.ThinkingLevelXHigh), want: "xhigh"},
		{name: "thinking max", got: string(ai.ThinkingLevelMax), want: "max"},
		{name: "model thinking off", got: string(ai.ModelThinkingLevelOff), want: "off"},
		{name: "stop pending", got: string(ai.StopReasonPending), want: "pending"},
		{name: "stop done", got: string(ai.StopReasonStop), want: "stop"},
		{name: "stop length", got: string(ai.StopReasonLength), want: "length"},
		{name: "stop tool use", got: string(ai.StopReasonToolUse), want: "toolUse"},
		{name: "stop error", got: string(ai.StopReasonError), want: "error"},
		{name: "stop aborted", got: string(ai.StopReasonAborted), want: "aborted"},
		{name: "stop deferred", got: string(ai.StopReasonDeferred), want: "deferred"},
		{name: "transport sse", got: string(ai.TransportSSE), want: "sse"},
		{name: "transport websocket", got: string(ai.TransportWebSocket), want: "websocket"},
		{name: "transport cached websocket", got: string(ai.TransportWebSocketCached), want: "websocket-cached"},
		{name: "transport auto", got: string(ai.TransportAuto), want: "auto"},
		{name: "cache none", got: string(ai.CacheRetentionNone), want: "none"},
		{name: "cache short", got: string(ai.CacheRetentionShort), want: "short"},
		{name: "cache long", got: string(ai.CacheRetentionLong), want: "long"},
	}
	for _, value := range values {
		if value.got != value.want {
			t.Errorf("%s = %q, want %q", value.name, value.got, value.want)
		}
	}

	usage := ai.Usage{
		Input: 1, Output: 2, CacheRead: 3, CacheWrite: 4,
		CacheWrite1H: ai.Some[int64](0), Reasoning: ai.Null[int64](), TotalTokens: 10,
		Cost: ai.UsageCost{Input: .1, Output: .2, CacheRead: .3, CacheWrite: .4, Total: 1},
	}
	encoded, err := json.Marshal(usage)
	if err != nil {
		t.Fatalf("json.Marshal(Usage) error = %v", err)
	}
	var decoded ai.Usage
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("json.Unmarshal(Usage) error = %v", err)
	}
	if value, ok := decoded.CacheWrite1H.Value(); !ok || value != 0 {
		t.Fatalf("CacheWrite1H.Value() = (%d, %t), want (0, true)", value, ok)
	}
	if !decoded.Reasoning.IsNull() {
		t.Fatalf("Reasoning.IsNull() = false")
	}

	handle := ai.DeferredHandle{
		Provider: ai.ProviderID("custom-provider"), ModelID: "model-1",
		API: ai.API("custom-api"), ID: "job-1", Data: ai.Null[ai.JSONValue](),
	}
	if handle.Provider != "custom-provider" || handle.API != "custom-api" || !handle.Data.IsNull() {
		t.Fatalf("DeferredHandle = %#v", handle)
	}
}

func TestModelKeepsNullThinkingLevelsAndRawCustomCompat(t *testing.T) {
	t.Parallel()

	rawCompat := json.RawMessage(`{"vendor_mode":false,"limit":0}`)
	model := ai.Model{
		ID: "model-1", Name: "Model One", API: ai.API("acme-wire-v1"),
		Provider: ai.ProviderID("acme-cloud"), BaseURL: "https://example.invalid/v1",
		Reasoning: true,
		ThinkingLevelMap: map[ai.ModelThinkingLevel]ai.Optional[string]{
			ai.ModelThinkingLevelOff:   ai.Some("disabled"),
			ai.ModelThinkingLevelXHigh: ai.Null[string](),
		},
		Input: []ai.ModelInput{ai.ModelInputText, ai.ModelInputImage},
		Cost: ai.ModelCost{
			ModelCostRates: ai.ModelCostRates{Input: 1, Output: 2, CacheRead: .5, CacheWrite: 1.5},
			Tiers:          []ai.ModelCostTier{{ModelCostRates: ai.ModelCostRates{Input: 3, Output: 4}, InputTokensAbove: 200000}},
		},
		ContextWindow: 262144, MaxTokens: 8192,
		SamplingParams: map[string]json.RawMessage{"top_p": json.RawMessage(`0`)},
		Headers:        map[string]string{"x-model": ""},
		Compat:         ai.Some(rawCompat),
	}

	encoded, err := json.Marshal(model)
	if err != nil {
		t.Fatalf("json.Marshal(Model) error = %v", err)
	}
	var decoded ai.Model
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("json.Unmarshal(Model) error = %v", err)
	}
	if got := decoded.ThinkingLevelMap[ai.ModelThinkingLevelXHigh]; !got.IsNull() {
		t.Fatalf("xhigh mapping = %#v, want explicit null", got)
	}
	gotCompat, ok := decoded.Compat.Value()
	if !ok {
		t.Fatal("Compat.Value() missing")
	}
	var gotObject, wantObject map[string]any
	if err := json.Unmarshal(gotCompat, &gotObject); err != nil {
		t.Fatalf("json.Unmarshal(decoded compat) error = %v", err)
	}
	if err := json.Unmarshal(rawCompat, &wantObject); err != nil {
		t.Fatalf("json.Unmarshal(want compat) error = %v", err)
	}
	if len(gotObject) != len(wantObject) || gotObject["vendor_mode"] != false || gotObject["limit"] != float64(0) {
		t.Fatalf("Compat = %s, want semantic value %s", gotCompat, rawCompat)
	}
}

func TestToolCodecPreservesEveryConstrainedSamplingState(t *testing.T) {
	t.Parallel()

	// The large integer and exponent would be at risk if the schema were
	// decoded through map[string]any and regenerated instead of retained raw.
	schema := json.RawMessage(`{"type":"object","x-vendor-id":9007199254740993,"x-scale":1e+09,"required":["query"],"properties":{"query":{"type":"string"}}}`)
	tests := []struct {
		name            string
		config          ai.ConstrainedSampling
		wantJSONSnippet []byte
		wantType        any
	}{
		{name: "absent", wantType: nil},
		{name: "explicit false", config: ai.ConstrainedSamplingDisabled{}, wantJSONSnippet: []byte(`"constrainedSampling":false`), wantType: ai.ConstrainedSamplingDisabled{}},
		{name: "JSON schema", config: ai.JSONSchemaConstrainedSampling{Strict: ai.ConstrainedSamplingStrictRequire}, wantJSONSnippet: []byte(`"type":"json_schema"`), wantType: ai.JSONSchemaConstrainedSampling{}},
		{name: "grammar", config: ai.GrammarConstrainedSampling{Variants: ai.GrammarVariants{OpenAILark: ai.Some(`start: /[a-z]+/`), OpenAIRegex: ai.Some(`^[a-z]+$`)}}, wantJSONSnippet: []byte(`"type":"grammar"`), wantType: ai.GrammarConstrainedSampling{}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			tool := ai.Tool{Name: "search", Description: "Search things", Parameters: append(json.RawMessage(nil), schema...), ConstrainedSampling: test.config}
			encoded, err := json.Marshal(tool)
			if err != nil {
				t.Fatalf("json.Marshal(Tool) error = %v", err)
			}
			if test.wantJSONSnippet == nil {
				if bytes.Contains(encoded, []byte(`"constrainedSampling"`)) {
					t.Fatalf("json.Marshal(Tool) = %s, want constrainedSampling omitted", encoded)
				}
			} else if !bytes.Contains(encoded, test.wantJSONSnippet) {
				t.Fatalf("json.Marshal(Tool) = %s, want snippet %s", encoded, test.wantJSONSnippet)
			}

			var decoded ai.Tool
			if err := json.Unmarshal(encoded, &decoded); err != nil {
				t.Fatalf("json.Unmarshal(Tool) error = %v", err)
			}
			if decoded.Name != tool.Name || decoded.Description != tool.Description {
				t.Fatalf("decoded metadata = %#v, want %#v", decoded, tool)
			}
			if !bytes.Equal(decoded.Parameters, schema) {
				t.Fatalf("decoded schema = %s, want exact bytes %s", decoded.Parameters, schema)
			}
			if test.wantType == nil {
				if decoded.ConstrainedSampling != nil {
					t.Fatalf("decoded constrained sampling = %#v, want nil", decoded.ConstrainedSampling)
				}
			} else if reflect.TypeOf(decoded.ConstrainedSampling) != reflect.TypeOf(test.wantType) {
				t.Fatalf("decoded constrained sampling type = %T, want %T", decoded.ConstrainedSampling, test.wantType)
			}
		})
	}
}

func TestToolCodecRejectsUnknownOrInvalidConstrainedSampling(t *testing.T) {
	t.Parallel()

	inputs := []string{
		`{"name":"x","description":"x","parameters":{},"constrainedSampling":true}`,
		`{"name":"x","description":"x","parameters":{},"constrainedSampling":null}`,
		`{"name":"x","description":"x","parameters":{},"constrainedSampling":{"type":"future"}}`,
		`{"name":"x","description":"x","parameters":{},"constrainedSampling":{"type":"json_schema","strict":"sometimes"}}`,
	}
	for _, input := range inputs {
		var tool ai.Tool
		if err := json.Unmarshal([]byte(input), &tool); !errors.Is(err, ai.ErrCodec) {
			t.Errorf("json.Unmarshal(%s) error = %v, want ErrCodec", input, err)
		}
	}
}

func TestProviderRequestOptionsPreserveHeaderAndZeroValueIntent(t *testing.T) {
	t.Parallel()

	empty := ""
	zeroDuration := int64(0)
	zeroRetries := 0
	options := ai.ProviderRequestOptions{
		APIKey:          &empty,
		Headers:         ai.ProviderHeaders{"delete-me": nil, "empty-value": &empty},
		TimeoutMS:       &zeroDuration,
		MaxRetries:      &zeroRetries,
		MaxRetryDelayMS: &zeroDuration,
	}

	encoded, err := json.Marshal(options)
	if err != nil {
		t.Fatalf("json.Marshal(ProviderRequestOptions) error = %v", err)
	}
	var decoded ai.ProviderRequestOptions
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("json.Unmarshal(ProviderRequestOptions) error = %v", err)
	}
	if _, exists := decoded.Headers["missing"]; exists {
		t.Fatal("missing header key became present")
	}
	deleted, exists := decoded.Headers["delete-me"]
	if !exists || deleted != nil {
		t.Fatalf("delete header = (%v, %t), want (nil, true)", deleted, exists)
	}
	explicitEmpty, exists := decoded.Headers["empty-value"]
	if !exists || explicitEmpty == nil || *explicitEmpty != "" {
		t.Fatalf("empty header = (%v, %t), want pointer to empty string", explicitEmpty, exists)
	}
	if decoded.APIKey == nil || *decoded.APIKey != "" || decoded.TimeoutMS == nil || *decoded.TimeoutMS != 0 || decoded.MaxRetries == nil || *decoded.MaxRetries != 0 || decoded.MaxRetryDelayMS == nil || *decoded.MaxRetryDelayMS != 0 {
		t.Fatalf("explicit zero options were not preserved: %#v", decoded)
	}
}

func TestPayloadHookResultCanReplaceWithNilOrZero(t *testing.T) {
	t.Parallel()

	model := ai.Model{ID: "model-1"}
	tests := []struct {
		name string
		hook ai.PayloadHook
		want ai.JSONValue
	}{
		{
			name: "nil",
			hook: func(context.Context, ai.JSONValue, ai.Model) (ai.PayloadHookResult, error) {
				return ai.PayloadHookResult{Value: nil, Replace: true}, nil
			},
			want: nil,
		},
		{
			name: "zero",
			hook: func(context.Context, ai.JSONValue, ai.Model) (ai.PayloadHookResult, error) {
				return ai.PayloadHookResult{Value: 0, Replace: true}, nil
			},
			want: 0,
		},
	}
	for _, test := range tests {
		result, err := test.hook(context.Background(), map[string]any{"old": true}, model)
		if err != nil {
			t.Fatalf("%s hook error = %v", test.name, err)
		}
		if !result.Replace || !reflect.DeepEqual(result.Value, test.want) {
			t.Fatalf("%s result = %#v, want Replace and %#v", test.name, result, test.want)
		}
	}
}

func TestEraseAPIAdapterDecodesBeforeCallingTypedStream(t *testing.T) {
	t.Parallel()

	type adapterOptions struct {
		Enabled bool `json:"enabled"`
		Limit   int  `json:"limit"`
	}
	calls := 0
	descriptor := ai.APIAdapterDescriptor[adapterOptions]{
		API: ai.API("acme-wire-v1"),
		Stream: func(_ context.Context, _ ai.Model, _ ai.Context, options adapterOptions) *ai.AssistantMessageEventStream {
			calls++
			if options.Enabled || options.Limit != 0 {
				t.Fatalf("decoded options = %#v, want explicit zero values", options)
			}
			return ai.NewAssistantMessageEventStream()
		},
	}
	erased := ai.EraseAPIAdapter(descriptor)

	stream, err := erased.Stream(context.Background(), ai.Model{}, ai.Context{}, json.RawMessage(`{`))
	if err == nil || stream != nil || calls != 0 {
		t.Fatalf("malformed options = (%v, %v), calls=%d; want decode error, nil stream, zero calls", stream, err, calls)
	}
	stream, err = erased.Stream(context.Background(), ai.Model{}, ai.Context{}, json.RawMessage(`{"enabled":false,"limit":0}`))
	if err != nil || stream == nil || calls != 1 {
		t.Fatalf("valid options = (%v, %v), calls=%d; want stream, nil, one call", stream, err, calls)
	}
}

func TestEraseAPIAdapterPreservesCustomRawOptionsAndRejectsNilStream(t *testing.T) {
	t.Parallel()

	raw := json.RawMessage(`{ "vendor_flag": false, "limit": 0 }`)
	var received json.RawMessage
	erased := ai.EraseAPIAdapter(ai.APIAdapterDescriptor[json.RawMessage]{
		API: ai.API("custom-api"),
		Stream: func(_ context.Context, _ ai.Model, _ ai.Context, options json.RawMessage) *ai.AssistantMessageEventStream {
			received = append(json.RawMessage(nil), options...)
			return nil
		},
	})

	stream, err := erased.Stream(context.Background(), ai.Model{}, ai.Context{}, raw)
	if stream != nil || !errors.Is(err, ai.ErrEventStreamInvariant) {
		t.Fatalf("nil typed stream = (%v, %v), want nil and ErrEventStreamInvariant", stream, err)
	}
	if !bytes.Equal(received, raw) {
		t.Fatalf("raw options = %s, want exact bytes %s", received, raw)
	}
}

func TestStubAPIAdapterReturnsNotImplementedWithoutInvokingAnything(t *testing.T) {
	t.Parallel()

	adapter := ai.NewStubAPIAdapter(ai.API("future-api"))
	stream, err := adapter.Stream(context.Background(), ai.Model{}, ai.Context{}, json.RawMessage(`{`))
	if stream != nil || !errors.Is(err, ai.ErrNotImplemented) {
		t.Fatalf("stub Stream() = (%v, %v), want nil and ErrNotImplemented", stream, err)
	}
	var target *ai.NotImplementedError
	if !errors.As(err, &target) || target.Module != "ai" || target.Operation != "APIAdapter.Stream" {
		t.Fatalf("NotImplementedError = %#v", target)
	}
}

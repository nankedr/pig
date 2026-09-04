package ai_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"

	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/telemetry"
)

type recordingTelemetryContext struct {
	starts atomic.Int64
}

func (c *recordingTelemetryContext) StartSpan(context.Context, telemetry.SpanOptions, telemetry.SpanFunc) (any, error) {
	c.starts.Add(1)
	return nil, errors.New("unexpected telemetry span")
}

type openAICompletionsNoOpFixture struct {
	ID         string   `json:"id"`
	CatalogIDs []string `json:"catalog_ids"`
	Cases      []struct {
		ID        string `json:"id"`
		CatalogID string `json:"catalog_id"`
		Input     struct {
			Entrypoint string `json:"entrypoint"`
			Variants   []struct {
				ID     string         `json:"id"`
				Option map[string]any `json:"option"`
			} `json:"variants"`
		} `json:"input"`
	} `json:"cases"`
}

var (
	_ ai.PiMessagesEvent       = ai.PiMessagesStartEvent{}         // upstream: PiMessagesEvent
	_ ai.PiMessagesEvent       = ai.PiMessagesTextStartEvent{}     // upstream: PiMessagesEvent
	_ ai.PiMessagesEvent       = ai.PiMessagesTextDeltaEvent{}     // upstream: PiMessagesEvent
	_ ai.PiMessagesEvent       = ai.PiMessagesTextEndEvent{}       // upstream: PiMessagesEvent
	_ ai.PiMessagesEvent       = ai.PiMessagesThinkingStartEvent{} // upstream: PiMessagesEvent
	_ ai.PiMessagesEvent       = ai.PiMessagesThinkingDeltaEvent{} // upstream: PiMessagesEvent
	_ ai.PiMessagesEvent       = ai.PiMessagesThinkingEndEvent{}   // upstream: PiMessagesEvent
	_ ai.PiMessagesEvent       = ai.PiMessagesToolCallStartEvent{} // upstream: PiMessagesEvent
	_ ai.PiMessagesEvent       = ai.PiMessagesToolCallDeltaEvent{} // upstream: PiMessagesEvent
	_ ai.PiMessagesEvent       = ai.PiMessagesToolCallEndEvent{}   // upstream: PiMessagesEvent
	_ ai.PiMessagesEvent       = ai.PiMessagesDoneEvent{}          // upstream: PiMessagesEvent
	_ ai.PiMessagesEvent       = ai.PiMessagesErrorEvent{}         // upstream: PiMessagesEvent
	_ ai.PiMessagesDoneReason  = ai.PiMessagesDoneReasonStop
	_ ai.PiMessagesErrorReason = ai.PiMessagesErrorReasonError

	_ = ai.PiMessagesRewriteImpact{} // upstream: PiMessagesRewriteImpact

	_ = ai.OpenAICodexWebSocketDebugStats{} // upstream: OpenAICodexWebSocketDebugStats

	_ ai.FauxContentBlock    = ai.TextContent{}                 // upstream: FauxContentBlock
	_                        = ai.FauxModelDefinition{}         // upstream: FauxModelDefinition
	_                        = ai.FauxProviderHandle{}          // upstream: FauxProviderHandle
	_                        = ai.FauxProviderRegistration{}    // upstream: FauxProviderRegistration
	_                        = ai.FauxProviderState{}           // upstream: FauxProviderState
	_ ai.FauxResponseFactory = nil                              // upstream: FauxResponseFactory
	_ ai.FauxResponseStep    = ai.AssistantMessage{}            // upstream: FauxResponseStep
	_ ai.FauxResponseStep    = ai.FauxResponseFactory(nil)      // upstream: FauxResponseStep
	_                        = ai.RegisterFauxProviderOptions{} // upstream: RegisterFauxProviderOptions
	_                        = ai.CreateFauxCore                // upstream: createFauxCore
	_                        = ai.FauxAssistantMessage          // upstream: fauxAssistantMessage
	_                        = ai.NewFauxProvider               // upstream: fauxProvider
	_                        = ai.FauxText                      // upstream: fauxText
	_                        = ai.FauxThinking                  // upstream: fauxThinking
	_                        = ai.FauxToolCall                  // upstream: fauxToolCall

	_                                                                                                                                  = ai.ConvertOpenAICompletionsMessagesOptions{} // upstream: ConvertCompletionsMessagesOptions
	_ func(ai.Model, ai.Context, ai.OpenAICompletionsCompat, ...ai.ConvertOpenAICompletionsMessagesOptions) ([]json.RawMessage, error) = ai.ConvertOpenAICompletionsMessages          // upstream: convertMessages
)

func TestFauxPublicConstructorsSupportThinkingAndDeferred(t *testing.T) {
	t.Parallel()

	if text := ai.FauxText("hello"); text.Type != ai.ContentTypeText || text.Text != "hello" {
		t.Fatalf("FauxText = %#v", text)
	}
	if thinking := ai.FauxThinking("hmm"); thinking.Type != ai.ContentTypeThinking || thinking.Thinking != "hmm" {
		t.Fatalf("FauxThinking = %#v", thinking)
	}
	toolCall, err := ai.FauxToolCall("tool", map[string]any{"value": "ok"})
	if err != nil || toolCall.Type != ai.ContentTypeToolCall || toolCall.ID == "" {
		t.Fatalf("FauxToolCall = (%#v, %v)", toolCall, err)
	}
	emptyID, err := ai.FauxToolCall("tool", nil, ai.FauxToolCallOptions{ID: ai.Some("")})
	if err != nil || emptyID.ID != "" {
		t.Fatalf("FauxToolCall explicit empty id = (%#v, %v)", emptyID, err)
	}
	message, err := ai.FauxAssistantMessage(ai.FauxAssistantText("response"))
	if err != nil || len(message.Content) != 1 || message.StopReason != ai.StopReasonStop {
		t.Fatalf("FauxAssistantMessage = (%#v, %v)", message, err)
	}
	if core, err := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{}); err != nil || core == nil {
		t.Fatalf("CreateFauxCore = (%#v, %v)", core, err)
	}
	if provider, err := ai.NewFauxProvider(); err != nil || provider == nil {
		t.Fatalf("NewFauxProvider = (%#v, %v)", provider, err)
	}
	if _, err := ai.FauxAssistantMessage(ai.FauxAssistantText("deferred"), ai.FauxAssistantMessageOptions{
		StopReason: ai.Some(ai.StopReasonDeferred),
	}); err != nil {
		t.Fatalf("deferred FauxAssistantMessage error = %v", err)
	}
	if core, err := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{Deferred: &ai.FauxDeferredOptions{}}); core == nil || err != nil {
		t.Fatalf("deferred CreateFauxCore = (%#v, %v)", core, err)
	}
}

func TestConvertOpenAICompletionsMessagesHandlesEmptyTextContext(t *testing.T) {
	messages, err := ai.ConvertOpenAICompletionsMessages(
		ai.Model{API: ai.APIOpenAICompletions},
		ai.Context{},
		ai.OpenAICompletionsCompat{},
		ai.ConvertOpenAICompletionsMessagesOptions{GrammarToolInputProperties: map[string]string{"grammar": "input"}},
	)
	if err != nil || len(messages) != 0 {
		t.Fatalf("ConvertOpenAICompletionsMessages = (%#v, %v), want empty live conversion", messages, err)
	}
}

func TestOpenAICompletionsReasoningUsesRuntimeCapability(t *testing.T) {
	reasoning := ai.OpenAIReasoningEffortHigh
	key := "test-key"
	var payload map[string]any
	options := ai.OpenAICompletionsOptions{
		StreamOptions: ai.StreamOptions{
			ProviderRequestOptions: ai.ProviderRequestOptions{
				APIKey: &key,
				Fetch: func(_ context.Context, request ai.FetchRequest) (ai.FetchResponse, error) {
					if err := json.Unmarshal(request.Body, &payload); err != nil {
						t.Fatalf("decode request: %v", err)
					}
					return ai.FetchResponse{Status: 200, Body: []byte(openAITextSSE)}, nil
				},
			},
		},
		ReasoningEffort: &reasoning,
	}
	model := openAITextModel("https://example.test/v1")
	model.Reasoning = true
	result, err := ai.StreamOpenAICompletions(context.Background(), model, ai.Context{}, options).Result(context.Background())
	if err != nil || result.StopReason != ai.StopReasonStop || payload["reasoning_effort"] != "high" {
		t.Fatalf("reasoning runtime = (%#v, %v), payload %#v", result, err, payload)
	}

	streams := ai.OpenAICompletionsAPI()
	if streams.Stream == nil || streams.StreamSimple == nil {
		t.Fatal("OpenAICompletionsAPI returned an incomplete capability bundle")
	}
}

func TestOpenAICompletionsRuntimeAcceptsDocumentedNoOpOptions(t *testing.T) {
	t.Parallel()

	fixture := loadOpenAICompletionsNoOpFixture(t)
	if fixture.ID != "ai/openai-completions/m0-no-op" || len(fixture.Cases) != 6 {
		t.Fatalf("fixture = %#v, want the six M0 no-op field groups", fixture)
	}
	for _, testCase := range fixture.Cases {
		testCase := testCase
		for _, variant := range testCase.Input.Variants {
			variant := variant
			t.Run(testCase.ID+"/"+variant.ID, func(t *testing.T) {
				telemetryRecorder := &recordingTelemetryContext{}
				streamOptions, deferred := noOpOptionsFromFixture(t, variant.Option, telemetryRecorder)
				key := "fixture-key"
				streamOptions.APIKey = &key
				streamOptions.Fetch = func(context.Context, ai.FetchRequest) (ai.FetchResponse, error) {
					return ai.FetchResponse{Status: 200, Body: []byte(openAITextSSE)}, nil
				}
				model := openAITextModel("https://fixture.example.test/v1")
				var stream *ai.AssistantMessageEventStream
				if testCase.Input.Entrypoint == "streamSimple" {
					stream = ai.StreamSimpleOpenAICompletions(context.Background(), model, ai.Context{}, ai.SimpleStreamOptions{StreamOptions: streamOptions, Deferred: deferred})
				} else {
					stream = ai.StreamOpenAICompletions(context.Background(), model, ai.Context{}, ai.OpenAICompletionsOptions{StreamOptions: streamOptions})
				}
				result, err := stream.Result(context.Background())
				if err != nil || result.StopReason != ai.StopReasonStop {
					t.Fatalf("no-op option outcome = (%#v, %v)", result, err)
				}

				reasoning := ai.ThinkingLevelHigh
				if testCase.Input.Entrypoint == "streamSimple" {
					stream = ai.StreamSimpleOpenAICompletions(context.Background(), model, ai.Context{}, ai.SimpleStreamOptions{StreamOptions: streamOptions, Deferred: deferred, Reasoning: &reasoning})
				} else {
					effort := ai.OpenAIReasoningEffortHigh
					stream = ai.StreamOpenAICompletions(context.Background(), model, ai.Context{}, ai.OpenAICompletionsOptions{StreamOptions: streamOptions, ReasoningEffort: &effort})
				}
				if result, err := stream.Result(context.Background()); err != nil || result.StopReason != ai.StopReasonStop {
					t.Fatalf("reasoning option outcome = (%#v, %v)", result, err)
				}
				if got := telemetryRecorder.starts.Load(); got != 0 {
					t.Fatalf("telemetry StartSpan calls = %d, want 0", got)
				}
			})
		}
	}
}

func loadOpenAICompletionsNoOpFixture(t *testing.T) openAICompletionsNoOpFixture {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	path := filepath.Join(filepath.Dir(file), "..", "parity", "oracle", "fixtures", "openai-completions-m0-no-op.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read M0 no-op fixture: %v", err)
	}
	var fixture openAICompletionsNoOpFixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatalf("decode M0 no-op fixture: %v", err)
	}
	return fixture
}

func noOpOptionsFromFixture(t *testing.T, option map[string]any, telemetryContext telemetry.TelemetryContext) (ai.StreamOptions, ai.DeferredOption) {
	t.Helper()
	var stream ai.StreamOptions
	var deferred ai.DeferredOption
	for name, value := range option {
		switch name {
		case "metadata":
			raw, err := json.Marshal(value)
			if err != nil {
				t.Fatalf("decode metadata fixture: %v", err)
			}
			if err := json.Unmarshal(raw, &stream.Metadata); err != nil {
				t.Fatalf("decode metadata fixture: %v", err)
			}
		case "telemetryContext":
			stream.TelemetryContext = telemetryContext
		case "transport":
			transport := ai.Transport(value.(string))
			stream.Transport = &transport
		case "websocketConnectTimeoutMs":
			timeout := int64(value.(float64))
			stream.WebSocketConnectTimeoutMS = &timeout
		case "deferred":
			switch value := value.(type) {
			case bool:
				deferred = ai.DeferredBoolean{Enabled: value}
			case map[string]any:
				windowOptions := ai.DeferredWindowOptions{}
				if rawWindow, ok := value["window"]; ok {
					window := ai.DeferredWindow(rawWindow.(string))
					windowOptions.Window = &window
				}
				deferred = windowOptions
			default:
				t.Fatalf("unsupported deferred fixture value %#v", value)
			}
		default:
			t.Fatalf("unsupported no-op fixture option %q", name)
		}
	}
	return stream, deferred
}

func TestOpenAICompletionsFutureMilestoneOptionsRemainExplicitStubs(t *testing.T) {
	t.Parallel()

	retention := ai.CacheRetentionLong
	assertOpenAICompletionsStubOutcome(t, ai.StreamOpenAICompletions(
		context.Background(), ai.Model{API: ai.APIOpenAICompletions}, ai.Context{}, ai.OpenAICompletionsOptions{
			StreamOptions: ai.StreamOptions{CacheRetention: &retention},
		},
	), "OpenAICompletions.AdvancedOptions")
	model := ai.Model{
		API:    ai.APIOpenAICompletions,
		Compat: ai.Some(json.RawMessage(`{"supportsStore":false}`)),
	}
	assertOpenAICompletionsStubOutcome(t, ai.StreamOpenAICompletions(
		context.Background(), model, ai.Context{}, ai.OpenAICompletionsOptions{},
	), "OpenAICompletions.Compat.supportsStore")
	_, err := ai.ConvertOpenAICompletionsMessages(ai.Model{API: ai.APIOpenAICompletions}, ai.Context{Messages: []ai.Message{
		ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserBlocks(ai.ImageContent{Type: ai.ContentTypeImage, Data: "aGk=", MIMEType: "image/png"})},
	}}, ai.OpenAICompletionsCompat{})
	assertOpenAICompletionsStubError(t, err, "OpenAICompletions.ConvertMessages.Image")
}

func assertOpenAICompletionsStubOutcome(t *testing.T, stream *ai.AssistantMessageEventStream, wantOperation string) {
	t.Helper()
	if stream == nil {
		t.Fatal("OpenAI Completions Capability Stub returned a nil stream")
	}
	event, ok, err := stream.Next(context.Background())
	if event != nil || ok || err != nil {
		t.Fatalf("Next() = (%#v, %t, %v), want an event-free closed stream", event, ok, err)
	}
	_, err = stream.Result(context.Background())
	assertOpenAICompletionsStubError(t, err, wantOperation)
}

func assertOpenAICompletionsStubError(t *testing.T, err error, wantOperation string) {
	t.Helper()
	if !errors.Is(err, ai.ErrNotImplemented) {
		t.Fatalf("stream error = %v, want ErrNotImplemented", err)
	}
	var notImplemented *ai.NotImplementedError
	if !errors.As(err, &notImplemented) {
		t.Fatalf("stream error = %T, want *ai.NotImplementedError", err)
	}
	if notImplemented.Module != "ai" || notImplemented.Operation != wantOperation {
		t.Fatalf("NotImplementedError = %#v, want ai.%s", notImplemented, wantOperation)
	}
}

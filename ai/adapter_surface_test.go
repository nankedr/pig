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

func TestFauxPureValuesAndRuntimeStubsStaySeparated(t *testing.T) {
	t.Parallel()

	if text := ai.FauxText("hello"); text.Type != ai.ContentTypeText || text.Text != "hello" {
		t.Fatalf("FauxText = %#v", text)
	}
	if thinking := ai.FauxThinking("hmm"); thinking.Type != ai.ContentTypeThinking || thinking.Thinking != "hmm" {
		t.Fatalf("FauxThinking = %#v", thinking)
	}
	if _, err := ai.FauxToolCall("tool", nil); !errors.Is(err, ai.ErrNotImplemented) {
		t.Fatalf("FauxToolCall error = %v", err)
	}
	if _, err := ai.FauxAssistantMessage(ai.FauxAssistantText("response")); !errors.Is(err, ai.ErrNotImplemented) {
		t.Fatalf("FauxAssistantMessage error = %v", err)
	}
	if core, err := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{}); core != nil || !errors.Is(err, ai.ErrNotImplemented) {
		t.Fatalf("CreateFauxCore = (%#v, %v)", core, err)
	}
	if provider, err := ai.NewFauxProvider(); provider != nil || !errors.Is(err, ai.ErrNotImplemented) {
		t.Fatalf("NewFauxProvider = (%#v, %v)", provider, err)
	}
}

func TestConvertOpenAICompletionsMessagesIsAnExplicitCapabilityStub(t *testing.T) {
	messages, err := ai.ConvertOpenAICompletionsMessages(
		ai.Model{API: ai.APIOpenAICompletions},
		ai.Context{},
		ai.OpenAICompletionsCompat{},
		ai.ConvertOpenAICompletionsMessagesOptions{GrammarToolInputProperties: map[string]string{"grammar": "input"}},
	)
	if messages != nil || !errors.Is(err, ai.ErrNotImplemented) {
		t.Fatalf("ConvertOpenAICompletionsMessages = (%#v, %v), want nil and ErrNotImplemented", messages, err)
	}
}

func TestOpenAICompletionsEntrypointsRemainExplicitCapabilityStubs(t *testing.T) {
	options := ai.OpenAICompletionsOptions{
		StreamOptions: ai.StreamOptions{
			ProviderRequestOptions: ai.ProviderRequestOptions{
				OnPayload: func(context.Context, ai.JSONValue, ai.Model) (ai.PayloadHookResult, error) {
					t.Fatal("stub must not invoke request hooks")
					return ai.PayloadHookResult{}, nil
				},
			},
		},
	}
	stream := ai.StreamOpenAICompletions(context.Background(), ai.Model{API: ai.APIOpenAICompletions}, ai.Context{}, options)
	if stream == nil {
		t.Fatal("StreamOpenAICompletions returned nil stream")
	}
	if _, err := stream.Result(context.Background()); !errors.Is(err, ai.ErrNotImplemented) {
		t.Fatalf("StreamOpenAICompletions.Result() error = %v, want ErrNotImplemented", err)
	}

	streams := ai.OpenAICompletionsAPI()
	if streams.Stream == nil || streams.StreamSimple == nil {
		t.Fatal("OpenAICompletionsAPI returned an incomplete capability bundle")
	}
	if _, err := streams.Stream(context.Background(), ai.Model{API: ai.APIOpenAICompletions}, ai.Context{}, ai.StreamOptions{}).Result(context.Background()); !errors.Is(err, ai.ErrNotImplemented) {
		t.Fatalf("OpenAICompletionsAPI().Stream().Result() error = %v, want ErrNotImplemented", err)
	}
	temperature := 0.5
	assertOpenAICompletionsStubOutcome(t, streams.Stream(
		context.Background(), ai.Model{API: ai.APIOpenAICompletions}, ai.Context{}, ai.StreamOptions{Temperature: &temperature},
	), "OpenAICompletions.Stream.Options")
}

func TestOpenAICompletionsStubAcceptsDocumentedNoOpOptions(t *testing.T) {
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
				var stream *ai.AssistantMessageEventStream
				wantOperation := "OpenAICompletions.Stream"
				if testCase.Input.Entrypoint == "streamSimple" {
					wantOperation = "OpenAICompletions.StreamSimple"
					stream = ai.StreamSimpleOpenAICompletions(context.Background(), ai.Model{API: ai.APIOpenAICompletions}, ai.Context{}, ai.SimpleStreamOptions{StreamOptions: streamOptions, Deferred: deferred})
				} else {
					stream = ai.StreamOpenAICompletions(context.Background(), ai.Model{API: ai.APIOpenAICompletions}, ai.Context{}, ai.OpenAICompletionsOptions{StreamOptions: streamOptions})
				}
				assertOpenAICompletionsStubOutcome(t, stream, wantOperation)

				// A documented no-op must not erase an unrelated unsupported
				// option while the adapter remains a Capability Stub.
				if testCase.Input.Entrypoint == "streamSimple" {
					reasoning := ai.ThinkingLevelHigh
					stream = ai.StreamSimpleOpenAICompletions(context.Background(), ai.Model{API: ai.APIOpenAICompletions}, ai.Context{}, ai.SimpleStreamOptions{StreamOptions: streamOptions, Deferred: deferred, Reasoning: &reasoning})
					assertOpenAICompletionsStubOutcome(t, stream, "OpenAICompletions.StreamSimple.Options")
				} else {
					temperature := 0.5
					streamOptions.Temperature = &temperature
					stream = ai.StreamOpenAICompletions(context.Background(), ai.Model{API: ai.APIOpenAICompletions}, ai.Context{}, ai.OpenAICompletionsOptions{StreamOptions: streamOptions})
					assertOpenAICompletionsStubOutcome(t, stream, "OpenAICompletions.Stream.Options")
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

func TestOpenAICompletionsStubRejectsOtherNonDefaultOptionsExplicitly(t *testing.T) {
	t.Parallel()

	temperature := 0.5
	reasoning := ai.ThinkingLevelHigh
	assertOpenAICompletionsStubOutcome(t, ai.StreamOpenAICompletions(
		context.Background(), ai.Model{API: ai.APIOpenAICompletions}, ai.Context{}, ai.OpenAICompletionsOptions{
			StreamOptions: ai.StreamOptions{Temperature: &temperature},
		},
	), "OpenAICompletions.Stream.Options")
	assertOpenAICompletionsStubOutcome(t, ai.StreamSimpleOpenAICompletions(
		context.Background(), ai.Model{API: ai.APIOpenAICompletions}, ai.Context{}, ai.SimpleStreamOptions{Reasoning: &reasoning},
	), "OpenAICompletions.StreamSimple.Options")
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

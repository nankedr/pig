package agent_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
)

func TestRunAgentLoopCompletesToolFreeTextTurnThroughContextBoundary(t *testing.T) {
	custom, err := agent.NewRawAgentMessage(json.RawMessage(`{"role":"notice","text":"private"}`))
	if err != nil {
		t.Fatal(err)
	}
	response, err := ai.FauxAssistantMessage(ai.FauxAssistantText("hello"), ai.FauxAssistantMessageOptions{
		Timestamp: ai.Some(int64(2)),
	})
	if err != nil {
		t.Fatal(err)
	}
	minTokenSize := 100
	core, err := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{TokenSize: &ai.FauxTokenSize{Min: &minTokenSize}})
	if err != nil {
		t.Fatal(err)
	}
	var calls []string
	core.SetResponses([]ai.FauxResponseStep{ai.FauxResponseFactory(func(input ai.Context, _ *ai.SimpleStreamOptions, _ *ai.FauxProviderState, _ ai.Model) (ai.AssistantMessage, error) {
		calls = append(calls, "stream")
		systemPrompt, _ := input.SystemPrompt.Value()
		if systemPrompt != "system" || len(input.Messages) != 1 || input.Messages[0].MessageRole() != ai.MessageRoleUser {
			t.Fatalf("model context = %#v, want system prompt and one user message", input)
		}
		return response, nil
	})})
	model, _ := core.GetModel()
	prompt := ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText("prompt"), Timestamp: 1}
	config := agent.AgentLoopConfig{
		Model: model,
		TransformContext: func(_ context.Context, messages []agent.AgentMessage) ([]agent.AgentMessage, error) {
			calls = append(calls, "transform")
			return messages, nil
		},
		ConvertToLLM: func(ctx context.Context, messages []agent.AgentMessage) ([]ai.Message, error) {
			calls = append(calls, "convert")
			return agent.DefaultConvertToLLM(ctx, messages)
		},
	}
	var eventTypes []agent.AgentEventType
	messages, err := agent.RunAgentLoop(
		context.Background(),
		[]agent.AgentMessage{prompt},
		agent.AgentContext{SystemPrompt: "system", Messages: []agent.AgentMessage{custom}},
		config,
		func(_ context.Context, event agent.AgentEvent) error {
			eventTypes = append(eventTypes, event.AgentEventType())
			return nil
		},
		agent.StreamFunction(core.StreamSimple),
	)
	if err != nil {
		t.Fatalf("RunAgentLoop() error = %v", err)
	}
	if !slices.Equal(calls, []string{"transform", "convert", "stream"}) {
		t.Fatalf("context boundary calls = %v", calls)
	}
	wantEvents := []agent.AgentEventType{
		agent.AgentEventTypeAgentStart,
		agent.AgentEventTypeTurnStart,
		agent.AgentEventTypeMessageStart,
		agent.AgentEventTypeMessageEnd,
		agent.AgentEventTypeMessageStart,
		agent.AgentEventTypeMessageUpdate,
		agent.AgentEventTypeMessageUpdate,
		agent.AgentEventTypeMessageUpdate,
		agent.AgentEventTypeMessageEnd,
		agent.AgentEventTypeTurnEnd,
		agent.AgentEventTypeAgentEnd,
	}
	if !slices.Equal(eventTypes, wantEvents) {
		t.Fatalf("event types = %v, want %v", eventTypes, wantEvents)
	}
	if len(messages) != 2 || messages[0].MessageRole() != ai.MessageRoleUser || messages[1].MessageRole() != ai.MessageRoleAssistant {
		t.Fatalf("new messages = %#v, want user and assistant", messages)
	}
	assistant := messages[1].(ai.AssistantMessage)
	if len(assistant.Content) != 1 || assistant.Content[0].(ai.TextContent).Text != "hello" {
		t.Fatalf("assistant = %#v", assistant)
	}
}

func TestRunAgentLoopCallsShouldStopAfterSuccessfulTurn(t *testing.T) {
	response, err := ai.FauxAssistantMessage(ai.FauxAssistantText("done"))
	if err != nil {
		t.Fatal(err)
	}
	core, err := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	core.SetResponses([]ai.FauxResponseStep{response})
	model, _ := core.GetModel()
	var timeline []string
	config := agent.AgentLoopConfig{
		Model: model,
		ShouldStopAfterTurn: func(_ context.Context, turn agent.ShouldStopAfterTurnContext) (bool, error) {
			timeline = append(timeline, "should_stop")
			if len(turn.Context.Messages) != 2 || len(turn.NewMessages) != 2 || turn.Message.StopReason != ai.StopReasonStop {
				t.Fatalf("turn context = %#v", turn)
			}
			return false, nil
		},
	}
	_, err = agent.RunAgentLoop(
		context.Background(),
		[]agent.AgentMessage{userMessage("prompt")},
		agent.AgentContext{},
		config,
		func(_ context.Context, event agent.AgentEvent) error {
			timeline = append(timeline, string(event.AgentEventType()))
			return nil
		},
		agent.StreamFunction(core.StreamSimple),
	)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"turn_end", "should_stop", "agent_end"}
	if !slices.Equal(timeline[len(timeline)-len(want):], want) {
		t.Fatalf("timeline tail = %v, want %v", timeline, want)
	}
}

func TestRunAgentLoopKeepsToolExecutionAsExplicitCapabilityStub(t *testing.T) {
	toolCall, err := ai.FauxToolCall("echo", map[string]any{"text": "hello"})
	if err != nil {
		t.Fatal(err)
	}
	response, err := ai.FauxAssistantMessage(ai.FauxAssistantBlocks(toolCall), ai.FauxAssistantMessageOptions{
		StopReason: ai.Some(ai.StopReasonToolUse),
	})
	if err != nil {
		t.Fatal(err)
	}
	core, err := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	core.SetResponses([]ai.FauxResponseStep{response})
	model, _ := core.GetModel()
	var events []agent.AgentEventType
	_, err = agent.RunAgentLoop(
		context.Background(),
		[]agent.AgentMessage{userMessage("prompt")},
		agent.AgentContext{},
		agent.AgentLoopConfig{Model: model},
		func(_ context.Context, event agent.AgentEvent) error {
			events = append(events, event.AgentEventType())
			return nil
		},
		agent.StreamFunction(core.StreamSimple),
	)
	if !errors.Is(err, agent.ErrNotImplemented) {
		t.Fatalf("RunAgentLoop() error = %v, want ErrNotImplemented", err)
	}
	if slices.Contains(events, agent.AgentEventTypeAgentEnd) {
		t.Fatalf("Tool capability stub published agent_end: %v", events)
	}
}

func TestProxyAssistantMessageEventCodecRoundTripsClosedVariants(t *testing.T) {
	t.Parallel()
	signature := "sig"
	errorMessage := "failed"
	usage := ai.Usage{Input: 1, Output: 2, TotalTokens: 3}
	tests := []agent.ProxyAssistantMessageEvent{
		agent.ProxyStartEvent{Type: ai.AssistantMessageEventTypeStart},
		agent.ProxyTextStartEvent{Type: ai.AssistantMessageEventTypeTextStart, ContentIndex: 1},
		agent.ProxyTextDeltaEvent{Type: ai.AssistantMessageEventTypeTextDelta, ContentIndex: 1, Delta: "x"},
		agent.ProxyTextEndEvent{Type: ai.AssistantMessageEventTypeTextEnd, ContentIndex: 1, ContentSignature: &signature},
		agent.ProxyThinkingStartEvent{Type: ai.AssistantMessageEventTypeThinkingStart, ContentIndex: 2},
		agent.ProxyThinkingDeltaEvent{Type: ai.AssistantMessageEventTypeThinkingDelta, ContentIndex: 2, Delta: "y"},
		agent.ProxyThinkingEndEvent{Type: ai.AssistantMessageEventTypeThinkingEnd, ContentIndex: 2, ContentSignature: &signature},
		agent.ProxyToolCallStartEvent{Type: ai.AssistantMessageEventTypeToolCallStart, ContentIndex: 3, ID: "call-1", ToolName: "read"},
		agent.ProxyToolCallDeltaEvent{Type: ai.AssistantMessageEventTypeToolCallDelta, ContentIndex: 3, Delta: "{}"},
		agent.ProxyToolCallEndEvent{Type: ai.AssistantMessageEventTypeToolCallEnd, ContentIndex: 3, ToolCall: ai.ToolCall{Type: ai.ContentTypeToolCall, ID: "call-1", Name: "read", Arguments: map[string]any{}}},
		agent.ProxyDoneEvent{Type: ai.AssistantMessageEventTypeDone, Reason: ai.StopReasonStop, Usage: usage},
		agent.ProxyErrorEvent{Type: ai.AssistantMessageEventTypeError, Reason: ai.StopReasonError, ErrorMessage: &errorMessage, Usage: usage},
	}
	for _, event := range tests {
		encoded, err := agent.MarshalProxyAssistantMessageEvent(event)
		if err != nil {
			t.Fatalf("MarshalProxyAssistantMessageEvent(%T): %v", event, err)
		}
		decoded, err := agent.UnmarshalProxyAssistantMessageEvent(encoded)
		if err != nil {
			t.Fatalf("UnmarshalProxyAssistantMessageEvent(%s): %v", encoded, err)
		}
		if decoded.ProxyAssistantMessageEventType() != event.ProxyAssistantMessageEventType() {
			t.Fatalf("decoded type = %q, want %q", decoded.ProxyAssistantMessageEventType(), event.ProxyAssistantMessageEventType())
		}
	}
}

func TestProxyAssistantMessageEventCodecRejectsUnknownMismatchedAndExtraFields(t *testing.T) {
	t.Parallel()
	for _, input := range [][]byte{
		[]byte(`{"type":"future"}`),
		[]byte(`{"type":"text_delta","contentIndex":0}`),
		[]byte(`{"type":"text_delta","contentIndex":null,"delta":"x"}`),
		[]byte(`{"type":"text_delta","contentIndex":0,"delta":null}`),
		[]byte(`{"type":"text_end","contentIndex":0,"contentSignature":null}`),
		[]byte(`{"type":"toolcall_start","contentIndex":0,"id":null,"toolName":"read"}`),
		[]byte(`{"type":"toolcall_end","contentIndex":0,"toolCall":null}`),
		[]byte(`{"type":"toolcall_end","contentIndex":0,"toolCall":{"type":"text","id":"call-1","name":"read","arguments":{}}}`),
		[]byte(`{"type":"done","reason":"error","usage":{}}`),
		[]byte(`{"type":"done","reason":"stop","usage":null}`),
		[]byte(`{"type":"error","reason":"error","errorMessage":null,"usage":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0,"totalTokens":0,"cost":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0,"total":0}}`),
	} {
		if _, err := agent.UnmarshalProxyAssistantMessageEvent(input); err == nil {
			t.Fatalf("accepted invalid event %s", input)
		}
	}
	if _, err := agent.MarshalProxyAssistantMessageEvent(agent.ProxyStartEvent{Type: ai.AssistantMessageEventTypeDone}); err == nil {
		t.Fatal("accepted mismatched concrete type and discriminator")
	}
	encoded, err := agent.MarshalProxyAssistantMessageEvent(agent.ProxyStartEvent{Type: ai.AssistantMessageEventTypeStart})
	if err != nil || !bytes.Equal(encoded, json.RawMessage(`{"type":"start"}`)) {
		t.Fatalf("start encoding = %s, %v", encoded, err)
	}
}

func TestProxyAssistantMessageEventCodecAcceptsUnknownEnvelopeAndNestedFields(t *testing.T) {
	t.Parallel()

	for _, input := range [][]byte{
		[]byte(`{"type":"start","extra":true}`),
		[]byte(`{"type":"done","reason":"stop","usage":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0,"totalTokens":0,"extra":true,"cost":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0,"total":0,"extra":true}}}`),
		[]byte(`{"type":"toolcall_end","contentIndex":0,"toolCall":{"type":"toolCall","id":"call-1","name":"read","arguments":{},"extra":true}}`),
	} {
		event, err := agent.UnmarshalProxyAssistantMessageEvent(input)
		if err != nil {
			t.Fatalf("accepted unknown fields in %s: %v", input, err)
		}
		if event == nil {
			t.Fatalf("decoded nil event from %s", input)
		}
	}
}

func TestProxyStreamOptionsExposePinnedFields(t *testing.T) {
	t.Parallel()

	typ := reflect.TypeOf(agent.ProxyStreamOptions{})
	got := make(map[string]reflect.StructField, typ.NumField())
	for i := range typ.NumField() {
		field := typ.Field(i)
		got[field.Name] = field
	}

	want := []string{
		"AuthToken",
		"CacheRetention",
		"Headers",
		"MaxRetryDelayMS",
		"MaxTokens",
		"Metadata",
		"ProxyURL",
		"Reasoning",
		"SamplingParams",
		"SessionID",
		"Signal",
		"Temperature",
		"ThinkingBudgets",
		"Transport",
	}
	if len(got) != len(want) {
		t.Fatalf("ProxyStreamOptions field count = %d, want %d (%v)", len(got), len(want), want)
	}
	for _, name := range want {
		if _, ok := got[name]; !ok {
			t.Fatalf("ProxyStreamOptions missing field %q; got %v", name, reflect.VisibleFields(typ))
		}
	}
	for _, forbidden := range []string{
		"SimpleStreamOptions",
		"Deferred",
		"APIKey",
		"Env",
		"Fetch",
		"OnPayload",
		"OnResponse",
		"TimeoutMS",
		"MaxRetries",
		"TelemetryContext",
		"WebSocketConnectTimeoutMS",
	} {
		if _, ok := got[forbidden]; ok {
			t.Fatalf("ProxyStreamOptions unexpectedly exposes %q", forbidden)
		}
	}
}

func TestAgentLoopPublishesEventsAndRepeatableResult(t *testing.T) {
	response, err := ai.FauxAssistantMessage(ai.FauxAssistantText("done"))
	if err != nil {
		t.Fatal(err)
	}
	core, err := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	core.SetResponses([]ai.FauxResponseStep{response})
	model, _ := core.GetModel()
	stream := agent.AgentLoop(
		context.Background(),
		[]agent.AgentMessage{userMessage("prompt")},
		agent.AgentContext{},
		agent.AgentLoopConfig{Model: model},
		agent.StreamFunction(core.StreamSimple),
	)
	if stream == nil {
		t.Fatal("AgentLoop returned nil stream")
	}
	var events []agent.AgentEventType
	for {
		event, ok, err := stream.Next(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			break
		}
		events = append(events, event.AgentEventType())
	}
	first, err := stream.Result(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	second, err := stream.Result(context.Background())
	if err != nil || !reflect.DeepEqual(first, second) {
		t.Fatalf("repeat Result() = (%#v, %v), want %#v", second, err, first)
	}
	if len(first) != 2 || events[0] != agent.AgentEventTypeAgentStart || events[len(events)-1] != agent.AgentEventTypeAgentEnd {
		t.Fatalf("events = %v, result = %#v", events, first)
	}
}

func TestRunAgentLoopRejectsNilEventSinkBeforeStreaming(t *testing.T) {
	t.Parallel()

	called := false
	streamFn := func(context.Context, ai.Model, ai.Context, ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
		called = true
		return nil
	}
	result, err := agent.RunAgentLoop(context.Background(), nil, agent.AgentContext{}, agent.AgentLoopConfig{}, nil, streamFn)
	if result != nil || err == nil {
		t.Fatalf("RunAgentLoop() = (%v, %v), want nil and error", result, err)
	}
	if called {
		t.Fatal("RunAgentLoop invoked stream function after rejecting nil sink")
	}
}

func TestSetDefaultStreamFunctionIsSideEffectFreeStub(t *testing.T) {
	t.Parallel()
	called := false
	err := agent.SetDefaultStreamFunction(func(context.Context, ai.Model, ai.Context, ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
		called = true
		return nil
	})
	if !errors.Is(err, agent.ErrNotImplemented) {
		t.Fatalf("SetDefaultStreamFunction() error = %v, want ErrNotImplemented", err)
	}
	if called {
		t.Fatal("SetDefaultStreamFunction invoked the supplied function")
	}
}

func TestStreamProxyStubIsImmediateEventFreeAndDoesNotInvokeFetch(t *testing.T) {
	t.Parallel()
	called := false
	stream := agent.StreamProxy(context.Background(), ai.Model{}, ai.Context{}, agent.ProxyStreamOptions{
		AuthToken: "token",
	})
	if stream == nil {
		t.Fatal("StreamProxy returned nil stream")
	}
	if event, ok, err := stream.Next(context.Background()); event != nil || ok || err != nil {
		t.Fatalf("Next() = (%T, %t, %v), want (nil, false, nil)", event, ok, err)
	}
	if _, err := stream.Result(context.Background()); !errors.Is(err, agent.ErrNotImplemented) {
		t.Fatalf("Result() error = %v, want ErrNotImplemented", err)
	}
	if called {
		t.Fatal("StreamProxy stub invoked Fetch")
	}
}

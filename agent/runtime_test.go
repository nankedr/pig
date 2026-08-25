package agent_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
)

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

func TestAgentLoopStubIsImmediateEventFreeAndDoesNotInvokeCallbacks(t *testing.T) {
	t.Parallel()

	called := false
	streamFn := func(context.Context, ai.Model, ai.Context, ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
		called = true
		return ai.NewAssistantMessageEventStream()
	}
	config := agent.AgentLoopConfig{
		ConvertToLLM: func(context.Context, []agent.AgentMessage) ([]ai.Message, error) {
			called = true
			return nil, nil
		},
		GetSteeringMessages: func(context.Context) ([]agent.AgentMessage, error) {
			called = true
			return nil, nil
		},
	}

	stream := agent.AgentLoop(context.Background(), nil, agent.AgentContext{}, config, streamFn)
	if stream == nil {
		t.Fatal("AgentLoop returned nil stream")
	}
	if event, ok, err := stream.Next(context.Background()); event != nil || ok || err != nil {
		t.Fatalf("Next() = (%T, %t, %v), want (nil, false, nil)", event, ok, err)
	}
	if result, err := stream.Result(context.Background()); result != nil || !errors.Is(err, agent.ErrNotImplemented) {
		t.Fatalf("Result() = (%v, %v), want nil and ErrNotImplemented", result, err)
	}
	if called {
		t.Fatal("AgentLoop stub invoked a callback")
	}
}

func TestRunAgentLoopStubDoesNotInvokeSinkOrStreamFunction(t *testing.T) {
	t.Parallel()

	called := false
	sink := func(context.Context, agent.AgentEvent) error { called = true; return nil }
	streamFn := func(context.Context, ai.Model, ai.Context, ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
		called = true
		return nil
	}
	result, err := agent.RunAgentLoop(context.Background(), nil, agent.AgentContext{}, agent.AgentLoopConfig{}, sink, streamFn)
	if result != nil || !errors.Is(err, agent.ErrNotImplemented) {
		t.Fatalf("RunAgentLoop() = (%v, %v), want nil and ErrNotImplemented", result, err)
	}
	if called {
		t.Fatal("RunAgentLoop stub invoked a callback")
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

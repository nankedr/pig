package agent_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
)

const proxyUsage = `{"input":3,"output":2,"cacheRead":1,"cacheWrite":0,"totalTokens":6,"cost":{"input":0.1,"output":0.2,"cacheRead":0.01,"cacheWrite":0,"total":0.31}}`
const proxyText = "data: {\"type\":\"start\"}\n\ndata: {\"type\":\"text_start\",\"contentIndex\":0}\n\ndata: {\"type\":\"text_delta\",\"contentIndex\":0,\"delta\":\"hello\"}\n\ndata: {\"type\":\"text_end\",\"contentIndex\":0,\"contentSignature\":\"signed\"}\n\n"

func TestProxyStreamsTextAndForwardsOptions(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	request := make(chan map[string]json.RawMessage, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/api/stream" || r.Header.Get("Authorization") != "Bearer proxy-secret" || r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("unexpected request: %s %s %v", r.Method, r.URL, r.Header)
		}
		var body map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		request <- body
		fmt.Fprint(w, proxyText+`data: {"type":"done","reason":"stop","usage":`+proxyUsage+"}\n\n")
	}))
	defer server.Close()
	var options agent.ProxyStreamOptions
	const wireOptions = `{"temperature":0,"samplingParams":{"top_p":0.8},"maxTokens":128,"reasoning":"high","cacheRetention":"long","sessionId":"session","headers":{"X-Provider":"key","Remove":null},"metadata":{"trace":[1,true]},"transport":"sse","thinkingBudgets":{"high":100},"maxRetryDelayMs":0}`
	if err := json.Unmarshal([]byte(wireOptions), &options); err != nil {
		t.Fatal(err)
	}
	options.ProxyURL, options.AuthToken = server.URL, "proxy-secret"
	model := ai.Model{ID: "proxy-model", API: ai.APIOpenAICompletions, Provider: "controlled"}
	input := ai.Context{Messages: []ai.Message{ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText("hi"), Timestamp: 1}}}
	stream := agent.StreamProxy(ctx, model, input, options)
	result, err := stream.Result(ctx)
	if err != nil || result.StopReason != ai.StopReasonStop || len(result.Content) != 1 {
		t.Fatalf("result = %+v, %v", result, err)
	}
	block := result.Content[0].(ai.TextContent)
	if block.Text != "hello" || block.TextSignature != ai.Some("signed") || result.Usage.TotalTokens != 6 || result.Usage.Cost.Total != 0.31 {
		t.Fatalf("result = %+v", result)
	}
	got := <-request
	for name, want := range map[string]any{"model": model, "context": input, "options": json.RawMessage(wireOptions)} {
		expected, _ := json.Marshal(want)
		var a, b any
		json.Unmarshal(got[name], &a)
		json.Unmarshal(expected, &b)
		if !reflect.DeepEqual(a, b) {
			t.Errorf("%s = %s, want %s", name, got[name], expected)
		}
	}
	var types []ai.AssistantMessageEventType
	for {
		event, ok, err := stream.Next(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			break
		}
		types = append(types, event.AssistantMessageEventType())
		if event, ok := event.(ai.AssistantMessageTextStartEvent); ok && event.Partial.Content[0].(ai.TextContent).Text != "" {
			t.Fatalf("start snapshot = %+v", event)
		}
	}
	if !reflect.DeepEqual(types, []ai.AssistantMessageEventType{"start", "text_start", "text_delta", "text_end", "done"}) {
		t.Fatal(types)
	}
}

func proxyBuffered(events string) agent.ProxyStreamOptions {
	return agent.ProxyStreamOptions{ProxyURL: "https://proxy.invalid", Fetch: func(context.Context, ai.FetchRequest) (ai.FetchResponse, error) {
		return ai.FetchResponse{Status: 200, Body: []byte(events)}, nil
	}}
}

func TestProxyReconstructsThinkingAndPartialToolArguments(t *testing.T) {
	const events = `data: {"type":"start"}

data: {"type":"thinking_start","contentIndex":0}

data: {"type":"thinking_delta","contentIndex":0,"delta":"plan"}

data: {"type":"thinking_end","contentIndex":0,"contentSignature":"thought-signature"}

data: {"type":"toolcall_start","contentIndex":1,"id":"call-1","toolName":"read"}

data: {"type":"toolcall_delta","contentIndex":1,"delta":"{\"path\":\"hel"}

data: {"type":"toolcall_delta","contentIndex":1,"delta":"lo\",\"nested\":{\"list\":[1,true]}}"}

data: {"type":"toolcall_end","contentIndex":1,"toolCall":{"type":"toolCall","id":"call-1","name":"read","arguments":{"path":"hello","nested":{"list":[1,true]}},"thoughtSignature":"tool-signature"}}

`
	stream := agent.StreamProxy(context.Background(), ai.Model{}, ai.Context{}, proxyBuffered(events+`data: {"type":"done","reason":"toolUse","usage":`+proxyUsage+"}\n\n"))
	result, err := stream.Result(context.Background())
	if err != nil || result.StopReason != ai.StopReasonToolUse || len(result.Content) != 2 {
		t.Fatalf("result = %+v %v", result, err)
	}
	if result.Content[0].(ai.ThinkingContent).ThinkingSignature != ai.Some("thought-signature") || result.Content[1].(ai.ToolCall).ThoughtSignature != ai.Some("tool-signature") {
		t.Fatal(result)
	}
	var deltas []map[string]any
	for {
		event, ok, err := stream.Next(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			break
		}
		if delta, ok := event.(ai.AssistantMessageToolCallDeltaEvent); ok {
			deltas = append(deltas, delta.Partial.Content[1].(ai.ToolCall).Arguments)
		}
	}
	if len(deltas) != 2 || deltas[0]["path"] != "hel" || deltas[1]["path"] != "hello" {
		t.Fatal(deltas)
	}
	deltas[1]["path"] = "mutated"
	again, _ := stream.Result(context.Background())
	if again.Content[1].(ai.ToolCall).Arguments["path"] != "hello" {
		t.Fatal("published partial aliases outcome")
	}
	result.Content[1].(ai.ToolCall).Arguments["path"] = "mutated"
	again, _ = stream.Result(context.Background())
	if again.Content[1].(ai.ToolCall).Arguments["path"] != "hello" {
		t.Fatal("Result aliases previous Result")
	}
}

func TestProxyProtocolFailuresRetainReceivedContent(t *testing.T) {
	for _, invalid := range []string{
		`{"type":"future"}`,
		`{"type":"text_delta","contentIndex":-1,"delta":"x"}`,
		`{"type":"text_start","contentIndex":999999999}`,
		`{"type":"text_delta","contentIndex":0.5,"delta":"x"}`,
		`{"type":"text_delta","contentIndex":0,"delta":null}`,
		`{"type":"thinking_delta","contentIndex":0,"delta":"x"}`,
		`{"type":"text_end","contentIndex":0,"contentSignature":null}`,
		`{"type":"toolcall_end","contentIndex":0,"toolCall":{"type":"toolCall","id":"c","name":"read","arguments":null}}`,
		`{"type":"done","reason":"stop","usage":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0,"totalTokens":0,"cost":null}}`,
		`{"type":"done","reason":"stop","usage":` + strings.Replace(proxyUsage, `"input":3`, `"input":null`, 1) + `}`,
		`{`,
		"",
	} {
		t.Run(invalid, func(t *testing.T) {
			events := strings.Replace(proxyText, `data: {"type":"text_end","contentIndex":0,"contentSignature":"signed"}`+"\n\n", "", 1)
			if invalid != "" {
				events += "data: " + invalid + "\n\n"
			}
			stream := agent.StreamProxy(context.Background(), ai.Model{}, ai.Context{}, proxyBuffered(events))
			result, err := stream.Result(context.Background())
			message, _ := result.ErrorMessage.Value()
			if err != nil || result.StopReason != ai.StopReasonError || !strings.Contains(message, "Proxy protocol error") || len(result.Content) != 1 || result.Content[0].(ai.TextContent).Text != "hello" {
				t.Fatalf("result = %+v %v", result, err)
			}
			terminal := 0
			for {
				event, ok, err := stream.Next(context.Background())
				if err != nil {
					t.Fatal(err)
				}
				if !ok {
					break
				}
				if event.AssistantMessageEventType() == ai.AssistantMessageEventTypeError {
					terminal++
				}
			}
			if terminal != 1 {
				t.Fatalf("error terminals=%d", terminal)
			}
		})
	}
}

func TestProxyHTTPAndRemoteErrorsAreFailureOutcomes(t *testing.T) {
	for _, reason := range []string{"error", "aborted"} {
		stream := agent.StreamProxy(context.Background(), ai.Model{}, ai.Context{}, proxyBuffered(proxyText+`data: {"type":"error","reason":"`+reason+`","errorMessage":"remote failure","usage":`+proxyUsage+"}\n\n"))
		result, err := stream.Result(context.Background())
		if err != nil || string(result.StopReason) != reason || result.Usage.TotalTokens != 6 || result.ErrorMessage != ai.Some("remote failure") || len(result.Content) != 1 {
			t.Fatalf("remote outcome=%+v %v", result, err)
		}
	}
	for _, status := range []int{401, 403, 429, 500} {
		options := agent.ProxyStreamOptions{Fetch: func(context.Context, ai.FetchRequest) (ai.FetchResponse, error) {
			return ai.FetchResponse{Status: status, Body: []byte(`{"error":"denied"}`)}, nil
		}}
		result, err := agent.StreamProxy(context.Background(), ai.Model{}, ai.Context{}, options).Result(context.Background())
		if err != nil || result.StopReason != ai.StopReasonError || result.ErrorMessage != ai.Some("Proxy error: denied") {
			t.Fatalf("HTTP outcome=%+v %v", result, err)
		}
	}
	options := agent.ProxyStreamOptions{Fetch: func(context.Context, ai.FetchRequest) (ai.FetchResponse, error) {
		return ai.FetchResponse{}, errors.New("transport failed")
	}}
	result, err := agent.StreamProxy(context.Background(), ai.Model{}, ai.Context{}, options).Result(context.Background())
	if err != nil || result.StopReason != ai.StopReasonError || result.ErrorMessage != ai.Some("transport failed") {
		t.Fatalf("transport outcome=%+v %v", result, err)
	}
}

func TestProxyCancellationClosesTransportAndRetainsPartial(t *testing.T) {
	for _, signal := range []bool{false, true} {
		t.Run(fmt.Sprint(signal), func(t *testing.T) {
			ctx, cancel := context.WithCancelCause(context.Background())
			defer cancel(nil)
			closed := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer close(closed)
				fmt.Fprint(w, proxyText)
				w.(http.Flusher).Flush()
				<-r.Context().Done()
			}))
			defer server.Close()
			options := agent.ProxyStreamOptions{ProxyURL: server.URL}
			requestContext := ctx
			if signal {
				options.Signal = ctx
				requestContext = context.Background()
			}
			stream := agent.StreamProxy(requestContext, ai.Model{}, ai.Context{}, options)
			for {
				event, ok, err := stream.Next(context.Background())
				if err != nil || !ok {
					t.Fatalf("early end: %v %v", ok, err)
				}
				if event.AssistantMessageEventType() == ai.AssistantMessageEventTypeTextDelta {
					break
				}
			}
			cancel(errors.New("local cancellation"))
			wait, finish := context.WithTimeout(context.Background(), 3*time.Second)
			defer finish()
			result, err := stream.Result(wait)
			if err != nil || result.StopReason != ai.StopReasonAborted || result.ErrorMessage != ai.Some("local cancellation") || result.Content[0].(ai.TextContent).Text != "hello" {
				t.Fatalf("cancel outcome=%+v %v", result, err)
			}
			select {
			case <-closed:
			case <-wait.Done():
				t.Fatal("HTTP request did not release server")
			}
		})
	}
}

func TestProxyCancellationReleasesInjectedReaderAndFetch(t *testing.T) {
	for _, duringFetch := range []bool{false, true} {
		t.Run(fmt.Sprint(duringFetch), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			started, finished := make(chan struct{}), make(chan struct{})
			reader, writer := io.Pipe()
			defer writer.Close()
			options := agent.ProxyStreamOptions{Fetch: func(ctx context.Context, _ ai.FetchRequest) (ai.FetchResponse, error) {
				close(started)
				if duringFetch {
					defer close(finished)
					<-ctx.Done()
					return ai.FetchResponse{}, context.Cause(ctx)
				}
				go func() {
					defer close(finished)
					_, _ = io.WriteString(writer, proxyText)
					_, _ = io.WriteString(writer, strings.Repeat(" ", 1<<20))
				}()
				return ai.FetchResponse{Status: 200, BodyReader: reader}, nil
			}}
			stream := agent.StreamProxy(ctx, ai.Model{}, ai.Context{}, options)
			<-started
			if !duringFetch {
				for {
					event, _, err := stream.Next(context.Background())
					if err != nil {
						t.Fatal(err)
					}
					if event.AssistantMessageEventType() == ai.AssistantMessageEventTypeTextDelta {
						break
					}
				}
			}
			cancel()
			wait, done := context.WithTimeout(context.Background(), 3*time.Second)
			defer done()
			result, err := stream.Result(wait)
			if err != nil || result.StopReason != ai.StopReasonAborted {
				t.Fatalf("outcome=%+v %v", result, err)
			}
			select {
			case <-finished:
			case <-wait.Done():
				t.Fatal("transport producer leaked")
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, signal := range []bool{false, true} {
		options := agent.ProxyStreamOptions{Fetch: func(context.Context, ai.FetchRequest) (ai.FetchResponse, error) {
			t.Error("canceled request invoked Fetch")
			return ai.FetchResponse{}, nil
		}}
		requestContext := ctx
		if signal {
			options.Signal = ctx
			requestContext = context.Background()
		}
		result, err := agent.StreamProxy(requestContext, ai.Model{}, ai.Context{}, options).Result(context.Background())
		if err != nil || result.StopReason != ai.StopReasonAborted {
			t.Fatalf("pre-cancel=%+v %v", result, err)
		}
	}
}

func TestProxySupportsConcurrentReadersAndResultWaiters(t *testing.T) {
	events := `data: {"type":"start"}` + "\n\n" + `data: {"type":"text_start","contentIndex":0}` + "\n\n"
	for range 100 {
		events += `data: {"type":"text_delta","contentIndex":0,"delta":"x"}` + "\n\n"
	}
	events += `data: {"type":"done","reason":"length","usage":` + proxyUsage + "}\n\n"
	stream := agent.StreamProxy(context.Background(), ai.Model{}, ai.Context{}, proxyBuffered(events))
	var wg sync.WaitGroup
	var count atomic.Int32
	for range 8 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			for {
				event, ok, err := stream.Next(context.Background())
				if err != nil {
					t.Error(err)
					return
				}
				if !ok {
					return
				}
				count.Add(1)
				if e, ok := event.(ai.AssistantMessageTextDeltaEvent); ok {
					e.Partial.Content[0] = ai.TextContent{Type: ai.ContentTypeText, Text: "mutated"}
				}
			}
		}()
		go func() {
			defer wg.Done()
			for range 5 {
				result, err := stream.Result(context.Background())
				if err != nil || result.StopReason != ai.StopReasonLength || result.Content[0].(ai.TextContent).Text != strings.Repeat("x", 100) {
					t.Errorf("result=%+v %v", result, err)
					return
				}
				result.Content[0] = ai.TextContent{Type: ai.ContentTypeText, Text: "mutated"}
			}
		}()
	}
	wg.Wait()
	if count.Load() != 103 {
		t.Fatalf("events=%d, want 103", count.Load())
	}
}

func TestLegacyAgentRunsProxyThroughPublicSDK(t *testing.T) {
	for _, failed := range []bool{false, true} {
		t.Run(fmt.Sprint(failed), func(t *testing.T) {
			terminal := `data: {"type":"done","reason":"stop","usage":` + proxyUsage + "}\n\n"
			if failed {
				terminal = `data: {"type":"error","reason":"error","errorMessage":"denied","usage":` + proxyUsage + "}\n\n"
			}
			created, err := agent.NewAgent(agent.AgentOptions{StreamFunction: func(ctx context.Context, m ai.Model, c ai.Context, o ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
				return agent.StreamProxy(ctx, m, c, proxyBuffered(proxyText+terminal)).AssistantMessageEventStream()
			}})
			if err != nil {
				t.Fatal(err)
			}
			var events []agent.AgentEventType
			created.Subscribe(func(_ context.Context, e agent.AgentEvent) error {
				events = append(events, e.AgentEventType())
				return nil
			})
			if err := created.PromptText(context.Background(), "hello"); err != nil {
				t.Fatal(err)
			}
			state := created.State()
			if state.IsStreaming || len(state.Messages) != 2 || events[len(events)-1] != agent.AgentEventTypeAgentEnd {
				t.Fatalf("state=%+v events=%v", state, events)
			}
			last := state.Messages[1].(ai.AssistantMessage)
			want := ai.StopReasonStop
			if failed {
				want = ai.StopReasonError
			}
			if last.StopReason != want || last.Content[0].(ai.TextContent).Text != "hello" {
				t.Fatal(last)
			}
		})
	}
}

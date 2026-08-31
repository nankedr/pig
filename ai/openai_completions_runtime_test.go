package ai_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/nankedr/pig/ai"
)

const openAITextSSE = "data: {\"id\":\"chatcmpl-44\",\"model\":\"reply-model\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"你\"}}]}\n\n" +
	"data: {\"id\":\"chatcmpl-44\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"好\"},\"finish_reason\":\"stop\"}]}\n\n" +
	"data: {\"id\":\"chatcmpl-44\",\"choices\":[],\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":2,\"prompt_cache_hit_tokens\":99,\"prompt_tokens_details\":{\"cached_tokens\":3,\"cache_write_tokens\":1}}}\n\n" +
	"data: [DONE]\n\n"

func TestOpenAICompletionsTextStreamUsesPublicRequestAndEventSeams(t *testing.T) {
	fixture := loadOpenAICompletionsTextFixture(t)
	if fixture.Input.SSE != openAITextSSE {
		t.Fatal("Go text stream input differs from the pinned Pi fixture")
	}
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/chat/completions" {
			t.Errorf("request = %s %s, want POST /v1/chat/completions", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization = %q, want Bearer test-key", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, openAITextSSE)
	}))
	defer server.Close()

	key := "test-key"
	temperature := 0.0
	maxTokens := int64(512)
	model := openAITextModel(server.URL + "/v1")
	input := ai.Context{
		SystemPrompt: ai.Some("简洁回答"),
		Messages: []ai.Message{
			ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText("你好"), Timestamp: 1},
			ai.AssistantMessage{Role: ai.MessageRoleAssistant, Content: []ai.AssistantContent{
				ai.TextContent{Type: ai.ContentTypeText, Text: "此前回答"},
			}, API: model.API, Provider: model.Provider, Model: model.ID, StopReason: ai.StopReasonStop, Timestamp: 2},
			ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserBlocks(ai.TextContent{Type: ai.ContentTypeText, Text: "继续"}), Timestamp: 3},
		},
	}
	stream := ai.StreamOpenAICompletions(context.Background(), model, input, ai.OpenAICompletionsOptions{
		StreamOptions: ai.StreamOptions{
			ProviderRequestOptions: ai.ProviderRequestOptions{APIKey: &key},
			Temperature:            &temperature,
			MaxTokens:              &maxTokens,
		},
	})

	events := collectAssistantEvents(t, stream)
	wantTypes := []ai.AssistantMessageEventType{
		ai.AssistantMessageEventTypeStart,
		ai.AssistantMessageEventTypeTextStart,
		ai.AssistantMessageEventTypeTextDelta,
		ai.AssistantMessageEventTypeTextDelta,
		ai.AssistantMessageEventTypeTextEnd,
		ai.AssistantMessageEventTypeDone,
	}
	if got := eventTypes(events); !reflect.DeepEqual(got, wantTypes) {
		t.Fatalf("event types = %#v, want %#v", got, wantTypes)
	}
	if len(fixture.Actual.Events) != len(wantTypes) {
		t.Fatalf("pinned Pi event count = %d, want %d", len(fixture.Actual.Events), len(wantTypes))
	}
	for index, event := range events {
		got := normalizedOpenAIJSON(t, event)
		want := normalizedOpenAIJSON(t, fixture.Actual.Events[index])
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("event %d = %#v, want pinned Pi %#v", index, got, want)
		}
	}
	firstDelta := events[2].(ai.AssistantMessageTextDeltaEvent)
	mutated := firstDelta.Partial.Content[0].(ai.TextContent)
	mutated.Text = "mutated"
	firstDelta.Partial.Content[0] = mutated
	secondDelta := events[3].(ai.AssistantMessageTextDeltaEvent)
	if got := secondDelta.Partial.Content[0].(ai.TextContent).Text; got != "你好" {
		t.Fatalf("second immutable partial text = %q, want 你好", got)
	}

	result, err := stream.Result(context.Background())
	if err != nil {
		t.Fatalf("Result() error = %v", err)
	}
	assertOpenAITextResult(t, result)
	if got, want := normalizedOpenAIJSON(t, result), normalizedOpenAIJSON(t, fixture.Actual.Outcome); !reflect.DeepEqual(got, want) {
		t.Fatalf("result = %#v, want pinned Pi %#v", got, want)
	}
	if requestBody["model"] != model.ID || requestBody["stream"] != true || requestBody["temperature"] != float64(0) {
		t.Fatalf("request fields = %#v", requestBody)
	}
	if requestBody["store"] != false {
		t.Fatalf("store = %#v, want false", requestBody["store"])
	}
	if requestBody["max_completion_tokens"] != float64(maxTokens) {
		t.Fatalf("max_completion_tokens = %#v, want %d", requestBody["max_completion_tokens"], maxTokens)
	}
	if !reflect.DeepEqual(requestBody["stream_options"], map[string]any{"include_usage": true}) {
		t.Fatalf("stream_options = %#v", requestBody["stream_options"])
	}
	wantMessages := []any{
		map[string]any{"role": "system", "content": "简洁回答"},
		map[string]any{"role": "user", "content": "你好"},
		map[string]any{"role": "assistant", "content": "此前回答"},
		map[string]any{"role": "user", "content": []any{map[string]any{"type": "text", "text": "继续"}}},
	}
	if !reflect.DeepEqual(requestBody["messages"], wantMessages) {
		t.Fatalf("messages = %#v, want %#v", requestBody["messages"], wantMessages)
	}
	if !reflect.DeepEqual(requestBody, fixture.Actual.Request.Body) {
		t.Fatalf("request body = %#v, want pinned Pi %#v", requestBody, fixture.Actual.Request.Body)
	}
}

func TestOpenAICompletionsFunctionToolsAndChoiceEnterRequest(t *testing.T) {
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()

	key := "test-key"
	stream := ai.StreamOpenAICompletions(context.Background(), openAITextModel(server.URL), ai.Context{
		Messages: []ai.Message{ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText("Read README.md"), Timestamp: 1}},
		Tools: []ai.Tool{{
			Name: "read", Description: "Read a file",
			Parameters: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`),
		}},
	}, ai.OpenAICompletionsOptions{
		StreamOptions: ai.StreamOptions{ProviderRequestOptions: ai.ProviderRequestOptions{APIKey: &key}},
		ToolChoice:    ai.OpenAIChatToolChoiceRequired,
	})
	if result, err := stream.Result(context.Background()); err != nil || result.StopReason != ai.StopReasonStop {
		t.Fatalf("Result() = (%#v, %v)", result, err)
	}

	wantTools := []any{map[string]any{
		"type": "function",
		"function": map[string]any{
			"name": "read", "description": "Read a file", "strict": false,
			"parameters": map[string]any{
				"type": "object", "properties": map[string]any{"path": map[string]any{"type": "string"}}, "required": []any{"path"},
			},
		},
	}}
	if !reflect.DeepEqual(requestBody["tools"], wantTools) {
		t.Fatalf("tools = %#v, want %#v", requestBody["tools"], wantTools)
	}
	if requestBody["tool_choice"] != "required" {
		t.Fatalf("tool_choice = %#v, want required", requestBody["tool_choice"])
	}
}

func TestOpenAICompletionsInterleavesToolCallsWithImmutablePartialArguments(t *testing.T) {
	sse := "data: {\"id\":\"chatcmpl-tools\",\"choices\":[{\"delta\":{\"tool_calls\":[" +
		"{\"index\":0,\"id\":\"call-read\",\"type\":\"function\",\"function\":{\"name\":\"read\",\"arguments\":\"{\\\"path\\\":\\\"READ\"}}," +
		"{\"id\":\"call-write\",\"type\":\"function\",\"function\":{\"name\":\"write\",\"arguments\":\"{\\\"path\\\":\\\"out\"}}]}}]}\n\n" +
		"data: {\"id\":\"chatcmpl-tools\",\"choices\":[{\"delta\":{\"tool_calls\":[" +
		"{\"id\":\"call-write\",\"function\":{\"arguments\":\".txt\\\",\\\"content\\\":\\\"ok\\\"}\"}}," +
		"{\"index\":0,\"id\":\"changed-read\",\"function\":{\"arguments\":\"ME.md\\\"}\"}}]},\"finish_reason\":\"tool_calls\"}]}\n\n" +
		"data: [DONE]\n\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		for _, fragment := range strings.SplitAfter(sse, "\n\n") {
			_, _ = io.WriteString(w, fragment)
			flusher.Flush()
		}
	}))
	defer server.Close()

	key := "test-key"
	stream := ai.StreamOpenAICompletions(context.Background(), openAITextModel(server.URL), ai.Context{}, ai.OpenAICompletionsOptions{
		StreamOptions: ai.StreamOptions{ProviderRequestOptions: ai.ProviderRequestOptions{APIKey: &key}},
	})
	events := collectAssistantEvents(t, stream)
	wantTypes := []ai.AssistantMessageEventType{
		ai.AssistantMessageEventTypeStart,
		ai.AssistantMessageEventTypeToolCallStart, ai.AssistantMessageEventTypeToolCallDelta,
		ai.AssistantMessageEventTypeToolCallStart, ai.AssistantMessageEventTypeToolCallDelta,
		ai.AssistantMessageEventTypeToolCallDelta, ai.AssistantMessageEventTypeToolCallDelta,
		ai.AssistantMessageEventTypeToolCallEnd, ai.AssistantMessageEventTypeToolCallEnd,
		ai.AssistantMessageEventTypeDone,
	}
	if got := eventTypes(events); !reflect.DeepEqual(got, wantTypes) {
		t.Fatalf("event types = %#v, want %#v", got, wantTypes)
	}
	wantIndexes := []int{0, 0, 1, 1, 1, 0, 0, 1}
	var gotIndexes []int
	for _, event := range events {
		switch event := event.(type) {
		case ai.AssistantMessageToolCallStartEvent:
			gotIndexes = append(gotIndexes, event.ContentIndex)
		case ai.AssistantMessageToolCallDeltaEvent:
			gotIndexes = append(gotIndexes, event.ContentIndex)
		case ai.AssistantMessageToolCallEndEvent:
			gotIndexes = append(gotIndexes, event.ContentIndex)
		}
	}
	if !reflect.DeepEqual(gotIndexes, wantIndexes) {
		t.Fatalf("tool content indexes = %#v, want %#v", gotIndexes, wantIndexes)
	}
	firstRead := events[2].(ai.AssistantMessageToolCallDeltaEvent)
	if got := firstRead.Partial.Content[0].(ai.ToolCall).Arguments["path"]; got != "READ" {
		t.Fatalf("first read partial path = %#v, want READ", got)
	}
	firstRead.Partial.Content[0].(ai.ToolCall).Arguments["path"] = "mutated"
	lastRead := events[6].(ai.AssistantMessageToolCallDeltaEvent)
	if got := lastRead.Partial.Content[0].(ai.ToolCall).Arguments["path"]; got != "README.md" {
		t.Fatalf("last immutable read partial path = %#v, want README.md", got)
	}

	result, err := stream.Result(context.Background())
	if err != nil || result.StopReason != ai.StopReasonToolUse || len(result.Content) != 2 {
		t.Fatalf("Result() = (%#v, %v)", result, err)
	}
	read := result.Content[0].(ai.ToolCall)
	write := result.Content[1].(ai.ToolCall)
	if read.ID != "call-read" || read.Name != "read" || !reflect.DeepEqual(read.Arguments, map[string]any{"path": "README.md"}) {
		t.Fatalf("read tool call = %#v", read)
	}
	if write.ID != "call-write" || write.Name != "write" || !reflect.DeepEqual(write.Arguments, map[string]any{"path": "out.txt", "content": "ok"}) {
		t.Fatalf("write tool call = %#v", write)
	}
}

func TestOpenAICompletionsEveryToolArgumentPrefixMatchesPi(t *testing.T) {
	fixture := loadOpenAICompletionsToolsFixture(t)
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		for _, frame := range strings.SplitAfter(fixture.Input.SSE, "\n\n") {
			_, _ = io.WriteString(w, frame)
			flusher.Flush()
		}
	}))
	defer server.Close()

	key := "test-key"
	model := openAITextModel(server.URL)
	model.ID = "tool-model"
	model.Cost = ai.ModelCost{}
	tools := []ai.Tool{
		{
			Name: "read", Description: "Read a file",
			Parameters: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"query":{"type":"string"}},"required":["path"]}`),
		},
		{
			Name: "write", Description: "Write a file",
			Parameters: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"content":{"type":"string"}},"required":["path","content"]}`),
		},
	}
	stream := ai.StreamOpenAICompletions(context.Background(), model, ai.Context{
		Messages: []ai.Message{ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText("Use tools"), Timestamp: 1}},
		Tools:    tools,
	}, ai.OpenAICompletionsOptions{
		StreamOptions: ai.StreamOptions{ProviderRequestOptions: ai.ProviderRequestOptions{APIKey: &key}},
		ToolChoice:    ai.OpenAIChatToolChoiceRequired,
	})
	events := collectAssistantEvents(t, stream)
	if len(events) != len(fixture.Actual.Events) {
		t.Fatalf("event count = %d, want Pi %d", len(events), len(fixture.Actual.Events))
	}
	for i, event := range events {
		got := projectOpenAIToolEvent(t, event)
		if !reflect.DeepEqual(got, fixture.Actual.Events[i]) {
			t.Fatalf("event %d = %#v, want Pi %#v", i, got, fixture.Actual.Events[i])
		}
	}
	result, err := stream.Result(context.Background())
	if err != nil {
		t.Fatalf("Result() error = %v", err)
	}
	if got := normalizedOpenAIJSON(t, result); !reflect.DeepEqual(got, fixture.Actual.Outcome) {
		t.Fatalf("outcome = %#v, want Pi %#v", got, fixture.Actual.Outcome)
	}
	repeated, err := stream.Result(context.Background())
	if err != nil || !reflect.DeepEqual(normalizedOpenAIJSON(t, repeated), fixture.Actual.Outcome) {
		t.Fatalf("repeated Result() = (%#v, %v), want stable Pi outcome", repeated, err)
	}
	if !reflect.DeepEqual(requestBody, fixture.Actual.Request.Body) {
		t.Fatalf("request body = %#v, want Pi %#v", requestBody, fixture.Actual.Request.Body)
	}
	readDeltas := 0
	var surrogateDeltas [][]byte
	for _, event := range fixture.Actual.Events {
		if event["type"] == "toolcall_delta" && event["contentIndex"] == float64(0) {
			readDeltas++
		}
	}
	for _, event := range events {
		if delta, ok := event.(ai.AssistantMessageToolCallDeltaEvent); ok && !utf8.ValidString(delta.Delta) {
			surrogateDeltas = append(surrogateDeltas, []byte(delta.Delta))
			if len(surrogateDeltas) == 1 {
				native := delta.Partial.Content[delta.ContentIndex].(ai.ToolCall).Arguments["native"]
				if got, ok := native.(string); !ok || !bytes.Equal([]byte(got), append([]byte("原生"), 0xed, 0xa0, 0xbd)) {
					t.Fatalf("high-surrogate partial native = %#v, want Pi UTF-16 prefix", native)
				}
			}
		}
	}
	if !reflect.DeepEqual(surrogateDeltas, [][]byte{{0xed, 0xa0, 0xbd}, {0xed, 0xb8, 0x80}}) {
		t.Fatalf("surrogate deltas = %#v, want original UTF-16 pair", surrogateDeltas)
	}
	wantReadDeltas := len(utf16.Encode([]rune(fixture.Input.Arguments.Read))) + 3
	if readDeltas != wantReadDeltas {
		t.Fatalf("read delta prefixes = %d, want %d", readDeltas, wantReadDeltas)
	}
}

func TestOpenAICompletionsInvalidToolArgumentsHaveExplicitErrorOutcome(t *testing.T) {
	for _, test := range []struct {
		name, arguments string
		wantPartial     map[string]any
	}{
		{name: "truncated", arguments: `{"path":"README`, wantPartial: map[string]any{"path": "README"}},
		{name: "malformed", arguments: `{"path":}`, wantPartial: map[string]any{}},
		{name: "repaired invalid escape", arguments: `{"path":"bad\q"}`, wantPartial: map[string]any{"path": `bad\q`}},
		{name: "repaired control character", arguments: "{\"path\":\"line\nnext\"}", wantPartial: map[string]any{"path": "line\nnext"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			chunk, err := json.Marshal(map[string]any{
				"id": "chatcmpl-invalid-tool",
				"choices": []any{map[string]any{
					"delta": map[string]any{"tool_calls": []any{map[string]any{
						"index": 0, "id": "call-read", "type": "function",
						"function": map[string]any{"name": "read", "arguments": test.arguments},
					}}},
					"finish_reason": "tool_calls",
				}},
			})
			if err != nil {
				t.Fatalf("marshal chunk: %v", err)
			}
			stream := openAIStreamFromReader(strings.NewReader("data: " + string(chunk) + "\n\ndata: [DONE]\n\n"))
			events := collectAssistantEvents(t, stream)
			wantTypes := []ai.AssistantMessageEventType{
				ai.AssistantMessageEventTypeStart,
				ai.AssistantMessageEventTypeToolCallStart,
				ai.AssistantMessageEventTypeToolCallDelta,
				ai.AssistantMessageEventTypeError,
			}
			if got := eventTypes(events); !reflect.DeepEqual(got, wantTypes) {
				t.Fatalf("event types = %#v, want %#v", got, wantTypes)
			}
			result, err := stream.Result(context.Background())
			if err != nil || result.StopReason != ai.StopReasonError || len(result.Content) != 1 {
				t.Fatalf("Result() = (%#v, %v)", result, err)
			}
			call := result.Content[0].(ai.ToolCall)
			if call.ID != "call-read" || call.Name != "read" || !reflect.DeepEqual(call.Arguments, test.wantPartial) {
				t.Fatalf("partial tool call = %#v, want arguments %#v", call, test.wantPartial)
			}
			message, ok := result.ErrorMessage.Value()
			if !ok || !strings.Contains(message, `parse tool call "read" arguments`) {
				t.Fatalf("error message = %#v", result.ErrorMessage)
			}
		})
	}
}

func TestOpenAICompletionsInvalidLaterToolCallEmitsNoToolEnd(t *testing.T) {
	chunk, err := json.Marshal(map[string]any{
		"choices": []any{map[string]any{
			"delta": map[string]any{"tool_calls": []any{
				map[string]any{"index": 0, "id": "valid", "function": map[string]any{"name": "read", "arguments": `{"path":"README.md"}`}},
				map[string]any{"index": 1, "id": "invalid", "function": map[string]any{"name": "write", "arguments": `{"path":"out.txt"`}},
			}},
			"finish_reason": "tool_calls",
		}},
	})
	if err != nil {
		t.Fatalf("marshal chunk: %v", err)
	}
	events := collectAssistantEvents(t, openAIStreamFromReader(strings.NewReader("data: "+string(chunk)+"\n\ndata: [DONE]\n\n")))
	want := []ai.AssistantMessageEventType{
		ai.AssistantMessageEventTypeStart,
		ai.AssistantMessageEventTypeToolCallStart, ai.AssistantMessageEventTypeToolCallDelta,
		ai.AssistantMessageEventTypeToolCallStart, ai.AssistantMessageEventTypeToolCallDelta,
		ai.AssistantMessageEventTypeError,
	}
	if got := eventTypes(events); !reflect.DeepEqual(got, want) {
		t.Fatalf("event types = %#v, want atomic terminal %#v", got, want)
	}
}

func TestOpenAICompletionsToolCallKeepsFirstIndexBinding(t *testing.T) {
	chunks := []map[string]any{
		{"choices": []any{
			map[string]any{"delta": map[string]any{"tool_calls": []any{
				map[string]any{"index": 0, "id": "call-a", "function": map[string]any{"name": "a", "arguments": `{"a":`}},
			}}},
		}},
		{"choices": []any{
			map[string]any{"delta": map[string]any{"tool_calls": []any{
				map[string]any{"index": 1, "id": "call-a", "function": map[string]any{"arguments": `1}`}},
			}}},
		}},
		{"choices": []any{
			map[string]any{
				"delta": map[string]any{"tool_calls": []any{
					map[string]any{"index": 1, "id": "call-b", "function": map[string]any{"name": "b", "arguments": `{"b":2}`}},
				}},
				"finish_reason": "tool_calls",
			},
		}},
	}
	var sse strings.Builder
	for _, chunk := range chunks {
		encoded, err := json.Marshal(chunk)
		if err != nil {
			t.Fatalf("marshal chunk: %v", err)
		}
		sse.WriteString("data: " + string(encoded) + "\n\n")
	}
	sse.WriteString("data: [DONE]\n\n")
	stream := openAIStreamFromReader(strings.NewReader(sse.String()))
	events := collectAssistantEvents(t, stream)
	result, err := stream.Result(context.Background())
	if err != nil || result.StopReason != ai.StopReasonToolUse || len(result.Content) != 2 {
		t.Fatalf("Result() = (%#v, %v), event types %#v", result, err, eventTypes(events))
	}
	if got := result.Content[0].(ai.ToolCall).Arguments; !reflect.DeepEqual(got, map[string]any{"a": float64(1)}) {
		t.Fatalf("first arguments = %#v", got)
	}
	if got := result.Content[1].(ai.ToolCall).Arguments; !reflect.DeepEqual(got, map[string]any{"b": float64(2)}) {
		t.Fatalf("second arguments = %#v", got)
	}
}

func TestOpenAICompletionsMissingFinishReasonInfersToolUse(t *testing.T) {
	model := openAISSEModel("https://example.test/v1")
	model.Compat = ai.Some(json.RawMessage(`{"supportsFinishReason":false}`))
	key := "test-key"
	stream := ai.StreamOpenAICompletions(context.Background(), model, ai.Context{}, ai.OpenAICompletionsOptions{
		StreamOptions: ai.StreamOptions{ProviderRequestOptions: ai.ProviderRequestOptions{
			APIKey: &key,
			Fetch: func(context.Context, ai.FetchRequest) (ai.FetchResponse, error) {
				body := "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call-a\",\"function\":{\"name\":\"a\",\"arguments\":\"{}\"}}]}}]}\n\ndata: [DONE]\n\n"
				return ai.FetchResponse{Status: http.StatusOK, BodyReader: io.NopCloser(strings.NewReader(body))}, nil
			},
		}},
	})
	events := collectAssistantEvents(t, stream)
	result, err := stream.Result(context.Background())
	if err != nil || result.StopReason != ai.StopReasonToolUse {
		t.Fatalf("Result() = (%#v, %v), event types %#v", result, err, eventTypes(events))
	}
	if done := events[len(events)-1].(ai.AssistantMessageDoneEvent); done.Reason != ai.StopReasonToolUse {
		t.Fatalf("done reason = %q, want toolUse", done.Reason)
	}
}

func TestOpenAICompletionsSSELineSyntaxHasEquivalentOutcome(t *testing.T) {
	fixture := loadOpenAICompletionsSSEFixture(t)
	wantEvents := normalizedOpenAIJSON(t, fixture.Actual.Whole.Events)
	wantResult := normalizedOpenAIJSON(t, fixture.Actual.Whole.Outcome)
	for _, test := range []struct{ name, newline string }{
		{name: "lf", newline: "\n"},
		{name: "crlf", newline: "\r\n"},
		{name: "cr", newline: "\r"},
	} {
		name, newline := test.name, test.newline
		t.Run(name, func(t *testing.T) {
			if !fixture.Actual.LineEndings[name] {
				t.Fatalf("pinned Pi %s equivalence = false", name)
			}
			gotEvents, gotResult := observeOpenAISSE(t, strings.NewReader(strings.Join(fixture.Input.Lines, newline)))
			if !reflect.DeepEqual(gotEvents, wantEvents) || !reflect.DeepEqual(gotResult, wantResult) {
				t.Fatalf("observation differs from pinned Pi fixture")
			}
		})
	}
}

func TestOpenAICompletionsSSEEveryByteSplitMatchesWholeBody(t *testing.T) {
	fixture := loadOpenAICompletionsSSEFixture(t)
	if !fixture.Actual.Fragmentation.OneByte {
		t.Fatal("pinned Pi one-byte fragmentation equivalence = false")
	}
	for index, equivalent := range fixture.Actual.Fragmentation.SplitEquivalent {
		if !equivalent {
			t.Fatalf("pinned Pi representative split %d equivalence = false", index)
		}
	}
	sse := strings.Join(fixture.Input.Lines, "\r\n")
	wantEvents := normalizedOpenAIJSON(t, fixture.Actual.Whole.Events)
	wantResult := normalizedOpenAIJSON(t, fixture.Actual.Whole.Outcome)
	for split := 0; split <= len(sse); split++ {
		reader := io.MultiReader(strings.NewReader(sse[:split]), strings.NewReader(sse[split:]))
		gotEvents, gotResult := observeOpenAISSE(t, reader)
		if !reflect.DeepEqual(gotEvents, wantEvents) || !reflect.DeepEqual(gotResult, wantResult) {
			t.Fatalf("split at byte %d changed observation", split)
		}
	}
}

func TestOpenAICompletionsSSEDoesNotDispatchUnterminatedTrailingFrame(t *testing.T) {
	fixture := loadOpenAICompletionsSSEFixture(t)
	assertPinnedOpenAISSEErrorObservation(t, fixture.Actual.Trailing, "start", "error")
	stream := openAIStreamFromReader(strings.NewReader(fixture.Input.Scenarios.Trailing))
	_, err := stream.Result(context.Background())
	if !errors.Is(err, ai.ErrOpenAISSETruncated) {
		t.Fatalf("Result() error = %v, want truncated SSE protocol error", err)
	}
	if got, want := eventTypes(collectAssistantEvents(t, stream)), []ai.AssistantMessageEventType{ai.AssistantMessageEventTypeStart}; !reflect.DeepEqual(got, want) {
		t.Fatalf("event types = %#v, want %#v", got, want)
	}
}

func TestOpenAICompletionsMalformedSSEIsGoProtocolError(t *testing.T) {
	fixture := loadOpenAICompletionsSSEFixture(t)
	for _, test := range []struct {
		name        string
		sse         string
		observation openAICompletionsSSEObservation
	}{
		{name: "object", sse: fixture.Input.Scenarios.Malformed, observation: fixture.Actual.Malformed},
		{name: "non-object", sse: fixture.Input.Scenarios.MalformedNonObject, observation: fixture.Actual.MalformedNonObject},
	} {
		assertPinnedOpenAISSEErrorObservation(t, test.observation, "start", "error")
		stream := openAIStreamFromReader(strings.NewReader(test.sse))
		_, err := stream.Result(context.Background())
		if !errors.Is(err, ai.ErrOpenAISSEProtocol) || !errors.Is(err, ai.ErrOpenAISSEMalformed) {
			t.Fatalf("%s: Result() error = %v, want malformed SSE protocol error", test.name, err)
		}
		if got, want := eventTypes(collectAssistantEvents(t, stream)), []ai.AssistantMessageEventType{ai.AssistantMessageEventTypeStart}; !reflect.DeepEqual(got, want) {
			t.Fatalf("%s: event types = %#v, want %#v", test.name, got, want)
		}
	}
}

func TestOpenAICompletionsTruncatedJSONHasDistinctProtocolError(t *testing.T) {
	fixture := loadOpenAICompletionsSSEFixture(t)
	assertPinnedOpenAISSEErrorObservation(t, fixture.Actual.Truncated, "start", "error")
	stream := openAIStreamFromReader(strings.NewReader(fixture.Input.Scenarios.Truncated))
	_, err := stream.Result(context.Background())
	if !errors.Is(err, ai.ErrOpenAISSEProtocol) || !errors.Is(err, ai.ErrOpenAISSETruncated) || errors.Is(err, ai.ErrOpenAISSEMalformed) {
		t.Fatalf("Result() error = %v, want truncated SSE protocol error", err)
	}
}

func TestOpenAICompletionsEmptyBodyHasDistinctProtocolError(t *testing.T) {
	fixture := loadOpenAICompletionsSSEFixture(t)
	assertPinnedOpenAISSEErrorObservation(t, fixture.Actual.Empty, "start", "error")
	stream := openAIStreamFromReader(strings.NewReader(fixture.Input.Scenarios.Empty))
	_, err := stream.Result(context.Background())
	if !errors.Is(err, ai.ErrOpenAISSEProtocol) || !errors.Is(err, ai.ErrOpenAISSEEmpty) || errors.Is(err, ai.ErrOpenAISSETruncated) {
		t.Fatalf("Result() error = %v, want empty SSE protocol error", err)
	}
}

func TestOpenAICompletionsPrematureCloseIsTruncatedProtocolError(t *testing.T) {
	fixture := loadOpenAICompletionsSSEFixture(t)
	assertPinnedOpenAISSEErrorObservation(t, fixture.Actual.PrematureClose, "start", "text_start", "text_delta", "text_end", "error")
	stream := openAIStreamFromReader(strings.NewReader(fixture.Input.Scenarios.PrematureClose))
	_, err := stream.Result(context.Background())
	if !errors.Is(err, ai.ErrOpenAISSEProtocol) || !errors.Is(err, ai.ErrOpenAISSETruncated) {
		t.Fatalf("Result() error = %v, want truncated SSE protocol error", err)
	}
	if got, want := eventTypes(collectAssistantEvents(t, stream)), []ai.AssistantMessageEventType{
		ai.AssistantMessageEventTypeStart,
		ai.AssistantMessageEventTypeTextStart,
		ai.AssistantMessageEventTypeTextDelta,
		ai.AssistantMessageEventTypeTextEnd,
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("event types = %#v, want %#v", got, want)
	}
}

func TestOpenAICompletionsHeadersHooksAndStreamingResponseBody(t *testing.T) {
	timeline := make([]string, 0, 4)
	body := &trackedReadCloser{Reader: bytes.NewBufferString(openAITextSSE), onRead: func() {
		timeline = append(timeline, "read")
	}}
	model := openAITextModel("https://example.test/v1")
	model.Headers = map[string]string{"X-Keep": "model", "X-Override": "model", "X-Delete": "model"}
	override, empty, authorization := "request", "", "Bearer custom"
	stream := ai.StreamOpenAICompletions(context.Background(), model, ai.Context{Messages: []ai.Message{
		ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText("original")},
	}}, ai.OpenAICompletionsOptions{StreamOptions: ai.StreamOptions{ProviderRequestOptions: ai.ProviderRequestOptions{
		Headers: ai.ProviderHeaders{
			"x-override":    &override,
			"x-delete":      nil,
			"X-Empty":       &empty,
			"Authorization": &authorization,
		},
		OnPayload: func(_ context.Context, payload ai.JSONValue, _ ai.Model) (ai.PayloadHookResult, error) {
			timeline = append(timeline, "payload")
			if payload.(map[string]any)["model"] != model.ID {
				t.Fatalf("payload = %#v", payload)
			}
			return ai.PayloadHookResult{Replace: true, Value: map[string]any{
				"model": model.ID, "messages": []any{}, "stream": true,
			}}, nil
		},
		Fetch: func(_ context.Context, request ai.FetchRequest) (ai.FetchResponse, error) {
			timeline = append(timeline, "fetch")
			if request.Headers["X-Keep"] != "model" || request.Headers["x-override"] != "request" {
				t.Fatalf("request headers = %#v", request.Headers)
			}
			if _, ok := request.Headers["X-Delete"]; ok {
				t.Fatalf("deleted header retained: %#v", request.Headers)
			}
			if value, ok := request.Headers["X-Empty"]; !ok || value != "" {
				t.Fatalf("empty header = (%q, %t), want present empty", value, ok)
			}
			if got := request.Headers["Authorization"]; got != authorization {
				t.Fatalf("Authorization = %q, want %q", got, authorization)
			}
			var payload map[string]any
			if err := json.Unmarshal(request.Body, &payload); err != nil || len(payload["messages"].([]any)) != 0 {
				t.Fatalf("replacement body = %#v, error = %v", payload, err)
			}
			return ai.FetchResponse{
				Status:     http.StatusOK,
				Headers:    map[string]string{"X-Response": "ready"},
				BodyReader: body,
			}, nil
		},
		OnResponse: func(_ context.Context, response ai.ProviderResponse, _ ai.Model) error {
			timeline = append(timeline, "response")
			if response.Status != http.StatusOK || response.Headers["X-Response"] != "ready" || body.reads.Load() != 0 {
				t.Fatalf("response hook = %#v, body reads = %d", response, body.reads.Load())
			}
			return nil
		},
	}}})
	result, err := stream.Result(context.Background())
	if err != nil {
		t.Fatalf("Result() error = %v", err)
	}
	assertOpenAITextResult(t, result)
	if !body.closed.Load() {
		t.Fatal("streaming response body was not closed")
	}
	if want := []string{"payload", "fetch", "response", "read"}; !reflect.DeepEqual(timeline, want) {
		t.Fatalf("timeline = %#v, want %#v", timeline, want)
	}
}

func TestOpenAICompletionsAuthenticationPrecedesPayloadHook(t *testing.T) {
	var payloadCalls atomic.Int32
	stream := ai.StreamOpenAICompletions(context.Background(), openAITextModel("https://example.test/v1"), ai.Context{}, ai.OpenAICompletionsOptions{
		StreamOptions: ai.StreamOptions{ProviderRequestOptions: ai.ProviderRequestOptions{
			OnPayload: func(context.Context, ai.JSONValue, ai.Model) (ai.PayloadHookResult, error) {
				payloadCalls.Add(1)
				return ai.PayloadHookResult{}, nil
			},
		}},
	})
	result, err := stream.Result(context.Background())
	if err != nil {
		t.Fatalf("Result() error = %v", err)
	}
	if result.StopReason != ai.StopReasonError {
		t.Fatalf("stop reason = %q, want error", result.StopReason)
	}
	if message, ok := result.ErrorMessage.Value(); !ok || !strings.Contains(message, "no API key") {
		t.Fatalf("error message = (%q, %t), want missing API key diagnostic", message, ok)
	}
	if got := payloadCalls.Load(); got != 0 {
		t.Fatalf("payload hook calls = %d, want 0 before authentication succeeds", got)
	}
}

func TestSimpleOpenAICompletionsClampsFromLatestApplicableUsage(t *testing.T) {
	model := openAITextModel("https://example.test/v1")
	model.ContextWindow = 4_200
	model.MaxTokens = 50
	key := "test-key"
	var requestBody map[string]any
	stream := ai.StreamSimpleOpenAICompletions(context.Background(), model, ai.Context{
		SystemPrompt: ai.Some(strings.Repeat("ignored", 100)),
		Messages: []ai.Message{
			ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText(strings.Repeat("old", 100)), Timestamp: 1},
			ai.AssistantMessage{Role: ai.MessageRoleAssistant, Content: []ai.AssistantContent{
				ai.TextContent{Type: ai.ContentTypeText, Text: "prefix"},
			}, Usage: ai.Usage{TotalTokens: 100}, StopReason: ai.StopReasonStop, Timestamp: 2},
			ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText("tail"), Timestamp: 3},
		},
	}, ai.SimpleStreamOptions{StreamOptions: ai.StreamOptions{ProviderRequestOptions: ai.ProviderRequestOptions{
		APIKey: &key,
		Fetch: func(_ context.Context, request ai.FetchRequest) (ai.FetchResponse, error) {
			if err := json.Unmarshal(request.Body, &requestBody); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			return ai.FetchResponse{Status: http.StatusOK, Body: []byte(openAITextSSE)}, nil
		},
	}}})
	if _, err := stream.Result(context.Background()); err != nil {
		t.Fatalf("Result() error = %v", err)
	}
	if got := requestBody["max_completion_tokens"]; got != float64(3) {
		t.Fatalf("max_completion_tokens = %#v, want 3", got)
	}
}

func TestOpenAICompletionsPublicEntrypointsReachLocalService(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, openAITextSSE)
	}))
	defer server.Close()

	if err := ai.ResetAPIProviders(); err != nil {
		t.Fatalf("ResetAPIProviders() error = %v", err)
	}
	t.Cleanup(func() { _ = ai.ResetAPIProviders() })
	model := openAITextModel(server.URL)
	key := "test-key"
	request := ai.ProviderRequestOptions{APIKey: &key}
	streamOptions := ai.StreamOptions{ProviderRequestOptions: request}
	simpleOptions := ai.SimpleStreamOptions{StreamOptions: streamOptions}

	tests := []struct {
		name string
		run  func() (ai.AssistantMessage, error)
	}{
		{"direct stream", func() (ai.AssistantMessage, error) {
			return ai.StreamOpenAICompletions(context.Background(), model, ai.Context{}, ai.OpenAICompletionsOptions{StreamOptions: streamOptions}).Result(context.Background())
		}},
		{"api stream", func() (ai.AssistantMessage, error) {
			return ai.OpenAICompletionsAPI().Stream(context.Background(), model, ai.Context{}, streamOptions).Result(context.Background())
		}},
		{"api simple", func() (ai.AssistantMessage, error) {
			return ai.OpenAICompletionsAPI().StreamSimple(context.Background(), model, ai.Context{}, simpleOptions).Result(context.Background())
		}},
		{"direct simple", func() (ai.AssistantMessage, error) {
			return ai.StreamSimpleOpenAICompletions(context.Background(), model, ai.Context{}, simpleOptions).Result(context.Background())
		}},
		{"compat stream", func() (ai.AssistantMessage, error) {
			return ai.Stream(context.Background(), model, ai.Context{}, streamOptions).Result(context.Background())
		}},
		{"compat typed stream", func() (ai.AssistantMessage, error) {
			return ai.Stream(context.Background(), model, ai.Context{}, ai.OpenAICompletionsOptions{StreamOptions: streamOptions}).Result(context.Background())
		}},
		{"compat complete", func() (ai.AssistantMessage, error) {
			return ai.Complete(context.Background(), model, ai.Context{}, streamOptions)
		}},
		{"compat simple", func() (ai.AssistantMessage, error) {
			return ai.StreamSimple(context.Background(), model, ai.Context{}, simpleOptions).Result(context.Background())
		}},
		{"compat complete simple", func() (ai.AssistantMessage, error) {
			return ai.CompleteSimple(context.Background(), model, ai.Context{}, simpleOptions)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := test.run()
			if err != nil {
				t.Fatalf("entrypoint error = %v", err)
			}
			assertOpenAITextResult(t, result)
		})
	}
	if got := calls.Load(); got != int32(len(tests)) {
		t.Fatalf("local service calls = %d, want %d", got, len(tests))
	}
}

func TestOpenAICompletionsCancellationClosesStreamingBodyAndKeepsPartialText(t *testing.T) {
	fixture := loadOpenAICompletionsSSEFixture(t)
	assertPinnedOpenAISSEErrorObservation(t, fixture.Actual.Cancel, "start", "text_start", "text_delta", "text_end", "error")
	body := newBlockingReadCloser(fixture.Input.Scenarios.Blocked)
	key := "test-key"
	ctx, cancel := context.WithCancel(context.Background())
	var signalForwarded atomic.Bool
	stream := ai.StreamOpenAICompletions(ctx, openAISSEModel("https://example.test/v1"), ai.Context{}, ai.OpenAICompletionsOptions{
		StreamOptions: ai.StreamOptions{ProviderRequestOptions: ai.ProviderRequestOptions{
			APIKey: &key,
			Fetch: func(fetchContext context.Context, _ ai.FetchRequest) (ai.FetchResponse, error) {
				signalForwarded.Store(fetchContext == ctx)
				return ai.FetchResponse{Status: http.StatusOK, BodyReader: body}, nil
			},
		}},
	})
	var observed []ai.AssistantMessageEvent
	for {
		event, ok, err := stream.Next(context.Background())
		if err != nil || !ok {
			t.Fatalf("Next() before cancellation = (%#v, %t, %v)", event, ok, err)
		}
		observed = append(observed, event)
		if event.AssistantMessageEventType() == ai.AssistantMessageEventTypeTextDelta {
			break
		}
	}
	cancel()
	waitCtx, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()
	result, err := stream.Result(waitCtx)
	if err != nil {
		t.Fatalf("Result() error = %v", err)
	}
	if result.StopReason != ai.StopReasonAborted || len(result.Content) != 1 || result.Content[0].(ai.TextContent).Text != "你" {
		t.Fatalf("aborted result = %#v", result)
	}
	if got, want := normalizedOpenAIJSON(t, result), normalizedOpenAIJSON(t, fixture.Actual.Cancel.Outcome); !reflect.DeepEqual(got, want) {
		t.Fatalf("aborted result = %#v, want pinned Pi %#v", got, want)
	}
	if !body.closed.Load() {
		t.Fatal("cancellation did not close the streaming body")
	}
	if !signalForwarded.Load() {
		t.Fatal("caller cancellation context was not forwarded to fetch")
	}
	var remaining []ai.AssistantMessageEventType
	var terminalError ai.AssistantMessageErrorEvent
	for {
		event, ok, nextErr := stream.Next(context.Background())
		if nextErr != nil {
			t.Fatalf("Next() after cancellation error = %v", nextErr)
		}
		if !ok {
			break
		}
		observed = append(observed, event)
		remaining = append(remaining, event.AssistantMessageEventType())
		if value, ok := event.(ai.AssistantMessageErrorEvent); ok {
			terminalError = value
		}
	}
	if want := []ai.AssistantMessageEventType{ai.AssistantMessageEventTypeTextEnd, ai.AssistantMessageEventTypeError}; !reflect.DeepEqual(remaining, want) {
		t.Fatalf("events after cancellation = %#v, want %#v", remaining, want)
	}
	gotEvents := make([]any, len(observed))
	for index, event := range observed {
		gotEvents[index] = normalizedOpenAIJSON(t, event)
	}
	if want := normalizedOpenAIJSON(t, fixture.Actual.Cancel.Events); !reflect.DeepEqual(gotEvents, want) {
		t.Fatalf("cancellation events = %#v, want pinned Pi %#v", gotEvents, want)
	}
	if terminalError.Type != ai.AssistantMessageEventTypeError || terminalError.Reason != ai.StopReasonAborted || terminalError.Error.StopReason != ai.StopReasonAborted {
		t.Fatalf("terminal error event = %#v", terminalError)
	}
	if len(terminalError.Error.Content) != 1 || terminalError.Error.Content[0].(ai.TextContent).Text != "你" {
		t.Fatalf("terminal error partial content = %#v", terminalError.Error.Content)
	}
	if message, ok := terminalError.Error.ErrorMessage.Value(); !ok || message != "Request was aborted" {
		t.Fatalf("terminal error message = (%q, %t), want Request was aborted", message, ok)
	}
}

func TestOpenAICompletionsTimeoutClosesStreamingBodyAndKeepsPartialText(t *testing.T) {
	fixture := loadOpenAICompletionsSSEFixture(t)
	assertPinnedOpenAISSEErrorObservation(t, fixture.Actual.Timeout, "start", "text_start", "text_delta", "text_end", "error")
	body := newBlockingReadCloser(fixture.Input.Scenarios.Blocked)
	key := "test-key"
	timeout := int64(200)
	stream := ai.StreamOpenAICompletions(context.Background(), openAISSEModel("https://example.test/v1"), ai.Context{}, ai.OpenAICompletionsOptions{
		StreamOptions: ai.StreamOptions{ProviderRequestOptions: ai.ProviderRequestOptions{
			APIKey:    &key,
			TimeoutMS: &timeout,
			Fetch: func(context.Context, ai.FetchRequest) (ai.FetchResponse, error) {
				return ai.FetchResponse{Status: http.StatusOK, BodyReader: body}, nil
			},
		}},
	})
	result, err := stream.Result(context.Background())
	if err != nil {
		t.Fatalf("Result() error = %v", err)
	}
	if result.StopReason != ai.StopReasonAborted || len(result.Content) != 1 || result.Content[0].(ai.TextContent).Text != "你" {
		t.Fatalf("aborted result = %#v", result)
	}
	if message, ok := result.ErrorMessage.Value(); !ok || message != "Request timed out" {
		t.Fatalf("error message = (%q, %t), want Request timed out", message, ok)
	}
	if !body.closed.Load() {
		t.Fatal("timeout did not close the streaming body")
	}
}

func TestOpenAICompletionsSSEBoundariesThroughLocalService(t *testing.T) {
	fixture := loadOpenAICompletionsSSEFixture(t)
	timeoutCanceled := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		scenario := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")[0]
		switch scenario {
		case "success":
			for _, value := range []byte(strings.Join(fixture.Input.Lines, "\r\n")) {
				_, _ = w.Write([]byte{value})
				w.(http.Flusher).Flush()
			}
		case "malformed":
			_, _ = io.WriteString(w, fixture.Input.Scenarios.Malformed)
		case "malformed-non-object":
			_, _ = io.WriteString(w, fixture.Input.Scenarios.MalformedNonObject)
		case "trailing":
			_, _ = io.WriteString(w, fixture.Input.Scenarios.Trailing)
		case "premature":
			_, _ = io.WriteString(w, fixture.Input.Scenarios.PrematureClose)
		case "timeout":
			_, _ = io.WriteString(w, fixture.Input.Scenarios.Blocked)
			w.(http.Flusher).Flush()
			<-r.Context().Done()
			close(timeoutCanceled)
		}
	}))
	defer server.Close()

	key := "test-key"
	for _, test := range []struct {
		name string
		err  error
	}{
		{name: "success"},
		{name: "malformed", err: ai.ErrOpenAISSEMalformed},
		{name: "malformed-non-object", err: ai.ErrOpenAISSEMalformed},
		{name: "trailing", err: ai.ErrOpenAISSETruncated},
		{name: "premature", err: ai.ErrOpenAISSETruncated},
		{name: "empty", err: ai.ErrOpenAISSEEmpty},
	} {
		t.Run(test.name, func(t *testing.T) {
			stream := ai.StreamOpenAICompletions(context.Background(), openAITextModel(server.URL+"/"+test.name), ai.Context{}, ai.OpenAICompletionsOptions{
				StreamOptions: ai.StreamOptions{ProviderRequestOptions: ai.ProviderRequestOptions{APIKey: &key}},
			})
			result, err := stream.Result(context.Background())
			if test.err != nil {
				if !errors.Is(err, test.err) {
					t.Fatalf("Result() error = %v, want %v", err, test.err)
				}
				return
			}
			if err != nil || result.StopReason != ai.StopReasonStop || len(result.Content) != 1 || result.Content[0].(ai.TextContent).Text != "你好" {
				t.Fatalf("Result() = (%#v, %v)", result, err)
			}
		})
	}

	timeout := int64(200)
	result, err := ai.StreamOpenAICompletions(context.Background(), openAITextModel(server.URL+"/timeout"), ai.Context{}, ai.OpenAICompletionsOptions{
		StreamOptions: ai.StreamOptions{ProviderRequestOptions: ai.ProviderRequestOptions{APIKey: &key, TimeoutMS: &timeout}},
	}).Result(context.Background())
	if err != nil || result.StopReason != ai.StopReasonAborted || len(result.Content) != 1 || result.Content[0].(ai.TextContent).Text != "你" {
		t.Fatalf("timeout Result() = (%#v, %v)", result, err)
	}
	select {
	case <-timeoutCanceled:
	case <-time.After(time.Second):
		t.Fatal("timeout did not cancel the local service request")
	}
}

func openAITextModel(baseURL string) ai.Model {
	return ai.Model{
		ID: "text-model", API: ai.APIOpenAICompletions, Provider: "local-openai", BaseURL: baseURL,
		ContextWindow: 32_000, MaxTokens: 2_048,
		Cost: ai.ModelCost{ModelCostRates: ai.ModelCostRates{Input: 1, Output: 2, CacheRead: .5, CacheWrite: 1.5}},
	}
}

func openAISSEModel(baseURL string) ai.Model {
	model := openAITextModel(baseURL)
	model.ID = "sse-model"
	model.Cost = ai.ModelCost{}
	return model
}

func openAIStreamFromReader(reader io.Reader) *ai.AssistantMessageEventStream {
	key := "test-key"
	return ai.StreamOpenAICompletions(context.Background(), openAISSEModel("https://example.test/v1"), ai.Context{}, ai.OpenAICompletionsOptions{
		StreamOptions: ai.StreamOptions{ProviderRequestOptions: ai.ProviderRequestOptions{
			APIKey: &key,
			Fetch: func(context.Context, ai.FetchRequest) (ai.FetchResponse, error) {
				return ai.FetchResponse{Status: http.StatusOK, BodyReader: io.NopCloser(reader)}, nil
			},
		}},
	})
}

func observeOpenAISSE(t *testing.T, reader io.Reader) ([]any, any) {
	t.Helper()
	stream := openAIStreamFromReader(reader)
	events := collectAssistantEvents(t, stream)
	result, err := stream.Result(context.Background())
	if err != nil {
		t.Fatalf("Result() error = %v", err)
	}
	normalizedEvents := make([]any, len(events))
	for index, event := range events {
		normalizedEvents[index] = normalizedOpenAIJSON(t, event)
	}
	return normalizedEvents, normalizedOpenAIJSON(t, result)
}

func collectAssistantEvents(t *testing.T, stream *ai.AssistantMessageEventStream) []ai.AssistantMessageEvent {
	t.Helper()
	var events []ai.AssistantMessageEvent
	for {
		event, ok, err := stream.Next(context.Background())
		if err != nil {
			t.Fatalf("Next() error = %v", err)
		}
		if !ok {
			return events
		}
		events = append(events, event)
	}
}

func eventTypes(events []ai.AssistantMessageEvent) []ai.AssistantMessageEventType {
	types := make([]ai.AssistantMessageEventType, len(events))
	for index, event := range events {
		types[index] = event.AssistantMessageEventType()
	}
	return types
}

func projectOpenAIToolEvent(t *testing.T, event ai.AssistantMessageEvent) map[string]any {
	t.Helper()
	projected := map[string]any{"type": string(event.AssistantMessageEventType())}
	var contentIndex int
	var partial ai.AssistantMessage
	switch event := event.(type) {
	case ai.AssistantMessageToolCallStartEvent:
		contentIndex, partial = event.ContentIndex, event.Partial
	case ai.AssistantMessageToolCallDeltaEvent:
		contentIndex, partial = event.ContentIndex, event.Partial
		projected["delta"] = strings.ToValidUTF8(event.Delta, "�")
	case ai.AssistantMessageToolCallEndEvent:
		contentIndex, partial = event.ContentIndex, event.Partial
		projected["toolCall"] = normalizedOpenAIJSON(t, event.ToolCall)
	case ai.AssistantMessageDoneEvent:
		projected["reason"] = string(event.Reason)
		return projected
	default:
		return projected
	}
	projected["contentIndex"] = float64(contentIndex)
	arguments := partial.Content[contentIndex].(ai.ToolCall).Arguments
	normalizedArguments := normalizedOpenAIJSON(t, arguments).(map[string]any)
	if native, ok := arguments["native"].(string); ok {
		normalizedArguments["native"] = strings.ToValidUTF8(native, "�")
		projected["partialNativeCodeUnits"] = openAIUTF16CodeUnits(native)
	}
	projected["partialArguments"] = normalizedArguments
	return projected
}

func openAIUTF16CodeUnits(value string) []any {
	units := make([]any, 0, len(value))
	for i := 0; i < len(value); {
		if i+2 < len(value) && value[i] == 0xed && value[i+1]&0xe0 == 0xa0 && value[i+2]&0xc0 == 0x80 {
			unit := uint16(value[i]&0x0f)<<12 | uint16(value[i+1]&0x3f)<<6 | uint16(value[i+2]&0x3f)
			if unit >= 0xd800 && unit <= 0xdfff {
				units = append(units, float64(unit))
				i += 3
				continue
			}
		}
		r, size := utf8.DecodeRuneInString(value[i:])
		for _, unit := range utf16.Encode([]rune{r}) {
			units = append(units, float64(unit))
		}
		i += size
	}
	return units
}

func assertOpenAITextResult(t *testing.T, result ai.AssistantMessage) {
	t.Helper()
	if result.StopReason != ai.StopReasonStop || result.API != ai.APIOpenAICompletions || result.ResponseID != ai.Some("chatcmpl-44") || result.ResponseModel != ai.Some("reply-model") {
		t.Fatalf("result identity = %#v", result)
	}
	if len(result.Content) != 1 || result.Content[0].(ai.TextContent).Text != "你好" {
		t.Fatalf("result content = %#v", result.Content)
	}
	if result.Timestamp <= 0 {
		t.Fatalf("result timestamp = %d, want a fresh completion timestamp", result.Timestamp)
	}
	wantUsage := ai.Usage{Input: 6, Output: 2, CacheRead: 3, CacheWrite: 1, TotalTokens: 12}
	gotUsage := result.Usage
	gotCost := gotUsage.Cost
	gotUsage.Cost = ai.UsageCost{}
	if !reflect.DeepEqual(gotUsage, wantUsage) || math.Abs(gotCost.Input-.000006) > 1e-12 ||
		math.Abs(gotCost.Output-.000004) > 1e-12 || math.Abs(gotCost.CacheRead-.0000015) > 1e-12 ||
		math.Abs(gotCost.CacheWrite-.0000015) > 1e-12 || math.Abs(gotCost.Total-.000013) > 1e-12 {
		t.Fatalf("usage = %#v", result.Usage)
	}
}

type trackedReadCloser struct {
	io.Reader
	onRead func()
	reads  atomic.Int32
	closed atomic.Bool
}

type openAICompletionsTextFixture struct {
	ID             string   `json:"id"`
	CatalogIDs     []string `json:"catalog_ids"`
	BaselineCommit string   `json:"baseline_commit"`
	Deterministic  bool     `json:"deterministic"`
	Input          struct {
		SSE string `json:"sse"`
	} `json:"input"`
	Actual struct {
		Request struct {
			Body map[string]any `json:"body"`
		} `json:"request"`
		Events  []any `json:"events"`
		Outcome any   `json:"outcome"`
	} `json:"actual"`
}

type openAICompletionsSSEFixture struct {
	ID             string   `json:"id"`
	CatalogIDs     []string `json:"catalog_ids"`
	BaselineCommit string   `json:"baseline_commit"`
	Deterministic  bool     `json:"deterministic"`
	Input          struct {
		Lines        []string `json:"lines"`
		SplitIndexes []int    `json:"split_indexes"`
		Scenarios    struct {
			Trailing           string `json:"trailing"`
			Malformed          string `json:"malformed"`
			MalformedNonObject string `json:"malformed_non_object"`
			Truncated          string `json:"truncated"`
			Empty              string `json:"empty"`
			PrematureClose     string `json:"premature_close"`
			Blocked            string `json:"blocked"`
		} `json:"scenarios"`
	} `json:"input"`
	Actual struct {
		Whole         openAICompletionsSSEObservation `json:"whole"`
		LineEndings   map[string]bool                 `json:"line_endings"`
		Fragmentation struct {
			OneByte         bool   `json:"one_byte"`
			SplitEquivalent []bool `json:"split_equivalent"`
		} `json:"fragmentation"`
		Trailing           openAICompletionsSSEObservation `json:"trailing"`
		Malformed          openAICompletionsSSEObservation `json:"malformed"`
		MalformedNonObject openAICompletionsSSEObservation `json:"malformed_non_object"`
		Truncated          openAICompletionsSSEObservation `json:"truncated"`
		Empty              openAICompletionsSSEObservation `json:"empty"`
		PrematureClose     openAICompletionsSSEObservation `json:"premature_close"`
		Cancel             openAICompletionsSSEObservation `json:"cancel"`
		Timeout            openAICompletionsSSEObservation `json:"timeout"`
	} `json:"actual"`
}

type openAICompletionsToolsFixture struct {
	ID             string `json:"id"`
	BaselineCommit string `json:"baseline_commit"`
	Deterministic  bool   `json:"deterministic"`
	Input          struct {
		SSE       string `json:"sse"`
		Arguments struct {
			Read  string `json:"read"`
			Write string `json:"write"`
		} `json:"arguments"`
	} `json:"input"`
	Actual struct {
		Request struct {
			Body map[string]any `json:"body"`
		} `json:"request"`
		Events  []map[string]any `json:"events"`
		Outcome map[string]any   `json:"outcome"`
	} `json:"actual"`
}

type openAICompletionsSSEObservation struct {
	Events  []any          `json:"events"`
	Outcome map[string]any `json:"outcome"`
}

func assertPinnedOpenAISSEErrorObservation(t *testing.T, observation openAICompletionsSSEObservation, wantTypes ...string) {
	t.Helper()
	gotTypes := make([]string, len(observation.Events))
	for index, event := range observation.Events {
		gotTypes[index], _ = event.(map[string]any)["type"].(string)
	}
	if !reflect.DeepEqual(gotTypes, wantTypes) {
		t.Fatalf("pinned Pi event types = %#v, want %#v", gotTypes, wantTypes)
	}
	last := observation.Events[len(observation.Events)-1].(map[string]any)
	if observation.Outcome["stopReason"] == nil || observation.Outcome["errorMessage"] == nil || !reflect.DeepEqual(last["error"], observation.Outcome) {
		t.Fatalf("pinned Pi terminal observation = %#v", observation)
	}
}

func normalizedOpenAIJSON(t *testing.T, value any) any {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal JSON value: %v", err)
	}
	var normalized any
	if err := json.Unmarshal(data, &normalized); err != nil {
		t.Fatalf("decode JSON value: %v", err)
	}
	removeOpenAIM1OutOfScopeFields(normalized)
	return normalized
}

func removeOpenAIM1OutOfScopeFields(value any) {
	switch value := value.(type) {
	case map[string]any:
		delete(value, "timestamp")
		delete(value, "reasoning")
		for _, child := range value {
			removeOpenAIM1OutOfScopeFields(child)
		}
	case []any:
		for _, child := range value {
			removeOpenAIM1OutOfScopeFields(child)
		}
	}
}

func loadOpenAICompletionsTextFixture(t *testing.T) openAICompletionsTextFixture {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	data, err := os.ReadFile(filepath.Join(filepath.Dir(file), "..", "parity", "oracle", "fixtures", "openai-completions-text.json"))
	if err != nil {
		t.Fatalf("read text fixture: %v", err)
	}
	var fixture openAICompletionsTextFixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatalf("decode text fixture: %v", err)
	}
	if fixture.ID != "ai/openai-completions/m1-text" || fixture.BaselineCommit != "936aff00918de1187f085f123c2812d8f2d67745" || !fixture.Deterministic || len(fixture.CatalogIDs) == 0 {
		t.Fatalf("text fixture provenance = %#v", fixture)
	}
	return fixture
}

func loadOpenAICompletionsSSEFixture(t *testing.T) openAICompletionsSSEFixture {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	data, err := os.ReadFile(filepath.Join(filepath.Dir(file), "..", "parity", "oracle", "fixtures", "openai-completions-sse.json"))
	if err != nil {
		t.Fatalf("read SSE fixture: %v", err)
	}
	var fixture openAICompletionsSSEFixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatalf("decode SSE fixture: %v", err)
	}
	if fixture.ID != "ai/openai-completions/m1-sse-boundaries" || fixture.BaselineCommit != "936aff00918de1187f085f123c2812d8f2d67745" || !fixture.Deterministic || len(fixture.CatalogIDs) == 0 {
		t.Fatalf("SSE fixture provenance = %#v", fixture)
	}
	return fixture
}

func loadOpenAICompletionsToolsFixture(t *testing.T) openAICompletionsToolsFixture {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	data, err := os.ReadFile(filepath.Join(filepath.Dir(file), "..", "parity", "oracle", "fixtures", "openai-completions-tools.json"))
	if err != nil {
		t.Fatalf("read tools fixture: %v", err)
	}
	var fixture openAICompletionsToolsFixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatalf("decode tools fixture: %v", err)
	}
	if fixture.ID != "ai/openai-completions/m1-streaming-tools" || fixture.BaselineCommit != "936aff00918de1187f085f123c2812d8f2d67745" || !fixture.Deterministic {
		t.Fatalf("tools fixture provenance = %#v", fixture)
	}
	return fixture
}

type blockingReadCloser struct {
	first  *bytes.Reader
	done   chan struct{}
	once   sync.Once
	closed atomic.Bool
}

func newBlockingReadCloser(first string) *blockingReadCloser {
	return &blockingReadCloser{first: bytes.NewReader([]byte(first)), done: make(chan struct{})}
}

func (r *blockingReadCloser) Read(buffer []byte) (int, error) {
	if r.first.Len() > 0 {
		return r.first.Read(buffer)
	}
	<-r.done
	return 0, io.EOF
}

func (r *blockingReadCloser) Close() error {
	r.once.Do(func() {
		r.closed.Store(true)
		close(r.done)
	})
	return nil
}

func (r *trackedReadCloser) Read(buffer []byte) (int, error) {
	if r.reads.Add(1) == 1 && r.onRead != nil {
		r.onRead()
	}
	return r.Reader.Read(buffer)
}

func (r *trackedReadCloser) Close() error {
	r.closed.Store(true)
	return nil
}

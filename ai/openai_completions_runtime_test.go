package ai_test

import (
	"bytes"
	"context"
	"encoding/json"
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
	body := newBlockingReadCloser("data: {\"id\":\"partial\",\"choices\":[{\"delta\":{\"content\":\"你\"}}]}\n\n")
	key := "test-key"
	ctx, cancel := context.WithCancel(context.Background())
	var signalForwarded atomic.Bool
	stream := ai.StreamOpenAICompletions(ctx, openAITextModel("https://example.test/v1"), ai.Context{}, ai.OpenAICompletionsOptions{
		StreamOptions: ai.StreamOptions{ProviderRequestOptions: ai.ProviderRequestOptions{
			APIKey: &key,
			Fetch: func(fetchContext context.Context, _ ai.FetchRequest) (ai.FetchResponse, error) {
				signalForwarded.Store(fetchContext == ctx)
				return ai.FetchResponse{Status: http.StatusOK, BodyReader: body}, nil
			},
		}},
	})
	for {
		event, ok, err := stream.Next(context.Background())
		if err != nil || !ok {
			t.Fatalf("Next() before cancellation = (%#v, %t, %v)", event, ok, err)
		}
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
		remaining = append(remaining, event.AssistantMessageEventType())
		if value, ok := event.(ai.AssistantMessageErrorEvent); ok {
			terminalError = value
		}
		if event.AssistantMessageEventType() == ai.AssistantMessageEventTypeTextEnd {
			t.Fatal("cancellation emitted text_end for an unfinished text block")
		}
	}
	if want := []ai.AssistantMessageEventType{ai.AssistantMessageEventTypeError}; !reflect.DeepEqual(remaining, want) {
		t.Fatalf("events after cancellation = %#v, want %#v", remaining, want)
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

func openAITextModel(baseURL string) ai.Model {
	return ai.Model{
		ID: "text-model", API: ai.APIOpenAICompletions, Provider: "local-openai", BaseURL: baseURL,
		ContextWindow: 32_000, MaxTokens: 2_048,
		Cost: ai.ModelCost{ModelCostRates: ai.ModelCostRates{Input: 1, Output: 2, CacheRead: .5, CacheWrite: 1.5}},
	}
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

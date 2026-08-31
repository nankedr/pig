package ai_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sync"
	"testing"

	"github.com/nankedr/pig/ai"
)

func TestOpenAICompletionsContinuesStreamedToolCallWithTextResult(t *testing.T) {
	fixture := loadOpenAICompletionsToolResultFixture(t)
	const (
		readID  = "call+/read|item=read"
		emptyID = "call-empty+with/special|item-empty=abcdefghijklmnopqrstuvwxyz0123456789"
		errorID = "call-error-abcdefghijklmnopqrstuvwxyz-0123456789"
	)
	var mu sync.Mutex
	var requests []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		mu.Lock()
		requests = append(requests, body)
		call := len(requests)
		mu.Unlock()
		want := fixture.Actual.Requests[1]
		if call == 1 {
			want = fixture.Actual.Requests[0]
		}
		if !reflect.DeepEqual(body, want) {
			t.Errorf("request %d = %#v, want Pi %#v", call, body, want)
			http.Error(w, "request mismatch", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		if call == 1 {
			_, _ = io.WriteString(w, fixture.Input.FirstSSE)
			return
		}
		_, _ = io.WriteString(w, fixture.Input.FinalSSE)
	}))
	defer server.Close()

	key := "test-key"
	tools := []ai.Tool{
		{Name: "read", Description: "Read a file", Parameters: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`)},
		{Name: "empty", Description: "Return no output", Parameters: json.RawMessage(`{"type":"object","properties":{}}`)},
		{Name: "fail", Description: "Return an error", Parameters: json.RawMessage(`{"type":"object","properties":{}}`)},
	}
	sourceModel := openAITextModel(server.URL + "/v1")
	sourceModel.ID = "source-model"
	firstStream := ai.StreamOpenAICompletions(context.Background(), sourceModel, ai.Context{
		Messages: []ai.Message{ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText("Read README"), Timestamp: 1}},
		Tools:    tools,
	}, ai.OpenAICompletionsOptions{
		StreamOptions: ai.StreamOptions{ProviderRequestOptions: ai.ProviderRequestOptions{APIKey: &key}},
		ToolChoice:    ai.OpenAIChatToolChoiceRequired,
	})
	firstEvents := collectAssistantEvents(t, firstStream)
	first, err := firstStream.Result(context.Background())
	if err != nil || first.StopReason != ai.StopReasonToolUse {
		t.Fatalf("first Result() = (%#v, %v)", first, err)
	}
	if got := openAIEventTypeObservations(firstEvents); !reflect.DeepEqual(got, fixture.Actual.First.Events) {
		t.Fatalf("first events = %#v, want Pi %#v", got, fixture.Actual.First.Events)
	}
	if got := normalizedOpenAIJSON(t, first); !reflect.DeepEqual(got, fixture.Actual.First.Outcome) {
		t.Fatalf("first outcome = %#v, want Pi %#v", got, fixture.Actual.First.Outcome)
	}
	if len(first.Content) != 3 {
		t.Fatalf("streamed tool calls = %#v", first.Content)
	}
	readCall := first.Content[0].(ai.ToolCall)
	if readCall.ID != readID || readCall.Name != "read" || !reflect.DeepEqual(readCall.Arguments, map[string]any{"path": "README.md"}) {
		t.Fatalf("streamed read call = %#v", readCall)
	}

	targetModel := sourceModel
	targetModel.ID = "target-model"
	targetModel.Provider = ai.ProviderIDOpenAI
	continuation := ai.Context{Messages: []ai.Message{
		ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText("Read README"), Timestamp: 1},
		first,
		ai.ToolResultMessage{
			Role: ai.MessageRoleToolResult, ToolCallID: readID, ToolName: "read",
			Content: []ai.ToolResultContent{ai.TextContent{Type: ai.ContentTypeText, Text: "README contents"}}, Timestamp: 2,
		},
		ai.ToolResultMessage{Role: ai.MessageRoleToolResult, ToolCallID: emptyID, ToolName: "empty", Content: []ai.ToolResultContent{}, Timestamp: 3},
		ai.ToolResultMessage{
			Role: ai.MessageRoleToolResult, ToolCallID: errorID, ToolName: "fail",
			Content: []ai.ToolResultContent{ai.TextContent{Type: ai.ContentTypeText, Text: "permission denied"}}, IsError: true, Timestamp: 4,
		},
	}, Tools: tools}
	streamOptions := ai.StreamOptions{ProviderRequestOptions: ai.ProviderRequestOptions{APIKey: &key}}
	secondStream := ai.StreamOpenAICompletions(context.Background(), targetModel, continuation, ai.OpenAICompletionsOptions{StreamOptions: streamOptions})
	secondEvents := collectAssistantEvents(t, secondStream)
	second, err := secondStream.Result(context.Background())
	if err != nil {
		t.Fatalf("second Result() error = %v", err)
	}
	if got := openAIEventTypeObservations(secondEvents); !reflect.DeepEqual(got, fixture.Actual.Second.Events) {
		t.Fatalf("second events = %#v, want Pi %#v", got, fixture.Actual.Second.Events)
	}
	if got := normalizedOpenAIJSON(t, second); !reflect.DeepEqual(got, fixture.Actual.Second.Outcome) {
		t.Fatalf("second outcome = %#v, want Pi %#v", got, fixture.Actual.Second.Outcome)
	}
	completeContinuation := continuation
	completeContinuation.Messages = append([]ai.Message(nil), continuation.Messages...)
	errorResult := completeContinuation.Messages[4].(ai.ToolResultMessage)
	errorResult.IsError = false
	completeContinuation.Messages[4] = errorResult
	completed, err := ai.Complete(context.Background(), targetModel, completeContinuation, streamOptions)
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if got, want := normalizedOpenAIJSON(t, completed), normalizedOpenAIJSON(t, second); !reflect.DeepEqual(got, want) {
		t.Fatalf("Complete() = %#v, want Stream result %#v", got, want)
	}
}

func TestOpenAICompletionsRejectsDeferredToolResultsBeforeRequest(t *testing.T) {
	called := false
	key := "test-key"
	result, err := ai.StreamOpenAICompletions(context.Background(), openAITextModel("https://example.test/v1"), ai.Context{
		Messages: []ai.Message{ai.ToolResultMessage{
			Role: ai.MessageRoleToolResult, AddedToolNames: ai.Some([]string{}),
		}},
	}, ai.OpenAICompletionsOptions{StreamOptions: ai.StreamOptions{ProviderRequestOptions: ai.ProviderRequestOptions{
		APIKey: &key,
		Fetch: func(context.Context, ai.FetchRequest) (ai.FetchResponse, error) {
			called = true
			return ai.FetchResponse{}, nil
		},
	}}}).Result(context.Background())
	if !errors.Is(err, ai.ErrNotImplemented) || result.StopReason != "" {
		t.Fatalf("Result() = (%#v, %v), want Capability Stub", result, err)
	}
	if called {
		t.Fatal("deferred ToolResult reached transport")
	}
}

func openAIEventTypeObservations(events []ai.AssistantMessageEvent) []any {
	result := make([]any, len(events))
	for i, event := range events {
		result[i] = map[string]any{"type": string(event.AssistantMessageEventType())}
	}
	return result
}

type openAICompletionsToolResultFixture struct {
	ID             string   `json:"id"`
	CatalogIDs     []string `json:"catalog_ids"`
	BaselineCommit string   `json:"baseline_commit"`
	Deterministic  bool     `json:"deterministic"`
	Input          struct {
		FirstSSE string `json:"firstSSE"`
		FinalSSE string `json:"finalSSE"`
	} `json:"input"`
	Actual struct {
		Requests []map[string]any `json:"requests"`
		First    struct {
			Events  []any `json:"events"`
			Outcome any   `json:"outcome"`
		} `json:"first"`
		Second struct {
			Events  []any `json:"events"`
			Outcome any   `json:"outcome"`
		} `json:"second"`
	} `json:"actual"`
}

func loadOpenAICompletionsToolResultFixture(t *testing.T) openAICompletionsToolResultFixture {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	data, err := os.ReadFile(filepath.Join(filepath.Dir(file), "..", "parity", "oracle", "fixtures", "openai-completions-tool-result.json"))
	if err != nil {
		t.Fatalf("read ToolResult fixture: %v", err)
	}
	var fixture openAICompletionsToolResultFixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatalf("decode ToolResult fixture: %v", err)
	}
	if fixture.ID != "ai/openai-completions/m1-tool-result-round-trip" || fixture.BaselineCommit != "936aff00918de1187f085f123c2812d8f2d67745" || !fixture.Deterministic || len(fixture.CatalogIDs) == 0 {
		t.Fatalf("ToolResult fixture provenance = %#v", fixture)
	}
	return fixture
}

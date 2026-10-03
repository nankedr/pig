package ai_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nankedr/pig/ai"
)

func responsesModel(endpoint string) ai.Model {
	return ai.Model{ID: "configured-text-model", Provider: ai.ProviderIDDeepSeek, API: ai.APIOpenAIResponses, BaseURL: endpoint, Input: []ai.ModelInput{ai.ModelInputText}, Reasoning: true, MaxTokens: 512, ContextWindow: 32000}
}

func responsesInput() ai.Context {
	return ai.Context{SystemPrompt: ai.Some("简洁回答"), Messages: []ai.Message{ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText("你好")}}}
}

func responsesFrame(value any) string {
	data, _ := json.Marshal(value)
	return "data: " + string(data) + "\n\n"
}

func TestResponsesSDKCompletesThroughDeepSeekAndPublicHelpers(t *testing.T) {
	t.Setenv("DEEPSEEK_API_KEY", "responses-test-key")
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path != "/responses" || r.Header.Get("Authorization") != "Bearer responses-test-key" {
			t.Errorf("invalid target or auth")
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body["model"] != "configured-text-model" || body["instructions"] != "简洁回答" || body["stream"] != true || body["store"] != false || body["messages"] != nil {
			t.Errorf("payload = %#v", body)
		}
		want := []any{map[string]any{"role": "user", "content": []any{map[string]any{"type": "input_text", "text": "你好"}}}}
		if !reflect.DeepEqual(body["input"], want) {
			t.Errorf("input = %#v", body["input"])
		}
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, responsesFrame(map[string]any{"type": "response.output_item.added", "output_index": 0, "item": map[string]any{"type": "message", "id": "msg-1", "content": []any{}}}))
		io.WriteString(w, responsesFrame(map[string]any{"type": "response.content_part.added", "output_index": 0, "content_index": 0, "part": map[string]any{"type": "output_text", "text": ""}}))
		io.WriteString(w, responsesFrame(map[string]any{"type": "response.output_text.delta", "output_index": 0, "content_index": 0, "delta": "你好🙂"}))
		io.WriteString(w, responsesFrame(map[string]any{"type": "response.completed", "response": map[string]any{"id": "resp-1", "status": "completed", "model": "served-model", "usage": map[string]any{"input_tokens": 10, "output_tokens": 5, "total_tokens": 15, "input_tokens_details": map[string]any{"cached_tokens": 3, "cache_write_tokens": 1}, "output_tokens_details": map[string]any{"reasoning_tokens": 2}}}}))
	}))
	defer server.Close()
	model := responsesModel(server.URL)
	key := "responses-test-key"
	options := ai.StreamOptions{ProviderRequestOptions: ai.ProviderRequestOptions{APIKey: &key}}
	models := ai.BuiltinModels()
	runners := map[string]func() (ai.AssistantMessage, error){
		"typed stream": func() (ai.AssistantMessage, error) {
			return ai.StreamOpenAIResponses(context.Background(), model, responsesInput(), ai.OpenAIResponsesOptions{StreamOptions: options}).Result(context.Background())
		},
		"Complete": func() (ai.AssistantMessage, error) {
			return ai.Complete(context.Background(), model, responsesInput(), options)
		},
		"CompleteSimple": func() (ai.AssistantMessage, error) {
			return ai.CompleteSimple(context.Background(), model, responsesInput(), ai.SimpleStreamOptions{StreamOptions: options})
		},
		"Models Complete": func() (ai.AssistantMessage, error) {
			return models.Complete(context.Background(), model, responsesInput())
		},
		"Models StreamSimple": func() (ai.AssistantMessage, error) {
			return models.StreamSimple(context.Background(), model, responsesInput()).Result(context.Background())
		},
		"API entry": func() (ai.AssistantMessage, error) {
			return ai.OpenAIResponsesAPI().Stream(context.Background(), model, responsesInput(), options).Result(context.Background())
		},
	}
	for name, run := range runners {
		t.Run(name, func(t *testing.T) {
			out, err := run()
			if err != nil || out.StopReason != ai.StopReasonStop || len(out.Content) != 1 || out.Content[0].(ai.TextContent).Text != "你好🙂" {
				t.Fatalf("outcome = %#v, %v", out, err)
			}
			if out.Usage.Input != 6 || out.Usage.Output != 5 || out.Usage.CacheRead != 3 || out.Usage.CacheWrite != 1 || out.Usage.TotalTokens != 15 {
				t.Fatalf("usage = %#v", out.Usage)
			}
			if reasoning, ok := out.Usage.Reasoning.Value(); !ok || reasoning != 2 {
				t.Fatalf("reasoning usage = %#v", out.Usage)
			}
			if id, _ := out.ResponseID.Value(); id != "resp-1" {
				t.Fatalf("response ID = %q", id)
			}
		})
	}
	if requests.Load() != 6 {
		t.Fatalf("requests = %d", requests.Load())
	}
}

func TestResponsesSDKEndsEachContentPartExactlyOnce(t *testing.T) {
	key := "test-key"
	stream := ai.StreamOpenAIResponses(context.Background(), responsesModel("https://local.invalid"), responsesInput(), ai.OpenAIResponsesOptions{StreamOptions: ai.StreamOptions{ProviderRequestOptions: ai.ProviderRequestOptions{APIKey: &key, Fetch: func(context.Context, ai.FetchRequest) (ai.FetchResponse, error) {
		return ai.FetchResponse{Status: 200, Body: []byte(responsesFrame(map[string]any{"type": "response.output_text.delta", "delta": "partial"}) + responsesFrame(map[string]any{"type": "response.completed", "response": map[string]any{"status": "completed"}}))}, nil
	}}}})
	var types []ai.AssistantMessageEventType
	for {
		event, ok, err := stream.Next(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			break
		}
		types = append(types, event.AssistantMessageEventType())
	}
	want := []ai.AssistantMessageEventType{ai.AssistantMessageEventTypeStart, ai.AssistantMessageEventTypeTextStart, ai.AssistantMessageEventTypeTextDelta, ai.AssistantMessageEventTypeTextEnd, ai.AssistantMessageEventTypeDone}
	if !reflect.DeepEqual(types, want) {
		t.Fatalf("events = %v, want %v", types, want)
	}
}

func TestResponsesSDKPreservesReasoningMultiplePartsAndTerminalOutcomes(t *testing.T) {
	for _, tc := range []struct {
		name, status, reason string
		stop                 ai.StopReason
	}{
		{"completed", "completed", "", ai.StopReasonStop},
		{"length", "incomplete", "max_output_tokens", ai.StopReasonLength},
		{"filtered", "incomplete", "content_filter", ai.StopReasonError},
		{"failed", "failed", "", ai.StopReasonError},
		{"truncated", "", "", ai.StopReasonError},
		{"malformed", "malformed", "", ai.StopReasonError},
		{"done is not terminal", "done", "", ai.StopReasonError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "text/event-stream")
				frames := responsesFrame(map[string]any{"type": "response.reasoning_text.delta", "output_index": 0, "content_index": 0, "delta": "想一想"}) + responsesFrame(map[string]any{"type": "response.output_text.delta", "output_index": 1, "content_index": 0, "delta": "你🙂"}) + responsesFrame(map[string]any{"type": "response.output_text.delta", "output_index": 1, "content_index": 1, "delta": "好"})
				if tc.status == "malformed" {
					frames += "data: {broken}\n\n"
				} else if tc.status == "done" {
					frames += "data: [DONE]\n\n"
				} else if tc.status != "" {
					response := map[string]any{"status": tc.status, "error": map[string]any{"code": "bad", "message": "failure test-key"}, "incomplete_details": map[string]any{"reason": tc.reason}, "output": []any{map[string]any{"type": "reasoning", "content": []any{map[string]any{"type": "reasoning_text", "text": "想一想"}}}, map[string]any{"type": "message", "content": []any{map[string]any{"type": "output_text", "text": "你🙂"}, map[string]any{"type": "output_text", "text": "好"}}}}}
					response["usage"] = map[string]any{"input_tokens": 10, "output_tokens": 5, "total_tokens": 15, "input_tokens_details": map[string]any{"cached_tokens": 3, "cache_write_tokens": 1}, "output_tokens_details": map[string]any{"reasoning_tokens": 2}}
					frames += responsesFrame(map[string]any{"type": "response." + tc.status, "response": response})
				}
				for _, b := range []byte(frames) {
					w.Write([]byte{b})
					w.(http.Flusher).Flush()
				}
			}))
			defer server.Close()
			key := "test-key"
			retries := 2
			model := responsesModel(server.URL)
			model.Cost = ai.ModelCost{ModelCostRates: ai.ModelCostRates{Input: 1, Output: 2, CacheRead: 0.5, CacheWrite: 1.5}}
			out, err := ai.Complete(context.Background(), model, responsesInput(), ai.OpenAIResponsesOptions{StreamOptions: ai.StreamOptions{ProviderRequestOptions: ai.ProviderRequestOptions{APIKey: &key, MaxRetries: &retries}}})
			if err != nil || out.StopReason != tc.stop || len(out.Content) != 3 {
				t.Fatalf("outcome = %#v, %v", out, err)
			}
			if out.Content[0].(ai.ThinkingContent).Thinking != "想一想" || out.Content[1].(ai.TextContent).Text != "你🙂" || out.Content[2].(ai.TextContent).Text != "好" {
				t.Fatalf("content = %#v", out.Content)
			}
			if calls.Load() != 1 {
				t.Fatalf("stream was retried %d times", calls.Load())
			}
			if tc.status == "completed" || tc.status == "incomplete" || tc.status == "failed" {
				reasoning, ok := out.Usage.Reasoning.Value()
				if out.Usage.Input != 6 || out.Usage.Output != 5 || out.Usage.TotalTokens != 15 || out.Usage.CacheRead != 3 || out.Usage.CacheWrite != 1 || !ok || reasoning != 2 || out.Usage.Cost.Total < 0.000018999 || out.Usage.Cost.Total > 0.000019001 {
					t.Fatalf("terminal usage/cost = %#v", out.Usage)
				}
			}
			if message, _ := out.ErrorMessage.Value(); strings.Contains(message, key) {
				t.Fatal("error exposed credential")
			}
		})
	}
}

func TestResponsesSDKCancelAndTimeoutKeepPartialOutcome(t *testing.T) {
	for _, timeout := range []bool{false, true} {
		t.Run(map[bool]string{false: "cancel", true: "timeout"}[timeout], func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				io.WriteString(w, responsesFrame(map[string]any{"type": "response.output_text.delta", "delta": "partial"}))
				w.(http.Flusher).Flush()
				<-r.Context().Done()
			}))
			defer server.Close()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			key := "test-key"
			ms := int64(300)
			options := ai.OpenAIResponsesOptions{StreamOptions: ai.StreamOptions{ProviderRequestOptions: ai.ProviderRequestOptions{APIKey: &key}}}
			if timeout {
				options.TimeoutMS = &ms
			}
			stream := ai.StreamOpenAIResponses(ctx, responsesModel(server.URL), responsesInput(), options)
			bound, release := context.WithTimeout(context.Background(), 3*time.Second)
			defer release()
			for {
				event, ok, err := stream.Next(bound)
				if err != nil || !ok {
					t.Fatalf("stream ended before partial: %v", err)
				}
				if event.AssistantMessageEventType() == ai.AssistantMessageEventTypeTextDelta {
					break
				}
			}
			if !timeout {
				cancel()
			}
			out, err := stream.Result(bound)
			if err != nil || out.StopReason != ai.StopReasonAborted || len(out.Content) != 1 || out.Content[0].(ai.TextContent).Text != "partial" {
				t.Fatalf("outcome = %#v, %v", out, err)
			}
			message, _ := out.ErrorMessage.Value()
			if timeout && !strings.Contains(message, "timed out") {
				t.Fatalf("timeout = %q", message)
			}
			var wg sync.WaitGroup
			for range 8 {
				wg.Add(1)
				go func() {
					defer wg.Done()
					copy, err := stream.Result(bound)
					if err != nil || !reflect.DeepEqual(copy, out) {
						t.Errorf("repeat outcome differs: %v", err)
					}
					copy.Content[0] = ai.TextContent{Type: ai.ContentTypeText, Text: "changed"}
				}()
			}
			wg.Wait()
		})
	}
}

func TestResponsesSDKRetriesOnlyBeforeOutputAndHonorsRetryAfter(t *testing.T) {
	for _, status := range []int{429, 401} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if calls.Add(1) == 1 {
					w.Header().Set("Retry-After", "0")
					w.WriteHeader(status)
					io.WriteString(w, `{"error":"denied"}`)
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				io.WriteString(w, responsesFrame(map[string]any{"type": "response.completed", "response": map[string]any{"status": "completed", "output": []any{map[string]any{"type": "message", "content": []any{map[string]any{"type": "output_text", "text": "retry success"}}}}}}))
			}))
			defer server.Close()
			key := "test-key"
			retries := 1
			maxDelay := int64(5)
			out, err := ai.Complete(context.Background(), responsesModel(server.URL), responsesInput(), ai.StreamOptions{ProviderRequestOptions: ai.ProviderRequestOptions{APIKey: &key, MaxRetries: &retries, MaxRetryDelayMS: &maxDelay}})
			if err != nil {
				t.Fatal(err)
			}
			if status == 429 && (calls.Load() != 2 || out.StopReason != ai.StopReasonStop) {
				t.Fatalf("retry = %d, %#v", calls.Load(), out)
			}
			if status == 401 && (calls.Load() != 1 || out.StopReason != ai.StopReasonError) {
				t.Fatalf("auth retry = %d, %#v", calls.Load(), out)
			}
		})
	}
}

func TestResponsesSDKHooksHeadersAndExplicitStubBoundaries(t *testing.T) {
	key := "test-key"
	custom := "custom"
	effort := ai.OpenAIReasoningEffortHigh
	maxTokens := int64(123)
	var hooks atomic.Int32
	options := ai.OpenAIResponsesOptions{ReasoningEffort: &effort, StreamOptions: ai.StreamOptions{MaxTokens: &maxTokens, ProviderRequestOptions: ai.ProviderRequestOptions{APIKey: &key, Headers: ai.ProviderHeaders{"X-Custom": &custom, "X-Removed": nil}, OnPayload: func(_ context.Context, value ai.JSONValue, _ ai.Model) (ai.PayloadHookResult, error) {
		hooks.Add(1)
		body := value.(map[string]any)
		if body["max_output_tokens"] != int64(123) || body["reasoning"].(map[string]any)["effort"] != effort {
			t.Errorf("options = %#v", body)
		}
		body["instructions"] = "hooked"
		return ai.PayloadHookResult{}, nil
	}, OnResponse: func(_ context.Context, response ai.ProviderResponse, _ ai.Model) error {
		hooks.Add(1)
		if response.Headers["x-local"] != "yes" {
			t.Errorf("metadata = %#v", response)
		}
		return nil
	}, Fetch: func(_ context.Context, r ai.FetchRequest) (ai.FetchResponse, error) {
		hooks.Add(1)
		var body map[string]any
		json.Unmarshal(r.Body, &body)
		if body["instructions"] != "hooked" || r.Headers["X-Custom"] != "custom" {
			t.Errorf("hook request = %#v", r)
		}
		if _, ok := r.Headers["X-Removed"]; ok {
			t.Error("deleted header survived")
		}
		return ai.FetchResponse{Status: 200, Headers: map[string]string{"x-local": "yes"}, Body: []byte(responsesFrame(map[string]any{"type": "response.completed", "response": map[string]any{"status": "completed"}}))}, nil
	}}}}
	model := responsesModel("https://local.invalid")
	model.Headers = map[string]string{"X-Removed": "old"}
	if out, err := ai.Complete(context.Background(), model, responsesInput(), options); err != nil || out.StopReason != ai.StopReasonStop {
		t.Fatalf("hooks = %#v, %v", out, err)
	}
	if hooks.Load() != 3 {
		t.Fatalf("hooks = %d", hooks.Load())
	}
	for _, mutate := range []func(*ai.Model, *ai.OpenAIResponsesOptions){
		func(m *ai.Model, o *ai.OpenAIResponsesOptions) { m.Provider = ai.ProviderIDOpenAI },
		func(m *ai.Model, o *ai.OpenAIResponsesOptions) { o.ServiceTier = ai.Some(ai.OpenAIServiceTierAuto) },
		func(m *ai.Model, o *ai.OpenAIResponsesOptions) {
			o.ReasoningSummary = ai.Null[ai.OpenAIReasoningSummary]()
		},
		func(m *ai.Model, o *ai.OpenAIResponsesOptions) {
			transport := ai.TransportWebSocket
			o.Transport = &transport
		},
	} {
		m, o := model, options
		mutate(&m, &o)
		if _, err := ai.Complete(context.Background(), m, responsesInput(), o); !errors.Is(err, ai.ErrNotImplemented) {
			t.Fatalf("stub = %v", err)
		}
	}
	if hooks.Load() != 3 {
		t.Fatal("stub invoked transport or hooks")
	}
}

func TestResponsesSDKRedactsSecretsFromProviderStopDetails(t *testing.T) {
	key := "secret-key"
	stream := ai.StreamOpenAIResponses(context.Background(), responsesModel("https://local.invalid"), responsesInput(), ai.OpenAIResponsesOptions{StreamOptions: ai.StreamOptions{ProviderRequestOptions: ai.ProviderRequestOptions{APIKey: &key, Fetch: func(context.Context, ai.FetchRequest) (ai.FetchResponse, error) {
		return ai.FetchResponse{Status: 200, Body: []byte(responsesFrame(map[string]any{"type": "response.output_text.delta", "delta": "partial"}) + responsesFrame(map[string]any{"type": "response.incomplete", "response": map[string]any{"status": "incomplete", "incomplete_details": map[string]any{"reason": "failure " + key}}}))}, nil
	}}}})
	for {
		event, ok, err := stream.Next(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			break
		}
		data, err := json.Marshal(event)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), key) {
			t.Fatal("provider stop details exposed credential in event")
		}
	}
	out, err := stream.Result(context.Background())
	if err != nil || out.StopReason != ai.StopReasonError {
		t.Fatalf("outcome = %#v, %v", out, err)
	}
	data, _ := json.Marshal(out)
	if strings.Contains(string(data), key) {
		t.Fatal("provider stop details exposed credential in outcome")
	}
}

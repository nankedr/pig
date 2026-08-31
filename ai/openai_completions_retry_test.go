package ai_test

import (
	"bytes"
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
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nankedr/pig/ai"
)

func TestOpenAICompletionsRetriesNetworkFailure(t *testing.T) {
	var calls atomic.Int32
	key := "test-key"
	retries := 1
	stream := ai.StreamOpenAICompletions(context.Background(), openAITextModel("https://example.test/v1"), ai.Context{}, ai.OpenAICompletionsOptions{
		StreamOptions: ai.StreamOptions{ProviderRequestOptions: ai.ProviderRequestOptions{
			APIKey:     &key,
			MaxRetries: &retries,
			Fetch: func(context.Context, ai.FetchRequest) (ai.FetchResponse, error) {
				if calls.Add(1) == 1 {
					return ai.FetchResponse{}, errors.New("connection reset")
				}
				return ai.FetchResponse{Status: http.StatusOK, Body: []byte(openAITextSSE)}, nil
			},
		}},
	})
	result, err := stream.Result(context.Background())
	if err != nil || result.StopReason != ai.StopReasonStop || calls.Load() != 2 {
		t.Fatalf("Result() = (%#v, %v), calls = %d", result, err, calls.Load())
	}
}

func TestOpenAICompletionsRetriesRetryableStatuses(t *testing.T) {
	for _, status := range []int{http.StatusRequestTimeout, http.StatusConflict, http.StatusTooManyRequests, http.StatusInternalServerError, 599} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var calls atomic.Int32
			key := "test-key"
			retries := 1
			stream := ai.StreamOpenAICompletions(context.Background(), openAITextModel("https://example.test/v1"), ai.Context{}, ai.OpenAICompletionsOptions{
				StreamOptions: ai.StreamOptions{ProviderRequestOptions: ai.ProviderRequestOptions{
					APIKey:     &key,
					MaxRetries: &retries,
					Fetch: func(context.Context, ai.FetchRequest) (ai.FetchResponse, error) {
						if calls.Add(1) == 1 {
							return ai.FetchResponse{Status: status, Headers: map[string]string{"retry-after-ms": "0"}}, nil
						}
						return ai.FetchResponse{Status: http.StatusOK, Body: []byte(openAITextSSE)}, nil
					},
				}},
			})
			result, err := stream.Result(context.Background())
			if err != nil {
				t.Fatalf("Result() error = %v", err)
			}
			if result.StopReason != ai.StopReasonStop || calls.Load() != 2 {
				t.Fatalf("Result() stop reason = %q, calls = %d", result.StopReason, calls.Load())
			}
		})
	}
}

func TestOpenAICompletionsRetryHeaderOverridesStatus(t *testing.T) {
	run := func(status int, headers map[string]string) (ai.AssistantMessage, int32) {
		t.Helper()
		var calls atomic.Int32
		key := "test-key"
		retries := 1
		result, err := ai.StreamOpenAICompletions(context.Background(), openAITextModel("https://example.test/v1"), ai.Context{}, ai.OpenAICompletionsOptions{
			StreamOptions: ai.StreamOptions{ProviderRequestOptions: ai.ProviderRequestOptions{
				APIKey: &key, MaxRetries: &retries,
				Fetch: func(context.Context, ai.FetchRequest) (ai.FetchResponse, error) {
					if calls.Add(1) == 1 {
						return ai.FetchResponse{Status: status, Headers: headers}, nil
					}
					return ai.FetchResponse{Status: http.StatusOK, Body: []byte(openAITextSSE)}, nil
				},
			}},
		}).Result(context.Background())
		if err != nil {
			t.Fatalf("Result() error = %v", err)
		}
		return result, calls.Load()
	}

	result, calls := run(http.StatusTooManyRequests, map[string]string{"X-Should-Retry": "false"})
	if result.StopReason != ai.StopReasonError || calls != 1 {
		t.Fatalf("forbidden retry Result() = %#v, calls = %d", result, calls)
	}
	result, calls = run(http.StatusBadRequest, map[string]string{"x-should-retry": "true"})
	if result.StopReason != ai.StopReasonStop || calls != 2 {
		t.Fatalf("forced retry Result() = %#v, calls = %d", result, calls)
	}
	result, calls = run(http.StatusNotFound, nil)
	if result.StopReason != ai.StopReasonError || calls != 1 {
		t.Fatalf("non-retryable Result() = %#v, calls = %d", result, calls)
	}
}

func TestOpenAICompletionsRejectsServerDelayAboveConfiguredCap(t *testing.T) {
	var calls atomic.Int32
	key := "test-key"
	retries := 1
	maxDelay := int64(1_000)
	result, err := ai.StreamOpenAICompletions(context.Background(), openAITextModel("https://example.test/v1"), ai.Context{}, ai.OpenAICompletionsOptions{
		StreamOptions: ai.StreamOptions{ProviderRequestOptions: ai.ProviderRequestOptions{
			APIKey: &key, MaxRetries: &retries, MaxRetryDelayMS: &maxDelay,
			Fetch: func(context.Context, ai.FetchRequest) (ai.FetchResponse, error) {
				calls.Add(1)
				return ai.FetchResponse{Status: http.StatusTooManyRequests, Headers: map[string]string{"Retry-After": "2"}, Body: []byte("rate limited")}, nil
			},
		}},
	}).Result(context.Background())
	if err != nil {
		t.Fatalf("Result() error = %v", err)
	}
	message, ok := result.ErrorMessage.Value()
	if result.StopReason != ai.StopReasonError || calls.Load() != 1 || !ok || !strings.Contains(message, "Server requested 2s retry delay (max: 1s). provider response 429: rate limited") {
		t.Fatalf("Result() = %#v, calls = %d", result, calls.Load())
	}
}

func TestOpenAICompletionsCancellationStopsRetryWait(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	called := make(chan struct{})
	var calls atomic.Int32
	key := "test-key"
	retries := 2
	disabledCap := int64(0)
	stream := ai.StreamOpenAICompletions(ctx, openAITextModel("https://example.test/v1"), ai.Context{}, ai.OpenAICompletionsOptions{
		StreamOptions: ai.StreamOptions{ProviderRequestOptions: ai.ProviderRequestOptions{
			APIKey: &key, MaxRetries: &retries, MaxRetryDelayMS: &disabledCap,
			Fetch: func(context.Context, ai.FetchRequest) (ai.FetchResponse, error) {
				if calls.Add(1) == 1 {
					close(called)
				}
				return ai.FetchResponse{Status: http.StatusTooManyRequests, Headers: map[string]string{"retry-after": "277403"}}, nil
			},
		}},
	})
	<-called
	cancel()
	waitCtx, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()
	result, err := stream.Result(waitCtx)
	if err != nil || result.StopReason != ai.StopReasonAborted || calls.Load() != 1 {
		t.Fatalf("Result() = (%#v, %v), calls = %d", result, err, calls.Load())
	}
}

func TestOpenAICompletionsCancellationStopsBlockedRequest(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	called := make(chan struct{})
	var calls atomic.Int32
	key := "test-key"
	retries := 2
	stream := ai.StreamOpenAICompletions(ctx, openAITextModel("https://example.test/v1"), ai.Context{}, ai.OpenAICompletionsOptions{
		StreamOptions: ai.StreamOptions{ProviderRequestOptions: ai.ProviderRequestOptions{
			APIKey: &key, MaxRetries: &retries,
			Fetch: func(fetchCtx context.Context, _ ai.FetchRequest) (ai.FetchResponse, error) {
				if calls.Add(1) == 1 {
					close(called)
				}
				<-fetchCtx.Done()
				return ai.FetchResponse{}, fetchCtx.Err()
			},
		}},
	})
	<-called
	cancel()
	waitCtx, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()
	result, err := stream.Result(waitCtx)
	if err != nil || result.StopReason != ai.StopReasonAborted || calls.Load() != 1 {
		t.Fatalf("Result() = (%#v, %v), calls = %d", result, err, calls.Load())
	}
}

func TestOpenAICompletionsCancellationClosesBlockedErrorBody(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	body := newRetryBlockingBody()
	var calls atomic.Int32
	key := "test-key"
	retries := 2
	stream := ai.StreamOpenAICompletions(ctx, openAITextModel("https://example.test/v1"), ai.Context{}, ai.OpenAICompletionsOptions{
		StreamOptions: ai.StreamOptions{ProviderRequestOptions: ai.ProviderRequestOptions{
			APIKey: &key, MaxRetries: &retries,
			Fetch: func(context.Context, ai.FetchRequest) (ai.FetchResponse, error) {
				calls.Add(1)
				return ai.FetchResponse{Status: http.StatusTooManyRequests, BodyReader: body}, nil
			},
		}},
	})
	<-body.readStarted
	cancel()
	waitCtx, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()
	result, err := stream.Result(waitCtx)
	if err != nil || result.StopReason != ai.StopReasonAborted || calls.Load() != 1 || !body.closed.Load() {
		t.Fatalf("Result() = (%#v, %v), calls = %d, body closed = %t", result, err, calls.Load(), body.closed.Load())
	}
}

func TestOpenAICompletionsRetryReusesPayloadAndCallsResponseHookAfterSuccess(t *testing.T) {
	timeline := make([]string, 0, 4)
	var payloadCalls, responseCalls atomic.Int32
	var bodies [][]byte
	failedBody := &trackedReadCloser{Reader: strings.NewReader("server error")}
	key := "test-key"
	retries := 1
	result, err := ai.StreamOpenAICompletions(context.Background(), openAITextModel("https://example.test/v1"), ai.Context{}, ai.OpenAICompletionsOptions{
		StreamOptions: ai.StreamOptions{ProviderRequestOptions: ai.ProviderRequestOptions{
			APIKey: &key, MaxRetries: &retries,
			OnPayload: func(context.Context, ai.JSONValue, ai.Model) (ai.PayloadHookResult, error) {
				payloadCalls.Add(1)
				timeline = append(timeline, "payload")
				return ai.PayloadHookResult{Replace: true, Value: map[string]any{"model": "replacement", "stream": true}}, nil
			},
			Fetch: func(_ context.Context, request ai.FetchRequest) (ai.FetchResponse, error) {
				bodies = append(bodies, bytes.Clone(request.Body))
				timeline = append(timeline, "fetch")
				if len(bodies) == 1 {
					return ai.FetchResponse{Status: http.StatusInternalServerError, Headers: map[string]string{"retry-after-ms": "0"}, BodyReader: failedBody}, nil
				}
				return ai.FetchResponse{Status: http.StatusOK, Body: []byte(openAITextSSE)}, nil
			},
			OnResponse: func(_ context.Context, response ai.ProviderResponse, _ ai.Model) error {
				responseCalls.Add(1)
				timeline = append(timeline, "response")
				if response.Status != http.StatusOK {
					t.Fatalf("response hook status = %d", response.Status)
				}
				return nil
			},
		}},
	}).Result(context.Background())
	if err != nil || result.StopReason != ai.StopReasonStop {
		t.Fatalf("Result() = (%#v, %v)", result, err)
	}
	if payloadCalls.Load() != 1 || responseCalls.Load() != 1 || len(bodies) != 2 || !bytes.Equal(bodies[0], bodies[1]) || !failedBody.closed.Load() {
		t.Fatalf("payload calls = %d, response calls = %d, bodies = %q, failed body closed = %t", payloadCalls.Load(), responseCalls.Load(), bodies, failedBody.closed.Load())
	}
	if want := []string{"payload", "fetch", "fetch", "response"}; !reflect.DeepEqual(timeline, want) {
		t.Fatalf("timeline = %#v, want %#v", timeline, want)
	}
}

func TestOpenAICompletionsRetryBudgetControlsAttemptsAndErrorOutcome(t *testing.T) {
	zero, two := 0, 2
	for _, test := range []struct {
		name       string
		maxRetries *int
		wantCalls  int32
	}{
		{name: "absent", wantCalls: 1},
		{name: "zero", maxRetries: &zero, wantCalls: 1},
		{name: "exhausted", maxRetries: &two, wantCalls: 3},
	} {
		t.Run(test.name, func(t *testing.T) {
			var calls atomic.Int32
			key := "test-key"
			stream := ai.StreamOpenAICompletions(context.Background(), openAITextModel("https://example.test/v1"), ai.Context{}, ai.OpenAICompletionsOptions{
				StreamOptions: ai.StreamOptions{ProviderRequestOptions: ai.ProviderRequestOptions{
					APIKey: &key, MaxRetries: test.maxRetries,
					Fetch: func(context.Context, ai.FetchRequest) (ai.FetchResponse, error) {
						attempt := calls.Add(1)
						return ai.FetchResponse{Status: http.StatusServiceUnavailable, Headers: map[string]string{"retry-after-ms": "0"}, Body: []byte("attempt " + string(rune('0'+attempt)))}, nil
					},
				}},
			})
			result, err := stream.Result(context.Background())
			if err != nil || result.StopReason != ai.StopReasonError || calls.Load() != test.wantCalls {
				t.Fatalf("Result() = (%#v, %v), calls = %d, want %d", result, err, calls.Load(), test.wantCalls)
			}
			message, ok := result.ErrorMessage.Value()
			if !ok || !strings.Contains(message, "provider response 503: attempt ") {
				t.Fatalf("error message = (%q, %t)", message, ok)
			}
			events := collectAssistantEvents(t, stream)
			if got, want := eventTypes(events), []ai.AssistantMessageEventType{ai.AssistantMessageEventTypeError}; !reflect.DeepEqual(got, want) {
				t.Fatalf("event types = %#v, want %#v", got, want)
			}
		})
	}
}

func TestOpenAICompletionsTransportRetryThroughLocalService(t *testing.T) {
	fixture := loadOpenAIRetryAttemptsFixture(t)
	var calls, payloadCalls, responseCalls atomic.Int32
	var bodies [][]byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
			return
		}
		bodies = append(bodies, body)
		if calls.Add(1) == 1 {
			w.Header().Set("retry-after-ms", "0")
			http.Error(w, "rate limited", http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, openAITextSSE)
	}))
	defer server.Close()

	key := "test-key"
	retries := 1
	result, err := ai.StreamOpenAICompletions(context.Background(), openAITextModel(server.URL+"/v1"), ai.Context{}, ai.OpenAICompletionsOptions{
		StreamOptions: ai.StreamOptions{ProviderRequestOptions: ai.ProviderRequestOptions{
			APIKey: &key, MaxRetries: &retries,
			OnPayload: func(context.Context, ai.JSONValue, ai.Model) (ai.PayloadHookResult, error) {
				payloadCalls.Add(1)
				return ai.PayloadHookResult{Replace: true, Value: map[string]any{"model": "replacement", "stream": true}}, nil
			},
			OnResponse: func(context.Context, ai.ProviderResponse, ai.Model) error {
				responseCalls.Add(1)
				return nil
			},
		}},
	}).Result(context.Background())
	wantCalls := int32(fixture.Actual.Status.Retryable["429"].Attempts)
	if err != nil || result.StopReason != ai.StopReasonStop || calls.Load() != wantCalls || payloadCalls.Load() != 1 || responseCalls.Load() != 1 {
		t.Fatalf("Result() = (%#v, %v), calls = %d, payload = %d, response = %d", result, err, calls.Load(), payloadCalls.Load(), responseCalls.Load())
	}
	if len(bodies) != 2 || !bytes.Equal(bodies[0], bodies[1]) {
		t.Fatalf("request bodies = %q", bodies)
	}
}

type openAIRetryAttemptsFixture struct {
	BaselineCommit string `json:"baseline_commit"`
	Deterministic  bool   `json:"deterministic"`
	Actual         struct {
		Status struct {
			Retryable map[string]struct {
				Attempts int `json:"attempts"`
			} `json:"retryable"`
		} `json:"status"`
	} `json:"actual"`
}

func loadOpenAIRetryAttemptsFixture(t *testing.T) openAIRetryAttemptsFixture {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	data, err := os.ReadFile(filepath.Join(filepath.Dir(file), "..", "parity", "oracle", "fixtures", "openai-completions-retry.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture openAIRetryAttemptsFixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	if !fixture.Deterministic || fixture.BaselineCommit != "936aff00918de1187f085f123c2812d8f2d67745" {
		t.Fatalf("invalid retry fixture provenance: %#v", fixture)
	}
	return fixture
}

type retryBlockingBody struct {
	readStarted chan struct{}
	done        chan struct{}
	readOnce    sync.Once
	closeOnce   sync.Once
	closed      atomic.Bool
}

func newRetryBlockingBody() *retryBlockingBody {
	return &retryBlockingBody{readStarted: make(chan struct{}), done: make(chan struct{})}
}

func (b *retryBlockingBody) Read([]byte) (int, error) {
	b.readOnce.Do(func() { close(b.readStarted) })
	<-b.done
	return 0, io.EOF
}

func (b *retryBlockingBody) Close() error {
	b.closeOnce.Do(func() {
		b.closed.Store(true)
		close(b.done)
	})
	return nil
}

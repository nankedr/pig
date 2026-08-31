package ai_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/nankedr/pig/ai"
)

func TestDeepSeekBuiltinModelsStreamAndCompleteThroughLocalEndpoint(t *testing.T) {
	t.Setenv("DEEPSEEK_API_KEY", "deepseek-test-key")

	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path != "/chat/completions" {
			t.Errorf("request path = %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer deepseek-test-key" {
			t.Errorf("Authorization = %q", got)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if body["model"] != "deepseek-v4-flash" || body["stream"] != true {
			t.Errorf("request body = %#v", body)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"id\":\"chatcmpl-deepseek\",\"model\":\"deepseek-v4-flash\",\"choices\":[{\"delta\":{\"content\":\"hello\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":2,\"completion_tokens\":1}}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()

	models := ai.BuiltinModels()
	model, ok := models.GetModel(ai.ProviderIDDeepSeek, "deepseek-v4-flash")
	if !ok {
		t.Fatal("BuiltinModels has no deepseek-v4-flash")
	}
	model.BaseURL = server.URL
	input := ai.Context{Messages: []ai.Message{
		ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText("hello")},
	}}

	streamResult, err := models.Stream(context.Background(), model, input).Result(context.Background())
	if err != nil || streamResult.StopReason != ai.StopReasonStop || streamResult.Content[0].(ai.TextContent).Text != "hello" {
		t.Fatalf("Stream result = (%#v, %v)", streamResult, err)
	}
	completeResult, err := models.Complete(context.Background(), model, input)
	if err != nil || completeResult.StopReason != ai.StopReasonStop || completeResult.Content[0].(ai.TextContent).Text != "hello" {
		t.Fatalf("Complete result = (%#v, %v)", completeResult, err)
	}
	if got := requests.Load(); got != 2 {
		t.Fatalf("local endpoint requests = %d, want 2", got)
	}
	refresh := models.Refresh(context.Background(), ai.ModelsRefreshOptions{Providers: []ai.ProviderID{ai.ProviderIDDeepSeek}})
	if refresh.Aborted || len(refresh.Errors) != 0 || requests.Load() != 2 {
		t.Fatalf("DeepSeek refresh = %#v, local requests = %d", refresh, requests.Load())
	}
}

func TestDeepSeekBuiltinModelsReturnStructuredCredentialErrors(t *testing.T) {
	modelFor := func(t *testing.T, models ai.Models) ai.Model {
		t.Helper()
		model, ok := models.GetModel(ai.ProviderIDDeepSeek, "deepseek-v4-flash")
		if !ok {
			t.Fatal("BuiltinModels has no deepseek-v4-flash")
		}
		return model
	}

	t.Run("missing", func(t *testing.T) {
		t.Setenv("DEEPSEEK_API_KEY", "")
		models := ai.BuiltinModels()
		calls := 0
		result, err := models.Complete(context.Background(), modelFor(t, models), ai.Context{}, ai.ModelsStreamOptions{
			StreamOptions: ai.StreamOptions{ProviderRequestOptions: ai.ProviderRequestOptions{Fetch: func(context.Context, ai.FetchRequest) (ai.FetchResponse, error) {
				calls++
				return ai.FetchResponse{}, nil
			}}},
		})
		message, present := result.ErrorMessage.Value()
		if err != nil || result.StopReason != ai.StopReasonError || !present || !strings.Contains(message, "auth") || !strings.Contains(message, "not configured") {
			t.Fatalf("missing credential result = (%#v, %v)", result, err)
		}
		if calls != 0 {
			t.Fatalf("missing credential invoked transport %d times", calls)
		}
	})

	t.Run("invalid", func(t *testing.T) {
		t.Setenv("DEEPSEEK_API_KEY", "invalid-key")
		models := ai.BuiltinModels()
		calls := 0
		result, err := models.Complete(context.Background(), modelFor(t, models), ai.Context{}, ai.ModelsStreamOptions{
			StreamOptions: ai.StreamOptions{ProviderRequestOptions: ai.ProviderRequestOptions{Fetch: func(_ context.Context, request ai.FetchRequest) (ai.FetchResponse, error) {
				calls++
				if request.Headers["Authorization"] != "Bearer invalid-key" {
					t.Errorf("Authorization = %q", request.Headers["Authorization"])
				}
				return ai.FetchResponse{Status: http.StatusUnauthorized, Body: []byte(`{"error":"invalid credential"}`)}, nil
			}}},
		})
		message, present := result.ErrorMessage.Value()
		if err != nil || result.StopReason != ai.StopReasonError || !present || !strings.Contains(message, "401") || !strings.Contains(message, "invalid credential") {
			t.Fatalf("invalid credential result = (%#v, %v)", result, err)
		}
		if calls != 1 {
			t.Fatalf("invalid credential transport calls = %d", calls)
		}
	})
}

func TestDeepSeekActivationDoesNotChangeOpenAIProviderAPIOwnership(t *testing.T) {
	t.Parallel()

	provider := ai.OpenAIProvider()
	calls := 0
	result, err := provider.Stream(context.Background(), ai.Model{
		ID: "ownership-probe", Provider: ai.ProviderIDOpenAI, API: ai.APIOpenAICompletions,
	}, ai.Context{}, ai.StreamOptions{ProviderRequestOptions: ai.ProviderRequestOptions{
		Fetch: func(context.Context, ai.FetchRequest) (ai.FetchResponse, error) {
			calls++
			return ai.FetchResponse{}, nil
		},
	}}).Result(context.Background())
	message, present := result.ErrorMessage.Value()
	if err != nil || result.StopReason != ai.StopReasonError || !present || !strings.Contains(message, "no API implementation") {
		t.Fatalf("OpenAI Chat Completions probe = (%#v, %v)", result, err)
	}
	if calls != 0 {
		t.Fatalf("OpenAI ownership probe invoked transport %d times", calls)
	}
}

func TestDeepSeekBuiltinModelsMatchLockedCatalogSnapshot(t *testing.T) {
	t.Parallel()

	models := ai.GetModels(ai.ProviderIDDeepSeek)
	if got := deepSeekModelIDs(models); !reflect.DeepEqual(got, []string{"deepseek-v4-flash", "deepseek-v4-pro"}) {
		t.Fatalf("GetModels(deepseek) IDs = %v", got)
	}

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	data, err := os.ReadFile(filepath.Join(filepath.Dir(file), "..", "parity", "baseline", "catalog", "chat", "source", "providers", "deepseek.json"))
	if err != nil {
		t.Fatal(err)
	}
	var snapshot map[ai.API]map[string]ai.Model
	if err := json.Unmarshal(data, &snapshot); err != nil {
		t.Fatal(err)
	}
	for _, model := range models {
		want := snapshot[ai.APIOpenAICompletions][model.ID]
		if !deepSeekJSONEqual(t, model, want) {
			t.Errorf("model %q does not match locked snapshot\ngot:  %#v\nwant: %#v", model.ID, model, want)
		}
		queried, found := ai.GetModel(ai.ProviderIDDeepSeek, model.ID)
		if !found || !deepSeekJSONEqual(t, queried, want) {
			t.Errorf("GetModel(deepseek, %q) = (%#v, %t)", model.ID, queried, found)
		}
	}

	if generatedAt, ok := ai.GetBuiltinModelDataGeneratedAt(); !ok || generatedAt != 1786081866002 {
		t.Fatalf("GetBuiltinModelDataGeneratedAt() = (%d, %t)", generatedAt, ok)
	}
	if got := ai.GetModels(ai.ProviderIDOpenAI); len(got) != 0 {
		t.Fatalf("GetModels(openai) = %#v, want deferred catalog", got)
	}
	faux, err := ai.NewFauxProvider()
	if err != nil || faux.Provider.ID() != "faux" || len(faux.Models) != 1 || faux.Models[0].Provider == ai.ProviderIDDeepSeek {
		t.Fatalf("independent Faux provider = (%#v, %v)", faux, err)
	}
}

func deepSeekModelIDs(models []ai.Model) []string {
	ids := make([]string, len(models))
	for i, model := range models {
		ids[i] = model.ID
	}
	sort.Strings(ids)
	return ids
}

func deepSeekJSONEqual(t *testing.T, left, right ai.Model) bool {
	t.Helper()
	normalize := func(model ai.Model) any {
		data, err := json.Marshal(model)
		if err != nil {
			t.Fatal(err)
		}
		var value any
		if err := json.Unmarshal(data, &value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	return reflect.DeepEqual(normalize(left), normalize(right))
}

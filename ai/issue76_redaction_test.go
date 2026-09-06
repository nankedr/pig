package ai_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/nankedr/pig/ai"
)

func TestCredential76ModelHeaderRedaction(t *testing.T) {
	const key = "SYNTHETIC_76_REQUEST_KEY"
	const token = "SYNTHETIC_76_MODEL_TOKEN"
	const cookie = "SYNTHETIC_76_MODEL_COOKIE"
	called := false
	model := ai.Model{API: ai.APIOpenAICompletions, Provider: "openai", ID: "synthetic", Headers: map[string]string{"Authorization": "Bearer " + token, "Cookie": cookie}}
	result, err := ai.StreamOpenAICompletions(context.Background(), model, ai.Context{}, ai.OpenAICompletionsOptions{StreamOptions: ai.StreamOptions{ProviderRequestOptions: ai.ProviderRequestOptions{APIKey: func() *string { v := key; return &v }(), Fetch: func(_ context.Context, request ai.FetchRequest) (ai.FetchResponse, error) {
		called = true
		if request.Headers["Authorization"] != "Bearer "+token || request.Headers["Cookie"] != cookie {
			t.Error("model auth headers not sent")
		}
		return ai.FetchResponse{Status: 401, Body: []byte("invalid " + key + " " + token + " " + cookie)}, nil
	}}}}).Result(context.Background())
	if err != nil || !called {
		t.Fatalf("request failed before transport: %v", err)
	}
	data, _ := json.Marshal(result)
	for _, secret := range []string{key, token, cookie} {
		if strings.Contains(string(data), secret) {
			t.Fatal("model credential leaked into error outcome")
		}
	}
	if result.StopReason != ai.StopReasonError {
		t.Fatal("error hidden")
	}
}

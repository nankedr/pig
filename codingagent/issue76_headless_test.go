package codingagent_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
	"github.com/nankedr/pig/telemetry"
)

func TestCredential76ExplicitHeadlessAuthPath(t *testing.T) {
	dir := t.TempDir()
	explicit := filepath.Join(t.TempDir(), "explicit.json")
	for path, key := range map[string]string{filepath.Join(dir, "auth.json"): "default-76", explicit: "explicit-76"} {
		store, err := codingagent.NewAuthStorage(path)
		if err != nil {
			t.Fatal(err)
		}
		_, err = store.Modify(context.Background(), "deepseek", func(context.Context, ai.Credential) (ai.Credential, error) {
			return ai.APIKeyCredential{Type: ai.AuthTypeAPIKey, Key: ai.Some("$SCOPED_KEY"), Env: ai.ProviderEnv{"SCOPED_KEY": key}}, nil
		}, ai.AuthOperationOptions{})
		if err != nil {
			t.Fatal(err)
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer explicit-76" {
			t.Error("explicit AuthPath was ignored")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"reply\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	runtime, err := codingagent.CreateHeadlessSession(context.Background(), codingagent.CreateHeadlessSessionOptions{CWD: dir, AgentDir: dir, AuthPath: explicit, Provider: "deepseek", Model: "deepseek-v4-flash", BaseURL: &server.URL, NoTools: codingagent.NoToolsAll, SessionManager: codingagent.NewInMemorySessionManager(dir)})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Session().Dispose()
	out, err := codingagent.RunHeadless(context.Background(), runtime, codingagent.HeadlessRunOptions{Messages: []string{"hello"}})
	if err != nil || strings.Join(out.Text, "") != "reply" {
		t.Fatalf("headless: %+v %v", out, err)
	}
}

func TestCredential76ModelsTelemetryAndErrors(t *testing.T) {
	store, err := codingagent.NewAuthStorage(filepath.Join(t.TempDir(), "auth.json"))
	if err != nil {
		t.Fatal(err)
	}
	const secret = "SYNTHETIC_76_TELEMETRY_SECRET"
	credential76Set(t, store, "deepseek", secret)
	models := ai.BuiltinModels(ai.CreateModelsOptions{Credentials: store})
	model, ok := models.GetModel("deepseek", "deepseek-v4-flash")
	if !ok {
		t.Fatal("missing model")
	}
	recorder := &telemetry.InMemoryTelemetryContext{}
	called := false
	options := ai.ModelsSimpleStreamOptions{SimpleStreamOptions: ai.SimpleStreamOptions{StreamOptions: ai.StreamOptions{ProviderRequestOptions: ai.ProviderRequestOptions{TelemetryContext: recorder, Fetch: func(_ context.Context, request ai.FetchRequest) (ai.FetchResponse, error) {
		called = true
		if request.Headers["Authorization"] != "Bearer "+secret {
			t.Fatal("saved key did not reach transport")
		}
		if strings.Contains(string(request.Body), secret) {
			t.Fatal("credential leaked into request body")
		}
		return ai.FetchResponse{Status: 401, Body: []byte("invalid key " + secret)}, nil
	}}}}}
	result, err := models.CompleteSimple(context.Background(), model, ai.Context{}, options)
	if !called {
		t.Fatalf("request never reached transport: %v", err)
	}
	data, _ := json.Marshal(struct {
		Message ai.AssistantMessage
		Spans   []telemetry.RecordedTelemetrySpan
	}{result, recorder.GetSpans()})
	if strings.Contains(string(data), secret) || err != nil && strings.Contains(err.Error(), secret) {
		t.Fatal("credential leaked into error or telemetry")
	}
	if result.StopReason != ai.StopReasonError {
		t.Fatal("authentication failure was hidden")
	}
}

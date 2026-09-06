package codingagent_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
)

func TestModelRuntime77SessionServices(t *testing.T) {
	t.Setenv("DEEPSEEK_API_KEY", "")
	ctx := context.Background()
	store := ai.NewInMemoryCredentialStore()
	_, err := store.Modify(ctx, "deepseek", func(context.Context, ai.Credential) (ai.Credential, error) {
		return ai.APIKeyCredential{Type: ai.AuthTypeAPIKey, Key: ai.Some("fixture")}, nil
	}, ai.AuthOperationOptions{})
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := codingagent.NewModelRuntime(ctx, codingagent.CreateModelRuntimeOptions{Credentials: store})
	if err != nil {
		t.Fatal(err)
	}
	provider, id := "deepseek", "deepseek-v4-flash"
	settings, err := codingagent.NewInMemorySettingsManager(codingagent.Settings{DefaultProvider: &provider, DefaultModel: &id})
	if err != nil {
		t.Fatal(err)
	}
	services, err := codingagent.CreateAgentSessionServices(ctx, codingagent.CreateAgentSessionServicesOptions{CWD: t.TempDir(), SettingsManager: settings, ModelRuntime: runtime})
	if err != nil {
		t.Fatal(err)
	}
	created, err := codingagent.CreateAgentSessionFromServices(ctx, codingagent.CreateAgentSessionFromServicesOptions{Services: services, SessionManager: codingagent.NewInMemorySessionManager(""), NoTools: codingagent.NoToolsAll})
	if err != nil {
		t.Fatal(err)
	}
	defer created.Session.Dispose()
	if created.Session.ModelRuntime() != runtime || created.Session.Agent().State().Model.ID != id {
		t.Fatal("services did not own selection and inference runtime")
	}
}

func TestModelRuntime77SDKAndHeadlessReadRequests(t *testing.T) {
	t.Setenv("DEEPSEEK_API_KEY", "")
	ctx := context.Background()
	cwd := t.TempDir()
	if err := os.WriteFile(filepath.Join(cwd, "note.txt"), []byte("runtime-note"), 0600); err != nil {
		t.Fatal(err)
	}
	requests := make(chan map[string]any, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		requests <- body
		if r.Header.Get("Authorization") != "Bearer fixture" {
			t.Error("runtime credentials missing")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		messages := body["messages"].([]any)
		if messages[len(messages)-1].(map[string]any)["role"] == "tool" {
			if messages[len(messages)-1].(map[string]any)["content"] != "runtime-note" {
				t.Error("read result lost")
			}
			fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"done\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
			return
		}
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"read-77\",\"type\":\"function\",\"function\":{\"name\":\"read\",\"arguments\":\"{\\\"path\\\":\\\"note.txt\\\"}\"}}]},\"finish_reason\":\"tool_calls\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	store := ai.NewInMemoryCredentialStore()
	_, err := store.Modify(ctx, "deepseek", func(context.Context, ai.Credential) (ai.Credential, error) {
		return ai.APIKeyCredential{Type: ai.AuthTypeAPIKey, Key: ai.Some("fixture")}, nil
	}, ai.AuthOperationOptions{})
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := codingagent.NewModelRuntime(ctx, codingagent.CreateModelRuntimeOptions{Credentials: store})
	if err != nil {
		t.Fatal(err)
	}
	model, ok, err := runtime.GetModel("deepseek", "deepseek-v4-flash")
	if err != nil || !ok {
		t.Fatal(err)
	}
	model.BaseURL = server.URL
	settings, err := codingagent.NewInMemorySettingsManager(codingagent.Settings{})
	if err != nil {
		t.Fatal(err)
	}
	sdk, err := codingagent.CreateAgentSession(ctx, codingagent.CreateAgentSessionOptions{CWD: cwd, AgentDir: cwd, Model: &model, ModelRuntime: runtime, SettingsManager: settings, SessionManager: codingagent.NewInMemorySessionManager(cwd)})
	if err != nil {
		t.Fatal(err)
	}
	defer sdk.Session.Dispose()
	if err = sdk.Session.Prompt(ctx, "read note.txt"); err != nil {
		t.Fatal(err)
	}
	headless, err := codingagent.CreateHeadlessSession(ctx, codingagent.CreateHeadlessSessionOptions{CWD: cwd, AgentDir: cwd, ModelRuntime: runtime, Provider: model.Provider, Model: model.ID, BaseURL: &server.URL, SettingsManager: settings, SessionManager: codingagent.NewInMemorySessionManager(cwd)})
	if err != nil {
		t.Fatal(err)
	}
	defer headless.Dispose(ctx)
	out, err := codingagent.RunHeadless(ctx, headless, codingagent.HeadlessRunOptions{Messages: []string{"read note.txt"}})
	if err != nil || strings.Join(out.Text, "") != "done" {
		t.Fatalf("%+v %v", out, err)
	}
	a, b, c, d := <-requests, <-requests, <-requests, <-requests
	if !reflect.DeepEqual(a, c) || !reflect.DeepEqual(b, d) {
		t.Fatalf("SDK and Headless request drift: SDK=%v/%v Headless=%v/%v", a, b, c, d)
	}
}

func TestModelRuntime77OfflineAndOutcomes(t *testing.T) {
	t.Setenv("DEEPSEEK_API_KEY", "")
	for _, value := range []string{"", "0", "false", "1", "TRUE", "yes"} {
		t.Setenv("PIG_OFFLINE", value)
		want := value == "1" || value == "TRUE" || value == "yes"
		if codingagent.ResolveOffline(false) != want || !codingagent.ResolveOffline(true) {
			t.Fatalf("offline %q", value)
		}
	}
	t.Setenv("PIG_OFFLINE", "1")
	store := ai.NewInMemoryCredentialStore()
	runtime, err := codingagent.NewModelRuntime(context.Background(), codingagent.CreateModelRuntimeOptions{Credentials: store, AllowModelNetwork: true})
	if err != nil {
		t.Fatal(err)
	}
	model, _, _ := runtime.GetModel("deepseek", "deepseek-v4-flash")
	calls := 0
	key := "runtime-secret"
	options := ai.ModelsSimpleStreamOptions{SimpleStreamOptions: ai.SimpleStreamOptions{StreamOptions: ai.StreamOptions{ProviderRequestOptions: ai.ProviderRequestOptions{APIKey: &key, Fetch: func(ctx context.Context, r ai.FetchRequest) (ai.FetchResponse, error) {
		calls++
		return ai.FetchResponse{Status: 401, Body: []byte(key)}, nil
	}}}}}
	out, err := runtime.CompleteSimple(context.Background(), model, ai.Context{}, options)
	if err != nil || out.StopReason != ai.StopReasonError || calls != 1 {
		t.Fatalf("provider error: %+v %v calls=%d", out, err, calls)
	}
	encoded, _ := json.Marshal(out)
	if strings.Contains(string(encoded), key) {
		t.Fatal("error leaked key")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	out, err = runtime.CompleteSimple(ctx, model, ai.Context{}, options)
	if err != nil || out.StopReason != ai.StopReasonAborted || calls != 1 {
		t.Fatalf("cancellation: %+v %v", out, err)
	}
	out, err = runtime.CompleteSimple(context.Background(), model, ai.Context{})
	if err != nil || out.StopReason != ai.StopReasonError {
		t.Fatalf("no auth: %+v %v", out, err)
	}
	others, _ := runtime.GetModels("anthropic")
	other := others[0]
	_, err = runtime.CompleteSimple(context.Background(), other, ai.Context{}, options)
	if !errors.Is(err, ai.ErrNotImplemented) {
		t.Fatalf("unsupported adapter: %v", err)
	}
}

func TestModelRuntime77SnapshotAndRestoration(t *testing.T) {
	t.Setenv("DEEPSEEK_API_KEY", "")
	ctx := context.Background()
	store := ai.NewInMemoryCredentialStore()
	_, err := store.Modify(ctx, "deepseek", func(context.Context, ai.Credential) (ai.Credential, error) {
		return ai.APIKeyCredential{Type: ai.AuthTypeAPIKey, Key: ai.Some("fixture")}, nil
	}, ai.AuthOperationOptions{})
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := codingagent.NewModelRuntime(ctx, codingagent.CreateModelRuntimeOptions{Credentials: store})
	if err != nil {
		t.Fatal(err)
	}
	registry := codingagent.NewModelRegistry(runtime)
	models, err := registry.GetAvailable()
	if err != nil || len(models) != 2 {
		t.Fatalf("%v %v", models, err)
	}
	models[0].ID = "mutated"
	again, _ := registry.GetAvailable()
	if again[0].ID == "mutated" {
		t.Fatal("snapshot aliased")
	}
	manager := codingagent.NewInMemorySessionManager("")
	manager.AppendModelChange("missing", "removed")
	manager.AppendThinkingLevelChange("high")
	msg, _ := ai.FauxAssistantMessage(ai.FauxAssistantText("saved"))
	msg.Provider = "missing"
	msg.Model = "removed"
	manager.AppendMessage(msg)
	settings, _ := codingagent.NewInMemorySettingsManager(codingagent.Settings{})
	created, err := codingagent.CreateAgentSession(ctx, codingagent.CreateAgentSessionOptions{CWD: t.TempDir(), AgentDir: t.TempDir(), ModelRuntime: runtime, SettingsManager: settings, SessionManager: manager, NoTools: codingagent.NoToolsAll})
	if err != nil {
		t.Fatal(err)
	}
	defer created.Session.Dispose()
	if created.ModelFallbackMessage == nil || *created.ModelFallbackMessage != "Could not restore model missing/removed. Using deepseek/deepseek-v4-pro" || created.Session.ThinkingLevel() != "high" {
		t.Fatalf("fallback: %s thinking=%s model=%s", *created.ModelFallbackMessage, created.Session.ThinkingLevel(), created.Session.Model().ID)
	}
	headless, err := codingagent.CreateHeadlessSession(ctx, codingagent.CreateHeadlessSessionOptions{CWD: t.TempDir(), AgentDir: t.TempDir(), ModelRuntime: runtime, SettingsManager: settings, SessionManager: manager, NoTools: codingagent.NoToolsAll})
	if err != nil {
		t.Fatal(err)
	}
	defer headless.Dispose(ctx)
	if !reflect.DeepEqual(headless.ModelFallbackMessage(), created.ModelFallbackMessage) {
		t.Fatalf("SDK/Headless fallback differs: %v %v", headless.ModelFallbackMessage(), created.ModelFallbackMessage)
	}
	if err := store.Delete(ctx, "deepseek", ai.AuthOperationOptions{}); err != nil {
		t.Fatal(err)
	}
	if err = runtime.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	models, _ = registry.GetAvailable()
	if len(models) != 0 {
		t.Fatal("deleted credentials left available models")
	}
}

func TestModelRuntime77EmbeddedSnapshotAndNoControlPlane(t *testing.T) {
	t.Setenv("DEEPSEEK_API_KEY", "")
	expected, err := os.ReadFile("../parity/baseline/catalog/chat/models.json")
	if err != nil {
		t.Fatal(err)
	}
	actual, err := os.ReadFile("data/models.json")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(expected, actual) {
		t.Fatal("embedded catalog drifted from fixed snapshot")
	}
	original := http.DefaultTransport
	defer func() { http.DefaultTransport = original }()
	calls := 0
	http.DefaultTransport = runtime77Transport(func(*http.Request) (*http.Response, error) {
		calls++
		return nil, errors.New("unexpected control-plane network")
	})
	for _, offline := range []bool{false, true} {
		runtime, err := codingagent.NewModelRuntime(context.Background(), codingagent.CreateModelRuntimeOptions{Credentials: ai.NewInMemoryCredentialStore(), Offline: offline})
		if err != nil {
			t.Fatal(err)
		}
		models, err := runtime.GetModels()
		if err != nil || len(models) != 1220 {
			t.Fatalf("snapshot: %d %v", len(models), err)
		}
		available, err := runtime.GetAvailable(context.Background())
		if err != nil || len(available) != 0 {
			t.Fatalf("availability: %d %v", len(available), err)
		}
		if err = runtime.Refresh(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 0 {
		t.Fatalf("startup/refresh made %d control-plane requests", calls)
	}
}

type runtime77Transport func(*http.Request) (*http.Response, error)

func (f runtime77Transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestModelRuntime77CorruptCredentialsRemainDiagnostic(t *testing.T) {
	t.Setenv("DEEPSEEK_API_KEY", "")
	dir := t.TempDir()
	auth := filepath.Join(dir, "auth.json")
	if err := os.WriteFile(auth, []byte("{invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	runtime, err := codingagent.NewModelRuntime(context.Background(), codingagent.CreateModelRuntimeOptions{AuthPath: auth})
	if err != nil {
		t.Fatal(err)
	}
	diagnostic, err := runtime.GetError()
	if err != nil || !strings.Contains(diagnostic, "invalid") {
		t.Fatalf("lost credential failure: %q %v", diagnostic, err)
	}
	settings, _ := codingagent.NewInMemorySettingsManager(codingagent.Settings{})
	_, err = codingagent.CreateAgentSession(context.Background(), codingagent.CreateAgentSessionOptions{CWD: dir, AgentDir: dir, ModelRuntime: runtime, SettingsManager: settings, SessionManager: codingagent.NewInMemorySessionManager(dir)})
	if err == nil || !strings.Contains(err.Error(), "invalid") {
		t.Fatalf("SDK hid corrupted credentials: %v", err)
	}
}

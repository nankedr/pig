package codingagent_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
)

func TestResponsesConfiguredModelsExcludeUnsupportedProviders(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "auth.json"), []byte(`{"deepseek":{"type":"api_key","key":"offline"},"openai":{"type":"api_key","key":"offline"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	store, err := codingagent.NewAuthStorage(filepath.Join(dir, "auth.json"))
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := codingagent.NewModelRuntime(context.Background(), codingagent.CreateModelRuntimeOptions{Credentials: store, Offline: true})
	if err != nil {
		t.Fatal(err)
	}
	models, err := runtime.GetAvailableSnapshot()
	if err != nil || len(models) == 0 {
		t.Fatalf("models: %v %v", models, err)
	}
	for _, model := range models {
		if model.Provider != ai.ProviderIDDeepSeek {
			t.Errorf("unsupported provider advertised as runnable: %s", model.Provider)
		}
	}
}

func TestResponsesHeadlessRestoresAPIAndModelSelection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" {
			t.Errorf("restored protocol path = %s", r.URL.Path)
			http.Error(w, "wrong protocol", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte(responseFrame132(map[string]any{"type": "response.output_text.delta", "delta": "done"}) + responseFrame132(map[string]any{"type": "response.completed", "response": map[string]any{"status": "completed"}})))
	}))
	defer server.Close()
	dir, key := t.TempDir(), "offline"
	options := codingagent.CreateHeadlessSessionOptions{CWD: dir, AgentDir: t.TempDir(), API: ai.APIOpenAIResponses, Provider: ai.ProviderIDDeepSeek, Model: "deepseek-v4-pro", APIKey: &key, BaseURL: &server.URL, NoTools: codingagent.NoToolsAll, NoContextFiles: true}
	options.SettingsManager, _ = codingagent.NewInMemorySettingsManager(codingagent.Settings{Compaction: &codingagent.CompactionSettings{KeepRecentTokens: 1, ReserveTokens: 100}})
	runtime, err := codingagent.CreateHeadlessSession(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.Session().Prompt(context.Background(), "first"); err != nil {
		t.Fatal(err)
	}
	path := *runtime.Session().SessionFile()
	runtime.Dispose(context.Background())
	manager, err := codingagent.OpenSessionManager(path, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	options.API, options.Model, options.SessionManager = "", "", manager
	target := manager.GetEntries()[len(manager.GetEntries())-1].ID
	runtime, err = codingagent.CreateHeadlessSession(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Dispose(context.Background())
	if runtime.Session().Model().API != ai.APIOpenAIResponses {
		t.Fatalf("restored API = %s", runtime.Session().Model().API)
	}
	next, _, err := runtime.Session().ModelRuntime().GetModel("deepseek", "deepseek-v4-flash")
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.Session().SetModel(next); err != nil {
		t.Fatal(err)
	}
	if runtime.Session().Model().API != ai.APIOpenAIResponses {
		t.Fatalf("selected API = %s", runtime.Session().Model().API)
	}
	if err := runtime.Session().Prompt(context.Background(), "continue"); err != nil {
		t.Fatal(err)
	}
	navigation, err := runtime.Session().NavigateTree(context.Background(), target, codingagent.NavigateTreeOptions{Summarize: true})
	if err != nil || navigation.SummaryEntry == nil {
		t.Fatalf("branch summary: %+v %v", navigation, err)
	}
	if err := runtime.Session().Prompt(context.Background(), "continue branch"); err != nil {
		t.Fatal(err)
	}
	compaction, err := runtime.Session().Compact(context.Background())
	if err != nil || !strings.Contains(compaction.Summary, "done") {
		t.Fatalf("compact: %+v %v", compaction, err)
	}
	if err := runtime.Session().Prompt(context.Background(), "continue compact"); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(runtime.Session().Messages())
	if err != nil || len(data) == 0 {
		t.Fatalf("history: %s %v", data, err)
	}
}

func TestResponsesSettingsAPIWithExplicitModel(t *testing.T) {
	api, provider, id, key := ai.APIOpenAIResponses, "deepseek", "deepseek-v4-pro", "offline"
	settings, err := codingagent.NewInMemorySettingsManager(codingagent.Settings{DefaultAPI: &api, DefaultProvider: &provider, DefaultModel: &id})
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := codingagent.CreateHeadlessSession(context.Background(), codingagent.CreateHeadlessSessionOptions{CWD: t.TempDir(), AgentDir: t.TempDir(), Model: id, Provider: ai.ProviderIDDeepSeek, APIKey: &key, SettingsManager: settings, NoTools: codingagent.NoToolsAll, NoContextFiles: true, SessionManager: codingagent.NewInMemorySessionManager("")})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Dispose(context.Background())
	if runtime.Session().Model().API != api {
		t.Fatalf("configured API = %s", runtime.Session().Model().API)
	}
}

func TestResponsesSessionAPIPrecedence(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		saved, explicit, want ai.API
		model                 string
	}{
		{"restore with explicit model", ai.APIOpenAIResponses, "", ai.APIOpenAIResponses, "deepseek-v4-pro"},
		{"explicit protocol overrides restore", ai.APIOpenAIResponses, ai.APIOpenAICompletions, ai.APIOpenAICompletions, ""},
		{"old session retains catalog protocol", ai.APIOpenAICompletions, "", ai.APIOpenAICompletions, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, api, provider, id, key := t.TempDir(), ai.APIOpenAIResponses, "deepseek", "deepseek-v4-pro", "offline"
			manager := codingagent.NewInMemorySessionManager(root)
			if _, err := manager.AppendMessage(ai.AssistantMessage{Role: ai.MessageRoleAssistant, API: tc.saved, Provider: ai.ProviderIDDeepSeek, Model: id, Content: []ai.AssistantContent{ai.TextContent{Type: ai.ContentTypeText, Text: "saved"}}, StopReason: ai.StopReasonStop}); err != nil {
				t.Fatal(err)
			}
			settings, err := codingagent.NewInMemorySettingsManager(codingagent.Settings{DefaultAPI: &api, DefaultProvider: &provider, DefaultModel: &id})
			if err != nil {
				t.Fatal(err)
			}
			runtime, err := codingagent.CreateHeadlessSession(context.Background(), codingagent.CreateHeadlessSessionOptions{CWD: root, AgentDir: t.TempDir(), API: tc.explicit, Model: tc.model, Provider: ai.ProviderIDDeepSeek, APIKey: &key, SettingsManager: settings, SessionManager: manager, NoTools: codingagent.NoToolsAll, NoContextFiles: true})
			if err != nil {
				t.Fatal(err)
			}
			defer runtime.Dispose(context.Background())
			if runtime.Session().Model().API != tc.want {
				t.Fatalf("API=%s want=%s", runtime.Session().Model().API, tc.want)
			}
		})
	}
}

func TestResponsesProductionSDKModelSelection(t *testing.T) {
	models, _, store := config87Runtime(t)
	credential76Set(t, store, "deepseek", "offline")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" {
			t.Errorf("SDK selected path: %s", r.URL.Path)
			http.Error(w, "wrong API", 400)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte(responseFrame132(map[string]any{"type": "response.output_text.delta", "delta": "SDK_DONE"}) + responseFrame132(map[string]any{"type": "response.completed", "response": map[string]any{"status": "completed"}})))
	}))
	defer server.Close()
	api, provider, id := ai.APIOpenAIResponses, "deepseek", "deepseek-v4-pro"
	settings, err := codingagent.NewInMemorySettingsManager(codingagent.Settings{DefaultAPI: &api, DefaultProvider: &provider, DefaultModel: &id})
	if err != nil {
		t.Fatal(err)
	}
	created, err := codingagent.CreateAgentSession(context.Background(), codingagent.CreateAgentSessionOptions{CWD: t.TempDir(), AgentDir: t.TempDir(), ModelRuntime: models, SettingsManager: settings, NoTools: codingagent.NoToolsAll, NoContextFiles: true})
	if err != nil {
		t.Fatal(err)
	}
	defer created.Session.Dispose()
	if created.Session.Model().API != api {
		t.Fatal("SDK settings API lost")
	}
	next, _, err := models.GetModel(provider, "deepseek-v4-flash")
	if err != nil {
		t.Fatal(err)
	}
	next.BaseURL = server.URL
	if err := created.Session.SetModel(next); err != nil {
		t.Fatal(err)
	}
	if created.Session.Model().API != api {
		t.Fatalf("SDK selected API: %s", created.Session.Model().API)
	}
	if err := created.Session.Prompt(context.Background(), "SDK continuation"); err != nil {
		t.Fatal(err)
	}
	last := created.Session.Messages()[len(created.Session.Messages())-1].(ai.AssistantMessage)
	if last.API != api || last.StopReason != ai.StopReasonStop || last.Content[0].(ai.TextContent).Text != "SDK_DONE" {
		t.Fatalf("SDK outcome: %+v", last)
	}
	cycled, err := created.Session.CycleModel(context.Background())
	if err != nil || cycled == nil || cycled.Model.API != api {
		t.Fatalf("SDK cycle: %+v %v", cycled, err)
	}
	unchanged, _, err := models.GetModel(provider, "deepseek-v4-flash")
	if err != nil || unchanged.API != ai.APIOpenAICompletions {
		t.Fatal("SDK changed shared Catalog")
	}
}

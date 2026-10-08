package codingagent_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
)

func imageFile134(t *testing.T) (string, ai.ImageContent) {
	t.Helper()
	data, err := os.ReadFile("../parity/services/user-image.png")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "misnamed.jpg")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	image, err := ai.LoadImageFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if image.MIMEType != "image/png" || image.Data != base64.StdEncoding.EncodeToString(data) {
		t.Fatal("loader changed actual format/bytes")
	}
	return path, image
}

func TestUserImagesProductionRestoreAndFork(t *testing.T) {
	path, image := imageFile134(t)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		raw, _ := json.Marshal(body["input"])
		if !strings.Contains(string(raw), "data:image/png;base64,"+image.Data) || body["previous_response_id"] != nil {
			t.Errorf("image absent from local replay")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, `data: {"type":"response.output_text.delta","delta":"seen"}`+"\n\n"+`data: {"type":"response.completed","response":{"status":"completed"}}`+"\n\n")
	}))
	defer server.Close()
	key := "offline"
	options := codingagent.CreateHeadlessSessionOptions{CWD: t.TempDir(), AgentDir: t.TempDir(), Provider: ai.ProviderIDDeepSeek, Model: "deepseek-flash", API: ai.APIOpenAIResponses, APIKey: &key, BaseURL: &server.URL, NoTools: codingagent.NoToolsAll, NoContextFiles: true}
	runtime, err := codingagent.CreateHeadlessSession(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Dispose(context.Background())
	if err := runtime.Session().Prompt(context.Background(), "describe", codingagent.PromptOptions{Images: []ai.ImageContent{image}}); err != nil {
		t.Fatal(err)
	}
	saved := *runtime.Session().SessionFile()
	target := runtime.Session().SessionManager().GetEntries()[len(runtime.Session().SessionManager().GetEntries())-1].ID
	runtime.Dispose(context.Background())
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	manager, err := codingagent.OpenSessionManager(saved, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	options.SessionManager, options.API, options.Model = manager, "", ""
	runtime, err = codingagent.CreateHeadlessSession(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Dispose(context.Background())
	if err := runtime.Session().Prompt(context.Background(), "continue same image"); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Session().NavigateTree(context.Background(), target); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Session().Prompt(context.Background(), "continue selected image branch"); err != nil {
		t.Fatal(err)
	}
	source, _ := os.ReadFile(saved)
	fork, err := codingagent.ForkSessionManager(saved, options.CWD, nil)
	if err != nil {
		t.Fatal(err)
	}
	options.SessionManager = fork
	branched, err := codingagent.CreateHeadlessSession(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	defer branched.Dispose(context.Background())
	if err := branched.Session().Prompt(context.Background(), "continue fork"); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(saved)
	if string(source) != string(after) || calls.Load() != 4 {
		t.Fatalf("fork source mutated/calls=%d", calls.Load())
	}
	for _, bad := range []string{"", "%%%%", "aGk="} {
		damaged := strings.Replace(string(source), image.Data, bad, 1)
		file := filepath.Join(t.TempDir(), "corrupt.jsonl")
		os.WriteFile(file, []byte(damaged), 0600)
		if _, err := codingagent.OpenSessionManager(file, nil, nil); err == nil || !strings.Contains(err.Error(), "image") {
			t.Fatalf("corrupt inline image silently dropped/accepted: %q %v", bad, err)
		}
	}
}

func TestUserImagesPromptRejectionDoesNotPersist(t *testing.T) {
	path, block := imageFile134(t)
	_ = path
	key := "offline"
	runtime, err := codingagent.CreateHeadlessSession(context.Background(), codingagent.CreateHeadlessSessionOptions{CWD: t.TempDir(), AgentDir: t.TempDir(), Model: "deepseek-flash", API: ai.APIOpenAIResponses, APIKey: &key, NoTools: codingagent.NoToolsAll, NoContextFiles: true})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Dispose(context.Background())
	before, _ := json.Marshal(runtime.Session().SessionManager().GetEntries())
	for _, images := range [][]ai.ImageContent{{{Type: ai.ContentTypeImage, Data: "aGk=", MIMEType: "image/png"}}, make([]ai.ImageContent, 65)} {
		if err := runtime.Session().Prompt(context.Background(), "invalid", codingagent.PromptOptions{Images: images}); err == nil {
			t.Fatal("invalid attachment accepted")
		}
	}
	after, _ := json.Marshal(runtime.Session().SessionManager().GetEntries())
	if string(before) != string(after) || !runtime.Session().IsIdle() {
		t.Fatal("invalid attachment had persistence effects")
	}
	model := runtime.Session().Model()
	model.Input = []ai.ModelInput{ai.ModelInputText}
	if err := runtime.Session().SetModel(model); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Session().Prompt(context.Background(), "nonvision", codingagent.PromptOptions{Images: []ai.ImageContent{block}}); err == nil {
		t.Fatal("nonvision accepted")
	}
}

func TestUserImagesConcurrentSessions(t *testing.T) {
	_, block := imageFile134(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		data, _ := json.Marshal(body["input"])
		if !strings.Contains(string(data), block.Data) {
			t.Error("concurrent image request lost bytes")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, `data: {"type":"response.completed","response":{"status":"completed"}}`+"\n\n")
	}))
	defer server.Close()
	key := "offline"
	var wg sync.WaitGroup
	for range 3 {
		runtime, err := codingagent.CreateHeadlessSession(context.Background(), codingagent.CreateHeadlessSessionOptions{CWD: t.TempDir(), AgentDir: t.TempDir(), Model: "deepseek-flash", API: ai.APIOpenAIResponses, APIKey: &key, BaseURL: &server.URL, NoTools: codingagent.NoToolsAll, NoContextFiles: true})
		if err != nil {
			t.Fatal(err)
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer runtime.Dispose(context.Background())
			if err := runtime.Session().Prompt(context.Background(), "concurrent picture", codingagent.PromptOptions{Images: []ai.ImageContent{block}}); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
}

func TestUserImagesModelRuntimeConfigurationIsolation(t *testing.T) {
	fixed, err := codingagent.NewModelRuntime(context.Background(), codingagent.CreateModelRuntimeOptions{Offline: true, Credentials: ai.NewInMemoryCredentialStore()})
	if err != nil {
		t.Fatal(err)
	}
	configured, err := codingagent.NewModelRuntime(context.Background(), codingagent.CreateModelRuntimeOptions{DeepSeekVision: true, Offline: true, Credentials: ai.NewInMemoryCredentialStore()})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := fixed.GetModel("deepseek", "deepseek-flash"); err != nil || ok {
		t.Fatal("current service model polluted fixed default")
	}
	model, ok, err := configured.GetModel("deepseek", "deepseek-flash")
	if err != nil || !ok || len(model.Input) != 2 || model.Input[1] != ai.ModelInputImage {
		t.Fatal("explicit current model unavailable")
	}
	model.Input[1] = ai.ModelInputText
	again, ok, err := configured.GetModel("deepseek", "deepseek-flash")
	if err != nil || !ok || again.Input[1] != ai.ModelInputImage {
		t.Fatal("vision configuration ownership leaked")
	}
}

func TestUserImagesRestoreValidationUsesSelectedPath(t *testing.T) {
	_, image := imageFile134(t)
	for _, test := range []struct {
		name, content, suffix string
		wantError             bool
	}{
		{"bad-before-flash", `[{"type":"image","data":"aGk=","mimeType":"image/png"}]`, `{"type":"model_change","id":"flash","parentId":"image","provider":"deepseek","modelId":"deepseek-flash"}`, true},
		{"object-container", `{"type":"image","data":"` + image.Data + `","mimeType":"image/png"}`, ``, true},
		{"bad-discriminator", `[{"type":"imag","data":"` + image.Data + `","mimeType":"image/png"}]`, ``, true},
		{"bad-message", `[{"type":"image","data":"` + image.Data + `","mimeType":"image/png"}]`, ``, true},
		{"bad-neighbor", `[{"type":"image","data":"` + image.Data + `","mimeType":"image/png"},{"type":"text","text":5}]`, ``, true},
		{"inactive-flash", `[{"type":"image","data":"aW1hZ2U=","mimeType":"image/png"}]`, `{"type":"model_change","id":"text","parentId":"image","provider":"deepseek","modelId":"deepseek-v4-flash"}`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "session.jsonl")
			data := `{"type":"session","version":3,"id":"images","timestamp":"2026-10-08T00:00:00Z"}` + "\n" +
				`{"type":"message","id":"image","parentId":null,"message":{"role":"user","content":` + test.content + `,"timestamp":1}}` + "\n" + test.suffix + "\n"
			if test.name == "bad-message" {
				data = strings.Replace(data, `"timestamp":1`, `"timestamp":"bad"`, 1)
			}
			if test.name == "inactive-flash" {
				data = strings.Replace(data, `{"type":"message"`, `{"type":"model_change","id":"flash","parentId":null,"provider":"deepseek","modelId":"deepseek-flash"}`+"\n"+`{"type":"message"`, 1)
			}
			if err := os.WriteFile(file, []byte(data), 0600); err != nil {
				t.Fatal(err)
			}
			manager, err := codingagent.OpenSessionManager(file, nil, nil)
			if test.wantError {
				if err == nil || !strings.Contains(err.Error(), "image") {
					t.Fatalf("damaged image silently accepted: %v", err)
				}
			} else if err != nil || len(manager.BuildSessionContext().Messages) != 1 {
				t.Fatalf("inactive branch polluted codec restore: %v", err)
			}
		})
	}
}

func TestUserImagesHistoryQuotaRejectionDoesNotPersist(t *testing.T) {
	_, image := imageFile134(t)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, `data: {"type":"response.completed","response":{"status":"completed"}}`+"\n\n")
	}))
	defer server.Close()
	key := "offline"
	runtime, err := codingagent.CreateHeadlessSession(context.Background(), codingagent.CreateHeadlessSessionOptions{CWD: t.TempDir(), AgentDir: t.TempDir(), Model: "deepseek-flash", API: ai.APIOpenAIResponses, APIKey: &key, BaseURL: &server.URL, NoTools: codingagent.NoToolsAll, NoContextFiles: true})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Dispose(context.Background())
	images := make([]ai.ImageContent, 64)
	for i := range images {
		images[i] = image
	}
	if err := runtime.Session().Prompt(context.Background(), "full", codingagent.PromptOptions{Images: images}); err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(runtime.Session().SessionManager().GetEntries())
	if err := runtime.Session().Prompt(context.Background(), "over", codingagent.PromptOptions{Images: []ai.ImageContent{image}}); err == nil {
		t.Fatal("history quota accepted")
	}
	after, _ := json.Marshal(runtime.Session().SessionManager().GetEntries())
	if string(before) != string(after) || calls.Load() != 1 {
		t.Fatal("over-quota prompt persisted or reached transport")
	}
}

func TestUserImagesHeadlessThinkingSelector(t *testing.T) {
	for _, selector := range []string{"deepseek-flash:high", "deepseek/deepseek-flash:high"} {
		key := "offline"
		runtime, err := codingagent.CreateHeadlessSession(context.Background(), codingagent.CreateHeadlessSessionOptions{CWD: t.TempDir(), AgentDir: t.TempDir(), Model: selector, APIKey: &key, NoTools: codingagent.NoToolsAll, NoContextFiles: true})
		if err != nil {
			t.Fatal(err)
		}
		if model := runtime.Session().Model(); model.ID != "deepseek-flash" || len(model.Input) != 2 {
			t.Fatal("vision selector lost current model")
		}
		runtime.Dispose(context.Background())
	}
}

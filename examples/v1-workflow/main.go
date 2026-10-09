package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"

	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	dir, err := os.MkdirTemp("", "pig-responses-session-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	if err := os.WriteFile(filepath.Join(dir, "sentinel.txt"), []byte("local value"), 0600); err != nil {
		return err
	}
	var picture bytes.Buffer
	if err := png.Encode(&picture, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		return err
	}
	imagePath := filepath.Join(dir, "image.png")
	if err := os.WriteFile(imagePath, picture.Bytes(), 0600); err != nil {
		return err
	}
	block, err := ai.LoadImageFile(imagePath)
	if err != nil {
		return err
	}
	var requests atomic.Int32
	var imageRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			panic(err)
		}
		if bytes.Contains(body, []byte("data:image/png;base64,"+block.Data)) {
			imageRequests.Add(1)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		if requests.Add(1) == 1 {
			item := map[string]any{"type": "function_call", "id": "fc-read", "call_id": "read-local", "name": "read", "arguments": `{"path":"sentinel.txt"}`}
			data, _ := json.Marshal(map[string]any{"type": "response.output_item.done", "item": item})
			fmt.Fprintf(w, "data: %s\n\n", data)
		} else {
			fmt.Fprint(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"local checkpoint\"}\n\n")
		}
		fmt.Fprint(w, "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\n\n")
	}))
	defer server.Close()
	key := "offline-fixture"
	settings, err := codingagent.NewInMemorySettingsManager(codingagent.Settings{Compaction: &codingagent.CompactionSettings{ReserveTokens: 100, KeepRecentTokens: 1}})
	if err != nil {
		return err
	}
	options := codingagent.CreateHeadlessSessionOptions{CWD: dir, AgentDir: dir, Provider: ai.ProviderIDDeepSeek, Model: "deepseek-flash", API: ai.APIOpenAIResponses, APIKey: &key, BaseURL: &server.URL, Tools: []string{"read"}, SettingsManager: settings, NoContextFiles: true}
	ctx := context.Background()
	runtime, err := codingagent.CreateHeadlessSession(ctx, options)
	if err != nil {
		return err
	}
	if err := runtime.Session().Prompt(ctx, "read sentinel.txt", codingagent.PromptOptions{Images: []ai.ImageContent{block}}); err != nil {
		runtime.Dispose(ctx)
		return err
	}
	path := *runtime.Session().SessionFile()
	if err := runtime.Dispose(ctx); err != nil {
		return err
	}
	if err := os.Remove(imagePath); err != nil {
		return err
	}
	manager, err := codingagent.OpenSessionManager(path, nil, nil)
	if err != nil {
		return err
	}
	options.API, options.Model, options.SessionManager = "", "", manager
	runtime, err = codingagent.CreateHeadlessSession(ctx, options)
	if err != nil {
		return err
	}
	defer runtime.Dispose(ctx)
	if runtime.Session().Model().API != ai.APIOpenAIResponses {
		return fmt.Errorf("restored API changed")
	}
	if err := runtime.Session().Prompt(ctx, "continue"); err != nil {
		return err
	}
	if imageRequests.Load() < 3 {
		return fmt.Errorf("image lost during tool continuation or restore")
	}
	if _, err := runtime.Session().ExportToHTML(ctx, filepath.Join(dir, "images.html")); err != nil {
		return err
	}
	if _, err := runtime.Session().Compact(ctx); err != nil {
		return err
	}
	if err := runtime.Session().Prompt(ctx, "continue after compaction"); err != nil {
		return err
	}
	stats, err := runtime.Session().GetSessionStats()
	if err != nil {
		return err
	}
	if stats.ToolCalls != 1 || stats.ToolResults != 1 {
		return fmt.Errorf("tool continuation repeated or missing")
	}
	fmt.Printf("API=%s tools=%d/%d requests=%d restored and compacted v3 session\n", runtime.Session().Model().API, stats.ToolCalls, stats.ToolResults, requests.Load())
	return nil
}

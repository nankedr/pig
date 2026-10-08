package codingagent_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
)

func toolImageReply135(w http.ResponseWriter, api ai.API, id, name string, args map[string]any) {
	w.Header().Set("Content-Type", "text/event-stream")
	if api == ai.APIOpenAIResponses {
		if name != "" {
			encoded, _ := json.Marshal(args)
			item := map[string]any{"type": "function_call", "id": "fc-" + id, "call_id": id, "name": name, "arguments": string(encoded)}
			fmt.Fprint(w, responseFrame132(map[string]any{"type": "response.output_item.added", "output_index": 0, "item": item})+responseFrame132(map[string]any{"type": "response.output_item.done", "output_index": 0, "item": item}))
		} else {
			fmt.Fprint(w, responseFrame132(map[string]any{"type": "response.output_text.delta", "delta": "IMAGE_TASK_DONE"}))
		}
		fmt.Fprint(w, responseFrame132(map[string]any{"type": "response.completed", "response": map[string]any{"status": "completed"}}))
		return
	}
	delta := map[string]any{"content": "IMAGE_TASK_DONE"}
	reason := "stop"
	if name != "" {
		encoded, _ := json.Marshal(args)
		delta = map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": id, "type": "function", "function": map[string]any{"name": name, "arguments": string(encoded)}}}}
		reason = "tool_calls"
	}
	fmt.Fprint(w, responseFrame132(map[string]any{"choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": reason}}})+"data: [DONE]\n\n")
}

func TestToolImagesSessionReadWriteRestoreForkAndCompaction(t *testing.T) {
	for _, api := range []ai.API{ai.APIOpenAIResponses, ai.APIOpenAICompletions} {
		t.Run(string(api), func(t *testing.T) {
			path, image := imageFile134(t)
			cwd := t.TempDir()
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
					return
				}
				n := requests.Add(1)
				if n == 1 {
					toolImageReply135(w, api, "read-image", "read", map[string]any{"path": path})
					return
				}
				raw, _ := json.Marshal(body)
				if !strings.Contains(string(raw), "data:image/png;base64,"+image.Data) {
					t.Error("tool image missing from next request")
				}
				if n == 2 {
					os.Remove(path)
					if api == ai.APIOpenAIResponses {
						w.Header().Set("Content-Type", "text/event-stream")
						fmt.Fprint(w, responseFrame132(map[string]any{"type": "response.failed", "response": map[string]any{"status": "failed", "error": map[string]any{"code": "503", "message": "service unavailable"}}}))
					} else {
						http.Error(w, "retry", 503)
					}
					return
				}
				if n == 3 {
					toolImageReply135(w, api, "write-code", "write", map[string]any{"path": "result.txt", "content": "red screenshot understood"})
					return
				}
				toolImageReply135(w, api, "", "", nil)
			}))
			defer server.Close()
			key := "offline"
			enabled, retries, delay := true, 1, int64(1)
			settings, _ := codingagent.NewInMemorySettingsManager(codingagent.Settings{Retry: &codingagent.RetrySettings{Enabled: &enabled, MaxRetries: &retries, BaseDelayMS: &delay}})
			options := codingagent.CreateHeadlessSessionOptions{CWD: cwd, AgentDir: t.TempDir(), SettingsManager: settings, Model: "deepseek-flash", API: api, APIKey: &key, BaseURL: &server.URL, NoContextFiles: true, Thinking: "off"}
			runtime, err := codingagent.CreateHeadlessSession(t.Context(), options)
			if err != nil {
				t.Fatal(err)
			}
			if err := runtime.Session().Prompt(t.Context(), "read screenshot then write result"); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(filepath.Join(cwd, "result.txt"))
			if err != nil || string(data) != "red screenshot understood" {
				t.Fatalf("task=%s %v", data, err)
			}
			saved := *runtime.Session().SessionFile()
			runtime.Dispose(context.Background())
			os.Remove(path)
			manager, err := codingagent.OpenSessionManager(saved, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			var toolEntry string
			for _, entry := range manager.GetEntries() {
				if m, ok := entry.Message.(ai.ToolResultMessage); ok && m.ToolName == "read" {
					toolEntry = entry.ID
					if len(m.Content) != 2 || m.Content[1].(ai.ImageContent) != image {
						t.Fatal("v3 lost tool identity/order")
					}
				}
			}
			if toolEntry == "" {
				t.Fatal("missing read entry")
			}
			options.SessionManager, options.Model, options.API = manager, "", ""
			runtime, err = codingagent.CreateHeadlessSession(t.Context(), options)
			if err != nil {
				t.Fatal(err)
			}
			if err := runtime.Session().Prompt(t.Context(), "same screenshot continue"); err != nil {
				t.Fatal(err)
			}
			runtime.Dispose(context.Background())
			source, _ := os.ReadFile(saved)
			fork, err := codingagent.ForkSessionManager(saved, cwd, nil)
			if err != nil {
				t.Fatal(err)
			}
			options.SessionManager = fork
			runtime, err = codingagent.CreateHeadlessSession(t.Context(), options)
			if err != nil {
				t.Fatal(err)
			}
			if err := runtime.Session().Prompt(t.Context(), "fork same screenshot"); err != nil {
				t.Fatal(err)
			}
			runtime.Dispose(context.Background())
			after, _ := os.ReadFile(saved)
			if string(source) != string(after) {
				t.Fatal("fork changed source")
			}
			for _, bad := range []string{"", "%%%%", "aGk="} {
				file := filepath.Join(t.TempDir(), "bad.jsonl")
				os.WriteFile(file, []byte(strings.Replace(string(source), image.Data, bad, 1)), 0600)
				if _, err := codingagent.OpenSessionManager(file, nil, nil); err == nil || !strings.Contains(err.Error(), "image") {
					t.Fatalf("corrupt tool attachment accepted: %v", err)
				}
			}
			entries := manager.GetBranch()
			firstKept := entries[len(entries)-1].ID
			if _, err := manager.AppendCompaction("screenshot was red; code written", firstKept, 1000); err != nil {
				t.Fatal(err)
			}
			compacted, err := codingagent.OpenSessionManager(saved, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			contextBytes, _ := json.Marshal(compacted.BuildSessionContext().Messages)
			if strings.Contains(string(contextBytes), image.Data) {
				t.Fatal("compacted image unexpectedly resurrected")
			}
			archive, _ := json.Marshal(compacted.GetEntries())
			if !strings.Contains(string(archive), image.Data) {
				t.Fatal("compaction deleted archived image")
			}
		})
	}
}

func TestToolImagesTextOnlyQueueBoundaries(t *testing.T) {
	_, image := imageFile134(t)
	started, release := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, responseFrame132(map[string]any{"type": "response.output_text.delta", "delta": "waiting"}))
		w.(http.Flusher).Flush()
		close(started)
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		toolImageReply135(w, ai.APIOpenAIResponses, "", "", nil)
	}))
	defer server.Close()
	key := "offline"
	runtime, err := codingagent.CreateHeadlessSession(t.Context(), codingagent.CreateHeadlessSessionOptions{CWD: t.TempDir(), AgentDir: t.TempDir(), Model: "deepseek-flash", API: ai.APIOpenAIResponses, APIKey: &key, BaseURL: &server.URL, NoTools: codingagent.NoToolsAll, NoContextFiles: true})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Dispose(context.Background())
	done := make(chan error, 1)
	go func() { done <- runtime.Session().Prompt(t.Context(), "start") }()
	<-started
	for _, delivery := range []string{"steer", "followUp"} {
		if err := runtime.Session().Prompt(t.Context(), "image", codingagent.PromptOptions{Images: []ai.ImageContent{image}, StreamingBehavior: delivery}); err == nil || !strings.Contains(err.Error(), "Queue.Images") {
			t.Fatal("text queue silently accepted image")
		}
		if err := runtime.Session().SendUserMessage(ai.UserBlocks(image), codingagent.SendUserMessageOptions{DeliverAs: codingagent.UserMessageDelivery(delivery)}); err == nil || !strings.Contains(err.Error(), "SendUserMessage.Images") {
			t.Fatal("SendUserMessage silently accepted image")
		}
	}
	if n, _ := runtime.Session().PendingMessageCount(); n != 0 {
		t.Fatal("rejected image changed queue")
	}
	steering, follow, err := runtime.Session().TakeQueuedMessages()
	if err != nil || len(steering)+len(follow) != 0 {
		t.Fatal("image entered text-only take")
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

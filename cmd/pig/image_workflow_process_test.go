package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
)

func TestImageWorkflowRPC136(t *testing.T) {
	binary := binary133(t)
	for _, api := range []ai.API{ai.APIOpenAIResponses, ai.APIOpenAICompletions} {
		t.Run(string(api), func(t *testing.T) {
			root, dir := taskRoot133(t)
			image, err := ai.LoadImageFile("../../parity/services/user-image.png")
			if err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var wire map[string]any
				if err := json.NewDecoder(r.Body).Decode(&wire); err != nil {
					t.Error(err)
					return
				}
				encoded, _ := json.Marshal(wire)
				if !bytes.Contains(encoded, []byte("data:image/png;base64,"+image.Data)) {
					t.Error("missing user image wire")
				}
				w.Header().Set("Content-Type", "text/event-stream")
				if api == ai.APIOpenAIResponses {
					fmt.Fprint(w, frame133(map[string]any{"type": "response.output_text.delta", "delta": "SEEN"})+frame133(map[string]any{"type": "response.completed", "response": map[string]any{"status": "completed"}}))
				} else {
					fmt.Fprint(w, frame133(map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"content": "SEEN"}, "finish_reason": "stop"}}})+"data: [DONE]\n\n")
				}
			}))
			defer server.Close()
			ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
			defer cancel()
			client := codingagent.NewRPCClient(codingagent.RPCClientOptions{CLIPath: &binary, CWD: &root, Args: []string{"--no-context-files", "--no-tools", "--model", "deepseek-flash", "--api", string(api), "--thinking", "off"}, Env: map[string]string{"HOME": root, "PIG_CODING_AGENT_DIR": dir, "PIG_DEEPSEEK_BASE_URL": server.URL}})
			if err := client.Start(ctx); err != nil {
				t.Fatal(err)
			}
			defer client.Stop(context.Background())
			events, err := client.PromptAndWait(ctx, "describe", []ai.ImageContent{image})
			if err != nil {
				t.Fatal(err)
			}
			projected, _ := json.Marshal(events)
			if !bytes.Contains(projected, []byte(image.Data)) {
				t.Fatal("RPC events lost attachment")
			}
			invalid := image
			invalid.Data = "aGk="
			if err := client.Prompt(ctx, "invalid", []ai.ImageContent{invalid}); err == nil {
				t.Fatal("invalid RPC image accepted")
			}
			messages, err := client.GetMessages(ctx)
			if err != nil {
				t.Fatal(err)
			}
			encoded, _ := json.Marshal(messages)
			if !bytes.Contains(encoded, []byte(image.Data)) {
				t.Fatal("RPC history lost image")
			}
			if err := client.Steer(ctx, "queued", []ai.ImageContent{image}); err == nil || !strings.Contains(err.Error(), "not implemented") {
				t.Fatalf("image queue must be explicit: %v", err)
			}
			html := filepath.Join(root, "images.html")
			if _, err := client.ExportHTML(ctx, html); err != nil {
				t.Fatal(err)
			}
		})
	}
}

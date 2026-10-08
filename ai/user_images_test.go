package ai_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nankedr/pig/ai"
)

const image134 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAIAAACQd1PeAAAADElEQVR4nGP4z8AAAAMBAQDJ/pLvAAAAAElFTkSuQmCC"

func TestUserImagesSDKWire(t *testing.T) {
	for _, api := range []ai.API{ai.APIOpenAIResponses, ai.APIOpenAICompletions} {
		t.Run(string(api), func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
					return
				}
				field, kind := "input", "input_image"
				if api == ai.APIOpenAICompletions {
					field, kind = "messages", "image_url"
				}
				parts := body[field].([]any)[0].(map[string]any)["content"].([]any)
				if len(parts) != 3 || parts[1].(map[string]any)["type"] != kind {
					t.Errorf("mixed content: %#v", parts)
					return
				}
				want := "data:image/png;base64," + image134
				url := parts[1].(map[string]any)["image_url"]
				if api == ai.APIOpenAICompletions {
					url = url.(map[string]any)["url"]
				}
				if url != want {
					t.Errorf("wire image: %v", url)
				}
				w.Header().Set("Content-Type", "text/event-stream")
				if api == ai.APIOpenAIResponses {
					fmt.Fprint(w, `data: {"type":"response.completed","response":{"status":"completed"}}`+"\n\n")
				} else {
					fmt.Fprint(w, `data: {"choices":[{"delta":{"content":"seen"},"finish_reason":"stop"}]}`+"\n\ndata: [DONE]\n\n")
				}
			}))
			defer server.Close()
			model := ai.Model{ID: "vision-fixture", Provider: ai.ProviderIDDeepSeek, API: api, BaseURL: server.URL, Input: []ai.ModelInput{ai.ModelInputText, ai.ModelInputImage}, MaxTokens: 512, ContextWindow: 32000}
			image := ai.ImageContent{Type: ai.ContentTypeImage, Data: image134, MIMEType: "image/png"}
			input := ai.Context{Messages: []ai.Message{ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserBlocks(ai.TextContent{Type: ai.ContentTypeText, Text: "before"}, &image, ai.TextContent{Type: ai.ContentTypeText, Text: "after"})}}}
			key := "offline"
			out, err := ai.Complete(context.Background(), model, input, ai.StreamOptions{ProviderRequestOptions: ai.ProviderRequestOptions{APIKey: &key}})
			if err != nil || out.StopReason != ai.StopReasonStop || calls.Load() != 1 {
				t.Fatalf("outcome: %+v %v calls=%d", out, err, calls.Load())
			}
			model.Provider = ai.ProviderIDOpenAI
			if _, err := ai.Complete(context.Background(), model, input, ai.StreamOptions{ProviderRequestOptions: ai.ProviderRequestOptions{APIKey: &key}}); err == nil || calls.Load() != 1 {
				t.Fatal("V2 provider image reached transport")
			}
			model.Provider = ai.ProviderIDDeepSeek
			model.Input = []ai.ModelInput{ai.ModelInputText}
			out, err = ai.Complete(context.Background(), model, input, ai.StreamOptions{ProviderRequestOptions: ai.ProviderRequestOptions{APIKey: &key}})
			if err == nil || calls.Load() != 1 {
				t.Fatalf("nonvision: %+v %v", out, err)
			}
			reason := err.Error()
			if !strings.Contains(reason, "Image") {
				t.Fatalf("unexplained rejection: %s", reason)
			}
		})
	}
}

func TestUserImagesInvalidInputHasNoEffects(t *testing.T) {
	for _, api := range []ai.API{ai.APIOpenAIResponses, ai.APIOpenAICompletions} {
		model := ai.Model{ID: "fixture", API: api, Provider: ai.ProviderIDDeepSeek, Input: []ai.ModelInput{ai.ModelInputImage}, MaxTokens: 512}
		for _, image := range []ai.ImageContent{
			{Type: ai.ContentTypeImage, Data: "%%%%", MIMEType: "image/png"},
			{Type: ai.ContentTypeImage, Data: "aGk=", MIMEType: "image/png"},
			{Type: ai.ContentTypeImage, Data: image134, MIMEType: "image/jpeg"},
			{Type: ai.ContentTypeImage, MIMEType: "image/png"},
		} {
			called := false
			input := ai.Context{Messages: []ai.Message{ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserBlocks(image)}}}
			key := "fixture"
			out, err := ai.Complete(context.Background(), model, input, ai.StreamOptions{ProviderRequestOptions: ai.ProviderRequestOptions{APIKey: &key, OnPayload: func(_ context.Context, p any, _ ai.Model) (ai.PayloadHookResult, error) {
				called = true
				return ai.PayloadHookResult{}, nil
			}}})
			if err == nil && out.StopReason != ai.StopReasonError || called {
				t.Fatalf("invalid image accepted/side effects: %v", err)
			}
		}
	}
}

func TestUserImagesRetryFailureAndCancel(t *testing.T) {
	for _, api := range []ai.API{ai.APIOpenAIResponses, ai.APIOpenAICompletions} {
		for _, mode := range []string{"retry", "failure", "cancel"} {
			t.Run(string(api)+"/"+mode, func(t *testing.T) {
				var calls atomic.Int32
				started := make(chan struct{})
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					call := calls.Add(1)
					var body map[string]any
					json.NewDecoder(r.Body).Decode(&body)
					raw, _ := json.Marshal(body)
					if !strings.Contains(string(raw), "data:image/png;base64,"+image134) {
						t.Error("retry lost image")
					}
					if mode == "retry" && call == 1 {
						w.Header().Set("Retry-After", "0")
						http.Error(w, "retry", 503)
						return
					}
					w.Header().Set("Content-Type", "text/event-stream")
					if api == ai.APIOpenAIResponses {
						fmt.Fprint(w, `data: {"type":"response.output_text.delta","delta":"prefix"}`+"\n\n")
					} else {
						fmt.Fprint(w, `data: {"choices":[{"delta":{"content":"prefix"}}]}`+"\n\n")
					}
					w.(http.Flusher).Flush()
					if mode == "cancel" {
						close(started)
						<-r.Context().Done()
						return
					}
					if mode == "failure" {
						return
					}
					if api == ai.APIOpenAIResponses {
						fmt.Fprint(w, `data: {"type":"response.completed","response":{"status":"completed"}}`+"\n\n")
					} else {
						fmt.Fprint(w, `data: {"choices":[{"delta":{},"finish_reason":"stop"}]}`+"\n\ndata: [DONE]\n\n")
					}
				}))
				defer server.Close()
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				model := ai.Model{ID: "fixture", API: api, Provider: ai.ProviderIDDeepSeek, BaseURL: server.URL, Input: []ai.ModelInput{ai.ModelInputImage}, MaxTokens: 512}
				input := ai.Context{Messages: []ai.Message{ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserBlocks(ai.ImageContent{Type: ai.ContentTypeImage, Data: image134, MIMEType: "image/png"})}}}
				key, retries, delay := "fixture", 1, int64(1)
				stream := ai.Stream(ctx, model, input, ai.StreamOptions{ProviderRequestOptions: ai.ProviderRequestOptions{APIKey: &key, MaxRetries: &retries, MaxRetryDelayMS: &delay}})
				if mode == "cancel" {
					select {
					case <-started:
						cancel()
					case <-time.After(5 * time.Second):
						t.Fatal("image request did not start")
					}
				}
				out, err := stream.Result(context.Background())
				if err != nil && mode != "failure" {
					t.Fatal(err)
				}
				if err != nil && mode == "failure" && api == ai.APIOpenAICompletions {
					var prefix string
					for {
						event, ok, _ := stream.Next(context.Background())
						if !ok {
							break
						}
						if delta, ok := event.(ai.AssistantMessageTextDeltaEvent); ok {
							prefix += delta.Delta
						}
					}
					if prefix != "prefix" || calls.Load() != 1 {
						t.Fatalf("partial image stream lost/no retry: %q %v", prefix, err)
					}
					return
				}
				want, count := ai.StopReasonStop, int32(2)
				if mode == "failure" {
					want, count = ai.StopReasonError, 1
				}
				if mode == "cancel" {
					want, count = ai.StopReasonAborted, 1
				}
				if out.StopReason != want || calls.Load() != count || mode != "cancel" && (len(out.Content) != 1 || out.Content[0].(ai.TextContent).Text != "prefix") {
					t.Fatalf("outcome %+v calls=%d", out, calls.Load())
				}
			})
		}
	}
}

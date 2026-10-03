package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"

	"github.com/nankedr/pig/ai"
)

func main() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"type\":\"response.reasoning_text.delta\",\"output_index\":0,\"content_index\":0,\"delta\":\"思考中\"}\n\ndata: {\"type\":\"response.output_text.delta\",\"output_index\":1,\"content_index\":0,\"delta\":\"你好，Responses！\"}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\n\n")
	}))
	defer server.Close()
	model := ai.Model{ID: "example-model", Provider: ai.ProviderIDDeepSeek, API: ai.APIOpenAIResponses, BaseURL: server.URL, Input: []ai.ModelInput{ai.ModelInputText}, MaxTokens: 64}
	key := "offline-example-key"
	out, err := ai.BuiltinModels().Complete(context.Background(), model, ai.Context{SystemPrompt: ai.Some("简洁回答"), Messages: []ai.Message{ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText("你好")}}}, ai.ModelsStreamOptions{StreamOptions: ai.StreamOptions{ProviderRequestOptions: ai.ProviderRequestOptions{APIKey: &key}}})
	if err != nil {
		panic(err)
	}
	for _, content := range out.Content {
		if text, ok := content.(ai.TextContent); ok {
			fmt.Println(text.Text)
		}
	}
	fmt.Println("stop:", out.StopReason)
}

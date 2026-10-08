package ai_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/nankedr/pig/ai"
)

func TestToolImagesInvalidAndNonvisionHaveNoTransport(t *testing.T) {
	for _, api := range []ai.API{ai.APIOpenAIResponses, ai.APIOpenAICompletions} {
		t.Run(string(api), func(t *testing.T) {
			image := ai.ImageContent{Type: ai.ContentTypeImage, MIMEType: "image/png", Data: image134}
			model := ai.Model{ID: "fixture", API: api, Provider: ai.ProviderIDDeepSeek, Input: []ai.ModelInput{ai.ModelInputText, ai.ModelInputImage}, MaxTokens: 512}
			assistant := ai.AssistantMessage{Role: ai.MessageRoleAssistant, API: api, Provider: model.Provider, Model: model.ID, StopReason: ai.StopReasonToolUse, Content: []ai.AssistantContent{ai.ToolCall{Type: ai.ContentTypeToolCall, ID: "read", Name: "read", Arguments: map[string]any{}}}}
			for _, bad := range []ai.ImageContent{{Type: ai.ContentTypeImage, MIMEType: "image/png", Data: "%%%%"}, {Type: ai.ContentTypeImage, MIMEType: "image/jpeg", Data: image134}, image} {
				input := ai.Context{Messages: []ai.Message{assistant, ai.ToolResultMessage{Role: ai.MessageRoleToolResult, ToolCallID: "read", ToolName: "read", Content: []ai.ToolResultContent{&bad}}}}
				target := model
				if bad == image {
					target.Input = []ai.ModelInput{ai.ModelInputText}
				}
				before, _ := json.Marshal(input)
				called := false
				key := "offline"
				out, err := ai.Complete(t.Context(), target, input, ai.StreamOptions{ProviderRequestOptions: ai.ProviderRequestOptions{APIKey: &key, Fetch: func(context.Context, ai.FetchRequest) (ai.FetchResponse, error) {
					called = true
					return ai.FetchResponse{}, nil
				}}})
				after, _ := json.Marshal(input)
				if called || err == nil && out.StopReason != ai.StopReasonError || string(before) != string(after) {
					t.Fatal("tool attachment rejected silently/mutated/transported")
				}
			}
			blocks := make([]ai.ToolResultContent, 64)
			for i := range blocks {
				blocks[i] = image
			}
			input := ai.Context{Messages: []ai.Message{ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserBlocks(image)}, assistant, ai.ToolResultMessage{Role: ai.MessageRoleToolResult, ToolCallID: "read", Content: blocks}}}
			if err := ai.ValidateUserImages(input); err == nil || !strings.Contains(err.Error(), "64") {
				t.Fatal("aggregate tool+user request quota ignored")
			}
		})
	}
}

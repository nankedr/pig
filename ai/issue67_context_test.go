package ai_test

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/nankedr/pig/ai"
)

func TestContextOverflowFromFauxCompleteAndStream(t *testing.T) {
	handle, err := ai.NewFauxProvider(ai.RegisterFauxProviderOptions{
		Models: []ai.FauxModelDefinition{{ID: "small-window", ContextWindow: ai.Some(100)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	models := ai.CreateModels()
	models.SetProvider(handle.Provider)
	model, _ := handle.GetModel()
	input := ai.Context{Messages: []ai.Message{ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText(strings.Repeat("x", 400)), Timestamp: 1}}}
	for _, reason := range []ai.StopReason{ai.StopReasonStop, ai.StopReasonLength, ai.StopReasonError} {
		t.Run(string(reason), func(t *testing.T) {
			content := ""
			if reason == ai.StopReasonStop {
				content = "done"
			}
			response, err := ai.FauxAssistantMessage(ai.FauxAssistantText(content), ai.FauxAssistantMessageOptions{
				Timestamp: ai.Some[int64](2), StopReason: ai.Some(reason), ErrorMessage: ai.Some("prompt is too long"),
			})
			if err != nil {
				t.Fatal(err)
			}
			handle.SetResponses([]ai.FauxResponseStep{response, response})
			complete, err := models.Complete(context.Background(), model, input, ai.ModelsStreamOptions{})
			if err != nil {
				t.Fatal(err)
			}
			stream := handle.Provider.Stream(context.Background(), model, input, ai.StreamOptions{})
			streamed, err := stream.Result(context.Background())
			if err != nil || !reflect.DeepEqual(streamed, complete) || !ai.IsContextOverflow(complete, model.ContextWindow) {
				t.Fatalf("overflow result = %+v, %v; Complete = %+v", streamed, err, complete)
			}
			if reason == ai.StopReasonStop {
				transcript := ai.Context{Messages: append([]ai.Message{input.Messages[0], complete}, ai.UserMessage{Content: ai.UserText("tail"), Timestamp: 3})}
				estimate := ai.EstimateContextTokens(transcript)
				if estimate.Tokens != 104 || estimate.UsageTokens != 103 || estimate.LastUsageIndex == nil || *estimate.LastUsageIndex != 1 {
					t.Fatalf("Faux transcript estimate = %+v", estimate)
				}
			}
		})
	}
}

func TestContextEstimateClampsSimpleRequestAndPreservesDesiredOutput(t *testing.T) {
	model := openAITextModel("https://example.test/v1")
	model.ContextWindow, model.MaxTokens = 4218, 100
	input := ai.Context{
		Messages: []ai.Message{ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText("test"), Timestamp: 1}},
		Tools:    []ai.Tool{{Name: "read", Description: "read", Parameters: json.RawMessage(`{"type":"object","properties":{}}`)}},
	}
	key, desired := "test-key", int64(100)
	var requestBody map[string]any
	options := ai.SimpleStreamOptions{StreamOptions: ai.StreamOptions{
		MaxTokens: &desired,
		ProviderRequestOptions: ai.ProviderRequestOptions{
			APIKey: &key,
			Fetch: func(_ context.Context, request ai.FetchRequest) (ai.FetchResponse, error) {
				if err := json.Unmarshal(request.Body, &requestBody); err != nil {
					return ai.FetchResponse{}, err
				}
				return ai.FetchResponse{Status: 200, Body: []byte("data: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"length\"}],\"usage\":{\"prompt_tokens\":100,\"completion_tokens\":99}}\n\ndata: [DONE]\n\n")}, nil
			},
		},
	}}
	message, err := ai.StreamSimpleOpenAICompletions(context.Background(), model, input, options).Result(context.Background())
	if err != nil || message.StopReason != ai.StopReasonLength {
		t.Fatalf("length result = %+v, %v", message, err)
	}
	if got := requestBody["max_completion_tokens"]; got != float64(99) {
		t.Fatalf("max_completion_tokens = %v, want 99", got)
	}
	if desired != 100 || !ai.IsRecoverableLength(message, desired) || ai.IsRecoverableLength(message, 99) {
		t.Fatalf("desired limit = %d, output = %d", desired, message.Usage.Output)
	}
}

func TestContextEstimatePointerMessagesAndInvalidJSONFallback(t *testing.T) {
	input := ai.Context{Messages: []ai.Message{
		&ai.UserMessage{Content: ai.UserText("👋你好abc"), Timestamp: 2},
		&ai.AssistantMessage{Timestamp: 1, StopReason: ai.StopReasonStop, Usage: ai.Usage{TotalTokens: 1000}, Content: []ai.AssistantContent{
			&ai.ThinkingContent{Thinking: "plan😀"}, &ai.ToolCall{Name: "go", Arguments: map[string]any{"value": make(chan int)}},
		}},
		&ai.ToolResultMessage{Timestamp: 3, Content: []ai.ToolResultContent{&ai.ImageContent{}, &ai.TextContent{Text: "done!"}}},
		nil, (*ai.UserMessage)(nil), (*ai.AssistantMessage)(nil), (*ai.ToolResultMessage)(nil),
	}}
	got := ai.EstimateContextTokens(input)
	if got.Tokens != 1210 || got.TrailingTokens != 1210 || got.UsageTokens != 0 || got.LastUsageIndex != nil {
		t.Fatalf("pointer estimate = %+v, want 1210 without usage", got)
	}
}

func TestOverflowPatternsAreIndependentCopies(t *testing.T) {
	patterns := ai.GetOverflowPatterns()
	if len(patterns) != 25 {
		t.Fatalf("patterns = %d, want 25 from locked Pi", len(patterns))
	}
	if err := patterns[0].UnmarshalText([]byte("changed")); err != nil {
		t.Fatal(err)
	}
	patterns[1] = nil
	message := ai.AssistantMessage{StopReason: ai.StopReasonError, ErrorMessage: ai.Some("PROMPT IS TOO LONG")}
	if !ai.IsContextOverflow(message) || !ai.GetOverflowPatterns()[0].MatchString("PROMPT IS TOO LONG") {
		t.Fatal("mutating returned patterns changed overflow detection")
	}
}

func TestContextEstimateDoesNotMutateInput(t *testing.T) {
	input := ai.Context{Tools: []ai.Tool{{Name: "read", Parameters: json.RawMessage(`{ "type": "object" }`)}}, Messages: []ai.Message{
		ai.AssistantMessage{Role: ai.MessageRoleAssistant, API: "faux", Provider: "faux", Model: "faux-1", Content: []ai.AssistantContent{}, StopReason: ai.StopReasonToolUse, Usage: ai.Usage{TotalTokens: 10}, Timestamp: 1},
		ai.ToolResultMessage{Role: ai.MessageRoleToolResult, ToolCallID: "call", ToolName: "discover", Content: []ai.ToolResultContent{}, AddedToolNames: ai.Some([]string{"read"}), Timestamp: 2},
	}}
	before, err := ai.MarshalContext(input)
	if err != nil {
		t.Fatal(err)
	}
	ai.EstimateContextTokens(input)
	after, err := ai.MarshalContext(input)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("estimate mutated context: %s, %v", after, err)
	}
}

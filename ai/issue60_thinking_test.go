package ai_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/nankedr/pig/ai"
)

func TestOpenAICompletionsReplaysThinkingAndSignatures(t *testing.T) {
	model := ai.Model{
		ID: "reasoning-model", API: ai.APIOpenAICompletions, Provider: ai.ProviderIDOpenCodeGo, Reasoning: true,
	}
	first := ai.ThinkingContent{Type: ai.ContentTypeThinking, Thinking: "plan", ThinkingSignature: ai.Some("reasoning")}
	second := ai.ThinkingContent{Type: ai.ContentTypeThinking, Thinking: "check", ThinkingSignature: ai.Some("reasoning_text")}
	detail := "{\"type\":\"reasoning.encrypted\",\"id\":\"call-1\",\"data\":\"secret\"}"
	messages, err := ai.ConvertOpenAICompletionsMessages(model, ai.Context{Messages: []ai.Message{
		ai.AssistantMessage{
			Role: ai.MessageRoleAssistant, API: model.API, Provider: model.Provider, Model: model.ID, StopReason: ai.StopReasonToolUse,
			Content: []ai.AssistantContent{
				first, second,
				ai.TextContent{Type: ai.ContentTypeText, Text: "answer", TextSignature: ai.Some("{\"v\":1,\"id\":\"text-1\"}")},
				ai.ToolCall{Type: ai.ContentTypeToolCall, ID: "call-1", Name: "read", Arguments: map[string]any{"path": "README.md"}, ThoughtSignature: ai.Some(detail)},
				ai.ToolCall{Type: ai.ContentTypeToolCall, ID: "call-2", Name: "noop", Arguments: map[string]any{}, ThoughtSignature: ai.Some("not-json")},
			},
		},
	}}, ai.OpenAICompletionsCompat{RequiresReasoningContentOnAssistantMessages: ai.Some(true)})
	if err != nil {
		t.Fatalf("ConvertOpenAICompletionsMessages() error = %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("messages = %#v", messages)
	}
	var got map[string]any
	if err := json.Unmarshal(messages[0], &got); err != nil {
		t.Fatalf("decode assistant message: %v", err)
	}
	want := map[string]any{
		"role": "assistant", "content": "answer", "reasoning_content": "plan\ncheck",
		"reasoning_details": []any{map[string]any{"type": "reasoning.encrypted", "id": "call-1", "data": "secret"}},
		"tool_calls": []any{
			map[string]any{"id": "call-1", "type": "function", "function": map[string]any{"name": "read", "arguments": "{\"path\":\"README.md\"}"}},
			map[string]any{"id": "call-2", "type": "function", "function": map[string]any{"name": "noop", "arguments": "{}"}},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("assistant replay = %#v, want %#v", got, want)
	}
}

func TestOpenAICompletionsTransformsThinkingAcrossModels(t *testing.T) {
	model := ai.Model{ID: "target", API: ai.APIOpenAICompletions, Provider: "target", Reasoning: true}
	messages, err := ai.ConvertOpenAICompletionsMessages(model, ai.Context{Messages: []ai.Message{
		ai.AssistantMessage{
			Role: ai.MessageRoleAssistant, API: ai.APIOpenAICompletions, Provider: "source", Model: "source", StopReason: ai.StopReasonToolUse,
			Content: []ai.AssistantContent{
				ai.ThinkingContent{Type: ai.ContentTypeThinking, Thinking: "visible reasoning", ThinkingSignature: ai.Some("reasoning_content")},
				ai.ThinkingContent{Type: ai.ContentTypeThinking, Thinking: "secret", Redacted: ai.Some(true)},
				ai.ThinkingContent{Type: ai.ContentTypeThinking, Thinking: "   "},
				ai.TextContent{Type: ai.ContentTypeText, Text: "answer"},
				ai.ToolCall{Type: ai.ContentTypeToolCall, ID: "call", Name: "read", Arguments: map[string]any{}, ThoughtSignature: ai.Some("{\"type\":\"reasoning.encrypted\"}")},
			},
		},
	}}, ai.OpenAICompletionsCompat{RequiresThinkingAsText: ai.Some(true)})
	if err != nil {
		t.Fatalf("ConvertOpenAICompletionsMessages() error = %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(messages[0], &got); err != nil {
		t.Fatalf("decode assistant message: %v", err)
	}
	if got["content"] != "visible reasoninganswer" {
		t.Fatalf("cross-model content = %#v, want visible reasoninganswer", got["content"])
	}
	if _, ok := got["reasoning_details"]; ok {
		t.Fatalf("cross-model thought signature leaked: %#v", got)
	}
}

func TestOpenAICompletionsReplaysSameModelThinkingAsTextParts(t *testing.T) {
	model := ai.Model{ID: "target", API: ai.APIOpenAICompletions, Provider: "target", Reasoning: true}
	messages, err := ai.ConvertOpenAICompletionsMessages(model, ai.Context{Messages: []ai.Message{
		ai.AssistantMessage{
			Role: ai.MessageRoleAssistant, API: model.API, Provider: model.Provider, Model: model.ID, StopReason: ai.StopReasonStop,
			Content: []ai.AssistantContent{
				ai.ThinkingContent{Type: ai.ContentTypeThinking, Thinking: "plan"},
				ai.ThinkingContent{Type: ai.ContentTypeThinking, Thinking: "check"},
				ai.TextContent{Type: ai.ContentTypeText, Text: "answer"},
			},
		},
	}}, ai.OpenAICompletionsCompat{RequiresThinkingAsText: ai.Some(true)})
	if err != nil {
		t.Fatalf("ConvertOpenAICompletionsMessages() error = %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(messages[0], &got); err != nil {
		t.Fatalf("decode assistant message: %v", err)
	}
	want := []any{
		map[string]any{"type": "text", "text": "plan\n\ncheck"},
		map[string]any{"type": "text", "text": "answer"},
	}
	if !reflect.DeepEqual(got["content"], want) {
		t.Fatalf("same-model content = %#v, want %#v", got["content"], want)
	}
}

func TestOpenAICompletionsMapsReasoningRequests(t *testing.T) {
	high := ai.OpenAIReasoningEffortHigh
	for _, test := range []struct {
		name   string
		compat string
		effort *ai.OpenAIReasoningEffort
		levels ai.ThinkingLevelMap
		want   map[string]any
	}{
		{name: "openai", compat: "{\"supportsReasoningEffort\":true,\"thinkingFormat\":\"openai\"}", effort: &high, levels: ai.ThinkingLevelMap{ai.ModelThinkingLevelHigh: ai.Some("hard")}, want: map[string]any{"reasoning_effort": "hard"}},
		{name: "deepseek enabled", compat: "{\"supportsReasoningEffort\":true,\"thinkingFormat\":\"deepseek\"}", effort: &high, want: map[string]any{"thinking": map[string]any{"type": "enabled"}, "reasoning_effort": "high"}},
		{name: "deepseek disabled", compat: "{\"thinkingFormat\":\"deepseek\"}", want: map[string]any{"thinking": map[string]any{"type": "disabled"}}},
		{name: "openrouter off mapping", compat: "{\"thinkingFormat\":\"openrouter\"}", levels: ai.ThinkingLevelMap{ai.ModelThinkingLevelOff: ai.Some("disabled")}, want: map[string]any{"reasoning": map[string]any{"effort": "disabled"}}},
		{name: "qwen", compat: "{\"supportsReasoningEffort\":true,\"thinkingFormat\":\"qwen\"}", effort: &high, want: map[string]any{"enable_thinking": true, "reasoning_effort": "high"}},
		{name: "zai", compat: "{\"thinkingFormat\":\"zai\"}", effort: &high, want: map[string]any{"thinking": map[string]any{"type": "enabled", "clear_thinking": false}}},
		{name: "together", compat: "{\"thinkingFormat\":\"together\"}", effort: &high, want: map[string]any{"reasoning": map[string]any{"enabled": true}}},
		{name: "string thinking", compat: "{\"thinkingFormat\":\"string-thinking\"}", effort: &high, want: map[string]any{"thinking": "high"}},
		{name: "ant ling", compat: "{\"thinkingFormat\":\"ant-ling\"}", effort: &high, levels: ai.ThinkingLevelMap{ai.ModelThinkingLevelHigh: ai.Some("enabled")}, want: map[string]any{"reasoning": map[string]any{"effort": "enabled"}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			model := issue60OpenAIModel()
			model.ThinkingLevelMap = test.levels
			model.Compat = ai.Some(json.RawMessage(test.compat))
			payload := captureIssue60OpenAIPayload(t, model, ai.OpenAICompletionsOptions{ReasoningEffort: test.effort})
			for field, want := range test.want {
				if !reflect.DeepEqual(payload[field], want) {
					t.Fatalf("payload[%q] = %#v, want %#v; payload=%#v", field, payload[field], want, payload)
				}
			}
		})
	}
}

func TestSimpleOpenAICompletionsClampsReasoningAndThinkingBudget(t *testing.T) {
	model := issue60OpenAIModel()
	model.MaxTokens = 16_384
	model.ThinkingLevelMap = ai.ThinkingLevelMap{
		ai.ModelThinkingLevelMedium: ai.Null[string](),
		ai.ModelThinkingLevelHigh:   ai.Some("hard"),
	}
	model.Compat = ai.Some(json.RawMessage("{\"supportsReasoningEffort\":true,\"supportsThinkingTokenBudget\":true,\"thinkingFormat\":\"openai\"}"))
	reasoning := ai.ThinkingLevelMedium
	budget := int64(8_192)
	maxTokens := int64(4_096)
	key := "test-key"
	var payload map[string]any
	stream := ai.StreamSimpleOpenAICompletions(context.Background(), model, ai.Context{}, ai.SimpleStreamOptions{
		Reasoning: &reasoning, ThinkingBudgets: &ai.ThinkingBudgets{High: &budget},
		StreamOptions: ai.StreamOptions{MaxTokens: &maxTokens, ProviderRequestOptions: ai.ProviderRequestOptions{
			APIKey: &key,
			Fetch: func(_ context.Context, request ai.FetchRequest) (ai.FetchResponse, error) {
				if err := json.Unmarshal(request.Body, &payload); err != nil {
					t.Fatalf("decode request: %v", err)
				}
				return ai.FetchResponse{Status: http.StatusOK, Body: []byte(issue60DoneSSE)}, nil
			},
		}},
	})
	if _, err := stream.Result(context.Background()); err != nil {
		t.Fatalf("Result() error = %v", err)
	}
	if payload["reasoning_effort"] != "hard" || payload["thinking_token_budget"] != float64(3_072) {
		t.Fatalf("reasoning payload = %#v, want hard effort and 3072 budget", payload)
	}
}

const issue60DoneSSE = "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"

func issue60OpenAIModel() ai.Model {
	return ai.Model{
		ID: "reasoning-model", API: ai.APIOpenAICompletions, Provider: "local", BaseURL: "https://example.test/v1",
		Reasoning: true, ContextWindow: 32_000, MaxTokens: 4_096,
	}
}

func captureIssue60OpenAIPayload(t *testing.T, model ai.Model, options ai.OpenAICompletionsOptions) map[string]any {
	t.Helper()
	key := "test-key"
	options.APIKey = &key
	var payload map[string]any
	options.Fetch = func(_ context.Context, request ai.FetchRequest) (ai.FetchResponse, error) {
		if err := json.Unmarshal(request.Body, &payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		return ai.FetchResponse{Status: http.StatusOK, Body: []byte(issue60DoneSSE)}, nil
	}
	if _, err := ai.StreamOpenAICompletions(context.Background(), model, ai.Context{}, options).Result(context.Background()); err != nil {
		t.Fatalf("Result() error = %v", err)
	}
	return payload
}

func TestOpenAICompletionsStreamsThinkingDetailsAndReasoningUsage(t *testing.T) {
	sse := strings.Join([]string{
		"data: {\"id\":\"chatcmpl-thinking\",\"choices\":[{\"delta\":{\"reasoning_content\":\"first\",\"reasoning\":\"duplicate\"}}]}",
		"data: {\"choices\":[{\"delta\":{\"reasoning\":\" second\"}}]}",
		"data: {\"choices\":[{\"delta\":{\"reasoning_details\":[{\"type\":\"reasoning.encrypted\",\"id\":\"call-1\",\"data\":\"old\"},{\"type\":\"invalid\",\"id\":\"call-1\",\"data\":\"ignored\"}]}}]}",
		"data: {\"choices\":[{\"delta\":{\"reasoning_details\":[{\"type\":\"reasoning.encrypted\",\"id\":\"call-1\",\"data\":\"secret\"}]}}]}",
		"data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call-1\",\"function\":{\"name\":\"read\",\"arguments\":\"{\\\"path\\\":\\\"README.md\\\"}\"}}]}}]}",
		"data: {\"choices\":[{\"delta\":{\"reasoning_details\":[{\"type\":\"reasoning.encrypted\",\"id\":\"call-2\",\"data\":\"later\"}],\"tool_calls\":[{\"index\":1,\"id\":\"call-2\",\"function\":{\"name\":\"write\",\"arguments\":\"{}\"}}]},\"finish_reason\":\"tool_calls\"}]}",
		"data: {\"choices\":[],\"usage\":{\"prompt_tokens\":5,\"completion_tokens\":8,\"completion_tokens_details\":{\"reasoning_tokens\":6}}}",
		"data: [DONE]",
	}, "\n\n") + "\n\n"
	model := issue60OpenAIModel()
	key := "test-key"
	stream := ai.StreamOpenAICompletions(context.Background(), model, ai.Context{}, ai.OpenAICompletionsOptions{
		StreamOptions: ai.StreamOptions{ProviderRequestOptions: ai.ProviderRequestOptions{
			APIKey: &key,
			Fetch: func(context.Context, ai.FetchRequest) (ai.FetchResponse, error) {
				return ai.FetchResponse{Status: http.StatusOK, Body: []byte(sse)}, nil
			},
		}},
	})
	var eventTypes []ai.AssistantMessageEventType
	var firstDelta ai.AssistantMessage
	for {
		event, ok, err := stream.Next(context.Background())
		if err != nil {
			t.Fatalf("Next() error = %v", err)
		}
		if !ok {
			break
		}
		eventTypes = append(eventTypes, event.AssistantMessageEventType())
		if delta, ok := event.(ai.AssistantMessageThinkingDeltaEvent); ok && len(firstDelta.Content) == 0 {
			firstDelta = delta.Partial
		}
	}
	wantEvents := []ai.AssistantMessageEventType{
		ai.AssistantMessageEventTypeStart,
		ai.AssistantMessageEventTypeThinkingStart,
		ai.AssistantMessageEventTypeThinkingDelta,
		ai.AssistantMessageEventTypeThinkingDelta,
		ai.AssistantMessageEventTypeToolCallStart,
		ai.AssistantMessageEventTypeToolCallDelta,
		ai.AssistantMessageEventTypeToolCallStart,
		ai.AssistantMessageEventTypeToolCallDelta,
		ai.AssistantMessageEventTypeThinkingEnd,
		ai.AssistantMessageEventTypeToolCallEnd,
		ai.AssistantMessageEventTypeToolCallEnd,
		ai.AssistantMessageEventTypeDone,
	}
	if !reflect.DeepEqual(eventTypes, wantEvents) {
		t.Fatalf("event types = %#v, want %#v", eventTypes, wantEvents)
	}
	if got := firstDelta.Content[0].(ai.ThinkingContent).Thinking; got != "first" {
		t.Fatalf("first immutable thinking delta = %q", got)
	}
	result, err := stream.Result(context.Background())
	if err != nil {
		t.Fatalf("Result() error = %v", err)
	}
	thinking := result.Content[0].(ai.ThinkingContent)
	if thinking.Thinking != "first second" || thinking.ThinkingSignature != ai.Some("reasoning_content") {
		t.Fatalf("thinking = %#v", thinking)
	}
	first := result.Content[1].(ai.ToolCall)
	second := result.Content[2].(ai.ToolCall)
	wantFirstSignature := ai.Some("{\"type\":\"reasoning.encrypted\",\"id\":\"call-1\",\"data\":\"secret\"}")
	wantSecondSignature := ai.Some("{\"type\":\"reasoning.encrypted\",\"id\":\"call-2\",\"data\":\"later\"}")
	if first.ThoughtSignature != wantFirstSignature || second.ThoughtSignature != wantSecondSignature {
		t.Fatalf("thought signatures = %#v / %#v", first.ThoughtSignature, second.ThoughtSignature)
	}
	if result.Usage.Output != 8 || result.Usage.Reasoning != ai.Some[int64](6) || result.Usage.TotalTokens != 13 {
		t.Fatalf("usage = %#v", result.Usage)
	}
}

func TestOpenAICompletionsThinkingAndSignatureParity(t *testing.T) {
	fixture := loadIssue60ThinkingFixture(t)
	model := issue60OpenAIModel()
	model.Provider = ai.ProviderIDOpenCodeGo
	model.MaxTokens = 16_384
	model.ThinkingLevelMap = ai.ThinkingLevelMap{
		ai.ModelThinkingLevelMedium: ai.Null[string](),
		ai.ModelThinkingLevelHigh:   ai.Some("hard"),
	}
	model.Compat = ai.Some(json.RawMessage(`{"supportsReasoningEffort":true,"requiresReasoningContentOnAssistantMessages":true,"thinkingFormat":"openai","supportsThinkingTokenBudget":true}`))
	detail := `{"type":"reasoning.encrypted","id":"history-call","data":"history-secret"}`
	input := ai.Context{
		Messages: []ai.Message{
			ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText("No thinking"), Timestamp: 1},
			ai.AssistantMessage{Role: ai.MessageRoleAssistant, API: model.API, Provider: model.Provider, Model: model.ID, StopReason: ai.StopReasonStop, Timestamp: 2, Content: []ai.AssistantContent{
				ai.TextContent{Type: ai.ContentTypeText, Text: "plain"},
			}},
			ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText("Earlier question"), Timestamp: 1},
			ai.AssistantMessage{
				Role: ai.MessageRoleAssistant, API: model.API, Provider: model.Provider, Model: model.ID, StopReason: ai.StopReasonToolUse, Timestamp: 2,
				Content: []ai.AssistantContent{
					ai.ThinkingContent{Type: ai.ContentTypeThinking, Thinking: "plan", ThinkingSignature: ai.Some("reasoning")},
					ai.ThinkingContent{Type: ai.ContentTypeThinking, Thinking: "check", ThinkingSignature: ai.Some("reasoning_text")},
					ai.TextContent{Type: ai.ContentTypeText, Text: "answer", TextSignature: ai.Some(`{"v":1,"id":"ignored"}`)},
					ai.ToolCall{Type: ai.ContentTypeToolCall, ID: "history-call", Name: "read", Arguments: map[string]any{"path": "README.md"}, ThoughtSignature: ai.Some(detail)},
				},
			},
			ai.ToolResultMessage{Role: ai.MessageRoleToolResult, ToolCallID: "history-call", ToolName: "read", Content: []ai.ToolResultContent{ai.TextContent{Type: ai.ContentTypeText, Text: "file"}}, Timestamp: 3},
			ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText("Continue"), Timestamp: 4},
		},
		Tools: []ai.Tool{{Name: "read", Description: "Read a file", Parameters: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`)}},
	}
	key := "test-key"
	maxTokens := int64(4096)
	reasoning := ai.ThinkingLevelMedium
	highBudget := int64(8192)
	var request map[string]any
	stream := ai.StreamSimpleOpenAICompletions(context.Background(), model, input, ai.SimpleStreamOptions{
		Reasoning: &reasoning, ThinkingBudgets: &ai.ThinkingBudgets{High: &highBudget},
		StreamOptions: ai.StreamOptions{MaxTokens: &maxTokens, ProviderRequestOptions: ai.ProviderRequestOptions{
			APIKey: &key,
			Fetch: func(_ context.Context, fetchRequest ai.FetchRequest) (ai.FetchResponse, error) {
				if err := json.Unmarshal(fetchRequest.Body, &request); err != nil {
					t.Fatalf("decode request: %v", err)
				}
				return ai.FetchResponse{Status: http.StatusOK, Body: []byte(fixture.Input.SSE)}, nil
			},
		}},
	})
	events := collectAssistantEvents(t, stream)
	result, err := stream.Result(context.Background())
	if err != nil {
		t.Fatalf("Result() error = %v", err)
	}
	if !reflect.DeepEqual(request, fixture.Actual.Request.Body) {
		t.Fatalf("request = %#v, want pinned Pi %#v", request, fixture.Actual.Request.Body)
	}
	if got := issue60ProjectEvents(t, events); !reflect.DeepEqual(got, fixture.Actual.Events) {
		t.Fatalf("events = %#v, want pinned Pi %#v", got, fixture.Actual.Events)
	}
	if got := issue60NormalizedJSON(t, result); !reflect.DeepEqual(got, fixture.Actual.Outcome) {
		t.Fatalf("result = %#v, want pinned Pi %#v", got, fixture.Actual.Outcome)
	}
	if got := issue60ReasoningRequestCases(t); !reflect.DeepEqual(got, fixture.Actual.RequestCases) {
		t.Fatalf("reasoning request cases = %#v, want pinned Pi %#v", got, fixture.Actual.RequestCases)
	}
	if got := issue60ConversionCases(t); !reflect.DeepEqual(got, fixture.Actual.Conversions) {
		t.Fatalf("conversion cases = %#v, want pinned Pi %#v", got, fixture.Actual.Conversions)
	}
}

func TestOpenAICompletionsEndsBlocksInContentOrder(t *testing.T) {
	sse := strings.Join([]string{
		`data: {"choices":[{"delta":{"reasoning_content":"plan"}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call-1","function":{"name":"read","arguments":"{}"}}]}}]}`,
		`data: {"choices":[{"delta":{"content":"answer"},"finish_reason":"tool_calls"}]}`,
		"data: [DONE]",
	}, "\n\n") + "\n\n"
	model := issue60OpenAIModel()
	key := "test-key"
	stream := ai.StreamOpenAICompletions(context.Background(), model, ai.Context{}, ai.OpenAICompletionsOptions{
		StreamOptions: ai.StreamOptions{ProviderRequestOptions: ai.ProviderRequestOptions{
			APIKey: &key,
			Fetch: func(context.Context, ai.FetchRequest) (ai.FetchResponse, error) {
				return ai.FetchResponse{Status: http.StatusOK, Body: []byte(sse)}, nil
			},
		}},
	})
	var ends []ai.AssistantMessageEventType
	for _, event := range collectAssistantEvents(t, stream) {
		switch event.AssistantMessageEventType() {
		case ai.AssistantMessageEventTypeThinkingEnd, ai.AssistantMessageEventTypeToolCallEnd, ai.AssistantMessageEventTypeTextEnd:
			ends = append(ends, event.AssistantMessageEventType())
		}
	}
	want := []ai.AssistantMessageEventType{
		ai.AssistantMessageEventTypeThinkingEnd,
		ai.AssistantMessageEventTypeToolCallEnd,
		ai.AssistantMessageEventTypeTextEnd,
	}
	if !reflect.DeepEqual(ends, want) {
		t.Fatalf("end events = %#v, want content order %#v", ends, want)
	}
}

func TestOpenAICompletionsCanonicalizesAndConsumesPendingReasoningDetail(t *testing.T) {
	sse := strings.Join([]string{
		`data: {"choices":[{"delta":{"reasoning_details":[{"type":"reasoning.encrypted","id":"call-1","data":"old"}]}}]}`,
		`data: {"choices":[{"delta":{"reasoning_details":[{ "type" : "reasoning.encrypted", "id" : "call-1", "data" : "latest" }]}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call-1","function":{"name":"first","arguments":"{}"}}]}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":1,"id":"call-2","function":{"name":"second","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}`,
		"data: [DONE]",
	}, "\n\n") + "\n\n"
	model := issue60OpenAIModel()
	key := "test-key"
	result, err := ai.StreamOpenAICompletions(context.Background(), model, ai.Context{}, ai.OpenAICompletionsOptions{
		StreamOptions: ai.StreamOptions{ProviderRequestOptions: ai.ProviderRequestOptions{
			APIKey: &key,
			Fetch: func(context.Context, ai.FetchRequest) (ai.FetchResponse, error) {
				return ai.FetchResponse{Status: http.StatusOK, Body: []byte(sse)}, nil
			},
		}},
	}).Result(context.Background())
	if err != nil {
		t.Fatalf("Result() error = %v", err)
	}
	first := result.Content[0].(ai.ToolCall)
	second := result.Content[1].(ai.ToolCall)
	if first.ThoughtSignature != ai.Some(`{"type":"reasoning.encrypted","id":"call-1","data":"latest"}`) {
		t.Fatalf("first thought signature = %#v", first.ThoughtSignature)
	}
	if second.ThoughtSignature.IsSet() {
		t.Fatalf("pending thought signature leaked to second tool call: %#v", second.ThoughtSignature)
	}
}

func TestOpenAICompletionsRejectsPostM2ThinkingFormats(t *testing.T) {
	for _, format := range []string{"chat-template", "qwen-chat-template", "baseten"} {
		t.Run(format, func(t *testing.T) {
			model := issue60OpenAIModel()
			model.Compat = ai.Some(json.RawMessage(`{"thinkingFormat":"` + format + `"}`))
			_, err := ai.StreamOpenAICompletions(context.Background(), model, ai.Context{}, ai.OpenAICompletionsOptions{}).Result(context.Background())
			if !errors.Is(err, ai.ErrNotImplemented) {
				t.Fatalf("Result() error = %v, want ErrNotImplemented", err)
			}
		})
	}
}

func TestFauxStreamsThinkingAndPreservesItsReplayMetadata(t *testing.T) {
	one := 1
	thinking := ai.FauxThinking("consider")
	thinking.ThinkingSignature = ai.Some("reasoning_content")
	thinking.Redacted = ai.Some(true)
	message, err := ai.FauxAssistantMessage(
		ai.FauxAssistantBlocks(thinking, ai.FauxText("answer")),
		ai.FauxAssistantMessageOptions{Timestamp: ai.Some[int64](1)},
	)
	if err != nil {
		t.Fatalf("FauxAssistantMessage() error = %v", err)
	}
	handle, err := ai.NewFauxProvider(ai.RegisterFauxProviderOptions{
		API: "faux:thinking", Provider: "faux-thinking", Models: []ai.FauxModelDefinition{{ID: "thinking-model"}},
		TokenSize: &ai.FauxTokenSize{Min: &one, Max: &one},
	})
	if err != nil {
		t.Fatalf("NewFauxProvider() error = %v", err)
	}
	handle.SetResponses([]ai.FauxResponseStep{message})
	model, _ := handle.GetModel()
	reasoning := ai.ThinkingLevelHigh
	budget := int64(4096)
	stream := handle.Provider.StreamSimple(context.Background(), model, ai.Context{}, ai.SimpleStreamOptions{
		Reasoning: &reasoning, ThinkingBudgets: &ai.ThinkingBudgets{High: &budget},
	})

	var eventTypes []ai.AssistantMessageEventType
	var firstDelta ai.AssistantMessage
	for {
		event, ok, err := stream.Next(context.Background())
		if err != nil {
			t.Fatalf("Next() error = %v", err)
		}
		if !ok {
			break
		}
		eventTypes = append(eventTypes, event.AssistantMessageEventType())
		if delta, ok := event.(ai.AssistantMessageThinkingDeltaEvent); ok && len(firstDelta.Content) == 0 {
			firstDelta = delta.Partial
		}
	}
	wantTypes := []ai.AssistantMessageEventType{
		ai.AssistantMessageEventTypeStart,
		ai.AssistantMessageEventTypeThinkingStart,
		ai.AssistantMessageEventTypeThinkingDelta,
		ai.AssistantMessageEventTypeThinkingDelta,
		ai.AssistantMessageEventTypeThinkingEnd,
		ai.AssistantMessageEventTypeTextStart,
		ai.AssistantMessageEventTypeTextDelta,
		ai.AssistantMessageEventTypeTextDelta,
		ai.AssistantMessageEventTypeTextEnd,
		ai.AssistantMessageEventTypeDone,
	}
	if !reflect.DeepEqual(eventTypes, wantTypes) {
		t.Fatalf("event types = %v, want %v", eventTypes, wantTypes)
	}
	if got := firstDelta.Content[0].(ai.ThinkingContent).Thinking; got != "cons" {
		t.Fatalf("first immutable thinking delta = %q, want cons", got)
	}
	result, err := stream.Result(context.Background())
	if err != nil {
		t.Fatalf("Result() error = %v", err)
	}
	if !reflect.DeepEqual(result.Content, message.Content) {
		t.Fatalf("Result content = %#v, want %#v", result.Content, message.Content)
	}
}

func TestFauxThinkingCancellationRetainsStreamedPartialContent(t *testing.T) {
	one := 1
	rate := float64(100)
	message, _ := ai.FauxAssistantMessage(
		ai.FauxAssistantBlocks(ai.FauxThinking("abcdefghijklmnop")),
		ai.FauxAssistantMessageOptions{Timestamp: ai.Some[int64](1)},
	)
	handle, err := ai.NewFauxProvider(ai.RegisterFauxProviderOptions{
		API: "faux:thinking-cancel", Provider: "faux-thinking-cancel",
		Models:    []ai.FauxModelDefinition{{ID: "thinking-cancel-model"}},
		TokenSize: &ai.FauxTokenSize{Min: &one, Max: &one}, TokensPerSecond: &rate,
	})
	if err != nil {
		t.Fatalf("NewFauxProvider() error = %v", err)
	}
	handle.SetResponses([]ai.FauxResponseStep{message})
	model, _ := handle.GetModel()
	ctx, cancel := context.WithCancel(context.Background())
	stream := handle.Provider.Stream(ctx, model, ai.Context{}, ai.StreamOptions{})
	for {
		event, ok, err := stream.Next(context.Background())
		if err != nil || !ok {
			t.Fatalf("Next() before cancellation = (%#v, %t, %v)", event, ok, err)
		}
		if event.AssistantMessageEventType() == ai.AssistantMessageEventTypeThinkingDelta {
			cancel()
			break
		}
	}

	wait, stopWaiting := context.WithTimeout(context.Background(), time.Second)
	defer stopWaiting()
	result, err := stream.Result(wait)
	if err != nil || result.StopReason != ai.StopReasonAborted {
		t.Fatalf("Result() = (%#v, %v), want aborted", result, err)
	}
	if len(result.Content) != 1 || result.Content[0].(ai.ThinkingContent).Thinking != "abcd" {
		t.Fatalf("aborted content = %#v, want first thinking chunk", result.Content)
	}
}

func TestFauxProviderErrorRetainsThinkingMetadata(t *testing.T) {
	thinking := ai.FauxThinking("partial reasoning")
	thinking.ThinkingSignature = ai.Some("reasoning_content")
	thinking.Redacted = ai.Some(true)
	message, err := ai.FauxAssistantMessage(
		ai.FauxAssistantBlocks(thinking),
		ai.FauxAssistantMessageOptions{
			StopReason: ai.Some(ai.StopReasonError), ErrorMessage: ai.Some("provider failed"), Timestamp: ai.Some[int64](1),
		},
	)
	if err != nil {
		t.Fatalf("FauxAssistantMessage() error = %v", err)
	}
	handle, err := ai.NewFauxProvider(ai.RegisterFauxProviderOptions{
		API: "faux:thinking-error", Provider: "faux-thinking-error",
		Models: []ai.FauxModelDefinition{{ID: "thinking-error-model"}},
	})
	if err != nil {
		t.Fatalf("NewFauxProvider() error = %v", err)
	}
	handle.SetResponses([]ai.FauxResponseStep{message})
	model, _ := handle.GetModel()
	result, err := handle.Provider.Stream(context.Background(), model, ai.Context{}, ai.StreamOptions{}).Result(context.Background())
	if err != nil || result.StopReason != ai.StopReasonError {
		t.Fatalf("Result() = (%#v, %v), want provider error", result, err)
	}
	if !reflect.DeepEqual(result.Content, message.Content) {
		t.Fatalf("Result content = %#v, want %#v", result.Content, message.Content)
	}
}

type issue60ThinkingFixture struct {
	ID             string   `json:"id"`
	CatalogIDs     []string `json:"catalog_ids"`
	BaselineCommit string   `json:"baseline_commit"`
	Deterministic  bool     `json:"deterministic"`
	Input          struct {
		SSE string `json:"sse"`
	} `json:"input"`
	Actual struct {
		Request struct {
			Body map[string]any `json:"body"`
		} `json:"request"`
		Events       []any          `json:"events"`
		Outcome      any            `json:"outcome"`
		RequestCases map[string]any `json:"request_cases"`
		Conversions  any            `json:"conversions"`
	} `json:"actual"`
}

func loadIssue60ThinkingFixture(t *testing.T) issue60ThinkingFixture {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	data, err := os.ReadFile(filepath.Join(filepath.Dir(file), "..", "parity", "oracle", "fixtures", "openai-completions-thinking.json"))
	if err != nil {
		t.Fatalf("read thinking fixture: %v", err)
	}
	var fixture issue60ThinkingFixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatalf("decode thinking fixture: %v", err)
	}
	if fixture.ID != "ai/openai-completions/m2-thinking-signatures" || fixture.BaselineCommit != "936aff00918de1187f085f123c2812d8f2d67745" || !fixture.Deterministic || len(fixture.CatalogIDs) == 0 {
		t.Fatalf("thinking fixture provenance = %#v", fixture)
	}
	return fixture
}

func issue60NormalizedJSON(t *testing.T, value any) any {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal JSON value: %v", err)
	}
	var normalized any
	if err := json.Unmarshal(data, &normalized); err != nil {
		t.Fatalf("decode JSON value: %v", err)
	}
	removeIssue60Timestamps(normalized)
	return normalized
}

func issue60ReasoningRequestCases(t *testing.T) any {
	t.Helper()
	high := ai.OpenAIReasoningEffortHigh
	tests := []struct {
		id, compat string
		effort     *ai.OpenAIReasoningEffort
		levels     ai.ThinkingLevelMap
	}{
		{id: "openai", compat: `{"supportsReasoningEffort":true,"thinkingFormat":"openai"}`, effort: &high, levels: ai.ThinkingLevelMap{ai.ModelThinkingLevelHigh: ai.Some("hard")}},
		{id: "deepseek-enabled", compat: `{"supportsReasoningEffort":true,"thinkingFormat":"deepseek"}`, effort: &high},
		{id: "deepseek-disabled", compat: `{"thinkingFormat":"deepseek"}`},
		{id: "openrouter-off", compat: `{"thinkingFormat":"openrouter"}`, levels: ai.ThinkingLevelMap{ai.ModelThinkingLevelOff: ai.Some("disabled")}},
		{id: "qwen", compat: `{"supportsReasoningEffort":true,"thinkingFormat":"qwen"}`, effort: &high},
		{id: "zai", compat: `{"thinkingFormat":"zai"}`, effort: &high},
		{id: "together", compat: `{"thinkingFormat":"together"}`, effort: &high},
		{id: "string-thinking", compat: `{"thinkingFormat":"string-thinking"}`, effort: &high},
		{id: "ant-ling", compat: `{"thinkingFormat":"ant-ling"}`, effort: &high, levels: ai.ThinkingLevelMap{ai.ModelThinkingLevelHigh: ai.Some("enabled")}},
	}
	result := make(map[string]any, len(tests))
	for _, test := range tests {
		model := issue60OpenAIModel()
		model.ThinkingLevelMap = test.levels
		model.Compat = ai.Some(json.RawMessage(test.compat))
		payload := captureIssue60OpenAIPayload(t, model, ai.OpenAICompletionsOptions{ReasoningEffort: test.effort})
		fields := map[string]any{}
		for _, field := range []string{"enable_thinking", "reasoning", "reasoning_effort", "thinking"} {
			if value, ok := payload[field]; ok {
				fields[field] = value
			}
		}
		result[test.id] = fields
	}
	return issue60NormalizedJSON(t, result)
}

func issue60ConversionCases(t *testing.T) any {
	t.Helper()
	usage := ai.Usage{Cost: ai.UsageCost{}}
	message := ai.AssistantMessage{
		Role: ai.MessageRoleAssistant, API: ai.APIOpenAICompletions, Provider: "source", Model: "source", Usage: usage, StopReason: ai.StopReasonToolUse, Timestamp: 1,
		Content: []ai.AssistantContent{
			ai.ThinkingContent{Type: ai.ContentTypeThinking, Thinking: "visible reasoning", ThinkingSignature: ai.Some("reasoning_content")},
			ai.ThinkingContent{Type: ai.ContentTypeThinking, Thinking: "secret", ThinkingSignature: ai.Some("opaque"), Redacted: ai.Some(true)},
			ai.ThinkingContent{Type: ai.ContentTypeThinking, Thinking: "   "},
			ai.TextContent{Type: ai.ContentTypeText, Text: "answer", TextSignature: ai.Some(`{"v":1,"id":"ignored"}`)},
			ai.ToolCall{Type: ai.ContentTypeToolCall, ID: "call", Name: "read", Arguments: map[string]any{}, ThoughtSignature: ai.Some(`{"type":"reasoning.encrypted","id":"call","data":"secret"}`)},
		},
	}
	target := issue60OpenAIModel()
	target.ID, target.Provider = "target", "target"
	crossModel, err := ai.ConvertOpenAICompletionsMessages(target, ai.Context{Messages: []ai.Message{message}}, ai.OpenAICompletionsCompat{})
	if err != nil {
		t.Fatalf("cross-model conversion: %v", err)
	}
	message.Provider, message.Model, message.StopReason = target.Provider, target.ID, ai.StopReasonStop
	message.Content = []ai.AssistantContent{
		ai.ThinkingContent{Type: ai.ContentTypeThinking, Thinking: "plan"},
		ai.ThinkingContent{Type: ai.ContentTypeThinking, Thinking: "check"},
		ai.TextContent{Type: ai.ContentTypeText, Text: "answer"},
		ai.ToolCall{Type: ai.ContentTypeToolCall, ID: "valid", Name: "read", Arguments: map[string]any{}, ThoughtSignature: ai.Some(`{"type":"reasoning.encrypted","id":"valid","data":"ok"}`)},
		ai.ToolCall{Type: ai.ContentTypeToolCall, ID: "invalid", Name: "read", Arguments: map[string]any{}, ThoughtSignature: ai.Some("false")},
	}
	thinkingAsText, err := ai.ConvertOpenAICompletionsMessages(target, ai.Context{Messages: []ai.Message{message}}, ai.OpenAICompletionsCompat{RequiresThinkingAsText: ai.Some(true)})
	if err != nil {
		t.Fatalf("thinking-as-text conversion: %v", err)
	}
	decodeFirst := func(raw []json.RawMessage) any {
		var value any
		if err := json.Unmarshal(raw[0], &value); err != nil {
			t.Fatalf("decode converted message: %v", err)
		}
		return value
	}
	return issue60NormalizedJSON(t, map[string]any{"cross_model": decodeFirst(crossModel), "thinking_as_text": decodeFirst(thinkingAsText)})
}

func issue60ProjectEvents(t *testing.T, events []ai.AssistantMessageEvent) []any {
	t.Helper()
	result := make([]any, len(events))
	for i, event := range events {
		projected := map[string]any{"type": string(event.AssistantMessageEventType())}
		switch event := event.(type) {
		case ai.AssistantMessageThinkingStartEvent:
			projected["contentIndex"] = event.ContentIndex
			projected["thinkingSignature"], _ = event.Partial.Content[event.ContentIndex].(ai.ThinkingContent).ThinkingSignature.Value()
		case ai.AssistantMessageThinkingDeltaEvent:
			block := event.Partial.Content[event.ContentIndex].(ai.ThinkingContent)
			projected["contentIndex"], projected["delta"], projected["partialThinking"] = event.ContentIndex, event.Delta, block.Thinking
			projected["thinkingSignature"], _ = block.ThinkingSignature.Value()
		case ai.AssistantMessageThinkingEndEvent:
			block := event.Partial.Content[event.ContentIndex].(ai.ThinkingContent)
			projected["contentIndex"], projected["content"], projected["partialThinking"] = event.ContentIndex, event.Content, block.Thinking
			projected["thinkingSignature"], _ = block.ThinkingSignature.Value()
		case ai.AssistantMessageToolCallStartEvent:
			projected["contentIndex"] = event.ContentIndex
		case ai.AssistantMessageToolCallDeltaEvent:
			projected["contentIndex"], projected["delta"] = event.ContentIndex, event.Delta
		case ai.AssistantMessageToolCallEndEvent:
			projected["contentIndex"] = event.ContentIndex
			if signature, ok := event.ToolCall.ThoughtSignature.Value(); ok {
				projected["thoughtSignature"] = signature
			}
		case ai.AssistantMessageTextStartEvent:
			projected["contentIndex"] = event.ContentIndex
		case ai.AssistantMessageTextDeltaEvent:
			projected["contentIndex"], projected["delta"] = event.ContentIndex, event.Delta
		case ai.AssistantMessageTextEndEvent:
			projected["contentIndex"], projected["content"] = event.ContentIndex, event.Content
		case ai.AssistantMessageDoneEvent:
			projected["reason"] = string(event.Reason)
		}
		result[i] = issue60NormalizedJSON(t, projected)
	}
	return result
}

func removeIssue60Timestamps(value any) {
	switch value := value.(type) {
	case map[string]any:
		delete(value, "timestamp")
		for _, child := range value {
			removeIssue60Timestamps(child)
		}
	case []any:
		for _, child := range value {
			removeIssue60Timestamps(child)
		}
	}
}

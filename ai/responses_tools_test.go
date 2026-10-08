package ai_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/nankedr/pig/ai"
)

func responseCall(id, args string) map[string]any {
	return map[string]any{"type": "function_call", "id": "fc-" + id, "call_id": id, "name": "lookup", "arguments": args}
}
func responseOptions(sse string) ai.OpenAIResponsesOptions {
	key := "offline"
	return ai.OpenAIResponsesOptions{StreamOptions: ai.StreamOptions{ProviderRequestOptions: ai.ProviderRequestOptions{APIKey: &key, Fetch: func(context.Context, ai.FetchRequest) (ai.FetchResponse, error) {
		return ai.FetchResponse{Status: 200, Body: []byte(sse)}, nil
	}}}}
}

func TestResponsesSDKToolArgumentPrefixesAndReasoningReplay(t *testing.T) {
	model := responsesModel("https://local.invalid")
	sse := responsesFrame(map[string]any{"type": "response.output_item.added", "output_index": 0, "item": map[string]any{"type": "reasoning", "id": "rs-42", "content": []any{}}}) +
		responsesFrame(map[string]any{"type": "response.reasoning_text.delta", "output_index": 0, "delta": "思考🙂"}) +
		responsesFrame(map[string]any{"type": "response.output_item.added", "output_index": 1, "item": responseCall("a", "")})
	args := `{"query":"hi\uD83D\uDE42","n":12}`
	for _, c := range args {
		sse += responsesFrame(map[string]any{"type": "response.function_call_arguments.delta", "output_index": 1, "item_id": "fc-a", "delta": string(c)})
	}
	sse += responsesFrame(map[string]any{"type": "response.function_call_arguments.done", "output_index": 1, "arguments": args}) +
		responsesFrame(map[string]any{"type": "response.output_item.done", "output_index": 1, "item": responseCall("a", args)}) +
		responsesFrame(map[string]any{"type": "response.completed", "response": map[string]any{"status": "completed"}})
	stream := ai.StreamOpenAIResponses(context.Background(), model, responsesInput(), responseOptions(sse))
	var prefix string
	ends := 0
	for {
		event, ok, err := stream.Next(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			break
		}
		switch e := event.(type) {
		case ai.AssistantMessageToolCallDeltaEvent:
			prefix += e.Delta
			call := e.Partial.Content[e.ContentIndex].(ai.ToolCall)
			expected := map[string]map[string]any{
				`{`:                               {},
				`{"query":`:                       {},
				`{"query":"h`:                     {"query": "h"},
				`{"query":"hi\uD83D\uDE42"`:       {"query": "hi🙂"},
				`{"query":"hi\uD83D\uDE42","n":1`: {"query": "hi🙂", "n": float64(1)},
				args:                              {"query": "hi🙂", "n": float64(12)},
			}
			if want, ok := expected[prefix]; ok && !reflect.DeepEqual(call.Arguments, want) {
				t.Fatalf("prefix %q = %#v, want %#v", prefix, call.Arguments, want)
			}
		case ai.AssistantMessageToolCallEndEvent:
			ends++
		}
	}
	out, err := stream.Result(context.Background())
	if err != nil || out.StopReason != ai.StopReasonToolUse || ends != 1 || prefix != args {
		t.Fatalf("out=%#v err=%v ends=%d prefix=%q", out, err, ends, prefix)
	}
	thinking := out.Content[0].(ai.ThinkingContent)
	signature, _ := thinking.ThinkingSignature.Value()
	if !strings.Contains(signature, "rs-42") {
		t.Fatalf("missing reasoning metadata: %#v", thinking)
	}
	input := ai.Context{Messages: []ai.Message{out, ai.ToolResultMessage{Role: ai.MessageRoleToolResult, ToolCallID: "a|fc-a", ToolName: "lookup", Content: []ai.ToolResultContent{ai.TextContent{Type: ai.ContentTypeText, Text: "result"}}}}}
	options := responseOptions(responsesFrame(map[string]any{"type": "response.completed", "response": map[string]any{"status": "completed"}}))
	options.Fetch = func(_ context.Context, r ai.FetchRequest) (ai.FetchResponse, error) {
		var b map[string]any
		json.Unmarshal(r.Body, &b)
		item := b["input"].([]any)[0].(map[string]any)
		if item["id"] != "rs-42" || item["content"].([]any)[0].(map[string]any)["text"] != "思考🙂" || item["summary"] != nil || item["encrypted_content"] != nil {
			t.Errorf("reasoning replay=%#v", item)
		}
		return ai.FetchResponse{Status: 200, Body: []byte(responsesFrame(map[string]any{"type": "response.completed", "response": map[string]any{"status": "completed"}}))}, nil
	}
	if _, err := ai.Complete(context.Background(), model, input, options); err != nil {
		t.Fatal(err)
	}
}

func TestResponsesSDKRejectsBrokenToolPairingBeforeTransport(t *testing.T) {
	model := responsesModel("https://local.invalid")
	call := ai.ToolCall{Type: ai.ContentTypeToolCall, ID: "a", Name: "lookup", Arguments: map[string]any{}}
	assistant := ai.AssistantMessage{Role: ai.MessageRoleAssistant, API: model.API, Provider: model.Provider, Model: model.ID, StopReason: ai.StopReasonToolUse, Content: []ai.AssistantContent{call}}
	result := ai.ToolResultMessage{Role: ai.MessageRoleToolResult, ToolCallID: "a|fc-a", ToolName: "lookup", Content: []ai.ToolResultContent{ai.TextContent{Type: ai.ContentTypeText, Text: "value"}}}
	for _, messages := range [][]ai.Message{{result}, {assistant, result, result}, {assistant, assistant}} {
		var fetches atomic.Int32
		options := responseOptions("")
		options.Fetch = func(context.Context, ai.FetchRequest) (ai.FetchResponse, error) {
			fetches.Add(1)
			return ai.FetchResponse{}, nil
		}
		if _, err := ai.Complete(context.Background(), model, ai.Context{Messages: messages}, options); err == nil || fetches.Load() != 0 {
			t.Fatalf("invalid pairing accepted: err=%v fetches=%d", err, fetches.Load())
		}
	}
}

func TestResponsesSDKMissingResultAndUnsupportedOptions(t *testing.T) {
	model := responsesModel("https://local.invalid")
	assistant := ai.AssistantMessage{Role: ai.MessageRoleAssistant, API: model.API, Provider: model.Provider, Model: model.ID, StopReason: ai.StopReasonToolUse, Content: []ai.AssistantContent{ai.ToolCall{Type: ai.ContentTypeToolCall, ID: "missing", Name: "lookup", Arguments: map[string]any{}}}}
	options := responseOptions("")
	options.Fetch = func(_ context.Context, r ai.FetchRequest) (ai.FetchResponse, error) {
		var b map[string]any
		json.Unmarshal(r.Body, &b)
		items := b["input"].([]any)
		if len(items) != 2 || items[1].(map[string]any)["output"] != "No result provided" {
			t.Errorf("missing result replay=%#v", items)
		}
		return ai.FetchResponse{Status: 200, Body: []byte(responsesFrame(map[string]any{"type": "response.completed", "response": map[string]any{"status": "completed"}}))}, nil
	}
	if _, err := ai.Complete(context.Background(), model, ai.Context{Messages: []ai.Message{assistant}}, options); err != nil {
		t.Fatal(err)
	}
	var fetches atomic.Int32
	options.Fetch = func(context.Context, ai.FetchRequest) (ai.FetchResponse, error) {
		fetches.Add(1)
		return ai.FetchResponse{}, nil
	}
	for _, choice := range []ai.OpenAIResponsesToolChoice{ai.NewOpenAIResponsesToolChoiceCustom("apply_patch"), ai.NewOpenAIResponsesToolChoiceHosted(ai.OpenAIResponsesHostedTool("web_search"))} {
		options.ToolChoice = choice
		if _, err := ai.Complete(context.Background(), model, responsesInput(), options); !errors.Is(err, ai.ErrNotImplemented) {
			t.Fatalf("choice stub=%v", err)
		}
	}
	if fetches.Load() != 0 {
		t.Fatal("stub requested transport")
	}
}

func TestResponsesSDKReplaysPlainReasoningWithoutSummaryOrEncryption(t *testing.T) {
	model := responsesModel("https://local.invalid")
	item := map[string]any{"type": "reasoning", "id": "rs-plain", "content": []any{map[string]any{"type": "reasoning_text", "text": "plain reasoning"}}, "summary": []any{map[string]any{"type": "summary_text", "text": "unused summary"}}, "encrypted_content": "unused encrypted blob"}
	sse := responsesFrame(map[string]any{"type": "response.output_item.done", "item": item}) + responsesFrame(map[string]any{"type": "response.completed", "response": map[string]any{"status": "completed", "output": []any{item}}})
	out, err := ai.Complete(context.Background(), model, responsesInput(), responseOptions(sse))
	if err != nil || out.StopReason != ai.StopReasonStop || len(out.Content) != 1 {
		t.Fatalf("out=%#v err=%v", out, err)
	}
	block := out.Content[0].(ai.ThinkingContent)
	signature, _ := block.ThinkingSignature.Value()
	if block.Thinking != "plain reasoning" || strings.Contains(signature, "unused") {
		t.Fatalf("reasoning=%#v", block)
	}
	options := responseOptions("")
	options.Fetch = func(_ context.Context, r ai.FetchRequest) (ai.FetchResponse, error) {
		var body map[string]any
		json.Unmarshal(r.Body, &body)
		reasoning := body["input"].([]any)[0].(map[string]any)
		if reasoning["encrypted_content"] != nil || reasoning["summary"] != nil || reasoning["content"].([]any)[0].(map[string]any)["text"] != "plain reasoning" {
			t.Errorf("replay=%#v", reasoning)
		}
		return ai.FetchResponse{Status: 200, Body: []byte(responsesFrame(map[string]any{"type": "response.completed", "response": map[string]any{"status": "completed"}}))}, nil
	}
	if _, err := ai.Complete(context.Background(), model, ai.Context{Messages: []ai.Message{out}}, options); err != nil {
		t.Fatal(err)
	}
}

func TestResponsesSDKIncompleteKeepsPartialToolArgumentsAndLength(t *testing.T) {
	for _, itemDone := range []bool{false, true} {
		t.Run(map[bool]string{false: "terminal output", true: "item done"}[itemDone], func(t *testing.T) {
			item := responseCall("partial", `{"query":"prefix`)
			item["status"] = "incomplete"
			sse := responsesFrame(map[string]any{"type": "response.output_item.added", "item": responseCall("partial", "")}) + responsesFrame(map[string]any{"type": "response.function_call_arguments.delta", "delta": `{"query":"prefix`})
			if itemDone {
				sse += responsesFrame(map[string]any{"type": "response.output_item.done", "item": item})
			}
			sse += responsesFrame(map[string]any{"type": "response.incomplete", "response": map[string]any{"status": "incomplete", "output": []any{item}, "incomplete_details": map[string]any{"reason": "max_output_tokens"}}})
			out, err := ai.Complete(context.Background(), responsesModel("https://local.invalid"), responsesInput(), responseOptions(sse))
			raw, _ := out.RawStopReason.Value()
			if err != nil || out.StopReason != ai.StopReasonLength || raw != "incomplete.max_output_tokens" || len(out.Content) != 1 || out.Content[0].(ai.ToolCall).Arguments["query"] != "prefix" {
				t.Fatalf("out=%#v err=%v raw=%q", out, err, raw)
			}
		})
	}
}

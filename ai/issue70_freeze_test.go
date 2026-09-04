package ai_test

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"testing"

	"github.com/nankedr/pig/ai"
)

func TestM2ThinkingBudgetValueStates(t *testing.T) {
	for _, level := range []struct {
		name   string
		budget int64
	}{{"minimal", 1024}, {"low", 2048}, {"medium", 8192}} {
		for _, state := range []struct {
			name, json string
			want       int64
		}{{"absent", `{}`, level.budget}, {"zero", fmt.Sprintf(`{"%s":0}`, level.name), 0}, {"value", fmt.Sprintf(`{"%s":512}`, level.name), 512}} {
			t.Run(level.name+"/"+state.name, func(t *testing.T) {
				var budgets ai.ThinkingBudgets
				if err := json.Unmarshal([]byte(state.json), &budgets); err != nil {
					t.Fatal(err)
				}
				model := issue60OpenAIModel()
				model.MaxTokens = 32768
				model.Compat = ai.Some(json.RawMessage(`{"supportsThinkingTokenBudget":true,"supportsReasoningEffort":true,"thinkingFormat":"openai"}`))
				effort := ai.OpenAIReasoningEffort(level.name)
				payload := captureIssue60OpenAIPayload(t, model, ai.OpenAICompletionsOptions{ReasoningEffort: &effort, ThinkingBudgets: &budgets})
				budget, present := payload["thinking_token_budget"]
				if state.want == 0 && present || state.want != 0 && budget != float64(state.want) {
					t.Fatalf("thinking_token_budget=%v, present=%t; want %d (zero omitted)", budget, present, state.want)
				}
			})
		}
	}
}

func TestM2ThinkingLevelMapValueStates(t *testing.T) {
	for _, level := range []ai.ModelThinkingLevel{ai.ModelThinkingLevelOff, ai.ModelThinkingLevelMinimal, ai.ModelThinkingLevelLow, ai.ModelThinkingLevelMedium, ai.ModelThinkingLevelHigh, ai.ModelThinkingLevelXHigh, ai.ModelThinkingLevelMax} {
		for _, state := range []struct {
			name  string
			value ai.Optional[string]
		}{{"absent", ai.Optional[string]{}}, {"null", ai.Null[string]()}, {"empty", ai.Some("")}, {"value", ai.Some("mapped")}} {
			t.Run(string(level)+"/"+state.name, func(t *testing.T) {
				model := issue60OpenAIModel()
				model.ThinkingLevelMap = ai.ThinkingLevelMap{}
				if state.name != "absent" {
					model.ThinkingLevelMap[level] = state.value
				}
				wantSupported := state.name != "null" && (state.name != "absent" || level != ai.ModelThinkingLevelXHigh && level != ai.ModelThinkingLevelMax)
				if got := slices.Contains(ai.GetSupportedThinkingLevels(model), level); got != wantSupported {
					t.Fatalf("supported=%t, want %t", got, wantSupported)
				}
				model.Compat = ai.Some(json.RawMessage(`{"supportsReasoningEffort":true,"thinkingFormat":"openai"}`))
				var effort *ai.OpenAIReasoningEffort
				if level != ai.ModelThinkingLevelOff {
					value := ai.OpenAIReasoningEffort(level)
					effort = &value
				}
				payload := captureIssue60OpenAIPayload(t, model, ai.OpenAICompletionsOptions{ReasoningEffort: effort})
				want := string(level)
				if state.name == "empty" {
					want = ""
				}
				if state.name == "value" {
					want = "mapped"
				}
				got, present := payload["reasoning_effort"]
				if level == ai.ModelThinkingLevelOff && (state.name == "absent" || state.name == "null") {
					if present {
						t.Fatalf("off reasoning_effort=%v, want absent", got)
					}
				} else if got != want {
					t.Fatalf("reasoning_effort=%v, want %q", got, want)
				}
			})
		}
	}
}

func TestM2DeferredHandleValueStates(t *testing.T) {
	for _, fields := range []string{
		``, `,"expiresAt":0,"pollAfterMs":0,"data":null`,
		`,"expiresAt":42,"pollAfterMs":25,"data":false`,
		`,"data":0`, `,"data":""`, `,"data":{}`, `,"data":{"response":"r1"}`,
	} {
		for _, identity := range []string{`"provider":"faux","modelId":"m","api":"faux","id":"job"`, `"provider":"","modelId":"","api":"","id":""`} {
			wire := []byte(`{` + identity + fields + `}`)
			var handle ai.DeferredHandle
			if err := json.Unmarshal(wire, &handle); err != nil {
				t.Fatal(err)
			}
			message, err := ai.FauxAssistantMessage(ai.FauxAssistantText(""))
			if err != nil {
				t.Fatal(err)
			}
			message.StopReason, message.Deferred = ai.StopReasonDeferred, ai.Some(handle)
			encoded, err := ai.MarshalMessage(message)
			if err != nil {
				t.Fatal(err)
			}
			decoded, err := ai.UnmarshalMessage(encoded)
			if err != nil {
				t.Fatal(err)
			}
			got, _ := decoded.(ai.AssistantMessage).Deferred.Value()
			if !reflect.DeepEqual(got, handle) {
				t.Fatalf("handle changed: %+v, want %+v", got, handle)
			}
			encoded, err = json.Marshal(got)
			if err != nil {
				t.Fatal(err)
			}
			var actual, expected any
			if err := json.Unmarshal(encoded, &actual); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(wire, &expected); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(actual, expected) {
				t.Fatalf("handle JSON=%s, want %s", encoded, wire)
			}
		}
	}
}

func TestM2SignatureAndRedactedValueStates(t *testing.T) {
	for _, wire := range []string{`{"v":1,"id":""}`, `{"v":1,"id":"item","phase":"commentary"}`, `{"v":1,"id":"item","phase":"final_answer"}`} {
		var signature ai.TextSignatureV1
		if err := json.Unmarshal([]byte(wire), &signature); err != nil {
			t.Fatal(err)
		}
		encoded, err := json.Marshal(signature)
		if err != nil || string(encoded) != wire {
			t.Fatalf("signature=%s, err=%v; want %s", encoded, err, wire)
		}
		for _, redacted := range []ai.Optional[bool]{{}, ai.Some(false), ai.Some(true)} {
			message, err := ai.FauxAssistantMessage(ai.FauxAssistantText("answer"))
			if err != nil {
				t.Fatal(err)
			}
			message.Content = []ai.AssistantContent{ai.TextContent{Type: ai.ContentTypeText, Text: "answer", TextSignature: ai.Some(wire)}, ai.ThinkingContent{Type: ai.ContentTypeThinking, Thinking: "reason", Redacted: redacted}}
			core, err := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
			if err != nil {
				t.Fatal(err)
			}
			core.SetResponses([]ai.FauxResponseStep{message})
			model, _ := core.GetModel()
			result, err := core.StreamSimple(context.Background(), model, ai.Context{}, ai.SimpleStreamOptions{}).Result(context.Background())
			if err != nil || len(result.Content) != 2 {
				t.Fatalf("result=%+v, err=%v", result, err)
			}
			if result.Content[0].(ai.TextContent).TextSignature != ai.Some(wire) || result.Content[1].(ai.ThinkingContent).Redacted != redacted {
				t.Fatal("signature/redacted state lost")
			}
		}
	}
}

func TestM2ReasoningDetailsShapesAndConsumption(t *testing.T) {
	for _, test := range []struct {
		details string
		signed  bool
	}{{`null`, false}, {`{}`, false}, {`"ignored"`, false}, {`false`, false}, {`0`, false}, {`[]`, false}, {`[{"type":"reasoning.encrypted","id":"same","data":"secret"}]`, true}} {
		details := test.details
		sse := `data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"name":"first","arguments":"{}"}},{"index":1,"function":{"name":"second","arguments":"{}"}}]}}]}` + "\n\n" +
			`data: {"choices":[{"delta":{"reasoning_details":` + details + `}}]}` + "\n\n" +
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"same"},{"index":1,"id":"same"}]},"finish_reason":"tool_calls"}]}` + "\n\ndata: [DONE]\n\n"
		key := "test-key"
		result, err := ai.StreamOpenAICompletions(context.Background(), issue60OpenAIModel(), ai.Context{}, ai.OpenAICompletionsOptions{StreamOptions: ai.StreamOptions{ProviderRequestOptions: ai.ProviderRequestOptions{
			APIKey: &key, Fetch: func(context.Context, ai.FetchRequest) (ai.FetchResponse, error) {
				return ai.FetchResponse{Status: 200, Body: []byte(sse)}, nil
			},
		}}}).Result(context.Background())
		if err != nil || result.StopReason != ai.StopReasonToolUse || len(result.Content) != 2 {
			t.Fatalf("details=%s: %+v, %v", details, result, err)
		}
		first, second := result.Content[0].(ai.ToolCall), result.Content[1].(ai.ToolCall)
		if first.ThoughtSignature.IsSet() != test.signed || second.ThoughtSignature.IsSet() {
			t.Fatalf("details=%s: signatures=%+v, %+v", details, first.ThoughtSignature, second.ThoughtSignature)
		}
	}
}

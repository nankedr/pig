package ai_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"

	"github.com/nankedr/pig/ai"
)

type deprecatedAlias struct {
	api     ai.API
	options ai.ProviderStreamOptions
	stream  ai.CompatAPIStreamFunction
	simple  ai.CompatAPISimpleStreamFunction
}

func aliasStream[T ai.ProviderStreamOptions](stream func(context.Context, ai.Model, ai.Context, T) *ai.AssistantMessageEventStream) ai.CompatAPIStreamFunction {
	return func(ctx context.Context, model ai.Model, input ai.Context, options ai.ProviderStreamOptions) *ai.AssistantMessageEventStream {
		return stream(ctx, model, input, options.(T))
	}
}

func deprecatedAliases() []deprecatedAlias {
	session, zero, disabled := "alias-session", int64(0), false
	base := ai.StreamOptions{SessionID: &session, MaxTokens: &zero, Metadata: map[string]json.RawMessage{"extension": json.RawMessage(`null`)}, ProviderRequestOptions: ai.ProviderRequestOptions{Headers: ai.ProviderHeaders{"removed": nil, "set": &session}}}
	return []deprecatedAlias{
		{ai.APIAnthropicMessages, ai.AnthropicOptions{StreamOptions: base, ThinkingEnabled: &disabled}, aliasStream(ai.StreamAnthropic), ai.StreamSimpleAnthropic},
		{ai.APIAzureOpenAIResponses, ai.AzureOpenAIResponsesOptions{StreamOptions: base, AzureDeploymentName: &session}, aliasStream(ai.StreamAzureOpenAIResponses), ai.StreamSimpleAzureOpenAIResponses},
		{ai.APIGoogleGenerativeAI, ai.GoogleOptions{StreamOptions: base, Thinking: &ai.GoogleThinkingOptions{}}, aliasStream(ai.StreamGoogle), ai.StreamSimpleGoogle},
		{ai.APIGoogleVertex, ai.GoogleVertexOptions{StreamOptions: base, Project: &session}, aliasStream(ai.StreamGoogleVertex), ai.StreamSimpleGoogleVertex},
		{ai.APIMistralConversations, ai.MistralOptions{StreamOptions: base, ToolChoice: ai.MistralToolChoiceAuto}, aliasStream(ai.StreamMistral), ai.StreamSimpleMistral},
		{ai.APIOpenAICodexResponses, ai.OpenAICodexResponsesOptions{StreamOptions: base, ReasoningSummary: ai.Null[ai.CodexReasoningSummary]()}, aliasStream(ai.StreamOpenAICodexResponses), ai.StreamSimpleOpenAICodexResponses},
		{ai.APIOpenAICompletions, ai.OpenAICompletionsOptions{StreamOptions: base, ThinkingBudgets: &ai.ThinkingBudgets{High: &zero}}, aliasStream(ai.StreamOpenAICompletions), ai.StreamSimpleOpenAICompletions},
		{ai.APIOpenAIResponses, ai.OpenAIResponsesOptions{StreamOptions: base, ReasoningSummary: ai.Null[ai.OpenAIReasoningSummary]()}, aliasStream(ai.StreamOpenAIResponses), ai.StreamSimpleOpenAIResponses},
	}
}

func TestCompatDeprecatedAliasesShareRegisteredProvider(t *testing.T) {
	_ = ai.ResetAPIProviders()
	defer ai.ResetAPIProviders()
	for _, alias := range deprecatedAliases() {
		t.Run(string(alias.api), func(t *testing.T) {
			core, err := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{API: alias.api})
			if err != nil {
				t.Fatal(err)
			}
			model, _ := core.GetModel()
			response, _ := ai.FauxAssistantMessage(ai.FauxAssistantText("shared"), ai.FauxAssistantMessageOptions{Timestamp: ai.Some[int64](1)})
			core.SetResponses([]ai.FauxResponseStep{response})
			shared := core.Stream(context.Background(), model, ai.Context{}, ai.StreamOptions{})
			want, err := shared.Result(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			key := struct{}{}
			ctx := context.WithValue(context.Background(), key, "context")
			input := ai.Context{SystemPrompt: ai.Some("system")}
			var seen ai.ProviderStreamOptions
			var seenSimple ai.SimpleStreamOptions
			err = ai.RegisterAPIProvider(ai.APIProvider{API: alias.api,
				Stream: func(got context.Context, _ ai.Model, in ai.Context, options ai.ProviderStreamOptions) *ai.AssistantMessageEventStream {
					if got.Value(key) != "context" || !reflect.DeepEqual(in, input) {
						t.Error("context or input changed")
					}
					seen = options
					return shared
				},
				StreamSimple: func(got context.Context, _ ai.Model, in ai.Context, options ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
					if got.Value(key) != "context" || !reflect.DeepEqual(in, input) {
						t.Error("simple context or input changed")
					}
					seenSimple = options
					return shared
				},
			}, "override")
			if err != nil {
				t.Fatal(err)
			}
			if got := alias.stream(ctx, model, input, alias.options); got != shared || !reflect.DeepEqual(seen, alias.options) {
				t.Fatal("deprecated stream bypassed registry or changed options")
			}
			if got := ai.Stream(ctx, model, input, alias.options); got != shared {
				t.Fatal("compat stream changed stream identity")
			}
			got, err := ai.Complete(ctx, model, input, alias.options)
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Fatalf("Complete = %#v, %v", got, err)
			}
			session, zero, level := "session", int64(0), ai.ThinkingLevelHigh
			simple := ai.SimpleStreamOptions{StreamOptions: ai.StreamOptions{SessionID: &session, MaxTokens: &zero}, Reasoning: &level}
			if got := alias.simple(ctx, model, input, simple); got != shared || !reflect.DeepEqual(seenSimple, simple) {
				t.Fatal("deprecated simple stream bypassed registry or changed options")
			}
			if got := ai.StreamSimple(ctx, model, input, simple); got != shared {
				t.Fatal("compat simple stream changed stream identity")
			}
			got, err = ai.CompleteSimple(ctx, model, input, simple)
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Fatalf("CompleteSimple = %#v, %v", got, err)
			}
		})
	}
}

func TestCompatRegistryOrderOwnershipAndConcurrentReset(t *testing.T) {
	_ = ai.ResetAPIProviders()
	defer ai.ResetAPIProviders()
	builtin := ai.GetAPIProviders()
	first := customCompatProvider("first")
	second := customCompatProvider("second")
	_ = ai.RegisterAPIProvider(first, "old")
	_ = ai.RegisterAPIProvider(second, "second")
	_ = ai.RegisterAPIProvider(first, "new")
	ai.UnregisterAPIProviders("old")
	providers := ai.GetAPIProviders()
	if len(providers) != 12 || providers[10].API != "first" || providers[11].API != "second" {
		t.Fatalf("override order = %v", providers)
	}
	_ = ai.RegisterAPIProvider(customCompatProvider(builtin[0].API), "builtin-override")
	_ = ai.RegisterBuiltinAPIProviders()
	ai.UnregisterAPIProviders("builtin-override")
	if _, ok := ai.GetAPIProvider(builtin[0].API); ok {
		t.Fatal("builtin registration clobbered override")
	}
	_ = ai.RegisterBuiltinAPIProviders()
	providers = ai.GetAPIProviders()
	if providers[len(providers)-1].API != builtin[0].API {
		t.Fatal("restored builtin did not append")
	}
	providers[0].API = "mutated"
	if ai.GetAPIProviders()[0].API == "mutated" {
		t.Fatal("registry snapshot leaked")
	}
	var workers sync.WaitGroup
	for worker := range 6 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for i := range 30 {
				source := fmt.Sprintf("worker-%d", worker)
				_ = ai.RegisterAPIProvider(customCompatProvider(ai.API(source)), source)
				_ = ai.GetAPIProviders()
				_, _ = ai.GetAPIProvider("first")
				if i%2 == 0 {
					_ = ai.ResetAPIProviders()
				} else {
					_ = ai.RegisterBuiltinAPIProviders()
				}
				ai.UnregisterAPIProviders(source)
			}
		}()
	}
	workers.Wait()
	_ = ai.ResetAPIProviders()
	for i, provider := range ai.GetAPIProviders() {
		if provider.API != builtin[i].API {
			t.Fatal("reset order changed")
		}
	}
}

func TestCompatMissingAndNilOptions(t *testing.T) {
	_ = ai.ResetAPIProviders()
	defer ai.ResetAPIProviders()
	if _, err := ai.Complete(nil, ai.Model{API: "missing"}, ai.Context{}); err == nil || errors.Is(err, ai.ErrNotImplemented) {
		t.Fatalf("missing provider error = %v", err)
	}
	registration, err := ai.RegisterFauxProvider()
	if err != nil {
		t.Fatal(err)
	}
	defer registration.Unregister()
	model, _ := registration.GetModel()
	var options *ai.OpenAICompletionsOptions
	for _, option := range []ai.ProviderStreamOptions{nil, options} {
		if _, err := ai.Complete(nil, model, ai.Context{}, option); !errors.Is(err, ai.ErrEventStreamInvariant) {
			t.Fatalf("nil options = %v", err)
		}
	}
	if registration.State.CallCount != 0 {
		t.Fatal("invalid options consumed a response")
	}
}

func TestCompatFauxQueueIsolationAndCompletionOutcomes(t *testing.T) {
	_ = ai.ResetAPIProviders()
	defer ai.ResetAPIProviders()
	options := ai.RegisterFauxProviderOptions{API: "compat-faux", Provider: "fixture"}
	direct, err := ai.NewFauxProvider(options)
	if err != nil {
		t.Fatal(err)
	}
	old, err := ai.RegisterFauxProvider(options)
	if err != nil {
		t.Fatal(err)
	}
	defer old.Unregister()
	current, err := ai.RegisterFauxProvider(options)
	if err != nil {
		t.Fatal(err)
	}
	defer current.Unregister()
	old.Unregister()
	model, _ := current.GetModel()
	for _, reason := range []ai.StopReason{ai.StopReasonStop, ai.StopReasonError, ai.StopReasonAborted} {
		response, err := ai.FauxAssistantMessage(ai.FauxAssistantText("retained"), ai.FauxAssistantMessageOptions{StopReason: ai.Some(reason), ErrorMessage: ai.Some("failure"), Timestamp: ai.Some[int64](1)})
		if err != nil {
			t.Fatal(err)
		}
		direct.SetResponses([]ai.FauxResponseStep{response})
		current.SetResponses(nil)
		current.AppendResponses([]ai.FauxResponseStep{response})
		want, err := direct.Provider.Stream(context.Background(), model, ai.Context{}, ai.StreamOptions{}).Result(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		got, err := ai.Complete(context.Background(), model, ai.Context{})
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("%s Complete = %#v, %v; want %#v", reason, got, err, want)
		}
	}
	response, _ := ai.FauxAssistantMessage(ai.FauxAssistantText("cancelled"), ai.FauxAssistantMessageOptions{Timestamp: ai.Some[int64](1)})
	for _, simple := range []bool{false, true} {
		current.SetResponses([]ai.FauxResponseStep{response})
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		var got ai.AssistantMessage
		if simple {
			got, err = ai.CompleteSimple(ctx, model, ai.Context{})
		} else {
			got, err = ai.Complete(ctx, model, ai.Context{})
		}
		if err != nil || got.StopReason != ai.StopReasonAborted {
			t.Fatalf("cancelled Complete = %#v, %v", got, err)
		}
	}
	if current.GetPendingResponseCount() != 0 || old.State.CallCount != 0 || direct.State.CallCount != 3 {
		t.Fatal("Faux queues shared state")
	}
	current.Unregister()
	if _, err := ai.Complete(nil, model, ai.Context{}); err == nil {
		t.Fatal("unregistered Faux still callable")
	}
	fresh, err := ai.RegisterFauxProvider(options)
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Unregister()
	if fresh.State.CallCount != 0 || fresh.GetPendingResponseCount() != 0 {
		t.Fatal("new registration inherited old state")
	}
}

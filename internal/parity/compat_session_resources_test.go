package parity_test

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/internal/baseline"
	"github.com/nankedr/pig/internal/parity"
)

func loadCompatFixture(t *testing.T, name string) (parity.Fixture, parity.Baseline) {
	t.Helper()
	root := parityRepoRoot(t)
	lock, _, err := baseline.Load(filepath.Join(root, "parity", "baseline"))
	if err != nil {
		t.Fatal(err)
	}
	locked := parity.Baseline{ID: lock.BaselineID, Commit: lock.Upstream.Commit, Repository: lock.Upstream.Repository}
	fixture, err := parity.LoadFixture(filepath.Join(root, "parity", "oracle", "fixtures", name+".json"), locked)
	if err != nil {
		t.Fatal(err)
	}
	return fixture, locked
}

func TestCompatSessionResourcesParity(t *testing.T) {
	fixture, locked := loadCompatFixture(t, "compat-session-resources")
	oracle, err := parity.NewFixtureDriver(fixture, locked)
	if err != nil {
		t.Fatal(err)
	}
	var input struct {
		API           ai.API
		Provider      ai.ProviderID
		Text, Session string
	}
	if err := json.Unmarshal(fixture.Case.Input, &input); err != nil {
		t.Fatal(err)
	}
	_ = ai.ResetAPIProviders()
	defer ai.ResetAPIProviders()
	apis := func() []ai.API {
		result := []ai.API{}
		for _, p := range ai.GetAPIProviders() {
			result = append(result, p.API)
		}
		return result
	}
	core, err := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{API: input.API, Provider: input.Provider})
	if err != nil {
		t.Fatal(err)
	}
	register := func(api ai.API, source string) {
		t.Helper()
		err := ai.RegisterAPIProvider(ai.APIProvider{API: api,
			Stream: func(ctx context.Context, model ai.Model, in ai.Context, _ ai.ProviderStreamOptions) *ai.AssistantMessageEventStream {
				return core.Stream(ctx, model, in, ai.StreamOptions{})
			},
			StreamSimple: ai.CompatAPISimpleStreamFunction(core.StreamSimple),
		}, source)
		if err != nil {
			t.Fatal(err)
		}
	}
	initial := apis()
	register("first", "old")
	register("second", "second")
	register("first", "new")
	ai.UnregisterAPIProviders("old")
	overwritten := apis()
	register(initial[0], "override")
	_ = ai.RegisterBuiltinAPIProviders()
	ai.UnregisterAPIProviders("override")
	removed := apis()
	_ = ai.RegisterBuiltinAPIProviders()
	restored := apis()
	_ = ai.ResetAPIProviders()
	reset := apis()
	size := 100
	options := ai.RegisterFauxProviderOptions{API: input.API, Provider: input.Provider, TokenSize: &ai.FauxTokenSize{Min: &size, Max: &size}}
	old, err := ai.RegisterFauxProvider(options)
	if err != nil {
		t.Fatal(err)
	}
	defer old.Unregister()
	registered, err := ai.RegisterFauxProvider(options)
	if err != nil {
		t.Fatal(err)
	}
	defer registered.Unregister()
	old.Unregister()
	model, _ := registered.GetModel()
	reply, err := ai.FauxAssistantMessage(ai.FauxAssistantText(input.Text), ai.FauxAssistantMessageOptions{Timestamp: ai.Some[int64](1)})
	if err != nil {
		t.Fatal(err)
	}
	factoryOptions := []map[string]any{}
	factory := ai.FauxResponseFactory(func(_ ai.Context, options *ai.SimpleStreamOptions, state *ai.FauxProviderState, _ ai.Model) (ai.AssistantMessage, error) {
		factoryOptions = append(factoryOptions, map[string]any{"session": *options.SessionID, "maxTokens": *options.MaxTokens, "reasoning": *options.Reasoning, "count": state.CallCount})
		return reply, nil
	})
	registered.SetResponses([]ai.FauxResponseStep{reply})
	registered.AppendResponses([]ai.FauxResponseStep{factory, reply, reply, reply})
	pendingBefore := registered.GetPendingResponseCount()
	in := ai.Context{Messages: []ai.Message{ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText("prompt"), Timestamp: 1}}}
	ctx := context.Background()
	stream := ai.Stream(ctx, model, in, ai.StreamOptions{SessionID: &input.Session})
	events := []ai.AssistantMessageEventType{}
	for {
		event, ok, err := stream.Next(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			break
		}
		events = append(events, event.AssistantMessageEventType())
	}
	first, err := stream.Result(ctx)
	if err != nil {
		t.Fatal(err)
	}
	zero, level, other := int64(0), ai.ThinkingLevelHigh, "two"
	second, err := ai.CompleteSimple(ctx, model, in, ai.SimpleStreamOptions{StreamOptions: ai.StreamOptions{SessionID: &input.Session, MaxTokens: &zero}, Reasoning: &level})
	if err != nil {
		t.Fatal(err)
	}
	third, err := ai.Complete(ctx, model, in, ai.StreamOptions{SessionID: &other})
	if err != nil {
		t.Fatal(err)
	}
	direct, err := ai.NewFauxProvider(options)
	if err != nil {
		t.Fatal(err)
	}
	direct.SetResponses([]ai.FauxResponseStep{reply})
	directModel, _ := direct.GetModel()
	directResult, err := direct.Provider.Stream(ctx, directModel, in, ai.StreamOptions{SessionID: &other}).Result(ctx)
	if err != nil {
		t.Fatal(err)
	}
	results := []map[string]any{}
	for _, message := range []ai.AssistantMessage{first, second, third, directResult} {
		results = append(results, map[string]any{"text": message.Content[0].(ai.TextContent).Text, "reason": message.StopReason, "cacheRead": message.Usage.CacheRead})
	}
	pendingAfter := registered.GetPendingResponseCount()
	registered.Unregister()
	registered.Unregister()
	_, missingErr := ai.Complete(ctx, model, in)
	fresh, err := ai.RegisterFauxProvider(options)
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Unregister()
	freshState := map[string]int{"calls": fresh.State.CallCount, "pending": fresh.GetPendingResponseCount()}
	fresh.Unregister()
	calls := []string{}
	failure := errors.New("failure")
	cleanup := func(name string, fail bool) func() {
		t.Helper()
		remove, err := ai.RegisterSessionResourceCleanup(func(ids ...string) {
			id := "<all>"
			if len(ids) > 0 {
				id = ids[0]
			}
			calls = append(calls, name+":"+id)
			if fail {
				panic(failure)
			}
		})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(remove)
		return remove
	}
	removeFirst := cleanup("first", false)
	removeFailure := cleanup("failure", true)
	removeLast := cleanup("last", false)
	failures := [][]string{}
	for range 2 {
		err := ai.CleanupSessionResources(input.Session)
		if !errors.Is(err, failure) {
			t.Fatalf("cleanup error = %v", err)
		}
		failures = append(failures, []string{failure.Error()})
	}
	removeFailure()
	removeFailure()
	if err := ai.CleanupSessionResources("two"); err != nil {
		t.Fatal(err)
	}
	if err := ai.CleanupSessionResources(); err != nil {
		t.Fatal(err)
	}
	removeFirst()
	removeLast()
	outcome, err := json.Marshal(map[string]any{
		"registry": map[string]any{"initial": initial, "overwritten": overwritten, "removed": removed, "restored": restored, "reset": reset},
		"faux":     map[string]any{"events": events, "results": results, "factoryOptions": factoryOptions, "pendingBefore": pendingBefore, "pendingAfter": pendingAfter, "calls": registered.State.CallCount, "oldCalls": old.State.CallCount, "missing": missingErr != nil, "fresh": freshState},
		"cleanup":  map[string]any{"calls": calls, "failures": failures},
	})
	if err != nil {
		t.Fatal(err)
	}
	sideEffects := []parity.SideEffect{}
	result, err := parity.RunCase(ctx, fixture.Case, oracle, parity.DriverFunc{SurfaceName: parity.SurfaceGoSDK, ObserveFunc: func(context.Context, parity.Case) (parity.Observation, error) {
		return parity.Observation{Outcome: outcome, SideEffects: &sideEffects}, nil
	}})
	if err != nil || !result.Match || len(result.Normalizations) != 0 {
		t.Fatalf("compat parity = %+v, %v", result, err)
	}
}

func TestCompatSessionResourcesDeviation(t *testing.T) {
	fixture, _ := loadCompatFixture(t, "compat-session-resources-deviation")
	var upstream map[string]any
	if err := json.Unmarshal(fixture.Observation.Outcome, &upstream); err != nil {
		t.Fatal(err)
	}
	wantPi := map[string]any{"compat": "registered", "alias": "adapter", "mutation": []any{"first", "added"}}
	if !reflect.DeepEqual(upstream, wantPi) {
		t.Fatalf("Pi deviation = %v", upstream)
	}
	_ = ai.ResetAPIProviders()
	defer ai.ResetAPIProviders()
	registration, err := ai.RegisterFauxProvider(ai.RegisterFauxProviderOptions{API: ai.APIOpenAICompletions, Provider: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	defer registration.Unregister()
	model, _ := registration.GetModel()
	reply, _ := ai.FauxAssistantMessage(ai.FauxAssistantText("registered"), ai.FauxAssistantMessageOptions{Timestamp: ai.Some[int64](1)})
	registration.SetResponses([]ai.FauxResponseStep{reply, reply})
	key, zero := "fixture", 0
	request := ai.StreamOptions{ProviderRequestOptions: ai.ProviderRequestOptions{APIKey: &key, MaxRetries: &zero, Fetch: func(context.Context, ai.FetchRequest) (ai.FetchResponse, error) {
		return ai.FetchResponse{Status: 200, Headers: map[string]string{"content-type": "text/event-stream"}, Body: []byte("data: {\"choices\":[{\"delta\":{\"content\":\"adapter\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")}, nil
	}}}
	fromCompat, err := ai.Complete(context.Background(), model, ai.Context{}, request)
	if err != nil {
		t.Fatal(err)
	}
	fromAlias, err := ai.StreamOpenAICompletions(context.Background(), model, ai.Context{}, ai.OpenAICompletionsOptions{StreamOptions: request}).Result(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// Direct adapters remain independent of compatibility overrides.
	direct, err := ai.OpenAICompletionsAPI().Stream(context.Background(), model, ai.Context{}, request).Result(context.Background())
	if err != nil || direct.Content[0].(ai.TextContent).Text != "adapter" {
		t.Fatalf("direct adapter = %+v, %v", direct, err)
	}
	mutation := []string{}
	var removeTail, removeAdded func()
	removeHead, err := ai.RegisterSessionResourceCleanup(func(...string) {
		mutation = append(mutation, "first")
		removeTail()
		if removeAdded == nil {
			removeAdded, _ = ai.RegisterSessionResourceCleanup(func(...string) { mutation = append(mutation, "added") })
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	defer removeHead()
	defer func() {
		if removeAdded != nil {
			removeAdded()
		}
	}()
	removeTail, err = ai.RegisterSessionResourceCleanup(func(...string) { mutation = append(mutation, "last") })
	if err != nil {
		t.Fatal(err)
	}
	defer removeTail()
	if err := ai.CleanupSessionResources(); err != nil {
		t.Fatal(err)
	}
	if fromCompat.Content[0].(ai.TextContent).Text != "registered" || fromAlias.Content[0].(ai.TextContent).Text != "registered" || !reflect.DeepEqual(mutation, []string{"first", "last"}) {
		t.Fatalf("Pig deviation = %v / %v / %v", fromCompat, fromAlias, mutation)
	}
}

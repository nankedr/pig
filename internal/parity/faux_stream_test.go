package parity_test

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/internal/baseline"
	"github.com/nankedr/pig/internal/catalog"
	"github.com/nankedr/pig/internal/parity"
)

type fauxParityInput struct {
	API             string               `json:"api"`
	Provider        string               `json:"provider"`
	ModelID         string               `json:"model_id"`
	Network         string               `json:"network"`
	RepeatResult    int                  `json:"repeat_result"`
	TokensPerSecond float64              `json:"tokens_per_second"`
	Entrypoints     map[string]string    `json:"entrypoints"`
	Context         fauxParityContext    `json:"context"`
	TokenSize       fauxParityTokenSize  `json:"token_size"`
	Scenarios       []fauxParityScenario `json:"scenarios"`
	Projection      map[string][]string  `json:"projection"`
}

type fauxParityContext struct {
	Messages []fauxParityUserMessage `json:"messages"`
}

type fauxParityUserMessage struct {
	Role      string `json:"role"`
	Content   string `json:"content"`
	Timestamp int64  `json:"timestamp"`
}

type fauxParityTokenSize struct {
	Min int `json:"min"`
	Max int `json:"max"`
}

type fauxParityScenario struct {
	ID           string              `json:"id"`
	Method       string              `json:"method"`
	AbortAfter   string              `json:"abort_after"`
	Content      []fauxParityContent `json:"content"`
	ErrorMessage *string             `json:"error_message"`
	StopReason   string              `json:"stop_reason"`
	Timestamp    int64               `json:"timestamp"`
}

type fauxParityContent struct {
	Type      string         `json:"type"`
	Text      string         `json:"text"`
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

func TestFauxStreamOutcomeParity(t *testing.T) {
	root := parityRepoRoot(t)
	baselineDir := filepath.Join(root, "parity", "baseline")
	if err := baseline.Verify(baselineDir); err != nil {
		t.Fatal(err)
	}
	lock, _, err := baseline.Load(baselineDir)
	if err != nil {
		t.Fatal(err)
	}
	locked := parity.Baseline{ID: lock.BaselineID, Commit: lock.Upstream.Commit, Repository: lock.Upstream.Repository}
	fixture, err := parity.LoadFixture(filepath.Join(root, "parity", "oracle", "fixtures", "faux-stream-outcomes.json"), locked)
	if err != nil {
		t.Fatal(err)
	}
	oracle, err := parity.NewFixtureDriver(fixture, locked)
	if err != nil {
		t.Fatal(err)
	}
	pig := parity.DriverFunc{SurfaceName: parity.SurfaceGoSDK, ObserveFunc: observeFauxStreamOutcome}
	result, err := parity.RunCase(context.Background(), fixture.Case, oracle, pig)
	if err != nil {
		t.Fatalf("RunCase() = %v; differences=%+v", err, result.Differences)
	}
	if !result.Match || len(result.Normalizations) != 0 {
		t.Fatalf("parity result = %+v", result)
	}
	assertFauxCatalogEvidence(t, root, locked, fixture, result)
}

func observeFauxStreamOutcome(ctx context.Context, declaration parity.Case) (parity.Observation, error) {
	var input fauxParityInput
	if err := json.Unmarshal(declaration.Input, &input); err != nil {
		return parity.Observation{}, err
	}
	if input.Network != "forbidden" || input.RepeatResult != 2 || input.Entrypoints["stream"] != "Provider.stream" || input.Entrypoints["complete"] != "Models.complete" {
		return parity.Observation{}, fmt.Errorf("unsupported Faux parity declaration")
	}
	responses, err := fauxParityResponses(input.Scenarios)
	if err != nil {
		return parity.Observation{}, err
	}
	rate := input.TokensPerSecond
	minSize, maxSize := input.TokenSize.Min, input.TokenSize.Max
	handle, err := ai.NewFauxProvider(ai.RegisterFauxProviderOptions{
		API: ai.API(input.API), Provider: ai.ProviderID(input.Provider), Models: []ai.FauxModelDefinition{{ID: input.ModelID}},
		TokenSize: &ai.FauxTokenSize{Min: &minSize, Max: &maxSize}, TokensPerSecond: &rate,
	})
	if err != nil {
		return parity.Observation{}, err
	}
	handle.SetResponses(responses)
	model, ok := handle.GetModel(input.ModelID)
	if !ok {
		return parity.Observation{}, fmt.Errorf("configured model %q is unavailable", input.ModelID)
	}
	models := ai.CreateModels()
	models.SetProvider(handle.Provider)
	var fetchCalls atomic.Int64
	fetch := func(context.Context, ai.FetchRequest) (ai.FetchResponse, error) {
		fetchCalls.Add(1)
		return ai.FetchResponse{}, fmt.Errorf("unexpected Faux network request")
	}
	streamOptions := ai.StreamOptions{ProviderRequestOptions: ai.ProviderRequestOptions{Fetch: fetch}}
	modelContext := ai.Context{Messages: make([]ai.Message, len(input.Context.Messages))}
	for i, message := range input.Context.Messages {
		modelContext.Messages[i] = ai.UserMessage{
			Role: ai.MessageRole(message.Role), Content: ai.UserText(message.Content), Timestamp: message.Timestamp,
		}
	}

	events := make([]json.RawMessage, 0)
	results := make(map[string][]map[string]any)
	var completeResult map[string]any
	for _, scenario := range input.Scenarios {
		if scenario.Method == "complete" {
			message, completeErr := models.Complete(ctx, model, modelContext, ai.ModelsStreamOptions{StreamOptions: streamOptions})
			if completeErr != nil {
				return parity.Observation{}, completeErr
			}
			completeResult, err = projectFauxMessage(message)
			if err != nil {
				return parity.Observation{}, err
			}
			continue
		}
		requestCtx := ctx
		var cancel context.CancelFunc
		if scenario.AbortAfter != "" {
			requestCtx, cancel = context.WithCancel(ctx)
		}
		stream := handle.Provider.Stream(requestCtx, model, modelContext, streamOptions)
		pair, streamErr := collectFauxStream(ctx, stream, scenario, cancel, &events, input.RepeatResult)
		if cancel != nil {
			cancel()
		}
		if streamErr != nil {
			return parity.Observation{}, streamErr
		}
		results[scenario.ID] = pair
	}
	streamPair, streamOK := results["stream"]
	errorPair, errorOK := results["error"]
	abortedPair, abortedOK := results["aborted"]
	if !streamOK || !errorOK || !abortedOK || completeResult == nil {
		return parity.Observation{}, fmt.Errorf("incomplete Faux parity scenario results")
	}
	if calls := fetchCalls.Load(); calls != 0 {
		return parity.Observation{}, fmt.Errorf("Faux made %d network requests", calls)
	}
	outcome, err := json.Marshal(map[string]any{
		"stream_result":         streamPair[0],
		"stream_result_repeat":  streamPair[1],
		"complete_result":       completeResult,
		"stream_complete_equal": reflect.DeepEqual(streamPair[0], completeResult),
		"error_result":          errorPair[0],
		"error_result_repeat":   errorPair[1],
		"aborted_result":        abortedPair[0],
		"aborted_result_repeat": abortedPair[1],
		"state":                 map[string]any{"call_count": handle.State.CallCount, "pending_response_count": handle.GetPendingResponseCount()},
	})
	if err != nil {
		return parity.Observation{}, err
	}
	sideEffects := make([]parity.SideEffect, 0)
	return parity.Observation{Events: &events, Outcome: outcome, SideEffects: &sideEffects}, nil
}

func fauxParityResponses(scenarios []fauxParityScenario) ([]ai.FauxResponseStep, error) {
	responses := make([]ai.FauxResponseStep, len(scenarios))
	for i, scenario := range scenarios {
		blocks := make([]ai.FauxContentBlock, len(scenario.Content))
		for j, block := range scenario.Content {
			switch block.Type {
			case "text":
				blocks[j] = ai.FauxText(block.Text)
			case "toolCall":
				toolCall, err := ai.FauxToolCall(block.Name, block.Arguments, ai.FauxToolCallOptions{ID: ai.Some(block.ID)})
				if err != nil {
					return nil, err
				}
				blocks[j] = toolCall
			default:
				return nil, fmt.Errorf("out-of-scope Faux content %q", block.Type)
			}
		}
		options := ai.FauxAssistantMessageOptions{
			StopReason: ai.Some(ai.StopReason(scenario.StopReason)), Timestamp: ai.Some(scenario.Timestamp),
		}
		if scenario.ErrorMessage != nil {
			options.ErrorMessage = ai.Some(*scenario.ErrorMessage)
		}
		message, err := ai.FauxAssistantMessage(ai.FauxAssistantBlocks(blocks...), options)
		if err != nil {
			return nil, err
		}
		responses[i] = message
	}
	return responses, nil
}

func collectFauxStream(ctx context.Context, stream *ai.AssistantMessageEventStream, scenario fauxParityScenario, cancel context.CancelFunc, events *[]json.RawMessage, repeats int) ([]map[string]any, error) {
	aborted := false
	for {
		event, ok, err := stream.Next(ctx)
		if err != nil {
			return nil, err
		}
		if !ok {
			break
		}
		projected, err := projectFauxEvent(scenario.ID, event)
		if err != nil {
			return nil, err
		}
		raw, err := json.Marshal(projected)
		if err != nil {
			return nil, err
		}
		*events = append(*events, raw)
		if !aborted && cancel != nil && scenario.AbortAfter == string(event.AssistantMessageEventType()) {
			aborted = true
			cancel()
		}
	}
	results := make([]map[string]any, repeats)
	for i := range results {
		message, err := stream.Result(ctx)
		if err != nil {
			return nil, err
		}
		results[i], err = projectFauxMessage(message)
		if err != nil {
			return nil, err
		}
	}
	return results, nil
}

func projectFauxEvent(scenario string, event ai.AssistantMessageEvent) (map[string]any, error) {
	projected := map[string]any{"scenario": scenario, "type": string(event.AssistantMessageEventType())}
	switch event := event.(type) {
	case ai.AssistantMessageStartEvent:
		projected["partial"] = projectFauxPartial(event.Partial)
	case ai.AssistantMessageTextStartEvent:
		projected["contentIndex"], projected["partial"] = event.ContentIndex, projectFauxPartial(event.Partial)
	case ai.AssistantMessageTextDeltaEvent:
		projected["contentIndex"], projected["delta"], projected["partial"] = event.ContentIndex, event.Delta, projectFauxPartial(event.Partial)
	case ai.AssistantMessageTextEndEvent:
		projected["contentIndex"], projected["content"], projected["partial"] = event.ContentIndex, event.Content, projectFauxPartial(event.Partial)
	case ai.AssistantMessageToolCallStartEvent:
		projected["contentIndex"], projected["partial"] = event.ContentIndex, projectFauxPartial(event.Partial)
	case ai.AssistantMessageToolCallDeltaEvent:
		projected["contentIndex"], projected["delta"], projected["partial"] = event.ContentIndex, event.Delta, projectFauxPartial(event.Partial)
	case ai.AssistantMessageToolCallEndEvent:
		toolCall, err := projectFauxContent(event.ToolCall)
		if err != nil {
			return nil, err
		}
		projected["contentIndex"], projected["toolCall"], projected["partial"] = event.ContentIndex, toolCall, projectFauxPartial(event.Partial)
	case ai.AssistantMessageDoneEvent:
		message, err := projectFauxMessage(event.Message)
		if err != nil {
			return nil, err
		}
		projected["reason"], projected["message"] = string(event.Reason), message
	case ai.AssistantMessageErrorEvent:
		message, err := projectFauxMessage(event.Error)
		if err != nil {
			return nil, err
		}
		projected["reason"], projected["error"] = string(event.Reason), message
	default:
		return nil, fmt.Errorf("out-of-scope Faux event %T", event)
	}
	return projected, nil
}

func projectFauxPartial(message ai.AssistantMessage) map[string]any {
	projected := map[string]any{
		"role": string(message.Role), "api": string(message.API), "provider": string(message.Provider),
		"model": message.Model, "stopReason": string(message.StopReason),
	}
	if value, ok := message.ErrorMessage.Value(); ok {
		projected["errorMessage"] = value
	}
	return projected
}

func projectFauxMessage(message ai.AssistantMessage) (map[string]any, error) {
	content := make([]any, len(message.Content))
	for i, block := range message.Content {
		projected, err := projectFauxContent(block)
		if err != nil {
			return nil, err
		}
		content[i] = projected
	}
	projected := projectFauxPartial(message)
	projected["content"] = content
	return projected, nil
}

func projectFauxContent(content ai.AssistantContent) (map[string]any, error) {
	switch content := content.(type) {
	case ai.TextContent:
		return map[string]any{"type": string(content.Type), "text": content.Text}, nil
	case ai.ToolCall:
		return map[string]any{
			"type": string(content.Type), "id": content.ID, "name": content.Name, "arguments": content.Arguments,
		}, nil
	default:
		return nil, fmt.Errorf("out-of-scope Faux content %T", content)
	}
}

func assertFauxCatalogEvidence(t *testing.T, root string, locked parity.Baseline, fixture parity.Fixture, result parity.Result) {
	t.Helper()
	entries, err := catalog.LoadCatalog(filepath.Join(root, "parity", "catalog.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	byID := make(map[string]*catalog.Entry, len(entries))
	for i := range entries {
		byID[entries[i].ID] = &entries[i]
	}
	entry := byID[fixture.Case.CatalogID]
	if entry == nil || entry.Status != catalog.StatusImplemented || entry.Upstream.Commit != locked.Commit || entry.Upstream.Repository != locked.Repository || entry.Upstream.Reference != fixture.Upstream.Reference || !strings.Contains(entry.Notes, "dist") {
		t.Fatalf("catalog entry is not honestly bound to source-only fixture: %+v", entry)
	}
	pigHash, err := parity.HashObservation(result.Pig)
	if err != nil {
		t.Fatal(err)
	}
	evidence := make(map[string]catalog.Evidence, len(entry.Evidence))
	for _, item := range entry.Evidence {
		evidence[item.Kind] = item
	}
	oracle := evidence[catalog.MatrixEvidenceOracle]
	if oracle.Ref != "parity/oracle/fixtures/faux-stream-outcomes.json" || oracle.Baseline != locked.Commit || oracle.CaseID != fixture.Case.ID || oracle.InputHash != fixture.InputHash || oracle.Platform != fixture.Platform || oracle.CatalogID != entry.ID || !strings.Contains(oracle.Expected, fixture.ObservationHash) || !strings.Contains(oracle.Actual, fixture.ObservationHash) {
		t.Errorf("oracle evidence does not bind fixture: %+v", oracle)
	}
	goTest := evidence[catalog.MatrixEvidenceGoTest]
	if goTest.Ref != "internal/parity/faux_stream_test.go#TestFauxStreamOutcomeParity" || goTest.Baseline != locked.Commit || goTest.CaseID != fixture.Case.ID || goTest.InputHash != fixture.InputHash || goTest.ExecutionMethod != "go test ./internal/parity -run '^TestFauxStreamOutcomeParity$' -count=1" || goTest.Platform != fixture.Platform || goTest.CatalogID != entry.ID || !strings.Contains(goTest.Expected, fixture.ObservationHash) || !strings.Contains(goTest.Actual, pigHash) {
		t.Errorf("Go test evidence does not bind result: %+v", goTest)
	}
	for _, id := range []string{"contract:ai/faux-provider", "contract:ai/event-stream", "contract:ai/compat", "contract:ai/models-runtime"} {
		partial := byID[id]
		if partial == nil || partial.Status != catalog.StatusPartial || partial.Partial == nil {
			t.Errorf("related capability %s is not partial: %+v", id, partial)
		}
	}
}

package parity_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/internal/baseline"
	"github.com/nankedr/pig/internal/catalog"
	"github.com/nankedr/pig/internal/parity"
)

type fauxFactoryStateInput struct {
	API        string `json:"api"`
	Calls      int    `json:"calls"`
	Entrypoint string `json:"entrypoint"`
	ModelID    string `json:"model_id"`
	Network    string `json:"network"`
	Provider   string `json:"provider"`
}

type fauxFactoryStateOutcome struct {
	FactoryCallCounts []int `json:"factory_call_counts"`
	ProviderCallCount int   `json:"provider_call_count"`
}

func TestFauxFactoryStateSnapshotDeviation(t *testing.T) {
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
	fixture, err := parity.LoadFixture(
		filepath.Join(root, "parity", "oracle", "fixtures", "faux-factory-state-snapshot-deviation.json"),
		locked,
	)
	if err != nil {
		t.Fatal(err)
	}
	if fixture.Case.ID != "go-sdk/ai/faux-factory-state-snapshot-deviation" || fixture.Case.CatalogID != "contract:ai/faux-provider" {
		t.Fatalf("fixture Case = %+v", fixture.Case)
	}

	oracleOutcome := assertFauxFactoryStateObservation(t, "Pi", fixture.Observation, fauxFactoryStateOutcome{
		FactoryCallCounts: []int{2, 2}, ProviderCallCount: 2,
	})
	pigObservation, err := observeFauxFactoryState(context.Background(), fixture.Case)
	if err != nil {
		t.Fatal(err)
	}
	pigOutcome := assertFauxFactoryStateObservation(t, "Pig", pigObservation, fauxFactoryStateOutcome{
		FactoryCallCounts: []int{1, 2}, ProviderCallCount: 2,
	})
	if reflect.DeepEqual(oracleOutcome, pigOutcome) {
		t.Fatal("Pi shared state and Pig per-stream snapshots unexpectedly matched")
	}
	assertFauxFactoryStateCatalogEvidence(t, root, locked, fixture, pigObservation)
}

func observeFauxFactoryState(ctx context.Context, declaration parity.Case) (parity.Observation, error) {
	var input fauxFactoryStateInput
	if err := json.Unmarshal(declaration.Input, &input); err != nil {
		return parity.Observation{}, err
	}
	if input.Calls != 2 || input.Entrypoint != "createFauxCore.stream" || input.Network != "forbidden" {
		return parity.Observation{}, fmt.Errorf("unsupported Faux factory-state declaration")
	}
	core, err := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{
		API: ai.API(input.API), Provider: ai.ProviderID(input.Provider), Models: []ai.FauxModelDefinition{{ID: input.ModelID}},
	})
	if err != nil {
		return parity.Observation{}, err
	}
	release := make(chan struct{})
	factory := ai.FauxResponseFactory(func(_ ai.Context, _ *ai.SimpleStreamOptions, state *ai.FauxProviderState, _ ai.Model) (ai.AssistantMessage, error) {
		<-release
		return ai.FauxAssistantMessage(
			ai.FauxAssistantText(strconv.Itoa(state.CallCount)),
			ai.FauxAssistantMessageOptions{Timestamp: ai.Some[int64](1)},
		)
	})
	responses := make([]ai.FauxResponseStep, input.Calls)
	for i := range responses {
		responses[i] = factory
	}
	core.SetResponses(responses)
	model, ok := core.GetModel(input.ModelID)
	if !ok {
		return parity.Observation{}, fmt.Errorf("configured model %q is unavailable", input.ModelID)
	}
	var fetchCalls atomic.Int64
	options := ai.StreamOptions{ProviderRequestOptions: ai.ProviderRequestOptions{
		Fetch: func(context.Context, ai.FetchRequest) (ai.FetchResponse, error) {
			fetchCalls.Add(1)
			return ai.FetchResponse{}, fmt.Errorf("unexpected Faux network request")
		},
	}}
	streams := make([]*ai.AssistantMessageEventStream, input.Calls)
	for i := range streams {
		streams[i] = core.Stream(ctx, model, ai.Context{}, options)
	}
	close(release)

	counts := make([]int, len(streams))
	for i, stream := range streams {
		message, resultErr := stream.Result(ctx)
		if resultErr != nil {
			return parity.Observation{}, resultErr
		}
		if len(message.Content) != 1 {
			return parity.Observation{}, fmt.Errorf("factory result %d has %d content blocks", i, len(message.Content))
		}
		text, ok := message.Content[0].(ai.TextContent)
		if !ok {
			return parity.Observation{}, fmt.Errorf("factory result %d content = %T", i, message.Content[0])
		}
		counts[i], err = strconv.Atoi(text.Text)
		if err != nil {
			return parity.Observation{}, err
		}
	}
	if calls := fetchCalls.Load(); calls != 0 {
		return parity.Observation{}, fmt.Errorf("Faux made %d network requests", calls)
	}
	outcome, err := json.Marshal(fauxFactoryStateOutcome{
		FactoryCallCounts: counts, ProviderCallCount: core.State.CallCount,
	})
	if err != nil {
		return parity.Observation{}, err
	}
	sideEffects := make([]parity.SideEffect, 0)
	return parity.Observation{Outcome: outcome, SideEffects: &sideEffects}, nil
}

func assertFauxFactoryStateObservation(t *testing.T, label string, observation parity.Observation, want fauxFactoryStateOutcome) fauxFactoryStateOutcome {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(observation.Outcome))
	decoder.DisallowUnknownFields()
	var outcome fauxFactoryStateOutcome
	if err := decoder.Decode(&outcome); err != nil {
		t.Fatalf("%s outcome: %v", label, err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		t.Fatalf("%s outcome has trailing JSON: %v", label, err)
	}
	if !reflect.DeepEqual(outcome, want) {
		t.Fatalf("%s outcome = %+v, want %+v", label, outcome, want)
	}
	if observation.SideEffects == nil || len(*observation.SideEffects) != 0 {
		t.Fatalf("%s side effects = %+v, want []", label, observation.SideEffects)
	}
	extra := observation
	extra.Outcome = nil
	extra.SideEffects = nil
	if !reflect.DeepEqual(extra, parity.Observation{}) {
		t.Fatalf("%s observation has undeclared channels: %+v", label, extra)
	}
	return outcome
}

func assertFauxFactoryStateCatalogEvidence(t *testing.T, root string, locked parity.Baseline, fixture parity.Fixture, pig parity.Observation) {
	t.Helper()
	entries, err := catalog.LoadCatalog(filepath.Join(root, "parity", "catalog.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	var entry *catalog.Entry
	for i := range entries {
		if entries[i].ID == fixture.Case.CatalogID {
			entry = &entries[i]
			break
		}
	}
	if entry == nil || entry.Status != catalog.StatusPartial || entry.Deviation == nil || entry.Deviation.ADR != "ADR-0006" {
		t.Fatalf("Faux catalog deviation = %+v", entry)
	}
	if entry.Partial == nil || !strings.Contains(strings.Join(entry.Partial.Supported, " "), "per-stream factory state snapshots") {
		t.Fatalf("Faux supported branches omit state snapshots: %+v", entry.Partial)
	}
	pigHash, err := parity.HashObservation(pig)
	if err != nil {
		t.Fatal(err)
	}
	var oracleEvidence, goEvidence *catalog.Evidence
	for i := range entry.Evidence {
		evidence := &entry.Evidence[i]
		if evidence.CaseID != fixture.Case.ID {
			continue
		}
		switch evidence.Kind {
		case catalog.MatrixEvidenceOracle:
			oracleEvidence = evidence
		case catalog.MatrixEvidenceGoTest:
			goEvidence = evidence
		}
	}
	if oracleEvidence == nil || oracleEvidence.Ref != "parity/oracle/fixtures/faux-factory-state-snapshot-deviation.json" ||
		oracleEvidence.Baseline != locked.Commit || oracleEvidence.InputHash != fixture.InputHash ||
		oracleEvidence.CatalogID != entry.ID || oracleEvidence.Platform != fixture.Platform ||
		!strings.Contains(oracleEvidence.Expected, "[2,2]") || !strings.Contains(oracleEvidence.Actual, fixture.ObservationHash) {
		t.Errorf("oracle evidence does not bind factory-state fixture: %+v", oracleEvidence)
	}
	if goEvidence == nil || goEvidence.Ref != "internal/parity/faux_factory_state_test.go#TestFauxFactoryStateSnapshotDeviation" ||
		goEvidence.Baseline != locked.Commit || goEvidence.InputHash != fixture.InputHash ||
		goEvidence.ExecutionMethod != "go test -race ./internal/parity -run '^TestFauxFactoryStateSnapshotDeviation$' -count=1" ||
		goEvidence.CatalogID != entry.ID || goEvidence.Platform != fixture.Platform ||
		!strings.Contains(goEvidence.Expected, "[1,2]") || !strings.Contains(goEvidence.Actual, pigHash) {
		t.Errorf("Go evidence does not bind factory-state deviation: %+v", goEvidence)
	}
}

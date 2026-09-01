package parity_test

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/internal/baseline"
	"github.com/nankedr/pig/internal/catalog"
	"github.com/nankedr/pig/internal/parity"
)

type agentToolBatchInput struct {
	Entrypoint string `json:"entrypoint"`
	Network    string `json:"network"`
	Provider   struct {
		API     string `json:"api"`
		ID      string `json:"id"`
		ModelID string `json:"model_id"`
	} `json:"provider"`
	Prompt struct {
		Role      string `json:"role"`
		Content   string `json:"content"`
		Timestamp int64  `json:"timestamp"`
	} `json:"prompt"`
	Tool struct {
		Name  string `json:"name"`
		Calls []struct {
			ID        string         `json:"id"`
			Arguments map[string]any `json:"arguments"`
		} `json:"calls"`
	} `json:"tool"`
	TokenSize struct {
		Min int `json:"min"`
		Max int `json:"max"`
	} `json:"token_size"`
}

func TestAgentToolBatchParity(t *testing.T) {
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
	fixture, err := parity.LoadFixture(filepath.Join(root, "parity", "oracle", "fixtures", "agent-tool-batch.json"), locked)
	if err != nil {
		t.Fatal(err)
	}
	oracle, err := parity.NewFixtureDriver(fixture, locked)
	if err != nil {
		t.Fatal(err)
	}
	pig := parity.DriverFunc{SurfaceName: parity.SurfaceGoSDK, ObserveFunc: observeAgentToolBatch}
	result, err := parity.RunCase(context.Background(), fixture.Case, oracle, pig)
	if err != nil {
		t.Fatalf("RunCase() = %v; differences=%+v", err, result.Differences)
	}
	if !result.Match || len(result.Normalizations) != 0 {
		t.Fatalf("parity result = %+v", result)
	}
	assertAgentToolBatchCatalogEvidence(t, root, locked, fixture, result)
}

func observeAgentToolBatch(ctx context.Context, declaration parity.Case) (parity.Observation, error) {
	var input agentToolBatchInput
	if err := json.Unmarshal(declaration.Input, &input); err != nil {
		return parity.Observation{}, err
	}
	if input.Entrypoint != "runAgentLoop" || input.Network != "forbidden" || len(input.Tool.Calls) != 2 {
		return parity.Observation{}, fmt.Errorf("unsupported Agent Tool batch parity declaration")
	}
	releaseFirst := make(chan struct{})
	var releaseOnce sync.Once
	var executionCount atomic.Int64
	tool, err := agent.EraseAgentTool(agent.AgentTool[map[string]any, map[string]any]{
		Tool: ai.Tool{
			Name: input.Tool.Name, Description: "Run deterministic controlled work",
			Parameters: json.RawMessage(`{"type":"object","required":["value"],"properties":{"value":{"type":"string"}}}`),
		},
		DecodeValidated: func(value ai.JSONValue) map[string]any { return value.(map[string]any) },
		Execute: func(_ context.Context, toolCallID string, arguments map[string]any, _ agent.AgentToolUpdateCallback[map[string]any]) (agent.AgentToolResult[map[string]any], error) {
			executionCount.Add(1)
			if toolCallID == "call-1" {
				<-releaseFirst
			}
			value := arguments["value"].(string)
			return agent.AgentToolResult[map[string]any]{
				Content: []ai.ToolResultContent{ai.TextContent{Type: ai.ContentTypeText, Text: value}},
				Details: map[string]any{"value": value}, Terminate: ai.Some(true),
			}, nil
		},
	})
	if err != nil {
		return parity.Observation{}, err
	}
	blocks := make([]ai.AssistantContent, len(input.Tool.Calls))
	for i, call := range input.Tool.Calls {
		blocks[i] = agent.AgentToolCall{Type: ai.ContentTypeToolCall, ID: call.ID, Name: input.Tool.Name, Arguments: call.Arguments}
	}
	response, err := ai.FauxAssistantMessage(ai.FauxAssistantBlocks(blocks...), ai.FauxAssistantMessageOptions{
		StopReason: ai.Some(ai.StopReasonToolUse), Timestamp: ai.Some(int64(2)),
	})
	if err != nil {
		return parity.Observation{}, err
	}
	minSize, maxSize := input.TokenSize.Min, input.TokenSize.Max
	core, err := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{
		API: ai.API(input.Provider.API), Provider: ai.ProviderID(input.Provider.ID), Models: []ai.FauxModelDefinition{{ID: input.Provider.ModelID}},
		TokenSize: &ai.FauxTokenSize{Min: &minSize, Max: &maxSize},
	})
	if err != nil {
		return parity.Observation{}, err
	}
	providerContexts := make([][]map[string]any, 0, 1)
	core.SetResponses([]ai.FauxResponseStep{ai.FauxResponseFactory(func(modelContext ai.Context, _ *ai.SimpleStreamOptions, _ *ai.FauxProviderState, _ ai.Model) (ai.AssistantMessage, error) {
		projected, err := projectAgentContinuationMessages(modelContext.Messages)
		if err != nil {
			return ai.AssistantMessage{}, err
		}
		providerContexts = append(providerContexts, projected)
		return response, nil
	})})
	model, _ := core.GetModel()
	preflights := make([]string, 0, len(input.Tool.Calls))
	events := make([]json.RawMessage, 0)
	messages, err := agent.RunAgentLoop(
		ctx,
		[]agent.AgentMessage{ai.UserMessage{Role: ai.MessageRole(input.Prompt.Role), Content: ai.UserText(input.Prompt.Content), Timestamp: input.Prompt.Timestamp}},
		agent.AgentContext{Tools: []agent.ErasedAgentTool{tool}},
		agent.AgentLoopConfig{
			Model: model, ToolExecution: agent.ToolExecutionParallel,
			BeforeToolCall: func(_ context.Context, input agent.BeforeToolCallContext) (*agent.BeforeToolCallResult, error) {
				preflights = append(preflights, input.ToolCall.ID)
				return nil, nil
			},
		},
		func(_ context.Context, event agent.AgentEvent) error {
			projected, err := projectAgentContinuationEvent(event)
			if err != nil {
				return err
			}
			encoded, err := json.Marshal(projected)
			if err != nil {
				return err
			}
			events = append(events, encoded)
			if end, ok := event.(agent.ToolExecutionEndEvent); ok && end.ToolCallID == "call-2" {
				releaseOnce.Do(func() { close(releaseFirst) })
			}
			return nil
		},
		agent.StreamFunction(core.StreamSimple),
	)
	if err != nil {
		return parity.Observation{}, err
	}
	projectedMessages, err := projectAgentContinuationMessages(messages)
	if err != nil {
		return parity.Observation{}, err
	}
	outcome, err := json.Marshal(map[string]any{
		"messages": projectedMessages, "providerContexts": providerContexts, "preflights": preflights, "executionCount": executionCount.Load(),
		"state": map[string]any{"callCount": core.State.CallCount, "pendingResponseCount": core.GetPendingResponseCount()},
	})
	if err != nil {
		return parity.Observation{}, err
	}
	sideEffects := []parity.SideEffect{}
	return parity.Observation{Events: &events, Outcome: outcome, SideEffects: &sideEffects}, nil
}

func assertAgentToolBatchCatalogEvidence(t *testing.T, root string, locked parity.Baseline, fixture parity.Fixture, result parity.Result) {
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
	if entry == nil || entry.Status != catalog.StatusPartial || entry.Upstream.Commit != locked.Commit || entry.Upstream.Repository != locked.Repository || entry.Upstream.Reference != fixture.Upstream.Reference {
		t.Fatalf("catalog entry is not bound to Agent Tool batch fixture: %+v", entry)
	}
	pigHash, err := parity.HashObservation(result.Pig)
	if err != nil {
		t.Fatal(err)
	}
	var oracleEvidence, goEvidence *catalog.Evidence
	for i := range entry.Evidence {
		switch entry.Evidence[i].CaseID {
		case fixture.Case.ID + "-oracle":
			oracleEvidence = &entry.Evidence[i]
		case fixture.Case.ID + "-pig":
			goEvidence = &entry.Evidence[i]
		}
	}
	if oracleEvidence == nil || oracleEvidence.Kind != catalog.MatrixEvidenceOracle || oracleEvidence.Ref != "parity/oracle/fixtures/agent-tool-batch.json" || oracleEvidence.Baseline != locked.Commit || oracleEvidence.InputHash != fixture.InputHash || oracleEvidence.Platform != fixture.Platform || oracleEvidence.CatalogID != entry.ID || !strings.Contains(oracleEvidence.Expected, fixture.ObservationHash) || !strings.Contains(oracleEvidence.Actual, fixture.ObservationHash) {
		t.Errorf("oracle evidence does not bind Agent Tool batch fixture: %+v", oracleEvidence)
	}
	if goEvidence == nil || goEvidence.Kind != catalog.MatrixEvidenceGoTest || goEvidence.Ref != "internal/parity/agent_tool_batch_test.go#TestAgentToolBatchParity" || goEvidence.Baseline != locked.Commit || goEvidence.InputHash != fixture.InputHash || goEvidence.ExecutionMethod != "go test ./internal/parity -run '^TestAgentToolBatchParity$' -count=1" || goEvidence.Platform != fixture.Platform || goEvidence.CatalogID != entry.ID || !strings.Contains(goEvidence.Expected, fixture.ObservationHash) || !strings.Contains(goEvidence.Actual, pigHash) {
		t.Errorf("Go test evidence does not bind Agent Tool batch result: %+v", goEvidence)
	}
}

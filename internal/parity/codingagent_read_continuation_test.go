package parity_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
	"github.com/nankedr/pig/internal/baseline"
	"github.com/nankedr/pig/internal/catalog"
	"github.com/nankedr/pig/internal/parity"
)

type codingAgentReadContinuationInput struct {
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
	File struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	} `json:"file"`
	ToolCallID string `json:"tool_call_id"`
	FinalText  string `json:"final_text"`
	TokenSize  struct {
		Min int `json:"min"`
		Max int `json:"max"`
	} `json:"token_size"`
}

func TestCodingAgentReadContinuationParity(t *testing.T) {
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
	fixture, err := parity.LoadFixture(filepath.Join(root, "parity", "oracle", "fixtures", "codingagent-read-continuation.json"), locked)
	if err != nil {
		t.Fatal(err)
	}
	oracle, err := parity.NewFixtureDriver(fixture, locked)
	if err != nil {
		t.Fatal(err)
	}
	pig := parity.DriverFunc{SurfaceName: parity.SurfaceGoSDK, ObserveFunc: observeCodingAgentReadContinuation}
	result, err := parity.RunCase(context.Background(), fixture.Case, oracle, pig)
	if err != nil {
		t.Fatalf("RunCase() = %v; differences=%+v", err, result.Differences)
	}
	if !result.Match || len(result.Normalizations) != 0 {
		t.Fatalf("parity result = %+v", result)
	}
	assertCodingAgentReadCatalogEvidence(t, root, locked, fixture, result)
}

func observeCodingAgentReadContinuation(ctx context.Context, declaration parity.Case) (parity.Observation, error) {
	var input codingAgentReadContinuationInput
	if err := json.Unmarshal(declaration.Input, &input); err != nil {
		return parity.Observation{}, err
	}
	if input.Entrypoint != "createReadTool" || input.Network != "forbidden" {
		return parity.Observation{}, fmt.Errorf("unsupported Coding Agent read continuation declaration")
	}
	cwd, err := os.MkdirTemp("", "pig-read-parity-")
	if err != nil {
		return parity.Observation{}, err
	}
	defer os.RemoveAll(cwd)
	if err := os.WriteFile(filepath.Join(cwd, input.File.Path), []byte(input.File.Content), 0o600); err != nil {
		return parity.Observation{}, err
	}
	tool, err := codingagent.CreateReadTool(cwd)
	if err != nil {
		return parity.Observation{}, err
	}
	toolCall, err := ai.FauxToolCall("read", map[string]any{"path": input.File.Path}, ai.FauxToolCallOptions{ID: ai.Some(input.ToolCallID)})
	if err != nil {
		return parity.Observation{}, err
	}
	toolResponse, err := ai.FauxAssistantMessage(ai.FauxAssistantBlocks(toolCall), ai.FauxAssistantMessageOptions{StopReason: ai.Some(ai.StopReasonToolUse), Timestamp: ai.Some(int64(2))})
	if err != nil {
		return parity.Observation{}, err
	}
	finalResponse, err := ai.FauxAssistantMessage(ai.FauxAssistantText(input.FinalText), ai.FauxAssistantMessageOptions{Timestamp: ai.Some(int64(4))})
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
	providerContexts := make([][]map[string]any, 0, 2)
	core.SetResponses([]ai.FauxResponseStep{
		ai.FauxResponseFactory(func(modelContext ai.Context, _ *ai.SimpleStreamOptions, _ *ai.FauxProviderState, _ ai.Model) (ai.AssistantMessage, error) {
			projected, err := projectAgentContinuationMessages(modelContext.Messages)
			if err != nil {
				return ai.AssistantMessage{}, err
			}
			providerContexts = append(providerContexts, projected)
			return toolResponse, nil
		}),
		ai.FauxResponseFactory(func(modelContext ai.Context, _ *ai.SimpleStreamOptions, _ *ai.FauxProviderState, _ ai.Model) (ai.AssistantMessage, error) {
			projected, err := projectAgentContinuationMessages(modelContext.Messages)
			if err != nil {
				return ai.AssistantMessage{}, err
			}
			providerContexts = append(providerContexts, projected)
			result, ok := modelContext.Messages[len(modelContext.Messages)-1].(ai.ToolResultMessage)
			if !ok || len(result.Content) != 1 || result.Content[0].(ai.TextContent).Text != input.File.Content {
				return ai.AssistantMessage{}, fmt.Errorf("read ToolResult did not contain the real sentinel file")
			}
			return finalResponse, nil
		}),
	})
	model, _ := core.GetModel()
	events := make([]json.RawMessage, 0)
	messages, err := agent.RunAgentLoop(
		ctx,
		[]agent.AgentMessage{ai.UserMessage{Role: ai.MessageRole(input.Prompt.Role), Content: ai.UserText(input.Prompt.Content), Timestamp: input.Prompt.Timestamp}},
		agent.AgentContext{Tools: []agent.ErasedAgentTool{tool}},
		agent.AgentLoopConfig{Model: model},
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
	fileContent, err := os.ReadFile(filepath.Join(cwd, input.File.Path))
	if err != nil {
		return parity.Observation{}, err
	}
	outcome, err := json.Marshal(map[string]any{
		"messages": projectedMessages, "providerContexts": providerContexts,
		"file":  map[string]any{"path": input.File.Path, "content": string(fileContent)},
		"state": map[string]any{"callCount": core.State.CallCount, "pendingResponseCount": core.GetPendingResponseCount()},
	})
	if err != nil {
		return parity.Observation{}, err
	}
	sideEffects := []parity.SideEffect{}
	return parity.Observation{Events: &events, Outcome: outcome, SideEffects: &sideEffects}, nil
}

func assertCodingAgentReadCatalogEvidence(t *testing.T, root string, locked parity.Baseline, fixture parity.Fixture, result parity.Result) {
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
		t.Fatalf("catalog entry is not bound to Coding Agent read fixture: %+v", entry)
	}
	pigHash, err := parity.HashObservation(result.Pig)
	if err != nil {
		t.Fatal(err)
	}
	var oracleEvidence, goEvidence *catalog.Evidence
	for i := range entry.Evidence {
		switch {
		case entry.Evidence[i].Kind == catalog.MatrixEvidenceOracle && entry.Evidence[i].CaseID == fixture.Case.ID:
			oracleEvidence = &entry.Evidence[i]
		case entry.Evidence[i].Kind == catalog.MatrixEvidenceGoTest && entry.Evidence[i].CaseID == fixture.Case.ID:
			goEvidence = &entry.Evidence[i]
		}
	}
	if oracleEvidence == nil || oracleEvidence.Ref != "parity/oracle/fixtures/codingagent-read-continuation.json" || oracleEvidence.Baseline != locked.Commit || oracleEvidence.CaseID != fixture.Case.ID || oracleEvidence.InputHash != fixture.InputHash || oracleEvidence.Platform != fixture.Platform || oracleEvidence.CatalogID != entry.ID || !strings.Contains(oracleEvidence.Expected, fixture.ObservationHash) || !strings.Contains(oracleEvidence.Actual, fixture.ObservationHash) {
		t.Errorf("oracle evidence does not bind Coding Agent read fixture: %+v", oracleEvidence)
	}
	if goEvidence == nil || goEvidence.Ref != "internal/parity/codingagent_read_continuation_test.go#TestCodingAgentReadContinuationParity" || goEvidence.Baseline != locked.Commit || goEvidence.CaseID != fixture.Case.ID || goEvidence.InputHash != fixture.InputHash || goEvidence.ExecutionMethod != "go test ./internal/parity -run '^TestCodingAgentReadContinuationParity$' -count=1" || goEvidence.Platform != fixture.Platform || goEvidence.CatalogID != entry.ID || !strings.Contains(goEvidence.Expected, fixture.ObservationHash) || !strings.Contains(goEvidence.Actual, pigHash) {
		t.Errorf("Go evidence does not bind Coding Agent read result: %+v", goEvidence)
	}
}

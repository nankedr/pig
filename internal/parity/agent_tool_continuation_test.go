package parity_test

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/internal/baseline"
	"github.com/nankedr/pig/internal/catalog"
	"github.com/nankedr/pig/internal/parity"
)

type agentContinuationInput struct {
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
		Call struct {
			ID        string         `json:"id"`
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		} `json:"call"`
		Result struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
			Details map[string]any `json:"details"`
		} `json:"result"`
	} `json:"tool"`
	FinalText string `json:"final_text"`
	TokenSize struct {
		Min int `json:"min"`
		Max int `json:"max"`
	} `json:"token_size"`
}

func TestAgentToolContinuationParity(t *testing.T) {
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
	fixture, err := parity.LoadFixture(filepath.Join(root, "parity", "oracle", "fixtures", "agent-tool-continuation.json"), locked)
	if err != nil {
		t.Fatal(err)
	}
	oracle, err := parity.NewFixtureDriver(fixture, locked)
	if err != nil {
		t.Fatal(err)
	}
	pig := parity.DriverFunc{SurfaceName: parity.SurfaceGoSDK, ObserveFunc: observeAgentToolContinuation}
	result, err := parity.RunCase(context.Background(), fixture.Case, oracle, pig)
	if err != nil {
		t.Fatalf("RunCase() = %v; differences=%+v", err, result.Differences)
	}
	if !result.Match || len(result.Normalizations) != 0 {
		t.Fatalf("parity result = %+v", result)
	}
	assertAgentContinuationCatalogEvidence(t, root, locked, fixture, result)
}

func observeAgentToolContinuation(ctx context.Context, declaration parity.Case) (parity.Observation, error) {
	var input agentContinuationInput
	if err := json.Unmarshal(declaration.Input, &input); err != nil {
		return parity.Observation{}, err
	}
	if input.Entrypoint != "runAgentLoop" || input.Network != "forbidden" || len(input.Tool.Result.Content) != 1 || input.Tool.Result.Content[0].Type != string(ai.ContentTypeText) {
		return parity.Observation{}, fmt.Errorf("unsupported Agent continuation parity declaration")
	}
	executions := make([]map[string]any, 0, 1)
	tool, err := agent.EraseAgentTool(agent.AgentTool[map[string]any, map[string]any]{
		Tool:            ai.Tool{Name: input.Tool.Call.Name, Description: "Look up a deterministic value", Parameters: json.RawMessage(`{"type":"object","properties":{}}`)},
		DecodeValidated: func(value ai.JSONValue) map[string]any { return value.(map[string]any) },
		Execute: func(_ context.Context, toolCallID string, arguments map[string]any, _ agent.AgentToolUpdateCallback[map[string]any]) (agent.AgentToolResult[map[string]any], error) {
			executions = append(executions, map[string]any{"toolCallId": toolCallID, "args": arguments})
			return agent.AgentToolResult[map[string]any]{
				Content: []ai.ToolResultContent{ai.TextContent{Type: ai.ContentTypeText, Text: input.Tool.Result.Content[0].Text}},
				Details: input.Tool.Result.Details,
			}, nil
		},
	})
	if err != nil {
		return parity.Observation{}, err
	}
	toolCall, err := ai.FauxToolCall(input.Tool.Call.Name, input.Tool.Call.Arguments, ai.FauxToolCallOptions{ID: ai.Some(input.Tool.Call.ID)})
	if err != nil {
		return parity.Observation{}, err
	}
	firstResponse, err := ai.FauxAssistantMessage(ai.FauxAssistantBlocks(toolCall), ai.FauxAssistantMessageOptions{StopReason: ai.Some(ai.StopReasonToolUse), Timestamp: ai.Some(int64(2))})
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
			return firstResponse, nil
		}),
		ai.FauxResponseFactory(func(modelContext ai.Context, _ *ai.SimpleStreamOptions, _ *ai.FauxProviderState, _ ai.Model) (ai.AssistantMessage, error) {
			projected, err := projectAgentContinuationMessages(modelContext.Messages)
			if err != nil {
				return ai.AssistantMessage{}, err
			}
			providerContexts = append(providerContexts, projected)
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
	outcome, err := json.Marshal(map[string]any{
		"messages": projectedMessages, "providerContexts": providerContexts, "executions": executions,
		"state": map[string]any{"callCount": core.State.CallCount, "pendingResponseCount": core.GetPendingResponseCount()},
	})
	if err != nil {
		return parity.Observation{}, err
	}
	sideEffects := []parity.SideEffect{}
	return parity.Observation{Events: &events, Outcome: outcome, SideEffects: &sideEffects}, nil
}

func projectAgentContinuationEvent(event agent.AgentEvent) (map[string]any, error) {
	projected := map[string]any{"type": string(event.AgentEventType())}
	switch event := event.(type) {
	case agent.MessageStartEvent:
		message, err := projectAgentContinuationMessage(event.Message)
		if err != nil {
			return nil, err
		}
		projected["message"] = message
	case agent.MessageUpdateEvent:
		projected["assistantEventType"] = string(event.AssistantMessageEvent.AssistantMessageEventType())
	case agent.MessageEndEvent:
		message, err := projectAgentContinuationMessage(event.Message)
		if err != nil {
			return nil, err
		}
		projected["message"] = message
	case agent.ToolExecutionStartEvent:
		projected["toolCallId"], projected["toolName"], projected["args"] = event.ToolCallID, event.ToolName, event.Arguments
	case agent.ToolExecutionEndEvent:
		projected["toolCallId"], projected["toolName"], projected["isError"] = event.ToolCallID, event.ToolName, event.IsError
		result, err := projectAgentContinuationToolResult(event.Result)
		if err != nil {
			return nil, err
		}
		projected["result"] = result
	case agent.TurnEndEvent:
		message, err := projectAgentContinuationMessage(event.Message)
		if err != nil {
			return nil, err
		}
		results, err := projectAgentContinuationMessages(event.ToolResults)
		if err != nil {
			return nil, err
		}
		projected["message"], projected["toolResults"] = message, results
	case agent.AgentEndEvent:
		messages, err := projectAgentContinuationMessages(event.Messages)
		if err != nil {
			return nil, err
		}
		projected["messages"] = messages
	}
	return projected, nil
}

func projectAgentContinuationMessages[T interface{ MessageRole() ai.MessageRole }](messages []T) ([]map[string]any, error) {
	projected := make([]map[string]any, len(messages))
	for i, message := range messages {
		value, err := projectAgentContinuationMessage(message)
		if err != nil {
			return nil, err
		}
		projected[i] = value
	}
	return projected, nil
}

func projectAgentContinuationMessage(message interface{ MessageRole() ai.MessageRole }) (map[string]any, error) {
	switch message := message.(type) {
	case ai.UserMessage:
		text, ok := message.Content.Text()
		if !ok {
			return nil, fmt.Errorf("out-of-scope user content")
		}
		return map[string]any{"role": string(message.Role), "content": text}, nil
	case ai.AssistantMessage:
		content, err := projectAgentContinuationAssistantContent(message.Content)
		if err != nil {
			return nil, err
		}
		return map[string]any{"role": string(message.Role), "content": content, "stopReason": string(message.StopReason)}, nil
	case ai.ToolResultMessage:
		content, err := projectAgentContinuationToolContent(message.Content)
		if err != nil {
			return nil, err
		}
		details, _ := message.Details.Value()
		return map[string]any{
			"role": string(message.Role), "toolCallId": message.ToolCallID, "toolName": message.ToolName,
			"content": content, "details": details, "isError": message.IsError,
		}, nil
	default:
		return nil, fmt.Errorf("out-of-scope Agent continuation message %T", message)
	}
}

func projectAgentContinuationAssistantContent(content []ai.AssistantContent) ([]map[string]any, error) {
	projected := make([]map[string]any, len(content))
	for i, block := range content {
		switch block := block.(type) {
		case ai.TextContent:
			projected[i] = map[string]any{"type": string(block.Type), "text": block.Text}
		case ai.ToolCall:
			projected[i] = map[string]any{"type": string(block.Type), "id": block.ID, "name": block.Name, "arguments": block.Arguments}
		default:
			return nil, fmt.Errorf("out-of-scope Assistant content %T", block)
		}
	}
	return projected, nil
}

func projectAgentContinuationToolContent(content []ai.ToolResultContent) ([]map[string]any, error) {
	projected := make([]map[string]any, len(content))
	for i, block := range content {
		switch block := block.(type) {
		case ai.TextContent:
			projected[i] = map[string]any{"type": string(block.Type), "text": block.Text}
		default:
			return nil, fmt.Errorf("out-of-scope ToolResult content %T", block)
		}
	}
	return projected, nil
}

func projectAgentContinuationToolResult(result agent.ErasedAgentToolResult) (map[string]any, error) {
	content, err := projectAgentContinuationToolContent(result.Content)
	if err != nil {
		return nil, err
	}
	projected := map[string]any{"content": content, "details": result.Details}
	if terminate, ok := result.Terminate.Value(); ok {
		projected["terminate"] = terminate
	}
	return projected, nil
}

func assertAgentContinuationCatalogEvidence(t *testing.T, root string, locked parity.Baseline, fixture parity.Fixture, result parity.Result) {
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
		t.Fatalf("catalog entry is not bound to Agent continuation fixture: %+v", entry)
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
	if oracleEvidence == nil || oracleEvidence.Kind != catalog.MatrixEvidenceOracle || oracleEvidence.Ref != "parity/oracle/fixtures/agent-tool-continuation.json" || oracleEvidence.Baseline != locked.Commit || oracleEvidence.InputHash != fixture.InputHash || oracleEvidence.Platform != fixture.Platform || oracleEvidence.CatalogID != entry.ID || !strings.Contains(oracleEvidence.Expected, fixture.ObservationHash) || !strings.Contains(oracleEvidence.Actual, fixture.ObservationHash) {
		t.Errorf("oracle evidence does not bind Agent continuation fixture: %+v", oracleEvidence)
	}
	if goEvidence == nil || goEvidence.Kind != catalog.MatrixEvidenceGoTest || goEvidence.Ref != "internal/parity/agent_tool_continuation_test.go#TestAgentToolContinuationParity" || goEvidence.Baseline != locked.Commit || goEvidence.InputHash != fixture.InputHash || goEvidence.ExecutionMethod != "go test ./internal/parity -run '^TestAgentToolContinuationParity$' -count=1" || goEvidence.Platform != fixture.Platform || goEvidence.CatalogID != entry.ID || !strings.Contains(goEvidence.Expected, fixture.ObservationHash) || !strings.Contains(goEvidence.Actual, pigHash) {
		t.Errorf("Go test evidence does not bind Agent continuation result: %+v", goEvidence)
	}
}

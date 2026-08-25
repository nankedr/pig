package agent

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/nankedr/pig/ai"
)

func TestEraseAgentToolKeepsTypedAuthoringAndErasesValidatedExecution(t *testing.T) {
	t.Parallel()

	type parameters struct {
		Query string `json:"query"`
	}
	type details struct {
		Matches int `json:"matches"`
	}

	prepareCalls := 0
	executeCalls := 0
	descriptor := AgentTool[parameters, details]{
		Tool: ai.Tool{
			Name: "search", Description: "Search",
			Parameters: json.RawMessage(`{"type":"object","required":["query"],"properties":{"query":{"type":"string"}}}`),
		},
		Label: "Search",
		PrepareArguments: func(value ai.JSONValue) (ai.JSONValue, error) {
			prepareCalls++
			return value, nil
		},
		DecodeValidated: func(value ai.JSONValue) parameters {
			object := value.(map[string]any)
			return parameters{Query: object["query"].(string)}
		},
		Execute: func(_ context.Context, callID string, params parameters, update AgentToolUpdateCallback[details]) (AgentToolResult[details], error) {
			executeCalls++
			if callID != "call-1" || params.Query != "pig" {
				t.Fatalf("typed execution input = (%q, %#v)", callID, params)
			}
			update(AgentToolResult[details]{Details: details{Matches: 1}})
			return AgentToolResult[details]{
				Content: []ai.ToolResultContent{ai.TextContent{Type: ai.ContentTypeText, Text: "found"}},
				Details: details{Matches: 2}, Terminate: ai.Some(false),
			}, nil
		},
		ExecutionMode: ToolExecutionSequential,
	}

	erased, err := EraseAgentTool(descriptor)
	if err != nil {
		t.Fatalf("EraseAgentTool error = %v", err)
	}
	if err := validateErasedAgentTool(erased); err != nil {
		t.Fatalf("validateErasedAgentTool error = %v", err)
	}
	preparedValue, err := erased.prepareArguments(map[string]any{"query": "pig"})
	if err != nil {
		t.Fatalf("PrepareArguments error = %v", err)
	}
	prepared, err := sealValidatedAgentToolArguments(erased, preparedValue)
	if err != nil {
		t.Fatalf("sealValidatedAgentToolArguments error = %v", err)
	}
	var updates []ErasedAgentToolResult
	result, err := erased.executeValidated(context.Background(), "call-1", prepared, func(update ErasedAgentToolResult) {
		updates = append(updates, update)
	})
	if err != nil {
		t.Fatalf("ExecuteValidated error = %v", err)
	}
	if prepareCalls != 1 || executeCalls != 1 {
		t.Fatalf("calls = prepare %d, execute %d; want 1, 1", prepareCalls, executeCalls)
	}
	if erased.Name != "search" || erased.Label != "Search" || erased.ExecutionMode != ToolExecutionSequential {
		t.Fatalf("erased metadata = %#v", erased)
	}
	if len(updates) != 1 || !reflect.DeepEqual(updates[0].Details, map[string]any{"matches": float64(1)}) {
		t.Fatalf("erased updates = %#v", updates)
	}
	if !reflect.DeepEqual(result.Details, map[string]any{"matches": float64(2)}) {
		t.Fatalf("erased result details = %#v", result.Details)
	}
	if value, ok := result.Terminate.Value(); !ok || value {
		t.Fatalf("Terminate.Value() = (%t, %t), want (false, true)", value, ok)
	}
}

func TestErasedNilToolExecuteIsAnExplicitSideEffectFreeStub(t *testing.T) {
	t.Parallel()

	erased, err := EraseAgentTool(AgentTool[map[string]any, map[string]any]{
		Tool:  ai.Tool{Name: "later", Description: "Later", Parameters: json.RawMessage(`{"type":"object"}`)},
		Label: "Later",
	})
	if err != nil {
		t.Fatalf("EraseAgentTool error = %v", err)
	}
	prepared, err := sealValidatedAgentToolArguments(erased, map[string]any{})
	if err != nil {
		t.Fatalf("sealValidatedAgentToolArguments error = %v", err)
	}
	_, err = erased.executeValidated(context.Background(), "call-1", prepared, func(ErasedAgentToolResult) {
		t.Fatal("stub published a progress update")
	})
	if !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("ExecuteValidated error = %v, want ErrNotImplemented", err)
	}
	var capabilityErr *NotImplementedError
	if !errors.As(err, &capabilityErr) || capabilityErr.Module != "agent" || capabilityErr.Operation != "AgentTool.Execute" {
		t.Fatalf("ExecuteValidated error = %#v", err)
	}
}

func TestEraseAgentToolRejectsNonJSONDetailsAtTheErasedBoundary(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		execute func(AgentToolUpdateCallback[any]) AgentToolResult[any]
	}{
		{
			name: "progress",
			execute: func(update AgentToolUpdateCallback[any]) AgentToolResult[any] {
				update(AgentToolResult[any]{Details: make(chan int)})
				return AgentToolResult[any]{Details: map[string]any{"ok": true}}
			},
		},
		{
			name: "final",
			execute: func(AgentToolUpdateCallback[any]) AgentToolResult[any] {
				return AgentToolResult[any]{Details: make(chan int)}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			erased, err := EraseAgentTool(AgentTool[map[string]any, any]{
				Tool:  ai.Tool{Name: "json-only", Description: "JSON only", Parameters: json.RawMessage(`{"type":"object"}`)},
				Label: "JSON only",
				DecodeValidated: func(value ai.JSONValue) map[string]any {
					return value.(map[string]any)
				},
				Execute: func(_ context.Context, _ string, _ map[string]any, update AgentToolUpdateCallback[any]) (AgentToolResult[any], error) {
					return test.execute(update), nil
				},
			})
			if err != nil {
				t.Fatalf("EraseAgentTool error = %v", err)
			}

			updateCalls := 0
			prepared, err := sealValidatedAgentToolArguments(erased, map[string]any{})
			if err != nil {
				t.Fatalf("sealValidatedAgentToolArguments error = %v", err)
			}
			if _, err := erased.executeValidated(context.Background(), "call-1", prepared, func(ErasedAgentToolResult) {
				updateCalls++
			}); err == nil {
				t.Fatal("ExecuteValidated accepted non-JSON Tool details")
			}
			if updateCalls != 0 {
				t.Fatalf("invalid details published %d progress updates", updateCalls)
			}
		})
	}
}

func TestErasedAgentToolIgnoresUpdatesAfterExecuteReturns(t *testing.T) {
	t.Parallel()

	lateUpdate := make(chan struct{})
	lateDone := make(chan struct{})
	erased, err := EraseAgentTool(AgentTool[map[string]any, map[string]any]{
		Tool:  ai.Tool{Name: "late", Description: "Late update", Parameters: json.RawMessage(`{"type":"object"}`)},
		Label: "Late update",
		DecodeValidated: func(value ai.JSONValue) map[string]any {
			return value.(map[string]any)
		},
		Execute: func(_ context.Context, _ string, _ map[string]any, update AgentToolUpdateCallback[map[string]any]) (AgentToolResult[map[string]any], error) {
			go func() {
				defer close(lateDone)
				<-lateUpdate
				update(AgentToolResult[map[string]any]{Details: map[string]any{"late": true}})
			}()
			return AgentToolResult[map[string]any]{Details: map[string]any{"done": true}}, nil
		},
	})
	if err != nil {
		t.Fatalf("EraseAgentTool error = %v", err)
	}

	updateCalls := 0
	prepared, err := sealValidatedAgentToolArguments(erased, map[string]any{})
	if err != nil {
		t.Fatalf("sealValidatedAgentToolArguments error = %v", err)
	}
	if _, err := erased.executeValidated(context.Background(), "call-1", prepared, func(ErasedAgentToolResult) {
		updateCalls++
	}); err != nil {
		t.Fatalf("ExecuteValidated error = %v", err)
	}
	close(lateUpdate)
	<-lateDone
	if updateCalls != 0 {
		t.Fatalf("post-return update callback count = %d, want 0", updateCalls)
	}
}

func TestToolAndQueueModesExposeExactWireValues(t *testing.T) {
	t.Parallel()

	got := []string{
		string(ToolExecutionParallel), string(ToolExecutionSequential),
		string(QueueAll), string(QueueOneAtATime),
	}
	want := []string{"parallel", "sequential", "all", "one-at-a-time"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("mode values = %v, want %v", got, want)
	}
}

func TestEraseAgentToolAllowsEmptyLabel(t *testing.T) {
	t.Parallel()

	erased, err := EraseAgentTool(AgentTool[map[string]any, map[string]any]{
		Tool:            ai.Tool{Name: "search", Description: "Search", Parameters: json.RawMessage(`{"type":"object"}`)},
		DecodeValidated: func(value ai.JSONValue) map[string]any { return value.(map[string]any) },
	})
	if err != nil {
		t.Fatalf("EraseAgentTool error = %v", err)
	}
	if erased.Label != "" {
		t.Fatalf("erased label = %q, want empty", erased.Label)
	}
}

func TestEraseAgentToolAndRegistrySnapshotsCopyMutableMetadata(t *testing.T) {
	t.Parallel()

	schema := json.RawMessage(`{"type":"object"}`)
	constrained := &ai.JSONSchemaConstrainedSampling{Strict: ai.ConstrainedSamplingStrictPrefer}
	erased, err := EraseAgentTool(AgentTool[map[string]any, map[string]any]{
		Tool: ai.Tool{
			Name:                "copy",
			Description:         "Copy",
			Parameters:          schema,
			ConstrainedSampling: constrained,
		},
	})
	if err != nil {
		t.Fatalf("EraseAgentTool() error = %v", err)
	}

	schema[0] = '['
	constrained.Strict = ai.ConstrainedSamplingStrictRequire
	if got := string(erased.Parameters); got != `{"type":"object"}` {
		t.Fatalf("erased Parameters = %q, want an independent schema copy", got)
	}
	erasedConstrained, ok := erased.ConstrainedSampling.(*ai.JSONSchemaConstrainedSampling)
	if !ok || erasedConstrained.Strict != ai.ConstrainedSamplingStrictPrefer {
		t.Fatalf("erased ConstrainedSampling = %#v, want independent prefer value", erased.ConstrainedSampling)
	}

	agent, err := NewAgent(AgentOptions{InitialState: &AgentInitialState{Tools: []ErasedAgentTool{erased}}})
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}
	snapshot := agent.State()
	snapshot.Tools[0].Parameters[0] = '['
	snapshotConstrained := snapshot.Tools[0].ConstrainedSampling.(*ai.JSONSchemaConstrainedSampling)
	snapshotConstrained.Strict = ai.ConstrainedSamplingStrictRequire

	next := agent.State()
	if got := string(next.Tools[0].Parameters); got != `{"type":"object"}` {
		t.Fatalf("stored Parameters = %q after snapshot mutation", got)
	}
	nextConstrained := next.Tools[0].ConstrainedSampling.(*ai.JSONSchemaConstrainedSampling)
	if nextConstrained.Strict != ai.ConstrainedSamplingStrictPrefer {
		t.Fatalf("stored ConstrainedSampling = %#v after snapshot mutation", nextConstrained)
	}
}

func TestErasedAgentToolRejectsValuesOutsideValidatedPipeline(t *testing.T) {
	t.Parallel()

	executeCalls := 0
	erased, err := EraseAgentTool(AgentTool[map[string]any, map[string]any]{
		Tool:            ai.Tool{Name: "search", Description: "Search", Parameters: json.RawMessage(`{"type":"object"}`)},
		DecodeValidated: func(value ai.JSONValue) map[string]any { return value.(map[string]any) },
		Execute: func(context.Context, string, map[string]any, AgentToolUpdateCallback[map[string]any]) (AgentToolResult[map[string]any], error) {
			executeCalls++
			return AgentToolResult[map[string]any]{Details: map[string]any{"ok": true}}, nil
		},
	})
	if err != nil {
		t.Fatalf("EraseAgentTool error = %v", err)
	}

	_, err = erased.executeValidated(context.Background(), "call-1", map[string]any{}, nil)
	if err == nil {
		t.Fatal("ExecuteValidated accepted input outside validated pipeline")
	}
	if executeCalls != 0 {
		t.Fatalf("ExecuteValidated invoked Execute %d times", executeCalls)
	}
}

func TestValidatedArgumentsAuthenticateTheOriginatingTool(t *testing.T) {
	t.Parallel()

	toolA, err := EraseAgentTool(AgentTool[map[string]any, map[string]any]{
		Tool:            ai.Tool{Name: "search-a", Description: "Search", Parameters: json.RawMessage(`{"type":"object"}`)},
		DecodeValidated: func(value ai.JSONValue) map[string]any { return value.(map[string]any) },
		Execute: func(context.Context, string, map[string]any, AgentToolUpdateCallback[map[string]any]) (AgentToolResult[map[string]any], error) {
			return AgentToolResult[map[string]any]{Details: map[string]any{"ok": true}}, nil
		},
	})
	if err != nil {
		t.Fatalf("EraseAgentTool(toolA) error = %v", err)
	}
	toolB, err := EraseAgentTool(AgentTool[map[string]any, map[string]any]{
		Tool:            ai.Tool{Name: "search-b", Description: "Search", Parameters: json.RawMessage(`{"type":"object"}`)},
		DecodeValidated: func(value ai.JSONValue) map[string]any { return value.(map[string]any) },
		Execute: func(context.Context, string, map[string]any, AgentToolUpdateCallback[map[string]any]) (AgentToolResult[map[string]any], error) {
			return AgentToolResult[map[string]any]{Details: map[string]any{"ok": true}}, nil
		},
	})
	if err != nil {
		t.Fatalf("EraseAgentTool(toolB) error = %v", err)
	}

	prepared, err := sealValidatedAgentToolArguments(toolA, map[string]any{"query": "pig"})
	if err != nil {
		t.Fatalf("sealValidatedAgentToolArguments error = %v", err)
	}
	if _, err := toolB.executeValidated(context.Background(), "call-1", prepared, nil); err == nil {
		t.Fatal("ExecuteValidated accepted input minted by a different ErasedAgentTool")
	}
}

func TestValidateErasedAgentToolRejectsForgedDescriptors(t *testing.T) {
	t.Parallel()

	forged := ErasedAgentTool{
		Tool:             ai.Tool{Name: "forged", Description: "Forged", Parameters: json.RawMessage(`{"type":"object"}`)},
		prepareArguments: func(value ai.JSONValue) (ai.JSONValue, error) { return value, nil },
		executeValidated: func(context.Context, string, ai.JSONValue, AgentToolUpdateCallback[ai.JSONValue]) (ErasedAgentToolResult, error) {
			return ErasedAgentToolResult{}, nil
		},
	}

	if err := validateErasedAgentTool(forged); err == nil {
		t.Fatal("validateErasedAgentTool accepted a forged descriptor")
	}
}

func TestValidateErasedAgentToolRejectsMutatedMetadataCopies(t *testing.T) {
	t.Parallel()

	erased, err := EraseAgentTool(AgentTool[map[string]any, map[string]any]{
		Tool:            ai.Tool{Name: "search", Description: "Search", Parameters: json.RawMessage(`{"type":"object"}`)},
		Label:           "Search",
		DecodeValidated: func(value ai.JSONValue) map[string]any { return value.(map[string]any) },
		Execute: func(context.Context, string, map[string]any, AgentToolUpdateCallback[map[string]any]) (AgentToolResult[map[string]any], error) {
			return AgentToolResult[map[string]any]{Details: map[string]any{"ok": true}}, nil
		},
	})
	if err != nil {
		t.Fatalf("EraseAgentTool error = %v", err)
	}

	mutated := erased
	mutated.Label = "Mutated"
	if err := validateErasedAgentTool(mutated); err == nil {
		t.Fatal("validateErasedAgentTool accepted a mutated metadata copy")
	}
}

func TestEraseAgentToolTypedDecodePreservesAssignableGoValues(t *testing.T) {
	t.Parallel()

	input := map[string]any{
		"count": 1,
		"list":  []any{2, map[string]any{"nested": 3}},
	}
	var received map[string]any
	erased, err := EraseAgentTool(AgentTool[map[string]any, map[string]any]{
		Tool: ai.Tool{
			Name: "shape-preserving", Description: "Shape preserving",
			Parameters: json.RawMessage(`{"type":"object","properties":{"count":{"type":"integer"},"list":{"type":"array"}}}`),
		},
		DecodeValidated: func(value ai.JSONValue) map[string]any { return value.(map[string]any) },
		Execute: func(_ context.Context, _ string, params map[string]any, _ AgentToolUpdateCallback[map[string]any]) (AgentToolResult[map[string]any], error) {
			received = params
			return AgentToolResult[map[string]any]{Details: map[string]any{"ok": true}}, nil
		},
	})
	if err != nil {
		t.Fatalf("EraseAgentTool error = %v", err)
	}

	prepared, err := sealValidatedAgentToolArguments(erased, input)
	if err != nil {
		t.Fatalf("sealValidatedAgentToolArguments error = %v", err)
	}
	if _, err := erased.executeValidated(context.Background(), "call-1", prepared, nil); err != nil {
		t.Fatalf("ExecuteValidated error = %v", err)
	}
	if !reflect.DeepEqual(received, input) {
		t.Fatalf("typed parameters = %#v, want %#v", received, input)
	}
	if _, ok := received["count"].(int); !ok {
		t.Fatalf("typed parameter count type = %T, want int", received["count"])
	}
}

type rejectingJSONParameters struct {
	Count float64
}

func (*rejectingJSONParameters) UnmarshalJSON([]byte) error {
	return errors.New("secondary JSON decoding must not run")
}

func TestValidatedToolArgumentsUseTheAuthorSuppliedTotalMapping(t *testing.T) {
	t.Parallel()

	var received rejectingJSONParameters
	erased, err := EraseAgentTool(AgentTool[rejectingJSONParameters, map[string]any]{
		Tool: ai.Tool{
			Name: "no-secondary-decoder", Description: "No secondary decoder",
			Parameters: json.RawMessage(`{"type":"object","required":["count"],"properties":{"count":{"type":"number"}}}`),
		},
		DecodeValidated: func(value ai.JSONValue) rejectingJSONParameters {
			return rejectingJSONParameters{Count: value.(map[string]any)["count"].(float64)}
		},
		Execute: func(_ context.Context, _ string, params rejectingJSONParameters, _ AgentToolUpdateCallback[map[string]any]) (AgentToolResult[map[string]any], error) {
			received = params
			return AgentToolResult[map[string]any]{Details: map[string]any{"ok": true}}, nil
		},
	})
	if err != nil {
		t.Fatalf("EraseAgentTool() error = %v", err)
	}
	prepared, err := sealValidatedAgentToolArguments(erased, map[string]any{"count": 1.5})
	if err != nil {
		t.Fatalf("sealValidatedAgentToolArguments() error = %v", err)
	}
	if _, err := erased.executeValidated(context.Background(), "call-1", prepared, nil); err != nil {
		t.Fatalf("executeValidated() error = %v", err)
	}
	if received.Count != 1.5 {
		t.Fatalf("Execute received %#v, want count 1.5", received)
	}
}

package agent_test

import (
	"context"
	"encoding/json"
	"reflect"
	"slices"
	"testing"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
)

func TestDeferredToolsDiscoveryAndContinuationThroughFaux(t *testing.T) {
	ctx := context.Background()
	core, err := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	makeTool := func(name string, result agent.AgentToolResult[struct{}]) agent.ErasedAgentTool {
		tool, err := agent.EraseAgentTool(agent.AgentTool[struct{}, struct{}]{
			Tool:            ai.Tool{Name: name, Description: name, Parameters: json.RawMessage(`{"type":"object","properties":{}}`)},
			DecodeValidated: func(ai.JSONValue) struct{} { return struct{}{} },
			Execute: func(context.Context, string, struct{}, agent.AgentToolUpdateCallback[struct{}]) (agent.AgentToolResult[struct{}], error) {
				return result, nil
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		return tool
	}
	discover := makeTool("discover", agent.AgentToolResult[struct{}]{
		Content:        []ai.ToolResultContent{ai.FauxText("read and unused discovered")},
		AddedToolNames: ai.Some([]string{"read", "read", "unused"}), Terminate: ai.Some(true),
	})
	read := makeTool("read", agent.AgentToolResult[struct{}]{Content: []ai.ToolResultContent{ai.FauxText("file contents")}})
	unused := makeTool("unused", agent.AgentToolResult[struct{}]{})
	for _, step := range []struct {
		immediate []string
		deferred  []string
		call      string
	}{
		{[]string{"discover"}, nil, "discover"},
		{[]string{"discover"}, []string{"read", "unused"}, "read"},
		{[]string{"discover", "read"}, []string{"unused"}, ""},
	} {
		core.AppendResponses([]ai.FauxResponseStep{ai.FauxResponseFactory(func(input ai.Context, _ *ai.SimpleStreamOptions, _ *ai.FauxProviderState, _ ai.Model) (ai.AssistantMessage, error) {
			immediate, deferred := ai.SplitDeferredTools(input, true, nil)
			names := func(tools []ai.Tool) []string {
				result := make([]string, len(tools))
				for i, tool := range tools {
					result[i] = tool.Name
				}
				return result
			}
			if !slices.Equal(names(immediate), step.immediate) || !slices.Equal(names(deferred), step.deferred) {
				t.Errorf("request partition = %v / %v, want %v / %v", names(immediate), names(deferred), step.immediate, step.deferred)
			}
			if step.call == "" {
				return ai.FauxAssistantMessage(ai.FauxAssistantText("done"))
			}
			call, err := ai.FauxToolCall(step.call, map[string]any{}, ai.FauxToolCallOptions{ID: ai.Some(step.call + "-1")})
			if err != nil {
				return ai.AssistantMessage{}, err
			}
			return ai.FauxAssistantMessage(ai.FauxAssistantBlocks(call), ai.FauxAssistantMessageOptions{StopReason: ai.Some(ai.StopReasonToolUse)})
		})})
	}
	model, _ := core.GetModel()
	a, err := agent.NewAgent(agent.AgentOptions{
		InitialState:   &agent.AgentInitialState{Model: model, Tools: []agent.ErasedAgentTool{discover}},
		StreamFunction: agent.StreamFunction(core.StreamSimple),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.PromptText(ctx, "discover tools"); err != nil {
		t.Fatal(err)
	}
	discovery := a.State().Messages
	if len(discovery) != 3 || core.State.CallCount != 1 {
		t.Fatalf("discovery did not stop at its ToolResult: %#v", discovery)
	}
	if err := a.SetTools([]agent.ErasedAgentTool{discover, read, unused}); err != nil {
		t.Fatal(err)
	}
	if err := a.Continue(ctx); err != nil {
		t.Fatal(err)
	}
	messages := a.State().Messages
	if len(messages) != 6 || !reflect.DeepEqual(messages[:3], discovery) || core.State.CallCount != 3 {
		t.Fatalf("continuation transcript = %#v", messages)
	}
	marker := messages[2].(ai.ToolResultMessage)
	names, _ := marker.AddedToolNames.Value()
	if !slices.Equal(names, []string{"read", "read", "unused"}) {
		t.Fatalf("added tool names changed: %v", names)
	}
	result := messages[4].(ai.ToolResultMessage)
	if result.ToolName != "read" || result.ToolCallID != "read-1" || result.IsError || result.Content[0].(ai.TextContent).Text != "file contents" {
		t.Fatalf("dynamic Tool result = %#v", result)
	}
	final := messages[5].(ai.AssistantMessage)
	if final.StopReason != ai.StopReasonStop || final.Content[0].(ai.TextContent).Text != "done" {
		t.Fatalf("final response = %#v", final)
	}
}

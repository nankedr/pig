package agent_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
)

func TestAgentContinuesFromToolResultToFinalAssistantResponse(t *testing.T) {
	tool, err := agent.EraseAgentTool(agent.AgentTool[map[string]any, map[string]any]{
		Tool:            ai.Tool{Name: "lookup", Description: "Look up a value", Parameters: json.RawMessage(`{"type":"object"}`)},
		DecodeValidated: func(value ai.JSONValue) map[string]any { return value.(map[string]any) },
		Execute: func(context.Context, string, map[string]any, agent.AgentToolUpdateCallback[map[string]any]) (agent.AgentToolResult[map[string]any], error) {
			return agent.AgentToolResult[map[string]any]{
				Content: []ai.ToolResultContent{ai.TextContent{Type: ai.ContentTypeText, Text: "42"}},
				Details: map[string]any{"value": float64(42)},
			}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	toolCall, err := ai.FauxToolCall("lookup", map[string]any{}, ai.FauxToolCallOptions{ID: ai.Some("call-52")})
	if err != nil {
		t.Fatal(err)
	}
	firstResponse, err := ai.FauxAssistantMessage(ai.FauxAssistantBlocks(toolCall), ai.FauxAssistantMessageOptions{
		StopReason: ai.Some(ai.StopReasonToolUse), Timestamp: ai.Some(int64(2)),
	})
	if err != nil {
		t.Fatal(err)
	}
	finalResponse, err := ai.FauxAssistantMessage(ai.FauxAssistantText("The result is 42."), ai.FauxAssistantMessageOptions{
		Timestamp: ai.Some(int64(4)),
	})
	if err != nil {
		t.Fatal(err)
	}

	core, err := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	providerCalls := 0
	core.SetResponses([]ai.FauxResponseStep{
		ai.FauxResponseFactory(func(input ai.Context, _ *ai.SimpleStreamOptions, _ *ai.FauxProviderState, _ ai.Model) (ai.AssistantMessage, error) {
			providerCalls++
			if roles := messageRoles(input.Messages); !reflect.DeepEqual(roles, []ai.MessageRole{ai.MessageRoleUser}) {
				t.Fatalf("first Provider context roles = %v", roles)
			}
			return firstResponse, nil
		}),
		ai.FauxResponseFactory(func(input ai.Context, _ *ai.SimpleStreamOptions, _ *ai.FauxProviderState, _ ai.Model) (ai.AssistantMessage, error) {
			providerCalls++
			if roles := messageRoles(input.Messages); !reflect.DeepEqual(roles, []ai.MessageRole{
				ai.MessageRoleUser, ai.MessageRoleAssistant, ai.MessageRoleToolResult,
			}) {
				t.Fatalf("continuation Provider context roles = %v", roles)
			}
			assistant := input.Messages[1].(ai.AssistantMessage)
			call := assistant.Content[0].(ai.ToolCall)
			result := input.Messages[2].(ai.ToolResultMessage)
			if call.ID != "call-52" || result.ToolCallID != call.ID || result.Content[0].(ai.TextContent).Text != "42" {
				t.Fatalf("continuation context = %#v", input.Messages)
			}
			return finalResponse, nil
		}),
	})
	model, _ := core.GetModel()
	created, err := agent.NewAgent(agent.AgentOptions{
		InitialState:   &agent.AgentInitialState{Model: model, Tools: []agent.ErasedAgentTool{tool}},
		StreamFunction: agent.StreamFunction(core.StreamSimple),
	})
	if err != nil {
		t.Fatal(err)
	}
	var events []agent.AgentEventType
	var ended []agent.AgentMessage
	created.Subscribe(func(_ context.Context, event agent.AgentEvent) error {
		events = append(events, event.AgentEventType())
		if event, ok := event.(agent.AgentEndEvent); ok {
			ended = event.Messages
		}
		return nil
	})

	if err := created.Prompt(context.Background(), ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText("look it up"), Timestamp: 1}); err != nil {
		t.Fatalf("Prompt() error = %v", err)
	}
	if providerCalls != 2 {
		t.Fatalf("Provider calls = %d, want 2", providerCalls)
	}
	wantEvents := []agent.AgentEventType{
		agent.AgentEventTypeAgentStart, agent.AgentEventTypeTurnStart,
		agent.AgentEventTypeMessageStart, agent.AgentEventTypeMessageEnd,
		agent.AgentEventTypeMessageStart,
		agent.AgentEventTypeMessageUpdate, agent.AgentEventTypeMessageUpdate, agent.AgentEventTypeMessageUpdate,
		agent.AgentEventTypeMessageEnd,
		agent.AgentEventTypeToolExecutionStart, agent.AgentEventTypeToolExecutionEnd,
		agent.AgentEventTypeMessageStart, agent.AgentEventTypeMessageEnd, agent.AgentEventTypeTurnEnd,
		agent.AgentEventTypeTurnStart, agent.AgentEventTypeMessageStart,
		agent.AgentEventTypeMessageUpdate, agent.AgentEventTypeMessageUpdate, agent.AgentEventTypeMessageUpdate, agent.AgentEventTypeMessageUpdate,
		agent.AgentEventTypeMessageEnd, agent.AgentEventTypeTurnEnd, agent.AgentEventTypeAgentEnd,
	}
	if !reflect.DeepEqual(events, wantEvents) {
		t.Fatalf("events = %v, want %v", events, wantEvents)
	}
	state := created.State()
	if roles := messageRolesFromAgent(state.Messages); !reflect.DeepEqual(roles, []ai.MessageRole{
		ai.MessageRoleUser, ai.MessageRoleAssistant, ai.MessageRoleToolResult, ai.MessageRoleAssistant,
	}) {
		t.Fatalf("transcript roles = %v", roles)
	}
	final := state.Messages[3].(ai.AssistantMessage)
	if final.Content[0].(ai.TextContent).Text != "The result is 42." || !reflect.DeepEqual(ended, state.Messages) {
		t.Fatalf("final transcript = %#v, agent_end = %#v", state.Messages, ended)
	}
}

func TestAgentWaitsForToolTurnBarriersBeforeContinuationRequest(t *testing.T) {
	assistantEndStarted := make(chan struct{})
	releaseAssistantEnd := make(chan struct{})
	toolResultEndStarted := make(chan struct{})
	releaseToolResultEnd := make(chan struct{})
	var beforeCalls, executeCalls, providerCalls atomic.Int64

	tool, err := agent.EraseAgentTool(agent.AgentTool[map[string]any, map[string]any]{
		Tool:            ai.Tool{Name: "lookup", Description: "Look up a value", Parameters: json.RawMessage(`{"type":"object"}`)},
		DecodeValidated: func(value ai.JSONValue) map[string]any { return value.(map[string]any) },
		Execute: func(context.Context, string, map[string]any, agent.AgentToolUpdateCallback[map[string]any]) (agent.AgentToolResult[map[string]any], error) {
			executeCalls.Add(1)
			return agent.AgentToolResult[map[string]any]{Content: []ai.ToolResultContent{ai.TextContent{Type: ai.ContentTypeText, Text: "42"}}}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	toolCall, err := ai.FauxToolCall("lookup", map[string]any{}, ai.FauxToolCallOptions{ID: ai.Some("call-52")})
	if err != nil {
		t.Fatal(err)
	}
	firstResponse, err := ai.FauxAssistantMessage(ai.FauxAssistantBlocks(toolCall), ai.FauxAssistantMessageOptions{StopReason: ai.Some(ai.StopReasonToolUse)})
	if err != nil {
		t.Fatal(err)
	}
	finalResponse, err := ai.FauxAssistantMessage(ai.FauxAssistantText("done"))
	if err != nil {
		t.Fatal(err)
	}
	core, err := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	core.SetResponses([]ai.FauxResponseStep{
		ai.FauxResponseFactory(func(ai.Context, *ai.SimpleStreamOptions, *ai.FauxProviderState, ai.Model) (ai.AssistantMessage, error) {
			providerCalls.Add(1)
			return firstResponse, nil
		}),
		ai.FauxResponseFactory(func(ai.Context, *ai.SimpleStreamOptions, *ai.FauxProviderState, ai.Model) (ai.AssistantMessage, error) {
			providerCalls.Add(1)
			return finalResponse, nil
		}),
	})
	model, _ := core.GetModel()
	created, err := agent.NewAgent(agent.AgentOptions{
		InitialState:   &agent.AgentInitialState{Model: model, Tools: []agent.ErasedAgentTool{tool}},
		StreamFunction: agent.StreamFunction(core.StreamSimple),
		BeforeToolCall: func(context.Context, agent.BeforeToolCallContext) (*agent.BeforeToolCallResult, error) {
			beforeCalls.Add(1)
			return nil, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	created.Subscribe(func(_ context.Context, event agent.AgentEvent) error {
		messageEnd, ok := event.(agent.MessageEndEvent)
		if !ok {
			return nil
		}
		switch messageEnd.Message.MessageRole() {
		case ai.MessageRoleAssistant:
			assistant := messageEnd.Message.(ai.AssistantMessage)
			if assistant.StopReason == ai.StopReasonToolUse {
				close(assistantEndStarted)
				<-releaseAssistantEnd
			}
		case ai.MessageRoleToolResult:
			close(toolResultEndStarted)
			<-releaseToolResultEnd
		}
		return nil
	})

	promptDone := make(chan error, 1)
	go func() {
		promptDone <- created.Prompt(context.Background(), ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText("look it up"), Timestamp: 1})
	}()
	waitForSignal(t, assistantEndStarted, "first Assistant message_end listener")
	if providerCalls.Load() != 1 || beforeCalls.Load() != 0 || executeCalls.Load() != 0 {
		t.Fatalf("work crossed Assistant message_end barrier: provider=%d before=%d execute=%d", providerCalls.Load(), beforeCalls.Load(), executeCalls.Load())
	}
	close(releaseAssistantEnd)
	waitForSignal(t, toolResultEndStarted, "ToolResult message_end listener")
	if providerCalls.Load() != 1 || beforeCalls.Load() != 1 || executeCalls.Load() != 1 {
		t.Fatalf("continuation crossed ToolResult barrier: provider=%d before=%d execute=%d", providerCalls.Load(), beforeCalls.Load(), executeCalls.Load())
	}
	close(releaseToolResultEnd)
	select {
	case err := <-promptDone:
		if err != nil {
			t.Fatalf("Prompt() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Prompt() did not settle after continuation barriers")
	}
	if providerCalls.Load() != 2 {
		t.Fatalf("Provider calls = %d, want 2", providerCalls.Load())
	}
}

func TestAgentContinuationStopsAfterTurnOrTerminatingTool(t *testing.T) {
	tests := []struct {
		name       string
		terminate  bool
		shouldStop bool
	}{
		{name: "shouldStopAfterTurn", shouldStop: true},
		{name: "terminating ToolResult", terminate: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tool, err := agent.EraseAgentTool(agent.AgentTool[map[string]any, map[string]any]{
				Tool:            ai.Tool{Name: "lookup", Description: "Look up a value", Parameters: json.RawMessage(`{"type":"object"}`)},
				DecodeValidated: func(value ai.JSONValue) map[string]any { return value.(map[string]any) },
				Execute: func(context.Context, string, map[string]any, agent.AgentToolUpdateCallback[map[string]any]) (agent.AgentToolResult[map[string]any], error) {
					result := agent.AgentToolResult[map[string]any]{Content: []ai.ToolResultContent{ai.TextContent{Type: ai.ContentTypeText, Text: "42"}}}
					if test.terminate {
						result.Terminate = ai.Some(true)
					}
					return result, nil
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			toolCall, err := ai.FauxToolCall("lookup", map[string]any{}, ai.FauxToolCallOptions{ID: ai.Some("call-52")})
			if err != nil {
				t.Fatal(err)
			}
			firstResponse, err := ai.FauxAssistantMessage(ai.FauxAssistantBlocks(toolCall), ai.FauxAssistantMessageOptions{StopReason: ai.Some(ai.StopReasonToolUse)})
			if err != nil {
				t.Fatal(err)
			}
			forbiddenResponse, err := ai.FauxAssistantMessage(ai.FauxAssistantText("should not run"))
			if err != nil {
				t.Fatal(err)
			}
			core, err := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
			if err != nil {
				t.Fatal(err)
			}
			core.SetResponses([]ai.FauxResponseStep{firstResponse, forbiddenResponse})
			model, _ := core.GetModel()
			created, err := agent.NewAgent(agent.AgentOptions{
				InitialState:   &agent.AgentInitialState{Model: model, Tools: []agent.ErasedAgentTool{tool}},
				StreamFunction: agent.StreamFunction(core.StreamSimple),
				ShouldStopAfterTurn: func(context.Context, agent.ShouldStopAfterTurnContext) (bool, error) {
					return test.shouldStop, nil
				},
			})
			if err != nil {
				t.Fatal(err)
			}

			if err := created.Prompt(context.Background(), ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText("look it up"), Timestamp: 1}); err != nil {
				t.Fatalf("Prompt() error = %v", err)
			}
			if core.State.CallCount != 1 || core.GetPendingResponseCount() != 1 {
				t.Fatalf("Faux calls = %d, pending = %d; continuation should be stopped", core.State.CallCount, core.GetPendingResponseCount())
			}
			if roles := messageRolesFromAgent(created.State().Messages); !reflect.DeepEqual(roles, []ai.MessageRole{
				ai.MessageRoleUser, ai.MessageRoleAssistant, ai.MessageRoleToolResult,
			}) {
				t.Fatalf("transcript roles = %v", roles)
			}
		})
	}
}

func waitForSignal(t *testing.T, signal <-chan struct{}, name string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for %s", name)
	}
}

func messageRoles(messages []ai.Message) []ai.MessageRole {
	roles := make([]ai.MessageRole, len(messages))
	for i, message := range messages {
		roles[i] = message.MessageRole()
	}
	return roles
}

func messageRolesFromAgent(messages []agent.AgentMessage) []ai.MessageRole {
	roles := make([]ai.MessageRole, len(messages))
	for i, message := range messages {
		roles[i] = message.MessageRole()
	}
	return roles
}

func TestAgentRunsOneTerminatingToolThroughTheAuthoritativeArgumentPipeline(t *testing.T) {
	type parameters struct {
		Count   float64
		Enabled bool
	}
	type details struct {
		Doubled float64 `json:"doubled"`
	}

	var pipeline []string
	tool, err := agent.EraseAgentTool(agent.AgentTool[parameters, details]{
		Tool: ai.Tool{
			Name:        "finish",
			Description: "Finish deterministically",
			Parameters: json.RawMessage(`{
				"type":"object",
				"required":["count","enabled"],
				"properties":{
					"count":{"type":"number"},
					"enabled":{"type":"boolean"}
				}
			}`),
		},
		Label: "Finish",
		PrepareArguments: func(value ai.JSONValue) (ai.JSONValue, error) {
			pipeline = append(pipeline, "prepare")
			arguments := value.(map[string]any)
			if arguments["count"] != "2.5" {
				t.Fatalf("PrepareArguments count = %#v, want raw string", arguments["count"])
			}
			arguments["enabled"] = "true"
			return arguments, nil
		},
		DecodeValidated: func(value ai.JSONValue) parameters {
			pipeline = append(pipeline, "decode")
			arguments := value.(map[string]any)
			return parameters{Count: arguments["count"].(float64), Enabled: arguments["enabled"].(bool)}
		},
		Execute: func(_ context.Context, callID string, params parameters, update agent.AgentToolUpdateCallback[details]) (agent.AgentToolResult[details], error) {
			pipeline = append(pipeline, "execute")
			if callID != "call-51" || params != (parameters{Count: 2.5, Enabled: true}) {
				t.Fatalf("Execute input = (%q, %#v)", callID, params)
			}
			update(agent.AgentToolResult[details]{
				Content: []ai.ToolResultContent{ai.TextContent{Type: ai.ContentTypeText, Text: "working"}},
				Details: details{Doubled: 2.5},
			})
			return agent.AgentToolResult[details]{
				Content:        []ai.ToolResultContent{ai.TextContent{Type: ai.ContentTypeText, Text: "finished"}},
				Details:        details{Doubled: 5},
				AddedToolNames: ai.Some([]string{}),
				Terminate:      ai.Some(true),
			}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	toolCall, err := ai.FauxToolCall("finish", map[string]any{"count": "2.5"}, ai.FauxToolCallOptions{ID: ai.Some("call-51")})
	if err != nil {
		t.Fatal(err)
	}
	response, err := ai.FauxAssistantMessage(ai.FauxAssistantBlocks(toolCall), ai.FauxAssistantMessageOptions{
		StopReason: ai.Some(ai.StopReasonToolUse),
		Timestamp:  ai.Some(int64(2)),
	})
	if err != nil {
		t.Fatal(err)
	}
	core, err := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	core.SetResponses([]ai.FauxResponseStep{response})
	model, _ := core.GetModel()
	created, err := agent.NewAgent(agent.AgentOptions{
		InitialState:   &agent.AgentInitialState{Model: model, Tools: []agent.ErasedAgentTool{tool}},
		StreamFunction: agent.StreamFunction(core.StreamSimple),
		ShouldStopAfterTurn: func(_ context.Context, input agent.ShouldStopAfterTurnContext) (bool, error) {
			if len(input.ToolResults) != 1 || len(input.Context.Messages) != 3 || len(input.NewMessages) != 3 {
				t.Fatalf("ShouldStopAfterTurn input = %#v", input)
			}
			input.Message.Content[0].(ai.ToolCall).Arguments["count"] = "mutated by stop hook"
			return false, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	var events []agent.AgentEventType
	created.Subscribe(func(_ context.Context, event agent.AgentEvent) error {
		events = append(events, event.AgentEventType())
		switch event := event.(type) {
		case agent.ToolExecutionStartEvent:
			if _, pending := created.State().PendingToolCalls[event.ToolCallID]; !pending {
				t.Fatal("Tool was not pending before tool_execution_start listeners")
			}
			event.Arguments.(map[string]any)["count"] = "mutated by listener"
		case agent.ToolExecutionEndEvent:
			if _, pending := created.State().PendingToolCalls[event.ToolCallID]; pending {
				t.Fatal("Tool remained pending during tool_execution_end listeners")
			}
			if detail := event.Result.Details.(map[string]any); detail["doubled"] != float64(5) {
				t.Fatalf("tool_execution_end details = %#v", detail)
			}
			event.Result.Details.(map[string]any)["doubled"] = float64(99)
		case agent.TurnEndEvent:
			if len(event.ToolResults) != 1 || event.ToolResults[0].ToolCallID != "call-51" || event.ToolResults[0].IsError {
				t.Fatalf("turn_end ToolResults = %#v", event.ToolResults)
			}
			details, _ := event.ToolResults[0].Details.Value()
			if details.(map[string]any)["doubled"] != float64(5) {
				t.Fatalf("tool_execution_end listener mutated turn result = %#v", details)
			}
		case agent.AgentEndEvent:
			if len(event.Messages) != 3 || event.Messages[2].MessageRole() != ai.MessageRoleToolResult {
				t.Fatalf("agent_end messages = %#v", event.Messages)
			}
		}
		return nil
	})
	if err := created.Prompt(context.Background(), ai.UserMessage{
		Role: ai.MessageRoleUser, Content: ai.UserText("finish"), Timestamp: 1,
	}); err != nil {
		t.Fatalf("Prompt() error = %v", err)
	}

	if !reflect.DeepEqual(pipeline, []string{"prepare", "decode", "execute"}) {
		t.Fatalf("argument pipeline = %v", pipeline)
	}
	wantEvents := []agent.AgentEventType{
		agent.AgentEventTypeAgentStart, agent.AgentEventTypeTurnStart,
		agent.AgentEventTypeMessageStart, agent.AgentEventTypeMessageEnd,
		agent.AgentEventTypeMessageStart,
		agent.AgentEventTypeMessageUpdate, agent.AgentEventTypeMessageUpdate, agent.AgentEventTypeMessageUpdate, agent.AgentEventTypeMessageUpdate,
		agent.AgentEventTypeMessageEnd,
		agent.AgentEventTypeToolExecutionStart, agent.AgentEventTypeToolExecutionUpdate, agent.AgentEventTypeToolExecutionEnd,
		agent.AgentEventTypeMessageStart, agent.AgentEventTypeMessageEnd,
		agent.AgentEventTypeTurnEnd, agent.AgentEventTypeAgentEnd,
	}
	if !reflect.DeepEqual(events, wantEvents) {
		t.Fatalf("events = %v, want %v", events, wantEvents)
	}
	state := created.State()
	if len(state.Messages) != 3 {
		t.Fatalf("transcript length = %d, want 3", len(state.Messages))
	}
	result, ok := state.Messages[2].(ai.ToolResultMessage)
	if !ok {
		t.Fatalf("transcript result type = %T", state.Messages[2])
	}
	text := result.Content[0].(ai.TextContent).Text
	detailValue, detailOK := result.Details.Value()
	if result.ToolCallID != "call-51" || result.ToolName != "finish" || result.IsError || text != "finished" || !detailOK || !reflect.DeepEqual(detailValue, map[string]any{"doubled": float64(5)}) || result.AddedToolNames.IsSet() {
		t.Fatalf("ToolResult = %#v", result)
	}
	assistant := state.Messages[1].(ai.AssistantMessage)
	if call := assistant.Content[0].(ai.ToolCall); call.Arguments["count"] != "2.5" {
		t.Fatalf("ShouldStopAfterTurn mutated transcript ToolCall = %#v", call.Arguments)
	}
}

func TestAgentTurnsToolPreflightAndExecutionFailuresIntoErrorResults(t *testing.T) {
	tests := []struct {
		name             string
		toolName         string
		arguments        map[string]any
		parameters       json.RawMessage
		prepare          agent.PrepareArgumentsFunc
		before           func(context.Context, agent.BeforeToolCallContext) (*agent.BeforeToolCallResult, error)
		executeError     error
		wantErrorText    string
		wantPrepareCalls int
		wantBeforeCalls  int
		wantDecodeCalls  int
		wantExecuteCalls int
	}{
		{name: "missing Tool", toolName: "missing", arguments: map[string]any{"count": "2"}, wantErrorText: "Tool missing not found"},
		{name: "prepare failure", toolName: "finish", arguments: map[string]any{"count": "2"}, prepare: func(ai.JSONValue) (ai.JSONValue, error) {
			return nil, errors.New("cannot prepare")
		}, wantErrorText: "cannot prepare", wantPrepareCalls: 1},
		{name: "invalid arguments", toolName: "finish", arguments: map[string]any{"count": "nope"}, wantErrorText: `Validation failed for tool "finish"`, wantPrepareCalls: 1},
		{name: "schema constraint failure", toolName: "finish", arguments: map[string]any{"count": "0"}, wantErrorText: `Validation failed for tool "finish"`, wantPrepareCalls: 1},
		{name: "blocked by before hook", toolName: "finish", arguments: map[string]any{"count": "2"}, before: func(context.Context, agent.BeforeToolCallContext) (*agent.BeforeToolCallResult, error) {
			return &agent.BeforeToolCallResult{Block: true, Reason: "not allowed", Terminate: true}, nil
		}, wantErrorText: "not allowed", wantPrepareCalls: 1, wantBeforeCalls: 1, wantDecodeCalls: 1},
		{name: "Execute failure", toolName: "finish", arguments: map[string]any{"count": "2"}, executeError: errors.New("host failed"), wantErrorText: "host failed", wantPrepareCalls: 1, wantDecodeCalls: 1, wantExecuteCalls: 1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			prepareCalls, beforeCalls, decodeCalls, executeCalls := 0, 0, 0, 0
			parameters := test.parameters
			if parameters == nil {
				parameters = json.RawMessage(`{"type":"object","required":["count"],"properties":{"count":{"type":"number","minimum":1}}}`)
			}
			prepare := test.prepare
			if prepare == nil {
				prepare = func(value ai.JSONValue) (ai.JSONValue, error) {
					prepareCalls++
					return value, nil
				}
			} else {
				configured := prepare
				prepare = func(value ai.JSONValue) (ai.JSONValue, error) {
					prepareCalls++
					return configured(value)
				}
			}
			tool, err := agent.EraseAgentTool(agent.AgentTool[float64, map[string]any]{
				Tool:             ai.Tool{Name: "finish", Description: "Finish", Parameters: parameters},
				PrepareArguments: prepare,
				DecodeValidated: func(value ai.JSONValue) float64 {
					decodeCalls++
					return value.(map[string]any)["count"].(float64)
				},
				Execute: func(context.Context, string, float64, agent.AgentToolUpdateCallback[map[string]any]) (agent.AgentToolResult[map[string]any], error) {
					executeCalls++
					if test.executeError != nil {
						return agent.AgentToolResult[map[string]any]{}, test.executeError
					}
					return agent.AgentToolResult[map[string]any]{Terminate: ai.Some(true)}, nil
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			before := test.before
			if before != nil {
				configured := before
				before = func(ctx context.Context, input agent.BeforeToolCallContext) (*agent.BeforeToolCallResult, error) {
					beforeCalls++
					input.ToolCall.Arguments["count"] = "mutated by hook"
					return configured(ctx, input)
				}
			}
			created := newSingleToolAgent(t, test.toolName, test.arguments, []agent.ErasedAgentTool{tool}, before)
			if err := created.Prompt(context.Background(), ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText("go"), Timestamp: 1}); err != nil {
				t.Fatalf("Prompt() error = %v", err)
			}
			state := created.State()
			if len(state.Messages) != 3 {
				t.Fatalf("transcript length = %d, want 3", len(state.Messages))
			}
			result := state.Messages[2].(ai.ToolResultMessage)
			text := result.Content[0].(ai.TextContent).Text
			if !result.IsError || !strings.Contains(text, test.wantErrorText) {
				t.Fatalf("error ToolResult = %#v, text %q", result, text)
			}
			if prepareCalls != test.wantPrepareCalls || beforeCalls != test.wantBeforeCalls || decodeCalls != test.wantDecodeCalls || executeCalls != test.wantExecuteCalls {
				t.Fatalf("calls = prepare %d before %d decode %d execute %d", prepareCalls, beforeCalls, decodeCalls, executeCalls)
			}
			if before != nil {
				assistant := state.Messages[1].(ai.AssistantMessage)
				call := assistant.Content[0].(ai.ToolCall)
				if call.Arguments["count"] != "2" {
					t.Fatalf("before hook mutated transcript ToolCall = %#v", call.Arguments)
				}
			}
		})
	}
}

func TestEraseAgentToolRejectsAnExternalSchemaReferenceWithoutLoadingIt(t *testing.T) {
	_, err := agent.EraseAgentTool(agent.AgentTool[map[string]any, map[string]any]{
		Tool:            ai.Tool{Name: "finish", Description: "Finish", Parameters: json.RawMessage(`{"$ref":"https://example.invalid/tool-schema.json"}`)},
		DecodeValidated: func(value ai.JSONValue) map[string]any { return value.(map[string]any) },
		Execute: func(context.Context, string, map[string]any, agent.AgentToolUpdateCallback[map[string]any]) (agent.AgentToolResult[map[string]any], error) {
			t.Fatal("Execute called for invalid schema")
			return agent.AgentToolResult[map[string]any]{}, nil
		},
	})
	if err == nil || !strings.Contains(err.Error(), `invalid JSON Schema for tool "finish"`) {
		t.Fatalf("EraseAgentTool() error = %v", err)
	}
}

func TestAgentRunsAfterToolCallForExecutionResults(t *testing.T) {
	t.Run("override Execute failure", func(t *testing.T) {
		usage := ai.Usage{Input: 7, TotalTokens: 7}
		tool, err := agent.EraseAgentTool(agent.AgentTool[float64, map[string]any]{
			Tool: ai.Tool{Name: "finish", Description: "Finish", Parameters: json.RawMessage(`{"type":"object","required":["count"],"properties":{"count":{"type":"number"}}}`)},
			DecodeValidated: func(value ai.JSONValue) float64 {
				return value.(map[string]any)["count"].(float64)
			},
			Execute: func(context.Context, string, float64, agent.AgentToolUpdateCallback[map[string]any]) (agent.AgentToolResult[map[string]any], error) {
				return agent.AgentToolResult[map[string]any]{}, errors.New("host failed")
			},
		})
		if err != nil {
			t.Fatal(err)
		}

		afterCalls := 0
		created := newSingleToolAgentWithOptions(t, "finish", map[string]any{"count": "2"}, []agent.ErasedAgentTool{tool}, func(options *agent.AgentOptions) {
			options.ShouldStopAfterTurn = nil
			options.AfterToolCall = func(_ context.Context, input agent.AfterToolCallContext) (*agent.AfterToolCallResult, error) {
				afterCalls++
				if input.Args.(map[string]any)["count"] != float64(2) || !input.IsError {
					t.Fatalf("AfterToolCall input = %#v", input)
				}
				if text := input.Result.Content[0].(ai.TextContent).Text; text != "host failed" {
					t.Fatalf("AfterToolCall result text = %q", text)
				}
				if len(input.Context.Messages) != 2 || input.Context.Messages[1].MessageRole() != ai.MessageRoleAssistant {
					t.Fatalf("AfterToolCall context = %#v", input.Context.Messages)
				}
				input.ToolCall.Arguments["count"] = "mutated by hook"
				return &agent.AfterToolCallResult{
					Content:   ai.Some([]ai.ToolResultContent{ai.TextContent{Type: ai.ContentTypeText, Text: "recovered"}}),
					Details:   ai.Some[ai.JSONValue](map[string]any{"source": "after"}),
					IsError:   ai.Some(false),
					Usage:     ai.Some(usage),
					Terminate: ai.Some(true),
				}, nil
			}
		})
		if err := created.Prompt(context.Background(), ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText("go"), Timestamp: 1}); err != nil {
			t.Fatalf("Prompt() error = %v", err)
		}
		if afterCalls != 1 {
			t.Fatalf("AfterToolCall calls = %d, want 1", afterCalls)
		}
		result := created.State().Messages[2].(ai.ToolResultMessage)
		details, detailsOK := result.Details.Value()
		gotUsage, usageOK := result.Usage.Value()
		if result.IsError || result.Content[0].(ai.TextContent).Text != "recovered" || !detailsOK || !reflect.DeepEqual(details, map[string]any{"source": "after"}) || !usageOK || !reflect.DeepEqual(gotUsage, usage) {
			t.Fatalf("overridden ToolResult = %#v", result)
		}
		assistant := created.State().Messages[1].(ai.AssistantMessage)
		if call := assistant.Content[0].(ai.ToolCall); call.Arguments["count"] != "2" {
			t.Fatalf("after hook mutated transcript ToolCall = %#v", call.Arguments)
		}
	})

	t.Run("hook failure becomes error ToolResult", func(t *testing.T) {
		tool, err := agent.EraseAgentTool(agent.AgentTool[map[string]any, map[string]any]{
			Tool:            ai.Tool{Name: "finish", Description: "Finish", Parameters: json.RawMessage(`{"type":"object"}`)},
			DecodeValidated: func(value ai.JSONValue) map[string]any { return value.(map[string]any) },
			Execute: func(context.Context, string, map[string]any, agent.AgentToolUpdateCallback[map[string]any]) (agent.AgentToolResult[map[string]any], error) {
				return agent.AgentToolResult[map[string]any]{Terminate: ai.Some(true)}, nil
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		created := newSingleToolAgentWithOptions(t, "finish", map[string]any{}, []agent.ErasedAgentTool{tool}, func(options *agent.AgentOptions) {
			options.AfterToolCall = func(context.Context, agent.AfterToolCallContext) (*agent.AfterToolCallResult, error) {
				return nil, errors.New("after failed")
			}
		})
		if err := created.Prompt(context.Background(), ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText("go"), Timestamp: 1}); err != nil {
			t.Fatalf("Prompt() error = %v", err)
		}
		result := created.State().Messages[2].(ai.ToolResultMessage)
		if !result.IsError || result.Content[0].(ai.TextContent).Text != "after failed" {
			t.Fatalf("AfterToolCall failure result = %#v", result)
		}
	})

	t.Run("explicit null overrides replace executed fields", func(t *testing.T) {
		usage := ai.Usage{Output: 3, TotalTokens: 3}
		tool, err := agent.EraseAgentTool(agent.AgentTool[map[string]any, map[string]any]{
			Tool:            ai.Tool{Name: "finish", Description: "Finish", Parameters: json.RawMessage(`{"type":"object"}`)},
			DecodeValidated: func(value ai.JSONValue) map[string]any { return value.(map[string]any) },
			Execute: func(context.Context, string, map[string]any, agent.AgentToolUpdateCallback[map[string]any]) (agent.AgentToolResult[map[string]any], error) {
				return agent.AgentToolResult[map[string]any]{
					Content: []ai.ToolResultContent{ai.TextContent{Type: ai.ContentTypeText, Text: "original"}},
					Details: map[string]any{"source": "execute"}, Usage: ai.Some(usage), Terminate: ai.Some(true),
				}, nil
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		created := newSingleToolAgentWithOptions(t, "finish", map[string]any{}, []agent.ErasedAgentTool{tool}, func(options *agent.AgentOptions) {
			options.AfterToolCall = func(context.Context, agent.AfterToolCallContext) (*agent.AfterToolCallResult, error) {
				return &agent.AfterToolCallResult{
					Content: ai.Null[[]ai.ToolResultContent](), Details: ai.Null[ai.JSONValue](), IsError: ai.Null[bool](),
					Usage: ai.Null[ai.Usage](), Terminate: ai.Null[bool](),
				}, nil
			}
		})
		err = created.Prompt(context.Background(), ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText("go"), Timestamp: 1})
		if err != nil {
			t.Fatalf("Prompt() error = %v", err)
		}
		result := created.State().Messages[2].(ai.ToolResultMessage)
		if result.IsError || len(result.Content) != 0 || !result.Details.IsNull() || !result.Usage.IsNull() {
			t.Fatalf("null-overridden ToolResult = %#v", result)
		}
	})

	t.Run("explicit null clears the error flag", func(t *testing.T) {
		tool, err := agent.EraseAgentTool(agent.AgentTool[map[string]any, map[string]any]{
			Tool:            ai.Tool{Name: "finish", Description: "Finish", Parameters: json.RawMessage(`{"type":"object"}`)},
			DecodeValidated: func(value ai.JSONValue) map[string]any { return value.(map[string]any) },
			Execute: func(context.Context, string, map[string]any, agent.AgentToolUpdateCallback[map[string]any]) (agent.AgentToolResult[map[string]any], error) {
				return agent.AgentToolResult[map[string]any]{}, errors.New("host failed")
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		created := newSingleToolAgentWithOptions(t, "finish", map[string]any{}, []agent.ErasedAgentTool{tool}, func(options *agent.AgentOptions) {
			options.AfterToolCall = func(context.Context, agent.AfterToolCallContext) (*agent.AfterToolCallResult, error) {
				return &agent.AfterToolCallResult{IsError: ai.Null[bool](), Terminate: ai.Some(true)}, nil
			}
		})
		if err := created.Prompt(context.Background(), ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText("go"), Timestamp: 1}); err != nil {
			t.Fatalf("Prompt() error = %v", err)
		}
		if result := created.State().Messages[2].(ai.ToolResultMessage); result.IsError {
			t.Fatalf("null-overridden IsError = true in %#v", result)
		}
	})
}

func TestAgentPropagatesToolPhaseCancellationCause(t *testing.T) {
	tests := []struct {
		name  string
		stage string
	}{
		{name: "argument preparation", stage: "prepare"},
		{name: "before hook", stage: "before"},
		{name: "Execute", stage: "execute"},
		{name: "after hook", stage: "after"},
		{name: "should-stop hook", stage: "should-stop"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cause := errors.New("stop requested during " + test.stage)
			ctx, cancel := context.WithCancelCause(context.Background())
			tool, err := agent.EraseAgentTool(agent.AgentTool[map[string]any, map[string]any]{
				Tool: ai.Tool{Name: "finish", Description: "Finish", Parameters: json.RawMessage(`{"type":"object"}`)},
				PrepareArguments: func(value ai.JSONValue) (ai.JSONValue, error) {
					if test.stage == "prepare" {
						cancel(cause)
						return value, errors.New("ordinary preparation error")
					}
					return value, nil
				},
				DecodeValidated: func(value ai.JSONValue) map[string]any { return value.(map[string]any) },
				Execute: func(context.Context, string, map[string]any, agent.AgentToolUpdateCallback[map[string]any]) (agent.AgentToolResult[map[string]any], error) {
					if test.stage == "execute" {
						cancel(cause)
						return agent.AgentToolResult[map[string]any]{}, errors.New("ordinary Execute error")
					}
					return agent.AgentToolResult[map[string]any]{Terminate: ai.Some(true)}, nil
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			created := newSingleToolAgentWithOptions(t, "finish", map[string]any{}, []agent.ErasedAgentTool{tool}, func(options *agent.AgentOptions) {
				if test.stage == "before" {
					options.BeforeToolCall = func(context.Context, agent.BeforeToolCallContext) (*agent.BeforeToolCallResult, error) {
						cancel(cause)
						return nil, errors.New("ordinary before-hook error")
					}
				}
				if test.stage == "after" {
					options.AfterToolCall = func(context.Context, agent.AfterToolCallContext) (*agent.AfterToolCallResult, error) {
						cancel(cause)
						return nil, errors.New("ordinary after-hook error")
					}
				}
				if test.stage == "should-stop" {
					options.ShouldStopAfterTurn = func(context.Context, agent.ShouldStopAfterTurnContext) (bool, error) {
						cancel(cause)
						return false, errors.New("ordinary should-stop error")
					}
				}
			})
			err = created.Prompt(ctx, ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText("go"), Timestamp: 1})
			if !errors.Is(err, cause) {
				t.Fatalf("Prompt() error = %v, want cancellation cause %v", err, cause)
			}
			wantMessages := 2
			if test.stage == "should-stop" {
				wantMessages = 3
			}
			if messages := created.State().Messages; len(messages) != wantMessages {
				t.Fatalf("messages after cancellation = %#v, want %d", messages, wantMessages)
			}
		})
	}
}

func TestAgentPropagatesToolFreeShouldStopCancellationCause(t *testing.T) {
	cause := errors.New("stop requested during tool-free should-stop")
	ctx, cancel := context.WithCancelCause(context.Background())
	response, err := ai.FauxAssistantMessage(ai.FauxAssistantBlocks(ai.TextContent{Type: ai.ContentTypeText, Text: "done"}), ai.FauxAssistantMessageOptions{
		StopReason: ai.Some(ai.StopReasonStop), Timestamp: ai.Some(int64(2)),
	})
	if err != nil {
		t.Fatal(err)
	}
	core, err := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	core.SetResponses([]ai.FauxResponseStep{response})
	model, _ := core.GetModel()
	created, err := agent.NewAgent(agent.AgentOptions{
		InitialState:   &agent.AgentInitialState{Model: model},
		StreamFunction: agent.StreamFunction(core.StreamSimple),
		ShouldStopAfterTurn: func(context.Context, agent.ShouldStopAfterTurnContext) (bool, error) {
			cancel(cause)
			return false, errors.New("ordinary should-stop error")
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	err = created.Prompt(ctx, ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText("go"), Timestamp: 1})
	if !errors.Is(err, cause) {
		t.Fatalf("Prompt() error = %v, want cancellation cause %v", err, cause)
	}
}

func TestAgentRejectsLengthTruncatedToolCallsWithoutHostSideEffects(t *testing.T) {
	var prepareCalls, decodeCalls, executeCalls atomic.Int64
	tool, err := agent.EraseAgentTool(agent.AgentTool[map[string]any, map[string]any]{
		Tool: ai.Tool{Name: "write", Description: "Write", Parameters: json.RawMessage(`{"type":"object","required":["content"],"properties":{"content":{"type":"string"}}}`)},
		PrepareArguments: func(value ai.JSONValue) (ai.JSONValue, error) {
			prepareCalls.Add(1)
			return value, nil
		},
		DecodeValidated: func(value ai.JSONValue) map[string]any {
			decodeCalls.Add(1)
			return value.(map[string]any)
		},
		Execute: func(context.Context, string, map[string]any, agent.AgentToolUpdateCallback[map[string]any]) (agent.AgentToolResult[map[string]any], error) {
			executeCalls.Add(1)
			return agent.AgentToolResult[map[string]any]{Terminate: ai.Some(true)}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	toolCall, err := ai.FauxToolCall("write", map[string]any{"content": "partial"}, ai.FauxToolCallOptions{ID: ai.Some("call-51")})
	if err != nil {
		t.Fatal(err)
	}
	response, err := ai.FauxAssistantMessage(ai.FauxAssistantBlocks(toolCall), ai.FauxAssistantMessageOptions{
		StopReason: ai.Some(ai.StopReasonLength), Timestamp: ai.Some(int64(2)),
	})
	if err != nil {
		t.Fatal(err)
	}
	core, err := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	core.SetResponses([]ai.FauxResponseStep{response})
	model, _ := core.GetModel()
	created, err := agent.NewAgent(agent.AgentOptions{
		InitialState: &agent.AgentInitialState{Model: model, Tools: []agent.ErasedAgentTool{tool}}, StreamFunction: agent.StreamFunction(core.StreamSimple),
		ShouldStopAfterTurn: func(context.Context, agent.ShouldStopAfterTurnContext) (bool, error) { return true, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := created.Prompt(context.Background(), ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText("write"), Timestamp: 1}); err != nil {
		t.Fatal(err)
	}
	if prepareCalls.Load() != 0 || decodeCalls.Load() != 0 || executeCalls.Load() != 0 {
		t.Fatalf("truncated call side effects = prepare %d decode %d execute %d", prepareCalls.Load(), decodeCalls.Load(), executeCalls.Load())
	}
	result := created.State().Messages[2].(ai.ToolResultMessage)
	if !result.IsError || !strings.Contains(result.Content[0].(ai.TextContent).Text, "output token limit") {
		t.Fatalf("truncated ToolResult = %#v", result)
	}
}

func TestAgentWaitsForAcceptedToolUpdateListenersBeforeFinalResult(t *testing.T) {
	updateListenerStarted := make(chan struct{})
	releaseUpdateListener := make(chan struct{})
	executeSettled := make(chan struct{})
	releaseLateUpdate := make(chan struct{})
	lateUpdateReturned := make(chan struct{})
	toolEndSeen := make(chan struct{})
	var updateEvents atomic.Int64

	tool, err := agent.EraseAgentTool(agent.AgentTool[map[string]any, map[string]any]{
		Tool:            ai.Tool{Name: "finish", Description: "Finish", Parameters: json.RawMessage(`{"type":"object"}`)},
		DecodeValidated: func(value ai.JSONValue) map[string]any { return value.(map[string]any) },
		Execute: func(_ context.Context, _ string, _ map[string]any, update agent.AgentToolUpdateCallback[map[string]any]) (agent.AgentToolResult[map[string]any], error) {
			update(agent.AgentToolResult[map[string]any]{Details: map[string]any{"step": "accepted"}})
			close(executeSettled)
			go func() {
				<-releaseLateUpdate
				update(agent.AgentToolResult[map[string]any]{Details: map[string]any{"step": "late"}})
				close(lateUpdateReturned)
			}()
			return agent.AgentToolResult[map[string]any]{Details: map[string]any{"done": true}, Terminate: ai.Some(true)}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	created := newSingleToolAgent(t, "finish", map[string]any{}, []agent.ErasedAgentTool{tool}, nil)
	created.Subscribe(func(_ context.Context, event agent.AgentEvent) error {
		switch event.AgentEventType() {
		case agent.AgentEventTypeToolExecutionUpdate:
			if updateEvents.Add(1) == 1 {
				close(updateListenerStarted)
				<-releaseUpdateListener
			}
		case agent.AgentEventTypeToolExecutionEnd:
			close(toolEndSeen)
		}
		return nil
	})
	promptDone := make(chan error, 1)
	go func() {
		promptDone <- created.Prompt(context.Background(), ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText("go"), Timestamp: 1})
	}()
	select {
	case <-updateListenerStarted:
	case <-time.After(time.Second):
		t.Fatal("update listener did not start")
	}
	select {
	case <-executeSettled:
	case <-time.After(time.Second):
		t.Fatal("Tool Execute did not settle independently of its accepted update listener")
	}
	select {
	case <-toolEndSeen:
		t.Fatal("tool_execution_end overtook an accepted update listener")
	case <-time.After(20 * time.Millisecond):
	}
	close(releaseUpdateListener)
	select {
	case err := <-promptDone:
		if err != nil {
			t.Fatalf("Prompt() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Prompt() did not settle after the accepted update listener")
	}
	close(releaseLateUpdate)
	select {
	case <-lateUpdateReturned:
	case <-time.After(time.Second):
		t.Fatal("late update callback remained blocked")
	}
	if updateEvents.Load() != 1 {
		t.Fatalf("tool update events = %d, want only the accepted update", updateEvents.Load())
	}
}

func TestAgentPropagatesAcceptedToolUpdateListenerFailureBeforeFinalResult(t *testing.T) {
	tool, err := agent.EraseAgentTool(agent.AgentTool[map[string]any, map[string]any]{
		Tool:            ai.Tool{Name: "finish", Description: "Finish", Parameters: json.RawMessage(`{"type":"object"}`)},
		DecodeValidated: func(value ai.JSONValue) map[string]any { return value.(map[string]any) },
		Execute: func(_ context.Context, _ string, _ map[string]any, update agent.AgentToolUpdateCallback[map[string]any]) (agent.AgentToolResult[map[string]any], error) {
			update(agent.AgentToolResult[map[string]any]{Details: map[string]any{"step": 1}})
			return agent.AgentToolResult[map[string]any]{Terminate: ai.Some(true)}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	created := newSingleToolAgent(t, "finish", map[string]any{}, []agent.ErasedAgentTool{tool}, nil)
	created.Subscribe(func(_ context.Context, event agent.AgentEvent) error {
		if event.AgentEventType() == agent.AgentEventTypeToolExecutionUpdate {
			return errors.New("update listener failed")
		}
		return nil
	})
	err = created.Prompt(context.Background(), ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText("go"), Timestamp: 1})
	if err == nil || err.Error() != "update listener failed" {
		t.Fatalf("Prompt() error = %v, want update listener failure", err)
	}
	if messages := created.State().Messages; len(messages) != 2 {
		t.Fatalf("messages after listener failure = %#v, want no final ToolResult", messages)
	}
}

func TestAgentAppliesPiCompatibleNestedAndUnionCoercion(t *testing.T) {
	type parameters struct {
		Values   []int
		Nullable any
		Choice   any
		Spaced   float64
		Hex      int
		Binary   int
		Octal    int
	}
	var received parameters
	tool, err := agent.EraseAgentTool(agent.AgentTool[parameters, map[string]any]{
		Tool: ai.Tool{Name: "coerce", Description: "Coerce", Parameters: json.RawMessage(`{
			"type":"object","required":["values","nullable","choice","spaced","hex","binary","octal"],"properties":{
				"values":{"type":"array","items":{"type":"integer","minimum":1}},
				"nullable":{"type":["array","null"],"items":{"type":"string"}},
				"choice":{"anyOf":[{"type":"number"},{"type":"null"}]},
				"spaced":{"type":"number"},"hex":{"type":"integer"},
				"binary":{"type":"integer"},"octal":{"type":"integer"}
			}}`)},
		DecodeValidated: func(value ai.JSONValue) parameters {
			arguments := value.(map[string]any)
			values := arguments["values"].([]any)
			return parameters{
				Values: []int{int(values[0].(float64)), int(values[1].(float64))}, Nullable: arguments["nullable"], Choice: arguments["choice"],
				Spaced: arguments["spaced"].(float64), Hex: int(arguments["hex"].(float64)), Binary: int(arguments["binary"].(float64)), Octal: int(arguments["octal"].(float64)),
			}
		},
		Execute: func(_ context.Context, _ string, params parameters, _ agent.AgentToolUpdateCallback[map[string]any]) (agent.AgentToolResult[map[string]any], error) {
			received = params
			return agent.AgentToolResult[map[string]any]{Terminate: ai.Some(true)}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	created := newSingleToolAgent(t, "coerce", map[string]any{
		"values": []any{"1", "2"}, "nullable": nil, "choice": "3.5",
		"spaced": " 42 ", "hex": "0x10", "binary": "0b11", "octal": "0o10",
	}, []agent.ErasedAgentTool{tool}, nil)
	if err := created.Prompt(context.Background(), ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText("go"), Timestamp: 1}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(received.Values, []int{1, 2}) || received.Nullable != nil || received.Choice != float64(3.5) || received.Spaced != 42 || received.Hex != 16 || received.Binary != 3 || received.Octal != 8 {
		t.Fatalf("coerced parameters = %#v", received)
	}
}

func TestAgentParallelToolBatchPreflightsBeforeExecutionAndCollatesInSourceOrder(t *testing.T) {
	var mu sync.Mutex
	preflightOrder := make([]string, 0, 2)
	executionSawAllPreflights := true
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})

	tool, err := agent.EraseAgentTool(agent.AgentTool[map[string]any, map[string]any]{
		Tool: ai.Tool{
			Name: "work", Description: "Run controlled work",
			Parameters: json.RawMessage(`{"type":"object","required":["name"],"properties":{"name":{"type":"string"}}}`),
		},
		DecodeValidated: func(value ai.JSONValue) map[string]any { return value.(map[string]any) },
		Execute: func(_ context.Context, callID string, _ map[string]any, _ agent.AgentToolUpdateCallback[map[string]any]) (agent.AgentToolResult[map[string]any], error) {
			mu.Lock()
			executionSawAllPreflights = executionSawAllPreflights && len(preflightOrder) == 2
			mu.Unlock()
			if callID == "call-1" {
				close(firstStarted)
				<-releaseFirst
			} else {
				<-firstStarted
			}
			return agent.AgentToolResult[map[string]any]{
				Content: []ai.ToolResultContent{ai.TextContent{Type: ai.ContentTypeText, Text: callID}},
				Details: map[string]any{"call": callID}, Terminate: ai.Some(true),
			}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	created := newToolBatchAgent(t, []agent.AgentToolCall{
		{Type: ai.ContentTypeToolCall, ID: "call-1", Name: "work", Arguments: map[string]any{"name": "first"}},
		{Type: ai.ContentTypeToolCall, ID: "call-2", Name: "work", Arguments: map[string]any{"name": "second"}},
	}, []agent.ErasedAgentTool{tool}, func(options *agent.AgentOptions) {
		options.BeforeToolCall = func(_ context.Context, input agent.BeforeToolCallContext) (*agent.BeforeToolCallResult, error) {
			mu.Lock()
			preflightOrder = append(preflightOrder, input.ToolCall.ID)
			mu.Unlock()
			return nil, nil
		}
	})
	var completionOrder, resultOrder []string
	var pendingAtSecondStart, pendingAfterSecondEnd, pendingAfterFirstEnd []string
	created.Subscribe(func(_ context.Context, event agent.AgentEvent) error {
		mu.Lock()
		defer mu.Unlock()
		switch event := event.(type) {
		case agent.ToolExecutionStartEvent:
			if event.ToolCallID == "call-2" {
				pendingAtSecondStart = sortedPendingToolCallIDs(created.State().PendingToolCalls)
			}
		case agent.ToolExecutionEndEvent:
			completionOrder = append(completionOrder, event.ToolCallID)
			if event.ToolCallID == "call-2" {
				pendingAfterSecondEnd = sortedPendingToolCallIDs(created.State().PendingToolCalls)
				close(releaseFirst)
			} else {
				pendingAfterFirstEnd = sortedPendingToolCallIDs(created.State().PendingToolCalls)
			}
		case agent.MessageEndEvent:
			if result, ok := event.Message.(ai.ToolResultMessage); ok {
				resultOrder = append(resultOrder, result.ToolCallID)
			}
		}
		return nil
	})

	if err := created.Prompt(context.Background(), ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText("run both"), Timestamp: 1}); err != nil {
		t.Fatalf("Prompt() error = %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if !executionSawAllPreflights || !reflect.DeepEqual(preflightOrder, []string{"call-1", "call-2"}) {
		t.Fatalf("preflight order = %v, executionSawAllPreflights = %t", preflightOrder, executionSawAllPreflights)
	}
	if !reflect.DeepEqual(completionOrder, []string{"call-2", "call-1"}) {
		t.Fatalf("tool_execution_end order = %v, want completion order", completionOrder)
	}
	if !reflect.DeepEqual(resultOrder, []string{"call-1", "call-2"}) {
		t.Fatalf("ToolResult event order = %v, want source order", resultOrder)
	}
	if !reflect.DeepEqual(pendingAtSecondStart, []string{"call-1", "call-2"}) || !reflect.DeepEqual(pendingAfterSecondEnd, []string{"call-1"}) || len(pendingAfterFirstEnd) != 0 {
		t.Fatalf("pending snapshots = at second start %v, after second end %v, after first end %v", pendingAtSecondStart, pendingAfterSecondEnd, pendingAfterFirstEnd)
	}
	state := created.State()
	if len(state.Messages) != 4 || state.Messages[2].(ai.ToolResultMessage).ToolCallID != "call-1" || state.Messages[3].(ai.ToolResultMessage).ToolCallID != "call-2" {
		t.Fatalf("transcript = %#v, want source-ordered ToolResults", state.Messages)
	}
}

func TestAgentSequentialToolForcesWholeBatchSerialAfterPreflightBarrier(t *testing.T) {
	for _, test := range []struct {
		name      string
		batchMode agent.ToolExecutionMode
		toolMode  agent.ToolExecutionMode
	}{
		{name: "Tool override", batchMode: agent.ToolExecutionParallel, toolMode: agent.ToolExecutionSequential},
		{name: "global override", batchMode: agent.ToolExecutionSequential, toolMode: agent.ToolExecutionParallel},
	} {
		t.Run(test.name, func(t *testing.T) {
			var mu sync.Mutex
			preflightOrder := make([]string, 0, 2)
			executionOrder := make([]string, 0, 2)
			executionSawAllPreflights := true
			tool, err := agent.EraseAgentTool(agent.AgentTool[map[string]any, map[string]any]{
				Tool: ai.Tool{
					Name: "serial", Description: "Run serial work",
					Parameters: json.RawMessage(`{"type":"object","required":["name"],"properties":{"name":{"type":"string"}}}`),
				},
				DecodeValidated: func(value ai.JSONValue) map[string]any { return value.(map[string]any) },
				Execute: func(_ context.Context, callID string, _ map[string]any, _ agent.AgentToolUpdateCallback[map[string]any]) (agent.AgentToolResult[map[string]any], error) {
					mu.Lock()
					executionSawAllPreflights = executionSawAllPreflights && len(preflightOrder) == 2
					executionOrder = append(executionOrder, callID)
					mu.Unlock()
					return agent.AgentToolResult[map[string]any]{Terminate: ai.Some(true)}, nil
				},
				ExecutionMode: test.toolMode,
			})
			if err != nil {
				t.Fatal(err)
			}
			created := newToolBatchAgent(t, []agent.AgentToolCall{
				{Type: ai.ContentTypeToolCall, ID: "call-1", Name: "serial", Arguments: map[string]any{"name": "first"}},
				{Type: ai.ContentTypeToolCall, ID: "call-2", Name: "serial", Arguments: map[string]any{"name": "second"}},
			}, []agent.ErasedAgentTool{tool}, func(options *agent.AgentOptions) {
				options.ToolExecution = test.batchMode
				options.BeforeToolCall = func(_ context.Context, input agent.BeforeToolCallContext) (*agent.BeforeToolCallResult, error) {
					mu.Lock()
					preflightOrder = append(preflightOrder, input.ToolCall.ID)
					mu.Unlock()
					return nil, nil
				}
			})
			if err := created.Prompt(context.Background(), ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText("run both"), Timestamp: 1}); err != nil {
				t.Fatalf("Prompt() error = %v", err)
			}
			mu.Lock()
			defer mu.Unlock()
			if !executionSawAllPreflights {
				t.Fatalf("execution began before all preflights: preflight %v, execution %v", preflightOrder, executionOrder)
			}
			if !reflect.DeepEqual(preflightOrder, []string{"call-1", "call-2"}) || !reflect.DeepEqual(executionOrder, []string{"call-1", "call-2"}) {
				t.Fatalf("preflight order = %v, execution order = %v", preflightOrder, executionOrder)
			}
		})
	}
}

func TestAgentParallelToolBatchStartsEveryCallWithoutWorkerLimit(t *testing.T) {
	const callCount = 64
	allStarted := make(chan struct{})
	release := make(chan struct{})
	var started atomic.Int64
	tool, err := agent.EraseAgentTool(agent.AgentTool[map[string]any, map[string]any]{
		Tool:            ai.Tool{Name: "work", Description: "Run controlled work", Parameters: json.RawMessage(`{"type":"object"}`)},
		DecodeValidated: func(value ai.JSONValue) map[string]any { return value.(map[string]any) },
		Execute: func(context.Context, string, map[string]any, agent.AgentToolUpdateCallback[map[string]any]) (agent.AgentToolResult[map[string]any], error) {
			if started.Add(1) == callCount {
				close(allStarted)
			}
			<-release
			return agent.AgentToolResult[map[string]any]{Terminate: ai.Some(true)}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	calls := make([]agent.AgentToolCall, callCount)
	for i := range calls {
		calls[i] = agent.AgentToolCall{Type: ai.ContentTypeToolCall, ID: fmt.Sprintf("call-%02d", i), Name: "work", Arguments: map[string]any{}}
	}
	created := newToolBatchAgent(t, calls, []agent.ErasedAgentTool{tool}, nil)
	done := make(chan error, 1)
	go func() {
		done <- created.Prompt(context.Background(), ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText("run all"), Timestamp: 1})
	}()
	select {
	case <-allStarted:
	case <-time.After(5 * time.Second):
		close(release)
		t.Fatalf("started %d of %d calls before release; parallel batch appears worker-limited", started.Load(), callCount)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatalf("Prompt() error = %v", err)
	}
}

func TestAgentToolBatchTerminatesOnlyWhenEveryFinalResultTerminates(t *testing.T) {
	tests := []struct {
		name              string
		terminates        []bool
		wantProviderCalls int
		wantRoles         []ai.MessageRole
	}{
		{name: "all terminating", terminates: []bool{true, true}, wantProviderCalls: 1, wantRoles: []ai.MessageRole{ai.MessageRoleUser, ai.MessageRoleAssistant, ai.MessageRoleToolResult, ai.MessageRoleToolResult}},
		{name: "mixed", terminates: []bool{true, false}, wantProviderCalls: 2, wantRoles: []ai.MessageRole{ai.MessageRoleUser, ai.MessageRoleAssistant, ai.MessageRoleToolResult, ai.MessageRoleToolResult, ai.MessageRoleAssistant}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tool, err := agent.EraseAgentTool(agent.AgentTool[map[string]any, map[string]any]{
				Tool: ai.Tool{
					Name: "work", Description: "Run work",
					Parameters: json.RawMessage(`{"type":"object","required":["terminate"],"properties":{"terminate":{"type":"boolean"}}}`),
				},
				DecodeValidated: func(value ai.JSONValue) map[string]any { return value.(map[string]any) },
				Execute: func(_ context.Context, _ string, params map[string]any, _ agent.AgentToolUpdateCallback[map[string]any]) (agent.AgentToolResult[map[string]any], error) {
					return agent.AgentToolResult[map[string]any]{Terminate: ai.Some(params["terminate"].(bool))}, nil
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			calls := make([]ai.AssistantContent, len(test.terminates))
			for i, terminate := range test.terminates {
				calls[i] = agent.AgentToolCall{Type: ai.ContentTypeToolCall, ID: fmt.Sprintf("call-%d", i+1), Name: "work", Arguments: map[string]any{"terminate": terminate}}
			}
			batchResponse, err := ai.FauxAssistantMessage(ai.FauxAssistantBlocks(calls...), ai.FauxAssistantMessageOptions{StopReason: ai.Some(ai.StopReasonToolUse)})
			if err != nil {
				t.Fatal(err)
			}
			finalResponse, err := ai.FauxAssistantMessage(ai.FauxAssistantText("continued"))
			if err != nil {
				t.Fatal(err)
			}
			core, err := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
			if err != nil {
				t.Fatal(err)
			}
			core.SetResponses([]ai.FauxResponseStep{batchResponse, finalResponse})
			model, _ := core.GetModel()
			created, err := agent.NewAgent(agent.AgentOptions{
				InitialState:   &agent.AgentInitialState{Model: model, Tools: []agent.ErasedAgentTool{tool}},
				StreamFunction: agent.StreamFunction(core.StreamSimple),
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := created.Prompt(context.Background(), ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText("run both"), Timestamp: 1}); err != nil {
				t.Fatalf("Prompt() error = %v", err)
			}
			if core.State.CallCount != test.wantProviderCalls {
				t.Fatalf("Provider call count = %d, want %d", core.State.CallCount, test.wantProviderCalls)
			}
			if roles := messageRolesFromAgent(created.State().Messages); !reflect.DeepEqual(roles, test.wantRoles) {
				t.Fatalf("transcript roles = %v, want %v", roles, test.wantRoles)
			}
		})
	}
}

func TestAgentParallelToolBatchCancellationFinalizesAndJoinsEveryStartedCall(t *testing.T) {
	baselineGoroutines := runtime.NumGoroutine()
	cause := errors.New("cancel the batch")
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	bothStarted := make(chan struct{})
	cancellationTriggered := make(chan struct{})
	releaseCanceledCall := make(chan struct{})
	secondExited := make(chan struct{})
	var started atomic.Int64
	tool, err := agent.EraseAgentTool(agent.AgentTool[map[string]any, map[string]any]{
		Tool:            ai.Tool{Name: "work", Description: "Run controlled work", Parameters: json.RawMessage(`{"type":"object"}`)},
		DecodeValidated: func(value ai.JSONValue) map[string]any { return value.(map[string]any) },
		Execute: func(runContext context.Context, callID string, _ map[string]any, _ agent.AgentToolUpdateCallback[map[string]any]) (agent.AgentToolResult[map[string]any], error) {
			if started.Add(1) == 2 {
				close(bothStarted)
			}
			<-bothStarted
			if callID == "call-1" {
				<-cancellationTriggered
				return agent.AgentToolResult[map[string]any]{
					Content:   []ai.ToolResultContent{ai.TextContent{Type: ai.ContentTypeText, Text: "finished"}},
					Terminate: ai.Some(true),
				}, nil
			}
			cancel(cause)
			close(cancellationTriggered)
			<-runContext.Done()
			<-releaseCanceledCall
			close(secondExited)
			return agent.AgentToolResult[map[string]any]{}, context.Cause(runContext)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	created := newToolBatchAgent(t, []agent.AgentToolCall{
		{Type: ai.ContentTypeToolCall, ID: "call-1", Name: "work", Arguments: map[string]any{}},
		{Type: ai.ContentTypeToolCall, ID: "call-2", Name: "work", Arguments: map[string]any{}},
	}, []agent.ErasedAgentTool{tool}, nil)
	var mu sync.Mutex
	var endOrder, resultOrder []string
	var turnEnds, agentEnds, canceledListenerCalls int
	created.Subscribe(func(runContext context.Context, event agent.AgentEvent) error {
		mu.Lock()
		defer mu.Unlock()
		switch event := event.(type) {
		case agent.ToolExecutionEndEvent:
			endOrder = append(endOrder, event.ToolCallID)
			if event.ToolCallID == "call-1" {
				close(releaseCanceledCall)
			}
		case agent.MessageEndEvent:
			if result, ok := event.Message.(ai.ToolResultMessage); ok {
				resultOrder = append(resultOrder, result.ToolCallID)
			}
		case agent.TurnEndEvent:
			turnEnds++
		case agent.AgentEndEvent:
			agentEnds++
		}
		if cancellation := context.Cause(runContext); cancellation != nil {
			canceledListenerCalls++
			return cancellation
		}
		return nil
	})

	err = created.Prompt(ctx, ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText("run both"), Timestamp: 1})
	if !errors.Is(err, cause) {
		t.Fatalf("Prompt() error = %v, want cancellation cause %v", err, cause)
	}
	select {
	case <-secondExited:
	default:
		t.Fatal("Prompt settled before the canceled Tool worker exited")
	}
	mu.Lock()
	if !reflect.DeepEqual(endOrder, []string{"call-1", "call-2"}) || !reflect.DeepEqual(resultOrder, []string{"call-1", "call-2"}) || turnEnds != 1 || agentEnds != 1 || canceledListenerCalls == 0 {
		t.Fatalf("end order = %v, result order = %v, turn ends = %d, agent ends = %d, canceled listener calls = %d", endOrder, resultOrder, turnEnds, agentEnds, canceledListenerCalls)
	}
	eventCount := len(endOrder) + len(resultOrder) + turnEnds + agentEnds
	mu.Unlock()
	state := created.State()
	if created.Busy() || len(state.PendingToolCalls) != 0 || len(state.Messages) != 4 {
		t.Fatalf("settled state = busy %t, pending %v, messages %#v", created.Busy(), state.PendingToolCalls, state.Messages)
	}
	canceledResult := state.Messages[3].(ai.ToolResultMessage)
	completedResult := state.Messages[2].(ai.ToolResultMessage)
	if completedResult.IsError || len(completedResult.Content) != 1 || completedResult.Content[0].(ai.TextContent).Text != "finished" {
		t.Fatalf("completed ToolResult was overwritten by cancellation: %#v", completedResult)
	}
	if !canceledResult.IsError || len(canceledResult.Content) != 1 || canceledResult.Content[0].(ai.TextContent).Text != "Operation aborted" {
		t.Fatalf("canceled ToolResult = %#v", canceledResult)
	}
	time.Sleep(20 * time.Millisecond)
	mu.Lock()
	if got := len(endOrder) + len(resultOrder) + turnEnds + agentEnds; got != eventCount {
		mu.Unlock()
		t.Fatalf("late final events after Prompt settled: before %d, after %d", eventCount, got)
	}
	mu.Unlock()
	assertGoroutineCountReturnsToBaseline(t, baselineGoroutines)
}

func assertGoroutineCountReturnsToBaseline(t *testing.T, baseline int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for runtime.NumGoroutine() > baseline && time.Now().Before(deadline) {
		runtime.Gosched()
		time.Sleep(time.Millisecond)
	}
	if got := runtime.NumGoroutine(); got > baseline {
		stack := make([]byte, 1<<20)
		n := runtime.Stack(stack, true)
		t.Fatalf("goroutines after canceled batch = %d, baseline = %d; active stacks:\n%s", got, baseline, stack[:n])
	}
}

func TestAgentToolBatchCancellationDuringPreflightSkipsExecutionAndFinalizesEveryCall(t *testing.T) {
	for _, mode := range []agent.ToolExecutionMode{agent.ToolExecutionParallel, agent.ToolExecutionSequential} {
		t.Run(string(mode), func(t *testing.T) {
			cause := errors.New("cancel during preflight")
			ctx, cancel := context.WithCancelCause(context.Background())
			defer cancel(nil)
			var executions atomic.Int64
			tool, err := agent.EraseAgentTool(agent.AgentTool[map[string]any, map[string]any]{
				Tool:            ai.Tool{Name: "work", Description: "Run work", Parameters: json.RawMessage(`{"type":"object"}`)},
				DecodeValidated: func(value ai.JSONValue) map[string]any { return value.(map[string]any) },
				Execute: func(context.Context, string, map[string]any, agent.AgentToolUpdateCallback[map[string]any]) (agent.AgentToolResult[map[string]any], error) {
					executions.Add(1)
					return agent.AgentToolResult[map[string]any]{}, nil
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			created := newToolBatchAgent(t, []agent.AgentToolCall{
				{Type: ai.ContentTypeToolCall, ID: "call-1", Name: "work", Arguments: map[string]any{}},
				{Type: ai.ContentTypeToolCall, ID: "call-2", Name: "work", Arguments: map[string]any{}},
			}, []agent.ErasedAgentTool{tool}, func(options *agent.AgentOptions) {
				options.ToolExecution = mode
				options.BeforeToolCall = func(_ context.Context, input agent.BeforeToolCallContext) (*agent.BeforeToolCallResult, error) {
					if input.ToolCall.ID == "call-2" {
						cancel(cause)
					}
					return nil, nil
				}
			})
			var endIDs []string
			created.Subscribe(func(_ context.Context, event agent.AgentEvent) error {
				if event, ok := event.(agent.ToolExecutionEndEvent); ok {
					endIDs = append(endIDs, event.ToolCallID)
				}
				return nil
			})
			err = created.Prompt(ctx, ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText("run both"), Timestamp: 1})
			if !errors.Is(err, cause) {
				t.Fatalf("Prompt() error = %v, want %v", err, cause)
			}
			if executions.Load() != 0 || !reflect.DeepEqual(endIDs, []string{"call-1", "call-2"}) {
				t.Fatalf("executions = %d, end IDs = %v", executions.Load(), endIDs)
			}
			state := created.State()
			if len(state.Messages) != 4 {
				t.Fatalf("transcript = %#v, want two cancellation ToolResults", state.Messages)
			}
			for _, message := range state.Messages[2:] {
				result := message.(ai.ToolResultMessage)
				if !result.IsError || result.Content[0].(ai.TextContent).Text != "Operation aborted" {
					t.Fatalf("cancellation ToolResult = %#v", result)
				}
			}
		})
	}
}

func TestAgentParallelToolBatchDrainsAcceptedUpdatesBeforeFinalEvents(t *testing.T) {
	listenerStarted := make(chan struct{})
	releaseListener := make(chan struct{})
	allExecutionsSettled := make(chan struct{})
	var settled atomic.Int64
	tool, err := agent.EraseAgentTool(agent.AgentTool[map[string]any, map[string]any]{
		Tool:            ai.Tool{Name: "work", Description: "Run work", Parameters: json.RawMessage(`{"type":"object"}`)},
		DecodeValidated: func(value ai.JSONValue) map[string]any { return value.(map[string]any) },
		Execute: func(_ context.Context, callID string, _ map[string]any, update agent.AgentToolUpdateCallback[map[string]any]) (agent.AgentToolResult[map[string]any], error) {
			update(agent.AgentToolResult[map[string]any]{Details: map[string]any{"call": callID}})
			if settled.Add(1) == 2 {
				close(allExecutionsSettled)
			}
			return agent.AgentToolResult[map[string]any]{Terminate: ai.Some(true)}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	created := newToolBatchAgent(t, []agent.AgentToolCall{
		{Type: ai.ContentTypeToolCall, ID: "call-1", Name: "work", Arguments: map[string]any{}},
		{Type: ai.ContentTypeToolCall, ID: "call-2", Name: "work", Arguments: map[string]any{}},
	}, []agent.ErasedAgentTool{tool}, nil)
	var mu sync.Mutex
	var eventOrder []string
	var updateListeners atomic.Int64
	created.Subscribe(func(_ context.Context, event agent.AgentEvent) error {
		var label string
		switch event := event.(type) {
		case agent.ToolExecutionUpdateEvent:
			label = "update:" + event.ToolCallID
		case agent.ToolExecutionEndEvent:
			label = "end:" + event.ToolCallID
		case agent.MessageStartEvent:
			if result, ok := event.Message.(ai.ToolResultMessage); ok {
				label = "result:" + result.ToolCallID
			}
		}
		if label != "" {
			mu.Lock()
			eventOrder = append(eventOrder, label)
			mu.Unlock()
		}
		if _, ok := event.(agent.ToolExecutionUpdateEvent); ok && updateListeners.Add(1) == 1 {
			close(listenerStarted)
			<-releaseListener
		}
		return nil
	})
	done := make(chan error, 1)
	go func() {
		done <- created.Prompt(context.Background(), ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText("run both"), Timestamp: 1})
	}()
	select {
	case <-listenerStarted:
	case <-time.After(time.Second):
		t.Fatal("update listener did not start")
	}
	select {
	case <-allExecutionsSettled:
	case <-time.After(time.Second):
		t.Fatal("parallel Execute calls did not settle while the update listener was blocked")
	}
	mu.Lock()
	for _, event := range eventOrder {
		if strings.HasPrefix(event, "end:") || strings.HasPrefix(event, "result:") {
			mu.Unlock()
			t.Fatalf("final event %q overtook an accepted update listener: %v", event, eventOrder)
		}
	}
	mu.Unlock()
	close(releaseListener)
	if err := <-done; err != nil {
		t.Fatalf("Prompt() error = %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	for _, callID := range []string{"call-1", "call-2"} {
		if indexOfString(eventOrder, "update:"+callID) >= indexOfString(eventOrder, "end:"+callID) {
			t.Fatalf("events = %v, update for %s must precede its final event", eventOrder, callID)
		}
	}
	firstResult := indexOfString(eventOrder, "result:call-1")
	if firstResult < indexOfString(eventOrder, "end:call-1") || firstResult < indexOfString(eventOrder, "end:call-2") {
		t.Fatalf("events = %v, ToolResults must wait for all final events", eventOrder)
	}
}

func indexOfString(values []string, target string) int {
	for i, value := range values {
		if value == target {
			return i
		}
	}
	return -1
}

func sortedPendingToolCallIDs(pending map[string]struct{}) []string {
	ids := make([]string, 0, len(pending))
	for id := range pending {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}

func newSingleToolAgent(
	t *testing.T,
	toolName string,
	arguments map[string]any,
	tools []agent.ErasedAgentTool,
	before func(context.Context, agent.BeforeToolCallContext) (*agent.BeforeToolCallResult, error),
) *agent.Agent {
	return newSingleToolAgentWithOptions(t, toolName, arguments, tools, func(options *agent.AgentOptions) {
		options.BeforeToolCall = before
	})
}

func newToolBatchAgent(t *testing.T, calls []agent.AgentToolCall, tools []agent.ErasedAgentTool, configure func(*agent.AgentOptions)) *agent.Agent {
	t.Helper()
	blocks := make([]ai.AssistantContent, len(calls))
	for i := range calls {
		blocks[i] = calls[i]
	}
	response, err := ai.FauxAssistantMessage(ai.FauxAssistantBlocks(blocks...), ai.FauxAssistantMessageOptions{
		StopReason: ai.Some(ai.StopReasonToolUse), Timestamp: ai.Some(int64(2)),
	})
	if err != nil {
		t.Fatal(err)
	}
	core, err := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	core.SetResponses([]ai.FauxResponseStep{response})
	model, _ := core.GetModel()
	options := agent.AgentOptions{
		InitialState: &agent.AgentInitialState{Model: model, Tools: tools}, StreamFunction: agent.StreamFunction(core.StreamSimple),
		ShouldStopAfterTurn: func(context.Context, agent.ShouldStopAfterTurnContext) (bool, error) { return true, nil },
	}
	if configure != nil {
		configure(&options)
	}
	created, err := agent.NewAgent(options)
	if err != nil {
		t.Fatal(err)
	}
	return created
}

func newSingleToolAgentWithOptions(
	t *testing.T,
	toolName string,
	arguments map[string]any,
	tools []agent.ErasedAgentTool,
	configure func(*agent.AgentOptions),
) *agent.Agent {
	t.Helper()
	toolCall, err := ai.FauxToolCall(toolName, arguments, ai.FauxToolCallOptions{ID: ai.Some("call-51")})
	if err != nil {
		t.Fatal(err)
	}
	response, err := ai.FauxAssistantMessage(ai.FauxAssistantBlocks(toolCall), ai.FauxAssistantMessageOptions{
		StopReason: ai.Some(ai.StopReasonToolUse), Timestamp: ai.Some(int64(2)),
	})
	if err != nil {
		t.Fatal(err)
	}
	core, err := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	core.SetResponses([]ai.FauxResponseStep{response})
	model, _ := core.GetModel()
	options := agent.AgentOptions{
		InitialState: &agent.AgentInitialState{Model: model, Tools: tools}, StreamFunction: agent.StreamFunction(core.StreamSimple),
		ShouldStopAfterTurn: func(context.Context, agent.ShouldStopAfterTurnContext) (bool, error) { return true, nil },
	}
	if configure != nil {
		configure(&options)
	}
	created, err := agent.NewAgent(options)
	if err != nil {
		t.Fatal(err)
	}
	return created
}

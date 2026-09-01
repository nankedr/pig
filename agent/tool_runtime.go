package agent

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/nankedr/pig/ai"
)

type preparedAgentToolCall struct {
	toolCall AgentToolCall
	tool     ErasedAgentTool
	args     validatedAgentToolArguments
}

type finalizedAgentToolCall struct {
	toolCall AgentToolCall
	result   ErasedAgentToolResult
	isError  bool
}

type agentToolUpdateDispatcher struct {
	mu      sync.Mutex
	changed *sync.Cond
	queue   []ErasedAgentToolResult
	closed  bool
	err     error
	done    chan struct{}

	ctx      context.Context
	emit     AgentEventSink
	toolCall AgentToolCall
}

func newAgentToolUpdateDispatcher(ctx context.Context, emit AgentEventSink, toolCall AgentToolCall) *agentToolUpdateDispatcher {
	dispatcher := &agentToolUpdateDispatcher{ctx: ctx, emit: emit, toolCall: toolCall, done: make(chan struct{})}
	dispatcher.changed = sync.NewCond(&dispatcher.mu)
	go dispatcher.run()
	return dispatcher
}

func (d *agentToolUpdateDispatcher) admit(update ErasedAgentToolResult) {
	d.mu.Lock()
	if !d.closed {
		d.queue = append(d.queue, cloneErasedAgentToolResult(update))
		d.changed.Signal()
	}
	d.mu.Unlock()
}

func (d *agentToolUpdateDispatcher) closeAndWait() error {
	d.mu.Lock()
	d.closed = true
	d.changed.Signal()
	d.mu.Unlock()
	<-d.done
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.err
}

func (d *agentToolUpdateDispatcher) run() {
	defer close(d.done)
	for {
		d.mu.Lock()
		for len(d.queue) == 0 && !d.closed {
			d.changed.Wait()
		}
		if len(d.queue) == 0 && d.closed {
			d.mu.Unlock()
			return
		}
		update := d.queue[0]
		d.queue[0] = ErasedAgentToolResult{}
		d.queue = d.queue[1:]
		d.mu.Unlock()

		err := emitAgentEvent(d.ctx, d.emit, ToolExecutionUpdateEvent{
			Type: AgentEventTypeToolExecutionUpdate, ToolCallID: d.toolCall.ID, ToolName: d.toolCall.Name,
			Arguments: d.toolCall.Arguments, PartialResult: update,
		})
		if err != nil {
			d.mu.Lock()
			if d.err == nil {
				d.err = err
			}
			d.mu.Unlock()
		}
	}
}

func assistantToolCalls(message ai.AssistantMessage) []AgentToolCall {
	var calls []AgentToolCall
	for _, content := range message.Content {
		switch content := content.(type) {
		case ai.ToolCall:
			calls = append(calls, content)
		case *ai.ToolCall:
			if content != nil {
				calls = append(calls, *content)
			}
		}
	}
	return calls
}

func finishSingleToolTurn(ctx context.Context, emit AgentEventSink, agentContext AgentContext, newMessages []AgentMessage, config AgentLoopConfig, message ai.AssistantMessage, toolCall AgentToolCall) ([]AgentMessage, error) {
	if err := emitAgentEvent(ctx, emit, ToolExecutionStartEvent{
		Type: AgentEventTypeToolExecutionStart, ToolCallID: toolCall.ID, ToolName: toolCall.Name, Arguments: toolCall.Arguments,
	}); err != nil {
		return nil, err
	}

	var finalized finalizedAgentToolCall
	if message.StopReason == ai.StopReasonLength {
		finalized = finalizedAgentToolCall{
			toolCall: toolCall,
			result:   errorAgentToolResult(fmt.Sprintf("Tool call %q was not executed: the response hit the output token limit, so its arguments may be truncated. Re-issue the tool call with complete arguments.", toolCall.Name)),
			isError:  true,
		}
	} else {
		prepared, immediate, err := prepareSingleAgentToolCall(ctx, agentContext, message, config, toolCall)
		if err != nil {
			return nil, err
		}
		if immediate != nil {
			finalized = *immediate
		} else {
			executed, err := executeSingleAgentToolCall(ctx, emit, prepared)
			if err != nil {
				return nil, err
			}
			finalized, err = finalizeSingleAgentToolCall(ctx, agentContext, message, config, prepared, executed)
			if err != nil {
				return nil, err
			}
		}
	}

	if err := emitAgentEvent(ctx, emit, ToolExecutionEndEvent{
		Type: AgentEventTypeToolExecutionEnd, ToolCallID: toolCall.ID, ToolName: toolCall.Name,
		Result: finalized.result, IsError: finalized.isError,
	}); err != nil {
		return nil, err
	}
	toolResult := createAgentToolResultMessage(finalized)
	if err := emitMessageLifecycle(ctx, emit, toolResult); err != nil {
		return nil, err
	}
	agentContext.Messages = append(agentContext.Messages, cloneToolResultMessage(toolResult))
	newMessages = append(newMessages, cloneToolResultMessage(toolResult))
	toolResults := []ai.ToolResultMessage{toolResult}
	if err := emitAgentEvent(ctx, emit, TurnEndEvent{Type: AgentEventTypeTurnEnd, Message: message, ToolResults: toolResults}); err != nil {
		return nil, err
	}
	shouldStop := false
	if config.ShouldStopAfterTurn != nil {
		callbackContext, err := cloneAgentContext(agentContext)
		if err != nil {
			return nil, err
		}
		shouldStop, err = config.ShouldStopAfterTurn(ctx, ShouldStopAfterTurnContext{
			Message: ai.CloneAssistantMessage(message), ToolResults: cloneToolResultMessages(toolResults), Context: callbackContext, NewMessages: cloneAgentMessages(newMessages),
		})
		if cause := context.Cause(ctx); cause != nil {
			return nil, cause
		}
		if err != nil {
			return nil, err
		}
	}
	terminate, _ := finalized.result.Terminate.Value()
	if !terminate && !shouldStop {
		return cloneAgentMessages(newMessages), newNotImplemented("Agent.ToolContinuation")
	}
	if err := emitAgentEvent(ctx, emit, AgentEndEvent{Type: AgentEventTypeAgentEnd, Messages: newMessages}); err != nil {
		return nil, err
	}
	return cloneAgentMessages(newMessages), nil
}

func prepareSingleAgentToolCall(ctx context.Context, agentContext AgentContext, assistantMessage ai.AssistantMessage, config AgentLoopConfig, toolCall AgentToolCall) (preparedAgentToolCall, *finalizedAgentToolCall, error) {
	if cause := context.Cause(ctx); cause != nil {
		return preparedAgentToolCall{}, nil, cause
	}
	tool, ok := findAgentTool(agentContext.Tools, toolCall.Name)
	if !ok {
		return preparedAgentToolCall{}, immediateAgentToolError(toolCall, fmt.Sprintf("Tool %s not found", toolCall.Name)), nil
	}
	arguments := cloneAgentJSONValue(toolCall.Arguments)
	preparedArguments, err := tool.prepareArguments(arguments)
	if cause := context.Cause(ctx); cause != nil {
		return preparedAgentToolCall{}, nil, cause
	}
	if err != nil {
		return preparedAgentToolCall{}, immediateAgentToolError(toolCall, err.Error()), nil
	}
	validated, err := validateAndSealAgentToolArguments(tool, preparedArguments)
	if cause := context.Cause(ctx); cause != nil {
		return preparedAgentToolCall{}, nil, cause
	}
	if err != nil {
		return preparedAgentToolCall{}, immediateAgentToolError(toolCall, err.Error()), nil
	}
	if config.BeforeToolCall != nil {
		callbackContext, err := cloneAgentContext(agentContext)
		if err != nil {
			return preparedAgentToolCall{}, immediateAgentToolError(toolCall, err.Error()), nil
		}
		before, err := config.BeforeToolCall(ctx, BeforeToolCallContext{
			AssistantMessage: ai.CloneAssistantMessage(assistantMessage), ToolCall: cloneAgentToolCall(toolCall), Args: cloneAgentJSONValue(validated.value), Context: callbackContext,
		})
		if cause := context.Cause(ctx); cause != nil {
			return preparedAgentToolCall{}, nil, cause
		}
		if err != nil {
			return preparedAgentToolCall{}, immediateAgentToolError(toolCall, err.Error()), nil
		}
		if before != nil && before.Block {
			reason := before.Reason
			if reason == "" {
				reason = "Tool execution was blocked"
			}
			immediate := immediateAgentToolError(toolCall, reason)
			if before.Terminate {
				immediate.result.Terminate = ai.Some(true)
			}
			return preparedAgentToolCall{}, immediate, nil
		}
	}
	if cause := context.Cause(ctx); cause != nil {
		return preparedAgentToolCall{}, nil, cause
	}
	return preparedAgentToolCall{toolCall: toolCall, tool: tool, args: validated}, nil, nil
}

func executeSingleAgentToolCall(ctx context.Context, emit AgentEventSink, prepared preparedAgentToolCall) (finalizedAgentToolCall, error) {
	dispatcher := newAgentToolUpdateDispatcher(ctx, emit, prepared.toolCall)
	result, err := prepared.tool.executeValidated(ctx, prepared.toolCall.ID, prepared.args, dispatcher.admit)
	updateErr := dispatcher.closeAndWait()
	if cause := context.Cause(ctx); cause != nil {
		return finalizedAgentToolCall{}, cause
	}
	if updateErr != nil {
		return finalizedAgentToolCall{}, updateErr
	}
	if err != nil {
		return finalizedAgentToolCall{toolCall: prepared.toolCall, result: errorAgentToolResult(err.Error()), isError: true}, nil
	}
	return finalizedAgentToolCall{toolCall: prepared.toolCall, result: result}, nil
}

func finalizeSingleAgentToolCall(ctx context.Context, agentContext AgentContext, assistantMessage ai.AssistantMessage, config AgentLoopConfig, prepared preparedAgentToolCall, executed finalizedAgentToolCall) (finalizedAgentToolCall, error) {
	if cause := context.Cause(ctx); cause != nil {
		return finalizedAgentToolCall{}, cause
	}
	if config.AfterToolCall == nil {
		return executed, nil
	}
	callbackContext, err := cloneAgentContext(agentContext)
	if err != nil {
		return *immediateAgentToolError(prepared.toolCall, err.Error()), nil
	}
	override, err := config.AfterToolCall(ctx, AfterToolCallContext{
		AssistantMessage: ai.CloneAssistantMessage(assistantMessage), ToolCall: cloneAgentToolCall(prepared.toolCall), Args: cloneAgentJSONValue(prepared.args.value),
		Result: cloneErasedAgentToolResult(executed.result), IsError: executed.isError, Context: callbackContext,
	})
	if cause := context.Cause(ctx); cause != nil {
		return finalizedAgentToolCall{}, cause
	}
	if err != nil {
		return *immediateAgentToolError(prepared.toolCall, err.Error()), nil
	}
	if override == nil {
		return executed, nil
	}
	if override.Content.IsSet() {
		value, _ := override.Content.Value()
		executed.result.Content = cloneToolResultContent(value)
	}
	if override.Details.IsSet() {
		value, _ := override.Details.Value()
		executed.result.Details = cloneAgentJSONValue(value)
	}
	if override.IsError.IsSet() {
		value, _ := override.IsError.Value()
		executed.isError = value
	}
	if override.Usage.IsSet() {
		executed.result.Usage = override.Usage
	}
	if override.Terminate.IsSet() {
		executed.result.Terminate = override.Terminate
	}
	return executed, nil
}

func findAgentTool(tools []ErasedAgentTool, name string) (ErasedAgentTool, bool) {
	for _, tool := range tools {
		if tool.Name == name {
			return tool, true
		}
	}
	return ErasedAgentTool{}, false
}

func immediateAgentToolError(toolCall AgentToolCall, message string) *finalizedAgentToolCall {
	return &finalizedAgentToolCall{toolCall: toolCall, result: errorAgentToolResult(message), isError: true}
}

func errorAgentToolResult(message string) ErasedAgentToolResult {
	return ErasedAgentToolResult{
		Content: []ai.ToolResultContent{ai.TextContent{Type: ai.ContentTypeText, Text: message}},
		Details: map[string]any{},
	}
}

func createAgentToolResultMessage(finalized finalizedAgentToolCall) ai.ToolResultMessage {
	content := cloneToolResultContent(finalized.result.Content)
	if content == nil {
		content = []ai.ToolResultContent{}
	}
	addedToolNames := ai.Absent[[]string]()
	if names, ok := finalized.result.AddedToolNames.Value(); ok && len(names) != 0 {
		addedToolNames = ai.Some(append([]string(nil), names...))
	}
	details := ai.Some(cloneAgentJSONValue(finalized.result.Details))
	if finalized.result.Details == nil {
		details = ai.Null[ai.JSONValue]()
	}
	return ai.ToolResultMessage{
		Role: ai.MessageRoleToolResult, ToolCallID: finalized.toolCall.ID, ToolName: finalized.toolCall.Name,
		Content: content, Details: details, Usage: finalized.result.Usage,
		AddedToolNames: addedToolNames, IsError: finalized.isError, Timestamp: time.Now().UnixMilli(),
	}
}

func cloneToolResultMessages(messages []ai.ToolResultMessage) []ai.ToolResultMessage {
	if messages == nil {
		return nil
	}
	cloned := make([]ai.ToolResultMessage, len(messages))
	for i := range messages {
		cloned[i] = cloneToolResultMessage(messages[i])
	}
	return cloned
}

func cloneErasedAgentToolResult(result ErasedAgentToolResult) ErasedAgentToolResult {
	result.Content = cloneToolResultContent(result.Content)
	result.Details = cloneAgentJSONValue(result.Details)
	result.AddedToolNames = cloneOptionalStrings(result.AddedToolNames)
	return result
}

func cloneToolResultContent(content []ai.ToolResultContent) []ai.ToolResultContent {
	if content == nil {
		return nil
	}
	cloned := make([]ai.ToolResultContent, len(content))
	for i, item := range content {
		switch item := item.(type) {
		case ai.TextContent:
			cloned[i] = item
		case *ai.TextContent:
			if item != nil {
				copy := *item
				cloned[i] = &copy
			}
		case ai.ImageContent:
			cloned[i] = item
		case *ai.ImageContent:
			if item != nil {
				copy := *item
				cloned[i] = &copy
			}
		default:
			cloned[i] = item
		}
	}
	return cloned
}

func cloneAgentJSONValue(value ai.JSONValue) ai.JSONValue {
	switch value := value.(type) {
	case map[string]any:
		cloned := make(map[string]any, len(value))
		for key, item := range value {
			cloned[key] = cloneAgentJSONValue(item)
		}
		return cloned
	case []any:
		cloned := make([]any, len(value))
		for i, item := range value {
			cloned[i] = cloneAgentJSONValue(item)
		}
		return cloned
	default:
		return value
	}
}

func cloneAgentToolCall(toolCall AgentToolCall) AgentToolCall {
	if toolCall.Arguments != nil {
		toolCall.Arguments = cloneAgentJSONValue(toolCall.Arguments).(map[string]any)
	}
	return toolCall
}

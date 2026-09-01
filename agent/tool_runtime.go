package agent

import (
	"context"
	"errors"
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

type indexedFinalizedAgentToolCall struct {
	index     int
	finalized finalizedAgentToolCall
	err       error
}

type preparedAgentToolBatchEntry struct {
	toolCall  AgentToolCall
	prepared  preparedAgentToolCall
	immediate *finalizedAgentToolCall
}

const agentToolCancellationMessage = "Operation aborted"

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

func finishToolTurn(ctx context.Context, emit AgentEventSink, agentContext AgentContext, newMessages []AgentMessage, config AgentLoopConfig, message ai.AssistantMessage, toolCalls []AgentToolCall) (agentTurnCompletion, error) {
	emit = tolerateAgentToolBatchCancellation(ctx, emit, len(toolCalls))
	var (
		finalized []finalizedAgentToolCall
		err       error
	)
	if message.StopReason == ai.StopReasonLength {
		finalized, err = failTruncatedAgentToolCalls(ctx, emit, toolCalls)
	} else {
		finalized, err = executeAgentToolBatch(ctx, emit, agentContext, message, config, toolCalls)
	}
	if err != nil {
		return agentTurnCompletion{}, err
	}

	toolResults := make([]ai.ToolResultMessage, 0, len(finalized))
	for _, call := range finalized {
		toolResult := createAgentToolResultMessage(call)
		if err := emitMessageLifecycle(ctx, emit, toolResult); err != nil {
			return agentTurnCompletion{}, err
		}
		agentContext.Messages = append(agentContext.Messages, cloneToolResultMessage(toolResult))
		newMessages = append(newMessages, cloneToolResultMessage(toolResult))
		toolResults = append(toolResults, toolResult)
	}
	if err := emitAgentEvent(ctx, emit, TurnEndEvent{Type: AgentEventTypeTurnEnd, Message: message, ToolResults: toolResults}); err != nil {
		return agentTurnCompletion{}, err
	}
	completed := agentTurnCompletion{agentContext: agentContext, newMessages: newMessages}
	if cause := context.Cause(ctx); cause != nil {
		if err := emitAgentEvent(ctx, emit, AgentEndEvent{Type: AgentEventTypeAgentEnd, Messages: newMessages}); err != nil {
			return agentTurnCompletion{}, err
		}
		return completed, cause
	}
	shouldStop := false
	if config.ShouldStopAfterTurn != nil {
		callbackContext, err := cloneAgentContext(agentContext)
		if err != nil {
			return agentTurnCompletion{}, err
		}
		shouldStop, err = config.ShouldStopAfterTurn(ctx, ShouldStopAfterTurnContext{
			Message: ai.CloneAssistantMessage(message), ToolResults: cloneToolResultMessages(toolResults), Context: callbackContext, NewMessages: cloneAgentMessages(newMessages),
		})
		if cause := context.Cause(ctx); cause != nil {
			return agentTurnCompletion{}, cause
		}
		if err != nil {
			return agentTurnCompletion{}, err
		}
	}
	if !shouldTerminateAgentToolBatch(finalized) && !shouldStop {
		completed.continueRun = true
		return completed, nil
	}
	if err := emitAgentEvent(ctx, emit, AgentEndEvent{Type: AgentEventTypeAgentEnd, Messages: newMessages}); err != nil {
		return agentTurnCompletion{}, err
	}
	return completed, nil
}

func failTruncatedAgentToolCalls(ctx context.Context, emit AgentEventSink, toolCalls []AgentToolCall) ([]finalizedAgentToolCall, error) {
	finalized := make([]finalizedAgentToolCall, 0, len(toolCalls))
	for _, toolCall := range toolCalls {
		if err := emitAgentToolStart(ctx, emit, toolCall); err != nil {
			return nil, err
		}
		call := finalizedAgentToolCall{
			toolCall: toolCall,
			result:   errorAgentToolResult(fmt.Sprintf("Tool call %q was not executed: the response hit the output token limit, so its arguments may be truncated. Re-issue the tool call with complete arguments.", toolCall.Name)),
			isError:  true,
		}
		if err := emitAgentToolEnd(ctx, emit, call); err != nil {
			return nil, err
		}
		finalized = append(finalized, call)
	}
	return finalized, nil
}

func executeAgentToolBatch(ctx context.Context, emit AgentEventSink, agentContext AgentContext, message ai.AssistantMessage, config AgentLoopConfig, toolCalls []AgentToolCall) ([]finalizedAgentToolCall, error) {
	if agentToolBatchIsSequential(agentContext.Tools, toolCalls, config.ToolExecution) {
		return executeAgentToolCallsSequential(ctx, emit, agentContext, message, config, toolCalls)
	}
	return executeAgentToolCallsParallel(ctx, emit, agentContext, message, config, toolCalls)
}

func executeAgentToolCallsSequential(ctx context.Context, emit AgentEventSink, agentContext AgentContext, message ai.AssistantMessage, config AgentLoopConfig, toolCalls []AgentToolCall) ([]finalizedAgentToolCall, error) {
	entries, err := prepareAgentToolBatch(ctx, emit, agentContext, message, config, toolCalls)
	if err != nil {
		return nil, err
	}
	finalized := make([]finalizedAgentToolCall, 0, len(toolCalls))
	for index, entry := range entries {
		var call finalizedAgentToolCall
		if entry.immediate != nil {
			call = *entry.immediate
		} else if canceled, ok := canceledAgentToolBatchCall(ctx, entry.toolCall, len(entries)); ok {
			call = canceled
		} else {
			executed, settled, err := executeSingleAgentToolCall(ctx, emit, entry.prepared)
			if err != nil {
				call, err = resolveCanceledAgentToolBatchCall(ctx, entry.toolCall, len(entries), executed, settled, err)
			}
			if err != nil {
				return nil, err
			}
			if !settled {
				call = executed
			} else {
				call, err = finalizeSingleAgentToolCall(ctx, agentContext, message, config, entry.prepared, executed)
				call, err = resolveCanceledAgentToolBatchCall(ctx, entry.toolCall, len(entries), call, true, err)
			}
			if err != nil {
				return nil, err
			}
		}
		if err := emitAgentToolEnd(ctx, emit, call); err != nil {
			return nil, err
		}
		finalized = append(finalized, call)
		if _, canceled := canceledAgentToolBatchCall(ctx, entry.toolCall, len(entries)); canceled {
			for _, remaining := range entries[index+1:] {
				aborted := canceledAgentToolCall(remaining.toolCall)
				if err := emitAgentToolEnd(ctx, emit, aborted); err != nil {
					return nil, err
				}
				finalized = append(finalized, aborted)
			}
			break
		}
	}
	return finalized, nil
}

func executeAgentToolCallsParallel(ctx context.Context, emit AgentEventSink, agentContext AgentContext, message ai.AssistantMessage, config AgentLoopConfig, toolCalls []AgentToolCall) ([]finalizedAgentToolCall, error) {
	finalized := make([]finalizedAgentToolCall, len(toolCalls))
	entries, err := prepareAgentToolBatch(ctx, emit, agentContext, message, config, toolCalls)
	if err != nil {
		return nil, err
	}
	for i, entry := range entries {
		if entry.immediate != nil {
			finalized[i] = *entry.immediate
			if err := emitAgentToolEnd(ctx, emit, *entry.immediate); err != nil {
				return nil, err
			}
		}
	}

	var emitMu sync.Mutex
	orderedEmit := func(ctx context.Context, event AgentEvent) error {
		emitMu.Lock()
		defer emitMu.Unlock()
		return emitAgentEvent(ctx, emit, event)
	}
	results := make(chan indexedFinalizedAgentToolCall, len(toolCalls))
	running := 0
	for i, entry := range entries {
		if entry.immediate != nil {
			continue
		}
		running++
		go func(index int, call preparedAgentToolCall) {
			var (
				executed finalizedAgentToolCall
				settled  bool
				err      error
			)
			if canceled, ok := canceledAgentToolBatchCall(ctx, call.toolCall, len(entries)); ok {
				executed = canceled
			} else {
				executed, settled, err = executeSingleAgentToolCall(ctx, orderedEmit, call)
				if err != nil {
					executed, err = resolveCanceledAgentToolBatchCall(ctx, call.toolCall, len(entries), executed, settled, err)
				}
			}
			if err == nil && settled {
				executed, err = finalizeSingleAgentToolCall(ctx, agentContext, message, config, call, executed)
				executed, err = resolveCanceledAgentToolBatchCall(ctx, call.toolCall, len(entries), executed, true, err)
			}
			if err == nil {
				err = emitAgentToolEnd(ctx, orderedEmit, executed)
			}
			results <- indexedFinalizedAgentToolCall{index: index, finalized: executed, err: err}
		}(i, entry.prepared)
	}
	var errorsBySource = make([]error, len(toolCalls))
	for range running {
		result := <-results
		finalized[result.index] = result.finalized
		errorsBySource[result.index] = result.err
	}
	for _, err := range errorsBySource {
		if err != nil {
			return nil, err
		}
	}
	return finalized, nil
}

func prepareAgentToolBatch(ctx context.Context, emit AgentEventSink, agentContext AgentContext, message ai.AssistantMessage, config AgentLoopConfig, toolCalls []AgentToolCall) ([]preparedAgentToolBatchEntry, error) {
	entries := make([]preparedAgentToolBatchEntry, len(toolCalls))
	for i, toolCall := range toolCalls {
		if err := emitAgentToolStart(ctx, emit, toolCall); err != nil {
			return nil, err
		}
		call, immediate, err := prepareSingleAgentToolCall(ctx, agentContext, message, config, toolCall)
		if err != nil {
			if _, canceled := canceledAgentToolBatchCall(ctx, toolCall, len(toolCalls)); canceled {
				for j := 0; j < i; j++ {
					if entries[j].immediate == nil {
						entries[j].immediate = pointerToFinalizedAgentToolCall(canceledAgentToolCall(entries[j].toolCall))
					}
				}
				entries[i] = preparedAgentToolBatchEntry{toolCall: toolCall, immediate: pointerToFinalizedAgentToolCall(canceledAgentToolCall(toolCall))}
				for j := i + 1; j < len(toolCalls); j++ {
					if err := emitAgentToolStart(ctx, emit, toolCalls[j]); err != nil {
						return nil, err
					}
					entries[j] = preparedAgentToolBatchEntry{toolCall: toolCalls[j], immediate: pointerToFinalizedAgentToolCall(canceledAgentToolCall(toolCalls[j]))}
				}
				return entries, nil
			}
			return nil, err
		}
		entries[i] = preparedAgentToolBatchEntry{toolCall: toolCall, prepared: call, immediate: immediate}
	}
	return entries, nil
}

func canceledAgentToolCall(toolCall AgentToolCall) finalizedAgentToolCall {
	return finalizedAgentToolCall{toolCall: toolCall, result: errorAgentToolResult(agentToolCancellationMessage), isError: true}
}

func canceledAgentToolBatchCall(ctx context.Context, toolCall AgentToolCall, batchSize int) (finalizedAgentToolCall, bool) {
	if batchSize <= 1 || context.Cause(ctx) == nil {
		return finalizedAgentToolCall{}, false
	}
	return canceledAgentToolCall(toolCall), true
}

func resolveCanceledAgentToolBatchCall(ctx context.Context, toolCall AgentToolCall, batchSize int, result finalizedAgentToolCall, settled bool, err error) (finalizedAgentToolCall, error) {
	if err == nil {
		return result, nil
	}
	cause := context.Cause(ctx)
	if batchSize <= 1 || cause == nil || !errors.Is(err, cause) {
		return finalizedAgentToolCall{}, err
	}
	if settled {
		return result, nil
	}
	return canceledAgentToolCall(toolCall), nil
}

func tolerateAgentToolBatchCancellation(ctx context.Context, emit AgentEventSink, batchSize int) AgentEventSink {
	if batchSize <= 1 {
		return emit
	}
	return func(eventContext context.Context, event AgentEvent) error {
		err := emit(eventContext, event)
		cause := context.Cause(ctx)
		if cause != nil && errors.Is(err, cause) {
			return nil
		}
		return err
	}
}

func pointerToFinalizedAgentToolCall(call finalizedAgentToolCall) *finalizedAgentToolCall {
	return &call
}

func agentToolBatchIsSequential(tools []ErasedAgentTool, toolCalls []AgentToolCall, mode ToolExecutionMode) bool {
	if mode == ToolExecutionSequential {
		return true
	}
	for _, toolCall := range toolCalls {
		if tool, ok := findAgentTool(tools, toolCall.Name); ok && tool.ExecutionMode == ToolExecutionSequential {
			return true
		}
	}
	return false
}

func shouldTerminateAgentToolBatch(finalized []finalizedAgentToolCall) bool {
	if len(finalized) == 0 {
		return false
	}
	for _, call := range finalized {
		terminate, ok := call.result.Terminate.Value()
		if !ok || !terminate {
			return false
		}
	}
	return true
}

func emitAgentToolStart(ctx context.Context, emit AgentEventSink, toolCall AgentToolCall) error {
	return emitAgentEvent(ctx, emit, ToolExecutionStartEvent{
		Type: AgentEventTypeToolExecutionStart, ToolCallID: toolCall.ID, ToolName: toolCall.Name, Arguments: toolCall.Arguments,
	})
}

func emitAgentToolEnd(ctx context.Context, emit AgentEventSink, finalized finalizedAgentToolCall) error {
	return emitAgentEvent(ctx, emit, ToolExecutionEndEvent{
		Type: AgentEventTypeToolExecutionEnd, ToolCallID: finalized.toolCall.ID, ToolName: finalized.toolCall.Name,
		Result: finalized.result, IsError: finalized.isError,
	})
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

func executeSingleAgentToolCall(ctx context.Context, emit AgentEventSink, prepared preparedAgentToolCall) (finalizedAgentToolCall, bool, error) {
	dispatcher := newAgentToolUpdateDispatcher(ctx, emit, prepared.toolCall)
	result, err := prepared.tool.executeValidated(ctx, prepared.toolCall.ID, prepared.args, dispatcher.admit)
	updateErr := dispatcher.closeAndWait()
	if updateErr != nil {
		return finalizedAgentToolCall{}, false, updateErr
	}
	if err != nil {
		if cause := context.Cause(ctx); cause != nil && (errors.Is(err, cause) || errors.Is(err, ctx.Err())) {
			return finalizedAgentToolCall{}, false, cause
		}
		return finalizedAgentToolCall{toolCall: prepared.toolCall, result: errorAgentToolResult(err.Error()), isError: true}, true, nil
	}
	executed := finalizedAgentToolCall{toolCall: prepared.toolCall, result: result}
	if cause := context.Cause(ctx); cause != nil {
		return executed, true, cause
	}
	return executed, true, nil
}

func finalizeSingleAgentToolCall(ctx context.Context, agentContext AgentContext, assistantMessage ai.AssistantMessage, config AgentLoopConfig, prepared preparedAgentToolCall, executed finalizedAgentToolCall) (finalizedAgentToolCall, error) {
	if cause := context.Cause(ctx); cause != nil {
		return executed, cause
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
	if err != nil {
		executed = *immediateAgentToolError(prepared.toolCall, err.Error())
	} else if override != nil {
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
	}
	if cause := context.Cause(ctx); cause != nil {
		return executed, cause
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

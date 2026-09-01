package agent

import (
	"context"
	"fmt"
	"sync"

	"github.com/nankedr/pig/ai"
)

// ThinkingLevel is the Agent-facing alias of the model reasoning level.
type ThinkingLevel = ai.ModelThinkingLevel

// StreamFunction is the model-stream boundary used by the legacy Agent loop.
type StreamFunction func(context.Context, ai.Model, ai.Context, ai.SimpleStreamOptions) *ai.AssistantMessageEventStream

// AgentEventSink consumes loop events in order. Listener is the same callback
// shape used by the stateful Agent subscription surface.
type AgentEventSink func(context.Context, AgentEvent) error
type AgentEventListener = AgentEventSink

// AgentContext is the open Agent transcript and the tools visible for a run.
type AgentContext struct {
	SystemPrompt string
	Messages     []AgentMessage
	Tools        []ErasedAgentTool
}

type BeforeToolCallContext struct {
	AssistantMessage ai.AssistantMessage
	ToolCall         AgentToolCall
	Args             ai.JSONValue
	Context          AgentContext
}

type BeforeToolCallResult struct {
	Block     bool
	Reason    string
	Terminate bool
}

type AfterToolCallContext struct {
	AssistantMessage ai.AssistantMessage
	ToolCall         AgentToolCall
	Args             ai.JSONValue
	Result           ErasedAgentToolResult
	IsError          bool
	Context          AgentContext
}

// AfterToolCallResult uses Optional fields because omission retains the
// executed value while explicit zero or empty values replace it.
type AfterToolCallResult struct {
	Content   ai.Optional[[]ai.ToolResultContent]
	Details   ai.Optional[ai.JSONValue]
	IsError   ai.Optional[bool]
	Usage     ai.Optional[ai.Usage]
	Terminate ai.Optional[bool]
}

type ShouldStopAfterTurnContext struct {
	Message     ai.AssistantMessage
	ToolResults []ai.ToolResultMessage
	Context     AgentContext
	NewMessages []AgentMessage
}

type PrepareNextTurnContext = ShouldStopAfterTurnContext

type AgentLoopTurnUpdate struct {
	Context       *AgentContext
	Model         *ai.Model
	ThinkingLevel *ThinkingLevel
}

// AgentLoopConfig describes callbacks and options used by the low-level loop.
type AgentLoopConfig struct {
	ai.SimpleStreamOptions
	Model               ai.Model
	ConvertToLLM        func(context.Context, []AgentMessage) ([]ai.Message, error)
	TransformContext    func(context.Context, []AgentMessage) ([]AgentMessage, error)
	GetAPIKey           func(context.Context, ai.ProviderID) (string, bool, error)
	ShouldStopAfterTurn func(context.Context, ShouldStopAfterTurnContext) (bool, error)
	PrepareNextTurn     func(context.Context, PrepareNextTurnContext) (*AgentLoopTurnUpdate, error)
	GetSteeringMessages func(context.Context) ([]AgentMessage, error)
	GetFollowUpMessages func(context.Context) ([]AgentMessage, error)
	ToolExecution       ToolExecutionMode
	BeforeToolCall      func(context.Context, BeforeToolCallContext) (*BeforeToolCallResult, error)
	AfterToolCall       func(context.Context, AfterToolCallContext) (*AfterToolCallResult, error)
}

// AgentEventStream is a consumer-only event stream. Producers remain private
// to the Agent loop runtime.
type AgentEventStream struct {
	mu        sync.Mutex
	queue     []AgentEvent
	done      bool
	result    []AgentMessage
	resultErr error
	changed   chan struct{}
}

func newAgentEventStream() *AgentEventStream {
	return &AgentEventStream{changed: make(chan struct{})}
}

func (s *AgentEventStream) push(event AgentEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.done {
		return
	}
	s.queue = append(s.queue, event)
	s.signalLocked()
}

func (s *AgentEventStream) end(result []AgentMessage, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.done {
		return
	}
	s.done = true
	s.result = cloneAgentMessages(result)
	s.resultErr = err
	s.signalLocked()
}

func (s *AgentEventStream) signalLocked() {
	close(s.changed)
	s.changed = make(chan struct{})
}

func (s *AgentEventStream) Next(ctx context.Context) (AgentEvent, bool, error) {
	for {
		s.mu.Lock()
		if len(s.queue) != 0 {
			event := s.queue[0]
			s.queue = s.queue[1:]
			s.mu.Unlock()
			return event, true, nil
		}
		if s.done {
			s.mu.Unlock()
			return nil, false, nil
		}
		changed := s.changed
		s.mu.Unlock()
		select {
		case <-changed:
		case <-ctx.Done():
			return nil, false, context.Cause(ctx)
		}
	}
}

func (s *AgentEventStream) Result(ctx context.Context) ([]AgentMessage, error) {
	for {
		s.mu.Lock()
		if s.done {
			result, err := cloneAgentMessages(s.result), s.resultErr
			s.mu.Unlock()
			return result, err
		}
		changed := s.changed
		s.mu.Unlock()
		select {
		case <-changed:
		case <-ctx.Done():
			return nil, context.Cause(ctx)
		}
	}
}

func AgentLoop(ctx context.Context, prompts []AgentMessage, agentContext AgentContext, config AgentLoopConfig, streamFunction StreamFunction) *AgentEventStream {
	stream := newAgentEventStream()
	go func() {
		result, err := RunAgentLoop(ctx, prompts, agentContext, config, func(_ context.Context, event AgentEvent) error {
			stream.push(event)
			return nil
		}, streamFunction)
		stream.end(result, err)
	}()
	return stream
}

func AgentLoopContinue(ctx context.Context, agentContext AgentContext, config AgentLoopConfig, streamFunction StreamFunction) *AgentEventStream {
	stream := newAgentEventStream()
	go func() {
		result, err := RunAgentLoopContinue(ctx, agentContext, config, func(_ context.Context, event AgentEvent) error {
			stream.push(event)
			return nil
		}, streamFunction)
		stream.end(result, err)
	}()
	return stream
}

func RunAgentLoop(ctx context.Context, prompts []AgentMessage, agentContext AgentContext, config AgentLoopConfig, emit AgentEventSink, streamFunction StreamFunction) ([]AgentMessage, error) {
	ownedPrompts, err := cloneAgentMessagesForOwnership(prompts)
	if err != nil {
		return nil, err
	}
	currentContext, err := cloneAgentContext(agentContext)
	if err != nil {
		return nil, err
	}
	if err := validateAgentLoop(ctx, config, emit, streamFunction); err != nil {
		return nil, err
	}
	currentContext.Messages = append(currentContext.Messages, cloneAgentMessages(ownedPrompts)...)
	newMessages := cloneAgentMessages(ownedPrompts)
	if err := emitAgentEvent(ctx, emit, AgentStartEvent{Type: AgentEventTypeAgentStart}); err != nil {
		return nil, err
	}
	if err := emitAgentEvent(ctx, emit, TurnStartEvent{Type: AgentEventTypeTurnStart}); err != nil {
		return nil, err
	}
	for _, prompt := range ownedPrompts {
		if err := emitMessageLifecycle(ctx, emit, prompt); err != nil {
			return nil, err
		}
	}
	return runAgentTurn(ctx, currentContext, newMessages, config, emit, streamFunction)
}

func RunAgentLoopContinue(ctx context.Context, agentContext AgentContext, config AgentLoopConfig, emit AgentEventSink, streamFunction StreamFunction) ([]AgentMessage, error) {
	currentContext, err := cloneAgentContext(agentContext)
	if err != nil {
		return nil, err
	}
	if len(currentContext.Messages) == 0 {
		return nil, fmt.Errorf("cannot continue: no messages in context")
	}
	if currentContext.Messages[len(currentContext.Messages)-1].MessageRole() == ai.MessageRoleAssistant {
		return nil, fmt.Errorf("cannot continue from message role: assistant")
	}
	if err := validateAgentLoop(ctx, config, emit, streamFunction); err != nil {
		return nil, err
	}
	if err := emitAgentEvent(ctx, emit, AgentStartEvent{Type: AgentEventTypeAgentStart}); err != nil {
		return nil, err
	}
	if err := emitAgentEvent(ctx, emit, TurnStartEvent{Type: AgentEventTypeTurnStart}); err != nil {
		return nil, err
	}
	return runAgentTurn(ctx, currentContext, []AgentMessage{}, config, emit, streamFunction)
}

func runAgentTurn(ctx context.Context, agentContext AgentContext, newMessages []AgentMessage, config AgentLoopConfig, emit AgentEventSink, streamFunction StreamFunction) ([]AgentMessage, error) {
	messages := cloneAgentMessages(agentContext.Messages)
	var err error
	if config.TransformContext != nil {
		messages, err = config.TransformContext(ctx, messages)
		if err != nil {
			return nil, err
		}
	}
	convertToLLM := config.ConvertToLLM
	if convertToLLM == nil {
		convertToLLM = DefaultConvertToLLM
	}
	llmMessages, err := convertToLLM(ctx, messages)
	if err != nil {
		return nil, err
	}
	options := config.SimpleStreamOptions
	if config.GetAPIKey != nil {
		if key, ok, err := config.GetAPIKey(ctx, config.Model.Provider); err != nil {
			return nil, err
		} else if ok {
			options.APIKey = &key
		}
	}
	modelContext := ai.Context{SystemPrompt: ai.Some(agentContext.SystemPrompt), Messages: llmMessages}
	if len(agentContext.Tools) != 0 {
		modelContext.Tools = make([]ai.Tool, len(agentContext.Tools))
		for i := range agentContext.Tools {
			modelContext.Tools[i] = cloneAgentToolMetadata(agentContext.Tools[i].Tool)
		}
	}
	response := streamFunction(ctx, config.Model, modelContext, options)
	if response == nil {
		return nil, fmt.Errorf("Agent stream function returned nil")
	}
	waitContext := context.WithoutCancel(ctx)
	started := false
	for {
		event, ok, err := response.Next(waitContext)
		if err != nil {
			return nil, err
		}
		if !ok {
			break
		}
		partial, hasPartial := assistantEventPartial(event)
		switch event.AssistantMessageEventType() {
		case ai.AssistantMessageEventTypeStart:
			started = true
			if err := emitAgentEvent(ctx, emit, MessageStartEvent{Type: AgentEventTypeMessageStart, Message: partial}); err != nil {
				return nil, err
			}
		case ai.AssistantMessageEventTypeDone, ai.AssistantMessageEventTypeError:
			message, err := response.Result(waitContext)
			if err != nil {
				return nil, err
			}
			return finishAgentTurn(ctx, emit, agentContext, newMessages, config, message, started)
		default:
			if hasPartial {
				if err := emitAgentEvent(ctx, emit, MessageUpdateEvent{
					Type: AgentEventTypeMessageUpdate, Message: partial, AssistantMessageEvent: event,
				}); err != nil {
					return nil, err
				}
			}
		}
	}
	message, err := response.Result(waitContext)
	if err != nil {
		return nil, err
	}
	return finishAgentTurn(ctx, emit, agentContext, newMessages, config, message, started)
}

func validateAgentLoop(ctx context.Context, config AgentLoopConfig, emit AgentEventSink, streamFunction StreamFunction) error {
	if ctx == nil {
		return fmt.Errorf("Agent loop context must not be nil")
	}
	if emit == nil {
		return fmt.Errorf("Agent event sink must not be nil")
	}
	if streamFunction == nil {
		return newNotImplemented("Agent.DefaultStreamFunction")
	}
	if config.PrepareNextTurn != nil {
		return newNotImplemented("Agent.PrepareNextTurn")
	}
	return nil
}

func finishAgentTurn(ctx context.Context, emit AgentEventSink, agentContext AgentContext, newMessages []AgentMessage, config AgentLoopConfig, message ai.AssistantMessage, started bool) ([]AgentMessage, error) {
	message = ai.CloneAssistantMessage(message)
	if !started {
		if err := emitAgentEvent(ctx, emit, MessageStartEvent{Type: AgentEventTypeMessageStart, Message: message}); err != nil {
			return nil, err
		}
	}
	if err := emitAgentEvent(ctx, emit, MessageEndEvent{Type: AgentEventTypeMessageEnd, Message: message}); err != nil {
		return nil, err
	}
	newMessages = append(newMessages, message)
	agentContext.Messages = append(agentContext.Messages, ai.CloneAssistantMessage(message))
	if message.StopReason == ai.StopReasonError || message.StopReason == ai.StopReasonAborted {
		if err := emitAgentEvent(ctx, emit, TurnEndEvent{Type: AgentEventTypeTurnEnd, Message: message, ToolResults: []ai.ToolResultMessage{}}); err != nil {
			return nil, err
		}
		if err := emitAgentEvent(ctx, emit, AgentEndEvent{Type: AgentEventTypeAgentEnd, Messages: newMessages}); err != nil {
			return nil, err
		}
		return cloneAgentMessages(newMessages), nil
	}
	toolCalls := assistantToolCalls(message)
	if len(toolCalls) > 1 {
		return cloneAgentMessages(newMessages), newNotImplemented("Agent.MultiToolExecution")
	}
	if len(toolCalls) == 1 {
		return finishSingleToolTurn(ctx, emit, agentContext, newMessages, config, message, toolCalls[0])
	}
	if err := emitAgentEvent(ctx, emit, TurnEndEvent{Type: AgentEventTypeTurnEnd, Message: message, ToolResults: []ai.ToolResultMessage{}}); err != nil {
		return nil, err
	}
	if message.StopReason != ai.StopReasonError && message.StopReason != ai.StopReasonAborted && config.ShouldStopAfterTurn != nil {
		callbackContext, err := cloneAgentContext(agentContext)
		if err != nil {
			return nil, err
		}
		if _, err := config.ShouldStopAfterTurn(ctx, ShouldStopAfterTurnContext{
			Message:     ai.CloneAssistantMessage(message),
			ToolResults: []ai.ToolResultMessage{},
			Context:     callbackContext,
			NewMessages: cloneAgentMessages(newMessages),
		}); err != nil {
			return nil, err
		}
	}
	if err := emitAgentEvent(ctx, emit, AgentEndEvent{Type: AgentEventTypeAgentEnd, Messages: newMessages}); err != nil {
		return nil, err
	}
	return cloneAgentMessages(newMessages), nil
}

func emitMessageLifecycle(ctx context.Context, emit AgentEventSink, message AgentMessage) error {
	if err := emitAgentEvent(ctx, emit, MessageStartEvent{Type: AgentEventTypeMessageStart, Message: message}); err != nil {
		return err
	}
	return emitAgentEvent(ctx, emit, MessageEndEvent{Type: AgentEventTypeMessageEnd, Message: message})
}

func emitAgentEvent(ctx context.Context, emit AgentEventSink, event AgentEvent) error {
	if emit == nil {
		return fmt.Errorf("Agent event sink must not be nil")
	}
	return emit(ctx, cloneAgentEvent(event))
}

func cloneAgentContext(agentContext AgentContext) (AgentContext, error) {
	messages, err := cloneAgentMessagesForOwnership(agentContext.Messages)
	if err != nil {
		return AgentContext{}, err
	}
	return AgentContext{
		SystemPrompt: agentContext.SystemPrompt,
		Messages:     messages,
		Tools:        cloneAgentTools(agentContext.Tools),
	}, nil
}

func cloneAgentEvent(event AgentEvent) AgentEvent {
	switch event := event.(type) {
	case AgentStartEvent, TurnStartEvent:
		return event
	case AgentEndEvent:
		event.Messages = cloneAgentMessages(event.Messages)
		return event
	case MessageStartEvent:
		event.Message = cloneAgentMessage(event.Message)
		return event
	case MessageUpdateEvent:
		event.Message = cloneAgentMessage(event.Message)
		event.AssistantMessageEvent = cloneAssistantMessageEvent(event.AssistantMessageEvent)
		return event
	case MessageEndEvent:
		event.Message = cloneAgentMessage(event.Message)
		return event
	case TurnEndEvent:
		event.Message = cloneAgentMessage(event.Message)
		event.ToolResults = cloneToolResultMessages(event.ToolResults)
		return event
	case ToolExecutionStartEvent:
		event.Arguments = cloneAgentJSONValue(event.Arguments)
		return event
	case ToolExecutionUpdateEvent:
		event.Arguments = cloneAgentJSONValue(event.Arguments)
		event.PartialResult = cloneErasedAgentToolResult(event.PartialResult)
		return event
	case ToolExecutionEndEvent:
		event.Result = cloneErasedAgentToolResult(event.Result)
		return event
	default:
		return event
	}
}

func cloneAssistantMessageEvent(event ai.AssistantMessageEvent) ai.AssistantMessageEvent {
	encoded, err := ai.MarshalAssistantMessageEvent(event)
	if err != nil {
		return event
	}
	cloned, err := ai.UnmarshalAssistantMessageEvent(encoded)
	if err != nil {
		return event
	}
	return cloned
}

func assistantEventPartial(event ai.AssistantMessageEvent) (ai.AssistantMessage, bool) {
	switch event := event.(type) {
	case ai.AssistantMessageStartEvent:
		return event.Partial, true
	case ai.AssistantMessageTextStartEvent:
		return event.Partial, true
	case ai.AssistantMessageTextDeltaEvent:
		return event.Partial, true
	case ai.AssistantMessageTextEndEvent:
		return event.Partial, true
	case ai.AssistantMessageThinkingStartEvent:
		return event.Partial, true
	case ai.AssistantMessageThinkingDeltaEvent:
		return event.Partial, true
	case ai.AssistantMessageThinkingEndEvent:
		return event.Partial, true
	case ai.AssistantMessageToolCallStartEvent:
		return event.Partial, true
	case ai.AssistantMessageToolCallDeltaEvent:
		return event.Partial, true
	case ai.AssistantMessageToolCallEndEvent:
		return event.Partial, true
	case *ai.AssistantMessageStartEvent:
		return event.Partial, true
	case *ai.AssistantMessageTextStartEvent:
		return event.Partial, true
	case *ai.AssistantMessageTextDeltaEvent:
		return event.Partial, true
	case *ai.AssistantMessageTextEndEvent:
		return event.Partial, true
	case *ai.AssistantMessageThinkingStartEvent:
		return event.Partial, true
	case *ai.AssistantMessageThinkingDeltaEvent:
		return event.Partial, true
	case *ai.AssistantMessageThinkingEndEvent:
		return event.Partial, true
	case *ai.AssistantMessageToolCallStartEvent:
		return event.Partial, true
	case *ai.AssistantMessageToolCallDeltaEvent:
		return event.Partial, true
	case *ai.AssistantMessageToolCallEndEvent:
		return event.Partial, true
	default:
		return ai.AssistantMessage{}, false
	}
}

// SetDefaultStreamFunction is an M0 side-effect-free stub: it deliberately
// does not install process-global mutable state.
func SetDefaultStreamFunction(StreamFunction) error {
	return newNotImplemented("SetDefaultStreamFunction")
}

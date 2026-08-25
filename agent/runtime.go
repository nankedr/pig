package agent

import (
	"context"
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

// AgentLoopConfig describes every callback used by the low-level loop. The
// M0 callables below remain explicit capability stubs.
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
// to the future Agent runtime.
type AgentEventStream struct {
	mu        sync.Mutex
	queue     []AgentEvent
	done      bool
	result    []AgentMessage
	resultErr error
	changed   chan struct{}
}

func failedAgentEventStream(operation string) *AgentEventStream {
	return &AgentEventStream{done: true, resultErr: newNotImplemented(operation), changed: make(chan struct{})}
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
			result, err := append([]AgentMessage(nil), s.result...), s.resultErr
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

func AgentLoop(context.Context, []AgentMessage, AgentContext, AgentLoopConfig, StreamFunction) *AgentEventStream {
	return failedAgentEventStream("AgentLoop")
}

func AgentLoopContinue(context.Context, AgentContext, AgentLoopConfig, StreamFunction) *AgentEventStream {
	return failedAgentEventStream("AgentLoopContinue")
}

func RunAgentLoop(context.Context, []AgentMessage, AgentContext, AgentLoopConfig, AgentEventSink, StreamFunction) ([]AgentMessage, error) {
	return nil, newNotImplemented("RunAgentLoop")
}

func RunAgentLoopContinue(context.Context, AgentContext, AgentLoopConfig, AgentEventSink, StreamFunction) ([]AgentMessage, error) {
	return nil, newNotImplemented("RunAgentLoopContinue")
}

// SetDefaultStreamFunction is an M0 side-effect-free stub: it deliberately
// does not install process-global mutable state.
func SetDefaultStreamFunction(StreamFunction) error {
	return newNotImplemented("SetDefaultStreamFunction")
}

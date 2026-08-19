package ai

import (
	"context"
	"errors"
	"sync"
)

// ErrEventStreamInvariant identifies an internal stream protocol violation. It
// is distinct from a Provider failure (a terminal AssistantMessage outcome)
// and from an unimplemented capability.
var ErrEventStreamInvariant = errors.New("AI event stream invariant violated")

var errEventStreamEndedWithoutResult = errors.Join(
	ErrEventStreamInvariant,
	errors.New("event stream ended without a result"),
)

var errEventStreamInvalidCallbacks = errors.Join(
	ErrEventStreamInvariant,
	errors.New("event stream requires non-nil completion callbacks"),
)

// EventStream is a concurrent-safe, unbounded FIFO event stream with an
// independently readable final result.
type EventStream[T, R any] struct {
	mu            sync.Mutex
	queue         []T
	done          bool
	result        R
	resultErr     error
	changed       chan struct{}
	isComplete    func(T) bool
	extractResult func(T) R
}

// NewEventStream constructs an EventStream whose terminal event is recognized
// by isComplete and converted to its final result by extractResult.
func NewEventStream[T, R any](isComplete func(T) bool, extractResult func(T) R) *EventStream[T, R] {
	stream := &EventStream[T, R]{
		changed:       make(chan struct{}),
		isComplete:    isComplete,
		extractResult: extractResult,
	}
	if isComplete == nil || extractResult == nil {
		stream.done = true
		stream.resultErr = errEventStreamInvalidCallbacks
	}
	return stream
}

// Push appends event unless the stream has already ended. A terminal event is
// still appended and remains observable through Next.
func (s *EventStream[T, R]) Push(event T) {
	// Completion callbacks belong to the caller. Run them outside the mutex so
	// a callback can safely end or inspect the same stream without self-deadlock.
	s.mu.Lock()
	if s.done {
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()

	complete := s.isComplete(event)
	var result R
	if complete {
		result = s.extractResult(event)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	// A callback or another producer may have completed the stream while the
	// callbacks ran. In that case this event arrived after the winning terminal
	// transition and is intentionally ignored.
	if s.done {
		return
	}
	s.queue = append(s.queue, event)
	if complete {
		s.done = true
		s.result = result
	}
	s.signalLocked()
}

// End closes the event side of the stream. Supplying a result completes the
// outcome; omitting it records an internal invariant error for Result.
func (s *EventStream[T, R]) End(result ...R) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.done {
		return
	}

	s.done = true
	if len(result) == 1 {
		s.result = result[0]
	} else {
		s.resultErr = errEventStreamEndedWithoutResult
	}
	s.signalLocked()
}

// Next waits for and removes the next event. ok is false after every queued
// event has been consumed and the stream has ended.
func (s *EventStream[T, R]) Next(ctx context.Context) (event T, ok bool, err error) {
	for {
		s.mu.Lock()
		if len(s.queue) != 0 {
			event = s.queue[0]
			var zero T
			s.queue[0] = zero
			s.queue = s.queue[1:]
			s.mu.Unlock()
			return event, true, nil
		}
		if s.done {
			s.mu.Unlock()
			return event, false, nil
		}
		changed := s.changed
		s.mu.Unlock()

		select {
		case <-changed:
		case <-ctx.Done():
			return event, false, context.Cause(ctx)
		}
	}
}

// Result waits for the stream outcome. Once complete, every call returns the
// same value and error.
func (s *EventStream[T, R]) Result(ctx context.Context) (result R, err error) {
	for {
		s.mu.Lock()
		if s.done {
			result, err = s.result, s.resultErr
			s.mu.Unlock()
			return result, err
		}
		changed := s.changed
		s.mu.Unlock()

		select {
		case <-changed:
		case <-ctx.Done():
			return result, context.Cause(ctx)
		}
	}
}

func (s *EventStream[T, R]) signalLocked() {
	close(s.changed)
	s.changed = make(chan struct{})
}

func (s *EventStream[T, R]) fail(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.done {
		return
	}
	s.done = true
	s.resultErr = errors.Join(ErrEventStreamInvariant, err)
	s.signalLocked()
}

// AssistantMessageEventStream is the typed stream of AssistantMessage events
// and their final AssistantMessage outcome.
type AssistantMessageEventStream struct {
	stream *EventStream[AssistantMessageEvent, AssistantMessage]
}

// NewAssistantMessageEventStream maps Pi's upstream
// createAssistantMessageEventStream factory to an idiomatic Go constructor.
func NewAssistantMessageEventStream() *AssistantMessageEventStream {
	return &AssistantMessageEventStream{
		stream: NewEventStream(
			func(event AssistantMessageEvent) bool {
				typeOf, _ := assistantMessageEventVariantType(event)
				return typeOf == AssistantMessageEventTypeDone || typeOf == AssistantMessageEventTypeError
			},
			func(event AssistantMessageEvent) AssistantMessage {
				switch event := event.(type) {
				case AssistantMessageDoneEvent:
					return CloneAssistantMessage(event.Message)
				case *AssistantMessageDoneEvent:
					return CloneAssistantMessage(event.Message)
				case AssistantMessageErrorEvent:
					return CloneAssistantMessage(event.Error)
				case *AssistantMessageErrorEvent:
					return CloneAssistantMessage(event.Error)
				default:
					return AssistantMessage{}
				}
			},
		),
	}
}

// Next waits for the next AssistantMessage event.
func (s *AssistantMessageEventStream) Next(ctx context.Context) (AssistantMessageEvent, bool, error) {
	return s.stream.Next(ctx)
}

// Result waits for the final AssistantMessage outcome.
func (s *AssistantMessageEventStream) Result(ctx context.Context) (AssistantMessage, error) {
	result, err := s.stream.Result(ctx)
	if err != nil {
		return result, err
	}
	return CloneAssistantMessage(result), nil
}

// Push publishes an AssistantMessage event.
func (s *AssistantMessageEventStream) Push(event AssistantMessageEvent) {
	if err := validateAssistantMessageEvent(event); err != nil {
		s.stream.fail(err)
		return
	}
	s.stream.Push(cloneAssistantMessageEvent(event))
}

// End closes the stream, optionally with an explicit final AssistantMessage.
func (s *AssistantMessageEventStream) End(result ...AssistantMessage) {
	if len(result) == 1 {
		s.stream.End(CloneAssistantMessage(result[0]))
		return
	}
	s.stream.End(result...)
}

func cloneAssistantMessageEvent(event AssistantMessageEvent) AssistantMessageEvent {
	switch event := event.(type) {
	case AssistantMessageStartEvent:
		event.Partial = CloneAssistantMessage(event.Partial)
		return event
	case *AssistantMessageStartEvent:
		if event == nil {
			return nil
		}
		clone := *event
		clone.Partial = CloneAssistantMessage(event.Partial)
		return &clone
	case AssistantMessageTextStartEvent:
		event.Partial = CloneAssistantMessage(event.Partial)
		return event
	case *AssistantMessageTextStartEvent:
		if event == nil {
			return nil
		}
		clone := *event
		clone.Partial = CloneAssistantMessage(event.Partial)
		return &clone
	case AssistantMessageTextDeltaEvent:
		event.Partial = CloneAssistantMessage(event.Partial)
		return event
	case *AssistantMessageTextDeltaEvent:
		if event == nil {
			return nil
		}
		clone := *event
		clone.Partial = CloneAssistantMessage(event.Partial)
		return &clone
	case AssistantMessageTextEndEvent:
		event.Partial = CloneAssistantMessage(event.Partial)
		return event
	case *AssistantMessageTextEndEvent:
		if event == nil {
			return nil
		}
		clone := *event
		clone.Partial = CloneAssistantMessage(event.Partial)
		return &clone
	case AssistantMessageThinkingStartEvent:
		event.Partial = CloneAssistantMessage(event.Partial)
		return event
	case *AssistantMessageThinkingStartEvent:
		if event == nil {
			return nil
		}
		clone := *event
		clone.Partial = CloneAssistantMessage(event.Partial)
		return &clone
	case AssistantMessageThinkingDeltaEvent:
		event.Partial = CloneAssistantMessage(event.Partial)
		return event
	case *AssistantMessageThinkingDeltaEvent:
		if event == nil {
			return nil
		}
		clone := *event
		clone.Partial = CloneAssistantMessage(event.Partial)
		return &clone
	case AssistantMessageThinkingEndEvent:
		event.Partial = CloneAssistantMessage(event.Partial)
		return event
	case *AssistantMessageThinkingEndEvent:
		if event == nil {
			return nil
		}
		clone := *event
		clone.Partial = CloneAssistantMessage(event.Partial)
		return &clone
	case AssistantMessageToolCallStartEvent:
		event.Partial = CloneAssistantMessage(event.Partial)
		return event
	case *AssistantMessageToolCallStartEvent:
		if event == nil {
			return nil
		}
		clone := *event
		clone.Partial = CloneAssistantMessage(event.Partial)
		return &clone
	case AssistantMessageToolCallDeltaEvent:
		event.Partial = CloneAssistantMessage(event.Partial)
		return event
	case *AssistantMessageToolCallDeltaEvent:
		if event == nil {
			return nil
		}
		clone := *event
		clone.Partial = CloneAssistantMessage(event.Partial)
		return &clone
	case AssistantMessageToolCallEndEvent:
		event.ToolCall = cloneToolCall(event.ToolCall)
		event.Partial = CloneAssistantMessage(event.Partial)
		return event
	case *AssistantMessageToolCallEndEvent:
		if event == nil {
			return nil
		}
		clone := *event
		clone.ToolCall = cloneToolCall(event.ToolCall)
		clone.Partial = CloneAssistantMessage(event.Partial)
		return &clone
	case AssistantMessageDoneEvent:
		event.Message = CloneAssistantMessage(event.Message)
		return event
	case *AssistantMessageDoneEvent:
		if event == nil {
			return nil
		}
		clone := *event
		clone.Message = CloneAssistantMessage(event.Message)
		return &clone
	case AssistantMessageErrorEvent:
		event.Error = CloneAssistantMessage(event.Error)
		return event
	case *AssistantMessageErrorEvent:
		if event == nil {
			return nil
		}
		clone := *event
		clone.Error = CloneAssistantMessage(event.Error)
		return &clone
	default:
		return event
	}
}

func validateAssistantMessageEvent(event AssistantMessageEvent) error {
	want, ok := assistantMessageEventVariantType(event)
	if !ok || isNilClosedUnion(event) {
		return newCodecError("assistant message event", "", errors.New("nil or unsupported event variant"))
	}
	if got := event.AssistantMessageEventType(); got != want {
		return newCodecError("assistant message event", string(got), errors.New("concrete variant and discriminator disagree"))
	}
	switch value := event.(type) {
	case AssistantMessageDoneEvent:
		if !validDoneReason(value.Reason) {
			return newCodecError("assistant message event", string(want), errors.New("invalid done reason"))
		}
	case *AssistantMessageDoneEvent:
		if !validDoneReason(value.Reason) {
			return newCodecError("assistant message event", string(want), errors.New("invalid done reason"))
		}
	case AssistantMessageErrorEvent:
		if !validErrorReason(value.Reason) {
			return newCodecError("assistant message event", string(want), errors.New("invalid error reason"))
		}
	case *AssistantMessageErrorEvent:
		if !validErrorReason(value.Reason) {
			return newCodecError("assistant message event", string(want), errors.New("invalid error reason"))
		}
	}
	return nil
}

func cloneToolCall(toolCall ToolCall) ToolCall {
	message := CloneAssistantMessage(AssistantMessage{Content: []AssistantContent{toolCall}})
	return message.Content[0].(ToolCall)
}

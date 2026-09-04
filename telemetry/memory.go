package telemetry

import (
	"context"
	"reflect"
	"sync"
)

// RecordedTelemetryEvent is a detached snapshot of a recorded span event.
type RecordedTelemetryEvent struct {
	Name       string
	Attributes SpanAttributes
}

// RecordedTelemetrySpan is a detached snapshot of a recorded span. ParentID is
// nil for a root span and EndSequence is nil until the span settles, mapping
// Pi's `number | null` and optional `number` (docs/design/compatibility.md
// three-state fields).
type RecordedTelemetrySpan struct {
	ID          int
	ParentID    *int
	Name        string
	Attributes  SpanAttributes
	Events      []RecordedTelemetryEvent
	Status      SpanStatus
	Settled     bool
	EndSequence *int
}

// InMemoryTelemetryContext records an unbounded set of spans. Its zero value is
// ready to use. Methods are safe for concurrent use; do not copy it after use.
type InMemoryTelemetryContext struct {
	mu          sync.Mutex
	spans       []*memorySpan
	endSequence int
}

type memorySpan struct {
	owner          *InMemoryTelemetryContext
	recorded       RecordedTelemetrySpan
	explicitStatus bool
}

func (m *InMemoryTelemetryContext) StartSpan(ctx context.Context, options SpanOptions, fn SpanFunc) (any, error) {
	return m.startSpan(ctx, nil, options, fn)
}

func (m *InMemoryTelemetryContext) startSpan(ctx context.Context, parent *memorySpan, options SpanOptions, fn SpanFunc) (result any, err error) {
	m.mu.Lock()
	if parent != nil && parent.recorded.Settled {
		m.mu.Unlock()
		return noopSpan{}.StartSpan(ctx, options, fn)
	}
	attributes, ok := copyAttributes(options.Attributes)
	if !ok {
		m.mu.Unlock()
		return noopSpan{}.StartSpan(ctx, options, fn)
	}
	span := &memorySpan{owner: m, recorded: RecordedTelemetrySpan{ID: len(m.spans) + 1, Name: options.Name, Attributes: attributes, Events: []RecordedTelemetryEvent{}, Status: SpanStatus{Status: SpanStatusOK}}}
	if parent != nil {
		id := parent.recorded.ID
		span.recorded.ParentID = &id
	}
	m.spans = append(m.spans, span)
	m.mu.Unlock()
	defer func() {
		if failure := recover(); failure != nil {
			span.settle(true, failure)
			panic(failure)
		}
		span.settle(err != nil, err)
	}()
	return fn(ctx, span)
}

func (s *memorySpan) settle(failed bool, failure any) {
	m := s.owner
	m.mu.Lock()
	automatic := failed && !s.explicitStatus
	m.mu.Unlock()
	var status SpanStatus
	if automatic {
		status = automaticErrorStatus(failure)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if automatic && !s.explicitStatus {
		s.recorded.Status = status
	}
	m.endSequence++
	end := m.endSequence
	s.recorded.EndSequence = &end
	s.recorded.Settled = true
}

func automaticErrorStatus(failure any) (status SpanStatus) {
	status.Status = SpanStatusError
	defer func() { _ = recover() }()
	if err, ok := failure.(error); ok {
		status.Error = &SpanError{Name: "Error", Message: err.Error()}
	}
	return status
}

func copyStatus(status SpanStatus) SpanStatus {
	if status.Status == SpanStatusOK {
		return SpanStatus{Status: SpanStatusOK}
	}
	if status.Error != nil {
		detail := *status.Error
		status.Error = &detail
	}
	return status
}

// GetSpans returns detached snapshots in span-start order.
func (m *InMemoryTelemetryContext) GetSpans() []RecordedTelemetrySpan {
	m.mu.Lock()
	defer m.mu.Unlock()
	spans := make([]RecordedTelemetrySpan, len(m.spans))
	for i, span := range m.spans {
		spans[i] = span.recorded
		spans[i].Attributes, _ = copyAttributes(span.recorded.Attributes)
		spans[i].Status = copyStatus(span.recorded.Status)
		spans[i].Events = make([]RecordedTelemetryEvent, len(span.recorded.Events))
		for j, event := range span.recorded.Events {
			spans[i].Events[j] = event
			spans[i].Events[j].Attributes, _ = copyAttributes(event.Attributes)
		}
		if span.recorded.ParentID != nil {
			id := *span.recorded.ParentID
			spans[i].ParentID = &id
		}
		if span.recorded.EndSequence != nil {
			end := *span.recorded.EndSequence
			spans[i].EndSequence = &end
		}
	}
	return spans
}

func (s *memorySpan) StartSpan(ctx context.Context, options SpanOptions, fn SpanFunc) (any, error) {
	return s.owner.startSpan(ctx, s, options, fn)
}
func (s *memorySpan) AddEvent(name string, attributes SpanAttributes) {
	s.owner.mu.Lock()
	defer s.owner.mu.Unlock()
	if s.recorded.Settled {
		return
	}
	if copied, ok := copyAttributes(attributes); ok {
		s.recorded.Events = append(s.recorded.Events, RecordedTelemetryEvent{Name: name, Attributes: copied})
	}
}
func (s *memorySpan) SetAttributes(attributes SpanAttributes) {
	s.owner.mu.Lock()
	defer s.owner.mu.Unlock()
	if s.recorded.Settled {
		return
	}
	if copied, ok := copyAttributes(attributes); ok {
		for key, value := range copied {
			s.recorded.Attributes[key] = value
		}
	}
}
func (s *memorySpan) SetStatus(status SpanStatus) {
	s.owner.mu.Lock()
	defer s.owner.mu.Unlock()
	if s.recorded.Settled || (status.Status != SpanStatusOK && status.Status != SpanStatusError) {
		return
	}
	s.recorded.Status = copyStatus(status)
	s.explicitStatus = true
}

func copyAttributes(attributes SpanAttributes) (SpanAttributes, bool) {
	copied := make(SpanAttributes, len(attributes))
	for key, value := range attributes {
		if value == nil {
			continue
		}
		v := reflect.ValueOf(value)
		if v.Kind() == reflect.Slice || v.Kind() == reflect.Array {
			array := reflect.New(v.Type()).Elem()
			if v.Kind() == reflect.Slice {
				array = reflect.MakeSlice(v.Type(), v.Len(), v.Len())
			}
			for i := 0; i < v.Len(); i++ {
				if !isAttributeScalar(reflect.ValueOf(v.Index(i).Interface())) {
					return nil, false
				}
			}
			reflect.Copy(array, v)
			copied[key] = array.Interface()
		} else if isAttributeScalar(v) {
			copied[key] = value
		} else {
			return nil, false
		}
	}
	return copied, true
}

func isAttributeScalar(value reflect.Value) bool {
	switch value.Kind() {
	case reflect.String, reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Float32, reflect.Float64:
		return true
	}
	return false
}

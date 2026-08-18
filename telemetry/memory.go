package telemetry

import "context"

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

// InMemoryTelemetryContext is Pi's backend-neutral reference context that
// records spans in process memory.
//
// M0 (issue #23): the type and its recorded-data surface are mapped so callers
// compile, but the recording behavior is Pi's M2 work. Rather than fake success
// by running callbacks while recording nothing, StartSpan fails explicitly with
// a structured *NotImplementedError and GetSpans returns no spans. Applications
// that only need a safe default should use NOOPTelemetryContext, which is live.
type InMemoryTelemetryContext struct{}

// StartSpan reports that in-memory recording is not implemented at M0. It does
// not run fn, so the failure is unambiguous rather than a silent no-record.
func (*InMemoryTelemetryContext) StartSpan(context.Context, SpanOptions, SpanFunc) (any, error) {
	return nil, &NotImplementedError{Module: "telemetry", Operation: "InMemoryTelemetryContext.StartSpan"}
}

// GetSpans returns the recorded spans in span-start order. Because StartSpan is
// not implemented at M0, no spans are ever recorded and this returns nil.
func (*InMemoryTelemetryContext) GetSpans() []RecordedTelemetrySpan {
	return nil
}

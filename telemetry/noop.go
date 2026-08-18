package telemetry

import "context"

// noopSpan is the shared no-op span. It is both the NOOP context and the span
// handed to every callback: StartSpan runs the callback and returns its result
// or error unchanged, and the recording methods do nothing. It carries no
// exporter, endpoint, buffer or global current span, so using it is free of
// side effects. A single shared value is safe because it holds no state.
type noopSpan struct{}

// StartSpan runs fn against the shared no-op span and returns its result and
// error unchanged, mirroring Pi's noop that resolves or rejects with the
// callback outcome.
func (noopSpan) StartSpan(ctx context.Context, _ SpanOptions, fn SpanFunc) (any, error) {
	return fn(ctx, noopSpan{})
}

// AddEvent discards the event.
func (noopSpan) AddEvent(string, SpanAttributes) {}

// SetAttributes discards the attributes.
func (noopSpan) SetAttributes(SpanAttributes) {}

// SetStatus discards the status.
func (noopSpan) SetStatus(SpanStatus) {}

// NOOPTelemetryContext is the shared telemetry context used when an application
// does not provide one (Pi: NOOP_TELEMETRY_CONTEXT). It is a real, always-safe
// default: it invokes callbacks and propagates their outcome while recording
// nothing and reaching no backend.
var NOOPTelemetryContext TelemetryContext = noopSpan{}

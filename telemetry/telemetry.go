// Package telemetry is the Go mapping of Pi's vendor-neutral telemetry
// contract (packages/telemetry, baseline 936aff0). It defines how a
// nested, attributed unit of work (a span) is described and recorded, so the
// ai and agent modules can instrument themselves without binding to any
// concrete observability backend.
//
// The default context is NOOP. InMemoryTelemetryContext provides unbounded,
// concurrent recording and detached snapshots; telemetry/testing supplies
// executable adapter conformance cases.
//
// Language mapping (docs/design/compatibility.md): Pi's Promise-returning
// startSpan becomes a context.Context-taking StartSpan whose callback returns
// (any, error); a rejected Promise maps to a non-nil error. A panic settles
// the span as failed and propagates unchanged. The result value is
// type-erased to any, matching the erased-registry approach in ADR-0007. This
// package is stdlib-only and carries no exporter, endpoint or global current
// span; it is not imported by cmd/pig or cmd/pig-ai.
package telemetry

import "context"

// AttributeValue admits strings, numbers, booleans and arrays/slices of those.
// Other dynamic values cause the whole recording call to be ignored. The
// recorder never invokes user conversion, formatting or serialization hooks.
type AttributeValue = any

// SpanAttributes is a bag of named attribute values. A nil or absent value is
// treated as unset, mirroring Pi's `AttributeValue | undefined` index signature.
type SpanAttributes map[string]AttributeValue

// SpanOptions parameterises a span. Name is required; Attributes is optional.
type SpanOptions struct {
	Name       string
	Attributes SpanAttributes
}

// Span status discriminants (Pi: `{status:"ok"} | {status:"error"; error?}`).
const (
	SpanStatusOK    = "ok"
	SpanStatusError = "error"
)

// SpanError carries the optional name/message of an error span status.
type SpanError struct {
	Name    string
	Message string
}

// SpanStatus is a span's terminal status: SpanStatusOK, or SpanStatusError with
// an optional SpanError. Error is nil for an error status without details.
type SpanStatus struct {
	Status string
	Error  *SpanError
}

// SpanFunc is the work executed within a span. It receives the span (which is
// itself a TelemetryContext, so it can open child spans) and returns the value
// StartSpan resolves to, or an error that maps to Pi's rejected Promise.
type SpanFunc func(ctx context.Context, span TelemetrySpan) (any, error)

// TelemetryContext can open a span around a unit of work. StartSpan runs fn,
// then returns its result and error unchanged. This is the single seam that a
// backend adapter implements.
type TelemetryContext interface {
	StartSpan(ctx context.Context, options SpanOptions, fn SpanFunc) (any, error)
}

// TelemetrySpan is an active span. It is also a TelemetryContext, so calling
// StartSpan on it opens a child span. AddEvent, SetAttributes and SetStatus
// record into the span; Pi specifies these as passive (never throwing) and
// inert after the span settles.
type TelemetrySpan interface {
	TelemetryContext
	AddEvent(name string, attributes SpanAttributes)
	SetAttributes(attributes SpanAttributes)
	SetStatus(status SpanStatus)
}

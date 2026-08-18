package telemetry_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/nankedr/pig/telemetry"
)

// TestNOOPRunsCallbackAndReturnsResult verifies the NOOP context is a live,
// side-effect-free default: it invokes the callback exactly once and returns its
// value unchanged.
func TestNOOPRunsCallbackAndReturnsResult(t *testing.T) {
	calls := 0
	got, err := telemetry.NOOPTelemetryContext.StartSpan(context.Background(),
		telemetry.SpanOptions{Name: "success"},
		func(context.Context, telemetry.TelemetrySpan) (any, error) {
			calls++
			return 42, nil
		})
	if err != nil {
		t.Fatalf("NOOP StartSpan err = %v", err)
	}
	if calls != 1 {
		t.Fatalf("callback calls = %d, want 1", calls)
	}
	if got != 42 {
		t.Fatalf("result = %v, want 42", got)
	}
}

// TestNOOPPropagatesError verifies a rejected callback (non-nil error) is
// returned unchanged, mirroring Pi's noop rejecting with the callback outcome.
func TestNOOPPropagatesError(t *testing.T) {
	sentinel := errors.New("boom")
	got, err := telemetry.NOOPTelemetryContext.StartSpan(context.Background(),
		telemetry.SpanOptions{Name: "failure"},
		func(context.Context, telemetry.TelemetrySpan) (any, error) {
			return nil, sentinel
		})
	if !errors.Is(err, sentinel) {
		t.Fatalf("errors.Is(%v, sentinel) = false", err)
	}
	if got != nil {
		t.Fatalf("result = %v, want nil", got)
	}
}

// TestNOOPRecordingAndNestingAreInert verifies the recording methods never panic
// and a NOOP span is itself a context that opens working child spans.
func TestNOOPRecordingAndNestingAreInert(t *testing.T) {
	_, err := telemetry.NOOPTelemetryContext.StartSpan(context.Background(),
		telemetry.SpanOptions{Name: "parent", Attributes: telemetry.SpanAttributes{"k": "v"}},
		func(ctx context.Context, span telemetry.TelemetrySpan) (any, error) {
			span.AddEvent("event", telemetry.SpanAttributes{"i": 1})
			span.SetAttributes(telemetry.SpanAttributes{"more": true})
			span.SetStatus(telemetry.SpanStatus{Status: telemetry.SpanStatusError, Error: &telemetry.SpanError{Name: "E", Message: "m"}})

			child, cerr := span.StartSpan(ctx, telemetry.SpanOptions{Name: "child"},
				func(context.Context, telemetry.TelemetrySpan) (any, error) { return "child-result", nil })
			if cerr != nil {
				return nil, cerr
			}
			if child != "child-result" {
				t.Errorf("child result = %v, want child-result", child)
			}
			return nil, nil
		})
	if err != nil {
		t.Fatalf("NOOP nested StartSpan err = %v", err)
	}
}

// TestDefineTelemetrySchemaIdentity verifies the typed identity helper returns
// its input unchanged.
func TestDefineTelemetrySchemaIdentity(t *testing.T) {
	schema := telemetry.TelemetrySchemaDefinition{
		Version: 1,
		Spans: map[string]telemetry.TelemetrySpanDefinition{
			"work": {
				Description: "a unit of work",
				Parents:     telemetry.TelemetryParentDefinition{Kind: telemetry.ParentKindAny},
				Status:      telemetry.TelemetrySpanStatusRule{Default: telemetry.SpanStatusOK, ErrorWhen: "callback rejects"},
			},
		},
	}
	if got := telemetry.DefineTelemetrySchema(schema); !reflect.DeepEqual(got, schema) {
		t.Fatalf("DefineTelemetrySchema mutated its input: %#v", got)
	}
}

// TestInMemoryStartSpanFailsExplicitly verifies the M0 deferral is honest: the
// in-memory recorder does not run the callback (no disguised no-record success)
// and returns a structured *NotImplementedError identifying the module.
func TestInMemoryStartSpanFailsExplicitly(t *testing.T) {
	mem := &telemetry.InMemoryTelemetryContext{}
	ran := false
	got, err := mem.StartSpan(context.Background(), telemetry.SpanOptions{Name: "s"},
		func(context.Context, telemetry.TelemetrySpan) (any, error) {
			ran = true
			return "unexpected", nil
		})
	if ran {
		t.Fatal("in-memory StartSpan ran the callback; a deferred stub must not")
	}
	if got != nil {
		t.Fatalf("result = %v, want nil", got)
	}
	if !errors.Is(err, telemetry.ErrNotImplemented) {
		t.Fatalf("errors.Is(%v, ErrNotImplemented) = false", err)
	}
	var nie *telemetry.NotImplementedError
	if !errors.As(err, &nie) {
		t.Fatalf("errors.As(%v, *NotImplementedError) = false", err)
	}
	if nie.Module != "telemetry" {
		t.Fatalf("NotImplementedError.Module = %q, want telemetry", nie.Module)
	}
	if spans := mem.GetSpans(); spans != nil {
		t.Fatalf("GetSpans = %v, want nil", spans)
	}
}

// TestCreateTypedSpanStarterOverNOOP verifies the typed starter is a genuine
// thin binding: over the NOOP context it runs the callback, hands back a usable
// child starter, and returns the result unchanged.
func TestCreateTypedSpanStarterOverNOOP(t *testing.T) {
	schema := telemetry.DefineTelemetrySchema(telemetry.TelemetrySchemaDefinition{Version: 1})
	starter := telemetry.CreateTypedSpanStarter(telemetry.NOOPTelemetryContext, schema)

	childRan := false
	got, err := starter(context.Background(), "span", telemetry.SpanAttributes{"a": 1},
		func(ctx context.Context, span telemetry.TelemetrySpan, startChild telemetry.TypedSpanStarter) (any, error) {
			_, cerr := startChild(ctx, "child", nil,
				func(context.Context, telemetry.TelemetrySpan, telemetry.TypedSpanStarter) (any, error) {
					childRan = true
					return nil, nil
				})
			return "done", cerr
		})
	if err != nil {
		t.Fatalf("typed starter err = %v", err)
	}
	if got != "done" {
		t.Fatalf("result = %v, want done", got)
	}
	if !childRan {
		t.Fatal("child typed starter did not run")
	}
}

// TestCreateTypedSpanStarterOverInMemoryFails verifies the typed starter does
// not paper over a deferred backend: bound to the in-memory recorder it surfaces
// the same structured failure.
func TestCreateTypedSpanStarterOverInMemoryFails(t *testing.T) {
	starter := telemetry.CreateTypedSpanStarter(&telemetry.InMemoryTelemetryContext{},
		telemetry.TelemetrySchemaDefinition{Version: 1})
	_, err := starter(context.Background(), "span", nil,
		func(context.Context, telemetry.TelemetrySpan, telemetry.TypedSpanStarter) (any, error) {
			return nil, nil
		})
	if !errors.Is(err, telemetry.ErrNotImplemented) {
		t.Fatalf("errors.Is(%v, ErrNotImplemented) = false", err)
	}
}

// Compile-time surface parity: every one of the 34 exported symbols on Pi's
// telemetry root (`.`) export subpath maps to a Go declaration here. This is the
// Go-side API snapshot proving the acceptance criterion of no omission; it can
// be checked line-for-line against the root entries in parity/surface/symbols.jsonl.
var (
	// Core span contract.
	_ telemetry.AttributeValue   // AttributeValue
	_ telemetry.SpanAttributes   // SpanAttributes
	_ telemetry.SpanOptions      // SpanOptions
	_ telemetry.SpanStatus       // SpanStatus
	_ telemetry.TelemetryContext // TelemetryContext
	_ telemetry.TelemetrySpan    // TelemetrySpan

	// Dynamic schema definition data.
	_ telemetry.TelemetryAttributeType            // TelemetryAttributeType
	_ telemetry.TelemetryAttributeMetadata        // TelemetryAttributeMetadata
	_ telemetry.TelemetryAttributeDefinition      // TelemetryAttributeDefinition
	_ telemetry.TelemetryStartAttributeDefinition // TelemetryStartAttributeDefinition
	_ telemetry.TelemetryEventAttributeDefinition // TelemetryEventAttributeDefinition
	_ telemetry.TelemetryEventDefinition          // TelemetryEventDefinition
	_ telemetry.TelemetryParentDefinition         // TelemetryParentDefinition
	_ telemetry.TelemetrySpanDefinition           // TelemetrySpanDefinition
	_ telemetry.TelemetrySchemaDefinition         // TelemetrySchemaDefinition

	// Erased typed-helper surface (concrete typed forms come from the generator).
	_ telemetry.InferRequiredAndOptionalAttributes // InferRequiredAndOptionalAttributes
	_ telemetry.InferStartAttributes               // InferStartAttributes
	_ telemetry.InferOptionalAttributes            // InferOptionalAttributes
	_ telemetry.InferEventAttributes               // InferEventAttributes
	_ telemetry.ExactTelemetryAttributes           // ExactTelemetryAttributes
	_ telemetry.SchemaTelemetrySpan                // SchemaTelemetrySpan
	_ telemetry.TelemetrySchemaSpanName            // TelemetrySchemaSpanName
	_ telemetry.TelemetrySchemaSpanStartAttributes // TelemetrySchemaSpanStartAttributes
	_ telemetry.TelemetrySchemaSpanEndAttributes   // TelemetrySchemaSpanEndAttributes
	_ telemetry.TelemetrySchemaSpanEventName       // TelemetrySchemaSpanEventName
	_ telemetry.TelemetrySchemaSpanEventAttributes // TelemetrySchemaSpanEventAttributes
	_ telemetry.TelemetrySchemaSpanUnion           // TelemetrySchemaSpanUnion
	_ telemetry.TypedSpanStarter                   // TypedSpanStarter

	// In-memory reference recorder.
	_ telemetry.InMemoryTelemetryContext // InMemoryTelemetryContext
	_ telemetry.RecordedTelemetryEvent   // RecordedTelemetryEvent
	_ telemetry.RecordedTelemetrySpan    // RecordedTelemetrySpan

	// Functions and the NOOP default.
	_ = telemetry.DefineTelemetrySchema  // defineTelemetrySchema
	_ = telemetry.CreateTypedSpanStarter // createTypedSpanStarter
	_ = telemetry.NOOPTelemetryContext   // NOOP_TELEMETRY_CONTEXT
)

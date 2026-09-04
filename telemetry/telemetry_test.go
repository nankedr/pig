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

func TestInMemorySpanLifecycle(t *testing.T) {
	mem := &telemetry.InMemoryTelemetryContext{}
	expected := &struct{ Value int }{42}
	calls := 0
	got, err := mem.StartSpan(context.Background(), telemetry.SpanOptions{Name: "work"},
		func(ctx context.Context, span telemetry.TelemetrySpan) (any, error) {
			calls++
			active := mem.GetSpans()
			if len(active) != 1 || active[0].Settled || active[0].EndSequence != nil || active[0].Status.Status != telemetry.SpanStatusOK {
				t.Fatalf("active span = %#v", active)
			}
			return expected, nil
		})
	if got != expected || err != nil || calls != 1 {
		t.Fatalf("callback = %v, %v, calls %d", got, err, calls)
	}
	spans := mem.GetSpans()
	if len(spans) != 1 || spans[0].ID != 1 || spans[0].Name != "work" || spans[0].ParentID != nil || !spans[0].Settled || spans[0].EndSequence == nil || *spans[0].EndSequence != 1 {
		t.Fatalf("settled span = %#v", spans)
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

func TestCreateTypedSpanStarterOverInMemory(t *testing.T) {
	mem := &telemetry.InMemoryTelemetryContext{}
	starter := telemetry.CreateTypedSpanStarter(mem, telemetry.TelemetrySchemaDefinition{Version: 1})
	got, err := starter(context.Background(), "parent", nil,
		func(ctx context.Context, _ telemetry.TelemetrySpan, child telemetry.TypedSpanStarter) (any, error) {
			return child(ctx, "child", nil, func(context.Context, telemetry.TelemetrySpan, telemetry.TypedSpanStarter) (any, error) { return 7, nil })
		})
	if got != 7 || err != nil {
		t.Fatalf("typed callback = %v, %v", got, err)
	}
	spans := mem.GetSpans()
	if len(spans) != 2 || spans[1].ParentID == nil || *spans[1].ParentID != spans[0].ID {
		t.Fatalf("typed spans = %#v", spans)
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

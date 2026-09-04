package telemetry_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"

	"github.com/nankedr/pig/telemetry"
)

type unreadableError struct{}

func (unreadableError) Error() string { panic("unreadable error") }

func TestInMemoryPreservesFailuresAndExplicitStatus(t *testing.T) {
	for _, tc := range []struct {
		name     string
		failure  any
		panic    bool
		explicit *telemetry.SpanStatus
		want     telemetry.SpanStatus
	}{
		{name: "error", failure: errors.New("failed"), want: telemetry.SpanStatus{Status: "error", Error: &telemetry.SpanError{Name: "Error", Message: "failed"}}},
		{name: "panic", failure: &struct{ Kind string }{"panic"}, panic: true, want: telemetry.SpanStatus{Status: "error"}},
		{name: "unreadable", failure: unreadableError{}, want: telemetry.SpanStatus{Status: "error"}},
		{name: "explicit ok", failure: errors.New("failed"), explicit: &telemetry.SpanStatus{Status: "ok"}, want: telemetry.SpanStatus{Status: "ok"}},
		{name: "explicit error", failure: errors.New("failed"), panic: true, explicit: &telemetry.SpanStatus{Status: "error", Error: &telemetry.SpanError{Name: "Expected", Message: "recorded"}}, want: telemetry.SpanStatus{Status: "error", Error: &telemetry.SpanError{Name: "Expected", Message: "recorded"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mem := &telemetry.InMemoryTelemetryContext{}
			func() {
				defer func() {
					if got := recover(); got != nil || tc.panic {
						if !tc.panic || got != tc.failure {
							t.Fatalf("panic = %#v", got)
						}
					}
				}()
				got, err := mem.StartSpan(context.Background(), telemetry.SpanOptions{Name: tc.name}, func(_ context.Context, span telemetry.TelemetrySpan) (any, error) {
					if tc.explicit != nil {
						span.SetStatus(telemetry.SpanStatus{Status: "error"})
						span.SetStatus(*tc.explicit)
					}
					if tc.panic {
						panic(tc.failure)
					}
					return 42, tc.failure.(error)
				})
				if got != 42 || err != tc.failure {
					t.Fatalf("outcome = %v, %v", got, err)
				}
			}()
			spans := mem.GetSpans()
			if len(spans) != 1 || !spans[0].Settled || spans[0].EndSequence == nil || !reflect.DeepEqual(spans[0].Status, tc.want) {
				t.Fatalf("spans = %#v, want status %#v", spans, tc.want)
			}
		})
	}
}

func TestInMemorySnapshotsAndPassiveRecording(t *testing.T) {
	mem := &telemetry.InMemoryTelemetryContext{}
	strings := []string{"initial"}
	numbers := []float64{1, 2}
	attributes := telemetry.SpanAttributes{"strings": strings, "numbers": numbers, "flag": true, "unset": nil}
	detail := &telemetry.SpanError{Name: "Expected", Message: "initial"}
	var captured telemetry.TelemetrySpan
	_, err := mem.StartSpan(context.Background(), telemetry.SpanOptions{Name: "parent", Attributes: attributes}, func(ctx context.Context, span telemetry.TelemetrySpan) (any, error) {
		captured = span
		strings[0] = "mutated"
		numbers[0] = 99
		attributes["flag"] = false
		span.SetAttributes(telemetry.SpanAttributes{"count": 1, "flag": nil})
		span.SetAttributes(telemetry.SpanAttributes{"partial": "discard", "bad": func() { panic("unreadable") }})
		span.AddEvent("bad", telemetry.SpanAttributes{"partial": true, "bad": map[string]any{"nested": true}})
		eventValues := []bool{true}
		span.AddEvent("first", telemetry.SpanAttributes{"values": eventValues})
		eventValues[0] = false
		span.AddEvent("second", nil)
		span.SetStatus(telemetry.SpanStatus{Status: "error", Error: detail})
		detail.Message = "mutated"
		span.SetStatus(telemetry.SpanStatus{Status: "invalid"})
		_, err := span.StartSpan(ctx, telemetry.SpanOptions{Name: "child"}, func(context.Context, telemetry.TelemetrySpan) (any, error) { return nil, nil })
		return nil, err
	})
	if err != nil {
		t.Fatal(err)
	}
	spans := mem.GetSpans()
	wantAttributes := telemetry.SpanAttributes{"strings": []string{"initial"}, "numbers": []float64{1, 2}, "flag": true, "count": 1}
	if len(spans) != 2 || !reflect.DeepEqual(spans[0].Attributes, wantAttributes) || len(spans[0].Events) != 2 || spans[0].Events[0].Name != "first" || !reflect.DeepEqual(spans[0].Events[0].Attributes, telemetry.SpanAttributes{"values": []bool{true}}) || spans[0].Events[1].Name != "second" || spans[0].Status.Error == nil || spans[0].Status.Error.Message != "initial" {
		t.Fatalf("recorded = %#v", spans)
	}
	before := mem.GetSpans()
	spans[0].Name = "changed"
	spans[0].Attributes["strings"].([]string)[0] = "changed"
	spans[0].Attributes["numbers"].([]float64)[0] = 999
	spans[0].Events[0].Attributes["values"].([]bool)[0] = false
	spans[0].Status.Error.Message = "changed"
	*spans[1].ParentID = 999
	*spans[0].EndSequence = 999
	captured.SetAttributes(telemetry.SpanAttributes{"late": func() { panic("late") }})
	captured.AddEvent("late", nil)
	captured.SetStatus(telemetry.SpanStatus{Status: "ok"})
	got, err := captured.StartSpan(context.Background(), telemetry.SpanOptions{Name: "late-child"}, func(ctx context.Context, span telemetry.TelemetrySpan) (any, error) {
		return span.StartSpan(ctx, telemetry.SpanOptions{Name: "late-grandchild"}, func(context.Context, telemetry.TelemetrySpan) (any, error) { return 7, nil })
	})
	if got != 7 || err != nil || !reflect.DeepEqual(mem.GetSpans(), before) {
		t.Fatal("snapshots or late writes mutated recorded spans")
	}
	got, err = mem.StartSpan(context.Background(), telemetry.SpanOptions{Name: "bad", Attributes: telemetry.SpanAttributes{"partial": true, "bad": make(chan int)}}, func(ctx context.Context, span telemetry.TelemetrySpan) (any, error) {
		return span.StartSpan(ctx, telemetry.SpanOptions{Name: "ignored-child"}, func(context.Context, telemetry.TelemetrySpan) (any, error) { return 9, nil })
	})
	if got != 9 || err != nil || !reflect.DeepEqual(mem.GetSpans(), before) {
		t.Fatal("invalid options did not fall back atomically to NOOP")
	}
}

func TestInMemoryConcurrentRecordingAndSnapshots(t *testing.T) {
	mem := &telemetry.InMemoryTelemetryContext{}
	const workers = 32
	_, err := mem.StartSpan(context.Background(), telemetry.SpanOptions{Name: "parent"}, func(ctx context.Context, parent telemetry.TelemetrySpan) (any, error) {
		active := mem.GetSpans()
		var wg sync.WaitGroup
		for i := 0; i < workers; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				_, err := parent.StartSpan(ctx, telemetry.SpanOptions{Name: "child"}, func(_ context.Context, child telemetry.TelemetrySpan) (any, error) {
					for j := 0; j < 8; j++ {
						child.AddEvent("event", telemetry.SpanAttributes{"index": j})
						parent.AddEvent("child-event", nil)
						parent.SetAttributes(telemetry.SpanAttributes{fmt.Sprint(i): j})
						_ = mem.GetSpans()
					}
					return nil, nil
				})
				if err != nil {
					t.Error(err)
				}
			}(i)
		}
		wg.Wait()
		if active[0].Settled || len(active[0].Events) != 0 || len(active[0].Attributes) != 0 {
			t.Error("active snapshot mutated")
		}
		return nil, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	spans := mem.GetSpans()
	if len(spans) != workers+1 || len(spans[0].Events) != workers*8 || len(spans[0].Attributes) != workers || *spans[0].EndSequence != workers+1 {
		t.Fatalf("concurrent spans = %#v", spans)
	}
	ends := map[int]bool{}
	for i, span := range spans {
		if span.ID != i+1 || !span.Settled || span.EndSequence == nil || ends[*span.EndSequence] {
			t.Fatalf("invalid span = %#v", span)
		}
		ends[*span.EndSequence] = true
		if i > 0 && (span.ParentID == nil || *span.ParentID != 1 || len(span.Events) != 8) {
			t.Fatalf("child = %#v", span)
		}
	}
}

type inspectingError struct{ inspect func() }

func (e inspectingError) Error() string { e.inspect(); return "inspected" }

func TestInMemoryErrorInspectionIsReentrant(t *testing.T) {
	mem := &telemetry.InMemoryTelemetryContext{}
	expected := inspectingError{inspect: func() { _ = mem.GetSpans() }}
	_, err := mem.StartSpan(context.Background(), telemetry.SpanOptions{Name: "failure"}, func(context.Context, telemetry.TelemetrySpan) (any, error) { return nil, expected })
	if err == nil || mem.GetSpans()[0].Status.Error.Message != "inspected" {
		t.Fatal("error inspection failed")
	}
}

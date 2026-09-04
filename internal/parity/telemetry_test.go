package parity_test

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"github.com/nankedr/pig/internal/baseline"
	"github.com/nankedr/pig/internal/parity"
	"github.com/nankedr/pig/telemetry"
	telemetrytesting "github.com/nankedr/pig/telemetry/testing"
)

func TestTelemetryMemoryParity(t *testing.T) {
	root := parityRepoRoot(t)
	lock, _, err := baseline.Load(filepath.Join(root, "parity", "baseline"))
	if err != nil {
		t.Fatal(err)
	}
	locked := parity.Baseline{ID: lock.BaselineID, Commit: lock.Upstream.Commit, Repository: lock.Upstream.Repository}
	fixture, err := parity.LoadFixture(filepath.Join(root, "parity", "oracle", "fixtures", "telemetry.json"), locked)
	if err != nil {
		t.Fatal(err)
	}
	oracle, err := parity.NewFixtureDriver(fixture, locked)
	if err != nil {
		t.Fatal(err)
	}
	result, err := parity.RunCase(context.Background(), fixture.Case, oracle, parity.DriverFunc{SurfaceName: parity.SurfaceGoSDK, ObserveFunc: observeTelemetry})
	if err != nil || !result.Match || len(result.Normalizations) != 0 {
		t.Fatalf("telemetry parity = %+v, %v", result, err)
	}
}

type telemetryFixture struct {
	memory telemetry.InMemoryTelemetryContext
}

func (f *telemetryFixture) Context() telemetry.TelemetryContext { return &f.memory }
func (f *telemetryFixture) GetSpans(context.Context) ([]telemetry.RecordedTelemetrySpan, error) {
	return f.memory.GetSpans(), nil
}
func (*telemetryFixture) Close(context.Context) error { return nil }

func observeTelemetry(ctx context.Context, declaration parity.Case) (parity.Observation, error) {
	var input struct {
		Attributes telemetry.SpanAttributes `json:"attributes"`
		Failure    string                   `json:"failure"`
	}
	if err := json.Unmarshal(declaration.Input, &input); err != nil {
		return parity.Observation{}, err
	}
	memory := &telemetry.InMemoryTelemetryContext{}
	expected := &struct{ Value int }{42}
	var captured telemetry.TelemetrySpan
	var active []telemetry.RecordedTelemetrySpan
	result, err := memory.StartSpan(ctx, telemetry.SpanOptions{Name: "parent", Attributes: input.Attributes}, func(ctx context.Context, parent telemetry.TelemetrySpan) (any, error) {
		captured = parent
		active = memory.GetSpans()
		values := input.Attributes["values"].([]any)
		values[0] = "changed"
		parent.SetAttributes(telemetry.SpanAttributes{"count": 1, "ignored": nil})
		parent.AddEvent("first", telemetry.SpanAttributes{"values": values})
		values[0] = "changed-again"
		started, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
		go func() {
			_, err := parent.StartSpan(ctx, telemetry.SpanOptions{Name: "first-child"}, func(context.Context, telemetry.TelemetrySpan) (any, error) {
				close(started)
				<-release
				return nil, nil
			})
			done <- err
		}()
		<-started
		_, err := parent.StartSpan(ctx, telemetry.SpanOptions{Name: "second-child"}, func(_ context.Context, span telemetry.TelemetrySpan) (any, error) {
			span.SetStatus(telemetry.SpanStatus{Status: "error", Error: &telemetry.SpanError{Name: "Expected", Message: "handled"}})
			return nil, nil
		})
		close(release)
		err = errors.Join(err, <-done)
		parent.AddEvent("second", nil)
		return expected, err
	})
	if err != nil {
		return parity.Observation{}, err
	}
	failure := errors.New(input.Failure)
	_, gotError := memory.StartSpan(ctx, telemetry.SpanOptions{Name: "failure"}, func(context.Context, telemetry.TelemetrySpan) (any, error) { return nil, failure })
	_, explicitError := memory.StartSpan(ctx, telemetry.SpanOptions{Name: "explicit"}, func(_ context.Context, span telemetry.TelemetrySpan) (any, error) {
		span.SetStatus(telemetry.SpanStatus{Status: "ok"})
		return nil, failure
	})
	if explicitError != failure {
		return parity.Observation{}, errors.New("explicit status changed callback failure")
	}
	detached := memory.GetSpans()
	detached[0].Attributes["values"].([]any)[0] = "snapshot-mutation"
	detached[0].Events[0].Attributes["values"].([]any)[0] = "snapshot-mutation"
	detached[2].Status.Error.Message = "snapshot-mutation"
	captured.SetAttributes(telemetry.SpanAttributes{"late": true})
	captured.AddEvent("late", nil)
	late, err := captured.StartSpan(ctx, telemetry.SpanOptions{Name: "late-child"}, func(context.Context, telemetry.TelemetrySpan) (any, error) { return 7, nil })
	if err != nil {
		return parity.Observation{}, err
	}
	conformance := []map[string]any{}
	for _, c := range telemetrytesting.CreateTelemetryAdapterConformance(func(context.Context) (telemetrytesting.TelemetryAdapterFixture, error) {
		return &telemetryFixture{}, nil
	}) {
		if err := c.Run(ctx); err != nil {
			return parity.Observation{}, err
		}
		conformance = append(conformance, map[string]any{"group": c.Group(), "name": c.Name(), "passed": true})
	}
	outcome, err := json.Marshal(map[string]any{"active": projectTelemetrySpans(active), "spans": projectTelemetrySpans(memory.GetSpans()), "same_result": result == expected, "same_error": gotError == failure, "late_result": late, "conformance": conformance})
	sideEffects := []parity.SideEffect{}
	return parity.Observation{Outcome: outcome, SideEffects: &sideEffects}, err
}

func projectTelemetrySpans(spans []telemetry.RecordedTelemetrySpan) []map[string]any {
	projected := make([]map[string]any, len(spans))
	for i, span := range spans {
		status := map[string]any{"status": span.Status.Status}
		if span.Status.Error != nil {
			status["error"] = map[string]any{"name": span.Status.Error.Name, "message": span.Status.Error.Message}
		}
		events := make([]map[string]any, len(span.Events))
		for j, event := range span.Events {
			events[j] = map[string]any{"name": event.Name, "attributes": event.Attributes}
		}
		projected[i] = map[string]any{"id": span.ID, "parentId": span.ParentID, "name": span.Name, "attributes": span.Attributes, "events": events, "status": status, "settled": span.Settled}
		if span.EndSequence != nil {
			projected[i]["endSequence"] = *span.EndSequence
		}
	}
	return projected
}

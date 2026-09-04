package main

import (
	"context"
	"fmt"

	"github.com/nankedr/pig/telemetry"
	telemetrytesting "github.com/nankedr/pig/telemetry/testing"
)

type fixture struct {
	memory telemetry.InMemoryTelemetryContext
}

func (f *fixture) Context() telemetry.TelemetryContext { return &f.memory }
func (f *fixture) GetSpans(context.Context) ([]telemetry.RecordedTelemetrySpan, error) {
	return f.memory.GetSpans(), nil
}
func (*fixture) Close(context.Context) error { return nil }

func main() {
	ctx := context.Background()
	memory := &telemetry.InMemoryTelemetryContext{}
	result, err := memory.StartSpan(ctx, telemetry.SpanOptions{Name: "work", Attributes: telemetry.SpanAttributes{"attempt": 1}}, func(ctx context.Context, parent telemetry.TelemetrySpan) (any, error) {
		return parent.StartSpan(ctx, telemetry.SpanOptions{Name: "step"}, func(_ context.Context, span telemetry.TelemetrySpan) (any, error) {
			span.AddEvent("completed", nil)
			return 42, nil
		})
	})
	if err != nil {
		panic(err)
	}
	fmt.Printf("result=%v\n", result)
	for _, span := range memory.GetSpans() {
		parent := 0
		if span.ParentID != nil {
			parent = *span.ParentID
		}
		fmt.Printf("span=%d name=%s parent=%d status=%s settled=%t end=%d\n", span.ID, span.Name, parent, span.Status.Status, span.Settled, *span.EndSequence)
	}
	cases := telemetrytesting.CreateTelemetryAdapterConformance(func(context.Context) (telemetrytesting.TelemetryAdapterFixture, error) { return &fixture{}, nil })
	for _, c := range cases {
		if err := c.Run(ctx); err != nil {
			panic(err)
		}
	}
	fmt.Printf("conformance=%d passed\n", len(cases))
}

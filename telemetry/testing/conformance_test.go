package testing_test

import (
	"context"
	"errors"
	stdtesting "testing"

	"github.com/nankedr/pig/telemetry"
	telemetrytesting "github.com/nankedr/pig/telemetry/testing"
)

type memoryFixture struct {
	telemetry.InMemoryTelemetryContext
}

func (f *memoryFixture) Context() telemetry.TelemetryContext { return &f.InMemoryTelemetryContext }
func (f *memoryFixture) GetSpans(context.Context) ([]telemetry.RecordedTelemetrySpan, error) {
	return f.InMemoryTelemetryContext.GetSpans(), nil
}
func (*memoryFixture) Close(context.Context) error { return nil }

func TestInMemoryAdapterConformance(t *stdtesting.T) {
	factories := 0
	cases := telemetrytesting.CreateTelemetryAdapterConformance(func(context.Context) (telemetrytesting.TelemetryAdapterFixture, error) {
		factories++
		return &memoryFixture{}, nil
	})
	if len(cases) != 9 {
		t.Fatalf("case count = %d", len(cases))
	}
	for _, c := range cases {
		t.Run(c.Group()+"/"+c.Name(), func(t *stdtesting.T) {
			if err := c.Run(context.Background()); err != nil {
				t.Fatal(err)
			}
		})
	}
	if factories != 9 {
		t.Fatalf("fresh fixtures = %d", factories)
	}
}

func TestConformancePropagatesFactoryFailure(t *stdtesting.T) {
	expected := errors.New("factory failed")
	for _, c := range telemetrytesting.CreateTelemetryAdapterConformance(func(context.Context) (telemetrytesting.TelemetryAdapterFixture, error) { return nil, expected }) {
		if err := c.Run(context.Background()); !errors.Is(err, expected) {
			t.Fatalf("%s: %v", c.Name(), err)
		}
	}
}

// Compile-time surface parity: the four exported symbols on Pi's telemetry
// ./testing export subpath each map to a Go declaration here. Checked
// line-for-line against the ./testing entries in parity/surface/symbols.jsonl.
var (
	_ telemetrytesting.TelemetryAdapterFixture             // TelemetryAdapterFixture
	_ telemetrytesting.TelemetryAdapterFixtureFactory      // TelemetryAdapterFixtureFactory
	_ telemetrytesting.TelemetryAdapterConformanceCase     // TelemetryAdapterConformanceCase
	_ = telemetrytesting.CreateTelemetryAdapterConformance // createTelemetryAdapterConformance
)

type brokenFixture struct {
	memoryFixture
	noRecording           bool
	readError, closeError error
	closed                *int
}

func (f *brokenFixture) Context() telemetry.TelemetryContext {
	if f.noRecording {
		return telemetry.NOOPTelemetryContext
	}
	return f.memoryFixture.Context()
}
func (f *brokenFixture) GetSpans(ctx context.Context) ([]telemetry.RecordedTelemetrySpan, error) {
	if f.readError != nil {
		return nil, f.readError
	}
	return f.memoryFixture.GetSpans(ctx)
}
func (f *brokenFixture) Close(context.Context) error { *f.closed++; return f.closeError }

func TestConformanceRejectsBrokenAdaptersAndClosesFixtures(t *stdtesting.T) {
	for _, mode := range []string{"noop", "snapshot failure", "close failure"} {
		t.Run(mode, func(t *stdtesting.T) {
			closed := 0
			readError, closeError := errors.New("snapshot failed"), errors.New("close failed")
			cases := telemetrytesting.CreateTelemetryAdapterConformance(func(context.Context) (telemetrytesting.TelemetryAdapterFixture, error) {
				f := &brokenFixture{closed: &closed}
				switch mode {
				case "noop":
					f.noRecording = true
				case "snapshot failure":
					f.readError = readError
					f.closeError = closeError
				case "close failure":
					f.closeError = closeError
				}
				return f, nil
			})
			for _, c := range cases {
				err := c.Run(context.Background())
				if err == nil {
					t.Errorf("%s accepted broken adapter", c.Name())
				}
				if mode == "snapshot failure" && (!errors.Is(err, readError) || !errors.Is(err, closeError)) {
					t.Errorf("%s lost fixture errors: %v", c.Name(), err)
				}
				if mode == "close failure" && !errors.Is(err, closeError) {
					t.Errorf("lost close failure: %v", err)
				}
			}
			if closed != 9 {
				t.Fatalf("closed %d fixtures", closed)
			}
		})
	}
}

type panicOverwriteContext struct{ telemetry.TelemetryContext }

func (c panicOverwriteContext) StartSpan(ctx context.Context, options telemetry.SpanOptions, fn telemetry.SpanFunc) (any, error) {
	return c.TelemetryContext.StartSpan(ctx, options, func(ctx context.Context, span telemetry.TelemetrySpan) (any, error) {
		defer func() {
			if failure := recover(); failure != nil {
				span.SetStatus(telemetry.SpanStatus{Status: "error"})
				panic(failure)
			}
		}()
		return fn(ctx, span)
	})
}

type panicOverwriteFixture struct{ memoryFixture }

func (f *panicOverwriteFixture) Context() telemetry.TelemetryContext {
	return panicOverwriteContext{f.memoryFixture.Context()}
}

func TestConformanceRejectsExplicitStatusOverwrittenByPanic(t *stdtesting.T) {
	cases := telemetrytesting.CreateTelemetryAdapterConformance(func(context.Context) (telemetrytesting.TelemetryAdapterFixture, error) {
		return &panicOverwriteFixture{}, nil
	})
	for _, c := range cases {
		if c.Group() == "status" {
			if err := c.Run(context.Background()); err == nil {
				t.Fatal("adapter overwrites explicit status on panic but passed conformance")
			}
			return
		}
	}
	t.Fatal("missing status case")
}

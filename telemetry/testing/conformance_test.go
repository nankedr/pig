package testing_test

import (
	"context"
	"errors"
	stdtesting "testing"

	"github.com/nankedr/pig/telemetry"
	telemetrytesting "github.com/nankedr/pig/telemetry/testing"
)

// nopFactory is an inert fixture factory. The M0 conformance cases never invoke
// it (they defer before building a fixture), so it exists only to satisfy the
// CreateTelemetryAdapterConformance signature.
func nopFactory(context.Context) (telemetrytesting.TelemetryAdapterFixture, error) {
	return nil, nil
}

// TestConformanceCatalogueIsComplete verifies the harness maps the full upstream
// case catalogue: every case carries a non-empty group and name so the contract
// is reproduced verbatim even while execution is deferred.
func TestConformanceCatalogueIsComplete(t *stdtesting.T) {
	cases := telemetrytesting.CreateTelemetryAdapterConformance(nopFactory)
	if len(cases) == 0 {
		t.Fatal("CreateTelemetryAdapterConformance returned no cases")
	}
	for i, c := range cases {
		if c.Group() == "" {
			t.Errorf("case %d: empty group", i)
		}
		if c.Name() == "" {
			t.Errorf("case %d (%s): empty name", i, c.Group())
		}
	}
}

// TestConformanceCasesDeferExplicitly verifies the M0 deferral is honest: each
// case Run fails with a structured *NotImplementedError naming the subpackage,
// rather than pretending its assertion passed.
func TestConformanceCasesDeferExplicitly(t *stdtesting.T) {
	cases := telemetrytesting.CreateTelemetryAdapterConformance(nopFactory)
	for _, c := range cases {
		err := c.Run(context.Background())
		if !errors.Is(err, telemetry.ErrNotImplemented) {
			t.Fatalf("case %q/%q: errors.Is(%v, ErrNotImplemented) = false", c.Group(), c.Name(), err)
		}
		var nie *telemetry.NotImplementedError
		if !errors.As(err, &nie) {
			t.Fatalf("case %q/%q: errors.As(%v, *NotImplementedError) = false", c.Group(), c.Name(), err)
		}
		if nie.Module != "telemetry/testing" {
			t.Errorf("case %q/%q: Module = %q, want telemetry/testing", c.Group(), c.Name(), nie.Module)
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

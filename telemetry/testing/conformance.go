// Package testing is the Go mapping of Pi's telemetry ./testing export
// subpath (packages/telemetry/src/testing, baseline 936aff0): the
// runner-independent conformance harness that verifies a telemetry adapter
// honours the callback span contract.
//
// The package name mirrors the upstream ./testing subpath for parity with the
// surface snapshot. It collides with the standard library testing package, so a
// consumer's *_test.go must import one of them under an alias; that cost is
// contained to test files.
//
// M0 scope (issue #23): the harness types and the full, faithfully named case
// catalogue are mapped so callers compile, but every case Run fails with a
// structured *NotImplementedError. The assertions depend on Pi's in-memory
// recording behaviour, which lands at M2; reproducing the case names without
// executing them keeps the surface complete without faking success.
package testing

import (
	"context"

	"github.com/nankedr/pig/telemetry"
)

// TelemetryAdapterFixture is a fresh adapter instance and snapshot reader owned
// by one conformance case. Pi's fixture is AsyncDisposable; Close maps that
// async disposal so a case can release the adapter when it finishes.
type TelemetryAdapterFixture interface {
	Context() telemetry.TelemetryContext
	GetSpans(ctx context.Context) ([]telemetry.RecordedTelemetrySpan, error)
	Close(ctx context.Context) error
}

// TelemetryAdapterFixtureFactory creates an isolated fixture for one case.
type TelemetryAdapterFixtureFactory func(ctx context.Context) (TelemetryAdapterFixture, error)

// TelemetryAdapterConformanceCase is a runner-independent conformance case that
// can be registered with any test framework.
type TelemetryAdapterConformanceCase interface {
	Group() string
	Name() string
	Run(ctx context.Context) error
}

// deferredCase is a mapped-but-unimplemented conformance case. It preserves the
// upstream group and name so the case catalogue is complete, while Run reports
// the M0 deferral explicitly.
type deferredCase struct {
	group string
	name  string
}

func (c deferredCase) Group() string { return c.group }
func (c deferredCase) Name() string  { return c.name }

// Run reports that executing the conformance case is not implemented at M0.
func (c deferredCase) Run(context.Context) error {
	return &telemetry.NotImplementedError{
		Module:    "telemetry/testing",
		Operation: "conformance case: " + c.group + ": " + c.name,
	}
}

// conformanceCatalogue is the faithful list of Pi's adapter conformance cases,
// in upstream order. The names are part of the contract, so they are mapped
// verbatim even though execution is deferred to M2.
var conformanceCatalogue = []deferredCase{
	{"callback lifecycle", "admits once synchronously and preserves the result"},
	{"callback lifecycle", "preserves synchronous and asynchronous rejection values"},
	{"status", "uses last explicit status without automatic overwrite"},
	{"recording", "merges attributes and records ordered events"},
	{"recording", "ignores failed attribute calls atomically"},
	{"recording", "makes calls after settlement inert"},
	{"parentage", "records nested and concurrent child relationships"},
	{"passivity", "suppresses unreadable telemetry payload failures"},
	{"passivity", "ignores failed status calls atomically"},
}

// CreateTelemetryAdapterConformance returns the runner-independent cases for the
// callback telemetry adapter contract. The factory is accepted for parity and
// will build fixtures once the cases execute at M2; at M0 every returned case
// fails with a structured *NotImplementedError instead of running assertions.
func CreateTelemetryAdapterConformance(factory TelemetryAdapterFixtureFactory) []TelemetryAdapterConformanceCase {
	_ = factory
	cases := make([]TelemetryAdapterConformanceCase, 0, len(conformanceCatalogue))
	for _, c := range conformanceCatalogue {
		cases = append(cases, c)
	}
	return cases
}

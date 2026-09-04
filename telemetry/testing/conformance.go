// Package testing is the Go mapping of Pi's telemetry ./testing export
// subpath (packages/telemetry/src/testing, baseline 936aff0): the
// runner-independent conformance harness that verifies a telemetry adapter
// honours the callback span contract.
//
// The package name mirrors the upstream ./testing subpath for parity with the
// surface snapshot. It collides with the standard library testing package, so a
// consumer's *_test.go must import one of them under an alias; that cost is
// contained to test files.
package testing

import (
	"context"
	"errors"
	"fmt"
	"reflect"

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

type conformanceCase struct {
	group, name string
	factory     TelemetryAdapterFixtureFactory
	test        func(context.Context, TelemetryAdapterFixture)
}

func (c conformanceCase) Group() string { return c.group }
func (c conformanceCase) Name() string  { return c.name }
func (c conformanceCase) Run(ctx context.Context) (err error) {
	defer func() {
		if failure := recover(); failure != nil {
			cause, ok := failure.(error)
			if !ok {
				cause = fmt.Errorf("adapter assertion or panic: %v", failure)
			}
			err = errors.Join(err, cause)
		}
		if err != nil {
			err = fmt.Errorf("%s: %s: %w", c.group, c.name, err)
		}
	}()
	fixture, err := c.factory(ctx)
	if err != nil {
		return err
	}
	require(fixture != nil, "factory returned nil fixture")
	defer func() { err = errors.Join(err, fixture.Close(context.WithoutCancel(ctx))) }()
	c.test(ctx, fixture)
	return nil
}

// CreateTelemetryAdapterConformance returns all nine fixed-Pi cases. Each Run
// creates and closes its own fixture and reports assertion failures as errors.
// Go invalid dynamic attributes/statuses replace unreadable JavaScript proxies.
func CreateTelemetryAdapterConformance(factory TelemetryAdapterFixtureFactory) []TelemetryAdapterConformanceCase {
	cases := []conformanceCase{
		{group: "callback lifecycle", name: "admits once synchronously and preserves the result", test: callbackSuccess},
		{group: "callback lifecycle", name: "preserves synchronous and asynchronous rejection values", test: callbackFailures},
		{group: "status", name: "uses last explicit status without automatic overwrite", test: explicitStatus},
		{group: "recording", name: "merges attributes and records ordered events", test: recording},
		{group: "recording", name: "ignores failed attribute calls atomically", test: atomicAttributes},
		{group: "recording", name: "makes calls after settlement inert", test: settledCalls},
		{group: "parentage", name: "records nested and concurrent child relationships", test: parentage},
		{group: "passivity", name: "suppresses unreadable telemetry payload failures", test: passivity},
		{group: "passivity", name: "ignores failed status calls atomically", test: atomicStatus},
	}
	result := make([]TelemetryAdapterConformanceCase, len(cases))
	for i, c := range cases {
		c.factory = factory
		result[i] = c
	}
	return result
}

func require(ok bool, message string) {
	if !ok {
		panic(message)
	}
}
func equal(actual, expected any) {
	require(reflect.DeepEqual(actual, expected), fmt.Sprintf("got %#v; want %#v", actual, expected))
}
func readSpans(ctx context.Context, f TelemetryAdapterFixture) []telemetry.RecordedTelemetrySpan {
	spans, err := f.GetSpans(ctx)
	if err != nil {
		panic(err)
	}
	return spans
}
func findSpan(ctx context.Context, f TelemetryAdapterFixture, name string) telemetry.RecordedTelemetrySpan {
	for _, span := range readSpans(ctx, f) {
		if span.Name == name {
			return span
		}
	}
	panic("missing recorded span " + name)
}
func callbackSuccess(ctx context.Context, f TelemetryAdapterFixture) {
	calls := 0
	expected := &struct{ Value int }{42}
	got, err := f.Context().StartSpan(ctx, telemetry.SpanOptions{Name: "success"}, func(callbackCtx context.Context, _ telemetry.TelemetrySpan) (any, error) {
		require(callbackCtx == ctx, "callback context changed")
		calls++
		return expected, nil
	})
	require(got == expected && err == nil && calls == 1, "callback result or admission changed")
	span := findSpan(ctx, f, "success")
	equal(span.Status, telemetry.SpanStatus{Status: "ok"})
	require(span.Settled && span.EndSequence != nil, "success span did not settle")
}

type unreadableError struct{}

func (unreadableError) Error() string { panic("unreadable error") }

func callbackFailures(ctx context.Context, f TelemetryAdapterFixture) {
	for _, failure := range []struct {
		name          string
		value         any
		panics, async bool
	}{
		{"sync-error", errors.New("sync"), true, false},
		{"async-error", errors.New("async"), false, true},
		{"value-error", &struct{ Kind string }{"rejected"}, true, false},
		{"unreadable-error", &unreadableError{}, true, false},
		{"async-unreadable-error", &unreadableError{}, false, true},
	} {
		func() {
			if failure.panics {
				defer func() { require(recover() == failure.value, "panic value changed") }()
			}
			got, err := f.Context().StartSpan(ctx, telemetry.SpanOptions{Name: failure.name}, func(context.Context, telemetry.TelemetrySpan) (any, error) {
				if failure.panics {
					panic(failure.value)
				}
				if failure.async {
					result := make(chan error, 1)
					go func() { result <- failure.value.(error) }()
					return 42, <-result
				}
				return 42, failure.value.(error)
			})
			require(!failure.panics && got == 42 && err == failure.value, "callback failure changed")
		}()
		span := findSpan(ctx, f, failure.name)
		require(span.Status.Status == "error" && span.Settled && span.EndSequence != nil, "failure span did not settle as error")
	}
}

func explicitStatus(ctx context.Context, f TelemetryAdapterFixture) {
	expectedError := telemetry.SpanStatus{Status: "error", Error: &telemetry.SpanError{Name: "Expected", Message: "recorded"}}
	for _, tc := range []struct {
		name    string
		status  telemetry.SpanStatus
		failure error
	}{
		{"last-status", telemetry.SpanStatus{Status: "ok"}, nil},
		{"explicit-before-throw", telemetry.SpanStatus{Status: "ok"}, errors.New("failure")},
		{"explicit-before-rejection", expectedError, errors.New("failure")},
		{"expected-failure", expectedError, nil},
	} {
		func() {
			if tc.name == "explicit-before-throw" {
				defer func() { require(recover() == tc.failure, "explicit status changed panic value") }()
			}
			got, err := f.Context().StartSpan(ctx, telemetry.SpanOptions{Name: tc.name}, func(_ context.Context, span telemetry.TelemetrySpan) (any, error) {
				span.SetStatus(telemetry.SpanStatus{Status: "error"})
				span.SetStatus(tc.status)
				if tc.name == "explicit-before-throw" {
					panic(tc.failure)
				}
				return 7, tc.failure
			})
			require(tc.name != "explicit-before-throw" && got == 7 && err == tc.failure, "explicit status changed callback result")
		}()
		equal(findSpan(ctx, f, tc.name).Status, tc.status)
	}
}

func recording(ctx context.Context, f TelemetryAdapterFixture) {
	_, err := f.Context().StartSpan(ctx, telemetry.SpanOptions{Name: "recording", Attributes: telemetry.SpanAttributes{"start": "value", "overwrite": "start", "ignored": nil}}, func(_ context.Context, span telemetry.TelemetrySpan) (any, error) {
		span.SetAttributes(telemetry.SpanAttributes{"count": 1, "overwrite": "middle"})
		span.SetAttributes(telemetry.SpanAttributes{"count": nil, "overwrite": "end"})
		span.AddEvent("first", telemetry.SpanAttributes{"index": 1, "ignored": nil})
		span.AddEvent("second", telemetry.SpanAttributes{"index": 2})
		return nil, nil
	})
	require(err == nil, "recording changed callback result")
	span := findSpan(ctx, f, "recording")
	equal(span.Attributes, telemetry.SpanAttributes{"start": "value", "overwrite": "end", "count": 1})
	equal(span.Events, []telemetry.RecordedTelemetryEvent{{Name: "first", Attributes: telemetry.SpanAttributes{"index": 1}}, {Name: "second", Attributes: telemetry.SpanAttributes{"index": 2}}})
}

func invalidAttributes() telemetry.SpanAttributes {
	return telemetry.SpanAttributes{"partial": "must not survive", "unreadable": func() { panic("read") }}
}
func atomicAttributes(ctx context.Context, f TelemetryAdapterFixture) {
	_, err := f.Context().StartSpan(ctx, telemetry.SpanOptions{Name: "atomic-attributes", Attributes: telemetry.SpanAttributes{"retained": "value"}}, func(_ context.Context, span telemetry.TelemetrySpan) (any, error) {
		span.SetAttributes(invalidAttributes())
		return nil, nil
	})
	require(err == nil, "invalid attributes changed callback result")
	equal(findSpan(ctx, f, "atomic-attributes").Attributes, telemetry.SpanAttributes{"retained": "value"})
}

func settledCalls(ctx context.Context, f TelemetryAdapterFixture) {
	var captured telemetry.TelemetrySpan
	_, err := f.Context().StartSpan(ctx, telemetry.SpanOptions{Name: "settled", Attributes: telemetry.SpanAttributes{"value": "initial"}}, func(_ context.Context, span telemetry.TelemetrySpan) (any, error) { captured = span; return nil, nil })
	require(err == nil && captured != nil, "missing callback span")
	captured.SetAttributes(telemetry.SpanAttributes{"value": "late"})
	captured.AddEvent("late", telemetry.SpanAttributes{"value": true})
	captured.SetStatus(telemetry.SpanStatus{Status: "error"})
	calls := 0
	result, err := captured.StartSpan(ctx, telemetry.SpanOptions{Name: "late-child"}, func(context.Context, telemetry.TelemetrySpan) (any, error) { calls++; return 7, nil })
	require(result == 7 && err == nil && calls == 1, "late child callback changed")
	spans := readSpans(ctx, f)
	equal(len(spans), 1)
	equal(spans[0].Attributes, telemetry.SpanAttributes{"value": "initial"})
	require(len(spans[0].Events) == 0, "late event recorded")
	equal(spans[0].Status, telemetry.SpanStatus{Status: "ok"})
}

func parentage(ctx context.Context, f TelemetryAdapterFixture) {
	_, err := f.Context().StartSpan(ctx, telemetry.SpanOptions{Name: "parent"}, func(ctx context.Context, parent telemetry.TelemetrySpan) (any, error) {
		started, release := make(chan struct{}), make(chan struct{})
		done := make(chan error, 1)
		go func() {
			defer func() {
				if failure := recover(); failure != nil {
					done <- fmt.Errorf("child panic: %v", failure)
				}
			}()
			_, err := parent.StartSpan(ctx, telemetry.SpanOptions{Name: "first-child"}, func(ctx context.Context, _ telemetry.TelemetrySpan) (any, error) {
				close(started)
				select {
				case <-release:
					return nil, nil
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			})
			done <- err
		}()
		func() {
			defer close(release)
			select {
			case <-started:
			case err := <-done:
				panic(fmt.Sprintf("first child was not admitted: %v", err))
			case <-ctx.Done():
				panic(ctx.Err())
			}
			result, err := parent.StartSpan(ctx, telemetry.SpanOptions{Name: "second-child"}, func(context.Context, telemetry.TelemetrySpan) (any, error) { return "done", nil })
			require(result == "done" && err == nil, "second child result changed")
		}()
		return nil, <-done
	})
	require(err == nil, "parent callback failed")
	spans := readSpans(ctx, f)
	equal(len(spans), 3)
	parent, first, second := spans[0], spans[1], spans[2]
	require(parent.Name == "parent" && first.Name == "first-child" && second.Name == "second-child", "spans not in start order")
	require(parent.ParentID == nil && first.ParentID != nil && second.ParentID != nil && *first.ParentID == parent.ID && *second.ParentID == parent.ID, "incorrect parentage")
	require(parent.ID != first.ID && parent.ID != second.ID && first.ID != second.ID, "duplicate span IDs")
	require(parent.Settled && first.Settled && second.Settled && parent.EndSequence != nil && first.EndSequence != nil && second.EndSequence != nil, "unsettled spans")
	require(*second.EndSequence < *first.EndSequence && *first.EndSequence < *parent.EndSequence, "incorrect completion order")
}

func passivity(ctx context.Context, f TelemetryAdapterFixture) {
	calls := 0
	got, err := f.Context().StartSpan(ctx, telemetry.SpanOptions{Name: "unreadable-options", Attributes: invalidAttributes()}, func(context.Context, telemetry.TelemetrySpan) (any, error) { calls++; return 9, nil })
	require(got == 9 && err == nil && calls == 1, "invalid options changed callback")
	equal(len(readSpans(ctx, f)), 0)
	_, err = f.Context().StartSpan(ctx, telemetry.SpanOptions{Name: "unreadable-recording"}, func(_ context.Context, span telemetry.TelemetrySpan) (any, error) {
		span.SetAttributes(invalidAttributes())
		span.AddEvent("unreadable-event", invalidAttributes())
		span.SetStatus(telemetry.SpanStatus{Status: "invalid"})
		return nil, nil
	})
	require(err == nil, "invalid payload changed callback")
	spans := readSpans(ctx, f)
	equal(len(spans), 1)
	require(len(spans[0].Attributes) == 0 && len(spans[0].Events) == 0, "partial record survived")
	equal(spans[0].Status, telemetry.SpanStatus{Status: "ok"})
}

func atomicStatus(ctx context.Context, f TelemetryAdapterFixture) {
	expected := errors.New("rejected after unreadable status")
	_, err := f.Context().StartSpan(ctx, telemetry.SpanOptions{Name: "unreadable-status"}, func(_ context.Context, span telemetry.TelemetrySpan) (any, error) {
		span.SetStatus(telemetry.SpanStatus{Status: "invalid", Error: &telemetry.SpanError{Name: "partial"}})
		return nil, expected
	})
	require(err == expected, "invalid status changed callback error")
	equal(findSpan(ctx, f, "unreadable-status").Status, telemetry.SpanStatus{Status: "error", Error: &telemetry.SpanError{Name: "Error", Message: expected.Error()}})
}

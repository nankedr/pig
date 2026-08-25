package testing_test

import (
	"context"
	"errors"
	stdtesting "testing"

	"github.com/nankedr/pig/agent"
	sessiontesting "github.com/nankedr/pig/agent/session/testing"
)

func TestSessionBackendConformanceCatalogueDefersExplicitly(t *stdtesting.T) {
	called := false
	cases := sessiontesting.CreateSessionBackendConformance(func(context.Context) (sessiontesting.SessionBackendFixture, error) {
		called = true
		return nil, nil
	})
	if len(cases) != 30 {
		t.Fatalf("case count = %d, want 30", len(cases))
	}
	for _, testCase := range cases {
		if testCase.Group() == "" || testCase.Name() == "" {
			t.Fatalf("empty case identity: %q/%q", testCase.Group(), testCase.Name())
		}
		if err := testCase.Run(context.Background()); !errors.Is(err, agent.ErrNotImplemented) {
			t.Fatalf("case %q/%q: Run() = %v", testCase.Group(), testCase.Name(), err)
		}
	}
	if called {
		t.Fatal("deferred conformance cases invoked the fixture factory")
	}
}

var (
	_ sessiontesting.SessionBackendFixture
	_ sessiontesting.SessionBackendFixtureFactory
	_ sessiontesting.SessionBackendConformanceCase
	_ = sessiontesting.CreateSessionBackendConformance
)

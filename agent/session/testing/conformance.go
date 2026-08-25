package testing

import (
	"context"

	"github.com/nankedr/pig/agent"
)

type SessionBackendFixture interface {
	Repository() agent.SessionRepo
	Close(context.Context) error
}

type SessionBackendFixtureFactory func(context.Context) (SessionBackendFixture, error)

type SessionBackendConformanceCase interface {
	Group() string
	Name() string
	Run(context.Context) error
}

type deferredCase struct {
	group string
	name  string
}

func (c deferredCase) Group() string { return c.group }
func (c deferredCase) Name() string  { return c.name }

func (c deferredCase) Run(context.Context) error {
	return &agent.NotImplementedError{
		Module:    "agent/session/testing",
		Operation: "conformance case: " + c.group + ": " + c.name,
	}
}

var conformanceCatalogue = []deferredCase{
	{"entries and lanes", "assigns parents and one sequence across every mutation"},
	{"records and log", "commits records and lane moves as separate mutations"},
	{"entries and lanes", "rejects duplicate ids without changing state"},
	{"entries and lanes", "isolates lanes while sharing the tree"},
	{"queries and facts", "rejects invalid queries before empty reads"},
	{"queries and facts", "supports bounded filtered and cursor-based queries"},
	{"records and log", "keeps lane names permanent with their recovery records"},
	{"records and log", "persists queue cancellation without consuming its target"},
	{"records and log", "filters records by lane type run sequence and order"},
	{"records and log", "filters operation starts by operation kind"},
	{"records and log", "tracks and enforces one open operation per lane"},
	{"records and log", "does not let an earlier finish close a later start"},
	{"records and log", "scopes open operations by lane and limit"},
	{"validation and immutability", "returns immutable open-operation records"},
	{"queries and facts", "keeps latest-value facts and computes ledger statistics across lanes"},
	{"queries and facts", "clears session names durably"},
	{"validation and immutability", "returns immutable copies from reads"},
	{"entries and lanes", "validates lane lifecycle and targets"},
	{"entries and lanes", "binds lane views without caching leaves"},
	{"entries and lanes", "appends provisioned entries with their existing ids"},
	{"entries and lanes", "persists tool-result termination decisions"},
	{"validation and immutability", "rejects non-JSON entries before storage mutation"},
	{"validation and immutability", "rejects non-JSON records before storage mutation"},
	{"entries and lanes", "linearizes concurrent writes across two lanes"},
	{"repository and forks", "creates lists and opens sessions"},
	{"repository and forks", "deletes sessions idempotently"},
	{"repository and forks", "forks one branch with selected facts and no records"},
	{"repository and forks", "forks a complete tree with lanes and facts"},
	{"repository and forks", "forks before an entry without modifying the source"},
	{"repository and forks", "validates the default fork target"},
}

func CreateSessionBackendConformance(factory SessionBackendFixtureFactory) []SessionBackendConformanceCase {
	_ = factory
	cases := make([]SessionBackendConformanceCase, len(conformanceCatalogue))
	for index, item := range conformanceCatalogue {
		cases[index] = item
	}
	return cases
}

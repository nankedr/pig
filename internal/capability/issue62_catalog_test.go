package capability_test

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/nankedr/pig/internal/catalog"
)

var updateIssue62Catalog = flag.Bool("update-issue62-catalog", false, "regenerate issue #62 deferred lifecycle evidence")

func TestIssue62CatalogRecordsDeferredLifecycle(t *testing.T) {
	root := issue56RepoRoot(t)
	path := filepath.Join(root, "parity", "catalog.jsonl")
	entries, err := catalog.LoadCatalog(path)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "parity", "oracle", "fixtures", "deferred-lifecycle.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture issue61Fixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	if !fixture.Deterministic || fixture.BaselineCommit != issue56BaselineCommit || fixture.Case.ID != "go-sdk/ai/deferred-lifecycle" || fixture.Case.CatalogID != "contract:ai/faux-provider" {
		t.Fatalf("deferred fixture provenance = %#v", fixture)
	}
	want := append([]catalog.Entry(nil), entries...)
	found := 0
	for i := range want {
		entry := &want[i]
		if !slices.Contains([]string{"contract:ai/faux-provider", "contract:ai/provider", "contract:ai/models-runtime", "contract:ai/event-stream"}, entry.ID) {
			continue
		}
		found++
		entry.Status = catalog.StatusPartial
		partial := catalog.Partial{Unsupported: []string{"remaining Provider adapter and model-discovery parity retain their own Catalog milestones"}}
		if entry.Partial != nil {
			partial = *entry.Partial
		}
		partial.Unsupported = slices.DeleteFunc(slices.Clone(partial.Unsupported), func(value string) bool {
			return value == "deferred submission, polling and deferred-handle cancellation remain structured ErrNotImplemented branches for M10" || value == "deferred event branches remain outside the Issue #61 case"
		})
		const supported = "M2.3 deferred submission, configurable pending polls, stable final/error reads and cancellation execute through public Faux, Provider and Models APIs"
		if !slices.Contains(partial.Supported, supported) {
			partial.Supported = append(slices.Clone(partial.Supported), supported)
		}
		entry.Partial = &partial
		entry.Notes = "Issue #62 adds fixed-Pi deferred lifecycle evidence and Go concurrency/request propagation coverage. ADR-0015 records stricter handle validation, immutable submission snapshots and single-resolution cancellation ordering. The broader contract remains partial."
		entry.Evidence = issue61UpsertEvidence(entry.Evidence, catalog.Evidence{
			Kind: "fixture", Ref: "parity/oracle/fixtures/deferred-lifecycle.json", Baseline: fixture.BaselineCommit,
			CaseID: fixture.Case.ID, InputHash: fixture.InputHash, CatalogID: entry.ID, Platform: "any",
			ExecutionMethod: "node --experimental-strip-types parity/oracle/deferred-lifecycle.mjs .upstream/pi --check",
			Expected:        "fixed Pi returns deferred handles, two pending polls, stable final/error results and compatible invalid-fetch/cancelled-fetch errors",
			Actual:          "PASS; fixed Pi observation reproduced at " + fixture.ObservationHash,
		})
		entry.Evidence = issue61UpsertEvidence(entry.Evidence, catalog.Evidence{
			Kind: "go-test", Ref: "internal/parity/deferred_lifecycle_test.go#TestDeferredLifecycleParity", Baseline: fixture.BaselineCommit,
			CaseID: "go-sdk/ai/deferred-lifecycle-go", InputHash: fixture.InputHash, CatalogID: entry.ID, Platform: "any",
			ExecutionMethod: "go test -race ./internal/parity -run '^TestDeferredLifecycleParity$' -count=1",
			Expected:        "public Provider and Models calls match the fixed Pi case under its declared projection",
			Actual:          "PASS; events, outcomes, handle metadata, hooks and queue/factory counters matched without runner normalization",
		})
		entry.Evidence = issue61UpsertEvidence(entry.Evidence, catalog.Evidence{
			Kind: "go-test", Ref: "ai/issue62_deferred_test.go", Baseline: fixture.BaselineCommit,
			CaseID: "issue62-deferred-concurrency", InputHash: fixture.InputHash, CatalogID: entry.ID, Platform: "any",
			ExecutionMethod: "go test -race ./ai -run '^TestDeferred' -count=1",
			Expected:        "single resolution, request-local and handle cancellation, immutable snapshots, terminal ordering, auth/transforms/telemetry/hooks and genuine capability detection",
			Actual:          "PASS; deterministic channel barriers and public SDK assertions cover the ADR-0015 deviations and request semantics",
		})
	}
	if found != 4 {
		t.Fatalf("deferred Catalog rows = %d", found)
	}
	if *updateIssue62Catalog {
		encoded, err := catalog.EncodeEntries(want)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, encoded, 0o644); err != nil {
			t.Fatal(err)
		}
	} else if !reflect.DeepEqual(entries, want) {
		t.Fatal("deferred evidence drifted; regenerate with -update-issue62-catalog")
	}
}

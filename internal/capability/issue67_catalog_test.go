package capability_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/nankedr/pig/internal/baseline"
	"github.com/nankedr/pig/internal/catalog"
	"github.com/nankedr/pig/internal/parity"
)

func TestIssue67CatalogBindsContextOverflowEvidence(t *testing.T) {
	root := issue56RepoRoot(t)
	lock, _, err := baseline.Load(filepath.Join(root, "parity", "baseline"))
	if err != nil {
		t.Fatal(err)
	}
	fixture, err := parity.LoadFixture(filepath.Join(root, "parity", "oracle", "fixtures", "context-overflow.json"), parity.Baseline{ID: lock.BaselineID, Commit: lock.Upstream.Commit, Repository: lock.Upstream.Repository})
	if err != nil {
		t.Fatal(err)
	}
	entries, err := catalog.LoadCatalog(filepath.Join(root, "parity", "catalog.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	covered := 0
	for _, entry := range entries {
		if entry.ID != "contract:ai/context-overflow" && !strings.HasPrefix(entry.ID, "symbol:ai/src/utils/overflow.ts#") {
			continue
		}
		covered++
		if entry.Status != catalog.StatusVerified || entry.Milestone != "M2" {
			t.Fatalf("context overflow status = %+v", entry)
		}
		fixedPi, goReplay := false, false
		for _, evidence := range entry.Evidence {
			if evidence.InputHash != fixture.InputHash || evidence.Baseline != lock.Upstream.Commit || evidence.CatalogID != entry.ID {
				continue
			}
			fixedPi = fixedPi || evidence.Kind == "fixture" && evidence.Ref == "parity/oracle/fixtures/context-overflow.json" && strings.Contains(evidence.Actual, fixture.ObservationHash)
			goReplay = goReplay || evidence.Kind == "go-test" && evidence.Ref == "internal/parity/context_overflow_test.go#TestContextOverflowParity"
		}
		if !fixedPi || !goReplay {
			t.Fatalf("%s missing fixture-bound evidence", entry.ID)
		}
	}
	if covered != 4 {
		t.Fatalf("covered entries = %d, want context contract and three overflow symbols", covered)
	}
}

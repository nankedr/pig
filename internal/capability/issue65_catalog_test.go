package capability_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/nankedr/pig/internal/baseline"
	"github.com/nankedr/pig/internal/catalog"
	"github.com/nankedr/pig/internal/parity"
)

func TestIssue65CatalogBindsCompatEvidenceAndKeepsDeferredBranches(t *testing.T) {
	root := issue56RepoRoot(t)
	lock, _, err := baseline.Load(filepath.Join(root, "parity", "baseline"))
	if err != nil {
		t.Fatal(err)
	}
	fixture, err := parity.LoadFixture(filepath.Join(root, "parity", "oracle", "fixtures", "compat-session-resources.json"), parity.Baseline{ID: lock.BaselineID, Commit: lock.Upstream.Commit, Repository: lock.Upstream.Repository})
	if err != nil {
		t.Fatal(err)
	}
	entries, err := catalog.LoadCatalog(filepath.Join(root, "parity", "catalog.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	aliases, resources, registry, deferred := 0, 0, 0, 0
	for _, entry := range entries {
		covered := false
		switch {
		case strings.HasPrefix(entry.ID, "symbol:ai/src/legacy-api-aliases.ts#"):
			aliases++
			covered = true
			if entry.Status != catalog.StatusPartial || entry.Deviation == nil {
				t.Fatalf("alias boundary = %+v", entry)
			}
		case strings.HasPrefix(entry.ID, "symbol:ai/src/session-resources.ts#"):
			resources++
			covered = true
			if entry.Status != catalog.StatusImplemented || entry.Deviation == nil {
				t.Fatalf("cleanup boundary = %+v", entry)
			}
		case strings.HasPrefix(entry.ID, "symbol:ai/src/compat.ts#") && entry.Milestone == "M2":
			registry++
			covered = true
		case entry.ID == "contract:ai/compat":
			covered = true
			if entry.Status != catalog.StatusPartial || entry.Partial == nil || entry.Deviation == nil {
				t.Fatalf("compat boundary = %+v", entry)
			}
		case entry.ID == "contract:ai/auth-entrypoints" || entry.ID == "contract:ai/api-entrypoints" || entry.ID == "contract:ai/images":
			deferred++
			if entry.Status != catalog.StatusPartial || entry.Partial == nil || len(entry.Partial.Unsupported) == 0 {
				t.Fatalf("deferred boundary = %+v", entry)
			}
		}
		if !covered {
			continue
		}
		fixedPi, goReplay := false, false
		for _, evidence := range entry.Evidence {
			if evidence.InputHash != fixture.InputHash || evidence.Baseline != lock.Upstream.Commit || evidence.CatalogID != entry.ID {
				continue
			}
			fixedPi = fixedPi || evidence.Kind == "fixture" && evidence.Ref == "parity/oracle/fixtures/compat-session-resources.json" && strings.Contains(evidence.Actual, fixture.ObservationHash)
			goReplay = goReplay || evidence.Kind == "go-test" && evidence.Ref == "internal/parity/compat_session_resources_test.go#TestCompatSessionResourcesParity"
		}
		if !fixedPi || !goReplay {
			t.Fatalf("%s missing fixture-bound evidence", entry.ID)
		}
	}
	if aliases != 16 || resources != 3 || registry != 14 || deferred != 3 {
		t.Fatalf("coverage = aliases:%d cleanup:%d compat:%d deferred:%d", aliases, resources, registry, deferred)
	}
}

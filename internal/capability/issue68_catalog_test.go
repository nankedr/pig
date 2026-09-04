package capability_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/nankedr/pig/internal/baseline"
	"github.com/nankedr/pig/internal/catalog"
	"github.com/nankedr/pig/internal/parity"
)

func TestIssue68CatalogBindsQueueEvidence(t *testing.T) {
	root := issue56RepoRoot(t)
	lock, _, err := baseline.Load(filepath.Join(root, "parity/baseline"))
	if err != nil {
		t.Fatal(err)
	}
	fixture, err := parity.LoadFixture(filepath.Join(root, "parity/oracle/fixtures/legacy-agent-queues.json"), parity.Baseline{ID: lock.BaselineID, Commit: lock.Upstream.Commit, Repository: lock.Upstream.Repository})
	if err != nil {
		t.Fatal(err)
	}
	entries, err := catalog.LoadCatalog(filepath.Join(root, "parity/catalog.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	covered := 0
	for _, entry := range entries {
		if entry.ID != "contract:agent/legacy-queues" && entry.ID != "contract:agent/runtime" && entry.ID != "contract:agent/state-and-queues" && !(entry.Milestone == "M2" && (strings.HasPrefix(entry.ID, "member:agent/src/agent.ts#") || strings.HasPrefix(entry.ID, "member:agent/src/types.ts#AgentLoopConfig.get"))) {
			continue
		}
		covered++
		if entry.Deviation == nil || entry.Deviation.ADR != "docs/adr/0018-legacy-agent-queue-admission.md" {
			t.Fatalf("missing deviation: %s", entry.ID)
		}
		if entry.ID == "contract:agent/runtime" || entry.ID == "contract:agent/state-and-queues" {
			if entry.Status != catalog.StatusPartial || entry.Partial == nil || !strings.Contains(strings.Join(entry.Partial.Unsupported, " "), "PrepareNextTurn") {
				t.Fatalf("deferred boundary: %+v", entry)
			}
		} else if entry.Status != catalog.StatusImplemented {
			t.Fatalf("queue status: %+v", entry)
		}
		oracle, replay, deviation := false, false, false
		for _, evidence := range entry.Evidence {
			if evidence.Baseline != lock.Upstream.Commit || evidence.CatalogID != entry.ID {
				continue
			}
			if evidence.InputHash == fixture.InputHash {
				oracle = oracle || evidence.Kind == "fixture" && strings.Contains(evidence.Actual, fixture.ObservationHash)
				replay = replay || evidence.Ref == "internal/parity/legacy_agent_queues_test.go#TestLegacyAgentQueuesParity"
			}
			deviation = deviation || evidence.Ref == "parity/oracle/fixtures/legacy-agent-queues-deviation.json"
		}
		if !oracle || !replay || !deviation {
			t.Fatalf("missing queue evidence: %s", entry.ID)
		}
	}
	if covered != 20 {
		t.Fatalf("queue entries = %d, want 20", covered)
	}
}

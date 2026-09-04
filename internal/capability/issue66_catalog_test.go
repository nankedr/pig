package capability_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/nankedr/pig/internal/baseline"
	"github.com/nankedr/pig/internal/catalog"
	"github.com/nankedr/pig/internal/parity"
)

func TestIssue66CatalogBindsHandoffEvidenceAndKeepsM12Boundary(t *testing.T) {
	root := issue56RepoRoot(t)
	lock, _, err := baseline.Load(filepath.Join(root, "parity", "baseline"))
	if err != nil {
		t.Fatal(err)
	}
	fixture, err := parity.LoadFixture(filepath.Join(root, "parity", "oracle", "fixtures", "message-handoff.json"), parity.Baseline{ID: lock.BaselineID, Commit: lock.Upstream.Commit, Repository: lock.Upstream.Repository})
	if err != nil {
		t.Fatal(err)
	}
	entries, err := catalog.LoadCatalog(filepath.Join(root, "parity", "catalog.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, entry := range entries {
		if entry.ID != "contract:ai/message-handoff" && entry.ID != "symbol:ai/src/api/transform-messages.ts#transformMessages" {
			continue
		}
		found++
		if entry.Status != catalog.StatusPartial || entry.Milestone != "M2" || entry.Mapping.Target != "github.com/nankedr/pig/ai.TransformMessages" || entry.Partial == nil || !strings.Contains(strings.Join(entry.Partial.Unsupported, " "), "M12") {
			t.Fatalf("handoff boundary = %+v", entry)
		}
		fixedPi, replay, agent := false, false, false
		for _, evidence := range entry.Evidence {
			if evidence.CatalogID != entry.ID || evidence.InputHash != fixture.InputHash || evidence.Baseline != lock.Upstream.Commit {
				continue
			}
			assertIssue33EvidenceRefPath(t, root, evidence.Ref)
			if evidence.CaseID == "" || evidence.ExecutionMethod == "" || evidence.Expected == "" || evidence.Actual == "" || evidence.Platform == "" {
				t.Fatalf("incomplete evidence: %+v", evidence)
			}
			fixedPi = fixedPi || evidence.Ref == "parity/oracle/fixtures/message-handoff.json" && strings.Contains(evidence.Actual, fixture.ObservationHash)
			replay = replay || evidence.Ref == "internal/parity/message_handoff_test.go#TestMessageHandoffParity"
			agent = agent || evidence.Ref == "agent/issue66_handoff_test.go#TestAgentModelHandoffReplaysHistoryWithoutMutatingIt"
		}
		if !fixedPi || !replay || !agent {
			t.Fatalf("%s lacks fixture-bound public SDK evidence", entry.ID)
		}
	}
	if found != 2 {
		t.Fatalf("handoff catalog rows = %d, want 2", found)
	}
}

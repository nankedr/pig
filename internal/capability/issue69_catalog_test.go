package capability_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/nankedr/pig/internal/baseline"
	"github.com/nankedr/pig/internal/catalog"
	"github.com/nankedr/pig/internal/parity"
)

func TestIssue69CatalogBindsProxyEvidence(t *testing.T) {
	root := issue56RepoRoot(t)
	lock, _, err := baseline.Load(filepath.Join(root, "parity/baseline"))
	if err != nil {
		t.Fatal(err)
	}
	fixture, err := parity.LoadFixture(filepath.Join(root, "parity/oracle/fixtures/agent-proxy.json"), parity.Baseline{ID: lock.BaselineID, Commit: lock.Upstream.Commit, Repository: lock.Upstream.Repository})
	if err != nil {
		t.Fatal(err)
	}
	entries, err := catalog.LoadCatalog(filepath.Join(root, "parity/catalog.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, entry := range entries {
		if entry.ID != "contract:agent/proxy" && !strings.HasPrefix(entry.ID, "member:agent/src/proxy.ts#") && !strings.HasPrefix(entry.ID, "symbol:agent/src/proxy.ts#") {
			continue
		}
		count++
		if entry.Status != catalog.StatusImplemented || entry.Partial != nil || entry.Deviation == nil || entry.Deviation.ADR != "docs/adr/0019-proxy-stream-reconstruction.md" {
			t.Fatalf("proxy status=%+v", entry)
		}
		oracle, replay, sdk := false, false, false
		for _, e := range entry.Evidence {
			if e.CatalogID != entry.ID || e.Baseline != lock.Upstream.Commit || e.InputHash != fixture.InputHash {
				continue
			}
			oracle = oracle || e.Kind == "fixture" && e.Ref == "parity/oracle/fixtures/agent-proxy.json" && strings.Contains(e.Actual, fixture.ObservationHash)
			replay = replay || e.Ref == "internal/parity/agent_proxy_test.go#TestAgentProxyParity" && strings.Contains(e.Actual, fixture.ObservationHash)
			sdk = sdk || e.Ref == "agent/proxy_runtime_test.go" && strings.Contains(e.ExecutionMethod, "-race")
		}
		if !oracle || !replay || !sdk {
			t.Fatalf("missing evidence for %s", entry.ID)
		}
	}
	if count != 19 {
		t.Fatalf("proxy entries=%d, want 19", count)
	}
}

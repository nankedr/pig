package codingagent_test

import (
	"github.com/nankedr/pig/internal/catalog"
	"testing"
)

func issue90CompactionEvidence(t *testing.T) []issue32ModuleEvidenceDescriptor {
	t.Helper()
	var out []issue32ModuleEvidenceDescriptor
	for _, e := range []struct{ kind, path, run string }{
		{"oracle", "parity/oracle/fixtures/auto-compaction.json", "node --experimental-strip-types parity/oracle/auto-compaction.mjs <locked-pi-checkout> --check"},
		{"go-test", "codingagent/issue90_compaction_test.go", "go test ./codingagent -run '^TestAutoCompactionParity$' -count=1"},
		{"go-test", "codingagent/issue90_runtime_test.go", "go test -race ./codingagent -run '^TestAutoCompaction' -count=1"},
	} {
		out = append(out, issue32ModuleEvidenceDescriptor{InputPath: e.path, Evidence: catalog.Evidence{Kind: e.kind, Ref: e.path, Baseline: issue32BaselineCommit, CaseID: "issue90-" + e.path, ExecutionMethod: e.run, Expected: "fixed Pi public SDK automatic threshold/overflow compaction, bounded recovery, retry ordering, queue modes and stale usage boundaries; Headless cancellation and v3 cross-process continuation", Actual: "PASS; source/dist Oracle and SDK parity, Headless terminal outcome, atomic failure and cancellation, settings persistence and cross-process reopen", Platform: "any", CatalogID: issue32CompactionCatalogID}})
	}
	return out
}
func issue90PromoteRuntimeEntry(entry *catalog.Entry) bool {
	switch entry.ID {
	case "member:codingagent/src/core/agent-session.ts#AgentSession.setAutoCompactionEnabled", "member:codingagent/src/core/agent-session.ts#AgentSession.autoCompactionEnabled":
		entry.Status = catalog.StatusImplemented
		entry.Evidence = []catalog.Evidence{{Kind: "go-test", Ref: "codingagent/issue90_runtime_test.go", Baseline: issue32BaselineCommit, CaseID: entry.ID, InputHash: issue90RuntimeHash, ExecutionMethod: "go test ./codingagent -run '^TestAutoCompactionSettings$' -count=1", Expected: "effective auto-compaction settings and persisted mutations through AgentSession", Actual: "PASS; default, persisted disable and override", Platform: "any", CatalogID: entry.ID}}
		entry.Notes = "Issue #90; automatic compaction behavior belongs to contract:codingagent/compaction."
		return true
	}
	return false
}

const issue90RuntimeHash = "sha256:e3b971d885bc098f8c80fa6090f265e7524498dbed00ebad268a2cc925ee5ec7"

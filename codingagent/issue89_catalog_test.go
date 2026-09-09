package codingagent_test

import (
	"github.com/nankedr/pig/internal/catalog"
	"testing"
)

func issue89CompactionEvidence(t *testing.T) []issue32ModuleEvidenceDescriptor {
	t.Helper()
	var out []issue32ModuleEvidenceDescriptor
	for _, e := range []struct{ kind, path, run string }{
		{"oracle", "parity/oracle/fixtures/manual-compaction.json", "node --experimental-strip-types parity/oracle/manual-compaction.mjs <locked-pi-checkout> --check"},
		{"go-test", "codingagent/issue89_compaction_test.go", "go test ./codingagent -run '^TestManualCompactionParity$' -count=1"},
		{"go-test", "codingagent/issue89_runtime_test.go", "go test -race ./codingagent -run '^TestManualCompaction' -count=1"},
	} {
		out = append(out, issue32ModuleEvidenceDescriptor{InputPath: e.path, Evidence: catalog.Evidence{Kind: e.kind, Ref: e.path, Baseline: issue32BaselineCommit, CaseID: "issue89-" + e.path, ExecutionMethod: e.run, Expected: "fixed Pi public SDK manual compaction requests, cuts, split turns, prior summaries, budgets, file operations and retry lifecycle; atomic cancellation and v3 Headless continuation", Actual: "PASS; source/dist Oracle, SDK parity, dynamic auth and real HTTP truncation retry, v3 reopen and failure/race checks", Platform: "any", CatalogID: issue32CompactionCatalogID}})
	}
	return out
}
func issue89PromoteRuntimeEntry(entry *catalog.Entry) bool {
	switch entry.ID {
	case "member:codingagent/src/core/agent-session.ts#AgentSession.compact", "member:codingagent/src/core/agent-session.ts#AgentSession.abortCompaction", "member:codingagent/src/core/agent-session.ts#AgentSession.isCompacting",
		"member:codingagent/src/core/session-manager.ts#SessionManager.appendCompaction",
		"symbol:codingagent/src/core/compaction/compaction.ts#compact", "symbol:codingagent/src/core/compaction/compaction.ts#generateSummary", "symbol:codingagent/src/core/compaction/compaction.ts#generateSummaryWithUsage":
		entry.Status = catalog.StatusPartial
		entry.Partial = &catalog.Partial{Supported: []string{"manual compaction through public AgentSession and typed SummaryOptions, v3 persistence and continuation; see contract:codingagent/compaction"}, Unsupported: []string{"extension hooks, RPC/TUI controls and later Adapter paths remain deferred"}}
		entry.Deviation = &catalog.Deviation{ADR: "docs/adr/0026-manual-compaction.md", Reason: "Typed Go options, serialized commit and cancellation with atomic v3 file replacement; no Harness v4 coupling."}
		entry.Evidence = []catalog.Evidence{{Kind: "go-test", Ref: "codingagent/issue89_compaction_test.go", Baseline: issue32BaselineCommit, CaseID: entry.ID, InputHash: issue89CompactionTestHash, ExecutionMethod: "go test -race ./codingagent -run '^TestManualCompaction' -count=1", Expected: "public SDK manual compaction agrees with fixed Pi and survives error/cancellation/reopen", Actual: "PASS; SDK Oracle and runtime/Headless regressions", Platform: "any", CatalogID: entry.ID}}
		entry.Notes = "Issue #89 implements manual production v3 compaction; behavior owner contract:codingagent/compaction records remaining compatibility branches."
		return true
	}
	return false
}

const issue89CompactionTestHash = "sha256:ccece445ffacfe0d73d66af850e48429cb550c98ba354a6eeb5a35c1eb566e09"

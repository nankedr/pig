package codingagent_test

import (
	"github.com/nankedr/pig/internal/catalog"
	"testing"
)

const issue92SummaryCatalogID = "contract:codingagent/branch-summary"

func issue92SummaryCatalogEntry() catalog.Entry {
	return catalog.Entry{
		SchemaVersion: catalog.SchemaVersion, ID: issue92SummaryCatalogID,
		Upstream: catalog.Upstream{Module: "coding-agent", Repository: "https://github.com/badlogic/pi-mono", Commit: issue32BaselineCommit, Reference: "packages/coding-agent/src/core/agent-session.ts#navigateTree"},
		Mapping:  catalog.Mapping{Module: "codingagent", Target: issue32GoPackage + ".AgentSession.NavigateTree", Kind: "contract"}, Status: catalog.StatusPartial, Milestone: "M4", Classification: "public-api",
		Partial:   &catalog.Partial{Supported: []string{"public SDK branch-summary navigation: common ancestor, newest-first budget, prior summaries/compaction, file operations, custom/replace instructions, root/user/custom/assistant targets and summary labels", "Provider/ambient custom-stream auth, isolated no-cache summary requests, settings retry and summarization_retry events; AbortBranchSummary, IsCompacting, WaitForIdle and failure rollback", "v3 branch_summary persistence, SDK reopen, model-visible continuation, cross-branch roundtrip and same-leaf no-op preserve original branches; no-summary navigation remains covered by contract:codingagent/session-tree-navigation"}, Unsupported: []string{"extension session_before_tree/session_tree hooks, extension summary overrides/cancellation, RPC/TUI controls and later Provider Adapter paths remain deferred", "automatic compaction and queue draining during summarization remain deferred"}},
		Deviation: &catalog.Deviation{ADR: "docs/adr/0028-branch-summary-navigation.md", Reason: "Go context cancellation retains error identity; serialized admission and staged v3 replacement preserve state on storage failure."},
		Notes:     "Issue #92 verifies fixed Pi source/dist at createAgentSession and Go public SDK; standalone GenerateBranchSummary uses typed options. This slice does not freeze M4 or implement extension hooks.",
	}
}
func issue92SummaryEvidence(t *testing.T) []issue32ModuleEvidenceDescriptor {
	t.Helper()
	var out []issue32ModuleEvidenceDescriptor
	for _, e := range []struct{ kind, path, run string }{
		{"oracle", "parity/oracle/fixtures/branch-summary.json", "node --experimental-strip-types parity/oracle/branch-summary.mjs <locked-pi-checkout> --check"},
		{"go-test", "codingagent/issue92_summary_test.go", "go test ./codingagent -run '^TestBranchSummaryParity$' -count=1"},
		{"go-test", "codingagent/issue92_runtime_test.go", "go test -race ./codingagent -run '^TestBranchSummary' -count=1"},
	} {
		out = append(out, issue32ModuleEvidenceDescriptor{InputPath: e.path, Evidence: catalog.Evidence{Kind: e.kind, Ref: e.path, Baseline: issue32BaselineCommit, CaseID: "issue92-" + e.path, ExecutionMethod: e.run, Expected: "fixed Pi branch-summary requests, navigation commits, retries, cancellation, file tracking and persisted SDK continuation", Actual: "PASS; source/dist Oracle and public SDK parity, HTTP auth/truncation retry, ownership and atomic failures", Platform: "any", CatalogID: issue92SummaryCatalogID}})
	}
	return out
}
func issue92PromoteRuntimeEntry(entry *catalog.Entry) bool {
	switch entry.ID {
	case "member:codingagent/src/core/agent-session.ts#AgentSession.navigateTree", "member:codingagent/src/core/agent-session.ts#AgentSession.abortBranchSummary", "member:codingagent/src/core/agent-session.ts#AgentSession.isCompacting", "symbol:codingagent/src/core/compaction/branch-summarization.ts#generateBranchSummary":
		contract := issue92SummaryCatalogEntry()
		entry.Status, entry.Partial, entry.Deviation, entry.Notes = contract.Status, contract.Partial, contract.Deviation, contract.Notes
		entry.Evidence = []catalog.Evidence{{Kind: "go-test", Ref: "codingagent/issue92_summary_test.go", Baseline: issue32BaselineCommit, CaseID: entry.ID, InputHash: issue92SummaryTestHash, ExecutionMethod: "go test -race ./codingagent -run '^TestBranchSummary' -count=1", Expected: "public SDK summary navigation and continuation match fixed Pi", Actual: "PASS; request/context/persistence/retry Oracle and runtime cancellation/auth checks", Platform: "any", CatalogID: entry.ID}}
		return true
	}
	return false
}

const issue92SummaryTestHash = "sha256:831e21ad96535fa0271375a0e01c8ac4cb86b1451a7e02a39a705cfa051c29c1"

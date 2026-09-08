package codingagent_test

import (
	"github.com/nankedr/pig/internal/catalog"
	"testing"
)

const issue91TreeCatalogID = "contract:codingagent/session-tree-navigation"

func issue91TreeCatalogEntry() catalog.Entry {
	return catalog.Entry{
		SchemaVersion: catalog.SchemaVersion, ID: issue91TreeCatalogID,
		Upstream: catalog.Upstream{Module: "coding-agent", Repository: "https://github.com/badlogic/pi-mono", Commit: issue32BaselineCommit, Reference: "packages/coding-agent/src/core/agent-session.ts#navigateTree"},
		Mapping:  catalog.Mapping{Module: "codingagent", Target: issue32GoPackage + ".AgentSession.NavigateTree", Kind: "contract"}, Status: catalog.StatusPartial, Milestone: "M4", Classification: "public-api",
		Partial:   &catalog.Partial{Supported: []string{"public SDK no-summary navigation: root/user/custom editor text, ancestor/other branch/non-message targets, labels, same-leaf no-op, context reconstruction and subsequent Prompt", "fixed Pi live model/thinking retention versus selected-path SessionContext; same-file continuation, reopen and original-branch access", "invalid/busy/disposed/cancelled failures and label-write rollback preserve state and resources; subscriptions survive navigation"}, Unsupported: []string{"summarize=true on a different target remains an explicit Capability Stub; branch-summary generation, AbortBranchSummary and summarization retry remain deferred", "extension session_before_tree cancellation/overrides and session_tree hooks, RPC and TUI tree controls remain deferred", "unlabelled navigation does not persist a cursor; reopen follows the last appended entry, as in fixed Pi"}},
		Deviation: &catalog.Deviation{ADR: "docs/adr/0027-session-tree-navigation.md", Reason: "Go context cancellation, complete-run exclusion and staged label writes strengthen failure atomicity without replacing Session resources."},
		Notes:     "Issue #91 verifies fixed Pi public createAgentSession source/dist and public Go SDK continuation. This slice does not create independent Session forks or claim complete summary navigation.",
	}
}
func issue91TreeEvidence(t *testing.T) []issue32ModuleEvidenceDescriptor {
	t.Helper()
	var out []issue32ModuleEvidenceDescriptor
	for _, e := range []struct{ kind, path, run string }{
		{"oracle", "parity/oracle/fixtures/session-tree-navigation.json", "node --experimental-strip-types parity/oracle/session-tree-navigation.mjs <locked-pi-checkout> --check"},
		{"go-test", "codingagent/issue91_navigation_test.go", "go test ./codingagent -run '^TestSessionTreeNavigationParity$' -count=1"},
		{"go-test", "codingagent/issue91_runtime_test.go", "go test -race ./codingagent -run '^TestSessionTreeNavigation' -count=1"},
	} {
		out = append(out, issue32ModuleEvidenceDescriptor{InputPath: e.path, Evidence: catalog.Evidence{Kind: e.kind, Ref: e.path, Baseline: issue32BaselineCommit, CaseID: "issue91-" + e.path, ExecutionMethod: e.run, Expected: "fixed Pi tree navigation, editor text, labels, live/path configuration and public SDK continuation with safe failures", Actual: "PASS; Oracle source/dist comparison, same-file branches, SDK reopen, listener lifecycle and rollback", Platform: "any", CatalogID: issue91TreeCatalogID}})
	}
	return out
}
func issue91PromoteRuntimeEntry(entry *catalog.Entry) bool {
	if entry.ID != "member:codingagent/src/core/agent-session.ts#AgentSession.navigateTree" {
		return false
	}
	contract := issue91TreeCatalogEntry()
	entry.Status, entry.Partial, entry.Deviation, entry.Notes = contract.Status, contract.Partial, contract.Deviation, contract.Notes
	entry.Evidence = []catalog.Evidence{{Kind: "go-test", Ref: "codingagent/issue91_navigation_test.go", Baseline: issue32BaselineCommit, CaseID: entry.ID, InputHash: issue91TreeTestHash, ExecutionMethod: "go test ./codingagent -run '^TestSessionTreeNavigationParity$' -count=1", Expected: "public SDK navigation and continuation matches fixed Pi", Actual: "PASS; root/custom/non-message targets, branching, labels and live/path configuration", Platform: "any", CatalogID: entry.ID}}
	return true
}

const issue91TreeTestHash = "sha256:7e77b32d0c02d995274dcfce48fe8b356f98d1cf5f38a6f2871bbbf5c0fdaeb8"

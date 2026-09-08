package codingagent_test

import (
	"strings"
	"testing"

	"github.com/nankedr/pig/internal/catalog"
)

const issue88StatsCatalogID = "contract:codingagent/session-stats"
const issue88StatsTestHash = "sha256:efe35ac44ff361c5217dbfb2117455e2352ba8a7b87b176449407b72db30fa17"

func issue88StatsCatalogEntry() catalog.Entry {
	return catalog.Entry{
		SchemaVersion: catalog.SchemaVersion, ID: issue88StatsCatalogID,
		Upstream: catalog.Upstream{Module: "coding-agent", Repository: "https://github.com/badlogic/pi-mono", Commit: issue32BaselineCommit, Reference: "packages/coding-agent/src/core/agent-session.ts#getSessionStats/getContextUsage/getLastAssistantText"},
		Mapping:  catalog.Mapping{Module: "codingagent", Target: issue32GoPackage + ".AgentSession.GetSessionStats", Kind: "contract"}, Status: catalog.StatusImplemented, Milestone: "M4", Classification: "public-api",
		Notes: "Issue #88 implements read-only public SDK statistics over all persisted entries, including other branches, ToolResult/summary usage and partial failures; context estimates use current messages and the latest branch compaction boundary. nil/unknown/zero and last-text selection match fixed Pi. Existing ai.Usage carrier uses TotalTokens for Pi tokens.total; only the four token components and TotalTokens are populated, billing is SessionStats.Cost. Unconfigured constructors retain structured errors. RPCClient/RPC commands, TUI rendering, extension query hooks, compaction orchestration and cost breakdown UI remain deferred; the ContextUsage carrier does not activate extensions.",
	}
}
func issue88StatsEvidence(t *testing.T) []issue32ModuleEvidenceDescriptor {
	t.Helper()
	var result []issue32ModuleEvidenceDescriptor
	for _, e := range []struct{ kind, path, run string }{
		{"oracle", "parity/oracle/fixtures/session-stats.json", "node --experimental-strip-types parity/oracle/session-stats.mjs <locked-pi-checkout> --check"},
		{"go-test", "codingagent/issue88_stats_test.go", "go test ./codingagent -run '^TestSessionStatsParity$' -count=1"},
		{"go-test", "codingagent/issue88_runtime_test.go", "go test -race ./codingagent -run '^TestSessionStats' -count=1"},
	} {
		result = append(result, issue32ModuleEvidenceDescriptor{InputPath: e.path, Evidence: catalog.Evidence{Kind: e.kind, Ref: e.path, Baseline: issue32BaselineCommit, CaseID: "issue88-" + e.path, ExecutionMethod: e.run, Expected: "fixed Pi public SDK statistics, current-context/complete-history distinction, immutable queries and cross-process restoration", Actual: "PASS; 17 source/dist Oracle scenarios plus real Tool, partial error/abort, streaming/race and independent process checks", Platform: "any", CatalogID: issue88StatsCatalogID}})
	}
	return result
}
func issue88PromoteRuntimeEntry(entry *catalog.Entry) bool {
	match := false
	switch entry.ID {
	case "member:codingagent/src/core/agent-session.ts#AgentSession.getSessionStats", "member:codingagent/src/core/agent-session.ts#AgentSession.getContextUsage", "member:codingagent/src/core/agent-session.ts#AgentSession.getLastAssistantText", "symbol:codingagent/src/core/agent-session.ts#SessionStats", "symbol:codingagent/src/core/extensions/types.ts#ContextUsage":
		match = true
	}
	if !match && !strings.HasPrefix(entry.ID, "member:codingagent/src/core/agent-session.ts#SessionStats.") && !strings.HasPrefix(entry.ID, "member:codingagent/src/core/extensions/types.ts#ContextUsage.") {
		return false
	}
	entry.Status = catalog.StatusImplemented
	entry.Evidence = []catalog.Evidence{{Kind: "go-test", Ref: "codingagent/issue88_stats_test.go", Baseline: issue32BaselineCommit, CaseID: entry.ID, InputHash: issue88StatsTestHash, ExecutionMethod: "go test -race ./codingagent -run '^TestSessionStats' -count=1", Expected: "public SDK queries reproduce fixed Pi counts, usage/cost, context and optional last-text observations", Actual: "PASS; public create/reopen queries match 17 fixed Pi source/dist cases and runtime/restore checks", Platform: "any", CatalogID: entry.ID}}
	entry.Notes = "Issue #88 public SDK query/carrier implementation; evidence and deferred adjacent branches are recorded in " + issue88StatsCatalogID + "."
	return true
}

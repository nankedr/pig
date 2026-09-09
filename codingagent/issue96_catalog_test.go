package codingagent_test

import (
	"github.com/nankedr/pig/internal/catalog"
	"strings"
	"testing"
)

const issue96RPCCatalogID = "contract:rpc/session-lifecycle"
const issue96RPCTestHash = "c9be63901cab6161321db88afcaf943da445240c4c441881bb9258c8eb3211c8"

func issue96RPCCatalogEntry() catalog.Entry {
	return catalog.Entry{SchemaVersion: catalog.SchemaVersion, ID: issue96RPCCatalogID, Upstream: catalog.Upstream{Module: "coding-agent", Repository: "https://github.com/badlogic/pi-mono", Commit: issue32BaselineCommit, Reference: "packages/coding-agent/src/modes/rpc/rpc-mode.ts"}, Mapping: catalog.Mapping{Module: "codingagent", Target: issue32GoPackage + ".RPCClient", Kind: "contract"}, Status: catalog.StatusPartial, Milestone: "M4", Classification: "public-api", Partial: &catalog.Partial{Supported: []string{
		"Issue #96: fixed wire new_session, switch_session, fork, clone, get_fork_messages, get_entries/since, get_tree, set_session_name, compact and set_auto_compaction with existing public RPCClient signatures",
		"Source Pi runRpcMode and public RpcClient lifecycle fixture compared with real pig subprocesses; append-only compaction, source isolation, parent/header identity, typed v3 entry/tree codec, restart/reopen continuation and automatic overflow recovery",
		"Replacement admission cancels and drains active Prompt/summary/Bash before reading the source; target factory failure retains the live original Session; generation-scoped event forwarding drops late old-session events; race-built subprocess verifies concurrent queries, prompt rejection, abort and replacement",
	}, Unsupported: []string{
		"Fixed Pi has no navigate_tree or abort_compaction wire command and no RPCClient NavigateTree method. Branch-summary tree navigation is exposed only through extension command contexts; extension runtime/UI remains deferred, so RPC branch-summary generation is not claimed",
		"Images, get_commands, extension callbacks/custom summaries and unsupported provider adapters remain deferred; SDK direct NavigateTree remains delivered by #91/#92; Issue #97 delivers export_html",
		"Generic runtime rebind callback failures retain the existing invalidated-target behavior; RPC's internal rebind only attaches a listener to a freshly constructed Session. Request Context cancellation is local; Abort controls remote work",
	}}, Deviation: &catalog.Deviation{ADR: "docs/adr/0032-rpc-session-lifecycle.md", Reason: "Stage target assembly before invalidating the old Session, reuse Session admission and v3 codecs, and preserve the fixed wire without inventing extension navigation commands."}, Notes: "Pi source Oracle only; no new dist build claim. No M4 freeze claim."}
}
func issue96RPCEvidence(t *testing.T) []issue32ModuleEvidenceDescriptor {
	t.Helper()
	var result []issue32ModuleEvidenceDescriptor
	for _, e := range []struct{ kind, path, run string }{
		{"oracle", "parity/oracle/fixtures/rpc-lifecycle.json", "node --experimental-strip-types parity/oracle/rpc-lifecycle.mjs <locked-pi-checkout> --check"},
		{"go-test", "cmd/pig/issue96_rpc_test.go", "go test -race ./cmd/pig -run '^TestRPC96' -count=1"},
		{"go-test", "codingagent/issue96_runtime_test.go", "go test -race ./codingagent -run '^TestIssue96Replacement' -count=1"},
		{"go-test", "codingagent/issue96_surface_test.go", "go test ./codingagent -run '^TestIssue96RPC' -count=1"},
	} {
		result = append(result, issue32ModuleEvidenceDescriptor{InputPath: e.path, Evidence: catalog.Evidence{Kind: e.kind, Ref: e.path, Baseline: issue32BaselineCommit, CaseID: "issue96-" + e.path, ExecutionMethod: e.run, Expected: "fixed Pi lifecycle and public Session invariants", Actual: "PASS; source Oracle and real RPC lifecycle, compaction, cancellation and v3 reopen", Platform: "any", CatalogID: issue96RPCCatalogID}})
	}
	return result
}
func issue96PromoteRuntimeEntry(e *catalog.Entry) bool {
	name, ok := strings.CutPrefix(e.ID, "member:codingagent/src/modes/rpc/rpc-client.ts#RpcClient.")
	if !ok || !strings.Contains("|newSession|switchSession|fork|clone|getForkMessages|getEntries|getTree|setSessionName|compact|setAutoCompaction|", "|"+name+"|") {
		return false
	}
	e.Status = catalog.StatusImplemented
	e.Evidence = []catalog.Evidence{{Kind: "go-test", Ref: "cmd/pig/issue96_rpc_test.go", Baseline: issue32BaselineCommit, CaseID: e.ID, InputHash: issue96RPCTestHash, ExecutionMethod: "go test -race ./cmd/pig -run '^TestRPC96' -count=1", Expected: "public RPCClient lifecycle matches the fixed wire", Actual: "PASS; Pi fixture, real subprocesses and v3 continuation", Platform: "any", CatalogID: e.ID}}
	e.Notes = "Issue #96 RPC lifecycle; scope and deviations in " + issue96RPCCatalogID + "."
	return true
}

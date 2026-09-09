package codingagent_test

import (
	"github.com/nankedr/pig/internal/catalog"
	"strings"
	"testing"
)

const issue95RPCCatalogID = "contract:rpc/session-control"
const issue95RPCTestHash = "ce9e7277c9f194fccfbd72ae765657098f478c4379b47f2d0d7015ac484b8db6"

func issue95RPCCatalogEntry() catalog.Entry {
	return catalog.Entry{SchemaVersion: catalog.SchemaVersion, ID: issue95RPCCatalogID, Upstream: catalog.Upstream{Module: "coding-agent", Repository: "https://github.com/badlogic/pi-mono", Commit: issue32BaselineCommit, Reference: "packages/coding-agent/src/modes/rpc/rpc-mode.ts"}, Mapping: catalog.Mapping{Module: "codingagent", Target: issue32GoPackage + ".RPCClient", Kind: "contract"}, Status: catalog.StatusPartial, Milestone: "M4", Classification: "public-api", Partial: &catalog.Partial{Supported: []string{
		"Issue #95: text steer/follow_up, queue modes, set/cycle/list model and thinking, retry toggle/abort, Bash/abort and statistics through real JSONL subprocesses and public RPCClient",
		"Ordered synchronous commands with an asynchronous FIFO writer, concurrent request correlation, arbitrary JSON Bash event IDs, active queue continuation, busy configuration failures, process endpoint override across model changes, deferred Bash history and cancelled retry persistence",
		"Wire stats tokens.total maps to SDK Tokens.TotalTokens; null cycle_model maps to zero ModelCycleResult; null cycle_thinking_level maps to empty ThinkingLevel without changing published signatures",
	}, Unsupported: []string{
		"Images, resource/extension/UI commands, session replacement/tree/export and compaction RPC commands remain explicit Capability Stubs; the fixed baseline has no Tool control RPC command",
		"RPCClient.Bash keeps its existing command-only signature; excludeFromContext is supported on raw wire only; extension user_bash interception remains deferred",
		"Idle/late/cancelled queue admission and active configuration updates retain ADR-0018/0025 Go invariants; invalid typed modes/thinking are rejected; only existing DeepSeek runtime adapters are supported",
	}}, Deviation: &catalog.Deviation{ADR: "docs/adr/0031-rpc-session-control.md", Reason: "Reuse Session admission/configuration invariants and existing typed client signatures; preserve raw wire IDs separately from SDK event IDs."}, Notes: "Fixed Pi source runRpcMode and public RpcClient fixture plus real pig tests. Pi dist rebuild is not claimed: the locked checkout has an existing cloudflare-ai-gateway TypeScript error."}
}

func issue95RPCEvidence(t *testing.T) []issue32ModuleEvidenceDescriptor {
	t.Helper()
	var result []issue32ModuleEvidenceDescriptor
	for _, e := range []struct{ kind, path, run string }{
		{"oracle", "parity/oracle/fixtures/rpc-control.json", "node --experimental-strip-types parity/oracle/rpc-control.mjs <locked-pi-checkout> --check"},
		{"go-test", "cmd/pig/issue95_rpc_test.go", "go test -race ./cmd/pig -run '^TestRPC95' -count=1"},
		{"go-test", "codingagent/issue95_surface_test.go", "go test ./codingagent -run '^TestIssue95RPC' -count=1"},
	} {
		result = append(result, issue32ModuleEvidenceDescriptor{InputPath: e.path, Evidence: catalog.Evidence{Kind: e.kind, Ref: e.path, Baseline: issue32BaselineCommit, CaseID: "issue95-" + e.path, ExecutionMethod: e.run, Expected: "fixed Pi RPC controls, typed client results, request correlation and Session invariants", Actual: "PASS; source Oracle dispatch/client lifecycle and real pig queue/configuration/Bash/retry/persistence/SDK-observer checks", Platform: "any", CatalogID: issue95RPCCatalogID}})
	}
	return result
}
func issue95PromoteRuntimeEntry(e *catalog.Entry) bool {
	name, ok := strings.CutPrefix(e.ID, "member:codingagent/src/modes/rpc/rpc-client.ts#RpcClient.")
	if !ok || !strings.Contains("|steer|followUp|setSteeringMode|setFollowUpMode|setModel|cycleModel|getAvailableModels|setThinkingLevel|cycleThinkingLevel|getAvailableThinkingLevels|setAutoRetry|abortRetry|bash|abortBash|getSessionStats|", "|"+name+"|") {
		return false
	}
	e.Status = catalog.StatusImplemented
	e.Evidence = []catalog.Evidence{{Kind: "go-test", Ref: "cmd/pig/issue95_rpc_test.go", Baseline: issue32BaselineCommit, CaseID: e.ID, InputHash: issue95RPCTestHash, ExecutionMethod: "go test -race ./cmd/pig -run '^TestRPC95' -count=1", Expected: "public RPCClient drives real Session control", Actual: "PASS; controls, events, concurrency and persistence", Platform: "any", CatalogID: e.ID}}
	e.Notes = "Issue #95 RPC control; scope and deviations in " + issue95RPCCatalogID + "."
	return true
}

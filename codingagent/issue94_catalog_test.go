package codingagent_test

import (
	"strings"
	"testing"

	"github.com/nankedr/pig/internal/catalog"
)

const issue94RPCCatalogID = "contract:rpc/jsonl-transport"
const issue94RPCTestHash = "27cdce78cc6039da239d56a63d32b9fa27b23362c7375d84e0b753bc3b4d83d8"

func issue94RPCCatalogEntry() catalog.Entry {
	return catalog.Entry{SchemaVersion: catalog.SchemaVersion, ID: issue94RPCCatalogID, Upstream: catalog.Upstream{Module: "coding-agent", Repository: "https://github.com/badlogic/pi-mono", Commit: issue32BaselineCommit, Reference: "packages/coding-agent/src/modes/rpc/jsonl.ts"}, Mapping: catalog.Mapping{Module: "codingagent", Target: issue32GoPackage + ".RunRPCMode", Kind: "contract"}, Status: catalog.StatusPartial, Milestone: "M4", Classification: "public-api", Partial: &catalog.Partial{Supported: []string{"Issue #94: real pig --mode rpc and RPCClient Start/Stop, text Prompt/PromptAndWait, events/CollectEvents/WaitForIdle, Abort, GetState/GetMessages/GetLastAssistantText/GetStderr", "LF writer and LF/CRLF UTF-8 chunked reader including final unterminated record, loose parsed-value dispatch, arbitrary wire IDs, concurrent responses and projected events", "local cancellation without remote abort, late response disposal, EOF/write/startup failure and process/subscription cleanup; shared v3 Session/auth/trust startup", "Issue #95 session-control commands are implemented and evidenced separately in contract:rpc/session-control"}, Unsupported: []string{"images, extensions/UI and export commands remain explicit capability failures; #96 delivers session replacement, compaction controls and tree queries; Issue #95 adds steer/follow_up, model/thinking/queue controls, retry, Bash and statistics", "RPCCommand.Data is still an inert scaffold carrier, not a typed serializer for flattened command fields; RPCResponse.ID is the typed client string ID while runtime accepts any parsed JSON id", "Pi's null top-level input terminates via an unhandled error; Pig terminates with a controlled error. JSON parser wording, startup readiness handshake, Context cancellation and idle-state probe are documented Go mappings"}}, Notes: "Fixed Pi source/dist runRpcMode and RpcClient observations are verified by real subprocess tests. RPC uses JSON event projection and never the Remote Session Protocol CBOR decoder. See docs/adr/0029-jsonl-rpc.md."}
}
func issue94RPCEvidence(t *testing.T) []issue32ModuleEvidenceDescriptor {
	t.Helper()
	var out []issue32ModuleEvidenceDescriptor
	for _, e := range []struct{ kind, path, run string }{{"oracle", "parity/oracle/fixtures/rpc.json", "node --experimental-strip-types parity/oracle/rpc.mjs <locked-pi-checkout> --check"}, {"go-test", "cmd/pig/issue94_rpc_test.go", "go test -race ./cmd/pig -run '^TestRPC94' -count=1"}, {"go-test", "codingagent/issue94_surface_test.go", "go test ./codingagent -run '^TestIssue94RPC' -count=1"}} {
		out = append(out, issue32ModuleEvidenceDescriptor{InputPath: e.path, Evidence: catalog.Evidence{Kind: e.kind, Ref: e.path, Baseline: issue32BaselineCommit, CaseID: "issue94-" + e.path, ExecutionMethod: e.run, Expected: "fixed Pi JSONL dispatch and client event projection with real process lifecycle and failure cleanup", Actual: "PASS; 19 source/dist dispatch cases, source/dist public client lifecycle, real pig and adversarial subprocess checks", Platform: "any", CatalogID: issue94RPCCatalogID}})
	}
	return out
}
func issue94PromoteRuntimeEntry(e *catalog.Entry) bool {
	prefix := "member:codingagent/src/modes/rpc/rpc-client.ts#RpcClient."
	implemented := false
	if name, ok := strings.CutPrefix(e.ID, prefix); ok {
		implemented = strings.Contains("|start|stop|prompt|promptAndWait|abort|getState|getMessages|getLastAssistantText|getStderr|onEvent|collectEvents|waitForIdle|", "|"+name+"|")
	}
	partial := e.ID == "symbol:codingagent/src/modes/rpc/rpc-client.ts#RpcClient" || e.ID == "symbol:codingagent/src/modes/rpc/rpc-mode.ts#runRpcMode"
	if !implemented && !partial {
		return false
	}
	e.Status = catalog.StatusImplemented
	if partial {
		e.Status = catalog.StatusPartial
		e.Partial = issue94RPCCatalogEntry().Partial
	}
	e.Evidence = []catalog.Evidence{{Kind: "go-test", Ref: "cmd/pig/issue94_rpc_test.go", Baseline: issue32BaselineCommit, CaseID: e.ID, InputHash: issue94RPCTestHash, ExecutionMethod: "go test -race ./cmd/pig -run '^TestRPC94' -count=1", Expected: "real JSONL subprocess conversation and complete client lifecycle", Actual: "PASS; fixed Pi dispatch/lifecycle plus cancellation, EOF, failure cleanup and asynchronous query checks", Platform: "any", CatalogID: e.ID}}
	e.Notes = "Issue #94 basic RPC lifecycle; scope and deviations in " + issue94RPCCatalogID + "."
	return true
}

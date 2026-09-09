package codingagent_test

import (
	"github.com/nankedr/pig/internal/catalog"
	"strings"
	"testing"
)

const issue93BashCatalogID = "contract:codingagent/session-bash"
const issue93BashTestHash = "sha256:43a5870b4104f5d8f369d0d358d145d814ce6796bfcafd646cf459f8926b0bac"

func issue93BashCatalogEntry() catalog.Entry {
	return catalog.Entry{SchemaVersion: catalog.SchemaVersion, ID: issue93BashCatalogID, Upstream: catalog.Upstream{Module: "coding-agent", Repository: "https://github.com/badlogic/pi-mono", Commit: issue32BaselineCommit, Reference: "packages/coding-agent/src/core/agent-session.ts"}, Mapping: catalog.Mapping{Module: "codingagent", Target: issue32GoPackage + ".AgentSession.ExecuteBash", Kind: "contract"}, Status: catalog.StatusPartial, Milestone: "M4", Classification: "public-api", Partial: &catalog.Partial{Supported: []string{"public SDK ExecuteBash/AbortBash/RecordBashResult, chunk events with optional ID, injected BashOperations and shell settings", "sanitized output, exit/cancellation, rolling tail, retained full output and read回读", "parallel executions, deferred generation-time recording, v3 reopen and subsequent model input with ExcludeFromContext"}, Unsupported: []string{"TUI command routing and extension runtime remain deferred; RPC Bash is covered by contract:rpc/session-control", "local process execution remains limited to the existing darwin/linux Bash contract; injected Operations are portable"}}, Notes: "Issue #93. Fixed Pi source SDK Oracle and public SDK tests; no implicit timeout. Cancellation returns a BashResult; execution failures do not record. Dispose cancels active Bash and lets each call finish recording. Existing SessionManager delayed flush and filesystem failure semantics apply."}
}
func issue93BashEvidence(t *testing.T) []issue32ModuleEvidenceDescriptor {
	t.Helper()
	var out []issue32ModuleEvidenceDescriptor
	for _, e := range []struct{ kind, path, run, platform string }{
		{"oracle", "parity/oracle/fixtures/session-bash.json", "node --experimental-strip-types parity/oracle/session-bash.mjs <locked-pi-checkout> --check", "any"},
		{"go-test", "codingagent/issue93_bash_test.go", "go test ./codingagent -run '^TestSessionBashParity$' -count=1", "any"},
		{"go-test", "codingagent/issue93_runtime_test.go", "go test -race ./codingagent -run '^TestSessionBash' -count=1", "darwin,linux"},
		{"go-test", "codingagent/issue93_process_unix_test.go", "go test -race ./codingagent -run '^TestSessionBashAbortKillsProcessTree$' -count=1", "darwin,linux"},
	} {
		out = append(out, issue32ModuleEvidenceDescriptor{InputPath: e.path, Evidence: catalog.Evidence{Kind: e.kind, Ref: e.path, Baseline: issue32BaselineCommit, CaseID: "issue93-" + e.path, ExecutionMethod: e.run, Expected: "fixed Pi public Session Bash observations and host lifecycle contract", Actual: "PASS; SDK parity, concurrent execution/cancellation, context exclusion, persistence and full-output read", Platform: e.platform, CatalogID: issue93BashCatalogID}})
	}
	return out
}
func issue93PromoteRuntimeEntry(entry *catalog.Entry) bool {
	name := strings.TrimPrefix(entry.ID, "member:codingagent/src/core/agent-session.ts#AgentSession.")
	if name == entry.ID {
		return false
	}
	switch name {
	case "executeBash", "abortBash", "recordBashResult", "isBashRunning", "hasPendingBashMessages":
		entry.Status = catalog.StatusImplemented
		entry.Evidence = []catalog.Evidence{{Kind: "go-test", Ref: "codingagent/issue93_bash_test.go", Baseline: issue32BaselineCommit, CaseID: entry.ID, InputHash: issue93BashTestHash, ExecutionMethod: "go test ./codingagent -run '^TestSessionBashParity$' -count=1", Expected: "fixed Pi SDK Bash result, chunks, context and deferred recording", Actual: "PASS; fixture replay through CreateAgentSession", Platform: "any", CatalogID: entry.ID}}
		entry.Notes = "Issue #93; public SDK Bash implemented within existing host platform contract. See " + issue93BashCatalogID
		return true
	}
	return false
}

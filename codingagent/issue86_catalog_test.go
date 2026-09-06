package codingagent_test

import (
	"testing"

	"github.com/nankedr/pig/internal/catalog"
)

const issue86RetryCatalogID = "contract:codingagent/turn-retry"
const issue86RetryTestHash = "sha256:583f4359d3d202deb088d439541b5c583eb8b136ef2a79096a2047a83c88f6c0"

func issue86RetryCatalogEntry() catalog.Entry {
	return catalog.Entry{
		SchemaVersion: catalog.SchemaVersion, ID: issue86RetryCatalogID,
		Upstream: catalog.Upstream{Module: "coding-agent", Repository: "https://github.com/badlogic/pi-mono", Commit: issue32BaselineCommit, Reference: "packages/coding-agent/src/core/agent-session.ts"},
		Mapping:  catalog.Mapping{Module: "codingagent", Target: issue32GoPackage + ".AgentSession.Prompt", Kind: "contract"}, Status: catalog.StatusPartial, Milestone: "M4", Classification: "public-api",
		Partial:   &catalog.Partial{Supported: []string{"public SDK/Headless turn retry: enabled setting, fixed-Pi error classification, independent exponential budget, attempt state and abortable waits", "Oracle-backed error Assistant removal from Agent context while preserving v3 history, retry/settled ordering and completed Tool side effects; complete recovery Tool loops", "Headless partial cancellation outcome and no late retries via AbortRetry/Abort/Context/Dispose; text/json CLI transport versus Session budgets and persisted failures"}, Unsupported: []string{"RPC retry controls, extension hooks, compaction and summarization retry remain deferred", "AI trailing-frame/malformed protocol errors retain existing validation errors without retry; execution remains limited to already implemented Provider adapters; noninteger settings and configured/computed delays outside int64 have no Go mapping"}},
		Deviation: &catalog.Deviation{ADR: "docs/adr/0024-session-turn-retry.md", Reason: "Arm cancellation before synchronous auto_retry_start listeners; preserve canceled partial HeadlessOutcome independently of the pruned Agent context, following ADR-0006."},
		Notes:     "Issue #86; fixed Pi public createAgentSession Oracle, public SDK/Headless race tests and real text/json HTTP process tests. AbortRetry only cancels backoff; Prompt awaits the full recovered Tool loop. No M4-wide freeze claim.",
	}
}

func issue86RetryEvidence(t *testing.T) []issue32ModuleEvidenceDescriptor {
	t.Helper()
	var out []issue32ModuleEvidenceDescriptor
	for _, e := range []struct{ kind, path, ref, id, run string }{
		{"oracle", "parity/oracle/fixtures/turn-retry.json", "parity/oracle/fixtures/turn-retry.json", "issue86-turn-retry-oracle", "node --experimental-strip-types parity/oracle/turn-retry.mjs <locked-pi-checkout> --check"},
		{"go-test", "codingagent/issue86_retry_test.go", "codingagent/issue86_retry_test.go#TestTurnRetryParity", "issue86-turn-retry-sdk", "go test -race ./codingagent -run '^TestTurnRetry' -count=1"},
		{"go-test", "cmd/pig/issue86_process_test.go", "cmd/pig/issue86_process_test.go#TestPigTurnRetryLayers", "issue86-turn-retry-cli", "go test ./cmd/pig -run '^TestPigTurnRetryLayers$' -count=1"},
	} {
		out = append(out, issue32ModuleEvidenceDescriptor{InputPath: e.path, Evidence: catalog.Evidence{Kind: e.kind, Ref: e.ref, Baseline: issue32BaselineCommit, CaseID: e.id, ExecutionMethod: e.run, Expected: "fixed Pi retry classification, events, context/history and Tool effects; cancellable Headless partial outcomes and independent transport budgets", Actual: "PASS; SDK Oracle and Headless/process tests recover, exhaust or cancel without duplicate Tool effects or late retries", Platform: "darwin-arm64", CatalogID: issue86RetryCatalogID}})
	}
	return out
}

func issue86PromoteRuntimeEntry(entry *catalog.Entry) bool {
	switch entry.ID {
	case "member:codingagent/src/core/agent-session.ts#AgentSession.abortRetry", "member:codingagent/src/core/agent-session.ts#AgentSession.autoRetryEnabled", "member:codingagent/src/core/agent-session.ts#AgentSession.isRetrying", "member:codingagent/src/core/agent-session.ts#AgentSession.retryAttempt", "member:codingagent/src/core/agent-session.ts#AgentSession.setAutoRetryEnabled":
		entry.Status = catalog.StatusImplemented
		entry.Evidence = []catalog.Evidence{{Kind: "go-test", Ref: "codingagent/issue86_retry_test.go", Baseline: issue32BaselineCommit, CaseID: entry.ID, InputHash: issue86RetryTestHash, ExecutionMethod: "go test -race ./codingagent -run '^TestTurnRetry' -count=1", Expected: "public SDK observes effective retry configuration, accurate attempt/wait state and cancellation", Actual: "PASS; fixed Pi parity, setting persistence and no late retry after cancellation", Platform: "any", CatalogID: entry.ID}}
		entry.Notes = "Issue #86 implements this public retry control; behavioral evidence belongs to " + issue86RetryCatalogID + ". Synchronous cancellation and Headless partial ownership follow ADR-0024."
		return true
	}
	return false
}

package codingagent_test

import (
	"github.com/nankedr/pig/internal/catalog"
	"testing"
)

const issue81ToolsCatalogID = "contract:codingagent/default-coding-tools"

func issue81ToolsCatalogEntry() catalog.Entry {
	return catalog.Entry{
		SchemaVersion: catalog.SchemaVersion, ID: issue81ToolsCatalogID,
		Upstream: catalog.Upstream{Module: "coding-agent", Repository: "https://github.com/badlogic/pi-mono", Commit: issue32BaselineCommit, Reference: "packages/coding-agent/src/core/sdk.ts"},
		Mapping:  catalog.Mapping{Module: "codingagent", Target: issue32GoPackage + ".CreateAgentSession", Kind: "contract"}, Status: catalog.StatusPartial, Milestone: "M3", Classification: "public-api",
		Partial:   &catalog.Partial{Supported: []string{"default read/bash/edit/write and shared CLI/SDK assembly, explicit selection/exclusion/suppression and matching system prompt", "trusted shell path/prefix, Provider retry/timeout and Agent transport/queue/thinking settings; session identity reaches Provider with cache disabled and current PIG_* Bash environment", "real text/json CLI write/read/edit/bash, persistent Tool failure and signal cancellation, exit/reopen/continue/fork without duplicate history or source mutation; side-effect-free in-memory state"}, Unsupported: []string{"grep/find/ls and image Tools, RPC, compaction and orchestrated tree navigation retain later milestone gates", "Provider caching/affinity, extension runtime and six-platform shell behavior remain deferred"}},
		Deviation: &catalog.Deviation{ADR: "docs/adr/0008-pig-identity-state-and-services.md", Reason: "Session environment uses PIG_* and retained OS temporary output uses pig-bash-*; no PI_* identity is generated. Built-in definition execution follows ADR-0020."},
		Notes:     "Issue #81 verifies M3.11 through the highest public Session and process boundaries. AgentTools injection and explicit in-memory managers retain the M1/M2 SDK path. Chat Completions accepts SessionID only with explicit CacheRetentionNone; no cache key or affinity header is sent. Pi selection Oracle is source-locked. No M3-wide freeze claim.",
	}
}
func issue81ToolsEvidence(t *testing.T) []issue32ModuleEvidenceDescriptor {
	t.Helper()
	var out []issue32ModuleEvidenceDescriptor
	for _, e := range []struct{ kind, path, ref, id, run string }{
		{"oracle", "parity/oracle/fixtures/coding-tools.json", "parity/oracle/fixtures/coding-tools.json", "issue81-coding-tools-oracle", "node --experimental-strip-types parity/oracle/coding-tools.mjs <locked-pi-checkout> --check"},
		{"go-test", "codingagent/issue81_tools_test.go", "codingagent/issue81_tools_test.go#TestDefaultCodingToolsParity", "issue81-default-tools", "go test -race ./codingagent -run '^TestDefaultCodingTools' -count=1"},
		{"go-test", "cmd/pig/issue81_process_test.go", "cmd/pig/issue81_process_test.go#TestPigDefaultCodingTaskResumeAndFork", "issue81-cli-resume-fork", "go test ./cmd/pig -run '^TestPigDefaultCoding' -count=1"},
		{"go-test", "cmd/pig/issue80_signal_unix_test.go", "cmd/pig/issue80_signal_unix_test.go#TestPigBashShutdownSignalsKillProcessTree", "issue81-cancel-reopen", "go test ./cmd/pig -run '^TestPigBashShutdownSignalsKillProcessTree$' -count=1"},
		{"go-test", "ai/issue81_session_test.go", "ai/issue81_session_test.go#TestOpenAICompletionsSessionIdentityWithoutCache", "issue81-provider-identity", "go test ./ai -run '^TestOpenAICompletionsSessionIdentityWithoutCache$' -count=1"},
	} {
		out = append(out, issue32ModuleEvidenceDescriptor{InputPath: e.path, Evidence: catalog.Evidence{Kind: e.kind, Ref: e.ref, Baseline: issue32BaselineCommit, CaseID: e.id, ExecutionMethod: e.run, Expected: "four default Tools, effective settings, consistent identities and recoverable coding history match the supported Pi boundaries", Actual: "PASS; fixed Pi fixture, public SDK settings, text/json resume/fork and signal cancellation/reopen", Platform: "darwin-arm64", CatalogID: issue81ToolsCatalogID}})
	}
	return out
}

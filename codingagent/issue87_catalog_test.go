package codingagent_test

import (
	"testing"

	"github.com/nankedr/pig/internal/catalog"
)

const issue87ConfigurationCatalogID = "contract:codingagent/session-configuration"
const issue87ConfigurationTestHash = "sha256:ee92e2cad1378f77920dd44d29c0fa90dac8bf41a93bb8a569d4848cd7a702f8"

func issue87ConfigurationCatalogEntry() catalog.Entry {
	return catalog.Entry{
		SchemaVersion: catalog.SchemaVersion, ID: issue87ConfigurationCatalogID,
		Upstream: catalog.Upstream{Module: "coding-agent", Repository: "https://github.com/badlogic/pi-mono", Commit: issue32BaselineCommit, Reference: "packages/coding-agent/src/core/agent-session.ts"},
		Mapping:  catalog.Mapping{Module: "codingagent", Target: issue32GoPackage + ".AgentSession.SetModel", Kind: "contract"}, Status: catalog.StatusPartial, Milestone: "M4", Classification: "public-api",
		Partial:   &catalog.Partial{Supported: []string{"public SDK model set/cycle, available/scoped snapshot filtering, thinking set/cycle/capability levels and preference restoration", "dynamic authentication and subsequent runtime requests, active Tool registry restrictions with prompt rebuild, v3 model/thinking restore", "busy rejection, ordered thinking notifications, immutable Session snapshots and persistence failure rollback"}, Unsupported: []string{"extension model/thinking hooks and active-tool changes within a running turn, RPC/TUI controls, full ToolDefinition metadata and custom extension registry remain deferred", "runtime execution remains limited to existing DeepSeek/OpenAI Chat Completions Adapter; no models.json overlays or dynamic Provider registration", "cross-file crash atomicity and storage backends that partially write before returning an error are outside the existing persistence guarantee"}},
		Deviation: &catalog.Deviation{ADR: "docs/adr/0025-session-configuration.md", Reason: "Serialize public Session configuration against complete runs and synchronous notifications; reject unknown/restricted/duplicate tools and invalid model/thinking; stage persistence and report write errors."},
		Notes:     "Issue #87; fixed Pi public createAgentSession source/dist Oracle and public SDK/real HTTP/race tests. Tools and scoped models do not persist. No expanded Adapter or M4-wide freeze claim.",
	}
}
func issue87ConfigurationEvidence(t *testing.T) []issue32ModuleEvidenceDescriptor {
	t.Helper()
	var out []issue32ModuleEvidenceDescriptor
	for _, e := range []struct{ kind, path, run string }{
		{"oracle", "parity/oracle/fixtures/session-configuration.json", "node --experimental-strip-types parity/oracle/session-configuration.mjs <locked-pi-checkout> --check"},
		{"go-test", "codingagent/issue87_configuration_test.go", "go test ./codingagent -run '^TestSessionConfigurationParity$' -count=1"},
		{"go-test", "codingagent/issue87_runtime_test.go", "go test -race ./codingagent -run '^TestSessionConfiguration' -count=1"},
	} {
		out = append(out, issue32ModuleEvidenceDescriptor{InputPath: e.path, Evidence: catalog.Evidence{Kind: e.kind, Ref: e.path, Baseline: issue32BaselineCommit, CaseID: "issue87-" + e.path, ExecutionMethod: e.run, Expected: "fixed Pi model/thinking/scope/tool configuration observations and safe public SDK request/persistence boundaries", Actual: "PASS; Oracle comparison, runtime requests, auth refresh, restore, invalid/busy and rollback cases", Platform: "any", CatalogID: issue87ConfigurationCatalogID}})
	}
	return out
}
func issue87PromoteRuntimeEntry(entry *catalog.Entry) bool {
	switch entry.ID {
	case "member:codingagent/src/core/agent-session.ts#AgentSession.setModel", "member:codingagent/src/core/agent-session.ts#AgentSession.cycleModel", "member:codingagent/src/core/agent-session.ts#AgentSession.setThinkingLevel", "member:codingagent/src/core/agent-session.ts#AgentSession.cycleThinkingLevel", "member:codingagent/src/core/agent-session.ts#AgentSession.getAvailableThinkingLevels", "member:codingagent/src/core/agent-session.ts#AgentSession.supportsThinking", "member:codingagent/src/core/agent-session.ts#AgentSession.setScopedModels", "member:codingagent/src/core/agent-session.ts#AgentSession.scopedModels", "member:codingagent/src/core/agent-session.ts#AgentSession.setActiveToolsByName", "member:codingagent/src/core/agent-session.ts#AgentSession.getAllTools", "member:codingagent/src/core/agent-session.ts#AgentSession.getActiveToolNames":
		entry.Status = catalog.StatusPartial
		entry.Partial = &catalog.Partial{Supported: []string{"public SDK configuration and immutable metadata snapshots within the shipped Tool/ModelRuntime scope"}, Unsupported: []string{"extension hooks/full ToolInfo metadata and changes within a running Prompt remain deferred; see contract:codingagent/session-configuration"}}
		entry.Deviation = &catalog.Deviation{ADR: "docs/adr/0025-session-configuration.md", Reason: "Explicit failures and serialized Session configuration under Go snapshot ownership."}
		entry.Evidence = []catalog.Evidence{{Kind: "go-test", Ref: "codingagent/issue87_runtime_test.go", Baseline: issue32BaselineCommit, CaseID: entry.ID, InputHash: issue87ConfigurationTestHash, ExecutionMethod: "go test -race ./codingagent -run '^TestSessionConfiguration' -count=1", Expected: "public SDK observes model/thinking/tool state, request changes and failure atomicity", Actual: "PASS; fixed Pi parity, real HTTP, restore, restriction and concurrent configuration checks", Platform: "any", CatalogID: entry.ID}}
		entry.Notes = "Issue #87 implements the shipped SDK configuration slice; details and Oracle evidence in " + issue87ConfigurationCatalogID + "."
		return true
	}
	return false
}

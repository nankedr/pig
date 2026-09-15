package codingagent_test

import (
	"strings"
	"testing"

	"github.com/nankedr/pig/internal/catalog"
)

const issue106ReloadCatalogID = "contract:codingagent/session-reload"

func issue106ReloadCatalogEntry() catalog.Entry {
	return catalog.Entry{SchemaVersion: catalog.SchemaVersion, ID: issue106ReloadCatalogID, Upstream: catalog.Upstream{Module: "coding-agent", Repository: "https://github.com/badlogic/pi-mono", Commit: issue32BaselineCommit, Reference: "packages/coding-agent/src/core/agent-session.ts"}, Mapping: catalog.Mapping{Module: "codingagent", Target: issue32GoPackage + ".AgentSession.Reload", Kind: "contract"}, Status: catalog.StatusPartial, Milestone: "M5", Classification: "public-api", Partial: &catalog.Partial{Supported: []string{
		"Issue #106: public AgentSession.Reload refreshes settings and queue modes, then local Context Files, system/append prompts, templates, Skills and Themes; add/edit/delete effects reach queries and subsequent real SDK generation",
		"Locked Pi public session fixture verifies failure stage semantics and reload while a Provider request remains active; no history, Session file/ID, branch, model/thinking or Tool reset",
		"SDK acceptance covers cancellation before/during reload, query failure and recovery, concurrent owned queries, command admission, trust preservation/revocation and reopened branch persistence",
		"Default loader publishes local resources together after cancellation checks; prompt options publish only after successful resource queries and final context/lifecycle checks",
	}, Unsupported: []string{
		"Extension runtime reload, trust hooks, overrides, ExtendResources, extension ABI and beforeSessionStart callbacks remain explicit stubs; unassembled scaffolded NewAgentSession instances reject Reload; no new RPC wire command",
		"Package manifests/registration, npm/git, dependencies and lifecycle remain deferred under #99; custom loaders own their publication transaction; six-platform runtime parity remains unverified",
	}}, Deviation: &catalog.Deviation{ADR: "docs/adr/0035-session-resource-reload.md", Reason: "Go cancellation keeps completed stages and refuses final prompt publication; concurrent reload, new message/configuration/replacement admission is rejected while an existing turn can continue. Established trust is preserved, with project settings gated before reads under ADR-0010."}}
}

func issue106ReloadEvidence(t *testing.T) []issue32ModuleEvidenceDescriptor {
	t.Helper()
	out := []issue32ModuleEvidenceDescriptor{}
	for _, e := range []struct{ kind, path, run string }{
		{"oracle", "parity/oracle/fixtures/session-reload.json", "node --experimental-strip-types parity/oracle/session-reload.mjs <locked-pi-checkout> --check"},
		{"go-test", "codingagent/issue106_reload_test.go", "go test ./codingagent -run '^TestSessionReloadSDKParity$' -count=1"},
		{"go-test", "codingagent/issue106_lifecycle_test.go", "go test -race ./codingagent -run '^TestSessionReload(CancelFailureAndRecovery|SettingsTrustAndBranch|ConcurrentQueries|AddedSkillsThemesAndSettings|RejectsUnassembledConstructor)$' -count=1"},
		{"go-test", "codingagent/issue106_trust_unix_test.go", "go test ./codingagent -run '^TestSessionReloadPreTrustDoesNotReadProjectSettings$' -count=1"},
		{"go-test", "codingagent/issue106_surface_test.go", "go test ./codingagent -run '^TestIssue106ReloadAPISnapshot$' -count=1"},
		{"manual", "examples/session-reload/main.go", "go run ./examples/session-reload"},
	} {
		out = append(out, issue32ModuleEvidenceDescriptor{InputPath: e.path, Evidence: catalog.Evidence{Kind: e.kind, Ref: e.path, Baseline: issue32BaselineCommit, CaseID: "issue106-" + e.path, ExecutionMethod: e.run, Expected: "locked Pi local reload stages, public queries, continued generation and preserved session", Actual: "PASS; public SDK parity, cancellation, trust, concurrent generation/queries and branch persistence", Platform: "darwin", CatalogID: issue106ReloadCatalogID}})
	}
	return out
}

func issue106PromoteRuntimeEntry(e *catalog.Entry) bool {
	_, member, _ := strings.Cut(e.ID, "#")
	if member != "AgentSession.reload" && member != "DefaultResourceLoader.reload" {
		return false
	}
	e.Status = catalog.StatusPartial
	e.Partial = issue106ReloadCatalogEntry().Partial
	e.Evidence = []catalog.Evidence{{Kind: "go-test", Ref: "codingagent/issue106_reload_test.go", Baseline: issue32BaselineCommit, CaseID: e.ID, InputHash: issue106ReloadFixtureHash, ExecutionMethod: "go test ./codingagent -run '^TestSessionReload' -count=1", Expected: "local session reload, continued generation and preserved state", Actual: "PASS; public SDK reload parity", Platform: "darwin", CatalogID: e.ID}}
	e.Notes = "Issue #106 session resource reload; precise scope and evidence: " + issue106ReloadCatalogID + "."
	return true
}

const issue106ReloadFixtureHash = "sha256:6c941b07fc7f25418896a8823521c0170b7884c6db74c3edbae8ac463e1ffc0d"

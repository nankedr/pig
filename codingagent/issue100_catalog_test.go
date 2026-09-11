package codingagent_test

import (
	"github.com/nankedr/pig/internal/catalog"
	"testing"
)

const issue100ContextCatalogID = "contract:codingagent/context-files"

func issue100ContextCatalogEntry() catalog.Entry {
	return catalog.Entry{SchemaVersion: catalog.SchemaVersion, ID: issue100ContextCatalogID, Upstream: catalog.Upstream{Module: "coding-agent", Repository: "https://github.com/badlogic/pi-mono", Commit: issue32BaselineCommit, Reference: "packages/coding-agent/src/core/resource-loader.ts"}, Mapping: catalog.Mapping{Module: "codingagent", Target: issue32GoPackage + ".NewDefaultResourceLoader", Kind: "contract"}, Status: catalog.StatusPartial, Milestone: "M5", Classification: "public-api", Partial: &catalog.Partial{Supported: []string{
		"Issue #100: default and injected ResourceLoader feed Context Files into real SDK/CLI model requests on startup and resume; Session services share the loaded resource instance; no default package manager dependency",
		"Locked Pi fixture: global-first candidate priority, ancestors through filesystem root, empty and missing files, unreadable fallback diagnostics, lexical symlink paths, file URLs, global path dedup and valid/incomplete nested worktree metadata",
		"NoContextFiles and CLI --no-context-files/-nc suppress reads and model context; Context Files remain available without project trust, preserving ADR-0010 pre-trust settings protection; public paths and defensive file/diagnostic snapshots; cancel-safe local Reload",
	}, Unsupported: []string{
		"Skill, Prompt Template, Theme, system/append prompt resources and ExtendResources remain explicit stubs pending subsequent M5 slices; opaque overrides and extension execution/ABI remain M7",
		"Package manifest/registration, npm/git, dependency installation and lifecycle remain stubs and are deferred under #99; this local chain does not install or execute packages; live session resource reload orchestration remains deferred",
	}}, Deviation: &catalog.Deviation{ADR: "docs/adr/0010-trust-and-host-security.md", Reason: "Resolve trust before reading trust-sensitive project settings. Local Context File loading never invokes a package manager or opaque extension callback."}}
}

func issue100ContextEvidence(t *testing.T) []issue32ModuleEvidenceDescriptor {
	t.Helper()
	out := []issue32ModuleEvidenceDescriptor{}
	for _, e := range []struct{ kind, path, run string }{
		{"oracle", "parity/oracle/fixtures/context-files.json", "node --experimental-strip-types parity/oracle/context-files.mjs <locked-pi-checkout> --check"},
		{"go-test", "codingagent/issue100_context_test.go", "go test -race ./codingagent -run '^TestContextFiles' -count=1"},
		{"go-test", "codingagent/issue100_injected_test.go", "go test ./codingagent -run '^TestContextFilesRuntimeInjectionDoesNotDiscover$' -count=1"},
		{"go-test", "cmd/pig/issue100_context_test.go", "go test ./cmd/pig -run '^TestPigContextFiles' -count=1"},
		{"go-test", "cmd/pig/issue75_process_test.go", "go test ./cmd/pig -run '^TestPigUntrustedProjectHasNoSensitiveReadsOrEffects$' -count=1"},
		{"go-test", "codingagent/issue100_surface_test.go", "go test ./codingagent -run '^TestIssue100ContextAPISnapshot$' -count=1"},
	} {
		out = append(out, issue32ModuleEvidenceDescriptor{InputPath: e.path, Evidence: catalog.Evidence{Kind: e.kind, Ref: e.path, Baseline: issue32BaselineCommit, CaseID: "issue100-" + e.path, ExecutionMethod: e.run, Expected: "locked Pi Context File selection/source/order and actual SDK/CLI model input, diagnostics, resume and trust boundary", Actual: "PASS; Faux Provider generations and real CLI requests, ResourceLoader ownership and unsupported boundaries", Platform: "darwin/linux non-root", CatalogID: issue100ContextCatalogID}})
	}
	return out
}

func issue100PromoteRuntimeEntry(e *catalog.Entry) bool {
	switch e.ID {
	case "symbol:codingagent/src/core/resource-loader.ts#DefaultResourceLoader", "constructor:codingagent/src/core/resource-loader.ts#DefaultResourceLoader", "member:codingagent/src/core/resource-loader.ts#DefaultResourceLoader.getAgentsFiles", "member:codingagent/src/core/resource-loader.ts#DefaultResourceLoader.reload", "symbol:codingagent/src/core/resource-loader.ts#loadProjectContextFiles":
	default:
		return false
	}
	e.Status = catalog.StatusPartial
	e.Partial = issue100ContextCatalogEntry().Partial
	e.Evidence = []catalog.Evidence{{Kind: "go-test", Ref: "codingagent/issue100_context_test.go", Baseline: issue32BaselineCommit, CaseID: e.ID, InputHash: "sha256:1e1d2ebd5f02745018ff137fa52311563801a31cf5136095b34f08b671e7322d", ExecutionMethod: "go test ./codingagent -run '^TestContextFiles' -count=1", Expected: "public local Context File loader/session chain", Actual: "PASS; fixed Pi fixture and SDK Faux Provider generation", Platform: "darwin/linux non-root", CatalogID: e.ID}}
	e.Notes = "Issue #100 local Context File behavior and exact deferred branches: " + issue100ContextCatalogID + "."
	return true
}

package codingagent_test

import (
	"github.com/nankedr/pig/internal/catalog"
	"testing"
)

const issue107ExtensionCatalogID = "contract:codingagent/local-extensions"

func issue107ExtensionCatalogEntry() catalog.Entry {
	return catalog.Entry{SchemaVersion: catalog.SchemaVersion, ID: issue107ExtensionCatalogID, Upstream: catalog.Upstream{Module: "coding-agent", Repository: "https://github.com/badlogic/pi-mono", Commit: issue32BaselineCommit, Reference: "packages/coding-agent/src/core/package-manager.ts"}, Mapping: catalog.Mapping{Module: "codingagent", Target: issue32GoPackage + ".DefaultResourceLoader.GetExtensionDiscovery", Kind: "contract"}, Status: catalog.StatusPartial, Milestone: "M5", Classification: "public-api", Partial: &catalog.Partial{Supported: []string{
		"Issue #107: public SDK discovery of local global, trusted project, settings and explicit paths with source metadata; index.ts/index.js, ignore rules, selection filters and canonical deduplication without importing modules",
		"Pinned Pi failed-load fixture supplies discovery paths/order/metadata and invalid-factory failures; Pig deliberately substitutes structured NotImplementedError/ErrNotImplemented for execution",
		"Real CLI explicit -e diagnostics and nonzero exit, unknown long flag routing, no-extensions plus explicit paths, ordinary template commands remain usable with discovery warnings",
		"Public session reload updates discovery only; FIFO trust/manifest probes, process and network traps, owned snapshots, cancellation and unchanged filesystem validate side-effect-free discovery",
	}, Unsupported: []string{
		"Extension execution, factories, trust hooks, commands, override/resource callbacks, ExtendResources and extension-runtime reload remain stubs; no language, callback or loading ABI frozen before M7 research/ADR",
		"Package manifest/registration, npm/git acquisition, dependencies and lifecycle deferred under #99; complex minimatch extglob and six-platform runtime parity remain unverified",
	}}, Deviation: &catalog.Deviation{ADR: "docs/adr/0034-defer-package-ecosystem.md", Reason: "Discovery is a separate data-only Go query; explicit directories use local index/one-level scanning without manifest resolution, explicit origin is top-level rather than Pi package-manager package metadata. Extension execution remains a structured stub, and auto candidates only produce not-executed diagnostics. ADR-0010 gates project reads before trust."}}
}

func issue107ExtensionEvidence(t *testing.T) []issue32ModuleEvidenceDescriptor {
	t.Helper()
	out := []issue32ModuleEvidenceDescriptor{}
	for _, e := range []struct{ kind, path, run string }{
		{"oracle", "parity/oracle/fixtures/local-extensions.json", "node --experimental-strip-types parity/oracle/local-extensions.mjs <locked-pi-checkout> --check"},
		{"go-test", "codingagent/issue107_extensions_test.go", "go test -race ./codingagent -run '^TestLocalExtensions' -count=1"},
		{"go-test", "codingagent/issue107_trust_unix_test.go", "go test ./codingagent -run '^TestLocalExtensionsNoSensitiveReads$' -count=1"},
		{"go-test", "cmd/pig/issue107_extensions_test.go", "go test ./cmd/pig -run '^TestPigLocalExtensions' -count=1"},
		{"go-test", "codingagent/issue107_surface_test.go", "go test ./codingagent -run '^TestIssue107ExtensionAPISnapshot$' -count=1"},
		{"manual", "examples/local-extensions/main.go", "go run ./examples/local-extensions"},
	} {
		out = append(out, issue32ModuleEvidenceDescriptor{InputPath: e.path, Evidence: catalog.Evidence{Kind: e.kind, Ref: e.path, Baseline: issue32BaselineCommit, CaseID: "issue107-" + e.path, ExecutionMethod: e.run, Expected: "local discovery with source metadata and explicit execution failure without side effects", Actual: "PASS; public SDK discovery and reload, CLI diagnostics and no-execution probes", Platform: "darwin", CatalogID: issue107ExtensionCatalogID}})
	}
	return out
}

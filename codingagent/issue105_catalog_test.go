package codingagent_test

import (
	"strings"
	"testing"

	"github.com/nankedr/pig/internal/catalog"
)

const issue105ResourceCatalogID = "contract:codingagent/local-resources"

func issue105ResourceCatalogEntry() catalog.Entry {
	return catalog.Entry{SchemaVersion: catalog.SchemaVersion, ID: issue105ResourceCatalogID, Upstream: catalog.Upstream{Module: "coding-agent", Repository: "https://github.com/badlogic/pi-mono", Commit: issue32BaselineCommit, Reference: "packages/coding-agent/src/core/resource-loader.ts"}, Mapping: catalog.Mapping{Module: "codingagent", Target: issue32GoPackage + ".DefaultResourceLoader", Kind: "contract"}, Status: catalog.StatusPartial, Milestone: "M5", Classification: "public-api", Partial: &catalog.Partial{Supported: []string{
		"Issue #105: 19 controlled cross-resource scenarios from the locked Pi baseline cover project/global settings, auto discovery, ancestor .agents and home, explicit paths, filtering, canonical path deduplication and first-name wins without resource-name sorting",
		"Disabled paths reserve canonical identity before filtering; explicit paths can reenable resources and preserve discovery metadata even when default discovery is disabled",
		"Resource-specific traversal: settings prompt/theme directories recurse, auto and explicit prompt/theme directories do not; Skill recursion, git-root boundaries, ignore rules and symlinks remain distinct",
		"Public SDK session prompt expansion, skill lists and selected Theme ANSI effects agree with resource queries; trust rejection preserves global and explicitly authorized resources",
		"Collision queries expose winner/loser paths and scope:source labels with owned snapshots; real CLI diagnostics display both sources and RPC command queries agree with SDK winners",
	}, Unsupported: []string{
		"Package source priority, manifests/registration, npm/git, dependencies and lifecycle remain deferred under #99 and ADR-0034; no implicit installation or extension execution/ABI freeze",
		"Extension reload orchestration, extension-provided resources and full minimatch extglob remain outside this slice; six-platform runtime parity remains unverified",
	}}, Deviation: &catalog.Deviation{ADR: "docs/adr/0010-trust-and-host-security.md", Reason: "Project resources are gated before reads; global and explicit paths are independently authorized. Collision source labels supplement the pinned diagnostics without changing their winner/loser paths or order."}}
}

func issue105ResourceEvidence(t *testing.T) []issue32ModuleEvidenceDescriptor {
	t.Helper()
	out := []issue32ModuleEvidenceDescriptor{}
	for _, e := range []struct{ kind, path, run string }{
		{"oracle", "parity/oracle/fixtures/local-resources.json", "node --experimental-strip-types parity/oracle/local-resources.mjs <locked-pi-checkout> --check"},
		{"go-test", "codingagent/issue105_resources_test.go", "go test ./codingagent -run '^TestLocalResource' -count=1"},
		{"go-test", "cmd/pig/issue105_resources_test.go", "go test ./cmd/pig -run '^TestPigLocalResourceDiagnostics$' -count=1"},
		{"go-test", "codingagent/issue105_surface_test.go", "go test ./codingagent -run '^TestIssue105ResourceAPISnapshot$' -count=1"},
		{"manual", "examples/local-resources/main.go", "go run ./examples/local-resources"},
	} {
		out = append(out, issue32ModuleEvidenceDescriptor{InputPath: e.path, Evidence: catalog.Evidence{Kind: e.kind, Ref: e.path, Baseline: issue32BaselineCommit, CaseID: "issue105-" + e.path, ExecutionMethod: e.run, Expected: "locked Pi local resource order, enablement, metadata, conflicts and public effects", Actual: "PASS; cross-resource public SDK parity and real CLI source diagnostics", Platform: "darwin", CatalogID: issue105ResourceCatalogID}})
	}
	return out
}

func issue105PromoteRuntimeEntry(e *catalog.Entry) bool {
	if !strings.Contains(e.ID, "core/diagnostics.ts#ResourceCollision") && !strings.Contains(e.ID, "core/diagnostics.ts#ResourceDiagnostic") {
		return false
	}
	e.Status = catalog.StatusPartial
	e.Partial = issue105ResourceCatalogEntry().Partial
	e.Evidence = []catalog.Evidence{{Kind: "go-test", Ref: "codingagent/issue105_resources_test.go", Baseline: issue32BaselineCommit, CaseID: e.ID, InputHash: issue105ResourceFixtureHash, ExecutionMethod: "go test ./codingagent -run '^TestLocalResource' -count=1", Expected: "winner/loser paths, source labels and owned collision queries", Actual: "PASS; public SDK local resource diagnostics", Platform: "darwin", CatalogID: e.ID}}
	e.Notes = "Issue #105 local resources; precise scope and evidence: " + issue105ResourceCatalogID + "."
	return true
}

const issue105ResourceFixtureHash = "sha256:c587856d0e803b99844e340f0e7c0003e29e7dd42948e9d1c2cb5f147fd13fc2"

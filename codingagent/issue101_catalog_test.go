package codingagent_test

import (
	"strings"
	"testing"

	"github.com/nankedr/pig/internal/catalog"
)

const issue101PromptCatalogID = "contract:codingagent/system-prompts"

func issue101PromptCatalogEntry() catalog.Entry {
	return catalog.Entry{SchemaVersion: catalog.SchemaVersion, ID: issue101PromptCatalogID, Upstream: catalog.Upstream{Module: "coding-agent", Repository: "https://github.com/badlogic/pi-mono", Commit: issue32BaselineCommit, Reference: "packages/coding-agent/src/core/resource-loader.ts"}, Mapping: catalog.Mapping{Module: "codingagent", Target: issue32GoPackage + ".NewDefaultResourceLoader", Kind: "contract"}, Status: catalog.StatusPartial, Milestone: "M5", Classification: "public-api", Partial: &catalog.Partial{Supported: []string{
		"Issue #102 adds local Prompt Template discovery, invocation and RPC command queries; exact evidence: contract:codingagent/prompt-templates",
		"Issue #101: global and trusted project SYSTEM.md/APPEND_SYSTEM.md discovery, independent explicit text/file inputs, project-over-global precedence, nil versus empty options/files, ordered replacement append lists, source snapshots and deterministic read-failure warnings",
		"CLI, Headless, default/injected SDK and Session services feed loaded prompts and Context Files into actual model inputs; tool selection rebuild preserves resource content; disabled unrelated resources do not disable system prompts",
		"Trust gates project settings and prompt reads from the first load; global defaults and saved decisions are resolved before project reads, explicit resources retain independent trust, Headless ask fails closed; local Reload publishes owned snapshots only after successful completion",
	}, Unsupported: []string{
		"SystemPromptOverride, AppendSystemPromptOverride, ResolveProjectTrust and LoadProjectTrustExtensions remain explicit M7 Capability Stubs; no extension execution or frozen extension ABI",
		"Skill, Theme and live Session resource reload orchestration remain later M5 slices; package manifest/registration, npm/git installation, dependencies and lifecycle remain deferred under #99",
	}}, Deviation: &catalog.Deviation{ADR: "docs/adr/0010-trust-and-host-security.md", Reason: "Resolve trust before project settings reads. Non-regular prompt inputs produce a warning and retain the input value instead of opening a device or blocking on a FIFO."}}
}

func issue101PromptEvidence(t *testing.T) []issue32ModuleEvidenceDescriptor {
	t.Helper()
	out := []issue32ModuleEvidenceDescriptor{}
	for _, e := range []struct{ kind, path, run string }{
		{"oracle", "parity/oracle/fixtures/system-prompts.json", "node --experimental-strip-types parity/oracle/system-prompts.mjs <locked-pi-checkout> --check"},
		{"go-test", "codingagent/issue101_prompt_test.go", "go test ./codingagent -run '^TestSystemPromptsSDKParity$' -count=1"},
		{"go-test", "codingagent/issue101_trust_test.go", "go test ./codingagent -run '^TestSystemPrompts(TrustAndReload|InjectedFailuresAndOwnership|SDKSettingsTrust)$' -count=1"},
		{"go-test", "cmd/pig/issue101_prompt_test.go", "go test ./cmd/pig -run '^TestPigSystemPrompts$' -count=1"},
		{"go-test", "cmd/pig/issue75_process_test.go", "go test ./cmd/pig -run '^TestPigUntrustedProjectHasNoSensitiveReadsOrEffects$' -count=1"},
		{"go-test", "codingagent/issue100_context_test.go", "go test ./codingagent -run '^TestContextFilesSessionServices$' -count=1"},
		{"go-test", "codingagent/issue101_surface_test.go", "go test ./codingagent -run '^TestIssue101PromptAPISnapshot$' -count=1"},
		{"manual", "examples/system-prompts/main.go", "go run ./examples/system-prompts"},
	} {
		out = append(out, issue32ModuleEvidenceDescriptor{InputPath: e.path, Evidence: catalog.Evidence{Kind: e.kind, Ref: e.path, Baseline: issue32BaselineCommit, CaseID: "issue101-" + e.path, ExecutionMethod: e.run, Expected: "fixed Pi prompt precedence/empty/source/failure semantics and actual SDK/CLI generation with first-load trust", Actual: "PASS; public sessions and CLI requests retain system/append resources and Context Files across tool selection, with deterministic diagnostics and explicit stubs", Platform: "darwin/linux non-root", CatalogID: issue101PromptCatalogID}})
	}
	return out
}

func issue101PromoteRuntimeEntry(e *catalog.Entry) bool {
	if !strings.Contains(e.ID, "core/resource-loader.ts#") {
		return false
	}
	_, member, _ := strings.Cut(e.ID, "#")
	switch member {
	case "DefaultResourceLoader.getSystemPrompt", "DefaultResourceLoader.getSystemPromptSource", "DefaultResourceLoader.getAppendSystemPrompt", "DefaultResourceLoader.getAppendSystemPromptSources":
	default:
		return false
	}
	e.Status = catalog.StatusPartial
	e.Partial = issue101PromptCatalogEntry().Partial
	e.Evidence = []catalog.Evidence{{Kind: "go-test", Ref: "codingagent/issue101_prompt_test.go", Baseline: issue32BaselineCommit, CaseID: e.ID, InputHash: issue101PromptFixtureHash, ExecutionMethod: "go test ./codingagent -run '^TestSystemPromptsSDKParity$' -count=1", Expected: "locked Pi selection, sources and model-visible prompts", Actual: "PASS; real SDK generations before and after tool selection", Platform: "darwin/linux non-root", CatalogID: e.ID}}
	e.Notes = "Issue #101 local prompt behavior; exact scope and evidence: " + issue101PromptCatalogID + "."
	return true
}

const issue101PromptFixtureHash = "sha256:7cded7bf214e6b1281d213fa7045fe6d9cba24734e79e687244d71c363675e66"

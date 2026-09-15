package codingagent_test

import (
	"strings"
	"testing"

	"github.com/nankedr/pig/internal/catalog"
)

const issue102TemplateCatalogID = "contract:codingagent/prompt-templates"

func issue102TemplateCatalogEntry() catalog.Entry {
	return catalog.Entry{SchemaVersion: catalog.SchemaVersion, ID: issue102TemplateCatalogID, Upstream: catalog.Upstream{Module: "coding-agent", Repository: "https://github.com/badlogic/pi-mono", Commit: issue32BaselineCommit, Reference: "packages/coding-agent/src/core/prompt-templates.ts"}, Mapping: catalog.Mapping{Module: "codingagent", Target: issue32GoPackage + ".AgentSession.PromptTemplates", Kind: "contract"}, Status: catalog.StatusPartial, Milestone: "M5", Classification: "public-api", Partial: &catalog.Partial{Supported: []string{
		"Issue #102: local project/global settings and default directories, explicit files/directories/file URLs, disabled discovery with explicit paths, first-load project trust, project/local precedence, canonical path deduplication and name collision/source diagnostics",
		"YAML frontmatter/body/newlines, descriptions and argument hints, malformed/duplicate metadata, aliases, scalar/list/null roots, YAML 1.2 dates and non-merge keys; settings recursive directories, ignore files and wildcard/exact inclusion/exclusion",
		"Real CLI subprocess and public SDK model inputs/replies match locked Pi fixtures for positional and quoted args, defaults, slices, unknown commands, nonrecursive substitution and expansion opt-out; local Reload publishes owned snapshots",
		"Public AgentSession.PromptTemplates and ResourceLoader.GetPrompts; existing JSONL RPC get_commands returns prompt/source information without extension registration; normal Prompt and streaming queues expand templates while SendUserMessage bypasses expansion",
	}, Unsupported: []string{
		"Package manifest/registration, npm/git, dependencies and lifecycle remain deferred under #99; extension commands, opaque PromptsOverride and ExtendResources retain M7 Capability Stubs; Theme is delivered in #104; local Session reload is delivered by #106; extension reload remains deferred",
		"General-purpose YAML schema parity outside the typed template metadata contract and full minimatch extglob grammar are not verified by this local-resource slice; six-platform runtime parity remains M13",
	}}, Deviation: &catalog.Deviation{ADR: "docs/adr/0010-trust-and-host-security.md", Reason: "Project settings and templates are read only after trust; explicit paths are independently authorized. Non-regular files are skipped, and recursive symlink cycles are not followed."}}
}

func issue102TemplateEvidence(t *testing.T) []issue32ModuleEvidenceDescriptor {
	t.Helper()
	out := []issue32ModuleEvidenceDescriptor{}
	for _, e := range []struct{ kind, path, run string }{
		{"oracle", "parity/oracle/fixtures/prompt-templates.json", "node --experimental-strip-types parity/oracle/prompt-templates.mjs <locked-pi-checkout> --check"},
		{"go-test", "codingagent/issue102_templates_test.go", "go test ./codingagent -run '^TestPromptTemplates(SDKParity|StreamingQueuesAndSendUserMessage)$' -count=1"},
		{"go-test", "codingagent/issue102_trust_test.go", "go test ./codingagent -run '^TestPromptTemplatesTrustReloadAndOwnership$' -count=1"},
		{"go-test", "cmd/pig/issue102_templates_test.go", "go test ./cmd/pig -run 'Test(PigPromptTemplates|RPC102TemplateCommands)$' -count=1"},
		{"go-test", "cmd/pig/issue75_process_test.go", "go test ./cmd/pig -run '^TestPigUntrustedProjectHasNoSensitiveReadsOrEffects$' -count=1"},
		{"go-test", "codingagent/issue102_surface_test.go", "go test ./codingagent -run '^TestIssue102TemplateAPISnapshot$' -count=1"},
		{"manual", "examples/prompt-templates/main.go", "go run ./examples/prompt-templates"},
	} {
		out = append(out, issue32ModuleEvidenceDescriptor{InputPath: e.path, Evidence: catalog.Evidence{Kind: e.kind, Ref: e.path, Baseline: issue32BaselineCommit, CaseID: "issue102-" + e.path, ExecutionMethod: e.run, Expected: "locked Pi template discovery, metadata, source/diagnostics and expanded model inputs with normal replies", Actual: "PASS; local public SDK/CLI/RPC template contract, first-load trust and owned reload snapshots", Platform: "darwin/linux", CatalogID: issue102TemplateCatalogID}})
	}
	return out
}

func issue102PromoteRuntimeEntry(e *catalog.Entry) bool {
	_, member, _ := strings.Cut(e.ID, "#")
	matches := strings.Contains(e.ID, "codingagent/src/core/prompt-templates.ts#PromptTemplate") || strings.Contains(e.ID, "codingagent/src/utils/frontmatter.ts#")
	for _, name := range []string{"AgentSession.promptTemplates", "PromptOptions.expandPromptTemplates", "DefaultResourceLoader.getPrompts", "DefaultResourceLoaderOptions.additionalPromptTemplatePaths", "DefaultResourceLoaderOptions.noPromptTemplates"} {
		matches = matches || member == name
	}
	if !matches {
		return false
	}
	e.Status = catalog.StatusPartial
	e.Partial = issue102TemplateCatalogEntry().Partial
	e.Evidence = []catalog.Evidence{{Kind: "go-test", Ref: "codingagent/issue102_templates_test.go", Baseline: issue32BaselineCommit, CaseID: e.ID, InputHash: issue102TemplateFixtureHash, ExecutionMethod: "go test ./codingagent -run '^TestPromptTemplates(SDKParity|StreamingQueuesAndSendUserMessage)$' -count=1", Expected: "locked Pi template queries and model-visible expansion", Actual: "PASS; public SDK sessions produce normal replies from expanded inputs", Platform: "darwin/linux", CatalogID: e.ID}}
	e.Notes = "Issue #102 local Prompt Template behavior; exact scope and evidence: " + issue102TemplateCatalogID + "."
	return true
}

const issue102TemplateFixtureHash = "sha256:84e27b1c2b10e4c01fdbf2140bb9a4bb1fa98e9c1e8550752c9c5942a01aa3ff"

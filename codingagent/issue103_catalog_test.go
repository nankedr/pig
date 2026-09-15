package codingagent_test

import (
	"strings"
	"testing"

	"github.com/nankedr/pig/internal/catalog"
)

const issue103SkillCatalogID = "contract:codingagent/skills"

func issue103SkillCatalogEntry() catalog.Entry {
	return catalog.Entry{SchemaVersion: catalog.SchemaVersion, ID: issue103SkillCatalogID, Upstream: catalog.Upstream{Module: "coding-agent", Repository: "https://github.com/badlogic/pi-mono", Commit: issue32BaselineCommit, Reference: "packages/coding-agent/src/core/skills.ts"}, Mapping: catalog.Mapping{Module: "codingagent", Target: issue32GoPackage + ".DefaultResourceLoader.GetSkills", Kind: "contract"}, Status: catalog.StatusPartial, Milestone: "M5", Classification: "public-api", Partial: &catalog.Partial{Supported: []string{
		"Issue #103: local global/trusted project/ancestor .agents discovery, git-root boundary, explicit file/directory/URL paths, disabled discovery with explicit paths and settings patterns",
		"Skill frontmatter name/description validation, disable-model-invocation, source metadata, real-path deduplication, same-name collisions, ignore files and SKILL.md recursion boundaries",
		"Public SDK sessions and real CLI subprocesses match locked Pi fixtures for model-visible lists gated by read Tool and explicit skill blocks; unknown commands and expansion opt-out preserve original text",
		"RPC get_commands includes skill/source information and explicit-only skills; first-load trust, owned reload snapshots, malformed metadata, symlink cycles and streaming queue delivery are covered",
	}, Unsupported: []string{
		"Package manifest/registration, npm/git, dependencies and lifecycle are deferred under #99; opaque SkillsOverride, ExtendResources, extension error callbacks and extension reload orchestration remains deferred; local Session reload is delivered by #106",
		"General-purpose YAML error-text parity and full minimatch extglob grammar are not verified; six-platform runtime parity remains M13; auxiliary scripts require the host tools named by each skill",
	}}, Deviation: &catalog.Deviation{ADR: "docs/adr/0010-trust-and-host-security.md", Reason: "Project settings and skills are read only after trust; explicit paths are independently authorized. Non-regular files are skipped, and recursive symlink cycles are not followed."}}
}

func issue103SkillEvidence(t *testing.T) []issue32ModuleEvidenceDescriptor {
	t.Helper()
	out := []issue32ModuleEvidenceDescriptor{}
	for _, e := range []struct{ kind, path, run string }{
		{"oracle", "parity/oracle/fixtures/skills.json", "node --experimental-strip-types parity/oracle/skills.mjs <locked-pi-checkout> --check"},
		{"go-test", "codingagent/issue103_skills_test.go", "go test ./codingagent -run '^TestSkills(SDKParity|StreamingQueuesAndSendUserMessage|InvalidMetadataAndSymlinkCycle)$' -count=1"},
		{"go-test", "codingagent/issue103_trust_test.go", "go test ./codingagent -run '^TestSkills(TrustReloadAndOwnership|MissingHomeFailsBeforeRelativeDiscovery)$' -count=1"},
		{"go-test", "cmd/pig/issue103_skills_test.go", "go test ./cmd/pig -run 'Test(PigSkills|RPC103SkillCommands)$' -count=1"},
		{"go-test", "cmd/pig/issue75_process_test.go", "go test ./cmd/pig -run '^TestPigUntrustedProjectHasNoSensitiveReadsOrEffects$' -count=1"},
		{"go-test", "codingagent/issue103_surface_test.go", "go test ./codingagent -run '^TestIssue103SkillAPISnapshot$' -count=1"},
		{"manual", "examples/skills/main.go", "go run ./examples/skills"},
	} {
		out = append(out, issue32ModuleEvidenceDescriptor{InputPath: e.path, Evidence: catalog.Evidence{Kind: e.kind, Ref: e.path, Baseline: issue32BaselineCommit, CaseID: "issue103-" + e.path, ExecutionMethod: e.run, Expected: "locked Pi skill discovery, metadata, source/diagnostics and expanded model inputs with normal replies", Actual: "PASS; local public SDK/CLI/RPC skill contract, first-load trust and owned reload snapshots", Platform: "darwin/linux", CatalogID: issue103SkillCatalogID}})
	}
	return out
}

func issue103PromoteRuntimeEntry(e *catalog.Entry) bool {
	_, member, _ := strings.Cut(e.ID, "#")
	matches := strings.Contains(e.ID, "codingagent/src/core/skills.ts#")
	for _, name := range []string{"DefaultResourceLoader.getSkills", "DefaultResourceLoaderOptions.additionalSkillPaths", "DefaultResourceLoaderOptions.noSkills"} {
		matches = matches || member == name
	}
	if !matches {
		return false
	}
	e.Status = catalog.StatusPartial
	e.Partial = issue103SkillCatalogEntry().Partial
	e.Evidence = []catalog.Evidence{{Kind: "go-test", Ref: "codingagent/issue103_skills_test.go", Baseline: issue32BaselineCommit, CaseID: e.ID, InputHash: issue103SkillFixtureHash, ExecutionMethod: "go test ./codingagent -run '^TestSkills(SDKParity|StreamingQueuesAndSendUserMessage)$' -count=1", Expected: "locked Pi skill queries and model-visible expansion", Actual: "PASS; public SDK sessions produce normal replies from expanded inputs", Platform: "darwin/linux", CatalogID: e.ID}}
	e.Notes = "Issue #103 local Skill behavior; exact scope and evidence: " + issue103SkillCatalogID + "."
	return true
}

const issue103SkillFixtureHash = "sha256:da8ac8d6ee075629fd5635fe3630868cefc73ace3667daa9f0ce0373ed3cd16d"

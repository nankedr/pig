package codingagent_test

import (
	"github.com/nankedr/pig/internal/catalog"
	"testing"
)

const issue75TrustCatalogID = "contract:security/project-trust"

func issue75TrustCatalogEntry() catalog.Entry {
	return catalog.Entry{
		SchemaVersion: catalog.SchemaVersion, ID: issue75TrustCatalogID,
		Upstream: catalog.Upstream{Module: "coding-agent", Repository: "https://github.com/badlogic/pi-mono", Commit: issue32BaselineCommit, Reference: "packages/coding-agent/src/core/project-trust.ts"},
		Mapping:  catalog.Mapping{Module: "codingagent", Target: issue32GoPackage + ".CreateHeadlessSession", Kind: "contract"}, Status: catalog.StatusPartial, Milestone: "M3", Classification: "public-api",
		Partial:   &catalog.Partial{Supported: []string{"resource existence, explicit run-only override, no-resource trust, canonical nearest ancestor persistent decisions, global ask/always/never and headless fail closed", "locked Pig-only trust store with strict data validation, 0700 directory and 0600 file permissions, atomic batch updates and null deletion", "Headless resolves trust before project settings or Session path selection; trusted project settings deep-merge with global settings and revoke restores trusted configuration", "Context Files load independently of trust with candidate priority, ancestor layering, worktree deduplication and --no-context-files"}, Unsupported: []string{"TUI trust prompts remain M6; pre-trust extension execution remains M7", "project writes and the full package/resource/system-prompt runtime remain M5; no Tool approvals or workspace sandbox are introduced"}},
		Deviation: &catalog.Deviation{ADR: "docs/adr/0010-trust-and-host-security.md", Reason: "Pig never reads any project settings before trust, including sessionDir; SettingsManager defaults to untrusted. Trust state stays in Pig directories with restricted permissions."},
		Notes:     "Issue #75 uses fixed Pi SDK and real CLI fixtures plus FIFO file-access evidence. Explicitly injected SettingsManager is caller-owned trusted configuration; its project gate is controlled by its public API or an explicit Headless override. Context instructions remain visible to the model even in untrusted projects.",
	}
}

func issue75TrustEvidence(t *testing.T) []issue32ModuleEvidenceDescriptor {
	t.Helper()
	var descriptors []issue32ModuleEvidenceDescriptor
	for _, item := range []struct{ name, ref, run string }{{"project-trust", "internal/parity/project_trust_test.go#TestProjectTrustParity", "go test ./internal/parity -run '^TestProjectTrustParity$' -count=1"}, {"project-trust-startup", "cmd/pig/issue75_process_test.go#TestPigProjectTrustStartupParity", "go test ./cmd/pig -run '^TestPigProjectTrustStartupParity$' -count=1"}} {
		path := "parity/oracle/fixtures/" + item.name + ".json"
		for _, kind := range []string{catalog.MatrixEvidenceOracle, catalog.MatrixEvidenceGoTest} {
			ref, run := item.ref, item.run
			if kind == catalog.MatrixEvidenceOracle {
				ref = path
				run = "node --experimental-strip-types parity/oracle/" + item.name + ".mjs <locked-pi-checkout> --check"
			}
			descriptors = append(descriptors, issue32ModuleEvidenceDescriptor{InputPath: path, Evidence: catalog.Evidence{Kind: kind, Ref: ref, Baseline: issue32BaselineCommit, CaseID: "issue75-" + item.name, ExecutionMethod: run, Expected: "public trust decisions and real Headless project model/context observations match locked Pi", Actual: "PASS; deterministic SDK and CLI fixtures match without normalization", Platform: "any", CatalogID: issue75TrustCatalogID}})
		}
	}
	for _, item := range []struct{ file, test string }{{"codingagent/issue75_trust_test.go", "TestProjectTrustStoreConcurrentProcessesAndPermissions"}, {"codingagent/issue75_headless_test.go", "TestHeadlessProjectTrustPriority"}, {"codingagent/issue75_settings_test.go", "TestProjectSettingsLoadOnlyAfterTrustAndRevoke"}, {"codingagent/issue75_context_test.go", "TestProjectContextLayeringAndWorktreeDedup"}, {"cmd/pig/issue75_process_test.go", "TestPigUntrustedProjectHasNoSensitiveReadsOrEffects"}} {
		descriptors = append(descriptors, issue32ModuleEvidenceDescriptor{InputPath: item.file, Evidence: catalog.Evidence{Kind: "go-test", Ref: item.file + "#" + item.test, Baseline: issue32BaselineCommit, CaseID: "issue75-" + item.test, ExecutionMethod: "go test ./codingagent ./cmd/pig -run '^" + item.test + "$' -count=1", Expected: "trust decisions, merge/revoke, locked persistence and Context File exception are observable at public SDK or real CLI boundaries", Actual: "PASS; public behavior and file-access evidence preserve the project trust boundary", Platform: "darwin/linux", CatalogID: issue75TrustCatalogID}})
	}
	return descriptors
}

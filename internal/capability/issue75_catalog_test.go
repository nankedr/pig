package capability_test

import (
	"github.com/nankedr/pig/internal/catalog"
	"testing"
)

func issue75ExtendProductEntry(t *testing.T, entry *catalog.Entry) {
	switch entry.ID {
	case "cmd-pig", "contract:cli/pig/args", "contract:codingagent/headless":
	default:
		return
	}
	const caseID = "issue75-product-project-trust-startup"
	entry.Evidence = issue56UpsertEvidence(entry.Evidence, catalog.Evidence{Kind: "go-test", Ref: "cmd/pig/issue75_process_test.go#TestPigProjectTrustStartupParity", Baseline: issue56BaselineCommit, CaseID: caseID, InputHash: issue33CanonicalInputHash(t, issue56RepoRoot(t), issue33EvidenceInputPaths(caseID)), ExecutionMethod: "go test ./cmd/pig -run '^(TestPigProjectTrustStartupParity|TestPigUntrustedProjectHasNoSensitiveReadsOrEffects)$' -count=1", Expected: "trust gates project settings before Session selection; Context Files remain visible in untrusted Headless prompts", Actual: "PASS; real CLI matches Pi trust/context behavior and FIFO tests prove no sensitive pre-trust reads", Platform: "darwin/linux", CatalogID: entry.ID})
	entry.Partial.Supported = append(entry.Partial.Supported, "Issue #75: --approve/--no-approve and saved/global trust gate project settings before Session path selection; Context Files remain independent and --no-context-files disables discovery")
	entry.Notes += " Issue #75 activates Headless Project Trust; it does not add Tool approvals, sandboxing, TUI prompts or extension execution."
}

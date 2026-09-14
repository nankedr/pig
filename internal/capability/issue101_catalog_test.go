package capability_test

import (
	"strings"
	"testing"

	"github.com/nankedr/pig/internal/catalog"
)

func issue101ExtendProductEntry(t *testing.T, entry *catalog.Entry) {
	if entry.ID != "cmd-pig" && entry.ID != "contract:cli/pig/args" && entry.ID != "contract:codingagent/headless" {
		return
	}
	const caseID = "issue101-system-prompts-product"
	entry.Evidence = issue56UpsertEvidence(entry.Evidence, catalog.Evidence{Kind: "go-test", Ref: "cmd/pig/issue101_prompt_test.go", Baseline: issue56BaselineCommit, CaseID: caseID, InputHash: issue33CanonicalInputHash(t, issue56RepoRoot(t), issue33EvidenceInputPaths(caseID)), ExecutionMethod: "go test ./cmd/pig -run '^TestPigSystemPrompts$' -count=1", Expected: "actual CLI model input follows fixed Pi system/append prompt selection and read diagnostics", Actual: "PASS; explicit/default/empty/file input, project trust and disabled resource combinations", Platform: "darwin/linux non-root", CatalogID: entry.ID})
	entry.Partial.Supported = append(entry.Partial.Supported, "Issue #101: local system/append prompt resources and explicit CLI values/files, first-load trust and diagnostics; contract:codingagent/system-prompts owns exact scope and evidence")
	for i, s := range entry.Partial.Unsupported {
		entry.Partial.Unsupported[i] = strings.ReplaceAll(s, "resource and extension", "remaining resource and extension")
	}
	entry.Notes += " Issue #101 implements local system/append prompts; M7 trust callbacks and #99 packages remain explicit stubs."
}

package capability_test

import (
	"github.com/nankedr/pig/internal/catalog"
	"strings"
	"testing"
)

func issue97ExtendProductEntry(t *testing.T, entry *catalog.Entry) {
	if entry.ID != "cmd-pig" && entry.ID != "contract:cli/pig/args" {
		return
	}
	const caseID = "issue97-html-export-product"
	entry.Evidence = issue56UpsertEvidence(entry.Evidence, catalog.Evidence{Kind: "go-test", Ref: "codingagent/issue97_export_test.go", Baseline: issue56BaselineCommit, CaseID: caseID, InputHash: issue33CanonicalInputHash(t, issue56RepoRoot(t), issue33EvidenceInputPaths(caseID)), ExecutionMethod: "go test ./codingagent -run '^TestIssue97Export' -count=1", Expected: "CLI export generates actual offline HTML without modifying the source", Actual: "PASS; fixed Pi v3 data, paths, errors and deferred branches", Platform: "any", CatalogID: entry.ID})
	entry.Partial.Supported = append(entry.Partial.Supported, "Issue #97: --export input [output] and shared SDK/RPC HTML export; scope and browser evidence in contract:codingagent/html-export")
	for i, s := range entry.Partial.Unsupported {
		entry.Partial.Unsupported[i] = strings.ReplaceAll(strings.ReplaceAll(s, "export, ", ""), "export remain", "remaining operations remain")
	}
	entry.Notes += " Issue #97 implements safe, embedded, offline HTML export with explicit image/extension/theme boundaries."
}

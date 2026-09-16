package capability_test

import (
	"strings"
	"testing"

	"github.com/nankedr/pig/internal/catalog"
)

func issue107ExtendProductEntry(t *testing.T, entry *catalog.Entry) {
	if entry.ID != "cmd-pig" && entry.ID != "contract:cli/pig/args" && entry.ID != "contract:codingagent/headless" {
		return
	}
	const caseID = "issue107-local-extensions-product"
	entry.Evidence = issue56UpsertEvidence(entry.Evidence, catalog.Evidence{Kind: "go-test", Ref: "cmd/pig/issue107_extensions_test.go", Baseline: issue56BaselineCommit, CaseID: caseID, InputHash: issue33CanonicalInputHash(t, issue56RepoRoot(t), issue33EvidenceInputPaths(caseID)), ExecutionMethod: "go test ./cmd/pig -run '^TestPigLocalExtensions' -count=1", Expected: "explicit extension discovery diagnostics and execution failure without side effects; normal local resources remain usable", Actual: "PASS; real CLI sources, no-extensions, unknown flags, no code execution, network, state writes or extension success output", Platform: "darwin", CatalogID: entry.ID})
	entry.Partial.Supported = append(entry.Partial.Supported, "Issue #107: local extension discovery and source diagnostics; explicit -e still fails execution, --no-extensions preserves explicit paths, normal resources remain usable; contract:codingagent/local-extensions owns exact scope and evidence")
	for i, s := range entry.Partial.Unsupported {
		entry.Partial.Unsupported[i] = strings.ReplaceAll(s, "resource and extension operations", "resource and extension execution operations")
	}
	entry.Notes += " Issue #107 separates local extension discovery from execution; factories, trust hooks, callbacks and runtime reload remain stubs with no ABI frozen."
}

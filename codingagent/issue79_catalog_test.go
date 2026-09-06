package codingagent_test

import (
	"github.com/nankedr/pig/internal/catalog"
	"testing"
)

const issue79EditCatalogID = "contract:codingagent/edit-tool"

func issue79EditCatalogEntry() catalog.Entry {
	return catalog.Entry{
		SchemaVersion: catalog.SchemaVersion, ID: issue79EditCatalogID,
		Upstream: catalog.Upstream{Module: "coding-agent", Repository: "https://github.com/badlogic/pi-mono", Commit: issue32BaselineCommit, Reference: "packages/coding-agent/src/core/tools/edit.ts"},
		Mapping:  catalog.Mapping{Module: "codingagent", Target: issue32GoPackage + ".CreateEditTool", Kind: "contract"}, Status: catalog.StatusPartial, Milestone: "M3", Classification: "public-api",
		Partial: &catalog.Partial{Supported: []string{
			"explicit SDK/Headless edit-read continuation, executable definition and pre-validation legacy/stringified edits preparation",
			"original-file multi-edit exact/fuzzy matching, normalized uniqueness, overlap/missing/no-change errors and unchanged-line preservation; NFKC, BOM, CRLF/LF, empty text and UTF-8 decoding Oracle cases",
			"jsdiff 8.0.4 line diff and unified patch semantics, firstChangedLine and four-line context; process-wide shared write/edit FIFO, host paths/permissions, settled cancellation and errors",
		}, Unsupported: []string{"interactive diff preview/rendering and extension host remain deferred to M7", "default complete coding-tools assembly remains deferred; non-Unix access behavior has build coverage only"}},
		Notes: "Issue #79 ports fixed Pi edit/edit-diff and the baseline jsdiff dependency. All replacements validate before a single host write; I/O failure or cancellation does not promise rollback of bytes already written. Queue scope and symlink identity follow issue #78. ToolDefinition.PrepareArguments uses the existing agent callback (ADR-0020); JSON Details carry diff, patch and firstChangedLine.",
	}
}
func issue79EditEvidence(t *testing.T) []issue32ModuleEvidenceDescriptor {
	t.Helper()
	var out []issue32ModuleEvidenceDescriptor
	for _, e := range []struct{ kind, path, ref, id, run string }{
		{"oracle", "parity/oracle/fixtures/edit-tool.json", "parity/oracle/fixtures/edit-tool.json", "issue79-edit-oracle", "node --experimental-strip-types parity/oracle/edit-tool.mjs <locked-pi-checkout> --check"},
		{"go-test", "codingagent/issue79_edit_test.go", "codingagent/issue79_edit_test.go#TestEditToolSessionParity", "issue79-edit-session", "go test ./codingagent -run '^TestEditToolSessionParity$' -count=1"},
		{"go-test", "codingagent/issue79_queue_test.go", "codingagent/issue79_queue_test.go#TestEditToolMixedMutationQueue", "issue79-edit-queue", "go test -race ./codingagent -run '^TestEditTool(MixedMutationQueue|SessionAbort|InvalidArguments)' -count=1"},
		{"go-test", "codingagent/issue79_paths_test.go", "codingagent/issue79_paths_test.go#TestEditToolDefinitionAndHostPaths", "issue79-edit-paths", "go test ./codingagent -run '^TestEditTool(DefinitionAndHostPaths|PreCanceled|HeadlessExplicit)' -count=1"},
		{"go-test", "cmd/pig/issue79_process_test.go", "cmd/pig/issue79_process_test.go#TestPigEditReadContinuation", "issue79-edit-cli", "go test ./cmd/pig -run '^TestPigEditReadContinuation$' -count=1"},
	} {
		out = append(out, issue32ModuleEvidenceDescriptor{InputPath: e.path, Evidence: catalog.Evidence{Kind: e.kind, Ref: e.ref, Baseline: issue32BaselineCommit, CaseID: e.id, ExecutionMethod: e.run, Expected: "edit ToolResult, exact diff/patch, firstChangedLine and read content reach the next model request; invalid edits do not partially apply; shared write/edit queues retain in-flight operations", Actual: "PASS; fixed Pi Oracle, public SDK/CLI continuation and mixed mutation/cancellation/host-path regressions", Platform: "darwin-arm64", CatalogID: issue79EditCatalogID}})
	}
	return out
}

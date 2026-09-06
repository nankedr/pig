package codingagent_test

import (
	"github.com/nankedr/pig/internal/catalog"
	"testing"
)

const issue78WriteCatalogID = "contract:codingagent/write-tool"

func issue78WriteCatalogEntry() catalog.Entry {
	return catalog.Entry{
		SchemaVersion: catalog.SchemaVersion, ID: issue78WriteCatalogID,
		Upstream: catalog.Upstream{Module: "coding-agent", Repository: "https://github.com/badlogic/pi-mono", Commit: issue32BaselineCommit, Reference: "packages/coding-agent/src/core/tools/write.ts"},
		Mapping:  catalog.Mapping{Module: "codingagent", Target: issue32GoPackage + ".CreateWriteTool", Kind: "contract"}, Status: catalog.StatusPartial, Milestone: "M3", Classification: "public-api",
		Partial:   &catalog.Partial{Supported: []string{"explicit SDK and Headless write/read continuation, real create/overwrite, recursive parent creation, UTF-8 contents and Pi UTF-16 success counts", "host absolute/parent/home/file URL paths and existing symlink aliases; process-wide per-file FIFO mutation queues retain in-flight work until settlement and release on cancellation/failure", "built-in write definition execution and prompt metadata"}, Unsupported: []string{"interactive Tool rendering and extension host invocation remain deferred to M7", "default all-coding-tools assembly remains a Capability Stub; edit is tracked by contract:codingagent/edit-tool"}},
		Deviation: &catalog.Deviation{ADR: "docs/adr/0020-builtin-tool-definition-execution.md", Reason: "The built-in Execute callback uses Go JSON values and context without choosing an extension host ABI. Empty result details use the existing Go nil/null representation; Oracle compares absence of a details payload."},
		Notes:     "Issue #78 enables write only through explicit selection in Headless. Queue registration resolves existing symlinks and falls back to the absolute lexical path for ENOENT/ENOTDIR, exactly as the fixed Pi baseline; it does not impose a workspace boundary or per-call approval. Queue scope is one process, not a cross-process lock.",
	}
}
func issue78WriteEvidence(t *testing.T) []issue32ModuleEvidenceDescriptor {
	t.Helper()
	var out []issue32ModuleEvidenceDescriptor
	for _, e := range []struct{ kind, path, ref, id, run string }{
		{"oracle", "parity/oracle/fixtures/write-tool.json", "parity/oracle/fixtures/write-tool.json", "issue78-write-oracle", "node --experimental-strip-types parity/oracle/write-tool.mjs <locked-pi-checkout> --check"},
		{"go-test", "codingagent/issue78_write_test.go", "codingagent/issue78_write_test.go#TestWriteToolSessionParity", "issue78-write-session", "go test -race ./codingagent -run '^TestWriteTool' -count=1"},
		{"go-test", "cmd/pig/issue78_process_test.go", "cmd/pig/issue78_process_test.go#TestPigWriteReadContinuation", "issue78-write-cli", "go test ./cmd/pig -run '^TestPigWriteReadContinuation$' -count=1"},
	} {
		out = append(out, issue32ModuleEvidenceDescriptor{InputPath: e.path, Evidence: catalog.Evidence{Kind: e.kind, Ref: e.ref, Baseline: issue32BaselineCommit, CaseID: e.id, ExecutionMethod: e.run, Expected: "explicit write creates or overwrites real host files, read consumes the result, and public sessions continue with success or error ToolResults", Actual: "PASS; deterministic fixture and SDK/CLI tests cover the slice, including shared queue cancellation and failure", Platform: "any", CatalogID: issue78WriteCatalogID}})
	}
	return out
}

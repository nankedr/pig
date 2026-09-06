package codingagent_test

import (
	"github.com/nankedr/pig/internal/catalog"
	"testing"
)

const issue83GrepCatalogID = "contract:codingagent/grep-tool"

func issue83GrepCatalogEntry() catalog.Entry {
	return catalog.Entry{
		SchemaVersion: catalog.SchemaVersion, ID: issue83GrepCatalogID,
		Upstream: catalog.Upstream{Module: "coding-agent", Repository: "https://github.com/badlogic/pi-mono", Commit: issue32BaselineCommit, Reference: "packages/coding-agent/src/core/tools/grep.ts"},
		Mapping:  catalog.Mapping{Module: "codingagent", Target: issue32GoPackage + ".CreateGrepTool", Kind: "contract"}, Status: catalog.StatusPartial, Milestone: "M4", Classification: "public-api",
		Partial:   &catalog.Partial{Supported: []string{"explicit CLI/SDK grep selection, authoritative Agent validation, shared Tool/definition execution and search/error ToolResult continuation into editing", "real ripgrep regex/literal/case/glob/hidden/ignore semantics, host paths, custom context reads, fractional numbers, UTF-16 line limits and 50KB/global match truncation against locked Pi fixtures", "Pig bin before PATH version probe, process arguments and exit/spawn errors, explicit no-download missing behavior, Unix cancellation and match-limit process-group cleanup"}, Unsupported: []string{"automatic external-tool downloads/installations remain explicit unsupported; missing rg returns installation guidance without network, including Offline", "interactive Tool rendering and extension host remain M5/M7; Windows/Termux execution and six-platform behavior gates remain M13"}},
		Deviation: &catalog.Deviation{ADR: "docs/adr/0022-grep-external-tool-lifecycle.md", Reason: "Pig uses its own bin directory and does not silently download executables. Missing rg fails visibly; automatic installation is deferred. Cancellation retains Go context causes and kills/reaps Unix process groups. Lone UTF-16 surrogates are projected as UTF-8 replacement characters; empty error details are normalized as no payload."},
		Notes:     "Issue #83. Default tools remain read/bash/edit/write. GrepOperations replaces stat/context reads, not the ripgrep search engine. GrepToolInput numeric pointers and GrepToolDetails.MatchLimitReached preserve Pi number semantics before the M4 freeze. Host permissions are inherited with no workspace restriction or per-Tool approval. Behavior evidence is owned here; symbol inventory rows are not wholesale promoted.",
	}
}
func issue83GrepEvidence(t *testing.T) []issue32ModuleEvidenceDescriptor {
	t.Helper()
	var out []issue32ModuleEvidenceDescriptor
	for _, e := range []struct{ kind, path, ref, id, run string }{
		{"oracle", "parity/oracle/fixtures/grep-tool.json", "parity/oracle/fixtures/grep-tool.json", "issue83-grep-oracle", "node --experimental-strip-types parity/oracle/grep-tool.mjs <locked-pi-checkout> --check"},
		{"go-test", "codingagent/issue83_grep_test.go", "codingagent/issue83_grep_test.go#TestGrepToolSessionParity", "issue83-grep-session", "go test -race ./codingagent -run '^TestGrepTool' -count=1"},
		{"go-test", "codingagent/issue83_process_unix_test.go", "codingagent/issue83_process_unix_test.go#TestGrepToolSessionProcessCleanup", "issue83-grep-process", "go test -race ./codingagent -run '^TestGrepTool(SessionProcessCleanup|ExternalProcessContract)$' -count=1"},
		{"go-test", "cmd/pig/issue83_process_test.go", "cmd/pig/issue83_process_test.go#TestPigGrepEditContinuation", "issue83-grep-cli", "go test ./cmd/pig -run '^TestPigGrep' -count=1"},
	} {
		out = append(out, issue32ModuleEvidenceDescriptor{InputPath: e.path, Evidence: catalog.Evidence{Kind: e.kind, Ref: e.ref, Baseline: issue32BaselineCommit, CaseID: e.id, ExecutionMethod: e.run, Expected: "real grep success, error and cancellation settle at the public Session boundary; CLI consumes search ToolResults and edits files; supported results match locked Pi", Actual: "PASS; deterministic Oracle, SDK/definition equivalence, text/json CLI recovery and editing, external executable lifecycle", Platform: "darwin-arm64", CatalogID: issue83GrepCatalogID}})
	}
	return out
}

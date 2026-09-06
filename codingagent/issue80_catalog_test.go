package codingagent_test

import (
	"github.com/nankedr/pig/internal/catalog"
	"testing"
)

const issue80BashCatalogID = "contract:codingagent/bash-tool"

func issue80BashCatalogEntry() catalog.Entry {
	return catalog.Entry{
		SchemaVersion: catalog.SchemaVersion, ID: issue80BashCatalogID,
		Upstream: catalog.Upstream{Module: "coding-agent", Repository: "https://github.com/badlogic/pi-mono", Commit: issue32BaselineCommit, Reference: "packages/coding-agent/src/core/tools/bash.ts"},
		Mapping:  catalog.Mapping{Module: "codingagent", Target: issue32GoPackage + ".CreateBashTool", Kind: "contract"}, Status: catalog.StatusPartial, Milestone: "M3", Classification: "public-api",
		Partial:   &catalog.Partial{Supported: []string{"explicit SDK/Headless bash continuation, executable definition and replaceable local operations; host cwd/environment and Pig session metadata", "optional timeout and cancellation terminate Unix process groups; bounded UTF-8 tail snapshots and retained full output files readable after Session disposal and CLI exit", "darwin-arm64 shell, process tree, SIGINT/SIGTERM/SIGHUP and streaming update/final barriers verified"}, Unsupported: []string{"Windows/Termux shell resolution and six-platform behavior gates remain M13", "interactive user bash, extension host/rendering and default complete coding tools assembly remain deferred"}},
		Deviation: &catalog.Deviation{ADR: "docs/adr/0008-pig-identity-state-and-services.md", Reason: "Session environment uses PIG_* and retained OS temporary output uses pig-bash-*; no PI_* identity is generated. Built-in definition execution follows ADR-0020."},
		Notes:     "Issue #80 runs commands with host permissions and inherits the host environment. Full raw stdout/stderr is retained once output exceeds 2000 lines or 50KB; files are never automatically deleted by Session cleanup. Shell platform boundary is replaceable; Linux build support is not a six-platform behavioral claim.",
	}
}
func issue80BashEvidence(t *testing.T) []issue32ModuleEvidenceDescriptor {
	t.Helper()
	var out []issue32ModuleEvidenceDescriptor
	for _, e := range []struct{ kind, path, ref, id, run string }{
		{"oracle", "parity/oracle/fixtures/bash-tool.json", "parity/oracle/fixtures/bash-tool.json", "issue80-bash-oracle", "node --experimental-strip-types parity/oracle/bash-tool.mjs <locked-pi-checkout> --check"},
		{"go-test", "codingagent/issue80_bash_test.go", "codingagent/issue80_bash_test.go#TestBashToolSessionParity", "issue80-bash-session", "go test -race ./codingagent -run '^TestBashTool' -count=1"},
		{"go-test", "codingagent/issue80_process_unix_test.go", "codingagent/issue80_process_unix_test.go#TestBashToolSessionAbortKillsProcessTree", "issue80-bash-tree", "go test -race ./codingagent -run '^TestBashToolSessionAbortKillsProcessTree$' -count=1"},
		{"go-test", "cmd/pig/issue80_process_test.go", "cmd/pig/issue80_process_test.go#TestPigBashReadContinuation", "issue80-bash-cli", "go test ./cmd/pig -run '^TestPigBash' -count=1"},
		{"go-test", "cmd/pig/issue80_signal_unix_test.go", "cmd/pig/issue80_signal_unix_test.go#TestPigBashShutdownSignalsKillProcessTree", "issue80-bash-signals", "go test ./cmd/pig -run '^TestPigBashShutdownSignalsKillProcessTree$' -count=1"},
	} {
		out = append(out, issue32ModuleEvidenceDescriptor{InputPath: e.path, Evidence: catalog.Evidence{Kind: e.kind, Ref: e.ref, Baseline: issue32BaselineCommit, CaseID: e.id, ExecutionMethod: e.run, Expected: "host shell results continue through the public Session, output updates settle before final events, cancellation kills the process group, and read consumes retained output after cleanup", Actual: "PASS; fixed Pi fixture, SDK/CLI continuation, UTF-8 truncation, retained output and native signal tests", Platform: "darwin-arm64", CatalogID: issue80BashCatalogID}})
	}
	return out
}

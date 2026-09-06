package codingagent_test

import (
	"github.com/nankedr/pig/internal/catalog"
	"testing"
)

const issue84FindLsCatalogID = "contract:codingagent/find-ls"

func issue84FindLsCatalogEntry() catalog.Entry {
	return catalog.Entry{
		SchemaVersion: catalog.SchemaVersion, ID: issue84FindLsCatalogID,
		Upstream: catalog.Upstream{Module: "coding-agent", Repository: "https://github.com/badlogic/pi-mono", Commit: issue32BaselineCommit, Reference: "packages/coding-agent/src/core/tools/find.ts"},
		Mapping:  catalog.Mapping{Module: "codingagent", Target: issue32GoPackage + ".CreateFindTool", Kind: "contract"}, Status: catalog.StatusPartial, Milestone: "M4", Classification: "public-api",
		Partial:   &catalog.Partial{Supported: []string{"explicit find/ls/read SDK and CLI continuation, executable built-in definitions, unchanged default four Tools", "host paths, hidden entries, fd glob and hierarchical gitignore, relative paths and directory suffixes, ls case-insensitive ordering, limits and byte truncation", "custom Operations, local fd then PATH fd/fdfind detection, argv/exit handling, missing binary without downloads, cancellation and process-tree cleanup"}, Unsupported: []string{"interactive rendering and extension host invocation remain M7", "custom FindOperations Limit remains Go int: fractional or overflowing custom limits fail explicitly", "locale-dependent ICU ordering beyond the recorded English collation cases and six-platform runtime verification remain M13; default local fd execution outside darwin/linux is an explicit FindOperations.platform Stub; fd line framing and whitespace trimming retain Pi filename limitations", "automatic binary installation is deliberately unavailable under the platform contract"}},
		Deviation: &catalog.Deviation{ADR: "docs/adr/0022-find-ls-platform-contract.md", Reason: "Missing fd never triggers network or downloads. Go Operations use synchronous context-aware methods and integral FindGlobOptions.Limit. Error-details absence is normalized across the existing dispatcher mapping."},
		Notes:     "Issue #84. Pi commit 936aff0 source and tools/regression tests read before implementation; fixture must replay with prepared fd on PATH. Real filesystem Oracle and CLI tests skip when fd is absent; platform and custom-Operations tests remain offline and do not skip. Find preserves fd/custom order; ls uses English Go collation. No cross-platform or full interactive parity claim.",
	}
}
func issue84FindLsEvidence(t *testing.T) []issue32ModuleEvidenceDescriptor {
	t.Helper()
	var out []issue32ModuleEvidenceDescriptor
	for _, e := range []struct{ kind, path, ref, id, run string }{
		{"oracle", "parity/oracle/fixtures/find-ls.json", "parity/oracle/fixtures/find-ls.json", "issue84-find-ls-oracle", "node --experimental-strip-types parity/oracle/find-ls.mjs <locked-pi-checkout> --check"},
		{"go-test", "codingagent/issue84_find_ls_test.go", "codingagent/issue84_find_ls_test.go#TestFindLsSessionParity", "issue84-find-ls-sdk", "go test -race ./codingagent -run '^TestFindLs' -count=1 (prepared fd on PATH)"},
		{"go-test", "codingagent/issue84_fd_unix_test.go", "codingagent/issue84_fd_unix_test.go#TestFindLsFDCancellationKillsTree", "issue84-find-ls-platform", "go test -race ./codingagent -run '^TestFindLsFD' -count=1"},
		{"go-test", "cmd/pig/issue84_process_test.go", "cmd/pig/issue84_process_test.go#TestPigFindLsReadContinuation", "issue84-find-ls-cli", "go test ./cmd/pig -run '^TestPigFindLsReadContinuation$' -count=1 (prepared fd on PATH)"},
	} {
		out = append(out, issue32ModuleEvidenceDescriptor{InputPath: e.path, Evidence: catalog.Evidence{Kind: e.kind, Ref: e.ref, Baseline: issue32BaselineCommit, CaseID: e.id, ExecutionMethod: e.run, Expected: "locate paths, read real files, continue generation; exact supported Pi outputs and metadata, platform effects and cancellation", Actual: "PASS; locked Oracle, public Session/definition and real text/json CLI, deterministic fd process tests", Platform: "darwin-arm64", CatalogID: issue84FindLsCatalogID}})
	}
	return out
}

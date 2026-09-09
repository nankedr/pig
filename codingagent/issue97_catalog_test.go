package codingagent_test

import (
	"github.com/nankedr/pig/internal/catalog"
	"strings"
	"testing"
)

const issue97HTMLCatalogID = "contract:codingagent/html-export"

func issue97HTMLCatalogEntry() catalog.Entry {
	return catalog.Entry{SchemaVersion: catalog.SchemaVersion, ID: issue97HTMLCatalogID, Upstream: catalog.Upstream{Module: "coding-agent", Repository: "https://github.com/badlogic/pi-mono", Commit: issue32BaselineCommit, Reference: "packages/coding-agent/src/core/export-html/index.ts"}, Mapping: catalog.Mapping{Module: "codingagent", Target: issue32GoPackage + ".ExportFromFile", Kind: "contract"}, Status: catalog.StatusPartial, Milestone: "M4", Classification: "public-api", Partial: &catalog.Partial{Supported: []string{
		"Issue #97: CLI --export input [output], ExportFromFile, AgentSession.ExportToHTML and RPC export_html/RPCClient.ExportHTML share a v3 whole-tree export with current leaf, SDK system prompt/tools and Pig output naming",
		"Locked Pi browser fixture derived from existing v3 interoperability branches and summaries; Markdown/GFM, highlighted code, ToolResult whitespace, branch navigation, search/filter and summary toggles",
		"Embedded offline assets and full third-party licenses; base64 session data, escaped attributes and hash-only script CSP; browser XSS tests at the real CLI boundary; deterministic input/path/write/cancellation errors without source mutation or implicit Pi state/services",
	}, Unsupported: []string{
		"Message/tool-result image blocks fail explicitly (M12); Markdown images display a visible deferred placeholder and make no network request. Extension HTML renderers remain deferred to M7; custom tools use Pi's plain-text/JSON fallback",
		"Only the locked dark theme is embedded; configured/custom themes wait for M5. Legacy/future versions, unknown roles/blocks/entries and corrupt topology are rejected; explicit session migration remains available separately",
		"No hosted sharing service or remote assets. ExportToJSONL SDK method remains a stub; the browser's existing JSONL download exports the embedded whole tree. No M4 freeze/release claim",
	}}, Deviation: &catalog.Deviation{ADR: "docs/adr/0033-safe-html-export.md", Reason: "Embed locked licensed assets; reject invalid input without rewriting it; remove inline handlers, escape tool offsets and use a hash CSP with no automatic network access."}}
}
func issue97HTMLEvidence(t *testing.T) []issue32ModuleEvidenceDescriptor {
	t.Helper()
	var out []issue32ModuleEvidenceDescriptor
	for _, e := range []struct{ kind, path, run string }{
		{"oracle", "parity/oracle/fixtures/export-html.json", "node --experimental-strip-types parity/oracle/export-html.mjs <locked-pi-checkout> --check"},
		{"go-test", "codingagent/issue97_export_test.go", "go test ./codingagent -run '^TestIssue97Export' -count=1"},
		{"go-test", "cmd/pig/issue97_export_test.go", "go test -race ./cmd/pig -run '^TestRPC97' -count=1"},
		{"go-test", "codingagent/issue97_surface_test.go", "go test ./codingagent -run '^TestIssue97Export' -count=1"},
		{"manual", "parity/export-html/check.mjs", "node parity/export-html/check.mjs"},
	} {
		out = append(out, issue32ModuleEvidenceDescriptor{InputPath: e.path, Evidence: catalog.Evidence{Kind: e.kind, Ref: e.path, Baseline: issue32BaselineCommit, CaseID: "issue97-" + e.path, ExecutionMethod: e.run, Expected: "fixed Pi export data/rendering and safe real CLI, SDK and RPC files", Actual: "PASS; whole-tree fixture, actual browser navigation and XSS, errors and public boundaries", Platform: "any", CatalogID: issue97HTMLCatalogID}})
	}
	return out
}
func issue97PromoteRuntimeEntry(e *catalog.Entry) bool {
	if !strings.HasSuffix(e.ID, "#AgentSession.exportToHtml") && !strings.HasSuffix(e.ID, "#RpcClient.exportHtml") {
		return false
	}
	e.Status = catalog.StatusPartial
	e.Partial = issue97HTMLCatalogEntry().Partial
	e.Evidence = []catalog.Evidence{{Kind: "go-test", Ref: "codingagent/issue97_export_test.go", Baseline: issue32BaselineCommit, CaseID: e.ID, InputHash: "5b4a59ab93e91208df2ef16827d1e66c12df49a6a37c936fd54b46f6c9268acf", ExecutionMethod: "go test ./codingagent ./cmd/pig -run '^(TestIssue97Export|TestRPC97)' -count=1", Expected: "shared real file export", Actual: "PASS; CLI/SDK/RPC and browser validation", Platform: "any", CatalogID: e.ID}}
	e.Notes = "Issue #97; supported and deferred export branches in " + issue97HTMLCatalogID + "."
	return true
}

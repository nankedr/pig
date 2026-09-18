package tui_test

import (
	"github.com/nankedr/pig/internal/catalog"
	"strings"
)

func issue112Promote(e *catalog.Entry) bool {
	name := strings.TrimPrefix(e.Mapping.Target, issue31GoPackage+".")
	supported := false
	for _, n := range []string{"Markdown", "VisibleWidth", "StripTerminalSequences", "ExtractANSICode", "WrapTextWithANSI", "TruncateToWidth"} {
		supported = supported || name == n || strings.HasPrefix(name, n+".")
	}
	if !supported {
		return false
	}
	e.Status = catalog.StatusPartial
	e.Partial = &catalog.Partial{Supported: []string{"Issue #112: Markdown text/code/links/nested lists, ANSI safe wrapping, visible widths and truncation through public SDK fixtures"}, Unsupported: []string{"Full Markdown dialect, syntax highlighting, OSC hyperlinks, LaTeX and exact theme styling remain partial; see contract:codingagent/text-rendering"}}
	e.Evidence = []catalog.Evidence{{Kind: "go-test", Ref: "tui/issue112_markdown_test.go", Baseline: issue31BaselineCommit, CaseID: e.ID, InputHash: "sha256:2732320dba39013b77be4e1476d6186faf524fbf053aa31bed89736091cdd9c9", ExecutionMethod: "go test ./tui -run '112' -count=1", Expected: "pinned Pi Markdown semantic lines and ANSI/grapheme handling", Actual: "PASS with explicit partial branches", Platform: "any", CatalogID: e.ID}}
	e.Notes = "Issue #112; contract:codingagent/text-rendering"
	return true
}

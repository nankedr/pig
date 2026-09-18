package codingagent_test

import (
	"crypto/sha256"
	"flag"
	"fmt"
	"github.com/nankedr/pig/internal/catalog"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

var updateIssue112Catalog = flag.Bool("update-issue112-catalog", false, "refresh text rendering catalog")

func issue112Partial() *catalog.Partial {
	return &catalog.Partial{Supported: []string{"Issue #112: Markdown text, code, links, nested lists, thinking groups, errors and partial outcomes; Transcript renders user/assistant/tool/custom/bash/summary history without changing model context", "Streaming Tool arguments and execution updates retain call order; final results ignore late updates; live Ctrl+O/Ctrl+T and history restoration through real CLI PTY", "Pure Go ANSI sanitization, grapheme widths, wrapping and truncation; public SDK and pinned Pi component/CLI fixtures"}, Unsupported: []string{"Specialized built-in Tool layouts, exact theme colors/syntax highlighting, word-level diff inversion, complex Markdown/table/LaTeX/Mermaid, OSC hyperlinks and shell integration markers remain partial", "Summary-specific standalone components, full TUI layouts/scrollback remain Stub or unverified; images M12, extension custom rendering M7 and six-platform runtime acceptance M13 remain deferred"}}
}
func issue112PromoteRuntimeEntry(e *catalog.Entry) bool {
	name := strings.TrimPrefix(e.Mapping.Target, issue32GoPackage+".")
	supported := false
	for _, n := range []string{"AssistantMessageComponent", "UserMessageComponent", "ToolExecutionComponent", "RenderDiff", "GetMarkdownTheme", "TruncateToVisualLines"} {
		supported = supported || name == n || strings.HasPrefix(name, n+".")
	}
	if !supported || strings.HasSuffix(name, ".SetShowImages") || strings.HasSuffix(name, ".SetImageWidthCells") {
		return false
	}
	e.Status = catalog.StatusPartial
	e.Partial = issue112Partial()
	e.Evidence = []catalog.Evidence{{Kind: "go-test", Ref: "codingagent/issue112_messages_test.go", Baseline: issue32BaselineCommit, CaseID: e.ID, InputHash: issue112FixtureHash, ExecutionMethod: "go test ./codingagent -run '112' -count=1", Expected: "public text message rendering and final Tool ordering", Actual: "PASS; fixed component fixture and CLI evidence in contract:codingagent/text-rendering", Platform: "any (SDK), darwin/linux (PTY)", CatalogID: e.ID}}
	e.Notes = "Issue #112 text rendering; contract:codingagent/text-rendering"
	return true
}
func TestTextRenderingCatalog112(t *testing.T) {
	root := issue32RepoRoot(t)
	e := catalog.Entry{SchemaVersion: catalog.SchemaVersion, ID: "contract:codingagent/text-rendering", Upstream: catalog.Upstream{Module: "coding-agent", Repository: "https://github.com/badlogic/pi-mono", Commit: issue32BaselineCommit, Reference: "packages/coding-agent/src/modes/interactive/interactive-mode.ts"}, Mapping: catalog.Mapping{Module: "codingagent", Target: issue32GoPackage + ".Transcript", Kind: "contract"}, Status: catalog.StatusPartial, Milestone: "M6", Classification: "public-api", Partial: issue112Partial(), Notes: "See docs/learning/m6-text-rendering.md. CLI fixture uses actual pinned Pi subprocess and gated PTY/SSE; ordinary Go acceptance is offline."}
	for _, item := range []struct{ kind, path, run string }{{"oracle", "parity/oracle/fixtures/text-rendering.json", "node parity/oracle/text-rendering.mjs <locked-pi-checkout> --check"}, {"oracle", "parity/oracle/fixtures/text-rendering-cli.json", "node parity/oracle/text-rendering-cli.mjs <locked-pi-checkout> --check"}, {"go-test", "codingagent/issue112_messages_test.go", "go test ./codingagent -run '112' -count=1"}, {"go-test", "tui/issue112_markdown_test.go", "go test ./tui -run '112' -count=1"}, {"go-test", "cmd/pig/issue112_rendering_test.go", "go test ./cmd/pig -run 'TestPig.*112' -count=1"}, {"go-test", "codingagent/testdata/issue112_surface_golden.txt", "go test ./codingagent -run TestTextRenderingAPISnapshot112 -count=1"}, {"manual", "examples/text-rendering/main.go", "go run ./examples/text-rendering"}} {
		data, err := os.ReadFile(filepath.Join(root, item.path))
		if err != nil {
			t.Fatal(err)
		}
		hash := fmt.Sprintf("sha256:%x", sha256.Sum256(data))
		e.Evidence = append(e.Evidence, catalog.Evidence{Kind: item.kind, Ref: item.path, Baseline: issue32BaselineCommit, CaseID: "issue112-" + item.path, InputHash: hash, ExecutionMethod: item.run, Expected: "text/Tool rendering through public boundaries", Actual: "PASS with explicitly recorded partial branches", Platform: "any (SDK), darwin/linux (PTY)", CatalogID: e.ID})
	}
	path := filepath.Join(root, "parity/catalog.jsonl")
	entries, err := catalog.LoadCatalog(path)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for i, row := range entries {
		if row.ID == e.ID {
			found = true
			if *updateIssue112Catalog {
				entries[i] = e
			} else if !reflect.DeepEqual(row, e) {
				t.Fatal("text rendering catalog drift")
			}
		}
	}
	if *updateIssue112Catalog {
		if !found {
			entries = append(entries, e)
		}
		data, err := catalog.EncodeEntries(entries)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(path, data, 0644); err != nil {
			t.Fatal(err)
		}
	} else if !found {
		t.Fatal("missing text rendering contract")
	}
}

const issue112FixtureHash = "sha256:2732320dba39013b77be4e1476d6186faf524fbf053aa31bed89736091cdd9c9"

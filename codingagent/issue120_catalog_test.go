package codingagent_test

import (
	"crypto/sha256"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/nankedr/pig/codingagent"
	"github.com/nankedr/pig/internal/catalog"
	"github.com/nankedr/pig/tui"
)

var updateIssue120 = flag.Bool("update-issue120", false, "refresh interactive themes evidence and API snapshot")

func partial120() *catalog.Partial {
	return &catalog.Partial{Supported: []string{
		"Issue #120: M5 resource precedence and trust; fixed and light/dark automatic themes; preview, cancel, confirm and global settings persistence",
		"Public SDK and real CLI PTY compare locked terminal reports, selection events, truecolor/256color file reload, history and draft preservation and terminal restoration",
		"Per-session rendering, 100ms cancellable polling, last-valid fallback with diagnostics, resource replacement and terminal color notification cleanup",
	}, Unsupported: []string{
		"Exact settings submenu layout/search, syntax highlighting and complete Markdown styling remain partial",
		"Native filesystem event debounce is replaced by CGO-free content polling; local M5 source paths are watched beyond Pi's global named-theme directory",
		"Extensions M7, full authentication M11, images M12, six-platform acceptance M13 and package ecosystem #99 remain outside scope",
	}}
}
func TestInteractiveThemesCatalog120(t *testing.T) {
	root := issue32RepoRoot(t)
	entry := catalog.Entry{SchemaVersion: catalog.SchemaVersion, ID: "contract:codingagent/interactive-themes", Upstream: catalog.Upstream{Module: "coding-agent", Repository: "https://github.com/badlogic/pi-mono", Commit: issue32BaselineCommit, Reference: "packages/coding-agent/src/modes/interactive/interactive-mode.ts"}, Mapping: catalog.Mapping{Module: "codingagent", Target: issue32GoPackage + ".InteractiveMode", Kind: "contract"}, Status: catalog.StatusPartial, Milestone: "M6", Classification: "public-api", Partial: partial120(), Deviation: &catalog.Deviation{ADR: "docs/adr/0038-interactive-themes.md", Reason: "Instance-owned theme state; cancellable file polling of M5-approved paths and explicit diagnostics preserve last valid theme."}, Notes: "Issue #120; docs/learning/m6-interactive-themes.md. Reuses M5 resources, legacy AgentSession and v3 Session."}
	for _, item := range []struct{ kind, path, run string }{
		{"oracle", "parity/oracle/fixtures/theme-runtime.json", "node parity/oracle/theme-runtime.mjs <locked-pi-checkout> --check"},
		{"oracle", "parity/oracle/fixtures/themes-cli.json", "node parity/oracle/themes-cli.mjs <locked-pi-checkout> --check"},
		{"go-test", "codingagent/issue120_themes_test.go", "go test -race ./codingagent -run 'TestTheme.*120' -count=1"},
		{"go-test", "codingagent/issue120_interactive_test.go", "go test -race ./codingagent -run TestInteractiveThemeNotifications120 -count=1"},
		{"go-test", "cmd/pig/issue120_themes_test.go", "PIG_TEST_RACE=1 go test -race ./cmd/pig -run 120 -count=1"},
		{"go-test", "parity/terminal/themes.py", "go test ./cmd/pig -run 120 -count=1"},
		{"go-test", "codingagent/testdata/issue120_surface_golden.txt", "go test ./codingagent -run TestInteractiveThemesAPISnapshot120 -count=1"},
		{"manual", "examples/interactive-themes/main.go", "go run ./examples/interactive-themes"},
	} {
		data, err := os.ReadFile(filepath.Join(root, item.path))
		if err != nil {
			t.Fatal(err)
		}
		entry.Evidence = append(entry.Evidence, catalog.Evidence{Kind: item.kind, Ref: item.path, Baseline: issue32BaselineCommit, CaseID: "issue120-" + item.path, InputHash: fmt.Sprintf("sha256:%x", sha256.Sum256(data)), ExecutionMethod: item.run, Expected: "Theme selection, colors, hot reload, automatic updates, cancellation, draft and terminal restoration", Actual: "PASS; explicit partial scope", Platform: "any (SDK), darwin/linux (PTY)", CatalogID: entry.ID})
	}
	path := filepath.Join(root, "parity/catalog.jsonl")
	entries, err := catalog.LoadCatalog(path)
	if err != nil {
		t.Fatal(err)
	}
	constructors := map[string]catalog.Entry{}
	for _, symbol := range issue32Symbols(t) {
		if symbol.Name == "ThemeSelectorComponent" {
			expected, e := issue32ConstructorEntry(symbol, "M6")
			if e != nil {
				t.Fatal(e)
			}
			constructors[expected.ID] = expected
		}
	}
	found := false
	for i, e := range entries {
		if expected, ok := constructors[e.ID]; ok {
			if *updateIssue120 {
				entries[i] = expected
			} else if !reflect.DeepEqual(e, expected) {
				t.Fatal("theme selector constructor mapping drift")
			}
			delete(constructors, e.ID)
		}
		if e.ID == entry.ID {
			found = true
			if *updateIssue120 {
				entries[i] = entry
			} else if !reflect.DeepEqual(e, entry) {
				t.Fatal("theme evidence drift")
			}
		}
	}
	if len(constructors) > 0 {
		t.Fatal("missing theme selector constructor mapping")
	}
	if *updateIssue120 {
		if !found {
			entries = append(entries, entry)
		}
		data, err := catalog.EncodeEntries(entries)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(path, data, 0644); err != nil {
			t.Fatal(err)
		}
	} else if !found {
		t.Fatal("missing interactive themes contract")
	}
}

func TestInteractiveThemesAPISnapshot120(t *testing.T) {
	var b strings.Builder
	for _, v := range []any{codingagent.NewThemeController, codingagent.NewThemeSelectorComponent, codingagent.DetectTerminalTheme, codingagent.ThemeForRGB, (*codingagent.Theme).MarkdownTheme, (*codingagent.Transcript).SetTheme, (*tui.TextUI).SetStyles, (*tui.TextUI).SetTerminalColorHandler} {
		fmt.Fprintln(&b, reflect.TypeOf(v))
	}
	for _, typ := range []reflect.Type{reflect.TypeOf((*codingagent.ThemeController)(nil)), reflect.TypeOf((*codingagent.ThemeSelectorComponent)(nil))} {
		for i := 0; i < typ.NumMethod(); i++ {
			m := typ.Method(i)
			fmt.Fprintln(&b, m.Name, m.Type)
		}
	}
	path := "testdata/issue120_surface_golden.txt"
	if *updateIssue120 {
		if err := os.WriteFile(path, []byte(b.String()), 0644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(want) != b.String() {
		t.Fatal("Interactive branches API drift")
	}
}

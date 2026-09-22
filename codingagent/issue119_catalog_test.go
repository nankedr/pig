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

var updateIssue119 = flag.Bool("update-issue119", false, "refresh interactive branches evidence and API snapshot")

func partial119() *catalog.Partial {
	return &catalog.Partial{Supported: []string{
		"Issue #119: public SDK selectors compare pinned Pi fork history, tree active branch ordering, search, filters, paging, folding, labels and copy callbacks",
		"Real CLI PTY covers fork draft, user/assistant navigation, summary options cancellation, labels, branch summary, Provider context, original preservation, continuation, formal reader reopen and terminal restoration",
		"Interactive SDK checks summary failure/cancellation/custom instructions/skip preference, active generation cancellation/exclusion, draft preservation and persisted recovery",
	}, Unsupported: []string{
		"Exact tree layout, horizontal viewport panning, tool argument formatting and localized label timestamps remain partial; SDK empty fork selector has no timed auto-close",
		"System clipboard remains an explicit Capability Stub; SDK OnCopy callback is supported",
		"Extension hooks M7, full auth M11, images M12, six-platform M13 and package ecosystem #99 remain outside scope",
	}}
}
func TestInteractiveBranchesCatalog119(t *testing.T) {
	root := issue32RepoRoot(t)
	entry := catalog.Entry{SchemaVersion: catalog.SchemaVersion, ID: "contract:codingagent/interactive-branches", Upstream: catalog.Upstream{Module: "coding-agent", Repository: "https://github.com/badlogic/pi-mono", Commit: issue32BaselineCommit, Reference: "packages/coding-agent/src/modes/interactive/interactive-mode.ts"}, Mapping: catalog.Mapping{Module: "codingagent", Target: issue32GoPackage + ".InteractiveMode", Kind: "contract"}, Status: catalog.StatusPartial, Milestone: "M6", Classification: "public-api", Partial: partial119(), Deviation: &catalog.Deviation{ADR: "docs/adr/0037-interactive-branch-navigation.md", Reason: "Interactive fork and tree navigation drain their Prompt worker before committing; label writes reject active generation and keep editor state on persistence failure; visual and clipboard branches remain partial."}, Notes: "Issue #119; docs/learning/m6-interactive-branches.md. Reuses legacy AgentSession and v3 Session, with no transport retry changes."}
	for _, item := range []struct{ kind, path, run string }{
		{"oracle", "parity/oracle/fixtures/fork-selector.json", "node parity/oracle/branch-selectors.mjs <locked-pi-checkout> --check"},
		{"oracle", "parity/oracle/fixtures/tree-selector.json", "node parity/oracle/branch-selectors.mjs <locked-pi-checkout> --check"},
		{"oracle", "parity/oracle/fixtures/interactive-branches-cli.json", "node parity/oracle/interactive-branches-cli.mjs <locked-pi-checkout> --check"},
		{"go-test", "codingagent/issue119_selectors_test.go", "go test ./codingagent -run 'Test(Fork|Tree)SelectorParity119' -count=1"},
		{"go-test", "codingagent/issue119_interactive_test.go", "go test -race ./codingagent -run 'TestInteractive.*119' -count=1"},
		{"go-test", "codingagent/issue119_runtime_test.go", "go test ./codingagent -run 'Test(BranchSelectionRuntime|TreeLabelWriteFailure)119' -count=1"},
		{"go-test", "cmd/pig/issue119_branches_test.go", "PIG_TEST_RACE=1 go test -race ./cmd/pig -run 119 -count=1"},
		{"go-test", "parity/terminal/interactive-branches.py", "PIG_TEST_RACE=1 go test -race ./cmd/pig -run 119 -count=1"},
		{"go-test", "codingagent/testdata/issue119_surface_golden.txt", "go test ./codingagent -run TestInteractiveBranchesAPISnapshot119 -count=1"},
		{"manual", "examples/interactive-branches/main.go", "go run ./examples/interactive-branches"},
	} {
		data, err := os.ReadFile(filepath.Join(root, item.path))
		if err != nil {
			t.Fatal(err)
		}
		entry.Evidence = append(entry.Evidence, catalog.Evidence{Kind: item.kind, Ref: item.path, Baseline: issue32BaselineCommit, CaseID: "issue119-" + item.path, InputHash: fmt.Sprintf("sha256:%x", sha256.Sum256(data)), ExecutionMethod: item.run, Expected: "Fork and tree selection, labels, summary, cancellation, Provider context and persisted continuation", Actual: "PASS; explicit partial scope", Platform: "any (SDK), darwin/linux (PTY)", CatalogID: entry.ID})
	}
	path := filepath.Join(root, "parity/catalog.jsonl")
	entries, err := catalog.LoadCatalog(path)
	if err != nil {
		t.Fatal(err)
	}
	constructors := map[string]catalog.Entry{}
	for _, symbol := range issue32Symbols(t) {
		if symbol.Name == "TreeSelectorComponent" || symbol.Name == "UserMessageSelectorComponent" {
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
			if *updateIssue119 {
				entries[i] = expected
			} else if !reflect.DeepEqual(e, expected) {
				t.Fatal("branch selector constructor mapping drift")
			}
			delete(constructors, e.ID)
		}
		if e.ID == entry.ID {
			found = true
			if *updateIssue119 {
				entries[i] = entry
			} else if !reflect.DeepEqual(e, entry) {
				t.Fatal("session selection evidence drift")
			}
		}
	}
	if len(constructors) > 0 {
		t.Fatal("missing branch selector constructor mapping")
	}
	if *updateIssue119 {
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
		t.Fatal("missing interactive branches contract")
	}
}

func TestInteractiveBranchesAPISnapshot119(t *testing.T) {
	var b strings.Builder
	for _, v := range []any{codingagent.NewUserMessageSelectorComponent, codingagent.NewTreeSelectorComponent, (*codingagent.AgentSessionRuntime).Fork, (*codingagent.AgentSession).NavigateTree, (*tui.TextUI).SetEditorText} {
		fmt.Fprintln(&b, reflect.TypeOf(v))
	}
	for _, typ := range []reflect.Type{reflect.TypeOf((*codingagent.UserMessageSelectorComponent)(nil)), reflect.TypeOf((*codingagent.TreeSelectorComponent)(nil))} {
		for i := 0; i < typ.NumMethod(); i++ {
			m := typ.Method(i)
			fmt.Fprintln(&b, m.Name, m.Type)
		}
	}
	fmt.Fprintln(&b, issue71TypeSnapshot("TreeSelectorOptions", reflect.TypeOf(codingagent.TreeSelectorOptions{})))
	path := "testdata/issue119_surface_golden.txt"
	if *updateIssue119 {
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

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

var updateIssue122 = flag.Bool("update-issue122", false, "refresh interactive Bash evidence and API snapshot")

func partial122() *catalog.Partial {
	return &catalog.Partial{Supported: []string{"CLI !/!! routing, streaming merged output, exit/cancel status, preview/expand, truncation file and persisted history", "Single interactive Bash with concurrent Agent generation, deferred Session context, follow-up queue, cancellation and process/terminal cleanup", "Public BashExecutionComponent SDK, completed-state ownership, Session reader and real controlled Provider requests"}, Unsupported: []string{"Exact Pi borders/spinner animation and inherited Container custom child rendering", "Extension user_bash runtime M7, package ecosystem #99, full authentication M11, images M12, six-platform acceptance M13"}}
}
func issue122PromoteRuntimeEntry(e *catalog.Entry) bool {
	name := strings.TrimPrefix(e.Mapping.Target, issue32GoPackage+".")
	if strings.HasPrefix(e.ID, "constructor:") {
		name = strings.TrimPrefix(name, "New")
	}
	if name != "BashExecutionComponent" && !strings.HasPrefix(name, "BashExecutionComponent.") {
		return false
	}
	e.Status, e.Partial = catalog.StatusPartial, partial122()
	e.Notes = "Issue #122: contract:codingagent/interactive-bash owns lifecycle and CLI evidence; Container custom rendering remains partial."
	return true
}
func TestInteractiveBashCatalog122(t *testing.T) {
	root := issue32RepoRoot(t)
	entry := catalog.Entry{SchemaVersion: catalog.SchemaVersion, ID: "contract:codingagent/interactive-bash", Upstream: catalog.Upstream{Module: "coding-agent", Repository: "https://github.com/badlogic/pi-mono", Commit: issue32BaselineCommit, Reference: "packages/coding-agent/src/modes/interactive/interactive-mode.ts"}, Mapping: catalog.Mapping{Module: "codingagent", Target: issue32GoPackage + ".InteractiveMode", Kind: "contract"}, Status: catalog.StatusPartial, Milestone: "M6", Classification: "public-api", Partial: partial122(), Deviation: &catalog.Deviation{ADR: "docs/adr/0040-interactive-bash.md", Reason: "Completed components reject late chunks; Bash execution identity isolates late interrupts."}, Notes: "Issue #122; docs/learning/m6-interactive-bash.md and docs/mappings/typescript-to-go/m6-interactive-bash.md. Reuses legacy AgentSession and v3 Session."}
	for _, item := range []struct{ kind, path, run string }{
		{"oracle", "parity/oracle/fixtures/bash-cli.json", "make m6-bash-oracle PIG_PI_ORACLE_CHECKOUT=<locked-pi-checkout>"},
		{"go-test", "parity/terminal/bash.py", "go test ./cmd/pig -run 122 -count=1"},
		{"go-test", "codingagent/issue122_interactive_test.go", "go test -race ./codingagent -run 122 -count=1"},
		{"go-test", "codingagent/issue122_component_test.go", "go test ./codingagent -run TestBashComponent122 -count=1"},
		{"go-test", "cmd/pig/issue122_bash_test.go", "PIG_TEST_RACE=1 go test -race ./cmd/pig -run 122 -count=1"},
		{"go-test", "codingagent/testdata/issue122_surface_golden.txt", "go test ./codingagent -run TestInteractiveBashAPISnapshot122 -count=1"},
		{"manual", "examples/interactive-bash/main.go", "go run ./examples/interactive-bash"},
	} {
		data, err := os.ReadFile(filepath.Join(root, item.path))
		if err != nil {
			t.Fatal(err)
		}
		entry.Evidence = append(entry.Evidence, catalog.Evidence{Kind: item.kind, Ref: item.path, Baseline: issue32BaselineCommit, CaseID: "issue122-" + item.path, InputHash: fmt.Sprintf("sha256:%x", sha256.Sum256(data)), ExecutionMethod: item.run, Expected: "Bash routing, context inclusion, output, cancellation, concurrency, retained Session and cleanup through public SDK and CLI", Actual: "PASS; explicit partial scope", Platform: "any (SDK), darwin/linux (PTY)", CatalogID: entry.ID})
	}
	path := filepath.Join(root, "parity/catalog.jsonl")
	entries, err := catalog.LoadCatalog(path)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for i, e := range entries {
		if e.ID == entry.ID {
			found = true
			if *updateIssue122 {
				entries[i] = entry
			} else if !reflect.DeepEqual(e, entry) {
				t.Fatal("interactive Bash evidence drift")
			}
		}
	}
	if *updateIssue122 {
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
		t.Fatal("missing interactive Bash contract")
	}
}

func TestInteractiveBashAPISnapshot122(t *testing.T) {
	var b strings.Builder
	for _, fn := range []any{codingagent.NewBashExecutionComponent, (*codingagent.BashExecutionComponent).AppendOutput, (*codingagent.BashExecutionComponent).SetComplete, (*codingagent.BashExecutionComponent).SetExpanded, (*codingagent.BashExecutionComponent).Render, (*codingagent.BashExecutionComponent).GetOutput, (*codingagent.BashExecutionComponent).GetCommand, (*codingagent.AgentSession).ExecuteBash, (*codingagent.AgentSession).AbortBash, (*tui.TextUI).SetBash} {
		fmt.Fprintln(&b, reflect.TypeOf(fn))
	}
	fmt.Fprintln(&b, issue71TypeSnapshot("TextInput", reflect.TypeOf(tui.TextInput{})))
	path := "testdata/issue122_surface_golden.txt"
	if *updateIssue122 {
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
		t.Fatal("interactive Bash API drift")
	}
}

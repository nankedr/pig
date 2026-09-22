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

var updateIssue118 = flag.Bool("update-issue118", false, "refresh session selections evidence and API snapshot")

func partial118() *catalog.Partial {
	return &catalog.Partial{Supported: []string{
		"Issue #118: pinned Pi public selector fixture covers fuzzy token/phrase/regex search, named filter, threaded/recent/relevance sorting, current/all scope, clamped selection, pagination, paths and cancellation",
		"Real CLI PTY compares startup resume, in-session cancel, new/switch Session, generation, saved names and user histories, model/thinking requests and reopen; public SDK exercises persistence and recoverable failures",
		"Interactive SDK race verifies cancellation without aborting generation, switching abort/drain, old resource cleanup, CWD trust denial and authorized Context Files; active Session deletion protection and rename/delete confirmations",
	}, Unsupported: []string{
		"Regex is the Go RE2 subset; JavaScript lookaround/backreferences and all Unicode ranking branches remain partial",
		"Trash CLI fallback, missing-CWD replacement dialog, asynchronous discovery progress, exact tree connectors/relative dates/pixel layout and loader concurrency are unimplemented or unverified",
		"Startup rename is unavailable as in Pi; extension callbacks M7, full auth M11, images M12, six-platform M13 and package ecosystem #99 remain outside scope",
	}}
}
func TestSessionSelectionCatalog118(t *testing.T) {
	root := issue32RepoRoot(t)
	entry := catalog.Entry{SchemaVersion: catalog.SchemaVersion, ID: "contract:codingagent/session-selection", Upstream: catalog.Upstream{Module: "coding-agent", Repository: "https://github.com/badlogic/pi-mono", Commit: issue32BaselineCommit, Reference: "packages/coding-agent/src/modes/interactive/interactive-mode.ts"}, Mapping: catalog.Mapping{Module: "codingagent", Target: issue32GoPackage + ".InteractiveMode", Kind: "contract"}, Status: catalog.StatusPartial, Milestone: "M6", Classification: "public-api", Partial: partial118(), Deviation: &catalog.Deviation{ADR: "docs/adr/0036-interactive-session-selection.md", Reason: "Interactive replacement drains its Prompt worker and validates persisted targets before replacement; synchronous discovery, RE2 regex and unlink-only deletion have explicit partial scope."}, Notes: "Issue #118; docs/learning/m6-session-selection.md. Reuses legacy AgentSession and v3 Session, with no transport retry changes."}
	for _, item := range []struct{ kind, path, run string }{
		{"oracle", "parity/oracle/fixtures/session-selector.json", "node parity/oracle/session-selector.mjs <locked-pi-checkout> --check"},
		{"oracle", "parity/oracle/fixtures/session-selection-cli.json", "node parity/oracle/session-selection-cli.mjs <locked-pi-checkout> --check"},
		{"go-test", "codingagent/issue118_selector_test.go", "go test ./codingagent -run TestSessionSelectorParity118 -count=1"},
		{"go-test", "codingagent/issue118_sessions_test.go", "go test -race ./codingagent -run 'TestSessionSelect.*118' -count=1"},
		{"go-test", "codingagent/issue118_interactive_test.go", "go test -race ./codingagent -run TestInteractiveSessionSwitchTrustAndBusy118 -count=1"},
		{"go-test", "cmd/pig/issue118_sessions_test.go", "PIG_TEST_RACE=1 go test -race ./cmd/pig -run 118 -count=1"},
		{"go-test", "parity/terminal/session-selection.py", "PIG_TEST_RACE=1 go test -race ./cmd/pig -run 118 -count=1"},
		{"go-test", "codingagent/testdata/issue118_surface_golden.txt", "go test ./codingagent -run TestSessionSelectionAPISnapshot118 -count=1"},
		{"manual", "examples/session-selection/main.go", "go run ./examples/session-selection"},
	} {
		data, err := os.ReadFile(filepath.Join(root, item.path))
		if err != nil {
			t.Fatal(err)
		}
		entry.Evidence = append(entry.Evidence, catalog.Evidence{Kind: item.kind, Ref: item.path, Baseline: issue32BaselineCommit, CaseID: "issue118-" + item.path, InputHash: fmt.Sprintf("sha256:%x", sha256.Sum256(data)), ExecutionMethod: item.run, Expected: "Session discovery, selection, replacement, trust, cleanup and restored generation", Actual: "PASS; explicit partial scope", Platform: "any (SDK), darwin/linux (PTY)", CatalogID: entry.ID})
	}
	path := filepath.Join(root, "parity/catalog.jsonl")
	entries, err := catalog.LoadCatalog(path)
	if err != nil {
		t.Fatal(err)
	}
	constructors := map[string]catalog.Entry{}
	for _, symbol := range issue32Symbols(t) {
		if symbol.Name == "SessionSelectorComponent" {
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
			if *updateIssue118 {
				entries[i] = expected
			} else if !reflect.DeepEqual(e, expected) {
				t.Fatal("session selector constructor mapping drift")
			}
			delete(constructors, e.ID)
		}
		if e.ID == entry.ID {
			found = true
			if *updateIssue118 {
				entries[i] = entry
			} else if !reflect.DeepEqual(e, entry) {
				t.Fatal("session selection evidence drift")
			}
		}
	}
	if len(constructors) > 0 {
		t.Fatal("missing session selector constructor mapping")
	}
	if *updateIssue118 {
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
		t.Fatal("missing session selection contract")
	}
}

func TestSessionSelectionAPISnapshot118(t *testing.T) {
	var b strings.Builder
	fmt.Fprintln(&b, reflect.TypeOf(codingagent.NewSessionSelectorComponent))
	typ := reflect.TypeOf((*codingagent.SessionSelectorComponent)(nil))
	for i := 0; i < typ.NumMethod(); i++ {
		m := typ.Method(i)
		fmt.Fprintln(&b, m.Name, m.Type)
	}
	fmt.Fprintln(&b, issue71TypeSnapshot("SessionSelectorOptions", reflect.TypeOf(codingagent.SessionSelectorOptions{})))
	for _, value := range []any{codingagent.ListSessions, codingagent.ListAllSessions, (*codingagent.AgentSessionRuntime).SwitchSession, (*codingagent.AgentSessionRuntime).NewSession, (*tui.TextUI).ResetSession} {
		fmt.Fprintln(&b, reflect.TypeOf(value))
	}
	path := "testdata/issue118_surface_golden.txt"
	if *updateIssue118 {
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
		t.Fatal("Session selection API drift")
	}
}

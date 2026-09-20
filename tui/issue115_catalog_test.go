package tui_test

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

var updateIssue115 = flag.Bool("update-issue115", false, "refresh autocomplete evidence and API snapshot")

func partial115() *catalog.Partial {
	return &catalog.Partial{Supported: []string{
		"Issue #115: pinned Pi provider/editor fixtures for command fuzzy filtering, argument callbacks, path prefixes, fd scope/ranking, quotes, Unicode and middle replacement",
		"Editor async cancellation, stale result isolation, selection, Tab/Enter acceptance, Escape drafts, undo and configurable visible rows",
		"Session resource snapshot respects priority, diagnostics, disable switches and Project Trust; actual CLI PTY submits templates, Skills and file references through AgentSession to Provider",
	}, Unsupported: []string{
		"Model/login argument menus and undelivered product commands remain explicit Stub; extension commands/runtime M7, authentication M11, images M12, package ecosystem and six-platform acceptance M13 remain outside this slice",
		"Full locale/Unicode ordering, all symlink/fd-output branches, exact Pi debounce/request scheduling and autocomplete pixel layout remain partial; fd protocol is tested with controlled executables",
	}}
}
func issue115Promote(e *catalog.Entry) bool {
	name := strings.TrimPrefix(e.Mapping.Target, issue31GoPackage+".")
	supported := name == "CombinedAutocompleteProvider" || strings.HasPrefix(name, "CombinedAutocompleteProvider.")
	for _, member := range []string{"SetAutocompleteProvider", "SetAutocompleteMaxVisible", "IsShowingAutocomplete"} {
		supported = supported || name == "Editor."+member
	}
	if !supported {
		return false
	}
	e.Status = catalog.StatusPartial
	e.Partial = partial115()
	e.Notes = "Issue #115; contract:tui/autocomplete"
	e.Evidence = []catalog.Evidence{{Kind: "go-test", Ref: "tui/issue115_autocomplete_test.go", Baseline: issue31BaselineCommit, CaseID: e.ID, InputHash: issue31SurfaceHash, ExecutionMethod: "go test ./tui -run 115 -count=1", Expected: "pinned Pi public autocomplete semantics", Actual: "PASS with explicit partial branches; full evidence in contract:tui/autocomplete", Platform: "any (SDK), darwin/linux (controlled fd and PTY)", CatalogID: e.ID}}
	return true
}
func TestAutocompleteCatalog115(t *testing.T) {
	root := issue31RepoRoot(t)
	entry := catalog.Entry{SchemaVersion: catalog.SchemaVersion, ID: "contract:tui/autocomplete", Upstream: catalog.Upstream{Module: "tui", Repository: "https://github.com/badlogic/pi-mono", Commit: issue31BaselineCommit, Reference: "packages/tui/src/autocomplete.ts"}, Mapping: catalog.Mapping{Module: "tui", Target: issue31GoPackage + ".CombinedAutocompleteProvider", Kind: "contract"}, Status: catalog.StatusPartial, Milestone: "M6", Classification: "public-api", Partial: partial115(), Notes: "Issue #115; docs/learning/m6-autocomplete.md. Go cursor offsets are UTF-8 bytes. Existing AgentSession/v3 Session and authorized resource loading are reused; core remains CGO-free."}
	for _, item := range []struct{ kind, path, run string }{
		{"oracle", "parity/oracle/fixtures/autocomplete.json", "node parity/oracle/autocomplete.mjs <locked-pi-checkout> --check"},
		{"oracle", "parity/oracle/fixtures/autocomplete-editor.json", "node parity/oracle/autocomplete-editor.mjs <locked-pi-checkout> --check"},
		{"oracle", "parity/oracle/fixtures/autocomplete-cli.json", "node parity/oracle/autocomplete-cli.mjs <locked-pi-checkout> --check"},
		{"go-test", "tui/issue115_autocomplete_test.go", "go test -race ./tui -run 'TestAutocomplete.*115' -count=1"},
		{"go-test", "tui/issue115_fd_unix_test.go", "go test -race ./tui -run TestAutocompleteFDCancellation115 -count=1"},
		{"go-test", "codingagent/issue115_autocomplete_test.go", "go test -race ./codingagent -run 115 -count=1"},
		{"go-test", "cmd/pig/issue115_autocomplete_test.go", "go test ./cmd/pig -run 115 -count=1"},
		{"go-test", "cmd/pig/issue110_editor_test.go", "go test ./cmd/pig -run TestPigAutocomplete115 -count=1"},
		{"go-test", "tui/testdata/issue115_surface_golden.txt", "go test ./tui -run TestAutocompleteAPISnapshot115 -count=1"},
		{"manual", "examples/autocomplete/main.go", "go run ./examples/autocomplete"},
	} {
		data, err := os.ReadFile(filepath.Join(root, item.path))
		if err != nil {
			t.Fatal(err)
		}
		entry.Evidence = append(entry.Evidence, catalog.Evidence{Kind: item.kind, Ref: item.path, Baseline: issue31BaselineCommit, CaseID: "issue115-" + item.path, InputHash: fmt.Sprintf("sha256:%x", sha256.Sum256(data)), ExecutionMethod: item.run, Expected: "autocomplete and real resource submission through public SDK and CLI", Actual: "PASS; explicit partial scope", Platform: "any (SDK), darwin/linux (controlled fd and PTY)", CatalogID: entry.ID})
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
			if *updateIssue115 {
				entries[i] = entry
			} else if !reflect.DeepEqual(e, entry) {
				t.Fatal("autocomplete evidence drift")
			}
		}
	}
	if *updateIssue115 {
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
		t.Fatal("missing autocomplete contract")
	}
}
func TestAutocompleteAPISnapshot115(t *testing.T) {
	var b strings.Builder
	for _, v := range []any{(*tui.CombinedAutocompleteProvider)(nil), (*tui.Editor)(nil), (*tui.TextUI)(nil)} {
		typ := reflect.TypeOf(v)
		fmt.Fprintln(&b, typ)
		for i := 0; i < typ.NumMethod(); i++ {
			m := typ.Method(i)
			fmt.Fprintln(&b, m.Name, m.Type)
		}
	}
	fmt.Fprintln(&b, reflect.TypeOf(codingagent.NewSessionAutocompleteProvider))
	path := "testdata/issue115_surface_golden.txt"
	if *updateIssue115 {
		if err := os.WriteFile(path, []byte(b.String()), 0644); err != nil {
			t.Fatal(err)
		}
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != b.String() {
		t.Fatal("autocomplete API drift")
	}
}

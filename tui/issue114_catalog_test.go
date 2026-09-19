package tui_test

import (
	"crypto/sha256"
	"flag"
	"fmt"
	"github.com/nankedr/pig/codingagent"
	"github.com/nankedr/pig/internal/baseline"
	"github.com/nankedr/pig/internal/catalog"
	"github.com/nankedr/pig/internal/parity"
	"github.com/nankedr/pig/tui"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

var updateIssue114 = flag.Bool("update-issue114", false, "refresh dialogs and trust contract and API snapshot")

func partial114() *catalog.Partial {
	return &catalog.Partial{Supported: []string{"Issue #114: public SelectList filtering/navigation/selection and reusable SelectDialog lifecycle", "Overlay sizing, anchors, margins, clipping, visibility, focus and close/resize input restoration", "Real CLI trust choices and second startup compared with locked Pi PTY fixture; SDK priority, FIFO no-sensitive-read, save failure and context cancellation"}, Unsupported: []string{"Complex nested overlay resize focus restoration remains unverified; extension selectors/runtime M7, package ecosystem, images M12 and six-platform acceptance M13 remain partial or Stub"}}
}
func issue114Promote(e *catalog.Entry) bool {
	name := strings.TrimPrefix(e.Mapping.Target, issue31GoPackage+".")
	supported := name == "SelectList" || strings.HasPrefix(name, "SelectList.")
	for _, n := range []string{"TUIBase", "TUIAltScreen", "TUIMainScreen"} {
		for _, member := range []string{"ShowOverlay", "HideOverlay", "HasOverlay", "HasOverlayEntries"} {
			supported = supported || name == n+"."+member
		}
	}

	if !supported {
		return false
	}
	e.Status = catalog.StatusPartial
	e.Partial = partial114()
	e.Evidence = []catalog.Evidence{{Kind: "go-test", Ref: "tui/issue114_dialogs_test.go", Baseline: issue31BaselineCommit, CaseID: e.ID, InputHash: issue31SurfaceHash, ExecutionMethod: "go test ./tui -run '114' -count=1", Expected: "selection and overlays through public SDK", Actual: "PASS; fixture and CLI evidence in contract:tui/dialogs-trust", Platform: "any (SDK), darwin/linux (PTY)", CatalogID: e.ID}}
	e.Notes = "Issue #114; contract:tui/dialogs-trust"
	return true
}
func TestDialogsTrustCatalog114(t *testing.T) {
	root := issue31RepoRoot(t)
	entry := catalog.Entry{SchemaVersion: catalog.SchemaVersion, ID: "contract:tui/dialogs-trust", Upstream: catalog.Upstream{Module: "tui", Repository: "https://github.com/badlogic/pi-mono", Commit: issue31BaselineCommit, Reference: "packages/tui/src/tui.ts"}, Mapping: catalog.Mapping{Module: "tui", Target: issue31GoPackage + ".ShowSelectDialog", Kind: "contract"}, Status: catalog.StatusPartial, Milestone: "M6", Classification: "public-api", Partial: partial114(), Notes: "Issue #114; docs/learning/m6-trust-dialog.md; no parent issue mutation."}
	for _, item := range []struct{ kind, path, run string }{
		{"oracle", "parity/oracle/fixtures/dialogs.json", "node --experimental-strip-types parity/oracle/dialogs.mjs <locked-pi-checkout> --check"},
		{"oracle", "parity/oracle/fixtures/overlays.json", "node parity/oracle/overlays.mjs <locked-pi-checkout> --check"},
		{"oracle", "parity/oracle/fixtures/trust-dialog.json", "node parity/oracle/trust-dialog.mjs <locked-pi-checkout> --check"},
		{"go-test", "tui/issue114_dialogs_test.go", "go test -race ./tui -run 114 -count=1"},
		{"go-test", "codingagent/issue114_trust_test.go", "go test -race ./codingagent -run 114 -count=1"},
		{"go-test", "cmd/pig/issue114_trust_test.go", "go test ./cmd/pig -run 114 -count=1"},
		{"go-test", "tui/testdata/issue114_surface_golden.txt", "go test ./tui -run TestDialogsTrustAPISnapshot114 -count=1"},
		{"manual", "examples/trust-dialog/main.go", "go run ./examples/trust-dialog"},
	} {
		data, err := os.ReadFile(filepath.Join(root, item.path))
		if err != nil {
			t.Fatal(err)
		}
		actual := "PASS with explicit partial branches"
		if item.kind == "manual" {
			actual = "Runnable demo; see docs/learning/m6-trust-dialog.md for local terminal evidence"
		}
		entry.Evidence = append(entry.Evidence, catalog.Evidence{Kind: item.kind, Ref: item.path, Baseline: issue31BaselineCommit, CaseID: "issue114-" + item.path, InputHash: fmt.Sprintf("sha256:%x", sha256.Sum256(data)), ExecutionMethod: item.run, Expected: "trust decisions gate project resources and dialogs restore input", Actual: actual, Platform: "any (SDK), darwin/linux (PTY)", CatalogID: entry.ID})
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
			if *updateIssue114 {
				entries[i] = entry
			} else if !reflect.DeepEqual(e, entry) {
				t.Fatal("dialogs and trust catalog drift")
			}
		}
	}
	if *updateIssue114 {
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
		t.Fatal("missing dialogs and trust catalog")
	}
}
func TestDialogsTrustAPISnapshot114(t *testing.T) {
	var b strings.Builder
	for _, v := range []any{(*tui.SelectList)(nil), (*tui.SelectDialog)(nil)} {
		typ := reflect.TypeOf(v)
		fmt.Fprintln(&b, typ)
		for i := 0; i < typ.NumMethod(); i++ {
			m := typ.Method(i)
			fmt.Fprintln(&b, m.Name, m.Type)
		}
	}
	for _, v := range []any{codingagent.PrepareProjectSettingsOptions{}, codingagent.ProjectTrustOption{}} {
		typ := reflect.TypeOf(v)
		fmt.Fprintln(&b, typ)
		for i := 0; i < typ.NumField(); i++ {
			f := typ.Field(i)
			fmt.Fprintln(&b, f.Name, f.Type)
		}
	}
	for _, v := range []any{tui.ShowSelectDialog, tui.NewSelectDialog, codingagent.PrepareProjectSettings, codingagent.GetProjectTrustOptions} {
		fmt.Fprintln(&b, reflect.TypeOf(v))
	}

	path := "testdata/issue114_surface_golden.txt"
	if *updateIssue114 {
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
		t.Fatal("dialogs and trust API snapshot drift")
	}
}

func TestDialogFixturesLocked114(t *testing.T) {
	lock, _, err := baseline.Load("../parity/baseline")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"dialogs", "overlays", "trust-dialog"} {
		if _, err = parity.LoadFixture("../parity/oracle/fixtures/"+name+".json", parity.Baseline{ID: lock.BaselineID, Commit: lock.Upstream.Commit, Repository: lock.Upstream.Repository}); err != nil {
			t.Fatal(err)
		}
	}
}

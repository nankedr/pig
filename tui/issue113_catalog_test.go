package tui_test

import (
	"crypto/sha256"
	"flag"
	"fmt"
	"github.com/nankedr/pig/internal/catalog"
	"github.com/nankedr/pig/tui"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

var updateIssue113 = flag.Bool("update-issue113", false, "refresh layout scrolling contract and API snapshot")

func partial113() *catalog.Partial {
	return &catalog.Partial{Supported: []string{"Issue #113: public stack allocation, clipped layout frames, nested scroll hit testing, keyboard/wheel/scrollbar drag, tail following and bounded history anchors", "Pi native main-screen scrollback, differential main/alternate screen redraw, cursor-only updates, state restore and forced redraw", "CLI regular/fullscreen via legacy AgentSession and v3 Session; fixed Pi SDK and real CLI PTY fixtures; resize/editor preservation; no CGO"}, Unsupported: []string{"Selection/copy, URL opening, prompt jumps, overlays, flashes, color queries and resume hints remain Stub or unverified; images M12, extensions M7 and six-platform acceptance M13 are deferred"}}
}
func issue113Promote(e *catalog.Entry) bool {
	name := strings.TrimPrefix(e.Mapping.Target, issue31GoPackage+".")
	supported := false
	for _, n := range []string{"ScrollView", "Stack", "VStack", "HStack", "AllocateStackSizes", "VisibleStackEntries", "RenderLayoutFrame", "GetScrollViewBox", "GetScrollViewsAt", "GetScrollbarGeometry", "CompositeTUILine"} {
		supported = supported || name == n || strings.HasPrefix(name, n+".")
	}
	for _, n := range []string{"TUIBase", "TUIAltScreen", "TUIMainScreen"} {
		if name == n {
			supported = true
		}
		if strings.HasPrefix(name, n+".") {
			member := strings.TrimPrefix(name, n+".")
			for _, m := range []string{"Start", "Stop", "Render", "RenderNow", "RequestRender", "HandleInput", "SetFocus", "SetShowHardwareCursor", "SetClearOnShrink", "AddInputListener", "RemoveInputListener", "SetLayoutRoot", "ScrollBy", "ScrollToTop", "ScrollToBottom", "ViewportTop", "IsFollowingOutput", "CaptureRenderState", "RestoreRenderState"} {
				supported = supported || member == m
			}
		}
	}
	if !supported {
		return false
	}
	e.Status = catalog.StatusPartial
	e.Partial = partial113()
	e.Evidence = []catalog.Evidence{{Kind: "go-test", Ref: "tui/issue113_layout_test.go", Baseline: issue31BaselineCommit, CaseID: e.ID, InputHash: issue31SurfaceHash, ExecutionMethod: "go test ./tui -run '113' -count=1", Expected: "layout and scrolling through public SDK", Actual: "PASS; fixture and CLI evidence in contract:tui/layout-scrolling", Platform: "any (SDK), darwin/linux (PTY)", CatalogID: e.ID}}
	e.Notes = "Issue #113; contract:tui/layout-scrolling"
	return true
}
func TestLayoutScrollingCatalog113(t *testing.T) {
	root := issue31RepoRoot(t)
	entry := catalog.Entry{SchemaVersion: catalog.SchemaVersion, ID: "contract:tui/layout-scrolling", Upstream: catalog.Upstream{Module: "tui", Repository: "https://github.com/badlogic/pi-mono", Commit: issue31BaselineCommit, Reference: "packages/tui/src/layout.ts"}, Mapping: catalog.Mapping{Module: "tui", Target: issue31GoPackage + ".RenderLayoutFrame", Kind: "contract"}, Status: catalog.StatusPartial, Milestone: "M6", Classification: "public-api", Partial: partial113(), Notes: "Issue #113; docs/learning/m6-layout-scrolling.md; no parent issue mutation."}
	for _, item := range []struct{ kind, path, run string }{
		{"oracle", "parity/oracle/fixtures/main-screen.json", "node parity/oracle/main-screen.mjs <locked-pi-checkout> --check"},
		{"go-test", "tui/issue113_main_screen_test.go", "go test -race ./tui -run '113' -count=1"},
		{"oracle", "parity/oracle/fixtures/layout-scrolling.json", "node parity/oracle/layout-scrolling.mjs <locked-pi-checkout> --check"},
		{"oracle", "parity/oracle/fixtures/layout-scrolling-cli.json", "node parity/oracle/layout-scrolling-cli.mjs <locked-pi-checkout> --check"},
		{"go-test", "tui/issue113_layout_test.go", "go test ./tui -run 'TestLayout.*113' -count=1"},
		{"go-test", "tui/issue113_renderer_test.go", "go test -race ./tui -run '113' -count=1"},
		{"go-test", "cmd/pig/issue113_layout_test.go", "go test ./cmd/pig -run 'TestPig.*113' -count=1"},
		{"go-test", "tui/testdata/issue113_surface_golden.txt", "go test ./tui -run TestLayoutScrollingAPISnapshot113 -count=1"},
		{"manual", "examples/layout-scrolling/main.go", "go run ./examples/layout-scrolling"},
	} {
		data, err := os.ReadFile(filepath.Join(root, item.path))
		if err != nil {
			t.Fatal(err)
		}
		actual := "PASS with explicit partial branches"
		if item.kind == "manual" {
			actual = "Runnable demo; see docs/learning/m6-layout-scrolling.md for local terminal evidence"
		}
		entry.Evidence = append(entry.Evidence, catalog.Evidence{Kind: item.kind, Ref: item.path, Baseline: issue31BaselineCommit, CaseID: "issue113-" + item.path, InputHash: fmt.Sprintf("sha256:%x", sha256.Sum256(data)), ExecutionMethod: item.run, Expected: "long conversations retain viewport and editor across output and resize", Actual: actual, Platform: "any (SDK), darwin/linux (PTY)", CatalogID: entry.ID})
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
			if *updateIssue113 {
				entries[i] = entry
			} else if !reflect.DeepEqual(e, entry) {
				t.Fatal("layout scrolling catalog drift")
			}
		}
	}
	if *updateIssue113 {
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
		t.Fatal("missing layout scrolling catalog")
	}
}
func TestLayoutScrollingAPISnapshot113(t *testing.T) {
	var b strings.Builder
	for _, v := range []any{(*tui.ScrollView)(nil), (*tui.VStack)(nil), (*tui.HStack)(nil), (*tui.TUIAltScreen)(nil), (*tui.TUIMainScreen)(nil), (*tui.TextUI)(nil)} {
		typ := reflect.TypeOf(v)
		fmt.Fprintln(&b, typ)
		for i := 0; i < typ.NumMethod(); i++ {
			m := typ.Method(i)
			fmt.Fprintln(&b, m.Name, m.Type)
		}
	}
	typ := reflect.TypeOf(tui.TextUIOptions{})
	fmt.Fprintln(&b, typ)
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		fmt.Fprintln(&b, f.Name, f.Type)
	}
	path := "testdata/issue113_surface_golden.txt"
	if *updateIssue113 {
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
		t.Fatal("layout scrolling API snapshot drift")
	}
}

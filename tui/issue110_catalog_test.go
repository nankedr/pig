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

	"github.com/nankedr/pig/internal/catalog"
)

var updateIssue110Catalog = flag.Bool("update-issue110-catalog", false, "regenerate multiline editor evidence")

const issue110CatalogID = "contract:tui/multiline-editor"

func issue110Partial() *catalog.Partial {
	return &catalog.Partial{
		Supported: []string{
			"Issue #110: shared CLI and public Editor multiline insertion/deletion, grapheme/word/line/visual navigation, paging, character jumps, history drafts, undo, kill/yank and yank-pop",
			"Pinned Pi SDK fixture compares text, UTF-8 cursor projection, callbacks and complete rendered frames for CJK, combining marks, emoji/ZWJ/flags, empty lines and resize",
			"Bracketed paste buffers fragments without shortcut/submission effects, normalizes CRLF/tabs, collapses large input and expands it at submission; paste registry participates in undo",
			"Real PTY CLI compares edited Provider inputs with pinned Pi; empty input and canceled drafts produce no unintended requests; restored Session user messages seed input history",
		}, Unsupported: []string{
			"ICU dictionary word segmentation, Unicode-version/terminal-width differences and all atomic-marker vertical-navigation combinations remain partial",
			"Issue #111 keyboard negotiation/keybindings evidence is owned by contract:tui/terminal-keys; issue #115 autocomplete evidence is owned by contract:tui/autocomplete; fullscreen/Markdown/scrollback, images, extensions and six-platform acceptance remain outside this slice",
		}}
}
func issue110Promote(entry *catalog.Entry) bool {
	supported := entry.Mapping.Target == issue31GoPackage+".Editor" || entry.Mapping.Target == issue31GoPackage+".WordWrapLine"
	for _, member := range []string{"AddToHistory", "GetCursor", "GetExpandedText", "GetLines", "GetText", "HandleInput", "InsertTextAtCursor", "Render", "SetText", "SetPaddingX", "Focused", "OnSubmit", "OnChange", "DisableSubmit", "BorderColor"} {
		supported = supported || entry.Mapping.Target == issue31GoPackage+".Editor."+member
	}
	if !supported {
		return false
	}
	entry.Status = catalog.StatusPartial
	entry.Partial = issue110Partial()
	entry.Evidence = []catalog.Evidence{{Kind: "go-test", Ref: "tui/issue110_editor_test.go", Baseline: issue31BaselineCommit, CaseID: entry.ID, InputHash: issue110FixtureHash, ExecutionMethod: "go test ./tui -run '^TestEditorParity110$' -count=1", Expected: "pinned Pi public Editor state, render and callback behavior", Actual: "PASS; multiline editor fixture replay with explicit partial branches", Platform: "any", CatalogID: entry.ID}}
	entry.Notes = "Issue #110 behavior and detailed evidence: " + issue110CatalogID
	return true
}
func TestIssue110EditorCatalog(t *testing.T) {
	root := issue31RepoRoot(t)
	entry := catalog.Entry{SchemaVersion: catalog.SchemaVersion, ID: issue110CatalogID, Upstream: catalog.Upstream{Module: "tui", Repository: "https://github.com/badlogic/pi-mono", Commit: issue31BaselineCommit, Reference: "packages/tui/src/components/editor.ts"}, Mapping: catalog.Mapping{Module: "tui", Target: issue31GoPackage + ".Editor", Kind: "contract"}, Status: catalog.StatusPartial, Milestone: "M6", Classification: "public-api", Partial: issue110Partial(), Notes: "Public SDK cursor and wrap indices use UTF-8 bytes; Pi Oracle projects UTF-16 indices. TextUI serializes editor access; core remains CGO-free."}
	for _, item := range []struct{ kind, path, run string }{
		{"oracle", "parity/oracle/fixtures/editor.json", "node parity/oracle/editor.mjs <locked-pi-checkout> --check"},
		{"oracle", "parity/oracle/fixtures/editor-cli.json", "node parity/oracle/editor-cli.mjs <locked-pi-checkout> --check"},
		{"go-test", "tui/issue110_editor_test.go", "go test ./tui -run '^TestEditorParity110$' -count=1"},
		{"go-test", "cmd/pig/issue110_editor_test.go", "go test ./cmd/pig -run '^TestPigMultilineEditor110$' -count=1"},
		{"go-test", "codingagent/issue110_interactive_test.go", "go test -race ./codingagent -run '^TestInteractiveSDKRestoresEditorHistory110$' -count=1"},
		{"go-test", "tui/testdata/issue31_surface_golden.txt", "go test ./tui -run '^TestIssue31LockedGoAPISnapshot$' -count=1"},
		{"manual", "examples/multiline-editor/main.go", "go run ./examples/multiline-editor"},
	} {
		data, err := os.ReadFile(filepath.Join(root, item.path))
		if err != nil {
			t.Fatal(err)
		}
		hash := fmt.Sprintf("sha256:%x", sha256.Sum256(data))
		if strings.HasSuffix(item.path, "/editor.json") && hash != issue110FixtureHash {
			t.Fatal("Editor fixture evidence hash drift")
		}
		entry.Evidence = append(entry.Evidence, catalog.Evidence{Kind: item.kind, Ref: item.path, Baseline: issue31BaselineCommit, CaseID: "issue110-" + item.path, InputHash: hash, ExecutionMethod: item.run, Expected: "multiline editor behavior through public SDK and actual Provider inputs through CLI PTY", Actual: "PASS; pinned Pi fixture replay and explicit partial scope", Platform: "darwin/linux (CLI), any (SDK)", CatalogID: entry.ID})
	}
	path := filepath.Join(root, "parity/catalog.jsonl")
	entries, err := catalog.LoadCatalog(path)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for i, row := range entries {
		if row.ID == entry.ID {
			found = true
			if *updateIssue110Catalog {
				entries[i] = entry
			} else if !reflect.DeepEqual(row, entry) {
				t.Fatal("editor catalog evidence drift")
			}
		}
	}
	if *updateIssue110Catalog {
		if !found {
			entries = append(entries, entry)
		}
		data, err := catalog.EncodeEntries(entries)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0644); err != nil {
			t.Fatal(err)
		}
	} else if !found {
		t.Fatal("missing editor catalog contract")
	}
}

const issue110FixtureHash = "sha256:5d5ea83c0185f33a192a423840e5495aeddb7462614fb82e9641d9ca48c238a5"

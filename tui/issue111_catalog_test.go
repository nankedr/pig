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

var updateIssue111Catalog = flag.Bool("update-issue111-catalog", false, "regenerate terminal keys evidence")

const issue111CatalogID = "contract:tui/terminal-keys"

func issue111Partial() *catalog.Partial {
	return &catalog.Partial{Supported: []string{
		"Issue #111: public key parsing/matching/printable decoding, modifiers and press/repeat/release, default/custom/unbound keybindings and user conflict queries",
		"StdinBuffer preserves fragmented UTF-8, Escape timeout, bracketed paste and WezTerm deduplication; public SDK and real CLI PTY replay pinned Pi fixtures",
		"ProcessTerminal negotiates Kitty flags 7 and modifyOtherKeys fallback, consumes responses, drains and restarts without late callbacks; CGO-free macOS native Shift normalization",
		"CLI keybindings.json migration/loading, editor dispatch and clear/exit/interrupt/new-session/suspend actions; SIGTSTP/SIGCONT restores protocol and draft",
	}, Unsupported: []string{
		"High-bit single-byte Meta ambiguity is resolved in favor of UTF-8 byte fragments; ProcessTerminal extends incomplete non-Escape sequence timeout to 150ms for Go polling and paste safety",
		"Windows native input/modifiers and six-platform runtime acceptance remain M13; physical held-Shift GUI verification and malformed/rare escape combinations are not claimed",
		"Remaining application UI actions, autocomplete, extension runtime and image capabilities remain partial or explicit Stubs",
	}}
}
func issue111Promote(entry *catalog.Entry) bool {
	target := strings.TrimPrefix(entry.Mapping.Target, issue31GoPackage+".")
	supported := false
	for _, name := range []string{"SetKittyProtocolActive", "IsKittyProtocolActive", "IsKeyRelease", "IsKeyRepeat", "MatchesKey", "ParseKey", "DecodeKittyPrintable", "DecodePrintableKey", "KeybindingsManager", "SetKeybindings", "GetKeybindings", "StdinBuffer", "ParseKeyboardProtocolNegotiationSequence", "IsAppleTerminalSession", "NormalizeNativeShiftEnterInput", "NormalizeAppleTerminalInput", "IsNativeModifierPressed", "ProcessTerminal"} {
		supported = supported || target == name || strings.HasPrefix(target, name+".")
	}
	if !supported || strings.HasSuffix(target, ".SetTitle") || strings.HasSuffix(target, ".SetProgress") {
		return false
	}
	entry.Status = catalog.StatusPartial
	entry.Partial = issue111Partial()
	entry.Evidence = []catalog.Evidence{{Kind: "go-test", Ref: "tui/issue111_keys_test.go", Baseline: issue31BaselineCommit, CaseID: entry.ID, InputHash: issue111FixtureHash, ExecutionMethod: "go test ./tui -run '111' -count=1", Expected: "pinned Pi key semantics and public terminal lifecycle", Actual: "PASS; fixture replay and real PTY lifecycle with explicit partial branches", Platform: "darwin (PTY/native), any (pure SDK)", CatalogID: entry.ID}}
	entry.Notes = "Issue #111 behavior and evidence: contract:tui/terminal-keys"
	return true
}
func TestIssue111TerminalKeysCatalog(t *testing.T) {
	root := issue31RepoRoot(t)
	entry := catalog.Entry{SchemaVersion: catalog.SchemaVersion, ID: issue111CatalogID, Upstream: catalog.Upstream{Module: "tui", Repository: "https://github.com/badlogic/pi-mono", Commit: issue31BaselineCommit, Reference: "packages/tui/src/keys.ts"}, Mapping: catalog.Mapping{Module: "tui", Target: issue31GoPackage + ".KeybindingsManager", Kind: "contract"}, Status: catalog.StatusPartial, Milestone: "M6", Classification: "public-api", Partial: issue111Partial(), Notes: "Key parser, per-instance editor bindings, byte-stream buffering and terminal lifecycle; see docs/learning/m6-terminal-keys.md for scope and source preparation."}
	for _, item := range []struct{ kind, path, run string }{
		{"oracle", "parity/oracle/fixtures/keys.json", "node parity/oracle/keys.mjs <locked-pi-checkout> --check"},
		{"oracle", "parity/oracle/fixtures/keys-cli.json", "node parity/oracle/keys-cli.mjs <locked-pi-checkout> --check"},
		{"go-test", "tui/issue111_keys_test.go", "go test ./tui -run '111' -count=1"},
		{"go-test", "cmd/pig/issue110_editor_test.go", "go test ./cmd/pig -run '^TestPigTerminalKeys111$' -count=1"},
		{"go-test", "codingagent/issue109_interactive_test.go", "go test -race ./codingagent -run '^TestInteractiveSDKInterruptKey111$' -count=1"},
		{"go-test", "tui/testdata/issue31_surface_golden.txt", "go test ./tui -run '^TestIssue31LockedGoAPISnapshot$' -count=1"},
		{"go-test", "cmd/pig/issue111_suspend_test.go", "go test ./cmd/pig -run '^TestPigSuspend111$' -count=1"},
		{"go-test", "tui/issue111_terminal_test.go", "go test -race ./tui -run '^TestTerminalNegotiationRestart111$' -count=1"},
		{"go-test", "tui/issue111_buffer_test.go", "go test -race ./tui -run '^TestStdinBuffer.*111$' -count=1"},
		{"go-test", "codingagent/issue111_keybindings_test.go", "go test ./codingagent -run '^TestKeybindingsFile111$' -count=1"},
		{"manual", "examples/terminal-keys/main.go", "go run ./examples/terminal-keys"},
	} {
		data, err := os.ReadFile(filepath.Join(root, item.path))
		if err != nil {
			t.Fatal(err)
		}
		hash := fmt.Sprintf("sha256:%x", sha256.Sum256(data))
		if strings.HasSuffix(item.path, "/keys.json") && hash != issue111FixtureHash {
			t.Fatal("Keys fixture evidence hash drift")
		}
		entry.Evidence = append(entry.Evidence, catalog.Evidence{Kind: item.kind, Ref: item.path, Baseline: issue31BaselineCommit, CaseID: "issue111-" + item.path, InputHash: hash, ExecutionMethod: item.run, Expected: "terminal keys behavior through public SDK and actual Provider inputs through CLI PTY", Actual: "PASS; pinned Pi fixture replay and explicit partial scope", Platform: "darwin/linux (CLI), any (SDK)", CatalogID: entry.ID})
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
			if *updateIssue111Catalog {
				entries[i] = entry
			} else if !reflect.DeepEqual(row, entry) {
				t.Fatal("terminal keys catalog evidence drift")
			}
		}
	}
	if *updateIssue111Catalog {
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
		t.Fatal("missing terminal keys catalog contract")
	}
}

const issue111FixtureHash = "sha256:17c037eb78503c36136b78004b39f0a47ab22d3c47e1daa5f3a237a563f4e08d"

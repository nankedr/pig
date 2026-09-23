package codingagent_test

import (
	"crypto/sha256"
	"encoding/json"
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

var updateIssue121 = flag.Bool("update-issue121", false, "refresh interactive settings evidence and API snapshot")
var supportedSettings121 = []string{"autocompact", "skill-commands", "show-hardware-cursor", "editor-padding", "output-padding", "autocomplete-max-visible", "clear-on-shrink", "terminal-progress", "steering-mode", "follow-up-mode", "hide-thinking", "quiet-startup", "default-project-trust", "double-escape-action", "tree-filter-mode", "thinking", "tui-mode", "fullscreen-exit-output", "fullscreen-scrollbar", "theme"}
var unsupportedSettings121 = []string{"show-images", "image-width-cells", "auto-resize-images", "block-images", "transport", "http-idle-timeout", "mermaid-rendering", "cache-miss-notices", "collapse-changelog", "install-telemetry", "warnings"}

func TestInteractiveSettingsCatalog121(t *testing.T) {
	root := issue32RepoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "parity/interactive-settings-inventory.json"))
	if err != nil {
		t.Fatal(err)
	}
	var inventory struct{ Items []struct{ ID string } }
	if err = json.Unmarshal(data, &inventory); err != nil {
		t.Fatal(err)
	}
	coverage := map[string]bool{}
	for _, id := range append(append([]string{}, supportedSettings121...), unsupportedSettings121...) {
		coverage[id] = true
	}
	if len(coverage) != len(inventory.Items) {
		t.Fatal("settings inventory coverage changed")
	}
	for _, item := range inventory.Items {
		if !coverage[item.ID] {
			t.Fatal("unmapped Pi setting", item.ID)
		}
	}
	entry := catalog.Entry{SchemaVersion: catalog.SchemaVersion, ID: "contract:codingagent/interactive-settings", Upstream: catalog.Upstream{Module: "coding-agent", Repository: "https://github.com/badlogic/pi-mono", Commit: issue32BaselineCommit, Reference: "packages/coding-agent/src/modes/interactive/components/settings-selector.ts"}, Mapping: catalog.Mapping{Module: "codingagent", Target: issue32GoPackage + ".SettingsSelectorComponent", Kind: "contract"}, Status: catalog.StatusPartial, Milestone: "M6", Classification: "public-api", Partial: &catalog.Partial{Supported: supportedSettings121, Unsupported: unsupportedSettings121}, Deviation: &catalog.Deviation{ADR: "docs/adr/0039-interactive-settings.md", Reason: "Preserves Pig's thinking/theme entry order; strict save failure keeps previous values; trusted project overrides remain effective. Unsupported consumers are hidden."}, Notes: "Issue #121; each supported ID maps to AgentSession.UpdateInteractionSetting plus TextUI, except thinking/ThemeController use existing selectors. Complete Pi values inventory: parity/interactive-settings-inventory.json. Exclusions and ownership: ADR-0039; docs/learning/m6-interactive-settings.md. No M7/M11/M12/M13 or #99 expansion."}
	for _, file := range []string{"parity/interactive-settings-inventory.json", "parity/oracle/fixtures/settings-selector.json", "parity/oracle/fixtures/settings-cli.json", "parity/terminal/settings.py", "codingagent/issue121_selector_test.go", "codingagent/issue121_settings_test.go", "codingagent/issue121_interactive_test.go", "codingagent/issue121_runtime_test.go", "codingagent/issue121_terminal_effects_test.go", "cmd/pig/issue121_settings_test.go", "codingagent/testdata/issue121_surface_golden.txt", "examples/interactive-settings/main.go"} {
		data, err := os.ReadFile(filepath.Join(root, file))
		if err != nil {
			t.Fatal(err)
		}
		kind, run := "go-test", "go test ./codingagent ./cmd/pig -run 121 -count=1"
		if strings.HasSuffix(file, ".json") {
			kind = "oracle"
			run = "make m6-settings-oracle PIG_PI_ORACLE_CHECKOUT=<locked-pi-checkout>"
		}
		if strings.HasPrefix(file, "examples/") {
			kind = "manual"
			run = "go run ./examples/interactive-settings"
		}
		entry.Evidence = append(entry.Evidence, catalog.Evidence{Kind: kind, Ref: file, Baseline: issue32BaselineCommit, CaseID: "issue121-" + file, InputHash: fmt.Sprintf("sha256:%x", sha256.Sum256(data)), ExecutionMethod: run, Expected: "Fixed Pi menu values/callbacks, persisted settings, restart and real Session effects; trust, failures and effective key hints", Actual: "PASS for delivered settings; explicit partial branches in ADR-0039", Platform: "any (SDK), darwin/linux (PTY)", CatalogID: entry.ID})
	}
	path := filepath.Join(root, "parity/catalog.jsonl")
	entries, err := catalog.LoadCatalog(path)
	if err != nil {
		t.Fatal(err)
	}
	expected := map[string]catalog.Entry{entry.ID: entry}
	for _, symbol := range issue32Symbols(t) {
		if symbol.Name == "SettingsSelectorComponent" {
			e, err := issue32ConstructorEntry(symbol, "M6")
			if err != nil {
				t.Fatal(err)
			}
			expected[e.ID] = e
		}
	}
	for i, e := range entries {
		if want, ok := expected[e.ID]; ok {
			if *updateIssue121 {
				entries[i] = want
			} else if !reflect.DeepEqual(e, want) {
				t.Fatal("settings evidence drift", e.ID)
			}
			delete(expected, e.ID)
		}
	}
	if *updateIssue121 {
		for _, e := range expected {
			entries = append(entries, e)
		}
		data, err := catalog.EncodeEntries(entries)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(path, data, 0644); err != nil {
			t.Fatal(err)
		}
	} else if len(expected) > 0 {
		t.Fatal("missing settings catalog", expected)
	}
}
func TestInteractiveSettingsAPISnapshot121(t *testing.T) {
	var b strings.Builder
	for _, fn := range []any{codingagent.NewSettingsSelectorComponent, (*codingagent.SettingsSelectorComponent).GetSettingsList, (*codingagent.SettingsSelectorComponent).SetKeybindings, (*codingagent.SettingsSelectorComponent).HandleInput, (*codingagent.AgentSession).GetInteractionSettings, (*codingagent.AgentSession).UpdateInteractionSetting, (*tui.TextUI).ApplyInteractionOptions, (*tui.TextUI).HideThinking, tui.SelectionKeyHint, (*tui.SettingsList).UpdateDescription, (*codingagent.Transcript).SetOutputPad} {
		fmt.Fprintln(&b, reflect.TypeOf(fn))
	}
	typ := reflect.TypeOf(tui.TextUIInteractionOptions{})
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		fmt.Fprintln(&b, f.Name, f.Type)
	}
	path := "testdata/issue121_surface_golden.txt"
	if *updateIssue121 {
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
		t.Fatal("settings API drift")
	}
}

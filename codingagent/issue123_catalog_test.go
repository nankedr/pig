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

var updateIssue123 = flag.Bool("update-issue123", false, "refresh external editor evidence and API snapshot")

func partial123() *catalog.Partial {
	return &catalog.Partial{Supported: []string{"Configured/VISUAL/EDITOR/default selection; literal space splitting, expanded Unicode drafts and one final LF removal", "Real CLI and public SDK terminal handoff, regular/fullscreen recovery, silent background generation, cancellation, process-group and temporary-directory cleanup", "Visible errors preserve drafts; existing AgentSession/v3 Session receives the saved text without additional resource reads"}, Unsupported: []string{"Mid-write storage exhaustion, terminal device failures, detached process groups and exact visual layout", "Extension editor M7, package ecosystem #99, full authentication M11, images M12 and six-platform acceptance M13"}}
}
func TestExternalEditorCatalog123(t *testing.T) {
	root := issue32RepoRoot(t)
	entry := catalog.Entry{SchemaVersion: catalog.SchemaVersion, ID: "contract:codingagent/external-editor", Upstream: catalog.Upstream{Module: "coding-agent", Repository: "https://github.com/badlogic/pi-mono", Commit: issue32BaselineCommit, Reference: "packages/coding-agent/src/modes/interactive/interactive-mode.ts"}, Mapping: catalog.Mapping{Module: "codingagent", Target: issue32GoPackage + ".InteractiveMode", Kind: "contract"}, Status: catalog.StatusPartial, Milestone: "M6", Classification: "public-api", Partial: partial123(), Deviation: &catalog.Deviation{ADR: "docs/adr/0041-external-editor-terminal-ownership.md", Reason: "Visible spawn/exit/file errors preserve the session; context cancellation terminates the editor process group."}, Notes: "Issue #123; docs/learning/m6-external-editor.md and docs/mappings/typescript-to-go/m6-external-editor.md. Reuses legacy AgentSession and v3 Session."}
	for _, item := range []struct{ kind, path, run string }{
		{"oracle", "parity/oracle/fixtures/external-editor-cli.json", "make m6-external-editor-oracle PIG_PI_ORACLE_CHECKOUT=<locked-pi-checkout>"},
		{"go-test", "parity/terminal/external-editor.py", "go test ./cmd/pig -run ExternalEditor -count=1"},
		{"go-test", "tui/issue123_external_test.go", "go test -race ./tui -run ExternalEditor -count=1"},
		{"go-test", "codingagent/issue123_external_test.go", "go test -race ./codingagent -run 'ExternalEditor.*123' -count=1"},
		{"go-test", "cmd/pig/issue123_external_editor_test.go", "PIG_TEST_RACE=1 go test -race ./cmd/pig -run ExternalEditor -count=1"},
		{"go-test", "codingagent/testdata/issue123_surface_golden.txt", "go test ./codingagent -run TestExternalEditorAPISnapshot123 -count=1"},
		{"manual", "examples/external-editor/main.go", "go run ./examples/external-editor"},
		{"manual", "docs/verification/issue123-external-editor.md", "python3 parity/terminal/external-editor.py /tmp/pig-external-editor --pig --vim --fullscreen --failures"},
	} {
		data, err := os.ReadFile(filepath.Join(root, item.path))
		if err != nil {
			t.Fatal(err)
		}
		entry.Evidence = append(entry.Evidence, catalog.Evidence{Kind: item.kind, Ref: item.path, Baseline: issue32BaselineCommit, CaseID: "issue123-" + item.path, InputHash: fmt.Sprintf("sha256:%x", sha256.Sum256(data)), ExecutionMethod: item.run, Expected: "External editor draft round-trip, errors, cancellation and terminal ownership through public SDK and CLI", Actual: "PASS; explicit partial scope", Platform: "darwin-arm64 (execution); any (API snapshot)", CatalogID: entry.ID})
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
			if *updateIssue123 {
				entries[i] = entry
			} else if !reflect.DeepEqual(e, entry) {
				t.Fatal("external editor evidence drift")
			}
		}
	}
	if *updateIssue123 {
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
		t.Fatal("missing external editor contract")
	}
}

func TestExternalEditorAPISnapshot123(t *testing.T) {
	var b strings.Builder
	for _, fn := range []any{(*codingagent.InteractiveMode).OpenExternalEditor, (*tui.TextUI).EditExternally, (*tui.ProcessTerminal).RunCommand, codingagent.SettingsManager.GetExternalEditorCommand} {
		fmt.Fprintln(&b, reflect.TypeOf(fn))
	}

	path := "testdata/issue123_surface_golden.txt"
	if *updateIssue123 {
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
		t.Fatal("external editor API drift")
	}
}

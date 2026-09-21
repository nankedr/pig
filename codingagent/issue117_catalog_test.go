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

var updateIssue117 = flag.Bool("update-issue117", false, "refresh model selections evidence and API snapshot")

func partial117() *catalog.Partial {
	return &catalog.Partial{Supported: []string{
		"Issue #117: pinned Pi model selector and scope fixtures cover provider/ID/name fuzzy search, selection reset, wrapping, cancellation, scope toggling, provider changes, filtered bulk edits, order and explicit save",
		"Real CLI PTY compares model/thinking requests, v3 entries, settings, scope cycling and reopening Session for continued generation; public SDK verifies busy rejection, missing credentials, unsupported models and failed persistence",
		"Input single-line editing and shared fuzzy ranking, TextUI modal focus with preserved editor draft; existing AgentSession configuration transactions and fallback rules are reused",
	}, Unsupported: []string{
		"M10 dynamic catalog refresh and M11 full authentication remain Stub; selector uses the existing local ModelRuntime snapshot and does not trigger network refresh",
		"Settings menu currently exposes thinking only; full settings UI, exact Pi pixel layout, all Unicode/IME editing branches, extension runtime M7, images M12, package ecosystem #99 and six-platform runtime acceptance M13 remain partial/deferred",
	}}
}
func TestModelSelectionCatalog117(t *testing.T) {
	root := issue32RepoRoot(t)
	entry := catalog.Entry{SchemaVersion: catalog.SchemaVersion, ID: "contract:codingagent/model-selection", Upstream: catalog.Upstream{Module: "coding-agent", Repository: "https://github.com/badlogic/pi-mono", Commit: issue32BaselineCommit, Reference: "packages/coding-agent/src/modes/interactive/interactive-mode.ts"}, Mapping: catalog.Mapping{Module: "codingagent", Target: issue32GoPackage + ".InteractiveMode", Kind: "contract"}, Status: catalog.StatusPartial, Milestone: "M6", Classification: "public-api", Partial: partial117(), Deviation: &catalog.Deviation{ADR: "docs/adr/0025-session-configuration.md", Reason: "Pig rejects busy configuration changes and saves model defaults only with the successful Session transaction. Model selection uses static snapshots; no new catalog/auth network branches."}, Notes: "Issue #117; docs/learning/m6-model-selection.md. Reuses legacy AgentSession and v3 Session, with no transport retry changes."}
	for _, item := range []struct{ kind, path, run string }{
		{"oracle", "parity/oracle/fixtures/model-selector.json", "node parity/oracle/model-selector.mjs <locked-pi-checkout> --check"},
		{"oracle", "parity/oracle/fixtures/model-scope.json", "node parity/oracle/model-scope.mjs <locked-pi-checkout> --check"},
		{"oracle", "parity/oracle/fixtures/model-selection-cli.json", "node parity/oracle/model-selection-cli.mjs <locked-pi-checkout> --check"},
		{"go-test", "codingagent/issue117_selector_test.go", "go test -race ./codingagent -run TestModelSelectorParity117 -count=1"},
		{"go-test", "codingagent/issue117_scope_test.go", "go test -race ./codingagent -run 'TestModelScope.*117' -count=1"},
		{"go-test", "codingagent/issue117_interactive_test.go", "go test -race ./codingagent -run 'TestInteractiveModelsBusyAndDraft117|TestModelSelectionFailurePreservesConfiguration117' -count=1"},
		{"go-test", "cmd/pig/issue117_models_test.go", "PIG_TEST_RACE=1 go test -race ./cmd/pig -run 117 -count=1"},
		{"go-test", "codingagent/issue87_runtime_test.go", "go test -race ./codingagent -run TestSessionConfiguration -count=1"},
		{"go-test", "codingagent/testdata/issue117_surface_golden.txt", "go test ./codingagent -run TestModelSelectionAPISnapshot117 -count=1"},
		{"manual", "examples/model-selection/main.go", "go run ./examples/model-selection"},
	} {
		data, err := os.ReadFile(filepath.Join(root, item.path))
		if err != nil {
			t.Fatal(err)
		}
		entry.Evidence = append(entry.Evidence, catalog.Evidence{Kind: item.kind, Ref: item.path, Baseline: issue32BaselineCommit, CaseID: "issue117-" + item.path, InputHash: fmt.Sprintf("sha256:%x", sha256.Sum256(data)), ExecutionMethod: item.run, Expected: "model selection, thinking, scope, failure atomicity and restored Provider requests", Actual: "PASS; explicit partial scope", Platform: "any (SDK), darwin/linux (PTY)", CatalogID: entry.ID})
	}
	path := filepath.Join(root, "parity/catalog.jsonl")
	entries, err := catalog.LoadCatalog(path)
	if err != nil {
		t.Fatal(err)
	}
	constructors := map[string]catalog.Entry{}
	for _, symbol := range issue32Symbols(t) {
		if symbol.Name == "ModelSelectorComponent" || symbol.Name == "ThinkingSelectorComponent" {
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
			if *updateIssue117 {
				entries[i] = expected
			} else if !reflect.DeepEqual(e, expected) {
				t.Fatal("model selector constructor mapping drift")
			}
			delete(constructors, e.ID)
		}
		if e.ID == entry.ID {
			found = true
			if *updateIssue117 {
				entries[i] = entry
			} else if !reflect.DeepEqual(e, entry) {
				t.Fatal("model selection evidence drift")
			}
		}
	}
	if len(constructors) > 0 {
		t.Fatal("missing model selector constructor mapping")
	}
	if *updateIssue117 {
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
		t.Fatal("missing model selection contract")
	}
}

func TestModelSelectionAPISnapshot117(t *testing.T) {
	var b strings.Builder
	for _, fn := range []any{codingagent.NewModelSelectorComponent, codingagent.NewThinkingSelectorComponent, codingagent.NewScopedModelsSelectorComponent} {
		fmt.Fprintln(&b, reflect.TypeOf(fn))
	}
	for _, value := range []any{(*codingagent.ModelSelectorComponent)(nil), (*codingagent.ThinkingSelectorComponent)(nil), (*codingagent.ScopedModelsSelectorComponent)(nil)} {
		typ := reflect.TypeOf(value)
		for i := 0; i < typ.NumMethod(); i++ {
			m := typ.Method(i)
			fmt.Fprintln(&b, m.Name, m.Type)
		}
	}
	for _, value := range []any{codingagent.ModelsConfig{}, codingagent.ModelsCallbacks{}} {
		fmt.Fprintln(&b, issue71TypeSnapshot(reflect.TypeOf(value).Name(), reflect.TypeOf(value)))
	}
	for _, item := range []struct {
		value  any
		method string
	}{{(*codingagent.AgentSession)(nil), "SetEnabledModels"}, {(*tui.TextUI)(nil), "SetDialog"}} {
		m, _ := reflect.TypeOf(item.value).MethodByName(item.method)
		fmt.Fprintln(&b, m.Type)
	}
	path := "testdata/issue117_surface_golden.txt"
	if *updateIssue117 {
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
		t.Fatal("model selections API drift")
	}
}

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
)

var updateIssue124 = flag.Bool("update-issue124", false, "refresh interactive maintenance evidence and API snapshot")

func partial124() *catalog.Partial {
	return &catalog.Partial{Supported: []string{"Interactive compact/reload progress, cancellation, results, resource refresh and continued requests", "Authoritative current context versus full history usage/cost; existing safe local HTML export with visible errors", "Real CLI/PTY and public SDK lifecycle, locked Pi fixture, API snapshot and offline Faux example"}, Unsupported: []string{"Pi compaction queue auto-drain, exact animations/layout, per-model cost/cache waste breakdown, JSONL export and hosted sharing", "Extensions M7, package ecosystem #99, authentication M11, images M12, six-platform M13 and arbitrary terminal/storage failures"}}
}
func TestMaintenanceCatalog124(t *testing.T) {
	root := issue32RepoRoot(t)
	entry := catalog.Entry{SchemaVersion: catalog.SchemaVersion, ID: "contract:codingagent/interactive-maintenance", Upstream: catalog.Upstream{Module: "coding-agent", Repository: "https://github.com/badlogic/pi-mono", Commit: issue32BaselineCommit, Reference: "packages/coding-agent/src/modes/interactive/interactive-mode.ts"}, Mapping: catalog.Mapping{Module: "codingagent", Target: issue32GoPackage + ".InteractiveMode", Kind: "contract"}, Status: catalog.StatusPartial, Milestone: "M6", Classification: "public-api", Partial: partial124(), Deviation: &catalog.Deviation{ADR: "docs/adr/0042-interactive-maintenance.md", Reason: "Maintenance input returns to editor; cancellation preserves existing SDK context semantics and waits for worker cleanup."}, Notes: "Issue #124; docs/learning/m6-interactive-maintenance.md and docs/mappings/typescript-to-go/m6-interactive-maintenance.md. Reuses legacy AgentSession and v3 Session."}
	for _, item := range []struct{ kind, path, run string }{
		{"oracle", "parity/oracle/fixtures/maintenance-cli.json", "make m6-maintenance-oracle PIG_PI_ORACLE_CHECKOUT=<locked-pi-checkout>"},
		{"go-test", "parity/terminal/maintenance.py", "go test ./cmd/pig -run Maintenance -count=1"},
		{"go-test", "codingagent/issue124_interactive_test.go", "go test -race ./codingagent -run 'Maintenance.*124' -count=1"},
		{"go-test", "cmd/pig/issue124_maintenance_test.go", "PIG_TEST_RACE=1 go test -race ./cmd/pig -run Maintenance -count=1"},
		{"go-test", "codingagent/testdata/issue124_surface_golden.txt", "go test ./codingagent -run TestMaintenanceAPISnapshot124 -count=1"},
		{"manual", "examples/interactive-maintenance/main.go", "go run ./examples/interactive-maintenance"},
		{"manual", "docs/verification/issue124-maintenance.md", "go test ./codingagent ./cmd/pig -run 'Maintenance.*124' -count=1"},
	} {
		data, err := os.ReadFile(filepath.Join(root, item.path))
		if err != nil {
			t.Fatal(err)
		}
		entry.Evidence = append(entry.Evidence, catalog.Evidence{Kind: item.kind, Ref: item.path, Baseline: issue32BaselineCommit, CaseID: "issue124-" + item.path, InputHash: fmt.Sprintf("sha256:%x", sha256.Sum256(data)), ExecutionMethod: item.run, Expected: "Maintenance commands, resources, authoritative statistics, persisted compaction, local HTML and continued generation", Actual: "PASS; explicit partial scope", Platform: "darwin-arm64 (execution); any (API snapshot)", CatalogID: entry.ID})
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
			if *updateIssue124 {
				entries[i] = entry
			} else if !reflect.DeepEqual(e, entry) {
				t.Fatal("interactive maintenance evidence drift")
			}
		}
	}
	if *updateIssue124 {
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
		t.Fatal("missing interactive maintenance contract")
	}
}

func TestMaintenanceAPISnapshot124(t *testing.T) {
	var b strings.Builder
	for _, fn := range []any{codingagent.NewInteractiveMode, (*codingagent.InteractiveMode).Run, (*codingagent.AgentSession).Compact, (*codingagent.AgentSession).Reload, (*codingagent.AgentSession).GetSessionStats, (*codingagent.AgentSession).ExportToHTML} {
		fmt.Fprintln(&b, reflect.TypeOf(fn))
	}
	fmt.Fprintln(&b, issue71TypeSnapshot("SessionStats", reflect.TypeOf(codingagent.SessionStats{})))

	path := "testdata/issue124_surface_golden.txt"
	if *updateIssue124 {
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
		t.Fatal("interactive maintenance API drift")
	}
}

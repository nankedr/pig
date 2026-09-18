package codingagent_test

import (
	"github.com/nankedr/pig/internal/catalog"
	"strings"
)

func issue111PromoteRuntimeEntry(entry *catalog.Entry) bool {
	if !strings.Contains(entry.ID, "/core/keybindings.ts#KeybindingsManager") {
		return false
	}
	entry.Status = catalog.StatusPartial
	entry.Partial = &catalog.Partial{Supported: []string{"Issue #111: TUI manager inheritance, default application key definitions, keybindings.json loading/migration, explicit unbinding, conflicts, effective config and reload; editor/submit/clear/exit/interrupt/new-session/suspend are wired through CLI and public SDK"}, Unsupported: []string{"Remaining application UI actions and six-platform behavior remain partial; detailed evidence and protocol scope are owned by contract:tui/terminal-keys"}}
	entry.Evidence = []catalog.Evidence{{Kind: "go-test", Ref: "codingagent/issue111_keybindings_test.go", Baseline: issue32BaselineCommit, CaseID: entry.ID, InputHash: issue111KeysFixtureHash, ExecutionMethod: "go test ./codingagent -run '111' -count=1", Expected: "public SDK keybinding configuration and delivered interactive actions", Actual: "PASS; config/reload, PTY interrupt and CLI fixture under contract:tui/terminal-keys", Platform: "darwin/linux (PTY), any (config)", CatalogID: entry.ID}}
	entry.Notes = "Issue #111 keybindings; see contract:tui/terminal-keys and docs/learning/m6-terminal-keys.md"
	return true
}

const issue111KeysFixtureHash = "sha256:17c037eb78503c36136b78004b39f0a47ab22d3c47e1daa5f3a237a563f4e08d"

package codingagent_test

import (
	"strings"
	"testing"

	"github.com/nankedr/pig/internal/catalog"
)

const issue109InteractiveCatalogID = "contract:codingagent/interactive-text"

func issue109InteractivePartial() *catalog.Partial {
	return &catalog.Partial{Supported: []string{
		"Issue #109: real TTY CLI and public Go InteractiveMode share legacy AgentSession, v3 Session persistence and existing local resource assembly",
		"tui.TextUI owns basic UTF-8 input, backspace, bracketed paste, transcript rendering; ProcessTerminal owns CGO-free raw mode, resize polling, cancellable input and restoration on darwin/linux",
		"Pinned Pi real-process PTY fixture proves gated streaming, two-turn history, persisted roles, Ctrl+D, double Ctrl+C, /quit, SIGTERM and SIGHUP exit 0 and restored terminal/cursor/paste state",
		"Public SDK Init/Run/Stop, EOF, context cancellation, initialization/output failures and visible Provider failures; offline replay without Pi, Python or Node",
		"Raw SIGINT preserves signal termination after cleanup; pinned Pi leaves terminal modes active on this path, so cleanup intentionally follows issue109 restoration requirement and is not claimed identical",
	}, Unsupported: []string{
		"Full Markdown/layout parity, scrollback, overlays, fullscreen, advanced key protocols and Escape/suspend behavior remain partial or explicit stubs; issue110 editor evidence is owned by contract:tui/multiline-editor",
		"Image input, extension runtime, package ecosystem, update notifications, terminal-loss/stalled-output emergency exits and six-platform runtime acceptance remain unverified/deferred to their scheduled milestones",
	}}
}
func issue109InteractiveCatalogEntry() catalog.Entry {
	return catalog.Entry{SchemaVersion: catalog.SchemaVersion, ID: issue109InteractiveCatalogID, Upstream: catalog.Upstream{Module: "coding-agent", Repository: "https://github.com/badlogic/pi-mono", Commit: issue32BaselineCommit, Reference: "packages/coding-agent/src/modes/interactive/interactive-mode.ts"}, Mapping: catalog.Mapping{Module: "codingagent", Target: issue32GoPackage + ".InteractiveMode", Kind: "contract"}, Status: catalog.StatusPartial, Milestone: "M6", Classification: "public-api", Partial: issue109InteractivePartial(), Notes: "Additive Go Terminal injection and tui.TextUI provide the minimal text slice; existing complete TUI/component surfaces remain separately scaffolded. SDK context cancellation returns context.Canceled; CLI SIGTERM/SIGHUP map to Pi exit 0. Runtime ownership transfers to InteractiveMode lifecycle."}
}
func issue109InteractiveEvidence(t *testing.T) []issue32ModuleEvidenceDescriptor {
	t.Helper()
	var out []issue32ModuleEvidenceDescriptor
	for _, e := range []struct{ kind, path, run string }{
		{"oracle", "parity/oracle/fixtures/interactive.json", "node parity/oracle/interactive.mjs <locked-pi-checkout> --check"},
		{"go-test", "cmd/pig/issue109_interactive_test.go", "go test ./cmd/pig -run '^TestPigInteractive' -count=1"},
		{"go-test", "codingagent/issue109_interactive_test.go", "go test -race ./codingagent -run '^TestInteractiveSDK' -count=1"},
		{"go-test", "codingagent/issue109_surface_test.go", "go test ./codingagent -run '^TestIssue109InteractiveAPISnapshot$' -count=1"},
		{"manual", "examples/interactive-text/main.go", "go run ./examples/interactive-text"},
	} {
		out = append(out, issue32ModuleEvidenceDescriptor{InputPath: e.path, Evidence: catalog.Evidence{Kind: e.kind, Ref: e.path, Baseline: issue32BaselineCommit, CaseID: "issue109-" + e.path, ExecutionMethod: e.run, Expected: "two-turn streaming Interactive conversation and terminal lifecycle through public boundaries", Actual: "PASS; PTY CLI and SDK lifecycle replay with explicit partial branches", Platform: "darwin", CatalogID: issue109InteractiveCatalogID}})
	}
	return out
}
func issue109PromoteRuntimeEntry(entry *catalog.Entry) bool {
	if !strings.Contains(entry.ID, "/interactive-mode.ts#InteractiveMode") {
		return false
	}
	if strings.Contains(entry.ID, "#InteractiveModeOptions") {
		return false
	}
	implemented := entry.Mapping.Kind == "symbol"
	for _, name := range []string{".clearEditor", ".getUserInput", ".init", ".renderInitialMessages", ".run", ".showError", ".showWarning", ".stop"} {
		implemented = implemented || strings.HasSuffix(entry.ID, name)
	}
	if !implemented {
		return false
	}
	entry.Evidence = []catalog.Evidence{{Kind: "go-test", Ref: "codingagent/issue109_interactive_test.go", Baseline: issue32BaselineCommit, CaseID: entry.ID, InputHash: "sha256:56d62da2adf3b299c910d5e236b24cd4ac617f4eb9d6c00a2a0b28ed05a64f87", ExecutionMethod: "go test -race ./codingagent -run '^TestInteractiveSDK' -count=1", Expected: "public Interactive lifecycle and terminal restoration", Actual: "PASS; minimal text slice", Platform: "darwin", CatalogID: entry.ID}}
	entry.Status = catalog.StatusPartial
	entry.Partial = issue109InteractivePartial()
	entry.Notes = "Issue #109 opens the minimal text lifecycle; behavior and evidence owned by " + issue109InteractiveCatalogID
	return true
}

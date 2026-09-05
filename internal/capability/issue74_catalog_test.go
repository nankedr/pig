package capability_test

import (
	"github.com/nankedr/pig/internal/catalog"
	"testing"
)

func issue74ExtendProductEntry(t *testing.T, entry *catalog.Entry) {
	switch entry.ID {
	case "cmd-pig", "contract:cli/pig/args", "contract:codingagent/headless":
	default:
		return
	}
	const caseID = "issue74-global-settings-startup"
	entry.Evidence = issue56UpsertEvidence(entry.Evidence, catalog.Evidence{Kind: "go-test", Ref: "cmd/pig/issue74_process_test.go#TestPigGlobalSettingsStartupParity", Baseline: issue56BaselineCommit, CaseID: caseID, InputHash: issue33CanonicalInputHash(t, issue56RepoRoot(t), issue33EvidenceInputPaths(caseID)), ExecutionMethod: "go test ./cmd/pig -run '^TestPigGlobalSettingsStartupParity$' -count=1", Expected: "global settings drive Headless startup and restart with Pi-proven argument precedence", Actual: "PASS; real processes match the fixed Pi CLI fixture, retaining Session history and respecting explicit and environment paths", Platform: "darwin/linux", CatalogID: entry.ID})
	entry.Partial.Supported = append(entry.Partial.Supported, "Issue #74: saved global Provider/model/thinking and Session paths drive Headless startup; project settings remain unread")
	entry.Notes += " Issue #74 adds trusted global settings, locked saves and observable diagnostics; current built-in Provider support remains DeepSeek."
}

package capability_test

import (
	"strings"
	"testing"

	"github.com/nankedr/pig/internal/catalog"
)

func issue77ExtendProductEntry(t *testing.T, entry *catalog.Entry) {
	switch entry.ID {
	case "cmd-pig", "contract:cli/pig/args", "contract:codingagent/headless":
	default:
		return
	}
	const caseID = "issue77-model-runtime-product"
	entry.Evidence = issue56UpsertEvidence(entry.Evidence, catalog.Evidence{Kind: "go-test", Ref: "cmd/pig/issue77_process_test.go#TestPigModelRuntime77", Baseline: issue56BaselineCommit, CaseID: caseID, InputHash: issue33CanonicalInputHash(t, issue56RepoRoot(t), issue33EvidenceInputPaths(caseID)), ExecutionMethod: "go test ./cmd/pig -run '^TestPigModelRuntime77$' -count=1", Expected: "model listing, settings/explicit scopes, authenticated bare model IDs and scoped API keys execute through the shared runtime without implicit control-plane network", Actual: "PASS; real processes validate model/thinking requests, fixed Pi fallback warnings and missing-model diagnostics", Platform: "darwin/linux", CatalogID: entry.ID})
	entry.Partial.Supported = append(entry.Partial.Supported, "Issue #77: shared ModelRuntime, available-model listing, CLI/settings scope and provider-scoped explicit key; static Catalog Snapshot and Offline preserve explicit inference")
	for i, v := range entry.Partial.Unsupported {
		v = strings.ReplaceAll(v, "model-list, ", "")
		v = strings.ReplaceAll(v, "stored credential resolution, ", "")
		entry.Partial.Unsupported[i] = strings.ReplaceAll(v, "export and model listing", "export")
	}
	entry.Notes += " Issue #77 unifies model selection, canonical credentials, Session services and inference through ModelRuntime; full catalog refresh and OAuth/ambient auth remain deferred."
}

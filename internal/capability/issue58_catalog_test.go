package capability_test

import (
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/nankedr/pig/internal/catalog"
)

var updateIssue58Catalog = flag.Bool("update-issue58-catalog", false, "regenerate the issue #58 Parity Catalog rows")

// TestIssue58CatalogRecordsDeepSeekLiveSmoke records the protected DeepSeek
// live smoke as a smoke-kind evidence row on the existing Headless contract.
// The row keeps the partial status and only stores irreversible execution
// metadata plus the pass conclusion; no live output becomes an Oracle fixture.
func TestIssue58CatalogRecordsDeepSeekLiveSmoke(t *testing.T) {
	root := issue56RepoRoot(t)
	path := filepath.Join(root, "parity", "catalog.jsonl")
	entries, err := catalog.LoadCatalog(path)
	if err != nil {
		t.Fatal(err)
	}
	want := issue58PromoteCatalog(t, entries)
	if *updateIssue58Catalog {
		encoded, err := catalog.EncodeEntries(want)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, encoded, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}

	if !reflect.DeepEqual(entries, want) {
		t.Fatal("issue #58 evidence drifted; regenerate with -update-issue58-catalog")
	}
	var smoke *catalog.Evidence
	for _, entry := range entries {
		if entry.ID != "contract:codingagent/headless" {
			continue
		}
		for index := range entry.Evidence {
			if entry.Evidence[index].CaseID == "issue58-deepseek-live-read-continuation" {
				smoke = &entry.Evidence[index]
			}
		}
	}
	if smoke == nil || smoke.Kind != catalog.MatrixEvidenceSmoke || smoke.InputHash != "" {
		t.Fatalf("DeepSeek live smoke evidence = %+v, want smoke kind with no fixture input hash", smoke)
	}
}

func issue58PromoteCatalog(t *testing.T, source []catalog.Entry) []catalog.Entry {
	t.Helper()
	entries := append([]catalog.Entry(nil), source...)
	found := false
	for index := range entries {
		entry := &entries[index]
		if entry.ID != "contract:codingagent/headless" {
			continue
		}
		found = true
		entry.Evidence = issue56UpsertEvidence(entry.Evidence, catalog.Evidence{
			Kind:            catalog.MatrixEvidenceSmoke,
			Ref:             "codingagent/live_smoke_test.go#TestDeepSeekLiveHeadlessReadContinuation",
			Baseline:        issue56BaselineCommit,
			CaseID:          "issue58-deepseek-live-read-continuation",
			ExecutionMethod: "DEEPSEEK_API_KEY=<restricted> PIG_REQUIRE_LIVE=1 go test ./codingagent -run '^TestDeepSeekLiveHeadlessReadContinuation$' -count=1; ordinary PRs without the key skip and Freeze/release without the key fail",
			Expected:        "through the public Headless product path, a real DeepSeek text stream completes with low tokens, then a two-request read Tool continuation reads a sentinel file and echoes it in the final Assistant text",
			Actual:          "PASS; both phases completed against real DeepSeek without leaking the key, request/response bodies, or file contents; gating skips ordinary PRs and fails protected runs that lack the key",
			Platform:        "darwin/linux",
			CatalogID:       "contract:codingagent/headless",
		})
	}
	if !found {
		t.Fatal("missing catalog row contract:codingagent/headless")
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].ID < entries[j].ID })
	return entries
}

package capability_test

import (
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"testing"

	"github.com/nankedr/pig/internal/catalog"
)

var updateIssue56Catalog = flag.Bool("update-issue56-catalog", false, "regenerate the issue #56 Parity Catalog rows")

const issue56BaselineCommit = "936aff00918de1187f085f123c2812d8f2d67745"

func TestIssue56CatalogRecordsHeadlessTextProductPath(t *testing.T) {
	root := issue56RepoRoot(t)
	path := filepath.Join(root, "parity", "catalog.jsonl")
	entries, err := catalog.LoadCatalog(path)
	if err != nil {
		t.Fatal(err)
	}
	want := issue56PromoteCatalog(t, entries)
	if *updateIssue56Catalog {
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
		t.Fatal("issue #56 evidence drifted; regenerate with -update-issue56-catalog")
	}
	byID := make(map[string]catalog.Entry, len(entries))
	for _, entry := range entries {
		byID[entry.ID] = entry
	}
	headless, ok := byID["contract:codingagent/headless"]
	if !ok || (headless.Status != catalog.StatusPartial && headless.Status != catalog.StatusImplemented) || len(headless.Evidence) < 2 {
		t.Fatalf("Headless contract = %+v, want active status with SDK and process evidence", headless)
	}
	for _, id := range []string{"cmd-pig", "contract:cli/pig/args", "contract:cli/pig/exit-codes"} {
		entry := byID[id]
		if entry.Status != catalog.StatusPartial || entry.Partial == nil {
			t.Errorf("%s = %+v, want partial with exact supported and unsupported branches", id, entry)
		}
	}
}

func issue56PromoteCatalog(t *testing.T, source []catalog.Entry) []catalog.Entry {
	t.Helper()
	entries := append([]catalog.Entry(nil), source...)
	found := map[string]bool{}
	for index := range entries {
		entry := &entries[index]
		for evidenceIndex := range entry.Evidence {
			evidence := &entry.Evidence[evidenceIndex]
			if paths := issue33EvidenceInputPaths(evidence.CaseID); len(paths) != 0 {
				evidence.InputHash = issue33CanonicalInputHash(t, issue56RepoRoot(t), paths)
			}
		}
		switch entry.ID {
		case "cmd-pig":
			found[entry.ID] = true
			entry.Evidence = issue56UpsertEvidence(entry.Evidence, issue56Evidence(t, entry.ID, "cmd/pig/headless_process_test.go", "cmd/pig/headless_process_test.go#TestPigProcessRunsHeadlessTextWithExplicitDeepSeekInputs", "issue56-pig-process-headless-text", "go test ./cmd/pig -run '^TestPigProcess' -count=1", "the real pig process runs text against a local DeepSeek endpoint with explicit or ambient credentials, stable failures, and interrupt propagation", "PASS; stdout contained only final Assistant text, stable failures used stderr and exit 1, and SIGINT exited 130", "darwin/linux"))
		case "contract:cli/pig/args":
			found[entry.ID] = true
		case "contract:cli/pig/exit-codes":
			found[entry.ID] = true
			entry.Evidence = issue56UpsertEvidence(entry.Evidence, issue56Evidence(t, entry.ID, "cmd/pig/headless_process_test.go", "cmd/pig/headless_process_test.go#TestPigProcessSIGINTCancelsHeadlessRun", "issue56-pig-headless-exit-status", "go test ./cmd/pig -run '^TestPigProcess' -count=1", "successful Headless text exits 0, argument/Provider/Capability failures exit 1, and SIGINT exits 130 after preserving the canceled outcome", "PASS; process tests observed exact stdout/stderr separation and exit codes 0, 1 and 130", "darwin/linux"))
		case "contract:codingagent/headless":
			found[entry.ID] = true
			entry.Evidence = issue56UpsertEvidence(entry.Evidence, issue56Evidence(t, entry.ID, "codingagent/headless_test.go", "codingagent/headless_test.go#TestHeadlessRunnerReturnsFinalAssistantText", "issue56-shared-headless-runner", "go test ./codingagent -run '^(TestHeadless|TestCreateHeadless)' -count=1", "the shared runner returns final text, preserves partial cancellation outcomes, uses an explicit in-memory boundary, and completes a Faux read continuation offline", "PASS; all shared runner and construction cases completed without persistent state or network access", "any"))
			entry.Evidence = issue56UpsertEvidence(entry.Evidence, issue56Evidence(t, entry.ID, "cmd/pig/headless_process_test.go", "cmd/pig/headless_process_test.go#TestPigProcessRunsHeadlessTextWithExplicitDeepSeekInputs", "issue56-headless-process-boundary", "go test ./cmd/pig -run '^TestPigProcess' -count=1", "the shared lifecycle works through a real process, local DeepSeek wire boundary, stable errors, and SIGINT", "PASS; explicit and ambient credentials, final text, failures and interruption matched the process contract", "darwin/linux"))
		}
	}

	for _, id := range []string{"cmd-pig", "contract:cli/pig/args", "contract:cli/pig/exit-codes", "contract:codingagent/headless"} {
		if !found[id] {
			t.Fatalf("missing catalog row %s", id)
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].ID < entries[j].ID })
	return entries
}

func issue56Evidence(t *testing.T, catalogID, inputPath, ref, caseID, execution, expected, actual, platform string) catalog.Evidence {
	t.Helper()
	return catalog.Evidence{
		Kind: "go-test", Ref: ref, Baseline: issue56BaselineCommit, CaseID: caseID,
		InputHash: issue33CanonicalInputHash(t, issue56RepoRoot(t), []string{inputPath}), ExecutionMethod: execution,
		Expected: expected, Actual: actual, Platform: platform, CatalogID: catalogID,
	}
}

func issue56UpsertEvidence(existing []catalog.Evidence, replacement catalog.Evidence) []catalog.Evidence {
	result := append([]catalog.Evidence(nil), existing...)
	for index := range result {
		if result[index].CaseID == replacement.CaseID {
			result[index] = replacement
			return result
		}
	}
	return append(result, replacement)
}

func issue56RepoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

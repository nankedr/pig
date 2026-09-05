package capability_test

import (
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/nankedr/pig/internal/catalog"
)

var updateIssue71Catalog = flag.Bool("update-issue71-catalog", false, "regenerate the issue #71 Parity Catalog rows")

func TestIssue71CatalogRecordsSessionPersistenceProductPath(t *testing.T) {
	root := issue56RepoRoot(t)
	path := filepath.Join(root, "parity", "catalog.jsonl")
	entries, err := catalog.LoadCatalog(path)
	if err != nil {
		t.Fatal(err)
	}
	want := issue71PromoteCatalog(t, entries)
	if *updateIssue71Catalog {
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
		t.Fatal("issue #71 catalog rows drifted; regenerate with -update-issue71-catalog")
	}
	for _, entry := range entries {
		if entry.ID != "cmd-pig" && entry.ID != "contract:cli/pig/args" && entry.ID != "contract:codingagent/headless" {
			continue
		}
		if !issue71HasEvidence(entry.Evidence) || !strings.Contains(entry.Notes, "#71") {
			t.Errorf("%s does not record issue #71", entry.ID)
		}
	}
}

func issue71PromoteCatalog(t *testing.T, source []catalog.Entry) []catalog.Entry {
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
			entry.Evidence = issue56UpsertEvidence(entry.Evidence, issue71Evidence(t, entry.ID, "cmd/pig/issue71_process_test.go#TestPigProcessesPersistAndReopenExplicitSessionPath", "issue71-pig-session-persistence", "real pig processes create, persist, and reopen one v3 Session while explicit memory and Pi isolation remain observable", "PASS; a second process received prior history without duplicated records, --no-session wrote no state, and adjacent Pi state remained untouched"))
			entry.Partial = &catalog.Partial{
				Supported: []string{
					"the public product entry and real pig process expose Pig-branded root and subcommand help plus version with exact stream and exit behavior",
					"Headless text and JSON use default Pig-owned v3 Session persistence, explicit-path reopen, custom Session IDs and directories, or explicit --no-session memory",
					"root, package and auth grammar select distinct product modes or Capability Stubs without migrating Pi state",
				},
				Unsupported: []string{
					"interactive and RPC execution remain milestone-specific Capability Stubs",
					"package, auth, export, model-list, resource and extension operations remain explicit Capability Stubs",
				},
			}
			entry.Notes = "Issue #33 establishes the public product entry and inert MainOptions with later Capability Stubs. Issues #56 and #57 add Headless text and JSONL. Issue #71 adds Pig-owned v3 create, append, and explicit reopen plus side-effect-free --no-session without migrating Pi state. Issue #73 adds continue, ID selection, naming and fork."
		case "contract:cli/pig/args":
			found[entry.ID] = true
			entry.Evidence = issue56UpsertEvidence(entry.Evidence, issue71Evidence(t, entry.ID, "cmd/pig/issue71_process_test.go#TestPigNoSessionDoesNotCreatePigState", "issue71-pig-cli-session-args", "session path, ID, directory, and explicit memory arguments select their documented Session boundary", "PASS; default and explicit storage persisted under Pig paths while --no-session created no Pig state"))
			entry.Partial = &catalog.Partial{
				Supported: []string{
					"Pig-branded command help and exact routing for every advertised flag, alias, mode, warning, and argument error",
					"Headless text and JSON consume explicit Provider, model, credential, prompt, Tool, and stdin inputs with --session, --session-id, --session-dir, or --no-session",
					"unknown long flags flow to the Extension Surface in argument order",
				},
				Unsupported: []string{
					"interactive, RPC, package management, stored credential resolution, export and model listing remain assigned to later milestones",
					"interactive --resume remains a Capability Stub; --session path or ID supports selection without TUI",
				},
			}
			entry.Notes = "Issue #33 owns the static command contract, deterministic mode parsing and Extension Surface routing. Issues #56 and #57 activate Headless text and JSON. Issue #71 activates Session path, ID, directory, and explicit memory selection while preserving later interactive and RPC boundaries. Issue #73 activates continue, ID selection, naming and fork."
		case "contract:codingagent/headless":
			found[entry.ID] = true
			entry.Evidence = issue71RemoveEvidence(entry.Evidence, "issue71-headless-session-persistence")
			entry.Evidence = issue56UpsertEvidence(entry.Evidence, issue71SDKOutcomeEvidence(t, entry.ID))
			entry.Partial = &catalog.Partial{
				Supported:   []string{"reusable Headless prompt lifecycle with injected memory or persisted Sessions, final outcome inspection, text and session-first JSONL presentation, Provider errors, cancellation, and cleanup"},
				Unsupported: []string{"image inputs remain deferred to M12"},
			}
			entry.Notes = "Issues #56 and #57 implement the reusable Headless lifecycle and text/JSON presentation. Issue #71 adds persisted create and reopen while retaining explicit memory injection; image inputs remain deferred to M12."
		}
		issue74ExtendProductEntry(t, entry)
	}
	for i := range entries {
		entry := &entries[i]
		if entry.ID == "cmd-pig" || entry.ID == "contract:cli/pig/args" {
			entry.Partial.Supported = append(entry.Partial.Supported, "Issue #73: --continue, --session exact or prefix ID, --name, and --fork preserve history and source isolation without TUI")
			entry.Evidence = issue56UpsertEvidence(entry.Evidence, catalog.Evidence{Kind: "go-test", Ref: "cmd/pig/issue73_process_test.go#TestPigContinueSelectAndForkSessions", Baseline: issue56BaselineCommit, CaseID: "issue73-cli-session-navigation", InputHash: issue33CanonicalInputHash(t, issue56RepoRoot(t), issue33EvidenceInputPaths("issue73-cli-session-navigation")), ExecutionMethod: "go test ./cmd/pig -run '^TestPigContinueSelectAndForkSessions$' -count=1", Expected: "continue, ID selection, naming and independent fork work through real CLI processes", Actual: "PASS; retained history, source bytes, name and parent metadata; invalid selection and duplicate target IDs failed", Platform: "darwin/linux", CatalogID: entry.ID})
		}
	}

	for _, id := range []string{"cmd-pig", "contract:cli/pig/args", "contract:codingagent/headless"} {
		if !found[id] {
			t.Fatalf("missing catalog row %s", id)
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].ID < entries[j].ID })
	return entries
}

func issue71RemoveEvidence(evidence []catalog.Evidence, caseID string) []catalog.Evidence {
	result := make([]catalog.Evidence, 0, len(evidence))
	for _, item := range evidence {
		if item.CaseID != caseID {
			result = append(result, item)
		}
	}
	return result
}

func issue71SDKOutcomeEvidence(t *testing.T, catalogID string) catalog.Evidence {
	t.Helper()
	caseID := "issue71-headless-session-outcomes"
	return catalog.Evidence{
		Kind: "go-test", Ref: "codingagent/issue71_session_persistence_test.go#TestPersistedAgentSessionKeepsProviderFailureAndReportsStorageFailure",
		Baseline: issue56BaselineCommit, CaseID: caseID,
		InputHash:       issue33CanonicalInputHash(t, issue56RepoRoot(t), issue33EvidenceInputPaths(caseID)),
		ExecutionMethod: "go test ./codingagent -run '^TestPersistedAgentSessionKeepsProviderFailureAndReportsStorageFailure$' -count=1",
		Expected:        "the reusable Headless lifecycle preserves Provider failure, cancellation, cleanup, and partial Stream outcomes while surfacing storage failures",
		Actual:          "PASS; terminal partial outcomes persisted, storage errors remained observable, and the completed in-memory Provider outcome was retained without claiming a stored file",
		Platform:        "any", CatalogID: catalogID,
	}
}

func issue71Evidence(t *testing.T, catalogID, ref, caseID, expected, actual string) catalog.Evidence {
	t.Helper()
	return catalog.Evidence{
		Kind: "go-test", Ref: ref, Baseline: issue56BaselineCommit, CaseID: caseID,
		InputHash:       issue33CanonicalInputHash(t, issue56RepoRoot(t), issue33EvidenceInputPaths(caseID)),
		ExecutionMethod: "go test ./cmd/pig -run '^TestPig(ProcessesPersistAndReopenExplicitSessionPath|NoSessionDoesNotCreatePigState|ExplicitPiSessionDoesNotMigrateAdjacentPiState)$' -count=1",
		Expected:        expected, Actual: actual, Platform: "darwin/linux", CatalogID: catalogID,
	}
}

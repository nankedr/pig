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

var updateIssue57Catalog = flag.Bool("update-issue57-catalog", false, "regenerate the issue #57 Parity Catalog rows")

func TestIssue57CatalogRecordsSessionFirstJSONProductPath(t *testing.T) {
	root := issue56RepoRoot(t)
	path := filepath.Join(root, "parity", "catalog.jsonl")
	entries, err := catalog.LoadCatalog(path)
	if err != nil {
		t.Fatal(err)
	}
	want := issue57PromoteCatalog(t, entries)
	if *updateIssue57Catalog {
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
		t.Fatal("issue #57 catalog rows drifted; regenerate with -update-issue57-catalog")
	}
	byID := make(map[string]catalog.Entry, len(entries))
	for _, entry := range entries {
		byID[entry.ID] = entry
	}
	for _, id := range []string{
		"cmd-pig",
		"contract:cli/pig/args",
		"contract:cli/pig/exit-codes",
		"contract:codingagent/headless",
		"module-codingagent",
		"symbol:codingagent/src/main.ts#main",
		"symbol:codingagent/src/modes/json-event.ts#JsonAgentSessionEvent",
		"symbol:codingagent/src/modes/print-mode.ts#runPrintMode",
	} {
		entry, ok := byID[id]
		if !ok {
			t.Errorf("missing catalog row %s", id)
			continue
		}
		if entry.Status != catalog.StatusPartial && entry.Status != catalog.StatusImplemented {
			t.Errorf("%s status = %q, want partial or implemented", id, entry.Status)
		}
		if !strings.Contains(entry.Notes, "#57") {
			t.Errorf("%s notes do not record issue #57: %q", id, entry.Notes)
		}
	}
}

func issue57PromoteCatalog(t *testing.T, source []catalog.Entry) []catalog.Entry {
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
			entry.Evidence = issue56UpsertEvidence(entry.Evidence, issue57Evidence(t, entry.ID, "cmd/pig/headless_process_test.go#TestPigProcessStreamsSessionFirstHeadlessJSON", "issue57-pig-session-first-json", "the real pig --mode json process writes a v3 Session header followed by parseable ordered AgentSessionEvent records for text, Tool, Provider-error, and cancellation flows", "PASS; the real process matched the normalized JSONL golden, preserved event ordering and projection, encoded Provider failure in-band with exit 0, and completed cancellation with exit 130"))
			entry.Partial = &catalog.Partial{
				Supported: []string{
					"the public product entry and real pig process expose Pig-branded root and subcommand help plus version with exact stream and exit behavior",
					"Headless text accepts explicit DeepSeek inputs and emits only final Assistant text; Headless JSON emits a v3 Session header followed by projected AgentSessionEvent JSONL",
					"root, package and auth grammar select distinct product modes or Capability Stubs without reading state; pinned nonfatal diagnostics are emitted before downstream results",
				},
				Unsupported: []string{
					"interactive and RPC execution remain milestone-specific Capability Stubs",
					"package, auth, export, model-list, persisted Session, resource and extension operations remain explicit Capability Stubs",
				},
			}
			entry.Notes = "Issue #33 establishes the public product entry through codingagent.Main and the pig command. Issues #56 and #57 add real in-memory Headless text and session-first JSONL, including exact Provider-error and SIGINT outcomes; later operations remain Capability Stubs and MainOptions remain inert until M7."
		case "contract:cli/pig/args":
			found[entry.ID] = true
			entry.Evidence = issue56UpsertEvidence(entry.Evidence, issue57Evidence(t, entry.ID, "cmd/pig/headless_process_test.go#TestPigProcessTreatsPipedJSONAsPromptNotRPCCommand", "issue57-pig-json-cli-contract", "--mode json selects one-way output and treats piped JSON as literal prompt input while --mode rpc remains unavailable", "PASS; piped JSON reached the Provider as user text and the RPC route retained its exact codingagent.mode.rpc Capability Stub"))
			entry.Partial = &catalog.Partial{
				Supported: []string{
					"Pig-branded command and subcommand help snapshots plus an exact routing matrix for every advertised flag and alias, effective mode, package/auth form, one-shot operation, Extension Surface form, recoverable warning, and distinct argument-error boundary",
					"Headless text and JSON consume explicit Provider, exact model, API key, system prompt, read Tool selection, prompt arguments, and non-terminal stdin without activating persisted state or interpreting RPC commands",
					"known argument errors are distinguished from nonfatal warnings and mode or command Capability Stubs; root unknown long flags flow to the Extension Surface in argument order",
				},
				Unsupported: []string{
					"interactive, RPC, package management, stored credential resolution, export and model listing remain assigned to later milestones",
					"the static parser accepts later-milestone flags, but Headless modes report precise Capability Stubs instead of activating those subsystems",
				},
			}
			entry.Notes = "Issue #33 owns the static command contract, deterministic parsing and routing, including the Extension Surface. Issues #56 and #57 exercise the text and one-way JSON Headless modes while preserving later state, resource, package, extension, interactive, and RPC boundaries."
		case "contract:cli/pig/exit-codes":
			found[entry.ID] = true
			entry.Evidence = issue56UpsertEvidence(entry.Evidence, issue57Evidence(t, entry.ID, "cmd/pig/headless_process_test.go#TestPigProcessStreamsHeadlessJSONProviderError", "issue57-pig-json-exit-status", "successful JSON and JSON-encoded Provider terminal errors exit 0, SIGINT exits 130 after complete parseable terminal events, and argument or unavailable-capability failures exit 1", "PASS; Provider failure ended with agent_settled and exit 0, cancellation ended with aborted plus agent_settled and exit 130, and RPC remained exit 1"))
			entry.Partial = &catalog.Partial{
				Supported: []string{
					"root and all advertised subcommand help plus version return exit 0 with stdout only",
					"successful Headless text and JSON return 0; JSON-encoded Provider failures return 0; text Provider, argument and unavailable-capability failures return 1; SIGINT returns 130",
					"recoverable diagnostics are written before the final unavailable-operation error",
				},
				Unsupported: []string{"auth check ready/not-ready/error exit distinctions remain unavailable with the M3 auth runtime"},
			}
			entry.Notes = "Issue #33 freezes static and unavailable-operation exits. Issue #56 adds Headless text outcomes; issue #57 adds JSON in-band Provider errors, successful exit 0, and parseable cancellation before exit 130."
		case "contract:codingagent/headless":
			found[entry.ID] = true
			entry.Evidence = issue56UpsertEvidence(entry.Evidence, issue57Evidence(t, entry.ID, "cmd/pig/headless_process_test.go#TestPigProcessStreamsSessionFirstHeadlessJSON", "issue57-headless-json-boundary", "the shared Headless lifecycle presents a one-way session-first JSONL stream with formal event projection and complete terminal events", "PASS; text, read Tool, Provider-error, piped-prompt, and SIGINT process cases produced the expected JSONL records and exit states"))
			entry.Partial = &catalog.Partial{
				Supported:   []string{"reusable in-memory Headless prompt lifecycle, final outcome inspection, text presentation, session-first JSONL event presentation, Provider errors, and cancellation"},
				Unsupported: []string{"image inputs remain deferred to M12"},
			}
			entry.Notes = "Issue #56 implements the reusable in-memory Headless lifecycle and text presenter. Issue #57 adds session-first JSONL presentation over the same lifecycle; image inputs remain deferred to M12."
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

func issue57Evidence(t *testing.T, catalogID, ref, caseID, expected, actual string) catalog.Evidence {
	t.Helper()
	return catalog.Evidence{
		Kind: "go-test", Ref: ref, Baseline: issue56BaselineCommit, CaseID: caseID,
		InputHash:       issue33CanonicalInputHash(t, issue56RepoRoot(t), issue33EvidenceInputPaths(caseID)),
		ExecutionMethod: "go test ./cmd/pig -run '^TestPigProcess' -count=1",
		Expected:        expected,
		Actual:          actual,
		Platform:        "darwin/linux",
		CatalogID:       catalogID,
	}
}

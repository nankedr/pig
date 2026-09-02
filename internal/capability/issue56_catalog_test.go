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
		t.Fatal("issue #56 catalog rows drifted; regenerate with -update-issue56-catalog")
	}
	byID := make(map[string]catalog.Entry, len(entries))
	for _, entry := range entries {
		byID[entry.ID] = entry
	}
	headless, ok := byID["contract:codingagent/headless"]
	if !ok || headless.Status != catalog.StatusPartial || headless.Partial == nil || len(headless.Evidence) != 2 {
		t.Fatalf("Headless contract = %+v, want partial with SDK and process evidence", headless)
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
			entry.Partial = &catalog.Partial{
				Supported: []string{
					"the public product entry and real pig process expose Pig-branded root and subcommand help plus version with exact stream and exit behavior",
					"Headless text accepts an explicit DeepSeek Provider and model plus --api-key or DEEPSEEK_API_KEY, assembles an in-memory Session, and emits only final Assistant text",
					"root, package and auth grammar select distinct product modes or Capability Stubs without reading state; pinned nonfatal diagnostics are emitted before downstream results",
				},
				Unsupported: []string{
					"interactive, JSON and RPC execution remain milestone-specific Capability Stubs",
					"package, auth, export, model-list, persisted Session, resource and extension operations remain explicit Capability Stubs",
				},
			}
			entry.Notes = "Issue #33 establishes the public product entry through codingagent.Main and the pig command. Issue #56 adds the real in-memory Headless text path with explicit Provider/model credentials, final-text stdout, stable failure stderr, and interrupt exit 130; later operations remain Capability Stubs and MainOptions remain inert until M7."
		case "contract:cli/pig/args":
			found[entry.ID] = true
			entry.Partial = &catalog.Partial{
				Supported: []string{
					"Pig-branded command and subcommand help snapshots plus an exact routing matrix for every advertised flag and alias, effective mode, package/auth form, one-shot operation, Extension Surface form, recoverable warning, and distinct argument-error boundary",
					"Headless text consumes explicit Provider, exact model, API key, system prompt, read Tool selection, prompt arguments, and non-terminal stdin without activating persisted state",
					"known argument errors are distinguished from nonfatal warnings and mode or command Capability Stubs; root unknown long flags flow to the Extension Surface in argument order",
				},
				Unsupported: []string{
					"interactive, JSON, RPC, package management, stored credential resolution, export and model listing remain assigned to later milestones",
					"the static parser accepts later-milestone flags, but Headless text reports precise Capability Stubs instead of activating those subsystems",
				},
			}
			entry.Notes = "Issue #33 owns the static command contract, deterministic parsing and routing, including the Extension Surface. Issue #56 documents and exercises the explicit Headless text subset while preserving later mode, state, resource, package and extension boundaries."
		case "contract:cli/pig/exit-codes":
			found[entry.ID] = true
			entry.Evidence = issue56UpsertEvidence(entry.Evidence, issue56Evidence(t, entry.ID, "cmd/pig/headless_process_test.go", "cmd/pig/headless_process_test.go#TestPigProcessSIGINTCancelsHeadlessRun", "issue56-pig-headless-exit-status", "go test ./cmd/pig -run '^TestPigProcess' -count=1", "successful Headless text exits 0, argument/Provider/Capability failures exit 1, and SIGINT exits 130 after preserving the canceled outcome", "PASS; process tests observed exact stdout/stderr separation and exit codes 0, 1 and 130", "darwin/linux"))
			entry.Partial = &catalog.Partial{
				Supported: []string{
					"root and all advertised subcommand help plus version return exit 0 with stdout only",
					"successful Headless text returns 0; argument, Provider and unavailable-capability failures return 1; SIGINT returns 130",
					"recoverable diagnostics are written before the final unavailable-operation error",
				},
				Unsupported: []string{"auth check ready/not-ready/error exit distinctions remain unavailable with the M3 auth runtime"},
			}
			entry.Notes = "Issue #33 freezes exit 0 for static success and exit 1 for argument and unavailable-operation failures. Issue #56 adds Headless text success, Provider failure, and SIGINT cancellation contracts with exit 130 reserved for interruption."
		}
	}

	for _, id := range []string{"cmd-pig", "contract:cli/pig/args", "contract:cli/pig/exit-codes"} {
		if !found[id] {
			t.Fatalf("missing catalog row %s", id)
		}
	}
	headless := catalog.Entry{
		SchemaVersion: catalog.SchemaVersion,
		ID:            "contract:codingagent/headless",
		Upstream:      catalog.Upstream{Module: "coding-agent", Repository: "https://github.com/badlogic/pi-mono", Commit: issue56BaselineCommit, Reference: "packages/coding-agent/src/modes/print-mode.ts#runPrintMode"},
		Mapping:       catalog.Mapping{Module: "codingagent", Target: "github.com/nankedr/pig/codingagent.RunHeadless", Kind: "contract"},
		Status:        catalog.StatusPartial, Milestone: "M1", Classification: "public-api",
		Evidence: []catalog.Evidence{
			issue56Evidence(t, "contract:codingagent/headless", "codingagent/headless_test.go", "codingagent/headless_test.go#TestHeadlessRunnerReturnsFinalAssistantText", "issue56-shared-headless-runner", "go test ./codingagent -run '^(TestHeadless|TestCreateHeadless)' -count=1", "the shared runner returns final text, preserves partial cancellation outcomes, uses an explicit in-memory boundary, and completes a Faux read continuation offline", "PASS; all shared runner and construction cases completed without persistent state or network access", "any"),
			issue56Evidence(t, "contract:codingagent/headless", "cmd/pig/headless_process_test.go", "cmd/pig/headless_process_test.go#TestPigProcessRunsHeadlessTextWithExplicitDeepSeekInputs", "issue56-headless-process-boundary", "go test ./cmd/pig -run '^TestPigProcess' -count=1", "the shared lifecycle works through a real process, local DeepSeek wire boundary, stable errors, and SIGINT", "PASS; explicit and ambient credentials, final text, failures and interruption matched the process contract", "darwin/linux"),
		},
		Partial: &catalog.Partial{
			Supported:   []string{"reusable in-memory Headless prompt lifecycle, final outcome inspection, text presentation, Provider errors, and cancellation"},
			Unsupported: []string{"JSON event presentation remains an explicit Capability Stub owned by issue #57"},
		},
		Notes: "Issue #56 implements the reusable in-memory Headless lifecycle and text presenter. The broader Headless contract remains partial until issue #57 implements JSON presentation.",
	}
	replaced := false
	for index := range entries {
		if entries[index].ID == headless.ID {
			entries[index] = headless
			replaced = true
			break
		}
	}
	if !replaced {
		entries = append(entries, headless)
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

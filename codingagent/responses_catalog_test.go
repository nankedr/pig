package codingagent_test

import (
	"crypto/sha256"
	"encoding/json"
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

var updateResponsesRuntime = flag.Bool("update-responses-runtime", false, "refresh Responses Coding Agent evidence and API snapshot")

func TestResponsesRuntimeCatalog(t *testing.T) {
	root := issue32RepoRoot(t)
	id := "contract:codingagent/responses-runtime"
	entry := catalog.Entry{
		SchemaVersion: catalog.SchemaVersion, ID: id,
		Upstream: catalog.Upstream{Module: "coding-agent", Repository: "https://github.com/badlogic/pi-mono", Commit: issue32BaselineCommit, Reference: "packages/coding-agent/src/core/agent-session.ts"},
		Mapping:  catalog.Mapping{Module: "codingagent", Target: issue32GoPackage + ".AgentSession", Kind: "contract"},
		Status:   catalog.StatusImplemented, Milestone: "M14", Classification: "public-api",
		Deviation: &catalog.Deviation{ADR: "docs/adr/0044-responses-session-api.md", Reason: "Pig records optional production-v3 API metadata and a matching-provider defaultAPI preference; fixed Pi derives the protocol from its model catalog."},
		Notes:     "V1/#133 DeepSeek Responses SDK/headless/Interactive/JSONL RPC. Canonical credentials/trust/settings and full local replay; cross-process restore, fork, tree/branch summaries, compaction, queues/abort/stats and concurrent requests/replacement. Service fixture and protected 2026-10-08 live restore smoke are independent service evidence, not Pi Oracle. Images, other Providers, models.json runtime/remote catalog, advanced Responses, Harness v4, Remote Session Protocol, extensions and package manager retain their explicit Stub/deferred boundaries. No V1 freeze/release claim.",
	}
	for _, item := range []struct{ kind, path, run, expected string }{
		{"go-test", "codingagent/responses_runtime_test.go", "go test -race ./codingagent -run '^TestResponses' -count=3 -shuffle=on", "SDK settings/API restore and model selection, public tree navigation/branch summary/compaction continuation; unsupported providers excluded"},
		{"go-test", "cmd/pig/responses_process_test.go", "PIG_TEST_RACE=1 go test -race ./cmd/pig -run '^TestResponses(CLI|RPC)' -count=3 -shuffle=on", "same file edit through real CLI/RPC subprocesses; formal v3 replay, cross-process restore/fork/source isolation, compaction and usage/stats"},
		{"go-test", "cmd/pig/responses_terminal_test.go", "PIG_TEST_RACE=1 go test -race ./cmd/pig -run '^TestResponsesTerminal' -count=3 -shuffle=on", "same coding task through a real PTY, model selection, continuation and terminal restoration"},
		{"go-test", "cmd/pig/responses_control_test.go", "PIG_TEST_RACE=1 go test -race ./cmd/pig -run '^TestResponsesRPCQueues' -count=3 -shuffle=on", "steer/follow-up, concurrent RPC state queries, busy model rejection, abort partial and active-session replacement"},
		{"go-test", "codingagent/responses_live_test.go", "PIG_REQUIRE_RESPONSES_SESSION_LIVE=1 go test ./codingagent -run '^TestResponsesCodingAgentLiveRestore$' -count=1", "protected DeepSeek production-v3 read once, restore Responses API/history and continue without repeating tools"},
		{"fixture", "parity/services/responses-codingagent.json", "go test ./cmd/pig -run '^TestResponses' -count=1", "independent service reasoning/function_call/output/usage fixture; no server-side conversation/cache dependency"},
		{"oracle", "parity/oracle/fixtures/responses-tools.json", "go test ./internal/parity -run '^TestResponsesToolsFixedPiOracle$' -count=1", "existing pinned Pi protocol conversion and tool streaming invariants; does not prove service-specific runtime restore"},
		{"oracle", "parity/oracle/fixtures/session-persistence.json", "go test ./internal/parity -run Session -count=1", "existing fixed Pi production-v3 persistence/navigation invariants; optional API extension is a separate deviation"},
		{"manual", "examples/responses-session/main.go", "go run ./examples/responses-session", "offline production SDK tool/restore/compaction continuation"},
		{"go-test", "codingagent/testdata/responses_runtime_surface_golden.txt", "go test ./codingagent -run '^TestResponsesRuntimeAPISnapshot$' -count=1", "public SessionModel includes optional API with original provider/model identity"},
	} {
		data, err := os.ReadFile(filepath.Join(root, item.path))
		if err != nil {
			t.Fatal(err)
		}
		entry.Evidence = append(entry.Evidence, catalog.Evidence{Kind: item.kind, Ref: item.path, Baseline: issue32BaselineCommit, CaseID: "issue133/" + item.path, InputHash: fmt.Sprintf("sha256:%x", sha256.Sum256(data)), ExecutionMethod: item.run, Expected: item.expected, Actual: "PASS; deterministic offline public-boundary evidence within V1 scope", Platform: "any; PTY darwin/linux", CatalogID: id})
		if item.path == "codingagent/responses_live_test.go" {
			entry.Evidence[len(entry.Evidence)-1].Actual = "PASS 2026-10-08 api.deepseek.com deepseek-v4-pro reasoning low; production-v3 read tool once, restored API/history and continued; current service evidence, not Pi Oracle"
			entry.Evidence[len(entry.Evidence)-1].Platform = "darwin-arm64"
		}
	}
	path := filepath.Join(root, "parity/catalog.jsonl")
	entries, err := catalog.LoadCatalog(path)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for i, existing := range entries {
		if existing.ID != id {
			continue
		}
		found = true
		if *updateResponsesRuntime {
			entries[i] = entry
		} else if !reflect.DeepEqual(existing, entry) {
			t.Fatal("Responses runtime evidence drift")
		}
	}
	if !found {
		if !*updateResponsesRuntime {
			t.Fatal("missing Responses runtime Catalog entry")
		}
		entries = append(entries, entry)
	}
	if *updateResponsesRuntime {
		data, err := catalog.EncodeEntries(entries)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestResponsesRuntimeAPISnapshot(t *testing.T) {
	got := issue71TypeSnapshot("SessionModel", reflect.TypeOf(codingagent.SessionModel{})) + "\n"
	path := "testdata/responses_runtime_surface_golden.txt"
	if *updateResponsesRuntime {
		if err := os.WriteFile(path, []byte(got), 0644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Fatal("Responses runtime API snapshot drift")
	}
}

func TestResponsesRuntimeDeliveryScope(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(issue32RepoRoot(t), "parity/delivery-scope.json"))
	if err != nil {
		t.Fatal(err)
	}
	var scope struct {
		Scopes []struct {
			ID, Version string
			Issues      []int
			CatalogIDs  []string `json:"catalog_ids"`
		}
	}
	if err := json.Unmarshal(data, &scope); err != nil {
		t.Fatal(err)
	}
	for _, group := range scope.Scopes {
		if group.ID == "responses" && group.Version == "V1" && strings.Contains(strings.Join(group.CatalogIDs, ","), "contract:codingagent/responses-runtime") {
			for _, issue := range group.Issues {
				if issue == 133 {
					return
				}
			}
		}
	}
	t.Fatal("#133 Responses runtime must belong to V1 delivery scope")
}

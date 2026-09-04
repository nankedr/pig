package capability_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/nankedr/pig/internal/catalog"
)

var updateIssue61Catalog = flag.Bool("update-issue61-catalog", false, "regenerate the issue #61 Parity Catalog rows")

type issue61Fixture struct {
	BaselineCommit  string `json:"baseline_commit"`
	Deterministic   bool   `json:"deterministic"`
	InputHash       string `json:"input_hash"`
	ObservationHash string `json:"observation_hash"`
	Case            struct {
		ID        string `json:"id"`
		CatalogID string `json:"catalog_id"`
	} `json:"case"`
}

func TestIssue61CatalogRecordsUsageCostAndCacheParity(t *testing.T) {
	root := issue56RepoRoot(t)
	path := filepath.Join(root, "parity", "catalog.jsonl")
	entries, err := catalog.LoadCatalog(path)
	if err != nil {
		t.Fatal(err)
	}
	want := issue61PromoteCatalog(t, root, entries)
	if *updateIssue61Catalog {
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
		t.Fatal("issue #61 catalog rows drifted; regenerate with -update-issue61-catalog")
	}
}

func issue61PromoteCatalog(t *testing.T, root string, source []catalog.Entry) []catalog.Entry {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, "parity", "oracle", "fixtures", "usage-cost-cache.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture issue61Fixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	if !fixture.Deterministic || fixture.BaselineCommit != issue56BaselineCommit || fixture.Case.ID != "go-sdk/ai/usage-cost-cache" || fixture.Case.CatalogID != "contract:ai/faux-provider" {
		t.Fatalf("usage fixture provenance = %#v", fixture)
	}

	entries := append([]catalog.Entry(nil), source...)
	found := map[string]bool{}
	for index := range entries {
		entry := &entries[index]
		if issue61HasEvidence(entry.Evidence, "go-sdk/ai/deferred-lifecycle-go") {
			found[entry.ID] = true
			continue
		}
		if issue61TrackedMatrix(entry.ID) {
			found[entry.ID] = true
			assertion := entry.Matrix.EvidenceRequirements[0].Assertion
			fixtureExpected, goExpected := "the fixed Pi fixture "+assertion, "Pig matches the fixed Pi fixture: "+assertion
			if entry.ID == "matrix:ai/openai-completions/usage/usage-reasoning" || entry.ID == "matrix:ai/openai-completions/usage/usage-completion-tokens-details-reasoning-tokens" {
				fixtureExpected, goExpected = "the fixed Pi fixture covers explicit zero/value reasoning accounting", "Pig matches fixed Pi for explicit zero/value reasoning accounting"
			}
			entry.Evidence = issue61CompleteEvidence(entry.Evidence)
			entry.Evidence = issue61UpsertEvidence(entry.Evidence, issue61Evidence(catalog.MatrixEvidenceFixture, entry.ID, fixture, "parity/oracle/fixtures/usage-cost-cache.json", "go-sdk/ai/usage-cost-cache", "node --experimental-strip-types parity/oracle/usage-cost-cache.mjs PIG_PI_ORACLE_CHECKOUT --check", fixtureExpected, "PASS; the locked source fixture reproduced at "+fixture.ObservationHash))
			entry.Evidence = issue61UpsertEvidence(entry.Evidence, issue61Evidence(catalog.MatrixEvidenceGoTest, entry.ID, fixture, "internal/parity/usage_cost_cache_test.go#TestUsageCostCacheParity", "go-sdk/ai/usage-cost-cache-go", "go test ./internal/parity -run '^TestUsageCostCacheParity$' -count=1", goExpected, "PASS; public Go streams, Complete and CalculateCost matched the fixed Pi observation without normalization"))
			entry.Notes = "Issue #61 verifies usage, cost and cache accounting through a fixed-Pi fixture and the public Go SDK."
			if entry.ID == "matrix:ai/openai-completions/usage/usage-reasoning" || entry.ID == "matrix:ai/openai-completions/usage/usage-completion-tokens-details-reasoning-tokens" {
				entry.Evidence = issue61UpsertEvidence(entry.Evidence, issue61UsageStateEvidence(t, root, entry.ID))
			}
			if issue61VerifiedMatrix(entry.ID) {
				entry.Status = catalog.StatusVerified
				entry.Partial = nil
			}
			if entry.ID == "matrix:ai/openai-completions/usage/usage-reasoning" {
				entry.Notes += " Pi's mapper folds an absent reasoning counter to zero; Pig additionally preserves absent versus explicit zero as required by its public Optional contract."
			} else if entry.ID == "matrix:ai/openai-completions/usage/usage-completion-tokens-details-reasoning-tokens" {
				entry.Status = catalog.StatusPartial
				entry.Partial = &catalog.Partial{Supported: []string{"explicit zero and value match Pi; Pig preserves a missing raw reasoning counter as public Optional absence"}, Unsupported: []string{"exact absent-state parity: fixed Pi folds a missing reasoning counter to zero"}}
				entry.Notes += " The absent raw reasoning counter remains a documented Pi/Pig deviation, so this row stays partial."
			}
		}

		switch entry.ID {
		case "contract:ai/faux-provider":
			found[entry.ID] = true
			entry.Evidence = issue61UpsertEvidence(entry.Evidence, issue61ContractEvidence(entry.ID, fixture))
			entry.Partial = &catalog.Partial{
				Supported: []string{
					"all fourteen Faux public symbols map to compile-usable Go declarations, with deterministic queued AssistantMessage and synchronous factory responses",
					"text, thinking and ToolCall streams preserve done, Provider-error and cancellation outcomes, including partial output usage",
					"same-session prompt cache writes and hits are deterministic, cache-disabled and cross-session requests remain isolated, and Stream/Result/Complete expose equal usage and cost",
					"immutable per-stream factory state snapshots preserve the ADR-0006 concurrency deviation",
				},
				Unsupported: []string{"deferred submission, polling and deferred-handle cancellation remain structured ErrNotImplemented branches for M10", "asynchronous response factories, random identifiers and timing parity are not claimed"},
			}
			entry.Notes = "Issues #43, #60 and #61 implement the core, thinking, usage, cost and session-cache slices; deferred and non-core simulation behavior remains partial."
		case "contract:ai/faux-provider/core-stream":
			entry.Evidence = issue61DropEvidence(entry.Evidence, "go-sdk/ai/usage-cost-cache-go")
			entry.Notes = "Issue #43 implements the deterministic core-stream slice with no network access. Status remains implemented, not verified, because the supplied tracked-clean checkout lacks node_modules and packages/ai/dist; promote only after the script passes --require-dist against a prepared locked checkout."
		case "contract:ai/event-stream":
			found[entry.ID] = true
			entry.Evidence = issue61UpsertEvidence(entry.Evidence, issue61ContractEvidence(entry.ID, fixture))
			entry.Partial = &catalog.Partial{Supported: []string{"concurrent unbounded FIFO, repeatable final outcomes and waiter-local context cancellation", "immutable AssistantMessage snapshots plus terminal usage/cost consistency across stream events, Result and Complete"}, Unsupported: []string{"network retry and remaining protocol partial-JSON behavior retain their own adapter milestones", "deferred event branches remain outside the Issue #61 case"}}
			entry.Notes = "Issue #61 verifies usage and cost consistency across event, Result and Complete observations while retaining the ADR-0006 immutable-snapshot deviation."
		case "contract:ai/model":
			found[entry.ID] = true
			entry.Evidence = issue61UpsertEvidence(entry.Evidence, issue61ContractEvidence(entry.ID, fixture))
			entry.Partial = &catalog.Partial{Supported: []string{"compile-usable Model, Usage, cost, compat and image value shapes", "Usage preserves optional cacheWrite1h/reasoning states and CalculateCost executes base, highest-threshold tier and one-hour cache-write pricing"}, Unsupported: []string{"built-in model inventory and generated-at provenance remain tracked by contract:ai/providers-all", "remaining runtime model helpers and Provider adapters keep their own Catalog statuses"}}
			entry.Notes = "Issue #61 verifies public usage value states and model cost calculation; wider model catalog and adapter behavior remains separately tracked."
		case "contract:ai/models-runtime":
			found[entry.ID] = true
			entry.Evidence = issue61UpsertEvidence(entry.Evidence, issue61ContractEvidence(entry.ID, fixture))
			entry.Partial = &catalog.Partial{Supported: []string{"Models registry/query/auth/delegation seams plus in-memory generation-checked refresh publication", "CalculateCost selects base or the highest matching input tier, prices one-hour cache writes, and Models.Complete preserves terminal usage/cost", "Faux and DeepSeek provide runnable Stream/Complete paths"}, Unsupported: []string{"fixed Pi/Pig cases remain absent for registry order, auth/header assembly and refresh supersession/publication", "filesystem ambient auth, durable catalog persistence, live refresh and remaining protocol adapters retain their own milestones"}}
			entry.Notes = "Issues #43, #49 and #61 cover runnable Provider paths, Complete usage propagation and cost helpers; the wider models runtime remains partial."
		}
	}
	for _, id := range []string{"contract:ai/faux-provider", "contract:ai/event-stream", "contract:ai/model", "contract:ai/models-runtime"} {
		if !found[id] {
			t.Errorf("missing issue #61 Catalog row %s", id)
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].ID < entries[j].ID })
	return entries
}

func issue61VerifiedMatrix(id string) bool {
	return issue61TrackedMatrix(id) && id != "matrix:ai/openai-completions/usage/usage-completion-tokens-details-reasoning-tokens"
}

func issue61TrackedMatrix(id string) bool {
	return strings.HasPrefix(id, "matrix:ai/openai-completions/request/model-cost") || strings.HasPrefix(id, "matrix:ai/openai-completions/usage/") && id != "matrix:ai/openai-completions/usage/usage-assistant-output-only"
}

func issue61Evidence(kind, catalogID string, fixture issue61Fixture, ref, caseID, execution, expected, actual string) catalog.Evidence {
	return catalog.Evidence{Kind: kind, Ref: ref, Baseline: issue56BaselineCommit, CaseID: caseID, InputHash: fixture.InputHash, ExecutionMethod: execution, Expected: expected, Actual: actual, Platform: "any", CatalogID: catalogID}
}

func issue61ContractEvidence(catalogID string, fixture issue61Fixture) catalog.Evidence {
	return issue61Evidence(catalog.MatrixEvidenceGoTest, catalogID, fixture, "internal/parity/usage_cost_cache_test.go#TestUsageCostCacheParity", "go-sdk/ai/usage-cost-cache-go", "go test ./internal/parity -run '^TestUsageCostCacheParity$' -count=1", "Pig matches fixed Pi usage, cost, Faux cache and Stream/Complete observations through public Go APIs", "PASS; Pig matched the source fixture without normalization")
}

func issue61UsageStateEvidence(t *testing.T, root, catalogID string) catalog.Evidence {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, "ai", "issue61_openai_usage_test.go"))
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(data)
	return catalog.Evidence{Kind: catalog.MatrixEvidenceGoTest, Ref: "ai/issue61_openai_usage_test.go#TestOpenAIUsagePreservesReasoningAbsenceAndExplicitZero", Baseline: issue56BaselineCommit, CaseID: "issue61-openai-reasoning-states", InputHash: "sha256:" + hex.EncodeToString(digest[:]), ExecutionMethod: "go test ./ai -run '^TestOpenAIUsagePreservesReasoningAbsenceAndExplicitZero$' -count=1", Expected: "the OpenAI adapter preserves a missing raw reasoning counter separately from explicit zero at the public Usage boundary", Actual: "PASS; absent remained Optional absent and explicit zero remained Optional Some(0) through done and Result", Platform: "any", CatalogID: catalogID}
}

func issue61UpsertEvidence(existing []catalog.Evidence, replacement catalog.Evidence) []catalog.Evidence {
	result := append([]catalog.Evidence(nil), existing...)
	for index := range result {
		if result[index].Kind == replacement.Kind && result[index].CaseID == replacement.CaseID {
			result[index] = replacement
			return result
		}
	}
	return append(result, replacement)
}

func issue61CompleteEvidence(existing []catalog.Evidence) []catalog.Evidence {
	result := make([]catalog.Evidence, 0, len(existing))
	seen := map[string]bool{}
	for _, evidence := range existing {
		key := evidence.Kind + "\x00" + evidence.CaseID
		if !seen[key] && evidence.CaseID != "" && evidence.InputHash != "" && evidence.ExecutionMethod != "" && evidence.Expected != "" && evidence.Actual != "" && evidence.Platform != "" && evidence.CatalogID != "" {
			result = append(result, evidence)
			seen[key] = true
		}
	}
	return result
}

func issue61DropEvidence(existing []catalog.Evidence, caseID string) []catalog.Evidence {
	result := make([]catalog.Evidence, 0, len(existing))
	for _, evidence := range existing {
		if evidence.CaseID != caseID {
			result = append(result, evidence)
		}
	}
	return result
}

func issue61HasEvidence(existing []catalog.Evidence, caseID string) bool {
	for _, evidence := range existing {
		if evidence.CaseID == caseID {
			return true
		}
	}
	return false
}

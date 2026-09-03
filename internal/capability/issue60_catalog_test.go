package capability_test

import (
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

var updateIssue60Catalog = flag.Bool("update-issue60-catalog", false, "regenerate the issue #60 Parity Catalog rows")

func TestIssue60CatalogRecordsThinkingAndSignatureParity(t *testing.T) {
	root := issue56RepoRoot(t)
	path := filepath.Join(root, "parity", "catalog.jsonl")
	entries, err := catalog.LoadCatalog(path)
	if err != nil {
		t.Fatal(err)
	}
	want := issue60PromoteCatalog(t, root, entries)
	if *updateIssue60Catalog {
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
		t.Fatal("issue #60 catalog rows drifted; regenerate with -update-issue60-catalog")
	}
}

type issue60Fixture struct {
	ID             string   `json:"id"`
	CatalogIDs     []string `json:"catalog_ids"`
	BaselineCommit string   `json:"baseline_commit"`
	Deterministic  bool     `json:"deterministic"`
	InputSHA256    string   `json:"input_sha256"`
}

func issue60PromoteCatalog(t *testing.T, root string, source []catalog.Entry) []catalog.Entry {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, "parity", "oracle", "fixtures", "openai-completions-thinking.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture issue60Fixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.ID != "ai/openai-completions/m2-thinking-signatures" || fixture.BaselineCommit != issue56BaselineCommit || !fixture.Deterministic || len(fixture.CatalogIDs) != 60 || len(fixture.InputSHA256) != 64 {
		t.Fatalf("thinking fixture provenance = %#v", fixture)
	}

	fixtureIDs := make(map[string]bool, len(fixture.CatalogIDs))
	for _, id := range fixture.CatalogIDs {
		if fixtureIDs[id] {
			t.Fatalf("duplicate thinking fixture catalog ID %s", id)
		}
		fixtureIDs[id] = true
	}
	verified := issue60VerifiedCatalogIDs()
	entries := append([]catalog.Entry(nil), source...)
	found := make(map[string]bool, len(fixtureIDs))
	for index := range entries {
		entry := &entries[index]
		if fixtureIDs[entry.ID] {
			found[entry.ID] = true
			if entry.Matrix == nil || len(entry.Matrix.EvidenceRequirements) == 0 {
				t.Fatalf("thinking fixture row %s has no matrix evidence requirement", entry.ID)
			}
			assertion := entry.Matrix.EvidenceRequirements[0].Assertion
			entry.Evidence = issue60UpsertEvidence(entry.Evidence, issue60FixtureEvidence(entry.ID, fixture.InputSHA256, assertion))
			entry.Evidence = issue60UpsertEvidence(entry.Evidence, issue60GoEvidence(entry.ID, fixture.InputSHA256, assertion))
			entry.Notes = "Issue #60 runs this M2.1 thinking/signature branch through a fixed-Pi fixture and the public Go Chat Completions adapter."
			if verified[entry.ID] {
				entry.Status = catalog.StatusVerified
				entry.Partial = nil
			} else {
				entry.Status = catalog.StatusPartial
				entry.Partial = &catalog.Partial{
					Supported:   []string{assertion},
					Unsupported: []string{"the fixture does not enumerate every declared value state (" + strings.Join(entry.Matrix.ValueSemantics.States, ", ") + "); uncaptured states retain their existing milestone assignment"},
				}
			}
		}

		switch entry.ID {
		case "contract:ai/faux-provider":
			entry.Evidence = issue60UpsertEvidence(entry.Evidence, catalog.Evidence{Kind: "go-test", Ref: "ai/issue60_thinking_test.go#TestFauxStreamsThinkingAndPreservesItsReplayMetadata", Baseline: issue56BaselineCommit})
			entry.Partial = &catalog.Partial{
				Supported: []string{
					"all fourteen Faux public symbols map to compile-usable Go declarations, with deterministic queued AssistantMessage and synchronous factory responses",
					"text, thinking and ToolCall delta streams preserve done, Provider-error and cancellation outcomes plus replayable thinking metadata",
					"immutable per-stream factory state snapshots for concurrent response factories, with the upstream shared-state observation retained as an ADR-0006 deviation",
				},
				Unsupported: []string{
					"deferred submission, polling and deferred-handle cancellation remain structured ErrNotImplemented branches for M10",
					"asynchronous response factories and Pi usage, cost, cache, random-identifier and timing parity are not claimed",
				},
			}
			entry.Notes = "Issues #43 and #60 implement the M1 core and M2.1 thinking stream slices. The broad Faux contract remains partial for deferred handles and non-core simulation details."
		case "contract:ai/content":
			entry.Evidence = issue60UpsertEvidence(entry.Evidence, catalog.Evidence{Kind: "go-test", Ref: "ai/issue60_thinking_test.go#TestOpenAICompletionsThinkingAndSignatureParity", Baseline: issue56BaselineCommit})
			entry.Partial = &catalog.Partial{
				Supported: []string{
					"closed text, thinking, image and toolCall variants with strict discriminator codecs and role-specific content sets",
					"OpenAI Chat Completions thinking replay, cross-model text conversion, redaction, thinking signatures and Tool thought signatures",
				},
				Unsupported: []string{"remaining Provider-specific signature interpretation is assigned to later adapter slices", "image generation behavior is deferred to M12"},
			}
			entry.Notes = "Issue #25 establishes the immutable content values; issue #60 executes the M2.1 OpenAI Chat Completions thinking/signature conversion slice."
		case "contract:ai/model":
			entry.Status = catalog.StatusPartial
			entry.Evidence = issue60UpsertEvidence(entry.Evidence, catalog.Evidence{Kind: "go-test", Ref: "ai/issue60_surface_test.go#TestIssue60LockedGoAPISnapshot", Baseline: issue56BaselineCommit})
			entry.Partial = &catalog.Partial{
				Supported:   []string{"compile-usable Model, Usage, cost, compat and image value shapes", "reasoning capability and thinking-level mappings execute through the M2.1 OpenAI Chat Completions request path"},
				Unsupported: []string{"built-in model inventory and generated-at provenance remain tracked by contract:ai/providers-all", "remaining runtime model helpers and Provider adapters keep their own Catalog statuses"},
			}
			entry.Notes = "Issue #25 declares the model value contract; issue #60 freezes and executes its reasoning and thinking-level fields without claiming the wider model catalog."
		case "contract:ai/options":
			entry.Evidence = issue60UpsertEvidence(entry.Evidence, catalog.Evidence{Kind: "go-test", Ref: "ai/issue60_thinking_test.go#TestOpenAICompletionsThinkingAndSignatureParity", Baseline: issue56BaselineCommit})
			entry.Partial = &catalog.Partial{
				Supported:   []string{"shared request, stream, simple-stream and deferred option shapes", "header deletion and explicit-zero semantics", "reasoning effort, thinking budgets and M2.1 thinking-format controls execute through OpenAI Chat Completions"},
				Unsupported: []string{"Provider SDK clients remain injected opaque seams", "deferred tools and remaining advanced Provider compatibility options retain their later milestone assignments"},
			}
			entry.Notes = "Issue #25 preserves option value states; issue #60 executes the M2.1 reasoning and thinking-budget subset."
		}
	}
	for id := range fixtureIDs {
		if !found[id] {
			t.Errorf("thinking fixture references missing catalog row %s", id)
		}
	}
	for id := range verified {
		if !fixtureIDs[id] {
			t.Errorf("verified thinking row %s is absent from fixture", id)
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].ID < entries[j].ID })
	return entries
}

func issue60VerifiedCatalogIDs() map[string]bool {
	ids := []string{
		"matrix:ai/openai-completions/content/assistant-message-content-thinking",
		"matrix:ai/openai-completions/content/thinking-content-type",
		"matrix:ai/openai-completions/delta/delta-reasoning-details-attach-after-tool-call",
		"matrix:ai/openai-completions/delta/delta-reasoning-details-pending-before-tool-call",
		"matrix:ai/openai-completions/delta/delta-reasoning-details-pending-last-wins",
		"matrix:ai/openai-completions/event/assistant-message-event-thinking-delta",
		"matrix:ai/openai-completions/event/assistant-message-event-thinking-delta-partial",
		"matrix:ai/openai-completions/event/assistant-message-event-thinking-delta-type",
		"matrix:ai/openai-completions/event/assistant-message-event-thinking-end",
		"matrix:ai/openai-completions/event/assistant-message-event-thinking-end-partial",
		"matrix:ai/openai-completions/event/assistant-message-event-thinking-end-type",
		"matrix:ai/openai-completions/event/assistant-message-event-thinking-start",
		"matrix:ai/openai-completions/event/assistant-message-event-thinking-start-partial",
		"matrix:ai/openai-completions/event/assistant-message-event-thinking-start-type",
		"matrix:ai/openai-completions/message/conversion-assistant-reasoning-details-invalid",
		"matrix:ai/openai-completions/message/conversion-assistant-reasoning-details-valid",
	}
	result := make(map[string]bool, len(ids))
	for _, id := range ids {
		result[id] = true
	}
	return result
}

func issue60FixtureEvidence(catalogID, inputHash, assertion string) catalog.Evidence {
	return catalog.Evidence{
		Kind: "fixture", Ref: "parity/oracle/fixtures/openai-completions-thinking.json", Baseline: issue56BaselineCommit,
		CaseID: "m2-thinking-signatures", InputHash: "sha256:" + inputHash,
		ExecutionMethod: "node --experimental-strip-types parity/oracle/openai-completions-thinking.mjs PIG_PI_ORACLE_CHECKOUT --check",
		Expected:        "the fixed Pi fixture " + assertion,
		Actual:          "PASS; the committed fixture reproduced the request formats, history conversion, reasoning stream, encrypted Tool signatures, usage and content-order terminal events",
		Platform:        "any", CatalogID: catalogID,
	}
}

func issue60GoEvidence(catalogID, inputHash, assertion string) catalog.Evidence {
	return catalog.Evidence{
		Kind: "go-test", Ref: "ai/issue60_thinking_test.go#TestOpenAICompletionsThinkingAndSignatureParity", Baseline: issue56BaselineCommit,
		CaseID: "m2-thinking-signatures-go", InputHash: "sha256:" + inputHash,
		ExecutionMethod: "go test ./ai -run '^TestOpenAICompletionsThinkingAndSignatureParity$' -count=1",
		Expected:        "Pig matches the fixed Pi fixture: " + assertion,
		Actual:          "PASS; the public Go adapter matched the fixture request cases, conversions, projected events and terminal outcome",
		Platform:        "any", CatalogID: catalogID,
	}
}

func issue60UpsertEvidence(existing []catalog.Evidence, replacement catalog.Evidence) []catalog.Evidence {
	result := append([]catalog.Evidence(nil), existing...)
	for index := range result {
		if result[index].Kind == replacement.Kind && result[index].CaseID == replacement.CaseID {
			result[index] = replacement
			return result
		}
	}
	return append(result, replacement)
}

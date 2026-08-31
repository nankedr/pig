package catalog_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/nankedr/pig/internal/catalog"
)

// update regenerates the committed catalog-derived artifacts when set:
//   - go test ./internal/catalog -run GenerateCatalog -update
//     regenerates parity/catalog.manifest.json and the report from the
//     authoritative parity/catalog.jsonl.
//   - go test ./internal/catalog -run Golden -update
//     regenerates only parity/reports/catalog.md from the committed catalog.
//
// The catalog is the sole authored authority. The manifest and report are
// generated views, so this keeps those derived artifacts in sync without a
// second hand-maintained status table. Regeneration is pure-Go and offline.
var update = flag.Bool("update", false, "regenerate committed artifacts derived from the Parity Catalog")

const baselineCommit = "936aff00918de1187f085f123c2812d8f2d67745"

const catalogBaselineCommit = "53fa77ccd8a279eb87e92294ef3687b03ff80112"

// repoRoot locates the repository root relative to this test file, mirroring
// the runtime.Caller approach used by internal/capability tests.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

// base returns a fresh, fully valid set of catalog entries plus a matching
// manifest. Each rejection test mutates a fresh copy so cases stay independent.
func base() ([]catalog.Entry, catalog.Manifest) {
	entries := []catalog.Entry{
		{
			SchemaVersion:  catalog.SchemaVersion,
			ID:             "a-inventoried",
			Upstream:       catalog.Upstream{Module: "ai", Repository: "https://github.com/badlogic/pi-mono", Commit: baselineCommit, Reference: "packages/ai"},
			Mapping:        catalog.Mapping{Module: "ai", Target: "github.com/nankedr/pig/ai", Kind: "package"},
			Status:         catalog.StatusInventoried,
			Milestone:      "M1",
			Classification: "public-api",
		},
		{
			SchemaVersion:  catalog.SchemaVersion,
			ID:             "b-scaffolded",
			Upstream:       catalog.Upstream{Module: "agent", Repository: "https://github.com/badlogic/pi-mono", Commit: baselineCommit, Reference: "packages/agent"},
			Mapping:        catalog.Mapping{Module: "agent", Target: "github.com/nankedr/pig/agent", Kind: "package"},
			Status:         catalog.StatusScaffolded,
			Milestone:      "M1",
			Classification: "public-api",
		},
		{
			SchemaVersion:  catalog.SchemaVersion,
			ID:             "c-partial",
			Upstream:       catalog.Upstream{Module: "tui", Repository: "https://github.com/badlogic/pi-mono", Commit: baselineCommit, Reference: "packages/tui"},
			Mapping:        catalog.Mapping{Module: "tui", Target: "github.com/nankedr/pig/tui", Kind: "package"},
			Status:         catalog.StatusPartial,
			Milestone:      "M6",
			Classification: "public-api",
			Partial:        &catalog.Partial{Supported: []string{"layout"}, Unsupported: []string{"images"}},
		},
		{
			SchemaVersion:  catalog.SchemaVersion,
			ID:             "d-implemented",
			Upstream:       catalog.Upstream{Module: "protocol", Repository: "https://github.com/badlogic/pi-mono", Commit: baselineCommit, Reference: "packages/protocol"},
			Mapping:        catalog.Mapping{Module: "protocol", Target: "github.com/nankedr/pig/protocol", Kind: "package"},
			Status:         catalog.StatusImplemented,
			Milestone:      "M9",
			Classification: "public-api",
			Evidence:       []catalog.Evidence{{Kind: "go-test", Ref: "protocol/codec_test.go", Baseline: baselineCommit}},
		},
		{
			SchemaVersion:  catalog.SchemaVersion,
			ID:             "e-verified",
			Upstream:       catalog.Upstream{Module: "client", Repository: "https://github.com/badlogic/pi-mono", Commit: baselineCommit, Reference: "packages/client"},
			Mapping:        catalog.Mapping{Module: "client", Target: "github.com/nankedr/pig/client", Kind: "package"},
			Status:         catalog.StatusVerified,
			Milestone:      "M9",
			Classification: "public-api",
			Evidence: []catalog.Evidence{{
				Kind: catalog.MatrixEvidenceOracle, Ref: "parity/cases/client", Baseline: baselineCommit,
				CaseID: "client/example", InputHash: "sha256:0000000000000000000000000000000000000000000000000000000000000000",
				ExecutionMethod: "go test ./internal/catalog", Expected: "fixture matches", Actual: "fixture matched", Platform: "any", CatalogID: "e-verified",
			}},
		},
		{
			SchemaVersion:  catalog.SchemaVersion,
			ID:             "f-deferred",
			Upstream:       catalog.Upstream{Module: "server", Repository: "https://github.com/badlogic/pi-mono", Commit: baselineCommit, Reference: "packages/server"},
			Mapping:        catalog.Mapping{Module: "server", Target: "github.com/nankedr/pig/codingagent", Kind: "contract"},
			Status:         catalog.StatusDeferred,
			Milestone:      "M0",
			Classification: "dormant-test-support",
			Deferred:       &catalog.Deferred{ADR: "ADR-0003", Milestone: "none", Reason: "Pig Server not implemented in V1"},
		},
	}
	counts := map[string]int{}
	for _, e := range entries {
		counts[e.Status]++
	}
	manifest := catalog.Manifest{
		SchemaVersion:           catalog.SchemaVersion,
		CatalogVersion:          catalog.SchemaVersion,
		BaselineCommit:          baselineCommit,
		Catalog:                 "catalog.jsonl",
		Schema:                  "catalog.schema.json",
		InventoryManifest:       "inventory/manifest.json",
		SurfaceManifest:         "surface/manifest.json",
		CatalogSnapshotManifest: "baseline/snapshot.manifest.json",
		GeneratedReport:         "reports/catalog.md",
		EntryCount:              len(entries),
		StatusCounts:            counts,
	}
	return entries, manifest
}

func TestValidateAcceptsSyntheticBaseline(t *testing.T) {
	entries, manifest := base()
	if err := catalog.Validate(entries, manifest); err != nil {
		t.Fatalf("Validate(valid baseline) = %v", err)
	}
}

func TestValidateAcceptsCatalogBaselineArtifact(t *testing.T) {
	entries, _ := base()
	const id = "contract:baseline/catalog-snapshot"
	entries = append(entries, catalog.Entry{
		SchemaVersion: catalog.SchemaVersion,
		ID:            id,
		BaselineRole:  catalog.BaselineRoleCatalog,
		Upstream: catalog.Upstream{
			Module: "ai", Repository: "https://github.com/earendil-works/pi", Commit: catalogBaselineCommit,
			Reference: "releases/download/v0.84.1/pi-0.84.1-source.tar.gz",
		},
		Mapping:        catalog.Mapping{Module: "ai", Target: "parity/baseline/snapshot.manifest.json", Kind: "contract"},
		Status:         catalog.StatusVerified,
		Milestone:      "M0",
		Classification: "direct-entry",
		Evidence: []catalog.Evidence{{
			Kind: "go-test", Ref: "internal/m0gate/gate_test.go#TestM0CatalogSnapshotIsCaptured", Baseline: catalogBaselineCommit,
			CaseID: "issue35-dual-source-catalog-snapshot", InputHash: "sha256:294d8067eb42327be0db4792d3be792daff588d8fc22549270a972ec9e5407e7",
			ExecutionMethod: "go test ./internal/m0gate", Expected: "lossless snapshot", Actual: "snapshot verified", Platform: "any", CatalogID: id,
		}},
	})
	manifest := catalog.BuildManifest(entries, baselineCommit, catalog.DefaultManifestPaths)

	if manifest.CatalogBaselineCommit != catalogBaselineCommit {
		t.Fatalf("catalog baseline commit = %q, want %q", manifest.CatalogBaselineCommit, catalogBaselineCommit)
	}
	if err := catalog.Validate(entries, manifest); err != nil {
		t.Fatalf("Validate(catalog baseline artifact) = %v", err)
	}

	entries[len(entries)-1].Evidence[0].Baseline = baselineCommit
	if err := catalog.Validate(entries, manifest); !errors.Is(err, catalog.ErrMissingEvidence) {
		t.Fatalf("Validate(wrong catalog evidence baseline) = %v, want ErrMissingEvidence", err)
	}
}

func TestValidateRejectsCatalogRoleWithWrongCommit(t *testing.T) {
	entries, _ := base()
	entries[0].BaselineRole = catalog.BaselineRoleCatalog
	manifest := catalog.BuildManifest(entries, baselineCommit, catalog.DefaultManifestPaths)
	manifest.CatalogBaselineCommit = catalogBaselineCommit

	if err := catalog.Validate(entries, manifest); !errors.Is(err, catalog.ErrCommitMismatch) {
		t.Fatalf("Validate(catalog role with code commit) = %v, want ErrCommitMismatch", err)
	}
}

func TestValidateAcceptsOpenAICompletionsMatrixEntry(t *testing.T) {
	entries, _ := base()
	entry := catalog.Entry{
		SchemaVersion:  catalog.SchemaVersion,
		ID:             "matrix:ai/openai-completions/option/temperature",
		Upstream:       catalog.Upstream{Module: "ai", Repository: "https://github.com/badlogic/pi-mono", Commit: baselineCommit, Reference: "packages/ai/src/types.ts#StreamOptions.temperature"},
		Mapping:        catalog.Mapping{Module: "ai", Target: "github.com/nankedr/pig/ai.OpenAICompletionsOptions.Temperature", Kind: "field"},
		Status:         catalog.StatusScaffolded,
		Milestone:      "M1",
		Classification: "public-api",
		Matrix: &catalog.CapabilityMatrix{
			API:               "openai-completions",
			Surface:           catalog.MatrixSurfacePublicAPI,
			Category:          catalog.MatrixCategoryOption,
			Pi:                catalog.MatrixField{Name: "temperature", Type: "number"},
			Go:                catalog.MatrixField{Name: "OpenAICompletionsOptions.Temperature", Type: "*float64"},
			Direction:         catalog.MatrixDirectionRequest,
			PreTargetBehavior: catalog.MatrixBehaviorErrNotImplemented,
			ValueSemantics: catalog.MatrixValueSemantics{
				States:      []string{catalog.MatrixValueAbsent, catalog.MatrixValueZero, catalog.MatrixValueValue},
				Description: "absent omits temperature; explicit zero is sent",
			},
			EvidenceRequirements: []catalog.MatrixEvidenceRequirement{{
				Kind:      catalog.MatrixEvidenceFixture,
				Assertion: "request fixture distinguishes absent from explicit zero",
			}},
		},
	}
	entries = append(entries, entry)
	manifest := catalog.BuildManifest(entries, baselineCommit, catalog.DefaultManifestPaths)

	if err := catalog.Validate(entries, manifest); err != nil {
		t.Fatalf("Validate(valid OpenAI Completions matrix entry) = %v", err)
	}
}

func TestValidateVerifiedMatrixEvidence(t *testing.T) {
	validEvidence := func(kind, catalogID string) catalog.Evidence {
		return catalog.Evidence{
			Kind: kind, Ref: "parity/oracle/fixtures/openai-completions-m0-no-op.json", Baseline: baselineCommit,
			CaseID: "metadata", InputHash: "sha256:0000000000000000000000000000000000000000000000000000000000000000",
			ExecutionMethod: "go test ./ai", Expected: "no-op", Actual: "no-op", Platform: "any", CatalogID: catalogID,
		}
	}
	verifiedEntry := func() catalog.Entry {
		const id = "matrix:verified"
		return catalog.Entry{
			SchemaVersion: catalog.SchemaVersion, ID: id,
			Upstream: catalog.Upstream{Module: "ai", Repository: "https://github.com/badlogic/pi-mono", Commit: baselineCommit, Reference: "packages/ai/src/types.ts#StreamOptions.metadata"},
			Mapping:  catalog.Mapping{Module: "ai", Target: "github.com/nankedr/pig/ai.OpenAICompletionsOptions.Metadata", Kind: "field"},
			Status:   catalog.StatusVerified, Milestone: "M0", Classification: "public-api",
			Matrix: &catalog.CapabilityMatrix{
				API: "openai-completions", Surface: catalog.MatrixSurfacePublicAPI, Category: catalog.MatrixCategoryOption,
				Pi: catalog.MatrixField{Name: "metadata", Type: "Record<string, unknown> | undefined"}, Go: catalog.MatrixField{Name: "Metadata", Type: "map[string]json.RawMessage"},
				Direction: catalog.MatrixDirectionRequest, PreTargetBehavior: catalog.MatrixBehaviorNoOp,
				ValueSemantics:       catalog.MatrixValueSemantics{States: []string{catalog.MatrixValueAbsent, catalog.MatrixValueValue}, Description: "ignored"},
				EvidenceRequirements: []catalog.MatrixEvidenceRequirement{{Kind: catalog.MatrixEvidenceOracle, Assertion: "Pi request is unchanged"}, {Kind: catalog.MatrixEvidenceGoTest, Assertion: "Pig stub is unchanged"}},
			},
			Evidence: []catalog.Evidence{validEvidence(catalog.MatrixEvidenceOracle, id), validEvidence(catalog.MatrixEvidenceGoTest, id)},
		}
	}

	t.Run("complete evidence", func(t *testing.T) {
		entries, _ := base()
		entries = append(entries, verifiedEntry())
		if err := catalog.Validate(entries, catalog.BuildManifest(entries, baselineCommit, catalog.DefaultManifestPaths)); err != nil {
			t.Fatalf("Validate(complete verified matrix evidence) = %v", err)
		}
	})

	for _, tt := range []struct {
		name   string
		mutate func(*catalog.Entry)
	}{
		{name: "missing case id", mutate: func(e *catalog.Entry) { e.Evidence[0].CaseID = "" }},
		{name: "malformed input hash", mutate: func(e *catalog.Entry) { e.Evidence[0].InputHash = "sha256:ABC" }},
		{name: "missing execution method", mutate: func(e *catalog.Entry) { e.Evidence[0].ExecutionMethod = "" }},
		{name: "missing expected semantics", mutate: func(e *catalog.Entry) { e.Evidence[0].Expected = "" }},
		{name: "missing actual result", mutate: func(e *catalog.Entry) { e.Evidence[0].Actual = "" }},
		{name: "missing platform", mutate: func(e *catalog.Entry) { e.Evidence[0].Platform = "" }},
		{name: "wrong catalog binding", mutate: func(e *catalog.Entry) { e.Evidence[0].CatalogID = "matrix:other" }},
		{name: "missing required kind", mutate: func(e *catalog.Entry) { e.Evidence = e.Evidence[:1] }},
		{name: "duplicate kind and case", mutate: func(e *catalog.Entry) { e.Evidence = append(e.Evidence, e.Evidence[0]) }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			entries, _ := base()
			entry := verifiedEntry()
			tt.mutate(&entry)
			entries = append(entries, entry)
			err := catalog.Validate(entries, catalog.BuildManifest(entries, baselineCommit, catalog.DefaultManifestPaths))
			if !errors.Is(err, catalog.ErrMissingEvidence) {
				t.Fatalf("Validate() = %v, want ErrMissingEvidence", err)
			}
		})
	}
}

func TestValidateVerifiedEntryRequiresCompleteEvidence(t *testing.T) {
	entries, _ := base()
	entries[4].Evidence[0].CaseID = ""
	err := catalog.Validate(entries, catalog.BuildManifest(entries, baselineCommit, catalog.DefaultManifestPaths))
	if !errors.Is(err, catalog.ErrMissingEvidence) {
		t.Fatalf("Validate() = %v, want ErrMissingEvidence", err)
	}
}

func TestValidateRejectsMatrixMappingKindMismatch(t *testing.T) {
	for _, kind := range []string{"field", "behavior"} {
		t.Run(kind+" without matrix", func(t *testing.T) {
			entries, _ := base()
			entries[0].Mapping.Kind = kind
			manifest := catalog.BuildManifest(entries, baselineCommit, catalog.DefaultManifestPaths)

			err := catalog.Validate(entries, manifest)
			if !errors.Is(err, catalog.ErrIncompleteMatrix) {
				t.Fatalf("Validate(%s without matrix) = %v, want ErrIncompleteMatrix", kind, err)
			}
		})
	}
}

func TestValidateRejectsDuplicateMatrixCoordinates(t *testing.T) {
	entries, _ := base()
	matrix := &catalog.CapabilityMatrix{
		API: "openai-completions", Surface: catalog.MatrixSurfaceInternalWire, Category: catalog.MatrixCategoryDelta,
		Pi: catalog.MatrixField{Name: "delta.content", Type: "string | null"}, Go: catalog.MatrixField{Name: "DecodedDelta.Content", Type: "*string"},
		Direction: catalog.MatrixDirectionResponse, PreTargetBehavior: catalog.MatrixBehaviorErrNotImplemented,
		ValueSemantics:       catalog.MatrixValueSemantics{States: []string{catalog.MatrixValueNull, catalog.MatrixValueValue}, Description: "null is ignored; text appends"},
		EvidenceRequirements: []catalog.MatrixEvidenceRequirement{{Kind: catalog.MatrixEvidenceFixture, Assertion: "delta fixture distinguishes null and text"}},
	}
	for _, id := range []string{"matrix:first", "matrix:second"} {
		copyOfMatrix := *matrix
		entries = append(entries, catalog.Entry{
			SchemaVersion: catalog.SchemaVersion, ID: id,
			Upstream: catalog.Upstream{Module: "ai", Repository: "https://github.com/badlogic/pi-mono", Commit: baselineCommit, Reference: "packages/ai/src/api/openai-completions.ts#delta.content"},
			Mapping:  catalog.Mapping{Module: "ai", Target: "github.com/nankedr/pig/ai/internal/openai.DecodedDelta.Content", Kind: "field"},
			Status:   catalog.StatusInventoried, Milestone: "M1", Classification: "private-impl", Matrix: &copyOfMatrix,
		})
	}
	manifest := catalog.BuildManifest(entries, baselineCommit, catalog.DefaultManifestPaths)
	if err := catalog.Validate(entries, manifest); !errors.Is(err, catalog.ErrDuplicateMatrix) {
		t.Fatalf("Validate(duplicate matrix coordinate) = %v, want ErrDuplicateMatrix", err)
	}
}

func TestValidateAcceptsEntrypointSymbolAndBehaviorRows(t *testing.T) {
	entries, _ := base()
	rows := []catalog.Entry{
		{
			SchemaVersion: catalog.SchemaVersion, ID: "matrix:entrypoint",
			Upstream: catalog.Upstream{Module: "ai", Repository: "https://github.com/badlogic/pi-mono", Commit: baselineCommit, Reference: "packages/ai/src/api/openai-completions.ts#stream"},
			Mapping:  catalog.Mapping{Module: "ai", Target: "github.com/nankedr/pig/ai.StreamOpenAICompletions", Kind: "symbol"},
			Status:   catalog.StatusScaffolded, Milestone: "M1", Classification: "public-api",
			Matrix: &catalog.CapabilityMatrix{
				API: "openai-completions", Surface: catalog.MatrixSurfacePublicAPI, Category: catalog.MatrixCategoryEntrypoint,
				Pi: catalog.MatrixField{Name: "stream", Type: "function"}, Go: catalog.MatrixField{Name: "StreamOpenAICompletions", Type: "func"},
				Direction: catalog.MatrixDirectionBidirectional, PreTargetBehavior: catalog.MatrixBehaviorErrNotImplemented,
				ValueSemantics:       catalog.MatrixValueSemantics{States: []string{catalog.MatrixValueValue}, Description: "returns a failed stream until M1"},
				EvidenceRequirements: []catalog.MatrixEvidenceRequirement{{Kind: catalog.MatrixEvidenceGoTest, Assertion: "entrypoint behavior"}},
			},
		},
		{
			SchemaVersion: catalog.SchemaVersion, ID: "matrix:behavior",
			Upstream: catalog.Upstream{Module: "ai", Repository: "https://github.com/badlogic/pi-mono", Commit: baselineCommit, Reference: "packages/ai/src/utils/provider-retry.ts#retry-status"},
			Mapping:  catalog.Mapping{Module: "ai", Target: "github.com/nankedr/pig/parity/cases/ai/openai-completions.RetryStatus", Kind: "behavior"},
			Status:   catalog.StatusInventoried, Milestone: "M1", Classification: "dormant-test-support",
			Matrix: &catalog.CapabilityMatrix{
				API: "openai-completions", Surface: catalog.MatrixSurfaceFixture, Category: catalog.MatrixCategoryError,
				Pi: catalog.MatrixField{Name: "retry.status.429", Type: "HTTP 429"}, Go: catalog.MatrixField{Name: "RetryFixture.Status429", Type: "bool"},
				Direction: catalog.MatrixDirectionResponse, PreTargetBehavior: catalog.MatrixBehaviorErrNotImplemented,
				ValueSemantics:       catalog.MatrixValueSemantics{States: []string{catalog.MatrixValueValue}, Description: "429 is retryable"},
				EvidenceRequirements: []catalog.MatrixEvidenceRequirement{{Kind: catalog.MatrixEvidenceLocalServer, Assertion: "retry status"}},
			},
		},
	}
	entries = append(entries, rows...)
	manifest := catalog.BuildManifest(entries, baselineCommit, catalog.DefaultManifestPaths)
	if err := catalog.Validate(entries, manifest); err != nil {
		t.Fatalf("Validate(entrypoint and behavior rows) = %v", err)
	}
}

func TestValidateRejectsEntrypointMappingKindMismatch(t *testing.T) {
	entries, _ := base()
	matrix := &catalog.CapabilityMatrix{
		API: "openai-completions", Surface: catalog.MatrixSurfacePublicAPI, Category: catalog.MatrixCategoryEntrypoint,
		Pi: catalog.MatrixField{Name: "stream", Type: "function"}, Go: catalog.MatrixField{Name: "StreamOpenAICompletions", Type: "func"},
		Direction: catalog.MatrixDirectionBidirectional, PreTargetBehavior: catalog.MatrixBehaviorErrNotImplemented,
		ValueSemantics:       catalog.MatrixValueSemantics{States: []string{catalog.MatrixValueValue}, Description: "stub"},
		EvidenceRequirements: []catalog.MatrixEvidenceRequirement{{Kind: catalog.MatrixEvidenceGoTest, Assertion: "stub"}},
	}
	for _, tt := range []struct {
		name, kind, category string
	}{
		{name: "entrypoint as field", kind: "field", category: catalog.MatrixCategoryEntrypoint},
		{name: "non-entrypoint as symbol", kind: "symbol", category: catalog.MatrixCategoryRequest},
	} {
		t.Run(tt.name, func(t *testing.T) {
			copyOfMatrix := *matrix
			copyOfMatrix.Category = tt.category
			got := append([]catalog.Entry(nil), entries...)
			got = append(got, catalog.Entry{
				SchemaVersion: catalog.SchemaVersion, ID: "matrix:mismatch",
				Upstream: catalog.Upstream{Module: "ai", Repository: "https://github.com/badlogic/pi-mono", Commit: baselineCommit, Reference: "packages/ai/src/api/openai-completions.ts#stream"},
				Mapping:  catalog.Mapping{Module: "ai", Target: "github.com/nankedr/pig/ai.StreamOpenAICompletions", Kind: tt.kind},
				Status:   catalog.StatusScaffolded, Milestone: "M1", Classification: "public-api", Matrix: &copyOfMatrix,
			})
			manifest := catalog.BuildManifest(got, baselineCommit, catalog.DefaultManifestPaths)
			if err := catalog.Validate(got, manifest); !errors.Is(err, catalog.ErrIncompleteMatrix) {
				t.Fatalf("Validate() = %v, want ErrIncompleteMatrix", err)
			}
		})
	}
}

func TestValidateRejectsIncompleteOpenAICompletionsMatrixMetadata(t *testing.T) {
	valid := catalog.CapabilityMatrix{
		API:                  "openai-completions",
		Surface:              catalog.MatrixSurfaceInternalWire,
		Category:             catalog.MatrixCategoryDelta,
		Pi:                   catalog.MatrixField{Name: "delta.content", Type: "string | null"},
		Go:                   catalog.MatrixField{Name: "AssistantMessageTextDeltaEvent.Delta", Type: "string"},
		Direction:            catalog.MatrixDirectionResponse,
		PreTargetBehavior:    catalog.MatrixBehaviorErrNotImplemented,
		ValueSemantics:       catalog.MatrixValueSemantics{States: []string{catalog.MatrixValueNull, catalog.MatrixValueEmpty, catalog.MatrixValueValue}, Description: "null and empty deltas emit no event"},
		EvidenceRequirements: []catalog.MatrixEvidenceRequirement{{Kind: catalog.MatrixEvidenceOracle, Assertion: "Pi/Pig delta event sequence matches"}},
	}

	for _, tt := range []struct {
		name     string
		mutate   func(*catalog.CapabilityMatrix)
		sentinel error
	}{
		{name: "missing API", mutate: func(m *catalog.CapabilityMatrix) { m.API = "" }, sentinel: catalog.ErrIncompleteMatrix},
		{name: "blank API", mutate: func(m *catalog.CapabilityMatrix) { m.API = "  " }, sentinel: catalog.ErrIncompleteMatrix},
		{name: "illegal surface", mutate: func(m *catalog.CapabilityMatrix) { m.Surface = "private" }, sentinel: catalog.ErrIllegalMatrixValue},
		{name: "illegal category", mutate: func(m *catalog.CapabilityMatrix) { m.Category = "chunkish" }, sentinel: catalog.ErrIllegalMatrixValue},
		{name: "missing Pi type", mutate: func(m *catalog.CapabilityMatrix) { m.Pi.Type = "" }, sentinel: catalog.ErrIncompleteMatrix},
		{name: "blank Pi name", mutate: func(m *catalog.CapabilityMatrix) { m.Pi.Name = " \t" }, sentinel: catalog.ErrIncompleteMatrix},
		{name: "missing Go name", mutate: func(m *catalog.CapabilityMatrix) { m.Go.Name = "" }, sentinel: catalog.ErrIncompleteMatrix},
		{name: "illegal direction", mutate: func(m *catalog.CapabilityMatrix) { m.Direction = "sideways" }, sentinel: catalog.ErrIllegalMatrixValue},
		{name: "illegal behavior", mutate: func(m *catalog.CapabilityMatrix) { m.PreTargetBehavior = "silent-drop" }, sentinel: catalog.ErrIllegalMatrixValue},
		{name: "missing value states", mutate: func(m *catalog.CapabilityMatrix) { m.ValueSemantics.States = nil }, sentinel: catalog.ErrIncompleteMatrix},
		{name: "duplicate value state", mutate: func(m *catalog.CapabilityMatrix) {
			m.ValueSemantics.States = []string{catalog.MatrixValueNull, catalog.MatrixValueNull}
		}, sentinel: catalog.ErrIllegalMatrixValue},
		{name: "illegal value state", mutate: func(m *catalog.CapabilityMatrix) { m.ValueSemantics.States = []string{"undefined-ish"} }, sentinel: catalog.ErrIllegalMatrixValue},
		{name: "missing value description", mutate: func(m *catalog.CapabilityMatrix) { m.ValueSemantics.Description = " " }, sentinel: catalog.ErrIncompleteMatrix},
		{name: "missing evidence requirements", mutate: func(m *catalog.CapabilityMatrix) { m.EvidenceRequirements = nil }, sentinel: catalog.ErrIncompleteMatrix},
		{name: "illegal evidence kind", mutate: func(m *catalog.CapabilityMatrix) { m.EvidenceRequirements[0].Kind = "hope" }, sentinel: catalog.ErrIllegalMatrixValue},
		{name: "missing evidence assertion", mutate: func(m *catalog.CapabilityMatrix) { m.EvidenceRequirements[0].Assertion = "" }, sentinel: catalog.ErrIncompleteMatrix},
	} {
		t.Run(tt.name, func(t *testing.T) {
			entries, _ := base()
			matrix := valid
			matrix.ValueSemantics.States = append([]string(nil), valid.ValueSemantics.States...)
			matrix.EvidenceRequirements = append([]catalog.MatrixEvidenceRequirement(nil), valid.EvidenceRequirements...)
			tt.mutate(&matrix)
			entries = append(entries, catalog.Entry{
				SchemaVersion: catalog.SchemaVersion, ID: "matrix:test",
				Upstream: catalog.Upstream{Module: "ai", Repository: "https://github.com/badlogic/pi-mono", Commit: baselineCommit, Reference: "packages/ai/src/api/openai-completions.ts"},
				Mapping:  catalog.Mapping{Module: "ai", Target: "github.com/nankedr/pig/ai", Kind: "field"},
				Status:   catalog.StatusInventoried, Milestone: "M1", Classification: "private-impl", Matrix: &matrix,
			})
			manifest := catalog.BuildManifest(entries, baselineCommit, catalog.DefaultManifestPaths)
			err := catalog.Validate(entries, manifest)
			if !errors.Is(err, tt.sentinel) {
				t.Fatalf("Validate() = %v, want %v", err, tt.sentinel)
			}
		})
	}
}

func TestGenerateReportRendersCapabilityMatrixFromEntries(t *testing.T) {
	entries, _ := base()
	entries = append(entries, catalog.Entry{
		SchemaVersion: catalog.SchemaVersion, ID: "matrix:ai/openai-completions/delta/content",
		Upstream: catalog.Upstream{Module: "ai", Repository: "https://github.com/badlogic/pi-mono", Commit: baselineCommit, Reference: "packages/ai/src/api/openai-completions.ts#delta.content"},
		Mapping:  catalog.Mapping{Module: "ai", Target: "github.com/nankedr/pig/ai.AssistantMessageTextDeltaEvent.Delta", Kind: "field"},
		Status:   catalog.StatusInventoried, Milestone: "M1", Classification: "private-impl",
		Matrix: &catalog.CapabilityMatrix{
			API: "openai-completions", Surface: catalog.MatrixSurfaceInternalWire, Category: catalog.MatrixCategoryDelta,
			Pi:        catalog.MatrixField{Name: "delta.content", Type: "string | null"},
			Go:        catalog.MatrixField{Name: "AssistantMessageTextDeltaEvent.Delta", Type: "string"},
			Direction: catalog.MatrixDirectionResponse, PreTargetBehavior: catalog.MatrixBehaviorErrNotImplemented,
			ValueSemantics:       catalog.MatrixValueSemantics{States: []string{catalog.MatrixValueNull, catalog.MatrixValueEmpty, catalog.MatrixValueValue}, Description: "null and empty deltas emit no event | non-empty text appends"},
			EvidenceRequirements: []catalog.MatrixEvidenceRequirement{{Kind: catalog.MatrixEvidenceFixture, Assertion: "delta sequence uses `text_delta` events"}},
		},
	})

	report := catalog.GenerateReport(entries)
	for _, want := range []string{
		"## OpenAI Chat Completions capability matrix",
		"| Surface | Category | Pi capability / type | Go mapping / type | Direction | Target | Status | Before target | Value semantics | Evidence required | Pi source |",
		"`delta.content`<br>`string \\| null`",
		"null, empty, value — null and empty deltas emit no event \\| non-empty text appends",
		"fixture: delta sequence uses `text_delta` events",
	} {
		if !strings.Contains(report, want) {
			t.Fatalf("generated report missing %q:\n%s", want, report)
		}
	}
}

func TestGenerateReportEscapesBackticksInsideCodeCells(t *testing.T) {
	entries, _ := base()
	entries = append(entries, catalog.Entry{
		SchemaVersion: catalog.SchemaVersion, ID: "matrix:backticks",
		Upstream: catalog.Upstream{Module: "ai", Repository: "https://github.com/badlogic/pi-mono", Commit: baselineCommit, Reference: "packages/ai/src/types.ts#literal`field"},
		Mapping:  catalog.Mapping{Module: "ai", Target: "github.com/nankedr/pig/ai.Field", Kind: "field"},
		Status:   catalog.StatusInventoried, Milestone: "M1", Classification: "public-api",
		Matrix: &catalog.CapabilityMatrix{
			API: "openai-completions", Surface: catalog.MatrixSurfacePublicAPI, Category: catalog.MatrixCategoryRequest,
			Pi: catalog.MatrixField{Name: "literal`field", Type: "`x` | string"}, Go: catalog.MatrixField{Name: "Field", Type: "string"},
			Direction: catalog.MatrixDirectionRequest, PreTargetBehavior: catalog.MatrixBehaviorErrNotImplemented,
			ValueSemantics:       catalog.MatrixValueSemantics{States: []string{catalog.MatrixValueValue}, Description: "literal"},
			EvidenceRequirements: []catalog.MatrixEvidenceRequirement{{Kind: catalog.MatrixEvidenceGoTest, Assertion: "literal"}},
		},
	})
	report := catalog.GenerateReport(entries)
	for _, want := range []string{"`` literal`field ``", "`` `x` \\| string ``", "`` packages/ai/src/types.ts#literal`field ``"} {
		if !strings.Contains(report, want) {
			t.Fatalf("generated report does not safely fence %q:\n%s", want, report)
		}
	}
}

func TestOpenAICompletionsMatrixCoversRequiredFamiliesAndMilestones(t *testing.T) {
	root := repoRoot(t)
	entries, err := catalog.LoadCatalog(filepath.Join(root, "parity", "catalog.jsonl"))
	if err != nil {
		t.Fatalf("LoadCatalog(real) = %v", err)
	}

	categories := map[string]bool{}
	surfaces := map[string]bool{}
	milestones := map[string]bool{}
	behaviors := map[string]bool{}
	count := 0
	for _, entry := range entries {
		if entry.Matrix == nil || entry.Matrix.API != "openai-completions" {
			continue
		}
		count++
		categories[entry.Matrix.Category] = true
		surfaces[entry.Matrix.Surface] = true
		milestones[entry.Milestone] = true
		behaviors[entry.Matrix.PreTargetBehavior] = true
	}

	if count < 500 {
		t.Fatalf("OpenAI Completions matrix has %d rows, want at least 500 field/entrypoint/behavior rows", count)
	}
	for _, category := range []string{
		catalog.MatrixCategoryEntrypoint, catalog.MatrixCategoryMessage, catalog.MatrixCategoryContent, catalog.MatrixCategoryTool,
		catalog.MatrixCategoryToolChoice, catalog.MatrixCategoryEvent, catalog.MatrixCategoryUsage,
		catalog.MatrixCategoryError, catalog.MatrixCategoryOption, catalog.MatrixCategoryRequest,
		catalog.MatrixCategoryHeader, catalog.MatrixCategorySSE, catalog.MatrixCategoryDelta,
		catalog.MatrixCategoryCompat,
	} {
		if !categories[category] {
			t.Errorf("OpenAI Completions matrix missing category %q", category)
		}
	}
	for _, surface := range []string{catalog.MatrixSurfacePublicAPI, catalog.MatrixSurfaceInternalWire, catalog.MatrixSurfaceFixture} {
		if !surfaces[surface] {
			t.Errorf("OpenAI Completions matrix missing surface %q", surface)
		}
	}
	for _, milestone := range []string{"M0", "M1", "M2", "M10", "M12"} {
		if !milestones[milestone] {
			t.Errorf("OpenAI Completions matrix missing milestone %q", milestone)
		}
	}
	for _, behavior := range []string{catalog.MatrixBehaviorErrNotImplemented, catalog.MatrixBehaviorIgnore, catalog.MatrixBehaviorNoOp} {
		if !behaviors[behavior] {
			t.Errorf("OpenAI Completions matrix missing pre-target behavior %q", behavior)
		}
	}
}

func TestOpenAICompletionsMatrixCoversEveryPublicOptionAndCompatField(t *testing.T) {
	want := []string{
		"OpenAICompletionsOptions.signal", "OpenAICompletionsOptions.telemetryContext", "OpenAICompletionsOptions.apiKey",
		"OpenAICompletionsOptions.fetch", "OpenAICompletionsOptions.env", "OpenAICompletionsOptions.onPayload",
		"OpenAICompletionsOptions.onResponse", "OpenAICompletionsOptions.headers", "OpenAICompletionsOptions.timeoutMs",
		"OpenAICompletionsOptions.maxRetries", "OpenAICompletionsOptions.maxRetryDelayMs", "OpenAICompletionsOptions.temperature",
		"OpenAICompletionsOptions.samplingParams", "OpenAICompletionsOptions.maxTokens", "OpenAICompletionsOptions.transport",
		"OpenAICompletionsOptions.cacheRetention", "OpenAICompletionsOptions.sessionId", "OpenAICompletionsOptions.websocketConnectTimeoutMs",
		"OpenAICompletionsOptions.metadata", "OpenAICompletionsOptions.toolChoice", "OpenAICompletionsOptions.reasoningEffort",
		"OpenAICompletionsOptions.thinkingBudgets",
		"OpenAICompletionsCompat.supportsStore", "OpenAICompletionsCompat.supportsDeveloperRole",
		"OpenAICompletionsCompat.supportsReasoningEffort", "OpenAICompletionsCompat.supportsUsageInStreaming",
		"OpenAICompletionsCompat.supportsFinishReason", "OpenAICompletionsCompat.maxTokensField",
		"OpenAICompletionsCompat.requiresToolResultName", "OpenAICompletionsCompat.requiresAssistantAfterToolResult",
		"OpenAICompletionsCompat.requiresThinkingAsText", "OpenAICompletionsCompat.requiresReasoningContentOnAssistantMessages",
		"OpenAICompletionsCompat.thinkingFormat", "OpenAICompletionsCompat.chatTemplateKwargs",
		"OpenAICompletionsCompat.chatTemplateArgs", "OpenAICompletionsCompat.openRouterRouting",
		"OpenAICompletionsCompat.vercelGatewayRouting", "OpenAICompletionsCompat.zaiToolStream",
		"OpenAICompletionsCompat.supportsThinkingTokenBudget", "OpenAICompletionsCompat.supportsOpenAIGrammarTools",
		"OpenAICompletionsCompat.supportsStrictMode", "OpenAICompletionsCompat.cacheControlFormat",
		"OpenAICompletionsCompat.sendSessionAffinityHeaders", "OpenAICompletionsCompat.deferredToolsMode",
		"OpenAICompletionsCompat.sessionAffinityFormat", "OpenAICompletionsCompat.supportsLongCacheRetention",
	}

	assertOpenAICompletionsMatrixNames(t, catalog.MatrixSurfacePublicAPI, want)
}

func TestOpenAICompletionsMatrixCoversEntrypointsAndNestedPublicFields(t *testing.T) {
	want := []string{
		"openAICompletionsApi", "stream", "streamSimple", "convertMessages",
		"SimpleStreamOptions.reasoning", "ProviderResponse.status", "ProviderResponse.headers",
		"ConvertCompletionsMessagesOptions.grammarToolInputProperties",
		"OpenAICompletionsOptions.toolChoice.mode",
		"OpenAICompletionsOptions.toolChoice.function.type", "OpenAICompletionsOptions.toolChoice.function.name",
		"OpenAICompletionsOptions.toolChoice.custom.type", "OpenAICompletionsOptions.toolChoice.custom.name",
		"OpenAICompletionsOptions.toolChoice.allowed_tools.type", "OpenAICompletionsOptions.toolChoice.allowed_tools.mode", "OpenAICompletionsOptions.toolChoice.allowed_tools.tools",
		"Tool.constrainedSampling.json_schema.type", "Tool.constrainedSampling.json_schema.strict",
		"Tool.constrainedSampling.grammar.type", "Tool.constrainedSampling.grammar.variants",
		"Tool.constrainedSampling.grammar.variants.openai_lark", "Tool.constrainedSampling.grammar.variants.openai_regex",
		"Usage.cost.input", "Usage.cost.output", "Usage.cost.cacheRead", "Usage.cost.cacheWrite", "Usage.cost.total",
		"ThinkingBudgets.minimal", "ThinkingBudgets.low", "ThinkingBudgets.medium", "ThinkingBudgets.high",
		"UserMessage.content.string", "UserMessage.content.text", "UserMessage.content.image",
		"AssistantMessage.content.text", "AssistantMessage.content.thinking", "AssistantMessage.content.toolCall",
		"ToolResultMessage.content.text", "ToolResultMessage.content.image",
		"AssistantMessageDiagnostic.type", "AssistantMessageDiagnostic.timestamp", "AssistantMessageDiagnostic.error", "AssistantMessageDiagnostic.details",
		"DiagnosticErrorInfo.name", "DiagnosticErrorInfo.message", "DiagnosticErrorInfo.stack", "DiagnosticErrorInfo.code",
		"TextSignatureV1.v", "TextSignatureV1.id", "TextSignatureV1.phase",
		"ChatTemplateKwargValue.$var", "ChatTemplateKwargValue.omitWhenOff",
		"ThinkingLevelMap.off", "ThinkingLevelMap.minimal", "ThinkingLevelMap.low", "ThinkingLevelMap.medium",
		"ThinkingLevelMap.high", "ThinkingLevelMap.xhigh", "ThinkingLevelMap.max",
		"Model.name",
		"ModelCost.input", "ModelCost.output", "ModelCost.cacheRead", "ModelCost.cacheWrite", "ModelCost.tiers",
		"ModelCostRates.input", "ModelCostRates.output", "ModelCostRates.cacheRead", "ModelCostRates.cacheWrite",
		"ModelCostTier.input", "ModelCostTier.output", "ModelCostTier.cacheRead", "ModelCostTier.cacheWrite", "ModelCostTier.inputTokensAbove",
		"OpenRouterRouting.sort.by", "OpenRouterRouting.sort.partition",
		"OpenRouterRouting.max_price.prompt", "OpenRouterRouting.max_price.completion", "OpenRouterRouting.max_price.image",
		"OpenRouterRouting.max_price.audio", "OpenRouterRouting.max_price.request",
		"OpenRouterRouting.preferred_min_throughput.p50", "OpenRouterRouting.preferred_min_throughput.p75",
		"OpenRouterRouting.preferred_min_throughput.p90", "OpenRouterRouting.preferred_min_throughput.p99",
		"OpenRouterRouting.preferred_max_latency.p50", "OpenRouterRouting.preferred_max_latency.p75",
		"OpenRouterRouting.preferred_max_latency.p90", "OpenRouterRouting.preferred_max_latency.p99",
		"DeferredHandle.provider", "DeferredHandle.modelId", "DeferredHandle.api", "DeferredHandle.id",
		"DeferredHandle.expiresAt", "DeferredHandle.pollAfterMs", "DeferredHandle.data",
		"SimpleStreamOptions.deferred.window",
	}
	for _, variant := range []struct {
		name   string
		fields []string
	}{
		{"start", []string{"partial"}},
		{"text_start", []string{"contentIndex", "partial"}},
		{"text_delta", []string{"contentIndex", "delta", "partial"}},
		{"text_end", []string{"contentIndex", "content", "partial"}},
		{"thinking_start", []string{"contentIndex", "partial"}},
		{"thinking_delta", []string{"contentIndex", "delta", "partial"}},
		{"thinking_end", []string{"contentIndex", "content", "partial"}},
		{"toolcall_start", []string{"contentIndex", "partial"}},
		{"toolcall_delta", []string{"contentIndex", "delta", "partial"}},
		{"toolcall_end", []string{"contentIndex", "toolCall", "partial"}},
		{"done", []string{"reason", "message"}},
		{"error", []string{"reason", "error"}},
	} {
		for _, field := range variant.fields {
			want = append(want, "AssistantMessageEvent."+variant.name+"."+field)
		}
	}

	assertOpenAICompletionsMatrixNames(t, catalog.MatrixSurfacePublicAPI, want)
}

func TestOpenAICompletionsMatrixCoversPublicMessageToolUsageAndEventFields(t *testing.T) {
	want := map[string][]string{
		"TextContent":       {"type", "text", "textSignature"},
		"ThinkingContent":   {"type", "thinking", "thinkingSignature", "redacted"},
		"ImageContent":      {"type", "data", "mimeType"},
		"ToolCall":          {"type", "id", "name", "arguments", "thoughtSignature", "namespace"},
		"UserMessage":       {"role", "content", "timestamp"},
		"AssistantMessage":  {"role", "content", "api", "provider", "model", "responseModel", "responseId", "diagnostics", "usage", "stopReason", "deferred", "errorMessage", "rawStopReason", "endTurn", "timestamp"},
		"ToolResultMessage": {"role", "toolCallId", "toolName", "content", "details", "usage", "addedToolNames", "isError", "timestamp"},
		"Tool":              {"name", "description", "parameters", "constrainedSampling"},
		"Usage":             {"input", "output", "cacheRead", "cacheWrite", "cacheWrite1h", "reasoning", "totalTokens", "cost"},
		"DeferredHandle":    {"provider", "modelId", "api", "id", "expiresAt", "pollAfterMs", "data"},
	}

	got := openAICompletionsMatrixNames(t, catalog.MatrixSurfacePublicAPI)
	for symbol, fields := range want {
		for _, field := range fields {
			name := symbol + "." + field
			if !got[name] {
				t.Errorf("OpenAI Completions matrix missing public field %q", name)
			}
		}
	}
	for _, event := range []string{"start", "text_start", "text_delta", "text_end", "thinking_start", "thinking_delta", "thinking_end", "toolcall_start", "toolcall_delta", "toolcall_end", "done", "error"} {
		if !got["AssistantMessageEvent."+event] {
			t.Errorf("OpenAI Completions matrix missing event variant %q", event)
		}
	}
}

func TestOpenAICompletionsMatrixCoversCoreWireFields(t *testing.T) {
	want := []string{
		"request.model", "request.messages", "request.stream", "request.stream_options.include_usage",
		"request.store", "request.max_tokens", "request.max_completion_tokens", "request.temperature",
		"request.tools", "request.tool_choice", "request.prompt_cache_key", "request.prompt_cache_retention",
		"request.cache_control", "request.thinking", "request.reasoning_effort", "request.enable_thinking",
		"request.chat_template_kwargs", "request.chat_template_args", "request.reasoning",
		"request.thinking_token_budget", "request.provider", "request.providerOptions.gateway", "request.tool_stream",
		"header.Authorization", "header.cf-aig-authorization", "header.x-session-id", "header.session_id",
		"header.x-client-request-id", "header.x-session-affinity", "header.x-should-retry",
		"header.retry-after-ms", "header.retry-after",
		"sse.data", "sse.[DONE]", "chunk.id", "chunk.model", "chunk.usage", "chunk.choices",
		"choice.finish_reason", "choice.delta", "choice.usage", "delta.content",
		"delta.reasoning_content", "delta.reasoning", "delta.reasoning_text", "delta.tool_calls",
		"delta.reasoning_details", "usage.prompt_tokens", "usage.completion_tokens",
		"usage.prompt_cache_hit_tokens", "usage.prompt_tokens_details.cached_tokens",
		"usage.prompt_tokens_details.cache_write_tokens", "usage.completion_tokens_details.reasoning_tokens",
	}

	got := openAICompletionsMatrixNames(t, "")
	for _, field := range want {
		if !got[field] {
			t.Errorf("OpenAI Completions matrix missing internal wire field %q", field)
		}
	}
}

func assertOpenAICompletionsMatrixNames(t *testing.T, surface string, want []string) {
	t.Helper()
	got := openAICompletionsMatrixNames(t, surface)
	for _, name := range want {
		if !got[name] {
			t.Errorf("OpenAI Completions matrix missing %s name %q", surface, name)
		}
	}
}

func openAICompletionsMatrixNames(t *testing.T, surface string) map[string]bool {
	t.Helper()
	entries, err := catalog.LoadCatalog(filepath.Join(repoRoot(t), "parity", "catalog.jsonl"))
	if err != nil {
		t.Fatalf("LoadCatalog(real) = %v", err)
	}
	got := make(map[string]bool)
	for _, entry := range entries {
		if entry.Matrix != nil && entry.Matrix.API == "openai-completions" && (entry.Matrix.Surface == surface || surface == "" && entry.Matrix.Surface != catalog.MatrixSurfacePublicAPI) {
			got[entry.Matrix.Pi.Name] = true
		}
	}
	return got
}

func TestOpenAICompletionsMatrixUsesExplicitStubOrBaselineNoOpPolicy(t *testing.T) {
	root := repoRoot(t)
	entries, err := catalog.LoadCatalog(filepath.Join(root, "parity", "catalog.jsonl"))
	if err != nil {
		t.Fatalf("LoadCatalog(real) = %v", err)
	}

	wantNonErrors := map[string]string{
		"OpenAICompletionsOptions.transport":                 catalog.MatrixBehaviorNoOp,
		"OpenAICompletionsOptions.websocketConnectTimeoutMs": catalog.MatrixBehaviorNoOp,
		"OpenAICompletionsOptions.metadata":                  catalog.MatrixBehaviorNoOp,
		"OpenAICompletionsOptions.telemetryContext":          catalog.MatrixBehaviorNoOp,
		"SimpleStreamOptions.deferred":                       catalog.MatrixBehaviorNoOp,
		"SimpleStreamOptions.deferred.window":                catalog.MatrixBehaviorNoOp,
		"TextContent.textSignature":                          catalog.MatrixBehaviorIgnore,
		"ToolCall.namespace":                                 catalog.MatrixBehaviorIgnore,
		"UserMessage.timestamp":                              catalog.MatrixBehaviorIgnore,
		"ToolResultMessage.details":                          catalog.MatrixBehaviorIgnore,
		"ToolResultMessage.usage":                            catalog.MatrixBehaviorIgnore,
		"ToolResultMessage.isError":                          catalog.MatrixBehaviorIgnore,
		"ToolResultMessage.timestamp":                        catalog.MatrixBehaviorIgnore,
		"chunk.null-or-non-object":                           catalog.MatrixBehaviorIgnore,
		"chunk.missing-first-choice":                         catalog.MatrixBehaviorIgnore,
		"delta.empty":                                        catalog.MatrixBehaviorIgnore,
	}
	found := map[string]bool{}
	for _, entry := range entries {
		if entry.Matrix == nil || entry.Matrix.API != "openai-completions" {
			continue
		}
		if entry.Matrix.PreTargetBehavior == catalog.MatrixBehaviorErrNotImplemented {
			continue
		}
		wantBehavior, allowed := wantNonErrors[entry.Matrix.Pi.Name]
		if !allowed {
			t.Errorf("matrix row %s silently ignores unsupported capability %q", entry.ID, entry.Matrix.Pi.Name)
		} else if entry.Matrix.PreTargetBehavior != wantBehavior {
			t.Errorf("matrix row %s behavior = %q, want %q", entry.ID, entry.Matrix.PreTargetBehavior, wantBehavior)
		}
		if entry.Matrix.PreTargetBehavior == catalog.MatrixBehaviorNoOp && entry.Milestone != "M0" {
			t.Errorf("baseline no-op row %s milestone = %q, want M0", entry.ID, entry.Milestone)
		}
		found[entry.Matrix.Pi.Name] = true
	}
	for name := range wantNonErrors {
		if !found[name] {
			t.Errorf("matrix does not separately register baseline ignore/no-op %q", name)
		}
	}
}

func TestOpenAICompletionsMatrixTracksVerifiedSlices(t *testing.T) {
	root := repoRoot(t)
	entries, err := catalog.LoadCatalog(filepath.Join(root, "parity", "catalog.jsonl"))
	if err != nil {
		t.Fatalf("LoadCatalog(real) = %v", err)
	}

	verifiedFixtureIDs := openAICompletionsFixtureCatalogIDs(t, root, "openai-completions-text.json", "ai/openai-completions/m1-text")
	for id := range openAICompletionsFixtureCatalogIDs(t, root, "openai-completions-sse.json", "ai/openai-completions/m1-sse-boundaries") {
		verifiedFixtureIDs[id] = true
	}
	for id := range openAICompletionsFixtureCatalogIDs(t, root, "openai-completions-retry.json", "ai/openai-completions/m1-transport-retry") {
		verifiedFixtureIDs[id] = true
	}
	toolFixtureIDs := openAICompletionsFixtureCatalogIDs(t, root, "openai-completions-tools.json", "ai/openai-completions/m1-streaming-tools")
	toolFixturePartialIDs := map[string]bool{
		"matrix:ai/openai-completions/content/assistant-message-content-tool-call": true,
		"matrix:ai/openai-completions/delta/partial-json-empty":                    true,
		"matrix:ai/openai-completions/request/request-tool-choice":                 true,
		"matrix:ai/openai-completions/request/request-tools":                       true,
		"matrix:ai/openai-completions/tool/tool-call-arguments":                    true,
		"matrix:ai/openai-completions/tool/tool-call-id":                           true,
		"matrix:ai/openai-completions/tool/tool-call-name":                         true,
		"matrix:ai/openai-completions/tool/tool-call-type":                         true,
	}
	toolResultFixtureIDs := openAICompletionsFixtureCatalogIDs(t, root, "openai-completions-tool-result.json", "ai/openai-completions/m1-tool-result-round-trip")
	toolResultFixturePartialIDs := map[string]bool{
		"matrix:ai/openai-completions/content/assistant-message-content-tool-call": true,
		"matrix:ai/openai-completions/entrypoint/convert-messages":                 true,
		"matrix:ai/openai-completions/request/context-messages":                    true,
		"matrix:ai/openai-completions/request/request-messages":                    true,
	}
	for id := range toolResultFixtureIDs {
		toolFixtureIDs[id] = true
	}
	for id := range toolResultFixturePartialIDs {
		toolFixturePartialIDs[id] = true
	}
	for _, entry := range entries {
		if entry.Matrix == nil || entry.Matrix.API != "openai-completions" {
			continue
		}
		if entry.Matrix.PreTargetBehavior == catalog.MatrixBehaviorNoOp {
			if entry.Status != catalog.StatusVerified {
				t.Errorf("baseline no-op matrix row %s status = %q, want verified", entry.ID, entry.Status)
			}
			if len(entry.Evidence) == 0 {
				t.Errorf("baseline no-op matrix row %s has no achieved evidence", entry.ID)
			}
			for _, kind := range []string{catalog.MatrixEvidenceOracle, catalog.MatrixEvidenceGoTest} {
				found := false
				for _, evidence := range entry.Evidence {
					if evidence.Kind == kind && evidence.CatalogID == entry.ID {
						found = true
					}
				}
				if !found {
					t.Errorf("baseline no-op matrix row %s lacks bound %s evidence", entry.ID, kind)
				}
			}
			continue
		}
		if verifiedFixtureIDs[entry.ID] {
			if entry.Status != catalog.StatusVerified || len(entry.Evidence) == 0 {
				t.Errorf("fixture-backed matrix row %s = status %q, evidence %d", entry.ID, entry.Status, len(entry.Evidence))
			}
			continue
		}
		if toolFixtureIDs[entry.ID] {
			if toolFixturePartialIDs[entry.ID] {
				if entry.Status != catalog.StatusPartial || entry.Partial == nil || len(entry.Evidence) == 0 {
					t.Errorf("partial tool fixture row %s = status %q, evidence %d", entry.ID, entry.Status, len(entry.Evidence))
				}
			} else if entry.Status != catalog.StatusVerified || len(entry.Evidence) == 0 {
				t.Errorf("verified tool fixture row %s = status %q, evidence %d", entry.ID, entry.Status, len(entry.Evidence))
			}
			continue
		}
		switch entry.Status {
		case catalog.StatusInventoried, catalog.StatusScaffolded:
		case catalog.StatusPartial:
			if entry.Partial == nil || len(entry.Partial.Supported) == 0 || len(entry.Partial.Unsupported) == 0 {
				t.Errorf("partial matrix row %s has no explicit supported/unsupported split", entry.ID)
			}
			if len(entry.Evidence) == 0 {
				t.Errorf("partial matrix row %s has no achieved evidence", entry.ID)
			}
			continue
		default:
			t.Errorf("matrix row %s status = %q, want inventoried, scaffolded, or partial until field-level parity evidence exists", entry.ID, entry.Status)
		}
		if len(entry.Evidence) != 0 {
			t.Errorf("matrix row %s claims achieved evidence before a Chat Completions parity case exists", entry.ID)
		}
	}
}

func openAICompletionsFixtureCatalogIDs(t *testing.T, root, name, wantID string) map[string]bool {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, "parity", "oracle", "fixtures", name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	var fixture struct {
		ID             string   `json:"id"`
		CatalogIDs     []string `json:"catalog_ids"`
		BaselineCommit string   `json:"baseline_commit"`
		Deterministic  bool     `json:"deterministic"`
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatalf("decode %s: %v", name, err)
	}
	if fixture.ID != wantID || fixture.BaselineCommit != baselineCommit || !fixture.Deterministic {
		t.Fatalf("%s provenance is incomplete: %#v", name, fixture)
	}
	ids := make(map[string]bool, len(fixture.CatalogIDs))
	for _, id := range fixture.CatalogIDs {
		ids[id] = true
	}
	return ids
}

func TestOpenAICompletionsNoOpFixtureBindsVerifiedCatalogRows(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "parity", "oracle", "fixtures", "openai-completions-m0-no-op.json"))
	if err != nil {
		t.Fatalf("read no-op fixture: %v", err)
	}
	var fixture struct {
		ID             string   `json:"id"`
		CatalogIDs     []string `json:"catalog_ids"`
		BaselineCommit string   `json:"baseline_commit"`
		Deterministic  bool     `json:"deterministic"`
		Cases          []struct {
			ID          string          `json:"id"`
			CatalogID   string          `json:"catalog_id"`
			Input       json.RawMessage `json:"input"`
			InputSHA256 string          `json:"input_sha256"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatalf("decode no-op fixture: %v", err)
	}
	if fixture.ID != "ai/openai-completions/m0-no-op" || fixture.BaselineCommit != baselineCommit || !fixture.Deterministic {
		t.Fatalf("no-op fixture provenance is incomplete: %#v", fixture)
	}
	caseByCatalogID := make(map[string]struct{ ID, InputHash string }, len(fixture.Cases))
	for _, testCase := range fixture.Cases {
		var input any
		if err := json.Unmarshal(testCase.Input, &input); err != nil {
			t.Fatalf("decode fixture input %s: %v", testCase.ID, err)
		}
		canonical, err := json.Marshal(input)
		if err != nil {
			t.Fatalf("canonicalize fixture input %s: %v", testCase.ID, err)
		}
		digest := sha256.Sum256(canonical)
		gotHash := hex.EncodeToString(digest[:])
		if gotHash != testCase.InputSHA256 {
			t.Errorf("fixture case %s input_sha256 = %s, want %s", testCase.ID, testCase.InputSHA256, gotHash)
		}
		caseByCatalogID[testCase.CatalogID] = struct{ ID, InputHash string }{testCase.ID, testCase.InputSHA256}
	}

	entries, err := catalog.LoadCatalog(filepath.Join(root, "parity", "catalog.jsonl"))
	if err != nil {
		t.Fatalf("LoadCatalog(real) = %v", err)
	}
	for _, entry := range entries {
		if entry.Matrix == nil || entry.Matrix.API != "openai-completions" || entry.Matrix.PreTargetBehavior != catalog.MatrixBehaviorNoOp {
			continue
		}
		fixtureCase, ok := caseByCatalogID[entry.ID]
		if !ok {
			t.Errorf("fixture does not bind no-op catalog row %s", entry.ID)
			continue
		}
		for _, evidence := range entry.Evidence {
			if evidence.CaseID != fixtureCase.ID || evidence.InputHash != "sha256:"+fixtureCase.InputHash {
				t.Errorf("entry %s evidence does not match fixture case/hash", entry.ID)
			}
		}
	}
}

func TestOpenAICompletionsMatrixRecordsBidirectionalHistoryFields(t *testing.T) {
	want := map[string]string{
		"AssistantMessage.api":              catalog.MatrixDirectionBidirectional,
		"AssistantMessage.provider":         catalog.MatrixDirectionBidirectional,
		"AssistantMessage.model":            catalog.MatrixDirectionBidirectional,
		"AssistantMessage.stopReason":       catalog.MatrixDirectionBidirectional,
		"AssistantMessage.content":          catalog.MatrixDirectionBidirectional,
		"AssistantMessage.content.text":     catalog.MatrixDirectionBidirectional,
		"AssistantMessage.content.thinking": catalog.MatrixDirectionBidirectional,
		"AssistantMessage.content.toolCall": catalog.MatrixDirectionBidirectional,
		"Model.cost":                        catalog.MatrixDirectionBidirectional,
	}
	entries, err := catalog.LoadCatalog(filepath.Join(repoRoot(t), "parity", "catalog.jsonl"))
	if err != nil {
		t.Fatalf("LoadCatalog(real) = %v", err)
	}
	found := map[string]bool{}
	for _, entry := range entries {
		if entry.Matrix == nil || entry.Matrix.API != "openai-completions" {
			continue
		}
		wantDirection, ok := want[entry.Matrix.Pi.Name]
		if !ok {
			continue
		}
		found[entry.Matrix.Pi.Name] = true
		if entry.Matrix.Direction != wantDirection {
			t.Errorf("matrix row %s direction = %q, want %q", entry.ID, entry.Matrix.Direction, wantDirection)
		}
	}
	for name := range want {
		if !found[name] {
			t.Errorf("OpenAI Completions matrix missing direction check for %q", name)
		}
	}
}

func TestValidateRejections(t *testing.T) {
	for _, tt := range []struct {
		name     string
		mutate   func(entries []catalog.Entry, m *catalog.Manifest) []catalog.Entry
		sentinel error
		kind     catalog.ErrorKind
	}{
		{
			name: "duplicate id",
			mutate: func(entries []catalog.Entry, m *catalog.Manifest) []catalog.Entry {
				dup := entries[0]
				entries = append(entries, dup)
				m.EntryCount = len(entries)
				m.StatusCounts[dup.Status]++
				return entries
			},
			sentinel: catalog.ErrDuplicateID,
			kind:     catalog.KindDuplicateID,
		},
		{
			name: "illegal status",
			mutate: func(entries []catalog.Entry, m *catalog.Manifest) []catalog.Entry {
				delete(m.StatusCounts, entries[0].Status)
				entries[0].Status = "done"
				m.StatusCounts["done"]++
				return entries
			},
			sentinel: catalog.ErrIllegalStatus,
			kind:     catalog.KindIllegalStatus,
		},
		{
			name: "illegal milestone",
			mutate: func(entries []catalog.Entry, m *catalog.Manifest) []catalog.Entry {
				entries[0].Milestone = "M99"
				return entries
			},
			sentinel: catalog.ErrIllegalMilestone,
			kind:     catalog.KindIllegalMilestone,
		},
		{
			name: "missing upstream commit",
			mutate: func(entries []catalog.Entry, m *catalog.Manifest) []catalog.Entry {
				entries[0].Upstream.Commit = ""
				return entries
			},
			sentinel: catalog.ErrMissingProvenance,
			kind:     catalog.KindMissingProvenance,
		},
		{
			name: "missing upstream repository",
			mutate: func(entries []catalog.Entry, m *catalog.Manifest) []catalog.Entry {
				entries[0].Upstream.Repository = ""
				return entries
			},
			sentinel: catalog.ErrMissingProvenance,
			kind:     catalog.KindMissingProvenance,
		},
		{
			name: "missing upstream reference",
			mutate: func(entries []catalog.Entry, m *catalog.Manifest) []catalog.Entry {
				entries[0].Upstream.Reference = ""
				return entries
			},
			sentinel: catalog.ErrMissingProvenance,
			kind:     catalog.KindMissingProvenance,
		},
		{
			name: "missing mapping target",
			mutate: func(entries []catalog.Entry, m *catalog.Manifest) []catalog.Entry {
				entries[0].Mapping.Target = ""
				return entries
			},
			sentinel: catalog.ErrMissingProvenance,
			kind:     catalog.KindMissingProvenance,
		},
		{
			name: "missing upstream module",
			mutate: func(entries []catalog.Entry, m *catalog.Manifest) []catalog.Entry {
				entries[0].Upstream.Module = ""
				return entries
			},
			sentinel: catalog.ErrMissingProvenance,
			kind:     catalog.KindMissingProvenance,
		},
		{
			name: "missing mapping module",
			mutate: func(entries []catalog.Entry, m *catalog.Manifest) []catalog.Entry {
				entries[0].Mapping.Module = ""
				return entries
			},
			sentinel: catalog.ErrMissingProvenance,
			kind:     catalog.KindMissingProvenance,
		},
		{
			name: "commit mismatch",
			mutate: func(entries []catalog.Entry, m *catalog.Manifest) []catalog.Entry {
				entries[0].Upstream.Commit = "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef"
				return entries
			},
			sentinel: catalog.ErrCommitMismatch,
			kind:     catalog.KindCommitMismatch,
		},
		{
			name: "deferred without adr",
			mutate: func(entries []catalog.Entry, m *catalog.Manifest) []catalog.Entry {
				entries[5].Deferred.ADR = ""
				return entries
			},
			sentinel: catalog.ErrDeferredWithoutADR,
			kind:     catalog.KindDeferredWithoutADR,
		},
		{
			name: "deferred without milestone",
			mutate: func(entries []catalog.Entry, m *catalog.Manifest) []catalog.Entry {
				entries[5].Deferred.Milestone = ""
				return entries
			},
			sentinel: catalog.ErrDeferredWithoutADR,
			kind:     catalog.KindDeferredWithoutADR,
		},
		{
			name: "deferred without reason",
			mutate: func(entries []catalog.Entry, m *catalog.Manifest) []catalog.Entry {
				entries[5].Deferred.Reason = ""
				return entries
			},
			sentinel: catalog.ErrDeferredWithoutADR,
			kind:     catalog.KindDeferredWithoutADR,
		},
		{
			name: "deferred without block",
			mutate: func(entries []catalog.Entry, m *catalog.Manifest) []catalog.Entry {
				entries[5].Deferred = nil
				return entries
			},
			sentinel: catalog.ErrDeferredWithoutADR,
			kind:     catalog.KindDeferredWithoutADR,
		},
		{
			name: "partial empty supported",
			mutate: func(entries []catalog.Entry, m *catalog.Manifest) []catalog.Entry {
				entries[2].Partial.Supported = nil
				return entries
			},
			sentinel: catalog.ErrIncompletePartial,
			kind:     catalog.KindIncompletePartial,
		},
		{
			name: "partial empty unsupported",
			mutate: func(entries []catalog.Entry, m *catalog.Manifest) []catalog.Entry {
				entries[2].Partial.Unsupported = nil
				return entries
			},
			sentinel: catalog.ErrIncompletePartial,
			kind:     catalog.KindIncompletePartial,
		},
		{
			name: "partial nil block",
			mutate: func(entries []catalog.Entry, m *catalog.Manifest) []catalog.Entry {
				entries[2].Partial = nil
				return entries
			},
			sentinel: catalog.ErrIncompletePartial,
			kind:     catalog.KindIncompletePartial,
		},
		{
			name: "deviation without reason",
			mutate: func(entries []catalog.Entry, m *catalog.Manifest) []catalog.Entry {
				entries[0].Deviation = &catalog.Deviation{ADR: "ADR-0002"}
				return entries
			},
			sentinel: catalog.ErrMissingProvenance,
			kind:     catalog.KindMissingProvenance,
		},
		{
			name: "implemented without evidence",
			mutate: func(entries []catalog.Entry, m *catalog.Manifest) []catalog.Entry {
				entries[3].Evidence = nil
				return entries
			},
			sentinel: catalog.ErrMissingEvidence,
			kind:     catalog.KindMissingEvidence,
		},
		{
			name: "verified without evidence",
			mutate: func(entries []catalog.Entry, m *catalog.Manifest) []catalog.Entry {
				entries[4].Evidence = nil
				return entries
			},
			sentinel: catalog.ErrMissingEvidence,
			kind:     catalog.KindMissingEvidence,
		},
		{
			name: "incomplete achieved evidence",
			mutate: func(entries []catalog.Entry, m *catalog.Manifest) []catalog.Entry {
				entries[3].Evidence[0].Ref = ""
				return entries
			},
			sentinel: catalog.ErrMissingEvidence,
			kind:     catalog.KindMissingEvidence,
		},
		{
			name: "schema version missing",
			mutate: func(entries []catalog.Entry, m *catalog.Manifest) []catalog.Entry {
				entries[0].SchemaVersion = ""
				return entries
			},
			sentinel: catalog.ErrSchemaVersion,
			kind:     catalog.KindSchemaVersion,
		},
		{
			name: "entry schema version mismatch",
			mutate: func(entries []catalog.Entry, m *catalog.Manifest) []catalog.Entry {
				entries[0].SchemaVersion = "1.0.0"
				return entries
			},
			sentinel: catalog.ErrSchemaVersion,
			kind:     catalog.KindSchemaVersion,
		},
		{
			name: "catalog schema version mismatch",
			mutate: func(entries []catalog.Entry, m *catalog.Manifest) []catalog.Entry {
				m.CatalogVersion = "1.0.0"
				return entries
			},
			sentinel: catalog.ErrSchemaVersion,
			kind:     catalog.KindSchemaVersion,
		},
		{
			name: "catalog snapshot manifest missing",
			mutate: func(entries []catalog.Entry, m *catalog.Manifest) []catalog.Entry {
				m.CatalogSnapshotManifest = ""
				return entries
			},
			sentinel: catalog.ErrManifestMismatch,
			kind:     catalog.KindManifestMismatch,
		},
		{
			name: "catalog baseline has no entry",
			mutate: func(entries []catalog.Entry, m *catalog.Manifest) []catalog.Entry {
				m.CatalogBaselineCommit = catalogBaselineCommit
				return entries
			},
			sentinel: catalog.ErrManifestMismatch,
			kind:     catalog.KindManifestMismatch,
		},
		{
			name: "invalid baseline role",
			mutate: func(entries []catalog.Entry, m *catalog.Manifest) []catalog.Entry {
				entries[0].BaselineRole = "mixed"
				return entries
			},
			sentinel: catalog.ErrCommitMismatch,
			kind:     catalog.KindCommitMismatch,
		},
		{
			name: "entry count mismatch",
			mutate: func(entries []catalog.Entry, m *catalog.Manifest) []catalog.Entry {
				m.EntryCount = len(entries) + 1
				return entries
			},
			sentinel: catalog.ErrManifestMismatch,
			kind:     catalog.KindManifestMismatch,
		},
		{
			name: "status counts mismatch",
			mutate: func(entries []catalog.Entry, m *catalog.Manifest) []catalog.Entry {
				m.StatusCounts[entries[0].Status]++
				return entries
			},
			sentinel: catalog.ErrManifestMismatch,
			kind:     catalog.KindManifestMismatch,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			entries, manifest := base()
			entries = tt.mutate(entries, &manifest)
			err := catalog.Validate(entries, manifest)
			if err == nil {
				t.Fatalf("Validate(%s) = nil, want error", tt.name)
			}
			if !errors.Is(err, tt.sentinel) {
				t.Fatalf("errors.Is(%v, %v) = false", err, tt.sentinel)
			}
			var verr *catalog.ValidationError
			if !errors.As(err, &verr) {
				t.Fatalf("errors.As(%v, *ValidationError) = false", err)
			}
			if verr.Kind != tt.kind {
				t.Fatalf("ValidationError.Kind = %q, want %q", verr.Kind, tt.kind)
			}
		})
	}
}

func TestLoadCatalogMalformedLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "catalog.jsonl")
	good := `{"schema_version":"1.0.0","id":"a","upstream":{"module":"ai","repository":"r","commit":"c","reference":"ref"},"mapping":{"module":"ai","target":"t","kind":"package"},"status":"scaffolded","milestone":"M1","classification":"public-api"}`
	content := good + "\n" + "{not valid json" + "\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := catalog.LoadCatalog(path)
	if err == nil {
		t.Fatal("LoadCatalog(malformed) = nil, want error")
	}
	if !errors.Is(err, catalog.ErrMalformedLine) {
		t.Fatalf("errors.Is(%v, ErrMalformedLine) = false", err)
	}
	var perr *catalog.ParseError
	if !errors.As(err, &perr) {
		t.Fatalf("errors.As(%v, *ParseError) = false", err)
	}
	if perr.Line != 2 {
		t.Fatalf("ParseError.Line = %d, want 2", perr.Line)
	}
	if !strings.Contains(err.Error(), "2") {
		t.Fatalf("error text %q does not name the line", err.Error())
	}
}

func TestLoadAndValidateRealCatalog(t *testing.T) {
	root := repoRoot(t)
	entries, err := catalog.LoadCatalog(filepath.Join(root, "parity", "catalog.jsonl"))
	if err != nil {
		t.Fatalf("LoadCatalog(real) = %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("real catalog has no entries")
	}

	// The canonical JSONL must be sorted by id for determinism.
	ids := make([]string, len(entries))
	for i, e := range entries {
		ids[i] = e.ID
	}
	if !sort.StringsAreSorted(ids) {
		t.Fatalf("real catalog ids not sorted: %v", ids)
	}

	manifest := loadManifest(t, filepath.Join(root, "parity", "catalog.manifest.json"))
	if err := catalog.Validate(entries, manifest); err != nil {
		t.Fatalf("Validate(real catalog) = %v", err)
	}
	if manifest.BaselineCommit != baselineCommit {
		t.Fatalf("manifest baseline = %q, want %q", manifest.BaselineCommit, baselineCommit)
	}
}

func TestGenerateReportBanner(t *testing.T) {
	entries, _ := base()
	report := catalog.GenerateReport(entries)
	if !strings.Contains(report, catalog.NonAuthoritativeBanner) {
		t.Fatalf("report missing non-authoritative banner:\n%s", report)
	}
	if !strings.Contains(strings.ToLower(report), "non-authoritative") {
		t.Fatal("report does not describe itself as non-authoritative")
	}
	if !strings.Contains(report, "catalog.jsonl") {
		t.Fatal("report does not point at parity/catalog.jsonl")
	}
	// Summary must include a count for at least one status.
	if !strings.Contains(report, catalog.StatusScaffolded) {
		t.Fatal("report summary missing status rows")
	}
}

func TestGenerateReportGoldenMatchesCommitted(t *testing.T) {
	root := repoRoot(t)
	entries, err := catalog.LoadCatalog(filepath.Join(root, "parity", "catalog.jsonl"))
	if err != nil {
		t.Fatalf("LoadCatalog(real) = %v", err)
	}
	got := catalog.GenerateReport(entries)
	goldenPath := filepath.Join(root, "parity", "reports", "catalog.md")
	if *update {
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0o755); err != nil {
			t.Fatalf("mkdir report dir: %v", err)
		}
		if err := os.WriteFile(goldenPath, []byte(got), 0o644); err != nil {
			t.Fatalf("write golden report: %v", err)
		}
		t.Logf("regenerated %s (%d bytes)", goldenPath, len(got))
		return
	}
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden report: %v", err)
	}
	if got != string(want) {
		t.Fatalf("generated report differs from committed golden %s.\n"+
			"Regenerate with the report generator and commit the result.\n"+
			"--- got (%d bytes) ---\n%s\n--- want (%d bytes) ---\n%s",
			goldenPath, len(got), got, len(want), string(want))
	}
}

// loadManifest reads and decodes the manifest JSON for tests without depending
// on package-private helpers.
func loadManifest(t *testing.T, path string) catalog.Manifest {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	m, err := catalog.ParseManifest(data)
	if err != nil {
		t.Fatalf("parse manifest: %v", err)
	}
	return m
}

// TestGenerateCatalog validates the authoritative catalog and regenerates its
// derived manifest and report when run with -update (go test
// ./internal/catalog -run GenerateCatalog -update). Without -update it is a
// byte-for-byte consistency check for those two generated artifacts.
func TestGenerateCatalog(t *testing.T) {
	root := repoRoot(t)
	parity := filepath.Join(root, "parity")

	catalogPath := filepath.Join(parity, "catalog.jsonl")
	entries, err := catalog.LoadCatalog(catalogPath)
	if err != nil {
		t.Fatalf("LoadCatalog: %v", err)
	}
	manifest := catalog.BuildManifest(entries, baselineCommit, catalog.DefaultManifestPaths)

	// The regenerated catalog must satisfy the validator before it is written.
	if err := catalog.Validate(entries, manifest); err != nil {
		t.Fatalf("regenerated catalog fails Validate: %v", err)
	}

	manifestData, err := catalog.EncodeManifest(manifest)
	if err != nil {
		t.Fatalf("EncodeManifest: %v", err)
	}
	report := catalog.GenerateReport(entries)

	manifestPath := filepath.Join(parity, "catalog.manifest.json")
	reportPath := filepath.Join(parity, "reports", "catalog.md")

	if *update {
		if err := os.MkdirAll(filepath.Dir(reportPath), 0o755); err != nil {
			t.Fatalf("mkdir report dir: %v", err)
		}
		writes := []struct {
			path string
			data []byte
		}{
			{manifestPath, manifestData},
			{reportPath, []byte(report)},
		}
		for _, w := range writes {
			if err := os.WriteFile(w.path, w.data, 0o644); err != nil {
				t.Fatalf("write %s: %v", w.path, err)
			}
		}
		t.Logf("regenerated catalog: %d entries", len(entries))
		return
	}

	// Live consistency: committed derived artifacts must equal the output from
	// the authoritative catalog byte-for-byte.
	for _, w := range []struct {
		name string
		path string
		want []byte
	}{
		{"catalog.manifest.json", manifestPath, manifestData},
		{"reports/catalog.md", reportPath, []byte(report)},
	} {
		got, err := os.ReadFile(w.path)
		if err != nil {
			t.Fatalf("read %s: %v", w.name, err)
		}
		if !bytes.Equal(got, w.want) {
			t.Fatalf("committed %s differs from catalog-derived output.\n"+
				"Regenerate with: go test ./internal/catalog -run GenerateCatalog -update\n"+
				"--- committed (%d bytes) ---\n%s\n--- regenerated (%d bytes) ---\n%s",
				w.name, len(got), string(got), len(w.want), string(w.want))
		}
	}
}

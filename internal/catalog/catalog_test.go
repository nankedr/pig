package catalog_test

import (
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

// updateGolden regenerates parity/reports/catalog.md from the committed catalog
// when set (go test ./internal/catalog/ -run Golden -update). The report is a
// generated artifact, so this keeps it in sync via the generator rather than by
// hand-editing.
var updateGolden = flag.Bool("update", false, "regenerate the committed catalog report golden")

const baselineCommit = "936aff00918de1187f085f123c2812d8f2d67745"

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
			Evidence:       []catalog.Evidence{{Kind: "oracle", Ref: "parity/cases/client", Baseline: baselineCommit}},
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
		SchemaVersion:   catalog.SchemaVersion,
		CatalogVersion:  "1.0.0",
		BaselineCommit:  baselineCommit,
		Catalog:         "catalog.jsonl",
		Schema:          "catalog.schema.json",
		GeneratedReport: "reports/catalog.md",
		EntryCount:      len(entries),
		StatusCounts:    counts,
	}
	return entries, manifest
}

func TestValidateAcceptsSyntheticBaseline(t *testing.T) {
	entries, manifest := base()
	if err := catalog.Validate(entries, manifest); err != nil {
		t.Fatalf("Validate(valid baseline) = %v", err)
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
			name: "schema version missing",
			mutate: func(entries []catalog.Entry, m *catalog.Manifest) []catalog.Entry {
				entries[0].SchemaVersion = ""
				return entries
			},
			sentinel: catalog.ErrSchemaVersion,
			kind:     catalog.KindSchemaVersion,
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
	if *updateGolden {
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

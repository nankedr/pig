package inventory_test

import (
	"errors"
	"flag"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"

	"github.com/nankedr/pig/internal/baseline"
	"github.com/nankedr/pig/internal/catalog"
	"github.com/nankedr/pig/internal/inventory"
)

// updateSnapshot regenerates parity/inventory/files.jsonl and manifest.json from
// a Pi checkout when set (go test ./internal/inventory -run Generate -update).
// The snapshot is a generated artifact, so this keeps it in sync via the walker
// rather than by hand-editing. Regeneration needs the checkout, so it also
// requires PIG_INVENTORY_DRIFT=1 and an available checkout.
var updateSnapshot = flag.Bool("update", false, "regenerate the committed inventory snapshot from the Pi checkout")

const baselineCommit = "936aff00918de1187f085f123c2812d8f2d67745"

// repoRoot locates the repository root relative to this test file, mirroring
// the runtime.Caller approach used by internal/catalog and internal/capability.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

// realCatalogIDs loads the committed Parity Catalog and returns the set of
// entry ids, so inventory validation resolves owning_catalog_id against the
// real authority rather than a hand-maintained list.
func realCatalogIDs(t *testing.T, root string) map[string]bool {
	t.Helper()
	entries, err := catalog.LoadCatalog(filepath.Join(root, "parity", "catalog.jsonl"))
	if err != nil {
		t.Fatalf("LoadCatalog: %v", err)
	}
	ids := make(map[string]bool, len(entries))
	for _, e := range entries {
		ids[e.ID] = true
	}
	return ids
}

// baseRecords returns a small, fully valid synthetic snapshot: one file per
// in-scope module plus the two command entries. Rejection tests mutate a fresh
// copy so cases stay independent.
func baseRecords() ([]inventory.Record, inventory.Manifest, map[string]bool) {
	records := []inventory.Record{
		{Path: "packages/agent/src/index.ts", SHA256: "a1", Module: "agent", Classification: inventory.ClassPublicAPI, OwningCatalog: "module-agent"},
		{Path: "packages/ai/src/index.ts", SHA256: "a2", Module: "ai", Classification: inventory.ClassPublicAPI, OwningCatalog: "module-ai"},
		{Path: "packages/ai/src/cli.ts", SHA256: "a3", Module: "ai", Classification: inventory.ClassDirectEntry, OwningCatalog: "cmd-pig-ai"},
		{Path: "packages/client/src/index.ts", SHA256: "a4", Module: "client", Classification: inventory.ClassPublicAPI, OwningCatalog: "module-client"},
		{Path: "packages/coding-agent/src/index.ts", SHA256: "a5", Module: "codingagent", Classification: inventory.ClassPublicAPI, OwningCatalog: "module-codingagent"},
		{Path: "packages/coding-agent/src/cli.ts", SHA256: "a6", Module: "codingagent", Classification: inventory.ClassDirectEntry, OwningCatalog: "cmd-pig"},
		{Path: "packages/protocol/src/index.ts", SHA256: "a7", Module: "protocol", Classification: inventory.ClassPublicAPI, OwningCatalog: "module-protocol"},
		{Path: "packages/telemetry/src/index.ts", SHA256: "a8", Module: "telemetry", Classification: inventory.ClassPublicAPI, OwningCatalog: "module-telemetry"},
		{Path: "packages/tui/src/index.ts", SHA256: "a9", Module: "tui", Classification: inventory.ClassPublicAPI, OwningCatalog: "module-tui"},
		{Path: "packages/tui/test/layout.test.ts", SHA256: "a10", Module: "tui", Classification: inventory.ClassDormantTestSupport, OwningCatalog: "module-tui"},
		{Path: "packages/tui/native/darwin/src/darwin-modifiers.c", SHA256: "a11", Module: "tui", Classification: inventory.ClassPrivateImpl, OwningCatalog: "module-tui"},
	}
	manifest := inventory.BuildManifest(records, baselineCommit)
	ids := map[string]bool{
		"module-agent": true, "module-ai": true, "module-client": true,
		"module-codingagent": true, "module-protocol": true, "module-telemetry": true,
		"module-tui": true, "cmd-pig": true, "cmd-pig-ai": true,
	}
	return records, manifest, ids
}

func TestValidateAcceptsSyntheticSnapshot(t *testing.T) {
	records, manifest, ids := baseRecords()
	if err := inventory.Validate(records, manifest, ids); err != nil {
		t.Fatalf("Validate(valid snapshot) = %v", err)
	}
}

func TestValidateRejections(t *testing.T) {
	for _, tt := range []struct {
		name     string
		mutate   func(records []inventory.Record, m *inventory.Manifest, ids map[string]bool) []inventory.Record
		sentinel error
		kind     inventory.Kind
	}{
		{
			name: "duplicate path",
			mutate: func(records []inventory.Record, m *inventory.Manifest, ids map[string]bool) []inventory.Record {
				dup := records[0]
				records = append(records, dup)
				*m = inventory.BuildManifest(records, baselineCommit)
				return records
			},
			sentinel: inventory.ErrDuplicatePath,
			kind:     inventory.KindDuplicatePath,
		},
		{
			name: "missing sha256",
			mutate: func(records []inventory.Record, m *inventory.Manifest, ids map[string]bool) []inventory.Record {
				records[0].SHA256 = ""
				return records
			},
			sentinel: inventory.ErrMissingField,
			kind:     inventory.KindMissingField,
		},
		{
			name: "illegal classification",
			mutate: func(records []inventory.Record, m *inventory.Manifest, ids map[string]bool) []inventory.Record {
				records[0].Classification = "misc"
				m.ClassificationCount = inventory.BuildManifest(records, baselineCommit).ClassificationCount
				return records
			},
			sentinel: inventory.ErrIllegalClass,
			kind:     inventory.KindIllegalClass,
		},
		{
			name: "unknown module",
			mutate: func(records []inventory.Record, m *inventory.Manifest, ids map[string]bool) []inventory.Record {
				records[0].Module = "server"
				m.ModuleCounts = inventory.BuildManifest(records, baselineCommit).ModuleCounts
				return records
			},
			sentinel: inventory.ErrUnknownModule,
			kind:     inventory.KindUnknownModule,
		},
		{
			name: "unmapped owning catalog id",
			mutate: func(records []inventory.Record, m *inventory.Manifest, ids map[string]bool) []inventory.Record {
				records[0].OwningCatalog = "module-nonexistent"
				return records
			},
			sentinel: inventory.ErrUnmapped,
			kind:     inventory.KindUnmapped,
		},
		{
			name: "empty owning catalog id",
			mutate: func(records []inventory.Record, m *inventory.Manifest, ids map[string]bool) []inventory.Record {
				records[0].OwningCatalog = ""
				return records
			},
			sentinel: inventory.ErrMissingField,
			kind:     inventory.KindMissingField,
		},
		{
			name: "module uncovered",
			mutate: func(records []inventory.Record, m *inventory.Manifest, ids map[string]bool) []inventory.Record {
				// Drop every protocol file, leaving that in-scope module empty.
				kept := records[:0:0]
				for _, r := range records {
					if r.Module != "protocol" {
						kept = append(kept, r)
					}
				}
				*m = inventory.BuildManifest(kept, baselineCommit)
				return kept
			},
			sentinel: inventory.ErrModuleUncovered,
			kind:     inventory.KindModuleUncovered,
		},
		{
			name: "file count mismatch",
			mutate: func(records []inventory.Record, m *inventory.Manifest, ids map[string]bool) []inventory.Record {
				m.FileCount = len(records) + 1
				return records
			},
			sentinel: inventory.ErrManifestMismatch,
			kind:     inventory.KindManifestMismatch,
		},
		{
			name: "module count mismatch",
			mutate: func(records []inventory.Record, m *inventory.Manifest, ids map[string]bool) []inventory.Record {
				m.ModuleCounts["ai"]++
				return records
			},
			sentinel: inventory.ErrManifestMismatch,
			kind:     inventory.KindManifestMismatch,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			records, manifest, ids := baseRecords()
			records = tt.mutate(records, &manifest, ids)
			err := inventory.Validate(records, manifest, ids)
			if err == nil {
				t.Fatalf("Validate(%s) = nil, want error", tt.name)
			}
			if !errors.Is(err, tt.sentinel) {
				t.Fatalf("errors.Is(%v, %v) = false", err, tt.sentinel)
			}
			var ierr *inventory.Error
			if !errors.As(err, &ierr) {
				t.Fatalf("errors.As(%v, *inventory.Error) = false", err)
			}
			if ierr.Kind != tt.kind {
				t.Fatalf("Error.Kind = %q, want %q", ierr.Kind, tt.kind)
			}
		})
	}
}

// TestDiffDetectsCoverageChanges is the pure-Go, offline demonstration that the
// coverage check fails on an added, removed or changed artifact.
func TestDiffDetectsCoverageChanges(t *testing.T) {
	committed, _, _ := baseRecords()

	t.Run("identical snapshots agree", func(t *testing.T) {
		fresh := append([]inventory.Record(nil), committed...)
		if d := inventory.Diff(committed, fresh); !d.Empty() {
			t.Fatalf("Diff(identical) = %+v, want empty", d)
		}
	})

	t.Run("added file fails coverage", func(t *testing.T) {
		fresh := append([]inventory.Record(nil), committed...)
		fresh = append(fresh, inventory.Record{
			Path: "packages/ai/src/brand-new.ts", SHA256: "zz", Module: "ai",
			Classification: inventory.ClassPrivateImpl, OwningCatalog: "module-ai",
		})
		d := inventory.Diff(committed, fresh)
		if d.Empty() {
			t.Fatal("Diff(added) = empty, want an added path")
		}
		if len(d.Added) != 1 || d.Added[0] != "packages/ai/src/brand-new.ts" {
			t.Fatalf("Diff(added).Added = %v", d.Added)
		}
	})

	t.Run("removed file fails coverage", func(t *testing.T) {
		fresh := append([]inventory.Record(nil), committed[1:]...)
		d := inventory.Diff(committed, fresh)
		if len(d.Removed) != 1 || d.Removed[0] != committed[0].Path {
			t.Fatalf("Diff(removed).Removed = %v", d.Removed)
		}
	})

	t.Run("changed content fails coverage", func(t *testing.T) {
		fresh := append([]inventory.Record(nil), committed...)
		fresh[0].SHA256 = "changed"
		d := inventory.Diff(committed, fresh)
		if len(d.Changed) != 1 || d.Changed[0] != committed[0].Path {
			t.Fatalf("Diff(changed).Changed = %v", d.Changed)
		}
	})

	t.Run("changed classification fails coverage", func(t *testing.T) {
		fresh := append([]inventory.Record(nil), committed...)
		fresh[0].Classification = inventory.ClassDormantTestSupport
		d := inventory.Diff(committed, fresh)
		if len(d.Changed) != 1 {
			t.Fatalf("Diff(reclassified).Changed = %v", d.Changed)
		}
	})
}

func TestEncodeFilesRoundTrip(t *testing.T) {
	records, _, _ := baseRecords()
	data, err := inventory.EncodeFiles(records)
	if err != nil {
		t.Fatalf("EncodeFiles: %v", err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "files.jsonl")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := inventory.LoadFiles(path)
	if err != nil {
		t.Fatalf("LoadFiles: %v", err)
	}
	if len(got) != len(records) {
		t.Fatalf("round-trip len = %d, want %d", len(got), len(records))
	}
	// EncodeFiles sorts by path; verify the on-disk order is sorted.
	paths := make([]string, len(got))
	for i, r := range got {
		paths[i] = r.Path
	}
	if !sort.StringsAreSorted(paths) {
		t.Fatalf("encoded files not sorted by path: %v", paths)
	}
}

func TestLoadFilesMalformedLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "files.jsonl")
	good := `{"path":"packages/ai/src/index.ts","sha256":"a","module":"ai","classification":"public-api","owning_catalog_id":"module-ai"}`
	content := good + "\n" + "{not valid json\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := inventory.LoadFiles(path)
	if err == nil {
		t.Fatal("LoadFiles(malformed) = nil, want error")
	}
	if !errors.Is(err, inventory.ErrMalformedLine) {
		t.Fatalf("errors.Is(%v, ErrMalformedLine) = false", err)
	}
	var perr *inventory.ParseError
	if !errors.As(err, &perr) {
		t.Fatalf("errors.As(%v, *ParseError) = false", err)
	}
	if perr.Line != 2 {
		t.Fatalf("ParseError.Line = %d, want 2", perr.Line)
	}
}

// TestLoadAndValidateRealInventory is the offline consistency check that runs in
// normal `go test`: the committed files.jsonl and manifest.json must load,
// validate against the real Parity Catalog ids, cover every in-scope module and
// carry the locked baseline commit.
func TestLoadAndValidateRealInventory(t *testing.T) {
	root := repoRoot(t)
	records, err := inventory.LoadFiles(filepath.Join(root, "parity", "inventory", "files.jsonl"))
	if err != nil {
		t.Fatalf("LoadFiles(real) = %v", err)
	}
	if len(records) == 0 {
		t.Fatal("real inventory has no records")
	}

	// The canonical JSONL must be sorted by path for determinism.
	paths := make([]string, len(records))
	for i, r := range records {
		paths[i] = r.Path
	}
	if !sort.StringsAreSorted(paths) {
		t.Fatal("real inventory not sorted by path")
	}

	manifest, err := inventory.LoadManifest(filepath.Join(root, "parity", "inventory", "manifest.json"))
	if err != nil {
		t.Fatalf("LoadManifest(real) = %v", err)
	}
	if manifest.BaselineCommit != baselineCommit {
		t.Fatalf("manifest baseline = %q, want %q", manifest.BaselineCommit, baselineCommit)
	}

	ids := realCatalogIDs(t, root)
	if err := inventory.Validate(records, manifest, ids); err != nil {
		t.Fatalf("Validate(real inventory) = %v", err)
	}
}

// TestInventoryDriftAgainstUpstream is the opt-in, pure-Go drift check. It is
// skipped unless PIG_INVENTORY_DRIFT=1 and a Pi checkout is available (default
// .upstream/pi, overridable via PIG_PI_CHECKOUT). It verifies the checkout is at
// the locked commit, re-walks it, and asserts the committed snapshot matches
// exactly — no added, removed or changed artifact. It performs the git HEAD
// check via internal/baseline and never runs under normal `go test`.
func TestInventoryDriftAgainstUpstream(t *testing.T) {
	if os.Getenv("PIG_INVENTORY_DRIFT") != "1" {
		t.Skip("set PIG_INVENTORY_DRIFT=1 to run the on-demand upstream drift check")
	}
	root := repoRoot(t)
	checkout := os.Getenv("PIG_PI_CHECKOUT")
	if checkout == "" {
		checkout = filepath.Join(root, ".upstream", "pi")
	}
	if _, err := os.Stat(checkout); err != nil {
		t.Skipf("Pi checkout unavailable at %s: %v", checkout, err)
	}

	// The drift walk must only run against a checkout that matches the lock.
	if err := baseline.Verify(filepath.Join(root, "parity", "baseline"), baseline.WithCheckout(checkout)); err != nil {
		t.Fatalf("baseline.Verify(WithCheckout) = %v", err)
	}

	fresh, err := inventory.Walk(checkout)
	if err != nil {
		t.Fatalf("Walk(%s) = %v", checkout, err)
	}
	committed, err := inventory.LoadFiles(filepath.Join(root, "parity", "inventory", "files.jsonl"))
	if err != nil {
		t.Fatalf("LoadFiles(committed) = %v", err)
	}
	if d := inventory.Diff(committed, fresh); !d.Empty() {
		t.Fatalf("inventory drift: added=%v removed=%v changed=%v", d.Added, d.Removed, d.Changed)
	}
}

// TestGenerateSnapshot regenerates the committed inventory artifacts from a Pi
// checkout when run with -update (go test ./internal/inventory -run Generate
// -update). It walks the checkout, then writes files.jsonl and manifest.json.
// Without -update it is a no-op skip, so it never runs during normal `go test`.
func TestGenerateSnapshot(t *testing.T) {
	if !*updateSnapshot {
		t.Skip("set -update (with a Pi checkout) to regenerate the inventory snapshot")
	}
	root := repoRoot(t)
	checkout := os.Getenv("PIG_PI_CHECKOUT")
	if checkout == "" {
		checkout = filepath.Join(root, ".upstream", "pi")
	}
	if _, err := os.Stat(checkout); err != nil {
		t.Fatalf("Pi checkout required for -update, unavailable at %s: %v", checkout, err)
	}
	if err := baseline.Verify(filepath.Join(root, "parity", "baseline"), baseline.WithCheckout(checkout)); err != nil {
		t.Fatalf("baseline.Verify(WithCheckout) = %v", err)
	}

	records, err := inventory.Walk(checkout)
	if err != nil {
		t.Fatalf("Walk(%s) = %v", checkout, err)
	}
	manifest := inventory.BuildManifest(records, baselineCommit)

	// Regenerated artifacts must satisfy the same validator as the committed
	// ones, resolving owning ids against the real catalog.
	if err := inventory.Validate(records, manifest, realCatalogIDs(t, root)); err != nil {
		t.Fatalf("regenerated snapshot fails Validate: %v", err)
	}

	filesData, err := inventory.EncodeFiles(records)
	if err != nil {
		t.Fatalf("EncodeFiles: %v", err)
	}
	manifestData, err := inventory.EncodeManifest(manifest)
	if err != nil {
		t.Fatalf("EncodeManifest: %v", err)
	}
	dir := filepath.Join(root, "parity", "inventory")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir inventory dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "files.jsonl"), filesData, 0o644); err != nil {
		t.Fatalf("write files.jsonl: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), manifestData, 0o644); err != nil {
		t.Fatalf("write manifest.json: %v", err)
	}
	t.Logf("regenerated inventory snapshot: %d files", len(records))
}

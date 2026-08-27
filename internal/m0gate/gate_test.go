package m0gate_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nankedr/pig/internal/baseline"
	"github.com/nankedr/pig/internal/catalog"
	"github.com/nankedr/pig/internal/inventory"
	"github.com/nankedr/pig/internal/surface"
)

const (
	catalogSnapshotCatalogID = "contract:baseline/catalog-snapshot"
	imageSnapshotCatalogID   = "contract:baseline/image-catalog-snapshot"
	codeBaselineCommit       = "936aff00918de1187f085f123c2812d8f2d67745"
	catalogBaselineCommit    = "53fa77ccd8a279eb87e92294ef3687b03ff80112"
	catalogReleaseSHA256     = "294d8067eb42327be0db4792d3be792daff588d8fc22549270a972ec9e5407e7"
	catalogManifestSHA256    = "ed1960291fe50bf7ba300bfea19ca9cefcc4b53001b367469a5827523b286ba5"
	catalogStructureHash     = "24c74ac10bb8ed4df2c96bdadcfd94a417f3c823d5038875f59a261e3c84424b"
	catalogResultSHA256      = "1268addeb40f0eb1b1b4a4229c1df59206b5e8bf5a55fdb627af58bc422a5583"
	imageSourceSHA256        = "76ffa23d2fbd3fa7505a979af972c8be38ee1be16cdcd3b9512411efb476f3a2"
	imageGeneratorSHA256     = "735c4a5901932eb7e2e0d434ca99cbca9a6437c93ffed06d486e60a3b6b7aaef"
	imageResultSHA256        = "fba8b88a29f861c0e0ab23960af7370e095198ca0bdd88c2721495cb36b50142"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func TestM0CatalogSnapshotIsCaptured(t *testing.T) {
	root := repoRoot(t)
	dir := filepath.Join(root, "parity", "baseline")
	if err := baseline.Verify(dir); err != nil {
		t.Fatalf("baseline.Verify: %v", err)
	}
	lock, manifest, err := baseline.Load(dir)
	if err != nil {
		t.Fatalf("baseline.Load: %v", err)
	}
	if manifest.Status != baseline.StatusCaptured {
		t.Fatalf("Catalog Snapshot status = %q, want %q", manifest.Status, baseline.StatusCaptured)
	}
	if lock.Upstream.Commit != codeBaselineCommit || manifest.BaselineCommit != codeBaselineCommit {
		t.Fatalf("code baseline = %q/%q, want %q", lock.Upstream.Commit, manifest.BaselineCommit, codeBaselineCommit)
	}
	if manifest.CatalogSource.Commit != catalogBaselineCommit || manifest.CatalogSource.Release != "v0.84.1" ||
		manifest.CatalogSource.SHA256 != catalogReleaseSHA256 || manifest.CatalogSource.CommitsBehindCodeBaseline != 40 {
		t.Fatalf("catalog baseline provenance = %+v", manifest.CatalogSource)
	}
	if manifest.CatalogSource.ManifestSHA256 != catalogManifestSHA256 || manifest.CatalogSource.StructureSHA256 != catalogStructureHash ||
		manifest.Derivation.ResultSHA256 != catalogResultSHA256 {
		t.Fatalf("catalog baseline hashes = manifest %q, structure %q, result %q",
			manifest.CatalogSource.ManifestSHA256, manifest.CatalogSource.StructureSHA256, manifest.Derivation.ResultSHA256)
	}
	if manifest.Providers != 39 || manifest.Models != 1220 {
		t.Fatalf("chat catalog counts = %d/%d, want 39/1220", manifest.Providers, manifest.Models)
	}
	if manifest.Image.Providers != 1 || manifest.Image.Models != 42 || manifest.Image.SourceCommit != codeBaselineCommit {
		t.Fatalf("image catalog provenance/counts = %+v, want code baseline with 1/42", manifest.Image)
	}
	if manifest.Image.SourceSHA256 != imageSourceSHA256 || manifest.Image.GeneratorSHA256 != imageGeneratorSHA256 {
		t.Fatalf("image source hashes = %q/%q", manifest.Image.SourceSHA256, manifest.Image.GeneratorSHA256)
	}
	if manifest.Derivation.SourceCommit != catalogBaselineCommit || manifest.Derivation.SemanticOverlays != 0 {
		t.Fatalf("catalog derivation = %+v, want lossless v0.84.1 with no semantic overlay", manifest.Derivation)
	}
	artifactHashes := make(map[string]string, len(manifest.Artifacts))
	for _, artifact := range manifest.Artifacts {
		artifactHashes[artifact.Path] = artifact.SHA256
	}
	if artifactHashes[manifest.Image.Artifact] != imageResultSHA256 {
		t.Fatalf("image artifact hash = %q, want %q", artifactHashes[manifest.Image.Artifact], imageResultSHA256)
	}

	files, err := inventory.LoadFiles(filepath.Join(root, "parity", "inventory", "files.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	fileHashes := make(map[string]string, len(files))
	for _, file := range files {
		fileHashes[file.Path] = file.SHA256
	}
	if fileHashes[manifest.Image.SourcePath] != manifest.Image.SourceSHA256 ||
		fileHashes[manifest.Image.GeneratorPath] != manifest.Image.GeneratorSHA256 {
		t.Fatalf("image provenance does not match locked source inventory")
	}
}

func TestM0CatalogCoversEveryLockedArtifactAndContract(t *testing.T) {
	root := repoRoot(t)
	entries, err := catalog.LoadCatalog(filepath.Join(root, "parity", "catalog.jsonl"))
	if err != nil {
		t.Fatalf("catalog.LoadCatalog: %v", err)
	}
	ids := make(map[string]bool, len(entries))
	for _, entry := range entries {
		ids[entry.ID] = true
	}

	files, err := inventory.LoadFiles(filepath.Join(root, "parity", "inventory", "files.jsonl"))
	if err != nil {
		t.Fatalf("inventory.LoadFiles: %v", err)
	}
	fileManifest, err := inventory.LoadManifest(filepath.Join(root, "parity", "inventory", "manifest.json"))
	if err != nil {
		t.Fatalf("inventory.LoadManifest: %v", err)
	}
	if err := inventory.Validate(files, fileManifest, ids); err != nil {
		t.Fatalf("inventory.Validate: %v", err)
	}

	symbols, err := surface.LoadSymbols(filepath.Join(root, "parity", "surface", "symbols.jsonl"))
	if err != nil {
		t.Fatalf("surface.LoadSymbols: %v", err)
	}
	surfaceManifest, err := surface.LoadManifest(filepath.Join(root, "parity", "surface", "manifest.json"))
	if err != nil {
		t.Fatalf("surface.LoadManifest: %v", err)
	}
	if err := surface.Validate(symbols, surfaceManifest, ids); err != nil {
		t.Fatalf("surface.Validate: %v", err)
	}

	var missing []string
	for _, symbol := range symbols {
		if !ids[symbol.ID] {
			missing = append(missing, symbol.ID)
		}
		identity := strings.TrimPrefix(symbol.ID, "symbol:")
		if symbol.Constructible && !ids["constructor:"+identity] {
			missing = append(missing, "constructor:"+identity)
		}
		prefix := "member:" + identity + "."
		for _, member := range symbol.Members {
			if !ids[prefix+member] {
				missing = append(missing, prefix+member)
			}
		}
		for _, member := range symbol.StaticMembers {
			id := "static-member:" + identity + "." + member
			if !ids[id] {
				missing = append(missing, id)
			}
		}
	}
	if len(missing) != 0 {
		limit := min(len(missing), 10)
		t.Fatalf("Parity Catalog misses %d public contracts; first %d: %v", len(missing), limit, missing[:limit])
	}
}

func TestM0ParityCatalogTracksDualSourceSnapshot(t *testing.T) {
	root := repoRoot(t)
	entries, err := catalog.LoadCatalog(filepath.Join(root, "parity", "catalog.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	byID := make(map[string]*catalog.Entry, len(entries))
	for i := range entries {
		byID[entries[i].ID] = &entries[i]
	}
	snapshot := byID[catalogSnapshotCatalogID]
	if snapshot == nil {
		t.Fatalf("Parity Catalog lacks %s", catalogSnapshotCatalogID)
	}
	if snapshot.BaselineRole != catalog.BaselineRoleCatalog || snapshot.Upstream.Commit != catalogBaselineCommit ||
		snapshot.Status != catalog.StatusVerified || snapshot.Milestone != "M0" {
		t.Fatalf("Catalog Snapshot entry = %+v", *snapshot)
	}
	if len(snapshot.Evidence) != 1 || snapshot.Evidence[0].Baseline != catalogBaselineCommit ||
		snapshot.Evidence[0].InputHash != "sha256:"+catalogReleaseSHA256 {
		t.Fatalf("Catalog Snapshot evidence = %+v", snapshot.Evidence)
	}
	imageSnapshot := byID[imageSnapshotCatalogID]
	if imageSnapshot == nil {
		t.Fatalf("Parity Catalog lacks %s", imageSnapshotCatalogID)
	}
	if imageSnapshot.BaselineRole != "" || imageSnapshot.Upstream.Commit != codeBaselineCommit ||
		imageSnapshot.Status != catalog.StatusVerified || imageSnapshot.Milestone != "M0" ||
		len(imageSnapshot.Evidence) != 1 || imageSnapshot.Evidence[0].Baseline != codeBaselineCommit ||
		imageSnapshot.Evidence[0].InputHash != "sha256:"+imageSourceSHA256 {
		t.Fatalf("Image Catalog Snapshot entry = %+v", *imageSnapshot)
	}

	data, err := os.ReadFile(filepath.Join(root, "parity", "catalog.manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := catalog.ParseManifest(data)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.CatalogBaselineCommit != catalogBaselineCommit ||
		manifest.CatalogSnapshotManifest != "baseline/snapshot.manifest.json" {
		t.Fatalf("Parity Catalog dual baseline link = %q/%q",
			manifest.CatalogBaselineCommit, manifest.CatalogSnapshotManifest)
	}
}

func TestM0CatalogManifestLinksCoverageArtifacts(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "parity", "catalog.manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		InventoryManifest       string `json:"inventory_manifest"`
		SurfaceManifest         string `json:"surface_manifest"`
		CatalogSnapshotManifest string `json:"catalog_snapshot_manifest"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.InventoryManifest != "inventory/manifest.json" || manifest.SurfaceManifest != "surface/manifest.json" ||
		manifest.CatalogSnapshotManifest != "baseline/snapshot.manifest.json" {
		t.Fatalf("coverage manifests = %q/%q/%q", manifest.InventoryManifest, manifest.SurfaceManifest, manifest.CatalogSnapshotManifest)
	}
}

func TestM0LearningNavigationAndSDKExampleExist(t *testing.T) {
	root := repoRoot(t)
	for _, name := range []string{
		"docs/learning/m0-compatibility-skeleton.md",
		"docs/mappings/typescript-to-go/m0.md",
		"examples/m0-contracts/main.go",
		"THIRD_PARTY_LICENSES/models.dev-MIT.txt",
		"Makefile",
	} {
		info, err := os.Stat(filepath.Join(root, filepath.FromSlash(name)))
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if info.Size() == 0 {
			t.Errorf("%s is empty", name)
		}
	}
}

func TestM0PackageAndCommandBoundaries(t *testing.T) {
	root := repoRoot(t)
	for _, name := range []string{
		"ai", "agent", "codingagent", "telemetry", "tui", "protocol", "client",
		"cmd/pig", "cmd/pig-ai",
	} {
		info, err := os.Stat(filepath.Join(root, filepath.FromSlash(name)))
		if err != nil || !info.IsDir() {
			t.Errorf("boundary %s is not a directory: %v", name, err)
		}
	}
}

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nankedr/pig/internal/catalog"
	"github.com/nankedr/pig/internal/surface"
)

const (
	baselineCommit        = "936aff00918de1187f085f123c2812d8f2d67745"
	catalogBaselineCommit = "53fa77ccd8a279eb87e92294ef3687b03ff80112"
	catalogReleaseSHA256  = "294d8067eb42327be0db4792d3be792daff588d8fc22549270a972ec9e5407e7"
	catalogResultSHA256   = "1268addeb40f0eb1b1b4a4229c1df59206b5e8bf5a55fdb627af58bc422a5583"
	catalogSnapshotID     = "contract:baseline/catalog-snapshot"
	imageSourceSHA256     = "76ffa23d2fbd3fa7505a979af972c8be38ee1be16cdcd3b9512411efb476f3a2"
	imageResultSHA256     = "fba8b88a29f861c0e0ab23960af7370e095198ca0bdd88c2721495cb36b50142"
	imageSnapshotID       = "contract:baseline/image-catalog-snapshot"
)

var coverageModules = map[string]bool{
	"ai":        true,
	"client":    true,
	"protocol":  true,
	"telemetry": true,
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	parity := filepath.Join(root, "parity")
	entries, err := catalog.LoadCatalog(filepath.Join(parity, "catalog.jsonl"))
	if err != nil {
		return err
	}
	symbols, err := surface.LoadSymbols(filepath.Join(parity, "surface", "symbols.jsonl"))
	if err != nil {
		return err
	}

	indexes := make(map[string]int, len(entries))
	for i, entry := range entries {
		entries[i].SchemaVersion = catalog.SchemaVersion
		indexes[entry.ID] = i
	}
	for _, managed := range []catalog.Entry{catalogSnapshotEntry(), imageSnapshotEntry()} {
		if i, ok := indexes[managed.ID]; ok {
			entries[i] = managed
		} else {
			indexes[managed.ID] = len(entries)
			entries = append(entries, managed)
		}
	}
	for _, symbol := range symbols {
		if !coverageModules[symbol.Module] {
			continue
		}
		target := packageTarget(symbol)
		entries = upsertCoverage(entries, indexes,
			coverageEntry(symbol.ID, symbol.Upstream.Reference, symbol.Module, target))
		identity := strings.TrimPrefix(symbol.ID, "symbol:")
		for _, member := range symbol.Members {
			id := "member:" + identity + "." + member
			entries = upsertCoverage(entries, indexes,
				coverageEntry(id, symbol.Upstream.Reference+"."+member, symbol.Module, target))
		}
	}

	manifest := catalog.BuildManifest(entries, baselineCommit, catalog.DefaultManifestPaths)
	if err := catalog.Validate(entries, manifest); err != nil {
		return err
	}
	data, err := catalog.EncodeEntries(entries)
	if err != nil {
		return err
	}
	manifestData, err := catalog.EncodeManifest(manifest)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(parity, "catalog.jsonl"), data, 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(parity, "catalog.manifest.json"), manifestData, 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(parity, "reports", "catalog.md"), []byte(catalog.GenerateReport(entries)), 0o644)
}

func coverageEntry(id, reference, module, target string) catalog.Entry {
	return catalog.Entry{
		SchemaVersion: catalog.SchemaVersion,
		ID:            id,
		Upstream: catalog.Upstream{
			Module: module, Repository: surface.Repository, Commit: baselineCommit, Reference: reference,
		},
		Mapping:        catalog.Mapping{Module: module, Target: target, Kind: "package"},
		Status:         catalog.StatusInventoried,
		Milestone:      "M14",
		Classification: "public-api",
		Notes:          "M0 exact surface coverage; a later reviewed contract or matrix row supersedes this default behavioral milestone, status and evidence.",
	}
}

func catalogSnapshotEntry() catalog.Entry {
	return catalog.Entry{
		SchemaVersion: catalog.SchemaVersion,
		ID:            catalogSnapshotID,
		BaselineRole:  catalog.BaselineRoleCatalog,
		Upstream: catalog.Upstream{
			Module:     "ai",
			Repository: "https://github.com/earendil-works/pi",
			Commit:     catalogBaselineCommit,
			Reference:  "releases/download/v0.84.1/pi-0.84.1-source.tar.gz",
		},
		Mapping:        catalog.Mapping{Module: "ai", Target: "parity/baseline/snapshot.manifest.json", Kind: "contract"},
		Status:         catalog.StatusVerified,
		Milestone:      "M0",
		Classification: "public-api",
		Evidence: []catalog.Evidence{{
			Kind:            "go-test",
			Ref:             "internal/m0gate/gate_test.go#TestM0CatalogSnapshotIsCaptured",
			Baseline:        catalogBaselineCommit,
			CaseID:          "issue35-dual-source-catalog-snapshot",
			InputHash:       "sha256:" + catalogReleaseSHA256,
			ExecutionMethod: "go test ./internal/m0gate -run '^TestM0CatalogSnapshotIsCaptured$'",
			Expected:        "v0.84.1 is losslessly flattened with zero semantic overlays",
			Actual:          "39 providers/1220 chat models; result sha256:" + catalogResultSHA256,
			Platform:        "any",
			CatalogID:       catalogSnapshotID,
		}},
		Notes: "Approved dual-source baseline: catalog data is from v0.84.1, not a fixed-run artifact of the code baseline.",
	}
}

func imageSnapshotEntry() catalog.Entry {
	return catalog.Entry{
		SchemaVersion: catalog.SchemaVersion,
		ID:            imageSnapshotID,
		Upstream: catalog.Upstream{
			Module:     "ai",
			Repository: surface.Repository,
			Commit:     baselineCommit,
			Reference:  "packages/ai/src/image-models.generated.ts#IMAGE_MODELS",
		},
		Mapping:        catalog.Mapping{Module: "ai", Target: "parity/baseline/catalog/image/models.json", Kind: "contract"},
		Status:         catalog.StatusVerified,
		Milestone:      "M0",
		Classification: "public-api",
		Evidence: []catalog.Evidence{{
			Kind:            "source-drift",
			Ref:             "Makefile#m0-source-drift",
			Baseline:        baselineCommit,
			CaseID:          "issue35-image-catalog-source-drift",
			InputHash:       "sha256:" + imageSourceSHA256,
			ExecutionMethod: "make m0-source-drift PIG_PI_SOURCE_CHECKOUT=/path/to/pristine/pi",
			Expected:        "IMAGE_MODELS serializes byte-identically with node-import-and-json-stringify",
			Actual:          "1 provider/42 image models; result sha256:" + imageResultSHA256,
			Platform:        "any",
			CatalogID:       imageSnapshotID,
		}},
		Notes: "Image catalog data is the exact committed source from the code baseline and remains outside the M0 runtime.",
	}
}

func upsertCoverage(entries []catalog.Entry, indexes map[string]int, replacement catalog.Entry) []catalog.Entry {
	i, ok := indexes[replacement.ID]
	if !ok {
		indexes[replacement.ID] = len(entries)
		return append(entries, replacement)
	}
	if entries[i].Mapping.Kind == "package" {
		entries[i] = replacement
	}
	return entries
}

func packageTarget(symbol surface.Symbol) string {
	target := "github.com/nankedr/pig/" + symbol.Module
	if symbol.Module == "client" && len(symbol.ExportSubpaths) == 1 && symbol.ExportSubpaths[0] == "./unix" {
		return target + "/unix"
	}
	if symbol.Module == "telemetry" && len(symbol.ExportSubpaths) == 1 && symbol.ExportSubpaths[0] == "./testing" {
		return target + "/testing"
	}
	return target
}

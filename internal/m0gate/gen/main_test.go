package main

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/nankedr/pig/internal/catalog"
	"github.com/nankedr/pig/internal/surface"
)

func TestUpsertCoverage(t *testing.T) {
	replacement := catalog.Entry{ID: "symbol:ai/example.ts#Example", Mapping: catalog.Mapping{Kind: "package"}, Milestone: "M14"}

	t.Run("repairs managed row", func(t *testing.T) {
		entries := []catalog.Entry{{ID: replacement.ID, Mapping: catalog.Mapping{Kind: "package"}, Milestone: "M1"}}
		got := upsertCoverage(entries, map[string]int{replacement.ID: 0}, replacement)
		if !reflect.DeepEqual(got, []catalog.Entry{replacement}) {
			t.Fatalf("upsertCoverage() = %+v, want replacement", got)
		}
	})

	t.Run("preserves reviewed row", func(t *testing.T) {
		reviewed := catalog.Entry{ID: replacement.ID, Mapping: catalog.Mapping{Kind: "contract"}, Milestone: "M2"}
		got := upsertCoverage([]catalog.Entry{reviewed}, map[string]int{replacement.ID: 0}, replacement)
		if !reflect.DeepEqual(got, []catalog.Entry{reviewed}) {
			t.Fatalf("upsertCoverage() = %+v, want reviewed row", got)
		}
	})
}

func TestCatalogSnapshotEntryPinsApprovedDualBaseline(t *testing.T) {
	entry := catalogSnapshotEntry()
	if entry.ID != catalogSnapshotID || entry.BaselineRole != catalog.BaselineRoleCatalog ||
		entry.Upstream.Commit != catalogBaselineCommit || entry.Status != catalog.StatusVerified || entry.Classification != "public-api" {
		t.Fatalf("catalogSnapshotEntry() = %+v", entry)
	}
	if len(entry.Evidence) != 1 || entry.Evidence[0].InputHash != "sha256:"+catalogReleaseSHA256 ||
		!strings.Contains(entry.Evidence[0].Actual, catalogResultSHA256) {
		t.Fatalf("catalogSnapshotEntry() evidence = %+v", entry.Evidence)
	}
}

func TestImageSnapshotEntryPinsCodeBaseline(t *testing.T) {
	entry := imageSnapshotEntry()
	if entry.ID != imageSnapshotID || entry.BaselineRole != "" || entry.Upstream.Commit != baselineCommit ||
		entry.Status != catalog.StatusVerified || entry.Classification != "public-api" {
		t.Fatalf("imageSnapshotEntry() = %+v", entry)
	}
	if len(entry.Evidence) != 1 || entry.Evidence[0].InputHash != "sha256:"+imageSourceSHA256 ||
		!strings.Contains(entry.Evidence[0].Actual, imageResultSHA256) {
		t.Fatalf("imageSnapshotEntry() evidence = %+v", entry.Evidence)
	}
}

func TestCatalogHasNoPendingSnapshotClaims(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
	entries, err := catalog.LoadCatalog(filepath.Join(root, "parity", "catalog.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		data, err := json.Marshal(entry)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "pending-capture") {
			t.Errorf("catalog entry %s still claims pending-capture", entry.ID)
		}
	}
}

func TestManagedCoverageRowsMatchGenerator(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
	entries, err := catalog.LoadCatalog(filepath.Join(root, "parity", "catalog.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	symbols, err := surface.LoadSymbols(filepath.Join(root, "parity", "surface", "symbols.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	byID := make(map[string]catalog.Entry, len(entries))
	for _, entry := range entries {
		byID[entry.ID] = entry
	}

	for _, symbol := range symbols {
		if !coverageModules[symbol.Module] {
			continue
		}
		target := packageTarget(symbol)
		assertManagedCoverage(t, byID[symbol.ID], coverageEntry(symbol.ID, symbol.Upstream.Reference, symbol.Module, target))
		identity := strings.TrimPrefix(symbol.ID, "symbol:")
		for _, member := range symbol.Members {
			id := "member:" + identity + "." + member
			assertManagedCoverage(t, byID[id], coverageEntry(id, symbol.Upstream.Reference+"."+member, symbol.Module, target))
		}
	}
}

func assertManagedCoverage(t *testing.T, got, want catalog.Entry) {
	t.Helper()
	if got.ID == "" {
		t.Errorf("managed coverage row %s is missing", want.ID)
		return
	}
	if got.Mapping.Kind == "package" && !reflect.DeepEqual(got, want) {
		t.Errorf("managed coverage row %s differs\n got: %+v\nwant: %+v", want.ID, got, want)
	}
}

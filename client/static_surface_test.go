package client_test

import (
	"context"
	"crypto/sha256"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"github.com/nankedr/pig/client"
	"github.com/nankedr/pig/internal/catalog"
	"github.com/nankedr/pig/internal/surface"
)

const (
	piClientSymbolID       = "symbol:client/src/client.ts#PiClient"
	piClientConnectID      = "static-member:client/src/client.ts#PiClient.connect"
	piClientBaselineCommit = "936aff00918de1187f085f123c2812d8f2d67745"
)

var updateClientStaticCatalog = flag.Bool("update-client-static-catalog", false, "regenerate the PiClient.connect static-member Catalog row")

var _ func(context.Context, client.ClientOptions) (*client.Client, error) = client.Dial

func TestPiClientConnectStaticSurfaceCatalog(t *testing.T) {
	root := clientStaticSurfaceRepoRoot(t)
	surfacePath := filepath.Join(root, "parity", "surface", "symbols.jsonl")
	surfaceJSONL, err := os.ReadFile(surfacePath)
	if err != nil {
		t.Fatalf("read surface: %v", err)
	}
	surfaceHash := fmt.Sprintf("sha256:%x", sha256.Sum256(surfaceJSONL))

	symbols, err := surface.LoadSymbols(surfacePath)
	if err != nil {
		t.Fatalf("LoadSymbols: %v", err)
	}
	var piClient *surface.Symbol
	for i := range symbols {
		if symbols[i].ID == piClientSymbolID {
			if piClient != nil {
				t.Fatalf("surface contains duplicate %s", piClientSymbolID)
			}
			piClient = &symbols[i]
		}
	}
	if piClient == nil {
		t.Fatalf("surface missing %s", piClientSymbolID)
	}
	if want := []string{"connect"}; !reflect.DeepEqual(piClient.StaticMembers, want) {
		t.Fatalf("%s static_members = %v, want %v", piClientSymbolID, piClient.StaticMembers, want)
	}

	entries, err := catalog.LoadCatalog(filepath.Join(root, "parity", "catalog.jsonl"))
	if err != nil {
		t.Fatalf("LoadCatalog: %v", err)
	}
	want := catalog.Entry{
		SchemaVersion: catalog.SchemaVersion,
		ID:            piClientConnectID,
		Upstream: catalog.Upstream{
			Module:     "client",
			Repository: "https://github.com/badlogic/pi-mono",
			Commit:     piClientBaselineCommit,
			Reference:  "packages/client/src/client.ts#PiClient.connect",
		},
		Mapping: catalog.Mapping{
			Module: "client",
			Target: "github.com/nankedr/pig/client.Dial",
			Kind:   "contract",
		},
		Status:         catalog.StatusScaffolded,
		Milestone:      "M9",
		Classification: "public-api",
		Evidence: []catalog.Evidence{{
			Kind:            "go-test",
			Ref:             "client/client_test.go#TestConnectionLifecycleStubsFailWithoutCreatingTransport",
			Baseline:        piClientBaselineCommit,
			CaseID:          piClientConnectID,
			InputHash:       surfaceHash,
			ExecutionMethod: "go test ./client -run '^TestConnectionLifecycleStubsFailWithoutCreatingTransport$' -count=1",
			Expected:        "the pinned static PiClient.connect operation maps to client.Dial and fails as an immediate side-effect-free capability stub",
			Actual:          "PASS; client.Dial returned nil and structured ErrNotImplemented without invoking the transport factory",
			Platform:        "any",
			CatalogID:       piClientConnectID,
		}},
		Notes: "The pinned PiClient.connect static member maps to the idiomatic client.Dial package function. Runtime behavior remains an immediate side-effect-free Capability Stub until M9.",
	}
	if *updateClientStaticCatalog {
		found := false
		for i := range entries {
			if entries[i].ID != piClientConnectID {
				continue
			}
			if found {
				t.Fatalf("catalog contains duplicate %s", piClientConnectID)
			}
			entries[i] = want
			found = true
		}
		if !found {
			entries = append(entries, want)
		}
		data, err := catalog.EncodeEntries(entries)
		if err != nil {
			t.Fatalf("EncodeEntries: %v", err)
		}
		if err := os.WriteFile(filepath.Join(root, "parity", "catalog.jsonl"), data, 0o644); err != nil {
			t.Fatalf("write catalog: %v", err)
		}
	}

	var got *catalog.Entry
	for i := range entries {
		if entries[i].ID == piClientConnectID {
			if got != nil {
				t.Fatalf("catalog contains duplicate %s", piClientConnectID)
			}
			got = &entries[i]
		}
	}
	if got == nil {
		t.Fatalf("catalog missing %s", piClientConnectID)
	}
	if !reflect.DeepEqual(*got, want) {
		t.Fatalf("%s = %+v, want %+v", piClientConnectID, *got, want)
	}
}

func clientStaticSurfaceRepoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), ".."))
}

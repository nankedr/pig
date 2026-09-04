package m2gate_test

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/nankedr/pig/codingagent"
	"github.com/nankedr/pig/internal/catalog"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate repository")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "../.."))
}

func TestM2CatalogEvidence(t *testing.T) {
	root := repoRoot(t)
	entries, err := catalog.LoadCatalog(filepath.Join(root, "parity/catalog.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, entry := range entries {
		if entry.Milestone != "M2" {
			continue
		}
		ids = append(ids, entry.ID)
		t.Run(entry.ID, func(t *testing.T) {
			switch entry.Status {
			case catalog.StatusVerified, catalog.StatusImplemented:
			case catalog.StatusPartial:
				if entry.Partial == nil || len(entry.Partial.Supported) == 0 || len(entry.Partial.Unsupported) == 0 || entry.Notes == "" {
					t.Error("partial capability must explain its supported and remaining scope")
				}
			default:
				t.Errorf("M2 capability is still %s", entry.Status)
			}
			replay := false
			for _, evidence := range entry.Evidence {
				if evidence.CatalogID != entry.ID || evidence.Baseline != entry.Upstream.Commit || evidence.InputHash == "" || evidence.CaseID == "" || evidence.ExecutionMethod == "" || evidence.Expected == "" || evidence.Actual == "" || evidence.Platform == "" {
					continue
				}
				path, fragment, _ := strings.Cut(evidence.Ref, "#")
				data, err := os.ReadFile(filepath.Join(root, path))
				if err != nil {
					t.Errorf("evidence %s: %v", evidence.Ref, err)
					continue
				}
				if evidence.Kind == "go-test" {
					if !strings.HasSuffix(path, "_test.go") || (fragment != "" && !strings.Contains(string(data), "func "+fragment+"(")) {
						t.Errorf("evidence does not resolve to a test: %s", evidence.Ref)
						continue
					}
					if evidence.CaseID == "issue70-m2-matrix-values" && evidence.InputHash != fmt.Sprintf("sha256:%x", sha256.Sum256(data)) {
						t.Error("M2 matrix evidence test source changed; re-audit the value-state evidence")
					}
					replay = true
				}
			}
			if !replay {
				t.Error("M2 capability needs a complete, executable Go evidence record")
			}
		})
	}
	sort.Strings(ids)
	want, err := os.ReadFile(filepath.Join(root, "internal/m2gate/testdata/catalog_ids.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(ids, "\n")+"\n" != string(want) {
		t.Error("M2 Catalog scope changed; review the frozen ID snapshot before changing milestone assignments")
	}
}

func TestM2ReleaseVersion(t *testing.T) {
	if codingagent.Version != "0.2.0" {
		t.Fatalf("SDK version = %s, want 0.2.0", codingagent.Version)
	}
	for _, flag := range []string{"--version", "-v"} {
		result, err := codingagent.RunCLI(context.Background(), codingagent.CLIInvocation{Arguments: []string{flag}})
		if err != nil || result.Stdout != "0.2.0\n" || result.Stderr != "" {
			t.Fatalf("%s: result=%+v, err=%v", flag, result, err)
		}
	}
}

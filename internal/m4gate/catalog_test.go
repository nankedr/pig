package m4gate_test

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/nankedr/pig/internal/catalog"
)

func TestM4CatalogEvidence(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "../.."))
	entries, err := catalog.LoadCatalog(filepath.Join(root, "parity/catalog.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	var scope []string
	for _, entry := range entries {
		if entry.Milestone != "M4" && entry.ID != "cmd-pig" && entry.ID != "module-codingagent" && entry.ID != "contract:cli/pig/args" {
			continue
		}
		boundary, _ := json.Marshal(entry.Partial)
		scope = append(scope, fmt.Sprintf("%s\t%s\t%s\t%x", entry.ID, entry.Status, entry.Mapping.Target, sha256.Sum256(boundary)))
		if !strings.HasPrefix(entry.ID, "contract:") || entry.ID == "contract:rpc/command-union" {
			continue
		}
		t.Run(entry.ID, func(t *testing.T) {
			if entry.Status != catalog.StatusPartial && entry.Status != catalog.StatusImplemented && entry.Status != catalog.StatusVerified {
				t.Errorf("behavior has no implementation: %s", entry.Status)
			}
			if entry.Status == catalog.StatusPartial && (entry.Partial == nil || len(entry.Partial.Supported) == 0 || len(entry.Partial.Unsupported) == 0) {
				t.Error("partial contract needs exact delivered and deferred boundaries")
			}
			replay := false
			for _, evidence := range entry.Evidence {
				if evidence.CatalogID != entry.ID || evidence.Baseline != entry.Upstream.Commit || evidence.InputHash == "" || evidence.CaseID == "" || evidence.ExecutionMethod == "" || evidence.Expected == "" || evidence.Actual == "" || evidence.Platform == "" {
					t.Errorf("incomplete evidence: %s", evidence.Ref)
					continue
				}
				path, fragment, _ := strings.Cut(evidence.Ref, "#")
				data, err := os.ReadFile(filepath.Join(root, path))
				if err != nil {
					t.Error(err)
					continue
				}
				if evidence.Kind == "go-test" {
					if !strings.HasSuffix(path, "_test.go") || (fragment != "" && !strings.Contains(string(data), "func "+fragment+"(")) {
						t.Errorf("unresolved test: %s", evidence.Ref)
					}
					replay = true
				}
			}
			if !replay {
				t.Error("behavior needs executable Go evidence")
			}
		})
	}
	sort.Strings(scope)
	want, err := os.ReadFile(filepath.Join(root, "internal/m4gate/testdata/catalog_scope.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(scope, "\n")+"\n" != string(want) {
		t.Error("M4 mapping, status or exact partial scope changed; audit before updating freeze snapshot")
	}
}

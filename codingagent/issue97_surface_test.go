package codingagent_test

import (
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/nankedr/pig/codingagent"
	"github.com/nankedr/pig/internal/baseline"
	"github.com/nankedr/pig/internal/parity"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

var updateIssue97Surface = flag.Bool("update-issue97-surface", false, "regenerate HTML export API snapshot")

func TestIssue97ExportAPISnapshot(t *testing.T) {
	text := fmt.Sprintf("ExportFromFile %s\nAgentSession.ExportToHTML %s\nRPCClient.ExportHTML %s\n", reflect.TypeOf(codingagent.ExportFromFile), reflect.TypeOf((*codingagent.AgentSession).ExportToHTML), reflect.TypeOf((*codingagent.RPCClient).ExportHTML))
	path := "testdata/issue97_surface_golden.txt"
	if *updateIssue97Surface {
		if err := os.WriteFile(path, []byte(text), 0644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil || text != string(want) {
		t.Fatal("HTML export API drift", err)
	}
}
func TestIssue97ExportFixtureIntegrity(t *testing.T) {
	root := issue32RepoRoot(t)
	lock, _, err := baseline.Load(filepath.Join(root, "parity/baseline"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = parity.LoadFixture(filepath.Join(root, "parity/oracle/fixtures/export-html.json"), parity.Baseline{ID: lock.BaselineID, Commit: lock.Upstream.Commit, Repository: lock.Upstream.Repository}); err != nil {
		t.Fatal(err)
	}
}

func TestIssue97ExportAssetIntegrity(t *testing.T) {
	var manifest struct {
		Commit string
		Assets []struct{ Path, SHA256 string }
	}
	raw, err := os.ReadFile("exporthtml/manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Commit != issue32BaselineCommit || len(manifest.Assets) != 5 {
		t.Fatal("invalid asset provenance")
	}
	for _, asset := range manifest.Assets {
		raw, err := os.ReadFile(filepath.Join("exporthtml", asset.Path))
		if err != nil {
			t.Fatal(err)
		}
		if fmt.Sprintf("%x", sha256.Sum256(raw)) != asset.SHA256 {
			t.Fatalf("asset drift: %s", asset.Path)
		}
	}
}

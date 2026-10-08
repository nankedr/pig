package codingagent_test

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/nankedr/pig/codingagent"
	"github.com/nankedr/pig/internal/catalog"
)

var updateToolImages135 = flag.Bool("update-tool-images", false, "refresh tool image Catalog evidence")

func toolImagesEvidence135(t *testing.T, id string) []issue32ModuleEvidenceDescriptor {
	var result []issue32ModuleEvidenceDescriptor
	for _, item := range []struct{ kind, path, run, want string }{
		{"go-test", "ai/tool_images_test.go", "go test -race ./ai -run '^TestToolImages' -count=1", "tool validation, aggregate user+tool quota and nonvision rejection before transport"},
		{"go-test", "agent/tool_images_test.go", "go test -race ./agent -run '^TestToolImages' -count=3", "parallel updates/finals, cancellation, source-order history and actual image wire"},
		{"go-test", "codingagent/tool_images_test.go", "go test ./codingagent -run '^TestToolImagesRead' -count=1", "42 fixed Pi read cases, exact unchanged bytes, orientation corners/dimensions/notes; declared corrupt no-resize deviation"},
		{"go-test", "codingagent/tool_images_session_test.go", "go test -race ./codingagent -run '^TestToolImagesSession|^TestToolImagesTextOnly' -count=1", "dual API read -> image -> write; HTTP and terminal Session retry, production v3 restore/fork/compaction; queues reject images explicitly"},
		{"go-test", "cmd/pig/tool_images_process_test.go", "PIG_TEST_RACE=1 go test -race ./cmd/pig -run '^TestToolImagesCLIAndRPC$' -count=1", "real CLI dual API read/write/cross-process restore/fork and RPC read/write/image history"},
		{"go-test", "internal/parity/tool_images_test.go", "go test ./internal/parity -run '^TestToolImagesFixedPiOracle$' -count=1", "public SDK exact tool-image wire matches fixed Pi in both APIs"},
		{"fixture", "parity/oracle/fixtures/tool-images.json", "node --experimental-strip-types parity/oracle/tool-images.mjs /locked/pi --check", "fixed Pi consecutive tool-result mixed/image-only wire"},
		{"fixture", "parity/oracle/fixtures/read-images.json", "node parity/oracle/read-images.mjs /locked/pi --check", "fixed Pi read semantic projection; resized/converted codec bytes excluded"},
		{"go-test", "codingagent/tool_images_live_test.go", "make tool-vision-live-smoke", "protected DeepSeek dual API read -> vision red/blue -> write; service evidence separate from Pi"},
	} {
		actual := "PASS; offline public boundary and fixed Pi projection evidence"
		if item.path == "codingagent/tool_images_live_test.go" {
			actual = "PASS 2026-10-08 darwin-arm64; current deepseek-flash Responses and Chat Completions both read local red/blue PNG, identify colors and write file; not Pi parity"
		}
		result = append(result, issue32ModuleEvidenceDescriptor{InputPath: item.path, Evidence: catalog.Evidence{Kind: item.kind, Ref: item.path, Baseline: issue32BaselineCommit, CaseID: "issue135/" + item.path, InputHash: issue32FileHash(t, item.path), ExecutionMethod: item.run, Expected: item.want, Actual: actual, Platform: "darwin-arm64 (live), darwin/linux (process), any (SDK)", CatalogID: id}})
	}
	return result
}

func TestToolImagesCatalog(t *testing.T) {
	id := "contract:codingagent/tool-images"
	entry := catalog.Entry{SchemaVersion: catalog.SchemaVersion, ID: id, Upstream: catalog.Upstream{Module: "coding-agent", Repository: "https://github.com/badlogic/pi-mono", Commit: issue32BaselineCommit, Reference: "packages/coding-agent/src/core/tools/read.ts#createReadTool"}, Mapping: catalog.Mapping{Module: "codingagent", Target: "github.com/nankedr/pig/codingagent#CreateReadTool", Kind: "contract"}, Status: catalog.StatusImplemented, Milestone: "M12", Classification: "public-api", Evidence: issue32EvidenceFromDescriptors(toolImagesEvidence135(t, id)), Notes: "V1/#135 production tool-image continuation: JPEG/PNG/GIF/static WebP/BMP, default orientation/2000x2000/4.5MiB-base64 processing, controlled read and tool updates/finals, ordered dual API wire, retry/cancel/parallel and v3 restore/fork/compaction. Go CatmullRom/PNG/JPEG codec differs from Photon; exact processed bytes not claimed. Corrupt/oversize/MIME mismatch read explains omission; nonvision fails explicitly. Production image queues and RPC user attachments stay explicit Stub (#136 boundary); remaining providers/generation/edit/URL/Files API/animated WebP/Harness/Remote are V2. API signatures unchanged. Protected dual API tool-vision smoke PASS 2026-10-08; separate current-service evidence. See ADR-0046 and parity/tool-images-matrix.json."}
	path := filepath.Join(issue32RepoRoot(t), "parity/catalog.jsonl")
	entries, err := catalog.LoadCatalog(path)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for i, current := range entries {
		if current.ID == id {
			found = true
			if *updateToolImages135 {
				entries[i] = entry
			} else if !reflect.DeepEqual(current, entry) {
				t.Fatal("tool image Catalog evidence drift")
			}
		}
	}
	if !found {
		if !*updateToolImages135 {
			t.Fatal("missing tool image contract")
		}
		entries = append(entries, entry)
	}
	if *updateToolImages135 {
		data, err := catalog.EncodeEntries(entries)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0644); err != nil {
			t.Fatal(err)
		}
	}
	var matrix struct {
		CatalogID string `json:"catalog_id"`
		Rows      []struct{ ID, Version, Result string }
	}
	data, err := os.ReadFile(filepath.Join(issue32RepoRoot(t), "parity/tool-images-matrix.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &matrix); err != nil {
		t.Fatal(err)
	}
	if matrix.CatalogID != id || len(matrix.Rows) < 10 {
		t.Fatal("incomplete tool-image matrix")
	}
	for _, row := range matrix.Rows {
		if row.ID == "" || row.Result == "" || row.Version != "V1" && row.Version != "V2" {
			t.Fatal("invalid matrix row")
		}
	}
	if reflect.TypeOf(codingagent.ResizeImage).String() != "func([]uint8, string, ...codingagent.ImageResizeOptions) (*codingagent.ResizedImage, error)" {
		t.Fatal("public ResizeImage signature drift")
	}
}

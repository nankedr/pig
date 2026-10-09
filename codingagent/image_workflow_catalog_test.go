package codingagent_test

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/nankedr/pig/codingagent"
	"github.com/nankedr/pig/internal/catalog"
	"github.com/nankedr/pig/tui"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

var updateImageWorkflow = flag.Bool("update-image-workflow", false, "refresh #136 Catalog evidence and API snapshot")

func TestImageWorkflowCatalog136(t *testing.T) {
	id := "contract:codingagent/image-workflow"
	entry := catalog.Entry{SchemaVersion: catalog.SchemaVersion, ID: id, Upstream: catalog.Upstream{Module: "coding-agent", Repository: "https://github.com/badlogic/pi-mono", Commit: issue32BaselineCommit, Reference: "packages/coding-agent/src/modes/interactive/interactive-mode.ts"}, Mapping: catalog.Mapping{Module: "codingagent", Target: issue32GoPackage + ".InteractiveMode", Kind: "contract"}, Status: catalog.StatusPartial, Milestone: "M12", Classification: "public-api", Partial: &catalog.Partial{Supported: []string{"V1/#136 local/initial/Ctrl+V pending attachments: add/list/remove/preview, combined and image-only submission, preflight restore, accepted cancellation and text queue rejection", "darwin-arm64 native NSPasteboard PNG/JPEG/GIF/static WebP plus text; Kitty/Ghostty bounded preview, conservative dimensions, resize/redraw/close cleanup, session image lookup and fallback", "real dual API RPC prompt images/events/history, actual regular/fullscreen CLI cross-process inline restore and independent safe HTML user/tool raster images", "fixed Pi Kitty chunk/placement/deletion Oracle; existing dual API user/tool wire and processing fixtures retained"}, Unsupported: []string{"V2/#15/#128: TIFF/helper/worker/full-platform clipboard, iTerm2, multiplexer passthrough, automatic image scrollback/cropping/cache and measured cell dimensions remain explicit Stub or unverified", "V2 image queues, URL/Files API, other providers, generation/edit, Harness/Remote and extension renderers remain Stub"}}, Deviation: &catalog.Deviation{ADR: "docs/adr/0047-image-workflow.md", Reason: "Explicit /image commands and a bounded preview reuse existing Session state; validated raster-only data CSP preserves offline/path/XSS boundaries. No full platform/image parity claim."}, Notes: "V1 image-input delivery branch; see parity/image-workflow-matrix.json, docs/learning/v1-image-workflow.md and examples/image-workflow. Native clipboard PASS 2026-10-09 darwin-arm64; fixed Pi protocol and current service evidence are separate."}
	for _, item := range []struct{ kind, path, run, actual string }{
		{"go-test", "codingagent/image_workflow_interactive_test.go", "go test -race ./codingagent -run '^TestImageWorkflow(Interactive|PreflightRestore|CancelAndQueue)136$' -count=1", "PASS public Interactive PTY attachment lifecycle/failed admission/cancel and queues"},
		{"go-test", "cmd/pig/image_workflow_process_test.go", "PIG_TEST_RACE=1 go test -race ./cmd/pig -run '^TestImageWorkflowRPC136$' -count=1", "PASS real RPC dual API image wire/events/history/export"},
		{"go-test", "cmd/pig/image_workflow_tui_test.go", "PIG_TEST_RACE=1 go test -race ./cmd/pig -run '^TestImageWorkflowCLI136$' -count=1", "PASS real regular/fullscreen CLI @image, source deletion, cross-process restore and export"},
		{"go-test", "tui/image_preview_test.go", "go test -race ./tui -run '^TestImageWorkflowPreview136$' -count=1", "PASS bounded preview resize/redraw/scroll/cancel/raw-mode and image deletion; unknown-terminal no graphics"},
		{"fixture", "parity/oracle/fixtures/image-workflow.json", "node --experimental-strip-types parity/oracle/image-workflow.mjs /locked/pi --check", "PASS fixed Pi Kitty protocol only"},
		{"go-test", "tui/image_workflow_oracle_test.go", "go test ./tui -run '^TestImageWorkflowKittyOracle136$' -count=1", "PASS exact fixed Pi chunk/placement/deletion and injection rejection"},
		{"manual", "parity/export-html/images.mjs", "node parity/export-html/images.mjs", "PASS 2026-10-09 Chrome actual CLI raster DOM/modal, independent file, no network/file requests and invalid/SVG/URL/XSS rejection"},
		{"go-test", "codingagent/image_workflow_clipboard_test.go", "PIG_REQUIRE_CLIPBOARD_IMAGE_NATIVE=1 go test ./codingagent -run '^TestImageWorkflowNativeClipboard136$' -count=1 -v", "PASS 2026-10-09 darwin-arm64 actual NSPasteboard PNG/JPEG/GIF/WebP/text/Ctrl+V/remove and original clipboard/terminal restore; ordinary gate skips"},
		{"go-test", "cmd/pig/image_workflow_live_test.go", "PIG_REQUIRE_IMAGE_WORKFLOW_LIVE=1 go test ./cmd/pig -run '^TestImageWorkflowRPCLive136$' -count=1 -v", "PASS 2026-10-09 protected DeepSeek dual API real RPC user image -> vision -> read tool image -> write semantic colors -> standalone export; current service, not Pi"},
	} {
		entry.Evidence = append(entry.Evidence, catalog.Evidence{Kind: item.kind, Ref: item.path, Baseline: issue32BaselineCommit, CaseID: "issue136/" + item.path, InputHash: issue32FileHash(t, item.path), ExecutionMethod: item.run, Expected: "approved V1 branch through public product/SDK/terminal/browser boundary", Actual: item.actual, Platform: "darwin-arm64 (native), darwin/linux (process), any (SDK/protocol)", CatalogID: id})
	}
	path := filepath.Join(issue32RepoRoot(t), "parity/catalog.jsonl")
	entries, err := catalog.LoadCatalog(path)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for i, current := range entries {
		if current.ID == id {
			found = true
			if *updateImageWorkflow {
				entries[i] = entry
			} else if !reflect.DeepEqual(current, entry) {
				t.Fatal("image workflow Catalog drift")
			}
		}
	}
	if !found {
		if !*updateImageWorkflow {
			t.Fatal("missing image workflow contract")
		}
		entries = append(entries, entry)
	}
	if *updateImageWorkflow {
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
	raw, err := os.ReadFile(filepath.Join(issue32RepoRoot(t), "parity/image-workflow-matrix.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &matrix); err != nil {
		t.Fatal(err)
	}
	if matrix.CatalogID != id || len(matrix.Rows) < 8 {
		t.Fatal("missing approved scope matrix")
	}
	for _, row := range matrix.Rows {
		if row.ID == "" || row.Result == "" || row.Version != "V1" && row.Version != "V2" {
			t.Fatal("invalid scope row")
		}
	}
	snapshot := fmt.Sprintf("InteractiveModeOptions %s\nTextUI.PreviewImage %s\nRPCClient.Prompt %s\nRPCClient.PromptAndWait %s\n", reflect.TypeOf(codingagent.InteractiveModeOptions{}), reflect.TypeOf((*tui.TextUI).PreviewImage), reflect.TypeOf((*codingagent.RPCClient).Prompt), reflect.TypeOf((*codingagent.RPCClient).PromptAndWait))
	snapshotPath := "testdata/image_workflow_surface_golden.txt"
	if *updateImageWorkflow {
		if err := os.WriteFile(snapshotPath, []byte(snapshot), 0644); err != nil {
			t.Fatal(err)
		}
	} else {
		want, err := os.ReadFile(snapshotPath)
		if err != nil || string(want) != snapshot {
			t.Fatal("image workflow API drift", err)
		}
	}
}

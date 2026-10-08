package codingagent_test

import (
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
	"github.com/nankedr/pig/internal/catalog"
)

var updateUserImages = flag.Bool("update-user-images", false, "refresh user image evidence and API snapshot")

func TestUserImagesCatalog(t *testing.T) {
	root := issue32RepoRoot(t)
	id := "contract:codingagent/user-images"
	entry := catalog.Entry{SchemaVersion: catalog.SchemaVersion, ID: id, Upstream: catalog.Upstream{Module: "coding-agent", Repository: "https://github.com/badlogic/pi-mono", Commit: issue32BaselineCommit, Reference: "packages/coding-agent/src/core/agent-session.ts#prompt"}, Mapping: catalog.Mapping{Module: "codingagent", Target: "github.com/nankedr/pig/codingagent#AgentSession.Prompt.Images", Kind: "contract"}, Status: catalog.StatusImplemented, Milestone: "M12", Classification: "public-api", Notes: "V1/#134 local user images only: JPEG/PNG/GIF/static WebP, actual format/MIME/encoding/limits, original bytes/orientation/dimensions, dual API wire and full production-v3 replay/restore/fork/tree. GIF validates all frames (64/40M total limit); Issue #135 tool pictures/processing are separately covered by contract:codingagent/tool-images; Interactive/RPC user attachments #136 stay Stub. URL/Files API/animated WebP/other providers/generation/edit V2. deepseek-flash is a separate current-service configuration; fixed Snapshot unchanged. Fixed Pi Oracle covers only valid image wire. Protected DeepSeek deepseek-flash Responses vision/v3 restore smoke PASS 2026-10-08 darwin-arm64; separate service evidence, no full verified/freeze claim. See ADR-0045 and parity/user-images-matrix.json."}
	for _, item := range []struct{ kind, path, run, want string }{
		{"go-test", "ai/user_images_test.go", "go test -race ./ai -run '^TestUserImages' -count=1", "public SDK mixed image wire, no placeholder/nonvision transport; retry preserves image; cancellation/failed stream retains declared error/partial semantics"},
		{"go-test", "ai/user_images_files_test.go", "go test ./ai -run '^TestUserImagesLocal' -count=1", "JPEG EXIF bytes, actual PNG/GIF/WebP format; invalid/missing/oversize file errors"},
		{"go-test", "codingagent/user_images_test.go", "go test -race ./codingagent -run '^TestUserImages' -count=1", "production v3 restore after deleting file, fork isolation, tree reconstruction, corrupt inline data errors, invalid prompt has no effects"},
		{"go-test", "cmd/pig/user_images_process_test.go", "PIG_TEST_RACE=1 go test -race ./cmd/pig -run '^TestUserImagesCLI' -count=1", "actual @FILE CLI wire and cross-process restore/fork"},
		{"go-test", "internal/parity/user_images_test.go", "go test ./internal/parity -run '^TestUserImagesFixedPiOracle$' -count=1", "public Go SDK legal vision image wire matches pinned Pi fixture in both APIs"},
		{"fixture", "parity/oracle/fixtures/user-images.json", "node --experimental-strip-types parity/oracle/user-images.mjs /locked/pi --check", "fixed Pi mixed/only inline user image wire; separate from current DeepSeek service"},
		{"go-test", "codingagent/testdata/user_images_surface_golden.txt", "go test ./codingagent -run '^TestUserImagesAPISnapshot$' -count=1", "LoadImageFile, ValidateUserImage(s), DeepSeekVisionModel and InitialImages public surface"},
		{"go-test", "codingagent/user_images_live_test.go", "PIG_REQUIRE_VISION_LIVE=1 go test ./codingagent -run '^TestUserImagesDeepSeekLiveRestore$' -count=1", "protected real local PNG red/blue semantic identification, v3 reopen after original deletion and same image re-question"},
	} {
		data, err := os.ReadFile(filepath.Join(root, item.path))
		if err != nil {
			t.Fatal(err)
		}
		entry.Evidence = append(entry.Evidence, catalog.Evidence{Kind: item.kind, Ref: item.path, Baseline: issue32BaselineCommit, CaseID: "issue134/" + item.path, InputHash: fmt.Sprintf("sha256:%x", sha256.Sum256(data)), ExecutionMethod: item.run, Expected: item.want, Actual: "PASS; offline public-boundary evidence, not live service verification", Platform: "darwin/linux", CatalogID: id})
		if item.path == "codingagent/user_images_live_test.go" {
			entry.Evidence[len(entry.Evidence)-1].Actual = "PASS 2026-10-08 api.deepseek.com deepseek-flash Responses; local PNG red/blue semantic identification and production-v3 restored image replay; current service evidence, not Pi Oracle"
			entry.Evidence[len(entry.Evidence)-1].Platform = "darwin-arm64"
		}
	}
	path := filepath.Join(root, "parity/catalog.jsonl")
	entries, err := catalog.LoadCatalog(path)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for i, current := range entries {
		if current.ID == id {
			found = true
			if *updateUserImages {
				entries[i] = entry
			} else if !reflect.DeepEqual(current, entry) {
				t.Fatal("user image Catalog evidence drift")
			}
		}
	}
	if !found {
		if !*updateUserImages {
			t.Fatal("missing user image Catalog contract")
		}
		entries = append(entries, entry)
	}
	if *updateUserImages {
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
	data, err := os.ReadFile(filepath.Join(root, "parity/user-images-matrix.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &matrix); err != nil {
		t.Fatal(err)
	}
	if matrix.CatalogID != id {
		t.Fatal("matrix must reference authoritative contract")
	}
	seen := map[string]string{}
	for _, row := range matrix.Rows {
		if row.Result == "" || seen[row.ID] != "" {
			t.Fatal("incomplete/duplicate image matrix")
		}
		seen[row.ID] = row.Version
	}
	for _, id := range []string{"url", "files-api", "animated-webp", "other-providers-generation-edit"} {
		if seen[id] != "V2" {
			t.Fatalf("remaining image branch must remain V2: %s", id)
		}
	}
}

func TestUserImagesAPISnapshot(t *testing.T) {
	got := issue71TypeSnapshot("HeadlessRunOptions", reflect.TypeOf(codingagent.HeadlessRunOptions{})) + "\n" + issue71TypeSnapshot("CreateModelRuntimeOptions", reflect.TypeOf(codingagent.CreateModelRuntimeOptions{})) + "\n"
	for _, fn := range []any{ai.LoadImageFile, ai.ValidateUserImage, ai.ValidateUserImages, ai.DeepSeekVisionModel} {
		got += reflect.TypeOf(fn).String() + "\n"
	}
	path := "testdata/user_images_surface_golden.txt"
	if *updateUserImages {
		if err := os.WriteFile(path, []byte(got), 0644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Fatal("user image public API drift")
	}
}

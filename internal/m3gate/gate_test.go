package m3gate_test

import (
	"github.com/nankedr/pig/internal/catalog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

func TestM3CatalogEvidence(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "../.."))
	entries, err := catalog.LoadCatalog(filepath.Join(root, "parity/catalog.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	var scope []string
	for _, entry := range entries {
		if entry.Milestone != "M3" {
			continue
		}
		scope = append(scope, entry.ID+"\t"+entry.Status)
		if !strings.HasPrefix(entry.ID, "contract:") {
			continue
		}
		t.Run(entry.ID, func(t *testing.T) {
			if entry.ID == "contract:config/models-json" {
				if entry.Status != catalog.StatusInventoried || !strings.Contains(entry.Notes, "M10") {
					t.Error("models.json remains unimplemented; record its M10 boundary explicitly")
				}
				return
			}
			switch entry.Status {
			case catalog.StatusVerified, catalog.StatusImplemented:
			case catalog.StatusPartial:
				if entry.Partial == nil || len(entry.Partial.Supported) == 0 || len(entry.Partial.Unsupported) == 0 || entry.Notes == "" {
					t.Error("partial contract must explain supported and remaining scope")
				}
			default:
				t.Errorf("M3 behavior contract is still %s", entry.Status)
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
						t.Errorf("evidence does not resolve to a test: %s", evidence.Ref)
					}
					replay = true
				}
			}
			if !replay {
				t.Error("M3 contract needs executable Go evidence")
			}
		})
	}
	sort.Strings(scope)
	want, err := os.ReadFile(filepath.Join(root, "internal/m3gate/testdata/catalog_scope.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(scope, "\n")+"\n" != string(want) {
		t.Error("M3 scope/status changed; audit behavioral evidence and remaining boundaries before updating snapshot")
	}
}

func TestM3FreezePlan(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "../.."))
	cmd := exec.Command("make", "-n", "m3-freeze", "PIG_PI_ORACLE_CHECKOUT=/prepared/pi", "PIG_PI_SOURCE_CHECKOUT=/pristine/pi")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("freeze plan: %v\n%s", err, output)
	}
	plan := string(output)
	for _, step := range []string{
		"M3 freeze requires a clean Pig checkout",
		"go test ./... -count=1", "go test -race ./... -count=1", "go vet ./...",
		"CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build ./...",
		"-count=20 -shuffle=on", "-count=5 -shuffle=on",
		"parity/oracle/session-interop.mjs", "parity/oracle/coding-tools.mjs",
		"parity/oracle/usage-cost-cache.mjs", "parity/oracle/agent-proxy.mjs",
		"TestInventoryDriftAgainstUpstream", "parity/extract/surface.mjs",
		"PIG_REQUIRE_LIVE=1 go test ./codingagent -run '^TestDeepSeekLiveHeadlessReadContinuation$'",
	} {
		if !strings.Contains(plan, step) {
			t.Errorf("freeze omits %s", step)
		}
	}
	for _, example := range []string{"session-persistence", "session-interop", "session-navigation", "global-settings", "project-trust", "credentials", "model-runtime", "write-read", "edit-read", "bash-read", "coding-task"} {
		if !strings.Contains(plan, "go run ./examples/"+example) {
			t.Errorf("freeze omits example %s", example)
		}
	}
	if strings.Index(plan, "M3 freeze requires") > strings.Index(plan, "go test ./...") {
		t.Error("clean checkout must be checked before running gates")
	}
}

func TestM3CleanRejectsDirtyCheckout(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	makefile := filepath.Join(filepath.Dir(file), "../../Makefile")
	dir := t.TempDir()
	git := exec.Command("git", "init", "--quiet", dir)
	if output, err := git.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}
	run := func() ([]byte, error) {
		cmd := exec.Command("make", "-f", makefile, "m3-clean")
		cmd.Dir = dir
		return cmd.CombinedOutput()
	}
	if output, err := run(); err != nil {
		t.Fatalf("clean checkout rejected: %v: %s", err, output)
	}
	if err := os.WriteFile(filepath.Join(dir, "untracked.txt"), []byte("dirty"), 0600); err != nil {
		t.Fatal(err)
	}
	if output, err := run(); err == nil || !strings.Contains(string(output), "M3 freeze requires a clean Pig checkout") {
		t.Fatalf("dirty checkout accepted or wrong failure: %v: %s", err, output)
	}
}

func TestM3NodePreflight(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "../.."))
	for _, unicode := range []string{"16.0", "17.0"} {
		t.Run(unicode, func(t *testing.T) {
			dir := t.TempDir()
			script := "#!/bin/sh\nif [ \"$1\" = --experimental-strip-types ]; then exit 0; fi\necho " + unicode + "\n"
			if err := os.WriteFile(filepath.Join(dir, "node"), []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
			cmd := exec.Command("make", "m3-node-preflight")
			cmd.Dir = root
			output, err := cmd.CombinedOutput()
			if unicode == "16.0" && err != nil {
				t.Fatalf("compatible Node rejected: %v: %s", err, output)
			}
			if unicode != "16.0" && (err == nil || !strings.Contains(string(output), "Unicode 16.0")) {
				t.Fatalf("incompatible Node accepted or wrong failure: %v: %s", err, output)
			}
		})
	}
}

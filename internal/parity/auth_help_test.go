package parity_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nankedr/pig/internal/baseline"
	"github.com/nankedr/pig/internal/catalog"
	"github.com/nankedr/pig/internal/parity"
)

func TestCLIAuthHelpParity(t *testing.T) {
	root := parityRepoRoot(t)
	baselineDir := filepath.Join(root, "parity", "baseline")
	if err := baseline.Verify(baselineDir); err != nil {
		t.Fatal(err)
	}
	lock, _, err := baseline.Load(baselineDir)
	if err != nil {
		t.Fatal(err)
	}
	locked := parity.Baseline{ID: lock.BaselineID, Commit: lock.Upstream.Commit, Repository: lock.Upstream.Repository}
	fixture, err := parity.LoadFixture(filepath.Join(root, "parity", "oracle", "fixtures", "codingagent-auth-help.json"), locked)
	if err != nil {
		t.Fatal(err)
	}
	oracle, err := parity.NewFixtureDriver(fixture, locked)
	if err != nil {
		t.Fatal(err)
	}

	temp := t.TempDir()
	binary := filepath.Join(temp, "pig")
	build := exec.Command("go", "build", "-o", binary, "./cmd/pig")
	build.Dir = root
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build pig: %v\n%s", err, output)
	}
	work := filepath.Join(temp, "work")
	home := filepath.Join(temp, "home")
	tempState := filepath.Join(temp, "tmp")
	for _, path := range []string{work, home, tempState} {
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	pig := parity.CommandDriver{
		Path: binary,
		Dir:  work,
		Env: []string{
			"HOME=" + home,
			"TMPDIR=" + tempState,
			"PATH=" + os.Getenv("PATH"),
			"PIG_OFFLINE=1",
			"NO_COLOR=1",
		},
	}

	result, err := parity.RunCase(context.Background(), fixture.Case, oracle, pig)
	if err != nil {
		t.Fatalf("RunCase() = %v; differences=%+v", err, result.Differences)
	}
	if !result.Match || result.Oracle.Stdout == nil || result.Pig.Stdout == nil || *result.Oracle.Stdout == *result.Pig.Stdout {
		t.Fatalf("raw/normalized result = %+v", result)
	}
	assertAuthHelpCatalogEvidence(t, root, locked, fixture, result)
}

func assertAuthHelpCatalogEvidence(t *testing.T, root string, locked parity.Baseline, fixture parity.Fixture, result parity.Result) {
	t.Helper()
	entries, err := catalog.LoadCatalog(filepath.Join(root, "parity", "catalog.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	var entry *catalog.Entry
	for i := range entries {
		if entries[i].ID == fixture.Case.CatalogID {
			entry = &entries[i]
			break
		}
	}
	if entry == nil || entry.Status != catalog.StatusVerified || entry.Upstream.Commit != locked.Commit || entry.Upstream.Repository != locked.Repository || entry.Upstream.Reference != fixture.Upstream.Reference {
		t.Fatalf("catalog entry is not bound to the fixture: %+v", entry)
	}
	pigHash, err := parity.HashObservation(result.Pig)
	if err != nil {
		t.Fatal(err)
	}
	normalizedOracleHash, err := parity.HashObservation(result.NormalizedOracle)
	if err != nil {
		t.Fatal(err)
	}
	normalizedPigHash, err := parity.HashObservation(result.NormalizedPig)
	if err != nil {
		t.Fatal(err)
	}
	if normalizedOracleHash != normalizedPigHash {
		t.Fatalf("normalized hashes differ: %s != %s", normalizedOracleHash, normalizedPigHash)
	}
	evidence := make(map[string]catalog.Evidence, len(entry.Evidence))
	for _, item := range entry.Evidence {
		evidence[item.Kind] = item
	}
	oracle := evidence[catalog.MatrixEvidenceOracle]
	if oracle.Ref != "parity/oracle/fixtures/codingagent-auth-help.json" || oracle.Baseline != locked.Commit || oracle.CaseID != fixture.Case.ID || oracle.InputHash != fixture.InputHash || oracle.Platform != fixture.Platform || oracle.CatalogID != entry.ID || !strings.Contains(oracle.Expected, fixture.ObservationHash) || !strings.Contains(oracle.Actual, fixture.ObservationHash) {
		t.Errorf("oracle evidence does not bind fixture: %+v", oracle)
	}
	goTest := evidence[catalog.MatrixEvidenceGoTest]
	if goTest.Ref != "internal/parity/auth_help_test.go#TestCLIAuthHelpParity" || goTest.Baseline != locked.Commit || goTest.CaseID != fixture.Case.ID || goTest.InputHash != fixture.InputHash || goTest.ExecutionMethod != "go test ./internal/parity -run '^TestCLIAuthHelpParity$' -count=1" || goTest.Platform != fixture.Platform || goTest.CatalogID != entry.ID || !strings.Contains(goTest.Expected, normalizedOracleHash) || !strings.Contains(goTest.Actual, pigHash) || !strings.Contains(goTest.Actual, normalizedPigHash) {
		t.Errorf("Go test evidence does not bind result: %+v", goTest)
	}
}

func parityRepoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

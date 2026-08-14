package baseline_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/nankedr/pig/internal/baseline"
)

const fixedCommit = "936aff00918de1187f085f123c2812d8f2d67745"

// repoBaselineDir locates the real committed parity/baseline directory relative
// to this test file, mirroring the runtime.Caller approach used by the existing
// capability tests.
func repoBaselineDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	return filepath.Join(root, "parity", "baseline")
}

func strptr(s string) *string { return &s }

func validLock() baseline.Lock {
	return baseline.Lock{
		SchemaVersion: "0.1.0",
		BaselineID:    "pi-test",
		Upstream: baseline.Upstream{
			Name:          "Pi",
			Repository:    "https://github.com/badlogic/pi-mono",
			Commit:        fixedCommit,
			License:       "MIT",
			LicenseHolder: "Mario Zechner",
			LicenseYear:   "2025",
		},
		SourceVerification: baseline.SourceVerification{
			Method:                "git-rev-parse",
			ExpectedCommit:        fixedCommit,
			CheckoutPath:          ".upstream/pi",
			NotASubmodule:         true,
			NotARuntimeDependency: true,
		},
		CatalogSnapshot: baseline.CatalogSnapshot{Manifest: "snapshot.manifest.json"},
	}
}

func validPendingManifest() baseline.Manifest {
	return baseline.Manifest{
		SchemaVersion:  "0.1.0",
		BaselineCommit: fixedCommit,
		Status:         baseline.StatusPendingCapture,
		Generation: baseline.Generation{
			GeneratedAt:     nil,
			GeneratorCommit: fixedCommit,
			ToolVersions:    map[string]string{},
			InputSources:    []string{},
		},
		Capture: baseline.Capture{
			Reason:       "commit does not commit full chat-model data; controlled capture pending",
			RequiredStep: "docs/specs/model-catalog.md",
			TrackedBy:    "M0 catalog-capture (future task)",
		},
		Artifacts:       []baseline.Artifact{},
		Providers:       0,
		Models:          0,
		Attribution:     baseline.Attribution{License: "MIT", Holder: "Mario Zechner", Source: "https://github.com/badlogic/pi-mono"},
		ExcludesSecrets: true,
	}
}

// writeBaseline marshals lock+manifest into a fresh temp dir and returns it.
func writeBaseline(t *testing.T, lock baseline.Lock, manifest baseline.Manifest) string {
	t.Helper()
	dir := t.TempDir()
	writeJSON(t, filepath.Join(dir, "upstream.lock.json"), lock)
	name := lock.CatalogSnapshot.Manifest
	if name == "" {
		name = "snapshot.manifest.json"
	}
	writeJSON(t, filepath.Join(dir, name), manifest)
	return dir
}

func writeJSON(t *testing.T, path string, v any) {
	t.Helper()
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatalf("marshal %s: %v", path, err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// assertKind checks the error matches both the sentinel (errors.Is) and the
// typed *baseline.Error carrying the expected Kind (errors.As), mirroring the
// dual identification style of the capability tests.
func assertKind(t *testing.T, err error, sentinel error, kind baseline.Kind) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("errors.Is(%v, %v) = false", err, sentinel)
	}
	var typed *baseline.Error
	if !errors.As(err, &typed) {
		t.Fatalf("errors.As(%v, *baseline.Error) = false", err)
	}
	if typed.Kind != kind {
		t.Fatalf("Kind = %v, want %v (err=%v)", typed.Kind, kind, err)
	}
}

func TestVerifyRealBaseline(t *testing.T) {
	dir := repoBaselineDir(t)
	if err := baseline.Verify(dir); err != nil {
		t.Fatalf("Verify(real baseline) = %v, want nil", err)
	}
}

func TestLoadRealBaseline(t *testing.T) {
	dir := repoBaselineDir(t)
	lock, manifest, err := baseline.Load(dir)
	if err != nil {
		t.Fatalf("Load = %v", err)
	}
	if lock.Upstream.Commit != fixedCommit {
		t.Errorf("lock.Upstream.Commit = %q, want %q", lock.Upstream.Commit, fixedCommit)
	}
	if manifest.BaselineCommit != fixedCommit {
		t.Errorf("manifest.BaselineCommit = %q, want %q", manifest.BaselineCommit, fixedCommit)
	}
	if manifest.Status != baseline.StatusPendingCapture {
		t.Errorf("manifest.Status = %q, want %q", manifest.Status, baseline.StatusPendingCapture)
	}
	if manifest.Generation.GeneratedAt != nil {
		t.Errorf("real pending manifest generated_at = %v, want nil", *manifest.Generation.GeneratedAt)
	}
	if len(manifest.Artifacts) != 0 {
		t.Errorf("real pending manifest artifacts = %v, want empty", manifest.Artifacts)
	}
}

func TestVerifyCommitMismatchInLock(t *testing.T) {
	lock := validLock()
	lock.Upstream.Commit = "0000000000000000000000000000000000000000"
	dir := writeBaseline(t, lock, validPendingManifest())
	assertKind(t, baseline.Verify(dir), baseline.ErrCommitMismatch, baseline.KindCommitMismatch)
}

func TestVerifyBaselineCommitMismatch(t *testing.T) {
	manifest := validPendingManifest()
	manifest.BaselineCommit = "1111111111111111111111111111111111111111"
	dir := writeBaseline(t, validLock(), manifest)
	assertKind(t, baseline.Verify(dir), baseline.ErrCommitMismatch, baseline.KindCommitMismatch)
}

func TestVerifyCheckout(t *testing.T) {
	dir := writeBaseline(t, validLock(), validPendingManifest())

	t.Run("match", func(t *testing.T) {
		resolver := func(path string) (string, error) { return fixedCommit, nil }
		err := baseline.Verify(dir, baseline.WithCheckout("/fake/checkout"), baseline.WithCommitResolver(resolver))
		if err != nil {
			t.Fatalf("Verify with matching HEAD = %v, want nil", err)
		}
	})

	t.Run("mismatch", func(t *testing.T) {
		resolver := func(path string) (string, error) {
			return "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef", nil
		}
		err := baseline.Verify(dir, baseline.WithCheckout("/fake/checkout"), baseline.WithCommitResolver(resolver))
		assertKind(t, err, baseline.ErrCommitMismatch, baseline.KindCommitMismatch)
	})

	t.Run("resolver error", func(t *testing.T) {
		sentinel := errors.New("boom")
		resolver := func(path string) (string, error) { return "", sentinel }
		err := baseline.Verify(dir, baseline.WithCheckout("/fake/checkout"), baseline.WithCommitResolver(resolver))
		if !errors.Is(err, sentinel) {
			t.Fatalf("Verify propagates resolver error = %v", err)
		}
	})

	t.Run("no checkout skips git", func(t *testing.T) {
		// A resolver that fails loudly proves the git step is skipped when no
		// checkout path is supplied.
		resolver := func(path string) (string, error) {
			t.Fatalf("resolver must not run without a checkout path")
			return "", nil
		}
		if err := baseline.Verify(dir, baseline.WithCommitResolver(resolver)); err != nil {
			t.Fatalf("Verify without checkout = %v, want nil", err)
		}
	})
}

func TestVerifyCapturedManifest(t *testing.T) {
	data := []byte(`{"providers":[{"id":"deepseek"}]}`)

	build := func(t *testing.T) (string, baseline.Manifest) {
		dir := t.TempDir()
		writeJSON(t, filepath.Join(dir, "upstream.lock.json"), validLock())
		if err := os.WriteFile(filepath.Join(dir, "chat-models.json"), data, 0o600); err != nil {
			t.Fatal(err)
		}
		m := validPendingManifest()
		m.Status = baseline.StatusCaptured
		m.Generation.GeneratedAt = strptr("2026-08-14T00:00:00Z")
		m.Generation.InputSources = []string{"https://github.com/badlogic/pi-mono"}
		m.Providers = 1
		m.Models = 1
		m.Artifacts = []baseline.Artifact{{Path: "chat-models.json", SHA256: sha256Hex(data)}}
		return dir, m
	}

	t.Run("valid hash", func(t *testing.T) {
		dir, m := build(t)
		writeJSON(t, filepath.Join(dir, "snapshot.manifest.json"), m)
		if err := baseline.Verify(dir); err != nil {
			t.Fatalf("Verify captured (valid hash) = %v, want nil", err)
		}
	})

	t.Run("wrong hash", func(t *testing.T) {
		dir, m := build(t)
		m.Artifacts[0].SHA256 = sha256Hex([]byte("different"))
		writeJSON(t, filepath.Join(dir, "snapshot.manifest.json"), m)
		assertKind(t, baseline.Verify(dir), baseline.ErrHashMismatch, baseline.KindHashMismatch)
	})

	t.Run("missing hash", func(t *testing.T) {
		dir, m := build(t)
		m.Artifacts[0].SHA256 = ""
		writeJSON(t, filepath.Join(dir, "snapshot.manifest.json"), m)
		assertKind(t, baseline.Verify(dir), baseline.ErrMissingHash, baseline.KindMissingHash)
	})

	t.Run("missing generated_at", func(t *testing.T) {
		dir, m := build(t)
		m.Generation.GeneratedAt = nil
		writeJSON(t, filepath.Join(dir, "snapshot.manifest.json"), m)
		assertKind(t, baseline.Verify(dir), baseline.ErrMissingField, baseline.KindMissingField)
	})

	t.Run("empty input sources", func(t *testing.T) {
		dir, m := build(t)
		m.Generation.InputSources = []string{}
		writeJSON(t, filepath.Join(dir, "snapshot.manifest.json"), m)
		assertKind(t, baseline.Verify(dir), baseline.ErrMissingField, baseline.KindMissingField)
	})
}

func TestVerifyPendingDishonest(t *testing.T) {
	t.Run("fabricated timestamp", func(t *testing.T) {
		m := validPendingManifest()
		m.Generation.GeneratedAt = strptr("2026-08-14T00:00:00Z")
		dir := writeBaseline(t, validLock(), m)
		assertKind(t, baseline.Verify(dir), baseline.ErrDishonestPending, baseline.KindDishonestPending)
	})

	t.Run("fabricated artifact", func(t *testing.T) {
		m := validPendingManifest()
		m.Artifacts = []baseline.Artifact{{Path: "chat-models.json", SHA256: sha256Hex([]byte("x"))}}
		dir := writeBaseline(t, validLock(), m)
		assertKind(t, baseline.Verify(dir), baseline.ErrDishonestPending, baseline.KindDishonestPending)
	})

	t.Run("empty reason", func(t *testing.T) {
		m := validPendingManifest()
		m.Capture.Reason = ""
		dir := writeBaseline(t, validLock(), m)
		assertKind(t, baseline.Verify(dir), baseline.ErrMissingField, baseline.KindMissingField)
	})
}

func TestVerifyIllegalStatus(t *testing.T) {
	m := validPendingManifest()
	m.Status = "bogus"
	dir := writeBaseline(t, validLock(), m)
	assertKind(t, baseline.Verify(dir), baseline.ErrIllegalStatus, baseline.KindIllegalStatus)
}

func TestVerifyNotIndependent(t *testing.T) {
	t.Run("submodule", func(t *testing.T) {
		lock := validLock()
		lock.SourceVerification.NotASubmodule = false
		dir := writeBaseline(t, lock, validPendingManifest())
		assertKind(t, baseline.Verify(dir), baseline.ErrNotIndependent, baseline.KindNotIndependent)
	})

	t.Run("runtime dependency", func(t *testing.T) {
		lock := validLock()
		lock.SourceVerification.NotARuntimeDependency = false
		dir := writeBaseline(t, lock, validPendingManifest())
		assertKind(t, baseline.Verify(dir), baseline.ErrNotIndependent, baseline.KindNotIndependent)
	})
}

func TestVerifyMissingField(t *testing.T) {
	for _, tt := range []struct {
		name   string
		mutate func(*baseline.Lock, *baseline.Manifest)
	}{
		{"lock schema_version", func(l *baseline.Lock, _ *baseline.Manifest) { l.SchemaVersion = "" }},
		{"upstream repository", func(l *baseline.Lock, _ *baseline.Manifest) { l.Upstream.Repository = "" }},
		{"upstream commit", func(l *baseline.Lock, _ *baseline.Manifest) { l.Upstream.Commit = "" }},
		{"expected commit", func(l *baseline.Lock, _ *baseline.Manifest) { l.SourceVerification.ExpectedCommit = "" }},
		{"manifest schema_version", func(_ *baseline.Lock, m *baseline.Manifest) { m.SchemaVersion = "" }},
		{"baseline commit", func(_ *baseline.Lock, m *baseline.Manifest) { m.BaselineCommit = "" }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			lock := validLock()
			manifest := validPendingManifest()
			tt.mutate(&lock, &manifest)
			dir := writeBaseline(t, lock, manifest)
			assertKind(t, baseline.Verify(dir), baseline.ErrMissingField, baseline.KindMissingField)
		})
	}
}

func TestVerifyMissingFiles(t *testing.T) {
	t.Run("missing lock", func(t *testing.T) {
		dir := t.TempDir()
		writeJSON(t, filepath.Join(dir, "snapshot.manifest.json"), validPendingManifest())
		if err := baseline.Verify(dir); err == nil {
			t.Fatal("Verify with missing lock = nil, want error")
		}
	})
	t.Run("missing manifest", func(t *testing.T) {
		dir := t.TempDir()
		writeJSON(t, filepath.Join(dir, "upstream.lock.json"), validLock())
		if err := baseline.Verify(dir); err == nil {
			t.Fatal("Verify with missing manifest = nil, want error")
		}
	})
}

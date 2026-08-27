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

const (
	fixedCommit   = "936aff00918de1187f085f123c2812d8f2d67745"
	catalogCommit = "53fa77ccd8a279eb87e92294ef3687b03ff80112"
)

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
		SchemaVersion: "0.2.0",
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
		CatalogSnapshot: baseline.CatalogSnapshot{
			Manifest: "snapshot.manifest.json", SourceCommit: catalogCommit,
			SourceRelease: "v0.84.1", SourceReleaseSHA256: sha256Hex([]byte("release")),
			SourceCommitsBehind: 40,
		},
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

func buildCapturedBaseline(t *testing.T) (string, baseline.Manifest) {
	t.Helper()
	dir := t.TempDir()
	writeJSON(t, filepath.Join(dir, "upstream.lock.json"), validLock())

	sourceDir := filepath.Join(dir, "catalog", "chat", "source")
	providersDir := filepath.Join(sourceDir, "providers")
	if err := os.MkdirAll(providersDir, 0o700); err != nil {
		t.Fatal(err)
	}
	sourceModel := map[string]any{
		"id": "model", "name": "Model", "api": "openai-responses", "provider": "provider",
		"baseUrl": "https://example.invalid", "reasoning": false, "input": []string{"text"},
		"cost":          map[string]any{"input": 0, "output": 0, "cacheRead": 0, "cacheWrite": 0},
		"contextWindow": 1, "maxTokens": 1,
	}
	shardPath := filepath.Join(providersDir, "provider.json")
	writeJSON(t, shardPath, map[string]any{"openai-responses": map[string]any{"model": sourceModel}})
	structure := map[string]map[string]string{"provider": {"model": "openai-responses"}}
	sourceManifestPath := filepath.Join(sourceDir, "manifest.json")
	writeJSON(t, sourceManifestPath, map[string]any{
		"schemaVersion": 3,
		"generatedAt":   "2026-08-07T05:51:06.002Z",
		"structureHash": jsonHash(t, structure),
		"files":         map[string]string{"provider.json": fileHash(t, shardPath)},
	})

	rulesPath := filepath.Join(dir, "catalog", "chat", "derivation.json")
	writeJSON(t, rulesPath, map[string]any{
		"schema_version":    "0.1.0",
		"source_commit":     catalogCommit,
		"semantic_overlays": []any{},
		"rules": []any{map[string]any{
			"operation": "flatten-api-groups", "preserves_fields_and_values": true,
			"ordering": []string{"provider", "model_id"},
		}},
	})

	derivedModel := cloneMap(t, sourceModel)
	modelsPath := filepath.Join(dir, "catalog", "chat", "models.json")
	writeJSON(t, modelsPath, map[string]any{"provider": map[string]any{"model": derivedModel}})
	if err := os.MkdirAll(filepath.Join(dir, "catalog", "image"), 0o700); err != nil {
		t.Fatal(err)
	}
	imagePath := filepath.Join(dir, "catalog", "image", "models.json")
	writeJSON(t, imagePath, map[string]any{"image-provider": map[string]any{"image-model": map[string]any{
		"id": "image-model", "name": "Image Model", "api": "openrouter-images", "provider": "image-provider",
		"baseUrl": "https://example.invalid", "input": []string{"text"}, "output": []string{"image"},
		"cost": map[string]any{"input": -1000000, "output": 0, "cacheRead": 0, "cacheWrite": 0},
	}}})

	m := validPendingManifest()
	m.SchemaVersion = "0.2.0"
	m.Status = baseline.StatusCaptured
	m.Generation.GeneratedAt = strptr("2026-08-07T05:51:06.002Z")
	m.Generation.CapturedAt = strptr("2026-08-27T04:30:00Z")
	m.Generation.GeneratorCommit = catalogCommit
	m.Generation.Method = "extract-official-release-and-losslessly-flatten"
	m.Generation.ToolVersions = map[string]string{"node": "v22.23.2", "npm": "10.9.8"}
	m.Generation.InputSources = []string{"https://example.invalid/pi-0.84.1-source.tar.gz"}
	m.CatalogSource = baseline.CatalogSource{
		Type: "github-release-source-tar", Release: "v0.84.1", Commit: catalogCommit,
		URL: "https://example.invalid/pi-0.84.1-source.tar.gz", SHA256: sha256Hex([]byte("release")),
		CommitsBehindCodeBaseline: 40, Manifest: "catalog/chat/source/manifest.json",
		ManifestSHA256: fileHash(t, sourceManifestPath), StructureSHA256: jsonHash(t, structure),
	}
	m.Attribution.Source = m.CatalogSource.URL
	m.Derivation = baseline.Derivation{
		Method: "lossless-flatten-provider-api-shards", SourceCommit: catalogCommit,
		Rules: "catalog/chat/derivation.json", RuleCount: 1, SemanticOverlays: 0,
		Result: "catalog/chat/models.json", ResultSHA256: fileHash(t, modelsPath),
	}
	m.Image = baseline.ImageSnapshot{
		SourceCommit: fixedCommit, SourcePath: "packages/ai/src/image-models.generated.ts",
		SourceSHA256:  sha256Hex([]byte("image source")),
		GeneratorPath: "packages/ai/scripts/generate-image-models.ts", GeneratorSHA256: sha256Hex([]byte("image generator")),
		Method: "node-import-json-stringify", Artifact: "catalog/image/models.json",
		Providers: 1, Models: 1,
	}
	m.Providers = 1
	m.Models = 1
	m.Artifacts = []baseline.Artifact{
		{Path: "catalog/chat/source/manifest.json", SHA256: fileHash(t, sourceManifestPath), Role: "source-manifest"},
		{Path: "catalog/chat/derivation.json", SHA256: fileHash(t, rulesPath), Role: "derivation-rules"},
		{Path: "catalog/chat/models.json", SHA256: fileHash(t, modelsPath), Role: "derived-chat-catalog"},
		{Path: "catalog/image/models.json", SHA256: fileHash(t, imagePath), Role: "image-catalog"},
	}
	return dir, m
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

func fileHash(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return sha256Hex(data)
}

func jsonHash(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return sha256Hex(data)
}

func cloneMap(t *testing.T, value map[string]any) map[string]any {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var cloned map[string]any
	if err := json.Unmarshal(data, &cloned); err != nil {
		t.Fatal(err)
	}
	return cloned
}

func setArtifactHash(t *testing.T, manifest *baseline.Manifest, path, hash string) {
	t.Helper()
	for i := range manifest.Artifacts {
		if manifest.Artifacts[i].Path == path {
			manifest.Artifacts[i].SHA256 = hash
			return
		}
	}
	t.Fatalf("artifact %q not found", path)
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
	if lock.SchemaVersion != "0.2.0" || manifest.SchemaVersion != "0.2.0" {
		t.Errorf("schemas = %q/%q, want 0.2.0/0.2.0", lock.SchemaVersion, manifest.SchemaVersion)
	}
	if lock.BaselineID != "pi-936aff0-catalog-v0.84.1-v1" {
		t.Errorf("baseline ID = %q, want dual-source ID", lock.BaselineID)
	}
	if manifest.BaselineCommit != fixedCommit {
		t.Errorf("manifest.BaselineCommit = %q, want %q", manifest.BaselineCommit, fixedCommit)
	}
	if manifest.Status != baseline.StatusCaptured {
		t.Errorf("manifest.Status = %q, want %q", manifest.Status, baseline.StatusCaptured)
	}
	if manifest.Generation.GeneratorCommit != catalogCommit {
		t.Errorf("generator commit = %q, want catalog source %q", manifest.Generation.GeneratorCommit, catalogCommit)
	}
	if manifest.CatalogSource.Commit != catalogCommit || manifest.CatalogSource.CommitsBehindCodeBaseline != 40 {
		t.Errorf("catalog source = %q/%d, want %q/40", manifest.CatalogSource.Commit, manifest.CatalogSource.CommitsBehindCodeBaseline, catalogCommit)
	}
	if manifest.Providers != 39 || manifest.Models != 1220 {
		t.Errorf("chat counts = %d/%d, want 39/1220", manifest.Providers, manifest.Models)
	}
	if manifest.Image.Providers != 1 || manifest.Image.Models != 42 {
		t.Errorf("image counts = %d/%d, want 1/42", manifest.Image.Providers, manifest.Image.Models)
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
	t.Run("valid hash", func(t *testing.T) {
		dir, m := buildCapturedBaseline(t)
		writeJSON(t, filepath.Join(dir, "snapshot.manifest.json"), m)
		if err := baseline.Verify(dir); err != nil {
			t.Fatalf("Verify captured (valid hash) = %v, want nil", err)
		}
	})

	t.Run("wrong hash", func(t *testing.T) {
		dir, m := buildCapturedBaseline(t)
		m.Artifacts[0].SHA256 = sha256Hex([]byte("different"))
		writeJSON(t, filepath.Join(dir, "snapshot.manifest.json"), m)
		assertKind(t, baseline.Verify(dir), baseline.ErrHashMismatch, baseline.KindHashMismatch)
	})

	t.Run("missing hash", func(t *testing.T) {
		dir, m := buildCapturedBaseline(t)
		m.Artifacts[0].SHA256 = ""
		writeJSON(t, filepath.Join(dir, "snapshot.manifest.json"), m)
		assertKind(t, baseline.Verify(dir), baseline.ErrMissingHash, baseline.KindMissingHash)
	})

	t.Run("missing generated_at", func(t *testing.T) {
		dir, m := buildCapturedBaseline(t)
		m.Generation.GeneratedAt = nil
		writeJSON(t, filepath.Join(dir, "snapshot.manifest.json"), m)
		assertKind(t, baseline.Verify(dir), baseline.ErrMissingField, baseline.KindMissingField)
	})

	t.Run("empty input sources", func(t *testing.T) {
		dir, m := buildCapturedBaseline(t)
		m.Generation.InputSources = []string{}
		writeJSON(t, filepath.Join(dir, "snapshot.manifest.json"), m)
		assertKind(t, baseline.Verify(dir), baseline.ErrMissingField, baseline.KindMissingField)
	})

	t.Run("tampered source shard", func(t *testing.T) {
		dir, m := buildCapturedBaseline(t)
		writeJSON(t, filepath.Join(dir, "snapshot.manifest.json"), m)
		if err := os.WriteFile(filepath.Join(dir, "catalog", "chat", "source", "providers", "provider.json"), []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
		assertKind(t, baseline.Verify(dir), baseline.ErrHashMismatch, baseline.KindHashMismatch)
	})

	t.Run("derived catalog must match lossless flatten", func(t *testing.T) {
		dir, m := buildCapturedBaseline(t)
		modelsPath := filepath.Join(dir, "catalog", "chat", "models.json")
		writeJSON(t, modelsPath, map[string]any{"provider": map[string]any{"model": map[string]any{
			"id": "model", "provider": "provider", "api": "openai-responses",
		}}})
		m.Derivation.ResultSHA256 = fileHash(t, modelsPath)
		for i := range m.Artifacts {
			if m.Artifacts[i].Path == m.Derivation.Result {
				m.Artifacts[i].SHA256 = m.Derivation.ResultSHA256
			}
		}
		writeJSON(t, filepath.Join(dir, "snapshot.manifest.json"), m)
		assertKind(t, baseline.Verify(dir), baseline.ErrCatalogMismatch, baseline.KindCatalogMismatch)
	})

	t.Run("manifest count", func(t *testing.T) {
		dir, m := buildCapturedBaseline(t)
		m.Models++
		writeJSON(t, filepath.Join(dir, "snapshot.manifest.json"), m)
		assertKind(t, baseline.Verify(dir), baseline.ErrCatalogMismatch, baseline.KindCatalogMismatch)
	})

	t.Run("duplicate model across APIs", func(t *testing.T) {
		dir, m := buildCapturedBaseline(t)
		shardPath := filepath.Join(dir, "catalog", "chat", "source", "providers", "provider.json")
		var shard map[string]map[string]map[string]any
		data, err := os.ReadFile(shardPath)
		if err != nil {
			t.Fatalf("read shard: %v", err)
		}
		if err := json.Unmarshal(data, &shard); err != nil {
			t.Fatalf("decode shard: %v", err)
		}
		duplicate := cloneMap(t, shard["openai-responses"]["model"])
		duplicate["api"] = "other-api"
		shard["other-api"] = map[string]map[string]any{"model": duplicate}
		writeJSON(t, shardPath, shard)

		sourceManifestPath := filepath.Join(dir, m.CatalogSource.Manifest)
		var sourceManifest map[string]any
		data, err = os.ReadFile(sourceManifestPath)
		if err != nil {
			t.Fatalf("read source manifest: %v", err)
		}
		if err := json.Unmarshal(data, &sourceManifest); err != nil {
			t.Fatalf("decode source manifest: %v", err)
		}
		sourceManifest["files"].(map[string]any)["provider.json"] = fileHash(t, shardPath)
		writeJSON(t, sourceManifestPath, sourceManifest)
		m.CatalogSource.ManifestSHA256 = fileHash(t, sourceManifestPath)
		setArtifactHash(t, &m, m.CatalogSource.Manifest, m.CatalogSource.ManifestSHA256)
		writeJSON(t, filepath.Join(dir, "snapshot.manifest.json"), m)
		assertKind(t, baseline.Verify(dir), baseline.ErrCatalogMismatch, baseline.KindCatalogMismatch)
	})

	t.Run("unknown provider reference", func(t *testing.T) {
		dir, m := buildCapturedBaseline(t)
		modelsPath := filepath.Join(dir, m.Derivation.Result)
		var models map[string]map[string]map[string]any
		data, err := os.ReadFile(modelsPath)
		if err != nil {
			t.Fatalf("read models: %v", err)
		}
		if err := json.Unmarshal(data, &models); err != nil {
			t.Fatalf("decode models: %v", err)
		}
		models["provider"]["model"]["provider"] = "unknown"
		writeJSON(t, modelsPath, models)
		m.Derivation.ResultSHA256 = fileHash(t, modelsPath)
		setArtifactHash(t, &m, m.Derivation.Result, m.Derivation.ResultSHA256)
		writeJSON(t, filepath.Join(dir, "snapshot.manifest.json"), m)
		assertKind(t, baseline.Verify(dir), baseline.ErrCatalogMismatch, baseline.KindCatalogMismatch)
	})

	t.Run("release provenance hash", func(t *testing.T) {
		dir, m := buildCapturedBaseline(t)
		m.CatalogSource.SHA256 = sha256Hex([]byte("other release"))
		writeJSON(t, filepath.Join(dir, "snapshot.manifest.json"), m)
		assertKind(t, baseline.Verify(dir), baseline.ErrHashMismatch, baseline.KindHashMismatch)
	})

	t.Run("generator provenance cannot claim code baseline", func(t *testing.T) {
		dir, m := buildCapturedBaseline(t)
		m.Generation.GeneratorCommit = fixedCommit
		writeJSON(t, filepath.Join(dir, "snapshot.manifest.json"), m)
		assertKind(t, baseline.Verify(dir), baseline.ErrCommitMismatch, baseline.KindCommitMismatch)
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

func TestVerifyRejectsManifestOutsideBaseline(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "baseline")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	writeJSON(t, filepath.Join(root, "outside.json"), validPendingManifest())
	lock := validLock()
	lock.CatalogSnapshot.Manifest = "../outside.json"
	writeJSON(t, filepath.Join(dir, "upstream.lock.json"), lock)
	if err := baseline.Verify(dir); err == nil {
		t.Fatal("Verify accepted a manifest outside the baseline directory")
	}
}

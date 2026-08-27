package baseline

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"
)

type releasedManifest struct {
	SchemaVersion int               `json:"schemaVersion"`
	GeneratedAt   string            `json:"generatedAt"`
	StructureHash string            `json:"structureHash"`
	Files         map[string]string `json:"files"`
}

type derivationRules struct {
	SchemaVersion    string            `json:"schema_version"`
	SourceCommit     string            `json:"source_commit"`
	SemanticOverlays []json.RawMessage `json:"semantic_overlays"`
	Rules            []derivationRule  `json:"rules"`
}

type derivationRule struct {
	Operation                string   `json:"operation"`
	PreservesFieldsAndValues bool     `json:"preserves_fields_and_values"`
	Ordering                 []string `json:"ordering"`
}

type flatCatalog map[string]map[string]map[string]any

func verifyArtifacts(dir string, artifacts []Artifact) (map[string]string, error) {
	verified := make(map[string]string, len(artifacts))
	for _, artifact := range artifacts {
		if strings.TrimSpace(artifact.Path) == "" {
			return nil, newError(KindMissingField, "captured artifact requires a path")
		}
		if strings.TrimSpace(artifact.SHA256) == "" {
			return nil, newError(KindMissingHash, "artifact %q has no sha256", artifact.Path)
		}
		if strings.TrimSpace(artifact.Role) == "" {
			return nil, newError(KindMissingField, "artifact %q has no role", artifact.Path)
		}
		if err := validSHA256(artifact.SHA256); err != nil {
			return nil, wrapError(KindHashMismatch, err, "artifact %q has invalid sha256", artifact.Path)
		}
		if _, exists := verified[artifact.Path]; exists {
			return nil, newError(KindCatalogMismatch, "duplicate artifact path %q", artifact.Path)
		}
		path, err := secureFile(dir, artifact.Path)
		if err != nil {
			return nil, wrapError(KindHashMismatch, err, "artifact %q unreadable", artifact.Path)
		}
		actual, err := hashFile(path)
		if err != nil {
			return nil, wrapError(KindHashMismatch, err, "artifact %q unreadable", artifact.Path)
		}
		if !strings.EqualFold(actual, strings.TrimSpace(artifact.SHA256)) {
			return nil, newError(KindHashMismatch,
				"artifact %q sha256 %s != recorded %s", artifact.Path, actual, artifact.SHA256)
		}
		verified[artifact.Path] = actual
	}
	return verified, nil
}

func verifySnapshotData(dir string, lock *Lock, manifest *Manifest, artifacts map[string]string) error {
	if err := verifyProvenance(lock, manifest, artifacts); err != nil {
		return err
	}
	source, sourceManifest, err := loadReleasedCatalog(dir, manifest)
	if err != nil {
		return err
	}
	if err := verifyLosslessDerivation(dir, manifest, artifacts, source); err != nil {
		return err
	}
	if err := verifyImageCatalog(dir, manifest); err != nil {
		return err
	}
	if *manifest.Generation.GeneratedAt != sourceManifest.GeneratedAt {
		return newError(KindCatalogMismatch, "generation.generated_at %q != release manifest %q",
			*manifest.Generation.GeneratedAt, sourceManifest.GeneratedAt)
	}
	return nil
}

func verifyProvenance(lock *Lock, manifest *Manifest, artifacts map[string]string) error {
	for _, field := range []struct{ name, value string }{
		{"generation.generator_commit", manifest.Generation.GeneratorCommit},
		{"generation.method", manifest.Generation.Method},
		{"catalog_source.type", manifest.CatalogSource.Type},
		{"catalog_source.release", manifest.CatalogSource.Release},
		{"catalog_source.commit", manifest.CatalogSource.Commit},
		{"catalog_source.url", manifest.CatalogSource.URL},
		{"catalog_source.manifest", manifest.CatalogSource.Manifest},
		{"derivation.method", manifest.Derivation.Method},
		{"derivation.source_commit", manifest.Derivation.SourceCommit},
		{"derivation.rules", manifest.Derivation.Rules},
		{"derivation.result", manifest.Derivation.Result},
		{"image.source_commit", manifest.Image.SourceCommit},
		{"image.source_path", manifest.Image.SourcePath},
		{"image.generator_path", manifest.Image.GeneratorPath},
		{"image.method", manifest.Image.Method},
		{"image.artifact", manifest.Image.Artifact},
		{"attribution.license", manifest.Attribution.License},
		{"attribution.holder", manifest.Attribution.Holder},
		{"attribution.source", manifest.Attribution.Source},
		{"lock.catalog_snapshot.source_commit", lock.CatalogSnapshot.SourceCommit},
		{"lock.catalog_snapshot.source_release", lock.CatalogSnapshot.SourceRelease},
	} {
		if strings.TrimSpace(field.value) == "" {
			return newError(KindMissingField, "%s is empty", field.name)
		}
	}
	if !timestampOK(*manifest.Generation.GeneratedAt) || !timestampOK(*manifest.Generation.CapturedAt) {
		return newError(KindCatalogMismatch, "generation timestamps must be RFC3339")
	}
	for name, value := range manifest.Generation.ToolVersions {
		if strings.TrimSpace(name) == "" || strings.TrimSpace(value) == "" {
			return newError(KindMissingField, "generation.tool_versions contains an empty key or value")
		}
	}
	foundSource := false
	for _, source := range manifest.Generation.InputSources {
		if strings.TrimSpace(source) == "" {
			return newError(KindMissingField, "generation.input_sources contains an empty value")
		}
		foundSource = foundSource || source == manifest.CatalogSource.URL
	}
	if !foundSource {
		return newError(KindCatalogMismatch, "generation.input_sources does not include catalog_source.url")
	}
	if strings.TrimSpace(manifest.Capture.Reason) == "" {
		return newError(KindMissingField, "captured manifest requires capture.reason")
	}
	if manifest.Providers <= 0 || manifest.Models <= 0 || manifest.Image.Providers <= 0 || manifest.Image.Models <= 0 {
		return newError(KindCatalogMismatch, "captured provider/model counts must be positive")
	}
	if manifest.CatalogSource.CommitsBehindCodeBaseline <= 0 ||
		manifest.CatalogSource.CommitsBehindCodeBaseline != lock.CatalogSnapshot.SourceCommitsBehind {
		return newError(KindCatalogMismatch, "catalog source commit distance %d != lock %d",
			manifest.CatalogSource.CommitsBehindCodeBaseline, lock.CatalogSnapshot.SourceCommitsBehind)
	}
	if commitEqual(manifest.CatalogSource.Commit, manifest.BaselineCommit) {
		return newError(KindCommitMismatch, "catalog source commit must remain distinct from code baseline")
	}
	for name, actual := range map[string]string{
		"generation.generator_commit":         manifest.Generation.GeneratorCommit,
		"derivation.source_commit":            manifest.Derivation.SourceCommit,
		"lock.catalog_snapshot.source_commit": lock.CatalogSnapshot.SourceCommit,
	} {
		if !commitEqual(actual, manifest.CatalogSource.Commit) {
			return newError(KindCommitMismatch, "%s %q != catalog source commit %q", name, actual, manifest.CatalogSource.Commit)
		}
	}
	if !commitEqual(manifest.Image.SourceCommit, manifest.BaselineCommit) {
		return newError(KindCommitMismatch, "image source commit %q != code baseline %q",
			manifest.Image.SourceCommit, manifest.BaselineCommit)
	}
	if manifest.CatalogSource.Release != lock.CatalogSnapshot.SourceRelease {
		return newError(KindCatalogMismatch, "catalog release %q != lock %q",
			manifest.CatalogSource.Release, lock.CatalogSnapshot.SourceRelease)
	}
	if manifest.Attribution.License != lock.Upstream.License ||
		manifest.Attribution.Holder != lock.Upstream.LicenseHolder ||
		manifest.Attribution.Source != manifest.CatalogSource.URL {
		return newError(KindCatalogMismatch, "catalog attribution does not match its locked source")
	}
	for name, value := range map[string]string{
		"catalog_source.sha256":                manifest.CatalogSource.SHA256,
		"catalog_source.manifest_sha256":       manifest.CatalogSource.ManifestSHA256,
		"catalog_source.structure_sha256":      manifest.CatalogSource.StructureSHA256,
		"derivation.result_sha256":             manifest.Derivation.ResultSHA256,
		"image.source_sha256":                  manifest.Image.SourceSHA256,
		"image.generator_sha256":               manifest.Image.GeneratorSHA256,
		"lock.catalog_snapshot.release_sha256": lock.CatalogSnapshot.SourceReleaseSHA256,
	} {
		if err := validSHA256(value); err != nil {
			return wrapError(KindHashMismatch, err, "%s is invalid", name)
		}
	}
	if !strings.EqualFold(manifest.CatalogSource.SHA256, lock.CatalogSnapshot.SourceReleaseSHA256) {
		return newError(KindHashMismatch, "catalog release sha256 %s != lock %s",
			manifest.CatalogSource.SHA256, lock.CatalogSnapshot.SourceReleaseSHA256)
	}
	for path, expected := range map[string]string{
		manifest.CatalogSource.Manifest: manifest.CatalogSource.ManifestSHA256,
		manifest.Derivation.Result:      manifest.Derivation.ResultSHA256,
		manifest.Derivation.Rules:       artifacts[manifest.Derivation.Rules],
		manifest.Image.Artifact:         artifacts[manifest.Image.Artifact],
	} {
		actual, ok := artifacts[path]
		if !ok || !strings.EqualFold(actual, expected) {
			return newError(KindHashMismatch, "referenced artifact %q is missing or has inconsistent hash", path)
		}
	}
	if manifest.Derivation.RuleCount != 1 || manifest.Derivation.SemanticOverlays != 0 {
		return newError(KindCatalogMismatch, "derivation must be lossless and contain no semantic overlays")
	}
	return nil
}

func loadReleasedCatalog(dir string, manifest *Manifest) (flatCatalog, *releasedManifest, error) {
	manifestPath, err := secureFile(dir, manifest.CatalogSource.Manifest)
	if err != nil {
		return nil, nil, wrapError(KindHashMismatch, err, "release manifest unreadable")
	}
	var released releasedManifest
	if err := readJSON(manifestPath, &released); err != nil {
		return nil, nil, wrapError(KindCatalogMismatch, err, "decode release manifest")
	}
	if released.SchemaVersion != 3 || !timestampOK(released.GeneratedAt) || len(released.Files) == 0 {
		return nil, nil, newError(KindCatalogMismatch, "invalid released model-data manifest")
	}
	if !strings.EqualFold(released.StructureHash, manifest.CatalogSource.StructureSHA256) {
		return nil, nil, newError(KindHashMismatch, "release structure hash %s != recorded %s",
			released.StructureHash, manifest.CatalogSource.StructureSHA256)
	}

	providersDir := filepath.Join(filepath.Dir(manifestPath), "providers")
	info, err := os.Lstat(providersDir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, nil, newError(KindCatalogMismatch, "released provider directory is missing or invalid")
	}
	entries, err := os.ReadDir(providersDir)
	if err != nil {
		return nil, nil, wrapError(KindCatalogMismatch, err, "read released provider directory")
	}
	actualFiles := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			return nil, nil, newError(KindCatalogMismatch, "invalid provider artifact %q", entry.Name())
		}
		actualFiles = append(actualFiles, entry.Name())
	}
	expectedFiles := make([]string, 0, len(released.Files))
	for name := range released.Files {
		expectedFiles = append(expectedFiles, name)
	}
	sort.Strings(actualFiles)
	sort.Strings(expectedFiles)
	if !reflect.DeepEqual(actualFiles, expectedFiles) {
		return nil, nil, newError(KindCatalogMismatch, "released provider file set does not match its manifest")
	}

	flat := make(flatCatalog, len(expectedFiles))
	structure := make(map[string]map[string]string, len(expectedFiles))
	modelCount := 0
	for _, name := range expectedFiles {
		if filepath.Base(name) != name || !strings.HasSuffix(name, ".json") {
			return nil, nil, newError(KindCatalogMismatch, "invalid provider filename %q", name)
		}
		provider := strings.TrimSuffix(name, ".json")
		path, err := secureFile(providersDir, name)
		if err != nil {
			return nil, nil, wrapError(KindHashMismatch, err, "provider shard %q unreadable", name)
		}
		actual, err := hashFile(path)
		if err != nil {
			return nil, nil, wrapError(KindHashMismatch, err, "provider shard %q unreadable", name)
		}
		if err := validSHA256(released.Files[name]); err != nil || !strings.EqualFold(actual, released.Files[name]) {
			return nil, nil, newError(KindHashMismatch, "provider shard %q does not match released manifest", name)
		}
		var shard map[string]map[string]map[string]any
		if err := readJSON(path, &shard); err != nil {
			return nil, nil, wrapError(KindCatalogMismatch, err, "decode provider shard %q", name)
		}
		if len(shard) == 0 {
			return nil, nil, newError(KindCatalogMismatch, "provider shard %q is empty", name)
		}
		flat[provider] = make(map[string]map[string]any)
		structure[provider] = make(map[string]string)
		for api, models := range shard {
			if api == "" || len(models) == 0 {
				return nil, nil, newError(KindCatalogMismatch, "provider %q has an empty API group", provider)
			}
			for id, model := range models {
				if _, duplicate := flat[provider][id]; duplicate {
					return nil, nil, newError(KindCatalogMismatch, "duplicate model %s/%s across APIs", provider, id)
				}
				if err := validateChatModel(provider, api, id, model); err != nil {
					return nil, nil, err
				}
				flat[provider][id] = model
				structure[provider][id] = api
				modelCount++
			}
		}
	}
	structureHash, err := jsonSHA256(structure)
	if err != nil || !strings.EqualFold(structureHash, released.StructureHash) {
		return nil, nil, newError(KindHashMismatch, "released catalog structure hash mismatch")
	}
	if len(flat) != manifest.Providers || modelCount != manifest.Models {
		return nil, nil, newError(KindCatalogMismatch, "chat counts %d/%d != manifest %d/%d",
			len(flat), modelCount, manifest.Providers, manifest.Models)
	}
	return flat, &released, nil
}

func verifyLosslessDerivation(dir string, manifest *Manifest, artifacts map[string]string, source flatCatalog) error {
	rulesPath, err := secureFile(dir, manifest.Derivation.Rules)
	if err != nil {
		return wrapError(KindHashMismatch, err, "derivation rules unreadable")
	}
	var rules derivationRules
	if err := readJSON(rulesPath, &rules); err != nil {
		return wrapError(KindCatalogMismatch, err, "decode derivation rules")
	}
	if rules.SchemaVersion == "" || !commitEqual(rules.SourceCommit, manifest.CatalogSource.Commit) ||
		len(rules.Rules) != manifest.Derivation.RuleCount || len(rules.SemanticOverlays) != 0 {
		return newError(KindCatalogMismatch, "derivation rules do not match manifest provenance")
	}
	for _, rule := range rules.Rules {
		if rule.Operation != "flatten-api-groups" || !rule.PreservesFieldsAndValues ||
			!reflect.DeepEqual(rule.Ordering, []string{"provider", "model_id"}) {
			return newError(KindCatalogMismatch, "unsupported or lossy derivation rule")
		}
	}
	resultPath, err := secureFile(dir, manifest.Derivation.Result)
	if err != nil {
		return wrapError(KindHashMismatch, err, "derived chat catalog unreadable")
	}
	var result flatCatalog
	if err := readJSON(resultPath, &result); err != nil {
		return wrapError(KindCatalogMismatch, err, "decode derived chat catalog")
	}
	for provider, models := range result {
		for id, model := range models {
			api, _ := model["api"].(string)
			if err := validateChatModel(provider, api, id, model); err != nil {
				return err
			}
		}
	}
	if !reflect.DeepEqual(source, result) {
		return newError(KindCatalogMismatch, "derived chat catalog is not a lossless flatten of v0.84.1 shards")
	}
	if _, ok := artifacts[manifest.Derivation.Result]; !ok {
		return newError(KindHashMismatch, "derived chat catalog is not locked as an artifact")
	}
	return nil
}

func verifyImageCatalog(dir string, manifest *Manifest) error {
	path, err := secureFile(dir, manifest.Image.Artifact)
	if err != nil {
		return wrapError(KindHashMismatch, err, "image catalog unreadable")
	}
	var catalog flatCatalog
	if err := readJSON(path, &catalog); err != nil {
		return wrapError(KindCatalogMismatch, err, "decode image catalog")
	}
	models := 0
	for provider, providerModels := range catalog {
		for id, model := range providerModels {
			if err := validateImageModel(provider, id, model); err != nil {
				return err
			}
			models++
		}
	}
	if len(catalog) != manifest.Image.Providers || models != manifest.Image.Models {
		return newError(KindCatalogMismatch, "image counts %d/%d != manifest %d/%d",
			len(catalog), models, manifest.Image.Providers, manifest.Image.Models)
	}
	return nil
}

func validateChatModel(provider, api, id string, model map[string]any) error {
	if stringValue(model["id"]) != id || stringValue(model["provider"]) != provider || stringValue(model["api"]) != api {
		return newError(KindCatalogMismatch, "invalid provider/model reference %s/%s", provider, id)
	}
	if stringValue(model["name"]) == "" {
		return newError(KindCatalogMismatch, "chat model %s/%s lacks required strings", provider, id)
	}
	if _, ok := model["baseUrl"].(string); !ok {
		return newError(KindCatalogMismatch, "chat model %s/%s has no baseUrl string", provider, id)
	}
	if _, ok := model["reasoning"].(bool); !ok || !positiveNumber(model["contextWindow"]) || !positiveNumber(model["maxTokens"]) {
		return newError(KindCatalogMismatch, "chat model %s/%s has invalid capabilities", provider, id)
	}
	if !stringArray(model["input"], false) || !validCost(model["cost"]) {
		return newError(KindCatalogMismatch, "chat model %s/%s has invalid modalities or cost", provider, id)
	}
	return nil
}

func validateImageModel(provider, id string, model map[string]any) error {
	if stringValue(model["id"]) != id || stringValue(model["provider"]) != provider || stringValue(model["api"]) == "" {
		return newError(KindCatalogMismatch, "invalid image provider/model reference %s/%s", provider, id)
	}
	_, baseURL := model["baseUrl"].(string)
	if stringValue(model["name"]) == "" || !baseURL || !stringArray(model["input"], false) ||
		!stringArray(model["output"], false) || !validCost(model["cost"]) {
		return newError(KindCatalogMismatch, "image model %s/%s has invalid schema", provider, id)
	}
	return nil
}

func stringValue(value any) string {
	text, _ := value.(string)
	return text
}

func positiveNumber(value any) bool {
	number, ok := value.(float64)
	return ok && number > 0
}

func stringArray(value any, allowEmpty bool) bool {
	items, ok := value.([]any)
	if !ok || (!allowEmpty && len(items) == 0) {
		return false
	}
	for _, item := range items {
		if stringValue(item) == "" {
			return false
		}
	}
	return true
}

func validCost(value any) bool {
	cost, ok := value.(map[string]any)
	if !ok {
		return false
	}
	for _, key := range []string{"input", "output", "cacheRead", "cacheWrite"} {
		if _, ok := cost[key].(float64); !ok {
			return false
		}
	}
	return true
}

func timestampOK(value string) bool {
	_, err := time.Parse(time.RFC3339Nano, value)
	return err == nil
}

func jsonSHA256(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return hashBytes(data), nil
}

func hashBytes(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}

func validSHA256(value string) error {
	value = strings.TrimSpace(value)
	if len(value) != 64 {
		return fmt.Errorf("expected 64 hexadecimal characters")
	}
	_, err := hex.DecodeString(value)
	return err
}

func secureFile(root, relative string) (string, error) {
	if relative == "" || filepath.IsAbs(relative) || strings.Contains(relative, "\\") {
		return "", fmt.Errorf("unsafe relative path %q", relative)
	}
	clean := filepath.Clean(filepath.FromSlash(relative))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || filepath.ToSlash(clean) != relative {
		return "", fmt.Errorf("unsafe relative path %q", relative)
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	rootAbs, err = filepath.EvalSymlinks(rootAbs)
	if err != nil {
		return "", err
	}
	path := filepath.Join(rootAbs, clean)
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return "", fmt.Errorf("path is not a regular file")
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(path))
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(rootAbs, parent)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes baseline directory")
	}
	return path, nil
}

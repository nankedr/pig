// Package inventory implements the pure-Go artifact inventory for Milestone 0
// (issue #20): it walks the fixed Pi baseline, classifies every in-scope file
// and binds it to an owning Parity Catalog entry, then enforces coverage so any
// added, removed or unmapped artifact fails a check.
//
// The package is stdlib-only. Its default consistency check is fully offline:
// it reads only the committed snapshot under parity/inventory and the Parity
// Catalog, performing no network access and no subprocess execution. The walk
// (Walk) reads a Pi checkout from disk on demand; it is invoked only when
// regenerating the snapshot or running the opt-in drift check, never by normal
// `go test`. Like internal/baseline, this package MUST NOT be imported by
// cmd/pig or cmd/pig-ai, whose binaries are asserted to carry no net/os/exec
// dependencies.
package inventory

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// SchemaVersion is the inventory snapshot schema version.
const SchemaVersion = "1.0.0"

// Classification is the extraction category of a file. The set mirrors the
// Parity Catalog classification enum (docs/specs/parity-verification.md, #20).
const (
	// ClassDirectEntry is a package bin/entry point (an executable entry).
	ClassDirectEntry = "direct-entry"
	// ClassPublicAPI is a source file reachable from a package's public export
	// surface (its exports/main targets and their transitive imports).
	ClassPublicAPI = "public-api"
	// ClassPrivateImpl is a shipped implementation file (under src/ or native/)
	// that is not part of the public export surface.
	ClassPrivateImpl = "private-impl"
	// ClassDormantTestSupport covers tests, fixtures, examples, docs, build
	// scripts and package metadata: present in the tree but not shipped runtime
	// behaviour.
	ClassDormantTestSupport = "dormant-test-support"
)

// classificationEnum is the set of valid classifications.
var classificationEnum = map[string]bool{
	ClassDirectEntry:        true,
	ClassPublicAPI:          true,
	ClassPrivateImpl:        true,
	ClassDormantTestSupport: true,
}

// Module describes one in-scope Pi module: its upstream package directory, the
// Pig module bucket it maps to, the Parity Catalog entry that owns its files by
// default, and any bin entry files that map to a dedicated command entry.
type Module struct {
	// UpstreamDir is the package directory relative to the Pi checkout root,
	// e.g. "packages/ai".
	UpstreamDir string
	// PigModule is the Pig module bucket, e.g. "ai" or "codingagent".
	PigModule string
	// CatalogID is the module-level catalog entry that owns files by default.
	CatalogID string
	// EntryOwners maps a package-relative bin/entry path to the catalog entry
	// that owns it (overriding CatalogID). Keys are slash paths relative to
	// UpstreamDir, e.g. "src/cli.ts".
	EntryOwners map[string]string
}

// Modules is the fixed, ordered set of seven in-scope Pi modules. The order is
// deterministic (by Pig module name) so generated output is stable.
var Modules = []Module{
	{
		UpstreamDir: "packages/agent",
		PigModule:   "agent",
		CatalogID:   "module-agent",
	},
	{
		UpstreamDir: "packages/ai",
		PigModule:   "ai",
		CatalogID:   "module-ai",
		EntryOwners: map[string]string{"src/cli.ts": "cmd-pig-ai"},
	},
	{
		UpstreamDir: "packages/client",
		PigModule:   "client",
		CatalogID:   "module-client",
	},
	{
		UpstreamDir: "packages/coding-agent",
		PigModule:   "codingagent",
		CatalogID:   "module-codingagent",
		EntryOwners: map[string]string{"src/cli.ts": "cmd-pig"},
	},
	{
		UpstreamDir: "packages/protocol",
		PigModule:   "protocol",
		CatalogID:   "module-protocol",
	},
	{
		UpstreamDir: "packages/telemetry",
		PigModule:   "telemetry",
		CatalogID:   "module-telemetry",
	},
	{
		UpstreamDir: "packages/tui",
		PigModule:   "tui",
		CatalogID:   "module-tui",
	},
}

// Record is a single inventoried file. The canonical files.jsonl holds exactly
// one such object per line, sorted by path.
type Record struct {
	Path           string `json:"path"`
	SHA256         string `json:"sha256"`
	Module         string `json:"module"`
	Classification string `json:"classification"`
	OwningCatalog  string `json:"owning_catalog_id"`
}

// Manifest is the versioned inventory manifest (parity/inventory/manifest.json).
type Manifest struct {
	SchemaVersion       string         `json:"schema_version"`
	BaselineCommit      string         `json:"baseline_commit"`
	Files               string         `json:"files"`
	FileCount           int            `json:"file_count"`
	ModuleCounts        map[string]int `json:"module_counts"`
	ClassificationCount map[string]int `json:"classification_counts"`
}

// Kind identifies a consistency failure category.
type Kind string

// Consistency failure kinds.
const (
	KindDuplicatePath    Kind = "duplicate_path"
	KindMissingField     Kind = "missing_field"
	KindIllegalClass     Kind = "illegal_classification"
	KindUnknownModule    Kind = "unknown_module"
	KindUnmapped         Kind = "unmapped"
	KindModuleUncovered  Kind = "module_uncovered"
	KindManifestMismatch Kind = "manifest_mismatch"
	KindDrift            Kind = "drift"
)

// Sentinel errors, matchable with errors.Is.
var (
	ErrDuplicatePath    = errors.New("inventory: duplicate file path")
	ErrMissingField     = errors.New("inventory: record missing required field")
	ErrIllegalClass     = errors.New("inventory: illegal classification")
	ErrUnknownModule    = errors.New("inventory: unknown module")
	ErrUnmapped         = errors.New("inventory: file has no resolvable owning catalog entry")
	ErrModuleUncovered  = errors.New("inventory: in-scope module has no inventoried files")
	ErrManifestMismatch = errors.New("inventory: manifest counts do not match records")
	ErrDrift            = errors.New("inventory: snapshot drifted from the upstream checkout")
	ErrMalformedLine    = errors.New("inventory: malformed files.jsonl line")
)

func sentinelFor(k Kind) error {
	switch k {
	case KindDuplicatePath:
		return ErrDuplicatePath
	case KindMissingField:
		return ErrMissingField
	case KindIllegalClass:
		return ErrIllegalClass
	case KindUnknownModule:
		return ErrUnknownModule
	case KindUnmapped:
		return ErrUnmapped
	case KindModuleUncovered:
		return ErrModuleUncovered
	case KindManifestMismatch:
		return ErrManifestMismatch
	case KindDrift:
		return ErrDrift
	default:
		return nil
	}
}

// Error is the typed consistency error. It carries a Kind (for errors.As) and
// unwraps to its Kind sentinel (for errors.Is).
type Error struct {
	Kind    Kind
	Path    string
	Message string
}

func (e *Error) Error() string {
	msg := string(e.Kind)
	if e.Path != "" {
		msg += " (" + e.Path + ")"
	}
	if e.Message != "" {
		msg += ": " + e.Message
	}
	return msg
}

// Unwrap returns the Kind sentinel so errors.Is matches it.
func (e *Error) Unwrap() error { return sentinelFor(e.Kind) }

func newError(kind Kind, path, format string, args ...any) *Error {
	return &Error{Kind: kind, Path: path, Message: fmt.Sprintf(format, args...)}
}

// ParseError identifies a malformed line while loading files.jsonl.
type ParseError struct {
	Line int
	err  error
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("%s: line %d: %v", ErrMalformedLine.Error(), e.Line, e.err)
}

// Unwrap returns ErrMalformedLine so callers can match it with errors.Is.
func (e *ParseError) Unwrap() error { return ErrMalformedLine }

// LoadFiles parses a files.jsonl inventory line by line. A malformed line is an
// error identifying the line number.
func LoadFiles(path string) ([]Record, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var records []Record
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	line := 0
	for scanner.Scan() {
		line++
		raw := scanner.Bytes()
		if len(bytes.TrimSpace(raw)) == 0 {
			return nil, &ParseError{Line: line, err: errors.New("blank line not allowed")}
		}
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.DisallowUnknownFields()
		var record Record
		if err := dec.Decode(&record); err != nil {
			return nil, &ParseError{Line: line, err: err}
		}
		if dec.More() {
			return nil, &ParseError{Line: line, err: errors.New("more than one JSON value on line")}
		}
		records = append(records, record)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return records, nil
}

// EncodeFiles serialises records as canonical JSONL: one compact object per
// line, sorted by path, with HTML escaping disabled so paths stay byte-clean.
func EncodeFiles(records []Record) ([]byte, error) {
	sorted := append([]Record(nil), records...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Path < sorted[j].Path })
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	for _, record := range sorted {
		if err := enc.Encode(record); err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}

// LoadManifest reads and decodes the inventory manifest JSON.
func LoadManifest(path string) (Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var m Manifest
	if err := dec.Decode(&m); err != nil {
		return Manifest{}, err
	}
	return m, nil
}

// BuildManifest derives the manifest counts from records.
func BuildManifest(records []Record, baselineCommit string) Manifest {
	moduleCounts := map[string]int{}
	classCounts := map[string]int{}
	for _, r := range records {
		moduleCounts[r.Module]++
		classCounts[r.Classification]++
	}
	return Manifest{
		SchemaVersion:       SchemaVersion,
		BaselineCommit:      baselineCommit,
		Files:               "files.jsonl",
		FileCount:           len(records),
		ModuleCounts:        moduleCounts,
		ClassificationCount: classCounts,
	}
}

// EncodeManifest serialises the manifest as pretty JSON with a trailing newline.
func EncodeManifest(m Manifest) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(m); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Validate enforces the offline coverage rules against a loaded snapshot. It
// requires the set of known Parity Catalog entry ids so it can reject files
// whose owning_catalog_id does not resolve (unmapped artifacts).
//
// It rejects: records missing a required field; duplicate paths; illegal
// classification; unknown module; unmapped owning_catalog_id; in-scope modules
// with no files; and manifests whose counts disagree with the records.
func Validate(records []Record, manifest Manifest, catalogIDs map[string]bool) error {
	knownModule := map[string]bool{}
	for _, m := range Modules {
		knownModule[m.PigModule] = true
	}

	seen := make(map[string]bool, len(records))
	moduleCounts := map[string]int{}
	classCounts := map[string]int{}

	for _, r := range records {
		switch {
		case r.Path == "":
			return newError(KindMissingField, r.Path, "path is empty")
		case r.SHA256 == "":
			return newError(KindMissingField, r.Path, "sha256 is empty")
		case r.Module == "":
			return newError(KindMissingField, r.Path, "module is empty")
		case r.Classification == "":
			return newError(KindMissingField, r.Path, "classification is empty")
		case r.OwningCatalog == "":
			return newError(KindMissingField, r.Path, "owning_catalog_id is empty")
		}
		if seen[r.Path] {
			return newError(KindDuplicatePath, r.Path, "")
		}
		seen[r.Path] = true

		if !classificationEnum[r.Classification] {
			return newError(KindIllegalClass, r.Path, "%s", r.Classification)
		}
		if !knownModule[r.Module] {
			return newError(KindUnknownModule, r.Path, "%s", r.Module)
		}
		if !catalogIDs[r.OwningCatalog] {
			return newError(KindUnmapped, r.Path, "owning_catalog_id %s is not a catalog entry", r.OwningCatalog)
		}

		moduleCounts[r.Module]++
		classCounts[r.Classification]++
	}

	for _, m := range Modules {
		if moduleCounts[m.PigModule] == 0 {
			return newError(KindModuleUncovered, "", "module %s has no inventoried files", m.PigModule)
		}
	}

	if manifest.FileCount != len(records) {
		return newError(KindManifestMismatch, "", "file_count=%d actual=%d", manifest.FileCount, len(records))
	}
	if err := compareCounts("module_counts", manifest.ModuleCounts, moduleCounts); err != nil {
		return err
	}
	if err := compareCounts("classification_counts", manifest.ClassificationCount, classCounts); err != nil {
		return err
	}
	return nil
}

func compareCounts(label string, manifestCounts, actual map[string]int) error {
	keys := map[string]bool{}
	for k := range manifestCounts {
		keys[k] = true
	}
	for k := range actual {
		keys[k] = true
	}
	for k := range keys {
		if manifestCounts[k] != actual[k] {
			return newError(KindManifestMismatch, "", "%s[%s]=%d actual=%d", label, k, manifestCounts[k], actual[k])
		}
	}
	return nil
}

// DiffResult is the outcome of comparing a committed snapshot to a fresh walk.
type DiffResult struct {
	Added   []string // paths present upstream but missing from the snapshot
	Removed []string // paths in the snapshot but missing upstream
	Changed []string // paths whose sha256 or classification differ
}

// Empty reports whether the snapshot and the fresh walk agree.
func (d DiffResult) Empty() bool {
	return len(d.Added) == 0 && len(d.Removed) == 0 && len(d.Changed) == 0
}

// Diff compares a committed snapshot to a freshly walked one, reporting added,
// removed and changed artifacts. It is the engine behind the coverage check:
// any add, delete or content/classification change is surfaced.
func Diff(committed, fresh []Record) DiffResult {
	byPath := func(rs []Record) map[string]Record {
		m := make(map[string]Record, len(rs))
		for _, r := range rs {
			m[r.Path] = r
		}
		return m
	}
	committedByPath := byPath(committed)
	freshByPath := byPath(fresh)

	var result DiffResult
	for p, fr := range freshByPath {
		cr, ok := committedByPath[p]
		if !ok {
			result.Added = append(result.Added, p)
			continue
		}
		if cr.SHA256 != fr.SHA256 || cr.Classification != fr.Classification ||
			cr.Module != fr.Module || cr.OwningCatalog != fr.OwningCatalog {
			result.Changed = append(result.Changed, p)
		}
	}
	for p := range committedByPath {
		if _, ok := freshByPath[p]; !ok {
			result.Removed = append(result.Removed, p)
		}
	}
	sort.Strings(result.Added)
	sort.Strings(result.Removed)
	sort.Strings(result.Changed)
	return result
}

// walkSkipDirs are directory names that never belong to the tracked snapshot
// (build output, installed dependencies, VCS metadata). Skipping them keeps a
// pure-Go filesystem walk equal to the git-tracked set on a clean checkout.
var walkSkipDirs = map[string]bool{
	".git":         true,
	"node_modules": true,
	"dist":         true,
	".turbo":       true,
	"coverage":     true,
}

// Walk inventories every in-scope file under a Pi checkout rooted at piRoot,
// returning records sorted by path. It is pure-Go and offline (it only reads
// the local checkout); callers gate it behind a verified baseline checkout.
func Walk(piRoot string) ([]Record, error) {
	var records []Record
	for _, module := range Modules {
		moduleRecords, err := walkModule(piRoot, module)
		if err != nil {
			return nil, err
		}
		records = append(records, moduleRecords...)
	}
	sort.Slice(records, func(i, j int) bool { return records[i].Path < records[j].Path })
	return records, nil
}

func walkModule(piRoot string, module Module) ([]Record, error) {
	moduleAbs := filepath.Join(piRoot, filepath.FromSlash(module.UpstreamDir))
	files, err := listFiles(moduleAbs)
	if err != nil {
		return nil, err
	}

	roots, binEntries, err := publicRoots(moduleAbs, files)
	if err != nil {
		return nil, err
	}
	reachable := reachableFrom(moduleAbs, roots)

	records := make([]Record, 0, len(files))
	for _, rel := range files {
		abs := filepath.Join(moduleAbs, filepath.FromSlash(rel))
		sum, err := hashFile(abs)
		if err != nil {
			return nil, err
		}
		class := classify(rel, binEntries, reachable)
		owner := module.CatalogID
		if module.EntryOwners != nil {
			if o, ok := module.EntryOwners[rel]; ok {
				owner = o
			}
		}
		records = append(records, Record{
			Path:           path.Join(module.UpstreamDir, rel),
			SHA256:         sum,
			Module:         module.PigModule,
			Classification: class,
			OwningCatalog:  owner,
		})
	}
	return records, nil
}

// listFiles returns every file under dir as slash paths relative to dir,
// skipping walkSkipDirs and the noisy .DS_Store artifact.
func listFiles(dir string) ([]string, error) {
	var rels []string
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if walkSkipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() == ".DS_Store" {
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		rels = append(rels, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(rels)
	return rels, nil
}

// classify assigns a file's classification. Order matters: bin entries first,
// then public-api by reachability, then shipped implementation, then the
// dormant/test-support remainder.
func classify(rel string, binEntries, reachable map[string]bool) string {
	if binEntries[rel] {
		return ClassDirectEntry
	}
	if reachable[rel] {
		return ClassPublicAPI
	}
	if isShippedImpl(rel) {
		return ClassPrivateImpl
	}
	return ClassDormantTestSupport
}

// isShippedImpl reports whether a file ships as implementation: it lives under
// src/ or native/ (the native addon sources compiled into the shipped binary).
func isShippedImpl(rel string) bool {
	return strings.HasPrefix(rel, "src/") || strings.HasPrefix(rel, "native/")
}

// packageJSON is the subset of a package manifest we read for entry points.
type packageJSON struct {
	Main    string          `json:"main"`
	Types   string          `json:"types"`
	Bin     json.RawMessage `json:"bin"`
	Exports json.RawMessage `json:"exports"`
}

// publicRoots resolves a module's public-export source roots and its bin entry
// files. Roots are the source files backing package.json main/exports targets
// (globs expanded); binEntries are the source files backing bin targets. Both
// are returned as sets of slash paths relative to the module directory.
func publicRoots(moduleAbs string, files []string) (roots, binEntries map[string]bool, err error) {
	roots = map[string]bool{}
	binEntries = map[string]bool{}

	data, err := os.ReadFile(filepath.Join(moduleAbs, "package.json"))
	if err != nil {
		return nil, nil, err
	}
	var pkg packageJSON
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil, nil, err
	}

	exists := make(map[string]bool, len(files))
	for _, f := range files {
		exists[f] = true
	}

	addTargets := func(distTarget string, dst map[string]bool) {
		for _, src := range distTargetSources(distTarget, files, exists) {
			dst[src] = true
		}
	}

	if pkg.Main != "" {
		addTargets(pkg.Main, roots)
	}
	if pkg.Types != "" {
		addTargets(pkg.Types, roots)
	}
	for _, t := range collectConditionTargets(pkg.Exports) {
		addTargets(t, roots)
	}
	for _, t := range collectBinTargets(pkg.Bin) {
		addTargets(t, binEntries)
	}

	// A bin entry is a direct-entry, never a public-api root.
	for entry := range binEntries {
		delete(roots, entry)
	}
	return roots, binEntries, nil
}

// distTargetSources maps a built dist target (e.g. "./dist/providers/*.js") to
// the source files backing it. It strips the ./ and dist/ prefixes and the
// .js/.d.ts suffix, then looks for src/<stem>.ts or a package-root sibling,
// expanding a single trailing/segment "*" glob against the file list.
func distTargetSources(distTarget string, files []string, exists map[string]bool) []string {
	rel := strings.TrimPrefix(distTarget, "./")
	rel = strings.TrimPrefix(rel, "dist/")
	rel = strings.TrimSuffix(rel, ".js")
	rel = strings.TrimSuffix(rel, ".d.ts")
	rel = strings.TrimSuffix(rel, ".ts")
	if rel == "" || rel == "package.json" {
		return nil
	}

	if strings.Contains(rel, "*") {
		return globSources(rel, files)
	}

	// Candidate source files, in priority order: a src/ TypeScript source, then
	// a package-root sibling (covers prebuilt root-level provider files).
	for _, cand := range []string{
		"src/" + rel + ".ts",
		"src/" + rel + ".tsx",
		rel + ".ts",
		rel + ".js",
		rel + ".d.ts",
	} {
		if exists[cand] {
			return []string{cand}
		}
	}
	return nil
}

// globSources expands a stem containing a single "*" (e.g. "providers/*" or
// "api/*") against src/<stem>.ts source files.
func globSources(stem string, files []string) []string {
	prefix := "src/" + stem[:strings.IndexByte(stem, '*')]
	var out []string
	for _, f := range files {
		if !strings.HasPrefix(f, prefix) || !strings.HasSuffix(f, ".ts") {
			continue
		}
		if strings.HasSuffix(f, ".d.ts") {
			continue
		}
		out = append(out, f)
	}
	return out
}

// collectConditionTargets walks an exports object and returns every string
// target reachable through condition/subpath maps (import, types, default, ...).
func collectConditionTargets(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var targets []string
	var walk func(v any)
	walk = func(v any) {
		switch t := v.(type) {
		case string:
			targets = append(targets, t)
		case map[string]any:
			for _, val := range t {
				walk(val)
			}
		case []any:
			for _, val := range t {
				walk(val)
			}
		}
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil
	}
	walk(v)
	return targets
}

// collectBinTargets returns the target scripts of a package.json bin field,
// which may be a single string or a name->path map.
func collectBinTargets(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var single string
	if err := json.Unmarshal(raw, &single); err == nil {
		return []string{single}
	}
	var m map[string]string
	if err := json.Unmarshal(raw, &m); err == nil {
		out := make([]string, 0, len(m))
		for _, v := range m {
			out = append(out, v)
		}
		return out
	}
	return nil
}

// relativeImport matches the specifier of a relative import/export/dynamic
// import or bare side-effect import in a TypeScript source file.
var relativeImport = regexp.MustCompile(`(?:from|import)\s*\(?\s*["'](\.[^"']*)["']`)

// reachableFrom computes the set of source files reachable from the public
// roots by following relative imports. Pi uses explicit file extensions on
// every relative import, so resolution is a direct path join with no index or
// extension inference. Returned as slash paths relative to moduleAbs.
func reachableFrom(moduleAbs string, roots map[string]bool) map[string]bool {
	reachable := map[string]bool{}
	var queue []string
	for r := range roots {
		if !reachable[r] {
			reachable[r] = true
			queue = append(queue, r)
		}
	}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if !isSourceCode(cur) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(moduleAbs, filepath.FromSlash(cur)))
		if err != nil {
			continue
		}
		for _, m := range relativeImport.FindAllSubmatch(data, -1) {
			spec := string(m[1])
			target := path.Clean(path.Join(path.Dir(cur), spec))
			if strings.HasPrefix(target, "../") || target == ".." {
				continue // escapes the package; cross-package deps are separate entries
			}
			if _, err := os.Stat(filepath.Join(moduleAbs, filepath.FromSlash(target))); err != nil {
				continue
			}
			if !reachable[target] {
				reachable[target] = true
				queue = append(queue, target)
			}
		}
	}
	return reachable
}

// isSourceCode reports whether a file is TypeScript/JavaScript source whose
// relative imports should be followed for reachability.
func isSourceCode(rel string) bool {
	switch {
	case strings.HasSuffix(rel, ".ts"), strings.HasSuffix(rel, ".tsx"),
		strings.HasSuffix(rel, ".mts"), strings.HasSuffix(rel, ".js"),
		strings.HasSuffix(rel, ".mjs"):
		return true
	default:
		return false
	}
}

func hashFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

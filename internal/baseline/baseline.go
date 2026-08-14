// Package baseline verifies Pig's fixed Parity Baseline: the locked Pi source
// commit and its companion Catalog Snapshot manifest.
//
// The default verification path is fully offline: it reads only the committed
// locked artifacts under parity/baseline and performs no network access and no
// subprocess execution. Optional checkout verification (comparing a working Pi
// checkout's HEAD against the locked commit) is opt-in via WithCheckout and uses
// an injectable CommitResolver so tests never touch real git or the network.
//
// This package MAY use os/exec (for the default git resolver) and therefore MUST
// NOT be imported by cmd/pig or cmd/pig-ai, whose binaries are asserted to carry
// no net/os/exec dependencies.
package baseline

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Status values for the Catalog Snapshot manifest.
const (
	// StatusCaptured means a real controlled capture ran and produced locked
	// artifacts with hashes, a generation timestamp and input sources.
	StatusCaptured = "captured"
	// StatusPendingCapture is the honest placeholder state used before any
	// controlled network+Node capture has been performed. It must carry no
	// fabricated hashes, timestamps, artifacts or model/provider counts.
	StatusPendingCapture = "pending-capture"
)

// defaultLockFile and defaultManifestFile are the canonical artifact names.
const (
	defaultLockFile     = "upstream.lock.json"
	defaultManifestFile = "snapshot.manifest.json"
)

// Lock is the upstream lock (parity/baseline/upstream.lock.json). It records the
// Pi repository, the fixed commit, source verification metadata and a pointer to
// the Catalog Snapshot manifest.
type Lock struct {
	SchemaVersion      string             `json:"schema_version"`
	BaselineID         string             `json:"baseline_id"`
	Upstream           Upstream           `json:"upstream"`
	SourceVerification SourceVerification `json:"source_verification"`
	CatalogSnapshot    CatalogSnapshot    `json:"catalog_snapshot"`
}

// Upstream identifies the Pi repository and its license.
type Upstream struct {
	Name          string `json:"name"`
	Repository    string `json:"repository"`
	Commit        string `json:"commit"`
	License       string `json:"license"`
	LicenseHolder string `json:"license_holder"`
	LicenseYear   string `json:"license_year"`
}

// SourceVerification describes how the fixed commit is checked and asserts Pi is
// neither a submodule nor a runtime dependency.
type SourceVerification struct {
	Method                string `json:"method"`
	ExpectedCommit        string `json:"expected_commit"`
	CheckoutPath          string `json:"checkout_path"`
	NotASubmodule         bool   `json:"not_a_submodule"`
	NotARuntimeDependency bool   `json:"not_a_runtime_dependency"`
}

// CatalogSnapshot points at the snapshot manifest file (relative to the baseline
// directory).
type CatalogSnapshot struct {
	Manifest string `json:"manifest"`
}

// Manifest is the Catalog Snapshot manifest
// (parity/baseline/snapshot.manifest.json).
type Manifest struct {
	SchemaVersion   string      `json:"schema_version"`
	BaselineCommit  string      `json:"baseline_commit"`
	Status          string      `json:"status"`
	Generation      Generation  `json:"generation"`
	Capture         Capture     `json:"capture"`
	Artifacts       []Artifact  `json:"artifacts"`
	Providers       int         `json:"providers"`
	Models          int         `json:"models"`
	Attribution     Attribution `json:"attribution"`
	ExcludesSecrets bool        `json:"excludes_secrets"`
}

// Generation records how the snapshot data was produced. GeneratedAt is a
// pointer so a null timestamp (honest pending-capture) is distinguishable from an
// empty string.
type Generation struct {
	GeneratedAt     *string           `json:"generated_at"`
	GeneratorCommit string            `json:"generator_commit"`
	ToolVersions    map[string]string `json:"tool_versions"`
	InputSources    []string          `json:"input_sources"`
}

// Capture explains why the snapshot is pending and how to complete it.
type Capture struct {
	Reason       string `json:"reason"`
	RequiredStep string `json:"required_step"`
	TrackedBy    string `json:"tracked_by"`
}

// Artifact is a single locked snapshot file plus its SHA-256.
type Artifact struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

// Attribution records the snapshot data's license and source.
type Attribution struct {
	License string `json:"license"`
	Holder  string `json:"holder"`
	Source  string `json:"source"`
}

// Kind identifies a verification failure category.
type Kind string

// Verification failure kinds.
const (
	KindMissingField     Kind = "missing-field"
	KindCommitMismatch   Kind = "commit-mismatch"
	KindHashMismatch     Kind = "hash-mismatch"
	KindMissingHash      Kind = "missing-hash"
	KindDishonestPending Kind = "dishonest-pending"
	KindIllegalStatus    Kind = "illegal-status"
	KindNotIndependent   Kind = "not-independent"
	KindCheckoutFailed   Kind = "checkout-failed"
)

// Sentinel errors, one per Kind, so callers can match failures with errors.Is
// without depending on message text.
var (
	ErrMissingField     = errors.New("baseline: missing required field")
	ErrCommitMismatch   = errors.New("baseline: commit mismatch")
	ErrHashMismatch     = errors.New("baseline: artifact hash mismatch")
	ErrMissingHash      = errors.New("baseline: artifact missing hash")
	ErrDishonestPending = errors.New("baseline: pending-capture carries fabricated data")
	ErrIllegalStatus    = errors.New("baseline: illegal snapshot status")
	ErrNotIndependent   = errors.New("baseline: upstream must not be a submodule or runtime dependency")
	ErrCheckoutFailed   = errors.New("baseline: checkout verification failed")
)

func sentinelFor(k Kind) error {
	switch k {
	case KindMissingField:
		return ErrMissingField
	case KindCommitMismatch:
		return ErrCommitMismatch
	case KindHashMismatch:
		return ErrHashMismatch
	case KindMissingHash:
		return ErrMissingHash
	case KindDishonestPending:
		return ErrDishonestPending
	case KindIllegalStatus:
		return ErrIllegalStatus
	case KindNotIndependent:
		return ErrNotIndependent
	case KindCheckoutFailed:
		return ErrCheckoutFailed
	default:
		return nil
	}
}

// Error is the typed verification error. It carries a Kind (for errors.As) and
// unwraps to both its Kind sentinel and any underlying wrapped error (for
// errors.Is), using Go's multi-error Unwrap.
type Error struct {
	Kind    Kind
	Message string
	wrapped error
}

func (e *Error) Error() string {
	msg := string(e.Kind) + ": " + e.Message
	if e.wrapped != nil {
		return msg + ": " + e.wrapped.Error()
	}
	return msg
}

// Unwrap returns the Kind sentinel plus any wrapped error so errors.Is matches
// either.
func (e *Error) Unwrap() []error {
	var errs []error
	if s := sentinelFor(e.Kind); s != nil {
		errs = append(errs, s)
	}
	if e.wrapped != nil {
		errs = append(errs, e.wrapped)
	}
	return errs
}

func newError(kind Kind, format string, args ...any) *Error {
	return &Error{Kind: kind, Message: fmt.Sprintf(format, args...)}
}

func wrapError(kind Kind, wrapped error, format string, args ...any) *Error {
	return &Error{Kind: kind, Message: fmt.Sprintf(format, args...), wrapped: wrapped}
}

// CommitResolver resolves the HEAD commit of a Pi checkout at path. It is
// injectable so verification is testable without real git or exec.
type CommitResolver func(path string) (string, error)

// DefaultCommitResolver resolves HEAD via `git -C <path> rev-parse HEAD`. It is
// only invoked when a checkout is supplied via WithCheckout.
func DefaultCommitResolver(path string) (string, error) {
	out, err := exec.Command("git", "-C", path, "rev-parse", "HEAD").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

type config struct {
	checkout string
	resolver CommitResolver
}

// Option configures Verify.
type Option func(*config)

// WithCheckout enables optional checkout verification: the resolved HEAD at path
// must equal the locked expected commit. Without it, the git step is skipped
// entirely and verification stays fully offline.
func WithCheckout(path string) Option {
	return func(c *config) { c.checkout = path }
}

// WithCommitResolver overrides the resolver used for checkout verification
// (defaults to DefaultCommitResolver). Primarily for tests.
func WithCommitResolver(r CommitResolver) Option {
	return func(c *config) { c.resolver = r }
}

// Load reads and JSON-decodes the upstream lock and the snapshot manifest from
// dir (the parity/baseline directory). The manifest filename comes from the
// lock's catalog_snapshot.manifest pointer, defaulting to snapshot.manifest.json.
func Load(dir string) (*Lock, *Manifest, error) {
	var lock Lock
	if err := readJSON(filepath.Join(dir, defaultLockFile), &lock); err != nil {
		return nil, nil, err
	}
	manifestName := lock.CatalogSnapshot.Manifest
	if strings.TrimSpace(manifestName) == "" {
		manifestName = defaultManifestFile
	}
	var manifest Manifest
	if err := readJSON(filepath.Join(dir, manifestName), &manifest); err != nil {
		return nil, nil, err
	}
	return &lock, &manifest, nil
}

func readJSON(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("baseline: read %s: %w", filepath.Base(path), err)
	}
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("baseline: decode %s: %w", filepath.Base(path), err)
	}
	return nil
}

// Verify performs offline structural and integrity verification of the baseline
// artifacts in dir. It fails explicitly with a typed *Error (identifiable via
// errors.Is against the sentinels and errors.As to inspect Kind) on any missing
// field, commit mismatch, hash mismatch or honesty violation. When a checkout is
// supplied, it also verifies the checkout HEAD matches the locked commit.
func Verify(dir string, opts ...Option) error {
	cfg := config{resolver: DefaultCommitResolver}
	for _, opt := range opts {
		opt(&cfg)
	}

	lock, manifest, err := Load(dir)
	if err != nil {
		return err
	}

	if err := verifyRequired(lock, manifest); err != nil {
		return err
	}
	if err := verifyIndependence(lock); err != nil {
		return err
	}
	if err := verifyCommitConsistency(lock, manifest); err != nil {
		return err
	}
	if err := verifyManifestIntegrity(dir, manifest); err != nil {
		return err
	}
	if cfg.checkout != "" {
		if err := verifyCheckout(cfg, lock.SourceVerification.ExpectedCommit); err != nil {
			return err
		}
	}
	return nil
}

func verifyRequired(lock *Lock, manifest *Manifest) error {
	for _, f := range []struct{ name, value string }{
		{"schema_version", lock.SchemaVersion},
		{"upstream.repository", lock.Upstream.Repository},
		{"upstream.commit", lock.Upstream.Commit},
		{"source_verification.expected_commit", lock.SourceVerification.ExpectedCommit},
		{"manifest.schema_version", manifest.SchemaVersion},
		{"manifest.baseline_commit", manifest.BaselineCommit},
	} {
		if strings.TrimSpace(f.value) == "" {
			return newError(KindMissingField, "%s is empty", f.name)
		}
	}
	return nil
}

func verifyIndependence(lock *Lock) error {
	if !lock.SourceVerification.NotASubmodule {
		return newError(KindNotIndependent, "source_verification.not_a_submodule must be true")
	}
	if !lock.SourceVerification.NotARuntimeDependency {
		return newError(KindNotIndependent, "source_verification.not_a_runtime_dependency must be true")
	}
	return nil
}

func verifyCommitConsistency(lock *Lock, manifest *Manifest) error {
	upstream := lock.Upstream.Commit
	expected := lock.SourceVerification.ExpectedCommit
	baselineCommit := manifest.BaselineCommit
	if !commitEqual(upstream, expected) {
		return newError(KindCommitMismatch,
			"upstream.commit %q != source_verification.expected_commit %q", upstream, expected)
	}
	if !commitEqual(upstream, baselineCommit) {
		return newError(KindCommitMismatch,
			"upstream.commit %q != manifest.baseline_commit %q", upstream, baselineCommit)
	}
	return nil
}

func commitEqual(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

func verifyManifestIntegrity(dir string, manifest *Manifest) error {
	switch manifest.Status {
	case StatusCaptured:
		return verifyCaptured(dir, manifest)
	case StatusPendingCapture:
		return verifyPending(manifest)
	default:
		return newError(KindIllegalStatus, "unknown status %q", manifest.Status)
	}
}

func verifyCaptured(dir string, manifest *Manifest) error {
	if manifest.Generation.GeneratedAt == nil || strings.TrimSpace(*manifest.Generation.GeneratedAt) == "" {
		return newError(KindMissingField, "captured manifest requires generation.generated_at")
	}
	if len(manifest.Generation.InputSources) == 0 {
		return newError(KindMissingField, "captured manifest requires generation.input_sources")
	}
	if len(manifest.Artifacts) == 0 {
		return newError(KindMissingField, "captured manifest requires at least one artifact")
	}
	for _, artifact := range manifest.Artifacts {
		if strings.TrimSpace(artifact.Path) == "" {
			return newError(KindMissingField, "captured artifact requires a path")
		}
		if strings.TrimSpace(artifact.SHA256) == "" {
			return newError(KindMissingHash, "artifact %q has no sha256", artifact.Path)
		}
		actual, err := hashFile(filepath.Join(dir, artifact.Path))
		if err != nil {
			return wrapError(KindHashMismatch, err, "artifact %q unreadable", artifact.Path)
		}
		if !strings.EqualFold(actual, strings.TrimSpace(artifact.SHA256)) {
			return newError(KindHashMismatch,
				"artifact %q sha256 %s != recorded %s", artifact.Path, actual, artifact.SHA256)
		}
	}
	return nil
}

func verifyPending(manifest *Manifest) error {
	if strings.TrimSpace(manifest.Capture.Reason) == "" {
		return newError(KindMissingField, "pending-capture manifest requires capture.reason")
	}
	if manifest.Generation.GeneratedAt != nil {
		return newError(KindDishonestPending,
			"pending-capture manifest must have null generated_at, got %q", *manifest.Generation.GeneratedAt)
	}
	if len(manifest.Artifacts) != 0 {
		return newError(KindDishonestPending,
			"pending-capture manifest must have no artifacts, got %d", len(manifest.Artifacts))
	}
	if len(manifest.Generation.InputSources) != 0 {
		return newError(KindDishonestPending,
			"pending-capture manifest must have no input_sources, got %d", len(manifest.Generation.InputSources))
	}
	if manifest.Providers != 0 || manifest.Models != 0 {
		return newError(KindDishonestPending,
			"pending-capture manifest must have zero providers/models, got %d/%d", manifest.Providers, manifest.Models)
	}
	return nil
}

func verifyCheckout(cfg config, expected string) error {
	head, err := cfg.resolver(cfg.checkout)
	if err != nil {
		return wrapError(KindCheckoutFailed, err, "resolve HEAD at %s", cfg.checkout)
	}
	if !commitEqual(head, expected) {
		return newError(KindCommitMismatch, "checkout HEAD %q != expected commit %q", head, expected)
	}
	return nil
}

func hashFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

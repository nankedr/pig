// Package catalog implements the Parity Catalog data model, its loader,
// validator and the non-authoritative report generator.
//
// The Parity Catalog is the single machine-readable authority for scope,
// mapping, capability status, deviation and evidence (see
// docs/specs/parity-verification.md). This package is stdlib-only: it does not
// pull in a JSON-schema runtime; parity/catalog.schema.json documents the shape
// for humans and tooling while Validate enforces the rules in Go.
package catalog

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

// SchemaVersion is the current catalog entry/manifest schema version.
const SchemaVersion = "1.1.0"

// NonAuthoritativeBanner is emitted verbatim at the top of the generated
// report. It must make clear the report is a non-authoritative, generated view
// that must not be hand-edited and is regenerated from parity/catalog.jsonl.
const NonAuthoritativeBanner = "<!-- GENERATED FILE - DO NOT EDIT BY HAND -->\n" +
	"# Parity Catalog Report (non-authoritative)\n" +
	"\n" +
	"> This is a NON-AUTHORITATIVE generated view of `parity/catalog.jsonl`.\n" +
	"> Do not hand-edit this file; it is regenerated from the Parity Catalog.\n" +
	"> The Parity Catalog (`parity/catalog.jsonl`) is the single machine-readable authority."

// Upstream records the fixed Pi baseline provenance for an entry.
type Upstream struct {
	Module     string `json:"module"`
	Repository string `json:"repository"`
	Commit     string `json:"commit"`
	Reference  string `json:"reference"`
}

// Mapping records the Go target the upstream artifact maps to.
type Mapping struct {
	Module string `json:"module"`
	Target string `json:"target"`
	Kind   string `json:"kind"`
}

// MatrixField names the Pi and Go sides of a capability-matrix row. Field rows
// use declared members, symbol rows use entrypoints, and behavior rows use
// stable branch names. Type is intentionally source-shaped text: public rows
// name the exact declared type while internal-wire and fixture rows name the
// semantic type without forcing a particular private implementation.
type MatrixField struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// MatrixValueSemantics records the observable states a field must preserve.
// Description says what those states mean for this particular row.
type MatrixValueSemantics struct {
	States      []string `json:"states"`
	Description string   `json:"description"`
}

// MatrixEvidenceRequirement describes evidence that must exist before a
// matrix row may advance to verified. It is prospective and distinct from an
// Entry's Evidence, which records evidence that already exists.
type MatrixEvidenceRequirement struct {
	Kind      string `json:"kind"`
	Assertion string `json:"assertion"`
}

// CapabilityMatrix is the field, entrypoint, or behavior-level contract for one
// API capability. The enclosing Entry supplies Pi provenance, Go target,
// milestone, status and any evidence already collected.
type CapabilityMatrix struct {
	API                  string                      `json:"api"`
	Surface              string                      `json:"surface"`
	Category             string                      `json:"category"`
	Pi                   MatrixField                 `json:"pi"`
	Go                   MatrixField                 `json:"go"`
	Direction            string                      `json:"direction"`
	PreTargetBehavior    string                      `json:"pre_target_behavior"`
	ValueSemantics       MatrixValueSemantics        `json:"value_semantics"`
	EvidenceRequirements []MatrixEvidenceRequirement `json:"evidence_requirements"`
}

// Evidence binds a catalog entry to repeatable verification material. Verified
// entries carry the full replay record required by parity-verification.md;
// older scaffold evidence may retain the compact provenance-only form.
type Evidence struct {
	Kind            string `json:"kind"`
	Ref             string `json:"ref"`
	Baseline        string `json:"baseline"`
	CaseID          string `json:"case_id,omitempty"`
	InputHash       string `json:"input_hash,omitempty"`
	ExecutionMethod string `json:"execution_method,omitempty"`
	Expected        string `json:"expected,omitempty"`
	Actual          string `json:"actual,omitempty"`
	Platform        string `json:"platform,omitempty"`
	CatalogID       string `json:"catalog_id,omitempty"`
}

// Deviation records an approved deviation and the ADR that authorises it.
type Deviation struct {
	ADR    string `json:"adr"`
	Reason string `json:"reason"`
}

// Partial lists supported and unsupported branches for a partial capability.
type Partial struct {
	Supported   []string `json:"supported"`
	Unsupported []string `json:"unsupported"`
}

// Deferred records an explicit deferral decision, its ADR and target milestone.
type Deferred struct {
	ADR       string `json:"adr"`
	Milestone string `json:"milestone"`
	Reason    string `json:"reason"`
}

// Entry is a single Parity Catalog record. The canonical JSONL holds exactly
// one such object per line. The optional blocks carry omitempty so a generated
// entry without an approved deviation, partial split, deferral or notes stays
// byte-clean and diff-minimal (see EncodeEntries).
type Entry struct {
	SchemaVersion  string            `json:"schema_version"`
	ID             string            `json:"id"`
	Upstream       Upstream          `json:"upstream"`
	Mapping        Mapping           `json:"mapping"`
	Status         string            `json:"status"`
	Milestone      string            `json:"milestone"`
	Classification string            `json:"classification"`
	Evidence       []Evidence        `json:"evidence,omitempty"`
	Deviation      *Deviation        `json:"deviation,omitempty"`
	Partial        *Partial          `json:"partial,omitempty"`
	Deferred       *Deferred         `json:"deferred,omitempty"`
	Matrix         *CapabilityMatrix `json:"matrix,omitempty"`
	Notes          string            `json:"notes,omitempty"`
}

// Manifest is the versioned catalog manifest.
type Manifest struct {
	SchemaVersion   string         `json:"schema_version"`
	CatalogVersion  string         `json:"catalog_version"`
	BaselineCommit  string         `json:"baseline_commit"`
	Catalog         string         `json:"catalog"`
	Schema          string         `json:"schema"`
	GeneratedReport string         `json:"generated_report"`
	EntryCount      int            `json:"entry_count"`
	StatusCounts    map[string]int `json:"status_counts"`
}

// Canonical capability status values (docs/specs/parity-verification.md).
const (
	StatusInventoried = "inventoried"
	StatusScaffolded  = "scaffolded"
	StatusPartial     = "partial"
	StatusImplemented = "implemented"
	StatusVerified    = "verified"
	StatusDeferred    = "deferred"
)

// StatusOrder lists the capability statuses in lifecycle order. It doubles as
// the deterministic ordering used by the report and the enum enforced by
// Validate.
var StatusOrder = []string{
	StatusInventoried,
	StatusScaffolded,
	StatusPartial,
	StatusImplemented,
	StatusVerified,
	StatusDeferred,
}

// milestoneEnum is the fixed set of roadmap milestones M0..M14.
var milestoneEnum = func() map[string]bool {
	m := make(map[string]bool, 15)
	for i := 0; i <= 14; i++ {
		m["M"+strconv.Itoa(i)] = true
	}
	return m
}()

// classificationEnum aligns with the extraction contract of issue #20.
var classificationEnum = map[string]bool{
	"direct-entry":         true,
	"public-api":           true,
	"private-impl":         true,
	"dormant-test-support": true,
}

// mappingKindEnum lists the valid mapping kinds.
var mappingKindEnum = map[string]bool{
	"package":  true,
	"command":  true,
	"symbol":   true,
	"contract": true,
	"field":    true,
	"behavior": true,
}

// Capability-matrix enum values. They are exported so catalog producers and
// tests do not duplicate wire strings.
const (
	MatrixSurfacePublicAPI    = "public-api"
	MatrixSurfaceInternalWire = "internal-wire"
	MatrixSurfaceFixture      = "fixture"

	MatrixCategoryEntrypoint = "entrypoint"
	MatrixCategoryMessage    = "message"
	MatrixCategoryContent    = "content"
	MatrixCategoryTool       = "tool"
	MatrixCategoryToolChoice = "tool-choice"
	MatrixCategoryEvent      = "event"
	MatrixCategoryUsage      = "usage"
	MatrixCategoryError      = "error"
	MatrixCategoryOption     = "option"
	MatrixCategoryRequest    = "request"
	MatrixCategoryHeader     = "header"
	MatrixCategorySSE        = "sse"
	MatrixCategoryDelta      = "delta"
	MatrixCategoryCompat     = "compat"

	MatrixDirectionRequest       = "request"
	MatrixDirectionResponse      = "response"
	MatrixDirectionBidirectional = "bidirectional"

	MatrixBehaviorErrNotImplemented = "err-not-implemented"
	MatrixBehaviorIgnore            = "ignore"
	MatrixBehaviorNoOp              = "no-op"

	MatrixValueAbsent  = "absent"
	MatrixValueNull    = "null"
	MatrixValueFalse   = "false"
	MatrixValueZero    = "zero"
	MatrixValueEmpty   = "empty"
	MatrixValueDefault = "default"
	MatrixValueValue   = "value"

	MatrixEvidenceGoTest      = "go-test"
	MatrixEvidenceFixture     = "fixture"
	MatrixEvidenceOracle      = "oracle"
	MatrixEvidenceLocalServer = "local-server"
	MatrixEvidenceSmoke       = "smoke"
)

var matrixSurfaceEnum = map[string]bool{
	MatrixSurfacePublicAPI:    true,
	MatrixSurfaceInternalWire: true,
	MatrixSurfaceFixture:      true,
}

var matrixCategoryEnum = map[string]bool{
	MatrixCategoryEntrypoint: true,
	MatrixCategoryMessage:    true,
	MatrixCategoryContent:    true,
	MatrixCategoryTool:       true,
	MatrixCategoryToolChoice: true,
	MatrixCategoryEvent:      true,
	MatrixCategoryUsage:      true,
	MatrixCategoryError:      true,
	MatrixCategoryOption:     true,
	MatrixCategoryRequest:    true,
	MatrixCategoryHeader:     true,
	MatrixCategorySSE:        true,
	MatrixCategoryDelta:      true,
	MatrixCategoryCompat:     true,
}

var matrixDirectionEnum = map[string]bool{
	MatrixDirectionRequest:       true,
	MatrixDirectionResponse:      true,
	MatrixDirectionBidirectional: true,
}

var matrixBehaviorEnum = map[string]bool{
	MatrixBehaviorErrNotImplemented: true,
	MatrixBehaviorIgnore:            true,
	MatrixBehaviorNoOp:              true,
}

var matrixValueStateEnum = map[string]bool{
	MatrixValueAbsent:  true,
	MatrixValueNull:    true,
	MatrixValueFalse:   true,
	MatrixValueZero:    true,
	MatrixValueEmpty:   true,
	MatrixValueDefault: true,
	MatrixValueValue:   true,
}

var matrixEvidenceKindEnum = map[string]bool{
	MatrixEvidenceGoTest:      true,
	MatrixEvidenceFixture:     true,
	MatrixEvidenceOracle:      true,
	MatrixEvidenceLocalServer: true,
	MatrixEvidenceSmoke:       true,
}

func statusIndex(status string) int {
	for i, s := range StatusOrder {
		if s == status {
			return i
		}
	}
	return len(StatusOrder)
}

func validStatus(status string) bool { return statusIndex(status) < len(StatusOrder) }

// ErrorKind identifies a validation failure category.
type ErrorKind string

// Validation failure kinds.
const (
	KindDuplicateID        ErrorKind = "duplicate_id"
	KindIllegalStatus      ErrorKind = "illegal_status"
	KindIllegalMilestone   ErrorKind = "illegal_milestone"
	KindIllegalClass       ErrorKind = "illegal_classification"
	KindIllegalKind        ErrorKind = "illegal_mapping_kind"
	KindMissingProvenance  ErrorKind = "missing_provenance"
	KindCommitMismatch     ErrorKind = "commit_mismatch"
	KindDeferredWithoutADR ErrorKind = "deferred_without_adr"
	KindIncompletePartial  ErrorKind = "incomplete_partial"
	KindMissingEvidence    ErrorKind = "missing_evidence"
	KindIncompleteMatrix   ErrorKind = "incomplete_matrix"
	KindIllegalMatrixValue ErrorKind = "illegal_matrix_value"
	KindDuplicateMatrix    ErrorKind = "duplicate_matrix"
	KindManifestMismatch   ErrorKind = "manifest_mismatch"
	KindSchemaVersion      ErrorKind = "schema_version"
)

// Sentinel errors, matchable with errors.Is.
var (
	ErrDuplicateID        = errors.New("duplicate catalog entry id")
	ErrIllegalStatus      = errors.New("illegal capability status")
	ErrIllegalMilestone   = errors.New("illegal milestone")
	ErrIllegalClass       = errors.New("illegal classification")
	ErrIllegalKind        = errors.New("illegal mapping kind")
	ErrMissingProvenance  = errors.New("missing upstream provenance or mapping target")
	ErrCommitMismatch     = errors.New("upstream commit does not match manifest baseline")
	ErrDeferredWithoutADR = errors.New("deferred entry missing adr or target milestone")
	ErrIncompletePartial  = errors.New("partial entry missing supported or unsupported branches")
	ErrMissingEvidence    = errors.New("implemented or verified entry missing evidence")
	ErrIncompleteMatrix   = errors.New("capability matrix entry is incomplete")
	ErrIllegalMatrixValue = errors.New("illegal capability matrix value")
	ErrDuplicateMatrix    = errors.New("duplicate capability matrix coordinate")
	ErrManifestMismatch   = errors.New("manifest counts do not match entries")
	ErrSchemaVersion      = errors.New("entry schema version missing or inconsistent")
	ErrMalformedLine      = errors.New("malformed catalog line")
)

// ValidationError identifies a single validation failure. It unwraps to a
// sentinel so callers can match either the Kind or errors.Is the sentinel.
type ValidationError struct {
	Kind   ErrorKind
	ID     string
	Detail string
	err    error
}

func (e *ValidationError) Error() string {
	msg := string(e.Kind) + ": " + e.err.Error()
	if e.ID != "" {
		msg += " (id=" + e.ID + ")"
	}
	if e.Detail != "" {
		msg += ": " + e.Detail
	}
	return msg
}

// Unwrap returns the sentinel error backing this validation failure.
func (e *ValidationError) Unwrap() error { return e.err }

func newValidationError(kind ErrorKind, sentinel error, id, detail string) *ValidationError {
	return &ValidationError{Kind: kind, ID: id, Detail: detail, err: sentinel}
}

// ParseError identifies a malformed line while loading JSONL.
type ParseError struct {
	Line int
	err  error
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("%s: line %d: %v", ErrMalformedLine.Error(), e.Line, e.err)
}

// Unwrap returns ErrMalformedLine so callers can match it with errors.Is.
func (e *ParseError) Unwrap() error { return ErrMalformedLine }

// LoadCatalog parses a JSONL catalog file line by line. A malformed line is an
// error identifying the line number.
func LoadCatalog(path string) ([]Entry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var entries []Entry
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
		var entry Entry
		if err := dec.Decode(&entry); err != nil {
			return nil, &ParseError{Line: line, err: err}
		}
		if dec.More() {
			return nil, &ParseError{Line: line, err: errors.New("more than one JSON value on line")}
		}
		entries = append(entries, entry)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}

// ParseManifest decodes a manifest JSON document.
func ParseManifest(data []byte) (Manifest, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var manifest Manifest
	if err := dec.Decode(&manifest); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

// Validate enforces the Parity Catalog rules against entries and manifest.
//
// It rejects: duplicate ids; illegal status/milestone/classification/kind;
// missing upstream provenance (repository/commit/reference) or mapping target;
// upstream commit that differs from the manifest baseline; deferred entries
// without an ADR or target milestone; partial entries without both supported
// and unsupported branches; implemented/verified entries without evidence; and
// manifests whose entry_count or status_counts disagree with the entries.
func Validate(entries []Entry, manifest Manifest) error {
	if manifest.SchemaVersion != SchemaVersion {
		return newValidationError(KindSchemaVersion, ErrSchemaVersion, "",
			fmt.Sprintf("manifest schema_version=%q want=%q", manifest.SchemaVersion, SchemaVersion))
	}
	if manifest.CatalogVersion != SchemaVersion {
		return newValidationError(KindSchemaVersion, ErrSchemaVersion, "",
			fmt.Sprintf("manifest catalog_version=%q want=%q", manifest.CatalogVersion, SchemaVersion))
	}

	seen := make(map[string]bool, len(entries))
	seenMatrix := make(map[string]string, len(entries))
	counts := make(map[string]int, len(StatusOrder))

	for _, entry := range entries {
		if entry.SchemaVersion != SchemaVersion {
			return newValidationError(KindSchemaVersion, ErrSchemaVersion, entry.ID,
				fmt.Sprintf("entry schema_version=%q want=%q", entry.SchemaVersion, SchemaVersion))
		}
		if entry.ID == "" {
			return newValidationError(KindMissingProvenance, ErrMissingProvenance, entry.ID, "entry id missing")
		}
		if seen[entry.ID] {
			return newValidationError(KindDuplicateID, ErrDuplicateID, entry.ID, "")
		}
		seen[entry.ID] = true

		if !validStatus(entry.Status) {
			return newValidationError(KindIllegalStatus, ErrIllegalStatus, entry.ID, entry.Status)
		}
		if !milestoneEnum[entry.Milestone] {
			return newValidationError(KindIllegalMilestone, ErrIllegalMilestone, entry.ID, entry.Milestone)
		}
		if !classificationEnum[entry.Classification] {
			return newValidationError(KindIllegalClass, ErrIllegalClass, entry.ID, entry.Classification)
		}
		if !mappingKindEnum[entry.Mapping.Kind] {
			return newValidationError(KindIllegalKind, ErrIllegalKind, entry.ID, entry.Mapping.Kind)
		}

		switch {
		case entry.Upstream.Module == "":
			return newValidationError(KindMissingProvenance, ErrMissingProvenance, entry.ID, "upstream.module")
		case entry.Upstream.Repository == "":
			return newValidationError(KindMissingProvenance, ErrMissingProvenance, entry.ID, "upstream.repository")
		case entry.Upstream.Commit == "":
			return newValidationError(KindMissingProvenance, ErrMissingProvenance, entry.ID, "upstream.commit")
		case entry.Upstream.Reference == "":
			return newValidationError(KindMissingProvenance, ErrMissingProvenance, entry.ID, "upstream.reference")
		case entry.Mapping.Module == "":
			return newValidationError(KindMissingProvenance, ErrMissingProvenance, entry.ID, "mapping.module")
		case entry.Mapping.Target == "":
			return newValidationError(KindMissingProvenance, ErrMissingProvenance, entry.ID, "mapping.target")
		}

		if entry.Upstream.Commit != manifest.BaselineCommit {
			return newValidationError(KindCommitMismatch, ErrCommitMismatch, entry.ID, entry.Upstream.Commit)
		}

		if entry.Status == StatusDeferred {
			if entry.Deferred == nil || strings.TrimSpace(entry.Deferred.ADR) == "" || strings.TrimSpace(entry.Deferred.Milestone) == "" || strings.TrimSpace(entry.Deferred.Reason) == "" {
				return newValidationError(KindDeferredWithoutADR, ErrDeferredWithoutADR, entry.ID, "")
			}
		}
		if entry.Deviation != nil && (strings.TrimSpace(entry.Deviation.ADR) == "" || strings.TrimSpace(entry.Deviation.Reason) == "") {
			return newValidationError(KindMissingProvenance, ErrMissingProvenance, entry.ID, "deviation adr and reason are required")
		}

		if entry.Status == StatusPartial {
			if entry.Partial == nil || len(entry.Partial.Supported) == 0 || len(entry.Partial.Unsupported) == 0 {
				return newValidationError(KindIncompletePartial, ErrIncompletePartial, entry.ID, "")
			}
		}

		if entry.Status == StatusImplemented || entry.Status == StatusVerified {
			if len(entry.Evidence) == 0 {
				return newValidationError(KindMissingEvidence, ErrMissingEvidence, entry.ID, entry.Status)
			}
		}
		if err := validateEvidence(entry, manifest.BaselineCommit); err != nil {
			return err
		}

		if (entry.Mapping.Kind == "field" || entry.Mapping.Kind == "behavior") && entry.Matrix == nil {
			return newValidationError(KindIncompleteMatrix, ErrIncompleteMatrix, entry.ID, entry.Mapping.Kind+" mapping requires matrix metadata")
		}
		if entry.Matrix != nil {
			if err := validateCapabilityMatrix(entry); err != nil {
				return err
			}
			coordinate := strings.Join([]string{entry.Matrix.API, entry.Matrix.Surface, entry.Matrix.Category, entry.Matrix.Pi.Name}, "\x00")
			if previousID, ok := seenMatrix[coordinate]; ok {
				return newValidationError(KindDuplicateMatrix, ErrDuplicateMatrix, entry.ID, "same api/surface/category/Pi name as "+previousID)
			}
			seenMatrix[coordinate] = entry.ID
		}

		counts[entry.Status]++
	}

	if manifest.EntryCount != len(entries) {
		return newValidationError(KindManifestMismatch, ErrManifestMismatch, "",
			fmt.Sprintf("entry_count=%d actual=%d", manifest.EntryCount, len(entries)))
	}
	if err := compareCounts(manifest.StatusCounts, counts); err != nil {
		return err
	}

	return nil
}

func validateCapabilityMatrix(entry Entry) error {
	matrix := entry.Matrix
	if entry.Mapping.Kind != "field" && entry.Mapping.Kind != "behavior" && entry.Mapping.Kind != "symbol" {
		return newValidationError(KindIncompleteMatrix, ErrIncompleteMatrix, entry.ID, "matrix mapping.kind must be field, behavior, or symbol")
	}
	if matrix.Category == MatrixCategoryEntrypoint && entry.Mapping.Kind != "symbol" {
		return newValidationError(KindIncompleteMatrix, ErrIncompleteMatrix, entry.ID, "entrypoint matrix rows require symbol mappings")
	}
	if entry.Mapping.Kind == "symbol" && matrix.Category != MatrixCategoryEntrypoint {
		return newValidationError(KindIncompleteMatrix, ErrIncompleteMatrix, entry.ID, "symbol matrix rows require entrypoint category")
	}
	if strings.TrimSpace(matrix.API) == "" || strings.TrimSpace(matrix.Pi.Name) == "" || strings.TrimSpace(matrix.Pi.Type) == "" || strings.TrimSpace(matrix.Go.Name) == "" || strings.TrimSpace(matrix.Go.Type) == "" {
		return newValidationError(KindIncompleteMatrix, ErrIncompleteMatrix, entry.ID, "api and Pi/Go names and types are required")
	}
	if !matrixSurfaceEnum[matrix.Surface] {
		return newValidationError(KindIllegalMatrixValue, ErrIllegalMatrixValue, entry.ID, "surface="+matrix.Surface)
	}
	if !matrixCategoryEnum[matrix.Category] {
		return newValidationError(KindIllegalMatrixValue, ErrIllegalMatrixValue, entry.ID, "category="+matrix.Category)
	}
	if !matrixDirectionEnum[matrix.Direction] {
		return newValidationError(KindIllegalMatrixValue, ErrIllegalMatrixValue, entry.ID, "direction="+matrix.Direction)
	}
	if !matrixBehaviorEnum[matrix.PreTargetBehavior] {
		return newValidationError(KindIllegalMatrixValue, ErrIllegalMatrixValue, entry.ID, "pre_target_behavior="+matrix.PreTargetBehavior)
	}
	if len(matrix.ValueSemantics.States) == 0 || strings.TrimSpace(matrix.ValueSemantics.Description) == "" {
		return newValidationError(KindIncompleteMatrix, ErrIncompleteMatrix, entry.ID, "value_semantics states and description are required")
	}
	seenStates := make(map[string]bool, len(matrix.ValueSemantics.States))
	for _, state := range matrix.ValueSemantics.States {
		if !matrixValueStateEnum[state] || seenStates[state] {
			return newValidationError(KindIllegalMatrixValue, ErrIllegalMatrixValue, entry.ID, "value_semantics state="+state)
		}
		seenStates[state] = true
	}
	if len(matrix.EvidenceRequirements) == 0 {
		return newValidationError(KindIncompleteMatrix, ErrIncompleteMatrix, entry.ID, "evidence_requirements are required")
	}
	for _, requirement := range matrix.EvidenceRequirements {
		if !matrixEvidenceKindEnum[requirement.Kind] {
			return newValidationError(KindIllegalMatrixValue, ErrIllegalMatrixValue, entry.ID, "evidence kind="+requirement.Kind)
		}
		if strings.TrimSpace(requirement.Assertion) == "" {
			return newValidationError(KindIncompleteMatrix, ErrIncompleteMatrix, entry.ID, "evidence assertion is required")
		}
	}
	return nil
}

func validateEvidence(entry Entry, baselineCommit string) error {
	complete := entry.Status == StatusVerified
	achievedKinds := make(map[string]bool, len(entry.Evidence))
	seenCases := make(map[string]bool, len(entry.Evidence))
	for _, evidence := range entry.Evidence {
		if strings.TrimSpace(evidence.Kind) == "" || strings.TrimSpace(evidence.Ref) == "" || strings.TrimSpace(evidence.Baseline) == "" {
			return newValidationError(KindMissingEvidence, ErrMissingEvidence, entry.ID, "evidence kind, ref and baseline are required")
		}
		if evidence.Baseline != baselineCommit {
			return newValidationError(KindMissingEvidence, ErrMissingEvidence, entry.ID, "evidence baseline must equal manifest baseline_commit")
		}
		achievedKinds[evidence.Kind] = true
		if !complete {
			continue
		}

		if strings.TrimSpace(evidence.CaseID) == "" || strings.TrimSpace(evidence.InputHash) == "" ||
			strings.TrimSpace(evidence.ExecutionMethod) == "" || strings.TrimSpace(evidence.Expected) == "" ||
			strings.TrimSpace(evidence.Actual) == "" || strings.TrimSpace(evidence.Platform) == "" ||
			strings.TrimSpace(evidence.CatalogID) == "" {
			return newValidationError(KindMissingEvidence, ErrMissingEvidence, entry.ID,
				"completed evidence requires case_id, input_hash, execution_method, expected, actual, platform and catalog_id")
		}
		if !validSHA256(evidence.InputHash) {
			return newValidationError(KindMissingEvidence, ErrMissingEvidence, entry.ID, "input_hash must be sha256:<64 lowercase hex>")
		}
		if evidence.CatalogID != entry.ID {
			return newValidationError(KindMissingEvidence, ErrMissingEvidence, entry.ID, "evidence catalog_id must equal its containing entry id")
		}
		caseKey := evidence.Kind + "\x00" + evidence.CaseID
		if seenCases[caseKey] {
			return newValidationError(KindMissingEvidence, ErrMissingEvidence, entry.ID, "duplicate evidence kind/case_id")
		}
		seenCases[caseKey] = true
	}

	if entry.Status != StatusVerified || entry.Matrix == nil {
		return nil
	}
	requiredKinds := make(map[string]bool, len(entry.Matrix.EvidenceRequirements)+2)
	for _, requirement := range entry.Matrix.EvidenceRequirements {
		requiredKinds[requirement.Kind] = true
	}
	if entry.Matrix.PreTargetBehavior == MatrixBehaviorNoOp {
		requiredKinds[MatrixEvidenceOracle] = true
		requiredKinds[MatrixEvidenceGoTest] = true
	}
	for kind := range requiredKinds {
		if !achievedKinds[kind] {
			return newValidationError(KindMissingEvidence, ErrMissingEvidence, entry.ID, "verified matrix entry lacks required "+kind+" evidence")
		}
	}
	return nil
}

func validSHA256(value string) bool {
	const prefix = "sha256:"
	if !strings.HasPrefix(value, prefix) || len(value) != len(prefix)+64 {
		return false
	}
	digest := value[len(prefix):]
	if digest != strings.ToLower(digest) {
		return false
	}
	_, err := hex.DecodeString(digest)
	return err == nil
}

// compareCounts checks that the manifest status_counts equal the actual counts,
// ignoring statuses that are explicitly zero on both sides.
func compareCounts(manifestCounts, actual map[string]int) error {
	keys := make(map[string]bool)
	for k := range manifestCounts {
		keys[k] = true
	}
	for k := range actual {
		keys[k] = true
	}
	for k := range keys {
		if manifestCounts[k] != actual[k] {
			return newValidationError(KindManifestMismatch, ErrManifestMismatch, "",
				fmt.Sprintf("status_counts[%s]=%d actual=%d", k, manifestCounts[k], actual[k]))
		}
	}
	return nil
}

// GenerateReport renders the non-authoritative Markdown report from entries.
// The output is deterministic and is the sole source of parity/reports/catalog.md.
func GenerateReport(entries []Entry) string {
	counts := make(map[string]int, len(StatusOrder))
	byModule := make(map[string][]Entry)
	var matrixEntries []Entry
	for _, entry := range entries {
		counts[entry.Status]++
		byModule[entry.Mapping.Module] = append(byModule[entry.Mapping.Module], entry)
		if entry.Matrix != nil {
			matrixEntries = append(matrixEntries, entry)
		}
	}

	var b strings.Builder
	b.WriteString(NonAuthoritativeBanner)
	b.WriteString("\n\n## Summary\n\n")
	fmt.Fprintf(&b, "- Total entries: %d\n\n", len(entries))
	b.WriteString("| Status | Count |\n| --- | --- |\n")
	for _, status := range StatusOrder {
		fmt.Fprintf(&b, "| %s | %d |\n", status, counts[status])
	}

	b.WriteString("\n## Entries by module\n")
	modules := make([]string, 0, len(byModule))
	for module := range byModule {
		modules = append(modules, module)
	}
	sort.Strings(modules)
	for _, module := range modules {
		group := byModule[module]
		sort.Slice(group, func(i, j int) bool {
			if si, sj := statusIndex(group[i].Status), statusIndex(group[j].Status); si != sj {
				return si < sj
			}
			return group[i].ID < group[j].ID
		})
		fmt.Fprintf(&b, "\n### %s\n\n", module)
		b.WriteString("| ID | Status | Milestone | Kind | Target | Upstream |\n")
		b.WriteString("| --- | --- | --- | --- | --- | --- |\n")
		for _, entry := range group {
			fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s |\n",
				entry.ID, entry.Status, entry.Milestone, entry.Mapping.Kind, entry.Mapping.Target, entry.Upstream.Module)
		}
	}

	if len(matrixEntries) > 0 {
		sort.Slice(matrixEntries, func(i, j int) bool {
			left, right := matrixEntries[i], matrixEntries[j]
			if left.Matrix.API != right.Matrix.API {
				return left.Matrix.API < right.Matrix.API
			}
			if left.Matrix.Surface != right.Matrix.Surface {
				return left.Matrix.Surface < right.Matrix.Surface
			}
			if left.Matrix.Category != right.Matrix.Category {
				return left.Matrix.Category < right.Matrix.Category
			}
			return left.ID < right.ID
		})
		b.WriteString("\n## OpenAI Chat Completions capability matrix\n\n")
		b.WriteString("This table is generated from field, entrypoint, and behavior entries in `parity/catalog.jsonl`; the catalog remains authoritative.\n\n")
		b.WriteString("| Surface | Category | Pi capability / type | Go mapping / type | Direction | Target | Status | Before target | Value semantics | Evidence required | Pi source |\n")
		b.WriteString("| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |\n")
		for _, entry := range matrixEntries {
			matrix := entry.Matrix
			fmt.Fprintf(&b, "| %s | %s | %s<br>%s | %s<br>%s | %s | %s | %s | %s | %s | %s | %s |\n",
				markdownCell(matrix.Surface), markdownCell(matrix.Category), markdownCode(matrix.Pi.Name), markdownCode(matrix.Pi.Type),
				markdownCode(matrix.Go.Name), markdownCode(matrix.Go.Type), markdownCell(matrix.Direction), markdownCell(entry.Milestone),
				markdownCell(entry.Status), markdownCell(matrix.PreTargetBehavior), markdownCell(formatValueSemantics(matrix.ValueSemantics)),
				markdownCell(formatEvidenceRequirements(matrix.EvidenceRequirements)), markdownCode(entry.Upstream.Reference))
		}
	}

	return b.String()
}

func markdownCell(value string) string {
	value = strings.ReplaceAll(value, "|", "\\|")
	value = strings.ReplaceAll(value, "\r\n", "<br>")
	value = strings.ReplaceAll(value, "\n", "<br>")
	return value
}

func markdownCode(value string) string {
	value = markdownCell(value)
	longestRun := 0
	currentRun := 0
	for _, r := range value {
		if r == '`' {
			currentRun++
			if currentRun > longestRun {
				longestRun = currentRun
			}
		} else {
			currentRun = 0
		}
	}
	if longestRun == 0 {
		return "`" + value + "`"
	}
	fence := strings.Repeat("`", longestRun+1)
	return fence + " " + value + " " + fence
}

func formatValueSemantics(semantics MatrixValueSemantics) string {
	return strings.Join(semantics.States, ", ") + " — " + semantics.Description
}

func formatEvidenceRequirements(requirements []MatrixEvidenceRequirement) string {
	formatted := make([]string, len(requirements))
	for i, requirement := range requirements {
		formatted[i] = requirement.Kind + ": " + requirement.Assertion
	}
	return strings.Join(formatted, "; ")
}

// EncodeEntries serialises entries as canonical catalog JSONL: one compact
// object per line, sorted by id, with HTML escaping disabled so ids and
// references (which contain <, >, & in generics like Promise<T> or Map<K,V>)
// stay byte-clean and the diff stays minimal.
func EncodeEntries(entries []Entry) ([]byte, error) {
	sorted := append([]Entry(nil), entries...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	for _, entry := range sorted {
		if err := enc.Encode(entry); err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}

// ManifestPaths carries the fixed relative paths the catalog manifest records.
// They are stable repository conventions, so BuildManifest takes them as one
// value rather than several positional strings.
type ManifestPaths struct {
	Catalog         string
	Schema          string
	GeneratedReport string
}

// DefaultManifestPaths are the committed relative paths under parity/.
var DefaultManifestPaths = ManifestPaths{
	Catalog:         "catalog.jsonl",
	Schema:          "catalog.schema.json",
	GeneratedReport: "reports/catalog.md",
}

// BuildManifest derives the catalog manifest from entries. status_counts always
// lists every status in StatusOrder (zeros included) so the manifest shape is
// stable and diff-minimal as statuses come and go.
func BuildManifest(entries []Entry, baselineCommit string, paths ManifestPaths) Manifest {
	counts := make(map[string]int, len(StatusOrder))
	for _, s := range StatusOrder {
		counts[s] = 0
	}
	for _, e := range entries {
		counts[e.Status]++
	}
	return Manifest{
		SchemaVersion:   SchemaVersion,
		CatalogVersion:  SchemaVersion,
		BaselineCommit:  baselineCommit,
		Catalog:         paths.Catalog,
		Schema:          paths.Schema,
		GeneratedReport: paths.GeneratedReport,
		EntryCount:      len(entries),
		StatusCounts:    counts,
	}
}

// EncodeManifest serialises the manifest as pretty JSON with a trailing newline,
// matching the committed formatting (two-space indent, HTML escaping off).
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

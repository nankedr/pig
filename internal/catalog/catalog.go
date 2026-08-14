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
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

// SchemaVersion is the current catalog entry/manifest schema version.
const SchemaVersion = "1.0.0"

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

// Evidence binds a catalog entry to repeatable verification material.
type Evidence struct {
	Kind     string `json:"kind"`
	Ref      string `json:"ref"`
	Baseline string `json:"baseline"`
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
// one such object per line.
type Entry struct {
	SchemaVersion  string     `json:"schema_version"`
	ID             string     `json:"id"`
	Upstream       Upstream   `json:"upstream"`
	Mapping        Mapping    `json:"mapping"`
	Status         string     `json:"status"`
	Milestone      string     `json:"milestone"`
	Classification string     `json:"classification"`
	Evidence       []Evidence `json:"evidence"`
	Deviation      *Deviation `json:"deviation"`
	Partial        *Partial   `json:"partial"`
	Deferred       *Deferred  `json:"deferred"`
	Notes          string     `json:"notes"`
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
	if manifest.SchemaVersion == "" {
		return newValidationError(KindSchemaVersion, ErrSchemaVersion, "", "manifest schema_version missing")
	}

	seen := make(map[string]bool, len(entries))
	counts := make(map[string]int, len(StatusOrder))

	for _, entry := range entries {
		if entry.SchemaVersion == "" {
			return newValidationError(KindSchemaVersion, ErrSchemaVersion, entry.ID, "entry schema_version missing")
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
		case entry.Upstream.Repository == "":
			return newValidationError(KindMissingProvenance, ErrMissingProvenance, entry.ID, "upstream.repository")
		case entry.Upstream.Commit == "":
			return newValidationError(KindMissingProvenance, ErrMissingProvenance, entry.ID, "upstream.commit")
		case entry.Upstream.Reference == "":
			return newValidationError(KindMissingProvenance, ErrMissingProvenance, entry.ID, "upstream.reference")
		case entry.Mapping.Target == "":
			return newValidationError(KindMissingProvenance, ErrMissingProvenance, entry.ID, "mapping.target")
		}

		if entry.Upstream.Commit != manifest.BaselineCommit {
			return newValidationError(KindCommitMismatch, ErrCommitMismatch, entry.ID, entry.Upstream.Commit)
		}

		if entry.Status == StatusDeferred {
			if entry.Deferred == nil || entry.Deferred.ADR == "" || entry.Deferred.Milestone == "" {
				return newValidationError(KindDeferredWithoutADR, ErrDeferredWithoutADR, entry.ID, "")
			}
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
	for _, entry := range entries {
		counts[entry.Status]++
		byModule[entry.Mapping.Module] = append(byModule[entry.Mapping.Module], entry)
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

	return b.String()
}

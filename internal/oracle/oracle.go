// Package oracle implements the offline, pure-stdlib replay of the Pi Oracle
// fixtures for Milestone 0 (issue #22).
//
// The Pi Oracle is the on-demand bridge to the frozen Pi baseline: the harness
// parity/oracle/run.mjs drives a deterministic protocol case through Pi's real
// codec and records the raw wire output plus full provenance to a committed
// fixture (parity/oracle/fixtures/protocol-frame.json). That capture is the ONLY
// step that needs Node and a Pi checkout.
//
// This package replays a captured fixture with no Node, no Pi checkout and no
// subprocess: it loads the fixture, verifies its provenance and honesty
// (refusing any fixture not marked deterministic), and independently re-derives
// the length-prefixed frame header via encoding/binary.BigEndian. Re-deriving
// the header through Go's stdlib codec — rather than transliterating Pi's
// hand-rolled bit shifts — makes the replay a genuine cross-implementation
// parity check. Like internal/baseline, internal/inventory and internal/surface,
// this package is stdlib-only and MUST NOT be imported by cmd/pig or cmd/pig-ai,
// whose binaries are asserted to carry no net/os/exec dependencies.
package oracle

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

// SchemaVersion is the fixture schema version. It matches the value emitted by
// parity/oracle/run.mjs.
const SchemaVersion = "1.0.0"

// Repository is the fixed upstream repository every fixture is captured from.
const Repository = "https://github.com/badlogic/pi-mono"

// frameHeaderLength is the size of the length prefix Pi's frame codec writes
// (packages/protocol/src/framing.ts, FRAME_HEADER_LENGTH). The header is an
// unsigned 32-bit big-endian byte count.
const frameHeaderLength = 4

// Upstream records the fixed Pi baseline provenance for a fixture. Its shape
// mirrors catalog.Upstream but is kept local so this package stays independent.
type Upstream struct {
	Module     string `json:"module"`
	Repository string `json:"repository"`
	Commit     string `json:"commit"`
	Reference  string `json:"reference"`
}

// Input describes the deterministic case fed to the Pi codec. Message is kept as
// raw JSON: it is informational provenance, while the authoritative output is the
// captured wire bytes in RawOutput.
type Input struct {
	Description string          `json:"description"`
	Message     json.RawMessage `json:"message"`
	Encoding    string          `json:"encoding"`
}

// RawOutput is the captured wire output: the intermediate CBOR bytes, the framed
// bytes, and the 4-byte length prefix, each as lowercase hex with its length.
type RawOutput struct {
	Encoding     string `json:"encoding"`
	CBORHex      string `json:"cbor_hex"`
	CBORLength   int    `json:"cbor_length"`
	FrameHex     string `json:"frame_hex"`
	FrameLength  int    `json:"frame_length"`
	HeaderHex    string `json:"header_hex"`
	HeaderLength int    `json:"header_length"`
}

// Hash carries the digest of the framed bytes and the algorithm that produced it.
type Hash struct {
	Algorithm   string `json:"algorithm"`
	FrameSHA256 string `json:"frame_sha256"`
}

// Env records the host the capture ran on. It is provenance only and never
// participates in replay verification, so a fixture reproduces on any host.
type Env struct {
	NodeVersion string `json:"node_version"`
	Platform    string `json:"platform"`
	Arch        string `json:"arch"`
}

// Fixture is one deterministic Pi Oracle capture. The committed
// protocol-frame.json holds exactly this shape.
type Fixture struct {
	SchemaVersion  string    `json:"schema_version"`
	ID             string    `json:"id"`
	CatalogID      string    `json:"catalog_id"`
	BaselineID     string    `json:"baseline_id"`
	BaselineCommit string    `json:"baseline_commit"`
	Deterministic  bool      `json:"deterministic"`
	Upstream       Upstream  `json:"upstream"`
	Input          Input     `json:"input"`
	RawOutput      RawOutput `json:"raw_output"`
	Hash           Hash      `json:"hash"`
	Env            Env       `json:"env"`
	ExecMethod     string    `json:"exec_method"`
}

// Kind identifies a replay failure category.
type Kind string

// Replay failure kinds.
const (
	KindNotDeterministic Kind = "not_deterministic"
	KindMissingField     Kind = "missing_field"
	KindProvenance       Kind = "provenance_mismatch"
	KindUnsupportedHash  Kind = "unsupported_hash"
	KindCommitMismatch   Kind = "commit_mismatch"
	KindBadHex           Kind = "bad_hex"
	KindLengthMismatch   Kind = "length_mismatch"
	KindHeaderMismatch   Kind = "header_mismatch"
	KindPayloadMismatch  Kind = "payload_mismatch"
	KindHashMismatch     Kind = "hash_mismatch"
)

// Sentinel errors, matchable with errors.Is.
var (
	ErrNotDeterministic = errors.New("oracle: fixture is not marked deterministic")
	ErrMissingField     = errors.New("oracle: fixture missing required field")
	ErrProvenance       = errors.New("oracle: fixture provenance does not match the fixed baseline")
	ErrUnsupportedHash  = errors.New("oracle: unsupported hash algorithm")
	ErrCommitMismatch   = errors.New("oracle: fixture commit does not match the expected baseline")
	ErrBadHex           = errors.New("oracle: fixture carries malformed hex")
	ErrLengthMismatch   = errors.New("oracle: recorded length does not match the decoded bytes")
	ErrHeaderMismatch   = errors.New("oracle: re-derived big-endian frame header does not match")
	ErrPayloadMismatch  = errors.New("oracle: frame payload does not match the recorded CBOR output")
	ErrHashMismatch     = errors.New("oracle: recorded hash does not match the framed bytes")
	ErrMalformedFixture = errors.New("oracle: malformed fixture file")
)

func sentinelFor(k Kind) error {
	switch k {
	case KindNotDeterministic:
		return ErrNotDeterministic
	case KindMissingField:
		return ErrMissingField
	case KindProvenance:
		return ErrProvenance
	case KindUnsupportedHash:
		return ErrUnsupportedHash
	case KindCommitMismatch:
		return ErrCommitMismatch
	case KindBadHex:
		return ErrBadHex
	case KindLengthMismatch:
		return ErrLengthMismatch
	case KindHeaderMismatch:
		return ErrHeaderMismatch
	case KindPayloadMismatch:
		return ErrPayloadMismatch
	case KindHashMismatch:
		return ErrHashMismatch
	default:
		return nil
	}
}

// Error is the typed replay error. It carries a Kind (for errors.As) and unwraps
// to its Kind sentinel (for errors.Is).
type Error struct {
	Kind    Kind
	ID      string
	Message string
}

func (e *Error) Error() string {
	msg := string(e.Kind)
	if e.ID != "" {
		msg += " (" + e.ID + ")"
	}
	if e.Message != "" {
		msg += ": " + e.Message
	}
	return msg
}

// Unwrap returns the Kind sentinel so errors.Is matches it.
func (e *Error) Unwrap() error { return sentinelFor(e.Kind) }

func newError(kind Kind, id, format string, args ...any) *Error {
	return &Error{Kind: kind, ID: id, Message: fmt.Sprintf(format, args...)}
}

// LoadFixture reads and decodes a Pi Oracle fixture JSON file. Unknown fields are
// rejected so the on-disk shape stays exactly what the harness writes; a decode
// failure is reported as ErrMalformedFixture.
func LoadFixture(path string) (Fixture, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Fixture{}, err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var f Fixture
	if err := dec.Decode(&f); err != nil {
		return Fixture{}, fmt.Errorf("%w: %v", ErrMalformedFixture, err)
	}
	if dec.More() {
		return Fixture{}, fmt.Errorf("%w: trailing data after fixture object", ErrMalformedFixture)
	}
	return f, nil
}

// Replay verifies a captured fixture fully offline against the expected baseline
// commit. It refuses any fixture not marked deterministic, checks required
// fields and provenance, then independently re-derives the length-prefixed frame
// header via encoding/binary.BigEndian and confirms the recorded output, header,
// payload and hash are mutually consistent.
//
// It rejects: a fixture not marked deterministic; missing required fields;
// provenance that does not match the fixed repository; an unsupported hash
// algorithm; a baseline commit that differs from the expected commit (or an
// upstream commit that disagrees with it); malformed hex; recorded lengths that
// disagree with the decoded bytes; a frame header that disagrees with the
// re-derived big-endian length of the payload; a payload that differs from the
// recorded CBOR output; and a recorded hash that differs from the framed bytes.
func Replay(f Fixture, expectedCommit string) error {
	// Honesty gate first: an on-demand Oracle must never accept a fixture that
	// does not claim to be a deterministic, reproducible capture.
	if !f.Deterministic {
		return newError(KindNotDeterministic, f.ID, "deterministic must be true")
	}

	if err := verifyRequired(f); err != nil {
		return err
	}

	if f.Upstream.Repository != Repository {
		return newError(KindProvenance, f.ID, "upstream.repository %q != %q", f.Upstream.Repository, Repository)
	}
	if f.Hash.Algorithm != "sha256" {
		return newError(KindUnsupportedHash, f.ID, "%s", f.Hash.Algorithm)
	}

	// Commit consistency: the fixture must be anchored to the expected baseline,
	// and its upstream commit must agree with that anchor.
	if f.BaselineCommit != expectedCommit {
		return newError(KindCommitMismatch, f.ID, "baseline_commit %s != expected %s", f.BaselineCommit, expectedCommit)
	}
	if f.Upstream.Commit != f.BaselineCommit {
		return newError(KindCommitMismatch, f.ID, "upstream.commit %s != baseline_commit %s", f.Upstream.Commit, f.BaselineCommit)
	}

	frame, err := hex.DecodeString(f.RawOutput.FrameHex)
	if err != nil {
		return newError(KindBadHex, f.ID, "frame_hex: %v", err)
	}
	cbor, err := hex.DecodeString(f.RawOutput.CBORHex)
	if err != nil {
		return newError(KindBadHex, f.ID, "cbor_hex: %v", err)
	}
	header, err := hex.DecodeString(f.RawOutput.HeaderHex)
	if err != nil {
		return newError(KindBadHex, f.ID, "header_hex: %v", err)
	}

	// Recorded lengths must match the decoded bytes, and the frame must be long
	// enough to carry the fixed-size header.
	switch {
	case f.RawOutput.HeaderLength != frameHeaderLength:
		return newError(KindLengthMismatch, f.ID, "header_length=%d, want %d", f.RawOutput.HeaderLength, frameHeaderLength)
	case len(frame) != f.RawOutput.FrameLength:
		return newError(KindLengthMismatch, f.ID, "frame_length=%d, decoded %d", f.RawOutput.FrameLength, len(frame))
	case len(cbor) != f.RawOutput.CBORLength:
		return newError(KindLengthMismatch, f.ID, "cbor_length=%d, decoded %d", f.RawOutput.CBORLength, len(cbor))
	case len(frame) < frameHeaderLength:
		return newError(KindLengthMismatch, f.ID, "frame %d bytes is shorter than the %d-byte header", len(frame), frameHeaderLength)
	}

	// Genuine cross-implementation check: the payload is everything after the
	// 4-byte prefix, and its length prefix is re-derived through Go's stdlib
	// big-endian codec — a code path fully independent of Pi's hand-rolled
	// frame[0]>>>24 bit shifts.
	payload := frame[frameHeaderLength:]

	// Read direction: parse the frame's own prefix and confirm it equals the
	// payload length.
	claimed := binary.BigEndian.Uint32(frame[:frameHeaderLength])
	if int(claimed) != len(payload) {
		return newError(KindHeaderMismatch, f.ID,
			"frame prefix claims %d payload bytes, frame carries %d", claimed, len(payload))
	}
	// Write direction: re-derive the prefix the writer must have produced and
	// confirm the separately recorded header_hex equals it.
	var want [frameHeaderLength]byte
	binary.BigEndian.PutUint32(want[:], uint32(len(payload)))
	if !bytes.Equal(header, want[:]) {
		return newError(KindHeaderMismatch, f.ID,
			"recorded header %x != re-derived big-endian length %x", header, want[:])
	}

	if !bytes.Equal(payload, cbor) {
		return newError(KindPayloadMismatch, f.ID,
			"frame payload %x != recorded cbor %x", payload, cbor)
	}

	sum := sha256.Sum256(frame)
	if got := hex.EncodeToString(sum[:]); got != f.Hash.FrameSHA256 {
		return newError(KindHashMismatch, f.ID, "sha256 %s != recorded %s", got, f.Hash.FrameSHA256)
	}
	return nil
}

// verifyRequired rejects a fixture missing any field replay depends on.
func verifyRequired(f Fixture) error {
	for _, field := range []struct{ name, value string }{
		{"schema_version", f.SchemaVersion},
		{"id", f.ID},
		{"catalog_id", f.CatalogID},
		{"baseline_id", f.BaselineID},
		{"baseline_commit", f.BaselineCommit},
		{"upstream.module", f.Upstream.Module},
		{"upstream.repository", f.Upstream.Repository},
		{"upstream.commit", f.Upstream.Commit},
		{"upstream.reference", f.Upstream.Reference},
		{"raw_output.frame_hex", f.RawOutput.FrameHex},
		{"raw_output.cbor_hex", f.RawOutput.CBORHex},
		{"raw_output.header_hex", f.RawOutput.HeaderHex},
		{"hash.algorithm", f.Hash.Algorithm},
		{"hash.frame_sha256", f.Hash.FrameSHA256},
		{"exec_method", f.ExecMethod},
	} {
		if field.value == "" {
			return newError(KindMissingField, f.ID, "%s is empty", field.name)
		}
	}
	return nil
}

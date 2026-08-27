package oracle_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/nankedr/pig/internal/baseline"
	"github.com/nankedr/pig/internal/oracle"
)

const (
	baselineID     = "pi-936aff0-catalog-v0.84.1-v1"
	baselineCommit = "936aff00918de1187f085f123c2812d8f2d67745"
)

// repoRoot locates the repository root relative to this test file, mirroring the
// runtime.Caller approach used by internal/inventory, internal/surface and
// internal/catalog.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

// baseFixture returns a fully valid, internally consistent Pi Oracle fixture: the
// known-good ClientHello vector captured by parity/oracle/run.mjs against the
// locked baseline. Rejection tests mutate a fresh copy so cases stay independent.
func baseFixture() oracle.Fixture {
	return oracle.Fixture{
		SchemaVersion:  oracle.SchemaVersion,
		ID:             "protocol/frame",
		CatalogID:      "contract:protocol/frame",
		BaselineID:     baselineID,
		BaselineCommit: baselineCommit,
		Deterministic:  true,
		Upstream: oracle.Upstream{
			Module:     "protocol",
			Repository: oracle.Repository,
			Commit:     baselineCommit,
			Reference:  "packages/protocol/src/framing.ts#encodeFrame",
		},
		Input: oracle.Input{
			Description: "Canonical ClientHello: the mandatory first client protocol message.",
			Message:     json.RawMessage(`{"type":"hello","version":1}`),
			Encoding:    "encodeFrame(encodeCbor(message, { maxByteLength }))",
		},
		RawOutput: oracle.RawOutput{
			Encoding:     "hex",
			CBORHex:      "a264747970656568656c6c6f6776657273696f6e01",
			CBORLength:   21,
			FrameHex:     "00000015a264747970656568656c6c6f6776657273696f6e01",
			FrameLength:  25,
			HeaderHex:    "00000015",
			HeaderLength: 4,
		},
		Hash: oracle.Hash{
			Algorithm:   "sha256",
			FrameSHA256: "645eb27af58427c5e92d657609e81e6d2b0652c233b6b37d14403ac7c3f59d5f",
		},
		Env: oracle.Env{
			NodeVersion: "v24.14.0",
			Platform:    "darwin",
			Arch:        "arm64",
		},
		ExecMethod: "node-native-typescript-import",
	}
}

func TestReplayAcceptsKnownGoodVector(t *testing.T) {
	if err := oracle.Replay(baseFixture(), baselineID, baselineCommit); err != nil {
		t.Fatalf("Replay(valid fixture) = %v", err)
	}
}

func TestReplayRejections(t *testing.T) {
	for _, tt := range []struct {
		name     string
		mutate   func(f *oracle.Fixture)
		sentinel error
		kind     oracle.Kind
	}{
		{
			name:     "not deterministic is refused",
			mutate:   func(f *oracle.Fixture) { f.Deterministic = false },
			sentinel: oracle.ErrNotDeterministic,
			kind:     oracle.KindNotDeterministic,
		},
		{
			name:     "missing schema version",
			mutate:   func(f *oracle.Fixture) { f.SchemaVersion = "" },
			sentinel: oracle.ErrMissingField,
			kind:     oracle.KindMissingField,
		},
		{
			name:     "missing upstream reference",
			mutate:   func(f *oracle.Fixture) { f.Upstream.Reference = "" },
			sentinel: oracle.ErrMissingField,
			kind:     oracle.KindMissingField,
		},
		{
			name:     "missing frame hash",
			mutate:   func(f *oracle.Fixture) { f.Hash.FrameSHA256 = "" },
			sentinel: oracle.ErrMissingField,
			kind:     oracle.KindMissingField,
		},
		{
			name:     "unexpected repository",
			mutate:   func(f *oracle.Fixture) { f.Upstream.Repository = "https://example.com/evil" },
			sentinel: oracle.ErrProvenance,
			kind:     oracle.KindProvenance,
		},
		{
			name:     "unsupported hash algorithm",
			mutate:   func(f *oracle.Fixture) { f.Hash.Algorithm = "md5" },
			sentinel: oracle.ErrUnsupportedHash,
			kind:     oracle.KindUnsupportedHash,
		},
		{
			name:     "baseline id mismatch",
			mutate:   func(f *oracle.Fixture) { f.BaselineID = "pi-other-v1" },
			sentinel: oracle.ErrProvenance,
			kind:     oracle.KindProvenance,
		},
		{
			name:     "baseline commit mismatch",
			mutate:   func(f *oracle.Fixture) { f.BaselineCommit = "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef" },
			sentinel: oracle.ErrCommitMismatch,
			kind:     oracle.KindCommitMismatch,
		},
		{
			name: "upstream commit disagrees with baseline",
			mutate: func(f *oracle.Fixture) {
				f.Upstream.Commit = "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef"
			},
			sentinel: oracle.ErrCommitMismatch,
			kind:     oracle.KindCommitMismatch,
		},
		{
			name:     "malformed frame hex",
			mutate:   func(f *oracle.Fixture) { f.RawOutput.FrameHex = "zzzz" },
			sentinel: oracle.ErrBadHex,
			kind:     oracle.KindBadHex,
		},
		{
			name:     "frame length disagrees with bytes",
			mutate:   func(f *oracle.Fixture) { f.RawOutput.FrameLength = 99 },
			sentinel: oracle.ErrLengthMismatch,
			kind:     oracle.KindLengthMismatch,
		},
		{
			name:     "cbor length disagrees with bytes",
			mutate:   func(f *oracle.Fixture) { f.RawOutput.CBORLength = 99 },
			sentinel: oracle.ErrLengthMismatch,
			kind:     oracle.KindLengthMismatch,
		},
		{
			name:     "header length not four",
			mutate:   func(f *oracle.Fixture) { f.RawOutput.HeaderLength = 3 },
			sentinel: oracle.ErrLengthMismatch,
			kind:     oracle.KindLengthMismatch,
		},
		{
			name: "recorded header disagrees with re-derived big-endian length",
			// header claims payload length 22 while the payload is 21 bytes; Go's
			// independent binary.BigEndian derivation catches the corruption.
			mutate:   func(f *oracle.Fixture) { f.RawOutput.HeaderHex = "00000016" },
			sentinel: oracle.ErrHeaderMismatch,
			kind:     oracle.KindHeaderMismatch,
		},
		{
			name: "frame header disagrees with payload length",
			// A frame whose 4-byte prefix says 22 but carries a 21-byte payload:
			// the cross-implementation re-derivation must reject it.
			mutate: func(f *oracle.Fixture) {
				f.RawOutput.FrameHex = "00000016a264747970656568656c6c6f6776657273696f6e01"
			},
			sentinel: oracle.ErrHeaderMismatch,
			kind:     oracle.KindHeaderMismatch,
		},
		{
			name: "frame payload disagrees with recorded cbor",
			// Same length, different final byte (version 0 not 1): the reconstructed
			// payload no longer matches the recorded CBOR.
			mutate: func(f *oracle.Fixture) {
				f.RawOutput.CBORHex = "a264747970656568656c6c6f6776657273696f6e00"
			},
			sentinel: oracle.ErrPayloadMismatch,
			kind:     oracle.KindPayloadMismatch,
		},
		{
			name: "hash disagrees with frame bytes",
			mutate: func(f *oracle.Fixture) {
				f.Hash.FrameSHA256 = "00000000000000000000000000000000000000000000000000000000000000ff"
			},
			sentinel: oracle.ErrHashMismatch,
			kind:     oracle.KindHashMismatch,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := baseFixture()
			tt.mutate(&f)
			err := oracle.Replay(f, baselineID, baselineCommit)
			if err == nil {
				t.Fatalf("Replay(%s) = nil, want error", tt.name)
			}
			if !errors.Is(err, tt.sentinel) {
				t.Fatalf("errors.Is(%v, %v) = false", err, tt.sentinel)
			}
			var oerr *oracle.Error
			if !errors.As(err, &oerr) {
				t.Fatalf("errors.As(%v, *oracle.Error) = false", err)
			}
			if oerr.Kind != tt.kind {
				t.Fatalf("Error.Kind = %q, want %q", oerr.Kind, tt.kind)
			}
		})
	}
}

// TestLoadFixtureMalformed verifies a corrupt fixture file is reported as a
// typed malformed-fixture error rather than surfacing a raw json error.
func TestLoadFixtureMalformed(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "protocol-frame.json")
	if err := os.WriteFile(tmp, []byte("{not valid json"), 0o644); err != nil {
		t.Fatalf("write temp: %v", err)
	}
	_, err := oracle.LoadFixture(tmp)
	if err == nil {
		t.Fatal("LoadFixture(malformed) = nil, want error")
	}
	if !errors.Is(err, oracle.ErrMalformedFixture) {
		t.Fatalf("errors.Is(%v, ErrMalformedFixture) = false", err)
	}
}

// TestLoadFixtureUnknownField rejects a fixture carrying an unexpected field, so
// the on-disk shape stays exactly what the harness writes.
func TestLoadFixtureUnknownField(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "protocol-frame.json")
	if err := os.WriteFile(tmp, []byte(`{"schema_version":"1.0.0","surprise":true}`), 0o644); err != nil {
		t.Fatalf("write temp: %v", err)
	}
	if _, err := oracle.LoadFixture(tmp); err == nil {
		t.Fatal("LoadFixture(unknown field) = nil, want error")
	}
}

// TestReplayRealFixture is the offline consistency check that runs in normal
// `go test`: the committed fixture must load and replay against the locked
// baseline commit, independently re-deriving the frame header in pure Go with no
// Node and no Pi checkout. This is the genuine cross-implementation parity check
// backing the contract:protocol/frame verified evidence.
func TestReplayRealFixture(t *testing.T) {
	root := repoRoot(t)
	path := filepath.Join(root, "parity", "oracle", "fixtures", "protocol-frame.json")
	fixture, err := oracle.LoadFixture(path)
	if err != nil {
		t.Fatalf("LoadFixture(real) = %v", err)
	}
	if fixture.CatalogID != "contract:protocol/frame" {
		t.Fatalf("fixture catalog_id = %q, want contract:protocol/frame", fixture.CatalogID)
	}
	if !fixture.Deterministic {
		t.Fatal("committed fixture is not marked deterministic")
	}
	lock, _, err := baseline.Load(filepath.Join(root, "parity", "baseline"))
	if err != nil {
		t.Fatalf("baseline.Load: %v", err)
	}
	if err := oracle.Replay(fixture, lock.BaselineID, lock.Upstream.Commit); err != nil {
		t.Fatalf("Replay(real fixture) = %v", err)
	}
}

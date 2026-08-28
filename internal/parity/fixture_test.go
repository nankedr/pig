package parity_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/nankedr/pig/internal/parity"
)

func TestHashCaseCoversNormalizationAndCanonicalizesJSONInput(t *testing.T) {
	base := parity.Case{
		SchemaVersion: parity.CaseSchemaVersion,
		ID:            "cli/pig/auth-help",
		CatalogID:     "contract:cli/pig/auth-help",
		Surface:       parity.SurfaceCLI,
		Input:         json.RawMessage(`{"arguments":["auth","--help"],"env":{}}`),
		Observe:       []parity.Channel{parity.ChannelStdout, parity.ChannelStderr, parity.ChannelExitStatus},
		Normalizations: []parity.Normalization{{
			Target:       "/stdout",
			Kind:         parity.NormalizationBrandToken,
			Oracle:       "pi",
			Pig:          "pig",
			ExactMatches: 3,
		}},
	}
	hash, err := parity.HashCase(base)
	if err != nil {
		t.Fatal(err)
	}
	reformatted := base
	reformatted.Input = json.RawMessage(`{ "env": {}, "arguments": ["auth", "--help"] }`)
	reformattedHash, err := parity.HashCase(reformatted)
	if err != nil {
		t.Fatal(err)
	}
	if hash != reformattedHash {
		t.Fatalf("equivalent JSON hashes differ: %q != %q", hash, reformattedHash)
	}
	wider := base
	wider.Normalizations = append([]parity.Normalization(nil), base.Normalizations...)
	wider.Normalizations[0].ExactMatches = 4
	widerHash, err := parity.HashCase(wider)
	if err != nil {
		t.Fatal(err)
	}
	if hash == widerHash {
		t.Fatalf("normalization change kept hash %q", hash)
	}
}

func TestHashObservationPreservesTextBytesAndCanonicalizesJSONValues(t *testing.T) {
	leftOutcome := json.RawMessage(`{"a":1,"b":2}`)
	rightOutcome := json.RawMessage(`{ "b": 2, "a": 1 }`)
	left := parity.Observation{Stdout: pointer("same\n"), Outcome: leftOutcome}
	right := parity.Observation{Stdout: pointer("same\n"), Outcome: rightOutcome}
	leftHash, err := parity.HashObservation(left)
	if err != nil {
		t.Fatal(err)
	}
	rightHash, err := parity.HashObservation(right)
	if err != nil {
		t.Fatal(err)
	}
	if leftHash != rightHash {
		t.Fatalf("equivalent observations hash differently: %q != %q", leftHash, rightHash)
	}
	right.Stdout = pointer("different\n")
	changedHash, err := parity.HashObservation(right)
	if err != nil {
		t.Fatal(err)
	}
	if leftHash == changedHash {
		t.Fatalf("different stdout kept hash %q", leftHash)
	}
}

func TestHashObservationDistinguishesAbsentFromNullOutcome(t *testing.T) {
	absentHash, err := parity.HashObservation(parity.Observation{})
	if err != nil {
		t.Fatal(err)
	}
	nullHash, err := parity.HashObservation(parity.Observation{Outcome: json.RawMessage(`null`)})
	if err != nil {
		t.Fatal(err)
	}
	if absentHash == nullHash {
		t.Fatalf("absent and null outcomes share hash %q", absentHash)
	}
}

func TestHashObservationPreservesNestedAbsentAndEmptyBytes(t *testing.T) {
	empty := []byte{}
	absentWire := []parity.WireObservation{{Direction: "outbound", Encoding: "raw"}}
	emptyWire := []parity.WireObservation{{Direction: "outbound", Encoding: "raw", Data: &empty}}
	absentFiles := []parity.FileState{{Path: "empty", Exists: true}}
	emptyFiles := []parity.FileState{{Path: "empty", Exists: true, Mode: pointer(uint32(0)), Contents: &empty}}

	absentHash, err := parity.HashObservation(parity.Observation{Wire: &absentWire, Files: &absentFiles})
	if err != nil {
		t.Fatal(err)
	}
	emptyHash, err := parity.HashObservation(parity.Observation{Wire: &emptyWire, Files: &emptyFiles})
	if err != nil {
		t.Fatal(err)
	}
	if absentHash == emptyHash {
		t.Fatalf("nested absent and empty states share hash %q", absentHash)
	}
}

func TestHashObservationPreservesObservedEmptyCollections(t *testing.T) {
	emptyEvents := []json.RawMessage{}
	emptyWire := []parity.WireObservation{}
	emptySessions := []parity.SessionState{}
	emptyEffects := []parity.SideEffect{}
	observed := parity.Observation{
		Events: &emptyEvents, Wire: &emptyWire, Sessions: &emptySessions, SideEffects: &emptyEffects,
	}
	nilEvents := []json.RawMessage(nil)
	nilWire := []parity.WireObservation(nil)
	nilSessions := []parity.SessionState(nil)
	nilEffects := []parity.SideEffect(nil)
	collapsed := parity.Observation{
		Events: &nilEvents, Wire: &nilWire, Sessions: &nilSessions, SideEffects: &nilEffects,
	}

	observedHash, err := parity.HashObservation(observed)
	if err != nil {
		t.Fatal(err)
	}
	collapsedHash, err := parity.HashObservation(collapsed)
	if err != nil {
		t.Fatal(err)
	}
	if observedHash == collapsedHash {
		t.Fatalf("observed empty collections collapsed to null with hash %q", observedHash)
	}
}

func TestFixtureDriverReplaysOnlyItsDeclaredCase(t *testing.T) {
	baseline := parity.Baseline{
		ID:         "pi-936aff0-v1",
		Commit:     "936aff00918de1187f085f123c2812d8f2d67745",
		Repository: "https://github.com/badlogic/pi-mono",
	}
	caseDeclaration := parity.Case{
		SchemaVersion: parity.CaseSchemaVersion,
		ID:            "cli/pig/auth-help",
		CatalogID:     "contract:cli/pig/auth-help",
		Surface:       parity.SurfaceCLI,
		Input:         json.RawMessage(`{"arguments":["auth","--help"]}`),
		Observe:       []parity.Channel{parity.ChannelStdout, parity.ChannelStderr, parity.ChannelExitStatus},
		Normalizations: []parity.Normalization{{
			Target:       "/stdout",
			Kind:         parity.NormalizationBrandToken,
			Oracle:       "pi",
			Pig:          "pig",
			ExactMatches: 3,
		}},
	}
	observation := parity.Observation{
		Stdout:     pointer("pi auth\npi auth\npi auth\n"),
		Stderr:     pointer(""),
		ExitStatus: &parity.ExitStatus{Code: pointer(0)},
		Outcome:    json.RawMessage(`{ "b": 2, "a": 1 }`),
	}
	inputHash, err := parity.HashCase(caseDeclaration)
	if err != nil {
		t.Fatal(err)
	}
	observationHash, err := parity.HashObservation(observation)
	if err != nil {
		t.Fatal(err)
	}
	fixture := parity.Fixture{
		SchemaVersion:  parity.FixtureSchemaVersion,
		Deterministic:  true,
		BaselineID:     baseline.ID,
		BaselineCommit: baseline.Commit,
		Upstream: parity.FixtureUpstream{
			Repository: baseline.Repository,
			Commit:     baseline.Commit,
			Reference:  "packages/coding-agent/src/cli.ts",
		},
		Case:            caseDeclaration,
		Observation:     observation,
		InputHash:       inputHash,
		ObservationHash: observationHash,
		ExecutionMethod: "node packages/coding-agent/src/cli.ts auth --help",
		Platform:        "darwin-arm64",
		Environment:     map[string]string{"node": "v24.12.0"},
	}
	driver, err := parity.NewFixtureDriver(fixture, baseline)
	if err != nil {
		t.Fatalf("NewFixtureDriver() = %v", err)
	}
	missing := fixture
	missing.Observation.Stdout = nil
	missing.ObservationHash, err = parity.HashObservation(missing.Observation)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parity.NewFixtureDriver(missing, baseline); !errors.Is(err, parity.ErrInvalidFixture) {
		t.Fatalf("NewFixtureDriver(missing channel) = %v, want ErrInvalidFixture", err)
	}
	got, err := driver.Observe(context.Background(), caseDeclaration)
	if err != nil {
		t.Fatalf("Observe() = %v", err)
	}
	if got.Stdout == nil || *got.Stdout != *observation.Stdout {
		t.Fatalf("observation = %+v, want %+v", got, observation)
	}
	if string(got.Outcome) != string(observation.Outcome) {
		t.Fatalf("raw outcome = %s, want %s", got.Outcome, observation.Outcome)
	}
	*fixture.Observation.Stdout = "tampered fixture"
	*got.Stdout = "tampered result"
	again, err := driver.Observe(context.Background(), caseDeclaration)
	if err != nil {
		t.Fatal(err)
	}
	if again.Stdout == nil || *again.Stdout != "pi auth\npi auth\npi auth\n" {
		t.Fatalf("fixture driver retained mutable observation: %+v", again)
	}

	wider := caseDeclaration
	wider.Normalizations = append([]parity.Normalization(nil), caseDeclaration.Normalizations...)
	wider.Normalizations[0].ExactMatches++
	if _, err := driver.Observe(context.Background(), wider); !errors.Is(err, parity.ErrFixtureCaseMismatch) {
		t.Fatalf("Observe(wider case) = %v, want ErrFixtureCaseMismatch", err)
	}
}

func TestFixtureRejectsEmptyProvenanceAndEnvironment(t *testing.T) {
	fixture := parity.Fixture{
		SchemaVersion:   parity.FixtureSchemaVersion,
		Deterministic:   true,
		Upstream:        parity.FixtureUpstream{Reference: "source"},
		Case:            parity.Case{SchemaVersion: parity.CaseSchemaVersion, ID: "case", CatalogID: "catalog", Surface: parity.SurfaceCLI, Input: json.RawMessage(`{}`), Observe: []parity.Channel{parity.ChannelStdout}},
		ExecutionMethod: "command",
		Platform:        "any",
	}
	if err := parity.ValidateFixture(fixture, parity.Baseline{}); !errors.Is(err, parity.ErrInvalidFixture) {
		t.Fatalf("ValidateFixture() = %v, want ErrInvalidFixture", err)
	}
}

func TestLoadFixtureRejectsUnknownFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fixture.json")
	if err := os.WriteFile(path, []byte(`{"schema_version":"1.0.0","surprise":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := parity.LoadFixture(path, parity.Baseline{})
	if !errors.Is(err, parity.ErrInvalidFixture) {
		t.Fatalf("LoadFixture() error = %v, want ErrInvalidFixture", err)
	}
}

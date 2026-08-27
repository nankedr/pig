package parity

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

const FixtureSchemaVersion = "1.0.0"

var (
	ErrInvalidFixture      = errors.New("parity: invalid fixture")
	ErrFixtureProvenance   = errors.New("parity: fixture provenance mismatch")
	ErrFixtureHash         = errors.New("parity: fixture hash mismatch")
	ErrFixtureCaseMismatch = errors.New("parity: fixture does not belong to case")
)

type Baseline struct {
	ID         string
	Commit     string
	Repository string
}

type FixtureUpstream struct {
	Repository string `json:"repository"`
	Commit     string `json:"commit"`
	Reference  string `json:"reference"`
}

type Fixture struct {
	SchemaVersion   string            `json:"schema_version"`
	Deterministic   bool              `json:"deterministic"`
	BaselineID      string            `json:"baseline_id"`
	BaselineCommit  string            `json:"baseline_commit"`
	Upstream        FixtureUpstream   `json:"upstream"`
	Case            Case              `json:"case"`
	Observation     Observation       `json:"observation"`
	InputHash       string            `json:"input_hash"`
	ObservationHash string            `json:"observation_hash"`
	ExecutionMethod string            `json:"execution_method"`
	Platform        string            `json:"platform"`
	Environment     map[string]string `json:"environment,omitempty"`
}

type FixtureDriver struct {
	fixture Fixture
}

func LoadFixture(path string, baseline Baseline) (Fixture, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Fixture{}, err
	}
	var fixture Fixture
	if err := decodeSingleJSON(data, &fixture, disallowUnknownJSON); err != nil {
		return Fixture{}, fmt.Errorf("%w: %v", ErrInvalidFixture, err)
	}
	if err := ValidateFixture(fixture, baseline); err != nil {
		return Fixture{}, err
	}
	return fixture, nil
}

func NewFixtureDriver(fixture Fixture, baseline Baseline) (*FixtureDriver, error) {
	if err := ValidateFixture(fixture, baseline); err != nil {
		return nil, err
	}
	return &FixtureDriver{fixture: cloneFixture(fixture)}, nil
}

func (d *FixtureDriver) Surface() Surface { return d.fixture.Case.Surface }

func (d *FixtureDriver) Observe(ctx context.Context, c Case) (Observation, error) {
	if err := ctx.Err(); err != nil {
		return Observation{}, err
	}
	hash, err := HashCase(c)
	if err != nil {
		return Observation{}, err
	}
	if hash != d.fixture.InputHash {
		return Observation{}, fmt.Errorf("%w: hash %s != %s", ErrFixtureCaseMismatch, hash, d.fixture.InputHash)
	}
	return cloneObservation(d.fixture.Observation), nil
}

func ValidateFixture(fixture Fixture, baseline Baseline) error {
	if baseline.ID == "" || baseline.Commit == "" || baseline.Repository == "" || fixture.SchemaVersion != FixtureSchemaVersion || !fixture.Deterministic || fixture.ExecutionMethod == "" || fixture.Platform == "" || fixture.Upstream.Reference == "" || len(fixture.Environment) == 0 {
		return fmt.Errorf("%w: incomplete fixture metadata", ErrInvalidFixture)
	}
	if fixture.BaselineID != baseline.ID || fixture.BaselineCommit != baseline.Commit || fixture.Upstream.Commit != baseline.Commit || fixture.Upstream.Repository != baseline.Repository {
		return fmt.Errorf("%w: fixture baseline does not match lock", ErrFixtureProvenance)
	}
	if err := validateCase(fixture.Case); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidFixture, err)
	}
	if err := validateObservation(fixture.Case, fixture.Observation); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidFixture, err)
	}
	inputHash, err := HashCase(fixture.Case)
	if err != nil {
		return fmt.Errorf("%w: case: %v", ErrInvalidFixture, err)
	}
	observationHash, err := HashObservation(fixture.Observation)
	if err != nil {
		return fmt.Errorf("%w: observation: %v", ErrInvalidFixture, err)
	}
	if fixture.InputHash != inputHash {
		return fmt.Errorf("%w: input_hash=%s want=%s", ErrFixtureHash, fixture.InputHash, inputHash)
	}
	if fixture.ObservationHash != observationHash {
		return fmt.Errorf("%w: observation_hash=%s want=%s", ErrFixtureHash, fixture.ObservationHash, observationHash)
	}
	return nil
}

func validSurface(surface Surface) bool {
	switch surface {
	case SurfaceCLI, SurfaceGoSDK, SurfacePersistenceProtocol, SurfaceTUIPlatform:
		return true
	default:
		return false
	}
}

func cloneObservation(observation Observation) Observation {
	clone := observation
	clone.Outcome = cloneRawMessage(observation.Outcome)
	clone.Stdout = clonePointer(observation.Stdout)
	clone.Stderr = clonePointer(observation.Stderr)
	if observation.Events != nil {
		events := cloneSlice(*observation.Events)
		for i := range events {
			events[i] = cloneRawMessage(events[i])
		}
		clone.Events = &events
	}
	if observation.Error != nil {
		observedError := *observation.Error
		observedError.Details = cloneRawMessage(observedError.Details)
		clone.Error = &observedError
	}
	if observation.ExitStatus != nil {
		status := *observation.ExitStatus
		status.Code = clonePointer(status.Code)
		clone.ExitStatus = &status
	}
	if observation.Wire != nil {
		wire := cloneSlice(*observation.Wire)
		for i := range wire {
			wire[i].Data = cloneBytesPointer(wire[i].Data)
			wire[i].Value = cloneRawMessage(wire[i].Value)
		}
		clone.Wire = &wire
	}
	if observation.Sessions != nil {
		sessions := cloneSlice(*observation.Sessions)
		for i := range sessions {
			sessions[i].Value = cloneRawMessage(sessions[i].Value)
		}
		clone.Sessions = &sessions
	}
	if observation.Files != nil {
		files := cloneSlice(*observation.Files)
		for i := range files {
			files[i].Mode = clonePointer(files[i].Mode)
			files[i].Contents = cloneBytesPointer(files[i].Contents)
		}
		clone.Files = &files
	}
	if observation.SideEffects != nil {
		effects := cloneSlice(*observation.SideEffects)
		for i := range effects {
			effects[i].Detail = cloneRawMessage(effects[i].Detail)
		}
		clone.SideEffects = &effects
	}
	return clone
}

func cloneFixture(fixture Fixture) Fixture {
	clone := fixture
	clone.Case = cloneCase(fixture.Case)
	clone.Observation = cloneObservation(fixture.Observation)
	if fixture.Environment != nil {
		clone.Environment = make(map[string]string, len(fixture.Environment))
		for key, value := range fixture.Environment {
			clone.Environment[key] = value
		}
	}
	return clone
}

func clonePointer[T any](value *T) *T {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

func cloneSlice[T any](value []T) []T {
	if value == nil {
		return nil
	}
	return append([]T(nil), value...)
}

func cloneRawMessage(value json.RawMessage) json.RawMessage {
	if value == nil {
		return nil
	}
	return append(json.RawMessage(nil), value...)
}

func cloneBytesPointer(value *[]byte) *[]byte {
	if value == nil {
		return nil
	}
	clone := cloneSlice(*value)
	return &clone
}

func HashCase(c Case) (string, error) {
	input, err := canonicalJSON(c.Input)
	if err != nil {
		return "", err
	}
	c.Input = input
	encoded, err := marshalJSON(c)
	if err != nil {
		return "", err
	}
	return sha256Digest(encoded), nil
}

func HashObservation(observation Observation) (string, error) {
	canonical, err := canonicalizeObservation(observation)
	if err != nil {
		return "", err
	}
	encoded, err := marshalJSON(canonical)
	if err != nil {
		return "", err
	}
	return sha256Digest(encoded), nil
}

func sha256Digest(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

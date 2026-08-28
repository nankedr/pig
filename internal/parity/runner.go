package parity

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"reflect"
	"strings"
)

const CaseSchemaVersion = "1.0.0"

var ErrSurfaceMismatch = errors.New("parity: driver surface does not match case")
var ErrMismatch = errors.New("parity: observations differ")
var ErrInvalidCase = errors.New("parity: invalid case")
var ErrIncompleteObservation = errors.New("parity: incomplete observation")

type Surface string

type Channel string

const (
	SurfaceCLI                 Surface = "cli"
	SurfaceGoSDK               Surface = "go-sdk"
	SurfacePersistenceProtocol Surface = "persistence-protocol"
	SurfaceTUIPlatform         Surface = "tui-platform"
)

const (
	ChannelEvents      Channel = "events"
	ChannelOutcome     Channel = "outcome"
	ChannelError       Channel = "error"
	ChannelStdout      Channel = "stdout"
	ChannelStderr      Channel = "stderr"
	ChannelExitStatus  Channel = "exit_status"
	ChannelWire        Channel = "wire"
	ChannelSessions    Channel = "sessions"
	ChannelFiles       Channel = "files"
	ChannelSideEffects Channel = "side_effects"
)

type ExitStatus struct {
	Code   *int   `json:"code,omitempty"`
	Signal string `json:"signal,omitempty"`
}

type ErrorObservation struct {
	Type    string          `json:"type,omitempty"`
	Code    string          `json:"code,omitempty"`
	Message string          `json:"message"`
	Details json.RawMessage `json:"details,omitempty"`
}

type WireObservation struct {
	Direction string          `json:"direction"`
	Encoding  string          `json:"encoding"`
	Data      *[]byte         `json:"data,omitempty"`
	Value     json.RawMessage `json:"value,omitempty"`
}

type SessionState struct {
	ID     string          `json:"id"`
	Format string          `json:"format,omitempty"`
	Value  json.RawMessage `json:"value,omitempty"`
}

type FileState struct {
	Path     string  `json:"path"`
	Exists   bool    `json:"exists"`
	Mode     *uint32 `json:"mode,omitempty"`
	Contents *[]byte `json:"contents,omitempty"`
}

type SideEffect struct {
	Kind   string          `json:"kind"`
	Target string          `json:"target,omitempty"`
	Detail json.RawMessage `json:"detail,omitempty"`
}

type Observation struct {
	Events      *[]json.RawMessage `json:"events,omitempty"`
	Outcome     json.RawMessage    `json:"outcome,omitempty"`
	Error       *ErrorObservation  `json:"error,omitempty"`
	Stdout      *string            `json:"stdout,omitempty"`
	Stderr      *string            `json:"stderr,omitempty"`
	ExitStatus  *ExitStatus        `json:"exit_status,omitempty"`
	Wire        *[]WireObservation `json:"wire,omitempty"`
	Sessions    *[]SessionState    `json:"sessions,omitempty"`
	Files       *[]FileState       `json:"files,omitempty"`
	SideEffects *[]SideEffect      `json:"side_effects,omitempty"`
}

type Driver interface {
	Surface() Surface
	Observe(context.Context, Case) (Observation, error)
}

type DriverFunc struct {
	SurfaceName Surface
	ObserveFunc func(context.Context, Case) (Observation, error)
}

func (d DriverFunc) Surface() Surface { return d.SurfaceName }

func (d DriverFunc) Observe(ctx context.Context, c Case) (Observation, error) {
	return d.ObserveFunc(ctx, c)
}

type Case struct {
	SchemaVersion  string          `json:"schema_version"`
	ID             string          `json:"id"`
	CatalogID      string          `json:"catalog_id"`
	Surface        Surface         `json:"surface"`
	Input          json.RawMessage `json:"input"`
	Observe        []Channel       `json:"observe"`
	Normalizations []Normalization `json:"normalizations,omitempty"`
}

type Result struct {
	CaseID           string
	CatalogID        string
	Oracle           Observation
	Pig              Observation
	NormalizedOracle Observation
	NormalizedPig    Observation
	Normalizations   []NormalizationApplication
	Match            bool
	Differences      []Difference
}

type Difference struct {
	Path Channel `json:"path"`
}

type MismatchError struct {
	CaseID      string
	Differences []Difference
}

func (e *MismatchError) Error() string {
	return fmt.Sprintf("%s: case %s differs at %d path(s)", ErrMismatch, e.CaseID, len(e.Differences))
}

func (e *MismatchError) Unwrap() error { return ErrMismatch }

func RunCase(ctx context.Context, c Case, oracleDriver, pigDriver Driver) (Result, error) {
	if err := validateCase(c); err != nil {
		return Result{}, err
	}
	if oracleDriver == nil || pigDriver == nil {
		return Result{}, fmt.Errorf("%w: drivers are required", ErrInvalidCase)
	}
	if oracleDriver.Surface() != c.Surface {
		return Result{}, fmt.Errorf("%w: oracle is %q, case is %q", ErrSurfaceMismatch, oracleDriver.Surface(), c.Surface)
	}
	if pigDriver.Surface() != c.Surface {
		return Result{}, fmt.Errorf("%w: pig is %q, case is %q", ErrSurfaceMismatch, pigDriver.Surface(), c.Surface)
	}
	oracle, err := oracleDriver.Observe(ctx, cloneCase(c))
	if err != nil {
		return Result{}, err
	}
	if err := validateObservation(c, oracle); err != nil {
		return Result{}, fmt.Errorf("oracle: %w", err)
	}
	pig, err := pigDriver.Observe(ctx, cloneCase(c))
	if err != nil {
		return Result{}, err
	}
	if err := validateObservation(c, pig); err != nil {
		return Result{}, fmt.Errorf("pig: %w", err)
	}
	normalizedOracle, normalizedPig, applications, err := applyNormalizations(c.Normalizations, oracle, pig)
	if err != nil {
		return Result{}, err
	}
	comparableOracle, err := canonicalizeObservation(normalizedOracle)
	if err != nil {
		return Result{}, fmt.Errorf("canonicalize oracle observation: %w", err)
	}
	comparablePig, err := canonicalizeObservation(normalizedPig)
	if err != nil {
		return Result{}, fmt.Errorf("canonicalize pig observation: %w", err)
	}
	differences := compareObservations(comparableOracle, comparablePig)
	result := Result{
		CaseID:           c.ID,
		CatalogID:        c.CatalogID,
		Oracle:           oracle,
		Pig:              pig,
		NormalizedOracle: normalizedOracle,
		NormalizedPig:    normalizedPig,
		Normalizations:   applications,
		Match:            len(differences) == 0,
		Differences:      differences,
	}
	if !result.Match {
		return result, &MismatchError{CaseID: c.ID, Differences: differences}
	}
	return result, nil
}

func validateCase(c Case) error {
	if c.SchemaVersion != CaseSchemaVersion || strings.TrimSpace(c.ID) == "" || strings.TrimSpace(c.CatalogID) == "" || !validSurface(c.Surface) || len(c.Input) == 0 || len(c.Observe) == 0 {
		return fmt.Errorf("%w: incomplete declaration", ErrInvalidCase)
	}
	if _, err := canonicalJSON(c.Input); err != nil {
		return fmt.Errorf("%w: input: %v", ErrInvalidCase, err)
	}
	seen := make(map[Channel]bool, len(c.Observe))
	for _, channel := range c.Observe {
		if !validChannel(channel) || seen[channel] {
			return fmt.Errorf("%w: invalid observation channel %q", ErrInvalidCase, channel)
		}
		seen[channel] = true
	}
	return validateNormalizations(c.Normalizations)
}

func validChannel(channel Channel) bool {
	switch channel {
	case ChannelEvents, ChannelOutcome, ChannelError, ChannelStdout, ChannelStderr, ChannelExitStatus, ChannelWire, ChannelSessions, ChannelFiles, ChannelSideEffects:
		return true
	default:
		return false
	}
}

func validateObservation(c Case, observation Observation) error {
	observed := map[Channel]bool{
		ChannelEvents: observation.Events != nil, ChannelOutcome: len(observation.Outcome) != 0,
		ChannelError: observation.Error != nil, ChannelStdout: observation.Stdout != nil,
		ChannelStderr: observation.Stderr != nil, ChannelExitStatus: observation.ExitStatus != nil,
		ChannelWire: observation.Wire != nil, ChannelSessions: observation.Sessions != nil,
		ChannelFiles: observation.Files != nil, ChannelSideEffects: observation.SideEffects != nil,
	}
	for _, channel := range c.Observe {
		if !observed[channel] {
			return fmt.Errorf("%w: %s", ErrIncompleteObservation, channel)
		}
	}
	return nil
}

func cloneCase(c Case) Case {
	c.Input = append(json.RawMessage(nil), c.Input...)
	c.Observe = append([]Channel(nil), c.Observe...)
	c.Normalizations = append([]Normalization(nil), c.Normalizations...)
	return c
}

func canonicalizeObservation(observation Observation) (Observation, error) {
	result := observation
	if observation.Events != nil {
		events := make([]json.RawMessage, len(*observation.Events))
		for i, event := range *observation.Events {
			canonical, err := canonicalJSON(event)
			if err != nil {
				return Observation{}, fmt.Errorf("events[%d]: %w", i, err)
			}
			events[i] = canonical
		}
		result.Events = &events
	}
	if len(observation.Outcome) != 0 {
		canonical, err := canonicalJSON(observation.Outcome)
		if err != nil {
			return Observation{}, fmt.Errorf("outcome: %w", err)
		}
		result.Outcome = canonical
	}
	if observation.Error != nil {
		observedError := *observation.Error
		canonical, err := canonicalJSON(observedError.Details)
		if err != nil {
			return Observation{}, fmt.Errorf("error.details: %w", err)
		}
		observedError.Details = canonical
		result.Error = &observedError
	}
	if observation.Wire != nil {
		wire := cloneSlice(*observation.Wire)
		for i := range wire {
			canonical, err := canonicalJSON(wire[i].Value)
			if err != nil {
				return Observation{}, fmt.Errorf("wire[%d].value: %w", i, err)
			}
			wire[i].Value = canonical
		}
		result.Wire = &wire
	}
	if observation.Sessions != nil {
		sessions := cloneSlice(*observation.Sessions)
		for i := range sessions {
			canonical, err := canonicalJSON(sessions[i].Value)
			if err != nil {
				return Observation{}, fmt.Errorf("sessions[%d].value: %w", i, err)
			}
			sessions[i].Value = canonical
		}
		result.Sessions = &sessions
	}
	if observation.SideEffects != nil {
		effects := cloneSlice(*observation.SideEffects)
		for i := range effects {
			canonical, err := canonicalJSON(effects[i].Detail)
			if err != nil {
				return Observation{}, fmt.Errorf("side_effects[%d].detail: %w", i, err)
			}
			effects[i].Detail = canonical
		}
		result.SideEffects = &effects
	}
	return result, nil
}

func canonicalJSON(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var value any
	if err := decodeSingleJSON(raw, &value, useJSONNumber); err != nil {
		return nil, err
	}
	value, err := canonicalizeJSONValue(value)
	if err != nil {
		return nil, err
	}
	return marshalJSON(value)
}

func decodeSingleJSON(data []byte, value any, configure func(*json.Decoder)) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if configure != nil {
		configure(decoder)
	}
	if err := decoder.Decode(value); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}

func useJSONNumber(decoder *json.Decoder) { decoder.UseNumber() }

func disallowUnknownJSON(decoder *json.Decoder) { decoder.DisallowUnknownFields() }

func marshalJSON(value any) ([]byte, error) {
	var output bytes.Buffer
	encoder := json.NewEncoder(&output)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(output.Bytes(), []byte{'\n'}), nil
}

func canonicalizeJSONValue(value any) (any, error) {
	switch value := value.(type) {
	case json.Number:
		number, err := canonicalJSONNumber(value.String())
		if err != nil {
			return nil, err
		}
		return json.RawMessage(number), nil
	case []any:
		for i := range value {
			canonical, err := canonicalizeJSONValue(value[i])
			if err != nil {
				return nil, err
			}
			value[i] = canonical
		}
	case map[string]any:
		for key, item := range value {
			canonical, err := canonicalizeJSONValue(item)
			if err != nil {
				return nil, err
			}
			value[key] = canonical
		}
	}
	return value, nil
}

func canonicalJSONNumber(value string) (string, error) {
	sign := ""
	if strings.HasPrefix(value, "-") {
		sign, value = "-", value[1:]
	}
	mantissa, exponentText := value, "0"
	if index := strings.IndexAny(value, "eE"); index >= 0 {
		mantissa, exponentText = value[:index], value[index+1:]
	}
	exponent, ok := new(big.Int).SetString(exponentText, 10)
	if !ok {
		return "", fmt.Errorf("invalid JSON number %q", value)
	}
	integer, fraction := mantissa, ""
	if index := strings.IndexByte(mantissa, '.'); index >= 0 {
		integer, fraction = mantissa[:index], mantissa[index+1:]
	}
	digits := strings.TrimLeft(integer+fraction, "0")
	if digits == "" {
		return "0", nil
	}
	exponent.Sub(exponent, big.NewInt(int64(len(fraction))))
	trimmed := strings.TrimRight(digits, "0")
	exponent.Add(exponent, big.NewInt(int64(len(digits)-len(trimmed))))
	if exponent.Sign() == 0 {
		return sign + trimmed, nil
	}
	return sign + trimmed + "e" + exponent.String(), nil
}

func compareObservations(oracle, pig Observation) []Difference {
	fields := []struct {
		path  Channel
		left  any
		right any
	}{
		{ChannelEvents, oracle.Events, pig.Events},
		{ChannelOutcome, oracle.Outcome, pig.Outcome},
		{ChannelError, oracle.Error, pig.Error},
		{ChannelStdout, oracle.Stdout, pig.Stdout},
		{ChannelStderr, oracle.Stderr, pig.Stderr},
		{ChannelExitStatus, oracle.ExitStatus, pig.ExitStatus},
		{ChannelWire, oracle.Wire, pig.Wire},
		{ChannelSessions, oracle.Sessions, pig.Sessions},
		{ChannelFiles, oracle.Files, pig.Files},
		{ChannelSideEffects, oracle.SideEffects, pig.SideEffects},
	}
	var differences []Difference
	for _, field := range fields {
		if !reflect.DeepEqual(field.left, field.right) {
			differences = append(differences, Difference{Path: field.path})
		}
	}
	return differences
}

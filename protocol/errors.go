package protocol

import "github.com/nankedr/pig/internal/capability"

var ErrNotImplemented = capability.ErrNotImplemented

type NotImplementedError = capability.NotImplementedError

func newNotImplemented(operation string) *NotImplementedError {
	return &NotImplementedError{Module: "protocol", Operation: operation}
}

// CBORError classifies errors from the future CBOR implementation.
type CBORError struct {
	Message string
	Cause   error
}

func (e *CBORError) Error() string { return e.Message }
func (e *CBORError) Unwrap() error { return e.Cause }

// FrameError classifies errors from the future framing implementation.
type FrameError struct {
	Message string
	Cause   error
}

func (e *FrameError) Error() string { return e.Message }
func (e *FrameError) Unwrap() error { return e.Cause }

// ProtocolValidationError classifies protocol schema validation failures.
type ProtocolValidationError struct {
	Message string
	Cause   error
}

func (e *ProtocolValidationError) Error() string { return e.Message }
func (e *ProtocolValidationError) Unwrap() error { return e.Cause }

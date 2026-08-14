package capability

import "errors"

var ErrNotImplemented = errors.New("not implemented")

type NotImplementedError struct {
	Module    string
	Operation string
}

func (e *NotImplementedError) Error() string {
	return e.Module + "." + e.Operation + ": " + ErrNotImplemented.Error()
}

func (e *NotImplementedError) Unwrap() error {
	return ErrNotImplemented
}

func NewNotImplementedError(module, operation string) *NotImplementedError {
	return &NotImplementedError{Module: module, Operation: operation}
}

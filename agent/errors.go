package agent

import "github.com/nankedr/pig/internal/capability"

var ErrNotImplemented = capability.ErrNotImplemented

type NotImplementedError = capability.NotImplementedError

func newNotImplemented(operation string) *NotImplementedError {
	return &NotImplementedError{Module: "agent", Operation: operation}
}

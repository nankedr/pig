package codingagent

import "github.com/nankedr/pig/internal/capability"

var ErrNotImplemented = capability.ErrNotImplemented

type NotImplementedError = capability.NotImplementedError

func notImplemented(operation string) *NotImplementedError {
	return capability.NewNotImplementedError("codingagent", operation)
}

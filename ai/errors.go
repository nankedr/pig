package ai

import (
	"context"
	"errors"

	"github.com/nankedr/pig/internal/capability"
)

var ErrNotImplemented = capability.ErrNotImplemented

type NotImplementedError = capability.NotImplementedError

func newNotImplemented(operation string) *NotImplementedError {
	return &NotImplementedError{Module: "ai", Operation: operation}
}

// terminalErrorMessage returns the deliberately small, redacted projection
// that may cross the AssistantMessage/session boundary. ModelsError keeps an
// unexported constructor-controlled display message; its exported Message and
// Cause may contain extension/provider data and are never copied here.
func terminalErrorMessage(err error) string {
	if errors.Is(err, context.Canceled) {
		return "stream: request canceled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "stream: request deadline exceeded"
	}
	var modelsErr *ModelsError
	if errors.As(err, &modelsErr) {
		return modelsErr.safeErrorMessage()
	}
	return "provider: request failed"
}

package capability_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/client"
	"github.com/nankedr/pig/codingagent"
	"github.com/nankedr/pig/internal/capability"
	"github.com/nankedr/pig/protocol"
	"github.com/nankedr/pig/telemetry"
	"github.com/nankedr/pig/tui"
)

func TestNotImplementedError(t *testing.T) {
	err := capability.NewNotImplementedError("ai", "stream")
	if err == nil {
		t.Fatal("stub returned nil error")
	}
	if !errors.Is(err, capability.ErrNotImplemented) {
		t.Fatalf("errors.Is(%v, ErrNotImplemented) = false", err)
	}

	var target *capability.NotImplementedError
	if !errors.As(err, &target) {
		t.Fatalf("errors.As(%v, *NotImplementedError) = false", err)
	}
	if target.Module != "ai" || target.Operation != "stream" {
		t.Fatalf("NotImplementedError = %#v", target)
	}
	if text := err.Error(); !strings.Contains(text, target.Module) || !strings.Contains(text, target.Operation) {
		t.Fatalf("error text %q does not identify the capability", text)
	}
}

func TestPackagesShareNotImplementedContract(t *testing.T) {
	err := &ai.NotImplementedError{Module: "ai", Operation: "stream"}
	targets := []error{
		ai.ErrNotImplemented,
		agent.ErrNotImplemented,
		codingagent.ErrNotImplemented,
		telemetry.ErrNotImplemented,
		tui.ErrNotImplemented,
		protocol.ErrNotImplemented,
		client.ErrNotImplemented,
	}
	for _, target := range targets {
		if !errors.Is(err, target) {
			t.Errorf("errors.Is(%v, %v) = false", err, target)
		}
	}
}

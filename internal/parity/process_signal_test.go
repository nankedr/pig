//go:build !windows

package parity_test

import (
	"context"
	"encoding/json"
	"os"
	"syscall"
	"testing"

	"github.com/nankedr/pig/internal/parity"
)

func TestCommandDriverCapturesSignalExit(t *testing.T) {
	if os.Getenv("PIG_PARITY_SIGNAL_HELPER") == "1" {
		_ = syscall.Kill(os.Getpid(), syscall.SIGTERM)
		select {}
	}
	input, err := json.Marshal(parity.CLIInput{Arguments: []string{"-test.run=^TestCommandDriverCapturesSignalExit$"}})
	if err != nil {
		t.Fatal(err)
	}
	driver := parity.CommandDriver{
		Path: os.Args[0],
		Env:  append(os.Environ(), "PIG_PARITY_SIGNAL_HELPER=1"),
	}
	observation, err := driver.Observe(context.Background(), parity.Case{Input: input})
	if err != nil {
		t.Fatalf("Observe() = %v", err)
	}
	if observation.ExitStatus == nil || observation.ExitStatus.Code != nil || observation.ExitStatus.Signal != "SIGTERM" {
		t.Fatalf("exit status = %+v, want signal SIGTERM", observation.ExitStatus)
	}
}

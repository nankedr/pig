package tui_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nankedr/pig/tui"
)

type countingTerminalInput struct {
	reads int
}

func (r *countingTerminalInput) Read([]byte) (int, error) {
	r.reads++
	return 0, errors.New("unexpected terminal input read")
}

type countingTerminalOutput struct {
	writes int
}

func (w *countingTerminalOutput) Write(p []byte) (int, error) {
	w.writes++
	return len(p), nil
}

func TestProcessTerminalCapabilityStubHasNoTerminalSideEffects(t *testing.T) {
	input := &countingTerminalInput{}
	output := &countingTerminalOutput{}
	terminal := tui.NewProcessTerminal(input, output)
	inputCalls := 0
	resizeCalls := 0

	operations := []struct {
		name string
		call func() error
	}{
		{name: "drainInput", call: func() error {
			return terminal.DrainInput(context.Background(), time.Second, 50*time.Millisecond)
		}},
		{name: "setTitle", call: func() error { return terminal.SetTitle("pig") }},
		{name: "setProgress", call: func() error { return terminal.SetProgress(true) }},
	}

	for _, operation := range operations {
		t.Run(operation.name, func(t *testing.T) {
			err := operation.call()
			if !errors.Is(err, tui.ErrNotImplemented) {
				t.Fatalf("errors.Is(%v, ErrNotImplemented) = false", err)
			}
			var capabilityErr *tui.NotImplementedError
			if !errors.As(err, &capabilityErr) {
				t.Fatalf("errors.As(%v, *NotImplementedError) = false", err)
			}
			if capabilityErr.Module != "tui" || capabilityErr.Operation != "ProcessTerminal."+operation.name {
				t.Fatalf("NotImplementedError = %+v, want module tui and operation ProcessTerminal.%s", capabilityErr, operation.name)
			}
		})
	}

	if input.reads != 0 || output.writes != 0 || inputCalls != 0 || resizeCalls != 0 {
		t.Fatalf("stub side effects: input reads=%d output writes=%d input callbacks=%d resize callbacks=%d", input.reads, output.writes, inputCalls, resizeCalls)
	}
}

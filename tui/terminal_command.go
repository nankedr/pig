package tui

import (
	"context"
	"errors"
	"os/exec"
)

// RunCommand inherits this terminal's streams; the caller must stop terminal input first.
func (t *ProcessTerminal) RunCommand(ctx context.Context, name string, args ...string) (err error) {
	if ctx == nil {
		return errors.New("terminal command requires a context")
	}
	t.mu.Lock()
	active := t.started
	t.mu.Unlock()
	if active {
		return errors.New("terminal input must be stopped before running a command")
	}
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = t.input, t.output, t.output
	restore, err := configureTerminalCommand(cmd, t.input)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, restore()) }()
	return cmd.Run()
}

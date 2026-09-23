//go:build !darwin && !linux

package tui

import (
	"io"
	"os/exec"
)

func configureTerminalCommand(*exec.Cmd, io.Reader) (func() error, error) {
	return nil, newNotImplemented("ProcessTerminal.runCommand.platform")
}

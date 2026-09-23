//go:build darwin || linux

package tui

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

func configureTerminalCommand(cmd *exec.Cmd, input io.Reader) (func() error, error) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	restore := func() error { return nil }
	if file, ok := input.(*os.File); ok {
		fd := int(file.Fd())
		state, err := term.GetState(fd)
		if err != nil {
			return nil, err
		}
		restore = func() error { return term.Restore(fd, state) }
		if group, err := unix.IoctlGetInt(fd, unix.TIOCGPGRP); err == nil {
			cmd.SysProcAttr.Foreground, cmd.SysProcAttr.Ctty = true, fd
			restore = func() error {
				ignored := signal.Ignored(syscall.SIGTTOU)
				signal.Ignore(syscall.SIGTTOU)
				err := unix.IoctlSetPointerInt(fd, unix.TIOCSPGRP, group)
				if !ignored {
					signal.Reset(syscall.SIGTTOU)
				}
				return errors.Join(err, term.Restore(fd, state))
			}
		}
	}
	return restore, nil
}

//go:build darwin || linux

package codingagent

import (
	"os"
	"os/exec"
	"syscall"
)

func configureRPCProcess(cmd *exec.Cmd)  { cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} }
func killRPCProcess(process *os.Process) { _ = syscall.Kill(-process.Pid, syscall.SIGKILL) }

func openRPCPipe(file *os.File) (*os.File, error) {
	fd, err := syscall.Dup(int(file.Fd()))
	if err != nil {
		return nil, err
	}
	if err = syscall.SetNonblock(fd, true); err != nil {
		syscall.Close(fd)
		return nil, err
	}
	return os.NewFile(uintptr(fd), file.Name()), nil
}

//go:build darwin || linux

package codingagent

import (
	"os"
	"os/exec"
	"syscall"
)

func defaultBashShell() (ShellConfig, error) {
	shell := "/bin/bash"
	if _, err := os.Stat(shell); err != nil {
		shell, err = exec.LookPath("bash")
		if err != nil {
			shell = "sh"
		}
	}
	return ShellConfig{Shell: shell, Args: []string{"-c"}}, nil
}
func configureBashProcess(cmd *exec.Cmd) error {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return nil
}
func killBashProcess(process *os.Process) {
	if err := syscall.Kill(-process.Pid, syscall.SIGKILL); err != nil {
		_ = process.Kill()
	}
}

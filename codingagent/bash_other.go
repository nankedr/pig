//go:build !darwin && !linux

package codingagent

import (
	"os"
	"os/exec"
)

func defaultBashShell() (ShellConfig, error) {
	return ShellConfig{}, notImplemented("GetShellConfig.platform")
}
func configureBashProcess(*exec.Cmd) error { return notImplemented("BashOperations.platform") }
func killBashProcess(process *os.Process)  { _ = process.Kill() }

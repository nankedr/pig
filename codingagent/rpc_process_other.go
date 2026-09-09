//go:build !darwin && !linux

package codingagent

import (
	"os"
	"os/exec"
)

func configureRPCProcess(*exec.Cmd)      {}
func killRPCProcess(process *os.Process) { _ = process.Kill() }

func openRPCPipe(file *os.File) (*os.File, error) { return file, nil }

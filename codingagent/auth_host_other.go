//go:build !darwin && !linux

package codingagent

import (
	"context"
	"os"
	"os/exec"
)

func lockAuthFile(context.Context, *os.File) error { return notImplemented("credential.host") }
func unlockAuthFile(*os.File)                      {}
func credentialCommand(ctx context.Context, command string) *exec.Cmd {
	return exec.CommandContext(ctx, "cmd.exe", "/c", command)
}

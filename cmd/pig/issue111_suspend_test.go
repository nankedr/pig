//go:build darwin || linux

package main

import (
	"context"
	"github.com/nankedr/pig/internal/terminaltest"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestPigSuspend111(t *testing.T) {
	tty := terminaltest.Open(t)
	root := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, buildPigBinary(t), "--provider", "deepseek", "--model", "deepseek-v4-flash", "--api-key", "synthetic", "--no-session", "--no-tools", "--no-extensions", "--no-skills")
	cmd.Dir = root
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + root, "TERM=xterm-256color", "PIG_OFFLINE=1"}
	cmd.Stdin = tty.Slave
	cmd.Stdout = tty.Slave
	cmd.Stderr = tty.Slave
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { cmd.Process.Kill(); cmd.Wait() }()
	tty.Wait(t, "> ")
	tty.Send(t, "draft\x1a")
	var state syscall.WaitStatus
	if _, err := syscall.Wait4(cmd.Process.Pid, &state, syscall.WUNTRACED, nil); err != nil {
		t.Fatal(err)
	}
	if !state.Stopped() {
		t.Fatalf("not suspended: %v", state)
	}
	if !tty.Restored(t) {
		t.Fatal("suspend left raw mode enabled")
	}
	tty.Wait(t, "\x1b[<u")
	if err := cmd.Process.Signal(syscall.SIGCONT); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for strings.Count(tty.Output(), "\x1b[>7u") < 2 {
		if time.Now().After(deadline) {
			t.Fatal("resume did not negotiate")
		}
		time.Sleep(time.Millisecond)
	}
	deadline = time.Now().Add(time.Second)
	for {
		output := tty.Output()
		start := strings.LastIndex(output, "\x1b[>7u")
		if start >= 0 && strings.Contains(output[start:], "> draft") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("draft lost after resume")
		}
		time.Sleep(time.Millisecond)
	}
	tty.Send(t, "\x03\x04")
	if err := cmd.Wait(); err != nil {
		t.Fatal(err)
	}
	if !tty.Restored(t) {
		t.Fatal("exit left raw mode enabled")
	}
}

//go:build darwin || linux

package main

import (
	"context"
	"github.com/nankedr/pig/internal/terminaltest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestPigAutocomplete115(t *testing.T) { runEditorCLI(t, "autocomplete-cli.json") }

func TestPigUndeliveredCommand115(t *testing.T) {
	tty := terminaltest.Open(t)
	root := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, buildPigBinary(t), "--provider", "deepseek", "--model", "deepseek-v4-flash", "--api-key", "synthetic", "--no-session", "--no-tools", "--no-extensions", "--no-skills")
	cmd.Dir = root
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + root, "TERM=xterm-256color", "PIG_DEEPSEEK_BASE_URL=http://127.0.0.1:1"}
	cmd.Stdin = tty.Slave
	cmd.Stdout = tty.Slave
	cmd.Stderr = tty.Slave
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	tty.Wait(t, "> ")
	tty.Send(t, "/sett")
	tty.Wait(t, "Open settings menu")
	tty.Send(t, "\r")
	tty.Wait(t, "InteractiveMode.command.settings: not implemented")
	tty.Send(t, "/quit\r")
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if strings.Contains(tty.Output(), "connection refused") {
		t.Fatal("undelivered command reached Provider")
	}
	if !tty.Restored(t) {
		t.Fatal("terminal not restored")
	}
}

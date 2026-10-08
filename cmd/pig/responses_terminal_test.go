//go:build darwin || linux

package main

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/nankedr/pig/codingagent"
	"github.com/nankedr/pig/internal/terminaltest"
)

func TestResponsesTerminalCodingTask(t *testing.T) {
	server, _ := taskServer133(t)
	root, dir := taskRoot133(t)
	tty := terminaltest.Open(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary133(t), "--tools", "edit", "--no-context-files")
	cmd.Dir, cmd.Env = root, taskEnv133(root, dir, server.URL)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = tty.Slave, tty.Slave, tty.Slave
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	tty.Wait(t, "> ")
	tty.Send(t, "change sentinel.txt\r")
	tty.Wait(t, "TASK_DONE")
	tty.Send(t, "/model deepseek-v4-flash\r")
	tty.Wait(t, "Model: deepseek-v4-flash")
	tty.Send(t, "continue\r")
	tty.Wait(t, "CONTINUED")
	tty.Send(t, "\x04")
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("terminal: %v %s", err, tty.Output())
		}
	case <-ctx.Done():
		t.Fatal("terminal exit timed out")
	}
	tty.Wait(t, "\x1b[?2004l")
	if !tty.Restored(t) || !tty.CursorVisible() || !tty.PasteDisabled() {
		t.Fatal("terminal state not restored")
	}
	sessions := filepath.Join(dir, "sessions")
	entries, err := codingagent.ListAllSessions(context.Background(), codingagent.SessionListOptions{SessionDir: &sessions})
	if err != nil || len(entries) != 1 {
		t.Fatalf("sessions: %v %v", entries, err)
	}
	assertTask133(t, root, entries[0].Path)
}

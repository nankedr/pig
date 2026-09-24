//go:build darwin || linux

package codingagent_test

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
	"github.com/nankedr/pig/internal/terminaltest"
	"github.com/nankedr/pig/tui"
)

func TestInteractiveMaintenanceReload124(t *testing.T) {
	s, loader, core, _, dir := reload106Session(t)
	original := s.SystemPrompt()
	var calls atomic.Int32
	entered := make(chan struct{})
	loader.reload = func(ctx context.Context) error {
		switch calls.Add(1) {
		case 1:
			return errors.New("RELOAD_FAILURE")
		case 2:
			close(entered)
			<-ctx.Done()
			return ctx.Err()
		default:
			return loader.ResourceLoader.Reload(ctx)
		}
	}
	response, _ := ai.FauxAssistantMessage(ai.FauxAssistantText("AFTER_RELOAD"))
	core.SetResponses([]ai.FauxResponseStep{response})
	runtime := codingagent.NewAgentSessionRuntime(s, codingagent.AgentSessionServices{AgentDir: dir}, nil, nil, nil)
	tty := terminaltest.Open(t)
	terminal := tui.NewProcessTerminal(tty.Slave, tty.Slave)
	mode := codingagent.NewInteractiveMode(runtime, codingagent.InteractiveModeOptions{Terminal: terminal})
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	done := make(chan error, 1)
	go func() { done <- mode.Run(ctx) }()
	t.Cleanup(func() { cancel(); _ = mode.Stop() })
	tty.Wait(t, "> ")
	tty.Send(t, "/reload\r")
	tty.Wait(t, "Reload failed: RELOAD_FAILURE")
	tty.Send(t, "/reload\r")
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("reload did not start")
	}
	tty.Wait(t, "Reloading keybindings")
	tty.Send(t, "retained question\r")
	tty.Wait(t, "input restored to editor")
	tty.Send(t, "\x1b[27u")
	tty.Wait(t, "Reload cancelled")
	if s.SystemPrompt() != original {
		t.Fatal("cancel changed system prompt", s.SystemPrompt())
	}
	tty.Send(t, "\r")
	tty.Wait(t, "AFTER_RELOAD")
	if err := s.WaitForIdle(ctx); err != nil {
		t.Fatal(err)
	}
	time.Sleep(30 * time.Millisecond)
	tty.Send(t, "/reload\r")
	tty.Wait(t, "Reloaded keybindings")
	tty.Send(t, "/session\r")
	tty.Wait(t, "Current context")
	tty.Wait(t, "Full history")
	tty.Send(t, "\x04")
	if err := awaitInteractive(t, done); err != nil {
		t.Fatal(err)
	}
	if !tty.Restored(t) {
		t.Fatal("terminal not restored")
	}
	select {
	case <-terminal.Done():
	default:
		t.Fatal("terminal reader survived")
	}
	if len(s.Messages()) != 2 || !strings.Contains(message85Text(s.Messages()[0]), "retained question") {
		t.Fatal("busy input lost or command entered history", s.Messages())
	}
}

func TestInteractiveMaintenanceStop124(t *testing.T) {
	for _, exit := range []string{"quit", "stop"} {
		t.Run(exit, func(t *testing.T) {
			s, loader, _, _, dir := reload106Session(t)
			entered, finished := make(chan struct{}), make(chan struct{})
			loader.reload = func(ctx context.Context) error { close(entered); <-ctx.Done(); close(finished); return ctx.Err() }
			runtime := codingagent.NewAgentSessionRuntime(s, codingagent.AgentSessionServices{AgentDir: dir}, nil, nil, nil)
			tty := terminaltest.Open(t)
			mode := codingagent.NewInteractiveMode(runtime, codingagent.InteractiveModeOptions{Terminal: tui.NewProcessTerminal(tty.Slave, tty.Slave)})
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			done := make(chan error, 1)
			go func() { done <- mode.Run(ctx) }()
			t.Cleanup(func() { cancel(); _ = mode.Stop() })
			tty.Wait(t, "> ")
			tty.Send(t, "/reload\r")
			select {
			case <-entered:
			case <-ctx.Done():
				t.Fatal("reload did not start")
			}
			if exit == "quit" {
				tty.Send(t, "/quit\r")
			} else if err := mode.Stop(); err != nil {
				t.Fatal(err)
			}
			if err := awaitInteractive(t, done); err != nil {
				t.Fatal(err)
			}
			select {
			case <-finished:
			default:
				t.Fatal("reload outlived Run")
			}
			if !tty.Restored(t) {
				t.Fatal("terminal not restored")
			}
		})
	}
}

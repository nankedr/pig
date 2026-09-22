//go:build darwin || linux

package codingagent_test

import (
	"context"
	"github.com/nankedr/pig/codingagent"
	"github.com/nankedr/pig/internal/terminaltest"
	"github.com/nankedr/pig/tui"
	"strings"
	"testing"
	"time"
)

func TestInteractiveThemeNotifications120(t *testing.T) {
	t.Setenv("COLORTERM", "truecolor")
	t.Setenv("COLORFGBG", "0;15")
	runtime := interactiveRuntime(t)
	if err := runtime.Session().SettingsManager().SetTheme("light/dark"); err != nil {
		t.Fatal(err)
	}
	tty := terminaltest.Open(t)
	mode := codingagent.NewInteractiveMode(runtime, codingagent.InteractiveModeOptions{Terminal: tui.NewProcessTerminal(tty.Slave, tty.Slave)})
	defer mode.Stop()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- mode.Run(ctx) }()
	tty.Wait(t, "\x1b[?996n")
	tty.Send(t, "\x1b]11;rgb:ffff/ffff/ffff\x07\x1b[?997;1n")
	tty.Wait(t, "\x1b[?2031h")
	tty.Send(t, "DRAFT")
	tty.Wait(t, "\x1b[38;2;212;212;212m> DRAFT")
	tty.Send(t, "\x1b[?997;2n")
	tty.Wait(t, "\x1b[38;2;31;35;40m> DRAFT")
	tty.Send(t, "\r")
	tty.Wait(t, "FIRST_DONE")
	tty.Send(t, "/settings\r")
	tty.Wait(t, "Thinking level")
	tty.Send(t, "\x1b[?997;1n")
	tty.Wait(t, "\x1b[38;2;138;190;183mSettings")
	tty.Send(t, "\x1b[27u")
	time.Sleep(100 * time.Millisecond)
	tty.Send(t, "\x04")
	if err := awaitInteractive(t, done); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(tty.Output(), "\x1b[?2031l") || !tty.Restored(t) {
		t.Fatal("notification/terminal cleanup")
	}
}

//go:build darwin || linux

package tui_test

import (
	"context"
	"github.com/nankedr/pig/internal/terminaltest"
	"github.com/nankedr/pig/tui"
	"testing"
	"time"
)

func TestTerminalNegotiationRestart111(t *testing.T) {
	tty := terminaltest.Open(t)
	terminal := tui.NewProcessTerminal(tty.Slave, tty.Slave)
	input := make(chan string, 64)
	for round := 0; round < 2; round++ {
		if err := terminal.Start(func(s string) { input <- s }, nil); err != nil {
			t.Fatal(err)
		}
		tty.Wait(t, "\x1b[>7u\x1b[?u\x1b[c")
		tty.Send(t, "\x1b[?1;2c")
		tty.Wait(t, "\x1b[>4;2m")
		tty.Send(t, "\x1b[?")
		time.Sleep(30 * time.Millisecond)
		tty.Send(t, "7u")
		tty.Wait(t, "\x1b[>4;0m")
		deadline := time.Now().Add(time.Second)
		for {
			active, _ := terminal.KittyProtocolActive()
			if active {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("Kitty not enabled")
			}
			time.Sleep(time.Millisecond)
		}
		tty.Send(t, "\x1b[97uaxa\x1b[97;1:3u")
		for _, want := range []string{"\x1b[97u", "x", "a", "\x1b[97;1:3u"} {
			select {
			case got := <-input:
				if got != want {
					t.Fatalf("input %q want %q", got, want)
				}
			case <-time.After(time.Second):
				t.Fatal("input timed out")
			}
		}
		tty.Send(t, "\x1b[")
		if err := terminal.DrainInput(context.Background(), 100*time.Millisecond, 20*time.Millisecond); err != nil {
			t.Fatal(err)
		}
		if err := terminal.Stop(); err != nil {
			t.Fatal(err)
		}
		if !tty.Restored(t) {
			t.Fatal("raw mode left enabled")
		}
		select {
		case got := <-input:
			t.Fatalf("late callback %q", got)
		case <-time.After(30 * time.Millisecond):
		}
	}
}

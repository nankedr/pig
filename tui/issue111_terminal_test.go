//go:build darwin || linux

package tui_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/nankedr/pig/internal/terminaltest"
	"github.com/nankedr/pig/tui"
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

func TestTerminalCallbackLifecycle111(t *testing.T) {
	for _, test := range []struct {
		key   string
		drain bool
	}{{"a", false}, {"\x1b", false}, {"a", true}, {"\x1b", true}} {
		t.Run(fmt.Sprintf("%q/drain=%v", test.key, test.drain), func(t *testing.T) {
			tty := terminaltest.Open(t)
			terminal := tui.NewProcessTerminal(tty.Slave, tty.Slave)
			finished := make(chan error, 1)
			entered, release := make(chan struct{}), make(chan struct{})
			if err := terminal.Start(func(string) {
				close(entered)
				<-release
				if test.drain {
					if err := terminal.DrainInput(context.Background(), 20*time.Millisecond, time.Millisecond); err != nil {
						finished <- err
						return
					}
				}
				finished <- terminal.Stop()
			}, nil); err != nil {
				t.Fatal(err)
			}
			tty.Send(t, test.key)
			select {
			case <-entered:
			case <-time.After(time.Second):
				t.Fatal("input callback not entered")
			}
			tty.Send(t, strings.Repeat("x", 256))
			time.Sleep(20 * time.Millisecond)
			close(release)
			select {
			case err := <-finished:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(time.Second):
				t.Fatal("input callback could not drain and stop terminal")
			}
			if !tty.Restored(t) {
				t.Fatal("raw mode left enabled")
			}
			select {
			case <-terminal.Done():
			case <-time.After(time.Second):
				t.Fatal("terminal callback dispatcher did not finish")
			}
		})
	}
}

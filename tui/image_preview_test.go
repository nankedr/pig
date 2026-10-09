//go:build darwin || linux

package tui_test

import (
	"context"
	"encoding/base64"
	"errors"
	"github.com/nankedr/pig/internal/terminaltest"
	"github.com/nankedr/pig/tui"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestImageWorkflowPreview136(t *testing.T) {
	for _, mode := range []tui.TUIMode{tui.TUIModeRegular, tui.TUIModeFullscreen} {
		for _, supported := range []bool{false, true} {
			t.Run(string(mode)+"/"+map[bool]string{false: "fallback", true: "kitty"}[supported], func(t *testing.T) {
				if supported && (runtime.GOOS != "darwin" || runtime.GOARCH != "arm64") {
					t.Skip("V1 Kitty requires darwin-arm64")
				}
				t.Setenv("TERM", "xterm-256color")
				t.Setenv("TERM_PROGRAM", "")
				t.Setenv("TMUX", "")
				t.Setenv("STY", "")
				t.Setenv("ZELLIJ", "")
				if supported {
					t.Setenv("TERM", "xterm-kitty")
				}
				tty := terminaltest.Open(t)
				terminal := tui.NewProcessTerminal(tty.Slave, tty.Slave)
				ui := tui.NewTextUI(terminal, tui.TextUIOptions{Mode: mode})
				if err := ui.Start(); err != nil {
					t.Fatal(err)
				}
				defer ui.Stop()
				data, err := os.ReadFile("../parity/services/user-image.png")
				if err != nil {
					t.Fatal(err)
				}
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				done := make(chan error, 1)
				go func() {
					done <- ui.PreviewImage(ctx, base64.StdEncoding.EncodeToString(data), tui.ImageDimensions{WidthPX: 1, HeightPX: 1})
				}()
				needle := "Image preview unavailable"
				if supported {
					needle = "\x1b_Ga=T,f=100,q=2"
				}
				tty.Wait(t, needle)
				if err := tty.SetSize(8, 18); err != nil {
					t.Fatal(err)
				}
				if err := ui.Refresh(); err != nil {
					t.Fatal(err)
				}
				if supported {
					tty.Wait(t, ",r=5,")
					position := "\x1b7\x1b[4A\r\x1b_G"
					if mode == tui.TUIModeFullscreen {
						position = "\x1b7\x1b[4;1H\x1b_G"
					}
					tty.Wait(t, position)
				} else {
					tty.Wait(t, "unavailable: V1")
				}
				deadline := time.Now().Add(3 * time.Second)
				for !strings.Contains(tty.ScreenText(), "Image preview\n") && time.Now().Before(deadline) {
					time.Sleep(time.Millisecond)
				}
				if !strings.Contains(tty.ScreenText(), "Image preview\n") {
					t.Fatalf("narrow preview lost its wrapped title: %q", tty.ScreenText())
				}
				tty.Send(t, "\x1b[5~") // Scroll input cannot corrupt the bounded preview.
				if err := ui.Refresh(); err != nil {
					t.Fatal(err)
				}
				cancel()
				select {
				case err := <-done:
					if !errors.Is(err, context.Canceled) {
						t.Fatal(err)
					}
				case <-time.After(3 * time.Second):
					t.Fatal("preview did not cancel")
				}
				if err := ui.Stop(); err != nil {
					t.Fatal(err)
				}
				if !tty.Restored(t) {
					t.Fatal("preview left raw mode")
				}
				if supported && !strings.Contains(tty.Output(), "\x1b_Ga=d,d=I,i=") {
					t.Fatal("image resources not deleted")
				}
				if !supported && strings.Contains(tty.Output(), "\x1b_G") {
					t.Fatal("unsupported terminal received graphics")
				}
			})
		}
	}
}

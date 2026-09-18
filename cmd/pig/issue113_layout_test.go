//go:build darwin || linux

package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/nankedr/pig/internal/terminaltest"
	"github.com/nankedr/pig/tui"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

func TestPigLayoutScrolling113(t *testing.T) {
	data, err := os.ReadFile("../../parity/oracle/fixtures/layout-scrolling-cli.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Observation struct{ Outcome map[string]map[string]any }
	}
	if err = json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	binary := buildPigBinary(t)
	for _, mode := range []string{"regular", "fullscreen"} {
		t.Run(mode, func(t *testing.T) {
			release := make(chan struct{})
			var once sync.Once
			defer once.Do(func() { close(release) })
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				chunk := func(text string, finish any) {
					b, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"content": text}, "finish_reason": finish}}})
					fmt.Fprintf(w, "data: %s\n\n", b)
					w.(http.Flusher).Flush()
				}
				lines := []string{"```text"}
				for i := 0; i < 60; i++ {
					lines = append(lines, fmt.Sprintf("ROW%03d", i))
				}
				chunk(strings.Join(lines, "\n")+"\n", nil)
				select {
				case <-release:
				case <-r.Context().Done():
					return
				}
				chunk("STREAM_DONE\n```", "stop")
				fmt.Fprint(w, "data: [DONE]\n\n")
			}))
			defer server.Close()
			tty := terminaltest.Open(t)
			if err := tty.SetSize(24, 60); err != nil {
				t.Fatal(err)
			}
			root := t.TempDir()
			agent := filepath.Join(root, ".pig", "agent")
			if err := os.MkdirAll(agent, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(agent, "settings.json"), []byte(`{"fullscreenScrollbar":"always","fullscreenExitOutput":"resume-hint"}`), 0600); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, binary, "--provider", "deepseek", "--model", "deepseek-v4-flash", "--api-key", "synthetic", "--no-session", "--no-tools", "--no-extensions", "--no-skills", "--tui-mode", mode)
			cmd.Dir = root
			cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + root, "TERM=xterm-256color", "PIG_OFFLINE=1", "PIG_DEEPSEEK_BASE_URL=" + server.URL}
			cmd.Stdin, cmd.Stdout, cmd.Stderr = tty.Slave, tty.Slave, tty.Slave
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			defer func() {
				if cmd.ProcessState == nil {
					cmd.Process.Kill()
					cmd.Wait()
				}
			}()
			frame := func() string { return tty.ScreenText() }
			wait := func(predicate func(string) bool) {
				t.Helper()
				deadline := time.Now().Add(5 * time.Second)
				for time.Now().Before(deadline) {
					s := frame()
					if strings.Contains(tty.Output(), "\x1b[?2026l") && predicate(s) {
						return
					}
					time.Sleep(10 * time.Millisecond)
				}
				t.Fatalf("missing expected frame: %q", frame())
			}
			tty.Wait(t, "> ")
			tty.Send(t, "browse\r")
			wait(func(s string) bool { return strings.Contains(s, "ROW059") })
			got := map[string]any{"tail_visible": true, "alternate_screen": strings.Contains(tty.Output(), "\x1b[?1049h")}
			if mode == "fullscreen" {
				tty.Send(t, "\x1b[H")
				wait(func(s string) bool { return strings.Contains(s, "ROW000") })
				got["history_visible"] = !strings.Contains(frame(), "ROW059")
				once.Do(func() { close(release) })
				time.Sleep(150 * time.Millisecond)
				got["append_keeps_history"] = strings.Contains(frame(), "ROW000") && !strings.Contains(frame(), "STREAM_DONE")
				tty.Send(t, "\x1b[F")
				wait(func(s string) bool { return strings.Contains(s, "STREAM_DONE") })
				got["end_restores_tail"] = true
				tty.Send(t, "\x1b[<64;2;2M")
			} else {
				once.Do(func() { close(release) })
				wait(func(s string) bool { return strings.Contains(s, "STREAM_DONE") })
			}
			if mode == "regular" {
				history := tty.ScrollbackText()
				if !strings.Contains(history, "ROW000") || !strings.Contains(history, "ROW020") {
					t.Fatalf("native scrollback lost history: %q", history)
				}
			}
			if mode == "regular" {
				got["native_scrollback"] = true
			}
			editOffset := len(tty.Output())
			tty.Send(t, "DRAFT\x1b[D\x1b[D")
			wait(func(s string) bool { return strings.Contains(s, "DRAFT") })
			if mode == "regular" {
				delta := tty.Output()[editOffset:]
				if strings.Contains(delta, "ROW000") || strings.Contains(delta, "\x1b[2J") {
					t.Fatalf("editing replayed the transcript: %q", delta)
				}
			}
			if mode == "regular" {
				got["incremental_editor"] = true
			}
			for _, size := range [][2]uint16{{12, 28}, {30, 80}, {8, 18}, {24, 60}} {
				if err := tty.SetSize(size[0], size[1]); err != nil {
					t.Fatal(err)
				}
				if err := cmd.Process.Signal(syscall.SIGWINCH); err != nil {
					t.Fatal(err)
				}
				time.Sleep(80 * time.Millisecond)
				wait(func(s string) bool { return strings.Contains(s, "DRAFT") })
				lines := strings.Split(frame(), "\n")
				if len(lines) > int(size[0]) {
					t.Fatalf("height overflow %d > %d", len(lines), size[0])
				}
				for _, l := range lines {
					w, _ := tui.VisibleWidth(l)
					if w > int(size[1]) {
						t.Fatalf("width overflow %q", l)
					}
				}
			}
			if mode == "fullscreen" {
				tty.Send(t, "\x1b[F")
			}
			wait(func(s string) bool { return strings.Contains(s, "STREAM_DONE") })
			tty.Send(t, "X")
			wait(func(s string) bool { return strings.Contains(s, "DRAXFT") })
			got["resize_keeps_editor"] = true
			tty.Send(t, "\x03")
			time.Sleep(30 * time.Millisecond)
			tty.Send(t, "\x04")
			if err := cmd.Wait(); err != nil {
				t.Fatal(err)
			}
			got["exit_code"] = float64(cmd.ProcessState.ExitCode())
			got["terminal_restored"] = tty.Restored(t)
			if !reflect.DeepEqual(got, fixture.Observation.Outcome[mode]) {
				t.Fatalf("CLI parity %v; want %v", got, fixture.Observation.Outcome[mode])
			}
		})
	}
}

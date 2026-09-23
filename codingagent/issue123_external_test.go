//go:build darwin || linux

package codingagent_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nankedr/pig/codingagent"
	"github.com/nankedr/pig/internal/terminaltest"
	"github.com/nankedr/pig/tui"
)

func TestExternalEditorSelection123(t *testing.T) {
	runtime := interactiveRuntime(t)
	defer runtime.Dispose(context.Background())
	settings := runtime.Session().SettingsManager()
	for _, tc := range []struct{ configured, visual, editor, want string }{
		{"configured --wait", "visual", "editor", "configured --wait"},
		{"  ", "visual --wait", "editor", "visual --wait"},
		{"", "", "editor -w", "editor -w"},
		{"", "", "", "nano"},
	} {
		t.Setenv("VISUAL", tc.visual)
		t.Setenv("EDITOR", tc.editor)
		if err := settings.ApplyOverrides(codingagent.Settings{ExternalEditor: &tc.configured}); err != nil {
			t.Fatal(err)
		}
		if got, err := settings.GetExternalEditorCommand(); err != nil || got != tc.want {
			t.Fatalf("got %q %v; want %q", got, err, tc.want)
		}
	}
}

func TestExternalEditorFailureRecovery123(t *testing.T) {
	for _, failure := range []string{"read", "create"} {
		t.Run(failure, func(t *testing.T) {
			runtime := interactiveRuntime(t)
			root := t.TempDir()
			script := filepath.Join(root, "editor")
			if err := os.WriteFile(script, []byte("#!/bin/sh\nrm \"$1\"\n"), 0700); err != nil {
				t.Fatal(err)
			}
			if err := runtime.Session().SettingsManager().ApplyOverrides(codingagent.Settings{ExternalEditor: &script}); err != nil {
				t.Fatal(err)
			}
			tty := terminaltest.Open(t)
			mode := codingagent.NewInteractiveMode(runtime, codingagent.InteractiveModeOptions{Terminal: tui.NewProcessTerminal(tty.Slave, tty.Slave)})
			defer mode.Stop()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := mode.Init(ctx); err != nil {
				t.Fatal(err)
			}
			if failure == "create" {
				t.Setenv("TMPDIR", filepath.Join(root, "missing"))
			}
			input := make(chan string, 1)
			go func() {
				value, err := mode.GetUserInput(ctx)
				if err != nil {
					value = err.Error()
				}
				input <- value
			}()
			tty.Send(t, "draft 保留\x07")
			tty.Wait(t, "Error: external editor:")
			tty.Send(t, "\r")
			select {
			case got := <-input:
				if got != "draft 保留" {
					t.Fatal(got)
				}
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			if err := mode.Stop(); err != nil {
				t.Fatal(err)
			}
			deadline := time.Now().Add(time.Second)
			for (!tty.CursorVisible() || !tty.PasteDisabled()) && time.Now().Before(deadline) {
				time.Sleep(5 * time.Millisecond)
			}
			if !tty.Restored(t) || !tty.CursorVisible() || !tty.PasteDisabled() {
				t.Fatal("terminal not restored")
			}
		})
	}
}

func TestExternalEditorStopAndCancel123(t *testing.T) {
	for _, exit := range []string{"stop", "cancel"} {
		t.Run(exit, func(t *testing.T) {
			runtime := interactiveRuntime(t)
			root := t.TempDir()
			marker := filepath.Join(root, "survived")
			capture := filepath.Join(root, "temp-path")
			release := filepath.Join(root, "release")
			script := filepath.Join(root, "editor")
			body := fmt.Sprintf("#!/bin/sh\nprintf '%%s' \"$1\" > %q\nstty raw -echo\nprintf 'EXTERNAL_READY\\n'\n(while [ ! -f %q ]; do sleep 0.05; done; touch %q) &\nwait\n", capture, release, marker)
			if err := os.WriteFile(script, []byte(body), 0700); err != nil {
				t.Fatal(err)
			}
			if err := runtime.Session().SettingsManager().ApplyOverrides(codingagent.Settings{ExternalEditor: &script}); err != nil {
				t.Fatal(err)
			}
			tty := terminaltest.Open(t)
			mode := codingagent.NewInteractiveMode(runtime, codingagent.InteractiveModeOptions{Terminal: tui.NewProcessTerminal(tty.Slave, tty.Slave), TUIMode: tui.TUIModeFullscreen})
			defer mode.Stop()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := mode.Init(ctx); err != nil {
				t.Fatal(err)
			}
			editCtx, stopEdit := context.WithCancel(ctx)
			defer stopEdit()
			done := make(chan error, 1)
			go func() { done <- mode.OpenExternalEditor(editCtx) }()
			tty.Wait(t, "EXTERNAL_READY")
			offset := len(tty.Output())
			if err := mode.ShowWarning("BACKGROUND_EVENT"); err != nil {
				t.Fatal(err)
			}
			if err := tty.SetSize(25, 90); err != nil {
				t.Fatal(err)
			}
			time.Sleep(100 * time.Millisecond)
			if output := tty.Output()[offset:]; output != "" {
				t.Fatalf("UI wrote into editor: %q", output)
			}
			if exit == "stop" {
				if err := mode.Stop(); err != nil {
					t.Fatal(err)
				}
			} else {
				stopEdit()
			}
			select {
			case err := <-done:
				if err == nil {
					t.Fatal("cancellation succeeded")
				}
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			path, err := os.ReadFile(capture)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = os.Stat(filepath.Dir(string(path))); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("temporary directory leaked", err)
			}
			if exit == "cancel" {
				tty.Wait(t, "BACKGROUND_EVENT")
				result := make(chan string, 1)
				go func() { s, _ := mode.GetUserInput(ctx); result <- s }()
				tty.Send(t, "after cancel\r")
				select {
				case s := <-result:
					if s != "after cancel" {
						t.Fatal(s)
					}
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				}
				if err := mode.Stop(); err != nil {
					t.Fatal(err)
				}
			}
			if !tty.Restored(t) {
				t.Fatal("terminal not restored")
			}
			if err := os.WriteFile(release, nil, 0600); err != nil {
				t.Fatal(err)
			}
			time.Sleep(200 * time.Millisecond)
			if _, err := os.Stat(marker); !os.IsNotExist(err) {
				t.Fatal("descendant survived cancellation", err)
			}
		})
	}
}

func TestExternalEditorExpandedDraft123(t *testing.T) {
	runtime := interactiveRuntime(t)
	root := t.TempDir()
	capture := filepath.Join(root, "draft")
	script := filepath.Join(root, "editor")
	if err := os.WriteFile(script, []byte(fmt.Sprintf("#!/bin/sh\ncp \"$1\" %q\nprintf '保存 ✅\\n第二行\\n\\n' > \"$1\"\n", capture)), 0700); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Session().SettingsManager().ApplyOverrides(codingagent.Settings{ExternalEditor: &script}); err != nil {
		t.Fatal(err)
	}
	tty := terminaltest.Open(t)
	mode := codingagent.NewInteractiveMode(runtime, codingagent.InteractiveModeOptions{Terminal: tui.NewProcessTerminal(tty.Slave, tty.Slave)})
	defer mode.Stop()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := mode.Init(ctx); err != nil {
		t.Fatal(err)
	}
	draft := strings.Repeat("草稿🐷\n", 1000)
	result := make(chan string, 1)
	go func() { s, _ := mode.GetUserInput(ctx); result <- s }()
	tty.Send(t, "\x1b[200~"+draft+"\x1b[201~\x07")
	tty.Wait(t, "保存 ✅")
	got, err := os.ReadFile(capture)
	if err != nil || string(got) != draft {
		t.Fatalf("expanded draft mismatch: %v, bytes %d", err, len(got))
	}
	if err := mode.OpenExternalEditor(ctx); err != nil {
		t.Fatal(err)
	}
	got, err = os.ReadFile(capture)
	if err != nil || string(got) != "保存 ✅\n第二行\n" {
		t.Fatalf("exactly one trailing LF must be removed: %q %v", got, err)
	}
	tty.Send(t, "\r")
	select {
	case s := <-result:
		if s != "保存 ✅\n第二行" {
			t.Fatal(s)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}

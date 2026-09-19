//go:build darwin || linux

package codingagent_test

import (
	"context"
	"errors"
	"github.com/nankedr/pig/codingagent"
	"github.com/nankedr/pig/internal/terminaltest"
	"github.com/nankedr/pig/tui"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestProjectTrustSDK114(t *testing.T) {
	for _, name := range []string{"confirm", "cancel", "save-failure", "context-cancel"} {
		t.Run(name, func(t *testing.T) {
			cwd, dir := t.TempDir(), t.TempDir()
			config := filepath.Join(cwd, ".pig")
			if err := os.MkdirAll(config, 0700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(config, "settings.json")
			if err := syscall.Mkfifo(path, 0600); err != nil {
				t.Fatal(err)
			}
			settings, err := codingagent.NewSettingsManager(cwd, &dir)
			if err != nil {
				t.Fatal(err)
			}
			tty := terminaltest.Open(t)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			done := make(chan error, 1)
			go func() {
				done <- codingagent.PrepareProjectSettings(ctx, codingagent.PrepareProjectSettingsOptions{CWD: cwd, AgentDir: dir, SettingsManager: settings, Terminal: tui.NewProcessTerminal(tty.Slave, tty.Slave)})
			}()
			tty.Wait(t, "Trust project folder?")
			if trusted, _ := settings.IsProjectTrusted(); trusted {
				t.Fatal("trusted before choice")
			}
			if diagnostics, _ := settings.DrainErrors(); len(diagnostics) != 0 {
				t.Fatal("project settings inspected before decision")
			}
			switch name {
			case "confirm":
				if err = os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err = os.WriteFile(path, []byte(`{"defaultModel":"trusted-model"}`), 0600); err != nil {
					t.Fatal(err)
				}
				tty.Send(t, "\r")
			case "save-failure":
				if err = os.Mkdir(filepath.Join(dir, "trust.json"), 0700); err != nil {
					t.Fatal(err)
				}
				tty.Send(t, "\r")
			case "context-cancel":
				cancel()
			default:
				tty.Send(t, "\x1b")
			}
			select {
			case err = <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("trust decision blocked on untrusted FIFO")
			}
			switch name {
			case "save-failure":
				if err == nil || !strings.Contains(err.Error(), "save project trust") {
					t.Fatalf("save failure: %v", err)
				}
			case "context-cancel":
				if !errors.Is(err, context.Canceled) {
					t.Fatal(err)
				}
			default:
				if err != nil {
					t.Fatal(err)
				}
			}
			if trusted, _ := settings.IsProjectTrusted(); trusted != (name == "confirm") {
				t.Fatalf("trust state %v", trusted)
			}
			if name == "confirm" {
				if model, _ := settings.GetDefaultModel(); model != "trusted-model" {
					t.Fatalf("model %s", model)
				}
			}
			if !tty.Restored(t) {
				t.Fatal("terminal not restored")
			}
		})
	}
}

func TestProjectTrustPrioritySDK114(t *testing.T) {
	for _, c := range []struct {
		name, policy    string
		saved, override *bool
		want            bool
	}{
		{"always", "always", nil, nil, true}, {"never", "never", nil, nil, false}, {"saved-deny", "always", codingagent.ProjectTrustDecisionUntrusted(), nil, false}, {"saved-trust", "never", codingagent.ProjectTrustDecisionTrusted(), nil, true}, {"override-deny", "always", codingagent.ProjectTrustDecisionTrusted(), codingagent.ProjectTrustDecisionUntrusted(), false}, {"override-trust", "never", codingagent.ProjectTrustDecisionUntrusted(), codingagent.ProjectTrustDecisionTrusted(), true},
	} {
		t.Run(c.name, func(t *testing.T) {
			cwd, dir := t.TempDir(), t.TempDir()
			if err := os.MkdirAll(filepath.Join(cwd, ".pig"), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(cwd, ".pig", "SYSTEM.md"), []byte("project"), 0600); err != nil {
				t.Fatal(err)
			}
			policy := codingagent.DefaultProjectTrust(c.policy)
			settings, err := codingagent.NewInMemorySettingsManager(codingagent.Settings{DefaultProjectTrust: &policy})
			if err != nil {
				t.Fatal(err)
			}
			if c.saved != nil {
				if err = codingagent.NewProjectTrustStore(dir).Set(context.Background(), cwd, c.saved); err != nil {
					t.Fatal(err)
				}
			}
			// A nonfunctional terminal makes an unexpected prompt observable.
			err = codingagent.PrepareProjectSettings(context.Background(), codingagent.PrepareProjectSettingsOptions{CWD: cwd, AgentDir: dir, SettingsManager: settings, Override: c.override, Terminal: tui.NewProcessTerminal(nil, nil)})
			if err != nil {
				t.Fatal(err)
			}
			if got, _ := settings.IsProjectTrusted(); got != c.want {
				t.Fatal(got)
			}
		})
	}
}

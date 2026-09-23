//go:build darwin || linux

package codingagent_test

import (
	"bytes"
	"context"
	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
	"github.com/nankedr/pig/internal/terminaltest"
	"github.com/nankedr/pig/tui"
	"os"
	"path/filepath"
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
	if err := runtime.Session().WaitForIdle(ctx); err != nil {
		t.Fatal(err)
	}
	tty.Send(t, "/settings\r")
	tty.Wait(t, "Thinking level")
	tty.Send(t, "\x1b[?997;1n")
	tty.Wait(t, "\x1b[38;2;138;190;183mThinking level")
	tty.Send(t, "\x1b[27u")
	time.Sleep(100 * time.Millisecond)
	tty.Send(t, "\x04")
	if err := awaitInteractive(t, done); err != nil {
		t.Fatal(err)
	}
	tty.Wait(t, "\x1b[?2031l")
	if !strings.Contains(tty.Output(), "\x1b[?2031l") || !tty.Restored(t) {
		t.Fatal("notification/terminal cleanup")
	}
}

func TestInteractiveFixedToAutomaticTheme120(t *testing.T) {
	t.Setenv("COLORTERM", "truecolor")
	t.Setenv("COLORFGBG", "")
	runtime := interactiveRuntime(t)
	if err := runtime.Session().SettingsManager().SetTheme("dark"); err != nil {
		t.Fatal(err)
	}
	tty := terminaltest.Open(t)
	mode := codingagent.NewInteractiveMode(runtime, codingagent.InteractiveModeOptions{Terminal: tui.NewProcessTerminal(tty.Slave, tty.Slave)})
	defer mode.Stop()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- mode.Run(ctx) }()
	tty.Wait(t, "> ")
	tty.Send(t, "/settings\r")
	tty.Wait(t, "Thinking level")
	tty.Send(t, "theme\r")
	tty.Wait(t, "Select a theme")
	tty.Send(t, "\x1b[A\r")
	tty.Wait(t, "Automatic Theme")
	tty.Send(t, "\r")
	waitSessionScreen118(t, tty, "Light Theme", true)
	tty.Send(t, "\x1b[B\r")
	waitSessionScreen118(t, tty, "save and go back", true)
	tty.Send(t, "\x1b[B\x1b[B\r")
	tty.Wait(t, "\x1b[?996n")
	tty.Send(t, "\x1b[?997;2n")
	waitSessionScreen118(t, tty, "Automatic Theme", false)
	waitSessionScreen118(t, tty, "Type to search", true)
	tty.Send(t, "\x1b[27u")
	waitSessionScreen118(t, tty, "Type to search", false)
	time.Sleep(100 * time.Millisecond)
	tty.Send(t, "LIGHT_DRAFT")
	tty.Wait(t, "\x1b[38;2;31;35;40m> LIGHT_DRAFT")
	if setting, _ := runtime.Session().SettingsManager().GetThemeSetting(); setting != "light/dark" {
		t.Fatal("automatic setting", setting)
	}
	tty.Send(t, "\x03")
	time.Sleep(100 * time.Millisecond)
	tty.Send(t, "\x04")
	if err := awaitInteractive(t, done); err != nil {
		t.Fatal(err)
	}
}

func TestInteractiveInvalidThemeSessionReplacement120(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	source := interactiveRuntime(t).Session()
	if err := source.Prompt(ctx, "SOURCE_HISTORY_120"); err != nil {
		t.Fatal(err)
	}
	settings := source.SettingsManager()
	if err := settings.SetTheme("dark"); err != nil {
		t.Fatal(err)
	}
	provider, err := ai.NewFauxProvider()
	if err != nil {
		t.Fatal(err)
	}
	model, _ := provider.GetModel()
	dir := t.TempDir()
	cwd := source.SessionManager().GetCWD()
	services := codingagent.AgentSessionServices{CWD: cwd, AgentDir: dir}
	runtime := codingagent.NewAgentSessionRuntime(source, services, func(ctx context.Context, options codingagent.CreateAgentSessionRuntimeOptions) (codingagent.CreateAgentSessionRuntimeResult, error) {
		created, err := codingagent.CreateAgentSession(ctx, codingagent.CreateAgentSessionOptions{CWD: cwd, AgentDir: dir, Model: &model, Provider: provider.Provider, SessionManager: options.SessionManager, SettingsManager: settings, NoTools: codingagent.NoToolsAll})
		return codingagent.CreateAgentSessionRuntimeResult{CreateAgentSessionResult: created, Services: services}, err
	}, nil, nil)
	tty := terminaltest.Open(t)
	mode := codingagent.NewInteractiveMode(runtime, codingagent.InteractiveModeOptions{Terminal: tui.NewProcessTerminal(tty.Slave, tty.Slave)})
	defer mode.Stop()
	done := make(chan error, 1)
	go func() { done <- mode.Run(ctx) }()
	tty.Wait(t, "SOURCE_HISTORY_120")
	if err = settings.SetTheme("missing-theme-120"); err != nil {
		t.Fatal(err)
	}
	tty.Send(t, "/new\r")
	tty.Wait(t, "Started new session")
	waitSessionScreen118(t, tty, "SOURCE_HISTORY_120", false)
	waitSessionScreen118(t, tty, "missing-theme-120", true)
	tty.Send(t, "\x04")
	if err = awaitInteractive(t, done); err != nil {
		t.Fatal(err)
	}
}

func TestInteractiveThemePreviewFailure120(t *testing.T) {
	dir := t.TempDir()
	data, err := os.ReadFile("themes/dark.json")
	if err != nil {
		t.Fatal(err)
	}
	data = bytes.Replace(data, []byte(`"name": "dark"`), []byte(`"name": "local"`), 1)
	path := filepath.Join(dir, "themes", "local.json")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	provider, err := ai.NewFauxProvider()
	if err != nil {
		t.Fatal(err)
	}
	model, _ := provider.GetModel()
	created, err := codingagent.CreateAgentSession(context.Background(), codingagent.CreateAgentSessionOptions{CWD: t.TempDir(), AgentDir: dir, Model: &model, Provider: provider.Provider, NoTools: codingagent.NoToolsAll})
	if err != nil {
		t.Fatal(err)
	}
	runtime := codingagent.NewAgentSessionRuntime(created.Session, codingagent.AgentSessionServices{}, nil, nil, nil)
	if err := runtime.Session().SettingsManager().SetTheme("dark"); err != nil {
		t.Fatal(err)
	}
	tty := terminaltest.Open(t)
	mode := codingagent.NewInteractiveMode(runtime, codingagent.InteractiveModeOptions{Terminal: tui.NewProcessTerminal(tty.Slave, tty.Slave)})
	defer mode.Stop()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- mode.Run(ctx) }()
	tty.Wait(t, "> ")
	if err := os.WriteFile(path, []byte("invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	tty.Send(t, "/settings\r")
	tty.Wait(t, "Thinking level")
	tty.Send(t, "theme\r")
	tty.Wait(t, "Select a theme")
	tty.Send(t, "\x1b[B\x1b[B")
	waitSessionScreen118(t, tty, "Failed to load theme", true)
	tty.Send(t, "\x1b[27u")
	waitSessionScreen118(t, tty, "Type to search", true)
	if setting, _ := runtime.Session().SettingsManager().GetThemeSetting(); setting != "dark" {
		t.Fatal("failed preview persisted", setting)
	}
	tty.Send(t, "\x1b[27u")
	waitSessionScreen118(t, tty, "Type to search", false)
	time.Sleep(100 * time.Millisecond)
	tty.Send(t, "\x04")
	if err := awaitInteractive(t, done); err != nil {
		t.Fatal(err)
	}
}

func TestInteractiveThemeRenderAfterStop120(t *testing.T) {
	tty := terminaltest.Open(t)
	ui := tui.NewTextUI(tui.NewProcessTerminal(tty.Slave, tty.Slave))
	if err := ui.Refresh(); err == nil {
		t.Fatal("refresh before Start should fail")
	}
	if err := ui.Start(); err != nil {
		t.Fatal(err)
	}
	defer ui.Stop()
	if err := ui.Stop(); err != nil {
		t.Fatal(err)
	}
	if err := ui.Refresh(); err != nil {
		t.Fatal("pending render after successful Stop", err)
	}
	if !tty.Restored(t) {
		t.Fatal("terminal not restored")
	}
}

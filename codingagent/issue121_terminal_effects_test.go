//go:build darwin || linux

package codingagent_test

import (
	"context"
	"fmt"
	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
	"github.com/nankedr/pig/internal/terminaltest"
	"github.com/nankedr/pig/tui"
	"strings"
	"testing"
	"time"
)

func persistentSettingsRuntime121(t *testing.T, history bool) *codingagent.AgentSessionRuntime {
	t.Helper()
	cwd, dir := t.TempDir(), t.TempDir()
	manager, err := codingagent.NewSessionManager(cwd, &dir)
	if err != nil {
		t.Fatal(err)
	}
	if history {
		for i := 0; i < 20; i++ {
			if _, err = manager.AppendMessage(ai.UserMessage{Role: "user", Content: ai.UserText(fmt.Sprintf("USER_HISTORY_121_%02d", i))}); err != nil {
				t.Fatal(err)
			}
			if _, err = manager.AppendMessage(ai.AssistantMessage{Role: "assistant", Content: []ai.AssistantContent{ai.TextContent{Type: "text", Text: fmt.Sprintf("ASSISTANT_HISTORY_121_%02d", i)}}, StopReason: ai.StopReasonStop}); err != nil {
				t.Fatal(err)
			}
		}
	}
	settings, err := codingagent.NewSettingsManager(cwd, &dir)
	if err != nil {
		t.Fatal(err)
	}
	if err = settings.SetTheme("dark"); err != nil {
		t.Fatal(err)
	}
	p, _ := ai.NewFauxProvider()
	model, _ := p.GetModel()
	created, err := codingagent.CreateAgentSession(context.Background(), codingagent.CreateAgentSessionOptions{CWD: cwd, AgentDir: dir, SessionManager: manager, SettingsManager: settings, Model: &model, Provider: p.Provider, NoTools: codingagent.NoToolsAll})
	if err != nil {
		t.Fatal(err)
	}
	runtime := codingagent.NewAgentSessionRuntime(created.Session, codingagent.AgentSessionServices{AgentDir: dir}, nil, nil, nil)
	t.Cleanup(func() { runtime.Dispose(context.Background()) })
	return runtime
}
func TestInteractiveSettingsStartupLayoutAndExit121(t *testing.T) {
	for _, tc := range []struct {
		name            string
		history, quiet  bool
		scrollbar, exit string
	}{
		{"unpersisted", false, false, "hidden", "resume-hint"}, {"persisted-hint", true, true, "always", "resume-hint"}, {"transcript", true, false, "hidden", "transcript"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runtime := persistentSettingsRuntime121(t, tc.history)
			for _, pair := range [][2]string{{"quiet-startup", fmt.Sprint(tc.quiet)}, {"tui-mode", "fullscreen"}, {"fullscreen-scrollbar", tc.scrollbar}, {"fullscreen-exit-output", tc.exit}} {
				if err := runtime.Session().UpdateInteractionSetting(pair[0], pair[1]); err != nil {
					t.Fatal(err)
				}
			}
			tty := terminaltest.Open(t)
			mode := codingagent.NewInteractiveMode(runtime, codingagent.InteractiveModeOptions{Terminal: tui.NewProcessTerminal(tty.Slave, tty.Slave)})
			defer mode.Stop()
			if err := mode.Init(context.Background()); err != nil {
				t.Fatal(err)
			}
			if err := mode.RenderInitialMessages(); err != nil {
				t.Fatal(err)
			}
			tty.Wait(t, "\x1b[?1049h")
			if tc.history {
				tty.Wait(t, "ASSISTANT_HISTORY_121_19")
			} else {
				tty.Wait(t, "> ")
			}
			time.Sleep(30 * time.Millisecond)
			if strings.Contains(tty.Output(), "pig · /settings") == tc.quiet {
				t.Fatal("quiet startup ignored")
			}
			if strings.Contains(tty.Output(), "\x1b[100m") != (tc.scrollbar == "always") {
				t.Fatal("fullscreen scrollbar ignored")
			}
			before := len(tty.Output())
			if err := mode.Stop(); err != nil {
				t.Fatal(err)
			}
			time.Sleep(30 * time.Millisecond)
			tail := tty.Output()[before:]
			if strings.Contains(tail, "Resume this session with:") != (tc.history && tc.exit == "resume-hint") {
				t.Fatal("invalid resume hint", tail)
			}
			if strings.Contains(tail, "ASSISTANT_HISTORY_121_00") != (tc.history && tc.exit == "transcript") {
				t.Fatal("exit output ignored", tail)
			}
			if !tty.Restored(t) {
				t.Fatal("terminal not restored")
			}
		})
	}
}
func TestInteractiveSettingsClearOnShrink121(t *testing.T) {
	for _, enabled := range []string{"false", "true"} {
		t.Run(enabled, func(t *testing.T) {
			runtime := persistentSettingsRuntime121(t, false)
			if err := runtime.Session().UpdateInteractionSetting("clear-on-shrink", enabled); err != nil {
				t.Fatal(err)
			}
			tty := terminaltest.Open(t)
			mode := codingagent.NewInteractiveMode(runtime, codingagent.InteractiveModeOptions{Terminal: tui.NewProcessTerminal(tty.Slave, tty.Slave)})
			defer mode.Stop()
			if err := mode.Init(context.Background()); err != nil {
				t.Fatal(err)
			}
			tty.Send(t, "\x1b[200~first\nsecond\nLAST_DRAFT_121\x1b[201~")
			waitSessionScreen118(t, tty, "LAST_DRAFT_121", true)
			before := len(tty.Output())
			if err := mode.ClearEditor(); err != nil {
				t.Fatal(err)
			}
			waitSessionScreen118(t, tty, "LAST_DRAFT_121", false)
			if strings.Contains(tty.Output()[before:], "\x1b[2J") != (enabled == "true") {
				t.Fatal("clear-on-shrink ignored")
			}
		})
	}
}
func TestInteractiveSettingsTreeFilter121(t *testing.T) {
	for _, filter := range []string{"all", "user-only"} {
		t.Run(filter, func(t *testing.T) {
			runtime := persistentSettingsRuntime121(t, true)
			if err := runtime.Session().UpdateInteractionSetting("tree-filter-mode", filter); err != nil {
				t.Fatal(err)
			}
			tty := terminaltest.Open(t)
			mode := codingagent.NewInteractiveMode(runtime, codingagent.InteractiveModeOptions{Terminal: tui.NewProcessTerminal(tty.Slave, tty.Slave)})
			defer mode.Stop()
			ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- mode.Run(ctx) }()
			tty.Wait(t, "> ")
			tty.Send(t, "/tree\r")
			waitSessionScreen118(t, tty, "Session Tree", true)
			time.Sleep(100 * time.Millisecond)
			screen := tty.ScreenText()
			at := strings.LastIndex(screen, "Session Tree")
			if at < 0 {
				t.Fatal(screen)
			}
			selection := screen[at:]
			if strings.Contains(selection, "assistant: ASSISTANT_HISTORY_121_") != (filter == "all") {
				t.Fatal("tree filter ignored", selection)
			}
			tty.Send(t, "\x1b[27u")
			waitSessionScreen118(t, tty, "Session Tree", false)
			tty.Send(t, "\x04")
			if err := awaitInteractive(t, done); err != nil {
				t.Fatal(err)
			}
		})
	}
}

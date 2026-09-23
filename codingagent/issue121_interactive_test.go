//go:build darwin || linux

package codingagent_test

import (
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

func settingMenu121(t *testing.T, tty *terminaltest.Terminal, label string) {
	t.Helper()
	tty.Send(t, "/settings\r")
	waitSessionScreen118(t, tty, "Type to search", true)
	tty.Send(t, label)
	waitSessionScreen118(t, tty, "> "+label, true)
	tty.Send(t, "\r")
	time.Sleep(80 * time.Millisecond)
	tty.Send(t, "\x1b[27u")
	waitSessionScreen118(t, tty, "Type to search", false)
}
func TestInteractiveSettingsEffects121(t *testing.T) {
	runtime := interactiveRuntime(t)
	session := runtime.Session()
	if err := session.SettingsManager().SetTheme("dark"); err != nil {
		t.Fatal(err)
	}
	message := ai.AssistantMessage{Role: "assistant", Content: []ai.AssistantContent{ai.ThinkingContent{Type: ai.ContentTypeThinking, Thinking: "THINK_SECRET_121"}, ai.TextContent{Type: ai.ContentTypeText, Text: "VISIBLE_REPLY_121"}}, StopReason: ai.StopReasonStop}
	if _, err := session.SessionManager().AppendMessage(message); err != nil {
		t.Fatal(err)
	}
	tty := terminaltest.Open(t)
	mode := codingagent.NewInteractiveMode(runtime, codingagent.InteractiveModeOptions{Terminal: tui.NewProcessTerminal(tty.Slave, tty.Slave)})
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	defer mode.Stop()
	done := make(chan error, 1)
	go func() { done <- mode.Run(ctx) }()
	waitSessionScreen118(t, tty, "THINK_SECRET_121", true)
	settingMenu121(t, tty, "Hide thinking")
	waitSessionScreen118(t, tty, "THINK_SECRET_121", false)
	waitSessionScreen118(t, tty, "VISIBLE_REPLY_121", true)
	tty.Send(t, "\x14")
	waitSessionScreen118(t, tty, "THINK_SECRET_121", true)
	tty.Send(t, "/settings\r")
	waitSessionScreen118(t, tty, "Type to search", true)
	tty.Send(t, "Hide thinking")
	waitSessionScreen118(t, tty, "false", true)
	tty.Send(t, "\x1b[27u")
	waitSessionScreen118(t, tty, "Type to search", false)
	settingMenu121(t, tty, "Output padding")
	if !strings.Contains(tty.ScreenText(), "\nVISIBLE_REPLY_121") {
		t.Fatal("output padding was not applied", tty.ScreenText())
	}
	settingMenu121(t, tty, "Editor padding")
	tty.Send(t, "DRAFT_121")
	waitSessionScreen118(t, tty, ">  DRAFT_121", true)
	tty.Send(t, "\x03")
	waitSessionScreen118(t, tty, "DRAFT_121", false)
	settingMenu121(t, tty, "Show hardware cursor")
	if !tty.CursorVisible() {
		t.Fatal("hardware cursor did not become visible")
	}
	settingMenu121(t, tty, "Terminal progress")
	tty.Send(t, "prompt\r")
	tty.Wait(t, "\x1b]9;4;3;\x07")
	tty.Wait(t, "FIRST_DONE")
	tty.Wait(t, "\x1b]9;4;0;\x07")
	if err := session.WaitForIdle(ctx); err != nil {
		t.Fatal(err)
	}
	settingMenu121(t, tty, "Steering mode")
	if session.SteeringMode() != "all" {
		t.Fatal("queue delivery mode not applied")
	}
	settingMenu121(t, tty, "Follow-up mode")
	if session.FollowUpMode() != "all" {
		t.Fatal("follow-up delivery mode not applied")
	}
	settingMenu121(t, tty, "Auto-compact")
	if session.AutoCompactionEnabled() {
		t.Fatal("compaction still enabled")
	}
	settingMenu121(t, tty, "Double-escape action")
	tty.Send(t, "\x1b[27u")
	time.Sleep(80 * time.Millisecond)
	tty.Send(t, "\x1b[27u")
	waitSessionScreen118(t, tty, "Fork from Message", true)
	tty.Send(t, "\x1b[27u")
	waitSessionScreen118(t, tty, "Fork from Message", false)
	tty.Send(t, "\x04")
	if err := awaitInteractive(t, done); err != nil {
		t.Fatal(err)
	}
	if !tty.Restored(t) {
		t.Fatal("terminal not restored")
	}
}
func TestInteractiveSettingsFailureAndBindings121(t *testing.T) {
	cwd, dir := t.TempDir(), t.TempDir()
	session := settingsSession121(t, cwd, dir, nil)
	runtime := codingagent.NewAgentSessionRuntime(session, codingagent.AgentSessionServices{AgentDir: dir}, nil, nil, nil)
	bindings, _ := codingagent.NewKeybindingsManager(dir)
	if err := bindings.SetUserBindings(tui.KeybindingsConfig{"tui.select.confirm": {"ctrl+y"}, "tui.select.cancel": {"ctrl+x"}, "app.message.followUp": {"ctrl+g"}, "app.interrupt": {"ctrl+b"}, "tui.input.submit": {"f2"}}); err != nil {
		t.Fatal(err)
	}
	tty := terminaltest.Open(t)
	mode := codingagent.NewInteractiveMode(runtime, codingagent.InteractiveModeOptions{Terminal: tui.NewProcessTerminal(tty.Slave, tty.Slave), Keybindings: bindings})
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	defer mode.Stop()
	done := make(chan error, 1)
	go func() { done <- mode.Run(ctx) }()
	tty.Wait(t, "> ")
	tty.Send(t, "/settings\x1bOQ")
	waitSessionScreen118(t, tty, "ctrl+y/Space", true)
	waitSessionScreen118(t, tty, "ctrl+x to cancel", true)
	tty.Send(t, "Steering mode")
	waitSessionScreen118(t, tty, "f2 while streaming", true)
	tty.Send(t, "\x15Double-escape action")
	waitSessionScreen118(t, tty, "ctrl+b twice", true)
	tty.Send(t, "\x15Follow-up mode")
	waitSessionScreen118(t, tty, "ctrl+g queues follow-up", true)
	path := filepath.Join(dir, "settings.json")
	_ = os.Remove(path)
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	tty.Send(t, "\x19")
	waitSessionScreen118(t, tty, "Setting not saved", true)
	if session.FollowUpMode() != "one-at-a-time" {
		t.Fatal("failed save changed running session")
	}
	tty.Send(t, "\x18")
	waitSessionScreen118(t, tty, "Type to search", false)
	tty.Send(t, "/hotkeys\x1bOQ")
	tty.Wait(t, "ctrl+g:")
	tty.Wait(t, "Steering: one-at-a-time · Follow-up: one-at-a-time")
	tty.Send(t, "\x04")
	if err := awaitInteractive(t, done); err != nil {
		t.Fatal(err)
	}
}

func TestInteractiveSettingsSkillCompletion121(t *testing.T) {
	cwd, dir := t.TempDir(), t.TempDir()
	for _, name := range []string{"one", "two", "three", "four", "five", "six", "seven"} {
		path := filepath.Join(dir, "skills", name)
		if err := os.MkdirAll(path, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, "SKILL.md"), []byte("---\nname: "+name+"\ndescription: SKILL_DESCRIPTION_121\n---\nDo the task."), 0600); err != nil {
			t.Fatal(err)
		}
	}
	session := settingsSession121(t, cwd, dir, nil)
	runtime := codingagent.NewAgentSessionRuntime(session, codingagent.AgentSessionServices{AgentDir: dir}, nil, nil, nil)
	tty := terminaltest.Open(t)
	mode := codingagent.NewInteractiveMode(runtime, codingagent.InteractiveModeOptions{Terminal: tui.NewProcessTerminal(tty.Slave, tty.Slave)})
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	defer mode.Stop()
	done := make(chan error, 1)
	go func() { done <- mode.Run(ctx) }()
	tty.Wait(t, "> ")
	tty.Send(t, "/skill:")
	waitSessionScreen118(t, tty, "SKILL_DESCRIPTION_121", true)
	waitCompletionCount121(t, tty, 5)
	tty.Send(t, "\x15")
	waitSessionScreen118(t, tty, "SKILL_DESCRIPTION_121", false)
	settingMenu121(t, tty, "Autocomplete max items")
	tty.Send(t, "/skill:")
	waitSessionScreen118(t, tty, "SKILL_DESCRIPTION_121", true)
	waitCompletionCount121(t, tty, 7)
	tty.Send(t, "\x15")
	waitSessionScreen118(t, tty, "SKILL_DESCRIPTION_121", false)
	settingMenu121(t, tty, "Skill commands")
	tty.Send(t, "/skill:")
	time.Sleep(100 * time.Millisecond)
	if strings.Contains(tty.ScreenText(), "SKILL_DESCRIPTION_121") {
		t.Fatal("disabled skill commands still offered")
	}
	tty.Send(t, "\x15")
	time.Sleep(100 * time.Millisecond)
	tty.Send(t, "\x04")
	if err := awaitInteractive(t, done); err != nil {
		t.Fatal(err)
	}
}

func waitCompletionCount121(t *testing.T, tty *terminaltest.Terminal, count int) {
	t.Helper()
	end := time.Now().Add(5 * time.Second)
	for strings.Count(tty.ScreenText(), "SKILL_DESCRIPTION_121") != count {
		if time.Now().After(end) {
			t.Fatal("completion count", count, tty.ScreenText())
		}
		time.Sleep(5 * time.Millisecond)
	}
}

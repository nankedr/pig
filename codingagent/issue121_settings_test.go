package codingagent_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
)

func settingsSession121(t *testing.T, cwd, dir string, settings *codingagent.SettingsManager) *codingagent.AgentSession {
	t.Helper()
	if settings == nil {
		var err error
		settings, err = codingagent.NewSettingsManager(cwd, &dir)
		if err != nil {
			t.Fatal(err)
		}
	}
	p, err := ai.NewFauxProvider()
	if err != nil {
		t.Fatal(err)
	}
	model, _ := p.GetModel()
	created, err := codingagent.CreateAgentSession(context.Background(), codingagent.CreateAgentSessionOptions{CWD: cwd, AgentDir: dir, Model: &model, Provider: p.Provider, NoTools: codingagent.NoToolsAll, SettingsManager: settings})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { created.Session.Dispose() })
	return created.Session
}
func TestInteractionSettingsPersistence121(t *testing.T) {
	cwd, dir := t.TempDir(), t.TempDir()
	session := settingsSession121(t, cwd, dir, nil)
	changes := []struct{ ID, Value string }{
		{"autocompact", "false"}, {"skill-commands", "false"}, {"show-hardware-cursor", "true"}, {"editor-padding", "3"}, {"output-padding", "0"}, {"autocomplete-max-visible", "20"}, {"clear-on-shrink", "true"}, {"terminal-progress", "true"}, {"steering-mode", "all"}, {"follow-up-mode", "all"}, {"hide-thinking", "true"}, {"quiet-startup", "true"}, {"default-project-trust", "Never trust"}, {"double-escape-action", "fork"}, {"tree-filter-mode", "user-only"}, {"tui-mode", "fullscreen"}, {"fullscreen-exit-output", "resume-hint"}, {"fullscreen-scrollbar", "hidden"}}
	for _, change := range changes {
		if err := session.UpdateInteractionSetting(change.ID, change.Value); err != nil {
			t.Fatal(change, err)
		}
	}
	c, err := session.GetInteractionSettings()
	if err != nil {
		t.Fatal(err)
	}
	if c.AutoCompact || c.EnableSkillCommands || !c.ShowHardwareCursor || c.EditorPaddingX != 3 || c.OutputPad != 0 || c.AutocompleteMaxVisible != 20 || !c.ClearOnShrink || !c.ShowTerminalProgress || c.SteeringMode != "all" || c.FollowUpMode != "all" || !c.HideThinkingBlock || !c.QuietStartup || c.DefaultProjectTrust != "never" || c.DoubleEscapeAction != "fork" || c.TreeFilterMode != "user-only" || c.TUIMode != "fullscreen" || c.FullscreenExitOutput != "resume-hint" || c.FullscreenScrollbar != "hidden" {
		t.Fatalf("effective: %+v", c)
	}
	if session.SteeringMode() != "all" || session.FollowUpMode() != "all" || session.AutoCompactionEnabled() {
		t.Fatal("Session still uses previous configuration")
	}
	restored := settingsSession121(t, cwd, dir, nil)
	got, err := restored.GetInteractionSettings()
	if err != nil || !reflect.DeepEqual(got, c) {
		t.Fatalf("restored=%+v want=%+v err=%v", got, c, err)
	}
	for _, change := range []struct{ ID, Value string }{{"editor-padding", "99"}, {"transport", "websocket"}, {"show-images", "true"}, {"thinking", "open"}} {
		if err := restored.UpdateInteractionSetting(change.ID, change.Value); err == nil {
			t.Fatal("accepted invalid/deferred setting", change)
		}
	}
	after, _ := restored.GetInteractionSettings()
	if !reflect.DeepEqual(after, got) {
		t.Fatal("invalid value changed settings")
	}
}
func TestInteractionSettingsTrustAndFailure121(t *testing.T) {
	cwd, dir := t.TempDir(), t.TempDir()
	if err := os.Mkdir(filepath.Join(cwd, ".pig"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cwd, ".pig/settings.json"), []byte(`{"steeringMode":"one-at-a-time","editorPaddingX":2}`), 0600); err != nil {
		t.Fatal(err)
	}
	settings, err := codingagent.NewSettingsManager(cwd, &dir)
	if err != nil {
		t.Fatal(err)
	}
	s := settingsSession121(t, cwd, dir, settings)
	if err = s.UpdateInteractionSetting("editor-padding", "3"); err != nil {
		t.Fatal(err)
	}
	c, _ := s.GetInteractionSettings()
	if c.EditorPaddingX != 3 {
		t.Fatal("untrusted project was read")
	}
	if err = settings.SetProjectTrusted(true); err != nil {
		t.Fatal(err)
	}
	if err = s.UpdateInteractionSetting("steering-mode", "all"); err != nil {
		t.Fatal(err)
	}
	c, _ = s.GetInteractionSettings()
	if c.SteeringMode != "one-at-a-time" || c.EditorPaddingX != 2 || s.SteeringMode() != "one-at-a-time" {
		t.Fatal("project precedence", c)
	}
	global, _ := settings.GetGlobalSettings()
	if *global.SteeringMode != "all" {
		t.Fatal("global choice not saved")
	}
	before, _ := s.GetInteractionSettings()
	path := filepath.Join(dir, "settings.json")
	if err = os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err = s.UpdateInteractionSetting("hide-thinking", "true"); err == nil {
		t.Fatal("save failure reported success")
	}
	after, _ := s.GetInteractionSettings()
	if !reflect.DeepEqual(before, after) {
		t.Fatal("failed save published new config")
	}
}
func TestInteractionSettingsPreserveUnknownAndNested121(t *testing.T) {
	cwd, dir := t.TempDir(), t.TempDir()
	path := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(path, []byte(`{"compaction":{"enabled":true,"reserveTokens":12345},"terminal":{"future":17},"future":{"option":true}}`), 0600); err != nil {
		t.Fatal(err)
	}
	s := settingsSession121(t, cwd, dir, nil)
	for _, pair := range [][2]string{{"autocompact", "false"}, {"clear-on-shrink", "true"}} {
		if err := s.UpdateInteractionSetting(pair[0], pair[1]); err != nil {
			t.Fatal(err)
		}
	}
	data, _ := os.ReadFile(path)
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got["compaction"].(map[string]any)["reserveTokens"] != float64(12345) || got["terminal"].(map[string]any)["future"] != float64(17) || got["future"].(map[string]any)["option"] != true {
		t.Fatal(string(data))
	}
}

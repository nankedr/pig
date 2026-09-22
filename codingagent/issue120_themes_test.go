package codingagent_test

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
	"github.com/nankedr/pig/tui"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestThemeRuntimeParity120(t *testing.T) {
	var fixture struct {
		Case struct {
			Input struct {
				Reports, Env []string
				Keys         [][]string
			}
		}
		Observation struct {
			Outcome struct {
				Reports []struct {
					RGB    *tui.RGBColor
					Scheme tui.TerminalColorScheme
				}
				Env       []string
				Selectors [][]string
			}
		}
	}
	data, err := os.ReadFile("../parity/oracle/fixtures/theme-runtime.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	for i, s := range fixture.Case.Input.Reports {
		rgb, ok, err := tui.ParseOSC11BackgroundColor(s)
		want := fixture.Observation.Outcome.Reports[i]
		if err != nil || ok != (want.RGB != nil) || ok && rgb != *want.RGB {
			t.Errorf("report %q: %v %v %v", s, rgb, ok, err)
		}
		scheme, _, err := tui.ParseTerminalColorSchemeReport(s)
		if err != nil || scheme != want.Scheme {
			t.Errorf("scheme %q: %s %v", s, scheme, err)
		}
	}
	for i, value := range fixture.Case.Input.Env {
		scheme, _ := codingagent.DetectTerminalTheme(value)
		if string(scheme) != fixture.Observation.Outcome.Env[i] {
			t.Errorf("COLORFGBG %q: %s", value, scheme)
		}
	}
	for i, keys := range fixture.Case.Input.Keys {
		events := []string{}
		selector := codingagent.NewThemeSelectorComponent("dark", codingagent.ThemeLoadResult{}, func(v string) { events = append(events, "select:"+v) }, func() { events = append(events, "cancel") }, func(v string) { events = append(events, "preview:"+v) })
		for _, k := range keys {
			if err := selector.HandleInput(k); err != nil {
				t.Fatal(err)
			}
		}
		if !reflect.DeepEqual(events, fixture.Observation.Outcome.Selectors[i]) {
			t.Fatalf("%v: %v", keys, events)
		}
	}
}

func TestThemeControllerReload120(t *testing.T) {
	data, err := os.ReadFile("themes/dark.json")
	if err != nil {
		t.Fatal(err)
	}
	path := t.TempDir() + "/local.json"
	data = bytes.Replace(data, []byte(`"name": "dark"`), []byte(`"name": "local"`), 1)
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	local, err := codingagent.LoadThemeFromPath(path)
	if err != nil {
		t.Fatal(err)
	}
	setting := "local"
	settings, _ := codingagent.NewInMemorySettingsManager(codingagent.Settings{Theme: &setting})
	c := codingagent.NewThemeController(settings, codingagent.ThemeLoadResult{Themes: []*codingagent.Theme{local}})
	if err = c.ApplySettings(); err != nil {
		t.Fatal(err)
	}
	original := c.Current().FG("accent", "X")
	if err = c.Preview("light"); err != nil {
		t.Fatal(err)
	}
	if saved, _ := settings.GetThemeSetting(); saved != "local" {
		t.Fatal("preview persisted", saved)
	}
	if err = c.ApplySettings(); err != nil || c.Current().Name != "local" {
		t.Fatal("cancel", err)
	}
	var doc map[string]any
	json.Unmarshal(data, &doc)
	doc["colors"].(map[string]any)["accent"] = "#010203"
	changed, _ := json.Marshal(doc)
	os.WriteFile(path, changed, 0600)
	if updated, err := c.Refresh(); err != nil || !updated || c.Current().FG("accent", "X") == original {
		t.Fatal("reload", updated, err)
	}
	good := c.Current().FG("accent", "X")
	if err = os.Chmod(path, 0); err != nil {
		t.Fatal(err)
	}
	if _, readErr := os.ReadFile(path); readErr != nil {
		if _, err = c.Refresh(); err == nil || c.Current().FG("accent", "X") != good {
			t.Fatal("unreadable fallback", err)
		}
	}
	if err = os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = c.Refresh(); err != nil {
		t.Fatal("readability recovery", err)
	}

	os.WriteFile(path, []byte("invalid"), 0600)
	if _, err = c.Refresh(); err == nil || c.Current().FG("accent", "X") != good {
		t.Fatal("invalid fallback", err)
	}
	os.Remove(path)
	if _, err = c.Refresh(); err == nil || c.Current().FG("accent", "X") != good {
		t.Fatal("deleted fallback", err)
	}
	os.WriteFile(path, changed, 0600)
	if _, err = c.Refresh(); err != nil {
		t.Fatal("recovery", err)
	}
	if err = c.SetTheme("light"); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(path, data, 0600)
	if err = c.SetTheme("local"); err != nil || c.Current().FG("accent", "X") != original {
		t.Fatal("inactive theme edit was lost", err)
	}
	if err = c.SetTheme("light"); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(path, []byte("invalid"), 0600)
	if _, err = c.Refresh(); err != nil || c.Current().Name != "light" {
		t.Fatal("stale watcher", err)
	}
	if saved, _ := settings.GetThemeSetting(); saved != "light" {
		t.Fatal("confirm not saved")
	}
	if err = c.SetTheme("missing"); err == nil || c.Current().Name != "dark" {
		t.Fatal("missing fallback", err)
	}
}

func TestThemeAutoAndWatchLifecycle120(t *testing.T) {
	setting := "light/dark"
	settings, _ := codingagent.NewInMemorySettingsManager(codingagent.Settings{Theme: &setting})
	c := codingagent.NewThemeController(settings, codingagent.ThemeLoadResult{})
	if err := c.ApplySettings(); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		for _, scheme := range []tui.TerminalColorScheme{tui.TerminalColorSchemeLight, tui.TerminalColorSchemeDark} {
			if err := c.SetTerminalTheme(scheme); err != nil || c.Current().Name != string(scheme) {
				t.Fatal("auto", err)
			}
		}
	}
	if err := c.Preview("light"); err != nil {
		t.Fatal(err)
	}
	c.SetTerminalTheme(tui.TerminalColorSchemeDark)
	if c.Current().Name != "light" {
		t.Fatal("notification overwrote preview")
	}
	if err := c.ApplySettings(); err != nil || c.Current().Name != "dark" {
		t.Fatal("cancel did not restore auto")
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { c.Watch(ctx, func(err error) { t.Error("unexpected callback", err) }); close(done) }()
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("watch did not release timer")
	}
	if err := c.SetTheme("light"); err != nil {
		t.Fatal(err)
	}
	c.SetTerminalTheme(tui.TerminalColorSchemeDark)
	if c.Current().Name != "light" {
		t.Fatal("fixed theme followed notification")
	}
}

func TestThemeTranscriptAndResourceReplacement120(t *testing.T) {
	dark, _ := codingagent.LoadBuiltinTheme("dark", codingagent.ColorModeTrueColor)
	light, _ := codingagent.LoadBuiltinTheme("light", codingagent.ColorModeTrueColor)
	transcript := codingagent.NewTranscript()
	assistant := ai.AssistantMessage{Role: ai.MessageRoleAssistant, Content: []ai.AssistantContent{ai.TextContent{Type: ai.ContentTypeText, Text: "# OLD_MESSAGE"}, ai.ToolCall{Type: ai.ContentTypeToolCall, ID: "call", Name: "read", Arguments: map[string]any{"path": "file"}}}}
	messages := []agent.AgentMessage{ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText("question")}, assistant, ai.ToolResultMessage{Role: ai.MessageRoleToolResult, ToolCallID: "call", ToolName: "read", Content: []ai.ToolResultContent{ai.TextContent{Type: ai.ContentTypeText, Text: "TOOL_OUTPUT"}}}}
	if err := transcript.SetMessages(messages); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		theme       *codingagent.Theme
		heading, bg string
	}{{dark, "\x1b[38;2;240;198;116m", "\x1b[48;2;40;50;40m"}, {light, "\x1b[38;2;154;115;38m", "\x1b[48;2;232;240;232m"}, {dark, "\x1b[38;2;240;198;116m", "\x1b[48;2;40;50;40m"}} {
		transcript.SetTheme(tc.theme)
		lines, err := transcript.Render(80)
		if err != nil {
			t.Fatal(err)
		}
		text := strings.Join(lines, "\n")
		if !strings.Contains(text, tc.heading) || !strings.Contains(text, tc.bg) || !strings.Contains(text, "TOOL_OUTPUT") || !strings.Contains(text, "OLD_MESSAGE") {
			t.Fatal("inconsistent transcript theme", text)
		}
	}
	settings, _ := codingagent.NewInMemorySettingsManager(codingagent.Settings{})
	c := codingagent.NewThemeController(settings, codingagent.ThemeLoadResult{})
	if err := c.SetTheme("dark"); err != nil {
		t.Fatal(err)
	}
	next := "light"
	replacement, _ := codingagent.NewInMemorySettingsManager(codingagent.Settings{Theme: &next})
	if err := c.ReplaceResources(replacement, codingagent.ThemeLoadResult{}); err != nil {
		t.Fatal(err)
	}
	if c.Current().Name != "light" {
		t.Fatal("replacement kept old settings")
	}
	if err := c.SetTheme("dark"); err != nil {
		t.Fatal(err)
	}
	if got, _ := replacement.GetThemeSetting(); got != "dark" {
		t.Fatal("replacement save used old manager")
	}
}

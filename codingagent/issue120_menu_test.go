package codingagent_test

import (
	"encoding/json"
	"github.com/nankedr/pig/codingagent"
	"github.com/nankedr/pig/tui"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestThemeSettingsParity120(t *testing.T) {
	data, err := os.ReadFile("../parity/oracle/fixtures/theme-runtime.json")
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Case        struct{ Input struct{ MenuKeys [][]string } }
		Observation struct {
			Outcome struct {
				Menus []struct {
					Events  []string
					Screens [][]string
				}
			}
		}
	}
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatal(err)
	}
	theme, _ := codingagent.LoadBuiltinTheme("dark")
	for i, keys := range f.Case.Input.MenuKeys {
		events := []string{}
		menu := codingagent.NewThemeSettingsComponent("dark", tui.TerminalColorSchemeDark, codingagent.ThemeLoadResult{}, codingagent.SettingsCallbacks{OnThemePreview: func(s string) { events = append(events, "preview:"+s) }, OnThemeChange: func(s string) { events = append(events, "select:"+s) }, OnCancel: func() {}}, func() *codingagent.Theme { return theme })
		for j, key := range keys {
			lines, err := menu.Render(80)
			if err != nil {
				t.Fatal(err)
			}
			for k, s := range lines {
				plain, _ := tui.StripTerminalSequences(s)
				lines[k] = strings.TrimRight(plain, " ")
			}
			if !reflect.DeepEqual(lines, f.Observation.Outcome.Menus[i].Screens[j]) {
				t.Fatalf("case %d screen %d\ngot %q\nwant %q", i, j, lines, f.Observation.Outcome.Menus[i].Screens[j])
			}
			if err := menu.HandleInput(key); err != nil {
				t.Fatal(err)
			}
		}
		if !reflect.DeepEqual(events, f.Observation.Outcome.Menus[i].Events) {
			t.Fatalf("case %d events %v want %v", i, events, f.Observation.Outcome.Menus[i].Events)
		}
	}
}

func TestThemeSettingsSearch120(t *testing.T) {
	data, err := os.ReadFile("../parity/oracle/fixtures/theme-runtime.json")
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Case struct {
			Input struct{ SettingsKeys []string }
		}
		Observation struct {
			Outcome struct {
				Settings struct {
					Events  []string
					Screens [][]string
				}
			}
		}
	}
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatal(err)
	}
	theme, _ := codingagent.GetSettingsListTheme()
	search := true
	first, second := "Color theme for the interface", "Theme for light appearance"
	events := []string{}
	list := tui.NewSettingsList([]tui.SettingItem{{ID: "theme", Label: "Theme", Description: &first, CurrentValue: "dark", Values: []string{"dark", "light"}}, {ID: "light", Label: "Light theme", Description: &second, CurrentValue: "light", Values: []string{"dark", "light"}}}, 10, theme, func(id, value string) { events = append(events, id+":"+value) }, func() { events = append(events, "cancel") }, tui.SettingsListOptions{EnableSearch: &search})
	for i, want := range f.Observation.Outcome.Settings.Screens {
		got, err := list.Render(80)
		if err != nil {
			t.Fatal(err)
		}
		for k, line := range got {
			plain, _ := tui.StripTerminalSequences(line)
			got[k] = strings.TrimRight(plain, " ")
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("step %d got %q want %q", i, got, want)
		}
		if i < len(f.Case.Input.SettingsKeys) {
			if err := list.HandleInput(f.Case.Input.SettingsKeys[i]); err != nil {
				t.Fatal(err)
			}
		}
	}
	if !reflect.DeepEqual(events, f.Observation.Outcome.Settings.Events) {
		t.Fatalf("events %v want %v", events, f.Observation.Outcome.Settings.Events)
	}
}

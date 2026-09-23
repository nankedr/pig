package codingagent_test

import (
	"encoding/json"
	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/codingagent"
	"github.com/nankedr/pig/tui"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestSettingsSelectorParity121(t *testing.T) {
	data, err := os.ReadFile("../parity/oracle/fixtures/settings-selector.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Case struct {
			Input struct {
				Cases []struct {
					Label string
					Keys  []string
				}
			}
		}
		Observation struct {
			Outcome []struct {
				Label  string
				Events []struct {
					Callback string
					Value    any
				}
				Cancelled bool
			}
		}
	}
	if err = json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	for i, tc := range fixture.Case.Input.Cases {
		t.Run(tc.Label, func(t *testing.T) {
			config := codingagent.SettingsConfig{AutoCompact: true, EnableSkillCommands: true, SteeringMode: "one-at-a-time", FollowUpMode: "one-at-a-time", OutputPad: 1, AutocompleteMaxVisible: 5, DefaultProjectTrust: "ask", DoubleEscapeAction: "tree", TreeFilterMode: "default", ThinkingLevel: "off", AvailableThinkingLevels: []agent.ThinkingLevel{"off", "low", "high"}, TUIMode: "regular", FullscreenExitOutput: "transcript", FullscreenScrollbar: "auto", CurrentTheme: "dark", TerminalTheme: "dark", AvailableThemes: []string{"dark", "light"}}
			callbacks := codingagent.SettingsCallbacks{}
			var got []struct {
				Callback string
				Value    any
			}
			cancelled := false
			rv := reflect.ValueOf(&callbacks).Elem()
			for j := 0; j < rv.NumField(); j++ {
				field := rv.Field(j)
				name := rv.Type().Field(j).Name
				name = strings.ToLower(name[:1]) + name[1:]
				if name == "onTUIModeChange" {
					name = "onTuiModeChange"
				}
				field.Set(reflect.MakeFunc(field.Type(), func(args []reflect.Value) []reflect.Value {
					if name == "onCancel" {
						cancelled = true
					} else {
						raw, _ := json.Marshal(args[0].Interface())
						var value any
						json.Unmarshal(raw, &value)
						got = append(got, struct {
							Callback string
							Value    any
						}{name, value})
					}
					return nil
				}))
			}
			menu := codingagent.NewSettingsSelectorComponent(config, callbacks)
			for _, key := range tc.Keys {
				if err := menu.HandleInput(key); err != nil {
					t.Fatal(err)
				}
			}
			want := fixture.Observation.Outcome[i]
			if len(got) != len(want.Events) || len(got) > 0 && !reflect.DeepEqual(got, want.Events) || cancelled != want.Cancelled {
				t.Fatalf("events=%v cancelled=%v want=%+v", got, cancelled, want)
			}
		})
	}
}

func TestSettingsSelectorBindingHints121(t *testing.T) {
	kb, _ := codingagent.NewKeybindingsManager(t.TempDir())
	kb.SetUserBindings(tui.KeybindingsConfig{"tui.input.submit": {"f2"}, "app.interrupt": {"ctrl+b"}, "app.message.followUp": {"ctrl+g"}})
	for _, tc := range []struct{ label, hint string }{{"Steering mode", "f2 while streaming"}, {"Double-escape action", "ctrl+b twice"}, {"Follow-up mode", "ctrl+g queues follow-up"}} {
		menu := codingagent.NewSettingsSelectorComponent(codingagent.SettingsConfig{}, codingagent.SettingsCallbacks{})
		menu.SetKeybindings(&kb.KeybindingsManager)
		if err := menu.HandleInput(tc.label); err != nil {
			t.Fatal(err)
		}
		lines, err := menu.Render(100)
		if err != nil {
			t.Fatal(err)
		}
		plain, _ := tui.StripTerminalSequences(strings.Join(lines, "\n"))
		if !strings.Contains(plain, tc.hint) {
			t.Fatalf("missing %s in %s", tc.hint, plain)
		}
	}
}

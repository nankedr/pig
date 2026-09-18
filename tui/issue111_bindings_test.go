package tui_test

import (
	"encoding/json"
	"github.com/nankedr/pig/tui"
	"os"
	"reflect"
	"testing"
)

func TestKeybindingsParity111(t *testing.T) {
	data, err := os.ReadFile("../parity/oracle/fixtures/keys.json")
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Case struct {
			Input struct{ Bindings tui.KeybindingsConfig }
		}
		Observation struct {
			Outcome struct {
				Resolved  tui.KeybindingsConfig
				Conflicts []tui.KeybindingConflict
			}
		}
	}
	if err = json.Unmarshal(data, &f); err != nil {
		t.Fatal(err)
	}
	m := tui.NewKeybindingsManager(tui.NewTUIKeybindings(), f.Case.Input.Bindings)
	got, err := m.GetResolvedBindings()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, f.Observation.Outcome.Resolved) {
		t.Fatalf("resolved got %#v want %#v", got, f.Observation.Outcome.Resolved)
	}
	conflicts, err := m.GetConflicts()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(conflicts, f.Observation.Outcome.Conflicts) {
		t.Fatalf("conflicts %#v", conflicts)
	}
	got[tui.KeybindingInputSubmit][0] = "x"
	match, err := m.Matches("\x13", tui.KeybindingInputSubmit)
	if err != nil || !match {
		t.Fatalf("owned bindings: %v %v", match, err)
	}
	if err = m.SetUserBindings(tui.KeybindingsConfig{tui.KeybindingInputSubmit: {}}); err != nil {
		t.Fatal(err)
	}
	if match, _ = m.Matches("\r", tui.KeybindingInputSubmit); match {
		t.Fatal("disabled submit matched")
	}
}

func TestEditorKeybindings111(t *testing.T) {
	data, err := os.ReadFile("../parity/oracle/fixtures/keys.json")
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Case struct {
			Input struct {
				Editor struct {
					Config map[tui.Keybinding]json.RawMessage
					Steps  []string
				}
			}
		}
		Observation struct {
			Outcome struct {
				Editor struct{ States, Submits []string }
			}
		}
	}
	if err = json.Unmarshal(data, &f); err != nil {
		t.Fatal(err)
	}
	config := tui.KeybindingsConfig{}
	for k, v := range f.Case.Input.Editor.Config {
		var single string
		var keys []tui.KeyID
		if json.Unmarshal(v, &single) == nil {
			keys = []tui.KeyID{tui.KeyID(single)}
		} else if err := json.Unmarshal(v, &keys); err != nil {
			t.Fatal(err)
		}
		config[k] = keys
	}
	m := tui.NewKeybindingsManager(tui.NewTUIKeybindings(), config)
	previous, _ := tui.GetKeybindings()
	defer tui.SetKeybindings(previous)
	tui.SetKeybindings(m)
	e := tui.NewEditor(nil, tui.EditorTheme{})
	var submits []string
	e.OnSubmit = func(s string) { submits = append(submits, s); e.AddToHistory(s) }
	for i, s := range f.Case.Input.Editor.Steps {
		if err := e.HandleInput(s); err != nil {
			t.Fatal(err)
		}
		if got := e.GetText(); got != f.Observation.Outcome.Editor.States[i] {
			t.Fatalf("step %d: %q want %q", i, got, f.Observation.Outcome.Editor.States[i])
		}
	}
	if !reflect.DeepEqual(submits, f.Observation.Outcome.Editor.Submits) {
		t.Fatalf("submits %#v", submits)
	}
}

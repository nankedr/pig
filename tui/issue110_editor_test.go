package tui_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/nankedr/pig/internal/baseline"
	"github.com/nankedr/pig/internal/parity"
	"github.com/nankedr/pig/tui"
)

func TestEditorParity110(t *testing.T) {
	lock, _, err := baseline.Load("../parity/baseline")
	if err != nil {
		t.Fatal(err)
	}
	fixture, err := parity.LoadFixture("../parity/oracle/fixtures/editor.json", parity.Baseline{ID: lock.BaselineID, Commit: lock.Upstream.Commit, Repository: lock.Upstream.Repository})
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		ID    string
		Steps []struct {
			Op, Text string
			Width    int
			Value    bool
		}
	}
	if err := json.Unmarshal(fixture.Case.Input, &cases); err != nil {
		t.Fatal(err)
	}
	type state struct {
		Frame []string

		Text, Expanded string
		Cursor         tui.EditorCursor
	}
	type outcome struct {
		ID               string
		States           []state
		Changes, Submits []string
	}
	var want []outcome
	if err := json.Unmarshal(fixture.Observation.Outcome, &want); err != nil {
		t.Fatal(err)
	}
	for i, c := range cases {
		t.Run(c.ID, func(t *testing.T) {
			e := tui.NewEditor(nil, tui.EditorTheme{})
			got := outcome{ID: c.ID, States: []state{}, Changes: []string{}, Submits: []string{}}
			e.SetOnChange(func(s string) { got.Changes = append(got.Changes, s) })
			e.SetOnSubmit(func(s string) { got.Submits = append(got.Submits, s) })
			for j, s := range c.Steps {
				var err error
				var frame []string
				switch s.Op {
				case "set":
					err = e.SetText(s.Text)
				case "insert":
					err = e.InsertTextAtCursor(s.Text)
				case "input":
					err = e.HandleInput(s.Text)
				case "history":
					err = e.AddToHistory(s.Text)
				case "focus":
					e.SetFocusState(s.Value)
				case "disable":
					e.DisableSubmit = s.Value
				case "render":
					frame, err = e.Render(s.Width)
				}
				if err != nil {
					t.Fatalf("step %d: %v", j, err)
				}
				expanded, err := e.GetExpandedText()
				if err != nil {
					t.Fatal(err)
				}
				actual := state{frame, e.GetText(), expanded, e.GetCursor()}
				got.States = append(got.States, actual)
				if !reflect.DeepEqual(actual, want[i].States[j]) {
					t.Fatalf("step %d (%s %q): got %#v want %#v", j, s.Op, s.Text, actual, want[i].States[j])
				}
			}
			if !reflect.DeepEqual(got, want[i]) {
				t.Fatalf("callbacks: got %#v want %#v", got, want[i])
			}
		})
	}
}

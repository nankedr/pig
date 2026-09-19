package tui_test

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/nankedr/pig/internal/terminaltest"
	"github.com/nankedr/pig/tui"
	"io"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestSelectListParity114(t *testing.T) {
	data, err := os.ReadFile("../parity/oracle/fixtures/dialogs.json")
	if err != nil {
		t.Fatal(err)
	}
	type frame struct {
		Selected *string
		Lines    []string
	}
	var f struct {
		Case struct {
			Input struct {
				Items []tui.SelectItem
				Steps []struct {
					Key    *string
					Filter *string
					Index  *int
				}
				Width, MaxVisible int
			}
		}
		Observation struct {
			Outcome struct {
				Frames []frame
				Events []string
			}
		}
	}
	if err = json.Unmarshal(data, &f); err != nil {
		t.Fatal(err)
	}
	list := tui.NewSelectList(f.Case.Input.Items, f.Case.Input.MaxVisible, tui.SelectListTheme{})
	var events []string
	list.OnSelect = func(i tui.SelectItem) { events = append(events, "select:"+i.Value) }
	list.OnCancel = func() { events = append(events, "cancel") }
	list.OnSelectionChange = func(i tui.SelectItem) { events = append(events, "change:"+i.Value) }
	for i, step := range f.Case.Input.Steps {
		if step.Key != nil {
			err = list.HandleInput(*step.Key)
		}
		if step.Filter != nil {
			err = list.SetFilter(*step.Filter)
		}
		if step.Index != nil {
			err = list.SetSelectedIndex(*step.Index)
		}
		if err != nil {
			t.Fatal(err)
		}
		selected, err := list.GetSelectedItem()
		if err != nil {
			t.Fatal(err)
		}
		got := frame{}
		if selected != nil {
			got.Selected = &selected.Value
		}
		got.Lines, err = list.Render(f.Case.Input.Width)
		if err != nil || !reflect.DeepEqual(got, f.Observation.Outcome.Frames[i]) {
			t.Fatalf("step %d: %+v want %+v (%v)", i, got, f.Observation.Outcome.Frames[i], err)
		}
	}
	if !reflect.DeepEqual(events, f.Observation.Outcome.Events) {
		t.Fatalf("events %v", events)
	}
}

func TestOverlayFocusAndResize114(t *testing.T) {
	terminal := &terminal113{width: 20, height: 6}
	ui := tui.NewTUIMainScreen(terminal)
	base := tui.NewSelectList([]tui.SelectItem{{Value: "base"}}, 1, tui.SelectListTheme{})
	inputs := 0
	base.OnSelect = func(tui.SelectItem) { inputs++ }
	ui.AddChild(base)
	_ = ui.SetFocus(base)
	var handle tui.OverlayHandle
	dialog := tui.NewSelectDialog("Choose", []tui.SelectItem{{Value: "Yes"}, {Value: "No"}}, func(tui.SelectItem) { _ = handle.Hide() }, nil)
	width := 10
	anchor := tui.OverlayAnchorTopRight
	var err error
	handle, err = ui.ShowOverlay(dialog, tui.OverlayOptions{Width: &tui.SizeValue{Absolute: &width}, Anchor: &anchor, Visible: func(w, h int) bool { return w >= 15 }})
	if err != nil {
		t.Fatal(err)
	}
	if !handle.IsFocused() {
		t.Fatal("dialog did not capture focus")
	}
	if err = ui.Start(); err != nil {
		t.Fatal(err)
	}
	defer ui.Stop()
	if err = ui.RenderNow(); err != nil {
		t.Fatal(err)
	}
	state := ui.CaptureRenderState()
	if !strings.Contains(state.PreviousLines[0], "Choose") {
		t.Fatalf("overlay missing: %q", state.PreviousLines)
	}
	terminal.mu.Lock()
	terminal.width = 12
	terminal.mu.Unlock()
	if err = ui.RenderNow(); err != nil {
		t.Fatal(err)
	}
	if ui.HasOverlay() || ui.GetFocusedComponent() != base {
		t.Fatal("invisible overlay retained focus")
	}
	terminal.mu.Lock()
	terminal.width = 20
	terminal.mu.Unlock()
	if err = ui.RenderNow(); err != nil {
		t.Fatal(err)
	}
	if !handle.IsFocused() {
		t.Fatal("resize did not restore dialog focus")
	}
	done := make(chan error, 1)
	go func() { done <- ui.HandleInput("\r") }()
	select {
	case err = <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("closing dialog deadlocked")
	}
	if ui.HasOverlay() || ui.GetFocusedComponent() != base {
		t.Fatal("close did not restore base focus")
	}
	if err = ui.HandleInput("\r"); err != nil {
		t.Fatal(err)
	}
	if inputs != 1 {
		t.Fatal("input not restored")
	}
}

func TestOverlayOracle114(t *testing.T) {
	data, err := os.ReadFile("../parity/oracle/fixtures/overlays.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Case struct {
			Input struct {
				Width, Height int
				Cases         []struct {
					Name    string
					Options map[string]json.RawMessage
					Lines   []string
				}
			}
		}
		Observation struct{ Outcome struct{ Frames [][]string } }
	}
	if err = json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	terminal := &terminal113{width: 20, height: 6}
	ui := tui.NewTUIMainScreen(terminal)
	base := lines113{"abcdefghijklmnopqrst", "abcdefghijklmnopqrst", "abcdefghijklmnopqrst", "abcdefghijklmnopqrst", "abcdefghijklmnopqrst", "abcdefghijklmnopqrst"}
	ui.AddChild(&base)
	if err = ui.Start(); err != nil {
		t.Fatal(err)
	}
	defer ui.Stop()
	for i, c := range fixture.Case.Input.Cases {
		t.Run(c.Name, func(t *testing.T) {
			var options tui.OverlayOptions
			for key, value := range c.Options {
				switch key {
				case "width", "maxHeight", "row", "col":
					v := &tui.SizeValue{}
					var n int
					if json.Unmarshal(value, &n) == nil {
						v.Absolute = &n
					} else {
						var text string
						_ = json.Unmarshal(value, &text)
						percent, err := strconv.ParseFloat(strings.TrimSuffix(text, "%"), 64)
						if err != nil {
							t.Fatal(err)
						}
						v.Percentage = &percent
					}
					switch key {
					case "width":
						options.Width = v
					case "maxHeight":
						options.MaxHeight = v
					case "row":
						options.Row = v
					case "col":
						options.Col = v
					}
				case "anchor":
					var anchor tui.OverlayAnchor
					_ = json.Unmarshal(value, &anchor)
					options.Anchor = &anchor
				default:
					var n int
					_ = json.Unmarshal(value, &n)
					switch key {
					case "margin":
						options.Margin = &tui.OverlayMarginValue{All: &n}
					case "minWidth":
						options.MinWidth = &n
					case "offsetX":
						options.OffsetX = &n
					case "offsetY":
						options.OffsetY = &n
					}
				}
			}
			content := lines113(c.Lines)
			handle, err := ui.ShowOverlay(&content, options)
			if err != nil {
				t.Fatal(err)
			}
			if err = ui.RenderNow(); err != nil {
				t.Fatal(err)
			}
			lines := ui.CaptureRenderState().PreviousLines
			for j, line := range lines {
				clean, _ := tui.StripTerminalSequences(line)
				lines[j] = strings.TrimRight(clean, " ")
			}
			if !reflect.DeepEqual(lines, fixture.Observation.Outcome.Frames[i]) {
				t.Fatalf("got %q want %q", lines, fixture.Observation.Outcome.Frames[i])
			}
			if err = handle.Hide(); err != nil {
				t.Fatal(err)
			}
			if err = ui.RenderNow(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

type failingDialogTerminal114 struct {
	terminal113
	writes atomic.Int32
}

func (t *failingDialogTerminal114) Write(s string) error {
	if t.writes.Add(1) > 1 {
		return io.ErrClosedPipe
	}
	return t.terminal113.Write(s)
}
func TestDialogRenderFailure114(t *testing.T) {
	terminal := &failingDialogTerminal114{terminal113: terminal113{width: 30, height: 10}}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := tui.ShowSelectDialog(ctx, terminal, "Select", []tui.SelectItem{{Value: "yes"}})
	if !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("render failure lost: %v", err)
	}
}

func TestOverlayStackAndNonCapturing114(t *testing.T) {
	terminal := &terminal113{width: 30, height: 10}
	ui := tui.NewTUIMainScreen(terminal)
	base := tui.NewSelectList([]tui.SelectItem{{Value: "base"}}, 1, tui.SelectListTheme{})
	_ = ui.SetFocus(base)
	first := tui.NewSelectDialog("first", nil, nil, nil)
	second := tui.NewSelectDialog("second", nil, nil, nil)
	a, err := ui.ShowOverlay(first)
	if err != nil {
		t.Fatal(err)
	}
	b, err := ui.ShowOverlay(second)
	if err != nil {
		t.Fatal(err)
	}
	if !b.IsFocused() {
		t.Fatal("top overlay not focused")
	}
	if err = a.Hide(); err != nil {
		t.Fatal(err)
	}
	if err = b.Hide(); err != nil {
		t.Fatal(err)
	}
	if ui.GetFocusedComponent() != base {
		t.Fatal("removed overlay restored stale focus")
	}
	hint, err := ui.ShowOverlay(first, tui.OverlayOptions{NonCapturing: ptr113(true)})
	if err != nil {
		t.Fatal(err)
	}
	if hint.IsFocused() {
		t.Fatal("noncapturing overlay took focus")
	}
	_ = hint.Focus()
	if !hint.IsFocused() {
		t.Fatal("explicit focus failed")
	}
	_ = hint.SetHidden(true)
	if ui.HasOverlay() || ui.GetFocusedComponent() != base {
		t.Fatal("hide did not restore focus")
	}
	_ = hint.SetHidden(false)
	if !ui.HasOverlay() || hint.IsFocused() {
		t.Fatal("noncapturing show changed focus")
	}
	_ = hint.Focus()
	_ = hint.Unfocus(tui.OverlayUnfocusOptions{Target: base})
	if hint.IsFocused() {
		t.Fatal("unfocus ignored")
	}
	_ = ui.HideOverlay()
	if ui.HasOverlayEntries() {
		t.Fatal("entry leaked")
	}
}

func TestOverlayHiddenParentInput114(t *testing.T) {
	terminal := &terminal113{width: 30, height: 10}
	ui := tui.NewTUIMainScreen(terminal)
	inputs := 0
	base := tui.NewSelectList([]tui.SelectItem{{Value: "base"}}, 1, tui.SelectListTheme{})
	base.OnSelect = func(tui.SelectItem) { inputs++ }
	_ = ui.SetFocus(base)
	a, err := ui.ShowOverlay(tui.NewSelectDialog("hidden", nil, nil, nil))
	if err != nil {
		t.Fatal(err)
	}
	b, err := ui.ShowOverlay(tui.NewSelectDialog("top", nil, nil, nil))
	if err != nil {
		t.Fatal(err)
	}
	_ = a.SetHidden(true)
	_ = b.Hide()
	if err = ui.HandleInput("\r"); err != nil {
		t.Fatal(err)
	}
	if inputs != 1 {
		t.Fatal("closing top overlay routed input to hidden parent")
	}
}

func TestOverlayPreservesUncoveredCursor114(t *testing.T) {
	terminal := &terminal113{width: 20, height: 6}
	ui := tui.NewTUIMainScreen(terminal, tui.TUIBaseOptions{ShowHardwareCursor: ptr113(true)})
	base := lines113{tui.CursorMarker + "base"}
	ui.AddChild(&base)
	_ = ui.SetFocus(&base)
	if err := ui.Start(); err != nil {
		t.Fatal(err)
	}
	defer ui.Stop()
	hint := lines113{"hint"}
	width := 5
	_, err := ui.ShowOverlay(&hint, tui.OverlayOptions{NonCapturing: ptr113(true), Width: &tui.SizeValue{Absolute: &width}, Anchor: ptr113(tui.OverlayAnchorTopRight)})
	if err != nil {
		t.Fatal(err)
	}
	if err = ui.RenderNow(); err != nil {
		t.Fatal(err)
	}
	screen := terminaltest.NewScreen(20, 6)
	screen.Feed(terminal.text())
	if !screen.Visible || screen.Col != 0 || ui.GetFocusedComponent() != &base {
		t.Fatalf("noncapturing hint lost cursor: visible=%v col=%d", screen.Visible, screen.Col)
	}
}

package tui_test

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nankedr/pig/internal/terminaltest"
	"github.com/nankedr/pig/tui"
)

type mainLines113 struct {
	mu    sync.Mutex
	lines []string
}

func (c *mainLines113) Render(int) ([]string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.lines...), nil
}
func (*mainLines113) Invalidate() error { return nil }

func TestMainScreenOracle113(t *testing.T) {
	t.Setenv("TERMUX_VERSION", "")
	data, err := os.ReadFile("../parity/oracle/fixtures/main-screen.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Case struct {
			Input struct {
				Width, Height int
				Steps         []struct {
					Name                 string
					Lines                []string
					Width, Height        int
					Force, ClearOnShrink bool
				}
			}
		}
		Observation struct {
			Outcome struct {
				Steps []struct {
					Output      string
					State       tui.TUIMainScreenRenderState
					FullRedraws int
				}
				StopOutput string
			}
		}
	}
	if err = json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	input := fixture.Case.Input
	terminal := &terminal113{width: input.Width, height: input.Height}
	ui := tui.NewTUIMainScreen(terminal, tui.TUIBaseOptions{ShowHardwareCursor: ptr113(true)})
	content := &mainLines113{}
	ui.AddChild(content)
	defer ui.Stop(tui.TUIStopOptions{PreserveScreen: ptr113(true)})
	gotScreen, wantScreen := terminaltest.NewScreen(input.Width, input.Height), terminaltest.NewScreen(input.Width, input.Height)
	offset := 0
	for i, step := range input.Steps {
		content.mu.Lock()
		if step.Lines != nil {
			content.lines = step.Lines
		}
		content.mu.Unlock()
		terminal.mu.Lock()
		if step.Width != 0 {
			terminal.width = step.Width
		}
		if step.Height != 0 {
			terminal.height = step.Height
		}
		terminal.mu.Unlock()
		gotScreen.Resize(terminal.width, terminal.height)
		wantScreen.Resize(terminal.width, terminal.height)
		if step.ClearOnShrink {
			ui.SetClearOnShrink(true)
		}
		if i == 0 {
			if err = ui.Start(); err != nil {
				t.Fatal(err)
			}
		}
		if err = ui.RenderNow(step.Force); err != nil {
			t.Fatal(err)
		}
		output := terminal.text()
		delta := output[offset:]
		offset = len(output)
		want := fixture.Observation.Outcome.Steps[i]
		gotScreen.Feed(delta)
		wantScreen.Feed(want.Output)
		if gotScreen.Text() != wantScreen.Text() || !reflect.DeepEqual(gotScreen.History, wantScreen.History) || gotScreen.Row != wantScreen.Row || gotScreen.Col != wantScreen.Col || gotScreen.Visible != wantScreen.Visible {
			t.Fatalf("%s: screen %q history %q cursor %d,%d visible %v; want %q history %q cursor %d,%d visible %v", step.Name, gotScreen.Text(), gotScreen.History, gotScreen.Row, gotScreen.Col, gotScreen.Visible, wantScreen.Text(), wantScreen.History, wantScreen.Row, wantScreen.Col, wantScreen.Visible)
		}
		state := ui.CaptureRenderState()
		if len(want.State.PreviousLines) == 0 {
			want.State.PreviousLines = nil
		}
		if !reflect.DeepEqual(state, want.State) || ui.FullRedraws() != want.FullRedraws {
			t.Fatalf("%s state %#v redraws %d want %#v redraws %d", step.Name, state, ui.FullRedraws(), want.State, want.FullRedraws)
		}
		if step.Name == "edit" || step.Name == "cursor" || step.Name == "append" {
			if strings.Contains(delta, "row 0") || strings.Contains(delta, "\x1b[2J") {
				t.Fatalf("%s replayed history: %q", step.Name, delta)
			}
		}
	}
	if err = ui.Stop(); err != nil {
		t.Fatal(err)
	}
	gotScreen.Feed(terminal.text()[offset:])
	wantScreen.Feed(fixture.Observation.Outcome.StopOutput)
	if gotScreen.Text() != wantScreen.Text() || gotScreen.Row != wantScreen.Row || gotScreen.Col != wantScreen.Col {
		t.Fatal("stop did not leave cursor after document")
	}
}

func TestFullscreenIncrementalOracle113(t *testing.T) {
	data, err := os.ReadFile("../parity/oracle/fixtures/main-screen.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Case struct {
			Input struct {
				Width, Height   int
				FullscreenSteps []struct {
					Name          string
					Lines         []string
					Width, Height int
				}
			}
		}
		Observation struct {
			Outcome struct {
				Fullscreen []struct {
					Output      string
					FullRedraws int
				}
			}
		}
	}
	if err = json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	terminal := &terminal113{width: fixture.Case.Input.Width, height: fixture.Case.Input.Height}
	ui := tui.NewTUIAltScreen(terminal, tui.TUIAltScreenOptions{ShowHardwareCursor: ptr113(true)})
	content := &mainLines113{}
	ui.AddChild(content)
	defer ui.Stop(tui.TUIStopOptions{PreserveScreen: ptr113(true)})
	gotScreen, wantScreen := terminaltest.NewScreen(20, 5), terminaltest.NewScreen(20, 5)
	offset := 0
	for i, step := range fixture.Case.Input.FullscreenSteps {
		content.mu.Lock()
		if step.Lines != nil {
			content.lines = step.Lines
		}
		content.mu.Unlock()
		terminal.mu.Lock()
		if step.Width > 0 {
			terminal.width = step.Width
		}
		if step.Height > 0 {
			terminal.height = step.Height
		}
		terminal.mu.Unlock()
		gotScreen.Resize(terminal.width, terminal.height)
		wantScreen.Resize(terminal.width, terminal.height)
		if i == 0 {
			if err = ui.Start(); err != nil {
				t.Fatal(err)
			}
		}
		if err = ui.RenderNow(); err != nil {
			t.Fatal(err)
		}
		output := terminal.text()
		delta := output[offset:]
		offset = len(output)
		want := fixture.Observation.Outcome.Fullscreen[i]
		gotScreen.Feed(delta)
		wantScreen.Feed(want.Output)
		if gotScreen.Text() != wantScreen.Text() || gotScreen.Row != wantScreen.Row || gotScreen.Col != wantScreen.Col || gotScreen.Visible != wantScreen.Visible || ui.FullRedraws() != want.FullRedraws {
			t.Fatalf("%s: screen %q cursor %d,%d redraws %d; want %q cursor %d,%d redraws %d", step.Name, gotScreen.Text(), gotScreen.Row, gotScreen.Col, ui.FullRedraws(), wantScreen.Text(), wantScreen.Row, wantScreen.Col, want.FullRedraws)
		}
		if i > 0 && step.Width == 0 && strings.Contains(delta, "heading") {
			t.Fatal("unchanged row was rewritten")
		}
	}
}

func TestMainScreenRestoreAndForcedRequest113(t *testing.T) {
	terminal := &terminal113{width: 20, height: 5}
	content := &mainLines113{lines: []string{"history 0", "history 1", "history 2", "history 3", "history 4", "> ab" + tui.CursorMarker}}
	first := tui.NewTUIMainScreen(terminal)
	first.AddChild(content)
	if err := first.Start(); err != nil {
		t.Fatal(err)
	}
	state := first.CaptureRenderState()
	if err := first.Stop(tui.TUIStopOptions{PreserveScreen: ptr113(true)}); err != nil {
		t.Fatal(err)
	}
	next := tui.NewTUIMainScreen(terminal)
	next.AddChild(content)
	if err := next.RestoreRenderState(state); err != nil {
		t.Fatal(err)
	}
	state.PreviousLines[0] = "mutated external snapshot"
	offset := len(terminal.text())
	if err := next.Start(); err != nil {
		t.Fatal(err)
	}
	defer next.Stop()
	if delta := terminal.text()[offset:]; strings.Contains(delta, "history") || strings.Contains(delta, "\x1b[2J") {
		t.Fatalf("restore replayed screen: %q", delta)
	}
	before := next.FullRedraws()
	if err := next.RequestRender(true); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for next.FullRedraws() == before && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if next.FullRedraws() != before+1 {
		t.Fatal("forced request did not redraw")
	}
	if !strings.Contains(terminal.text()[offset:], "\x1b[3J") {
		t.Fatal("forced redraw did not rebuild scrollback")
	}
}

func TestTextUINativeModeRoundtrip113(t *testing.T) {
	terminal := &terminal113{width: 20, height: 5}
	ui := tui.NewTextUI(terminal)
	if err := ui.Start(); err != nil {
		t.Fatal(err)
	}
	defer ui.Stop()
	if err := ui.Append("first\nsecond\nthird\nfourth\nfifth\nlast"); err != nil {
		t.Fatal(err)
	}
	terminal.input("中文abc")
	terminal.input("\x1b[D")
	terminal.input("\x1b[D")
	screen := terminaltest.NewScreen(20, 5)
	output := terminal.text()
	screen.Feed(output)
	offset := len(output)
	history := append([]string(nil), screen.History...)
	for _, mode := range []tui.TUIMode{tui.TUIModeFullscreen, tui.TUIModeRegular, tui.TUIModeFullscreen, tui.TUIModeRegular} {
		if err := ui.SetMode(mode); err != nil {
			t.Fatal(err)
		}
	}
	output = terminal.text()
	screen.Feed(output[offset:])
	offset = len(output)
	if !reflect.DeepEqual(history, screen.History) {
		t.Fatalf("mode switch duplicated/lost native history: %q want %q", screen.History, history)
	}
	if err := ui.SetMode(tui.TUIModeFullscreen); err != nil {
		t.Fatal(err)
	}
	if err := ui.Append("\nnew last"); err != nil {
		t.Fatal(err)
	}
	if err := ui.SetMode(tui.TUIModeRegular); err != nil {
		t.Fatal(err)
	}
	terminal.input("X")
	output = terminal.text()
	screen.Feed(output[offset:])
	if !strings.Contains(screen.Text(), "new last") || !strings.Contains(screen.Text(), "中文aXbc") {
		t.Fatalf("switch lost output or insertion point: %q", screen.Text())
	}
	if !strings.Contains(strings.Join(screen.History, "\n"), "first") {
		t.Fatal("earliest history disappeared")
	}
}

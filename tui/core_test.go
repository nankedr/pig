package tui_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nankedr/pig/tui"
)

type recordingTerminal struct{ calls int }

func (terminal *recordingTerminal) touch() { terminal.calls++ }
func (terminal *recordingTerminal) Start(func(string), func()) error {
	terminal.touch()
	return nil
}
func (terminal *recordingTerminal) Stop() error { terminal.touch(); return nil }
func (terminal *recordingTerminal) DrainInput(context.Context, time.Duration, time.Duration) error {
	terminal.touch()
	return nil
}
func (terminal *recordingTerminal) Write(string) error    { terminal.touch(); return nil }
func (terminal *recordingTerminal) Columns() (int, error) { terminal.touch(); return 80, nil }
func (terminal *recordingTerminal) Rows() (int, error)    { terminal.touch(); return 24, nil }
func (terminal *recordingTerminal) KittyProtocolActive() (bool, error) {
	terminal.touch()
	return false, nil
}
func (terminal *recordingTerminal) MoveBy(int) error { terminal.touch(); return nil }
func (terminal *recordingTerminal) HideCursor() error {
	terminal.touch()
	return nil
}
func (terminal *recordingTerminal) ShowCursor() error {
	terminal.touch()
	return nil
}
func (terminal *recordingTerminal) ClearLine() error { terminal.touch(); return nil }
func (terminal *recordingTerminal) ClearFromCursor() error {
	terminal.touch()
	return nil
}
func (terminal *recordingTerminal) ClearScreen() error {
	terminal.touch()
	return nil
}
func (terminal *recordingTerminal) SetTitle(string) error {
	terminal.touch()
	return nil
}
func (terminal *recordingTerminal) SetProgress(bool) error {
	terminal.touch()
	return nil
}

type runtimeTUI interface {
	Start() error
	Stop(...tui.TUIStopOptions) error
	Render(int) ([]string, error)
	RenderNow(...bool) error
}

func TestTUIRuntimeStubsDoNotTouchInjectedTerminal(t *testing.T) {
	tests := []struct {
		name string
		new  func(tui.Terminal) runtimeTUI
	}{
		{name: "base", new: func(terminal tui.Terminal) runtimeTUI { return tui.NewTUIBase(terminal) }},
		{name: "alternate screen", new: func(terminal tui.Terminal) runtimeTUI {
			return tui.NewTUIAltScreen(terminal)
		}},
		{name: "main screen", new: func(terminal tui.Terminal) runtimeTUI {
			return tui.NewTUIMainScreen(terminal)
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			terminal := &recordingTerminal{}
			ui := test.new(terminal)
			if terminal.calls != 0 {
				t.Fatalf("constructor invoked Terminal %d times", terminal.calls)
			}

			operations := []func() error{
				ui.Start,
				func() error { return ui.Stop() },
				func() error { _, err := ui.Render(80); return err },
				func() error { return ui.RenderNow() },
			}
			for index, operation := range operations {
				err := operation()
				if !errors.Is(err, tui.ErrNotImplemented) {
					t.Fatalf("operation %d error = %v, want ErrNotImplemented", index, err)
				}
				var capabilityErr *tui.NotImplementedError
				if !errors.As(err, &capabilityErr) || capabilityErr.Module != "tui" {
					t.Fatalf("operation %d error = %#v, want structured tui error", index, err)
				}
				if terminal.calls != 0 {
					t.Fatalf("operation %d invoked Terminal %d times", index, terminal.calls)
				}
			}
		})
	}
}

func TestTUIDebugCallbackCanBeWiredWithoutInvocation(t *testing.T) {
	t.Parallel()

	var ui tui.TUI = tui.NewTUIBase(nil)
	calls := 0
	ui.SetOnDebug(func() { calls++ })
	if calls != 0 {
		t.Fatalf("SetOnDebug invoked callback %d times, want 0", calls)
	}
	if ui.OnDebug() == nil {
		t.Fatal("OnDebug returned nil after SetOnDebug")
	}
}

func TestTUIQueriesReportInitializedLocalState(t *testing.T) {
	t.Parallel()

	base := tui.NewTUIBase(nil)
	if base.FullRedraws() != 0 || base.GetFocusedComponent() != nil || base.HasOverlay() || base.HasOverlayEntries() {
		t.Fatal("new TUIBase reported runtime state that has not been established")
	}
	alt := tui.NewTUIAltScreen(nil)
	if alt.ViewportTop() != 0 || !alt.IsFollowingOutput() {
		t.Fatalf("new TUIAltScreen viewport state = (%d, %t), want (0, true)", alt.ViewportTop(), alt.IsFollowingOutput())
	}
}

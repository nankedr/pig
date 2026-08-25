package tui_test

import (
	"context"
	"errors"
	"testing"

	"github.com/nankedr/pig/tui"
)

func TestInputAndPlatformCapabilityStubsAreExplicit(t *testing.T) {
	provider := tui.NewCombinedAutocompleteProvider(nil, ".", nil)
	buffer := tui.NewStdinBuffer()
	manager := tui.NewKeybindingsManager(tui.TUIKeybindings)

	operations := []struct {
		name string
		call func() error
	}{
		{name: "autocomplete suggestions", call: func() error {
			_, _, err := provider.GetSuggestions(context.Background(), nil, 0, 0, tui.AutocompleteOptions{})
			return err
		}},
		{name: "autocomplete application", call: func() error {
			_, err := provider.ApplyCompletion(nil, 0, 0, tui.AutocompleteItem{}, "")
			return err
		}},
		{name: "key matching", call: func() error {
			_, err := tui.MatchesKey("", tui.Key.Enter)
			return err
		}},
		{name: "key parsing", call: func() error {
			_, _, err := tui.ParseKey("")
			return err
		}},
		{name: "keybinding resolution", call: func() error {
			_, err := manager.GetResolvedBindings()
			return err
		}},
		{name: "stdin processing", call: func() error { return buffer.Process([]byte("x")) }},
		{name: "word navigation", call: func() error {
			_, err := tui.FindWordForward("word", 0)
			return err
		}},
		{name: "terminal color parsing", call: func() error {
			_, _, err := tui.ParseOSC11BackgroundColor("\x1b]11;#000000\x07")
			return err
		}},
		{name: "native modifier inspection", call: func() error {
			_, err := tui.IsNativeModifierPressed(tui.ModifierKeyShift)
			return err
		}},
	}

	for _, operation := range operations {
		t.Run(operation.name, func(t *testing.T) {
			if err := operation.call(); !errors.Is(err, tui.ErrNotImplemented) {
				t.Fatalf("error = %v, want ErrNotImplemented", err)
			}
		})
	}
}

func TestKillRing(t *testing.T) {
	ring := tui.NewKillRing()
	ring.Push("world", tui.KillRingPushOptions{})
	ring.Push("hello ", tui.KillRingPushOptions{Prepend: true, Accumulate: true})
	ring.Push("again", tui.KillRingPushOptions{})

	if got, ok := ring.Peek(); !ok || got != "again" || ring.Length() != 2 {
		t.Fatalf("Peek/Length = (%q, %t, %d), want (again, true, 2)", got, ok, ring.Length())
	}
	ring.Rotate()
	if got, ok := ring.Peek(); !ok || got != "hello world" {
		t.Fatalf("Peek after Rotate = (%q, %t), want (hello world, true)", got, ok)
	}
}

func TestConstructorsDoNotInvokeCallbacks(t *testing.T) {
	called := false
	command := tui.SlashCommand{
		Name: "test",
		GetArgumentCompletions: func(context.Context, string) ([]tui.AutocompleteItem, bool, error) {
			called = true
			return nil, false, nil
		},
	}
	_ = tui.NewCombinedAutocompleteProvider([]tui.AutocompleteEntry{command}, ".", nil)
	_ = tui.NewStdinBuffer(tui.StdinBufferOptions{})
	if called {
		t.Fatal("constructor invoked autocomplete callback")
	}
}

func TestScrollViewRecordsSafeInitialStateWithoutStartingRuntimeWork(t *testing.T) {
	t.Parallel()

	follow := tui.ScrollViewFollowEnd
	scrollbar := tui.ScrollViewScrollbarAlways
	delay := int64(250)
	view := tui.NewScrollView(nil, tui.ScrollViewOptions{
		Follow:               &follow,
		Scrollbar:            &scrollbar,
		ScrollbarHideDelayMS: &delay,
	})
	if !view.IsFollowingEnd() {
		t.Fatal("follow=end did not initialize following state")
	}
	if view.Scrollbar() != scrollbar || view.ScrollTop() != 0 || view.ViewportHeight() != 0 {
		t.Fatalf("unexpected initial scroll state: scrollbar=%q top=%d height=%d", view.Scrollbar(), view.ScrollTop(), view.ViewportHeight())
	}
	if view.IsScrollbarVisible() {
		t.Fatal("zero-height viewport reported a visible scrollbar")
	}
}

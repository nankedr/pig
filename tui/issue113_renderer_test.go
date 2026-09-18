package tui_test

import (
	"context"
	"encoding/json"
	"github.com/nankedr/pig/tui"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

type terminal113 struct {
	recordingTerminal
	mu            sync.Mutex
	output        string
	input         func(string)
	resize        func()
	width, height int
}

func (t *terminal113) Start(input func(string), resize func()) error {
	t.input, t.resize = input, resize
	return nil
}
func (t *terminal113) Write(s string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.output += s
	return nil
}
func (t *terminal113) Columns() (int, error)                                        { return t.width, nil }
func (t *terminal113) Rows() (int, error)                                           { return t.height, nil }
func (*terminal113) DrainInput(context.Context, time.Duration, time.Duration) error { return nil }
func (t *terminal113) text() string                                                 { t.mu.Lock(); defer t.mu.Unlock(); return t.output }
func TestFullscreenRendererScroll113(t *testing.T) {
	terminal := &terminal113{width: 12, height: 5}
	ui := tui.NewTUIAltScreen(terminal)
	content := lines113{"zero", "one", "two", "three", "four", "five", "six", "seven"}
	ui.AddChild(&content)
	if err := ui.Start(); err != nil {
		t.Fatal(err)
	}
	defer ui.Stop()
	if ui.ViewportTop() != 3 || !ui.IsFollowingOutput() {
		t.Fatalf("initial viewport %d", ui.ViewportTop())
	}
	if err := ui.HandleInput("\x1b[H"); err != nil {
		t.Fatal(err)
	}
	if ui.ViewportTop() != 0 || ui.IsFollowingOutput() {
		t.Fatal("home did not detach")
	}
	if err := ui.HandleInput("\x1b[<65;2;2M"); err != nil {
		t.Fatal(err)
	}
	if ui.ViewportTop() != 1 {
		t.Fatalf("wheel top %d", ui.ViewportTop())
	}
	if err := ui.HandleInput("\x1b[F"); err != nil {
		t.Fatal(err)
	}
	if !ui.IsFollowingOutput() {
		t.Fatal("end did not follow")
	}
	if err := ui.Stop(); err != nil {
		t.Fatal(err)
	}
	output := terminal.text()
	if !strings.Contains(output, "\x1b[?1049h") || !strings.Contains(output, "\x1b[?1049l") || !strings.Contains(output, "\x1b[?1006l") {
		t.Fatalf("screen lifecycle %q", output)
	}
}

func TestNestedMouseOracle113(t *testing.T) {
	data, err := os.ReadFile("../parity/oracle/fixtures/layout-scrolling.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Observation struct {
			Outcome struct {
				Mouse []struct {
					Overscroll tui.ScrollViewOverscroll
					Positions  [][2]int
				}
			}
		}
	}
	if err = json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	for _, c := range fixture.Observation.Outcome.Mouse {
		t.Run(string(c.Overscroll), func(t *testing.T) {
			terminal := &terminal113{width: 12, height: 5}
			content := lines113{"a", "b", "c", "d", "e"}
			footer := lines113{"input", "cursor" + tui.CursorMarker}
			inner := tui.NewScrollView(&content, tui.ScrollViewOptions{Scrollbar: ptr113(tui.ScrollViewScrollbarAlways), Overscroll: &c.Overscroll})
			outer := tui.NewScrollView(tui.NewVStack([]tui.StackChild{{Entry: &tui.StackEntry{Component: inner, StackEntryOptions: tui.StackEntryOptions{Basis: &tui.StackBasis{Size: 3}}}}, {Entry: &tui.StackEntry{Component: &footer, StackEntryOptions: tui.StackEntryOptions{Basis: &tui.StackBasis{Size: 6}}}}}), tui.ScrollViewOptions{Primary: ptr113(true)})
			ui := tui.NewTUIAltScreen(terminal, tui.TUIAltScreenOptions{WheelScrollLines: ptr113(3)})
			if err := ui.SetLayoutRoot(outer); err != nil {
				t.Fatal(err)
			}
			if err := ui.Start(); err != nil {
				t.Fatal(err)
			}
			defer ui.Stop()
			for i, key := range []string{"\x1b[<65;2;2M", "\x1b[<65;2;5M", "\x1b[H", "\x1b[<0;12;2M", "\x1b[<32;12;1M", "\x1b[<0;12;1m"} {
				if err := ui.HandleInput(key); err != nil {
					t.Fatal(err)
				}
				if err := ui.RenderNow(); err != nil {
					t.Fatal(err)
				}
				if got := [2]int{inner.ScrollTop(), outer.ScrollTop()}; got != c.Positions[i] {
					t.Fatalf("step %d got %v want %v", i, got, c.Positions[i])
				}
			}
		})
	}
}

func TestScrollbarAutoAndWideCells113(t *testing.T) {
	content := lines113{"中文", "中文", "中文", "中文"}
	scroll := tui.NewScrollView(&content, tui.ScrollViewOptions{Scrollbar: ptr113(tui.ScrollViewScrollbarAuto), ScrollbarHideDelayMS: ptr113(int64(20))})
	renders := make(chan struct{}, 10)
	if _, err := tui.RenderLayoutFrame(scroll, 4, 2, func() { renders <- struct{}{} }); err != nil {
		t.Fatal(err)
	}
	if _, err := scroll.ScrollBy(1); err != nil {
		t.Fatal(err)
	}
	if !scroll.IsScrollbarVisible() {
		t.Fatal("scroll should reveal scrollbar")
	}
	frame, err := tui.RenderLayoutFrame(scroll, 4, 2, func() { renders <- struct{}{} })
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(clean113(frame.Lines), []string{"中文", "中文"}) {
		t.Fatalf("scrollbar broke wide glyph %q", frame.Lines)
	}
	deadline := time.After(time.Second)
	for scroll.IsScrollbarVisible() {
		select {
		case <-renders:
		case <-deadline:
			t.Fatal("auto scrollbar did not hide")
		}
	}
}

func TestTextUIModeSwitchAndEditor113(t *testing.T) {
	terminal := &terminal113{width: 20, height: 5}
	ui := tui.NewTextUI(terminal, tui.TextUIOptions{Mode: tui.TUIModeFullscreen, Scrollbar: tui.ScrollViewScrollbarAlways, PreserveScreen: true})
	if err := ui.Start(); err != nil {
		t.Fatal(err)
	}
	defer ui.Stop()
	if err := ui.Append("first\nsecond\nthird\nfourth\nfifth\nlast"); err != nil {
		t.Fatal(err)
	}
	terminal.input("\x1b[200~中文\nsecond\nthird\x1b[201~")
	for _, mode := range []tui.TUIMode{tui.TUIModeRegular, tui.TUIModeFullscreen} {
		if err := ui.SetMode(mode); err != nil {
			t.Fatal(err)
		}
		if ui.Mode() != mode {
			t.Fatal("mode did not change")
		}
	}
	terminal.input("\r")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	input, err := ui.ReadLine(ctx)
	if err != nil || input != "中文\nsecond\nthird" {
		t.Fatalf("switch lost editor %q %v", input, err)
	}
	if err := ui.ScrollToTop(); err != nil {
		t.Fatal(err)
	}
	output := terminal.text()
	frame := output[strings.LastIndex(output, "\x1b[H\x1b[2J"):]
	if !strings.Contains(frame, "first") {
		t.Fatalf("history missing %q", frame)
	}
	if err := ui.Append("\nnew last"); err != nil {
		t.Fatal(err)
	}
	output = terminal.text()
	frame = output[strings.LastIndex(output, "\x1b[H\x1b[2J"):]
	if !strings.Contains(frame, "first") || strings.Contains(frame, "new last") {
		t.Fatalf("append moved reading position %q", frame)
	}
	if err := ui.ScrollToBottom(); err != nil {
		t.Fatal(err)
	}
	output = terminal.text()
	frame = output[strings.LastIndex(output, "\x1b[H\x1b[2J"):]
	if !strings.Contains(frame, "new last") {
		t.Fatal("cannot resume following")
	}
}

func TestScrollbarTrackDoesNotDragOracle113(t *testing.T) {
	data, err := os.ReadFile("../parity/oracle/fixtures/layout-scrolling.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Observation struct {
			Outcome struct {
				TrackDrag map[tui.ScrollViewScrollbar][]int
			}
		}
	}
	if err = json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	for mode, want := range fixture.Observation.Outcome.TrackDrag {
		t.Run(string(mode), func(t *testing.T) {
			terminal := &terminal113{width: 12, height: 5}
			content := lines113{}
			for i := 0; i < 20; i++ {
				content = append(content, "line")
			}
			scroll := tui.NewScrollView(&content, tui.ScrollViewOptions{Scrollbar: &mode})
			ui := tui.NewTUIAltScreen(terminal)
			if err := ui.SetLayoutRoot(scroll); err != nil {
				t.Fatal(err)
			}
			if err := ui.Start(); err != nil {
				t.Fatal(err)
			}
			defer ui.Stop()
			for i, key := range []string{"\x1b[<0;12;4M", "\x1b[<32;12;5M"} {
				if err := ui.HandleInput(key); err != nil {
					t.Fatal(err)
				}
				if err := ui.RenderNow(); err != nil {
					t.Fatal(err)
				}
				if scroll.ScrollTop() != want[i] {
					t.Fatalf("track click scrolled to %d, want %d", scroll.ScrollTop(), want[i])
				}
			}
		})
	}
}

func TestFocusLossStopsScrollbarDrag113(t *testing.T) {
	t.Setenv("TMUX", "test")
	terminal := &terminal113{width: 12, height: 5}
	content := lines113{}
	for i := 0; i < 20; i++ {
		content = append(content, "line")
	}
	scroll := tui.NewScrollView(&content, tui.ScrollViewOptions{Scrollbar: ptr113(tui.ScrollViewScrollbarAlways)})
	ui := tui.NewTUIAltScreen(terminal)
	if err := ui.SetLayoutRoot(scroll); err != nil {
		t.Fatal(err)
	}
	if err := ui.Start(); err != nil {
		t.Fatal(err)
	}
	defer ui.Stop()
	for _, key := range []string{"\x1b[<0;12;1M", "\x1b[O", "\x1b[I", "\x1b[<35;12;5M"} {
		if err := ui.HandleInput(key); err != nil {
			t.Fatal(err)
		}
	}
	if scroll.ScrollTop() != 0 {
		t.Fatal("pointer motion after focus loss continued the old drag")
	}
	output := terminal.text()
	if !strings.Contains(output, "\x1b[?1004h") || strings.Contains(output, "\x1b[?1003h") {
		t.Fatal("multiplexer must enable focus and button motion, without all-motion tracking")
	}
}

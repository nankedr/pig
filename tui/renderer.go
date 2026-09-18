package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"
	"time"
)

const enterAltScreen = "\x1b[?1049h\x1b[?7l"
const leaveAltScreen = "\x1b[?1000l\x1b[?1002l\x1b[?1003l\x1b[?1004l\x1b[?1006l\x1b[?7h\x1b[?1049l"

func enableMouse() string {
	sequence := "\x1b[?1000h\x1b[?1002h\x1b[?1004h\x1b[?1006h"
	term := strings.ToLower(os.Getenv("TERM"))
	if os.Getenv("TMUX") == "" && os.Getenv("ZELLIJ") == "" && os.Getenv("STY") == "" && !strings.HasPrefix(term, "tmux") && !strings.HasPrefix(term, "screen") {
		sequence += "\x1b[?1003h"
	}
	return sequence
}

func (t *TUIBase) Render(width int) ([]string, error)       { return t.Container.Render(width) }
func (t *TUIMainScreen) Render(width int) ([]string, error) { return t.Container.Render(width) }
func (t *TUIAltScreen) Render(width int) ([]string, error) {
	t.renderMu.Lock()
	defer t.renderMu.Unlock()
	if t.root != nil {
		return t.root.Render(width)
	}
	return t.Container.Render(width)
}

func (t *TUIBase) layoutFrame(width, height int) (LayoutFrame, error) {
	root := t.root
	if root == nil {
		if t.defaultScroll == nil {
			follow := ScrollViewFollowEnd
			t.defaultScroll = NewScrollView(&t.Container, ScrollViewOptions{Follow: &follow})
		}
		root = t.defaultScroll
	}
	frame, err := RenderLayoutFrame(root, width, height, func() { _ = t.RequestRender() })
	if err == nil {
		t.frame = frame
	}
	return frame, err
}
func (t *TUIAltScreen) SetLayoutRoot(root Component) error {
	t.renderMu.Lock()
	t.root = root
	t.layoutRoot = root
	t.renderMu.Unlock()
	return t.RequestRender()
}
func (t *TUIAltScreen) primary() *ScrollView {
	if t.frame.PrimaryScrollView != nil {
		return t.frame.PrimaryScrollView
	}
	return t.defaultScroll
}
func (t *TUIAltScreen) ViewportTop() int {
	t.renderMu.Lock()
	defer t.renderMu.Unlock()
	if p := t.primary(); p != nil {
		return p.ScrollTop()
	}
	return 0
}
func (t *TUIAltScreen) IsFollowingOutput() bool {
	t.renderMu.Lock()
	defer t.renderMu.Unlock()
	if p := t.primary(); p != nil {
		return p.IsFollowingEnd()
	}
	return true
}
func (t *TUIAltScreen) ScrollBy(lines int) error {
	t.renderMu.Lock()
	defer t.renderMu.Unlock()
	if p := t.primary(); p != nil {
		_, err := p.ScrollBy(lines)
		return err
	}
	return nil
}
func (t *TUIAltScreen) ScrollToTop() error {
	t.renderMu.Lock()
	defer t.renderMu.Unlock()
	if p := t.primary(); p != nil {
		return p.ScrollToStart()
	}
	return nil
}
func (t *TUIAltScreen) ScrollToBottom() error {
	t.renderMu.Lock()
	defer t.renderMu.Unlock()
	if p := t.primary(); p != nil {
		return p.ScrollToEnd()
	}
	return nil
}
func (t *TUIBase) SetFocus(c Component) error {
	t.renderMu.Lock()
	defer t.renderMu.Unlock()
	if f, ok := IsFocusable(t.focusedComponent); ok {
		f.SetFocusState(false)
	}
	t.focusedComponent = c
	if f, ok := IsFocusable(c); ok {
		f.SetFocusState(true)
	}
	return t.RequestRender()
}
func (t *TUIBase) SetShowHardwareCursor(show bool) error {
	t.renderMu.Lock()
	t.showHardwareCursor = show
	t.renderMu.Unlock()
	return t.RequestRender()
}
func (t *TUIBase) SetClearOnShrink(clear bool) error {
	t.renderMu.Lock()
	t.clearOnShrink = clear
	t.renderMu.Unlock()
	return t.RequestRender()
}
func (t *TUIBase) Start() error {
	t.renderMu.Lock()
	if t.started {
		t.renderMu.Unlock()
		return nil
	}
	if t.terminal == nil {
		t.renderMu.Unlock()
		return errors.New("TUI requires a terminal")
	}
	t.started = true
	t.renderStop = make(chan struct{})
	stop := t.renderStop
	t.renderMu.Unlock()
	if err := t.terminal.Start(func(data string) { _ = t.HandleInput(data) }, func() { _ = t.RequestRender(true) }); err != nil {
		_ = t.Stop()
		return err
	}
	if t.screenMode == TUIModeFullscreen {
		seq := enterAltScreen
		if t.mouseCapture {
			seq += enableMouse()
		}
		if err := t.terminal.Write(seq); err != nil {
			_ = t.Stop()
			return err
		}
	}
	if err := t.RenderNow(true); err != nil {
		_ = t.Stop()
		return err
	}
	go func() {
		for {
			select {
			case <-stop:
				return
			case <-t.renderWake:
				_ = t.RenderNow()
			}
		}
	}()
	return nil
}
func (t *TUIBase) Stop(options ...TUIStopOptions) error {
	t.renderMu.Lock()
	if !t.started {
		t.renderMu.Unlock()
		return nil
	}
	t.started = false
	close(t.renderStop)
	var err error
	if t.screenMode == TUIModeFullscreen {
		err = t.terminal.Write(leaveAltScreen)
		preserve := len(options) > 0 && options[0].PreserveScreen != nil && *options[0].PreserveScreen
		if !preserve {
			width, e := t.terminal.Columns()
			err = errors.Join(err, e)
			if e == nil {
				var lines []string
				if t.root != nil {
					lines, e = t.root.Render(width)
				} else {
					lines, e = t.Container.Render(width)
				}
				err = errors.Join(err, e)
				if e == nil {
					for i, l := range lines {
						lines[i] = strings.ReplaceAll(sliceCells(l, 0, width), CursorMarker, "")
					}
					err = errors.Join(err, t.terminal.Write(strings.Join(lines, "\r\n")+"\r\n"))
				}
			}
		}
	}
	t.renderMu.Unlock()
	return errors.Join(err, t.terminal.DrainInput(context.Background(), time.Second, 50*time.Millisecond), t.terminal.Stop())
}
func (t *TUIBase) RequestRender(_ ...bool) error {
	select {
	case t.renderWake <- struct{}{}:
	default:
	}
	return nil
}
func (t *TUIBase) RenderNow(_ ...bool) error {
	t.renderMu.Lock()
	defer t.renderMu.Unlock()
	if !t.started {
		return nil
	}
	width, err := t.terminal.Columns()
	if err != nil {
		return err
	}
	height, err := t.terminal.Rows()
	if err != nil {
		return err
	}
	frame, err := t.layoutFrame(width, height)
	if err != nil {
		return err
	}
	output := screenFrame(frame.Lines, width, height, t.showHardwareCursor)
	if err = t.terminal.Write(output); err != nil {
		return err
	}
	t.fullRedraws++
	t.mainState = TUIMainScreenRenderState{PreviousLines: append([]string(nil), frame.Lines...), PreviousWidth: width, PreviousHeight: height, MaxLinesRendered: len(frame.Lines)}
	return nil
}
func screenFrame(lines []string, width, height int, showCursor bool) string {
	width, height = max(1, width), max(1, height)
	row, col := -1, 0
	out := make([]string, min(len(lines), height))
	for i, l := range lines[:len(out)] {
		l = sliceCells(l, 0, width)
		if at := strings.Index(l, CursorMarker); at >= 0 {
			row = i
			col, _ = VisibleWidth(l[:at])
		}
		out[i] = strings.ReplaceAll(l, CursorMarker, "")
	}
	suffix := "\x1b[?25l"
	if showCursor && row >= 0 {
		suffix = fmt.Sprintf("\x1b[%d;%dH\x1b[?25h", row+1, min(width, col+1))
	}
	return "\x1b[?2026h\x1b[H\x1b[2J" + strings.Join(out, "\r\n") + suffix + "\x1b[?2026l"
}
func (t *TUIBase) HandleInput(data string) error {
	t.renderMu.Lock()
	ids := make([]uint64, 0, len(t.listeners))
	for id := range t.listeners {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	listeners := make([]TUIInputListener, 0, len(ids))
	for _, id := range ids {
		listeners = append(listeners, t.listeners[id])
	}
	t.renderMu.Unlock()
	for _, listener := range listeners {
		r := listener(data)
		if r.Data != nil {
			data = *r.Data
		}
		if r.Consume != nil && *r.Consume {
			return nil
		}
	}
	t.renderMu.Lock()
	defer t.renderMu.Unlock()
	if release, _ := IsKeyRelease(data); release {
		return nil
	}
	keys, _ := GetKeybindings()
	kitty, _ := t.terminal.KittyProtocolActive()
	if t.screenMode == TUIModeFullscreen {
		handled, err := t.mouse.handle(data, t.frame, t.frame.PrimaryScrollView, t.wheelLines, keys, kitty)
		if handled || err != nil {
			_ = t.RequestRender()
			return err
		}
	}
	if h, ok := t.focusedComponent.(ComponentInputHandler); ok {
		if err := h.HandleInput(data); err != nil {
			return err
		}
	}
	return t.RequestRender()
}
func (t *TUIBase) AddInputListener(listener TUIInputListener) (TUIUnsubscribe, error) {
	t.renderMu.Lock()
	defer t.renderMu.Unlock()
	if t.listeners == nil {
		t.listeners = map[uint64]TUIInputListener{}
	}
	t.listenerID++
	id := t.listenerID
	t.listeners[id] = listener
	return func() error { t.renderMu.Lock(); defer t.renderMu.Unlock(); delete(t.listeners, id); return nil }, nil
}
func (t *TUIBase) RemoveInputListener(listener TUIInputListener) error {
	t.renderMu.Lock()
	defer t.renderMu.Unlock()
	for id, l := range t.listeners {
		if reflect.ValueOf(l).Pointer() == reflect.ValueOf(listener).Pointer() {
			delete(t.listeners, id)
			break
		}
	}
	return nil
}
func (t *TUIMainScreen) CaptureRenderState() TUIMainScreenRenderState {
	t.renderMu.Lock()
	defer t.renderMu.Unlock()
	state := t.mainState
	state.PreviousLines = append([]string(nil), state.PreviousLines...)
	return state
}
func (t *TUIMainScreen) RestoreRenderState(state TUIMainScreenRenderState) error {
	t.renderMu.Lock()
	defer t.renderMu.Unlock()
	state.PreviousLines = append([]string(nil), state.PreviousLines...)
	t.mainState = state
	return nil
}

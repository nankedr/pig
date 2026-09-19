package tui

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

type overlayEntry struct {
	ui                  *TUIBase
	component, previous Component
	options             OverlayOptions
	hidden, removed     bool
	order               uint64
}

func (t *TUIBase) setFocus(c Component) {
	if f, ok := IsFocusable(t.focusedComponent); ok {
		f.SetFocusState(false)
	}
	t.focusedComponent = c
	if f, ok := IsFocusable(c); ok {
		f.SetFocusState(true)
	}
}
func (e *overlayEntry) visible(w, h int) bool {
	return !e.hidden && !e.removed && (e.options.Visible == nil || e.options.Visible(w, h))
}
func (t *TUIBase) overlaySize() (int, int) {
	if t.terminal == nil {
		return 0, 0
	}
	w, _ := t.terminal.Columns()
	h, _ := t.terminal.Rows()
	return w, h
}
func (t *TUIBase) topOverlay(except *overlayEntry) *overlayEntry {
	w, h := t.overlaySize()
	var top *overlayEntry
	for _, e := range t.overlays {
		if e != except && e.visible(w, h) && (e.options.NonCapturing == nil || !*e.options.NonCapturing) && (top == nil || e.order > top.order) {
			top = e
		}
	}
	return top
}
func (t *TUIBase) hasCapturingOverlay() bool { return t.topOverlay(nil) != nil }
func (t *TUIBase) HasOverlay() bool {
	t.renderMu.Lock()
	defer t.renderMu.Unlock()
	w, h := t.overlaySize()
	for _, e := range t.overlays {
		if e.visible(w, h) {
			return true
		}
	}
	return false
}
func (t *TUIBase) ShowOverlay(c Component, options ...OverlayOptions) (OverlayHandle, error) {
	if c == nil {
		return nil, fmt.Errorf("overlay component must not be nil")
	}
	t.renderMu.Lock()
	defer t.renderMu.Unlock()
	e := &overlayEntry{ui: t, component: c, previous: t.focusedComponent}
	if len(options) > 0 {
		e.options = options[0]
	}
	t.focusOrder++
	e.order = t.focusOrder
	t.overlays = append(t.overlays, e)
	w, h := t.overlaySize()
	if e.visible(w, h) && (e.options.NonCapturing == nil || !*e.options.NonCapturing) {
		t.resizeFocus = nil
		t.setFocus(c)
	}
	return e, t.RequestRender()
}
func (t *TUIBase) restoreOverlayFocus(e *overlayEntry) {
	if !sameComponent(t.focusedComponent, e.component) {
		return
	}
	if top := t.topOverlay(e); top != nil {
		t.setFocus(top.component)
	} else {
		target := e.previous
		w, h := t.overlaySize()
		for range len(t.overlays) + 1 {
			hidden := false
			for _, other := range t.overlays {
				if sameComponent(target, other.component) && !other.visible(w, h) {
					target = other.previous
					hidden = true
					break
				}
			}
			if !hidden {
				t.setFocus(target)
				return
			}
		}
		t.setFocus(nil)
	}
}
func (e *overlayEntry) remove() {
	t := e.ui
	if e.removed {
		return
	}
	e.removed = true
	for _, other := range t.overlays {
		if sameComponent(other.previous, e.component) {
			other.previous = e.previous
		}
	}
	for i, other := range t.overlays {
		if other == e {
			t.overlays = append(t.overlays[:i], t.overlays[i+1:]...)
			break
		}
	}
	if t.resizeFocus == e {
		t.resizeFocus = nil
	}
	t.restoreOverlayFocus(e)
}
func (t *TUIBase) HideOverlay() error {
	t.renderMu.Lock()
	defer t.renderMu.Unlock()
	if len(t.overlays) > 0 {
		t.overlays[len(t.overlays)-1].remove()
	}
	return t.RequestRender()
}
func (e *overlayEntry) Hide() error {
	e.ui.renderMu.Lock()
	defer e.ui.renderMu.Unlock()
	e.remove()
	return e.ui.RequestRender()
}
func (e *overlayEntry) SetHidden(hidden bool) error {
	t := e.ui
	t.renderMu.Lock()
	defer t.renderMu.Unlock()
	if e.removed || e.hidden == hidden {
		return nil
	}
	e.hidden = hidden
	if hidden {
		if t.resizeFocus == e {
			t.resizeFocus = nil
		}
		t.restoreOverlayFocus(e)
	} else {
		w, h := t.overlaySize()
		if e.visible(w, h) && (e.options.NonCapturing == nil || !*e.options.NonCapturing) {
			t.focusOrder++
			e.order = t.focusOrder
			t.resizeFocus = nil
			t.setFocus(e.component)
		}
	}
	return t.RequestRender()
}
func (e *overlayEntry) IsHidden() bool {
	e.ui.renderMu.Lock()
	defer e.ui.renderMu.Unlock()
	return e.hidden
}
func (e *overlayEntry) IsFocused() bool {
	e.ui.renderMu.Lock()
	defer e.ui.renderMu.Unlock()
	return !e.removed && sameComponent(e.ui.focusedComponent, e.component)
}
func (e *overlayEntry) Focus() error {
	t := e.ui
	t.renderMu.Lock()
	defer t.renderMu.Unlock()
	w, h := t.overlaySize()
	if e.visible(w, h) {
		t.focusOrder++
		e.order = t.focusOrder
		t.resizeFocus = nil
		t.setFocus(e.component)
	}
	return t.RequestRender()
}
func (e *overlayEntry) Unfocus(options ...OverlayUnfocusOptions) error {
	t := e.ui
	t.renderMu.Lock()
	defer t.renderMu.Unlock()
	if e.removed {
		return nil
	}
	if sameComponent(t.focusedComponent, e.component) || t.resizeFocus == e {
		t.resizeFocus = nil
		t.restoreOverlayFocus(e)
		if len(options) > 0 {
			t.setFocus(options[0].Target)
		}
	}
	return t.RequestRender()
}
func overlayValue(v *SizeValue, total, defaultValue int) int {
	if v == nil {
		return defaultValue
	}
	if v.Absolute != nil {
		return *v.Absolute
	}
	if v.Percentage != nil {
		return int(math.Floor(float64(total) * *v.Percentage / 100))
	}
	return defaultValue
}
func overlayInt(v *int) int {
	if v != nil {
		return *v
	}
	return 0
}
func overlayLayout(o OverlayOptions, w, h, contentHeight int) (width, height, row, col int) {
	top, right, bottom, left := 0, 0, 0, 0
	if o.Margin != nil {
		if o.Margin.All != nil {
			top = max(0, *o.Margin.All)
			right, bottom, left = top, top, top
		} else if m := o.Margin.Edges; m != nil {
			top, right, bottom, left = max(0, overlayInt(m.Top)), max(0, overlayInt(m.Right)), max(0, overlayInt(m.Bottom)), max(0, overlayInt(m.Left))
		}
	}
	aw, ah := max(1, w-left-right), max(1, h-top-bottom)
	width = max(1, min(aw, max(overlayInt(o.MinWidth), overlayValue(o.Width, w, min(80, aw)))))
	height = contentHeight
	if o.MaxHeight != nil {
		height = min(height, max(1, min(ah, overlayValue(o.MaxHeight, h, ah))))
	}
	anchor := OverlayAnchorCenter
	if o.Anchor != nil {
		anchor = *o.Anchor
	}
	row, col = top+int(math.Floor(float64(ah-height)/2)), left+(aw-width)/2
	if strings.HasPrefix(string(anchor), "top") {
		row = top
	}
	if strings.HasPrefix(string(anchor), "bottom") {
		row = top + ah - height
	}
	if strings.Contains(string(anchor), "left") {
		col = left
	}
	if strings.Contains(string(anchor), "right") {
		col = left + aw - width
	}
	if o.Row != nil {
		row = overlayValue(o.Row, max(0, ah-height), row)
		if o.Row.Percentage != nil {
			row += top
		}
	}
	if o.Col != nil {
		col = overlayValue(o.Col, max(0, aw-width), col)
		if o.Col.Percentage != nil {
			col += left
		}
	}
	row = max(top, min(row+overlayInt(o.OffsetY), h-bottom-height))
	col = max(left, min(col+overlayInt(o.OffsetX), w-right-width))
	return
}
func (t *TUIBase) compositeOverlays(lines []string, w, h int) ([]string, error) {
	if len(t.overlays) == 0 {
		return lines, nil
	}
	if e := t.resizeFocus; e != nil && e.visible(w, h) {
		t.setFocus(e.component)
		t.resizeFocus = nil
	}
	for _, e := range t.overlays {
		if sameComponent(t.focusedComponent, e.component) && !e.visible(w, h) {
			t.resizeFocus = e
			t.restoreOverlayFocus(e)
		}
	}
	entries := append([]*overlayEntry(nil), t.overlays...)
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].order < entries[j].order })
	result := append([]string(nil), lines...)
	for len(result) < h {
		result = append(result, "")
	}
	for _, e := range entries {
		if !e.visible(w, h) {
			continue
		}
		width, _, _, _ := overlayLayout(e.options, w, h, 0)
		overlay, err := e.component.Render(width)
		if err != nil {
			return nil, err
		}
		_, height, row, col := overlayLayout(e.options, w, h, len(overlay))
		height = min(height, max(0, h-row))
		start := max(0, len(result)-h)
		for i := 0; i < height; i++ {
			at := start + row + i
			if at >= len(result) {
				break
			}
			base := result[at]
			result[at], err = CompositeTUILine(base, lineReset+normalizeRenderLine(overlay[i])+lineReset, col, width, w)
			if err != nil {
				return nil, err
			}
		}
	}
	return result, nil
}

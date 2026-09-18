package tui

import (
	"math"
	"regexp"
	"strconv"
	"strings"
)

type scrollMouse struct {
	drag, hover *ScrollView
	grab        int
}

var sgrMouse = regexp.MustCompile(`^\x1b\[<(\d+);(\d+);(\d+)([Mm])$`)

func (m *scrollMouse) handle(data string, frame LayoutFrame, primary *ScrollView, wheel int, keys *KeybindingsManager, kitty bool) (bool, error) {
	if data == "\x1b[O" {
		if m.hover != nil {
			_ = m.hover.SetScrollbarActive(false)
		}
		m.hover = nil
		m.drag = nil
		return true, nil
	}
	if data == "\x1b[I" {
		return true, nil
	}
	if primary == nil {
		return false, nil
	}
	if release, _ := IsKeyRelease(data); release {
		return true, nil
	}
	button, x, y, mouse, release := 0, 0, 0, false, false
	if v := sgrMouse.FindStringSubmatch(data); v != nil {
		button, _ = strconv.Atoi(v[1])
		x, _ = strconv.Atoi(v[2])
		y, _ = strconv.Atoi(v[3])
		x--
		y--
		mouse = true
		release = v[4] == "m"
	} else if len(data) == 6 && strings.HasPrefix(data, "\x1b[M") {
		button = int(data[3]) - 32
		x = int(data[4]) - 33
		y = int(data[5]) - 33
		mouse = true
		release = button&3 == 3
	}
	if mouse {
		hits, _ := GetScrollViewsAt(frame, x, y)
		if button&64 != 0 {
			direction := button & 3
			if direction > 1 {
				return true, nil
			}
			remaining := -wheel
			if direction == 1 {
				remaining = wheel
			}
			seen := false
			for _, s := range hits {
				seen = seen || s == primary
				remaining, _ = s.ScrollBy(remaining)
				if remaining == 0 || s.Overscroll == ScrollViewOverscrollContain {
					break
				}
			}
			if remaining != 0 && !seen {
				_, _ = primary.ScrollBy(remaining)
			}
			m.updateHover(frame, x, y)
			return true, nil
		}
		if m.drag != nil {
			if release {
				m.drag = nil
				m.updateHover(frame, x, y)
				return true, nil
			}
			box, _, _ := GetScrollViewBox(frame, m.drag)
			if g, ok, _ := GetScrollbarGeometry(box); ok {
				span := g.TrackHeight - g.ThumbHeight
				top := 0
				if span > 0 {
					top = int(math.Round(float64(max(0, min(span, y-g.TrackTop-m.grab))) * float64(g.MaxScrollTop) / float64(span)))
				}
				return true, m.drag.ScrollTo(top)
			}
			return true, nil
		}
		target, geometry := m.updateHover(frame, x, y)
		if target != nil && !release && button&32 == 0 && button&3 == 0 {
			m.drag = target
			m.grab = y - geometry.ThumbTop
		}
		return true, nil
	}
	match := func(k Keybinding) bool { return keys.matches(data, k, kitty) }
	switch {
	case match(KeybindingAltScreenTop):
		return true, primary.ScrollToStart()
	case match(KeybindingAltScreenBottom):
		return true, primary.ScrollToEnd()
	case match(KeybindingAltScreenPageUp):
		_, err := primary.ScrollBy(-max(1, primary.ViewportHeight()-4))
		return true, err
	case match(KeybindingAltScreenPageDown):
		_, err := primary.ScrollBy(max(1, primary.ViewportHeight()-4))
		return true, err
	case match(KeybindingAltScreenHalfPageUp):
		_, err := primary.ScrollBy(-max(1, primary.ViewportHeight()/2))
		return true, err
	case match(KeybindingAltScreenHalfPageDown):
		_, err := primary.ScrollBy(max(1, primary.ViewportHeight()/2))
		return true, err
	}
	return false, nil
}

func (m *scrollMouse) updateHover(frame LayoutFrame, x, y int) (*ScrollView, ScrollbarGeometry) {
	hits, _ := GetScrollViewsAt(frame, x, y)
	var target *ScrollView
	var geometry ScrollbarGeometry
	for _, s := range hits {
		box, _, _ := GetScrollViewBox(frame, s)
		if g, ok, _ := GetScrollbarGeometry(box); ok && x == g.Column && y >= g.ThumbTop && y < g.ThumbTop+g.ThumbHeight {
			target = s
			geometry = g
			_ = s.SetScrollbarActive(true)
			break
		}
	}
	if m.hover != target {
		if m.hover != nil {
			_ = m.hover.SetScrollbarActive(false)
		}
		m.hover = target
	}
	return target, geometry
}

package tui

import (
	"errors"
	"time"
)

func (*ScrollView) AddChild(Component) error { return errors.New("ScrollView has exactly one child") }
func (*ScrollView) RemoveChild(Component) error {
	return errors.New("ScrollView child cannot be removed")
}
func (*ScrollView) Clear() error { return errors.New("ScrollView child cannot be cleared") }
func (s *ScrollView) GetContentWidth(width int) (int, error) {
	if s.Scrollbar() == ScrollViewScrollbarAlways && width > 1 {
		width--
	}
	return width, nil
}
func (s *ScrollView) Invalidate() error {
	if s.child != nil {
		return s.child.Invalidate()
	}
	return nil
}
func (s *ScrollView) IsFollowingEnd() bool {
	s.scrollMu.Lock()
	defer s.scrollMu.Unlock()
	return s.followingEnd
}
func (s *ScrollView) IsScrollbarVisible() bool {
	s.scrollMu.Lock()
	defer s.scrollMu.Unlock()
	return s.scrollbar == ScrollViewScrollbarAlways && s.viewportHeight > 0 || s.scrollbar == ScrollViewScrollbarAuto && s.contentHeight > s.viewportHeight && s.transientScrollbarVisible
}
func (s *ScrollView) IsPrimary() bool                          { return s.Primary }
func (s *ScrollView) OverscrollBehavior() ScrollViewOverscroll { return s.Overscroll }
func (s *ScrollView) Render(width int) ([]string, error) {
	if s.axis != ScrollViewAxisVertical {
		return nil, errors.New("unsupported ScrollView axis")
	}
	if s.child == nil {
		return nil, nil
	}
	w, _ := s.GetContentWidth(width)
	lines, err := s.child.Render(w)
	if w != width {
		lines = append([]string(nil), lines...)
		for i := range lines {
			lines[i] += " "
		}
	}
	return lines, err
}
func (s *ScrollView) LayoutNode() LayoutNode {
	return ScrollLayoutNode{Type: LayoutNodeTypeScroll, Component: s.child, State: s}
}
func (s *ScrollView) ScrollTop() int {
	s.scrollMu.Lock()
	defer s.scrollMu.Unlock()
	return s.scrollTop
}
func (s *ScrollView) ViewportHeight() int {
	s.scrollMu.Lock()
	defer s.scrollMu.Unlock()
	return s.viewportHeight
}
func (s *ScrollView) Scrollbar() ScrollViewScrollbar {
	s.scrollMu.Lock()
	defer s.scrollMu.Unlock()
	return s.scrollbar
}
func (s *ScrollView) hideScrollbar() {
	s.transientScrollbarVisible = false
	if s.scrollbarTimer != nil {
		s.scrollbarTimer.Stop()
		s.scrollbarTimer = nil
	}
}
func (s *ScrollView) activity() {
	if s.scrollbar != ScrollViewScrollbarAuto || s.contentHeight <= s.viewportHeight {
		return
	}
	s.hideScrollbar()
	s.transientScrollbarVisible = true
	if !s.scrollbarActive {
		s.scrollbarTimer = time.AfterFunc(time.Duration(max(0, s.scrollbarHideDelayMS))*time.Millisecond, func() {
			s.scrollMu.Lock()
			s.transientScrollbarVisible = false
			callback := s.requestRender
			s.scrollMu.Unlock()
			if callback != nil {
				callback()
			}
		})
	}
}
func (s *ScrollView) SetScrollbar(mode ScrollViewScrollbar) error {
	s.scrollMu.Lock()
	if s.scrollbar == mode {
		s.scrollMu.Unlock()
		return nil
	}
	s.scrollbar = mode
	if mode != ScrollViewScrollbarAuto {
		s.hideScrollbar()
	} else if s.scrollbarActive {
		s.activity()
	}
	callback := s.requestRender
	s.scrollMu.Unlock()
	if callback != nil {
		callback()
	}
	return nil
}
func (s *ScrollView) SetScrollbarActive(active bool) error {
	s.scrollMu.Lock()
	defer s.scrollMu.Unlock()
	if s.scrollbarActive != active {
		s.scrollbarActive = active
		s.activity()
	}
	return nil
}
func (s *ScrollView) UpdateLayout(content, viewport int, callback func()) error {
	s.scrollMu.Lock()
	defer s.scrollMu.Unlock()
	s.contentHeight = max(0, content)
	s.viewportHeight = max(0, viewport)
	s.requestRender = callback
	limit := max(0, s.contentHeight-s.viewportHeight)
	if s.followingEnd {
		s.scrollTop = limit
	} else {
		s.scrollTop = max(0, min(s.scrollTop, limit))
	}
	if s.follow == ScrollViewFollowEnd && s.scrollTop == limit {
		s.followingEnd = true
	}
	if s.contentHeight <= s.viewportHeight {
		s.hideScrollbar()
	}
	return nil
}
func (s *ScrollView) ScrollBy(lines int) (int, error) {
	s.scrollMu.Lock()
	if lines == 0 {
		s.scrollMu.Unlock()
		return 0, nil
	}
	limit := max(0, s.contentHeight-s.viewportHeight)
	start := s.scrollTop
	if s.followingEnd {
		start = limit
	}
	next := max(0, min(limit, start+lines))
	moved := next - start
	s.scrollTop = next
	s.followingEnd = s.follow == ScrollViewFollowEnd && next == limit
	var callback func()
	if moved != 0 {
		s.activity()
		callback = s.requestRender
	}
	s.scrollMu.Unlock()
	if callback != nil {
		callback()
	}
	return lines - moved, nil
}
func (s *ScrollView) ScrollTo(top int) error { return s.scrollTo(top, false, false) }
func (s *ScrollView) ScrollToStart() error   { return s.scrollTo(0, true, false) }
func (s *ScrollView) ScrollToEnd() error     { return s.scrollTo(0, true, true) }
func (s *ScrollView) scrollTo(top int, force, end bool) error {
	s.scrollMu.Lock()
	limit := max(0, s.contentHeight-s.viewportHeight)
	if end {
		top = limit
	}
	next := max(0, min(top, limit))
	follow := s.follow == ScrollViewFollowEnd && next == limit
	changed := next != s.scrollTop || force && follow != s.followingEnd
	var callback func()
	if changed {
		s.scrollTop = next
		s.followingEnd = follow
		s.activity()
		callback = s.requestRender
	}
	s.scrollMu.Unlock()
	if callback != nil {
		callback()
	}
	return nil
}

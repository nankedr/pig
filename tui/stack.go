package tui

import "math"

func (s *Stack) AddChild(c Component, options ...StackEntryOptions) error {
	e := StackEntry{Component: c}
	if len(options) > 0 {
		e.StackEntryOptions = options[0]
	}
	s.entries = append(s.entries, e)
	return nil
}
func (s *Stack) RemoveChild(c Component) error {
	for i, e := range s.entries {
		if sameComponent(e.Component, c) {
			s.entries = append(s.entries[:i], s.entries[i+1:]...)
			break
		}
	}
	return nil
}
func (s *Stack) Clear() error { s.entries = nil; return nil }
func (s *Stack) Invalidate() error {
	for _, e := range s.entries {
		if err := e.Component.Invalidate(); err != nil {
			return err
		}
	}
	return nil
}
func VisibleStackEntries(entries []StackEntry, viewport LayoutViewport) ([]StackEntry, error) {
	out := []StackEntry{}
	for _, e := range entries {
		if e.Visible == nil || e.Visible(viewport) {
			out = append(out, e)
		}
	}
	return out, nil
}
func stackSize(p *int, fallback int) int {
	if p == nil {
		return fallback
	}
	return max(0, *p)
}
func AllocateStackSizes(entries []StackEntry, intrinsic []int, available *int, gap int) ([]int, error) {
	sizes := make([]int, len(entries))
	total := 0
	for i, e := range entries {
		n := 0
		if i < len(intrinsic) {
			n = intrinsic[i]
		}
		if e.Basis != nil && !e.Basis.Auto {
			n = e.Basis.Size
		}
		lo := stackSize(e.MinSize, 0)
		sizes[i] = max(lo, min(max(0, n), max(lo, stackSize(e.MaxSize, math.MaxInt))))
		total += sizes[i]
	}
	if available == nil {
		return sizes, nil
	}
	space := max(0, *available-max(0, len(entries)-1)*max(0, gap))
	grow := total < space
	remaining := total - space
	if grow {
		remaining = space - total
	}
	for remaining > 0 {
		indices := []int{}
		weight := 0
		for i, e := range entries {
			if grow && stackSize(e.Grow, 0) > 0 && sizes[i] < stackSize(e.MaxSize, math.MaxInt) {
				indices = append(indices, i)
				weight += stackSize(e.Grow, 0)
			} else if !grow && stackSize(e.Shrink, 1) > 0 && sizes[i] > stackSize(e.MinSize, 0) {
				indices = append(indices, i)
				weight += stackSize(e.Shrink, 1) * max(1, sizes[i])
			}
		}
		if len(indices) == 0 {
			break
		}
		distributed := 0
		for _, i := range indices {
			e := entries[i]
			w := stackSize(e.Grow, 0)
			capacity := stackSize(e.MaxSize, math.MaxInt) - sizes[i]
			if !grow {
				w = stackSize(e.Shrink, 1) * max(1, sizes[i])
				capacity = sizes[i] - stackSize(e.MinSize, 0)
			}
			delta := min(remaining, max(1, int(float64(remaining)*float64(w)/float64(weight))), capacity)
			if grow {
				sizes[i] += delta
			} else {
				sizes[i] -= delta
			}
			remaining -= delta
			distributed += delta
		}
		if distributed == 0 {
			break
		}
	}
	return sizes, nil
}
func (s *Stack) node(kind LayoutNodeType) LayoutNode {
	entries := make([]StackLayoutEntry, len(s.entries))
	for i, e := range s.entries {
		entries[i] = StackLayoutEntry{Component: e.Component, StackEntryOptions: e.StackEntryOptions}
	}
	return StackLayoutNode{Type: kind, Entries: entries, Gap: s.gap, Align: s.align}
}
func (s *VStack) LayoutNode() LayoutNode             { return s.node(LayoutNodeTypeVStack) }
func (s *HStack) LayoutNode() LayoutNode             { return s.node(LayoutNodeTypeHStack) }
func (s *Stack) Render(width int) ([]string, error)  { return s.renderStack(width, false) }
func (s *VStack) Render(width int) ([]string, error) { return s.renderStack(width, false) }
func (s *HStack) Render(width int) ([]string, error) { return s.renderStack(width, true) }
func (s *Stack) renderStack(width int, horizontal bool) ([]string, error) {
	width = max(1, width)
	entries, _ := VisibleStackEntries(s.entries, LayoutViewport{width, math.MaxInt})
	rendered := make([][]string, len(entries))
	intrinsic := make([]int, len(entries))
	for i, e := range entries {
		lines, err := e.Component.Render(width)
		if err != nil {
			return nil, err
		}
		rendered[i] = lines
		intrinsic[i] = len(lines)
		if horizontal {
			intrinsic[i] = 0
			for _, l := range lines {
				w, _ := VisibleWidth(l)
				intrinsic[i] = max(intrinsic[i], w)
			}
		}
	}
	var available *int
	if horizontal {
		available = &width
	}
	sizes, _ := AllocateStackSizes(entries, intrinsic, available, s.gap)
	if !horizontal {
		out := []string{}
		for i, lines := range rendered {
			if i > 0 {
				out = append(out, make([]string, s.gap)...)
			}
			out = append(out, lines[:min(len(lines), sizes[i])]...)
			out = append(out, make([]string, max(0, sizes[i]-len(lines)))...)
		}
		return out, nil
	}
	height := 0
	for i, e := range entries {
		rendered[i] = nil
		if sizes[i] > 0 {
			lines, err := e.Component.Render(sizes[i])
			if err != nil {
				return nil, err
			}
			rendered[i] = lines
		}
		height = max(height, len(rendered[i]))
	}
	out := make([]string, height)
	x := 0
	for i, lines := range rendered {
		offset := 0
		if s.align == StackAlignCenter {
			offset = (height - len(lines)) / 2
		} else if s.align == StackAlignEnd {
			offset = height - len(lines)
		}
		for row, line := range lines {
			out[row+offset], _ = CompositeTUILine(out[row+offset], line, x, sizes[i], width)
		}
		x += sizes[i] + s.gap
	}
	return out, nil
}

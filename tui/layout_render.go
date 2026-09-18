package tui

import (
	"github.com/rivo/uniseg"
	"math"
	"strings"
)

func sliceCells(line string, start, width int) string {
	var out strings.Builder
	end := start + max(0, width)
	column := 0
	styled := false
	for len(line) > 0 {
		if code, ok, _ := ExtractANSICode(line, 0); ok {
			if code.Code == CursorMarker {
				if column >= start && column < end {
					out.WriteString(code.Code)
				}
			} else if strings.HasPrefix(code.Code, "\x1b[") && strings.HasSuffix(code.Code, "m") && column < end {
				out.WriteString(code.Code)
				styled = true
			}
			line = line[code.Length:]
			continue
		}
		g := uniseg.NewGraphemes(line)
		g.Next()
		w := g.Width()
		if column >= start && column+w <= end {
			out.WriteString(g.Str())
		} else if column < end && column+w > start {
			out.WriteString(strings.Repeat(" ", max(0, min(end, column+w)-max(start, column))))
		}
		column += w
		line = line[len(g.Str()):]
		if column > end {
			break
		}
	}
	if styled {
		out.WriteString("\x1b[0m")
	}
	return out.String()
}
func CompositeTUILine(base, overlay string, start, width, total int) (string, error) {
	total = max(0, total)
	left := max(0, start)
	right := min(total, start+max(0, width))
	if right <= left {
		return sliceCells(base, 0, total), nil
	}
	before := sliceCells(base, 0, left)
	w, _ := VisibleWidth(before)
	before += strings.Repeat(" ", max(0, left-w))
	middle := sliceCells(overlay, max(0, -start), right-left)
	w, _ = VisibleWidth(middle)
	middle += strings.Repeat(" ", max(0, right-left-w))
	return before + middle + sliceCells(base, right, total-right), nil
}
func intersectRect(a, b LayoutRect) LayoutRect {
	x, y := max(a.X, b.X), max(a.Y, b.Y)
	return LayoutRect{x, y, max(0, min(a.X+a.Width, b.X+b.Width)-x), max(0, min(a.Y+a.Height, b.Y+b.Height)-y)}
}
func translateBox(box *LayoutBox, dy int) {
	box.Rect.Y += dy
	for _, c := range box.Children {
		translateBox(c, dy)
	}
}
func clipBox(box *LayoutBox, clip LayoutRect) {
	box.Clip = intersectRect(box.Rect, clip)
	for _, c := range box.Children {
		clipBox(c, box.Clip)
	}
}

type layoutRender struct {
	viewport LayoutViewport
	request  func()
	primary  *ScrollView
	cache    []layoutCache
}
type layoutCache struct {
	component Component
	width     int
	lines     []string
}

func (r *layoutRender) render(c Component, width int) ([]string, error) {
	width = max(1, width)
	for _, v := range r.cache {
		if v.width == width && sameComponent(v.component, c) {
			return v.lines, nil
		}
	}
	if c == nil {
		return nil, nil
	}
	lines, err := c.Render(width)
	if err == nil {
		r.cache = append(r.cache, layoutCache{c, width, lines})
	}
	return lines, err
}
func (r *layoutRender) layout(c Component, x, y, width int, height *int, clip LayoutRect) (*LayoutBox, error) {
	width = max(1, width)
	box := &LayoutBox{Component: c, Rect: LayoutRect{X: x, Y: y, Width: width}}
	node, ok := GetLayoutNode(c)
	if !ok {
		lines, err := r.render(c, width)
		if err != nil {
			return nil, err
		}
		box.Lines = lines
		box.Rect.Height = len(lines)
		if height != nil {
			box.Rect.Height = max(0, *height)
		}
		offset := 0
		if len(lines) > box.Rect.Height && box.Rect.Height > 0 {
			for i, l := range lines {
				if strings.Contains(l, CursorMarker) {
					if i >= box.Rect.Height {
						offset = i - box.Rect.Height + 1
					}
					break
				}
			}
		}
		box.LineOffset = &offset
		box.Clip = intersectRect(clip, box.Rect)
		return box, nil
	}
	if n, ok := node.(ScrollLayoutNode); ok {
		old := n.State.ScrollTop()
		cw, err := n.State.GetContentWidth(width)
		if err != nil {
			return nil, err
		}
		child, err := r.layout(n.Component, x, y-old, cw, nil, clip)
		if err != nil {
			return nil, err
		}
		h := child.Rect.Height
		if height != nil {
			h = max(0, *height)
		}
		if err = n.State.UpdateLayout(child.Rect.Height, h, r.request); err != nil {
			return nil, err
		}
		translateBox(child, old-n.State.ScrollTop())
		box.Rect.Height = h
		box.ScrollView, _ = n.State.(*ScrollView)
		if box.ScrollView != nil && (n.State.IsPrimary() || r.primary == nil) {
			r.primary = box.ScrollView
		}
		box.ScrollContentLines, err = r.render(n.Component, cw)
		if err != nil {
			return nil, err
		}
		child.Parent = box
		box.Children = []*LayoutBox{child}
		clipBox(box, clip)
		return box, nil
	}
	n, ok := node.(StackLayoutNode)
	if !ok {
		return box, nil
	}
	entries := []StackEntry{}
	for _, e := range n.Entries {
		if e.Visible == nil || e.Visible(r.viewport) {
			entries = append(entries, StackEntry{Component: e.Component, StackEntryOptions: e.StackEntryOptions})
		}
	}
	intrinsic := make([]int, len(entries))
	vertical := n.Type == LayoutNodeTypeVStack
	for i, e := range entries {
		if e.Basis != nil && !e.Basis.Auto {
			intrinsic[i] = e.Basis.Size
			continue
		}
		lines, err := r.render(e.Component, width)
		if err != nil {
			return nil, err
		}
		if vertical {
			intrinsic[i] = len(lines)
		} else {
			for _, l := range lines {
				w, _ := VisibleWidth(l)
				intrinsic[i] = max(intrinsic[i], w)
			}
		}
	}
	available := height
	if !vertical {
		available = &width
	}
	sizes, _ := AllocateStackSizes(entries, intrinsic, available, n.Gap)
	heights := make([]int, len(entries))
	h := max(0, len(entries)-1) * n.Gap
	if vertical {
		for _, v := range sizes {
			h += v
		}
	} else {
		h = 0
		for i, e := range entries {
			lines, err := r.render(e.Component, max(1, sizes[i]))
			if err != nil {
				return nil, err
			}
			heights[i] = len(lines)
			h = max(h, len(lines))
		}
	}
	if height != nil {
		h = max(0, *height)
	}
	box.Rect.Height = h
	box.Clip = intersectRect(clip, box.Rect)
	cx, cy := x, y
	for i, e := range entries {
		cw, ch := width, sizes[i]
		if !vertical {
			cw = sizes[i]
			ch = h
			cy = y
			if n.Align != StackAlignStretch {
				ch = min(h, heights[i])
			}
			if n.Align == StackAlignCenter {
				cy += (h - ch) / 2
			} else if n.Align == StackAlignEnd {
				cy += h - ch
			}
		}
		var child *LayoutBox
		var err error
		if cw == 0 {
			child = &LayoutBox{Component: e.Component, Rect: LayoutRect{cx, cy, 0, ch}, Clip: LayoutRect{cx, cy, 0, 0}}
		} else {
			child, err = r.layout(e.Component, cx, cy, cw, &ch, box.Clip)
		}
		if err != nil {
			return nil, err
		}
		child.Parent = box
		box.Children = append(box.Children, child)
		if vertical {
			cy += ch + n.Gap
		} else {
			cx += cw + n.Gap
		}
	}
	return box, nil
}
func GetScrollbarGeometry(box *LayoutBox) (ScrollbarGeometry, bool, error) {
	if box == nil || box.ScrollView == nil || !box.ScrollView.IsScrollbarVisible() || box.Rect.Width <= 0 || box.Rect.Height <= 0 {
		return ScrollbarGeometry{}, false, nil
	}
	content := len(box.ScrollContentLines)
	if len(box.Children) > 0 {
		content = box.Children[0].Rect.Height
	}
	h := box.Rect.Height
	thumb := h
	if content > 0 {
		thumb = max(min(2, h), min(h, int(math.Round(float64(h)*float64(h)/float64(content)))))
	}
	limit := max(0, content-h)
	offset := 0
	if limit > 0 {
		offset = int(math.Round(float64(box.ScrollView.ScrollTop()) / float64(limit) * float64(h-thumb)))
	}
	col := box.Rect.X + box.Rect.Width - 1
	if col < box.Clip.X || col >= box.Clip.X+box.Clip.Width {
		return ScrollbarGeometry{}, false, nil
	}
	return ScrollbarGeometry{col, box.Rect.Y, h, box.Rect.Y + offset, thumb, limit}, true, nil
}
func paintLayout(box *LayoutBox, screen []string, width int) {
	offset := 0
	if box.LineOffset != nil {
		offset = *box.LineOffset
	}
	for row := max(0, box.Clip.Y); row < min(len(screen), box.Clip.Y+box.Clip.Height); row++ {
		index := offset + row - box.Rect.Y
		if index < 0 || index >= len(box.Lines) {
			continue
		}
		start := max(0, box.Clip.X)
		end := min(width, box.Clip.X+box.Clip.Width)
		if end <= start {
			continue
		}
		line := sliceCells(box.Lines[index], start-box.Rect.X, end-start)
		screen[row], _ = CompositeTUILine(screen[row], line, start, end-start, width)
	}
	for _, child := range box.Children {
		paintLayout(child, screen, width)
	}
	if g, ok, _ := GetScrollbarGeometry(box); ok {
		for row := max(0, g.ThumbTop, box.Clip.Y); row < min(len(screen), g.ThumbTop+g.ThumbHeight, box.Clip.Y+box.Clip.Height); row++ {
			start, end := graphemeCells(screen[row], g.Column)
			cell := sliceCells(screen[row], start, end-start)
			if cell == "" {
				cell = " "
			}
			if box.ScrollView.ScrollbarStyle != nil {
				cell = box.ScrollView.ScrollbarStyle(cell)
			} else {
				cell = "\x1b[100m" + cell + "\x1b[49m"
			}
			screen[row], _ = CompositeTUILine(screen[row], cell, start, end-start, width)
		}
	}
}
func RenderLayoutFrame(root Component, width, height int, request func()) (LayoutFrame, error) {
	width, height = max(1, width), max(1, height)
	r := layoutRender{viewport: LayoutViewport{width, height}, request: request}
	box, err := r.layout(root, 0, 0, width, &height, LayoutRect{0, 0, width, height})
	if err != nil {
		return LayoutFrame{}, err
	}
	lines := make([]string, height)
	paintLayout(box, lines, width)
	return LayoutFrame{Root: box, Width: width, Height: height, Lines: lines, PrimaryScrollView: r.primary}, nil
}
func GetScrollViewBox(frame LayoutFrame, scroll *ScrollView) (*LayoutBox, bool, error) {
	var visit func(*LayoutBox) *LayoutBox
	visit = func(b *LayoutBox) *LayoutBox {
		if b == nil {
			return nil
		}
		if b.ScrollView == scroll {
			return b
		}
		for _, c := range b.Children {
			if found := visit(c); found != nil {
				return found
			}
		}
		return nil
	}
	box := visit(frame.Root)
	return box, box != nil, nil
}
func GetScrollViewsAt(frame LayoutFrame, x, y int) ([]*ScrollView, error) {
	out := []*ScrollView{}
	contains := func(r LayoutRect) bool { return x >= r.X && x < r.X+r.Width && y >= r.Y && y < r.Y+r.Height }
	var visit func(*LayoutBox)
	visit = func(b *LayoutBox) {
		if b == nil || !contains(b.Clip) {
			return
		}
		for _, c := range b.Children {
			visit(c)
		}
		if b.ScrollView != nil && contains(b.Rect) {
			out = append(out, b.ScrollView)
		}
	}
	visit(frame.Root)
	return out, nil
}

func graphemeCells(line string, column int) (int, int) {
	pos := 0
	for len(line) > 0 {
		if code, ok, _ := ExtractANSICode(line, 0); ok {
			line = line[code.Length:]
			continue
		}
		g := uniseg.NewGraphemes(line)
		g.Next()
		end := pos + g.Width()
		if column >= pos && column < end {
			return pos, end
		}
		pos = end
		line = line[len(g.Str()):]
	}
	return column, column + 1
}

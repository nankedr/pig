package tui

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf16"

	"github.com/rivo/uniseg"
)

func WordWrapLine(line string, width int, preSegmented ...[]UnicodeSegment) ([]TextChunk, error) {
	if line == "" || width <= 0 {
		return []TextChunk{{}}, nil
	}
	if uniseg.StringWidth(line) <= width {
		return []TextChunk{{line, 0, len(line)}}, nil
	}
	segments := (&Editor{}).segments(line)
	if len(preSegmented) > 0 {
		segments = preSegmented[0]
	}
	var chunks []TextChunk
	start, used, opportunity, opportunityWidth := 0, 0, -1, 0
	for i, seg := range segments {
		w := uniseg.StringWidth(seg.Text)
		if used+w > width {
			if opportunity >= 0 && used-opportunityWidth+w <= width {
				chunks = append(chunks, TextChunk{line[start:opportunity], start, opportunity})
				start = opportunity
				used -= opportunityWidth
			} else if start < seg.ByteOffset {
				chunks = append(chunks, TextChunk{line[start:seg.ByteOffset], start, seg.ByteOffset})
				start = seg.ByteOffset
				used = 0
			}
			opportunity = -1
		}
		if w > width && uniseg.GraphemeClusterCount(seg.Text) > 1 {
			parts, _ := WordWrapLine(seg.Text, width)
			for _, part := range parts[:len(parts)-1] {
				chunks = append(chunks, TextChunk{part.Text, seg.ByteOffset + part.StartIndex, seg.ByteOffset + part.EndIndex})
			}
			last := parts[len(parts)-1]
			start = seg.ByteOffset + last.StartIndex
			used = uniseg.StringWidth(last.Text)
			opportunity = -1
			continue
		}
		used += w
		if i+1 < len(segments) {
			next := segments[i+1]
			if (strings.TrimSpace(seg.Text) == "" && strings.TrimSpace(next.Text) != "") || (strings.TrimSpace(seg.Text) != "" && strings.TrimSpace(next.Text) != "" && (cjk(seg.Text) || cjk(next.Text))) {
				opportunity = next.ByteOffset
				opportunityWidth = used
			}
		}
	}
	return append(chunks, TextChunk{line[start:], start, len(line)}), nil
}
func cjk(text string) bool {
	for _, r := range text {
		if unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul, unicode.Bopomofo) {
			return true
		}
	}
	return false
}

type editorVisualLine struct {
	text       string
	start, end int
	last       bool
}

func (e *Editor) layout(width int) ([]editorVisualLine, int) {
	var rows []editorVisualLine
	base, current := 0, 0
	for _, line := range e.GetLines() {
		chunks, _ := WordWrapLine(line, max(1, width), e.segments(line))
		for i, chunk := range chunks {
			row := editorVisualLine{chunk.Text, base + chunk.StartIndex, base + chunk.EndIndex, i == len(chunks)-1}
			if e.cursor >= row.start && (e.cursor < row.end || row.last && e.cursor == row.end) {
				current = len(rows)
			}
			rows = append(rows, row)
		}
		base += len(line) + 1
	}
	return rows, current
}
func (e *Editor) visibleRows() int {
	rows := e.terminalRows
	if rows <= 0 {
		rows = 32
	}
	if e.ui != nil && e.ui.Terminal() != nil {
		if n, err := e.ui.Terminal().Rows(); err == nil {
			rows = n
		}
	}
	return max(5, rows*3/10)
}
func (e *Editor) Render(width int) ([]string, error) {
	width = max(1, width)
	padding := min(max(0, e.paddingX), max(0, (width-1)/2))
	contentWidth := width - 2*padding
	e.width = contentWidth
	if padding == 0 {
		e.width = max(1, e.width-1)
	}
	rows, current := e.layout(e.width)
	visible := e.visibleRows()
	if current < e.scroll {
		e.scroll = current
	} else if current >= e.scroll+visible {
		e.scroll = current - visible + 1
	}
	e.scroll = max(0, min(e.scroll, len(rows)-visible))
	border := func(direction string, count int) string {
		line := strings.Repeat("─", width)
		if count > 0 {
			line = fmt.Sprintf("─── %s %d more ", direction, count)
			w := uniseg.StringWidth(line)
			if w <= width {
				line += strings.Repeat("─", width-w)
			} else {
				r := []rune(line)
				line = string(r[:max(0, width-3)]) + strings.Repeat(".", min(3, width))
			}
		}
		if e.BorderColor != nil {
			line = e.BorderColor(line)
		}
		return line
	}
	result := []string{border("↑", e.scroll)}
	for i := e.scroll; i < min(len(rows), e.scroll+visible); i++ {
		row := rows[i]
		text := terminalText(row.text)
		used := uniseg.StringWidth(text)
		if i == current {
			pos := e.cursor - row.start
			before, after := row.text[:pos], row.text[pos:]
			marker := ""
			if e.Focused {
				marker = CursorMarker
			}
			first := " "
			rest := ""
			if after != "" {
				parts := e.segments(after)
				first = parts[0].Text
				rest = after[len(first):]
			} else {
				used++
			}
			text = terminalText(before) + marker + "\x1b[7m" + terminalText(first) + "\x1b[0m" + terminalText(rest)
		}
		result = append(result, strings.Repeat(" ", padding)+text+strings.Repeat(" ", max(0, contentWidth-used))+strings.Repeat(" ", max(0, padding-max(0, used-contentWidth))))
	}
	return append(result, border("↓", max(0, len(rows)-e.scroll-visible))), nil
}
func (e *Editor) vertical(direction int) {
	rows, current := e.layout(e.width)
	if direction < 0 && current == 0 {
		if e.text == "" || e.historyIndex >= 0 || e.GetCursor().Col == 0 {
			e.navigateHistory(-1)
		} else {
			e.move(strings.LastIndex(e.text[:e.cursor], "\n") + 1)
		}
		return
	}
	if direction > 0 && current == len(rows)-1 {
		if e.historyIndex >= 0 {
			e.navigateHistory(1)
		} else {
			e.move(e.lineEnd())
		}
		return
	}
	e.moveVisual(rows, current, max(0, min(len(rows)-1, current+direction)))
}
func (e *Editor) moveVisual(rows []editorVisualLine, current, target int) {
	e.lastAction = ""
	if e.preferredCol < 0 {
		e.preferredCol = len(utf16.Encode([]rune(e.text[rows[current].start:e.cursor])))
	}
	row := rows[target]
	limit := len(utf16.Encode([]rune(row.text)))
	if !row.last {
		limit = max(0, limit-1)
	}
	desired := min(e.preferredCol, limit)
	pos, units := row.start, 0
	for _, r := range row.text {
		n := 1
		if r > 0xffff {
			n = 2
		}
		if units+n > desired {
			break
		}
		units += n
		pos += len(string(r))
	}
	e.cursor = pos
	for _, seg := range e.segments(e.text) {
		if seg.ByteOffset > pos {
			break
		}
		if pos < seg.ByteOffset+len(seg.Text) {
			e.cursor = seg.ByteOffset
			break
		}
	}
}

func (e *Editor) page(direction int) {
	rows, current := e.layout(e.width)
	e.moveVisual(rows, current, max(0, min(len(rows)-1, current+direction*e.visibleRows())))
}

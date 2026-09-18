package tui

import (
	"fmt"
	"os"
	"strings"
)

const lineReset = "\x1b[0m\x1b]8;;\x07"

func moveRows(delta int) string {
	if delta > 0 {
		return fmt.Sprintf("\x1b[%dB", delta)
	}
	if delta < 0 {
		return fmt.Sprintf("\x1b[%dA", -delta)
	}
	return ""
}

func mainScreenFrame(previous TUIMainScreenRenderState, lines []string, width, height int, showCursor, clearOnShrink bool) (string, TUIMainScreenRenderState, bool) {
	width, height = max(1, width), max(1, height)
	state := previous
	row, col := -1, 0
	next := make([]string, len(lines))
	for i, line := range lines {
		if at := strings.Index(line, CursorMarker); at >= 0 && i >= len(lines)-height {
			row = i
			col, _ = VisibleWidth(line[:at])
		}
		next[i] = normalizeRenderLine(strings.ReplaceAll(line, CursorMarker, "")) + lineReset
	}
	widthChanged := previous.PreviousWidth != 0 && previous.PreviousWidth != width
	heightChanged := previous.PreviousHeight != 0 && previous.PreviousHeight != height
	top := previous.PreviousViewportTop
	if heightChanged && previous.PreviousHeight > 0 {
		top = max(0, top+previous.PreviousHeight-height)
	}
	position := func() string {
		if row < 0 || len(next) == 0 {
			return "\x1b[?25l"
		}
		result := moveRows(row-state.HardwareCursorRow) + fmt.Sprintf("\x1b[%dG", col+1)
		state.HardwareCursorRow = row
		if showCursor {
			return result + "\x1b[?25h"
		}
		return result + "\x1b[?25l"
	}
	save := func() { state.PreviousLines = next; state.PreviousWidth = width; state.PreviousHeight = height }
	full := func(clear bool) (string, TUIMainScreenRenderState, bool) {
		output := "\x1b[?2026h"
		if clear {
			output += "\x1b[2J\x1b[H\x1b[3J"
			state.MaxLinesRendered = len(next)
		} else {
			state.MaxLinesRendered = max(state.MaxLinesRendered, len(next))
		}
		output += strings.Join(next, "\r\n") + "\x1b[?2026l"
		state.CursorRow = max(0, len(next)-1)
		state.HardwareCursorRow = state.CursorRow
		state.PreviousViewportTop = max(0, len(next)-height)
		output += position()
		save()
		return output, state, true
	}
	if len(previous.PreviousLines) == 0 && !widthChanged && !heightChanged {
		return full(false)
	}
	if widthChanged || heightChanged && os.Getenv("TERMUX_VERSION") == "" || clearOnShrink && len(next) < previous.MaxLinesRendered {
		return full(true)
	}
	first, last := -1, -1
	for i := 0; i < max(len(next), len(previous.PreviousLines)); i++ {
		old, newLine := "", ""
		if i < len(previous.PreviousLines) {
			old = previous.PreviousLines[i]
		}
		if i < len(next) {
			newLine = next[i]
		}
		if old != newLine {
			if first < 0 {
				first = i
			}
			last = i
		}
	}
	appended := len(next) > len(previous.PreviousLines)
	if appended {
		if first < 0 {
			first = len(previous.PreviousLines)
		}
		last = len(next) - 1
	}
	if first < 0 {
		output := position()
		state.PreviousViewportTop = top
		state.PreviousHeight = height
		return output, state, false
	}
	if first >= len(next) {
		target := max(0, len(next)-1)
		extra := len(previous.PreviousLines) - len(next)
		if target < top || extra > height {
			return full(true)
		}
		output := "\x1b[?2026h" + moveRows(target-state.HardwareCursorRow) + "\r"
		start := 0
		if len(next) > 0 {
			start = 1
			output += moveRows(1)
		}
		for i := 0; i < extra; i++ {
			output += "\r\x1b[2K"
			if i < extra-1 {
				output += moveRows(1)
			}
		}
		output += moveRows(-max(0, extra-1+start)) + "\x1b[?2026l"
		state.CursorRow = target
		state.HardwareCursorRow = target
		state.PreviousViewportTop = top
		output += position()
		save()
		return output, state, false
	}
	if first < top {
		return full(true)
	}
	appendStart := appended && first == len(previous.PreviousLines) && first > 0
	target := first
	if appendStart {
		target--
	}
	var output strings.Builder
	output.WriteString("\x1b[?2026h")
	if target > top+height-1 {
		output.WriteString(moveRows(height - 1 - max(0, min(height-1, state.HardwareCursorRow-top))))
		scroll := target - (top + height - 1)
		output.WriteString(strings.Repeat("\r\n", scroll))
		top += scroll
		state.HardwareCursorRow = target
	}
	output.WriteString(moveRows(target-state.HardwareCursorRow) + "\r")
	if appendStart {
		output.WriteString("\n")
	}
	end := min(last, len(next)-1)
	for i := first; i <= end; i++ {
		if i > first {
			output.WriteString("\r\n")
		}
		output.WriteString("\x1b[2K" + next[i])
	}
	final := end
	if len(previous.PreviousLines) > len(next) {
		output.WriteString(moveRows(len(next) - 1 - end))
		final = len(next) - 1
		extra := len(previous.PreviousLines) - len(next)
		output.WriteString(strings.Repeat("\r\n\x1b[2K", extra) + moveRows(-extra))
	}
	output.WriteString("\x1b[?2026l")
	state.CursorRow = max(0, len(next)-1)
	state.HardwareCursorRow = final
	state.MaxLinesRendered = max(state.MaxLinesRendered, len(next))
	state.PreviousViewportTop = max(top, final-height+1)
	output.WriteString(position())
	save()
	return output.String(), state, false
}

func mainScreenStop(state TUIMainScreenRenderState) string {
	if len(state.PreviousLines) == 0 {
		return ""
	}
	return " " + moveRows(len(state.PreviousLines)-state.HardwareCursorRow) + "\r\n"
}

func normalizeRenderLine(line string) string {
	line = strings.ReplaceAll(strings.ReplaceAll(line, "ำ", "ํา"), "ຳ", "ໍາ")
	if !strings.Contains(line, "\t") {
		return line
	}
	var output strings.Builder
	for len(line) > 0 {
		if code, ok, _ := ExtractANSICode(line, 0); ok {
			output.WriteString(code.Code)
			line = line[code.Length:]
			continue
		}
		if line[0] == '\t' {
			output.WriteString("   ")
		} else {
			output.WriteByte(line[0])
		}
		line = line[1:]
	}
	return output.String()
}

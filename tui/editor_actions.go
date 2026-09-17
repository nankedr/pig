package tui

import (
	"fmt"
	"maps"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/rivo/uniseg"
)

type editorSnapshot struct {
	text         string
	cursor       int
	pastes       map[string]string
	pasteCounter int
}

func (e *Editor) state() editorSnapshot {
	return editorSnapshot{e.text, e.cursor, maps.Clone(e.pastes), e.pasteCounter}
}
func (e *Editor) snapshot() {
	e.undo = append(e.undo, e.state())
}
func (e *Editor) restore(s editorSnapshot) {
	e.text = s.text
	e.cursor = s.cursor
	e.pastes = maps.Clone(s.pastes)
	e.pasteCounter = s.pasteCounter
	e.preferredCol = -1
	e.changed()
}
func (e *Editor) restoreUndo() {
	e.exitHistory()
	e.lastAction = ""
	if len(e.undo) > 0 {
		s := e.undo[len(e.undo)-1]
		e.undo = e.undo[:len(e.undo)-1]
		e.restore(s)
	}
}
func (e *Editor) exitHistory() { e.historyIndex = -1; e.draft = editorSnapshot{} }
func (e *Editor) move(pos int) { e.cursor = pos; e.lastAction = ""; e.preferredCol = -1 }
func (e *Editor) delete(start, end int, kill bool) {
	if start != end {
		e.snapshot()
		if kill {
			e.kills.Push(e.text[start:end], KillRingPushOptions{Prepend: start < e.cursor, Accumulate: e.lastAction == "kill"})
		}
	}
	e.exitHistory()
	e.lastAction = ""
	if kill {
		e.lastAction = "kill"
	}
	if !kill && start < end && end == e.cursor {
		if _, ok := e.pastes[e.text[start:end]]; ok {
			e.removePaste(start, end)
			return
		}
	}
	e.replace(start, end, "")
}
func (e *Editor) yank(pop bool) {
	if pop && (e.lastAction != "yank" || e.kills.Length() < 2) {
		return
	}
	if _, ok := e.kills.Peek(); !ok {
		return
	}
	e.snapshot()
	e.exitHistory()
	if pop {
		e.text = e.text[:e.yankStart] + e.text[e.yankEnd:]
		e.cursor = e.yankStart
		e.changed()
		e.kills.Rotate()
	}
	text, _ := e.kills.Peek()
	e.yankStart = e.cursor
	e.replace(e.cursor, e.cursor, text)
	e.yankEnd = e.cursor
	e.lastAction = "yank"
}
func wordClass(r rune) int {
	if unicode.IsSpace(r) {
		return 0
	}
	if unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.IsMark(r) {
		return 1
	}
	return 2
}
func (e *Editor) word(direction int) int {
	text := e.text[e.cursor:]
	if direction < 0 {
		text = e.text[:e.cursor]
	}
	segments := e.segments(text)
	index, step := 0, 1
	if direction < 0 {
		index, step = len(segments)-1, -1
	}
	pos, class := e.cursor, -1
	for index >= 0 && index < len(segments) {
		seg := segments[index]
		r, _ := utf8.DecodeRuneInString(seg.Text)
		if r == '\n' {
			if pos == e.cursor {
				return pos + direction
			}
			break
		}
		kind := wordClass(r)
		if _, ok := e.pastes[seg.Text]; ok {
			kind = 3
		}
		if class < 0 {
			if kind != 0 {
				class = kind
			}
		} else if kind != class || class == 3 {
			break
		}
		pos += direction * len(seg.Text)
		index += step
	}
	return pos
}
func (e *Editor) navigateHistory(direction int) {
	next := e.historyIndex - direction
	e.lastAction = ""
	if next < -1 || next >= len(e.history) {
		return
	}
	if e.historyIndex == -1 {
		e.snapshot()
		e.draft = e.state()
	}
	e.historyIndex = next
	if next == -1 {
		e.restore(e.draft)
		e.draft = editorSnapshot{}
		return
	}
	e.text = e.history[next]
	e.cursor = 0
	if direction > 0 {
		e.cursor = len(e.text)
	}
	e.preferredCol = -1
	e.changed()
}

var pasteControl = regexp.MustCompile(`\[(\d+);5u`)
var pasteMarker = regexp.MustCompile(`\[paste #(\d+)( (\+\d+ lines|\d+ chars))?\]`)

func (e *Editor) handlePasteInput(data string) error {
	if !e.inPaste {
		n := strings.Index(data, "\x1b[200~")
		if n > 0 {
			if err := e.HandleInput(data[:n]); err != nil {
				return err
			}
		}
		e.inPaste = true
		e.pasteBuffer = ""
		data = data[n+6:]
	}
	e.pasteBuffer += data
	end := strings.Index(e.pasteBuffer, "\x1b[201~")
	if end < 0 {
		return nil
	}
	text := e.pasteBuffer[:end]
	remaining := e.pasteBuffer[end+6:]
	e.inPaste = false
	e.pasteBuffer = ""
	if text != "" {
		e.snapshot()
		e.exitHistory()
		e.lastAction = ""
		text = pasteControl.ReplaceAllStringFunc(text, func(seq string) string {
			m := pasteControl.FindStringSubmatch(seq)
			cp, _ := strconv.Atoi(m[1])
			if cp >= 97 && cp <= 122 {
				return string(rune(cp - 96))
			}
			if cp >= 65 && cp <= 90 {
				return string(rune(cp - 64))
			}
			return seq
		})
		text = strings.Map(func(r rune) rune {
			if r == '\n' || r >= 32 {
				return r
			}
			return -1
		}, normalizeEditorText(text))
		if text != "" && strings.ContainsRune("/~.", rune(text[0])) && e.cursor > 0 {
			r, _ := utf8.DecodeLastRuneInString(e.text[:e.cursor])
			if r == '_' || r < 128 && (unicode.IsLetter(r) || unicode.IsDigit(r)) {
				text = " " + text
			}
		}
		lines := strings.Count(text, "\n") + 1
		chars := len(utf16.Encode([]rune(text)))
		if lines > 10 || chars > 1000 {
			e.pasteCounter++
			marker := fmt.Sprintf("[paste #%d %d chars]", e.pasteCounter, chars)
			if lines > 10 {
				marker = fmt.Sprintf("[paste #%d +%d lines]", e.pasteCounter, lines)
			}
			if e.pastes == nil {
				e.pastes = map[string]string{}
			}
			e.pastes[marker] = text
			text = marker
		}
		e.replace(e.cursor, e.cursor, text)
	}
	if remaining != "" {
		return e.HandleInput(remaining)
	}
	return nil
}
func (e *Editor) segments(text string) []UnicodeSegment {
	var result []UnicodeSegment
	for pos := 0; pos < len(text); {
		var marker []int
		if strings.HasPrefix(text[pos:], "[paste #") {
			marker = pasteMarker.FindStringIndex(text[pos:])
		}
		if marker != nil && marker[0] == 0 {
			value := text[pos : pos+marker[1]]
			if _, ok := e.pastes[value]; ok {
				result = append(result, UnicodeSegment{Text: value, ByteOffset: pos})
				pos += len(value)
				continue
			}
		}
		g := uniseg.NewGraphemes(text[pos:])
		g.Next()
		value := g.Str()
		result = append(result, UnicodeSegment{Text: value, ByteOffset: pos})
		pos += len(value)
	}
	return result
}

func (e *Editor) removePaste(start, end int) {
	marker := e.text[start:end]
	parts := pasteMarker.FindStringSubmatch(marker)
	id, _ := strconv.Atoi(parts[1])
	delete(e.pastes, marker)
	e.pasteCounter--
	renumber := func(marker string) string {
		parts := pasteMarker.FindStringSubmatch(marker)
		n, _ := strconv.Atoi(parts[1])
		if n <= id {
			return marker
		}
		return fmt.Sprintf("[paste #%d%s]", n-1, parts[2])
	}
	registry := map[string]string{}
	for marker, text := range e.pastes {
		registry[renumber(marker)] = text
	}
	e.pastes = registry
	before := pasteMarker.ReplaceAllStringFunc(e.text[:start], renumber)
	after := pasteMarker.ReplaceAllStringFunc(e.text[end:], renumber)
	e.text = before + after
	e.cursor = len(before)
	e.preferredCol = -1
	e.changed()
}

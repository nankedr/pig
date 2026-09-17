package tui

import "strings"

// EditorComponent is the extension seam for custom editors. Optional upstream
// capabilities are expressed by the smaller interfaces below so a custom Go
// editor only implements the features it supports.
type EditorComponent interface {
	Component
	GetText() string
	SetText(string) error
	HandleInput(string) error
}

// EditorSubmitSetter is implemented by editors whose optional submit callback
// can be wired by a host such as the Coding Agent. Setting the callback is a
// local state operation and does not invoke it.
type EditorSubmitSetter interface {
	SetOnSubmit(func(string))
}

// EditorChangeSetter is implemented by editors whose optional change callback
// can be wired by a host such as the Coding Agent. Setting the callback is a
// local state operation and does not invoke it.
type EditorChangeSetter interface {
	SetOnChange(func(string))
}

// EditorBorderColorSetter is implemented by editors whose optional border
// style can be replaced by a host. Setting the style does not invoke it.
type EditorBorderColorSetter interface {
	SetBorderColor(TextStyleFunc)
}

type EditorHistoryComponent interface {
	AddToHistory(string) error
}

type EditorInsertionComponent interface {
	InsertTextAtCursor(string) error
}

type EditorExpandedTextComponent interface {
	GetExpandedText() (string, error)
}

type AutocompleteEditorComponent interface {
	SetAutocompleteProvider(AutocompleteProvider) error
}

type PaddedEditorComponent interface {
	SetPaddingX(int) error
}

type AutocompleteSizeEditorComponent interface {
	SetAutocompleteMaxVisible(int) error
}

// EditorTheme combines the editor border style with its autocomplete list
// theme.
type EditorTheme struct {
	BorderColor TextStyleFunc
	SelectList  SelectListTheme
}

// EditorOptions configures the editor's horizontal padding and autocomplete
// list height. Pointers preserve absent versus explicitly-zero inputs.
type EditorOptions struct {
	PaddingX               *int
	AutocompleteMaxVisible *int
}

// TextChunk is one visual word-wrapped slice and its byte offsets in the
// original line.
type TextChunk struct {
	Text       string
	StartIndex int
	EndIndex   int
}

// Editor is Pi's concrete multi-line editor class. The constructor records its
// dependencies but does not register input, request a render, or start an
// autocomplete task.
type Editor struct {
	Focused       bool
	BorderColor   TextStyleFunc
	DisableSubmit bool
	OnSubmit      func(string)
	OnChange      func(string)

	ui                     EditorRuntime
	theme                  EditorTheme
	text                   string
	cursor                 int
	undo                   []editorSnapshot
	history                []string
	historyIndex           int
	draft                  editorSnapshot
	lastAction             string
	kills                  KillRing
	yankStart, yankEnd     int
	pastes                 map[string]string
	pasteCounter           int
	pasteBuffer            string
	inPaste                bool
	width                  int
	preferredCol           int
	scroll                 int
	terminalRows           int
	jump                   int
	paddingX               int
	autocompleteMaxVisible int
}

// EditorRuntime is the narrow TUI seam used by Editor.
type EditorRuntime interface {
	RenderRequester
	Terminal() Terminal
}

func NewEditor(ui EditorRuntime, theme EditorTheme, options ...EditorOptions) *Editor {
	e := &Editor{
		ui:           ui,
		historyIndex: -1, width: 80, preferredCol: -1,
		theme:                  theme,
		BorderColor:            theme.BorderColor,
		autocompleteMaxVisible: 5,
	}
	if len(options) != 0 {
		if options[0].PaddingX != nil {
			e.paddingX = *options[0].PaddingX
		}
		if options[0].AutocompleteMaxVisible != nil {
			e.autocompleteMaxVisible = *options[0].AutocompleteMaxVisible
		}
	}
	return e
}

func (e *Editor) AddToHistory(text string) error {
	text = strings.TrimSpace(text)
	if text != "" && (len(e.history) == 0 || e.history[0] != text) {
		e.history = append([]string{text}, e.history...)
		if len(e.history) > 100 {
			e.history = e.history[:100]
		}
	}
	return nil
}
func (e *Editor) SetFocusState(focused bool)         { e.Focused = focused }
func (e *Editor) SetOnSubmit(callback func(string))  { e.OnSubmit = callback }
func (e *Editor) SetOnChange(callback func(string))  { e.OnChange = callback }
func (e *Editor) SetBorderColor(style TextStyleFunc) { e.BorderColor = style }
func (e *Editor) GetAutocompleteMaxVisible() int     { return e.autocompleteMaxVisible }
func (e *Editor) GetCursor() EditorCursor {
	before := e.text[:e.cursor]
	return EditorCursor{Line: strings.Count(before, "\n"), Col: len(before) - strings.LastIndex(before, "\n") - 1}
}
func (e *Editor) GetExpandedText() (string, error) {
	return pasteMarker.ReplaceAllStringFunc(e.text, func(marker string) string {
		if text, ok := e.pastes[marker]; ok {
			return text
		}
		return marker
	}), nil
}

func (e *Editor) GetLines() []string {
	if e.text == "" {
		return []string{""}
	}
	lines := make([]string, 0, 1)
	start := 0
	for index, r := range e.text {
		if r != '\n' {
			continue
		}
		lines = append(lines, e.text[start:index])
		start = index + 1
	}
	return append(lines, e.text[start:])
}
func (e *Editor) GetPaddingX() int { return e.paddingX }
func (e *Editor) GetText() string  { return e.text }
func (e *Editor) HandleInput(data string) error {
	if e.inPaste || strings.Contains(data, "\x1b[200~") {
		return e.handlePasteInput(data)
	}
	if e.jump != 0 {
		direction := e.jump
		e.jump = 0
		if data != "" && data[0] >= 32 {
			if direction > 0 {
				if n := strings.Index(e.text[e.next():], data); n >= 0 {
					e.cursor = e.next() + n
				}
			} else if n := strings.LastIndex(e.text[:e.cursor], data); n >= 0 {
				e.cursor = n
			}
			e.lastAction = ""
			return nil
		}
	}
	switch data {
	case "\x1f", "\x1b[45;5u":
		e.restoreUndo()
	case "\x01", "\x1b[H", "\x1b[1~", "\x1b[1;5H":
		e.move(strings.LastIndex(e.text[:e.cursor], "\n") + 1)
	case "\x05", "\x1b[F", "\x1b[4~", "\x1b[1;5F":
		e.move(e.lineEnd())
	case "\x02", "\x1b[D":
		e.move(e.previous())
	case "\x06", "\x1b[C":
		e.move(e.next())
	case "\x1bb", "\x1b[1;3D", "\x1b[1;5D":
		e.move(e.word(-1))
	case "\x1bf", "\x1b[1;3C", "\x1b[1;5C":
		e.move(e.word(1))
	case "\x1b[5~", "\x1b[5;5~":
		e.page(-1)
	case "\x1b[6~", "\x1b[6;5~":
		e.page(1)
	case "\x1b[A":
		e.vertical(-1)
	case "\x1b[B":
		e.vertical(1)
	case "\x7f", "\b":
		e.delete(e.previous(), e.cursor, false)
	case "\x04", "\x1b[3~":
		e.delete(e.cursor, e.next(), false)
	case "\x17", "\x1b\x7f":
		e.delete(e.word(-1), e.cursor, true)
	case "\x1bd", "\x1b[3;3~":
		e.delete(e.cursor, e.word(1), true)
	case "\x15":
		start := strings.LastIndex(e.text[:e.cursor], "\n") + 1
		if start == e.cursor && start > 0 {
			start--
		}
		e.delete(start, e.cursor, true)
	case "\x0b":
		end := e.lineEnd()
		if end == e.cursor && end < len(e.text) {
			end++
		}
		e.delete(e.cursor, end, true)
	case "\x19":
		e.yank(false)
	case "\x1by":
		e.yank(true)
	case "\x1d":
		e.jump = 1
	case "\x1b\x1d":
		e.jump = -1
	case "\n", "\x1b\r", "\x1b[13;2u", "\x1b[13;2~":
		return e.InsertTextAtCursor("\n")
	case "\r", "\x1b[13u":
		if e.DisableSubmit {
			return nil
		}
		if e.cursor > 0 && e.text[e.cursor-1] == '\\' {
			e.delete(e.cursor-1, e.cursor, false)
			return e.InsertTextAtCursor("\n")
		}
		result, _ := e.GetExpandedText()
		e.text = ""
		e.cursor = 0
		e.undo = nil
		e.pastes = nil
		e.pasteCounter = 0
		e.exitHistory()
		e.lastAction = ""
		e.changed()
		if e.OnSubmit != nil {
			e.OnSubmit(strings.TrimSpace(result))
		}
	default:
		if data != "" && data[0] >= 32 {
			if strings.TrimSpace(data) == "" || e.lastAction != "type-word" {
				e.snapshot()
			}
			e.exitHistory()
			e.lastAction = "type-word"
			e.replace(e.cursor, e.cursor, normalizeEditorText(data))
		}
	}
	return nil
}
func (e *Editor) InsertTextAtCursor(text string) error {
	if text != "" {
		e.snapshot()
		e.exitHistory()
		e.lastAction = ""
		e.replace(e.cursor, e.cursor, normalizeEditorText(text))
	}
	return nil
}
func normalizeEditorText(text string) string {
	return strings.NewReplacer("\r\n", "\n", "\r", "\n", "\t", "    ").Replace(text)
}
func (e *Editor) changed() {
	if e.OnChange != nil {
		e.OnChange(e.text)
	}
}
func (e *Editor) replace(start, end int, text string) {
	e.preferredCol = -1
	e.text = e.text[:start] + text + e.text[end:]
	e.cursor = start + len(text)
	e.changed()
}
func (e *Editor) previous() int {
	segments := e.segments(e.text[:e.cursor])
	if len(segments) > 0 {
		return segments[len(segments)-1].ByteOffset
	}
	return 0
}
func (e *Editor) next() int {
	segments := e.segments(e.text[e.cursor:])
	if len(segments) > 0 {
		return e.cursor + len(segments[0].Text)
	}
	return e.cursor
}
func (e *Editor) lineEnd() int {
	if n := strings.IndexByte(e.text[e.cursor:], '\n'); n >= 0 {
		return e.cursor + n
	}
	return len(e.text)
}
func (*Editor) Invalidate() error           { return nil }
func (*Editor) IsShowingAutocomplete() bool { return false }

func (*Editor) SetAutocompleteMaxVisible(int) error {
	return newNotImplemented("Editor.setAutocompleteMaxVisible")
}
func (*Editor) SetAutocompleteProvider(AutocompleteProvider) error {
	return newNotImplemented("Editor.setAutocompleteProvider")
}
func (e *Editor) SetPaddingX(value int) error { e.paddingX = max(0, value); return nil }
func (e *Editor) SetText(text string) error {
	if e.text != normalizeEditorText(text) {
		e.snapshot()
	}
	e.exitHistory()
	e.lastAction = ""
	e.pastes = nil
	e.pasteCounter = 0
	e.text = normalizeEditorText(text)
	e.cursor = len(e.text)
	e.preferredCol = -1
	e.changed()
	return nil
}

// EditorCursor uses a zero-based line and a UTF-8 byte offset within that line.
type EditorCursor struct {
	Line int
	Col  int
}

var (
	_ EditorComponent         = (*Editor)(nil)
	_ EditorBorderColorSetter = (*Editor)(nil)
)

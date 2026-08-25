package tui

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

// WordWrapLine is deferred with the rest of rendering/layout behavior.
func WordWrapLine(string, int, ...[]UnicodeSegment) ([]TextChunk, error) {
	return nil, newNotImplemented("wordWrapLine")
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
		ui:                     ui,
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

func (*Editor) AddToHistory(string) error            { return newNotImplemented("Editor.addToHistory") }
func (e *Editor) SetFocusState(focused bool)         { e.Focused = focused }
func (e *Editor) SetOnSubmit(callback func(string))  { e.OnSubmit = callback }
func (e *Editor) SetOnChange(callback func(string))  { e.OnChange = callback }
func (e *Editor) SetBorderColor(style TextStyleFunc) { e.BorderColor = style }
func (e *Editor) GetAutocompleteMaxVisible() int     { return e.autocompleteMaxVisible }
func (*Editor) GetCursor() EditorCursor              { return EditorCursor{} }
func (e *Editor) GetExpandedText() (string, error)   { return e.text, nil }
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
func (e *Editor) GetPaddingX() int       { return e.paddingX }
func (e *Editor) GetText() string        { return e.text }
func (*Editor) HandleInput(string) error { return newNotImplemented("Editor.handleInput") }
func (*Editor) InsertTextAtCursor(string) error {
	return newNotImplemented("Editor.insertTextAtCursor")
}
func (*Editor) Invalidate() error            { return nil }
func (*Editor) IsShowingAutocomplete() bool  { return false }
func (*Editor) Render(int) ([]string, error) { return nil, newNotImplemented("Editor.render") }
func (*Editor) SetAutocompleteMaxVisible(int) error {
	return newNotImplemented("Editor.setAutocompleteMaxVisible")
}
func (*Editor) SetAutocompleteProvider(AutocompleteProvider) error {
	return newNotImplemented("Editor.setAutocompleteProvider")
}
func (*Editor) SetPaddingX(int) error { return newNotImplemented("Editor.setPaddingX") }
func (e *Editor) SetText(text string) error {
	e.text = text
	return nil
}

// EditorCursor is the Go record returned by Editor.GetCursor.
type EditorCursor struct {
	Line int
	Col  int
}

var (
	_ EditorComponent         = (*Editor)(nil)
	_ EditorBorderColorSetter = (*Editor)(nil)
)

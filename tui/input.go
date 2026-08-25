package tui

import "context"

// AutocompleteItem is one value offered by an autocomplete provider.
type AutocompleteItem struct {
	Value       string
	Label       string
	Description *string
}

func (AutocompleteItem) autocompleteEntry() {}

// SlashCommand describes a command that can participate in autocomplete. A
// nil GetArgumentCompletions callback means that the command has no argument
// completion provider. The bool returned by the callback distinguishes a null
// result from an empty, present result.
type SlashCommand struct {
	Name                   string
	Description            *string
	ArgumentHint           *string
	GetArgumentCompletions func(context.Context, string) ([]AutocompleteItem, bool, error)
}

func (SlashCommand) autocompleteEntry() {}

// AutocompleteEntry is the closed union accepted by
// NewCombinedAutocompleteProvider. Its concrete variants are SlashCommand and
// AutocompleteItem.
type AutocompleteEntry interface {
	autocompleteEntry()
}

// AutocompleteSuggestions contains the items matching Prefix.
type AutocompleteSuggestions struct {
	Items  []AutocompleteItem
	Prefix string
}

// AutocompleteOptions controls one suggestion request. Cancellation is carried
// separately by context.Context.
type AutocompleteOptions struct {
	Force bool
}

// AutocompleteResult is the editor state produced by applying a completion.
type AutocompleteResult struct {
	Lines      []string
	CursorLine int
	CursorCol  int
}

// AutocompleteProvider is the editor's narrow autocomplete boundary. Providers
// that omit an optional upstream capability return nil TriggerCharacters or a
// false ShouldTriggerFileCompletion result.
type AutocompleteProvider interface {
	TriggerCharacters() []string
	GetSuggestions(context.Context, []string, int, int, AutocompleteOptions) (AutocompleteSuggestions, bool, error)
	ApplyCompletion([]string, int, int, AutocompleteItem, string) (AutocompleteResult, error)
	ShouldTriggerFileCompletion([]string, int, int) (bool, error)
}

// CombinedAutocompleteProvider records the slash-command and file-completion
// configuration. It does not inspect the filesystem or start a process until
// the interactive TUI capability is implemented.
type CombinedAutocompleteProvider struct {
	commands []AutocompleteEntry
	basePath string
	fdPath   *string
}

// NewCombinedAutocompleteProvider is inert: it only records configuration.
func NewCombinedAutocompleteProvider(commands []AutocompleteEntry, basePath string, fdPath *string) *CombinedAutocompleteProvider {
	return &CombinedAutocompleteProvider{
		commands: append([]AutocompleteEntry(nil), commands...),
		basePath: basePath,
		fdPath:   inputCloneStringPointer(fdPath),
	}
}

func (*CombinedAutocompleteProvider) TriggerCharacters() []string { return nil }

func (*CombinedAutocompleteProvider) GetSuggestions(context.Context, []string, int, int, AutocompleteOptions) (AutocompleteSuggestions, bool, error) {
	return AutocompleteSuggestions{}, false, newNotImplemented("CombinedAutocompleteProvider.getSuggestions")
}

func (*CombinedAutocompleteProvider) ApplyCompletion([]string, int, int, AutocompleteItem, string) (AutocompleteResult, error) {
	return AutocompleteResult{}, newNotImplemented("CombinedAutocompleteProvider.applyCompletion")
}

func (*CombinedAutocompleteProvider) ShouldTriggerFileCompletion([]string, int, int) (bool, error) {
	return false, newNotImplemented("CombinedAutocompleteProvider.shouldTriggerFileCompletion")
}

// KeyID is a terminal key name, optionally prefixed by one or more modifiers
// such as "ctrl+" or "alt+". Validation belongs to the key parser.
type KeyID string

// KeyEventType identifies a Kitty keyboard protocol event.
type KeyEventType string

const (
	KeyEventTypePress   KeyEventType = "press"
	KeyEventTypeRepeat  KeyEventType = "repeat"
	KeyEventTypeRelease KeyEventType = "release"
)

// keyNames exposes the fixed baseline's named keys and pure modifier helpers.
// The Key value below is the Go counterpart of Pi's Key helper object.
type keyNames struct {
	Escape    KeyID
	Esc       KeyID
	Enter     KeyID
	Return    KeyID
	Tab       KeyID
	Space     KeyID
	Backspace KeyID
	Delete    KeyID
	Insert    KeyID
	Clear     KeyID
	Home      KeyID
	End       KeyID
	PageUp    KeyID
	PageDown  KeyID
	Up        KeyID
	Down      KeyID
	Left      KeyID
	Right     KeyID
	F1        KeyID
	F2        KeyID
	F3        KeyID
	F4        KeyID
	F5        KeyID
	F6        KeyID
	F7        KeyID
	F8        KeyID
	F9        KeyID
	F10       KeyID
	F11       KeyID
	F12       KeyID

	Backtick     KeyID
	Hyphen       KeyID
	Equals       KeyID
	LeftBracket  KeyID
	RightBracket KeyID
	Backslash    KeyID
	Semicolon    KeyID
	Quote        KeyID
	Comma        KeyID
	Period       KeyID
	Slash        KeyID
	Exclamation  KeyID
	At           KeyID
	Hash         KeyID
	Dollar       KeyID
	Percent      KeyID
	Caret        KeyID
	Ampersand    KeyID
	Asterisk     KeyID
	LeftParen    KeyID
	RightParen   KeyID
	Underscore   KeyID
	Plus         KeyID
	Pipe         KeyID
	Tilde        KeyID
	LeftBrace    KeyID
	RightBrace   KeyID
	Colon        KeyID
	LessThan     KeyID
	GreaterThan  KeyID
	Question     KeyID
}

// Key contains canonical identifiers for special and symbol keys.
var Key = keyNames{
	Escape: "escape", Esc: "esc", Enter: "enter", Return: "return",
	Tab: "tab", Space: "space", Backspace: "backspace", Delete: "delete",
	Insert: "insert", Clear: "clear", Home: "home", End: "end",
	PageUp: "pageUp", PageDown: "pageDown", Up: "up", Down: "down",
	Left: "left", Right: "right", F1: "f1", F2: "f2", F3: "f3",
	F4: "f4", F5: "f5", F6: "f6", F7: "f7", F8: "f8", F9: "f9",
	F10: "f10", F11: "f11", F12: "f12",

	Backtick: "`", Hyphen: "-", Equals: "=", LeftBracket: "[", RightBracket: "]",
	Backslash: "\\", Semicolon: ";", Quote: "'", Comma: ",", Period: ".",
	Slash: "/", Exclamation: "!", At: "@", Hash: "#", Dollar: "$", Percent: "%",
	Caret: "^", Ampersand: "&", Asterisk: "*", LeftParen: "(", RightParen: ")",
	Underscore: "_", Plus: "+", Pipe: "|", Tilde: "~", LeftBrace: "{",
	RightBrace: "}", Colon: ":", LessThan: "<", GreaterThan: ">", Question: "?",
}

func modifiedKey(prefix string, key KeyID) KeyID { return KeyID(prefix + string(key)) }

func (keyNames) Ctrl(key KeyID) KeyID           { return modifiedKey("ctrl+", key) }
func (keyNames) Shift(key KeyID) KeyID          { return modifiedKey("shift+", key) }
func (keyNames) Alt(key KeyID) KeyID            { return modifiedKey("alt+", key) }
func (keyNames) Super(key KeyID) KeyID          { return modifiedKey("super+", key) }
func (keyNames) CtrlShift(key KeyID) KeyID      { return modifiedKey("ctrl+shift+", key) }
func (keyNames) ShiftCtrl(key KeyID) KeyID      { return modifiedKey("shift+ctrl+", key) }
func (keyNames) CtrlAlt(key KeyID) KeyID        { return modifiedKey("ctrl+alt+", key) }
func (keyNames) AltCtrl(key KeyID) KeyID        { return modifiedKey("alt+ctrl+", key) }
func (keyNames) ShiftAlt(key KeyID) KeyID       { return modifiedKey("shift+alt+", key) }
func (keyNames) AltShift(key KeyID) KeyID       { return modifiedKey("alt+shift+", key) }
func (keyNames) CtrlSuper(key KeyID) KeyID      { return modifiedKey("ctrl+super+", key) }
func (keyNames) SuperCtrl(key KeyID) KeyID      { return modifiedKey("super+ctrl+", key) }
func (keyNames) ShiftSuper(key KeyID) KeyID     { return modifiedKey("shift+super+", key) }
func (keyNames) SuperShift(key KeyID) KeyID     { return modifiedKey("super+shift+", key) }
func (keyNames) AltSuper(key KeyID) KeyID       { return modifiedKey("alt+super+", key) }
func (keyNames) SuperAlt(key KeyID) KeyID       { return modifiedKey("super+alt+", key) }
func (keyNames) CtrlShiftAlt(key KeyID) KeyID   { return modifiedKey("ctrl+shift+alt+", key) }
func (keyNames) CtrlShiftSuper(key KeyID) KeyID { return modifiedKey("ctrl+shift+super+", key) }

// SetKittyProtocolActive is deferred because changing package-global parsing
// mode would make the M0 scaffold stateful.
func SetKittyProtocolActive(bool) error {
	return newNotImplemented("setKittyProtocolActive")
}

func IsKittyProtocolActive() (bool, error) {
	return false, newNotImplemented("isKittyProtocolActive")
}

func IsKeyRelease(string) (bool, error) {
	return false, newNotImplemented("isKeyRelease")
}

func IsKeyRepeat(string) (bool, error) {
	return false, newNotImplemented("isKeyRepeat")
}

func MatchesKey(string, KeyID) (bool, error) {
	return false, newNotImplemented("matchesKey")
}

func ParseKey(string) (KeyID, bool, error) {
	return "", false, newNotImplemented("parseKey")
}

func DecodeKittyPrintable(string) (string, bool, error) {
	return "", false, newNotImplemented("decodeKittyPrintable")
}

func DecodePrintableKey(string) (string, bool, error) {
	return "", false, newNotImplemented("decodePrintableKey")
}

// Keybinding identifies one extensible TUI action.
type Keybinding string

// Keybindings is the built-in portion of Pi's declaration-mergeable action
// registry. Custom actions remain expressible through the open Keybinding
// string type and KeybindingDefinitions maps.
type Keybindings struct {
	TUIEditorCursorUp           bool
	TUIEditorCursorDown         bool
	TUIEditorHistoryPrevious    bool
	TUIEditorHistoryNext        bool
	TUIEditorCursorLeft         bool
	TUIEditorCursorRight        bool
	TUIEditorCursorWordLeft     bool
	TUIEditorCursorWordRight    bool
	TUIEditorCursorLineStart    bool
	TUIEditorCursorLineEnd      bool
	TUIEditorJumpForward        bool
	TUIEditorJumpBackward       bool
	TUIEditorPageUp             bool
	TUIEditorPageDown           bool
	TUIEditorDeleteCharBackward bool
	TUIEditorDeleteCharForward  bool
	TUIEditorDeleteWordBackward bool
	TUIEditorDeleteWordForward  bool
	TUIEditorDeleteToLineStart  bool
	TUIEditorDeleteToLineEnd    bool
	TUIEditorYank               bool
	TUIEditorYankPop            bool
	TUIEditorUndo               bool
	TUIInputNewLine             bool
	TUIInputSubmit              bool
	TUIInputTab                 bool
	TUIInputCopy                bool
	TUISelectUp                 bool
	TUISelectDown               bool
	TUISelectPageUp             bool
	TUISelectPageDown           bool
	TUISelectConfirm            bool
	TUISelectCancel             bool
	TUIAltScreenPageUp          bool
	TUIAltScreenPageDown        bool
	TUIAltScreenHalfPageUp      bool
	TUIAltScreenHalfPageDown    bool
	TUIAltScreenPreviousPrompt  bool
	TUIAltScreenNextPrompt      bool
	TUIAltScreenTop             bool
	TUIAltScreenBottom          bool
}

const (
	KeybindingEditorCursorUp           Keybinding = "tui.editor.cursorUp"
	KeybindingEditorCursorDown         Keybinding = "tui.editor.cursorDown"
	KeybindingEditorHistoryPrevious    Keybinding = "tui.editor.historyPrevious"
	KeybindingEditorHistoryNext        Keybinding = "tui.editor.historyNext"
	KeybindingEditorCursorLeft         Keybinding = "tui.editor.cursorLeft"
	KeybindingEditorCursorRight        Keybinding = "tui.editor.cursorRight"
	KeybindingEditorCursorWordLeft     Keybinding = "tui.editor.cursorWordLeft"
	KeybindingEditorCursorWordRight    Keybinding = "tui.editor.cursorWordRight"
	KeybindingEditorCursorLineStart    Keybinding = "tui.editor.cursorLineStart"
	KeybindingEditorCursorLineEnd      Keybinding = "tui.editor.cursorLineEnd"
	KeybindingEditorJumpForward        Keybinding = "tui.editor.jumpForward"
	KeybindingEditorJumpBackward       Keybinding = "tui.editor.jumpBackward"
	KeybindingEditorPageUp             Keybinding = "tui.editor.pageUp"
	KeybindingEditorPageDown           Keybinding = "tui.editor.pageDown"
	KeybindingEditorDeleteCharBackward Keybinding = "tui.editor.deleteCharBackward"
	KeybindingEditorDeleteCharForward  Keybinding = "tui.editor.deleteCharForward"
	KeybindingEditorDeleteWordBackward Keybinding = "tui.editor.deleteWordBackward"
	KeybindingEditorDeleteWordForward  Keybinding = "tui.editor.deleteWordForward"
	KeybindingEditorDeleteToLineStart  Keybinding = "tui.editor.deleteToLineStart"
	KeybindingEditorDeleteToLineEnd    Keybinding = "tui.editor.deleteToLineEnd"
	KeybindingEditorYank               Keybinding = "tui.editor.yank"
	KeybindingEditorYankPop            Keybinding = "tui.editor.yankPop"
	KeybindingEditorUndo               Keybinding = "tui.editor.undo"
	KeybindingInputNewLine             Keybinding = "tui.input.newLine"
	KeybindingInputSubmit              Keybinding = "tui.input.submit"
	KeybindingInputTab                 Keybinding = "tui.input.tab"
	KeybindingInputCopy                Keybinding = "tui.input.copy"
	KeybindingSelectUp                 Keybinding = "tui.select.up"
	KeybindingSelectDown               Keybinding = "tui.select.down"
	KeybindingSelectPageUp             Keybinding = "tui.select.pageUp"
	KeybindingSelectPageDown           Keybinding = "tui.select.pageDown"
	KeybindingSelectConfirm            Keybinding = "tui.select.confirm"
	KeybindingSelectCancel             Keybinding = "tui.select.cancel"
	KeybindingAltScreenPageUp          Keybinding = "tui.altScreen.pageUp"
	KeybindingAltScreenPageDown        Keybinding = "tui.altScreen.pageDown"
	KeybindingAltScreenHalfPageUp      Keybinding = "tui.altScreen.halfPageUp"
	KeybindingAltScreenHalfPageDown    Keybinding = "tui.altScreen.halfPageDown"
	KeybindingAltScreenPreviousPrompt  Keybinding = "tui.altScreen.previousPrompt"
	KeybindingAltScreenNextPrompt      Keybinding = "tui.altScreen.nextPrompt"
	KeybindingAltScreenTop             Keybinding = "tui.altScreen.top"
	KeybindingAltScreenBottom          Keybinding = "tui.altScreen.bottom"
)

// KeybindingDefinition provides default keys for an action. A nil Description
// means no description was supplied.
type KeybindingDefinition struct {
	DefaultKeys []KeyID
	Description *string
}

type KeybindingDefinitions map[Keybinding]KeybindingDefinition

// KeybindingsConfig maps actions to their complete replacement key list. A
// missing entry uses defaults; a present empty slice disables the action.
type KeybindingsConfig map[Keybinding][]KeyID

type KeybindingConflict struct {
	Key         KeyID
	Keybindings []string
}

func keybindingDefinition(description string, keys ...KeyID) KeybindingDefinition {
	return KeybindingDefinition{DefaultKeys: keys, Description: &description}
}

// NewTUIKeybindings returns the fixed baseline's built-in definition set.
func NewTUIKeybindings() KeybindingDefinitions {
	return KeybindingDefinitions{
		KeybindingEditorCursorUp:           keybindingDefinition("Move cursor up", "up"),
		KeybindingEditorCursorDown:         keybindingDefinition("Move cursor down", "down"),
		KeybindingEditorHistoryPrevious:    keybindingDefinition("Select previous prompt history entry"),
		KeybindingEditorHistoryNext:        keybindingDefinition("Select next prompt history entry"),
		KeybindingEditorCursorLeft:         keybindingDefinition("Move cursor left", "left", "ctrl+b"),
		KeybindingEditorCursorRight:        keybindingDefinition("Move cursor right", "right", "ctrl+f"),
		KeybindingEditorCursorWordLeft:     keybindingDefinition("Move cursor word left", "alt+left", "ctrl+left", "alt+b"),
		KeybindingEditorCursorWordRight:    keybindingDefinition("Move cursor word right", "alt+right", "ctrl+right", "alt+f"),
		KeybindingEditorCursorLineStart:    keybindingDefinition("Move to line start", "home", "ctrl+home", "ctrl+a"),
		KeybindingEditorCursorLineEnd:      keybindingDefinition("Move to line end", "end", "ctrl+end", "ctrl+e"),
		KeybindingEditorJumpForward:        keybindingDefinition("Jump forward to character", "ctrl+]"),
		KeybindingEditorJumpBackward:       keybindingDefinition("Jump backward to character", "ctrl+alt+]"),
		KeybindingEditorPageUp:             keybindingDefinition("Page up", "pageUp", "ctrl+pageUp"),
		KeybindingEditorPageDown:           keybindingDefinition("Page down", "pageDown", "ctrl+pageDown"),
		KeybindingEditorDeleteCharBackward: keybindingDefinition("Delete character backward", "backspace"),
		KeybindingEditorDeleteCharForward:  keybindingDefinition("Delete character forward", "delete", "ctrl+d"),
		KeybindingEditorDeleteWordBackward: keybindingDefinition("Delete word backward", "ctrl+w", "alt+backspace"),
		KeybindingEditorDeleteWordForward:  keybindingDefinition("Delete word forward", "alt+d", "alt+delete"),
		KeybindingEditorDeleteToLineStart:  keybindingDefinition("Delete to line start", "ctrl+u"),
		KeybindingEditorDeleteToLineEnd:    keybindingDefinition("Delete to line end", "ctrl+k"),
		KeybindingEditorYank:               keybindingDefinition("Yank", "ctrl+y"),
		KeybindingEditorYankPop:            keybindingDefinition("Yank pop", "alt+y"),
		KeybindingEditorUndo:               keybindingDefinition("Undo", "ctrl+-"),
		KeybindingInputNewLine:             keybindingDefinition("Insert newline", "shift+enter", "ctrl+j"),
		KeybindingInputSubmit:              keybindingDefinition("Submit input", "enter"),
		KeybindingInputTab:                 keybindingDefinition("Tab / autocomplete", "tab"),
		KeybindingInputCopy:                keybindingDefinition("Copy selection", "ctrl+c"),
		KeybindingSelectUp:                 keybindingDefinition("Move selection up", "up"),
		KeybindingSelectDown:               keybindingDefinition("Move selection down", "down"),
		KeybindingSelectPageUp:             keybindingDefinition("Selection page up", "pageUp"),
		KeybindingSelectPageDown:           keybindingDefinition("Selection page down", "pageDown"),
		KeybindingSelectConfirm:            keybindingDefinition("Confirm selection", "enter"),
		KeybindingSelectCancel:             keybindingDefinition("Cancel selection", "escape", "ctrl+c"),
		KeybindingAltScreenPageUp:          keybindingDefinition("Scroll viewport up one page", "pageUp"),
		KeybindingAltScreenPageDown:        keybindingDefinition("Scroll viewport down one page", "pageDown"),
		KeybindingAltScreenHalfPageUp:      keybindingDefinition("Scroll viewport up half a page"),
		KeybindingAltScreenHalfPageDown:    keybindingDefinition("Scroll viewport down half a page"),
		KeybindingAltScreenPreviousPrompt:  keybindingDefinition("Jump to previous semantic prompt", "ctrl+shift+up"),
		KeybindingAltScreenNextPrompt:      keybindingDefinition("Jump to next semantic prompt", "ctrl+shift+down"),
		KeybindingAltScreenTop:             keybindingDefinition("Scroll viewport to top", "home"),
		KeybindingAltScreenBottom:          keybindingDefinition("Scroll viewport to bottom", "end"),
	}
}

// TUIKeybindings is the fixed baseline's built-in binding definition set. It
// must be treated as read-only; NewTUIKeybindings returns an independent map.
var TUIKeybindings = NewTUIKeybindings()

// KeybindingsManager records definition and user-binding inputs. Rebuilding and
// matching remain deferred because they depend on the terminal key parser.
type KeybindingsManager struct {
	definitions  KeybindingDefinitions
	userBindings KeybindingsConfig
}

func NewKeybindingsManager(definitions KeybindingDefinitions, userBindings ...KeybindingsConfig) *KeybindingsManager {
	manager := &KeybindingsManager{definitions: cloneKeybindingDefinitions(definitions)}
	if len(userBindings) != 0 {
		manager.userBindings = cloneKeybindingsConfig(userBindings[0])
	}
	return manager
}

func (*KeybindingsManager) Matches(string, Keybinding) (bool, error) {
	return false, newNotImplemented("KeybindingsManager.matches")
}

func (*KeybindingsManager) GetKeys(Keybinding) ([]KeyID, error) {
	return nil, newNotImplemented("KeybindingsManager.getKeys")
}

func (*KeybindingsManager) GetDefinition(Keybinding) (KeybindingDefinition, bool, error) {
	return KeybindingDefinition{}, false, newNotImplemented("KeybindingsManager.getDefinition")
}

func (*KeybindingsManager) GetConflicts() ([]KeybindingConflict, error) {
	return nil, newNotImplemented("KeybindingsManager.getConflicts")
}

func (*KeybindingsManager) SetUserBindings(KeybindingsConfig) error {
	return newNotImplemented("KeybindingsManager.setUserBindings")
}

func (*KeybindingsManager) GetUserBindings() (KeybindingsConfig, error) {
	return nil, newNotImplemented("KeybindingsManager.getUserBindings")
}

func (*KeybindingsManager) GetResolvedBindings() (KeybindingsConfig, error) {
	return nil, newNotImplemented("KeybindingsManager.getResolvedBindings")
}

// SetKeybindings does not mutate global state in the capability scaffold.
func SetKeybindings(*KeybindingsManager) error {
	return newNotImplemented("setKeybindings")
}

// GetKeybindings does not lazily create a process-global manager in the
// capability scaffold.
func GetKeybindings() (*KeybindingsManager, error) {
	return nil, newNotImplemented("getKeybindings")
}

// StdinBufferOptions uses milliseconds to preserve the fixed baseline's number
// unit. A nil Timeout selects the eventual implementation's default.
type StdinBufferOptions struct {
	Timeout *int64
}

// StdinBufferEventMap gives the typed callback shape for the two emitted event
// names. Callbacks are never invoked by the capability scaffold.
type StdinBufferEventMap struct {
	Data  func(string)
	Paste func(string)
}

// StdinBuffer is an inert input-sequence buffering capability. Construction
// never starts its eventual incomplete-sequence timer.
type StdinBuffer struct {
	options StdinBufferOptions
}

func NewStdinBuffer(options ...StdinBufferOptions) *StdinBuffer {
	buffer := &StdinBuffer{}
	if len(options) != 0 {
		buffer.options = options[0]
		buffer.options.Timeout = inputCloneInt64Pointer(options[0].Timeout)
	}
	return buffer
}

func (*StdinBuffer) Process([]byte) error {
	return newNotImplemented("StdinBuffer.process")
}

func (*StdinBuffer) Flush() ([]string, error) {
	return nil, newNotImplemented("StdinBuffer.flush")
}

func (*StdinBuffer) Clear() error {
	return newNotImplemented("StdinBuffer.clear")
}

func (*StdinBuffer) GetBuffer() (string, error) {
	return "", newNotImplemented("StdinBuffer.getBuffer")
}

func (*StdinBuffer) Destroy() error {
	return newNotImplemented("StdinBuffer.destroy")
}

// KillRingPushOptions controls how a kill is combined with the newest entry.
type KillRingPushOptions struct {
	Prepend    bool
	Accumulate bool
}

// KillRing is an in-memory Emacs-style kill ring.
type KillRing struct {
	ring []string
}

func NewKillRing() *KillRing { return &KillRing{} }

func (r *KillRing) Push(text string, options KillRingPushOptions) {
	if text == "" {
		return
	}
	if options.Accumulate && len(r.ring) != 0 {
		last := len(r.ring) - 1
		if options.Prepend {
			r.ring[last] = text + r.ring[last]
		} else {
			r.ring[last] += text
		}
		return
	}
	r.ring = append(r.ring, text)
}

func (r *KillRing) Peek() (string, bool) {
	if len(r.ring) == 0 {
		return "", false
	}
	return r.ring[len(r.ring)-1], true
}

func (r *KillRing) Rotate() {
	if len(r.ring) <= 1 {
		return
	}
	last := r.ring[len(r.ring)-1]
	copy(r.ring[1:], r.ring[:len(r.ring)-1])
	r.ring[0] = last
}

func (r *KillRing) Length() int { return len(r.ring) }

// UndoStack is the generic snapshot stack contract. Push remains deferred
// because Go assignment cannot reproduce structuredClone for arbitrary S.
type UndoStack[S any] struct{}

func NewUndoStack[S any]() *UndoStack[S] { return &UndoStack[S]{} }

func (*UndoStack[S]) Push(S) error {
	return newNotImplemented("UndoStack.push")
}

func (*UndoStack[S]) Pop() (S, bool, error) {
	var zero S
	return zero, false, newNotImplemented("UndoStack.pop")
}

func (*UndoStack[S]) Clear() error {
	return newNotImplemented("UndoStack.clear")
}

func (*UndoStack[S]) Length() int { return 0 }

// WordSegment is the Go counterpart of the Intl.SegmentData consumed by the
// custom word-navigation seam. Index is measured in the caller's string index
// convention.
type WordSegment struct {
	Segment    string
	Index      int
	Input      string
	IsWordLike bool
}

// WordNavigationOptions supplies optional segmentation and atomic-segment
// behavior.
type WordNavigationOptions struct {
	Segment         func(string) []WordSegment
	IsAtomicSegment func(string) bool
}

func FindWordBackward(string, int, ...WordNavigationOptions) (int, error) {
	return 0, newNotImplemented("findWordBackward")
}

func FindWordForward(string, int, ...WordNavigationOptions) (int, error) {
	return 0, newNotImplemented("findWordForward")
}

func inputCloneStringPointer(value *string) *string {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func inputCloneInt64Pointer(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneKeybindingDefinitions(definitions KeybindingDefinitions) KeybindingDefinitions {
	if definitions == nil {
		return nil
	}
	clone := make(KeybindingDefinitions, len(definitions))
	for keybinding, definition := range definitions {
		definition.DefaultKeys = append([]KeyID(nil), definition.DefaultKeys...)
		if definition.Description != nil {
			description := *definition.Description
			definition.Description = &description
		}
		clone[keybinding] = definition
	}
	return clone
}

func cloneKeybindingsConfig(config KeybindingsConfig) KeybindingsConfig {
	if config == nil {
		return nil
	}
	clone := make(KeybindingsConfig, len(config))
	for keybinding, keys := range config {
		clone[keybinding] = append([]KeyID(nil), keys...)
	}
	return clone
}

var _ AutocompleteProvider = (*CombinedAutocompleteProvider)(nil)

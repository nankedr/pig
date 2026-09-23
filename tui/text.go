package tui

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"time"
)

// TextUI renders a scrollable conversation and multiline editor.
type TextUI struct {
	editorStyle, dialogStyle TextStyleFunc
	colorHandler             func(string)
	renderWake               chan struct{}
	terminal                 Terminal
	mu                       sync.Mutex
	transcript               string
	editor                   *Editor
	dialog                   Component
	inputs                   []TextInput
	turn                     uint64
	ready                    chan struct{}
	done                     chan struct{}
	err                      error
	started, stopped         bool
	lastInterrupt            time.Time
	lastEscape               time.Time
	interaction              TextUIInteractionOptions
	keybindings              *KeybindingsManager
	onInterrupt              func()
	generation               uint64
	renderTranscript         func(int, bool, bool) ([]string, error)
	expanded, hideThinking   bool
	mode                     TUIMode
	scroll                   *ScrollView
	frame                    LayoutFrame
	mouse                    scrollMouse
	scrollbar                ScrollViewScrollbar
	exitTranscript           bool
	mainState                TUIMainScreenRenderState
	altState                 TUIMainScreenRenderState
}

type TextInput struct {
	Turn   uint64
	Text   string
	Action Keybinding
}
type TextUIOptions struct {
	Keybindings      *KeybindingsManager
	OnInterrupt      func()
	RenderTranscript func(width int, toolsExpanded, hideThinking bool) ([]string, error)
	HideThinking     bool
	Mode             TUIMode
	Scrollbar        ScrollViewScrollbar
	PreserveScreen   bool
}

func NewTextUI(terminal Terminal, options ...TextUIOptions) *TextUI {
	u := &TextUI{interaction: TextUIInteractionOptions{ShowHardwareCursor: true}, terminal: terminal, ready: make(chan struct{}, 1), done: make(chan struct{}), editor: NewEditor(nil, EditorTheme{})}
	if len(options) > 0 {
		u.keybindings = options[0].Keybindings
		u.onInterrupt = options[0].OnInterrupt
		u.renderTranscript = options[0].RenderTranscript
		u.hideThinking = options[0].HideThinking
		u.mode = options[0].Mode
		u.scrollbar = options[0].Scrollbar
		u.exitTranscript = !options[0].PreserveScreen
	}
	if u.keybindings == nil {
		defs := NewTUIKeybindings()
		for action, key := range map[Keybinding]KeyID{"app.clear": "ctrl+c", "app.exit": "ctrl+d", "app.interrupt": "escape", "app.suspend": "ctrl+z", "app.session.new": "", "app.tools.expand": "ctrl+o", "app.thinking.toggle": "ctrl+t", "app.message.followUp": "alt+enter", "app.message.dequeue": "alt+up"} {
			keys := []KeyID{}
			if key != "" {
				keys = append(keys, key)
			}
			defs[action] = KeybindingDefinition{DefaultKeys: keys}
		}
		u.keybindings = NewKeybindingsManager(defs)
	}
	if u.mode == "" {
		u.mode = TUIModeRegular
	}
	if u.scrollbar == "" {
		u.scrollbar = ScrollViewScrollbarAuto
	}
	follow := ScrollViewFollowEnd
	u.scroll = NewScrollView(nil, ScrollViewOptions{Follow: &follow, Scrollbar: &u.scrollbar})
	u.renderWake = make(chan struct{}, 1)
	u.editor.ui = u
	u.editor.keybindings = u.keybindings
	u.editor.kitty = func() bool { active, _ := u.terminal.KittyProtocolActive(); return active }
	u.editor.Focused = true
	u.editor.OnSubmit = func(text string) {
		if text != "" {
			u.editor.AddToHistory(text)
			u.inputs = append(u.inputs, TextInput{Text: text, Turn: u.turn})
			select {
			case u.ready <- struct{}{}:
			default:
			}
		}
	}
	return u
}

// SetDialog replaces the editor until cleared; component callbacks must not call TextUI methods.
func (u *TextUI) SetDialog(dialog Component) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.editor.cancelAutocomplete()
	u.dialog = dialog
	return u.render()
}

// SetStyles changes rendering without replacing the editor or dialog.
func (u *TextUI) SetStyles(editor, dialog TextStyleFunc) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.editorStyle, u.dialogStyle = editor, dialog
}

// SetTerminalColorHandler consumes color reports before focused components see input.
// The handler must not call TextUI methods.
func (u *TextUI) SetTerminalColorHandler(handler func(string)) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.colorHandler = handler
}
func (u *TextUI) Terminal() Terminal { return u.terminal }
func (u *TextUI) RequestRender(...bool) error {
	select {
	case u.renderWake <- struct{}{}:
	default:
	}
	return nil
}
func (u *TextUI) SetAutocompleteProvider(provider AutocompleteProvider) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.editor.SetAutocompleteProvider(provider)
}
func (u *TextUI) SetAutocompleteMaxVisible(value int) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.editor.SetAutocompleteMaxVisible(value)
}
func (u *TextUI) Done() <-chan struct{} { return u.done }
func (u *TextUI) Err() error            { u.mu.Lock(); defer u.mu.Unlock(); return u.err }
func (u *TextUI) Start() error {
	u.mu.Lock()
	if u.stopped {
		u.mu.Unlock()
		return errors.New("text UI is stopped")
	}
	if u.started {
		u.mu.Unlock()
		return nil
	}
	u.started = true
	u.mu.Unlock()
	if u.mode != TUIModeRegular && u.mode != TUIModeFullscreen {
		return errors.New("invalid TUI mode")
	}
	if u.terminal == nil {
		return errors.New("text UI requires a terminal")
	}
	if err := u.terminal.Start(u.input, func() { u.mu.Lock(); defer u.mu.Unlock(); u.render() }); err != nil {
		return errors.Join(err, u.Stop())
	}
	u.mu.Lock()
	if u.mode == TUIModeFullscreen {
		if err := u.terminal.Write(enterAltScreen + enableMouse()); err != nil {
			u.mu.Unlock()
			return errors.Join(err, u.Stop())
		}
	}
	err := u.render()
	u.mu.Unlock()
	if err != nil {
		return errors.Join(err, u.Stop())
	}
	u.watchTerminal()
	go func() {
		for {
			select {
			case <-u.done:
				return
			case <-u.renderWake:
				_ = u.Refresh()
			}
		}
	}()
	return nil
}
func (u *TextUI) Stop() error {
	u.mu.Lock()
	u.finish(nil)
	started := u.started
	u.started = false
	u.mu.Unlock()
	if started && u.terminal != nil {
		var screenErr error
		if u.interaction.ShowTerminalProgress {
			screenErr = u.terminal.Write("\x1b]9;4;0;\x07")
		}
		if u.mode == TUIModeFullscreen {
			screenErr = u.terminal.Write(leaveAltScreen)
			if u.exitTranscript && u.renderTranscript != nil {
				width, _ := u.terminal.Columns()
				lines, err := u.renderTranscript(max(1, width), u.expanded, u.hideThinking)
				screenErr = errors.Join(screenErr, err)
				if err == nil {
					screenErr = errors.Join(screenErr, u.terminal.Write(strings.Join(lines, "\r\n")+"\r\n"))
				}
			}
		}
		if u.mode == TUIModeRegular {
			screenErr = u.terminal.Write(mainScreenStop(u.mainState))
		}
		return errors.Join(screenErr, u.terminal.DrainInput(context.Background(), time.Second, 50*time.Millisecond), u.terminal.Stop())
	}
	return nil
}
func (u *TextUI) finish(err error) {
	if u.stopped {
		return
	}
	u.stopped = true
	if !errors.Is(err, io.EOF) {
		u.err = err
	}
	u.editor.cancelAutocomplete()
	close(u.done)
}
func (u *TextUI) ReadLine(ctx context.Context) (string, error) {
	for {
		input, err := u.ReadInput(ctx)
		if err != nil {
			return "", err
		}
		if input.Action == "" {
			return input.Text, nil
		}
	}
}
func (u *TextUI) ReadInput(ctx context.Context) (TextInput, error) {
	for {
		u.mu.Lock()
		if u.stopped {
			err := u.err
			u.mu.Unlock()
			if err != nil {
				return TextInput{}, err
			}
			return TextInput{}, io.EOF
		}
		if len(u.inputs) > 0 {
			line := u.inputs[0]
			u.inputs = u.inputs[1:]
			u.mu.Unlock()
			return line, nil
		}
		u.mu.Unlock()
		select {
		case <-ctx.Done():
			return TextInput{}, ctx.Err()
		case <-u.done:
		case <-u.ready:
		}
	}
}
func (u *TextUI) ClearEditor() error {
	u.mu.Lock()
	defer u.mu.Unlock()
	_ = u.editor.SetText("")
	return u.render()
}

// SetTurn tags subsequent inputs so delayed delivery cannot submit into another turn.
func (u *TextUI) SetTurn(turn uint64) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.turn = turn
	if u.started && !u.stopped && u.interaction.ShowTerminalProgress {
		_ = u.terminal.Write(u.progressSequence())
	}
}

func (u *TextUI) PrependEditor(text string) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	current, err := u.editor.GetExpandedText()
	if err != nil {
		return err
	}
	if strings.TrimSpace(current) != "" {
		text += "\n\n" + current
	}
	if err = u.editor.SetText(text); err != nil {
		return err
	}
	return u.render()
}

func (u *TextUI) AddToHistory(text string) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.editor.AddToHistory(text)
}
func (u *TextUI) Append(text string) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.transcript += terminalText(text)
	return u.render()
}
func terminalText(text string) string { return SafeTerminalText(text) }
func (u *TextUI) Refresh() error      { u.mu.Lock(); defer u.mu.Unlock(); return u.render() }
func (u *TextUI) render() error {
	if u.stopped {
		return u.err
	}
	if !u.started {
		return errors.New("text UI is not initialized")
	}
	rows, err := u.terminal.Rows()
	if err != nil {
		u.finish(err)
		return err
	}
	columns, err := u.terminal.Columns()
	if err != nil {
		u.finish(err)
		return err
	}
	columns = max(1, columns)
	rows = max(1, rows)
	u.editor.terminalRows = rows
	prefixWidth := min(2, columns-1)
	editorLines, err := u.editor.Render(columns - prefixWidth)
	if err != nil {
		return err
	}
	bottom := len(editorLines) - 1
	if u.editor.completion.list != nil {
		suggestions, _ := u.editor.completion.list.Render(columns - prefixWidth)
		bottom -= len(suggestions)
	}
	editorLines = append(editorLines[1:bottom], editorLines[bottom+1:]...)
	cursorRow := 0
	for i, line := range editorLines {
		if strings.Contains(line, CursorMarker) {
			cursorRow = i
		}
	}
	if len(editorLines) > rows {
		start := max(0, cursorRow-rows+1)
		editorLines = editorLines[start:min(len(editorLines), start+rows)]
	}
	var lines []string
	if u.renderTranscript != nil {
		lines, err = u.renderTranscript(columns-1, u.expanded, u.hideThinking)
		if err != nil {
			u.finish(err)
			return err
		}
	}
	if u.transcript != "" {
		extra, _ := WrapTextWithANSI(u.transcript, columns-1)
		for i, line := range extra {
			extra[i] = selectStyle(u.editorStyle, line)
		}
		lines = append(lines, extra...)
	}
	for i, line := range editorLines {
		prefix := "  "
		if i == 0 {
			prefix = "> "
		}
		editorLines[i] = selectStyle(u.editorStyle, prefix[:prefixWidth]+line)
	}
	if u.dialog != nil {
		editorLines, err = u.dialog.Render(columns)
		if err != nil {
			return err
		}
		for i, line := range editorLines {
			editorLines[i] = selectStyle(u.dialogStyle, line)
		}
		if len(editorLines) > rows {
			editorLines = editorLines[:rows]
		}
	}
	visible := lines
	if u.mode == TUIModeFullscreen {
		available := max(0, rows-len(editorLines))
		content := renderedLines(lines)
		u.scroll.child = &content
		_ = u.scroll.UpdateLayout(len(lines), available, func() {
			select {
			case u.renderWake <- struct{}{}:
			default:
			}
		})
		top := u.scroll.ScrollTop()
		visible = append([]string(nil), lines[top:min(len(lines), top+available)]...)
		visible = append(visible, make([]string, max(0, available-len(visible)))...)
		clip := LayoutRect{Width: columns, Height: len(visible)}
		box := &LayoutBox{Rect: clip, Clip: clip, ScrollView: u.scroll, ScrollContentLines: lines}
		u.frame = LayoutFrame{Root: box, Width: columns, Height: rows, PrimaryScrollView: u.scroll}
		paintLayout(box, visible, columns)
	}
	visible = append(visible, editorLines...)
	var output string
	var state TUIMainScreenRenderState
	if u.mode == TUIModeRegular {
		output, state, _ = mainScreenFrame(u.mainState, visible, columns, rows, u.interaction.ShowHardwareCursor, u.interaction.ClearOnShrink)
	} else {
		output, state, _ = screenFrame(u.altState, visible, columns, rows, u.interaction.ShowHardwareCursor)
	}
	if err = u.terminal.Write(output); err != nil {
		u.finish(err)
	} else if u.mode == TUIModeRegular {
		u.mainState = state
	} else {
		u.altState = state
	}

	return err
}
func (u *TextUI) input(data string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.stopped {
		return
	}
	if osc, _ := IsOSC11BackgroundColorResponse(data); osc {
		if u.colorHandler != nil {
			u.colorHandler(data)
		}
		return
	}
	if _, ok, _ := ParseTerminalColorSchemeReport(data); ok {
		if u.colorHandler != nil {
			u.colorHandler(data)
		}
		return
	}
	if release, _ := IsKeyRelease(data); release {
		return
	}
	if u.dialog != nil {
		if handler, ok := u.dialog.(interface{ HandleInput(string) error }); ok {
			if err := handler.HandleInput(data); err != nil {
				u.transcript += "\nError: " + SafeTerminalText(err.Error()) + "\n"
			}
		}
		_ = u.render()
		return
	}
	active, _ := u.terminal.KittyProtocolActive()
	match := func(action Keybinding) bool { return u.keybindings.matches(data, action, active) }
	if !strings.HasPrefix(data, "\x1b[200~") {
		scrollInput := u.mode == TUIModeFullscreen
		if scrollInput {
			if handled, err := u.mouse.handle(data, u.frame, u.scroll, 1, u.keybindings, active); handled || err != nil {
				if err != nil {
					u.finish(err)
				} else {
					_ = u.render()
				}
				return
			}
		}
	}
	if !match("app.interrupt") {
		u.lastEscape = time.Time{}
	}
	switch {
	case strings.HasPrefix(data, "\x1b[200~"):
		_ = u.editor.HandleInput(data)
	case match("app.clear"):
		now := time.Now()
		if now.Sub(u.lastInterrupt) < 500*time.Millisecond {
			u.finish(nil)
			return
		}
		u.lastInterrupt = now
		_ = u.editor.SetText("")
		u.editor.undo = nil
	case match("app.exit") && u.editor.GetText() == "":
		u.finish(nil)
		return
	case match("app.tools.expand"):
		u.expanded = !u.expanded
	case match("app.thinking.toggle"):
		u.hideThinking = !u.hideThinking
	case (u.editor.completion.list != nil || u.editor.completion.pending != nil) && match(KeybindingSelectCancel):
		u.editor.cancelAutocomplete()
	case match("app.interrupt"):
		now := time.Now()
		if u.turn == 0 && u.editor.GetText() == "" && u.interaction.DoubleEscapeAction != "none" && u.interaction.DoubleEscapeAction != "" {
			if now.Sub(u.lastEscape) < 500*time.Millisecond {
				u.lastEscape = time.Time{}
				u.enqueueAction(Keybinding("app.session."+u.interaction.DoubleEscapeAction), "")
				break
			}
			u.lastEscape = now
		} else {
			u.lastEscape = time.Time{}
		}
		if u.onInterrupt != nil {
			u.onInterrupt()
		} else {
			u.enqueueAction("app.interrupt", "")
		}
	case match("app.message.followUp"):
		text, _ := u.editor.GetExpandedText()
		text = strings.TrimSpace(text)
		if text != "" {
			_ = u.editor.AddToHistory(text)
			_ = u.editor.SetText("")
			u.enqueueAction("app.message.followUp", text)
		}
	case match("app.session.resume"), match("app.model.select"), match("app.model.cycleForward"), match("app.model.cycleBackward"), match("app.thinking.cycle"):
		for _, action := range []Keybinding{"app.session.resume", "app.model.select", "app.model.cycleForward", "app.model.cycleBackward", "app.thinking.cycle"} {
			if match(action) {
				u.enqueueAction(action, "")
				break
			}
		}
	case match("app.message.dequeue"):
		u.enqueueAction("app.message.dequeue", "")
	case match("app.session.new"), match("app.suspend"):
		action := Keybinding("app.session.new")
		if match("app.suspend") {
			action = "app.suspend"
		} else {
			_ = u.editor.SetText("")
		}
		u.inputs = append(u.inputs, TextInput{Action: action, Turn: u.turn})
		select {
		case u.ready <- struct{}{}:
		default:
		}
	default:
		_ = u.editor.HandleInput(data)
	}
	_ = u.render()
}
func (u *TextUI) watchTerminal() {
	source, ok := u.terminal.(interface {
		Done() <-chan struct{}
		Err() error
	})
	if !ok {
		return
	}
	u.mu.Lock()
	generation := u.generation
	done := source.Done()
	u.mu.Unlock()
	go func() {
		select {
		case <-u.done:
			return
		case <-done:
			u.mu.Lock()
			defer u.mu.Unlock()
			if generation == u.generation {
				u.finish(source.Err())
			}
		}
	}()
}
func (u *TextUI) Suspend() error {
	u.mu.Lock()
	if u.stopped {
		u.mu.Unlock()
		return io.EOF
	}
	u.generation++
	u.mu.Unlock()
	if u.mode == TUIModeFullscreen {
		if err := u.terminal.Write(leaveAltScreen); err != nil {
			return err
		}
	}
	if err := u.terminal.Stop(); err != nil {
		return err
	}
	if err := suspendProcess(); err != nil {
		return err
	}
	if err := u.terminal.Start(u.input, func() { u.mu.Lock(); defer u.mu.Unlock(); _ = u.render() }); err != nil {
		return err
	}
	if u.mode == TUIModeFullscreen {
		if err := u.terminal.Write(enterAltScreen + enableMouse()); err != nil {
			return err
		}
	}
	u.watchTerminal()
	u.mu.Lock()
	defer u.mu.Unlock()
	u.mainState = TUIMainScreenRenderState{PreviousWidth: -1, PreviousHeight: -1}
	u.altState = TUIMainScreenRenderState{}
	return u.render()
}

type renderedLines []string

func (l *renderedLines) Render(int) ([]string, error) { return []string(*l), nil }
func (*renderedLines) Invalidate() error              { return nil }
func (u *TextUI) Mode() TUIMode                       { u.mu.Lock(); defer u.mu.Unlock(); return u.mode }
func (u *TextUI) SetMode(mode TUIMode) error {
	if mode != TUIModeRegular && mode != TUIModeFullscreen {
		return errors.New("invalid TUI mode")
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.stopped {
		return errors.New("text UI is stopped")
	}
	if mode == u.mode {
		return nil
	}
	if u.started && !u.stopped {
		seq := leaveAltScreen
		if mode == TUIModeFullscreen {
			seq = enterAltScreen + enableMouse()
		}
		if err := u.terminal.Write(seq); err != nil {
			u.finish(err)
			return err
		}
	}
	u.mode = mode
	u.altState = TUIMainScreenRenderState{}
	u.mouse = scrollMouse{}
	if u.started {
		return u.render()
	}
	return nil
}
func (u *TextUI) ScrollToTop() error {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.mode == TUIModeRegular {
		return errors.New("scroll commands require fullscreen mode; use terminal scrollback in regular mode")
	}
	if err := u.scroll.ScrollToStart(); err != nil {
		return err
	}
	return u.render()
}
func (u *TextUI) ScrollToBottom() error {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.mode == TUIModeRegular {
		return errors.New("scroll commands require fullscreen mode; use terminal scrollback in regular mode")
	}
	if err := u.scroll.ScrollToEnd(); err != nil {
		return err
	}
	return u.render()
}

func (u *TextUI) enqueueAction(action Keybinding, text string) {
	u.inputs = append(u.inputs, TextInput{Action: action, Text: text, Turn: u.turn})
	select {
	case u.ready <- struct{}{}:
	default:
	}
}

// ResetSession replaces conversation-local editor history and output after a successful Session change.
func (u *TextUI) ResetSession(history []string) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.transcript = ""
	u.editor.history = nil
	u.editor.exitHistory()
	for _, text := range history {
		_ = u.editor.AddToHistory(text)
	}
	return u.render()
}

// SetEditorText restores a selected message, optionally preserving an existing draft.
func (u *TextUI) SetEditorText(text string, preserveDraft bool) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	if preserveDraft {
		current, err := u.editor.GetExpandedText()
		if err != nil {
			return err
		}
		if strings.TrimSpace(current) != "" {
			return nil
		}
	}
	if err := u.editor.SetText(text); err != nil {
		return err
	}
	return u.render()
}

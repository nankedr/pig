package tui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/rivo/uniseg"
)

// TextUI is the minimal text conversation renderer. Advanced layouts remain separate capabilities.
type TextUI struct {
	terminal         Terminal
	mu               sync.Mutex
	transcript       string
	editor           *Editor
	inputs           []TextInput
	ready            chan struct{}
	done             chan struct{}
	err              error
	started, stopped bool
	lastInterrupt    time.Time
	keybindings      *KeybindingsManager
	onInterrupt      func()
	generation       uint64
}

type TextInput struct {
	Text   string
	Action Keybinding
}
type TextUIOptions struct {
	Keybindings *KeybindingsManager
	OnInterrupt func()
}

func NewTextUI(terminal Terminal, options ...TextUIOptions) *TextUI {
	u := &TextUI{terminal: terminal, ready: make(chan struct{}, 1), done: make(chan struct{}), editor: NewEditor(nil, EditorTheme{})}
	if len(options) > 0 {
		u.keybindings = options[0].Keybindings
		u.onInterrupt = options[0].OnInterrupt
	}
	if u.keybindings == nil {
		defs := NewTUIKeybindings()
		for action, key := range map[Keybinding]KeyID{"app.clear": "ctrl+c", "app.exit": "ctrl+d", "app.interrupt": "escape", "app.suspend": "ctrl+z", "app.session.new": ""} {
			keys := []KeyID{}
			if key != "" {
				keys = append(keys, key)
			}
			defs[action] = KeybindingDefinition{DefaultKeys: keys}
		}
		u.keybindings = NewKeybindingsManager(defs)
	}
	u.editor.keybindings = u.keybindings
	u.editor.kitty = func() bool { active, _ := u.terminal.KittyProtocolActive(); return active }
	u.editor.Focused = true
	u.editor.OnSubmit = func(text string) {
		if text != "" {
			u.editor.AddToHistory(text)
			u.inputs = append(u.inputs, TextInput{Text: text})
			select {
			case u.ready <- struct{}{}:
			default:
			}
		}
	}
	return u
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
	if u.terminal == nil {
		return errors.New("text UI requires a terminal")
	}
	if err := u.terminal.Start(u.input, func() { u.mu.Lock(); defer u.mu.Unlock(); u.render() }); err != nil {
		return errors.Join(err, u.Stop())
	}
	u.mu.Lock()
	err := u.render()
	u.mu.Unlock()
	if err != nil {
		return errors.Join(err, u.Stop())
	}
	u.watchTerminal()
	return nil
}
func (u *TextUI) Stop() error {
	u.mu.Lock()
	u.finish(nil)
	started := u.started
	u.started = false
	u.mu.Unlock()
	if started && u.terminal != nil {
		return errors.Join(u.terminal.DrainInput(context.Background(), time.Second, 50*time.Millisecond), u.terminal.Stop())
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
func terminalText(text string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' || !unicode.IsControl(r) {
			return r
		}
		return -1
	}, text)
}
func (u *TextUI) render() error {
	if !u.started {
		return errors.New("text UI is not initialized")
	}
	if u.stopped {
		return u.err
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
	columns = max(4, columns)
	rows = max(1, rows)
	u.editor.terminalRows = rows
	editorLines, err := u.editor.Render(columns - 2)
	if err != nil {
		return err
	}
	editorLines = editorLines[1 : len(editorLines)-1]
	cursorRow, cursorCol := 0, 2
	for i, line := range editorLines {
		if n := strings.Index(line, CursorMarker); n >= 0 {
			cursorRow = i
			cursorCol = 2 + uniseg.StringWidth(line[:n])
			editorLines[i] = strings.ReplaceAll(line, CursorMarker, "")
		}
	}
	if len(editorLines) > rows {
		start := max(0, cursorRow-rows+1)
		editorLines = editorLines[start:min(len(editorLines), start+rows)]
		cursorRow -= start
	}
	var lines []string
	for _, line := range strings.Split(u.transcript, "\n") {
		chunks, _ := WordWrapLine(line, columns-1)
		for _, chunk := range chunks {
			lines = append(lines, chunk.Text)
		}
	}
	available := max(0, rows-len(editorLines))
	if len(lines) > available {
		lines = lines[len(lines)-available:]
	}
	cursorRow += len(lines)
	for i, line := range editorLines {
		prefix := "  "
		if i == 0 {
			prefix = "> "
		}
		lines = append(lines, prefix+line)
	}
	frame := strings.Join(lines, "\r\n")
	err = u.terminal.Write("\x1b[H\x1b[2J" + frame + fmt.Sprintf("\x1b[%d;%dH\x1b[?25h", cursorRow+1, cursorCol+1))
	if err != nil {
		u.finish(err)
	}
	return err
}
func (u *TextUI) input(data string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.stopped {
		return
	}
	if release, _ := IsKeyRelease(data); release {
		return
	}
	active, _ := u.terminal.KittyProtocolActive()
	match := func(action Keybinding) bool { return u.keybindings.matches(data, action, active) }
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
	case match("app.interrupt"):
		if u.onInterrupt != nil {
			u.onInterrupt()
		}
	case match("app.session.new"), match("app.suspend"):
		action := Keybinding("app.session.new")
		if match("app.suspend") {
			action = "app.suspend"
		} else {
			_ = u.editor.SetText("")
		}
		u.inputs = append(u.inputs, TextInput{Action: action})
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
	if err := u.terminal.Stop(); err != nil {
		return err
	}
	if err := suspendProcess(); err != nil {
		return err
	}
	if err := u.terminal.Start(u.input, func() { u.mu.Lock(); defer u.mu.Unlock(); _ = u.render() }); err != nil {
		return err
	}
	u.watchTerminal()
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.render()
}

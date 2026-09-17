package tui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"golang.org/x/term"
)

// Terminal is the process I/O boundary consumed by a TUI.
type Terminal interface {
	Start(onInput func(string), onResize func()) error
	Stop() error
	DrainInput(context.Context, time.Duration, time.Duration) error
	Write(string) error
	Columns() (int, error)
	Rows() (int, error)
	KittyProtocolActive() (bool, error)
	MoveBy(int) error
	HideCursor() error
	ShowCursor() error
	ClearLine() error
	ClearFromCursor() error
	ClearScreen() error
	SetTitle(string) error
	SetProgress(bool) error
}

// ProcessTerminal owns raw mode and input polling without taking ownership of the supplied files.
type ProcessTerminal struct {
	input   io.Reader
	output  io.Writer
	mu      sync.Mutex
	writeMu sync.Mutex
	state   *term.State
	file    *os.File
	stop    chan struct{}
	done    chan struct{}
	started bool
	stopped bool
	err     error
}

var _ Terminal = (*ProcessTerminal)(nil)

func NewProcessTerminal(input io.Reader, output io.Writer) *ProcessTerminal {
	return &ProcessTerminal{input: input, output: output, stop: make(chan struct{}), done: make(chan struct{})}
}
func (t *ProcessTerminal) Start(onInput func(string), onResize func()) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.started {
		return nil
	}
	if t.stopped {
		return errors.New("terminal is stopped")
	}
	file, ok := t.input.(*os.File)
	if !ok || file == nil || t.output == nil {
		return errors.New("terminal requires a file input and output writer")
	}
	if err := terminalSupported(); err != nil {
		return err
	}
	state, err := term.MakeRaw(int(file.Fd()))
	if err != nil {
		return err
	}
	if err := t.Write("\x1b[?2004h\x1b[?25l"); err != nil {
		return errors.Join(err, term.Restore(int(file.Fd()), state), t.Write("\x1b[?2004l\x1b[?25h"))
	}
	t.file, t.state, t.started = file, state, true
	go func() {
		err := readTerminal(file, t.stop, onInput, onResize)
		t.mu.Lock()
		t.err = err
		t.mu.Unlock()
		close(t.done)
	}()
	return nil
}
func (t *ProcessTerminal) Stop() error {
	t.mu.Lock()
	if t.stopped {
		t.mu.Unlock()
		return nil
	}
	t.stopped = true
	close(t.stop)
	started, file, state := t.started, t.file, t.state
	t.mu.Unlock()
	if !started {
		return nil
	}
	<-t.done
	return errors.Join(term.Restore(int(file.Fd()), state), t.Write("\x1b[?2004l\x1b[?25h\r\n"))
}
func (t *ProcessTerminal) Done() <-chan struct{} { return t.done }
func (t *ProcessTerminal) Err() error            { t.mu.Lock(); defer t.mu.Unlock(); return t.err }
func (t *ProcessTerminal) Write(text string) error {
	t.writeMu.Lock()
	defer t.writeMu.Unlock()
	if t.output == nil {
		return errors.New("terminal output is nil")
	}
	n, err := io.WriteString(t.output, text)
	if err == nil && n != len(text) {
		err = io.ErrShortWrite
	}
	return err
}
func (t *ProcessTerminal) Columns() (int, error) { width, _, err := t.size(); return width, err }
func (t *ProcessTerminal) Rows() (int, error)    { _, height, err := t.size(); return height, err }
func (t *ProcessTerminal) size() (int, int, error) {
	if file, ok := t.output.(*os.File); ok && file != nil {
		return term.GetSize(int(file.Fd()))
	}
	if file, ok := t.input.(*os.File); ok && file != nil {
		return term.GetSize(int(file.Fd()))
	}
	return 80, 24, nil
}
func (*ProcessTerminal) DrainInput(context.Context, time.Duration, time.Duration) error {
	return newNotImplemented("ProcessTerminal.drainInput")
}
func (*ProcessTerminal) KittyProtocolActive() (bool, error)   { return false, nil }
func (*ProcessTerminal) ModifyOtherKeysActive() (bool, error) { return false, nil }
func (t *ProcessTerminal) MoveBy(lines int) error {
	if lines < 0 {
		return t.Write(fmt.Sprintf("\x1b[%dA", -lines))
	}
	if lines > 0 {
		return t.Write(fmt.Sprintf("\x1b[%dB", lines))
	}
	return nil
}
func (t *ProcessTerminal) HideCursor() error      { return t.Write("\x1b[?25l") }
func (t *ProcessTerminal) ShowCursor() error      { return t.Write("\x1b[?25h") }
func (t *ProcessTerminal) ClearLine() error       { return t.Write("\x1b[2K") }
func (t *ProcessTerminal) ClearFromCursor() error { return t.Write("\x1b[J") }
func (t *ProcessTerminal) ClearScreen() error     { return t.Write("\x1b[2J\x1b[H") }
func (*ProcessTerminal) SetTitle(string) error    { return newNotImplemented("ProcessTerminal.setTitle") }
func (*ProcessTerminal) SetProgress(bool) error {
	return newNotImplemented("ProcessTerminal.setProgress")
}

// KeyboardProtocolNegotiationSequence is the parsed result of a terminal
// keyboard-protocol negotiation response.
type KeyboardProtocolNegotiationSequence struct {
	Type  string
	Flags int
}

func ParseKeyboardProtocolNegotiationSequence(string) (KeyboardProtocolNegotiationSequence, bool, error) {
	return KeyboardProtocolNegotiationSequence{}, false, newNotImplemented("parseKeyboardProtocolNegotiationSequence")
}

func IsAppleTerminalSession() (bool, error) {
	return false, newNotImplemented("isAppleTerminalSession")
}

func NormalizeNativeShiftEnterInput(string, bool, bool) (string, error) {
	return "", newNotImplemented("normalizeNativeShiftEnterInput")
}

func NormalizeAppleTerminalInput(string, bool, bool) (string, error) {
	return "", newNotImplemented("normalizeAppleTerminalInput")
}

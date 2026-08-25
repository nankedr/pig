package tui

import (
	"context"
	"io"
	"time"
)

// Terminal is the narrow process-terminal boundary consumed by a TUI. Methods
// that can touch process input, output, or terminal state report errors so the
// M0 capability scaffold can fail explicitly without doing I/O.
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

// ProcessTerminal is the CGO-free process-terminal capability boundary. It is
// deliberately inert until the interactive TUI milestone: its methods do not
// read input, write output, register handlers, or change terminal modes.
type ProcessTerminal struct {
	input  io.Reader
	output io.Writer
}

var _ Terminal = (*ProcessTerminal)(nil)

// NewProcessTerminal records the eventual process streams without touching
// them. Passing nil is valid for callers that only need the public contract.
func NewProcessTerminal(input io.Reader, output io.Writer) *ProcessTerminal {
	return &ProcessTerminal{input: input, output: output}
}

func (*ProcessTerminal) Start(func(string), func()) error {
	return newNotImplemented("ProcessTerminal.start")
}

func (*ProcessTerminal) Stop() error {
	return newNotImplemented("ProcessTerminal.stop")
}

func (*ProcessTerminal) DrainInput(context.Context, time.Duration, time.Duration) error {
	return newNotImplemented("ProcessTerminal.drainInput")
}

func (*ProcessTerminal) Write(string) error {
	return newNotImplemented("ProcessTerminal.write")
}

func (*ProcessTerminal) Columns() (int, error) {
	return 0, newNotImplemented("ProcessTerminal.columns")
}

func (*ProcessTerminal) Rows() (int, error) {
	return 0, newNotImplemented("ProcessTerminal.rows")
}

func (*ProcessTerminal) KittyProtocolActive() (bool, error) {
	return false, newNotImplemented("ProcessTerminal.kittyProtocolActive")
}

func (*ProcessTerminal) ModifyOtherKeysActive() (bool, error) {
	return false, newNotImplemented("ProcessTerminal.modifyOtherKeysActive")
}

func (*ProcessTerminal) MoveBy(int) error {
	return newNotImplemented("ProcessTerminal.moveBy")
}

func (*ProcessTerminal) HideCursor() error {
	return newNotImplemented("ProcessTerminal.hideCursor")
}

func (*ProcessTerminal) ShowCursor() error {
	return newNotImplemented("ProcessTerminal.showCursor")
}

func (*ProcessTerminal) ClearLine() error {
	return newNotImplemented("ProcessTerminal.clearLine")
}

func (*ProcessTerminal) ClearFromCursor() error {
	return newNotImplemented("ProcessTerminal.clearFromCursor")
}

func (*ProcessTerminal) ClearScreen() error {
	return newNotImplemented("ProcessTerminal.clearScreen")
}

func (*ProcessTerminal) SetTitle(string) error {
	return newNotImplemented("ProcessTerminal.setTitle")
}

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

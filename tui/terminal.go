package tui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"runtime"
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
// Input and resize callbacks are serialized and may call Stop or DrainInput.
// Stop cancels queued callbacks; an already running callback may finish afterward.
// Done closes after the reader and callback dispatcher finish.
type ProcessTerminal struct {
	input                   io.Reader
	output                  io.Writer
	mu                      sync.Mutex
	lifecycle               sync.Mutex
	buffer                  *StdinBuffer
	kitty, modify, draining bool
	pushed                  bool
	lastInput               time.Time
	writeMu                 sync.Mutex
	state                   *term.State
	file                    *os.File
	stop                    chan struct{}
	done                    chan struct{}
	readDone                chan struct{}
	inputStop               chan struct{}
	started                 bool
	stopped                 bool
	err                     error
}

var _ Terminal = (*ProcessTerminal)(nil)

func NewProcessTerminal(input io.Reader, output io.Writer) *ProcessTerminal {
	return &ProcessTerminal{input: input, output: output, stop: make(chan struct{}), done: make(chan struct{})}
}
func (t *ProcessTerminal) Start(onInput func(string), onResize func()) error {
	t.lifecycle.Lock()
	defer t.lifecycle.Unlock()
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.started {
		return nil
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
	if err = t.Write("\x1b[?2004h\x1b[?25l\x1b[>7u\x1b[?u\x1b[c"); err != nil {
		return errors.Join(err, term.Restore(int(file.Fd()), state), t.Write("\x1b[<u\x1b[?2004l\x1b[?25h"))
	}
	t.stop, t.done = make(chan struct{}), make(chan struct{})
	t.file, t.state, t.started, t.stopped, t.draining, t.err = file, state, true, false, false, nil
	t.pushed = true
	b := NewStdinBuffer()
	b.negotiation = true
	t.buffer = b
	t.readDone, t.inputStop = make(chan struct{}), make(chan struct{})
	stop, done, readDone, inputStop := t.stop, t.done, t.readDone, t.inputStop
	type event struct {
		data   string
		resize bool
	}
	events := make(chan event, 64)
	enqueue := func(e event) {
		select {
		case events <- e:
		case <-inputStop:
		}
	}
	b.SetHandlers(StdinBufferEventMap{
		Data:  func(seq string) { enqueue(event{data: seq}) },
		Paste: func(text string) { enqueue(event{data: "\x1b[200~" + text + "\x1b[201~"}) },
	})
	go func() {
		defer close(done)
		for e := range events {
			t.mu.Lock()
			active := t.stop == stop && !t.stopped && !t.draining
			t.mu.Unlock()
			if !active {
				continue
			}
			if e.resize {
				if onResize != nil {
					onResize()
				}
			} else {
				t.forward(e.data, onInput, stop)
			}
		}
	}()
	go func() {
		err := readTerminal(file, stop, func(data string) { t.mu.Lock(); t.lastInput = time.Now(); t.mu.Unlock(); _ = b.Process([]byte(data)) }, func() { enqueue(event{resize: true}) })
		b.Destroy()
		t.mu.Lock()
		t.err = errors.Join(t.err, err)
		t.mu.Unlock()
		close(events)
		close(readDone)
	}()
	return nil
}
func (t *ProcessTerminal) forward(seq string, onInput func(string), stop chan struct{}) {
	t.mu.Lock()
	if t.stop != stop || t.stopped || t.draining {
		t.mu.Unlock()
		return
	}
	response, ok, _ := ParseKeyboardProtocolNegotiationSequence(seq)
	if ok {
		var err error
		if response.Type == "kitty-flags" && response.Flags != 0 {
			if t.modify {
				err = t.Write("\x1b[>4;0m")
				t.modify = false
			}
			t.kitty = true
			_ = SetKittyProtocolActive(true)
		} else if !t.kitty && !t.modify {
			err = t.Write("\x1b[>4;2m")
			t.modify = true
		}
		if err != nil {
			t.err = errors.Join(t.err, err)
		}
		t.mu.Unlock()
		return
	}
	t.mu.Unlock()
	if onInput != nil {
		apple, _ := IsAppleTerminalSession()
		if seq == "\r" && apple {
			shift, _ := IsNativeModifierPressed(ModifierKeyShift)
			seq, _ = NormalizeNativeShiftEnterInput(seq, true, shift)
		}
		onInput(seq)
	}
}
func (t *ProcessTerminal) disableProtocols() error {
	text := ""
	if t.pushed {
		text = "\x1b[<u"
		t.pushed = false
	}
	if t.modify {
		text += "\x1b[>4;0m"
	}
	t.kitty, t.modify = false, false
	_ = SetKittyProtocolActive(false)
	if text == "" {
		return nil
	}
	return t.Write(text)
}
func (t *ProcessTerminal) Stop() error {
	t.lifecycle.Lock()
	defer t.lifecycle.Unlock()
	t.mu.Lock()
	if !t.started {
		t.mu.Unlock()
		return nil
	}
	t.stopped = true
	if !t.draining {
		close(t.inputStop)
	}
	close(t.stop)
	file, state, readDone, b := t.file, t.state, t.readDone, t.buffer
	err := t.disableProtocols()
	t.mu.Unlock()
	b.Destroy()
	<-readDone
	err = errors.Join(err, term.Restore(int(file.Fd()), state), t.Write("\x1b[?2004l\x1b[?25h\r\n"))
	t.mu.Lock()
	t.started = false
	t.mu.Unlock()
	return err
}
func (t *ProcessTerminal) Done() <-chan struct{} { t.mu.Lock(); defer t.mu.Unlock(); return t.done }
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
func (t *ProcessTerminal) DrainInput(ctx context.Context, maxTime, idleTime time.Duration) error {
	if ctx == nil {
		return errors.New("terminal drain requires a context")
	}
	t.mu.Lock()
	if !t.started {
		t.mu.Unlock()
		return nil
	}
	if !t.draining && !t.stopped {
		close(t.inputStop)
	}
	t.draining = true
	t.lastInput = time.Now()
	err := t.disableProtocols()
	b := t.buffer
	done := t.readDone
	t.mu.Unlock()
	b.Destroy()
	if err != nil {
		return err
	}
	deadline := time.NewTimer(max(0, maxTime))
	defer deadline.Stop()
	tick := time.NewTicker(max(time.Millisecond, idleTime))
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-done:
			return nil
		case <-deadline.C:
			return nil
		case <-tick.C:
			t.mu.Lock()
			idle := time.Since(t.lastInput) >= idleTime
			t.mu.Unlock()
			if idle {
				return nil
			}
		}
	}
}
func (t *ProcessTerminal) KittyProtocolActive() (bool, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.kitty, nil
}
func (t *ProcessTerminal) ModifyOtherKeysActive() (bool, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.modify, nil
}
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

var kittyResponse = regexp.MustCompile(`^\x1b\[\?(\d+)u$`)
var deviceResponse = regexp.MustCompile(`^\x1b\[\?[\d;]*c$`)

func ParseKeyboardProtocolNegotiationSequence(data string) (KeyboardProtocolNegotiationSequence, bool, error) {
	if m := kittyResponse.FindStringSubmatch(data); m != nil {
		return KeyboardProtocolNegotiationSequence{Type: "kitty-flags", Flags: number(m[1])}, true, nil
	}
	if deviceResponse.MatchString(data) {
		return KeyboardProtocolNegotiationSequence{Type: "device-attributes"}, true, nil
	}
	return KeyboardProtocolNegotiationSequence{}, false, nil
}
func IsAppleTerminalSession() (bool, error) {
	return runtime.GOOS == "darwin" && os.Getenv("TERM_PROGRAM") == "Apple_Terminal", nil
}
func NormalizeNativeShiftEnterInput(data string, detect, shift bool) (string, error) {
	if detect && shift && data == "\r" {
		return "\x1b[13;2u", nil
	}
	return data, nil
}
func NormalizeAppleTerminalInput(data string, apple, shift bool) (string, error) {
	return NormalizeNativeShiftEnterInput(data, apple, shift)
}

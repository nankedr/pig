package tui

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

// TextUI is the minimal text conversation renderer. Advanced layouts remain separate capabilities.
type TextUI struct {
	terminal                    Terminal
	mu                          sync.Mutex
	transcript, editor, pending string
	inputs                      []string
	ready                       chan struct{}
	done                        chan struct{}
	err                         error
	started, stopped            bool
	lastInterrupt               time.Time
	paste                       bool
}

func NewTextUI(terminal Terminal) *TextUI {
	return &TextUI{terminal: terminal, ready: make(chan struct{}, 1), done: make(chan struct{})}
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
	if source, ok := u.terminal.(interface {
		Done() <-chan struct{}
		Err() error
	}); ok {
		go func() {
			select {
			case <-u.done:
				return
			case <-source.Done():
				u.mu.Lock()
				defer u.mu.Unlock()
				u.finish(source.Err())
			}
		}()
	}
	return nil
}
func (u *TextUI) Stop() error {
	u.mu.Lock()
	u.finish(nil)
	started := u.started
	u.started = false
	u.mu.Unlock()
	if started && u.terminal != nil {
		return u.terminal.Stop()
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
		u.mu.Lock()
		if u.stopped {
			err := u.err
			u.mu.Unlock()
			if err != nil {
				return "", err
			}
			return "", io.EOF
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
			return "", ctx.Err()
		case <-u.done:
		case <-u.ready:
		}
	}
}
func (u *TextUI) ClearEditor() error {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.editor = ""
	return u.render()
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
	lines := strings.Split(u.transcript, "\n")
	if rows > 2 && len(lines) > rows-2 {
		lines = lines[len(lines)-(rows-2):]
	}
	frame := strings.Join(lines, "\r\n") + "\r\n> " + strings.ReplaceAll(u.editor, "\n", "\r\n  ")
	err = u.terminal.Write("\x1b[H\x1b[2J" + frame + "\x1b[?25h")
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
	u.pending += data
	for len(u.pending) > 0 {
		if u.pending[0] == 0x1b {
			if len(u.pending) == 1 {
				return
			}
			if u.pending[1] == '[' {
				end := 2
				for end < len(u.pending) && (u.pending[end] < 0x40 || u.pending[end] > 0x7e) {
					end++
				}
				if end == len(u.pending) {
					return
				}
				sequence := u.pending[:end+1]
				u.pending = u.pending[end+1:]
				if sequence == "\x1b[200~" {
					u.paste = true
				}
				if sequence == "\x1b[201~" {
					u.paste = false
				}
				continue
			}
			u.pending = u.pending[2:]
			continue
		}
		if !utf8.FullRuneInString(u.pending) {
			return
		}
		r, n := utf8.DecodeRuneInString(u.pending)
		u.pending = u.pending[n:]
		if u.paste {
			if r == '\r' {
				r = '\n'
			}
			if r == '\n' || r == '\t' || !unicode.IsControl(r) {
				u.editor += string(r)
			}
			continue
		}
		switch r {
		case '\x04':
			if u.editor == "" {
				u.finish(nil)
				return
			}
		case '\x03':
			now := time.Now()
			if now.Sub(u.lastInterrupt) < 500*time.Millisecond {
				u.finish(nil)
				return
			}
			u.lastInterrupt = now
			u.editor = ""
		case '\r', '\n':
			if strings.TrimSpace(u.editor) != "" {
				u.inputs = append(u.inputs, u.editor)
				select {
				case u.ready <- struct{}{}:
				default:
				}
				u.editor = ""
			}
		case '\x7f', '\b':
			if len(u.editor) > 0 {
				_, n := utf8.DecodeLastRuneInString(u.editor)
				u.editor = u.editor[:len(u.editor)-n]
			}
		default:
			if !unicode.IsControl(r) {
				u.editor += string(r)
			}
		}
	}
	u.render()
}

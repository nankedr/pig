//go:build darwin || linux

package terminaltest

import (
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/creack/pty"
	"golang.org/x/term"
)

type Terminal struct {
	Master, Slave *os.File
	before        *term.State
	mu            sync.Mutex
	output        string
	screen        *Screen
	screenOffset  int
}

func Open(t *testing.T) *Terminal {
	t.Helper()
	master, slave, err := pty.Open()
	if err != nil {
		t.Fatal(err)
	}
	before, err := term.GetState(int(slave.Fd()))
	if err != nil {
		t.Fatal(err)
	}
	if err := pty.Setsize(slave, &pty.Winsize{Rows: 32, Cols: 100}); err != nil {
		t.Fatal(err)
	}
	terminal := &Terminal{Master: master, Slave: slave, before: before, screen: NewScreen(100, 32)}
	t.Cleanup(func() { master.Close(); slave.Close() })
	go func() {
		data := make([]byte, 65536)
		for {
			n, err := master.Read(data)
			if n > 0 {
				terminal.mu.Lock()
				terminal.output += string(data[:n])
				terminal.mu.Unlock()
			}
			if err != nil {
				return
			}
		}
	}()
	return terminal
}
func (p *Terminal) Output() string { p.mu.Lock(); defer p.mu.Unlock(); return p.output }
func (p *Terminal) Send(t *testing.T, input string) {
	t.Helper()
	if _, err := p.Master.WriteString(input); err != nil {
		t.Fatal(err)
	}
}
func (p *Terminal) Wait(t *testing.T, text string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !strings.Contains(p.Output(), text) {
		if time.Now().After(deadline) {
			t.Fatalf("terminal never displayed %q: %q", text, p.Output())
		}
		time.Sleep(5 * time.Millisecond)
	}
}
func (p *Terminal) Restored(t *testing.T) bool {
	t.Helper()
	after, err := term.GetState(int(p.Slave.Fd()))
	if err != nil {
		t.Fatal(err)
	}
	return reflect.DeepEqual(p.before, after)
}

func (p *Terminal) CursorVisible() bool {
	output := p.Output()
	return strings.LastIndex(output, "\x1b[?25h") > strings.LastIndex(output, "\x1b[?25l")
}
func (p *Terminal) PasteDisabled() bool {
	output := p.Output()
	return strings.LastIndex(output, "\x1b[?2004l") > strings.LastIndex(output, "\x1b[?2004h")
}

func (p *Terminal) updateScreen() {
	p.screen.Feed(p.output[p.screenOffset:])
	p.screenOffset = len(p.output)
}
func (p *Terminal) ScreenText() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.updateScreen()
	return p.screen.Text()
}
func (p *Terminal) ScrollbackText() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.updateScreen()
	return strings.Join(p.screen.History, "\n")
}
func (p *Terminal) SetSize(rows, cols uint16) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.updateScreen()
	if err := pty.Setsize(p.Slave, &pty.Winsize{Rows: rows, Cols: cols}); err != nil {
		return err
	}
	p.screen.Resize(int(cols), int(rows))
	return nil
}

package tui

import (
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// StdinBuffer preserves byte fragments until a complete UTF-8 character or terminal sequence arrives.
// Handlers run serially; they must not call back into this buffer.
type StdinBuffer struct {
	negotiation bool
	mu          sync.Mutex
	buffer      string
	paste       bool
	pending     int
	timeout     time.Duration
	timer       *time.Timer
	generation  uint64
	handlers    StdinBufferEventMap
}

func NewStdinBuffer(options ...StdinBufferOptions) *StdinBuffer {
	timeout := 10 * time.Millisecond
	if len(options) > 0 && options[0].Timeout != nil {
		timeout = time.Duration(max(0, *options[0].Timeout)) * time.Millisecond
	}
	return &StdinBuffer{timeout: timeout, pending: -1}
}
func (b *StdinBuffer) SetHandlers(handlers StdinBufferEventMap) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers = handlers
}
func (b *StdinBuffer) cancelTimer() {
	b.generation++
	if b.timer != nil {
		b.timer.Stop()
		b.timer = nil
	}
}
func (b *StdinBuffer) Process(data []byte) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.cancelTimer()
	b.buffer += string(data)
	for b.buffer != "" {
		if b.paste {
			end := strings.Index(b.buffer, "\x1b[201~")
			if end < 0 {
				return nil
			}
			text := b.buffer[:end]
			b.buffer = b.buffer[end+6:]
			b.paste = false
			b.pending = -1
			if b.handlers.Paste != nil {
				b.handlers.Paste(text)
			}
			continue
		}
		n := sequenceLength(b.buffer)
		if n == 0 {
			break
		}
		seq := b.buffer[:n]
		b.buffer = b.buffer[n:]
		if seq == "\x1b[200~" {
			b.paste = true
			b.pending = -1
			continue
		}
		b.emit(seq)
	}
	if b.buffer != "" && !b.paste {
		generation := b.generation
		timeout := b.timeout
		if b.negotiation && len(b.buffer) > 1 && b.buffer[0] == 27 {
			timeout = 150 * time.Millisecond
		}
		b.timer = time.AfterFunc(timeout, func() {
			b.mu.Lock()
			defer b.mu.Unlock()
			if generation != b.generation {
				return
			}
			b.timer = nil
			// An input timeout resolves Escape ambiguity, never a partial UTF-8 encoding.
			if b.buffer != "" && utf8.ValidString(b.buffer) {
				text := b.buffer
				b.buffer = ""
				b.pending = -1
				b.emit(text)
			}
		})
	}
	return nil
}
func (b *StdinBuffer) emit(seq string) {
	r, n := utf8.DecodeRuneInString(seq)
	if n == len(seq) && int(r) == b.pending {
		b.pending = -1
		return
	}
	b.pending = -1
	if m := csiKey.FindStringSubmatch(seq); m != nil && m[4] == "" && m[5] == "" {
		if cp := number(m[1]); cp >= 32 {
			b.pending = cp
		}
	}
	if b.handlers.Data != nil {
		b.handlers.Data(seq)
	}
}
func sequenceLength(s string) int {
	if s[0] != 27 {
		if !utf8.FullRuneInString(s) {
			return 0
		}
		_, n := utf8.DecodeRuneInString(s)
		return n
	}
	if len(s) < 2 {
		return 0
	}
	if s[1] == 27 {
		if len(s) == 2 {
			return 0
		}
		if strings.ContainsRune("[]OP_", rune(s[2])) {
			return 1
		}
		return 2
	}
	switch s[1] {
	case '[':
		if strings.HasPrefix(s, "\x1b[M") {
			if len(s) < 6 {
				return 0
			}
			return 6
		}
		for i := 2; i < len(s); i++ {
			if s[i] >= 0x40 && s[i] <= 0x7e {
				return i + 1
			}
		}
	case ']', 'P', '_':
		for i := 2; i < len(s); i++ {
			if s[1] == ']' && s[i] == 7 {
				return i + 1
			}
			if s[i] == 27 && i+1 < len(s) && s[i+1] == '\\' {
				return i + 2
			}
		}
	case 'O':
		if len(s) >= 3 {
			return 3
		}
	default:
		if utf8.FullRuneInString(s[1:]) {
			_, n := utf8.DecodeRuneInString(s[1:])
			return 1 + n
		}
	}
	return 0
}
func (b *StdinBuffer) Flush() ([]string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.cancelTimer()
	out := []string{}
	if b.buffer != "" && !b.paste {
		out = append(out, b.buffer)
		b.buffer = ""
		b.pending = -1
	}
	return out, nil
}
func (b *StdinBuffer) Clear() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.cancelTimer()
	b.buffer = ""
	b.paste = false
	b.pending = -1
	return nil
}
func (b *StdinBuffer) GetBuffer() (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.paste {
		return "", nil
	}
	return b.buffer, nil
}
func (b *StdinBuffer) Destroy() error { return b.Clear() }

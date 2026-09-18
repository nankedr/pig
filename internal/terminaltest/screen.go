package terminaltest

import (
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/rivo/uniseg"
)

// Screen interprets the VT operations emitted by the text renderers, including
// main-screen scrollback. It is deliberately independent of the TUI renderer.
type Screen struct {
	Width, Height int
	Row, Col      int
	Visible       bool
	Lines         [][]string
	History       []string
	pending       string
	alternate     bool
	saved         *Screen
}

func NewScreen(width, height int) *Screen {
	s := &Screen{Visible: true}
	s.Resize(width, height)
	return s
}
func (s *Screen) Resize(width, height int) {
	s.Width, s.Height = max(1, width), max(1, height)
	for len(s.Lines) > s.Height {
		s.History = append(s.History, strings.TrimRight(strings.Join(s.Lines[0], ""), " "))
		s.Lines = s.Lines[1:]
		s.Row--
	}
	for len(s.Lines) < s.Height {
		s.Lines = append(s.Lines, nil)
	}
	for i, line := range s.Lines {
		if len(line) > s.Width {
			line = line[:s.Width]
		}
		for len(line) < s.Width {
			line = append(line, " ")
		}
		s.Lines[i] = line
	}
	s.Row = max(0, min(s.Height-1, s.Row))
	s.Col = min(s.Width-1, s.Col)
}
func (s *Screen) Text() string {
	lines := make([]string, len(s.Lines))
	for i, line := range s.Lines {
		lines[i] = strings.TrimRight(strings.Join(line, ""), " ")
	}
	return strings.TrimRight(strings.Join(lines, "\n"), "\n")
}
func (s *Screen) newline() {
	s.Row++
	if s.Row < s.Height {
		return
	}
	if !s.alternate {
		s.History = append(s.History, strings.TrimRight(strings.Join(s.Lines[0], ""), " "))
	}
	s.Lines = append(s.Lines[1:], make([]string, s.Width))
	for i := range s.Lines[s.Height-1] {
		s.Lines[s.Height-1][i] = " "
	}
	s.Row = s.Height - 1
}
func (s *Screen) Feed(data string) {
	data = s.pending + data
	s.pending = ""
	for len(data) > 0 {
		if data[0] == 27 {
			if len(data) < 2 {
				break
			}
			if data[1] == '[' {
				end := 2
				for end < len(data) && (data[end] < 0x40 || data[end] > 0x7e) {
					end++
				}
				if end == len(data) {
					break
				}
				s.csi(data[2:end], data[end])
				data = data[end+1:]
				continue
			}
			if strings.ContainsRune("]_P^", rune(data[1])) {
				end := 2
				for end < len(data) && data[end] != 7 && !(data[end] == 27 && end+1 < len(data) && data[end+1] == '\\') {
					end++
				}
				if end == len(data) {
					break
				}
				if data[end] == 27 {
					end++
				}
				data = data[end+1:]
				continue
			}
			data = data[2:]
			continue
		}
		switch data[0] {
		case '\r':
			s.Col = 0
			data = data[1:]
			continue
		case '\n':
			s.newline()
			data = data[1:]
			continue
		case '\b':
			s.Col = max(0, s.Col-1)
			data = data[1:]
			continue
		}
		if data[0] < 32 {
			data = data[1:]
			continue
		}
		if !utf8.FullRuneInString(data) {
			break
		}
		cluster, rest, _, _ := uniseg.FirstGraphemeClusterInString(data, -1)
		width := uniseg.StringWidth(cluster)
		if width > 0 {
			if s.Col+width > s.Width {
				s.Col = 0
				s.newline()
			}
			s.Lines[s.Row][s.Col] = cluster
			for i := 1; i < width && s.Col+i < s.Width; i++ {
				s.Lines[s.Row][s.Col+i] = ""
			}
			s.Col += width
		}
		data = rest
	}
	s.pending = data
}
func (s *Screen) csi(params string, command byte) {
	private := strings.HasPrefix(params, "?")
	params = strings.TrimPrefix(params, "?")
	parts := strings.Split(params, ";")
	numbers := make([]int, len(parts))
	for i, p := range parts {
		numbers[i], _ = strconv.Atoi(p)
	}
	n := max(1, numbers[0])
	switch command {
	case 'A':
		s.Row = max(0, s.Row-n)
	case 'B':
		s.Row = min(s.Height-1, s.Row+n)
	case 'C':
		s.Col = min(s.Width-1, s.Col+n)
	case 'D':
		s.Col = max(0, s.Col-n)
	case 'G':
		s.Col = min(s.Width-1, n-1)
	case 'H', 'f':
		s.Row = min(s.Height-1, n-1)
		s.Col = 0
		if len(numbers) > 1 {
			s.Col = min(s.Width-1, max(1, numbers[1])-1)
		}
	case 'J':
		if numbers[0] == 3 {
			s.History = nil
			return
		}
		for row := range s.Lines {
			for col := range s.Lines[row] {
				if numbers[0] == 2 || row > s.Row || row == s.Row && col >= s.Col {
					s.Lines[row][col] = " "
				}
			}
		}
	case 'K':
		for col := range s.Lines[s.Row] {
			if numbers[0] == 2 || numbers[0] == 0 && col >= s.Col || numbers[0] == 1 && col <= s.Col {
				s.Lines[s.Row][col] = " "
			}
		}
	case 'h', 'l':
		if !private {
			return
		}
		for _, mode := range numbers {
			switch mode {
			case 25:
				s.Visible = command == 'h'
			case 1049:
				if command == 'h' && !s.alternate {
					old := *s
					fresh := NewScreen(s.Width, s.Height)
					*s = *fresh
					s.saved = &old
					s.alternate = true
				} else if command == 'l' && s.saved != nil {
					width, height := s.Width, s.Height
					*s = *s.saved
					s.Resize(width, height)
				}
			}
		}
	}
}

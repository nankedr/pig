package tui

import (
	"github.com/rivo/uniseg"
	"strings"
	"unicode"
)

func ExtractANSICode(s string, pos int) (ANSICode, bool, error) {
	if pos < 0 || pos >= len(s) || s[pos] != 27 {
		return ANSICode{}, false, nil
	}
	end := pos + 1
	if end < len(s) {
		switch s[end] {
		case '[':
			end++
			for end < len(s) {
				b := s[end]
				end++
				if b >= 0x40 && b <= 0x7e {
					break
				}
			}
		case ']', 'P', '_', '^', 'X':
			end++
			for end < len(s) {
				if s[end] == 7 {
					end++
					break
				}
				if s[end] == 27 && end+1 < len(s) && s[end+1] == '\\' {
					end += 2
					break
				}
				end++
			}
		default:
			end++
		}
	}
	return ANSICode{Code: s[pos:end], Length: end - pos}, true, nil
}

// SafeTerminalText preserves colors while removing terminal commands from message content.
func SafeTerminalText(s string) string {
	var b strings.Builder
	for len(s) > 0 {
		if code, ok, _ := ExtractANSICode(s, 0); ok {
			if strings.HasPrefix(code.Code, "\x1b[") && strings.HasSuffix(code.Code, "m") {
				b.WriteString(code.Code)
			}
			s = s[code.Length:]
			continue
		}
		g := uniseg.NewGraphemes(s)
		g.Next()
		part := g.Str()
		for _, r := range part {
			if r == '\n' || r == '\t' || !unicode.IsControl(r) {
				b.WriteRune(r)
			}
		}
		s = s[len(part):]
	}
	return strings.ReplaceAll(b.String(), "\t", "   ")
}
func StripTerminalSequences(s string) (string, error) {
	var b strings.Builder
	for len(s) > 0 {
		if code, ok, _ := ExtractANSICode(s, 0); ok {
			s = s[code.Length:]
			continue
		}
		b.WriteByte(s[0])
		s = s[1:]
	}
	return b.String(), nil
}
func VisibleWidth(s string) (int, error) {
	s, _ = StripTerminalSequences(s)
	return uniseg.StringWidth(s), nil
}

func WrapTextWithANSI(s string, width int) ([]string, error) {
	width = max(1, width)
	var result []string
	active := ""
	for _, line := range strings.Split(SafeTerminalText(s), "\n") {
		plain, _ := StripTerminalSequences(line)
		chunks, _ := WordWrapLine(plain, width)
		offset, pos := 0, 0
		for _, chunk := range chunks {
			var b strings.Builder
			b.WriteString(active)
			for pos < len(line) {
				if code, ok, _ := ExtractANSICode(line, pos); ok {
					b.WriteString(code.Code)
					if code.Code == "\x1b[0m" || code.Code == "\x1b[m" {
						active = ""
					} else {
						active += code.Code
					}
					pos += code.Length
					continue
				}
				if offset >= chunk.EndIndex {
					break
				}
				b.WriteByte(line[pos])
				pos++
				offset++
			}
			text := strings.TrimRight(b.String(), " ")
			if active != "" {
				text += "\x1b[0m"
			}
			result = append(result, text)
		}
	}
	return result, nil
}
func TruncateToWidth(s string, width int, options ...TruncateOptions) (string, error) {
	width = max(0, width)
	opt := TruncateOptions{}
	if len(options) > 0 {
		opt = options[0]
	}
	ellipsis := "…"
	if opt.Ellipsis != nil {
		ellipsis = *opt.Ellipsis
	}
	s = SafeTerminalText(s)
	w, _ := VisibleWidth(s)
	if w <= width {
		if opt.Pad {
			s += strings.Repeat(" ", width-w)
		}
		return s, nil
	}
	ew, _ := VisibleWidth(ellipsis)
	if ew > width {
		ellipsis = ""
		ew = 0
	}
	var b strings.Builder
	used := 0
	for len(s) > 0 {
		if code, ok, _ := ExtractANSICode(s, 0); ok {
			b.WriteString(code.Code)
			s = s[code.Length:]
			continue
		}
		g := uniseg.NewGraphemes(s)
		g.Next()
		if used+g.Width() > width-ew {
			break
		}
		b.WriteString(g.Str())
		used += g.Width()
		s = s[len(g.Str()):]
	}
	b.WriteString(ellipsis)
	if strings.Contains(b.String(), "\x1b[") {
		b.WriteString("\x1b[0m")
	}
	if opt.Pad {
		b.WriteString(strings.Repeat(" ", width-used-ew))
	}
	return b.String(), nil
}

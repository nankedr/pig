package tui

import (
	"fmt"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	ext "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
	"html"
	"strings"
)

func styled(fn TextStyleFunc, s string) string {
	if fn != nil {
		return fn(s)
	}
	return s
}
func (m *Markdown) Render(width int) ([]string, error) {
	if width <= 0 {
		return nil, nil
	}
	px := min(max(0, m.paddingX), (width-1)/2)
	available := max(1, width-2*px)
	source := m.text
	if m.options.Transform != nil {
		source = m.options.Transform(source, available)
	}
	source = SafeTerminalText(source)
	if strings.TrimSpace(source) == "" {
		return nil, nil
	}
	data := []byte(source)
	root := goldmark.New(goldmark.WithExtensions(extension.Strikethrough, extension.TaskList, extension.Linkify)).Parser().Parse(text.NewReader(data))
	renderer := *m
	renderer.inlineStyle = renderer.defaultStyle
	renderer.inlinePrefix = stylePrefix(renderer.inlineStyle)
	lines := renderer.blocks(root, data, available, 0)
	var out []string
	blank := strings.Repeat(" ", width)
	for i := 0; i < max(0, m.paddingY); i++ {
		out = append(out, blank)
	}
	for _, line := range lines {
		wrapped, _ := WrapTextWithANSI(line, available)
		for _, part := range wrapped {
			w, _ := VisibleWidth(part)
			part = strings.Repeat(" ", px) + part + strings.Repeat(" ", max(0, width-px-w))
			if m.defaultTextStyle != nil {
				part = styled(m.defaultTextStyle.BGColor, part)
			}
			out = append(out, part)
		}
	}
	for i := 0; i < max(0, m.paddingY); i++ {
		out = append(out, blank)
	}
	return out, nil
}
func (m *Markdown) inline(parent ast.Node, data []byte) string {
	var b strings.Builder
	for node := parent.FirstChild(); node != nil; node = node.NextSibling() {
		var s string
		switch n := node.(type) {
		case *ast.Text:
			s = string(n.Segment.Value(data))
			if n.SoftLineBreak() || n.HardLineBreak() {
				s += "\n"
			}
			for next, ok := node.NextSibling().(*ast.Text); ok; next, ok = node.NextSibling().(*ast.Text) {
				s += string(next.Segment.Value(data))
				if next.SoftLineBreak() || next.HardLineBreak() {
					s += "\n"
				}
				node = next
			}
			if m.options.PreserveBackslashEscapes == nil || !*m.options.PreserveBackslashEscapes {
				s = string(util.UnescapePunctuations([]byte(s)))
			}
			s = html.UnescapeString(s)
			s = m.styleText(s)
		case *ast.String:
			s = m.styleText(string(n.Value))
		case *ast.Emphasis:
			s = m.inline(n, data)
			if n.Level == 2 {
				s = styled(m.theme.Bold, s)
			} else {
				s = styled(m.theme.Italic, s)
			}
			s += m.inlinePrefix
		case *ast.CodeSpan:
			s = styled(m.theme.Code, string(n.Text(data))) + m.inlinePrefix
		case *ast.Link:
			s = styled(m.theme.Link, styled(m.theme.Underline, m.inline(n, data)))
			url := string(n.Destination)
			label := string(n.Text(data))
			if label != url && label != strings.TrimPrefix(url, "mailto:") {
				s += styled(m.theme.LinkURL, " ("+url+")")
			}
			s += m.inlinePrefix
		case *ast.AutoLink:
			s = styled(m.theme.Link, styled(m.theme.Underline, m.styleText(string(n.Label(data))))) + m.inlinePrefix
		case *ast.RawHTML:
			for i := 0; i < n.Segments.Len(); i++ {
				seg := n.Segments.At(i)
				s += m.styleText(string(seg.Value(data)))
			}
		case *ext.Strikethrough:
			s = styled(m.theme.Strikethrough, m.inline(n, data)) + m.inlinePrefix
		case *ext.TaskCheckBox:
			if n.IsChecked {
				s = "[x] "
			} else {
				s = "[ ] "
			}
		default:
			s = m.inline(n, data)
		}
		b.WriteString(s)
	}
	result := b.String()
	for m.inlinePrefix != "" && strings.HasSuffix(result, m.inlinePrefix) {
		result = strings.TrimSuffix(result, m.inlinePrefix)
	}
	return result
}
func (m *Markdown) blocks(parent ast.Node, data []byte, width, depth int) []string {
	var out []string
	for n := parent.FirstChild(); n != nil; n = n.NextSibling() {
		blankPrevious := n.HasBlankPreviousLines()
		if _, quote := parent.(*ast.Blockquote); quote && !blankPrevious && n.PreviousSibling() != nil {
			previous := n.PreviousSibling()
			first := n
			for first.Lines().Len() == 0 && first.FirstChild() != nil {
				first = first.FirstChild()
			}
			if previous.Lines().Len() > 0 && first.Lines().Len() > 0 {
				end := previous.Lines().At(previous.Lines().Len() - 1).Stop
				start := first.Lines().At(0).Start
				blankPrevious = start > end && strings.Contains(string(data[end:start]), "\n")
			}
		}
		if len(out) > 0 && blankPrevious {
			out = append(out, "")
		}
		out = append(out, m.block(n, data, width, depth)...)
	}
	return out
}
func (m *Markdown) block(n ast.Node, data []byte, width, depth int) []string {
	var out []string
	switch n := n.(type) {
	case *ast.Heading:
		heading := *m
		heading.inlineStyle = func(s string) string {
			if n.Level == 1 {
				s = styled(m.theme.Underline, s)
			}
			return styled(m.theme.Heading, styled(m.theme.Bold, s))
		}
		heading.inlinePrefix = stylePrefix(heading.inlineStyle)
		s := heading.inline(n, data)
		if n.Level >= 3 {
			s = heading.inlineStyle(strings.Repeat("#", n.Level)+" ") + s
		}
		out = append(out, s)
		if next := n.NextSibling(); next != nil && !next.HasBlankPreviousLines() {
			out = append(out, "")
		}
	case *ast.Paragraph, *ast.TextBlock:
		out = append(out, m.inline(n, data))
	case *ast.FencedCodeBlock:
		lang := string(n.Language(data))
		out = append(out, m.code(n, data, lang)...)
	case *ast.CodeBlock:
		out = append(out, m.code(n, data, "")...)
	case *ast.List:
		out = append(out, m.list(n, data, width, depth)...)
	case *ast.Blockquote:
		quote := *m
		quoteStyle := func(s string) string { return styled(m.theme.Quote, styled(m.theme.Italic, s)) }
		quote.inlineStyle = nil
		quote.inlinePrefix = stylePrefix(quoteStyle)
		lines := quote.blocks(n, data, max(1, width-2), 0)
		for _, line := range lines {
			wrapped, _ := WrapTextWithANSI(line, max(1, width-2))
			for _, part := range wrapped {
				out = append(out, styled(m.theme.QuoteBorder, "│ ")+quoteStyle(strings.ReplaceAll(part, "\x1b[0m", "\x1b[0m"+quote.inlinePrefix)))
			}
		}
	case *ast.ThematicBreak:
		out = append(out, styled(m.theme.HR, strings.Repeat("─", min(80, max(1, width)))))
	default:
		if n.HasChildren() {
			out = append(out, m.blocks(n, data, width, depth)...)
		} else {
			for i := 0; i < n.Lines().Len(); i++ {
				seg := n.Lines().At(i)
				out = append(out, strings.TrimSuffix(string(seg.Value(data)), "\n"))
			}
		}
	}
	return out
}

func (m *Markdown) code(n ast.Node, data []byte, language string) []string {
	var code strings.Builder
	for i := 0; i < n.Lines().Len(); i++ {
		seg := n.Lines().At(i)
		code.Write(seg.Value(data))
	}
	body := strings.TrimSuffix(code.String(), "\n")
	lines := strings.Split(body, "\n")
	if m.theme.HighlightCode != nil {
		lines = m.theme.HighlightCode(body, &language)
	} else {
		for i, s := range lines {
			lines[i] = styled(m.theme.CodeBlock, s)
		}
	}
	indent := "  "
	if m.theme.CodeBlockIndent != nil {
		indent = *m.theme.CodeBlockIndent
	}
	out := []string{styled(m.theme.CodeBlockBorder, "```"+language)}
	for _, s := range lines {
		out = append(out, indent+s)
	}
	return append(out, styled(m.theme.CodeBlockBorder, "```"))
}

func (m *Markdown) list(n *ast.List, data []byte, width, depth int) []string {
	var out []string
	index := n.Start
	for item := n.FirstChild(); item != nil; item = item.NextSibling() {
		marker := "- "
		if n.IsOrdered() {
			marker = fmt.Sprintf("%d. ", index)
			index++
		}
		if m.options.PreserveOrderedListMarkers != nil && *m.options.PreserveOrderedListMarkers {
			first := item.FirstChild()
			if first != nil && first.Lines().Len() > 0 {
				seg := first.Lines().At(0)
				start := strings.LastIndex(string(data[:seg.Start]), "\n") + 1
				prefix := strings.TrimSpace(string(data[start:seg.Start]))
				if prefix != "" {
					marker = prefix + " "
				}
			}
		}
		prefix := strings.Repeat("    ", depth) + marker
		continuation := strings.Repeat(" ", len(prefix))
		first := true
		for block := item.FirstChild(); block != nil; block = block.NextSibling() {
			if !first && block.HasBlankPreviousLines() {
				out = append(out, "")
			}
			if list, ok := block.(*ast.List); ok {
				out = append(out, m.list(list, data, width, depth+1)...)
				continue
			}
			lines, _ := WrapTextWithANSI(strings.Join(m.block(block, data, max(1, width-len(prefix)), 0), "\n"), max(1, width-len(prefix)))
			for _, line := range lines {
				p := continuation
				if first {
					p = strings.Repeat("    ", depth) + styled(m.theme.ListBullet, marker)
					first = false
				}
				out = append(out, p+line)
			}
		}
	}

	return out
}

func stylePrefix(style TextStyleFunc) string {
	const sentinel = "\x00"
	value := styled(style, sentinel)
	index := strings.Index(value, sentinel)
	if index < 0 {
		return ""
	}
	return value[:index]
}
func (m *Markdown) styleText(s string) string {
	parts := strings.Split(s, "\n")
	for i := range parts {
		parts[i] = styled(m.inlineStyle, parts[i])
	}
	return strings.Join(parts, "\n")
}
func (m *Markdown) defaultStyle(s string) string {
	style := m.defaultTextStyle
	if style == nil {
		return s
	}
	s = styled(style.Color, s)
	for _, v := range []struct {
		enabled *bool
		style   TextStyleFunc
	}{{style.Bold, m.theme.Bold}, {style.Italic, m.theme.Italic}, {style.Strikethrough, m.theme.Strikethrough}, {style.Underline, m.theme.Underline}} {
		if v.enabled != nil && *v.enabled {
			s = styled(v.style, s)
		}
	}
	return s
}

package tui_test

import (
	"encoding/json"
	"github.com/nankedr/pig/tui"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestMarkdownParity112(t *testing.T) {
	data, err := os.ReadFile("../parity/oracle/fixtures/text-rendering.json")
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Case struct {
			Input struct {
				Markdown []struct {
					Text    string
					Width   int
					Options struct {
						PreserveOrderedListMarkers bool
						PreserveBackslashEscapes   bool
					}
				}
			}
		}
		Observation struct{ Outcome struct{ Markdown [][]string } }
	}
	if err = json.Unmarshal(data, &f); err != nil {
		t.Fatal(err)
	}
	for i, c := range f.Case.Input.Markdown {
		m := tui.NewMarkdown(c.Text, 0, 0, tui.MarkdownTheme{}, nil, tui.MarkdownOptions{PreserveOrderedListMarkers: &c.Options.PreserveOrderedListMarkers, PreserveBackslashEscapes: &c.Options.PreserveBackslashEscapes})
		lines, err := m.Render(c.Width)
		if err != nil {
			t.Fatal(err)
		}
		for j, line := range lines {
			line, err = tui.StripTerminalSequences(line)
			if err != nil {
				t.Fatal(err)
			}
			lines[j] = strings.TrimRight(line, " ")
		}
		if !reflect.DeepEqual(lines, f.Observation.Outcome.Markdown[i]) {
			t.Errorf("case %d: got %#v want %#v", i, lines, f.Observation.Outcome.Markdown[i])
		}
	}
}

func TestTerminalTextControls112(t *testing.T) {
	input := "\x1b[31m红色👨‍👩‍👧‍👦é\x1b[0m\x1b[2J\x1b]52;c;secret\a\x1b]8;;https://example.com\aLINK\x1b]8;;\a"
	safe := tui.SafeTerminalText(input)
	if strings.Contains(safe, "secret") || strings.Contains(safe, "[2J") || strings.Contains(safe, "https:") {
		t.Fatalf("unsafe controls: %q", safe)
	}
	plain, err := tui.StripTerminalSequences(safe)
	if err != nil || plain != "红色👨‍👩‍👧‍👦éLINK" {
		t.Fatalf("text: %q %v", plain, err)
	}
	lines, err := tui.WrapTextWithANSI(safe, 5)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range lines {
		width, _ := tui.VisibleWidth(line)
		if width > 5 {
			t.Fatalf("overflow %q", line)
		}
	}
	truncated, err := tui.TruncateToWidth("\x1b[31m你好👨‍👩‍👧‍👦abc", 5)
	if err != nil {
		t.Fatal(err)
	}
	text, _ := tui.StripTerminalSequences(truncated)
	if text != "你好…" {
		t.Fatalf("truncate: %q", text)
	}
	m := tui.NewMarkdown("ignored", 1, 1, tui.MarkdownTheme{}, nil, tui.MarkdownOptions{Transform: func(s string, width int) string {
		if width != 8 {
			t.Errorf("transform width %d", width)
		}
		return "**content**"
	}})
	out, err := m.Render(10)
	if err != nil || len(out) != 3 {
		t.Fatalf("padding %#v %v", out, err)
	}
}

package codingagent_test

import (
	"encoding/json"
	"fmt"
	"github.com/nankedr/pig/codingagent"
	"github.com/nankedr/pig/tui"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"
)

func markdownCells120(lines []string) [][]string {
	result := [][]string{}
	for _, line := range lines {
		cells := []string{}
		fg := ""
		bold, italic, underline, strike := false, false, false, false
		for len(line) > 0 {
			if line[0] == 27 {
				code, ok, _ := tui.ExtractANSICode(line, 0)
				if !ok {
					panic("ANSI")
				}
				raw := code.Code
				if strings.HasPrefix(raw, "\x1b[") && strings.HasSuffix(raw, "m") {
					parts := strings.Split(raw[2:len(raw)-1], ";")
					for i := 0; i < len(parts); i++ {
						n, _ := strconv.Atoi(parts[i])
						switch n {
						case 0:
							fg = ""
							bold = false
							italic = false
							underline = false
							strike = false
						case 1:
							bold = true
						case 22:
							bold = false
						case 3:
							italic = true
						case 23:
							italic = false
						case 4:
							underline = true
						case 24:
							underline = false
						case 9:
							strike = true
						case 29:
							strike = false
						case 39:
							fg = ""
						case 38:
							count := 2
							if i+1 < len(parts) && parts[i+1] == "2" {
								count = 4
							}
							if i+count < len(parts) {
								fg = strings.Join(parts[i+1:i+count+1], ";")
								i += count
							}
						}
					}
				}
				line = line[len(raw):]
				continue
			}
			r, n := utf8.DecodeRuneInString(line)
			line = line[n:]
			if r == ' ' {
				cells = append(cells, " ")
			} else {
				cells = append(cells, fmt.Sprintf("%c:%s:%t%t%t%t", r, fg, bold, italic, underline, strike))
			}
		}
		for len(cells) > 0 && cells[len(cells)-1] == " " {
			cells = cells[:len(cells)-1]
		}
		result = append(result, cells)
	}
	return result
}
func TestThemeMarkdownParity120(t *testing.T) {
	data, err := os.ReadFile("../parity/oracle/fixtures/theme-runtime.json")
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Case struct {
			Input struct {
				Markdown       []string
				MarkdownWidths []int
			}
		}
		Observation struct {
			Outcome struct{ Markdown map[string][][]string }
		}
	}
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatal(err)
	}
	for name, expected := range f.Observation.Outcome.Markdown {
		theme, _ := codingagent.LoadBuiltinTheme(name, codingagent.ColorModeTrueColor)
		yes := true
		for i, source := range f.Case.Input.Markdown {
			got, err := tui.NewMarkdown(source, 0, 0, theme.MarkdownTheme(), &tui.DefaultTextStyle{Color: func(s string) string { return theme.FG("text", s) }, Italic: &yes}).Render(f.Case.Input.MarkdownWidths[i])
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(markdownCells120(got), markdownCells120(expected[i])) {
				t.Errorf("%s case %d\ngot %q\nwant %q", name, i, got, expected[i])
			}
		}
	}
}

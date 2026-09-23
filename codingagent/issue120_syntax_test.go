package codingagent_test

import (
	"encoding/json"
	"github.com/nankedr/pig/codingagent"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestThemeSyntaxParity120(t *testing.T) {
	data, err := os.ReadFile("../parity/oracle/fixtures/theme-runtime.json")
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Case        struct{ Input struct{ Syntax [][2]string } }
		Observation struct {
			Outcome struct{ Syntax map[string][][]string }
		}
	}
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatal(err)
	}
	for name, want := range f.Observation.Outcome.Syntax {
		theme, err := codingagent.LoadBuiltinTheme(name, codingagent.ColorModeTrueColor)
		if err != nil {
			t.Fatal(err)
		}
		for i, sample := range f.Case.Input.Syntax {
			t.Run(name+"/"+sample[0], func(t *testing.T) {
				highlighter := theme.MarkdownTheme().HighlightCode
				if highlighter == nil {
					t.Fatal("theme has no syntax highlighter")
				}
				got := highlighter(sample[1], &sample[0])
				if !reflect.DeepEqual(got, want[i]) {
					t.Fatalf("got %q, want %q", got, want[i])
				}
			})
		}
	}
}

func TestThemePublicHelpers120(t *testing.T) {
	t.Setenv("COLORTERM", "truecolor")
	lines, err := codingagent.HighlightCode("var x = 42", "go")
	if err != nil || len(lines) != 1 || !strings.Contains(lines[0], "\x1b[") {
		t.Fatalf("highlight: %q %v", lines, err)
	}
	markdown, err := codingagent.GetMarkdownTheme()
	if err != nil || markdown.HighlightCode == nil {
		t.Fatal("markdown helper", err)
	}
	selectTheme, err := codingagent.GetSelectListTheme()
	if err != nil || selectTheme.SelectedText == nil {
		t.Fatal("select helper", err)
	}
	settings, err := codingagent.GetSettingsListTheme()
	if err != nil || settings.Value == nil {
		t.Fatal("settings helper", err)
	}
}

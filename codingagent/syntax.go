package codingagent

import (
	_ "embed"
	"html"
	"regexp"
	"strings"
	"sync"

	"github.com/dop251/goja"
)

//go:embed syntax/highlight.js
var highlightSource string

var syntaxEngine struct {
	sync.Mutex
	once  sync.Once
	vm    *goja.Runtime
	call  goja.Callable
	err   error
	cache map[[2]string]*string
}

func syntaxHTML(code, language string) (*string, error) {
	syntaxEngine.Lock()
	defer syntaxEngine.Unlock()
	syntaxEngine.once.Do(func() {
		syntaxEngine.vm = goja.New()
		_, syntaxEngine.err = syntaxEngine.vm.RunString(highlightSource)
		syntaxEngine.call, _ = goja.AssertFunction(syntaxEngine.vm.Get("pigHighlight"))
		syntaxEngine.cache = make(map[[2]string]*string)
	})
	if syntaxEngine.err != nil {
		return nil, syntaxEngine.err
	}
	key := [2]string{code, language}
	if value, ok := syntaxEngine.cache[key]; ok {
		return value, nil
	}
	value, err := syntaxEngine.call(goja.Undefined(), syntaxEngine.vm.ToValue(code), syntaxEngine.vm.ToValue(language))
	if err != nil {
		return nil, err
	}
	var result *string
	if !goja.IsNull(value) {
		s := value.String()
		result = &s
	}
	if len(syntaxEngine.cache) >= 128 {
		clear(syntaxEngine.cache)
	}
	if len(code) <= 65536 {
		syntaxEngine.cache[key] = result
	}
	return result, nil
}

var syntaxSpan = regexp.MustCompile(`<span class="([^"]*)">|</span>`)
var syntaxColors = map[string]ThemeColor{
	"keyword": "syntaxKeyword", "built_in": "syntaxType", "literal": "syntaxNumber", "number": "syntaxNumber", "regexp": "syntaxString", "string": "syntaxString", "comment": "syntaxComment", "doctag": "syntaxComment", "meta": "muted", "function": "syntaxFunction", "title": "syntaxFunction", "class": "syntaxType", "type": "syntaxType", "tag": "syntaxPunctuation", "name": "syntaxKeyword", "attr": "syntaxVariable", "variable": "syntaxVariable", "params": "syntaxVariable", "operator": "syntaxOperator", "punctuation": "syntaxPunctuation", "addition": "toolDiffAdded", "deletion": "toolDiffRemoved",
}

func (t *Theme) highlightCode(code string, language *string) []string {
	lang := ""
	if language != nil {
		lang = *language
	}
	highlighted, err := syntaxHTML(code, lang)
	if err != nil || highlighted == nil {
		lines := strings.Split(code, "\n")
		for i := range lines {
			lines[i] = t.FG("mdCodeBlock", lines[i])
		}
		return lines
	}
	var out strings.Builder
	var scopes []string
	emit := func(text string) {
		if text == "" {
			return
		}
		text = html.UnescapeString(text)
		for i := len(scopes) - 1; i >= 0; i-- {
			scope := scopes[i]
			if color, ok := syntaxColors[scope]; ok {
				text = t.FG(color, text)
				break
			}
			if scope == "emphasis" {
				text = t.Italic(text)
				break
			}
			if scope == "strong" {
				text = t.Bold(text)
				break
			}
			if scope == "link" {
				text = t.Underline(text)
				break
			}
		}
		out.WriteString(text)
	}
	pos := 0
	for _, match := range syntaxSpan.FindAllStringSubmatchIndex(*highlighted, -1) {
		emit((*highlighted)[pos:match[0]])
		if match[2] < 0 {
			if len(scopes) > 0 {
				scopes = scopes[:len(scopes)-1]
			}
		} else {
			scope := ""
			for _, class := range strings.Fields((*highlighted)[match[2]:match[3]]) {
				if strings.HasPrefix(class, "hljs-") {
					scope = strings.TrimPrefix(class, "hljs-")
					break
				}
			}
			if parts := strings.FieldsFunc(scope, func(r rune) bool { return r == '.' || r == '-' }); len(parts) > 0 {
				scope = parts[0]
			}
			scopes = append(scopes, scope)
		}
		pos = match[1]
	}
	emit((*highlighted)[pos:])
	return strings.Split(out.String(), "\n")
}

func HighlightCode(code string, language ...string) ([]string, error) {
	theme, err := LoadBuiltinTheme("dark")
	if err != nil {
		return nil, err
	}
	var lang *string
	if len(language) > 0 {
		lang = &language[0]
	}
	return theme.highlightCode(code, lang), nil
}

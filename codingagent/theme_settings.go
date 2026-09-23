package codingagent

import (
	"github.com/nankedr/pig/tui"
	"sort"
	"strings"
)

// ThemeSettingsComponent is the theme submenu, including automatic light/dark choices.
type ThemeSettingsComponent struct {
	content                       tui.Component
	input                         tui.Component
	original, single, light, dark string
	scheme                        tui.TerminalColorScheme
	names                         []string
	callbacks                     SettingsCallbacks
	theme                         func() *Theme
	keybindings                   *tui.KeybindingsManager
}

func NewThemeSettingsComponent(setting string, scheme tui.TerminalColorScheme, loaded ThemeLoadResult, callbacks SettingsCallbacks, theme func() *Theme) *ThemeSettingsComponent {
	s := &ThemeSettingsComponent{original: setting, scheme: scheme, callbacks: callbacks, theme: theme}
	names := map[string]bool{"dark": true, "light": true}
	for _, t := range loaded.Themes {
		if t != nil {
			names[t.Name] = true
		}
	}
	for name := range names {
		s.names = append(s.names, name)
	}
	sort.Strings(s.names)
	s.single = "dark"
	if names[setting] {
		s.single = setting
	}
	s.light, s.dark = s.single, s.single
	if _, auto := resolveThemeSetting(setting, scheme); auto {
		parts := strings.Split(setting, "/")
		s.light, s.dark = strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		s.single = s.active()
		s.showAutomatic()
	} else {
		s.showSingle()
	}
	return s
}
func (s *ThemeSettingsComponent) SetKeybindings(kb *tui.KeybindingsManager) {
	s.keybindings = kb
	s.bind()
}
func (s *ThemeSettingsComponent) bind() {
	switch v := s.input.(type) {
	case *tui.SelectList:
		v.SetKeybindings(s.keybindings)
	case *tui.SettingsList:
		v.SetKeybindings(s.keybindings)
	}
}
func (s *ThemeSettingsComponent) HandleInput(data string) error {
	return s.input.(tui.ComponentInputHandler).HandleInput(data)
}
func (s *ThemeSettingsComponent) Invalidate() error { return s.content.Invalidate() }
func (s *ThemeSettingsComponent) Render(width int) ([]string, error) {
	if list, ok := s.input.(*tui.SettingsList); ok {
		list.SetTheme(settingsListTheme(s.theme))
	}
	return s.content.Render(width)
}
func (s *ThemeSettingsComponent) active() string {
	if s.scheme == tui.TerminalColorSchemeLight {
		return s.light
	}
	return s.dark
}
func (s *ThemeSettingsComponent) preview(value string) {
	if s.callbacks.OnThemePreview != nil {
		s.callbacks.OnThemePreview(value)
	}
}
func (s *ThemeSettingsComponent) cancel() {
	s.preview(s.original)
	if s.callbacks.OnCancel != nil {
		s.callbacks.OnCancel()
	}
}
func (s *ThemeSettingsComponent) apply(value string) {
	if s.callbacks.OnThemeChange != nil {
		s.callbacks.OnThemeChange(value)
	}
}
func (s *ThemeSettingsComponent) items() []tui.SelectItem {
	items := []tui.SelectItem{}
	for _, name := range s.names {
		items = append(items, tui.SelectItem{Value: name, Label: name})
	}
	return items
}
func (s *ThemeSettingsComponent) selectMenu(title, description, current string, items []tui.SelectItem, selectValue func(string), cancel func(), preview func(string)) (tui.Component, *tui.SelectList) {
	minWidth, maxWidth := 12, 32
	fg := func(color ThemeColor) tui.TextStyleFunc {
		return func(v string) string { return s.theme().FG(color, v) }
	}
	list := tui.NewSelectList(items, min(10, len(items)), tui.SelectListTheme{SelectedText: fg("accent"), Description: fg("muted"), ScrollInfo: fg("muted"), NoMatch: fg("muted")}, tui.SelectListLayoutOptions{MinPrimaryColumnWidth: &minWidth, MaxPrimaryColumnWidth: &maxWidth})
	for i, item := range items {
		if item.Value == current {
			_ = list.SetSelectedIndex(i)
		}
	}
	list.OnSelect = func(i tui.SelectItem) { selectValue(i.Value) }
	list.OnCancel = cancel
	list.OnSelectionChange = func(i tui.SelectItem) {
		if preview != nil {
			preview(i.Value)
		}
	}
	list.SetKeybindings(s.keybindings)
	content := &themeMenuContent{title: title, description: description, body: list, theme: s.theme, hint: true, keybindings: func() *tui.KeybindingsManager { return s.keybindings }}
	return content, list
}
func (s *ThemeSettingsComponent) showSingle() {
	description := "Use separate themes for light and dark terminal appearance"
	items := append([]tui.SelectItem{{Value: "/", Label: "Automatic", Description: &description}}, s.items()...)
	s.content, s.input = s.selectMenu("Theme", "Select a theme, or choose Automatic to follow terminal appearance.", s.single, items, func(value string) {
		if value == "/" {
			s.preview(s.light + "/" + s.dark)
			s.showAutomatic()
		} else {
			s.single = value
			s.apply(value)
		}
	}, s.cancel, func(value string) {
		if value == "/" {
			value = s.light + "/" + s.dark
		}
		s.preview(value)
	})
}
func (s *ThemeSettingsComponent) showAutomatic() {
	items := []tui.SettingItem{}
	for _, kind := range []string{"Light", "Dark"} {
		lower := strings.ToLower(kind)
		value := s.light
		if lower == "dark" {
			value = s.dark
		}
		description := "Theme to use in automatic mode when the terminal is " + lower
		items = append(items, tui.SettingItem{ID: lower, Label: kind + " theme", Description: &description, CurrentValue: value, Submenu: func(current string, done func(*string)) tui.Component {
			content, _ := s.selectMenu(kind+" Theme", "Select the theme to use for "+lower+" terminal appearance", current, s.items(), func(value string) {
				if lower == "light" {
					s.light = value
				} else {
					s.dark = value
				}
				s.preview(s.light + "/" + s.dark)
				done(&value)
			}, func() { s.preview(s.light + "/" + s.dark); done(nil) }, s.preview)
			return content
		}})
	}
	applyDesc, modeDesc := "Save and go back", "Switch to one theme for light and dark"
	items = append(items, tui.SettingItem{ID: "apply", Label: "Apply", Description: &applyDesc, CurrentValue: "save and go back", Values: []string{"save and go back"}}, tui.SettingItem{ID: "single", Label: "Change mode", Description: &modeDesc, CurrentValue: "switch to single theme", Values: []string{"switch to single theme"}})
	list := tui.NewSettingsList(items, 10, settingsListTheme(s.theme), func(id, value string) {
		if id == "apply" {
			s.apply(s.light + "/" + s.dark)
		} else if id == "single" {
			s.single = s.active()
			s.preview(s.single)
			s.showSingle()
		}
	}, s.cancel)
	s.content = &themeMenuContent{title: "Automatic Theme", description: "Choose themes for terminal light and dark appearance.\nLight/dark detection requires terminal support.", body: list, theme: s.theme}
	s.input = list
	s.bind()
}
func settingsListTheme(theme func() *Theme) tui.SettingsListTheme {
	return tui.SettingsListTheme{Label: func(s string, selected bool) string {
		if selected {
			return theme().FG("accent", s)
		}
		return s
	}, Value: func(s string, selected bool) string {
		if selected {
			return theme().FG("accent", s)
		}
		return theme().FG("muted", s)
	}, Description: func(s string) string { return theme().FG("dim", s) }, Cursor: theme().FG("accent", "→ "), Hint: func(s string) string { return theme().FG("dim", s) }}
}

type themeMenuContent struct {
	title, description string
	body               tui.Component
	theme              func() *Theme
	hint               bool
	keybindings        func() *tui.KeybindingsManager
}

func (c *themeMenuContent) HandleInput(data string) error {
	return c.body.(tui.ComponentInputHandler).HandleInput(data)
}
func (c *themeMenuContent) Invalidate() error { return c.body.Invalidate() }
func (c *themeMenuContent) Render(width int) ([]string, error) {
	t := c.theme()
	title, _ := tui.WrapTextWithANSI(t.Bold(t.FG("accent", c.title)), max(1, width))
	description, _ := tui.WrapTextWithANSI(t.FG("muted", c.description), max(1, width))
	lines := append(append(append(title, ""), description...), "")
	body, err := c.body.Render(width)
	lines = append(lines, body...)
	if c.hint {
		confirm, cancel := "Enter", "Esc"
		if c.keybindings != nil && c.keybindings() != nil {
			confirm = tui.SelectionKeyHint(c.keybindings(), "tui.select.confirm")
			cancel = tui.SelectionKeyHint(c.keybindings(), "tui.select.cancel")
		}
		hint, _ := tui.WrapTextWithANSI(t.FG("dim", "  "+confirm+" to select · "+cancel+" to go back"), max(1, width))
		lines = append(append(lines, ""), hint...)
	}
	return lines, err
}

type themeSettingsFrame struct {
	diagnostic func() error
	body       tui.Component
	theme      func() *Theme
}

func (f *themeSettingsFrame) HandleInput(data string) error {
	return f.body.(tui.ComponentInputHandler).HandleInput(data)
}
func (f *themeSettingsFrame) Invalidate() error { return f.body.Invalidate() }
func (f *themeSettingsFrame) Render(width int) ([]string, error) {
	if list, ok := f.body.(*tui.SettingsList); ok {
		list.SetTheme(settingsListTheme(f.theme))
	}
	lines, err := f.body.Render(width)
	if f.diagnostic != nil {
		if diagnostic := f.diagnostic(); diagnostic != nil {
			message, _ := tui.WrapTextWithANSI(f.theme().FG("error", diagnostic.Error()), max(1, width))
			lines = append(append(lines, ""), message...)
		}
	}
	for i, line := range lines {
		lines[i] = f.theme().FG("text", line)
	}
	border := f.theme().FG("border", strings.Repeat("─", max(0, width)))
	return append(append([]string{border}, lines...), border), err
}

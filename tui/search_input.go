package tui

import "strings"

func (i *Input) prepare() {
	if i.editor == nil {
		i.editor = NewEditor(nil, EditorTheme{})
	}
	if i.editor.GetText() != i.value {
		_ = i.editor.SetText(i.value)
	}
	i.editor.Focused = i.Focused
}
func (i *Input) HandleInput(data string) error {
	i.prepare()
	kb, _ := GetKeybindings()
	confirm, _ := kb.Matches(data, "tui.select.confirm")
	cancel, _ := kb.Matches(data, "tui.select.cancel")
	if confirm {
		if i.OnSubmit != nil {
			i.OnSubmit(i.value)
		}
		return nil
	}
	if cancel {
		if i.OnEscape != nil {
			i.OnEscape()
		}
		return nil
	}
	err := i.editor.HandleInput(data)
	i.value = strings.Join(strings.FieldsFunc(i.editor.GetText(), func(r rune) bool { return r == '\n' || r == '\r' }), " ")
	if i.editor.GetText() != i.value {
		_ = i.editor.SetText(i.value)
	}
	return err
}
func (i *Input) Render(width int) ([]string, error) {
	i.prepare()
	lines, err := i.editor.Render(max(1, width))
	if len(lines) > 2 {
		return lines[1 : len(lines)-1], err
	}
	return lines, err
}

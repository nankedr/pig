package codingagent

import (
	"github.com/nankedr/pig/tui"
	"sort"
	"strings"
)

type ThemeSelectorComponent struct {
	tui.Container
	list *tui.SelectList
}

func NewThemeSelectorComponent(current string, loaded ThemeLoadResult, onSelect func(string), onCancel func(), onPreview func(string)) *ThemeSelectorComponent {
	names := map[string]bool{"dark": true, "light": true}
	for _, t := range loaded.Themes {
		if t != nil {
			names[t.Name] = true
		}
	}
	sorted := make([]string, 0, len(names))
	for name := range names {
		sorted = append(sorted, name)
	}
	sort.Strings(sorted)
	items := make([]tui.SelectItem, 0, len(sorted))
	index := 0
	for i, name := range sorted {
		item := tui.SelectItem{Value: name, Label: name}
		if name == current {
			description := "(current)"
			item.Description = &description
			index = i
		}
		items = append(items, item)
	}
	minimum, maximum := 12, 32
	list := tui.NewSelectList(items, 10, tui.SelectListTheme{}, tui.SelectListLayoutOptions{MinPrimaryColumnWidth: &minimum, MaxPrimaryColumnWidth: &maximum})
	list.SetSelectedIndex(index)
	list.OnSelect = func(item tui.SelectItem) {
		if onSelect != nil {
			onSelect(item.Value)
		}
	}
	list.OnCancel = onCancel
	list.OnSelectionChange = func(item tui.SelectItem) {
		if onPreview != nil {
			onPreview(item.Value)
		}
	}
	return &ThemeSelectorComponent{list: list}
}
func (s *ThemeSelectorComponent) GetSelectList() (*tui.SelectList, error) {
	if s.list == nil {
		return nil, notImplemented("ThemeSelectorComponent.GetSelectList")
	}
	return s.list, nil
}
func (s *ThemeSelectorComponent) HandleInput(data string) error {
	if s.list == nil {
		return nil
	}
	return s.list.HandleInput(data)
}
func (s *ThemeSelectorComponent) Render(width int) ([]string, error) {
	if s.list == nil {
		return s.Container.Render(width)
	}
	lines, err := s.list.Render(width)
	border := strings.Repeat("─", max(0, width))
	return append(append([]string{border}, lines...), border), err
}

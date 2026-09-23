package tui

import (
	"fmt"
	"strings"
)

func (s *SettingsList) SetTheme(theme SettingsListTheme) { s.theme = theme }

func (s *SettingsList) SetKeybindings(kb *KeybindingsManager) {
	s.keybindings = kb
	if s.search != nil {
		s.search.SetKeybindings(kb)
	}
}
func (s *SettingsList) UpdateValue(id, value string) error {
	for i := range s.items {
		if s.items[i].ID == id {
			s.items[i].CurrentValue = value
			break
		}
	}
	return nil
}
func (s *SettingsList) Invalidate() error {
	if s.submenu != nil {
		return s.submenu.Invalidate()
	}
	return nil
}
func (s *SettingsList) HandleInput(data string) error {
	if s.submenu != nil {
		if handler, ok := s.submenu.(ComponentInputHandler); ok {
			return handler.HandleInput(data)
		}
		return nil
	}
	kb := s.keybindings
	if kb == nil {
		kb, _ = GetKeybindings()
	}
	matches := func(key Keybinding) bool { ok, _ := kb.Matches(data, key); return ok }
	switch {
	case matches("tui.select.cancel"):
		if s.onCancel != nil {
			s.onCancel()
		}
	case matches("tui.select.up"), matches("tui.select.down"):
		if len(s.filtered) > 0 {
			delta := 1
			if matches("tui.select.up") {
				delta = -1
			}
			s.selected = (s.selected + delta + len(s.filtered)) % len(s.filtered)
		}
	case matches("tui.select.confirm"), data == " " && (s.search == nil || s.search.GetValue() == ""):
		if len(s.filtered) == 0 {
			return nil
		}
		item := &s.items[s.filtered[s.selected]]
		if item.Submenu != nil {
			s.submenu = item.Submenu(item.CurrentValue, func(value *string) {
				if value != nil {
					item.CurrentValue = *value
					if s.onChange != nil {
						s.onChange(item.ID, *value)
					}
				}
				s.submenu = nil
			})
		} else if len(item.Values) > 0 {
			index := -1
			for i, value := range item.Values {
				if value == item.CurrentValue {
					index = i
					break
				}
			}
			item.CurrentValue = item.Values[(index+1)%len(item.Values)]
			if s.onChange != nil {
				s.onChange(item.ID, item.CurrentValue)
			}
		}
	default:
		if s.search != nil {
			if err := s.search.HandleInput(data); err != nil {
				return err
			}
			indices := make([]int, len(s.items))
			for i := range indices {
				indices[i] = i
			}
			s.filtered, _ = FuzzyFilter(indices, s.search.GetValue(), func(i int) string { return s.items[i].Label })
			s.selected = 0
		}
	}
	return nil
}
func (s *SettingsList) Render(width int) ([]string, error) {
	if s.submenu != nil {
		return s.submenu.Render(width)
	}
	var lines []string
	if s.search != nil {
		rendered, err := s.search.Render(max(1, width-2))
		if err != nil {
			return nil, err
		}
		for i := range rendered {
			rendered[i] = "> " + rendered[i]
		}
		lines = append(append(lines, rendered...), "")
	}
	hint := func() {
		text := "  Enter/Space to change · Esc to cancel"
		if s.search != nil {
			text = "  Type to search · Enter/Space to change · Esc to cancel"
		}
		text, _ = TruncateToWidth(styled(s.theme.Hint, text), width)
		lines = append(lines, "", text)
	}
	if len(s.items) == 0 {
		lines = append(lines, styled(s.theme.Hint, "  No settings available"))
		if s.search != nil {
			hint()
		}
		return lines, nil
	}
	if len(s.filtered) == 0 {
		line, _ := TruncateToWidth(styled(s.theme.Hint, "  No matching settings"), width)
		lines = append(lines, line)
		hint()
		return lines, nil
	}
	maxLabel := 0
	for _, item := range s.items {
		w, _ := VisibleWidth(item.Label)
		maxLabel = max(maxLabel, w)
	}
	maxLabel = min(30, maxLabel)
	count := max(1, s.maxVisible)
	start := max(0, min(s.selected-count/2, len(s.filtered)-count))
	end := min(start+count, len(s.filtered))
	for i := start; i < end; i++ {
		item := s.items[s.filtered[i]]
		selected := i == s.selected
		prefix := "  "
		if selected {
			prefix = s.theme.Cursor
		}
		pw, _ := VisibleWidth(prefix)
		lw, _ := VisibleWidth(item.Label)
		label := item.Label + strings.Repeat(" ", max(0, maxLabel-lw))
		value, _ := TruncateToWidth(item.CurrentValue, max(0, width-pw-maxLabel-4), TruncateOptions{Ellipsis: new(string)})
		if s.theme.Label != nil {
			label = s.theme.Label(label, selected)
		}
		if s.theme.Value != nil {
			value = s.theme.Value(value, selected)
		}
		line, _ := TruncateToWidth(prefix+label+"  "+value, width)
		lines = append(lines, line)
	}
	if start > 0 || end < len(s.filtered) {
		lines = append(lines, styled(s.theme.Hint, fmt.Sprintf("  (%d/%d)", s.selected+1, len(s.filtered))))
	}
	if d := s.items[s.filtered[s.selected]].Description; d != nil && *d != "" {
		lines = append(lines, "")
		wrapped, _ := WrapTextWithANSI(*d, max(1, width-4))
		for _, line := range wrapped {
			lines = append(lines, styled(s.theme.Description, "  "+line))
		}
	}
	hint()
	return lines, nil
}

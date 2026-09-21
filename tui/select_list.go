package tui

import (
	"fmt"
	"strings"
)

func (s *SelectList) GetSelectedItem() (*SelectItem, error) {
	if s.selected < 0 || s.selected >= len(s.filtered) {
		return nil, nil
	}
	item := s.filtered[s.selected]
	return &item, nil
}
func (s *SelectList) SetFilter(filter string) error {
	s.filtered = nil
	for _, item := range s.items {
		if strings.HasPrefix(strings.ToLower(item.Value), strings.ToLower(filter)) {
			s.filtered = append(s.filtered, item)
		}
	}
	s.selected = 0
	return nil
}
func (s *SelectList) SetSelectedIndex(index int) error {
	s.selected = max(0, min(index, len(s.filtered)-1))
	return nil
}
func (s *SelectList) HandleInput(data string) error {
	kb := s.keybindings
	if kb == nil {
		kb, _ = GetKeybindings()
	}
	matches := func(action Keybinding) bool { v, _ := kb.Matches(data, action); return v }
	switch {
	case matches("tui.select.up"), matches("tui.select.down"):
		if len(s.filtered) == 0 {
			return nil
		}
		delta := 1
		if matches("tui.select.up") {
			delta = -1
		}
		s.selected = (s.selected + delta + len(s.filtered)) % len(s.filtered)
		if s.OnSelectionChange != nil {
			s.OnSelectionChange(s.filtered[s.selected])
		}
	case matches("tui.select.confirm"):
		if item, _ := s.GetSelectedItem(); item != nil && s.OnSelect != nil {
			s.OnSelect(*item)
		}
	case matches("tui.select.cancel"):
		if s.OnCancel != nil {
			s.OnCancel()
		}
	}
	return nil
}
func selectStyle(fn TextStyleFunc, text string) string {
	if fn != nil {
		return fn(text)
	}
	return text
}
func (s *SelectList) Render(width int) ([]string, error) {
	if len(s.filtered) == 0 {
		return []string{selectStyle(s.theme.NoMatch, "  No matching commands")}, nil
	}
	lo, hi := 32, 32
	if s.layout.MinPrimaryColumnWidth != nil {
		lo = *s.layout.MinPrimaryColumnWidth
		hi = lo
	}
	if s.layout.MaxPrimaryColumnWidth != nil {
		hi = *s.layout.MaxPrimaryColumnWidth
		if s.layout.MinPrimaryColumnWidth == nil {
			lo = hi
		}
	}
	lo, hi = max(1, min(lo, hi)), max(1, max(lo, hi))
	column := 0
	display := func(item SelectItem) string {
		if item.Label != "" {
			return item.Label
		}
		return item.Value
	}
	for _, item := range s.filtered {
		w, _ := VisibleWidth(display(item))
		column = max(column, w+2)
	}
	column = max(lo, min(hi, column))
	start := max(0, min(s.selected-s.maxVisible/2, len(s.filtered)-s.maxVisible))
	end := min(start+s.maxVisible, len(s.filtered))
	lines := []string{}
	for i := start; i < end; i++ {
		item := s.filtered[i]
		selected := i == s.selected
		prefix := "  "
		if selected {
			prefix = "→ "
		}
		truncate := func(w, c int) string {
			text := display(item)
			if s.layout.TruncatePrimary != nil {
				text = s.layout.TruncatePrimary(SelectListTruncatePrimaryContext{Text: text, MaxWidth: w, ColumnWidth: c, Item: item, IsSelected: selected})
			}
			return sliceCells(text, 0, max(0, w))
		}
		line := ""
		if item.Description != nil && *item.Description != "" && width > 40 {
			c := max(1, min(column, width-6))
			value := truncate(max(1, c-2), c)
			w, _ := VisibleWidth(value)
			spacing := strings.Repeat(" ", max(1, c-w))
			remaining := width - 2 - w - len(spacing) - 2
			if remaining > 10 {
				description := strings.TrimSpace(strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ").Replace(*item.Description))
				line = prefix + value + selectStyle(s.theme.Description, spacing+sliceCells(description, 0, remaining))
				if selected {
					line = prefix + value + spacing + sliceCells(description, 0, remaining)
				}
			}
		}
		if line == "" {
			line = prefix + truncate(width-4, width-4)
		}
		if selected {
			line = selectStyle(s.theme.SelectedText, line)
		}
		lines = append(lines, line)
	}
	if start > 0 || end < len(s.filtered) {
		lines = append(lines, selectStyle(s.theme.ScrollInfo, sliceCells(fmt.Sprintf("  (%d/%d)", s.selected+1, len(s.filtered)), 0, max(0, width-2))))
	}
	return lines, nil
}

func (s *SelectList) SetKeybindings(kb *KeybindingsManager) { s.keybindings = kb }

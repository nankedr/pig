package codingagent

import (
	"cmp"
	"fmt"
	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/tui"
	"slices"
	"strings"
)

type ModelSelectorComponent struct {
	tui.Container
	Focused                  bool
	search                   *tui.Input
	models, scoped, filtered []ai.Model
	current                  *ai.Model
	scopedOnly, closed       bool
	selected                 int
	onSelect                 func(ai.Model)
	onCancel                 func()
	keybindings              *tui.KeybindingsManager
}

// NewModelSelectorComponent selects from a local snapshot without refreshing catalogs or saving settings.
func NewModelSelectorComponent(models []ai.Model, current *ai.Model, scope []ScopedModel, onSelect func(ai.Model), onCancel func(), initialSearch ...string) *ModelSelectorComponent {
	if current != nil {
		copy := cloneRuntimeModels([]ai.Model{*current})[0]
		current = &copy
	}
	s := &ModelSelectorComponent{models: cloneRuntimeModels(models), current: current, search: tui.NewInput(), onSelect: onSelect, onCancel: onCancel, Focused: true}
	slices.SortStableFunc(s.models, func(a, b ai.Model) int {
		ac, bc := ai.ModelsAreEqual(current, &a), ai.ModelsAreEqual(current, &b)
		if ac != bc {
			if ac {
				return -1
			}
			return 1
		}
		return cmp.Compare(a.Provider, b.Provider)
	})
	for _, item := range scope {
		s.scoped = append(s.scoped, item.Model)
	}
	s.scoped = cloneRuntimeModels(s.scoped)
	s.scopedOnly = len(s.scoped) > 0
	if len(initialSearch) > 0 {
		s.search.SetValue(initialSearch[0])
	}
	s.filter()
	if s.search.GetValue() == "" {
		for i, m := range s.filtered {
			if ai.ModelsAreEqual(current, &m) {
				s.selected = i
				break
			}
		}
	}
	return s
}
func (s *ModelSelectorComponent) Dispose() error { s.closed = true; return nil }
func (s *ModelSelectorComponent) GetSearchInput() (*tui.Input, error) {
	if s.search == nil {
		return nil, notImplemented("ModelSelectorComponent.GetSearchInput")
	}
	return s.search, nil
}
func (s *ModelSelectorComponent) filter() {
	models := s.models
	if s.scopedOnly {
		models = s.scoped
	}
	query := s.search.GetValue()
	s.filtered, _ = tui.FuzzyFilter(models, query, func(m ai.Model) string {
		return fmt.Sprintf("%s %s/%s %s %s %s", m.Provider, m.Provider, m.ID, m.Provider, m.ID, m.Name)
	})
	if query != "" {
		s.selected = 0
	} else {
		s.selected = max(0, min(s.selected, len(s.filtered)-1))
	}
}
func (s *ModelSelectorComponent) HandleInput(data string) error {
	if s.search == nil {
		return notImplemented("ModelSelectorComponent.HandleInput")
	}
	if s.closed {
		return nil
	}
	kb := s.keybindings
	if kb == nil {
		kb, _ = tui.GetKeybindings()
	}
	match := func(action tui.Keybinding) bool { v, _ := kb.Matches(data, action); return v }
	switch {
	case match("tui.input.tab"):
		if len(s.scoped) > 0 {
			s.scopedOnly = !s.scopedOnly
			s.selected = 0
			s.filter()
			if s.search.GetValue() == "" {
				for i, model := range s.filtered {
					if ai.ModelsAreEqual(s.current, &model) {
						s.selected = i
						break
					}
				}
			}
		}
	case match("tui.select.up"), match("tui.select.down"):
		if len(s.filtered) > 0 {
			delta := 1
			if match("tui.select.up") {
				delta = -1
			}
			s.selected = (s.selected + delta + len(s.filtered)) % len(s.filtered)
		}
	case match("tui.select.confirm"):
		if len(s.filtered) > 0 {
			_ = s.Dispose()
			if s.onSelect != nil {
				s.onSelect(cloneRuntimeModels(s.filtered[s.selected : s.selected+1])[0])
			}
		}
	case match("tui.select.cancel"):
		_ = s.Dispose()
		if s.onCancel != nil {
			s.onCancel()
		}
	default:
		if err := s.search.HandleInput(data); err != nil {
			return err
		}
		s.filter()
	}
	return nil
}
func (s *ModelSelectorComponent) Render(width int) ([]string, error) {
	if s.search == nil {
		return s.Container.Render(width)
	}
	s.search.Focused = s.Focused
	input, err := s.search.Render(width)
	if err != nil {
		return nil, err
	}
	title := "Select Model — configured providers"
	if len(s.scoped) > 0 {
		scope := "all"
		if s.scopedOnly {
			scope = "scoped"
		}
		title += " · Scope: " + scope + " (Tab all/scoped)"
	}
	lines := append([]string{title}, input...)
	start := max(0, min(s.selected-5, len(s.filtered)-10))
	for i := start; i < min(start+10, len(s.filtered)); i++ {
		m := s.filtered[i]
		prefix := "  "
		if i == s.selected {
			prefix = "→ "
		}
		check := ""
		if ai.ModelsAreEqual(s.current, &m) {
			check = " ✓"
		}
		lines = append(lines, prefix+tui.SafeTerminalText(m.ID)+" ["+tui.SafeTerminalText(string(m.Provider))+"]"+check)
	}
	if len(s.filtered) == 0 {
		lines = append(lines, "No matching models")
	} else {
		lines = append(lines, "Model Name: "+tui.SafeTerminalText(s.filtered[s.selected].Name))
	}
	lines = append(lines, "↑↓ navigate · Enter select · Esc cancel")
	return selectorLines(lines, width), nil
}
func selectorLines(lines []string, width int) []string {
	for i, line := range lines {
		if !strings.Contains(line, tui.CursorMarker) {
			line = tui.SafeTerminalText(line)
		}
		lines[i], _ = tui.TruncateToWidth(line, max(1, width))
	}
	return lines
}

// ThinkingSelectorComponent offers only the supplied model's supported levels.
type ThinkingSelectorComponent struct {
	tui.Container
	list *tui.SelectList
}

func NewThinkingSelectorComponent(current agent.ThinkingLevel, levels []agent.ThinkingLevel, onSelect func(agent.ThinkingLevel), onCancel func()) *ThinkingSelectorComponent {
	descriptions := map[agent.ThinkingLevel]string{"off": "No reasoning", "minimal": "Very brief reasoning (~1k tokens)", "low": "Light reasoning (~2k tokens)", "medium": "Moderate reasoning (~8k tokens)", "high": "Deep reasoning (~16k tokens)", "xhigh": "Extra-high reasoning (~32k tokens)", "max": "Maximum reasoning"}
	items := []tui.SelectItem{}
	for _, level := range levels {
		description := descriptions[level]
		items = append(items, tui.SelectItem{Value: string(level), Label: string(level), Description: &description})
	}
	list := tui.NewSelectList(items, max(1, len(items)), tui.SelectListTheme{})
	_ = list.SetSelectedIndex(slices.Index(levels, current))
	list.OnSelect = func(item tui.SelectItem) {
		if onSelect != nil {
			onSelect(agent.ThinkingLevel(item.Value))
		}
	}
	list.OnCancel = onCancel
	s := &ThinkingSelectorComponent{list: list}
	s.AddChild(list)
	return s
}
func (s *ThinkingSelectorComponent) GetSelectList() (*tui.SelectList, error) {
	if s.list == nil {
		return nil, notImplemented("ThinkingSelectorComponent.GetSelectList")
	}
	return s.list, nil
}
func (s *ThinkingSelectorComponent) HandleInput(data string) error {
	if s.list == nil {
		return notImplemented("ThinkingSelectorComponent.HandleInput")
	}
	return s.list.HandleInput(data)
}

func (s *ThinkingSelectorComponent) Render(width int) ([]string, error) {
	lines, err := s.Container.Render(width)
	if s.list != nil {
		lines = append([]string{"Thinking Level"}, lines...)
	}
	return lines, err
}

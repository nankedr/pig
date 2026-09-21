package codingagent

import (
	"fmt"
	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/tui"
	"slices"
	"strings"
)

type ModelsConfig struct {
	AllModels       []ai.Model
	EnabledModelIDs []string
}
type ModelsCallbacks struct {
	OnChange  func([]string) error
	OnPersist func([]string) error
	OnCancel  func()
}

// ScopedModelsSelectorComponent changes the current scope immediately; saving is explicit.
type ScopedModelsSelectorComponent struct {
	tui.Container
	Focused           bool
	models            []ai.Model
	enabled, filtered []string
	selected          int
	search            *tui.Input
	callbacks         ModelsCallbacks
	dirty, closed     bool
	status            string
	keybindings       *tui.KeybindingsManager
}

func NewScopedModelsSelectorComponent(config ModelsConfig, callbacks ModelsCallbacks) *ScopedModelsSelectorComponent {
	s := &ScopedModelsSelectorComponent{Focused: true, models: cloneRuntimeModels(config.AllModels), enabled: slices.Clone(config.EnabledModelIDs), search: tui.NewInput(), callbacks: callbacks}
	s.refresh()
	return s
}
func (s *ScopedModelsSelectorComponent) GetSearchInput() (*tui.Input, error) { return s.search, nil }
func (s *ScopedModelsSelectorComponent) model(id string) *ai.Model {
	for i := range s.models {
		if modelID(s.models[i]) == id {
			return &s.models[i]
		}
	}
	return nil
}
func (s *ScopedModelsSelectorComponent) allIDs() []string {
	ids := []string{}
	for _, m := range s.models {
		ids = append(ids, modelID(m))
	}
	return ids
}
func (s *ScopedModelsSelectorComponent) refresh() {
	ids := slices.Clone(s.enabled)
	for _, id := range s.allIDs() {
		if !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	s.filtered, _ = tui.FuzzyFilter(ids, s.search.GetValue(), func(id string) string {
		if m := s.model(id); m != nil {
			return fmt.Sprintf("%s %s %s %s %s %s", m.ID, m.Provider, id, m.Provider, m.ID, m.Name)
		}
		return id
	})
	s.selected = max(0, min(s.selected, len(s.filtered)-1))
}
func (s *ScopedModelsSelectorComponent) change(next []string) error {
	if s.callbacks.OnChange != nil {
		if err := s.callbacks.OnChange(slices.Clone(next)); err != nil {
			return err
		}
	}
	s.enabled = next
	s.dirty = true
	s.status = ""
	s.refresh()
	return nil
}
func (s *ScopedModelsSelectorComponent) HandleInput(data string) error {
	if s.closed {
		return nil
	}
	kb := s.keybindings
	if kb == nil {
		kb = tui.NewKeybindingsManager(appKeybindings())
	}
	match := func(action tui.Keybinding) bool { v, _ := kb.Matches(data, action); return v }
	id := ""
	if len(s.filtered) > 0 {
		id = s.filtered[s.selected]
	}
	switch {
	case match("tui.select.up"), match("tui.select.down"):
		if len(s.filtered) > 0 {
			delta := 1
			if match("tui.select.up") {
				delta = -1
			}
			s.selected = (s.selected + delta + len(s.filtered)) % len(s.filtered)
		}
	case match("tui.select.confirm"):
		if id != "" {
			next := slices.Clone(s.enabled)
			if next == nil {
				next = []string{id}
			} else if i := slices.Index(next, id); i >= 0 {
				next = slices.Delete(next, i, i+1)
			} else {
				next = append(next, id)
			}
			return s.change(next)
		}
	case match("app.models.enableAll"), match("app.models.clearAll"), match("app.models.toggleProvider"):
		targets := s.filtered
		all := s.allIDs()
		enable := match("app.models.enableAll")
		if match("app.models.toggleProvider") {
			model := s.model(id)
			if model == nil {
				return nil
			}
			targets = nil
			for _, m := range s.models {
				if m.Provider == model.Provider {
					targets = append(targets, modelID(m))
				}
			}
			enable = false
			for _, v := range targets {
				if s.enabled != nil && !slices.Contains(s.enabled, v) {
					enable = true
				}
			}
		}
		next := slices.Clone(s.enabled)
		if enable && next == nil {
			return s.change(nil)
		}
		if !enable && next == nil {
			next = all
		}
		if next == nil {
			next = []string{}
		}
		for _, v := range targets {
			i := slices.Index(next, v)
			if enable && i < 0 {
				next = append(next, v)
			} else if !enable && i >= 0 {
				next = slices.Delete(next, i, i+1)
			}
		}
		if enable && len(next) == len(all) {
			onlyAvailable := true
			for _, v := range next {
				if !slices.Contains(all, v) {
					onlyAvailable = false
				}
			}
			if onlyAvailable {
				next = nil
			}
		}
		return s.change(next)
	case match("app.models.reorderUp"), match("app.models.reorderDown"):
		i := slices.Index(s.enabled, id)
		delta := 1
		if match("app.models.reorderUp") {
			delta = -1
		}
		j := i + delta
		if i >= 0 && j >= 0 && j < len(s.enabled) {
			next := slices.Clone(s.enabled)
			next[i], next[j] = next[j], next[i]
			if err := s.change(next); err != nil {
				return err
			}
			s.selected = max(0, min(s.selected+delta, len(s.filtered)-1))
		}
	case match("app.models.save"):
		if s.callbacks.OnPersist != nil {
			if err := s.callbacks.OnPersist(slices.Clone(s.enabled)); err != nil {
				return err
			}
		}
		s.dirty = false
		s.status = "Model selection saved to settings"
	case data == "\x03" && s.search.GetValue() != "":
		s.search.SetValue("")
		s.refresh()
	case match("tui.select.cancel"):
		s.closed = true
		if s.callbacks.OnCancel != nil {
			s.callbacks.OnCancel()
		}
	default:
		if err := s.search.HandleInput(data); err != nil {
			return err
		}
		s.refresh()
	}
	return nil
}
func (s *ScopedModelsSelectorComponent) Render(width int) ([]string, error) {
	s.search.Focused = s.Focused
	input, err := s.search.Render(width)
	if err != nil {
		return nil, err
	}
	lines := append([]string{"Model Configuration", "Session-only. Ctrl+S to save to settings."}, input...)
	start := max(0, min(s.selected-4, len(s.filtered)-8))
	for i := start; i < min(start+8, len(s.filtered)); i++ {
		id := s.filtered[i]
		prefix := "  "
		if i == s.selected {
			prefix = "→ "
		}
		label := id + " [unavailable]"
		if m := s.model(id); m != nil {
			label = m.ID + " [" + string(m.Provider) + "]"
		}
		if s.enabled != nil {
			if slices.Contains(s.enabled, id) {
				label += " ✓"
			} else {
				label += " ✗"
			}
		}
		lines = append(lines, prefix+tui.SafeTerminalText(label))
	}
	if len(s.filtered) == 0 {
		lines = append(lines, "No matching models")
	}
	status := s.status
	if s.dirty {
		status += " (unsaved)"
	}
	if s.enabled == nil {
		status += " all enabled"
	} else {
		status += fmt.Sprintf(" %d enabled", len(s.enabled))
	}
	lines = append(lines, strings.TrimSpace(status), "Enter toggle · Ctrl+A all · Ctrl+X clear · Ctrl+P provider", "Alt+↑/↓ reorder · Ctrl+S save · Esc close")
	return selectorLines(lines, width), nil
}

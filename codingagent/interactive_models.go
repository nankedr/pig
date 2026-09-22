package codingagent

import (
	"context"
	"fmt"
	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/tui"
	"io"
	"strings"
	"sync"
)

func (m *InteractiveMode) configurationReady() error {
	s := m.runtime.Session()
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.configurationReady()
}
func (m *InteractiveMode) modelStatus() error {
	s := m.runtime.Session()
	return m.ui.Append(fmt.Sprintf("\nModel: %s [%s] · Thinking: %s\n", s.Model().ID, s.Model().Provider, s.ThinkingLevel()))
}
func (m *InteractiveMode) modelAction(ctx context.Context, action tui.Keybinding) (bool, error) {
	s := m.runtime.Session()
	switch action {
	case "app.model.select":
		return true, m.selectModel(ctx, "")
	case "app.model.cycleForward", "app.model.cycleBackward":
		direction := ModelCycleForward
		if action == "app.model.cycleBackward" {
			direction = ModelCycleBackward
		}
		result, err := s.CycleModel(ctx, direction)
		if err != nil {
			return true, err
		}
		if result == nil {
			return true, m.ShowWarning("No other available models")
		}
		return true, m.modelStatus()
	case "app.thinking.cycle":
		_, err := s.CycleThinkingLevel()
		if err != nil {
			return true, err
		}
		return true, m.modelStatus()
	}
	return false, nil
}
func selectorSignal() (<-chan struct{}, func()) {
	done := make(chan struct{})
	var once sync.Once
	return done, func() { once.Do(func() { close(done) }) }
}
func (m *InteractiveMode) waitSelector(ctx context.Context, component tui.Component, done <-chan struct{}) error {
	if err := m.ui.SetDialog(component); err != nil {
		return err
	}
	defer m.ui.SetDialog(nil)
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-m.ui.Done():
		if err := m.ui.Err(); err != nil {
			return err
		}
		return io.EOF
	}
}
func (m *InteractiveMode) selectModel(ctx context.Context, query string) error {
	if err := m.configurationReady(); err != nil {
		return err
	}
	s := m.runtime.Session()
	models, err := s.ModelRuntime().GetAvailableSnapshot()
	if err != nil {
		return err
	}
	scope := s.ScopedModels()
	candidates := models
	if len(scope) > 0 {
		candidates = nil
		for _, entry := range scope {
			candidates = append(candidates, entry.Model)
		}
	}
	if query != "" {
		var matches []ai.Model
		for _, model := range candidates {
			if strings.EqualFold(query, string(model.Provider)+"/"+model.ID) || strings.EqualFold(query, model.ID) {
				matches = append(matches, model)
			}
		}
		if len(matches) == 1 {
			if err = s.SetModel(matches[0]); err != nil {
				return err
			}
			return m.modelStatus()
		}
	}
	done, finish := selectorSignal()
	var selected *ai.Model
	current := s.Model()
	selector := NewModelSelectorComponent(models, &current, scope, func(model ai.Model) { selected = &model; finish() }, finish, query)
	selector.keybindings = &m.options.Keybindings.KeybindingsManager
	selector.search.SetKeybindings(selector.keybindings)
	defer selector.Dispose()
	if err = m.waitSelector(ctx, selector, done); err != nil {
		return err
	}
	if selected == nil {
		return nil
	}
	if err = s.SetModel(*selected); err != nil {
		return err
	}
	return m.modelStatus()
}
func (m *InteractiveMode) selectThinking(ctx context.Context) error {
	if err := m.configurationReady(); err != nil {
		return err
	}
	done, finish := selectorSignal()
	choice := ""
	menu := tui.NewSelectDialog("Settings", []tui.SelectItem{{Value: "thinking", Label: "Thinking level"}, {Value: "theme", Label: "Theme"}}, func(item tui.SelectItem) { choice = item.Value; finish() }, finish)
	menu.List.SetKeybindings(&m.options.Keybindings.KeybindingsManager)
	if err := m.waitSelector(ctx, menu, done); err != nil {
		return err
	}
	if choice == "theme" {
		return m.selectTheme(ctx)
	}
	if choice == "" {
		return nil
	}
	s := m.runtime.Session()
	levels, err := s.GetAvailableThinkingLevels()
	if err != nil {
		return err
	}
	done, finish = selectorSignal()
	var level agent.ThinkingLevel
	selector := NewThinkingSelectorComponent(s.ThinkingLevel(), levels, func(value agent.ThinkingLevel) { level = value; finish() }, finish)
	selector.list.SetKeybindings(&m.options.Keybindings.KeybindingsManager)
	if err = m.waitSelector(ctx, selector, done); err != nil {
		return err
	}
	if level == "" {
		return nil
	}
	if err = s.SetThinkingLevel(level); err != nil {
		return err
	}
	return m.modelStatus()
}

func (m *InteractiveMode) selectModelScope(ctx context.Context) error {
	if err := m.configurationReady(); err != nil {
		return err
	}
	s := m.runtime.Session()
	models, err := s.ModelRuntime().GetAvailableSnapshot()
	if err != nil {
		return err
	}
	var ids []string
	if scope := s.ScopedModels(); len(scope) > 0 {
		for _, entry := range scope {
			ids = append(ids, modelID(entry.Model))
		}
	} else {
		patterns, e := s.SettingsManager().GetEnabledModels()
		if e != nil {
			return e
		}
		if len(patterns) > 0 {
			resolved, e := ResolveModelScopeWithDiagnostics(ctx, patterns, s.ModelRuntime())
			if e != nil {
				return e
			}
			ids = []string{}
			for _, entry := range resolved.ScopedModels {
				ids = append(ids, modelID(entry.Model))
			}
			for _, d := range resolved.Diagnostics {
				if d.Code == "no-match" {
					ids = append(ids, d.Pattern)
				}
			}
		}
	}
	done, finish := selectorSignal()
	selector := NewScopedModelsSelectorComponent(ModelsConfig{AllModels: models, EnabledModelIDs: ids}, ModelsCallbacks{OnChange: func(ids []string) error { return s.SetEnabledModels(ids, false) }, OnPersist: func(ids []string) error { return s.SetEnabledModels(ids, true) }, OnCancel: finish})
	selector.keybindings = &m.options.Keybindings.KeybindingsManager
	selector.search.SetKeybindings(selector.keybindings)
	return m.waitSelector(ctx, selector, done)
}

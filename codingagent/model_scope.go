package codingagent

import (
	"encoding/json"
	"fmt"
	"github.com/nankedr/pig/ai"
	"slices"
)

// SetEnabledModels applies ordered provider/model IDs. nil, all, or no available IDs restore unscoped cycling.
// persist saves the preference only after the write succeeds; unavailable IDs are retained in settings.
func (s *AgentSession) SetEnabledModels(ids []string, persist bool) error {
	if s == nil || s.agent == nil {
		return notImplemented("AgentSession.SetEnabledModels")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.configurationReady(); err != nil {
		return err
	}
	available, err := s.modelRuntime.GetAvailableSnapshot()
	if err != nil {
		return err
	}
	scope := []ScopedModel{}
	seen := map[string]bool{}
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		for _, model := range available {
			if id == string(model.Provider)+"/"+model.ID {
				scope = append(scope, ScopedModel{Model: model})
				break
			}
		}
	}
	allAvailable := len(scope) == len(available)
	if allAvailable {
		scope = nil
	}
	if persist {
		settings := s.settingsManager
		if settings == nil {
			return fmt.Errorf("AgentSession has no SettingsManager")
		}
		if err = settings.ready(); err != nil {
			return err
		}
		settings.mu.Lock()
		defer settings.mu.Unlock()
		if settings.loadError != nil {
			return settings.loadError
		}
		saved := slices.Clone(ids)
		if len(ids) == len(available) && allAvailable {
			saved = nil
		}
		value, err := json.Marshal(saved)
		if err != nil {
			return err
		}
		if err = settingsWithLock(settings.storage, func(current *string) *string {
			merged := parseSettings(current)
			if saved == nil {
				delete(merged, "enabledModels")
			} else {
				merged["enabledModels"] = value
			}
			data, e := json.MarshalIndent(merged, "", "  ")
			if e != nil {
				panic(e)
			}
			text := string(data)
			return &text
		}); err != nil {
			return err
		}
		if saved == nil {
			delete(settings.global, "enabledModels")
		} else {
			settings.global["enabledModels"] = value
		}
		delete(settings.dirty, "enabledModels")
		settings.settings = mergeSettings(settings.global, settings.project)
	}
	s.scopedModels = cloneScopedModels(scope)
	return nil
}
func modelID(model ai.Model) string { return string(model.Provider) + "/" + model.ID }

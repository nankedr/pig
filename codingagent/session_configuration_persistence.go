package codingagent

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
)

// Stage the v3 entries before writing settings; publish in-memory state only
// after both writes succeed. A failed rename restores the touched settings.
func (s *AgentSession) persistConfiguration(model *ai.Model, level *agent.ThinkingLevel) error {
	if s.sessionManager == nil {
		return fmt.Errorf("AgentSession has no SessionManager")
	}
	settings := s.settingsManager
	if settings == nil {
		return fmt.Errorf("AgentSession has no SettingsManager")
	}
	if err := settings.ready(); err != nil {
		return err
	}
	manager := s.sessionManager
	manager.mu.Lock()
	defer manager.mu.Unlock()
	settings.mu.Lock()
	defer settings.mu.Unlock()
	if settings.loadError != nil {
		return settings.loadError
	}
	fields := map[string]json.RawMessage{}
	entries := append([]SessionEntry{}, manager.entries...)
	leaf := cloneStringPointer(manager.leafID)
	appendEntry := func(entry SessionEntry) {
		entry.ParentID = leaf
		entries = append(entries, entry)
		id := entry.ID
		leaf = &id
	}
	if model != nil {
		entry := manager.newEntryLocked("model_change")
		entry.Provider = string(model.Provider)
		entry.ModelID = model.ID
		appendEntry(entry)
		fields["defaultProvider"], _ = json.Marshal(model.Provider)
		fields["defaultModel"], _ = json.Marshal(model.ID)
	}
	if level != nil {
		entry := manager.newEntryLocked("thinking_level_change")
		entry.ThinkingLevel = string(*level)
		appendEntry(entry)
		reasoning := s.agent.State().Model.Reasoning
		if model != nil {
			reasoning = model.Reasoning
		}
		if reasoning || *level != "off" {
			fields["defaultThinkingLevel"], _ = json.Marshal(*level)
		}
	}
	persist := manager.flushed
	for _, entry := range entries {
		if entry.Message != nil {
			if _, ok := sessionAssistantMessage(entry.Message); ok {
				persist = true
				break
			}
		}
	}
	staged := ""
	if persist && manager.sessionFile != "" {
		data, err := encodeSessionFile(manager.header, entries)
		if err != nil {
			return err
		}
		file, err := os.CreateTemp(filepath.Dir(manager.sessionFile), ".session-config-*")
		if err != nil {
			return err
		}
		staged = file.Name()
		defer os.Remove(staged)
		_, writeErr := file.Write(data)
		closeErr := file.Close()
		if err = errors.Join(writeErr, closeErr); err != nil {
			return err
		}
	}
	previous := map[string]json.RawMessage{}
	if len(fields) > 0 {
		err := settingsWithLock(settings.storage, func(current *string) *string {
			merged := parseSettings(current)
			for key, value := range fields {
				previous[key] = append(json.RawMessage(nil), merged[key]...)
				merged[key] = value
			}
			data, err := json.MarshalIndent(merged, "", "  ")
			if err != nil {
				panic(err)
			}
			text := string(data)
			return &text
		})
		if err != nil {
			return err
		}
	}
	if staged != "" {
		if err := os.Rename(staged, manager.sessionFile); err != nil {
			rollback := settingsWithLock(settings.storage, func(current *string) *string {
				merged := parseSettings(current)
				for key, value := range previous {
					if value == nil {
						delete(merged, key)
					} else {
						merged[key] = value
					}
				}
				data, e := json.MarshalIndent(merged, "", "  ")
				if e != nil {
					panic(e)
				}
				text := string(data)
				return &text
			})
			return errors.Join(err, rollback)
		}
		manager.flushed = true
	}
	manager.entries, manager.leafID = entries, leaf
	for key, value := range fields {
		settings.global[key] = value
		delete(settings.dirty, key)
	}
	settings.settings = mergeSettings(settings.global, settings.project)
	return nil
}

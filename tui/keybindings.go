package tui

import (
	"slices"
	"sort"
	"sync"
)

type KeybindingsManager struct {
	definitions  KeybindingDefinitions
	userBindings KeybindingsConfig
}

var keybindingsMu sync.RWMutex
var globalKeybindings = NewKeybindingsManager(NewTUIKeybindings())

func NewKeybindingsManager(definitions KeybindingDefinitions, userBindings ...KeybindingsConfig) *KeybindingsManager {
	m := &KeybindingsManager{definitions: cloneKeybindingDefinitions(definitions), userBindings: KeybindingsConfig{}}
	if len(userBindings) > 0 {
		m.userBindings = cloneKeybindingsConfig(userBindings[0])
	}
	return m
}
func (m *KeybindingsManager) Matches(data string, action Keybinding) (bool, error) {
	return m.matches(data, action, kittyActive.Load()), nil
}
func (m *KeybindingsManager) matches(data string, action Keybinding, active bool) bool {
	keys, _ := m.GetKeys(action)
	for _, key := range keys {
		if matchesKey(data, key, active) {
			return true
		}
	}
	return false
}
func (m *KeybindingsManager) keys(action Keybinding) []KeyID {
	d, ok := m.definitions[action]
	if !ok {
		return []KeyID{}
	}
	keys, ok := m.userBindings[action]
	if !ok {
		keys = d.DefaultKeys
	}
	out := []KeyID{}
	for _, key := range keys {
		if !slices.Contains(out, key) {
			out = append(out, key)
		}
	}
	return out
}
func (m *KeybindingsManager) GetKeys(action Keybinding) ([]KeyID, error) {
	keybindingsMu.RLock()
	defer keybindingsMu.RUnlock()
	return m.keys(action), nil
}
func (m *KeybindingsManager) GetDefinition(action Keybinding) (KeybindingDefinition, bool, error) {
	keybindingsMu.RLock()
	defer keybindingsMu.RUnlock()
	d, ok := m.definitions[action]
	d.DefaultKeys = slices.Clone(d.DefaultKeys)
	d.Description = inputCloneStringPointer(d.Description)
	return d, ok, nil
}
func (m *KeybindingsManager) GetConflicts() ([]KeybindingConflict, error) {
	keybindingsMu.RLock()
	defer keybindingsMu.RUnlock()
	claims := map[KeyID][]string{}
	for action, keys := range m.userBindings {
		if _, ok := m.definitions[action]; !ok {
			continue
		}
		for _, key := range keys {
			if !slices.Contains(claims[key], string(action)) {
				claims[key] = append(claims[key], string(action))
			}
		}
	}
	out := []KeybindingConflict{}
	for key, actions := range claims {
		if len(actions) > 1 {
			sort.Strings(actions)
			out = append(out, KeybindingConflict{key, actions})
		}
	}
	slices.SortFunc(out, func(a, b KeybindingConflict) int {
		if a.Key < b.Key {
			return -1
		}
		if a.Key > b.Key {
			return 1
		}
		return 0
	})
	return out, nil
}
func (m *KeybindingsManager) SetUserBindings(config KeybindingsConfig) error {
	keybindingsMu.Lock()
	defer keybindingsMu.Unlock()
	m.userBindings = cloneKeybindingsConfig(config)
	return nil
}
func (m *KeybindingsManager) GetUserBindings() (KeybindingsConfig, error) {
	keybindingsMu.RLock()
	defer keybindingsMu.RUnlock()
	return cloneKeybindingsConfig(m.userBindings), nil
}
func (m *KeybindingsManager) GetResolvedBindings() (KeybindingsConfig, error) {
	keybindingsMu.RLock()
	defer keybindingsMu.RUnlock()
	out := KeybindingsConfig{}
	for action := range m.definitions {
		out[action] = m.keys(action)
	}
	return out, nil
}
func SetKeybindings(m *KeybindingsManager) error {
	keybindingsMu.Lock()
	defer keybindingsMu.Unlock()
	if m == nil {
		m = NewKeybindingsManager(NewTUIKeybindings())
	}
	globalKeybindings = m
	return nil
}
func GetKeybindings() (*KeybindingsManager, error) {
	keybindingsMu.RLock()
	defer keybindingsMu.RUnlock()
	return globalKeybindings, nil
}

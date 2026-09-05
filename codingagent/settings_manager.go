package codingagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type SettingsManager struct{ *settingsManagerState }

type settingsManagerState struct {
	mu                      sync.Mutex
	storage                 SettingsStorage
	global, settings, dirty map[string]json.RawMessage
	loadError               error
	errors                  []error
}

type fileSettingsStorage struct{ path string }

func (s fileSettingsStorage) WithLock(_ SettingsScope, fn func(*string) *string) {
	// Read-only construction must not create the state directory.
	if _, err := os.Stat(s.path); errors.Is(err, os.ErrNotExist) {
		if fn(nil) == nil {
			return
		}
		if err = os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
			panic(err)
		}
	}
	lock := s.path + ".lock"
	var err error
	for attempt := 0; attempt < 10; attempt++ {
		err = os.Mkdir(lock, 0o700)
		if err == nil {
			break
		}
		if !errors.Is(err, os.ErrExist) {
			panic(err)
		}
		if info, statErr := os.Stat(lock); statErr == nil && time.Since(info.ModTime()) > 10*time.Second {
			if removeErr := os.Remove(lock); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
				panic(removeErr)
			}
		}
		if attempt < 9 {
			time.Sleep(20 * time.Millisecond)
		}
	}
	if err != nil {
		panic(fmt.Errorf("settings lock: %w", err))
	}
	defer os.Remove(lock)
	var current *string
	data, err := os.ReadFile(s.path)
	if err == nil {
		text := string(data)
		current = &text
	} else if !errors.Is(err, os.ErrNotExist) {
		panic(err)
	}
	if next := fn(current); next != nil {
		if err = os.WriteFile(s.path, []byte(*next), 0o600); err != nil {
			panic(err)
		}
	}
}

type memorySettingsStorage struct{ value *string }

func (s *memorySettingsStorage) WithLock(_ SettingsScope, fn func(*string) *string) {
	if next := fn(s.value); next != nil {
		s.value = next
	}
}

func NewSettingsManager(_ string, agentDir *string, options ...SettingsManagerCreateOptions) (*SettingsManager, error) {
	var dir string
	var err error
	if agentDir == nil {
		dir, err = GetAgentDir()
	} else {
		dir, err = resolveSessionPath(*agentDir)
	}
	if err != nil {
		return nil, err
	}
	return NewSettingsManagerFromStorage(fileSettingsStorage{filepath.Join(dir, "settings.json")}, options...)
}
func NewSettingsManagerFromStorage(storage SettingsStorage, options ...SettingsManagerCreateOptions) (*SettingsManager, error) {
	if len(options) > 1 {
		return nil, fmt.Errorf("expected at most one SettingsManagerCreateOptions")
	}
	if len(options) == 1 && options[0].ProjectTrusted != nil && *options[0].ProjectTrusted {
		return nil, notImplemented("SettingsManager.ProjectTrusted")
	}
	if storage == nil {
		return nil, fmt.Errorf("settings storage must not be nil")
	}
	m := &SettingsManager{&settingsManagerState{storage: storage, global: map[string]json.RawMessage{}, dirty: map[string]json.RawMessage{}}}
	m.reload()
	return m, nil
}
func NewInMemorySettingsManager(settings Settings, options ...SettingsManagerCreateOptions) (*SettingsManager, error) {
	data, err := json.Marshal(settings)
	if err != nil {
		return nil, err
	}
	text := string(data)
	return NewSettingsManagerFromStorage(&memorySettingsStorage{value: &text}, options...)
}
func settingsWithLock(storage SettingsStorage, fn func(*string) *string) (err error) {
	defer func() {
		if failure := recover(); failure != nil {
			if e, ok := failure.(error); ok {
				err = e
			} else {
				err = fmt.Errorf("%v", failure)
			}
		}
	}()
	storage.WithLock(SettingsScopeGlobal, fn)
	return nil
}
func parseSettings(current *string) map[string]json.RawMessage {
	result := map[string]json.RawMessage{}
	if current != nil && *current != "" {
		if err := json.Unmarshal([]byte(*current), &result); err != nil {
			panic(err)
		}
		if result == nil {
			panic(fmt.Errorf("settings must be an object"))
		}
	}
	migrateSettings(result)
	return result
}
func migrateSettings(s map[string]json.RawMessage) {
	if value, ok := s["queueMode"]; ok && s["steeringMode"] == nil {
		s["steeringMode"] = value
		delete(s, "queueMode")
	}
	var enabled bool
	if s["transport"] == nil && json.Unmarshal(s["websockets"], &enabled) == nil && string(s["websockets"]) != "null" {
		value := "sse"
		if enabled {
			value = "websocket"
		}
		s["transport"], _ = json.Marshal(value)
		delete(s, "websockets")
	}
	var skills map[string]json.RawMessage
	if json.Unmarshal(s["skills"], &skills) == nil && skills != nil {
		if s["enableSkillCommands"] == nil && skills["enableSkillCommands"] != nil {
			s["enableSkillCommands"] = skills["enableSkillCommands"]
		}
		var dirs []any
		if json.Unmarshal(skills["customDirectories"], &dirs) == nil && len(dirs) > 0 {
			s["skills"] = skills["customDirectories"]
		} else {
			delete(s, "skills")
		}
	}
	var retry map[string]json.RawMessage
	if json.Unmarshal(s["retry"], &retry) == nil && retry != nil {
		provider := map[string]json.RawMessage{}
		_ = json.Unmarshal(retry["provider"], &provider)
		if provider == nil {
			provider = map[string]json.RawMessage{}
		}
		var delay float64
		if json.Unmarshal(retry["maxDelayMs"], &delay) == nil && string(retry["maxDelayMs"]) != "null" && (provider["maxRetryDelayMs"] == nil || string(provider["maxRetryDelayMs"]) == "null") {
			provider["maxRetryDelayMs"] = retry["maxDelayMs"]
			retry["provider"], _ = json.Marshal(provider)
		}
		delete(retry, "maxDelayMs")
		s["retry"], _ = json.Marshal(retry)
	}
}
func mergeSettings(base, overrides map[string]json.RawMessage) map[string]json.RawMessage {
	result := map[string]json.RawMessage{}
	for key, value := range base {
		result[key] = append(json.RawMessage(nil), value...)
	}
	for key, value := range overrides {
		var left, right map[string]json.RawMessage
		if json.Unmarshal(base[key], &left) == nil && left != nil && json.Unmarshal(value, &right) == nil && right != nil {
			result[key], _ = json.Marshal(mergeSettings(left, right))
		} else {
			result[key] = append(json.RawMessage(nil), value...)
		}
	}
	return result
}
func (m SettingsManager) ready() error {
	if m.settingsManagerState == nil || m.storage == nil {
		return fmt.Errorf("SettingsManager is not initialized")
	}
	return nil
}
func (m SettingsManager) record(err error) {
	m.errors = append(m.errors, fmt.Errorf("global settings: %w", err))
}
func (m SettingsManager) reload() {
	var loaded map[string]json.RawMessage
	m.loadError = settingsWithLock(m.storage, func(current *string) *string { loaded = parseSettings(current); return nil })
	if m.loadError == nil {
		m.global = loaded
	} else {
		m.record(m.loadError)
	}
	m.dirty = map[string]json.RawMessage{}
	m.settings = mergeSettings(m.global, nil)
}
func (m SettingsManager) Reload(ctx context.Context) error {
	if err := m.ready(); err != nil {
		return err
	}
	if ctx == nil {
		return fmt.Errorf("settings context must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reload()
	return nil
}

// Setters complete their locked write synchronously; Flush is the Go completion boundary.
func (m SettingsManager) Flush(ctx context.Context) error {
	if err := m.ready(); err != nil {
		return err
	}
	if ctx == nil {
		return fmt.Errorf("settings context must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return nil
}
func (m SettingsManager) DrainErrors() ([]error, error) {
	if err := m.ready(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	result := append([]error{}, m.errors...)
	m.errors = nil
	return result, nil
}
func (m SettingsManager) ApplyOverrides(overrides Settings) error {
	if err := m.ready(); err != nil {
		return err
	}
	data, err := json.Marshal(overrides)
	if err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err = json.Unmarshal(data, &fields); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.settings = mergeSettings(m.settings, fields)
	return nil
}
func (m SettingsManager) GetGlobalSettings() (Settings, error) {
	if err := m.ready(); err != nil {
		return Settings{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	data, err := json.Marshal(m.global)
	if err != nil {
		return Settings{}, err
	}
	var settings Settings
	err = json.Unmarshal(data, &settings)
	return settings, err
}
func (m SettingsManager) GetProjectSettings() (Settings, error) { return Settings{}, m.ready() }
func (m SettingsManager) IsProjectTrusted() (bool, error)       { return false, m.ready() }
func (m SettingsManager) SetProjectTrusted(trusted bool) error {
	if trusted {
		return notImplemented("SettingsManager.SetProjectTrusted")
	}
	return m.ready()
}
func (m SettingsManager) set(fields map[string]json.RawMessage, nested bool) error {
	if err := m.ready(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for key, value := range fields {
		if nested {
			m.global = mergeSettings(m.global, map[string]json.RawMessage{key: value})
			m.dirty = mergeSettings(m.dirty, map[string]json.RawMessage{key: value})
		} else {
			m.global[key] = value
			m.dirty[key] = value
		}
	}
	m.settings = mergeSettings(m.global, nil)
	if m.loadError != nil {
		return nil
	}
	err := settingsWithLock(m.storage, func(current *string) *string {
		merged := parseSettings(current)
		for key, value := range m.dirty {
			// Nested setters merge just their touched child keys; arrays and whole objects replace.
			if nestedFields[key] {
				merged = mergeSettings(merged, map[string]json.RawMessage{key: value})
			} else {
				merged[key] = value
			}
		}
		data, err := json.MarshalIndent(merged, "", "  ")
		if err != nil {
			panic(err)
		}
		text := string(data)
		return &text
	})
	if err != nil {
		m.record(err)
	} else {
		m.dirty = map[string]json.RawMessage{}
	}
	return nil
}

var nestedFields = map[string]bool{"compaction": true, "retry": true, "terminal": true, "images": true, "markdown": true}

func (m SettingsManager) setValue(key string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return m.set(map[string]json.RawMessage{key: data}, false)
}
func (m SettingsManager) setNested(key, child string, value any) error {
	data, err := json.Marshal(map[string]any{child: value})
	if err != nil {
		return err
	}
	return m.set(map[string]json.RawMessage{key: data}, true)
}
func settingsValue[T any](m SettingsManager, key, child string, fallback T) (T, error) {
	if err := m.ready(); err != nil {
		return fallback, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	raw := m.settings[key]
	if child != "" {
		var object map[string]json.RawMessage
		if err := json.Unmarshal(raw, &object); err != nil && len(raw) != 0 && string(raw) != "null" {
			return fallback, err
		}
		raw = object[child]
	}
	if len(raw) == 0 || string(raw) == "null" {
		return fallback, nil
	}
	var value T
	if err := json.Unmarshal(raw, &value); err != nil {
		return fallback, fmt.Errorf("invalid %s setting: %w", key, err)
	}
	return value, nil
}

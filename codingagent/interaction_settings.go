package codingagent

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/nankedr/pig/ai"
	"slices"
	"strconv"
)

// GetInteractionSettings returns effective settings and the live Session queue/thinking configuration.
func (s *AgentSession) GetInteractionSettings() (SettingsConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.interactionSettings()
}
func (s *AgentSession) interactionSettings() (SettingsConfig, error) {
	c := SettingsConfig{}
	m := s.settingsManager
	if m == nil {
		return c, fmt.Errorf("AgentSession has no SettingsManager")
	}
	var failures []error
	var err error
	c.AutoCompact, err = m.GetCompactionEnabled()
	failures = append(failures, err)
	c.EnableSkillCommands, err = m.GetEnableSkillCommands()
	failures = append(failures, err)
	c.ShowHardwareCursor, err = m.GetShowHardwareCursor()
	failures = append(failures, err)
	c.EditorPaddingX, err = m.GetEditorPaddingX()
	failures = append(failures, err)
	c.OutputPad, err = m.GetOutputPad()
	failures = append(failures, err)
	c.AutocompleteMaxVisible, err = m.GetAutocompleteMaxVisible()
	failures = append(failures, err)
	c.ClearOnShrink, err = m.GetClearOnShrink()
	failures = append(failures, err)
	c.ShowTerminalProgress, err = m.GetShowTerminalProgress()
	failures = append(failures, err)
	c.SteeringMode, err = m.GetSteeringMode()
	failures = append(failures, err)
	c.FollowUpMode, err = m.GetFollowUpMode()
	failures = append(failures, err)
	c.HideThinkingBlock, err = m.GetHideThinkingBlock()
	failures = append(failures, err)
	c.QuietStartup, err = m.GetQuietStartup()
	failures = append(failures, err)
	c.DefaultProjectTrust, err = m.GetDefaultProjectTrust()
	failures = append(failures, err)
	vDoubleEscapeAction, err := m.GetDoubleEscapeAction()
	c.DoubleEscapeAction = DoubleEscapeAction(vDoubleEscapeAction)
	failures = append(failures, err)
	vTreeFilterMode, err := m.GetTreeFilterMode()
	c.TreeFilterMode = TreeFilterMode(vTreeFilterMode)
	failures = append(failures, err)
	c.TUIMode, err = m.GetTUIMode()
	failures = append(failures, err)
	c.FullscreenExitOutput, err = m.GetFullscreenExitOutput()
	failures = append(failures, err)
	c.FullscreenScrollbar, err = m.GetFullscreenScrollbar()
	failures = append(failures, err)
	c.CurrentTheme, err = m.GetThemeSetting()
	failures = append(failures, err)
	if c.CurrentTheme == "" {
		c.CurrentTheme = "dark"
	}
	if s.agent != nil {
		state := s.agent.State()
		c.SteeringMode = s.agent.SteeringMode()
		c.FollowUpMode = s.agent.FollowUpMode()
		c.ThinkingLevel = state.ThinkingLevel
		c.AvailableThinkingLevels = ai.GetSupportedThinkingLevels(state.Model)
	}
	return c, errors.Join(failures...)
}

// UpdateInteractionSetting validates a menu value, persists it globally, and applies trusted project precedence.
// Thinking and theme use their existing dedicated Session and ThemeController APIs.
func (s *AgentSession) UpdateInteractionSetting(id, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.configurationReady(); err != nil {
		return err
	}
	c, err := s.interactionSettings()
	if err != nil {
		return err
	}
	valid := false
	for _, item := range interactionSettingItems(c) {
		if item.ID == id {
			valid = slices.Contains(item.Values, value)
			break
		}
	}
	if !valid {
		return fmt.Errorf("invalid interaction setting %s=%q", id, value)
	}
	integer, _ := strconv.Atoi(value)
	var key, child string
	var setting any
	switch id {
	case "autocompact":
		key, child, setting = "compaction", "enabled", value == "true"
	case "skill-commands":
		key, child, setting = "enableSkillCommands", "", value == "true"
	case "show-hardware-cursor":
		key, child, setting = "showHardwareCursor", "", value == "true"
	case "editor-padding":
		key, child, setting = "editorPaddingX", "", integer
	case "output-padding":
		key, child, setting = "outputPad", "", integer
	case "autocomplete-max-visible":
		key, child, setting = "autocompleteMaxVisible", "", integer
	case "clear-on-shrink":
		key, child, setting = "terminal", "clearOnShrink", value == "true"
	case "terminal-progress":
		key, child, setting = "terminal", "showTerminalProgress", value == "true"
	case "steering-mode":
		key, child, setting = "steeringMode", "", value
	case "follow-up-mode":
		key, child, setting = "followUpMode", "", value
	case "hide-thinking":
		key, child, setting = "hideThinkingBlock", "", value == "true"
	case "quiet-startup":
		key, child, setting = "quietStartup", "", value == "true"
	case "default-project-trust":
		key, child, setting = "defaultProjectTrust", "", map[string]string{"Ask": "ask", "Always trust": "always", "Never trust": "never"}[value]
	case "double-escape-action":
		key, child, setting = "doubleEscapeAction", "", value
	case "tree-filter-mode":
		key, child, setting = "treeFilterMode", "", value
	case "tui-mode":
		key, child, setting = "tuiMode", "", value
	case "fullscreen-exit-output":
		key, child, setting = "fullscreenExitOutput", "", value
	case "fullscreen-scrollbar":
		key, child, setting = "fullscreenScrollbar", "", value
	default:
		return fmt.Errorf("setting %s requires its dedicated selector", id)
	}
	if err = s.settingsManager.saveInteractionSetting(key, child, setting); err != nil {
		return err
	}
	if id == "steering-mode" {
		v, e := s.settingsManager.GetSteeringMode()
		if e != nil {
			return e
		}
		return s.agent.SetSteeringMode(v)
	}
	if id == "follow-up-mode" {
		v, e := s.settingsManager.GetFollowUpMode()
		if e != nil {
			return e
		}
		return s.agent.SetFollowUpMode(v)
	}
	return nil
}
func (m *SettingsManager) saveInteractionSetting(key, child string, value any) error {
	if err := m.ready(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.loadError != nil {
		return m.loadError
	}
	if child != "" {
		value = map[string]any{child: value}
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	fields := map[string]json.RawMessage{key: raw}
	if err = settingsWithLock(m.storage, func(current *string) *string {
		merged := mergeSettings(parseSettings(current), fields)
		data, e := json.MarshalIndent(merged, "", "  ")
		if e != nil {
			panic(e)
		}
		text := string(data)
		return &text
	}); err != nil {
		return err
	}
	if child == "" {
		delete(m.dirty, key)
	} else if dirty := m.dirty[key]; dirty != nil {
		var pending map[string]json.RawMessage
		if json.Unmarshal(dirty, &pending) == nil {
			delete(pending, child)
			if len(pending) == 0 {
				delete(m.dirty, key)
			} else {
				m.dirty[key], _ = json.Marshal(pending)
			}
		}
	}
	m.global = mergeSettings(m.global, fields)
	m.settings = mergeSettings(m.global, m.project)
	return nil
}

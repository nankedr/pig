package codingagent

import (
	"fmt"
	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/tui"
	"strconv"
)

func interactionSettingItems(c SettingsConfig) []tui.SettingItem {
	item := func(id, label, description, value string, values ...string) tui.SettingItem {
		return tui.SettingItem{ID: id, Label: label, Description: &description, CurrentValue: value, Values: values}
	}
	trust := map[DefaultProjectTrust]string{"ask": "Ask", "always": "Always trust", "never": "Never trust"}
	return []tui.SettingItem{
		item("thinking", "Thinking level", "Reasoning depth for thinking-capable models", string(c.ThinkingLevel), "open"),
		item("theme", "Theme", "Color theme for the interface", c.CurrentTheme, "open"),
		item("autocompact", "Auto-compact", "Automatically compact context when it gets too large", fmt.Sprint(c.AutoCompact), "true", "false"),
		item("skill-commands", "Skill commands", "Register skills as /skill:name commands", fmt.Sprint(c.EnableSkillCommands), "true", "false"),
		item("show-hardware-cursor", "Show hardware cursor", "Show the terminal cursor while still positioning it for IME support", fmt.Sprint(c.ShowHardwareCursor), "true", "false"),
		item("editor-padding", "Editor padding", "Horizontal padding for input editor (0-3)", fmt.Sprint(c.EditorPaddingX), "0", "1", "2", "3"),
		item("output-padding", "Output padding", "Horizontal padding for user messages, assistant messages, and thinking", fmt.Sprint(c.OutputPad), "0", "1"),
		item("autocomplete-max-visible", "Autocomplete max items", "Max visible items in autocomplete dropdown (3-20)", fmt.Sprint(c.AutocompleteMaxVisible), "3", "5", "7", "10", "15", "20"),
		item("clear-on-shrink", "Clear on shrink", "Clear empty rows when content shrinks (may cause flicker)", fmt.Sprint(c.ClearOnShrink), "true", "false"),
		item("terminal-progress", "Terminal progress", "Show OSC 9;4 progress indicators in the terminal tab bar", fmt.Sprint(c.ShowTerminalProgress), "true", "false"),
		item("steering-mode", "Steering mode", "Submit while streaming queues steering messages. one-at-a-time: deliver one, wait for response. all: deliver all at once.", string(c.SteeringMode), "one-at-a-time", "all"),
		item("follow-up-mode", "Follow-up mode", "Queue follow-up messages until agent stops. one-at-a-time: deliver one, wait for response. all: deliver all at once.", string(c.FollowUpMode), "one-at-a-time", "all"),
		item("hide-thinking", "Hide thinking", "Hide thinking blocks in assistant responses", fmt.Sprint(c.HideThinkingBlock), "true", "false"),
		item("quiet-startup", "Quiet startup", "Disable verbose printing at startup (next startup)", fmt.Sprint(c.QuietStartup), "true", "false"),
		item("default-project-trust", "Default project trust", "Fallback for projects without a saved trust decision (next project session)", trust[c.DefaultProjectTrust], "Ask", "Always trust", "Never trust"),
		item("double-escape-action", "Double-escape action", "Action when pressing the interrupt key twice with empty editor", string(c.DoubleEscapeAction), "tree", "fork", "none"),
		item("tree-filter-mode", "Tree filter mode", "Default filter when opening /tree", string(c.TreeFilterMode), "default", "no-tools", "user-only", "labeled-only", "all"),
		item("tui-mode", "TUI mode", "Interface layout; fullscreen mode is experimental", string(c.TUIMode), "regular", "fullscreen"),
		item("fullscreen-exit-output", "Fullscreen exit output", "Print the transcript or only a session resume hint when exiting fullscreen mode", string(c.FullscreenExitOutput), "transcript", "resume-hint"),
		item("fullscreen-scrollbar", "Fullscreen scrollbar", "Scrollbar behavior in fullscreen mode; has no effect in regular mode", string(c.FullscreenScrollbar), "auto", "always", "hidden"),
	}
}
func dispatchInteractionSetting(callbacks SettingsCallbacks, id, value string) {
	integer, _ := strconv.Atoi(value)
	switch id {
	case "autocompact":
		if callbacks.OnAutoCompactChange != nil {
			callbacks.OnAutoCompactChange(value == "true")
		}
	case "skill-commands":
		if callbacks.OnEnableSkillCommandsChange != nil {
			callbacks.OnEnableSkillCommandsChange(value == "true")
		}
	case "show-hardware-cursor":
		if callbacks.OnShowHardwareCursorChange != nil {
			callbacks.OnShowHardwareCursorChange(value == "true")
		}
	case "editor-padding":
		if callbacks.OnEditorPaddingXChange != nil {
			callbacks.OnEditorPaddingXChange(integer)
		}
	case "output-padding":
		if callbacks.OnOutputPadChange != nil {
			callbacks.OnOutputPadChange(integer)
		}
	case "autocomplete-max-visible":
		if callbacks.OnAutocompleteMaxVisibleChange != nil {
			callbacks.OnAutocompleteMaxVisibleChange(integer)
		}
	case "clear-on-shrink":
		if callbacks.OnClearOnShrinkChange != nil {
			callbacks.OnClearOnShrinkChange(value == "true")
		}
	case "terminal-progress":
		if callbacks.OnShowTerminalProgressChange != nil {
			callbacks.OnShowTerminalProgressChange(value == "true")
		}
	case "steering-mode":
		if callbacks.OnSteeringModeChange != nil {
			callbacks.OnSteeringModeChange(agent.QueueMode(value))
		}
	case "follow-up-mode":
		if callbacks.OnFollowUpModeChange != nil {
			callbacks.OnFollowUpModeChange(agent.QueueMode(value))
		}
	case "hide-thinking":
		if callbacks.OnHideThinkingBlockChange != nil {
			callbacks.OnHideThinkingBlockChange(value == "true")
		}
	case "quiet-startup":
		if callbacks.OnQuietStartupChange != nil {
			callbacks.OnQuietStartupChange(value == "true")
		}
	case "default-project-trust":
		if callbacks.OnDefaultProjectTrustChange != nil {
			callbacks.OnDefaultProjectTrustChange(map[string]DefaultProjectTrust{"Ask": "ask", "Always trust": "always", "Never trust": "never"}[value])
		}
	case "double-escape-action":
		if callbacks.OnDoubleEscapeActionChange != nil {
			callbacks.OnDoubleEscapeActionChange(DoubleEscapeAction(value))
		}
	case "tree-filter-mode":
		if callbacks.OnTreeFilterModeChange != nil {
			callbacks.OnTreeFilterModeChange(TreeFilterMode(value))
		}
	case "tui-mode":
		if callbacks.OnTUIModeChange != nil {
			callbacks.OnTUIModeChange(tui.TUIMode(value))
		}
	case "fullscreen-exit-output":
		if callbacks.OnFullscreenExitOutputChange != nil {
			callbacks.OnFullscreenExitOutputChange(FullscreenExitOutput(value))
		}
	case "fullscreen-scrollbar":
		if callbacks.OnFullscreenScrollbarChange != nil {
			callbacks.OnFullscreenScrollbarChange(tui.ScrollViewScrollbar(value))
		}
	case "thinking":
		if callbacks.OnThinkingLevelChange != nil {
			callbacks.OnThinkingLevelChange(agent.ThinkingLevel(value))
		}
	case "theme":
		if callbacks.OnThemeChange != nil {
			callbacks.OnThemeChange(value)
		}
	}
}
func NewSettingsSelectorComponent(config SettingsConfig, callbacks SettingsCallbacks) *SettingsSelectorComponent {
	s := &SettingsSelectorComponent{}
	theme, _ := LoadBuiltinTheme("dark")
	items := interactionSettingItems(config)
	items[0].Submenu = func(current string, done func(*string)) tui.Component {
		selector := NewThinkingSelectorComponent(agent.ThinkingLevel(current), config.AvailableThinkingLevels, func(level agent.ThinkingLevel) { v := string(level); done(&v) }, func() { done(nil) })
		selector.list.SetKeybindings(s.keybindings)
		return selector
	}
	items[1].Submenu = func(current string, done func(*string)) tui.Component {
		loaded := ThemeLoadResult{}
		for _, name := range config.AvailableThemes {
			loaded.Themes = append(loaded.Themes, &Theme{Name: name})
		}
		selector := NewThemeSettingsComponent(current, config.TerminalTheme, loaded, SettingsCallbacks{OnThemeChange: func(v string) { done(&v) }, OnThemePreview: callbacks.OnThemePreview, OnCancel: func() { done(nil) }}, func() *Theme { return theme })
		selector.SetKeybindings(s.keybindings)
		return selector
	}
	search := true
	s.list = tui.NewSettingsList(items, 10, settingsListTheme(func() *Theme { return theme }), func(id, value string) { dispatchInteractionSetting(callbacks, id, value) }, callbacks.OnCancel, tui.SettingsListOptions{EnableSearch: &search})
	s.AddChild(s.list)
	s.SetKeybindings(tui.NewKeybindingsManager(appKeybindings()))
	return s
}
func (s *SettingsSelectorComponent) SetKeybindings(kb *tui.KeybindingsManager) {
	s.keybindings = kb
	if s.list != nil {
		s.list.SetKeybindings(kb)
		setInteractionHints(s.list, kb)
	}
}
func (s *SettingsSelectorComponent) HandleInput(data string) error {
	if s.list == nil {
		return nil
	}
	return s.list.HandleInput(data)
}

func setInteractionHints(list *tui.SettingsList, kb *tui.KeybindingsManager) {
	if kb == nil {
		kb = tui.NewKeybindingsManager(appKeybindings())
	}
	for _, item := range []struct{ id, description string }{
		{"steering-mode", tui.SelectionKeyHint(kb, "tui.input.submit") + " while streaming queues steering messages. one-at-a-time: deliver one, wait for response. all: deliver all at once."},
		{"follow-up-mode", tui.SelectionKeyHint(kb, "app.message.followUp") + " queues follow-up messages until agent stops. one-at-a-time: deliver one, wait for response. all: deliver all at once."},
		{"double-escape-action", "Action when pressing " + tui.SelectionKeyHint(kb, "app.interrupt") + " twice with empty editor"},
	} {
		list.UpdateDescription(item.id, item.description)
	}
}

package codingagent

import (
	"context"
	"fmt"
	"github.com/nankedr/pig/tui"
	"sort"
	"strings"
)

func (m *InteractiveMode) selectSettings(ctx context.Context) error {
	if err := m.configurationReady(); err != nil {
		return err
	}
	c, err := m.runtime.Session().GetInteractionSettings()
	if err != nil {
		return err
	}
	c.HideThinkingBlock = m.ui.HideThinking()
	c.TUIMode = m.ui.Mode()
	var finish func()
	id, value := "", ""
	search := true
	items := interactionSettingItems(c)

	menu := tui.NewSettingsList(items, 10, settingsListTheme(m.themes.Current), func(i, v string) { id, value = i, v; finish() }, func() { id = ""; finish() }, tui.SettingsListOptions{EnableSearch: &search})
	menu.SetKeybindings(&m.options.Keybindings.KeybindingsManager)
	setInteractionHints(menu, &m.options.Keybindings.KeybindingsManager)
	var diagnostic error
	for {
		done, complete := selectorSignal()
		finish = complete
		if err = m.waitSelector(ctx, &themeSettingsFrame{body: menu, theme: m.themes.Current, diagnostic: func() error { return diagnostic }}, done); err != nil {
			return err
		}
		if id == "" {
			return nil
		}
		diagnostic = nil
		switch id {
		case "theme":
			diagnostic = m.selectTheme(ctx)
		case "thinking":
			diagnostic = m.selectThinkingLevel(ctx)
		default:
			diagnostic = m.runtime.Session().UpdateInteractionSetting(id, value)
			if diagnostic != nil {
				diagnostic = fmt.Errorf("Setting not saved: %w", diagnostic)
			} else {
				diagnostic = m.applyInteractionSettings(id)
			}
		}
		c, err = m.runtime.Session().GetInteractionSettings()
		if err != nil {
			return err
		}
		c.HideThinkingBlock = m.ui.HideThinking()
		c.TUIMode = m.ui.Mode()
		for _, item := range interactionSettingItems(c) {
			_ = menu.UpdateValue(item.ID, item.CurrentValue)
		}
	}
}
func (m *InteractiveMode) applyInteractionSettings(changed string) error {
	c, err := m.runtime.Session().GetInteractionSettings()
	if err != nil {
		return err
	}
	mode := m.ui.Mode()
	if changed == "tui-mode" {
		mode = c.TUIMode
	} else if changed == "init" && m.options.TUIMode != "" {
		mode = m.options.TUIMode
	} else if changed == "init" || changed == "session" {
		mode = c.TUIMode
	}
	if err = m.ui.SetMode(mode); err != nil {
		return err
	}
	hide := m.ui.HideThinking()
	if changed == "hide-thinking" || changed == "init" || changed == "session" {
		hide = c.HideThinkingBlock
	}
	m.transcript.SetOutputPad(c.OutputPad)
	if err = m.ui.ApplyInteractionOptions(tui.TextUIInteractionOptions{HideThinking: hide, ShowHardwareCursor: c.ShowHardwareCursor, ClearOnShrink: c.ClearOnShrink, EditorPaddingX: c.EditorPaddingX, AutocompleteMaxVisible: c.AutocompleteMaxVisible, DoubleEscapeAction: string(c.DoubleEscapeAction), ShowTerminalProgress: c.ShowTerminalProgress, Scrollbar: c.FullscreenScrollbar, ExitTranscript: c.FullscreenExitOutput == FullscreenExitOutputTranscript}); err != nil {
		return err
	}
	if changed == "skill-commands" {
		fd, _ := findBinary(context.Background())
		var path *string
		if fd != "" {
			path = &fd
		}
		provider, e := NewSessionAutocompleteProvider(m.runtime.Session(), path)
		if e != nil {
			return e
		}
		return m.ui.SetAutocompleteProvider(provider)
	}
	return nil
}
func (m *InteractiveMode) bindingHint(action tui.Keybinding) string {
	keys, _ := m.options.Keybindings.GetKeys(action)
	values := make([]string, len(keys))
	for i, key := range keys {
		values[i] = string(key)
	}
	if len(values) == 0 {
		return "unbound"
	}
	return strings.Join(values, " / ")
}
func (m *InteractiveMode) showHotkeys() error {
	config, err := m.options.Keybindings.GetEffectiveConfig()
	if err != nil {
		return err
	}
	actions := make([]string, 0, len(config))
	for action := range config {
		actions = append(actions, string(action))
	}
	sort.Strings(actions)
	var out strings.Builder
	out.WriteString("\nKeyboard shortcuts\n")
	for _, action := range actions {
		definition, _, _ := m.options.Keybindings.GetDefinition(tui.Keybinding(action))
		description := action
		if definition.Description != nil {
			description = *definition.Description
		}
		fmt.Fprintf(&out, "%s: %s\n", m.bindingHint(tui.Keybinding(action)), description)
	}
	c, err := m.runtime.Session().GetInteractionSettings()
	if err != nil {
		return err
	}
	fmt.Fprintf(&out, "Empty editor double %s: %s · TUI: %s · Hide thinking: %t\nSteering: %s · Follow-up: %s\n/settings: change interaction settings\n", m.bindingHint("app.interrupt"), c.DoubleEscapeAction, m.ui.Mode(), m.ui.HideThinking(), c.SteeringMode, c.FollowUpMode)
	return m.appendNotice(out.String())
}

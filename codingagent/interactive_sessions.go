package codingagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/nankedr/pig/tui"
	"io"
	"os"
)

func validateResumeSession(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("Cannot resume Session: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() == 0 {
		return fmt.Errorf("Cannot resume Session: not a nonempty regular file: %s", path)
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	for {
		var row json.RawMessage
		err = decoder.Decode(&row)
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("Corrupt Session %s: %w", path, err)
		}
	}
	return nil
}
func sessionLoaders(cwd string, dir *string) (SessionsLoader, SessionsLoader) {
	return func(ctx context.Context) ([]SessionInfo, error) {
		return ListSessions(ctx, cwd, SessionListOptions{SessionDir: dir})
	}, func(ctx context.Context) ([]SessionInfo, error) {
		return ListAllSessions(ctx, SessionListOptions{SessionDir: dir})
	}
}
func selectStartupSession(ctx context.Context, cwd string, dir *string) (manager *SessionManager, err error) {
	bindings, err := NewKeybindingsManager()
	if err != nil {
		return nil, err
	}
	terminal := tui.NewProcessTerminal(os.Stdin, os.Stdout)
	ui := tui.NewTextUI(terminal, tui.TextUIOptions{Keybindings: &bindings.KeybindingsManager})
	if err = ui.Start(); err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, ui.Stop()); <-terminal.Done() }()
	current, all := sessionLoaders(cwd, dir)
	status := ""
	for {
		done, finish := selectorSignal()
		selected := ""
		selector, e := NewSessionSelectorComponent(ctx, current, all, func(path string) { selected = path; finish() }, finish, SessionSelectorOptions{Keybindings: bindings})
		if e != nil {
			return nil, e
		}
		selector.status = status
		if e = ui.SetDialog(selector); e != nil {
			return nil, e
		}
		select {
		case <-done:
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ui.Done():
			return nil, ui.Err()
		}
		if selected == "" {
			return nil, nil
		}
		e = validateResumeSession(selected)
		if e == nil {
			manager, e = OpenSessionManager(selected, dir, nil)
		}
		if e == nil {
			if stat, statErr := os.Stat(manager.GetCWD()); statErr != nil || !stat.IsDir() {
				e = fmt.Errorf("Session working directory does not exist: %s", manager.GetCWD())
			}
		}
		if e == nil {
			return manager, nil
		}
		status = e.Error()
	}
}
func (m *InteractiveMode) selectSession(ctx context.Context) error {
	manager := m.runtime.Session().SessionManager()
	var dir *string
	if !manager.UsesDefaultSessionDir() {
		value := manager.GetSessionDir()
		if value != "" {
			dir = &value
		}
	}
	current, all := sessionLoaders(manager.GetCWD(), dir)
	done, finish := selectorSignal()
	selected := ""
	selector, err := NewSessionSelectorComponent(ctx, current, all, func(path string) { selected = path; finish() }, finish, SessionSelectorOptions{Keybindings: m.options.Keybindings, CurrentSessionFilePath: optionalHeadlessString(manager.GetSessionFile()), RenameSession: func(path, name string) error {
		if sameSessionPath(path, optionalHeadlessString(manager.GetSessionFile())) {
			return m.runtime.Session().SetSessionName(name)
		}
		if err := validateResumeSession(path); err != nil {
			return err
		}
		target, err := OpenSessionManager(path, nil, nil)
		if err != nil {
			return err
		}
		_, err = target.AppendSessionInfo(name)
		return err
	}})
	if err != nil {
		return err
	}
	if err = m.waitSelector(ctx, selector, done); err != nil {
		return err
	}
	if selected == "" {
		return nil
	}
	if err = validateResumeSession(selected); err != nil {
		return err
	}
	if m.settleSession != nil {
		if err = m.settleSession(); err != nil {
			return err
		}
	}
	_, err = m.runtime.SwitchSession(ctx, selected, SwitchSessionOptions{ProjectTrustContextFactory: func(cwd string) ProjectTrustContext {
		return ProjectTrustContext{CWD: cwd, HasUI: true, prepareSettings: func(ctx context.Context, settings *SettingsManager) error {
			return m.prepareSessionSettings(ctx, cwd, settings)
		}}
	}})
	if err != nil {
		return err
	}
	return m.sessionChanged("Resumed session")
}
func (m *InteractiveMode) newSession(ctx context.Context) error {
	if m.settleSession != nil {
		if err := m.settleSession(); err != nil {
			return err
		}
	}
	if _, err := m.runtime.NewSession(ctx); err != nil {
		return err
	}
	return m.sessionChanged("Started new session")
}
func (m *InteractiveMode) sessionChanged(status string) error {
	session := m.runtime.Session()
	m.renderSession.Store(session)
	provider, err := NewSessionAutocompleteProvider(session, nil)
	if err != nil {
		return err
	}
	fd, _ := findBinary(context.Background())
	if fd != "" {
		provider, err = NewSessionAutocompleteProvider(session, &fd)
		if err != nil {
			return err
		}
	}
	if err = m.ui.SetAutocompleteProvider(provider); err != nil {
		return err
	}
	visible, err := session.SettingsManager().GetAutocompleteMaxVisible()
	if err != nil {
		return err
	}
	_ = m.ui.SetAutocompleteMaxVisible(visible)
	messages := m.transcriptMessages()
	history := []string{}
	for _, message := range messages {
		if message.MessageRole() == "user" {
			history = append(history, sessionUserText(message))
		}
	}
	if err = m.transcript.SetMessages(messages); err != nil {
		return err
	}
	if err = m.ui.ResetSession(history); err != nil {
		return err
	}
	if fallback := m.runtime.ModelFallbackMessage(); fallback != nil {
		_ = m.ShowWarning(*fallback)
	}
	for _, d := range m.runtime.Diagnostics() {
		_ = m.ShowWarning(d.Message)
	}
	return m.ui.Append("\n" + status + " · " + optionalHeadlessString(session.SessionManager().GetSessionName()) + "\n")
}
func (m *InteractiveMode) prepareSessionSettings(ctx context.Context, cwd string, settings *SettingsManager) error {
	override := m.options.ProjectTrustOverride
	if cwd == m.runtime.CWD() {
		trusted, err := m.runtime.Session().SettingsManager().IsProjectTrusted()
		if err != nil {
			return err
		}
		override = &trusted
	}
	return prepareProjectSettings(ctx, PrepareProjectSettingsOptions{CWD: cwd, AgentDir: m.runtime.Services().AgentDir, SettingsManager: settings, Override: override}, func(ctx context.Context, title string, items []tui.SelectItem) (*tui.SelectItem, error) {
		done, finish := selectorSignal()
		var selected *tui.SelectItem
		dialog := tui.NewSelectDialog(title, items, func(item tui.SelectItem) { selected = &item; finish() }, finish)
		dialog.List.SetKeybindings(&m.options.Keybindings.KeybindingsManager)
		err := m.waitSelector(ctx, dialog, done)
		return selected, err
	})
}

package codingagent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nankedr/pig/tui"
)

// OpenExternalEditor edits the current draft using settings, VISUAL, EDITOR or the platform default.
func (m *InteractiveMode) OpenExternalEditor(ctx context.Context) error {
	if m.runtime == nil || m.runtime.Session() == nil {
		return errors.New("external editor requires an AgentSession runtime")
	}
	command, err := m.runtime.Session().SettingsManager().GetExternalEditorCommand()
	if err != nil {
		return err
	}
	runner, ok := m.ui.Terminal().(interface {
		RunCommand(context.Context, string, ...string) error
	})
	if !ok {
		return errors.New("terminal does not support external commands")
	}
	err = m.ui.EditExternally(ctx, func(ctx context.Context, content string) (string, error) {
		if err := m.ui.Terminal().Write("\x1b[?2031l"); err != nil {
			return "", err
		}
		directory, err := os.MkdirTemp("", "pig-editor-")
		if err != nil {
			return "", fmt.Errorf("external editor: %w", err)
		}
		defer os.RemoveAll(directory)
		path := filepath.Join(directory, "prompt.md")
		if err = os.WriteFile(path, []byte(content), 0600); err != nil {
			return "", fmt.Errorf("external editor: %w", err)
		}
		parts := strings.Split(command, " ")
		if err = m.ui.Terminal().Write("Launching external editor: " + tui.SafeTerminalText(command) + "\r\nPig will resume when the editor exits.\r\n"); err != nil {
			return "", err
		}
		if err = runner.RunCommand(ctx, parts[0], append(parts[1:], path)...); err != nil {
			return "", fmt.Errorf("external editor: %w", err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("external editor: %w", err)
		}
		return strings.TrimSuffix(string(data), "\n"), nil
	})
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.initialized && !m.stopped {
		err = errors.Join(err, m.themeNotifications())
	}
	return err
}

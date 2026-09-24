package codingagent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode"
)

type maintenanceResult struct {
	compaction CompactionResult
	path       string
	err        error
}

func maintenanceProgress(name string) string {
	if name == "compact" {
		return "Compacting..."
	}
	return "Reloading keybindings, skills, prompts, themes, and context files..."
}
func (m *InteractiveMode) runMaintenance(ctx context.Context, name, argument string) (result maintenanceResult) {
	s := m.runtime.Session()
	switch name {
	case "compact":
		result.compaction, result.err = s.Compact(ctx, strings.TrimSpace(argument))
	case "reload":
		result.err = s.Reload(ctx)
	case "export":
		path := strings.TrimSpace(argument)
		if path != "" {
			if path[0] == '\'' || path[0] == '"' {
				if end := strings.IndexByte(path[1:], path[0]); end >= 0 {
					path = path[1 : 1+end]
				} else {
					path = ""
				}
			} else if end := strings.IndexFunc(path, unicode.IsSpace); end >= 0 {
				path = path[:end]
			}
		}
		if strings.HasSuffix(path, ".jsonl") {
			result.err = notImplemented("InteractiveMode.exportJSONL")
			return
		}
		if path == "" {
			result.path, result.err = s.ExportToHTML(ctx)
		} else {
			result.path, result.err = s.ExportToHTML(ctx, path)
		}
	}
	return
}
func (m *InteractiveMode) finishMaintenance(name string, result maintenanceResult) error {
	if result.err != nil {
		label := map[string]string{"compact": "Compaction", "reload": "Reload", "export": "Export"}[name]
		if errors.Is(result.err, context.Canceled) || errors.Is(result.err, context.DeadlineExceeded) {
			return m.ShowError(label + " cancelled")
		}
		if name == "export" {
			return m.ShowError("Failed to export session: " + result.err.Error())
		}
		return m.ShowError(label + " failed: " + result.err.Error())
	}
	switch name {
	case "compact":
		if err := m.transcript.SetMessages(m.transcriptMessages()); err != nil {
			return err
		}
		return m.appendNotice(fmt.Sprintf("\nCompacted from %d tokens\n%s\n", result.compaction.TokensBefore, result.compaction.Summary))
	case "reload":
		if err := m.refreshResources(); err != nil {
			return m.ShowError("Reload failed: " + err.Error())
		}
		return m.appendNotice("\nReloaded keybindings, skills, prompts, themes, and context files\n")
	case "export":
		return m.appendNotice("\nSession exported to: " + result.path + "\n")
	}
	return nil
}
func (m *InteractiveMode) refreshResources() error {
	if err := m.options.Keybindings.Reload(); err != nil {
		return err
	}
	if err := m.applyInteractionSettings("session"); err != nil {
		return err
	}
	if err := m.applyInteractionSettings("skill-commands"); err != nil {
		return err
	}
	if err := m.reloadThemes(); err != nil {
		return err
	}
	loader := m.runtime.Session().ResourceLoader()
	skills, err := loader.GetSkills()
	if err != nil {
		return err
	}
	prompts, err := loader.GetPrompts()
	if err != nil {
		return err
	}
	themes, err := loader.GetThemes()
	if err != nil {
		return err
	}
	diagnostics := append(append(skills.Diagnostics, prompts.Diagnostics...), themes.Diagnostics...)
	if l, ok := loader.(*DefaultResourceLoader); ok {
		extensions, err := l.GetExtensionDiscovery()
		if err != nil {
			return err
		}
		diagnostics = append(diagnostics, extensions.Diagnostics...)
		diagnostics = append(diagnostics, l.GetContextFileDiagnostics()...)
		diagnostics = append(diagnostics, l.GetSystemPromptDiagnostics()...)
	}
	for _, d := range diagnostics {
		message := d.Message
		if d.Path != "" {
			message += ": " + d.Path
		}
		if d.Collision != nil {
			message += ": " + d.Collision.WinnerPath + " overrides " + d.Collision.LoserPath
		}
		if err := m.ShowWarning(message); err != nil {
			return err
		}
	}
	return nil
}
func (m *InteractiveMode) showSessionStats() error {
	s := m.runtime.Session()
	stats, err := s.GetSessionStats()
	if err != nil {
		return err
	}
	var b strings.Builder
	file := "In-memory"
	if stats.SessionFile != nil {
		file = *stats.SessionFile
	}
	fmt.Fprintf(&b, "\nSession Info\nFile: %s\nID: %s\n", file, stats.SessionID)
	if name := s.SessionManager().GetSessionName(); name != nil {
		fmt.Fprintf(&b, "Name: %s\n", *name)
	}
	fmt.Fprintf(&b, "\nCurrent context\n")
	if c := stats.ContextUsage; c == nil {
		b.WriteString("Unavailable\n")
	} else if c.Tokens == nil {
		fmt.Fprintf(&b, "Tokens: unknown / %d (awaiting next response)\n", c.ContextWindow)
	} else {
		fmt.Fprintf(&b, "Tokens: %d / %d (%.1f%%)\n", *c.Tokens, c.ContextWindow, *c.Percent)
	}
	fmt.Fprintf(&b, "\nFull history\nMessages: %d (User: %d, Assistant: %d, Tools: %d calls, %d results)\nInput: %d\nOutput: %d\nCache read: %d\nCache write: %d\nTotal: %d\nCost: $%.6f\n", stats.TotalMessages, stats.UserMessages, stats.AssistantMessages, stats.ToolCalls, stats.ToolResults, stats.Tokens.Input+stats.Tokens.CacheRead+stats.Tokens.CacheWrite, stats.Tokens.Output, stats.Tokens.CacheRead, stats.Tokens.CacheWrite, stats.Tokens.TotalTokens, stats.Cost)
	return m.appendNotice(b.String())
}

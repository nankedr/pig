package codingagent

import (
	"context"
	"errors"
	"strings"

	"github.com/nankedr/pig/tui"
)

var interactiveCommands = []struct{ name, description, hint string }{
	{"settings", "Open settings menu", ""},
	{"model", "Select model (opens selector UI)", "<provider/model>"},
	{"scoped-models", "Enable/disable models for Ctrl+P cycling", ""},
	{"export", "Export session (HTML default, or specify path: .html/.jsonl)", ""},
	{"import", "Import and resume a session from a JSONL file", ""},
	{"share", "Share session as a secret GitHub gist", ""},
	{"copy", "Copy last agent message to clipboard", ""},
	{"name", "Set session display name", ""},
	{"session", "Show session info and stats", ""},
	{"changelog", "Show changelog entries", ""},
	{"hotkeys", "Show all keyboard shortcuts", ""},
	{"fork", "Create a new fork from a previous user message", ""},
	{"clone", "Duplicate the current session at the current position", ""},
	{"tree", "Navigate session tree (switch branches)", ""},
	{"trust", "Save project trust decision for future sessions", ""},
	{"login", "Configure provider authentication", "<provider>"},
	{"logout", "Remove provider authentication", ""},
	{"new", "Start a new session", ""},
	{"compact", "Manually compact the session context", ""},
	{"resume", "Resume a different session", ""},
	{"reload", "Reload keybindings, extensions, skills, prompts, themes, and context files", ""},
	{"quit", "Quit pig", ""},
}

// NewSessionAutocompleteProvider uses only the Session's already authorized resource snapshot.
// A nil fdPath disables @ file search; ordinary path completion remains available.
func NewSessionAutocompleteProvider(session *AgentSession, fdPath *string) (*tui.CombinedAutocompleteProvider, error) {
	if session == nil || session.ResourceLoader() == nil || session.SessionManager() == nil {
		return nil, errors.New("autocomplete requires a configured AgentSession")
	}
	entries := make([]tui.AutocompleteEntry, 0, len(interactiveCommands))
	for _, command := range interactiveCommands {
		entries = append(entries, tui.SlashCommand{Name: command.name, Description: &command.description, ArgumentHint: &command.hint})
	}
	templates, err := session.PromptTemplates()
	if err != nil {
		return nil, err
	}
	for _, template := range templates {
		description := autocompleteDescription(template.Description, template.SourceInfo)
		entries = append(entries, tui.SlashCommand{Name: template.Name, Description: &description, ArgumentHint: &template.ArgumentHint})
	}
	enabled, err := session.SettingsManager().GetEnableSkillCommands()
	if err != nil {
		return nil, err
	}
	if enabled {
		loaded, err := session.ResourceLoader().GetSkills()
		if err != nil {
			return nil, err
		}
		for _, skill := range loaded.Skills {
			description := autocompleteDescription(skill.Description, skill.SourceInfo)
			entries = append(entries, tui.SlashCommand{Name: "skill:" + skill.Name, Description: &description})
		}
	}
	return tui.NewCombinedAutocompleteProvider(entries, session.SessionManager().GetCWD(), fdPath), nil
}
func autocompleteDescription(description string, source SourceInfo) string {
	tag := "t"
	if source.Scope == SourceScopeUser {
		tag = "u"
	} else if source.Scope == SourceScopeProject {
		tag = "p"
	}
	return strings.TrimSpace("[" + tag + "] " + description)
}
func (m *InteractiveMode) handleCommand(ctx context.Context, prompt string) (bool, error) {
	name, argument, _ := strings.Cut(strings.TrimSpace(prompt), " ")
	if !strings.HasPrefix(name, "/") {
		return false, nil
	}
	name = strings.TrimPrefix(name, "/")
	for _, command := range interactiveCommands {
		if command.name != name {
			continue
		}
		switch name {
		case "model":
			return true, m.selectModel(ctx, strings.TrimSpace(argument))
		case "scoped-models":
			return true, m.selectModelScope(ctx)
		case "settings":
			return true, m.selectThinking(ctx)
		}
		if name == "new" {
			_, err := m.runtime.NewSession(ctx)
			if err == nil {
				_ = m.transcript.SetMessages(nil)
				err = m.ui.Refresh()
			}
			return true, err
		}
		return true, notImplemented("InteractiveMode.command." + name)
	}
	return false, nil
}

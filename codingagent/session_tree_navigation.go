package codingagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/nankedr/pig/ai"
)

type NavigateTreeOptions struct {
	Summarize           bool
	CustomInstructions  string
	ReplaceInstructions bool
	Label               string
}
type NavigateTreeResult struct {
	EditorText   *string
	Cancelled    bool
	Aborted      bool
	SummaryEntry *BranchSummaryEntry
}

func (s *AgentSession) NavigateTree(ctx context.Context, targetID string, options ...NavigateTreeOptions) (NavigateTreeResult, error) {
	result := NavigateTreeResult{}
	if ctx == nil {
		return result, fmt.Errorf("tree navigation context must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if len(options) > 1 {
		return result, fmt.Errorf("expected at most one tree navigation options value")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.configurationReady(); err != nil {
		return result, err
	}
	m := s.sessionManager
	if m == nil {
		return result, fmt.Errorf("AgentSession has no SessionManager")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if m.leafID != nil && targetID == *m.leafID {
		return result, nil
	}
	var target *SessionEntry
	for i := range m.entries {
		if m.entries[i].ID == targetID {
			target = &m.entries[i]
			break
		}
	}
	if target == nil {
		return result, fmt.Errorf("Entry %s not found", targetID)
	}
	option := NavigateTreeOptions{}
	if len(options) > 0 {
		option = options[0]
	}
	if option.Summarize {
		return result, notImplemented("AgentSession.NavigateTree.Summarize")
	}
	leaf := &targetID
	if target.Type == "message" && target.Message != nil && target.Message.MessageRole() == ai.MessageRoleUser {
		leaf = cloneStringPointer(target.ParentID)
		text := sessionUserText(target.Message)
		result.EditorText = &text
	} else if target.Type == "custom_message" {
		leaf = cloneStringPointer(target.ParentID)
		data, err := json.Marshal(target.Content)
		if err != nil {
			return NavigateTreeResult{}, err
		}
		text := sessionContentText(data, "")
		result.EditorText = &text
	}
	entries := m.entries
	if option.Label != "" {
		entry := m.newEntryLocked("label")
		entry.ParentID, entry.TargetID, entry.Label = leaf, targetID, &option.Label
		entries = append(append([]SessionEntry{}, entries...), entry)
		leaf = &entry.ID
	}
	messages := BuildSessionContext(entries, leaf).Messages
	staged := ""
	if option.Label != "" && m.sessionFile != "" {
		persist := m.flushed
		for _, entry := range entries {
			if _, ok := sessionAssistantMessage(entry.Message); ok {
				persist = true
				break
			}
		}
		if persist {
			data, err := encodeSessionFile(m.header, entries)
			if err != nil {
				return NavigateTreeResult{}, err
			}
			file, err := os.CreateTemp(filepath.Dir(m.sessionFile), ".session-tree-*")
			if err != nil {
				return NavigateTreeResult{}, err
			}
			staged = file.Name()
			defer os.Remove(staged)
			_, writeErr := file.Write(data)
			if err := errors.Join(writeErr, file.Close()); err != nil {
				return NavigateTreeResult{}, err
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return NavigateTreeResult{}, err
	}
	previous := s.agent.State().Messages
	if err := s.agent.ReplaceMessages(messages); err != nil {
		return NavigateTreeResult{}, err
	}
	if staged != "" {
		if err := os.Rename(staged, m.sessionFile); err != nil {
			return NavigateTreeResult{}, errors.Join(err, s.agent.ReplaceMessages(previous))
		}
		m.flushed = true
	}
	m.entries, m.leafID = entries, cloneStringPointer(leaf)
	return result, nil
}

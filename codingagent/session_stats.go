package codingagent

import (
	"strings"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
)

func (s *AgentSession) GetSessionStats() (SessionStats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.agent == nil || s.sessionManager == nil {
		return SessionStats{}, notImplemented("AgentSession.GetSessionStats")
	}
	m := s.sessionManager
	m.mu.RLock()
	defer m.mu.RUnlock()
	stats := SessionStats{SessionID: m.sessionID}
	if m.sessionFile != "" {
		file := m.sessionFile
		stats.SessionFile = &file
	}
	add := func(usage ai.Usage) {
		stats.Tokens.Input += usage.Input
		stats.Tokens.Output += usage.Output
		stats.Tokens.CacheRead += usage.CacheRead
		stats.Tokens.CacheWrite += usage.CacheWrite
		stats.Cost += usage.Cost.Total
	}
	for _, entry := range m.entries {
		if (entry.Type == "compaction" || entry.Type == "branch_summary") && entry.Usage != nil {
			add(*entry.Usage)
		}
		if entry.Type != "message" {
			continue
		}
		stats.TotalMessages++
		switch entry.Message.MessageRole() {
		case ai.MessageRoleUser:
			stats.UserMessages++
		case ai.MessageRoleToolResult:
			stats.ToolResults++
			switch message := entry.Message.(type) {
			case ai.ToolResultMessage:
				if usage, ok := message.Usage.Value(); ok {
					add(usage)
				}
			case *ai.ToolResultMessage:
				if usage, ok := message.Usage.Value(); ok {
					add(usage)
				}
			}
		case ai.MessageRoleAssistant:
			stats.AssistantMessages++
			if message, ok := sessionAssistantMessage(entry.Message); ok {
				for _, content := range message.Content {
					if content.ContentType() == ai.ContentTypeToolCall {
						stats.ToolCalls++
					}
				}
				add(message.Usage)
			}
		}
	}
	stats.Tokens.TotalTokens = stats.Tokens.Input + stats.Tokens.Output + stats.Tokens.CacheRead + stats.Tokens.CacheWrite
	stats.ContextUsage = sessionContextUsage(s.agent.State(), buildContextPath(m.entries, m.leafID, true))
	return stats, nil
}

func (s *AgentSession) GetContextUsage() (*ContextUsage, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.agent == nil {
		return nil, notImplemented("AgentSession.GetContextUsage")
	}
	var branch []SessionEntry
	if s.sessionManager != nil {
		branch = s.sessionManager.GetBranch()
	}
	return sessionContextUsage(s.agent.State(), branch), nil
}

func sessionContextUsage(state agent.AgentState, branch []SessionEntry) *ContextUsage {
	if state.Model.ContextWindow <= 0 {
		return nil
	}
	result := &ContextUsage{ContextWindow: state.Model.ContextWindow}
	hasUsage := false
	for i := len(branch) - 1; i >= 0; i-- {
		entry := branch[i]
		if entry.Type == "compaction" {
			if !hasUsage {
				return result
			}
			break
		}
		if entry.Type == "message" {
			if m, ok := sessionAssistantMessage(entry.Message); ok && m.StopReason != ai.StopReasonError && m.StopReason != ai.StopReasonAborted && CalculateContextTokens(m.Usage) > 0 {
				hasUsage = true
			}
		}
	}
	var tokens int64
	for i := len(state.Messages) - 1; i >= 0; i-- {
		message := state.Messages[i]
		if m, ok := sessionAssistantMessage(message); ok && m.StopReason != ai.StopReasonError && m.StopReason != ai.StopReasonAborted {
			if usage := CalculateContextTokens(m.Usage); usage > 0 {
				tokens += usage
				break
			}
		}
		tokens += EstimateTokens(message)
	}
	percent := float64(tokens) / float64(result.ContextWindow) * 100
	result.Tokens, result.Percent = &tokens, &percent
	return result
}

func (s *AgentSession) GetLastAssistantText() (*string, error) {
	if s.agent == nil {
		return nil, notImplemented("AgentSession.GetLastAssistantText")
	}
	messages := s.Messages()
	for i := len(messages) - 1; i >= 0; i-- {
		message, ok := sessionAssistantMessage(messages[i])
		if !ok || message.StopReason == ai.StopReasonAborted && len(message.Content) == 0 {
			continue
		}
		var text strings.Builder
		for _, content := range message.Content {
			switch block := content.(type) {
			case ai.TextContent:
				text.WriteString(block.Text)
			case *ai.TextContent:
				text.WriteString(block.Text)
			}
		}
		trimmed := strings.Trim(text.String(), "\t\n\v\f\r \u00a0\u1680\u2000\u2001\u2002\u2003\u2004\u2005\u2006\u2007\u2008\u2009\u200a\u2028\u2029\u202f\u205f\u3000\ufeff")
		if trimmed == "" {
			return nil, nil
		}
		return &trimmed, nil
	}
	return nil, nil
}

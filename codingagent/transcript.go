package codingagent

import (
	"fmt"
	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/tui"
	"strings"
	"sync"
)

// Transcript projects Session messages and events into text without changing model context.
type Transcript struct {
	mu                     sync.Mutex
	messages               []agent.AgentMessage
	current                int
	tools                  map[string]*ToolExecutionComponent
	expanded, hideThinking bool
}

func NewTranscript() *Transcript {
	return &Transcript{current: -1, tools: make(map[string]*ToolExecutionComponent)}
}
func (*Transcript) Invalidate() error { return nil }
func (t *Transcript) SetExpanded(v bool) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.expanded = v
	return nil
}
func (t *Transcript) SetHideThinkingBlock(v bool) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.hideThinking = v
	return nil
}
func (t *Transcript) SetMessages(messages []agent.AgentMessage) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.messages = nil
	t.tools = make(map[string]*ToolExecutionComponent)
	t.current = -1
	for _, m := range messages {
		m = cloneSessionAgentMessage(m)
		t.messages = append(t.messages, m)
		t.track(m)
	}
	return nil
}
func (t *Transcript) tool(id, name string, args any) *ToolExecutionComponent {
	if t.tools == nil {
		t.tools = make(map[string]*ToolExecutionComponent)
	}
	c := t.tools[id]
	if c == nil {
		c = NewToolExecutionComponent(name, id, args)
		t.tools[id] = c
	}
	return c
}
func (t *Transcript) track(m agent.AgentMessage) {
	switch m := m.(type) {
	case ai.AssistantMessage:
		for _, p := range m.Content {
			if call, ok := p.(ai.ToolCall); ok {
				_ = t.tool(call.ID, call.Name, call.Arguments).UpdateArgs(call.Arguments)
			}
		}
	case ai.ToolResultMessage:
		_ = t.tool(m.ToolCallID, m.ToolName, nil).UpdateResult(ToolExecutionResult{Content: m.Content, Details: m.Details, IsError: m.IsError})
	}
}
func (t *Transcript) Update(event AgentSessionEvent) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	switch e := event.(type) {
	case AgentSessionMessageStartEvent:
		m := cloneSessionAgentMessage(e.Message)
		t.messages = append(t.messages, m)
		t.current = len(t.messages) - 1
		t.track(m)
	case AgentSessionMessageUpdateEvent:
		if t.current >= 0 && t.current < len(t.messages) {
			m := cloneSessionAgentMessage(e.Message)
			t.messages[t.current] = m
			t.track(m)
		}
	case AgentSessionMessageEndEvent:
		m := cloneSessionAgentMessage(e.Message)
		if t.current >= 0 && t.current < len(t.messages) {
			t.messages[t.current] = m
		}
		t.current = -1
		t.track(m)
		if assistant, ok := m.(ai.AssistantMessage); ok && (assistant.StopReason == ai.StopReasonError || assistant.StopReason == ai.StopReasonAborted) {
			text, _ := assistant.ErrorMessage.Value()
			if text == "" {
				text = "Operation aborted"
			}
			for _, part := range assistant.Content {
				if call, ok := part.(ai.ToolCall); ok {
					_ = t.tool(call.ID, call.Name, call.Arguments).UpdateResult(ToolExecutionResult{Content: []ai.ToolResultContent{ai.TextContent{Type: ai.ContentTypeText, Text: text}}, IsError: true})
				}
			}
		}
	case AgentSessionToolExecutionStartEvent:
		c := t.tool(e.ToolCallID, e.ToolName, e.Arguments)
		_ = c.UpdateArgs(e.Arguments)
		_ = c.SetArgsComplete(true)
		return c.MarkExecutionStarted()
	case AgentSessionToolExecutionUpdateEvent:
		return t.tool(e.ToolCallID, e.ToolName, e.Arguments).UpdateResult(ToolExecutionResult{Content: e.PartialResult.Content, Details: ai.Some(e.PartialResult.Details)}, true)
	case AgentSessionToolExecutionEndEvent:
		return t.tool(e.ToolCallID, e.ToolName, nil).UpdateResult(ToolExecutionResult{Content: e.Result.Content, Details: ai.Some(e.Result.Details), IsError: e.IsError})
	}
	return nil
}
func (t *Transcript) Render(width int) ([]string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	var lines []string
	seen := make(map[string]bool)
	theme, _ := GetMarkdownTheme()
	toolLines := func(id string) error {
		if seen[id] {
			return nil
		}
		seen[id] = true
		c := t.tools[id]
		if c == nil {
			return nil
		}
		c.SetExpanded(t.expanded)
		part, err := c.Render(width)
		lines = append(lines, part...)
		return err
	}
	for _, message := range t.messages {
		var component tui.Component
		switch m := message.(type) {
		case ai.UserMessage:
			component = NewUserMessageComponent("user: " + sessionUserText(m))
		case ai.AssistantMessage:
			c := NewAssistantMessageComponent(m)
			c.SetHideThinkingBlock(t.hideThinking)
			component = c
		case ai.ToolResultMessage:
			if err := toolLines(m.ToolCallID); err != nil {
				return nil, err
			}
			continue
		case agent.CompactionSummaryMessage:
			component = tui.NewMarkdown("Compacted context\n\n"+m.Summary, 0, 1, theme, nil)
		case agent.BranchSummaryMessage:
			component = tui.NewMarkdown("Branch summary\n\n"+m.Summary, 0, 1, theme, nil)
		case agent.BashExecutionMessage:
			text := "$ " + m.Command + "\n" + m.Output
			if m.Cancelled {
				text += "\n(command cancelled)"
			} else if m.ExitCodeSet && m.ExitCode != 0 {
				text += fmt.Sprintf("\nCommand exited with code %d", m.ExitCode)
			}
			if m.Truncated && m.FullOutputPath != "" {
				text += "\n[Output truncated. Full output: " + m.FullOutputPath + "]"
			}
			part, err := tui.WrapTextWithANSI(text, max(1, width))
			if err != nil {
				return nil, err
			}
			lines = append(lines, append([]string{""}, part...)...)
			continue
		case agent.CustomMessage:
			if m.Display {
				component = tui.NewMarkdown("["+m.CustomType+"]\n\n"+sessionUserText(ai.UserMessage{Content: m.Content}), 0, 1, theme, nil)
			}
		}
		if component != nil {
			part, err := component.Render(width)
			if err != nil {
				return nil, err
			}
			lines = append(lines, part...)
		}
		if m, ok := message.(ai.AssistantMessage); ok {
			for _, p := range m.Content {
				if call, ok := p.(ai.ToolCall); ok {
					if err := toolLines(call.ID); err != nil {
						return nil, err
					}
				}
			}
		}
	}
	for i, s := range lines {
		lines[i] = strings.TrimRight(s, " ")
	}
	return lines, nil
}

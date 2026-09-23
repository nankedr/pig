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
	theme                  *Theme
	outputPad              int
	mu                     sync.Mutex
	messages               []agent.AgentMessage
	current                int
	tools                  map[string]*ToolExecutionComponent
	expanded, hideThinking bool
}

func (t *Transcript) SetTheme(theme *Theme) { t.mu.Lock(); defer t.mu.Unlock(); t.theme = theme }
func (t *Transcript) SetOutputPad(pad int)  { t.mu.Lock(); defer t.mu.Unlock(); t.outputPad = pad }
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
		t.finishMessage(m)
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
		t.finishMessage(m)
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
	if t.theme != nil {
		theme = t.theme.MarkdownTheme()
	}
	toolLines := func(id string) error {
		if seen[id] {
			return nil
		}
		seen[id] = true
		c := t.tools[id]
		if c == nil {
			return nil
		}
		c.theme = t.theme
		c.SetExpanded(t.expanded)
		part, err := c.Render(width)
		lines = append(lines, part...)
		return err
	}
	for _, message := range t.messages {
		var component tui.Component
		switch m := message.(type) {
		case ai.UserMessage:
			c := NewUserMessageComponent("user: " + sessionUserText(m))
			c.theme = t.theme
			c.SetOutputPad(t.outputPad != 0)
			component = c
		case ai.AssistantMessage:
			c := NewAssistantMessageComponent(m)
			c.theme = t.theme
			c.SetHideThinkingBlock(t.hideThinking)
			c.SetOutputPad(t.outputPad != 0)
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
			if t.theme != nil {
				for i, line := range part {
					part[i] = t.theme.FG("toolOutput", line)
				}
			}
			lines = append(lines, append([]string{""}, part...)...)
			continue
		case agent.CustomMessage:
			if m.Display {
				var style *tui.DefaultTextStyle
				if t.theme != nil {
					style = &tui.DefaultTextStyle{Color: func(s string) string { return t.theme.FG("customMessageText", s) }, BGColor: func(s string) string { return t.theme.BG("customMessageBg", s) }}
				}
				component = tui.NewMarkdown("["+m.CustomType+"]\n\n"+sessionUserText(ai.UserMessage{Content: m.Content}), t.outputPad, 1, theme, style)
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

func (t *Transcript) finishMessage(m agent.AgentMessage) {
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
}

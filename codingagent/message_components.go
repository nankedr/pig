package codingagent

import (
	"encoding/json"
	"fmt"
	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/tui"
	"strings"
)

type AssistantMessageComponent struct {
	tui.Container
	message                 ai.AssistantMessage
	hideThinking, outputPad bool
	hiddenThinkingLabel     string
}

func NewAssistantMessageComponent(message ...ai.AssistantMessage) *AssistantMessageComponent {
	c := new(AssistantMessageComponent)
	if len(message) > 0 {
		_ = c.UpdateContent(message[0])
	}
	return c
}
func (*AssistantMessageComponent) Invalidate() error { return nil }
func (c *AssistantMessageComponent) SetHiddenThinkingLabel(s string) error {
	c.hiddenThinkingLabel = s
	return nil
}
func (c *AssistantMessageComponent) SetHideThinkingBlock(hide bool) error {
	c.hideThinking = hide
	return nil
}
func (c *AssistantMessageComponent) SetOutputPad(pad bool) error { c.outputPad = pad; return nil }
func (c *AssistantMessageComponent) UpdateContent(message ai.AssistantMessage, _ ...bool) error {
	c.message = ai.CloneAssistantMessage(message)
	for i, part := range c.message.Content {
		switch p := part.(type) {
		case *ai.TextContent:
			if p != nil {
				c.message.Content[i] = *p
			}
		case *ai.ThinkingContent:
			if p != nil {
				c.message.Content[i] = *p
			}
		case *ai.ToolCall:
			if p != nil {
				c.message.Content[i] = *p
			}
		}
	}
	return nil
}
func (c *AssistantMessageComponent) Render(width int) ([]string, error) {
	var lines []string
	pad := 0
	if c.outputPad {
		pad = 1
	}
	theme, _ := GetMarkdownTheme()
	appendMarkdown := func(s string, thinking bool) error {
		var style *tui.DefaultTextStyle
		if thinking {
			yes := true
			style = &tui.DefaultTextStyle{Italic: &yes}
		}
		part, err := tui.NewMarkdown(s, pad, 0, theme, style).Render(width)
		lines = append(lines, part...)
		return err
	}
	visible := false
	for _, part := range c.message.Content {
		switch p := part.(type) {
		case ai.TextContent:
			visible = visible || strings.TrimSpace(p.Text) != ""
		case ai.ThinkingContent:
			visible = visible || strings.TrimSpace(p.Thinking) != ""
		}
	}
	if visible {
		lines = append(lines, "")
	}
	hasTools := false
	for i := 0; i < len(c.message.Content); i++ {
		switch part := c.message.Content[i].(type) {
		case ai.TextContent:
			if err := appendMarkdown(strings.TrimSpace(part.Text), false); err != nil {
				return nil, err
			}
		case ai.ToolCall:
			hasTools = true
		case ai.ThinkingContent:
			var blocks []string
			for ; i < len(c.message.Content); i++ {
				p, ok := c.message.Content[i].(ai.ThinkingContent)
				if !ok {
					break
				}
				if s := strings.TrimSpace(p.Thinking); s != "" {
					blocks = append(blocks, s)
				}
			}
			i--
			if len(blocks) == 0 {
				continue
			}
			s := strings.Join(blocks, "\n\n")
			if c.hideThinking {
				s = c.hiddenThinkingLabel
				if s == "" {
					s = "Thinking..."
				}
			}
			if err := appendMarkdown(s, true); err != nil {
				return nil, err
			}
			for _, next := range c.message.Content[i+1:] {
				show := false
				switch p := next.(type) {
				case ai.TextContent:
					show = strings.TrimSpace(p.Text) != ""
				case ai.ThinkingContent:
					show = strings.TrimSpace(p.Thinking) != ""
				}
				if show {
					lines = append(lines, "")
					break
				}
			}
		}
	}
	failure := ""
	message, _ := c.message.ErrorMessage.Value()
	switch c.message.StopReason {
	case ai.StopReasonLength:
		failure = "Response was truncated before completion."
	case ai.StopReasonAborted:
		if !hasTools {
			failure = message
			if failure == "" || failure == "Request was aborted" {
				failure = "Operation aborted"
			}
		}
	case ai.StopReasonError:
		if !hasTools {
			if message == "" {
				message = "Unknown error"
			}
			failure = "Error: " + message
		}
	}
	if failure != "" {
		lines = append(lines, "")
		part, _ := tui.WrapTextWithANSI("\x1b[31m"+failure+"\x1b[0m", max(1, width-2*pad))
		for _, line := range part {
			lines = append(lines, strings.Repeat(" ", pad)+line)
		}
	}
	return lines, nil
}

type UserMessageComponent struct {
	tui.Container
	text      string
	outputPad bool
}

func NewUserMessageComponent(text string) *UserMessageComponent {
	return &UserMessageComponent{text: text}
}
func (c *UserMessageComponent) SetOutputPad(pad bool) error { c.outputPad = pad; return nil }
func (c *UserMessageComponent) Render(width int) ([]string, error) {
	pad := 0
	if c.outputPad {
		pad = 1
	}
	yes := true
	theme, _ := GetMarkdownTheme()
	return tui.NewMarkdown(c.text, pad, 1, theme, nil, tui.MarkdownOptions{PreserveOrderedListMarkers: &yes, PreserveBackslashEscapes: &yes}).Render(width)
}

type ToolExecutionComponent struct {
	tui.Container
	name, id                               string
	args                                   any
	result                                 *ToolExecutionResult
	expanded, started, argsComplete, final bool
}

func NewToolExecutionComponent(name, id string, args any, options ...ToolExecutionOptions) *ToolExecutionComponent {
	c := &ToolExecutionComponent{name: name, id: id}
	_ = c.UpdateArgs(args)
	return c
}
func (*ToolExecutionComponent) Invalidate() error              { return nil }
func (c *ToolExecutionComponent) MarkExecutionStarted() error  { c.started = true; return nil }
func (c *ToolExecutionComponent) SetArgsComplete(v bool) error { c.argsComplete = v; return nil }
func (c *ToolExecutionComponent) SetExpanded(v bool) error     { c.expanded = v; return nil }
func (*ToolExecutionComponent) SetImageWidthCells(int) error {
	return notImplemented("ToolExecutionComponent.SetImageWidthCells")
}
func (*ToolExecutionComponent) SetShowImages(v bool) error {
	if v {
		return notImplemented("ToolExecutionComponent.SetShowImages")
	}
	return nil
}
func (c *ToolExecutionComponent) UpdateArgs(args any) error {
	if c.final {
		return nil
	}
	b, err := json.Marshal(args)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, &c.args)
}
func (c *ToolExecutionComponent) UpdateResult(result ToolExecutionResult, partial ...bool) error {
	if c.final {
		return nil
	}
	msg := ai.ToolResultMessage{Role: ai.MessageRoleToolResult, ToolCallID: c.id, ToolName: c.name, Content: result.Content, Details: result.Details, IsError: result.IsError}
	encoded, err := ai.MarshalMessage(msg)
	if err != nil {
		return err
	}
	decoded, err := ai.UnmarshalMessage(encoded)
	if err != nil {
		return err
	}
	cloned := decoded.(ai.ToolResultMessage)
	c.result = &ToolExecutionResult{Content: cloned.Content, Details: cloned.Details, IsError: cloned.IsError}
	c.final = len(partial) == 0 || !partial[0]
	return nil
}
func (c *ToolExecutionComponent) Render(width int) ([]string, error) {
	status := "preparing"
	if c.started {
		status = "running"
	}
	if c.final {
		status = "done"
		if c.result.IsError {
			status = "error"
		}
	}
	title := fmt.Sprintf("%s [%s]", c.name, status)
	args, err := json.MarshalIndent(c.args, "", "  ")
	if err != nil {
		return nil, err
	}
	if c.args != nil {
		title += "\n" + string(args)
	}
	header, _ := tui.WrapTextWithANSI(title, max(1, width))
	lines := append([]string{""}, header...)
	if c.result != nil {
		var output []string
		for _, block := range c.result.Content {
			switch p := block.(type) {
			case ai.TextContent:
				output = append(output, p.Text)
			case *ai.TextContent:
				if p != nil {
					output = append(output, p.Text)
				}
			case ai.ImageContent:
				output = append(output, "[image]")
			}
		}
		text := strings.Join(output, "\n")
		if c.name == "edit" {
			if details, ok := c.result.Details.Value(); ok {
				if fields, ok := details.(map[string]any); ok {
					if diff, ok := fields["diff"].(string); ok {
						text, _ = RenderDiff(diff)
					}
				}
			}
		}
		body, _ := tui.WrapTextWithANSI(text, max(1, width))
		if !c.expanded && len(body) > 10 {
			hidden := len(body) - 10
			body = append(append([]string(nil), body[:10]...), fmt.Sprintf("… (%d more lines, ctrl+o to expand)", hidden))
		}
		for _, line := range body {
			wrapped, _ := tui.WrapTextWithANSI(line, max(1, width))
			lines = append(lines, wrapped...)
		}
	}
	return lines, nil
}
func RenderDiff(text string, _ ...RenderDiffOptions) (string, error) {
	lines := strings.Split(tui.SafeTerminalText(text), "\n")
	for i, line := range lines {
		color := "2"
		if strings.HasPrefix(line, "+") {
			color = "32"
		} else if strings.HasPrefix(line, "-") {
			color = "31"
		}
		lines[i] = "\x1b[" + color + "m" + line + "\x1b[0m"
	}
	return strings.Join(lines, "\n"), nil
}

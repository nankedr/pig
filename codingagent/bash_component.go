package codingagent

import (
	"fmt"
	"strings"
	"sync"

	"github.com/nankedr/pig/tui"
)

type BashExecutionComponent struct {
	tui.Container
	mu                                     sync.Mutex
	command, output                        string
	excluded, expanded                     bool
	complete, cancelled, truncated, failed bool
	exitCode                               *int
	fullOutputPath                         string
	theme                                  *Theme
	cancelHint                             string
}

func NewBashExecutionComponent(command string, excludeFromContext ...bool) *BashExecutionComponent {
	return &BashExecutionComponent{command: command, excluded: len(excludeFromContext) > 0 && excludeFromContext[0], cancelHint: "esc"}
}
func (*BashExecutionComponent) Invalidate() error { return nil }
func (c *BashExecutionComponent) AppendOutput(chunk string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.complete {
		c.output += strings.ReplaceAll(strings.ReplaceAll(sessionBashANSI.ReplaceAllString(chunk, ""), "\r\n", "\n"), "\r", "\n")
	}
	return nil
}
func (c *BashExecutionComponent) GetCommand() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.command, nil
}
func (c *BashExecutionComponent) GetOutput() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.output, nil
}
func (c *BashExecutionComponent) SetComplete(exitCode *int, cancelled bool, truncation *TruncationResult, fullOutputPath *string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.complete {
		return nil
	}
	c.complete, c.cancelled = true, cancelled
	if exitCode != nil {
		code := *exitCode
		c.exitCode = &code
	}
	c.truncated = truncation != nil && truncation.Truncated
	if fullOutputPath != nil {
		c.fullOutputPath = *fullOutputPath
	}
	return nil
}
func (c *BashExecutionComponent) SetExpanded(expanded bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.expanded = expanded
	return nil
}
func (c *BashExecutionComponent) Render(width int) ([]string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	color := "bashMode"
	if c.excluded {
		color = "dim"
	}
	style := func(key, text string) string {
		if c.theme != nil {
			return c.theme.FG(ThemeColor(key), text)
		}
		return text
	}
	lines, err := tui.WrapTextWithANSI(style(color, "$ "+tui.SafeTerminalText(c.command)), max(1, width))
	if err != nil {
		return nil, err
	}
	tail := TruncateTail(c.output)
	output, err := tui.WrapTextWithANSI(tui.SafeTerminalText(tail.Content), max(1, width))
	if err != nil {
		return nil, err
	}
	hidden := max(0, len(output)-20)
	if !c.expanded && hidden > 0 {
		output = output[hidden:]
	}
	for _, line := range output {
		lines = append(lines, style("muted", line))
	}
	if !c.complete {
		lines = append(lines, "Running... ("+c.cancelHint+" to cancel)")
	} else {
		if hidden > 0 {
			if c.expanded {
				lines = append(lines, "(collapse tools)")
			} else {
				lines = append(lines, fmt.Sprintf("... %d more lines (expand tools)", hidden))
			}
		}
		if c.cancelled {
			lines = append(lines, style("warning", "(cancelled)"))
		} else if c.exitCode != nil && *c.exitCode != 0 {
			lines = append(lines, style("error", fmt.Sprintf("(exit %d)", *c.exitCode)))
		}
		if (c.truncated || tail.Truncated) && c.fullOutputPath != "" {
			lines = append(lines, style("warning", "Output truncated. Full output: "+tui.SafeTerminalText(c.fullOutputPath)))
		}
	}
	return append([]string{""}, lines...), nil
}

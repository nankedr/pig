package main

import (
	"fmt"
	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
	"strings"
)

func main() {
	transcript := codingagent.NewTranscript()
	transcript.SetMessages([]agent.AgentMessage{
		ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText("展示 **Markdown** 和 Tool 输出")},
		ai.AssistantMessage{Role: ai.MessageRoleAssistant, StopReason: ai.StopReasonToolUse, Content: []ai.AssistantContent{ai.ThinkingContent{Type: ai.ContentTypeThinking, Thinking: "先检查文件，再给出答案。"}, ai.ToolCall{Type: ai.ContentTypeToolCall, ID: "read-1", Name: "read", Arguments: map[string]any{"path": "hello.go"}}}},
	})
	transcript.Update(codingagent.AgentSessionToolExecutionEndEvent{ToolExecutionEndEvent: agent.ToolExecutionEndEvent{ToolCallID: "read-1", ToolName: "read", Result: agent.ErasedAgentToolResult{Content: []ai.ToolResultContent{ai.TextContent{Type: ai.ContentTypeText, Text: "package main\n\nfunc main() {}"}}}}})
	transcript.Update(codingagent.AgentSessionMessageStartEvent{MessageStartEvent: agent.MessageStartEvent{Message: ai.AssistantMessage{Role: ai.MessageRoleAssistant, Content: []ai.AssistantContent{ai.TextContent{Type: ai.ContentTypeText, Text: "## 完成\n\n- 已读取文件\n- [Go](https://go.dev)\n\n```go\nfunc main() {}\n```"}}}}})
	lines, err := transcript.Render(60)
	if err != nil {
		panic(err)
	}
	fmt.Println(strings.Join(lines, "\n"))
}

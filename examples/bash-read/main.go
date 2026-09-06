package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	bash, err := codingagent.CreateBashTool(cwd)
	if err != nil {
		return err
	}
	read, err := codingagent.CreateReadTool(cwd)
	if err != nil {
		return err
	}
	core, err := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
	if err != nil {
		return err
	}
	call, err := ai.FauxAssistantMessage(ai.FauxAssistantBlocks(ai.ToolCall{Type: "toolCall", ID: "bash", Name: "bash", Arguments: map[string]any{"command": "i=1; while [ $i -le 2100 ]; do printf 'line-%04d\\n' $i; i=$((i+1)); done"}}), ai.FauxAssistantMessageOptions{StopReason: ai.Some(ai.StopReasonToolUse)})
	if err != nil {
		return err
	}
	var path string
	core.SetResponses([]ai.FauxResponseStep{call, ai.FauxResponseFactory(func(input ai.Context, _ *ai.SimpleStreamOptions, _ *ai.FauxProviderState, _ ai.Model) (ai.AssistantMessage, error) {
		result, ok := input.Messages[len(input.Messages)-1].(ai.ToolResultMessage)
		if !ok || result.IsError {
			return ai.AssistantMessage{}, fmt.Errorf("bash failed")
		}
		details, _ := result.Details.Value()
		if object, ok := details.(map[string]any); ok {
			path, _ = object["fullOutputPath"].(string)
		}
		if path == "" {
			return ai.AssistantMessage{}, fmt.Errorf("missing retained output")
		}
		return ai.FauxAssistantMessage(ai.FauxAssistantBlocks(ai.ToolCall{Type: "toolCall", ID: "read", Name: "read", Arguments: map[string]any{"path": path, "limit": 2}}), ai.FauxAssistantMessageOptions{StopReason: ai.Some(ai.StopReasonToolUse)})
	}), ai.FauxResponseFactory(func(input ai.Context, _ *ai.SimpleStreamOptions, _ *ai.FauxProviderState, _ ai.Model) (ai.AssistantMessage, error) {
		result, ok := input.Messages[len(input.Messages)-1].(ai.ToolResultMessage)
		if !ok || result.IsError || len(result.Content) == 0 {
			return ai.AssistantMessage{}, fmt.Errorf("read failed")
		}
		text, ok := result.Content[0].(ai.TextContent)
		if !ok || !strings.HasPrefix(text.Text, "line-0001\nline-0002") {
			return ai.AssistantMessage{}, fmt.Errorf("full output did not retain its beginning")
		}
		return ai.FauxAssistantMessage(ai.FauxAssistantText("已执行宿主 shell，并通过 read 读取完整输出的开头。"))
	})})
	model, _ := core.GetModel()
	created, err := codingagent.CreateAgentSession(context.Background(), codingagent.CreateAgentSessionOptions{CWD: cwd, Model: &model, Tools: []string{"bash", "read"}, AgentTools: []agent.ErasedAgentTool{bash, read}, StreamFunction: agent.StreamFunction(core.StreamSimple)})
	if err != nil {
		return err
	}
	defer created.Session.Dispose()
	if err := created.Session.Prompt(context.Background(), "执行命令并回读完整输出"); err != nil {
		return err
	}
	created.Session.Dispose()
	if _, err := os.Stat(path); err != nil {
		return err
	}
	fmt.Printf("bash → read → continuation 完成。\nSession 已释放，完整输出仍保留：%s\n文件由用户管理，需要时可手动删除。\n", path)
	return nil
}

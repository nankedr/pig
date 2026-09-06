package main

import (
	"context"
	"fmt"
	"os"

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
	cwd, err := os.MkdirTemp("", "pig-edit-read-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(cwd)
	write, err := codingagent.CreateWriteTool(cwd)
	if err != nil {
		return err
	}
	edit, err := codingagent.CreateEditTool(cwd)
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
	var responses []ai.FauxResponseStep
	for _, call := range []ai.ToolCall{
		{Type: "toolCall", ID: "save", Name: "write", Arguments: map[string]any{"path": "nested/hello.txt", "content": "Hello\r\nworld\r\n"}},
		{Type: "toolCall", ID: "change", Name: "edit", Arguments: map[string]any{"path": "nested/hello.txt", "edits": []any{map[string]any{"oldText": "world\n", "newText": "你好🙂\n"}}}},
		{Type: "toolCall", ID: "verify", Name: "read", Arguments: map[string]any{"path": "nested/hello.txt"}},
	} {
		response, err := ai.FauxAssistantMessage(ai.FauxAssistantBlocks(call), ai.FauxAssistantMessageOptions{StopReason: ai.Some(ai.StopReasonToolUse)})
		if err != nil {
			return err
		}
		responses = append(responses, response)
	}
	final, err := ai.FauxAssistantMessage(ai.FauxAssistantText("文件已修改，diff 和回读已进入模型上下文。"))
	if err != nil {
		return err
	}
	responses = append(responses, ai.FauxResponseFactory(func(input ai.Context, _ *ai.SimpleStreamOptions, _ *ai.FauxProviderState, _ ai.Model) (ai.AssistantMessage, error) {
		result, ok := input.Messages[len(input.Messages)-1].(ai.ToolResultMessage)
		if !ok || result.IsError || len(result.Content) != 1 {
			return ai.AssistantMessage{}, fmt.Errorf("missing successful read result")
		}
		if text, ok := result.Content[0].(ai.TextContent); !ok || text.Text != "Hello\r\n你好🙂\r\n" {
			return ai.AssistantMessage{}, fmt.Errorf("read did not match edited content")
		}
		return final, nil
	}))
	core.SetResponses(responses)
	model, _ := core.GetModel()
	created, err := codingagent.CreateAgentSession(context.Background(), codingagent.CreateAgentSessionOptions{
		CWD: cwd, Model: &model, Tools: []string{"write", "edit", "read"}, AgentTools: []agent.ErasedAgentTool{write, edit, read}, StreamFunction: agent.StreamFunction(core.StreamSimple),
	})
	if err != nil {
		return err
	}
	defer created.Session.Dispose()
	if err := created.Session.Prompt(context.Background(), "创建文件、编辑并读回"); err != nil {
		return err
	}
	for _, message := range created.Session.Messages() {
		switch message := message.(type) {
		case ai.ToolResultMessage:
			if message.ToolName == "edit" {
				details, ok := message.Details.Value()
				if !ok {
					return fmt.Errorf("edit has no details")
				}
				fmt.Printf("edit details: %v\n", details)
			}
			if message.IsError {
				return fmt.Errorf("%s failed: %v", message.ToolName, message.Content)
			}
			for _, block := range message.Content {
				if text, ok := block.(ai.TextContent); ok {
					fmt.Println(text.Text)
				}
			}
		case ai.AssistantMessage:
			for _, block := range message.Content {
				if text, ok := block.(ai.TextContent); ok {
					fmt.Println(text.Text)
				}
			}
		}
	}
	return nil
}

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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
	cwd, err := os.MkdirTemp("", "pig-grep-example-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(cwd)
	if err := os.WriteFile(filepath.Join(cwd, "target.go"), []byte("package example\nconst Greeting = \"old\"\n"), 0600); err != nil {
		return err
	}
	core, err := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
	if err != nil {
		return err
	}
	search, err := ai.FauxAssistantMessage(ai.FauxAssistantBlocks(ai.ToolCall{Type: "toolCall", ID: "search", Name: "grep", Arguments: map[string]any{"pattern": "Greeting", "glob": "*.go"}}), ai.FauxAssistantMessageOptions{StopReason: ai.Some(ai.StopReasonToolUse)})
	if err != nil {
		return err
	}
	core.SetResponses([]ai.FauxResponseStep{search, ai.FauxResponseFactory(func(input ai.Context, _ *ai.SimpleStreamOptions, _ *ai.FauxProviderState, _ ai.Model) (ai.AssistantMessage, error) {
		result, ok := input.Messages[len(input.Messages)-1].(ai.ToolResultMessage)
		if !ok || result.IsError || len(result.Content) == 0 {
			return ai.AssistantMessage{}, fmt.Errorf("grep failed: %+v", result)
		}
		text, ok := result.Content[0].(ai.TextContent)
		if !ok || !strings.HasPrefix(text.Text, "target.go:2: const Greeting") {
			return ai.AssistantMessage{}, fmt.Errorf("unexpected grep result: %+v", result)
		}
		fmt.Println(text.Text)
		return ai.FauxAssistantMessage(ai.FauxAssistantBlocks(ai.ToolCall{Type: "toolCall", ID: "edit", Name: "edit", Arguments: map[string]any{"path": "target.go", "edits": []any{map[string]any{"oldText": "\"old\"", "newText": "\"你好🙂\""}}}}), ai.FauxAssistantMessageOptions{StopReason: ai.Some(ai.StopReasonToolUse)})
	}), ai.FauxResponseFactory(func(input ai.Context, _ *ai.SimpleStreamOptions, _ *ai.FauxProviderState, _ ai.Model) (ai.AssistantMessage, error) {
		result, ok := input.Messages[len(input.Messages)-1].(ai.ToolResultMessage)
		if !ok || result.IsError {
			return ai.AssistantMessage{}, fmt.Errorf("edit failed: %+v", result)
		}
		return ai.FauxAssistantMessage(ai.FauxAssistantText("已通过 grep 找到定义并完成编辑。"))
	})})
	model, _ := core.GetModel()
	created, err := codingagent.CreateAgentSession(context.Background(), codingagent.CreateAgentSessionOptions{CWD: cwd, Model: &model, Tools: []string{"grep", "edit"}, StreamFunction: agent.StreamFunction(core.StreamSimple)})
	if err != nil {
		return err
	}
	defer created.Session.Dispose()
	if err := created.Session.Prompt(context.Background(), "查找 Greeting 并修改问候语"); err != nil {
		return err
	}
	data, err := os.ReadFile(filepath.Join(cwd, "target.go"))
	if err != nil {
		return err
	}
	if string(data) != "package example\nconst Greeting = \"你好🙂\"\n" {
		return fmt.Errorf("unexpected edited file: %s", data)
	}
	fmt.Printf("grep → ToolResult → edit → continuation 完成：\n%s", data)
	return nil
}

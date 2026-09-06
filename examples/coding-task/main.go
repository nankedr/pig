package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

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
	cwd, err := os.MkdirTemp("", "pig-coding-task-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(cwd)
	dir := filepath.Join(cwd, "sessions")
	manager, err := codingagent.NewSessionManager(cwd, &dir)
	if err != nil {
		return err
	}
	core, err := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
	if err != nil {
		return err
	}
	model, _ := core.GetModel()
	execute := func(manager *codingagent.SessionManager, calls ...ai.ToolCall) error {
		var responses []ai.FauxResponseStep
		offset := len(manager.BuildSessionContext().Messages)
		for i, call := range calls {
			call.Type, call.ID = "toolCall", fmt.Sprintf("%s-%d", manager.GetSessionID(), offset+i)
			message, err := ai.FauxAssistantMessage(ai.FauxAssistantBlocks(call), ai.FauxAssistantMessageOptions{StopReason: ai.Some(ai.StopReasonToolUse)})
			if err != nil {
				return err
			}
			responses = append(responses, message)
		}
		final, err := ai.FauxAssistantMessage(ai.FauxAssistantText("编码任务完成"))
		if err != nil {
			return err
		}
		core.SetResponses(append(responses, final))
		created, err := codingagent.CreateAgentSession(context.Background(), codingagent.CreateAgentSessionOptions{CWD: cwd, Model: &model, SessionManager: manager, StreamFunction: agent.StreamFunction(core.StreamSimple)})
		if err != nil {
			return err
		}
		defer created.Session.Dispose()
		if err := created.Session.Prompt(context.Background(), "继续编码任务"); err != nil {
			return err
		}
		for _, message := range created.Session.Messages() {
			if result, ok := message.(ai.ToolResultMessage); ok && result.IsError {
				return fmt.Errorf("Tool failed: %+v", result.Content)
			}
			if result, ok := message.(ai.AssistantMessage); ok && result.StopReason == ai.StopReasonError {
				return fmt.Errorf("Provider failed: %v", result.ErrorMessage)
			}
		}
		fmt.Printf("Session %s：%d 条消息\n", created.Session.SessionID(), len(created.Session.Messages()))
		return nil
	}
	if err := execute(manager,
		ai.ToolCall{Name: "write", Arguments: map[string]any{"path": "task.txt", "content": "before\n"}},
		ai.ToolCall{Name: "read", Arguments: map[string]any{"path": "task.txt"}},
		ai.ToolCall{Name: "edit", Arguments: map[string]any{"path": "task.txt", "edits": []any{map[string]any{"oldText": "before", "newText": "after"}}}},
		ai.ToolCall{Name: "bash", Arguments: map[string]any{"command": "cat task.txt"}},
	); err != nil {
		return err
	}
	path := *manager.GetSessionFile()
	reopened, err := codingagent.OpenSessionManager(path, nil, nil)
	if err != nil {
		return err
	}
	if err := execute(reopened, ai.ToolCall{Name: "read", Arguments: map[string]any{"path": "task.txt"}}); err != nil {
		return err
	}
	fork, err := codingagent.ForkSessionManager(path, cwd, &dir)
	if err != nil {
		return err
	}
	if err := execute(fork, ai.ToolCall{Name: "bash", Arguments: map[string]any{"command": "printf '%s\n' \"$PIG_SESSION_ID\"; cat task.txt"}}); err != nil {
		return err
	}
	fmt.Println("默认四工具 → 退出并恢复 → fork 完成；演示目录随后清理。")
	return nil
}

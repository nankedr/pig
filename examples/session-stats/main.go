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
	ctx := context.Background()
	dir, err := os.MkdirTemp("", "pig-stats-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	core, err := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
	if err != nil {
		return err
	}
	reply, err := ai.FauxAssistantMessage(ai.FauxAssistantText("会话统计已就绪"))
	if err != nil {
		return err
	}
	core.SetResponses([]ai.FauxResponseStep{reply})
	model, _ := core.GetModel()
	model.ContextWindow = 32000
	manager, err := codingagent.NewSessionManager(dir, &dir)
	if err != nil {
		return err
	}
	created, err := codingagent.CreateAgentSession(ctx, codingagent.CreateAgentSessionOptions{CWD: dir, Model: &model, SessionManager: manager, StreamFunction: agent.StreamFunction(core.StreamSimple), NoTools: codingagent.NoToolsAll})
	if err != nil {
		return err
	}
	defer created.Session.Dispose()
	if err := created.Session.Prompt(ctx, "查询会话用量"); err != nil {
		return err
	}
	if err := printStats(created.Session); err != nil {
		return err
	}
	reopened, err := codingagent.OpenSessionManager(*created.Session.SessionFile(), &dir, &dir)
	if err != nil {
		return err
	}
	restored, err := codingagent.CreateAgentSession(ctx, codingagent.CreateAgentSessionOptions{CWD: dir, Model: &model, SessionManager: reopened, StreamFunction: agent.StreamFunction(core.StreamSimple), NoTools: codingagent.NoToolsAll})
	if err != nil {
		return err
	}
	defer restored.Session.Dispose()
	fmt.Println("重新打开后：")
	return printStats(restored.Session)
}
func printStats(session *codingagent.AgentSession) error {
	stats, err := session.GetSessionStats()
	if err != nil {
		return err
	}
	fmt.Printf("会话=%s 用户=%d Assistant=%d Tool调用=%d Tool结果=%d 历史tokens=%d cost=%.6f\n", stats.SessionID, stats.UserMessages, stats.AssistantMessages, stats.ToolCalls, stats.ToolResults, stats.Tokens.TotalTokens, stats.Cost)
	usage, err := session.GetContextUsage()
	if err != nil {
		return err
	}
	switch {
	case usage == nil:
		fmt.Println("当前模型未提供上下文窗口")
	case usage.Tokens == nil:
		fmt.Println("压缩后当前上下文用量未知")
	default:
		fmt.Printf("当前上下文=%d/%d (%.2f%%)\n", *usage.Tokens, usage.ContextWindow, *usage.Percent)
	}
	last, err := session.GetLastAssistantText()
	if err != nil {
		return err
	}
	if last != nil {
		fmt.Println("最后回复：", *last)
	}
	return nil
}

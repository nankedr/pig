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
	dir, err := os.MkdirTemp("", "pig-turn-retry-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	core, err := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
	if err != nil {
		return err
	}
	failed, _ := ai.FauxAssistantMessage(ai.FauxAssistantText("partial"), ai.FauxAssistantMessageOptions{StopReason: ai.Some(ai.StopReasonError), ErrorMessage: ai.Some("overloaded_error")})
	success, _ := ai.FauxAssistantMessage(ai.FauxAssistantText("恢复成功"))
	core.SetResponses([]ai.FauxResponseStep{failed, success})
	model, _ := core.GetModel()
	enabled, retries, delay := true, 2, int64(1)
	settings, err := codingagent.NewInMemorySettingsManager(codingagent.Settings{Retry: &codingagent.RetrySettings{Enabled: &enabled, MaxRetries: &retries, BaseDelayMS: &delay}})
	if err != nil {
		return err
	}
	created, err := codingagent.CreateAgentSession(context.Background(), codingagent.CreateAgentSessionOptions{CWD: dir, Model: &model, StreamFunction: agent.StreamFunction(core.StreamSimple), SettingsManager: settings, NoTools: codingagent.NoToolsAll})
	if err != nil {
		return err
	}
	defer created.Session.Dispose()
	runtime := codingagent.NewAgentSessionRuntime(created.Session, codingagent.AgentSessionServices{}, nil, nil, nil)
	outcome, err := codingagent.RunHeadless(context.Background(), runtime, codingagent.HeadlessRunOptions{Messages: []string{"请继续"}, OnEvent: func(event codingagent.AgentSessionEvent) {
		switch e := event.(type) {
		case codingagent.AgentSessionAutoRetryStartEvent:
			fmt.Printf("重试 %d/%d，等待 %dms\n", e.Attempt, e.MaxAttempts, e.DelayMS)
		case codingagent.AgentSessionAutoRetryEndEvent:
			fmt.Printf("重试结束，success=%t\n", e.Success)
		}
	}})
	if err != nil {
		return err
	}
	fmt.Printf("%s；模型上下文 %d 条，持久化历史 %d 条\n", outcome.Text[0], len(created.Session.Messages()), len(created.Session.SessionManager().BuildSessionContext().Messages))
	return nil
}

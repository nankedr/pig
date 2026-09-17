package main

import (
	"context"
	"fmt"
	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
	"os"
	"os/signal"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	provider, err := ai.NewFauxProvider()
	if err != nil {
		return err
	}
	first, err := ai.FauxAssistantMessage(ai.FauxAssistantText("你好，这是第一轮离线回复。"))
	if err != nil {
		return err
	}
	second, err := ai.FauxAssistantMessage(ai.FauxAssistantText("已收到第二轮输入。按 Ctrl+D 退出。"))
	if err != nil {
		return err
	}
	provider.SetResponses([]ai.FauxResponseStep{first, second})
	model, _ := provider.GetModel()
	created, err := codingagent.CreateAgentSession(ctx, codingagent.CreateAgentSessionOptions{Model: &model, Provider: provider.Provider, NoTools: codingagent.NoToolsAll})
	if err != nil {
		return err
	}
	runtime := codingagent.NewAgentSessionRuntime(created.Session, codingagent.AgentSessionServices{}, nil, nil, nil)
	return codingagent.NewInteractiveMode(runtime).Run(ctx)
}

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
	dir, err := os.MkdirTemp("", "pig-tree-navigation-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	manager, err := codingagent.NewSessionManager(dir, &dir)
	if err != nil {
		return err
	}
	core, err := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
	if err != nil {
		return err
	}
	model, ok := core.GetModel()
	if !ok {
		return fmt.Errorf("missing Faux model")
	}
	response := ai.FauxResponseFactory(func(c ai.Context, _ *ai.SimpleStreamOptions, _ *ai.FauxProviderState, _ ai.Model) (ai.AssistantMessage, error) {
		fmt.Printf("生成时的上下文消息数：%d\n", len(c.Messages))
		return ai.FauxAssistantMessage(ai.FauxAssistantText("完成"))
	})
	core.SetResponses([]ai.FauxResponseStep{response, response, response, response})
	created, err := codingagent.CreateAgentSession(ctx, codingagent.CreateAgentSessionOptions{CWD: dir, SessionManager: manager, Model: &model, StreamFunction: agent.StreamFunction(core.StreamSimple), Tools: []string{}})
	if err != nil {
		return err
	}
	s := created.Session
	defer s.Dispose()
	if err = s.Prompt(ctx, "设计一个方案"); err != nil {
		return err
	}
	first := *manager.GetLeafID()
	if err = s.Prompt(ctx, "继续原方案"); err != nil {
		return err
	}
	original := *manager.GetLeafID()
	if _, err = s.NavigateTree(ctx, first, codingagent.NavigateTreeOptions{Label: "分叉点"}); err != nil {
		return err
	}
	if err = s.Prompt(ctx, "试试另一个方案"); err != nil {
		return err
	}
	branch := *manager.GetLeafID()
	reopened, err := codingagent.OpenSessionManager(*s.SessionFile(), nil, nil)
	if err != nil {
		return err
	}
	restored, err := codingagent.CreateAgentSession(ctx, codingagent.CreateAgentSessionOptions{CWD: dir, SessionManager: reopened, Model: &model, StreamFunction: agent.StreamFunction(core.StreamSimple), Tools: []string{}})
	if err != nil {
		return err
	}
	defer restored.Session.Dispose()
	fmt.Printf("重开后位于新分支：%t；原分支保留：%t\n", *reopened.GetLeafID() == branch, reopened.GetEntry(original) != nil)
	if _, err = restored.Session.NavigateTree(ctx, original); err != nil {
		return err
	}
	fmt.Printf("同一 Session：%t；标签：%s\n", restored.Session.SessionID() == s.SessionID(), *reopened.GetLabel(first))
	return restored.Session.Prompt(ctx, "回到原方案继续")
}

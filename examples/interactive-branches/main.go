package main

import (
	"context"
	"fmt"
	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
	"os"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	ctx := context.Background()
	dir, err := os.MkdirTemp("", "pig-session-selection-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	provider, err := ai.NewFauxProvider()
	if err != nil {
		return err
	}
	model, _ := provider.GetModel()
	reply, _ := ai.FauxAssistantMessage(ai.FauxAssistantText("saved reply"))
	provider.SetResponses([]ai.FauxResponseStep{reply, reply, reply, reply})
	factory := func(ctx context.Context, o codingagent.CreateAgentSessionRuntimeOptions) (codingagent.CreateAgentSessionRuntimeResult, error) {
		created, err := codingagent.CreateAgentSession(ctx, codingagent.CreateAgentSessionOptions{CWD: o.CWD, AgentDir: dir, Model: &model, Provider: provider.Provider, SessionManager: o.SessionManager, NoTools: codingagent.NoToolsAll})
		return codingagent.CreateAgentSessionRuntimeResult{CreateAgentSessionResult: created, Services: codingagent.AgentSessionServices{CWD: o.CWD, AgentDir: dir}}, err
	}
	manager, err := codingagent.NewSessionManager(dir, &dir)
	if err != nil {
		return err
	}
	runtime, err := codingagent.CreateAgentSessionRuntime(ctx, factory, codingagent.CreateAgentSessionRuntimeOptions{CWD: dir, SessionManager: manager})
	if err != nil {
		return err
	}
	defer runtime.Dispose(ctx)
	if err = runtime.Session().Prompt(ctx, "first question"); err != nil {
		return err
	}
	if err = runtime.Session().Prompt(ctx, "second question"); err != nil {
		return err
	}
	source := *runtime.Session().SessionFile()
	messages, err := runtime.Session().GetUserMessagesForForking()
	if err != nil {
		return err
	}
	selected := ""
	selector := codingagent.NewUserMessageSelectorComponent(messages, func(id string) { selected = id }, nil)
	if err = selector.HandleInput("\r"); err != nil {
		return err
	}
	fork, err := runtime.Fork(ctx, selected)
	if err != nil {
		return err
	}
	fmt.Println("Fork draft:", *fork.SelectedText)
	if err = runtime.Session().Prompt(ctx, *fork.SelectedText+" revised"); err != nil {
		return err
	}
	manager = runtime.Session().SessionManager()
	tree, err := manager.GetTree()
	if err != nil {
		return err
	}
	selected = ""
	browser := codingagent.NewTreeSelectorComponent(tree, manager.GetLeafID(), 24, func(id string) { selected = id }, nil)
	for _, key := range []string{"first question", "\r"} {
		if err = browser.HandleInput(key); err != nil {
			return err
		}
	}
	result, err := runtime.Session().NavigateTree(ctx, selected)
	if err != nil {
		return err
	}
	if err = runtime.Session().Prompt(ctx, *result.EditorText+" from tree"); err != nil {
		return err
	}
	reopened, err := codingagent.OpenSessionManager(*manager.GetSessionFile(), nil, nil)
	if err != nil {
		return err
	}
	original, err := codingagent.OpenSessionManager(source, nil, nil)
	if err != nil {
		return err
	}
	fmt.Printf("Original: %d messages; fork active path: %d messages\n", len(original.BuildSessionContext().Messages), len(reopened.BuildSessionContext().Messages))
	return nil
}

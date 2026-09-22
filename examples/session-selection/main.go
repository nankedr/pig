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
	provider.SetResponses([]ai.FauxResponseStep{reply, reply, reply})
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
	if err = runtime.Session().SetSessionName("example target"); err != nil {
		return err
	}
	if err = runtime.Session().Prompt(ctx, "first question"); err != nil {
		return err
	}
	if _, err = runtime.NewSession(ctx); err != nil {
		return err
	}
	current := func(ctx context.Context) ([]codingagent.SessionInfo, error) {
		return codingagent.ListSessions(ctx, dir, codingagent.SessionListOptions{SessionDir: &dir})
	}
	selected := ""
	selector, err := codingagent.NewSessionSelectorComponent(ctx, current, current, func(path string) { selected = path }, nil)
	if err != nil {
		return err
	}
	for _, key := range []string{"example target", "\r"} {
		if err = selector.HandleInput(key); err != nil {
			return err
		}
	}
	if _, err = runtime.SwitchSession(ctx, selected); err != nil {
		return err
	}
	if err = runtime.Session().Prompt(ctx, "continued question"); err != nil {
		return err
	}
	reopened, err := codingagent.OpenSessionManager(selected, nil, nil)
	if err != nil {
		return err
	}
	fmt.Printf("%s: %d persisted messages\n", *reopened.GetSessionName(), len(reopened.BuildSessionContext().Messages))
	return nil
}

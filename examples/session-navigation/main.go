package main

import (
	"context"
	"fmt"
	"os"

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
	dir, err := os.MkdirTemp("", "pig-session-navigation-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	manager, err := codingagent.NewSessionManager(dir, &dir, codingagent.NewSessionOptions{ID: "example"})
	if err != nil {
		return err
	}
	if _, err = manager.AppendSessionInfo("Session navigation"); err != nil {
		return err
	}
	provider, err := ai.NewFauxProvider()
	if err != nil {
		return err
	}
	model, _ := provider.GetModel()
	reply, err := ai.FauxAssistantMessage(ai.FauxAssistantText("saved reply"))
	if err != nil {
		return err
	}
	provider.SetResponses([]ai.FauxResponseStep{reply, reply})
	factory := func(ctx context.Context, o codingagent.CreateAgentSessionRuntimeOptions) (codingagent.CreateAgentSessionRuntimeResult, error) {
		created, err := codingagent.CreateAgentSession(ctx, codingagent.CreateAgentSessionOptions{CWD: o.CWD, Model: &model, Provider: provider.Provider, SessionManager: o.SessionManager, SessionStartEvent: o.SessionStartEvent})
		return codingagent.CreateAgentSessionRuntimeResult{CreateAgentSessionResult: created, Services: codingagent.AgentSessionServices{CWD: o.CWD}}, err
	}
	ctx := context.Background()
	runtime, err := codingagent.CreateAgentSessionRuntime(ctx, factory, codingagent.CreateAgentSessionRuntimeOptions{CWD: dir, SessionManager: manager})
	if err != nil {
		return err
	}
	defer runtime.Dispose(ctx)
	if err = runtime.Session().Prompt(ctx, "first prompt"); err != nil {
		return err
	}
	sessions, err := codingagent.ListSessions(ctx, dir, codingagent.SessionListOptions{SessionDir: &dir})
	if err != nil {
		return err
	}
	fmt.Printf("sessions: %d, name: %s\n", len(sessions), *sessions[0].Name)
	recent, err := codingagent.ContinueRecentSessionManager(dir, &dir)
	if err != nil {
		return err
	}
	fmt.Printf("continue: %s\n", recent.GetSessionID())
	messages, err := runtime.Session().GetUserMessagesForForking()
	if err != nil {
		return err
	}
	fork, err := runtime.Fork(ctx, messages[0].EntryID)
	if err != nil {
		return err
	}
	fmt.Printf("fork selected: %s\n", *fork.SelectedText)
	if err = runtime.Session().Prompt(ctx, "independent prompt"); err != nil {
		return err
	}
	fmt.Printf("source messages: %d, fork messages: %d\n", len(manager.BuildSessionContext().Messages), len(runtime.Session().Messages()))
	return nil
}

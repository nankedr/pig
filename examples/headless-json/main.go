package main

import (
	"context"
	"fmt"
	"os"

	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
)

func main() {
	ctx := context.Background()
	cwd, err := os.Getwd()
	if err != nil {
		fail(err)
	}
	answer, err := ai.FauxAssistantMessage(ai.FauxAssistantText("Session-first JSONL completed."))
	if err != nil {
		fail(err)
	}
	provider, err := ai.NewFauxProvider()
	if err != nil {
		fail(err)
	}
	provider.SetResponses([]ai.FauxResponseStep{answer})
	model, _ := provider.GetModel()

	created, err := codingagent.CreateAgentSession(ctx, codingagent.CreateAgentSessionOptions{
		CWD:            cwd,
		Model:          &model,
		Provider:       provider.Provider,
		SessionManager: codingagent.NewInMemorySessionManager(cwd),
	})
	if err != nil {
		fail(err)
	}
	runtime := codingagent.NewAgentSessionRuntime(created.Session, codingagent.AgentSessionServices{CWD: cwd}, nil, nil, nil)
	prompt := "Show the JSON event stream."
	exitCode, err := codingagent.RunPrintMode(ctx, runtime, codingagent.PrintModeOptions{
		InitialMessage: &prompt,
		Mode:           codingagent.ModeJSON,
	})
	if err != nil {
		fail(err)
	}
	if exitCode != 0 {
		os.Exit(exitCode)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

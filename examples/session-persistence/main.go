package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
)

func main() {
	ctx := context.Background()
	dir, err := os.MkdirTemp("", "pig-session-example-")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(dir)

	provider, err := ai.NewFauxProvider()
	if err != nil {
		log.Fatal(err)
	}
	first, _ := ai.FauxAssistantMessage(ai.FauxAssistantText("first reply"))
	second, _ := ai.FauxAssistantMessage(ai.FauxAssistantText("second reply"))
	provider.SetResponses([]ai.FauxResponseStep{first, second})
	model, _ := provider.GetModel()

	manager, err := codingagent.NewSessionManager(dir, &dir, codingagent.NewSessionOptions{ID: "example-session"})
	if err != nil {
		log.Fatal(err)
	}
	created, err := codingagent.CreateAgentSession(ctx, codingagent.CreateAgentSessionOptions{CWD: dir, Model: &model, Provider: provider.Provider, SessionManager: manager})
	if err != nil {
		log.Fatal(err)
	}
	if err := created.Session.Prompt(ctx, "first prompt"); err != nil {
		log.Fatal(err)
	}
	path := *created.Session.SessionFile()
	_ = created.Session.Dispose()

	reopened, err := codingagent.OpenSessionManager(path, nil, nil)
	if err != nil {
		log.Fatal(err)
	}
	history := len(reopened.BuildSessionContext().Messages)
	created, err = codingagent.CreateAgentSession(ctx, codingagent.CreateAgentSessionOptions{CWD: dir, Model: &model, Provider: provider.Provider, SessionManager: reopened})
	if err != nil {
		log.Fatal(err)
	}
	defer created.Session.Dispose()
	if err := created.Session.Prompt(ctx, "second prompt"); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Reopened %d messages and continued to %d.\n", history, len(created.Session.Messages()))
}

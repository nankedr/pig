package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
)

func main() {
	ctx := context.Background()
	cwd, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	readTool, err := codingagent.CreateReadTool(cwd)
	if err != nil {
		log.Fatal(err)
	}
	readCall, err := ai.FauxToolCall("read", map[string]any{"path": "go.mod"})
	if err != nil {
		log.Fatal(err)
	}
	readRequest, err := ai.FauxAssistantMessage(
		ai.FauxAssistantBlocks(readCall),
		ai.FauxAssistantMessageOptions{StopReason: ai.Some(ai.StopReasonToolUse)},
	)
	if err != nil {
		log.Fatal(err)
	}
	answer, err := ai.FauxAssistantMessage(ai.FauxAssistantText("Headless read completed."))
	if err != nil {
		log.Fatal(err)
	}
	provider, err := ai.NewFauxProvider()
	if err != nil {
		log.Fatal(err)
	}
	provider.SetResponses([]ai.FauxResponseStep{readRequest, answer})
	model, _ := provider.GetModel()

	created, err := codingagent.CreateAgentSession(ctx, codingagent.CreateAgentSessionOptions{
		CWD:        cwd,
		Model:      &model,
		Provider:   provider.Provider,
		AgentTools: []agent.ErasedAgentTool{readTool},
	})
	if err != nil {
		log.Fatal(err)
	}
	runtime := codingagent.NewAgentSessionRuntime(created.Session, codingagent.AgentSessionServices{CWD: cwd}, nil, nil, nil)
	defer runtime.Dispose(context.Background())

	prompt := "Read go.mod."
	outcome, err := codingagent.RunHeadless(ctx, runtime, codingagent.HeadlessRunOptions{InitialMessage: &prompt})
	if err != nil {
		log.Fatal(err)
	}
	for _, text := range outcome.Text {
		fmt.Println(text)
	}
}

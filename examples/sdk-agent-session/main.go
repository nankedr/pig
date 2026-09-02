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
	toolCall, err := ai.FauxToolCall("read", map[string]any{"path": "go.mod"})
	if err != nil {
		log.Fatal(err)
	}
	readRequest, err := ai.FauxAssistantMessage(
		ai.FauxAssistantBlocks(toolCall),
		ai.FauxAssistantMessageOptions{StopReason: ai.Some(ai.StopReasonToolUse)},
	)
	if err != nil {
		log.Fatal(err)
	}
	answer, err := ai.FauxAssistantMessage(ai.FauxAssistantText("Read go.mod through the public AgentSession SDK."))
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
	defer created.Session.Dispose()

	if err := created.Session.Prompt(ctx, "Read the module file."); err != nil {
		log.Fatal(err)
	}
	messages := created.Session.Messages()
	last := messages[len(messages)-1].(ai.AssistantMessage)
	fmt.Println(last.Content[0].(ai.TextContent).Text)
}

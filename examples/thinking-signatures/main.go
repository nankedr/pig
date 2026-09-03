package main

import (
	"context"
	"fmt"
	"log"

	"github.com/nankedr/pig/ai"
)

func main() {
	thinking := ai.FauxThinking("inspect the request")
	thinking.ThinkingSignature = ai.Some("reasoning_content")
	message, err := ai.FauxAssistantMessage(
		ai.FauxAssistantBlocks(thinking, ai.FauxText("The request is valid.")),
	)
	if err != nil {
		log.Fatal(err)
	}
	provider, err := ai.NewFauxProvider()
	if err != nil {
		log.Fatal(err)
	}
	provider.SetResponses([]ai.FauxResponseStep{message})
	model, _ := provider.GetModel()
	level := ai.ThinkingLevelHigh
	stream := provider.Provider.StreamSimple(context.Background(), model, ai.Context{}, ai.SimpleStreamOptions{Reasoning: &level})
	for {
		event, ok, err := stream.Next(context.Background())
		if err != nil {
			log.Fatal(err)
		}
		if !ok {
			break
		}
		if delta, ok := event.(ai.AssistantMessageThinkingDeltaEvent); ok {
			fmt.Println("thinking delta:", delta.Delta)
		}
	}
	result, err := stream.Result(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	thinking = result.Content[0].(ai.ThinkingContent)
	signature, _ := thinking.ThinkingSignature.Value()
	fmt.Println("thinking signature:", signature)
	fmt.Println("answer:", result.Content[1].(ai.TextContent).Text)
}

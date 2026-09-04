package main

import (
	"context"
	"fmt"
	"log"

	"github.com/nankedr/pig/ai"
)

func main() {
	message, err := ai.FauxAssistantMessage(ai.FauxAssistantText("done"))
	if err != nil {
		log.Fatal(err)
	}
	provider, err := ai.NewFauxProvider(ai.RegisterFauxProviderOptions{
		Models: []ai.FauxModelDefinition{{
			ID: "usage-model", Cost: ai.Some(ai.ModelCostRates{Input: 1, Output: 2, CacheRead: .5, CacheWrite: 1.5}),
		}},
	})
	if err != nil {
		log.Fatal(err)
	}
	provider.SetResponses([]ai.FauxResponseStep{message, message})
	model, _ := provider.GetModel()
	models := ai.CreateModels()
	models.SetProvider(provider.Provider)
	input := ai.Context{
		SystemPrompt: ai.Some("Be concise."),
		Messages:     []ai.Message{ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText("Hello")}},
	}
	session := "example-session"

	first, err := provider.Provider.Stream(context.Background(), model, input, ai.StreamOptions{SessionID: &session}).Result(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	second, err := models.Complete(context.Background(), model, input, ai.ModelsStreamOptions{StreamOptions: ai.StreamOptions{SessionID: &session}})
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("first: input=%d output=%d cacheWrite=%d cost=%.8f\n", first.Usage.Input, first.Usage.Output, first.Usage.CacheWrite, first.Usage.Cost.Total)
	fmt.Printf("repeat: input=%d output=%d cacheRead=%d cost=%.8f\n", second.Usage.Input, second.Usage.Output, second.Usage.CacheRead, second.Usage.Cost.Total)
}

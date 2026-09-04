package main

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/nankedr/pig/ai"
)

func main() {
	provider, err := ai.NewFauxProvider(ai.RegisterFauxProviderOptions{
		Models: []ai.FauxModelDefinition{{ID: "small-window", ContextWindow: ai.Some(100)}},
	})
	if err != nil {
		log.Fatal(err)
	}
	models := ai.CreateModels()
	models.SetProvider(provider.Provider)
	model, _ := provider.GetModel()
	input := ai.Context{Messages: []ai.Message{ai.UserMessage{
		Role: ai.MessageRoleUser, Content: ai.UserText(strings.Repeat("x", 400)), Timestamp: 1,
	}}}
	fmt.Printf("before request: estimated=%d\n", ai.EstimateContextTokens(input).Tokens)

	for _, reason := range []ai.StopReason{ai.StopReasonStop, ai.StopReasonLength, ai.StopReasonError} {
		response, err := ai.FauxAssistantMessage(ai.FauxAssistantText(""), ai.FauxAssistantMessageOptions{
			StopReason: ai.Some(reason), ErrorMessage: ai.Some("prompt is too long"), Timestamp: ai.Some[int64](2),
		})
		if err != nil {
			log.Fatal(err)
		}
		provider.SetResponses([]ai.FauxResponseStep{response})
		message, err := models.Complete(context.Background(), model, input, ai.ModelsStreamOptions{})
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("%s: input=%d overflow=%t recoverableLength=%t\n", reason, message.Usage.Input,
			ai.IsContextOverflow(message, model.ContextWindow), ai.IsRecoverableLength(message, 16))
		if reason == ai.StopReasonStop {
			estimate := ai.EstimateContextTokens(ai.Context{Messages: []ai.Message{input.Messages[0], message,
				ai.UserMessage{Content: ai.UserText("tail"), Timestamp: 3},
			}})
			fmt.Printf("after response: estimated=%d usage=%d trailing=%d\n", estimate.Tokens, estimate.UsageTokens, estimate.TrailingTokens)
		}
	}
}

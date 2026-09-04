package main

import (
	"context"
	"fmt"

	"github.com/nankedr/pig/ai"
)

func main() {
	if err := run(); err != nil {
		panic(err)
	}
}

func run() error {
	registration, err := ai.RegisterFauxProvider(ai.RegisterFauxProviderOptions{API: ai.APIOpenAICompletions})
	if err != nil {
		return err
	}
	defer registration.Unregister()
	reply, err := ai.FauxAssistantMessage(ai.FauxAssistantText("compat ready"))
	if err != nil {
		return err
	}
	registration.SetResponses([]ai.FauxResponseStep{reply, reply})
	model, _ := registration.GetModel()
	message, err := ai.Complete(context.Background(), model, ai.Context{})
	if err != nil {
		return err
	}
	fmt.Println(message.Content[0].(ai.TextContent).Text)
	message, err = ai.StreamOpenAICompletions(context.Background(), model, ai.Context{}, ai.OpenAICompletionsOptions{}).Result(context.Background())
	if err != nil {
		return err
	}
	fmt.Println(message.Content[0].(ai.TextContent).Text)
	resources := map[string]bool{"example": true}
	unregister, err := ai.RegisterSessionResourceCleanup(func(ids ...string) {
		if len(ids) == 0 {
			clear(resources)
		} else {
			delete(resources, ids[0])
		}
	})
	if err != nil {
		return err
	}
	defer unregister()
	if err := ai.CleanupSessionResources("example"); err != nil {
		return err
	}
	fmt.Printf("resources remaining: %d\n", len(resources))
	return nil
}

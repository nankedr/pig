package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
)

func main() {
	provider, err := ai.NewFauxProvider()
	if err != nil {
		log.Fatal(err)
	}
	thinking := ai.FauxThinking("inspect the existing history")
	thinking.ThinkingSignature = ai.Some("reasoning_content")
	response, err := ai.FauxAssistantMessage(ai.FauxAssistantBlocks(thinking, ai.FauxText("Ready to continue.")))
	if err != nil {
		log.Fatal(err)
	}
	provider.SetResponses([]ai.FauxResponseStep{response})
	model, _ := provider.GetModel()
	created, err := agent.NewAgent(agent.AgentOptions{
		InitialState:   &agent.AgentInitialState{Model: model},
		StreamFunction: agent.StreamFunction(provider.Provider.StreamSimple),
	})
	if err != nil {
		log.Fatal(err)
	}
	if err := created.PromptText(context.Background(), "Start here."); err != nil {
		log.Fatal(err)
	}
	created.SetModel(ai.Model{ID: "target", API: ai.APIOpenAICompletions, Provider: ai.ProviderIDOpenAI, BaseURL: "https://example.test/v1", MaxTokens: 1024})
	created.SetStreamFunction(func(ctx context.Context, model ai.Model, input ai.Context, options ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
		key := "offline-example"
		options.APIKey = &key
		options.Fetch = func(_ context.Context, request ai.FetchRequest) (ai.FetchResponse, error) {
			var body struct {
				Messages []json.RawMessage `json:"messages"`
			}
			if err := json.Unmarshal(request.Body, &body); err != nil {
				return ai.FetchResponse{}, err
			}
			for _, message := range body.Messages {
				fmt.Println("replay:", string(message))
			}
			return ai.FetchResponse{Status: 200, Body: []byte("data: {\"choices\":[{\"delta\":{\"content\":\"Continued with the existing history.\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")}, nil
		}
		return ai.StreamSimpleOpenAICompletions(ctx, model, input, options)
	})
	if err := created.PromptText(context.Background(), "Continue with the new model."); err != nil {
		log.Fatal(err)
	}
	state := created.State()
	if state.ErrorMessage != nil {
		log.Fatal(*state.ErrorMessage)
	}
	original := state.Messages[1].(ai.AssistantMessage).Content[0].(ai.ThinkingContent)
	signature, _ := original.ThinkingSignature.Value()
	fmt.Println("original thinking signature:", signature)
	fmt.Println("answer:", state.Messages[len(state.Messages)-1].(ai.AssistantMessage).Content[0].(ai.TextContent).Text)
}

package main

import (
	"context"
	"fmt"
	"log"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	model := ai.Model{ID: "proxy-model", API: ai.APIOpenAICompletions, Provider: "controlled"}
	created, err := agent.NewAgent(agent.AgentOptions{
		InitialState: &agent.AgentInitialState{Model: model},
		StreamFunction: func(ctx context.Context, model ai.Model, input ai.Context, _ ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
			return agent.StreamProxy(ctx, model, input, agent.ProxyStreamOptions{
				ProxyURL: "https://proxy.example.test", AuthToken: "example-token",
				Fetch: func(context.Context, ai.FetchRequest) (ai.FetchResponse, error) {
					return ai.FetchResponse{Status: 200, Body: []byte(
						"data: {\"type\":\"start\"}\n\n" +
							"data: {\"type\":\"text_start\",\"contentIndex\":0}\n\n" +
							"data: {\"type\":\"text_delta\",\"contentIndex\":0,\"delta\":\"hello from proxy\"}\n\n" +
							"data: {\"type\":\"text_end\",\"contentIndex\":0,\"contentSignature\":\"signed\"}\n\n" +
							"data: {\"type\":\"done\",\"reason\":\"stop\",\"usage\":{\"input\":3,\"output\":2,\"cacheRead\":0,\"cacheWrite\":0,\"totalTokens\":5,\"cost\":{\"input\":0,\"output\":0,\"cacheRead\":0,\"cacheWrite\":0,\"total\":0}}}\n\n",
					)}, nil
				},
			}).AssistantMessageEventStream()
		},
	})
	if err != nil {
		return err
	}
	if err := created.PromptText(context.Background(), "hello"); err != nil {
		return err
	}
	state := created.State()
	message := state.Messages[len(state.Messages)-1].(ai.AssistantMessage)
	if message.StopReason != ai.StopReasonStop {
		return fmt.Errorf("proxy stopped with %s", message.StopReason)
	}
	text := message.Content[0].(ai.TextContent)
	signature, _ := text.TextSignature.Value()
	fmt.Printf("%s; signature=%s tokens=%d idle=%t\n", text.Text, signature, message.Usage.TotalTokens, !created.Busy())
	return nil
}

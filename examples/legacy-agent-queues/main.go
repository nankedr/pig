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
	core, err := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
	if err != nil {
		return err
	}
	reply, err := ai.FauxAssistantMessage(ai.FauxAssistantText("done"))
	if err != nil {
		return err
	}
	core.SetResponses([]ai.FauxResponseStep{reply, reply, reply})
	model, _ := core.GetModel()
	created, err := agent.NewAgent(agent.AgentOptions{
		InitialState:   &agent.AgentInitialState{Model: model},
		StreamFunction: agent.StreamFunction(core.StreamSimple),
	})
	if err != nil {
		return err
	}
	user := func(text string) ai.UserMessage {
		return ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText(text), Timestamp: 1}
	}
	queued := false
	created.Subscribe(func(_ context.Context, event agent.AgentEvent) error {
		if event.AgentEventType() == agent.AgentEventTypeMessageUpdate && !queued {
			queued = true
			if err := created.Steer(user("focus on the queue")); err != nil {
				return err
			}
			return created.FollowUp(user("then summarize"))
		}
		if event, ok := event.(agent.MessageEndEvent); ok {
			switch message := event.Message.(type) {
			case ai.UserMessage:
				text, _ := message.Content.Text()
				fmt.Println("user:", text)
			case ai.AssistantMessage:
				fmt.Println("assistant:", message.Content[0].(ai.TextContent).Text)
			}
		}
		return nil
	})
	if err := created.Prompt(context.Background(), user("explain the agent")); err != nil {
		return err
	}
	if err := created.WaitForIdle(context.Background()); err != nil {
		return err
	}
	fmt.Printf("idle=%t queued=%t\n", !created.Busy(), created.HasQueuedMessages())
	return nil
}

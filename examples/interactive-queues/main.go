package main

import (
	"context"
	"fmt"
	"os"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	dir, err := os.MkdirTemp("", "pig-queues-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	core, err := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
	if err != nil {
		return err
	}
	reply, _ := ai.FauxAssistantMessage(ai.FauxAssistantText("done"))
	core.SetResponses([]ai.FauxResponseStep{reply, reply, reply})
	model, _ := core.GetModel()
	created, err := codingagent.CreateAgentSession(context.Background(), codingagent.CreateAgentSessionOptions{CWD: dir, AgentDir: dir, Model: &model, StreamFunction: agent.StreamFunction(core.StreamSimple), NoTools: codingagent.NoToolsAll})
	if err != nil {
		return err
	}
	session := created.Session
	defer session.Dispose()
	queued := false
	var listenerErr error
	_, err = session.Subscribe(func(event codingagent.AgentSessionEvent) {
		if event.AgentSessionEventType() == codingagent.AgentSessionEventTypeAgentStart && !queued {
			queued = true
			if listenerErr = session.FollowUp("then summarize"); listenerErr != nil {
				return
			}
			if listenerErr = session.Steer("focus on cancellation"); listenerErr != nil {
				return
			}
			steering, followUp, e := session.TakeQueuedMessages()
			if e != nil {
				listenerErr = e
				return
			}
			fmt.Printf("restored steering=%q follow-up=%q\n", steering, followUp)
			if listenerErr = session.Steer(steering[0] + " and partial output"); listenerErr != nil {
				return
			}
			listenerErr = session.FollowUp(followUp[0])
		}
		if e, ok := event.(codingagent.AgentSessionMessageStartEvent); ok && e.Message.MessageRole() == ai.MessageRoleUser {
			m := e.Message.(ai.UserMessage)
			blocks, _ := m.Content.Blocks()
			for _, block := range blocks {
				if text, ok := block.(ai.TextContent); ok {
					fmt.Println("user:", text.Text)
				}
			}
		}
	})
	if err != nil {
		return err
	}
	if err = session.Prompt(context.Background(), "explain the queues"); err != nil {
		return err
	}
	if listenerErr != nil {
		return listenerErr
	}
	count, err := session.PendingMessageCount()
	fmt.Printf("idle=%t pending=%d\n", session.IsIdle(), count)
	return err
}

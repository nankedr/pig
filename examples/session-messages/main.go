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
	core, err := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
	if err != nil {
		return err
	}
	reply, err := ai.FauxAssistantMessage(ai.FauxAssistantText("收到"))
	if err != nil {
		return err
	}
	core.SetResponses([]ai.FauxResponseStep{reply, reply, reply})
	model, _ := core.GetModel()
	entered, release := make(chan struct{}), make(chan struct{})
	first := true
	created, err := codingagent.CreateAgentSession(context.Background(), codingagent.CreateAgentSessionOptions{Model: &model, NoTools: codingagent.NoToolsAll, StreamFunction: func(ctx context.Context, m ai.Model, input ai.Context, o ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
		if first {
			first = false
			close(entered)
			<-release
		}
		return core.StreamSimple(ctx, m, input, o)
	}})
	if err != nil {
		return err
	}
	session := created.Session
	defer session.Dispose()
	_, err = session.Subscribe(func(e codingagent.AgentSessionEvent) {
		if e, ok := e.(codingagent.AgentSessionQueueUpdateEvent); ok {
			fmt.Printf("队列：steer=%v followUp=%v\n", e.Steering, e.FollowUp)
		}
	})
	if err != nil {
		return err
	}
	if err = session.SetSteeringMode(agent.QueueAll); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() {
		done <- session.SendUserMessage(ai.UserBlocks(ai.TextContent{Type: ai.ContentTypeText, Text: "开始"}, ai.TextContent{Type: ai.ContentTypeText, Text: "这是一条含两个文本块的消息"}))
	}()
	<-entered
	err = session.Steer("补充当前任务的要求")
	if err == nil {
		err = session.SendUserMessage(ai.UserText("接着总结结果"), codingagent.SendUserMessageOptions{DeliverAs: codingagent.UserMessageDeliveryFollowUp})
	}
	close(release)
	runErr := <-done
	if err != nil {
		return err
	}
	if runErr != nil {
		return runErr
	}
	pending, err := session.PendingMessageCount()
	if err != nil {
		return err
	}
	fmt.Printf("已结束：消息=%d，待处理=%d，持久化=%v\n", len(session.Messages()), pending, session.SessionFile() != nil)
	return nil
}

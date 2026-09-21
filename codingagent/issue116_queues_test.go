package codingagent_test

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
)

func TestInteractiveQueueTakeConsumptionRace116(t *testing.T) {
	for _, mode := range []agent.QueueMode{agent.QueueAll, agent.QueueOneAtATime} {
		t.Run(string(mode), func(t *testing.T) {
			for attempt := 0; attempt < 20; attempt++ {
				core, err := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
				if err != nil {
					t.Fatal(err)
				}
				reply, _ := ai.FauxAssistantMessage(ai.FauxAssistantText("done"))
				core.SetResponses([]ai.FauxResponseStep{reply, reply, reply, reply})
				model, _ := core.GetModel()
				started, release := make(chan struct{}), make(chan struct{})
				var first sync.Once
				created, err := codingagent.CreateAgentSession(context.Background(), codingagent.CreateAgentSessionOptions{CWD: t.TempDir(), AgentDir: t.TempDir(), Model: &model, NoTools: codingagent.NoToolsAll, StreamFunction: func(ctx context.Context, m ai.Model, input ai.Context, options ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
					first.Do(func() {
						close(started)
						select {
						case <-release:
						case <-ctx.Done():
						}
					})
					return core.StreamSimple(ctx, m, input, options)
				}})
				if err != nil {
					t.Fatal(err)
				}
				session := created.Session
				if err := session.SetSteeringMode(mode); err != nil {
					t.Fatal(err)
				}
				if err := session.SetFollowUpMode(mode); err != nil {
					t.Fatal(err)
				}
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				done := make(chan error, 1)
				go func() { done <- session.Prompt(ctx, "first") }()
				<-started
				for _, call := range []func() error{func() error { return session.Steer("s1") }, func() error { return session.Steer("s2") }, func() error { return session.FollowUp("f1") }} {
					if err := call(); err != nil {
						t.Fatal(err)
					}
				}
				close(release)
				steering, follow, err := session.TakeQueuedMessages()
				if err != nil {
					t.Fatal(err)
				}
				if err := <-done; err != nil {
					t.Fatal(err)
				}
				if err := session.WaitForIdle(ctx); err != nil {
					t.Fatal(err)
				}
				counts := map[string]int{}
				for _, text := range append(steering, follow...) {
					counts[text]++
				}
				for _, message := range session.Messages() {
					if message.MessageRole() == ai.MessageRoleUser {
						counts[message85Text(message)]++
					}
				}
				if !reflect.DeepEqual(counts, map[string]int{"first": 1, "s1": 1, "s2": 1, "f1": 1}) {
					t.Fatalf("messages duplicated/lost: %v", counts)
				}
				if n, _ := session.PendingMessageCount(); n != 0 {
					t.Fatalf("stale displayed queue: %d", n)
				}
				if err := session.Steer("idle"); err == nil {
					t.Fatal("idle accepted")
				}
				cancel()
				session.Dispose()
			}
		})
	}
}

func TestInteractiveQueueTakeFromQueueListener116(t *testing.T) {
	core, _ := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
	reply, _ := ai.FauxAssistantMessage(ai.FauxAssistantText("done"))
	core.SetResponses([]ai.FauxResponseStep{reply})
	model, _ := core.GetModel()
	created, err := codingagent.CreateAgentSession(context.Background(), codingagent.CreateAgentSessionOptions{CWD: t.TempDir(), AgentDir: t.TempDir(), Model: &model, StreamFunction: agent.StreamFunction(core.StreamSimple), NoTools: codingagent.NoToolsAll})
	if err != nil {
		t.Fatal(err)
	}
	session := created.Session
	defer session.Dispose()
	var events []string
	_, err = session.Subscribe(func(event codingagent.AgentSessionEvent) {
		switch e := event.(type) {
		case codingagent.AgentSessionAgentStartEvent:
			if err := session.Steer("restore"); err != nil {
				t.Error(err)
			}
		case codingagent.AgentSessionQueueUpdateEvent:
			events = append(events, fmt.Sprint(e.Steering))
			if len(e.Steering) > 0 {
				steering, follow, err := session.TakeQueuedMessages()
				if err != nil || !reflect.DeepEqual(steering, []string{"restore"}) || len(follow) != 0 {
					t.Errorf("take: %v %v %v", steering, follow, err)
				}
			}
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Prompt(context.Background(), "first"); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(events, []string{"[restore]", "[]"}) {
		t.Fatal(events)
	}
}

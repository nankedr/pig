package agent_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
)

func TestLegacyAgentQueueModes(t *testing.T) {
	for _, steering := range []agent.QueueMode{agent.QueueOneAtATime, agent.QueueAll} {
		for _, follow := range []agent.QueueMode{agent.QueueOneAtATime, agent.QueueAll} {
			t.Run(string(steering)+"/"+string(follow), func(t *testing.T) {
				core, err := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
				if err != nil {
					t.Fatal(err)
				}
				response, err := ai.FauxAssistantMessage(ai.FauxAssistantText("reply"))
				if err != nil {
					t.Fatal(err)
				}
				core.SetResponses([]ai.FauxResponseStep{response, response, response, response, response})
				var created *agent.Agent
				var requests [][]string
				created, err = agent.NewAgent(agent.AgentOptions{
					SteeringMode: steering, FollowUpMode: follow,
					StreamFunction: func(ctx context.Context, model ai.Model, input ai.Context, options ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
						var users []string
						for i := len(input.Messages) - 1; i >= 0; i-- {
							message, ok := input.Messages[i].(ai.UserMessage)
							if !ok {
								break
							}
							text, _ := message.Content.Text()
							users = append([]string{text}, users...)
						}
						requests = append(requests, users)
						if len(requests) == 1 {
							for _, text := range []string{"s1", "s2"} {
								if err := created.Steer(userMessage(text)); err != nil {
									t.Error(err)
								}
							}
							for _, text := range []string{"f1", "f2"} {
								if err := created.FollowUp(userMessage(text)); err != nil {
									t.Error(err)
								}
							}
						}
						return core.StreamSimple(ctx, model, input, options)
					},
				})
				if err != nil {
					t.Fatal(err)
				}
				if err := created.Prompt(context.Background(), userMessage("prompt")); err != nil {
					t.Fatal(err)
				}
				want := [][]string{{"prompt"}}
				if steering == agent.QueueAll {
					want = append(want, []string{"s1", "s2"})
				} else {
					want = append(want, []string{"s1"}, []string{"s2"})
				}
				if follow == agent.QueueAll {
					want = append(want, []string{"f1", "f2"})
				} else {
					want = append(want, []string{"f1"}, []string{"f2"})
				}
				if !reflect.DeepEqual(requests, want) {
					t.Fatalf("requests = %v, want %v", requests, want)
				}
				if created.HasQueuedMessages() || created.Busy() {
					t.Fatal("completed run retained queued work or stayed busy")
				}
			})
		}
	}
}

func TestLegacyAgentQueueStateTransitions(t *testing.T) {
	core, err := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	response, err := ai.FauxAssistantMessage(ai.FauxAssistantText("partial response"))
	if err != nil {
		t.Fatal(err)
	}
	core.SetResponses([]ai.FauxResponseStep{response, response, response, response})
	requests := 0
	producerDone := make(chan struct{})
	created, err := agent.NewAgent(agent.AgentOptions{StreamFunction: func(ctx context.Context, model ai.Model, input ai.Context, options ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
		requests++
		if requests != 1 {
			return core.StreamSimple(ctx, model, input, options)
		}
		stream := ai.NewAssistantMessageEventStream()
		stream.Push(ai.AssistantMessageStartEvent{Type: ai.AssistantMessageEventTypeStart, Partial: response})
		stream.Push(ai.AssistantMessageTextDeltaEvent{Type: ai.AssistantMessageEventTypeTextDelta, ContentIndex: 0, Delta: "partial response", Partial: response})
		go func() {
			defer close(producerDone)
			<-ctx.Done()
			aborted := ai.CloneAssistantMessage(response)
			aborted.StopReason = ai.StopReasonAborted
			aborted.ErrorMessage = ai.Some("Request was aborted")
			stream.Push(ai.AssistantMessageErrorEvent{Type: ai.AssistantMessageEventTypeError, Reason: ai.StopReasonAborted, Error: aborted})
		}()
		return stream
	}})
	if err != nil {
		t.Fatal(err)
	}
	for name, call := range map[string]func() error{
		"steer":     func() error { return created.Steer(userMessage("illegal")) },
		"follow-up": func() error { return created.FollowUp(userMessage("illegal")) },
		"continue":  func() error { return created.Continue(context.Background()) },
	} {
		if err := call(); err == nil {
			t.Fatalf("idle %s succeeded", name)
		}
		if created.HasQueuedMessages() || len(created.State().Messages) != 0 {
			t.Fatalf("idle %s mutated state", name)
		}
	}
	injected := false
	created.Subscribe(func(_ context.Context, event agent.AgentEvent) error {
		if event.AgentEventType() != agent.AgentEventTypeMessageUpdate || injected {
			return nil
		}
		injected = true
		if err := created.Steer(userMessage("s1")); err != nil {
			return err
		}
		if err := created.Steer(userMessage("s2")); err != nil {
			return err
		}
		if err := created.FollowUp(userMessage("f1")); err != nil {
			return err
		}
		for name, call := range map[string]func() error{
			"prompt":   func() error { return created.PromptText(context.Background(), "illegal") },
			"continue": func() error { return created.Continue(context.Background()) },
			"reset":    created.Reset,
		} {
			if err := call(); err == nil {
				t.Errorf("busy %s succeeded", name)
			}
		}
		created.Abort()
		return nil
	})
	if err := created.Prompt(context.Background(), userMessage("prompt")); err != nil {
		t.Fatal(err)
	}
	<-producerDone
	state := created.State()
	if len(state.Messages) != 2 {
		t.Fatalf("abort transcript = %v", state.Messages)
	}
	partial := state.Messages[1].(ai.AssistantMessage)
	if partial.StopReason != ai.StopReasonAborted || len(partial.Content) == 0 || !created.HasQueuedMessages() {
		t.Fatalf("abort lost partial outcome or queues: %+v", state)
	}
	if err := created.Continue(context.Background()); err != nil {
		t.Fatal(err)
	}
	var users []string
	for _, message := range created.State().Messages {
		if message, ok := message.(ai.UserMessage); ok {
			text, _ := message.Content.Text()
			users = append(users, text)
		}
	}
	if !reflect.DeepEqual(users, []string{"prompt", "s1", "s2", "f1"}) || created.HasQueuedMessages() {
		t.Fatalf("continued transcript users = %v", users)
	}
	if err := created.Continue(context.Background()); err == nil {
		t.Fatal("continued assistant tail without pending work")
	}
	if err := created.Reset(); err != nil {
		t.Fatal(err)
	}
	if state := created.State(); len(state.Messages) != 0 || state.ErrorMessage != nil || state.IsStreaming || created.HasQueuedMessages() {
		t.Fatalf("reset state = %+v", state)
	}
}

func TestLegacyAgentClearQueuesAndStopBoundary(t *testing.T) {
	for _, clearQueue := range []string{"steering", "follow-up", "all", "none"} {
		t.Run(clearQueue, func(t *testing.T) {
			reply, err := ai.FauxAssistantMessage(ai.FauxAssistantText("reply"))
			if err != nil {
				t.Fatal(err)
			}
			core, err := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
			if err != nil {
				t.Fatal(err)
			}
			core.SetResponses([]ai.FauxResponseStep{reply, reply})
			created, err := agent.NewAgent(agent.AgentOptions{
				InitialState:        &agent.AgentInitialState{SystemPrompt: "system", Model: ai.Model{ID: "configured"}},
				StreamFunction:      agent.StreamFunction(core.StreamSimple),
				ShouldStopAfterTurn: func(context.Context, agent.ShouldStopAfterTurnContext) (bool, error) { return true, nil },
			})
			if err != nil {
				t.Fatal(err)
			}
			unsubscribe := created.Subscribe(func(_ context.Context, event agent.AgentEvent) error {
				if event.AgentEventType() != agent.AgentEventTypeTurnEnd {
					return nil
				}
				if err := created.Steer(userMessage("steering")); err != nil {
					return err
				}
				if err := created.FollowUp(userMessage("follow-up")); err != nil {
					return err
				}
				if err := created.SetSteeringMode(agent.QueueAll); err != nil {
					return err
				}
				if err := created.SetFollowUpMode(agent.QueueAll); err != nil {
					return err
				}
				if err := created.SetSteeringMode("invalid"); err == nil {
					t.Error("invalid mode succeeded")
				}
				if err := created.SetFollowUpMode("invalid"); err == nil {
					t.Error("invalid mode succeeded")
				}
				switch clearQueue {
				case "steering":
					created.ClearSteeringQueue()
				case "follow-up":
					created.ClearFollowUpQueue()
				case "all":
					created.ClearAllQueues()
				}
				return nil
			})
			if err := created.Prompt(context.Background(), userMessage("prompt")); err != nil {
				t.Fatal(err)
			}
			unsubscribe()
			if len(created.State().Messages) != 2 {
				t.Fatal("stop boundary consumed queue")
			}
			if clearQueue == "all" {
				if created.HasQueuedMessages() {
					t.Fatal("clear all retained messages")
				}
			} else if clearQueue != "none" {
				if err := created.Continue(context.Background()); err != nil {
					t.Fatal(err)
				}
				message := created.State().Messages[2].(ai.UserMessage)
				text, _ := message.Content.Text()
				if text == clearQueue || created.HasQueuedMessages() {
					t.Fatalf("cleared wrong queue: %q", text)
				}
			} else if !created.HasQueuedMessages() {
				t.Fatal("stop discarded queues")
			}
			if err := created.Reset(); err != nil {
				t.Fatal(err)
			}
			state := created.State()
			if created.HasQueuedMessages() || len(state.Messages) != 0 || state.ErrorMessage != nil || state.StreamingMessage != nil || len(state.PendingToolCalls) != 0 {
				t.Fatalf("reset left state: %+v", state)
			}
			if state.SystemPrompt != "system" || state.Model.ID != "configured" || created.SteeringMode() != agent.QueueAll || created.FollowUpMode() != agent.QueueAll {
				t.Fatal("reset changed configuration")
			}
		})
	}
}

func TestLegacyAgentFollowUpWaitsForToolsAndSteering(t *testing.T) {
	var order []string
	tool := erasedTestTool(t, "lookup", "", func(context.Context, string, map[string]any, agent.AgentToolUpdateCallback[map[string]any]) (agent.AgentToolResult[map[string]any], error) {
		order = append(order, "tool")
		return agent.AgentToolResult[map[string]any]{Content: []ai.ToolResultContent{ai.TextContent{Type: ai.ContentTypeText, Text: "result"}}}, nil
	})
	call, err := ai.FauxToolCall("lookup", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	toolReply, err := ai.FauxAssistantMessage(ai.FauxAssistantBlocks(call), ai.FauxAssistantMessageOptions{StopReason: ai.Some(ai.StopReasonToolUse)})
	if err != nil {
		t.Fatal(err)
	}
	reply, err := ai.FauxAssistantMessage(ai.FauxAssistantText("reply"))
	if err != nil {
		t.Fatal(err)
	}
	core, err := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	core.SetResponses([]ai.FauxResponseStep{toolReply, reply, reply})
	created, err := agent.NewAgent(agent.AgentOptions{InitialState: &agent.AgentInitialState{Tools: []agent.ErasedAgentTool{tool}}, StreamFunction: agent.StreamFunction(core.StreamSimple)})
	if err != nil {
		t.Fatal(err)
	}
	queued := false
	created.Subscribe(func(_ context.Context, event agent.AgentEvent) error {
		switch event := event.(type) {
		case agent.MessageEndEvent:
			if message, ok := event.Message.(ai.UserMessage); ok {
				text, _ := message.Content.Text()
				order = append(order, text)
			}
			if message, ok := event.Message.(ai.AssistantMessage); ok && !queued {
				queued = true
				if message.StopReason != ai.StopReasonToolUse {
					t.Error("expected tool response")
				}
				if err := created.FollowUp(userMessage("follow-up")); err != nil {
					return err
				}
				return created.Steer(userMessage("steering"))
			}
		case agent.TurnEndEvent:
			order = append(order, "turn-end")
		}
		return nil
	})
	if err := created.Prompt(context.Background(), userMessage("prompt")); err != nil {
		t.Fatal(err)
	}
	want := []string{"prompt", "tool", "turn-end", "steering", "turn-end", "follow-up", "turn-end"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
}

func TestLegacyAgentWaitsForAllEndListenersAfterError(t *testing.T) {
	reply, err := ai.FauxAssistantMessage(ai.FauxAssistantText("reply"))
	if err != nil {
		t.Fatal(err)
	}
	created := newFauxAgent(t, ai.RegisterFauxProviderOptions{}, reply)
	failure := errors.New("listener failed")
	var order []string
	created.Subscribe(func(_ context.Context, event agent.AgentEvent) error {
		if event.AgentEventType() == agent.AgentEventTypeAgentEnd {
			order = append(order, "first")
			return failure
		}
		return nil
	})
	reached, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	t.Cleanup(func() { once.Do(func() { close(release) }) })
	created.Subscribe(func(_ context.Context, event agent.AgentEvent) error {
		if event.AgentEventType() == agent.AgentEventTypeAgentEnd {
			order = append(order, "second-start")
			close(reached)
			<-release
			order = append(order, "second-end")
		}
		return nil
	})
	done := make(chan error, 1)
	go func() { done <- created.Prompt(context.Background(), userMessage("prompt")) }()
	select {
	case <-reached:
	case err := <-done:
		t.Fatalf("end listeners were skipped: %v", err)
	case <-time.After(time.Second):
		t.Fatal("listener not reached")
	}
	if !created.Busy() {
		t.Fatal("agent idle during end listener")
	}
	if err := created.Steer(userMessage("late")); err == nil {
		t.Fatal("accepted steering after run ended")
	}
	if err := created.FollowUp(userMessage("late")); err == nil {
		t.Fatal("accepted follow-up after run ended")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := created.WaitForIdle(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("wait error = %v", err)
	}
	once.Do(func() { close(release) })
	if err := created.WaitForIdle(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := <-done; !errors.Is(err, failure) {
		t.Fatalf("prompt error = %v", err)
	}
	if !reflect.DeepEqual(order, []string{"first", "second-start", "second-end"}) {
		t.Fatalf("listener order = %v", order)
	}
}

func TestLegacyAgentConcurrentDeliveryAndWaiters(t *testing.T) {
	reply, err := ai.FauxAssistantMessage(ai.FauxAssistantText("reply"))
	if err != nil {
		t.Fatal(err)
	}
	core, err := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	core.SetResponses([]ai.FauxResponseStep{reply, reply, reply})
	reached, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	t.Cleanup(func() { once.Do(func() { close(release) }) })
	calls := 0
	created, err := agent.NewAgent(agent.AgentOptions{SteeringMode: agent.QueueAll, FollowUpMode: agent.QueueAll,
		StreamFunction: func(ctx context.Context, model ai.Model, input ai.Context, options ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
			calls++
			if calls == 1 {
				close(reached)
				<-release
			}
			return core.StreamSimple(ctx, model, input, options)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- created.Prompt(context.Background(), userMessage("prompt")) }()
	<-reached
	var deliveries sync.WaitGroup
	for i := 0; i < 8; i++ {
		deliveries.Add(1)
		go func(i int) {
			defer deliveries.Done()
			for j := 0; j < 4; j++ {
				if err := created.Steer(userMessage(fmt.Sprintf("s-%d-%d", i, j))); err != nil {
					t.Error(err)
				}
				if err := created.FollowUp(userMessage(fmt.Sprintf("f-%d-%d", i, j))); err != nil {
					t.Error(err)
				}
				_ = created.State()
			}
		}(i)
	}
	deliveries.Wait()
	var waiters sync.WaitGroup
	for i := 0; i < 8; i++ {
		waiters.Add(1)
		go func() {
			defer waiters.Done()
			if err := created.WaitForIdle(context.Background()); err != nil {
				t.Error(err)
			}
		}()
	}
	once.Do(func() { close(release) })
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	waiters.Wait()
	var users []string
	for _, message := range created.State().Messages {
		if message, ok := message.(ai.UserMessage); ok {
			text, _ := message.Content.Text()
			users = append(users, text)
		}
	}
	if calls != 3 || len(users) != 65 || created.HasQueuedMessages() {
		t.Fatalf("calls=%d users=%d queued=%v", calls, len(users), created.HasQueuedMessages())
	}
	for i := 0; i < 8; i++ {
		for _, prefix := range []string{"s", "f"} {
			next := 0
			for _, text := range users {
				if text == fmt.Sprintf("%s-%d-%d", prefix, i, next) {
					next++
				}
			}
			if next != 4 {
				t.Fatalf("%s producer %d lost FIFO: %v", prefix, i, users)
			}
		}
	}
}

func TestLegacyAgentCancellationInStopHookSettlesListeners(t *testing.T) {
	reply, err := ai.FauxAssistantMessage(ai.FauxAssistantText("partial outcome"))
	if err != nil {
		t.Fatal(err)
	}
	core, err := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	core.SetResponses([]ai.FauxResponseStep{reply})
	var created *agent.Agent
	created, err = agent.NewAgent(agent.AgentOptions{
		StreamFunction: agent.StreamFunction(core.StreamSimple),
		ShouldStopAfterTurn: func(context.Context, agent.ShouldStopAfterTurnContext) (bool, error) {
			created.Abort()
			return false, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	ended := 0
	created.Subscribe(func(_ context.Context, event agent.AgentEvent) error {
		if event.AgentEventType() == agent.AgentEventTypeTurnEnd {
			return created.FollowUp(userMessage("retained"))
		}
		if event.AgentEventType() == agent.AgentEventTypeAgentEnd {
			ended++
		}
		return nil
	})
	if err := created.Prompt(context.Background(), userMessage("prompt")); !errors.Is(err, context.Canceled) {
		t.Fatalf("prompt error = %v", err)
	}
	if err := created.WaitForIdle(context.Background()); err != nil {
		t.Fatal(err)
	}
	if ended != 1 || created.Busy() || !created.HasQueuedMessages() || len(created.State().Messages) != 2 {
		t.Fatalf("canceled hook: end=%d busy=%v queued=%v state=%+v", ended, created.Busy(), created.HasQueuedMessages(), created.State())
	}
}

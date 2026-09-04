package parity_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/internal/parity"
)

func TestLegacyAgentQueuesParity(t *testing.T) {
	fixture, locked := loadCompatFixture(t, "legacy-agent-queues")
	oracle, err := parity.NewFixtureDriver(fixture, locked)
	if err != nil {
		t.Fatal(err)
	}
	result, err := parity.RunCase(context.Background(), fixture.Case, oracle, parity.DriverFunc{SurfaceName: parity.SurfaceGoSDK, ObserveFunc: observeLegacyAgentQueues})
	if err != nil || !result.Match || len(result.Normalizations) != 0 {
		t.Fatalf("queue parity = %+v, %v", result, err)
	}
}

func observeLegacyAgentQueues(ctx context.Context, declaration parity.Case) (parity.Observation, error) {
	var input struct {
		Modes    []agent.QueueMode `json:"modes"`
		Resume   []bool            `json:"resume"`
		Prompt   string            `json:"prompt"`
		Steering []string          `json:"steering"`
		FollowUp []string          `json:"follow_up"`
		Reply    string            `json:"reply"`
	}
	if err := json.Unmarshal(declaration.Input, &input); err != nil {
		return parity.Observation{}, err
	}
	user := func(text string) ai.UserMessage {
		return ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText(text), Timestamp: 1}
	}
	outcomes := []map[string]any{}
	for _, steering := range input.Modes {
		for _, follow := range input.Modes {
			for _, resume := range input.Resume {
				size := 100
				core, err := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{TokenSize: &ai.FauxTokenSize{Min: &size, Max: &size}})
				if err != nil {
					return parity.Observation{}, err
				}
				reply, err := ai.FauxAssistantMessage(ai.FauxAssistantText(input.Reply), ai.FauxAssistantMessageOptions{Timestamp: ai.Some[int64](2)})
				if err != nil {
					return parity.Observation{}, err
				}
				core.SetResponses([]ai.FauxResponseStep{reply, reply, reply, reply, reply})
				requests, events, listeners := [][]string{}, []string{}, []string{}
				var created *agent.Agent
				var deliveryError error
				created, err = agent.NewAgent(agent.AgentOptions{
					SteeringMode: steering, FollowUpMode: follow,
					ShouldStopAfterTurn: func(context.Context, agent.ShouldStopAfterTurnContext) (bool, error) {
						return resume && len(requests) == 1, nil
					},
					StreamFunction: func(ctx context.Context, model ai.Model, in ai.Context, options ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
						users := []string{}
						for i := len(in.Messages) - 1; i >= 0; i-- {
							message, ok := in.Messages[i].(ai.UserMessage)
							if !ok {
								break
							}
							text, _ := message.Content.Text()
							users = append([]string{text}, users...)
						}
						requests = append(requests, users)
						if len(requests) == 1 {
							for _, text := range input.Steering {
								deliveryError = errors.Join(deliveryError, created.Steer(user(text)))
							}
							for _, text := range input.FollowUp {
								deliveryError = errors.Join(deliveryError, created.FollowUp(user(text)))
							}
						}
						return core.StreamSimple(ctx, model, in, options)
					},
				})
				if err != nil {
					return parity.Observation{}, err
				}
				created.Subscribe(func(_ context.Context, event agent.AgentEvent) error {
					switch event := event.(type) {
					case agent.AgentStartEvent, agent.TurnStartEvent, agent.TurnEndEvent, agent.AgentEndEvent:
						events = append(events, string(event.AgentEventType()))
					case agent.MessageEndEvent:
						if message, ok := event.Message.(ai.UserMessage); ok {
							text, _ := message.Content.Text()
							events = append(events, "user:"+text)
						} else {
							events = append(events, "assistant")
						}
					}
					if event.AgentEventType() == agent.AgentEventTypeAgentEnd {
						listeners = append(listeners, "first-start", "first-end")
					}
					return nil
				})
				created.Subscribe(func(_ context.Context, event agent.AgentEvent) error {
					if event.AgentEventType() == agent.AgentEventTypeAgentEnd {
						listeners = append(listeners, "second")
					}
					return nil
				})
				if err := created.Prompt(ctx, user(input.Prompt)); err != nil {
					return parity.Observation{}, err
				}
				if deliveryError != nil {
					return parity.Observation{}, deliveryError
				}
				if resume {
					if err := created.Continue(ctx); err != nil {
						return parity.Observation{}, err
					}
				}
				if err := created.WaitForIdle(ctx); err != nil {
					return parity.Observation{}, err
				}
				outcomes = append(outcomes, map[string]any{"steeringMode": steering, "followUpMode": follow, "resume": resume, "requests": requests, "events": events, "listeners": listeners, "queued": created.HasQueuedMessages(), "streaming": created.State().IsStreaming})
			}
		}
	}
	outcome, err := json.Marshal(outcomes)
	sideEffects := []parity.SideEffect{}
	return parity.Observation{Outcome: outcome, SideEffects: &sideEffects}, err
}

func TestLegacyAgentQueueAdmissionDeviation(t *testing.T) {
	fixture, _ := loadCompatFixture(t, "legacy-agent-queues-deviation")
	var upstream struct {
		IdleAccepted, LateQueued bool
		ErrorListeners           []string
	}
	if err := json.Unmarshal(fixture.Observation.Outcome, &upstream); err != nil {
		t.Fatal(err)
	}
	if !upstream.IdleAccepted || !upstream.LateQueued || !reflect.DeepEqual(upstream.ErrorListeners, []string{"first", "first"}) {
		t.Fatalf("Pi deviation = %+v", upstream)
	}
	core, err := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	reply, err := ai.FauxAssistantMessage(ai.FauxAssistantText("reply"))
	if err != nil {
		t.Fatal(err)
	}
	core.SetResponses([]ai.FauxResponseStep{reply})
	created, err := agent.NewAgent(agent.AgentOptions{StreamFunction: agent.StreamFunction(core.StreamSimple)})
	if err != nil {
		t.Fatal(err)
	}
	user := ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText("queued"), Timestamp: 1}
	if created.Steer(user) == nil || created.FollowUp(user) == nil || created.HasQueuedMessages() {
		t.Fatal("Pig accepted idle queueing")
	}
	order := []string{}
	failure := errors.New("listener failed")
	created.Subscribe(func(_ context.Context, event agent.AgentEvent) error {
		if event.AgentEventType() != agent.AgentEventTypeAgentEnd {
			return nil
		}
		if created.Steer(user) == nil || created.FollowUp(user) == nil {
			t.Error("Pig accepted late queueing")
		}
		order = append(order, "first")
		return failure
	})
	created.Subscribe(func(_ context.Context, event agent.AgentEvent) error {
		if event.AgentEventType() == agent.AgentEventTypeAgentEnd {
			order = append(order, "second")
		}
		return nil
	})
	if err := created.Prompt(context.Background(), user); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(order, []string{"first", "second"}) || created.HasQueuedMessages() || created.Busy() {
		t.Fatalf("Pig deviation = %v", order)
	}
}

package codingagent_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"sync"
	"testing"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
	"github.com/nankedr/pig/internal/baseline"
	"github.com/nankedr/pig/internal/parity"
)

func message85Text(m agent.AgentMessage) string {
	if u, ok := m.(ai.UserMessage); ok {
		if text, ok := u.Content.Text(); ok {
			return text
		}
		blocks, _ := u.Content.Blocks()
		text := ""
		for _, b := range blocks {
			if b, ok := b.(ai.TextContent); ok {
				text += b.Text
			}
		}
		return text
	}
	if a, ok := m.(ai.AssistantMessage); ok {
		text := ""
		for _, b := range a.Content {
			if b, ok := b.(ai.TextContent); ok {
				text += b.Text
			}
		}
		return text
	}
	return ""
}
func message85History(messages []agent.AgentMessage) []string {
	out := []string{}
	for _, m := range messages {
		out = append(out, string(m.MessageRole())+":"+message85Text(m))
	}
	return out
}
func TestSessionMessagesParity(t *testing.T) {
	root, _ := filepath.Abs("..")
	lock, _, err := baseline.Load(filepath.Join(root, "parity/baseline"))
	if err != nil {
		t.Fatal(err)
	}
	pinned := parity.Baseline{ID: lock.BaselineID, Commit: lock.Upstream.Commit, Repository: lock.Upstream.Repository}
	fixture, err := parity.LoadFixture(filepath.Join(root, "parity/oracle/fixtures/session-messages.json"), pinned)
	if err != nil {
		t.Fatal(err)
	}
	oracle, err := parity.NewFixtureDriver(fixture, pinned)
	if err != nil {
		t.Fatal(err)
	}
	result, err := parity.RunCase(context.Background(), fixture.Case, oracle, parity.DriverFunc{SurfaceName: parity.SurfaceGoSDK, ObserveFunc: func(ctx context.Context, c parity.Case) (parity.Observation, error) {
		var input struct {
			Modes              []agent.QueueMode
			Prompt             ai.UserMessageContent
			Steering, FollowUp []string
			Reply              string
		}
		if err := json.Unmarshal(c.Input, &input); err != nil {
			return parity.Observation{}, err
		}
		outcomes := []map[string]any{}
		for _, steering := range input.Modes {
			for _, follow := range input.Modes {
				dir := t.TempDir()
				manager, err := codingagent.NewSessionManager(dir, &dir)
				if err != nil {
					return parity.Observation{}, err
				}
				core, err := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
				if err != nil {
					return parity.Observation{}, err
				}
				reply, _ := ai.FauxAssistantMessage(ai.FauxAssistantText(input.Reply), ai.FauxAssistantMessageOptions{Timestamp: ai.Some(int64(2))})
				core.SetResponses([]ai.FauxResponseStep{reply, reply, reply, reply, reply, reply})
				model, _ := core.GetModel()
				requests, events := [][]string{}, []string{}
				queues := []map[string][]string{}
				rejected := false
				var session *codingagent.AgentSession
				created, err := codingagent.CreateAgentSession(ctx, codingagent.CreateAgentSessionOptions{CWD: dir, AgentDir: dir, SessionManager: manager, Model: &model, NoTools: codingagent.NoToolsAll, StreamFunction: func(ctx context.Context, m ai.Model, input ai.Context, options ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
					messages := []agent.AgentMessage{}
					for _, m := range input.Messages {
						messages = append(messages, m)
					}
					requests = append(requests, message85History(messages))
					if len(requests) == 1 {
						for _, call := range []func() error{
							func() error { return session.Steer("cleared") }, func() error { return session.FollowUp("cleared") }, session.ClearQueue,
							func() error { return session.Steer("s1") },
							func() error {
								return session.SendUserMessage(ai.UserText("s2"), codingagent.SendUserMessageOptions{DeliverAs: codingagent.UserMessageDeliverySteer})
							},
							func() error { return session.FollowUp("f1") },
							func() error {
								return session.Prompt(ctx, "f2", codingagent.PromptOptions{StreamingBehavior: "followUp"})
							},
						} {
							if err := call(); err != nil {
								t.Error(err)
							}
						}
						rejected = session.SendUserMessage(ai.UserText("rejected")) != nil
					}
					return core.StreamSimple(ctx, m, input, options)
				}})
				if err != nil {
					return parity.Observation{}, err
				}
				session = created.Session
				defer session.Dispose()
				if err := session.SetSteeringMode(steering); err != nil {
					return parity.Observation{}, err
				}
				if err := session.SetFollowUpMode(follow); err != nil {
					return parity.Observation{}, err
				}
				_, err = session.Subscribe(func(e codingagent.AgentSessionEvent) {
					switch e := e.(type) {
					case codingagent.AgentSessionQueueUpdateEvent:
						queues = append(queues, map[string][]string{"steering": e.Steering, "followUp": e.FollowUp})
						events = append(events, fmt.Sprintf("queue:%d:%d", len(e.Steering), len(e.FollowUp)))
					case codingagent.AgentSessionMessageStartEvent:
						if e.Message.MessageRole() == ai.MessageRoleUser {
							events = append(events, "start:"+message85Text(e.Message))
						}
					case codingagent.AgentSessionAgentSettledEvent:
						events = append(events, "settled")
					}
				})
				if err != nil {
					return parity.Observation{}, err
				}
				if err := session.SendUserMessage(input.Prompt, codingagent.SendUserMessageOptions{DeliverAs: codingagent.UserMessageDeliveryFollowUp}); err != nil {
					return parity.Observation{}, err
				}
				reopened, err := codingagent.OpenSessionManager(*session.SessionFile(), nil, nil)
				if err != nil {
					return parity.Observation{}, err
				}
				restored := reopened.BuildSessionContext()
				pending, err := session.PendingMessageCount()
				if err != nil {
					return parity.Observation{}, err
				}
				sm, _ := session.SettingsManager().GetSteeringMode()
				fm, _ := session.SettingsManager().GetFollowUpMode()
				outcomes = append(outcomes, map[string]any{"steeringMode": steering, "followUpMode": follow, "requests": requests, "queues": queues, "events": events, "history": message85History(session.Messages()), "reopened": message85History(restored.Messages), "rejected": rejected, "pending": pending, "settings": []agent.QueueMode{sm, fm}})
			}
		}
		data, err := json.Marshal(outcomes)
		return parity.Observation{Outcome: data, SideEffects: &[]parity.SideEffect{}}, err
	}})
	if err != nil || !result.Match {
		t.Fatalf("Session message parity: %+v; Pig=%s; err=%v", result.Differences, result.Pig.Outcome, err)
	}
}

func newMessage85Session(t *testing.T, stream func(context.Context, ai.Model, ai.Context, ai.SimpleStreamOptions, *ai.FauxCore) *ai.AssistantMessageEventStream) *codingagent.AgentSession {
	t.Helper()
	core, err := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	reply, _ := ai.FauxAssistantMessage(ai.FauxAssistantText("reply"))
	steps := make([]ai.FauxResponseStep, 100)
	for i := range steps {
		steps[i] = reply
	}
	core.SetResponses(steps)
	model, _ := core.GetModel()
	dir := t.TempDir()
	manager, err := codingagent.NewSessionManager(dir, &dir)
	if err != nil {
		t.Fatal(err)
	}
	created, err := codingagent.CreateAgentSession(context.Background(), codingagent.CreateAgentSessionOptions{CWD: dir, AgentDir: dir, SessionManager: manager, Model: &model, NoTools: codingagent.NoToolsAll, StreamFunction: func(ctx context.Context, m ai.Model, input ai.Context, options ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
		if stream != nil {
			return stream(ctx, m, input, options, core)
		}
		return core.StreamSimple(ctx, m, input, options)
	}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { created.Session.Dispose() })
	return created.Session
}

func TestSessionMessagesCancelRetainsOnlyUnconsumedMessages(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	calls := 0
	session := newMessage85Session(t, func(ctx context.Context, m ai.Model, input ai.Context, options ai.SimpleStreamOptions, core *ai.FauxCore) *ai.AssistantMessageEventStream {
		calls++
		if calls == 1 {
			close(entered)
			<-release
		}
		return core.StreamSimple(ctx, m, input, options)
	})
	for _, call := range []func() error{func() error { return session.Steer("idle") }, func() error { return session.FollowUp("idle") }} {
		if call() == nil {
			t.Fatal("idle queue accepted")
		}
	}
	done := make(chan error, 1)
	go func() { done <- session.SendUserMessage(ai.UserText("start")) }()
	<-entered
	for _, text := range []string{"same", "", "same"} {
		if err := session.Steer(text); err != nil {
			t.Fatal(err)
		}
	}
	if err := session.FollowUp("same"); err != nil {
		t.Fatal(err)
	}
	if err := session.Abort(); err != nil {
		t.Fatal(err)
	}
	for _, call := range []func() error{func() error { return session.Steer("cancelled") }, func() error { return session.FollowUp("cancelled") }, func() error {
		return session.SendUserMessage(ai.UserText("cancelled"), codingagent.SendUserMessageOptions{DeliverAs: codingagent.UserMessageDeliverySteer})
	}} {
		if call() == nil {
			t.Error("cancelled queue accepted")
		}
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if n, _ := session.PendingMessageCount(); n != 4 {
		t.Fatalf("pending=%d", n)
	}
	if got, _ := session.GetSteeringMessages(); !reflect.DeepEqual(got, []string{"same", "", "same"}) {
		t.Fatal(got)
	}
	if err := session.SendUserMessage(ai.UserText("same")); err != nil {
		t.Fatal(err)
	}
	if n, _ := session.PendingMessageCount(); n != 0 {
		t.Fatalf("consumed queue still pending: %d", n)
	}
	users := []string{}
	for _, m := range session.Messages() {
		if m.MessageRole() == ai.MessageRoleUser {
			users = append(users, message85Text(m))
		}
	}
	if !reflect.DeepEqual(users, []string{"start", "same", "same", "", "same", "same"}) {
		t.Fatal(users)
	}
	reopened, err := codingagent.OpenSessionManager(*session.SessionFile(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := message85History(reopened.BuildSessionContext().Messages); !reflect.DeepEqual(got, message85History(session.Messages())) {
		t.Fatalf("reopened=%v", got)
	}
}

func TestSessionMessagesLastTurnAdmissionAndConcurrentDelivery(t *testing.T) {
	for _, cancelRun := range []bool{false, true} {
		t.Run(fmt.Sprint(cancelRun), func(t *testing.T) {
			session := newMessage85Session(t, nil)
			atEnd, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			_, _ = session.Subscribe(func(event codingagent.AgentSessionEvent) {
				if event.AgentSessionEventType() == codingagent.AgentSessionEventTypeTurnEnd {
					once.Do(func() { close(atEnd); <-release })
				}
				if event.AgentSessionEventType() == codingagent.AgentSessionEventTypeAgentEnd || event.AgentSessionEventType() == codingagent.AgentSessionEventTypeAgentSettled {
					for _, call := range []func() error{func() error { return session.Steer("late") }, func() error { return session.FollowUp("late") }, func() error {
						return session.SendUserMessage(ai.UserText("late"), codingagent.SendUserMessageOptions{DeliverAs: codingagent.UserMessageDeliveryFollowUp})
					}} {
						if call() == nil {
							t.Error("settling delivery accepted")
						}
					}
				}
			})
			done := make(chan error, 1)
			go func() { done <- session.Prompt(context.Background(), "start") }()
			<-atEnd
			var wg sync.WaitGroup
			var mu sync.Mutex
			accepted := map[string]bool{}
			start := make(chan struct{})
			for i := 0; i < 32; i++ {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					<-start
					text := fmt.Sprintf("message-%d", i)
					var err error
					if i%2 == 0 {
						err = session.Steer(text)
					} else {
						err = session.SendUserMessage(ai.UserText(text), codingagent.SendUserMessageOptions{DeliverAs: codingagent.UserMessageDeliveryFollowUp})
					}
					if err == nil {
						mu.Lock()
						accepted[text] = true
						mu.Unlock()
					}
					_, _ = session.GetSteeringMessages()
					_, _ = session.GetFollowUpMessages()
					_, _ = session.PendingMessageCount()
				}(i)
			}
			close(start)
			if cancelRun {
				session.Abort()
			}
			close(release)
			wg.Wait()
			if err := <-done; err != nil && !(cancelRun && errors.Is(err, context.Canceled)) {
				t.Fatal(err)
			}
			seen := map[string]int{}
			for _, m := range session.Messages() {
				if m.MessageRole() == ai.MessageRoleUser {
					seen[message85Text(m)]++
				}
			}
			pendingSteer, _ := session.GetSteeringMessages()
			pendingFollow, _ := session.GetFollowUpMessages()
			for _, text := range append(pendingSteer, pendingFollow...) {
				seen[text]++
			}
			delete(seen, "start")
			if len(seen) != len(accepted) {
				t.Fatalf("seen=%v accepted=%v", seen, accepted)
			}
			for text := range accepted {
				if seen[text] != 1 {
					t.Errorf("accepted %q observed %d times", text, seen[text])
				}
			}
			if !cancelRun {
				if n, _ := session.PendingMessageCount(); n != 0 {
					t.Fatalf("normal finish stranded %d", n)
				}
			}
			if err := session.ClearQueue(); err != nil {
				t.Fatal(err)
			}
			if session.Agent().HasQueuedMessages() {
				t.Fatal("clear did not reach Legacy Agent")
			}
		})
	}
}

func TestSessionMessagesQueueListenersAndCapabilityBounds(t *testing.T) {
	var session *codingagent.AgentSession
	first := true
	session = newMessage85Session(t, func(ctx context.Context, m ai.Model, input ai.Context, o ai.SimpleStreamOptions, core *ai.FauxCore) *ai.AssistantMessageEventStream {
		if first {
			first = false
			if err := session.Steer("clear-me"); err != nil {
				t.Error(err)
			}
		}
		return core.StreamSimple(ctx, m, input, o)
	})
	observed := []string{}
	_, _ = session.Subscribe(func(e codingagent.AgentSessionEvent) {
		if e, ok := e.(codingagent.AgentSessionQueueUpdateEvent); ok {
			snapshot, _ := session.GetSteeringMessages()
			if len(snapshot) > 0 {
				snapshot[0] = "mutated"
			}
			if len(e.Steering) > 0 && e.Steering[0] == "clear-me" {
				session.ClearQueue()
				session.FollowUp("keep")
				e.Steering[0] = "mutated"
			}
		}
	})
	_, _ = session.Subscribe(func(e codingagent.AgentSessionEvent) {
		if e, ok := e.(codingagent.AgentSessionQueueUpdateEvent); ok {
			observed = append(observed, fmt.Sprintf("%v/%v", e.Steering, e.FollowUp))
		}
	})
	before := session.Messages()
	for _, item := range []struct {
		call func() error
		op   string
	}{
		{func() error {
			return session.SendUserMessage(ai.UserBlocks(ai.ImageContent{Type: ai.ContentTypeImage, Data: "aGk=", MIMEType: "image/png"}))
		}, "AgentSession.SendUserMessage.Images"},
		{func() error {
			return session.Prompt(context.Background(), "template", codingagent.PromptOptions{ExpandPromptTemplates: pointerTo(true)})
		}, "AgentSession.Prompt.ExpandPromptTemplates"},
	} {
		assertSessionOperationNotImplemented(t, item.call(), item.op)
	}
	for _, call := range []func() error{func() error { return session.SetSteeringMode("invalid") }, func() error { return session.SetFollowUpMode("invalid") }, func() error {
		return session.SendUserMessage(ai.UserText("invalid"), codingagent.SendUserMessageOptions{DeliverAs: "invalid"})
	}, func() error {
		return session.Prompt(context.Background(), "invalid", codingagent.PromptOptions{StreamingBehavior: "invalid"})
	}} {
		if call() == nil {
			t.Error("invalid option accepted")
		}
	}
	if !reflect.DeepEqual(before, session.Messages()) || len(observed) != 0 {
		t.Fatal("unsupported inputs mutated session")
	}
	if err := session.SendUserMessage(ai.UserText("/literal")); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(observed, []string{"[clear-me]/[]", "[]/[]", "[]/[keep]", "[]/[]"}) {
		t.Fatal(observed)
	}
	users := []string{}
	for _, m := range session.Messages() {
		if m.MessageRole() == ai.MessageRoleUser {
			users = append(users, message85Text(m))
		}
	}
	if !reflect.DeepEqual(users, []string{"/literal", "keep"}) {
		t.Fatal(users)
	}
	session.Dispose()
	if session.Steer("disposed") == nil || session.FollowUp("disposed") == nil || session.SendUserMessage(ai.UserText("disposed")) == nil || session.ClearQueue() == nil || session.SetSteeringMode(agent.QueueAll) == nil {
		t.Fatal("disposed mutation accepted")
	}
}

func TestSessionMessagesAdmissionDeviation(t *testing.T) {
	root, _ := filepath.Abs("..")
	lock, _, err := baseline.Load(filepath.Join(root, "parity/baseline"))
	if err != nil {
		t.Fatal(err)
	}
	fixture, err := parity.LoadFixture(filepath.Join(root, "parity/oracle/fixtures/session-messages-deviation.json"), parity.Baseline{ID: lock.BaselineID, Commit: lock.Upstream.Commit, Repository: lock.Upstream.Repository})
	if err != nil {
		t.Fatal(err)
	}
	var observations []struct {
		Phase    string
		Accepted []string
	}
	if err := json.Unmarshal(fixture.Observation.Outcome, &observations); err != nil {
		t.Fatal(err)
	}
	phases := []string{}
	for _, o := range observations {
		phases = append(phases, o.Phase)
		if !reflect.DeepEqual(o.Accepted, []string{"steer", "followUp"}) {
			t.Fatalf("unexpected Pi admission: %+v", o)
		}
	}
	if !reflect.DeepEqual(phases, []string{"idle", "cancelled", "agent_end", "agent_settled"}) {
		t.Fatal(phases)
	}
	// The Go rejection and preserved-state assertions run through the SDK in the lifecycle tests above.
}

func TestSessionMessagesQueueModePersistence(t *testing.T) {
	dir := t.TempDir()
	settings, err := codingagent.NewSettingsManager(dir, &dir)
	if err != nil {
		t.Fatal(err)
	}
	core, err := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	model, _ := core.GetModel()
	created, err := codingagent.CreateAgentSession(context.Background(), codingagent.CreateAgentSessionOptions{CWD: dir, AgentDir: dir, SettingsManager: settings, Model: &model, StreamFunction: agent.StreamFunction(core.StreamSimple), NoTools: codingagent.NoToolsAll})
	if err != nil {
		t.Fatal(err)
	}
	defer created.Session.Dispose()
	if err := created.Session.SetSteeringMode(agent.QueueAll); err != nil {
		t.Fatal(err)
	}
	if err := created.Session.SetFollowUpMode(agent.QueueAll); err != nil {
		t.Fatal(err)
	}
	if err := created.Session.SetFollowUpMode("invalid"); err == nil {
		t.Fatal("invalid mode accepted")
	}
	reopened, err := codingagent.NewSettingsManager(dir, &dir)
	if err != nil {
		t.Fatal(err)
	}
	steering, err := reopened.GetSteeringMode()
	if err != nil {
		t.Fatal(err)
	}
	follow, err := reopened.GetFollowUpMode()
	if err != nil {
		t.Fatal(err)
	}
	if steering != agent.QueueAll || follow != agent.QueueAll || created.Session.Agent().SteeringMode() != steering || created.Session.Agent().FollowUpMode() != follow {
		t.Fatal("settings and Legacy Agent modes diverged")
	}
}

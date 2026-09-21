//go:build darwin || linux

package codingagent_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
	"github.com/nankedr/pig/internal/terminaltest"
	"github.com/nankedr/pig/tui"
)

func TestInteractiveSDKSettlementCancel116(t *testing.T) {
	runtime := interactiveRuntime(t)
	reached, release := make(chan struct{}), make(chan struct{})
	restored := make(chan struct{}, 1)
	var first, unblock sync.Once
	_, err := runtime.Session().Subscribe(func(event codingagent.AgentSessionEvent) {
		if event.AgentSessionEventType() == codingagent.AgentSessionEventTypeQueueUpdate {
			select {
			case restored <- struct{}{}:
			default:
			}
		}
		if event.AgentSessionEventType() == codingagent.AgentSessionEventTypeAgentSettled {
			first.Do(func() { close(reached); <-release })
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	tty := terminaltest.Open(t)
	terminal := tui.NewProcessTerminal(tty.Slave, tty.Slave)
	mode := codingagent.NewInteractiveMode(runtime, codingagent.InteractiveModeOptions{Terminal: terminal})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	defer unblock.Do(func() { close(release) })
	done := make(chan error, 1)
	go func() { done <- mode.Run(ctx) }()
	tty.Wait(t, "> ")
	tty.Send(t, "first question\r")
	select {
	case <-reached:
	case <-ctx.Done():
		t.Fatal("settlement not reached")
	}
	tty.Send(t, "held question\r")
	tty.Wait(t, "Waiting for current turn to settle")
	tty.Send(t, "\x1b[27u")
	select {
	case <-restored:
	case <-ctx.Done():
		t.Fatal("interrupt not handled")
	}
	unblock.Do(func() { close(release) })
	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(tty.ScreenText(), "> held question") && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if !strings.Contains(tty.ScreenText(), "> held question") {
		t.Fatalf("rejected text not restored: %s", tty.ScreenText())
	}
	if err := runtime.Session().WaitForIdle(ctx); err != nil {
		t.Fatal(err)
	}
	tty.Send(t, "\r")
	tty.Wait(t, "SECOND_DONE")
	if err := runtime.Session().WaitForIdle(ctx); err != nil {
		t.Fatal(err)
	}
	tty.Send(t, "\x04")
	if err := awaitInteractive(t, done); err != nil {
		t.Fatal(err)
	}
	var users []string
	for _, message := range runtime.Session().SessionManager().BuildSessionContext().Messages {
		if message.MessageRole() == ai.MessageRoleUser {
			users = append(users, message85Text(message))
		}
	}
	if strings.Join(users, "|") != "first question|held question" {
		t.Fatalf("duplicate or lost submission: %v", users)
	}
	select {
	case <-terminal.Done():
	default:
		t.Fatal("terminal reader survived exit")
	}
	if !tty.Restored(t) || !runtime.Session().IsIdle() {
		t.Fatal("runtime not cleaned up")
	}
}

type inputTerminal116 struct {
	tui.Terminal
	input func(string)
}

func (t *inputTerminal116) Start(input func(string), resize func()) error {
	t.input = input
	return t.Terminal.Start(input, resize)
}
func TestInteractiveSDKLateDequeue116(t *testing.T) {
	for attempt := 0; attempt < 10; attempt++ {
		t.Run(fmt.Sprint(attempt), func(t *testing.T) {
			core, _ := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
			reply, _ := ai.FauxAssistantMessage(ai.FauxAssistantText("done"))
			core.SetResponses([]ai.FauxResponseStep{reply, reply})
			model, _ := core.GetModel()
			secondRequest := make(chan struct{})
			requests := 0
			var session *codingagent.AgentSession
			created, err := codingagent.CreateAgentSession(context.Background(), codingagent.CreateAgentSessionOptions{CWD: t.TempDir(), AgentDir: t.TempDir(), Model: &model, NoTools: codingagent.NoToolsAll, StreamFunction: func(ctx context.Context, m ai.Model, input ai.Context, options ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
				requests++
				if requests == 2 {
					if err := session.Steer("new-turn"); err != nil {
						t.Error(err)
					}
					close(secondRequest)
					<-ctx.Done()
				}
				return core.StreamSimple(ctx, m, input, options)
			}})
			if err != nil {
				t.Fatal(err)
			}
			session = created.Session
			runtime := codingagent.NewAgentSessionRuntime(created.Session, codingagent.AgentSessionServices{}, nil, nil, nil)
			firstStarted, releaseFirst, barrier := make(chan struct{}), make(chan struct{}), make(chan struct{}, 1)
			var release sync.Once
			starts := 0
			_, err = runtime.Session().Subscribe(func(event codingagent.AgentSessionEvent) {
				switch e := event.(type) {
				case codingagent.AgentSessionAgentStartEvent:
					starts++
					if starts == 1 {
						close(firstStarted)
						<-releaseFirst
					}
				case codingagent.AgentSessionQueueUpdateEvent:
					if len(e.FollowUp) > 0 && e.FollowUp[0] == "barrier" {
						select {
						case barrier <- struct{}{}:
						default:
						}
					}
				}
			})
			if err != nil {
				t.Fatal(err)
			}
			tty := terminaltest.Open(t)
			terminal := &inputTerminal116{Terminal: tui.NewProcessTerminal(tty.Slave, tty.Slave)}
			mode := codingagent.NewInteractiveMode(runtime, codingagent.InteractiveModeOptions{Terminal: terminal, InitialMessages: []string{"first", "second"}})
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			done := make(chan error, 1)
			go func() { done <- mode.Run(ctx) }()
			defer func() { cancel(); release.Do(func() { close(releaseFirst) }); _ = mode.Stop() }()
			select {
			case <-firstStarted:
			case <-ctx.Done():
				t.Fatal("first start blocked")
			}
			terminal.input("\x1b[1;3A")
			release.Do(func() { close(releaseFirst) })
			select {
			case <-secondRequest:
			case <-ctx.Done():
				t.Fatal("second request blocked")
			}
			terminal.input("barrier")
			terminal.input("\x1b\r")
			select {
			case <-barrier:
			case <-ctx.Done():
				t.Fatal("input barrier blocked")
			}
			steering, err := runtime.Session().GetSteeringMessages()
			if err != nil || !reflect.DeepEqual(steering, []string{"new-turn"}) {
				t.Fatalf("old dequeue changed new queue: %v %v", steering, err)
			}
			cancel()
			if err := awaitInteractive(t, done); !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
		})
	}
}

func TestInteractiveSDKSettlingFollowUp116(t *testing.T) {
	runtime := interactiveRuntime(t)
	reached, release := make(chan struct{}), make(chan struct{})
	var first, unblock sync.Once
	_, err := runtime.Session().Subscribe(func(event codingagent.AgentSessionEvent) {
		if event.AgentSessionEventType() == codingagent.AgentSessionEventTypeAgentSettled {
			first.Do(func() { close(reached); <-release })
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	tty := terminaltest.Open(t)
	terminal := tui.NewProcessTerminal(tty.Slave, tty.Slave)
	mode := codingagent.NewInteractiveMode(runtime, codingagent.InteractiveModeOptions{Terminal: terminal})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	defer unblock.Do(func() { close(release) })
	done := make(chan error, 1)
	go func() { done <- mode.Run(ctx) }()
	tty.Wait(t, "> ")
	tty.Send(t, "first question\r")
	select {
	case <-reached:
	case <-ctx.Done():
		t.Fatal("settlement not reached")
	}
	tty.Send(t, "/quit\x1b\r")
	tty.Wait(t, "Waiting for current turn to settle")
	unblock.Do(func() { close(release) })
	tty.Wait(t, "SECOND_DONE")
	if err := runtime.Session().WaitForIdle(ctx); err != nil {
		t.Fatal(err)
	}
	tty.Send(t, "\x04")
	if err := awaitInteractive(t, done); err != nil {
		t.Fatal(err)
	}
	var users []string
	for _, message := range runtime.Session().SessionManager().BuildSessionContext().Messages {
		if message.MessageRole() == ai.MessageRoleUser {
			users = append(users, message85Text(message))
		}
	}
	if strings.Join(users, "|") != "first question|/quit" {
		t.Fatalf("duplicate or lost submission: %v", users)
	}
	select {
	case <-terminal.Done():
	default:
		t.Fatal("terminal reader survived exit")
	}
	if !tty.Restored(t) || !runtime.Session().IsIdle() {
		t.Fatal("runtime not cleaned up")
	}
}

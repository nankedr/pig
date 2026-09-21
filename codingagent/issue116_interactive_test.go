//go:build darwin || linux

package codingagent_test

import (
	"context"
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

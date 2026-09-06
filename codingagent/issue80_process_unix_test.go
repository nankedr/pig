//go:build darwin || linux

package codingagent_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
)

func TestBashToolSessionAbortKillsProcessTree(t *testing.T) { testBashToolProcessTree(t, true) }
func TestBashToolTimeoutKillsProcessTree(t *testing.T)      { testBashToolProcessTree(t, false) }
func testBashToolProcessTree(t *testing.T, abort bool) {
	t.Helper()
	args := map[string]any{"command": `sleep 30 & child=$!; printf '%s %s\n' $$ $child; wait`}
	if !abort {
		args["timeout"] = 0.2
	}
	core, err := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	response, err := ai.FauxAssistantMessage(ai.FauxAssistantBlocks(ai.ToolCall{Type: "toolCall", ID: "tree", Name: "bash", Arguments: args}), ai.FauxAssistantMessageOptions{StopReason: ai.Some(ai.StopReasonToolUse)})
	if err != nil {
		t.Fatal(err)
	}
	final, _ := ai.FauxAssistantMessage(ai.FauxAssistantText("done"))
	core.SetResponses([]ai.FauxResponseStep{response, final})
	model, _ := core.GetModel()
	cwd := t.TempDir()
	tool, err := codingagent.CreateBashTool(cwd)
	if err != nil {
		t.Fatal(err)
	}
	created, err := codingagent.CreateAgentSession(context.Background(), codingagent.CreateAgentSessionOptions{CWD: cwd, Model: &model, AgentTools: []agent.ErasedAgentTool{tool}, StreamFunction: agent.StreamFunction(core.StreamSimple)})
	if err != nil {
		t.Fatal(err)
	}
	defer created.Session.Dispose()
	var shell, child int
	var events []string
	_, err = created.Session.Subscribe(func(event codingagent.AgentSessionEvent) {
		events = append(events, string(event.AgentSessionEventType()))
		if update, ok := event.(codingagent.AgentSessionToolExecutionUpdateEvent); ok && child == 0 && len(update.PartialResult.Content) > 0 {
			if text, ok := update.PartialResult.Content[0].(ai.TextContent); ok {
				fmt.Sscanf(text.Text, "%d %d", &shell, &child)
			}
			if child > 0 && abort {
				created.Session.Abort()
			}
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := created.Session.Prompt(ctx, "run and cancel"); err != nil && !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if shell <= 0 || child <= 0 {
		t.Fatal("missing process IDs")
	}
	t.Cleanup(func() { syscall.Kill(shell, syscall.SIGKILL); syscall.Kill(child, syscall.SIGKILL) })
	for _, pid := range []int{shell, child} {
		deadline := time.Now().Add(2 * time.Second)
		for syscall.Kill(pid, 0) == nil && time.Now().Before(deadline) {
			time.Sleep(10 * time.Millisecond)
		}
		if err := syscall.Kill(pid, 0); err != syscall.ESRCH {
			t.Fatalf("process %d survived abort: %v", pid, err)
		}
	}
	ended := false
	for _, name := range events {
		if name == "tool_execution_end" {
			ended = true
		}
		if ended && name == "tool_execution_update" {
			t.Fatal("update after final")
		}
	}
	if !ended || events[len(events)-1] != "agent_settled" {
		t.Fatalf("events: %v", events)
	}
	found := false
	for _, message := range created.Session.Messages() {
		if result, ok := message.(ai.ToolResultMessage); ok {
			found = true
			status := "Command aborted"
			if !abort {
				status = "Command timed out after 0.2 seconds"
			}
			if !result.IsError || !strings.Contains(readToolResultText(t, result), status) {
				t.Fatalf("abort result: %v", result)
			}
		}
	}
	if !found {
		t.Fatal("missing partial aborted ToolResult")
	}
}

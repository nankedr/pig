//go:build darwin || linux

package codingagent_test

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
	"github.com/nankedr/pig/internal/terminaltest"
	"github.com/nankedr/pig/tui"
)

func TestInteractiveBranchSummaryRecovery119(t *testing.T) {
	for _, scenario := range []string{"failure", "cancel", "custom", "skip"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			entered := make(chan ai.Context, 1)
			var recovering atomic.Bool
			session, manager := compact89Session(t, func(ctx context.Context, m ai.Model, input ai.Context, opts ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
				if recovering.Load() {
					return compact89Response("RECOVERED_119", ai.StopReasonStop)
				}
				entered <- input
				if scenario == "cancel" {
					stream := ai.NewAssistantMessageEventStream()
					go func() {
						<-ctx.Done()
						stream.End(ai.AssistantMessage{Role: ai.MessageRoleAssistant, StopReason: ai.StopReasonAborted})
					}()
					return stream
				}
				if scenario == "failure" {
					stream := ai.NewAssistantMessageEventStream()
					stream.End(ai.AssistantMessage{Role: ai.MessageRoleAssistant, StopReason: ai.StopReasonError, ErrorMessage: ai.Some("SUMMARY_FAILURE_119")})
					return stream
				}
				return compact89Response("SAVED_SUMMARY_119", ai.StopReasonStop)
			}, true)
			runtime := codingagent.NewAgentSessionRuntime(session, codingagent.AgentSessionServices{}, nil, nil, nil)
			before := tree91Snapshot(t, session)
			file := *manager.GetSessionFile()
			bytesBefore, _ := os.ReadFile(file)
			if scenario == "skip" {
				recovering.Store(true)
				if err := session.SettingsManager().ApplyOverrides(codingagent.Settings{BranchSummary: &codingagent.BranchSummarySettings{SkipPrompt: true}}); err != nil {
					t.Fatal(err)
				}
			}
			tty := terminaltest.Open(t)
			mode := codingagent.NewInteractiveMode(runtime, codingagent.InteractiveModeOptions{Terminal: tui.NewProcessTerminal(tty.Slave, tty.Slave)})
			done := make(chan error, 1)
			finished := make(chan struct{})
			go func() { done <- mode.Run(ctx); close(finished) }()
			defer func() { cancel(); <-finished }()
			tty.Wait(t, "> ")
			tty.Send(t, "/tree\r")
			tty.Wait(t, "Session Tree")
			tty.Send(t, "old request\r")
			if scenario != "skip" {
				tty.Wait(t, "Summarize branch?")
				if scenario == "custom" {
					tty.Send(t, "\x1b[B\x1b[B\r")
					tty.Wait(t, "Custom summarization instructions")
					tty.Send(t, "\x1b[27u")
					waitSessionScreen118(t, tty, "Summarize branch?", true)
					tty.Send(t, "\x1b[B\x1b[B\r")
					waitSessionScreen118(t, tty, "Custom summarization instructions", true)
					tty.Send(t, "FOCUS_119\r")
				} else {
					tty.Send(t, "\x1b[B\r")
				}
				var input ai.Context
				select {
				case input = <-entered:
				case <-ctx.Done():
					t.Fatal("summary never started")
				}
				if scenario == "custom" {
					raw, _ := json.Marshal(input)
					if !strings.Contains(string(raw), "FOCUS_119") {
						t.Fatal("custom instructions missing", string(raw))
					}
				}
				if scenario == "cancel" {
					if _, err := session.NavigateTree(ctx, manager.GetEntries()[0].ID); err == nil {
						t.Fatal("concurrent navigation admitted")
					}
					if err := session.Prompt(ctx, "overlap"); err == nil {
						t.Fatal("concurrent Prompt admitted")
					}
					tty.Send(t, "saved draft\x1b[27u")
					tty.Wait(t, "Branch summarization cancelled")
					waitSessionScreen118(t, tty, "Session Tree", true)
					tty.Send(t, "\x1b[27u")
					waitSessionScreen118(t, tty, "Session Tree", false)
					waitSessionScreen118(t, tty, "> saved draft", true)
				} else if scenario == "failure" {
					tty.Wait(t, "SUMMARY_FAILURE_119")
				}
			}
			if scenario == "failure" || scenario == "cancel" {
				if before != tree91Snapshot(t, session) {
					t.Fatal("failed summary changed Session")
				}
				data, _ := os.ReadFile(file)
				if string(data) != string(bytesBefore) {
					t.Fatal("failed summary rewrote history")
				}
			} else {
				tty.Wait(t, "Navigated to selected point")
				if scenario == "skip" {
					select {
					case <-entered:
						t.Fatal("skip prompt generated summary")
					default:
					}
				}
			}
			recovering.Store(true)
			if scenario == "cancel" {
				tty.Send(t, "\r")
			} else {
				tty.Send(t, "\x15continue after navigation\r")
			}
			tty.Wait(t, "RECOVERED_119")
			if err := session.WaitForIdle(ctx); err != nil {
				t.Fatal(err)
			}
			tty.Send(t, "\x04")
			if err := awaitInteractive(t, done); err != nil {
				t.Fatal(err)
			}
			reopened, err := codingagent.OpenSessionManager(file, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			users := []string{}
			summaries := 0
			for _, e := range reopened.GetBranch() {
				if e.Type == "branch_summary" {
					summaries++
				}
				if e.Message != nil && e.Message.MessageRole() == ai.MessageRoleUser {
					users = append(users, message85Text(e.Message))
				}
			}
			want := "old request|recent request long enough to keep|continue after navigation"
			switch scenario {
			case "cancel":
				want = "old request|recent request long enough to keep|saved draft"
			case "custom", "skip":
				want = "continue after navigation"
			}
			if strings.Join(users, "|") != want {
				t.Fatal(users, want)
			}
			if (scenario == "custom") != (summaries == 1) {
				t.Fatal("wrong summary count", summaries)
			}
			if !tty.Restored(t) {
				t.Fatal("terminal mode leaked")
			}
		})
	}
}

func TestInteractiveTreeStreamingCancellation119(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	started, aborted := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	session, manager := compact89Session(t, func(ctx context.Context, m ai.Model, input ai.Context, opts ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
		if calls.Add(1) > 1 {
			return compact89Response("AFTER_BUSY_119", ai.StopReasonStop)
		}
		close(started)
		stream := ai.NewAssistantMessageEventStream()
		go func() {
			<-ctx.Done()
			close(aborted)
			stream.End(ai.AssistantMessage{Role: ai.MessageRoleAssistant, StopReason: ai.StopReasonAborted})
		}()
		return stream
	}, true)
	runtime := codingagent.NewAgentSessionRuntime(session, codingagent.AgentSessionServices{}, nil, nil, nil)
	tty := terminaltest.Open(t)
	mode := codingagent.NewInteractiveMode(runtime, codingagent.InteractiveModeOptions{Terminal: tui.NewProcessTerminal(tty.Slave, tty.Slave)})
	done := make(chan error, 1)
	finished := make(chan struct{})
	go func() { done <- mode.Run(ctx); close(finished) }()
	defer func() { cancel(); <-finished }()
	tty.Wait(t, "> ")
	tty.Send(t, "blocking turn\r")
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("no generation")
	}
	tty.Send(t, "/tree\r")
	tty.Wait(t, "Session Tree")
	tty.Send(t, "\x1b[27u")
	waitSessionScreen118(t, tty, "Session Tree", false)
	select {
	case <-aborted:
		t.Fatal("selector cancel stopped generation")
	default:
	}
	tty.Send(t, "/tree\r")
	waitSessionScreen118(t, tty, "Session Tree", true)
	tty.Send(t, "old request\r")
	tty.Wait(t, "Summarize branch?")
	select {
	case <-aborted:
		t.Fatal("summary prompt stopped generation")
	default:
	}
	tty.Send(t, "\r")
	tty.Wait(t, "Navigated to selected point")
	select {
	case <-aborted:
	case <-ctx.Done():
		t.Fatal("navigation did not abort old generation")
	}
	tty.Send(t, " revised\r")
	tty.Wait(t, "AFTER_BUSY_119")
	if err := session.WaitForIdle(ctx); err != nil {
		t.Fatal(err)
	}
	tty.Send(t, "\x04")
	if err := awaitInteractive(t, done); err != nil {
		t.Fatal(err)
	}
	reopened, err := codingagent.OpenSessionManager(*manager.GetSessionFile(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	users := []string{}
	for _, m := range reopened.BuildSessionContext().Messages {
		if m.MessageRole() == ai.MessageRoleUser {
			users = append(users, message85Text(m))
		}
	}
	if strings.Join(users, "|") != "old request revised" {
		t.Fatal(users)
	}
	found := false
	for _, e := range reopened.GetEntries() {
		if e.Message != nil && message85Text(e.Message) == "blocking turn" {
			found = true
		}
	}
	if !found {
		t.Fatal("old branch was lost")
	}
}

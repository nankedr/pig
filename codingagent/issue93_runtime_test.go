package codingagent_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
)

func TestSessionBashConcurrentExecutions(t *testing.T) {
	session := newMessage85Session(t, nil)
	started := make(chan string, 2)
	first := make(chan struct{})
	finished := make(chan codingagent.BashResult, 2)
	ops := bash93Operations(func(ctx context.Context, cmd, _ string, o codingagent.BashExecOptions) (codingagent.BashExecResult, error) {
		started <- cmd
		if cmd == "first" {
			<-first
		} else {
			<-ctx.Done()
		}
		code := 0
		return codingagent.BashExecResult{ExitCode: &code}, nil
	})
	for _, command := range []string{"first", "second"} {
		go func() {
			r, err := session.ExecuteBash(context.Background(), command, codingagent.ExecuteBashOptions{Operations: ops})
			if err != nil {
				t.Error(err)
			}
			finished <- r
		}()
	}
	<-started
	<-started
	close(first)
	r := <-finished
	if r.Cancelled {
		t.Fatal("first execution unexpectedly cancelled")
	}
	if running, err := session.IsBashRunning(); err != nil || !running {
		t.Fatalf("newer Bash lost: %v %v", running, err)
	}
	session.AbortBash()
	r = <-finished
	if !r.Cancelled || r.ExitCode != nil {
		t.Fatalf("cancel result: %+v", r)
	}
	if running, _ := session.IsBashRunning(); running {
		t.Fatal("execution retained after completion")
	}
	if len(session.Messages()) != 2 {
		t.Fatal("missing concurrent Bash history")
	}
}
func TestSessionBashAbortAllAndDispose(t *testing.T) {
	for _, dispose := range []bool{false, true} {
		t.Run(fmt.Sprint(dispose), func(t *testing.T) {
			session := newMessage85Session(t, nil)
			ready := make(chan struct{}, 2)
			done := make(chan codingagent.BashResult, 2)
			ops := bash93Operations(func(ctx context.Context, _, _ string, o codingagent.BashExecOptions) (codingagent.BashExecResult, error) {
				ready <- struct{}{}
				<-ctx.Done()
				o.OnData([]byte("partial"))
				return codingagent.BashExecResult{}, ctx.Err()
			})
			for range 2 {
				go func() {
					r, e := session.ExecuteBash(context.Background(), "waiting", codingagent.ExecuteBashOptions{Operations: ops})
					if e != nil {
						t.Error(e)
					}
					done <- r
				}()
			}
			<-ready
			<-ready
			if dispose {
				session.Dispose()
			} else {
				session.AbortBash()
			}
			for range 2 {
				if r := <-done; !r.Cancelled || r.Output != "partial" {
					t.Fatalf("result: %+v", r)
				}
			}
			if len(session.Messages()) != 2 {
				t.Fatal("cancelled results not retained")
			}
			if dispose {
				if _, e := session.ExecuteBash(context.Background(), "must not run"); e == nil {
					t.Fatal("disposed session executed")
				}
			}
		})
	}
}
func TestSessionBashEventsOwnIDAndRecordOwnsResult(t *testing.T) {
	session := newMessage85Session(t, nil)
	id := "original"
	var events []string
	session.Subscribe(func(e codingagent.AgentSessionEvent) {
		if e, ok := e.(codingagent.AgentSessionBashExecutionUpdateEvent); ok {
			*e.ID = "mutated"
		}
	})
	session.Subscribe(func(e codingagent.AgentSessionEvent) {
		if e, ok := e.(codingagent.AgentSessionBashExecutionUpdateEvent); ok {
			events = append(events, *e.ID+":"+e.Delta)
		}
	})
	result, err := session.ExecuteBash(context.Background(), "cmd", codingagent.ExecuteBashOptions{ID: &id, Operations: bash93Operations(func(_ context.Context, _, _ string, o codingagent.BashExecOptions) (codingagent.BashExecResult, error) {
		o.OnData([]byte("a"))
		o.OnData([]byte("b"))
		code := 3
		return codingagent.BashExecResult{ExitCode: &code}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	*result.ExitCode = 99
	if id != "original" || !reflect.DeepEqual(events, []string{"original:a", "original:b"}) {
		t.Fatalf("events: %v; id %s", events, id)
	}
	if got := codingagent.ConvertToLLM(session.Messages())[0].(ai.UserMessage); !strings.Contains(message85Text(got), "code 3") {
		t.Fatal("result pointer mutated history")
	}
}
func TestSessionBashTruncatedOutputReadAndReopen(t *testing.T) {
	dir := t.TempDir()
	manager, err := codingagent.NewSessionManager(dir, &dir)
	if err != nil {
		t.Fatal(err)
	}
	core, _ := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
	model, _ := core.GetModel()
	reply, _ := ai.FauxAssistantMessage(ai.FauxAssistantText("done"))
	core.SetResponses([]ai.FauxResponseStep{reply})
	created, err := codingagent.CreateAgentSession(context.Background(), codingagent.CreateAgentSessionOptions{CWD: dir, AgentDir: dir, SessionManager: manager, Model: &model, NoTools: codingagent.NoToolsAll, StreamFunction: agent.StreamFunction(core.StreamSimple)})
	if err != nil {
		t.Fatal(err)
	}
	session := created.Session
	defer session.Dispose()
	r, err := session.ExecuteBash(context.Background(), `i=1; while [ $i -le 4000 ]; do printf 'line-%04d\n' "$i"; i=$((i+1)); done`)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Truncated || r.FullOutputPath == nil || !strings.HasPrefix(r.Output, "line-2001\n") || !strings.HasSuffix(r.Output, "line-4000") {
		t.Fatalf("truncation: truncated=%v path=%v length=%d tail=%q", r.Truncated, r.FullOutputPath, len(r.Output), r.Output[max(0, len(r.Output)-30):])
	}
	defer os.Remove(*r.FullOutputPath)
	if err := session.Prompt(context.Background(), "remember"); err != nil {
		t.Fatal(err)
	}
	before := codingagent.ConvertToLLM(session.Messages())
	session.Dispose()
	reopened, err := codingagent.OpenSessionManager(*session.SessionFile(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := codingagent.ConvertToLLM(reopened.BuildSessionContext().Messages); !reflect.DeepEqual(got, before) {
		t.Fatal("reopened model input differs")
	}
	read, err := codingagent.CreateReadTool(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		offset int
		want   string
	}{{1, "line-0001\nline-0002"}, {3999, "line-3999\nline-4000"}} {
		got := runBashSession(t, context.Background(), dir, []agent.ErasedAgentTool{read}, []ai.ToolCall{{Type: "toolCall", ID: "read", Name: "read", Arguments: map[string]any{"path": *r.FullOutputPath, "offset": tc.offset, "limit": 2}}})
		if got[0].IsError || !strings.HasPrefix(readToolResultText(t, got[0]), tc.want) {
			t.Fatalf("read original: %+v", got)
		}
	}
}
func TestSessionBashFailureAndCancelledPromptFlush(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	session := newMessage85Session(t, func(ctx context.Context, m ai.Model, in ai.Context, o ai.SimpleStreamOptions, core *ai.FauxCore) *ai.AssistantMessageEventStream {
		once.Do(func() { close(entered); <-release })
		return core.StreamSimple(ctx, m, in, o)
	})
	done := make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { done <- session.Prompt(ctx, "start") }()
	<-entered
	code := 4
	if e := session.RecordBashResult("external", codingagent.BashResult{Output: "kept", ExitCode: &code}); e != nil {
		t.Fatal(e)
	}
	if p, _ := session.HasPendingBashMessages(); !p {
		t.Fatal("missing pending Bash")
	}
	cancel()
	close(release)
	if e := <-done; e != nil && !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if p, _ := session.HasPendingBashMessages(); p {
		t.Fatal("cancelled Prompt did not flush")
	}
	if last := session.Messages()[len(session.Messages())-1]; last.MessageRole() != "bashExecution" {
		t.Fatal("Bash ordering after cancellation")
	}
	before := len(session.Messages())
	session.SettingsManager().SetShellPath("/nonexistent/pig-issue93-shell")
	if _, e := session.ExecuteBash(context.Background(), "bad"); e == nil {
		t.Fatal("invalid shell succeeded")
	}
	if len(session.Messages()) != before {
		t.Fatal("execution failure recorded as a result")
	}
}
func TestSessionBashConcurrentRecordAtSettlement(t *testing.T) {
	session := newMessage85Session(t, nil)
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 8 {
				if err := session.RecordBashResult("external", codingagent.BashResult{Output: "x"}); err != nil {
					t.Error(err)
				}
			}
		}()
	}
	if err := session.Prompt(context.Background(), "start"); err != nil {
		t.Fatal(err)
	}
	wg.Wait()
	if err := session.Prompt(context.Background(), "next"); err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, m := range session.Messages() {
		if m.MessageRole() == "bashExecution" {
			count++
		}
	}
	if count != 96 {
		t.Fatalf("lost or duplicate Bash records: %d", count)
	}
}
func TestSessionBashContextCancellation(t *testing.T) {
	session := newMessage85Session(t, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	r, e := session.ExecuteBash(ctx, "wait", codingagent.ExecuteBashOptions{Operations: bash93Operations(func(ctx context.Context, _, _ string, o codingagent.BashExecOptions) (codingagent.BashExecResult, error) {
		o.OnData([]byte("before"))
		<-ctx.Done()
		return codingagent.BashExecResult{}, ctx.Err()
	})})
	if e != nil || !r.Cancelled || r.ExitCode != nil || r.Output != "before" {
		t.Fatalf("deadline: %+v %v", r, e)
	}
}

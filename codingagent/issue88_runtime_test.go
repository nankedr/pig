package codingagent_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
)

func TestSessionStatsRuntimeAndRestore(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "input.txt"), []byte("hello"), 0600); err != nil {
		t.Fatal(err)
	}
	manager, err := codingagent.NewSessionManager(dir, &dir)
	if err != nil {
		t.Fatal(err)
	}
	call, _ := ai.FauxToolCall("read", map[string]any{"path": "input.txt"}, ai.FauxToolCallOptions{ID: ai.Some("read-stats")})
	tool, _ := ai.FauxAssistantMessage(ai.FauxAssistantBlocks(call), ai.FauxAssistantMessageOptions{StopReason: ai.Some(ai.StopReasonToolUse)})
	tool.Usage = ai.Usage{Input: 10, Output: 2, CacheRead: 3, CacheWrite: 4, TotalTokens: 19, Cost: ai.UsageCost{Total: 0.5}}
	final, _ := ai.FauxAssistantMessage(ai.FauxAssistantText("  done  "))
	final.Usage = ai.Usage{Input: 20, Output: 3, TotalTokens: 23, Cost: ai.UsageCost{Total: 0.25}}
	responses := []ai.AssistantMessage{tool, final}
	calls := 0
	s := stats88Session(t, manager, 32000, func(_ context.Context, model ai.Model, input ai.Context, _ ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
		if calls == 1 {
			result, ok := input.Messages[len(input.Messages)-1].(ai.ToolResultMessage)
			if !ok || result.IsError || result.Content[0].(ai.TextContent).Text != "hello" {
				t.Error("read Tool did not run")
			}
		}
		message := responses[calls]
		calls++
		message.API, message.Provider, message.Model = model.API, model.Provider, model.ID
		stream := ai.NewAssistantMessageEventStream()
		stream.Push(ai.AssistantMessageDoneEvent{Type: ai.AssistantMessageEventTypeDone, Reason: message.StopReason, Message: message})
		return stream
	})
	if err := s.SetActiveToolsByName([]string{"read"}); err != nil {
		t.Fatal(err)
	}
	before, err := s.GetSessionStats()
	if err != nil || before.TotalMessages != 0 || before.ContextUsage == nil || before.ContextUsage.Tokens == nil || *before.ContextUsage.Tokens != 0 {
		t.Fatalf("empty: %+v %v", before, err)
	}
	if _, err := os.Stat(*s.SessionFile()); !os.IsNotExist(err) {
		t.Fatalf("empty query persisted a session: %v", err)
	}
	settled := false
	_, err = s.Subscribe(func(e codingagent.AgentSessionEvent) {
		if e.AgentSessionEventType() == codingagent.AgentSessionEventTypeAgentSettled {
			settled = true
			stats, err := s.GetSessionStats()
			if err != nil || stats.TotalMessages != 4 || stats.ToolCalls != 1 || stats.ToolResults != 1 {
				t.Errorf("settled stats: %+v %v", stats, err)
			}
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Prompt(context.Background(), "read input.txt"); err != nil {
		t.Fatal(err)
	}
	stats, err := s.GetSessionStats()
	if err != nil || !settled || stats.UserMessages != 1 || stats.AssistantMessages != 2 || stats.Tokens.TotalTokens != 42 || stats.Cost != 0.75 || *stats.ContextUsage.Tokens != 23 {
		t.Fatalf("runtime: %+v %v", stats, err)
	}
	last, err := s.GetLastAssistantText()
	if err != nil || last == nil || *last != "done" {
		t.Fatalf("last: %v %v", last, err)
	}
	*stats.ContextUsage.Tokens = 9999
	*stats.ContextUsage.Percent = 9999
	*stats.SessionFile = "changed"
	*last = "changed"
	next, _ := s.GetSessionStats()
	if *next.ContextUsage.Tokens != 23 || *next.SessionFile == "changed" {
		t.Fatal("query results alias the session")
	}
	stats88CheckProcess(t, s)
}

func TestSessionStatsPartialAndRunningReads(t *testing.T) {
	for _, reason := range []ai.StopReason{ai.StopReasonError, ai.StopReasonAborted} {
		t.Run(string(reason), func(t *testing.T) {
			dir := t.TempDir()
			manager, err := codingagent.NewSessionManager(dir, &dir)
			if err != nil {
				t.Fatal(err)
			}
			size := 1
			core, _ := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{TokenSize: &ai.FauxTokenSize{Min: &size}})
			response, _ := ai.FauxAssistantMessage(ai.FauxAssistantText("partial answer"), ai.FauxAssistantMessageOptions{StopReason: ai.Some(reason), ErrorMessage: ai.Some("terminal failure")})
			response.Usage = ai.Usage{Input: 10, Output: 2, TotalTokens: 12, Cost: ai.UsageCost{Total: 0.5}}
			core.SetResponses([]ai.FauxResponseStep{response})
			s := stats88Session(t, manager, 32000, agent.StreamFunction(core.StreamSimple))
			entered, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			_, err = s.Subscribe(func(e codingagent.AgentSessionEvent) {
				if update, ok := e.(codingagent.AgentSessionMessageUpdateEvent); ok {
					m := update.Message.(ai.AssistantMessage)
					if len(m.Content) > 0 {
						once.Do(func() {
							stats, err := s.GetSessionStats()
							last, lastErr := s.GetLastAssistantText()
							if err != nil || lastErr != nil || stats.UserMessages != 1 || stats.AssistantMessages != 0 || last != nil {
								t.Errorf("streaming query: %+v %v %v", stats, err, lastErr)
							}
							close(entered)
							<-release
							if reason == ai.StopReasonAborted {
								_ = s.Abort()
							}
						})
					}
				}
			})
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- s.Prompt(ctx, "go") }()
			select {
			case <-entered:
			case <-ctx.Done():
				t.Fatal("no streaming event")
			}
			var readers sync.WaitGroup
			for i := 0; i < 4; i++ {
				readers.Add(1)
				go func() {
					defer readers.Done()
					for j := 0; j < 20; j++ {
						stats, err := s.GetSessionStats()
						usage, usageErr := s.GetContextUsage()
						_, lastErr := s.GetLastAssistantText()
						if err != nil || usageErr != nil || lastErr != nil || stats.TotalMessages != 1 || usage == nil || usage.Tokens == nil || *usage.Tokens != 1 {
							t.Errorf("running snapshot: %+v %+v %v %v %v", stats, usage, err, usageErr, lastErr)
						}
					}
				}()
			}
			readers.Wait()
			close(release)
			select {
			case <-done:
			case <-ctx.Done():
				t.Fatal("prompt did not settle")
			}
			messages := s.Messages()
			if len(messages) != 2 || messages[1].(ai.AssistantMessage).StopReason != reason {
				t.Fatalf("partial history: %+v", messages)
			}
			stats, err := s.GetSessionStats()
			last, lastErr := s.GetLastAssistantText()
			if err != nil || lastErr != nil || stats.TotalMessages != 2 || stats.AssistantMessages != 1 || last == nil || *last == "" {
				t.Fatalf("partial queries: %+v %v %v %v", stats, last, err, lastErr)
			}
			if reason == ai.StopReasonError && (stats.Tokens.TotalTokens <= 0 || stats.Cost != 0 || *stats.ContextUsage.Tokens != 5) {
				t.Fatalf("error usage is billed but not a context anchor: %+v", stats)
			}
			stats88CheckProcess(t, s)
		})
	}
}

func stats88CheckProcess(t *testing.T, s *codingagent.AgentSession) {
	t.Helper()
	file := *s.SessionFile()
	before, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "result.json")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, executable, "-test.run=^TestSessionStatsProcessHelper$", "-test.count=1")
	command.Env = append(os.Environ(), "PIG_STATS88_SESSION="+file, "PIG_STATS88_RESULT="+output)
	if log, err := command.CombinedOutput(); err != nil {
		t.Fatalf("restored process: %v\n%s", err, log)
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := json.Marshal(stats88Observe(t, s))
	var a, b any
	if err := json.Unmarshal(data, &a); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(want, &b); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("restored query: %s; want %s", data, want)
	}
	after, _ := os.ReadFile(file)
	if string(before) != string(after) {
		t.Fatal("restored query changed persisted history")
	}
}

func TestSessionStatsProcessHelper(t *testing.T) {
	file := os.Getenv("PIG_STATS88_SESSION")
	if file == "" {
		return
	}
	dir := filepath.Dir(file)
	manager, err := codingagent.OpenSessionManager(file, &dir, &dir)
	if err != nil {
		t.Fatal(err)
	}
	s := stats88Session(t, manager, 32000, nil)
	data, err := json.Marshal(stats88Observe(t, s))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(os.Getenv("PIG_STATS88_RESULT"), data, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestSessionStatsMemoryIdentityAndMissingValues(t *testing.T) {
	manager := codingagent.NewInMemorySessionManager(t.TempDir())
	s := stats88Session(t, manager, 0, nil)
	stats, err := s.GetSessionStats()
	if err != nil || stats.SessionFile != nil || stats.SessionID == "" || stats.SessionID != s.SessionID() || stats.ContextUsage != nil || stats.Tokens.TotalTokens != 0 {
		t.Fatalf("memory stats: %+v %v", stats, err)
	}
	last, err := s.GetLastAssistantText()
	if err != nil || last != nil {
		t.Fatalf("empty text: %v %v", last, err)
	}
	if !stats.Tokens.Reasoning.IsZero() || !stats.Tokens.CacheWrite1H.IsZero() {
		t.Fatal("summary synthesized optional usage fields")
	}
}

func TestSessionStatsPointerAssistantUsage(t *testing.T) {
	s := stats88Session(t, codingagent.NewInMemorySessionManager(t.TempDir()), 32000, nil)
	message, err := ai.FauxAssistantMessage(ai.FauxAssistantText("hi"))
	if err != nil {
		t.Fatal(err)
	}
	message.Usage = ai.Usage{Input: 1000, TotalTokens: 1000}
	for _, value := range []agent.AgentMessage{message, &message} {
		if err := s.Agent().ReplaceMessages([]agent.AgentMessage{value}); err != nil {
			t.Fatal(err)
		}
		usage, err := s.GetContextUsage()
		if err != nil || usage == nil || usage.Tokens == nil || *usage.Tokens != 1000 {
			t.Fatalf("%T usage: %+v, %v", value, usage, err)
		}
		stats, err := s.GetSessionStats()
		if err != nil || stats.ContextUsage == nil || *stats.ContextUsage.Tokens != 1000 || stats.TotalMessages != 0 {
			t.Fatalf("%T stats: %+v, %v", value, stats, err)
		}
		text, err := s.GetLastAssistantText()
		if err != nil || text == nil || *text != "hi" {
			t.Fatalf("%T last: %v %v", value, text, err)
		}
	}
}

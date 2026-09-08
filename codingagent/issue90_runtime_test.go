package codingagent_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
)

func auto90Session(t *testing.T, stream agent.StreamFunction, manager *codingagent.SessionManager) *codingagent.AgentSession {
	t.Helper()
	core, _ := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
	model, _ := core.GetModel()
	model.ContextWindow = 1000
	model.MaxTokens = 100
	if manager == nil {
		dir := t.TempDir()
		var err error
		manager, err = codingagent.NewSessionManager(t.TempDir(), &dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, text := range []string{"old request", "recent request long enough to keep"} {
			manager.AppendMessage(ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText(text)})
			answer := auto90Assistant(model, "seed answer", auto90Response{})
			answer.Timestamp = 1
			manager.AppendMessage(answer)
		}
	}
	settings, _ := codingagent.NewInMemorySettingsManager(codingagent.Settings{Compaction: &codingagent.CompactionSettings{Enabled: true, ReserveTokens: 100, KeepRecentTokens: 10}, Retry: &codingagent.RetrySettings{Enabled: pointerTo(true), MaxRetries: pointerTo(2), BaseDelayMS: pointerTo(int64(1))}})
	created, err := codingagent.CreateAgentSession(context.Background(), codingagent.CreateAgentSessionOptions{CWD: manager.GetCWD(), Model: &model, StreamFunction: stream, SessionManager: manager, SettingsManager: settings, NoTools: codingagent.NoToolsAll})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { created.Session.Dispose() })
	return created.Session
}
func auto90Reply(model ai.Model, text string, opts auto90Response) *ai.AssistantMessageEventStream {
	stream := ai.NewAssistantMessageEventStream()
	stream.End(auto90Assistant(model, text, opts))
	return stream
}
func auto90Run(ctx context.Context, s *codingagent.AgentSession) (codingagent.HeadlessOutcome, error) {
	return codingagent.RunHeadless(ctx, codingagent.NewAgentSessionRuntime(s, codingagent.AgentSessionServices{}, nil, nil, nil), codingagent.HeadlessRunOptions{Messages: []string{"go"}})
}

func TestAutoCompactionHeadlessFailureAndCancellation(t *testing.T) {
	for _, mode := range []string{"abort-start", "abort-request", "context", "dispose", "provider-aborted", "storage", "summary-failed", "summary-empty"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var s *codingagent.AgentSession
			generations := 0
			s = auto90Session(t, func(_ context.Context, m ai.Model, input ai.Context, _ ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
				system, _ := input.SystemPrompt.Value()
				if !strings.HasPrefix(system, "You are a context summarization assistant.") {
					generations++
					return auto90Reply(m, "partial", auto90Response{Input: pointerTo(int64(0)), Error: "prompt is too long"})
				}
				switch mode {
				case "abort-request":
					s.AbortCompaction()
				case "context":
					cancel()
				case "dispose":
					s.Dispose()
				case "provider-aborted":
					return auto90Reply(m, "partial summary", auto90Response{Reason: ai.StopReasonAborted})
				case "summary-failed":
					return auto90Reply(m, "", auto90Response{Error: "insufficient_quota"})
				case "summary-empty":
					return auto90Reply(m, "", auto90Response{})
				}
				return auto90Reply(m, "checkpoint", auto90Response{})
			}, nil)
			var end *codingagent.AgentSessionCompactionEndEvent
			s.Subscribe(func(e codingagent.AgentSessionEvent) {
				if e.AgentSessionEventType() == codingagent.AgentSessionEventTypeCompactionStart {
					if mode == "abort-start" {
						s.AbortCompaction()
					}
					if mode == "storage" {
						dir := filepath.Dir(*s.SessionManager().GetSessionFile())
						if err := os.Rename(dir, dir+"-moved"); err != nil {
							t.Fatal(err)
						}
						t.Cleanup(func() { os.RemoveAll(dir + "-moved") })
					}
				}
				if e, ok := e.(codingagent.AgentSessionCompactionEndEvent); ok {
					end = &e
				}
			})
			outcome, err := auto90Run(ctx, s)
			if err != nil {
				t.Fatal(err)
			}
			canceled := mode == "abort-start" || mode == "abort-request" || mode == "context" || mode == "dispose" || mode == "provider-aborted"
			if outcome.FinalMessage == nil || outcome.FinalMessage.StopReason != ai.StopReasonError || strings.Join(outcome.Text, "") != "partial" || outcome.Canceled != canceled {
				t.Fatalf("terminal outcome=%+v message=%+v", outcome, outcome.FinalMessage)
			}
			compacted := 0
			for _, e := range s.SessionManager().GetEntries() {
				if e.Type == "compaction" {
					compacted++
				}
			}
			wantCompacted := 0
			wantGenerations := 1
			if mode == "summary-empty" {
				wantCompacted = 1
				wantGenerations = 2
			}
			if compacted != wantCompacted || generations != wantGenerations {
				t.Fatalf("compactions=%d generations=%d", compacted, generations)
			}
			if mode != "dispose" && (end == nil || end.WillRetry || end.Aborted != canceled) {
				t.Fatalf("end=%+v", end)
			}
			if err := s.WaitForIdle(context.Background()); err != nil {
				t.Fatal(err)
			}
			if busy, _ := s.IsCompacting(); busy {
				t.Fatal("compaction remains active")
			}
		})
	}
}

func TestAutoCompactionWaitAndQueuedCancellation(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var s *codingagent.AgentSession
	s = auto90Session(t, func(_ context.Context, m ai.Model, input ai.Context, _ ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
		system, _ := input.SystemPrompt.Value()
		if strings.HasPrefix(system, "You are a context summarization assistant.") {
			close(entered)
			<-release
			return auto90Reply(m, "checkpoint", auto90Response{})
		}
		return auto90Reply(m, "finished", auto90Response{Input: pointerTo(int64(950))})
	}, nil)
	done := make(chan error, 1)
	go func() { _, err := auto90Run(context.Background(), s); done <- err }()
	<-entered
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := s.WaitForIdle(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("premature idle: %v", err)
	}
	if err := s.Prompt(context.Background(), "queued prompt", codingagent.PromptOptions{StreamingBehavior: "steer"}); err != nil {
		t.Error(err)
	}
	if err := s.SetThinkingLevel("off"); err == nil {
		t.Error("configuration admitted during compaction")
	}
	s.AbortCompaction()
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if n, _ := s.PendingMessageCount(); n != 1 {
		t.Fatalf("canceled compaction lost queue: %d", n)
	}
	if err := s.ClearQueue(); err != nil {
		t.Fatal(err)
	}
	if n, _ := s.PendingMessageCount(); n != 0 {
		t.Fatal("ClearQueue left buffered messages")
	}
}

func TestAutoCompactionCrossProcessReopen(t *testing.T) {
	if file := os.Getenv("PIG_AUTO_COMPACTION_REOPEN"); file != "" {
		manager, err := codingagent.OpenSessionManager(file, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		sawSummary := false
		s := auto90Session(t, func(_ context.Context, m ai.Model, input ai.Context, _ ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
			var text strings.Builder
			for _, message := range input.Messages {
				v := auto90Project(message)
				text.WriteString(v["text"].(string))
			}
			sawSummary = strings.Contains(text.String(), "checkpoint") && !strings.Contains(text.String(), "old request")
			return auto90Reply(m, "reopened", auto90Response{})
		}, manager)
		outcome, err := auto90Run(context.Background(), s)
		if err != nil || !sawSummary || strings.Join(outcome.Text, "") != "reopened" {
			t.Fatalf("reopen: %+v, %v, summary=%v", outcome, err, sawSummary)
		}
		return
	}
	generations := 0
	s := auto90Session(t, func(_ context.Context, m ai.Model, input ai.Context, _ ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
		system, _ := input.SystemPrompt.Value()
		if strings.HasPrefix(system, "You are a context summarization assistant.") {
			return auto90Reply(m, "checkpoint", auto90Response{})
		}
		generations++
		if generations == 1 {
			return auto90Reply(m, "overflow", auto90Response{Input: pointerTo(int64(0)), Error: "prompt is too long"})
		}
		return auto90Reply(m, "recovered", auto90Response{})
	}, nil)
	outcome, err := auto90Run(context.Background(), s)
	if err != nil || strings.Join(outcome.Text, "") != "recovered" || generations != 2 {
		t.Fatalf("recovery: %+v %v generations=%d", outcome, err, generations)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, "-test.run=^TestAutoCompactionCrossProcessReopen$")
	cmd.Env = append(os.Environ(), "PIG_AUTO_COMPACTION_REOPEN="+*s.SessionManager().GetSessionFile())
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("child: %v\n%s", err, output)
	}
	reopened, err := codingagent.OpenSessionManager(*s.SessionManager().GetSessionFile(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	users, assistants, compactions := 0, 0, 0
	for _, e := range reopened.GetEntries() {
		if e.Type == "compaction" {
			compactions++
		}
		if e.Type == "message" {
			switch e.Message.MessageRole() {
			case ai.MessageRoleUser:
				users++
			case ai.MessageRoleAssistant:
				assistants++
			}
		}
	}
	if users != 4 || assistants != 5 || compactions != 1 {
		t.Fatalf("duplicate history: user=%d assistant=%d compaction=%d", users, assistants, compactions)
	}
}

func TestAutoCompactionSettings(t *testing.T) {
	dir := t.TempDir()
	settings, err := codingagent.NewSettingsManager(t.TempDir(), &dir)
	if err != nil {
		t.Fatal(err)
	}
	s := codingagent.NewAgentSession(codingagent.AgentSessionConfig{SettingsManager: settings})
	if !s.AutoCompactionEnabled() {
		t.Fatal("default compaction must be enabled")
	}
	if err := s.SetAutoCompactionEnabled(false); err != nil {
		t.Fatal(err)
	}
	reopened, err := codingagent.NewSettingsManager(t.TempDir(), &dir)
	if err != nil {
		t.Fatal(err)
	}
	if enabled, err := reopened.GetCompactionEnabled(); err != nil || enabled {
		t.Fatalf("persisted enabled=%v err=%v", enabled, err)
	}
	if err := settings.ApplyOverrides(codingagent.Settings{Compaction: &codingagent.CompactionSettings{Enabled: true}}); err != nil {
		t.Fatal(err)
	}
	if !s.AutoCompactionEnabled() {
		t.Fatal("getter ignores effective settings")
	}
}

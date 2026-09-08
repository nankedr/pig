package codingagent_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
)

func compact89Session(t *testing.T, stream agent.StreamFunction, persist bool) (*codingagent.AgentSession, *codingagent.SessionManager) {
	t.Helper()
	core, _ := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
	model, _ := core.GetModel()
	manager := codingagent.NewInMemorySessionManager(t.TempDir())
	if persist {
		dir := t.TempDir()
		var err error
		manager, err = codingagent.NewSessionManager(t.TempDir(), &dir)
		if err != nil {
			t.Fatal(err)
		}
	}
	answer, _ := ai.FauxAssistantMessage(ai.FauxAssistantText("old answer"))
	for _, m := range []agent.AgentMessage{ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText("old request")}, answer, ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText("recent request long enough to keep")}, answer} {
		if _, err := manager.AppendMessage(m); err != nil {
			t.Fatal(err)
		}
	}
	settings, _ := codingagent.NewInMemorySettingsManager(codingagent.Settings{Compaction: &codingagent.CompactionSettings{ReserveTokens: 100, KeepRecentTokens: 10}})
	if stream == nil {
		stream = func(context.Context, ai.Model, ai.Context, ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
			return compact89Response("checkpoint", ai.StopReasonStop)
		}
	}
	created, err := codingagent.CreateAgentSession(context.Background(), codingagent.CreateAgentSessionOptions{CWD: manager.GetCWD(), Model: &model, SessionManager: manager, SettingsManager: settings, StreamFunction: stream, Tools: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { created.Session.Dispose() })
	return created.Session, manager
}
func compact89Response(text string, reason ai.StopReason) *ai.AssistantMessageEventStream {
	message, _ := ai.FauxAssistantMessage(ai.FauxAssistantText(text))
	message.StopReason = reason
	stream := ai.NewAssistantMessageEventStream()
	stream.End(message)
	return stream
}

func TestManualCompactionHeadlessRuntimeAndReopen(t *testing.T) {
	runtime, _, store := config87Runtime(t)
	var requests []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		body["key"] = r.Header.Get("Authorization")
		requests = append(requests, body)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"checkpoint\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	model, _, _ := runtime.GetModel("deepseek", "deepseek-v4-flash")
	model.BaseURL = server.URL
	_, manager := compact89Session(t, nil, true)
	settings, _ := codingagent.NewInMemorySettingsManager(codingagent.Settings{Compaction: &codingagent.CompactionSettings{ReserveTokens: 100, KeepRecentTokens: 10}})
	created, err := codingagent.CreateAgentSession(context.Background(), codingagent.CreateAgentSessionOptions{CWD: manager.GetCWD(), AgentDir: t.TempDir(), Model: &model, ModelRuntime: runtime, SessionManager: manager, SettingsManager: settings, Tools: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	defer created.Session.Dispose()
	credential76Set(t, store, "deepseek", "summary-key")
	result, err := created.Session.Compact(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	file := *manager.GetSessionFile()
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"version":3`) || !strings.Contains(string(data), `"type":"compaction"`) || result.Summary != "checkpoint" {
		t.Fatalf("v3 result=%+v file=%s", result, data)
	}
	reopened, err := codingagent.OpenSessionManager(file, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(manager.BuildSessionContext(), reopened.BuildSessionContext()) {
		t.Fatal("reopen changed compaction context")
	}
	credential76Set(t, store, "deepseek", "continue-key")
	headless, err := codingagent.CreateHeadlessSession(context.Background(), codingagent.CreateHeadlessSessionOptions{CWD: manager.GetCWD(), AgentDir: t.TempDir(), ModelRuntime: runtime, Provider: "deepseek", Model: model.ID, BaseURL: &server.URL, SessionManager: reopened, SettingsManager: settings, NoContextFiles: true, NoTools: codingagent.NoToolsAll})
	if err != nil {
		t.Fatal(err)
	}
	defer headless.Dispose(context.Background())
	outcome, err := codingagent.RunHeadless(context.Background(), headless, codingagent.HeadlessRunOptions{Messages: []string{"continue"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(requests) != 2 || requests[0]["key"] != "Bearer summary-key" || requests[1]["key"] != "Bearer continue-key" || len(outcome.Text) == 0 {
		t.Fatalf("requests=%+v outcome=%+v", requests, outcome)
	}
	summary, _ := json.Marshal(requests[0]["messages"])
	continued, _ := json.Marshal(requests[1]["messages"])
	if !strings.Contains(string(summary), "old request") || !strings.Contains(string(continued), "checkpoint") || strings.Contains(string(continued), "old request") || !strings.Contains(string(continued), "recent request") {
		t.Fatalf("summary=%s continued=%s", summary, continued)
	}
}

func TestManualCompactionCancellationAndFailureAtomicity(t *testing.T) {
	for _, mode := range []string{"abort-start", "abort-request", "context", "dispose", "storage", "provider-aborted", "changed-history"} {
		t.Run(mode, func(t *testing.T) {
			var s *codingagent.AgentSession
			var manager *codingagent.SessionManager
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			stream := func(context.Context, ai.Model, ai.Context, ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
				switch mode {
				case "abort-request":
					s.AbortCompaction()
				case "context":
					cancel()
				case "dispose":
					s.Dispose()
				case "provider-aborted":
					return compact89Response("partial", ai.StopReasonAborted)
				case "changed-history":
					manager.AppendMessage(ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText("external change")})
				}
				return compact89Response("checkpoint", ai.StopReasonStop)
			}
			s, manager = compact89Session(t, stream, true)
			entries, before := manager.GetEntries(), s.Messages()
			file := *manager.GetSessionFile()
			data, _ := os.ReadFile(file)
			if mode == "storage" {
				if err := os.Rename(filepath.Dir(file), filepath.Dir(file)+"-moved"); err != nil {
					t.Fatal(err)
				}
			}
			s.Subscribe(func(e codingagent.AgentSessionEvent) {
				if mode == "abort-start" && e.AgentSessionEventType() == codingagent.AgentSessionEventTypeCompactionStart {
					s.AbortCompaction()
				}
			})
			result, err := s.Compact(ctx)
			if err == nil || !reflect.DeepEqual(result, codingagent.CompactionResult{}) {
				t.Fatalf("failed compaction returned %+v %v", result, err)
			}
			if !reflect.DeepEqual(before, s.Messages()) {
				t.Fatal("failed compaction trimmed context")
			}
			if mode != "changed-history" && !reflect.DeepEqual(entries, manager.GetEntries()) {
				t.Fatal("failed compaction appended entry")
			}
			if mode != "storage" && mode != "changed-history" {
				after, _ := os.ReadFile(file)
				if string(data) != string(after) {
					t.Fatal("failed compaction modified file")
				}
			}
			if mode == "abort-start" || mode == "abort-request" || mode == "context" || mode == "dispose" || mode == "provider-aborted" {
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("cancellation: %v", err)
				}
			}
			if active, _ := s.IsCompacting(); active {
				t.Fatal("compaction state not reset")
			}
		})
	}
}

func TestManualCompactionBusyAndEndListenerOwnership(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	s, manager := compact89Session(t, func(context.Context, ai.Model, ai.Context, ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
		close(entered)
		<-release
		return compact89Response("checkpoint", ai.StopReasonStop)
	}, false)
	done := make(chan error, 1)
	s.Subscribe(func(e codingagent.AgentSessionEvent) {
		if end, ok := e.(codingagent.AgentSessionCompactionEndEvent); ok && end.Result != nil {
			end.Result.Summary = "mutated"
			end.Result.Details.(map[string]any)["readFiles"] = []string{"mutated"}
			*end.Result.EstimatedTokensAfter = 999
		}
	})
	s.Subscribe(func(e codingagent.AgentSessionEvent) {
		if end, ok := e.(codingagent.AgentSessionCompactionEndEvent); ok && end.Result != nil {
			if end.Result.Summary != "checkpoint" || *end.Result.EstimatedTokensAfter == 999 {
				t.Error("listeners share result")
			}
			if err := s.SetActiveToolsByName([]string{}); err != nil {
				t.Error("end listener sees busy session", err)
			}
		}
	})
	go func() { _, err := s.Compact(context.Background()); done <- err }()
	<-entered
	for _, call := range []func() error{func() error { return s.Prompt(context.Background(), "busy") }, func() error { return s.SetThinkingLevel("low") }, func() error { _, err := s.Compact(context.Background()); return err }} {
		if call() == nil {
			t.Error("accepted overlapping mutation")
		}
	}
	timeout, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	if !errors.Is(s.WaitForIdle(timeout), context.DeadlineExceeded) {
		t.Error("wait did not include compaction")
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	result := manager.GetLeafEntry()
	if result.Summary != "checkpoint" || strings.Contains(string(result.Details), "mutated") {
		t.Fatalf("listener mutated persistence: %+v", result)
	}
}

func TestManualCompactionStopsActiveTurn(t *testing.T) {
	started := make(chan struct{})
	summaryCalls := 0
	s, manager := compact89Session(t, func(ctx context.Context, model ai.Model, input ai.Context, o ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
		prompt, _ := input.SystemPrompt.Value()
		if strings.Contains(prompt, "context summarization assistant") {
			summaryCalls++
			return compact89Response("checkpoint", ai.StopReasonStop)
		}
		stream := ai.NewAssistantMessageEventStream()
		message, _ := ai.FauxAssistantMessage(ai.FauxAssistantText("partial active response"))
		stream.Push(ai.AssistantMessageStartEvent{Type: ai.AssistantMessageEventTypeStart, Partial: message})
		close(started)
		go func() {
			<-ctx.Done()
			message.StopReason = ai.StopReasonAborted
			stream.Push(ai.AssistantMessageErrorEvent{Type: ai.AssistantMessageEventTypeError, Reason: ai.StopReasonAborted, Error: message})
		}()
		return stream
	}, true)
	done := make(chan error, 1)
	go func() { done <- s.Prompt(context.Background(), "active request") }()
	<-started
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result, err := s.Compact(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if summaryCalls < 1 || result.Summary == "" {
		t.Fatalf("summary=%+v calls=%d", result, summaryCalls)
	}
	<-done
	entries := manager.GetEntries()
	if entries[len(entries)-1].Type != "compaction" {
		t.Fatal("active transcript overtook compaction")
	}
	if err := s.WaitForIdle(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestManualCompactionRetriesRuntimeTruncation(t *testing.T) {
	runtime, _, _ := config87Runtime(t)
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "text/event-stream")
		if calls == 1 {
			fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\n")
			return
		}
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"checkpoint\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	model, _, _ := runtime.GetModel("deepseek", "deepseek-v4-flash")
	model.BaseURL = server.URL
	_, manager := compact89Session(t, nil, false)
	settings, _ := codingagent.NewInMemorySettingsManager(codingagent.Settings{Compaction: &codingagent.CompactionSettings{ReserveTokens: 100, KeepRecentTokens: 10}, Retry: &codingagent.RetrySettings{Enabled: pointerTo(true), MaxRetries: pointerTo(1), BaseDelayMS: pointerTo(int64(1))}})
	created, err := codingagent.CreateAgentSession(context.Background(), codingagent.CreateAgentSessionOptions{CWD: manager.GetCWD(), AgentDir: t.TempDir(), Model: &model, ModelRuntime: runtime, SessionManager: manager, SettingsManager: settings, Tools: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	defer created.Session.Dispose()
	retries := 0
	created.Session.Subscribe(func(e codingagent.AgentSessionEvent) {
		if e.AgentSessionEventType() == codingagent.AgentSessionEventTypeSummarizationRetryScheduled {
			retries++
		}
	})
	result, err := created.Session.Compact(context.Background())
	if err != nil || result.Summary != "checkpoint" || calls != 2 || retries != 1 {
		t.Fatalf("result=%+v error=%v calls=%d retries=%d", result, err, calls, retries)
	}
}

func TestManualCompactionCancellationWaitsForTurnCleanup(t *testing.T) {
	started, cleaning, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	s, _ := compact89Session(t, func(ctx context.Context, model ai.Model, input ai.Context, o ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
		stream := ai.NewAssistantMessageEventStream()
		message, _ := ai.FauxAssistantMessage(ai.FauxAssistantText("partial"))
		stream.Push(ai.AssistantMessageStartEvent{Type: ai.AssistantMessageEventTypeStart, Partial: message})
		close(started)
		go func() {
			<-ctx.Done()
			message.StopReason = ai.StopReasonAborted
			stream.Push(ai.AssistantMessageErrorEvent{Type: ai.AssistantMessageEventTypeError, Reason: ai.StopReasonAborted, Error: message})
		}()
		return stream
	}, false)
	s.Subscribe(func(e codingagent.AgentSessionEvent) {
		if e.AgentSessionEventType() == codingagent.AgentSessionEventTypeAgentSettled {
			close(cleaning)
			<-release
		}
	})
	prompt := make(chan error, 1)
	go func() { prompt <- s.Prompt(context.Background(), "active") }()
	<-started
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	compact := make(chan error, 1)
	go func() { _, err := s.Compact(ctx); compact <- err }()
	<-cleaning
	waiting := make(chan error, 1)
	go func() { waiting <- s.WaitForIdle(context.Background()) }()
	cancel()
	select {
	case err := <-compact:
		t.Errorf("compaction returned before cleanup: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	select {
	case err := <-waiting:
		t.Errorf("idle returned before cleanup: %v", err)
	default:
	}
	close(release)
	<-prompt
	select {
	case err := <-compact:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("compaction did not settle")
	}
	select {
	case err := <-waiting:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("idle did not settle")
	}
}

func TestManualCompactionPreservesCancellationCause(t *testing.T) {
	for _, cause := range []error{context.DeadlineExceeded, errors.New("caller stopped summarization")} {
		t.Run(cause.Error(), func(t *testing.T) {
			ctx, cancel := context.WithCancelCause(context.Background())
			defer cancel(nil)
			s, manager := compact89Session(t, func(context.Context, ai.Model, ai.Context, ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
				cancel(cause)
				return compact89Response("checkpoint", ai.StopReasonStop)
			}, false)
			before := manager.GetEntries()
			aborted := false
			s.Subscribe(func(e codingagent.AgentSessionEvent) {
				if end, ok := e.(codingagent.AgentSessionCompactionEndEvent); ok {
					aborted = end.Aborted
				}
			})
			_, err := s.Compact(ctx)
			if !errors.Is(err, cause) || !aborted || !reflect.DeepEqual(before, manager.GetEntries()) {
				t.Fatalf("cause=%v err=%v aborted=%t", cause, err, aborted)
			}
		})
	}
}

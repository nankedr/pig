package codingagent_test

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
	"github.com/nankedr/pig/internal/baseline"
	"github.com/nankedr/pig/internal/parity"
)

func retry86Message(message agent.AgentMessage) map[string]any {
	data, _ := ai.MarshalMessage(message.(ai.Message))
	var value struct {
		Role                     string
		Content                  json.RawMessage
		StopReason, ErrorMessage string
	}
	_ = json.Unmarshal(data, &value)
	var text string
	if json.Unmarshal(value.Content, &text) != nil {
		var blocks []struct{ Type, Text string }
		_ = json.Unmarshal(value.Content, &blocks)
		for _, block := range blocks {
			if block.Type == "text" {
				text += block.Text
			}
		}
	}
	result := map[string]any{"role": value.Role, "text": text}
	if value.Role == "assistant" {
		result["stopReason"], result["errorMessage"] = value.StopReason, value.ErrorMessage
	}
	return result
}

func TestTurnRetryParity(t *testing.T) {
	root := issue32RepoRoot(t)
	lock, _, err := baseline.Load(filepath.Join(root, "parity/baseline"))
	if err != nil {
		t.Fatal(err)
	}
	locked := parity.Baseline{ID: lock.BaselineID, Commit: lock.Upstream.Commit, Repository: lock.Upstream.Repository}
	fixture, err := parity.LoadFixture(filepath.Join(root, "parity/oracle/fixtures/turn-retry.json"), locked)
	if err != nil {
		t.Fatal(err)
	}
	oracle, err := parity.NewFixtureDriver(fixture, locked)
	if err != nil {
		t.Fatal(err)
	}
	result, err := parity.RunCase(context.Background(), fixture.Case, oracle, parity.DriverFunc{SurfaceName: parity.SurfaceGoSDK, ObserveFunc: func(ctx context.Context, c parity.Case) (parity.Observation, error) {
		var input struct {
			Scenarios []struct {
				Name        string
				Errors      []string
				MaxRetries  int
				BaseDelayMs int64
				Enabled     bool
				Cancel      bool
			}
		}
		if err := json.Unmarshal(c.Input, &input); err != nil {
			return parity.Observation{}, err
		}
		runs := []map[string]any{}
		for _, scenario := range input.Scenarios {
			core, err := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{API: "faux:retry-fixture", Provider: "retry-fixture", Models: []ai.FauxModelDefinition{{ID: "fixture"}}})
			if err != nil {
				return parity.Observation{}, err
			}
			model, _ := core.GetModel()
			settings, err := codingagent.NewInMemorySettingsManager(codingagent.Settings{Compaction: &codingagent.CompactionSettings{Enabled: false}, Retry: &codingagent.RetrySettings{Enabled: &scenario.Enabled, MaxRetries: &scenario.MaxRetries, BaseDelayMS: &scenario.BaseDelayMs}})
			if err != nil {
				return parity.Observation{}, err
			}
			manager := codingagent.NewInMemorySessionManager(t.TempDir())
			created, err := codingagent.CreateAgentSession(ctx, codingagent.CreateAgentSessionOptions{CWD: manager.GetCWD(), Model: &model, StreamFunction: agent.StreamFunction(core.StreamSimple), SettingsManager: settings, SessionManager: manager, Tools: []string{"write"}})
			if err != nil {
				return parity.Observation{}, err
			}
			session := created.Session
			defer session.Dispose()
			contexts := [][]map[string]any{}
			steps := []ai.FauxResponseStep{}
			for _, failure := range append(append([]string{}, scenario.Errors...), "") {
				options := ai.FauxAssistantMessageOptions{}
				text := "recovered"
				if failure != "" && failure != "@tool" {
					options.StopReason = ai.Some(ai.StopReasonError)
					options.ErrorMessage = ai.Some(failure)
					text = "partial"
				}
				message, err := ai.FauxAssistantMessage(ai.FauxAssistantText(text), options)
				if err != nil {
					return parity.Observation{}, err
				}
				if failure == "@tool" {
					call, _ := ai.FauxToolCall("write", map[string]any{"path": "once.txt", "content": "once"}, ai.FauxToolCallOptions{ID: ai.Some("call-once")})
					message, _ = ai.FauxAssistantMessage(ai.FauxAssistantBlocks(call), ai.FauxAssistantMessageOptions{StopReason: ai.Some(ai.StopReasonToolUse)})
				}
				steps = append(steps, ai.FauxResponseFactory(func(input ai.Context, _ *ai.SimpleStreamOptions, _ *ai.FauxProviderState, _ ai.Model) (ai.AssistantMessage, error) {
					messages := []map[string]any{}
					for _, m := range input.Messages {
						messages = append(messages, retry86Message(m))
					}
					contexts = append(contexts, messages)
					return message, nil
				}))
			}
			core.SetResponses(steps)
			events := []map[string]any{}
			_, err = session.Subscribe(func(event codingagent.AgentSessionEvent) {
				kind := string(event.AgentSessionEventType())
				if kind == "message_update" && len(events) > 0 && events[len(events)-1]["type"] == kind {
					return
				}
				e := map[string]any{"type": kind}
				switch event := event.(type) {
				case codingagent.AgentSessionMessageEndEvent:
					e["message"] = retry86Message(event.Message)
				case codingagent.AgentSessionAgentEndEvent:
					e["willRetry"] = event.WillRetry
				case codingagent.AgentSessionAutoRetryStartEvent:
					data, _ := json.Marshal(event)
					_ = json.Unmarshal(data, &e)
					e["retryAttempt"] = session.RetryAttempt()
					e["historyCount"] = len(manager.BuildSessionContext().Messages)
				case codingagent.AgentSessionAutoRetryEndEvent:
					data, _ := json.Marshal(event)
					_ = json.Unmarshal(data, &e)
					e["retryAttempt"] = session.RetryAttempt()
					e["historyCount"] = len(manager.BuildSessionContext().Messages)
				}
				events = append(events, e)
				if scenario.Cancel && kind == "auto_retry_start" {
					time.AfterFunc(time.Millisecond, func() { _ = session.AbortRetry() })
				}
			})
			if err != nil {
				return parity.Observation{}, err
			}
			runCtx := ctx
			cancel := func() {}
			if scenario.BaseDelayMs > 2147483647 {
				runCtx, cancel = context.WithTimeout(ctx, time.Second)
			}
			err = session.Prompt(runCtx, "retry please")
			cancel()
			if err != nil {
				return parity.Observation{}, err
			}
			messages, history := []map[string]any{}, []map[string]any{}
			for _, m := range session.Messages() {
				messages = append(messages, retry86Message(m))
			}
			for _, entry := range manager.GetEntries() {
				if entry.Message != nil {
					history = append(history, retry86Message(entry.Message))
				}
			}
			retrying, _ := session.IsRetrying()
			toolExecutions := 0
			for _, e := range events {
				if e["type"] == "tool_execution_end" {
					toolExecutions++
				}
			}
			runs = append(runs, map[string]any{"toolExecutions": toolExecutions, "name": scenario.Name, "events": events, "contexts": contexts, "messages": messages, "history": history, "attempt": session.RetryAttempt(), "retrying": retrying})
		}
		data, err := json.Marshal(map[string]any{"runs": runs})
		return parity.Observation{Outcome: data, SideEffects: &[]parity.SideEffect{}}, err
	}})
	if err != nil || !result.Match {
		var want, got struct{ Runs []json.RawMessage }
		_ = json.Unmarshal(fixture.Observation.Outcome, &want)
		_ = json.Unmarshal(result.Pig.Outcome, &got)
		for i := range got.Runs {
			if i >= len(want.Runs) {
				break
			}
			var a, b any
			_ = json.Unmarshal(want.Runs[i], &a)
			_ = json.Unmarshal(got.Runs[i], &b)
			if !reflect.DeepEqual(a, b) {
				t.Errorf("scenario %d: want=%s; got=%s", i, want.Runs[i], got.Runs[i])
			}
		}
		t.Fatalf("retry parity differences: %+v; err=%v", result.Differences, err)
	}
}

func TestTurnRetryHeadlessCancellation(t *testing.T) {
	for _, operation := range []string{"AbortRetry", "Abort", "context", "Dispose"} {
		t.Run(operation, func(t *testing.T) {
			partial, _ := ai.FauxAssistantMessage(ai.FauxAssistantText("partial"), ai.FauxAssistantMessageOptions{StopReason: ai.Some(ai.StopReasonError), ErrorMessage: ai.Some("503")})
			recovered, _ := ai.FauxAssistantMessage(ai.FauxAssistantText("recovered"))
			core, _ := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
			core.SetResponses([]ai.FauxResponseStep{partial, recovered})
			model, _ := core.GetModel()
			enabled, retries, delay := true, 2, int64(40)
			settings, _ := codingagent.NewInMemorySettingsManager(codingagent.Settings{Retry: &codingagent.RetrySettings{Enabled: &enabled, MaxRetries: &retries, BaseDelayMS: &delay}})
			manager := codingagent.NewInMemorySessionManager(t.TempDir())
			created, err := codingagent.CreateAgentSession(context.Background(), codingagent.CreateAgentSessionOptions{CWD: manager.GetCWD(), Model: &model, StreamFunction: agent.StreamFunction(core.StreamSimple), SettingsManager: settings, SessionManager: manager, NoTools: codingagent.NoToolsAll})
			if err != nil {
				t.Fatal(err)
			}
			session := created.Session
			defer session.Dispose()
			runtime := codingagent.NewAgentSessionRuntime(session, codingagent.AgentSessionServices{}, nil, nil, nil)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var starts, ends, settled int
			outcome, err := codingagent.RunHeadless(ctx, runtime, codingagent.HeadlessRunOptions{Messages: []string{"retry please"}, OnEvent: func(event codingagent.AgentSessionEvent) {
				switch e := event.(type) {
				case codingagent.AgentSessionAutoRetryStartEvent:
					starts++
					retrying, err := session.IsRetrying()
					if err != nil || !retrying || session.RetryAttempt() != 1 {
						t.Errorf("retry state: %v %v %d", retrying, err, session.RetryAttempt())
					}
					switch operation {
					case "AbortRetry":
						_ = session.AbortRetry()
					case "Abort":
						_ = session.Abort()
					case "context":
						cancel()
					case "Dispose":
						_ = session.Dispose()
					}
				case codingagent.AgentSessionAutoRetryEndEvent:
					ends++
					if e.Success || e.FinalError == nil || *e.FinalError != "Retry cancelled" {
						t.Errorf("end: %+v", e)
					}
				case codingagent.AgentSessionAgentSettledEvent:
					settled++
				}
			}})
			if err != nil || !outcome.Canceled || outcome.FinalMessage == nil || !reflect.DeepEqual(outcome.Text, []string{"partial"}) {
				t.Fatalf("canceled partial outcome: %+v err=%v", outcome, err)
			}
			if starts != 1 || (operation != "Dispose" && (ends != 1 || settled != 1)) {
				t.Fatalf("events: %d/%d/%d", starts, ends, settled)
			}
			if retrying, _ := session.IsRetrying(); retrying || session.RetryAttempt() != 0 || !session.IsIdle() {
				t.Fatal("retry state did not settle")
			}
			if err := session.WaitForIdle(context.Background()); err != nil {
				t.Fatal(err)
			}
			time.Sleep(80 * time.Millisecond)
			if core.State.CallCount != 1 || len(session.Messages()) != 1 || len(manager.BuildSessionContext().Messages) != 2 {
				t.Fatal("late retry or missing error history")
			}
		})
	}
}

func TestTurnRetrySettingsAndSubsequentPrompt(t *testing.T) {
	dir := t.TempDir()
	settings, err := codingagent.NewSettingsManager(dir, &dir)
	if err != nil {
		t.Fatal(err)
	}
	delay := int64(1)
	if err := settings.ApplyOverrides(codingagent.Settings{Retry: &codingagent.RetrySettings{BaseDelayMS: &delay}}); err != nil {
		t.Fatal(err)
	}
	core, _ := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
	model, _ := core.GetModel()
	failed, _ := ai.FauxAssistantMessage(ai.FauxAssistantText("partial"), ai.FauxAssistantMessageOptions{StopReason: ai.Some(ai.StopReasonError), ErrorMessage: ai.Some("503")})
	created, err := codingagent.CreateAgentSession(context.Background(), codingagent.CreateAgentSessionOptions{CWD: dir, Model: &model, StreamFunction: agent.StreamFunction(core.StreamSimple), SettingsManager: settings, NoTools: codingagent.NoToolsAll})
	if err != nil {
		t.Fatal(err)
	}
	session := created.Session
	defer session.Dispose()
	if !session.AutoRetryEnabled() {
		t.Fatal("retry must default to enabled")
	}
	if err := session.SetAutoRetryEnabled(false); err != nil {
		t.Fatal(err)
	}
	if session.AutoRetryEnabled() {
		t.Fatal("retry did not disable")
	}
	core.SetResponses([]ai.FauxResponseStep{failed})
	if err := session.Prompt(context.Background(), "disabled"); err != nil {
		t.Fatal(err)
	}
	if core.State.CallCount != 1 {
		t.Fatal("disabled retry still ran")
	}
	if err := session.SetAutoRetryEnabled(true); err != nil {
		t.Fatal(err)
	}
	reopened, err := codingagent.NewSettingsManager(dir, &dir)
	if err != nil {
		t.Fatal(err)
	}
	if enabled, err := reopened.GetRetryEnabled(); err != nil || !enabled {
		t.Fatalf("setting not persisted: %v %v", enabled, err)
	}
	core.SetResponses([]ai.FauxResponseStep{failed, ai.FauxResponseFactory(func(ai.Context, *ai.SimpleStreamOptions, *ai.FauxProviderState, ai.Model) (ai.AssistantMessage, error) {
		if retrying, _ := session.IsRetrying(); retrying || session.RetryAttempt() != 1 {
			t.Error("generation is not a backoff wait")
		}
		if err := session.AbortRetry(); err != nil {
			t.Error(err)
		}
		return ai.FauxAssistantMessage(ai.FauxAssistantText("recovered"))
	})})
	runtime := codingagent.NewAgentSessionRuntime(session, codingagent.AgentSessionServices{}, nil, nil, nil)
	outcome, err := codingagent.RunHeadless(context.Background(), runtime, codingagent.HeadlessRunOptions{Messages: []string{"enabled"}})
	if err != nil || outcome.Canceled || !reflect.DeepEqual(outcome.Text, []string{"recovered"}) || session.RetryAttempt() != 0 {
		t.Fatalf("subsequent prompt: %+v %v", outcome, err)
	}
}

func TestTurnRetryObserverSnapshots(t *testing.T) {
	for _, kind := range []string{"agent_end", "auto_retry_end"} {
		t.Run(kind, func(t *testing.T) {
			failed, _ := ai.FauxAssistantMessage(ai.FauxAssistantText("partial"), ai.FauxAssistantMessageOptions{StopReason: ai.Some(ai.StopReasonError), ErrorMessage: ai.Some("503")})
			runtime := newHeadlessFauxRuntime(t, []ai.FauxResponseStep{failed, failed}, nil, nil)
			session := runtime.Session()
			delay, retries := int64(1), 1
			if err := session.SettingsManager().ApplyOverrides(codingagent.Settings{Retry: &codingagent.RetrySettings{BaseDelayMS: &delay, MaxRetries: &retries}}); err != nil {
				t.Fatal(err)
			}
			_, _ = session.Subscribe(func(event codingagent.AgentSessionEvent) {
				if e, ok := event.(codingagent.AgentSessionAgentEndEvent); ok && kind == "agent_end" && e.WillRetry {
					_ = session.SetAutoRetryEnabled(false)
				}
				if e, ok := event.(codingagent.AgentSessionAutoRetryEndEvent); ok && e.FinalError != nil {
					*e.FinalError = "listener mutation"
				}
			})
			observed := false
			_, _ = session.Subscribe(func(event codingagent.AgentSessionEvent) {
				if e, ok := event.(codingagent.AgentSessionAgentEndEvent); ok && kind == "agent_end" {
					observed = true
					if !e.WillRetry {
						t.Error("listeners observed different willRetry snapshots")
					}
				}
				if e, ok := event.(codingagent.AgentSessionAutoRetryEndEvent); ok && kind == "auto_retry_end" {
					observed = true
					if e.FinalError == nil || *e.FinalError != "503" {
						t.Errorf("mutable retry event: %+v", e)
					}
				}
			})
			if err := session.Prompt(context.Background(), "retry please"); err != nil {
				t.Fatal(err)
			}
			if !observed {
				t.Fatal("missing observed event")
			}
		})
	}
}

func TestTurnRetryRejectsDelayOverflow(t *testing.T) {
	failed, _ := ai.FauxAssistantMessage(ai.FauxAssistantText("partial"), ai.FauxAssistantMessageOptions{StopReason: ai.Some(ai.StopReasonError), ErrorMessage: ai.Some("503")})
	runtime := newHeadlessFauxRuntime(t, []ai.FauxResponseStep{failed, failed}, nil, nil)
	session := runtime.Session()
	delay := int64(1 << 62)
	if err := session.SettingsManager().ApplyOverrides(codingagent.Settings{Retry: &codingagent.RetrySettings{BaseDelayMS: &delay}}); err != nil {
		t.Fatal(err)
	}
	starts, ends := 0, 0
	outcome, err := codingagent.RunHeadless(context.Background(), runtime, codingagent.HeadlessRunOptions{Messages: []string{"retry please"}, OnEvent: func(event codingagent.AgentSessionEvent) {
		switch event.(type) {
		case codingagent.AgentSessionAutoRetryStartEvent:
			starts++
		case codingagent.AgentSessionAutoRetryEndEvent:
			ends++
		}
	}})
	if err == nil || err.Error() != "retry delay exceeds int64 milliseconds" || outcome.Canceled || starts != 1 || ends != 1 {
		t.Fatalf("overflow: %+v %v, starts=%d ends=%d", outcome, err, starts, ends)
	}
	if retrying, _ := session.IsRetrying(); retrying || session.RetryAttempt() != 0 || !session.IsIdle() {
		t.Fatal("overflow did not settle")
	}
}

func TestTurnRetryPreservesTrailingFrameProtocolError(t *testing.T) {
	core, _ := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
	model, _ := core.GetModel()
	model.API = "openai-completions"
	model.Provider = "deepseek"
	model.BaseURL = "https://example.invalid"
	key := "fixture"
	calls := 0
	stream := func(ctx context.Context, model ai.Model, input ai.Context, _ ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
		return ai.StreamOpenAICompletions(ctx, model, input, ai.OpenAICompletionsOptions{StreamOptions: ai.StreamOptions{ProviderRequestOptions: ai.ProviderRequestOptions{APIKey: &key, Fetch: func(context.Context, ai.FetchRequest) (ai.FetchResponse, error) {
			calls++
			return ai.FetchResponse{Status: 200, Body: []byte("data: {\"choices\":[{\"delta\":{\"content\":\"done\"},\"finish_reason\":\"stop\"}]}\n\ndata: {\"choices\":[")}, nil
		}}}})
	}
	delay := int64(1)
	settings, _ := codingagent.NewInMemorySettingsManager(codingagent.Settings{Retry: &codingagent.RetrySettings{BaseDelayMS: &delay}})
	created, err := codingagent.CreateAgentSession(context.Background(), codingagent.CreateAgentSessionOptions{CWD: t.TempDir(), Model: &model, StreamFunction: stream, SettingsManager: settings, NoTools: codingagent.NoToolsAll})
	if err != nil {
		t.Fatal(err)
	}
	defer created.Session.Dispose()
	err = created.Session.Prompt(context.Background(), "retry please")
	if !errors.Is(err, ai.ErrOpenAISSETruncated) || calls != 1 {
		t.Fatalf("trailing protocol error was retried: err=%v calls=%d", err, calls)
	}
}

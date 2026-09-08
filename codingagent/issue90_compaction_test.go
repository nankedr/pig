package codingagent_test

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
	"github.com/nankedr/pig/internal/baseline"
	"github.com/nankedr/pig/internal/parity"
)

type auto90Response struct {
	Input                         *int64
	Output, CacheRead, CacheWrite int64
	Reason                        ai.StopReason
	Error                         string
	OtherModel                    bool
}

func auto90Assistant(model ai.Model, text string, opts auto90Response) ai.AssistantMessage {
	input := int64(20)
	if opts.Input != nil {
		input = *opts.Input
	}
	reason := opts.Reason
	if reason == "" {
		reason = ai.StopReasonStop
		if opts.Error != "" {
			reason = ai.StopReasonError
		}
	}
	m := ai.AssistantMessage{Role: ai.MessageRoleAssistant, Content: []ai.AssistantContent{ai.TextContent{Type: ai.ContentTypeText, Text: text}}, API: model.API, Provider: model.Provider, Model: model.ID, StopReason: reason, Timestamp: time.Now().UnixMilli(), Usage: ai.Usage{Input: input, Output: opts.Output, CacheRead: opts.CacheRead, CacheWrite: opts.CacheWrite, TotalTokens: input + opts.Output + opts.CacheRead + opts.CacheWrite}}
	if opts.OtherModel {
		m.Provider = "other"
	}
	if opts.Error != "" {
		m.ErrorMessage = ai.Some(opts.Error)
	}
	return m
}
func auto90Project(m agent.AgentMessage) map[string]any {
	result := compact89Project(m)
	if a, ok := m.(ai.AssistantMessage); ok {
		result["stopReason"] = a.StopReason
		result["error"], _ = a.ErrorMessage.Value()
	}
	return result
}
func TestAutoCompactionParity(t *testing.T) {
	root := issue32RepoRoot(t)
	lock, _, err := baseline.Load(filepath.Join(root, "parity/baseline"))
	if err != nil {
		t.Fatal(err)
	}
	locked := parity.Baseline{ID: lock.BaselineID, Commit: lock.Upstream.Commit, Repository: lock.Upstream.Repository}
	fixture, err := parity.LoadFixture(filepath.Join(root, "parity/oracle/fixtures/auto-compaction.json"), locked)
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
				Name                                                                    string
				Responses                                                               []auto90Response
				Seed                                                                    auto90Response
				SummaryErrors                                                           []string
				Disabled, RetryDisabled, Queue, QueueEnd, QueueAll, Stale, SecondPrompt bool
				QueueCount                                                              int
				Keep                                                                    int64
			}
		}
		data, _ := json.Marshal(c.Input)
		if err := json.Unmarshal(data, &input); err != nil {
			return parity.Observation{}, err
		}
		runs := []any{}
		for _, scenario := range input.Scenarios {
			core, _ := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
			model, _ := core.GetModel()
			model.ContextWindow = 1000
			model.MaxTokens = 100
			model.Reasoning = false
			manager := codingagent.NewInMemorySessionManager(t.TempDir())
			old := auto90Assistant(model, "old answer", auto90Response{})
			old.Timestamp = 1
			recent := auto90Assistant(model, "recent answer", scenario.Seed)
			recent.Timestamp = 1
			for _, m := range []agent.AgentMessage{ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText("old request"), Timestamp: 1}, old, ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText("recent request long enough to keep"), Timestamp: 1}, recent} {
				if _, err := manager.AppendMessage(m); err != nil {
					return parity.Observation{}, err
				}
			}
			if scenario.Stale {
				if _, err := manager.AppendCompaction("prior checkpoint", manager.GetEntries()[2].ID, 950); err != nil {
					return parity.Observation{}, err
				}
			}
			keep := scenario.Keep
			if keep == 0 {
				keep = 10
			}
			settings, _ := codingagent.NewInMemorySettingsManager(codingagent.Settings{Compaction: &codingagent.CompactionSettings{Enabled: !scenario.Disabled, ReserveTokens: 100, KeepRecentTokens: keep}, Retry: &codingagent.RetrySettings{Enabled: pointerTo(!scenario.RetryDisabled), MaxRetries: pointerTo(2), BaseDelayMS: pointerTo(int64(1))}})
			requests, events := []any{}, []any{}
			generations, summaries := 0, 0
			stream := func(ctx context.Context, m ai.Model, c ai.Context, o ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
				system, _ := c.SystemPrompt.Value()
				summary := strings.HasPrefix(system, "You are a context summarization assistant.")
				kind := "generation"
				if summary {
					kind = "summary"
				}
				messages := []any{}
				for _, m := range c.Messages {
					messages = append(messages, auto90Project(m))
				}
				requests = append(requests, map[string]any{"kind": kind, "messages": messages})
				if len(requests) > 15 {
					t.Fatalf("%s: unbounded recovery", scenario.Name)
				}
				opts := auto90Response{}
				text := "checkpoint"
				if summary {
					if summaries < len(scenario.SummaryErrors) {
						opts.Error = scenario.SummaryErrors[summaries]
					}
					summaries++
				} else {
					if generations < len(scenario.Responses) {
						opts = scenario.Responses[generations]
					}
					generations++
					text = fmt.Sprintf("answer-%d", generations)
				}
				time.Sleep(3 * time.Millisecond)
				response := auto90Assistant(model, text, opts)
				result := ai.NewAssistantMessageEventStream()
				result.End(response)
				return result
			}
			created, err := codingagent.CreateAgentSession(ctx, codingagent.CreateAgentSessionOptions{CWD: manager.GetCWD(), Model: &model, SessionManager: manager, SettingsManager: settings, StreamFunction: stream, Tools: []string{}})
			if err != nil {
				return parity.Observation{}, err
			}
			s := created.Session
			if scenario.QueueAll {
				s.SetSteeringMode(agent.QueueAll)
				s.SetFollowUpMode(agent.QueueAll)
			}
			queued := false
			s.Subscribe(func(e codingagent.AgentSessionEvent) {
				kind := string(e.AgentSessionEventType())
				v := map[string]any{"type": kind}
				switch e := e.(type) {
				case codingagent.AgentSessionMessageStartEvent:
					v["message"] = auto90Project(e.Message)
				case codingagent.AgentSessionMessageEndEvent:
					v["message"] = auto90Project(e.Message)
				case codingagent.AgentSessionAgentEndEvent:
					v["willRetry"] = e.WillRetry
				case codingagent.AgentSessionCompactionStartEvent:
					v["reason"] = e.Reason
				case codingagent.AgentSessionCompactionEndEvent:
					v["reason"] = e.Reason
					v["aborted"] = e.Aborted
					v["willRetry"] = e.WillRetry
					v["error"] = ""
					v["summary"] = ""
					if e.ErrorMessage != nil {
						v["error"] = *e.ErrorMessage
					}
					if e.Result != nil {
						v["summary"] = e.Result.Summary
					}
				case codingagent.AgentSessionAutoRetryStartEvent:
					v["attempt"] = e.Attempt
					v["maxAttempts"] = e.MaxAttempts
					v["delayMs"] = e.DelayMS
					v["errorMessage"] = e.ErrorMessage
				case codingagent.AgentSessionAutoRetryEndEvent:
					v["attempt"] = e.Attempt
					v["success"] = e.Success
					v["error"] = ""
					if e.FinalError != nil {
						v["error"] = *e.FinalError
					}
				case codingagent.AgentSessionSummarizationRetryScheduledEvent:
					v["attempt"] = e.Attempt
					v["maxAttempts"] = e.MaxAttempts
					v["delayMs"] = e.DelayMS
					v["errorMessage"] = e.ErrorMessage
				case codingagent.AgentSessionCompactionRetryAttemptStartEvent:
					v["source"] = e.Source
					v["reason"] = e.Reason
				}
				if kind != "message_update" && kind != "queue_update" {
					events = append(events, v)
				}
				queueEvent := "compaction_start"
				if scenario.QueueEnd {
					queueEvent = "compaction_end"
				}
				if scenario.Queue && !queued && kind == queueEvent {
					queued = true
					for i := 0; i < max(1, scenario.QueueCount); i++ {
						if err := s.Steer("steering"); err != nil {
							t.Error(err)
						}
						if err := s.FollowUp("follow-up"); err != nil {
							t.Error(err)
						}
					}
				}
			})
			errorText := ""
			if err := s.Prompt(ctx, "go"); err != nil {
				errorText = strings.ToLower(err.Error())
			}
			if scenario.SecondPrompt {
				if err := s.Prompt(ctx, "next"); err != nil {
					return parity.Observation{}, err
				}
			}
			messages, entries := []any{}, []string{}
			for _, m := range s.Messages() {
				messages = append(messages, auto90Project(m))
			}
			for _, e := range manager.GetEntries() {
				entries = append(entries, e.Type)
			}
			pending, _ := s.PendingMessageCount()
			runs = append(runs, map[string]any{"name": scenario.Name, "error": errorText, "requests": requests, "events": events, "messages": messages, "entries": entries, "pending": pending})
			s.Dispose()
		}
		out, _ := json.Marshal(map[string]any{"runs": runs})
		effects := []parity.SideEffect{}
		return parity.Observation{Outcome: out, SideEffects: &effects}, nil
	}})
	if err != nil || !result.Match {
		t.Fatalf("%v\nwant: %s\ngot: %s", err, fixture.Observation.Outcome, result.Pig.Outcome)
	}
}

package codingagent_test

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
	"github.com/nankedr/pig/internal/baseline"
	"github.com/nankedr/pig/internal/parity"
)

func TestManualCompactionParity(t *testing.T) {
	root := issue32RepoRoot(t)
	lock, _, err := baseline.Load(filepath.Join(root, "parity/baseline"))
	if err != nil {
		t.Fatal(err)
	}
	locked := parity.Baseline{ID: lock.BaselineID, Commit: lock.Upstream.Commit, Repository: lock.Upstream.Repository}
	fixture, err := parity.LoadFixture(filepath.Join(root, "parity/oracle/fixtures/manual-compaction.json"), locked)
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
				Name, Thinking                                          string
				Errors                                                  []string
				PrefixOnly, Files, Update, Disabled, Cancel, ToolResult bool
				Keep, Reserve, MaxTokens                                int64
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
			model.Reasoning = true
			model.MaxTokens = scenario.MaxTokens
			manager := codingagent.NewInMemorySessionManager(t.TempDir())
			usage := ai.Usage{Input: 80, Output: 20, TotalTokens: 100}
			assistant := func(text string) ai.AssistantMessage {
				m, _ := ai.FauxAssistantMessage(ai.FauxAssistantText(text))
				m.Usage = usage
				m.Timestamp = 1
				return m
			}
			seed := []agent.AgentMessage{ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText("old request"), Timestamp: 1}, assistant("old answer"), ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText("recent request long enough to keep"), Timestamp: 1}, assistant("recent answer")}
			if scenario.ToolResult {
				call := assistant("")
				call.StopReason = ai.StopReasonToolUse
				call.Content = []ai.AssistantContent{ai.ToolCall{Type: ai.ContentTypeToolCall, ID: "read-call", Name: "read", Arguments: map[string]any{"path": "context.go"}}}
				seed = append(seed[:3], call, ai.ToolResultMessage{Role: ai.MessageRoleToolResult, ToolCallID: "read-call", ToolName: "read", Content: []ai.ToolResultContent{ai.TextContent{Type: ai.ContentTypeText, Text: strings.Repeat("x", 5000)}}, Timestamp: 1}, seed[3])
			}
			if scenario.PrefixOnly {
				seed = seed[2:]
			}
			if scenario.Files {
				m := seed[1].(ai.AssistantMessage)
				for i, name := range []string{"read", "edit", "read", "write"} {
					m.Content = append(m.Content, ai.ToolCall{Type: ai.ContentTypeToolCall, ID: fmt.Sprint("call", i), Name: name, Arguments: map[string]any{"path": []string{"shared.go", "shared.go", "only-read.go", "written.go"}[i]}})
				}
				seed[1] = m
			}
			for i, m := range seed {
				if _, err := manager.AppendMessage(m); err != nil {
					return parity.Observation{}, err
				}
				if scenario.Update && i == 1 {
					_, err := manager.AppendCompaction("previous checkpoint", manager.GetEntries()[1].ID, 100, codingagent.AppendCompactionOptions{Details: json.RawMessage(`{"readFiles":["prior-read.go"],"modifiedFiles":["prior-edit.go"]}`), FromHook: pointerTo(false), Usage: &usage})
					if err != nil {
						return parity.Observation{}, err
					}
				}
			}
			ids := manager.GetEntries()
			settings, _ := codingagent.NewInMemorySettingsManager(codingagent.Settings{Retry: &codingagent.RetrySettings{Enabled: pointerTo(!scenario.Disabled), MaxRetries: pointerTo(2), BaseDelayMS: pointerTo(int64(1))}, Compaction: &codingagent.CompactionSettings{ReserveTokens: scenario.Reserve, KeepRecentTokens: scenario.Keep}})
			requests, events := []any{}, []any{}
			stream := func(ctx context.Context, m ai.Model, c ai.Context, o ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
				system, _ := c.SystemPrompt.Value()
				messages := []any{}
				for _, msg := range c.Messages {
					messages = append(messages, compact89Project(msg))
				}
				reasoning := ""
				if o.Reasoning != nil {
					reasoning = string(*o.Reasoning)
				}
				requests = append(requests, map[string]any{"system": system, "messages": messages, "maxTokens": o.MaxTokens, "reasoning": reasoning, "cacheRetention": o.CacheRetention, "freshSession": o.SessionID != nil && *o.SessionID != "" && *o.SessionID != manager.GetSessionID()})
				result := ai.NewAssistantMessageEventStream()
				response := assistant("checkpoint")
				if len(requests) <= len(scenario.Errors) && scenario.Errors[len(requests)-1] != "" {
					response.StopReason = ai.StopReasonError
					response.ErrorMessage = ai.Some(scenario.Errors[len(requests)-1])
				}
				result.End(response)
				return result
			}
			created, err := codingagent.CreateAgentSession(ctx, codingagent.CreateAgentSessionOptions{CWD: manager.GetCWD(), Model: &model, SessionManager: manager, SettingsManager: settings, StreamFunction: stream, ThinkingLevel: agent.ThinkingLevel(scenario.Thinking), Tools: []string{}})
			if err != nil {
				return parity.Observation{}, err
			}
			s := created.Session
			s.Subscribe(func(e codingagent.AgentSessionEvent) {
				kind := string(e.AgentSessionEventType())
				if scenario.Cancel && kind == "summarization_retry_scheduled" {
					_ = s.AbortCompaction()
				}
				if strings.HasPrefix(kind, "compaction_") || strings.HasPrefix(kind, "summarization_retry") {
					active, _ := s.IsCompacting()
					v := map[string]any{"type": kind, "compacting": active}
					if end, ok := e.(codingagent.AgentSessionCompactionEndEvent); ok {
						text := ""
						if end.ErrorMessage != nil {
							text = *end.ErrorMessage
						}
						v["aborted"] = end.Aborted
						v["willRetry"] = end.WillRetry
						v["error"] = text
					}
					switch event := e.(type) {
					case codingagent.AgentSessionSummarizationRetryScheduledEvent:
						v["attempt"] = event.Attempt
						v["maxAttempts"] = event.MaxAttempts
						v["delayMs"] = event.DelayMS
						v["errorMessage"] = event.ErrorMessage
					case codingagent.AgentSessionCompactionRetryAttemptStartEvent:
						v["source"] = event.Source
						v["reason"] = event.Reason
					}
					events = append(events, v)
				}
			})
			compact, e := s.Compact(ctx, "preserve constraints")
			errorText := ""
			var value any
			if e != nil {
				errorText = e.Error()
				if scenario.Cancel {
					errorText = "cancelled"
				}
			} else {
				index := -1
				for i, entry := range ids {
					if entry.ID == compact.FirstKeptEntryID {
						index = i
					}
				}
				value = map[string]any{"summary": compact.Summary, "firstKeptIndex": index, "tokensBefore": compact.TokensBefore, "estimatedTokensAfter": compact.EstimatedTokensAfter, "usage": compact.Usage, "details": compact.Details}
			}
			messages, entries := []any{}, []string{}
			for _, m := range s.Messages() {
				messages = append(messages, compact89Project(m))
			}
			for _, e := range manager.GetEntries() {
				entries = append(entries, e.Type)
			}
			runs = append(runs, map[string]any{"name": scenario.Name, "requests": requests, "events": events, "error": errorText, "result": value, "messages": messages, "entries": entries})
			s.Dispose()
		}
		out, _ := json.Marshal(map[string]any{"runs": runs})
		effects := []parity.SideEffect{}
		return parity.Observation{Outcome: out, SideEffects: &effects}, nil
	}})
	if err != nil {
		t.Fatalf("%v\nwant: %s\ngot: %s", err, fixture.Observation.Outcome, result.Pig.Outcome)
	}
	if !result.Match {
		t.Fatalf("manual compaction parity: %+v", result)
	}
}
func compact89Project(m agent.AgentMessage) map[string]any {
	text := ""
	switch v := m.(type) {
	case agent.CompactionSummaryMessage:
		text = v.Summary
	case ai.UserMessage:
		text, _ = v.Content.Text()
		if text == "" {
			blocks, _ := v.Content.Blocks()
			for _, b := range blocks {
				if b, ok := b.(ai.TextContent); ok {
					text += b.Text
				}
			}
		}
	case ai.ToolResultMessage:
		for _, b := range v.Content {
			if b, ok := b.(ai.TextContent); ok {
				text += b.Text
			}
		}
	case ai.AssistantMessage:
		for _, b := range v.Content {
			if b, ok := b.(ai.TextContent); ok {
				text += b.Text
			}
		}
	}
	return map[string]any{"role": m.MessageRole(), "text": text}
}

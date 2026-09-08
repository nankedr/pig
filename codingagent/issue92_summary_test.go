package codingagent_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
	"github.com/nankedr/pig/internal/baseline"
	"github.com/nankedr/pig/internal/parity"
)

func TestBranchSummaryParity(t *testing.T) {
	root := issue32RepoRoot(t)
	lock, _, err := baseline.Load(filepath.Join(root, "parity/baseline"))
	if err != nil {
		t.Fatal(err)
	}
	locked := parity.Baseline{ID: lock.BaselineID, Commit: lock.Upstream.Commit, Repository: lock.Upstream.Repository}
	fixture, err := parity.LoadFixture(filepath.Join(root, "parity/oracle/fixtures/branch-summary.json"), locked)
	if err != nil {
		t.Fatal(err)
	}
	oracle, err := parity.NewFixtureDriver(fixture, locked)
	if err != nil {
		t.Fatal(err)
	}
	result, err := parity.RunCase(context.Background(), fixture.Case, oracle, parity.DriverFunc{SurfaceName: parity.SurfaceGoSDK, ObserveFunc: func(ctx context.Context, c parity.Case) (parity.Observation, error) {
		var input struct {
			History   []json.RawMessage
			Scenarios []struct {
				Name, Target, Label, Instructions, Start                                 string
				Metadata, Hook, Compaction, Custom, Roundtrip, LongHistory, SmallSummary bool
				Window                                                                   int64
				Replace, Abort, AbortRetry                                               bool
				Errors                                                                   []string
				Reserve                                                                  int64
			}
		}
		if err := json.Unmarshal(c.Input, &input); err != nil {
			return parity.Observation{}, err
		}
		runs := []any{}
		for _, scenario := range input.Scenarios {
			runtime, models, _ := config87Runtime(t)
			models[0].MaxTokens = 4096
			models[0].ContextWindow = scenario.Window
			models[0].Cost = ai.ModelCost{}
			dir := t.TempDir()
			file := filepath.Join(dir, "session.jsonl")
			header, _ := json.Marshal(map[string]any{"type": "session", "version": 3, "id": "history", "cwd": dir, "timestamp": "2026-01-01T00:00:00Z"})
			data := append(header, '\n')

			for _, e := range input.History {
				var entry map[string]any
				if err := json.Unmarshal(e, &entry); err != nil {
					t.Fatal(err)
				}
				if scenario.SmallSummary {
					if entry["id"] == "explore" || entry["id"] == "tools" || entry["id"] == "toolresult" {
						continue
					}
					if entry["id"] == "prior" {
						entry["parentId"] = "shared"
					}
					if entry["id"] == "leaf" {
						entry["parentId"] = "prior"
						entry["message"].(map[string]any)["content"] = []any{map[string]any{"type": "text", "text": "abcdefgh"}}
					}
				}
				if entry["id"] == "explore" && scenario.LongHistory {
					entry["message"].(map[string]any)["content"] = []any{map[string]any{"type": "text", "text": strings.Repeat("old exploration ", 300)}}
				}
				if entry["id"] == "prior" {
					if scenario.Hook {
						entry["fromHook"] = true
					}
					if scenario.Compaction {
						entry["type"] = "compaction"
						entry["firstKeptEntryId"] = "tools"
						entry["tokensBefore"] = 100
					}
				}
				if entry["id"] == "target" && scenario.Custom {
					entry["type"] = "custom_message"
					entry["customType"] = "note"
					entry["content"] = []any{map[string]any{"type": "text", "text": "custom target"}}
					entry["display"] = true
				}
				encoded, err := json.Marshal(entry)
				if err != nil {
					t.Fatal(err)
				}
				data = append(data, encoded...)
				data = append(data, '\n')
			}
			if scenario.Metadata {
				data = append(data, []byte(`{"type":"custom","id":"metadata","parentId":"leaf","timestamp":"2026-01-01T00:00:00Z","customType":"state","data":{}}`+"\n")...)
			}

			if err := os.WriteFile(file, data, 0600); err != nil {
				return parity.Observation{}, err
			}
			manager, err := codingagent.OpenSessionManager(file, nil, nil)
			if err != nil {
				return parity.Observation{}, err
			}
			settingsJSON, _ := json.Marshal(map[string]any{"branchSummary": map[string]any{"reserveTokens": scenario.Reserve}, "retry": map[string]any{"enabled": true, "maxRetries": 2, "baseDelayMs": 1}})
			var settingsValue codingagent.Settings
			if err := json.Unmarshal(settingsJSON, &settingsValue); err != nil {
				t.Fatal(err)
			}
			settings, _ := codingagent.NewInMemorySettingsManager(settingsValue)
			core, _ := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
			created, err := codingagent.CreateAgentSession(ctx, codingagent.CreateAgentSessionOptions{CWD: dir, Model: &models[0], ModelRuntime: runtime, SessionManager: manager, SettingsManager: settings, ThinkingLevel: "high", Tools: []string{}, StreamFunction: agent.StreamFunction(core.StreamSimple)})
			if err != nil {
				return parity.Observation{}, err
			}
			s := created.Session
			defer s.Dispose()
			requests, events := []any{}, []any{}
			routing := true
			firstRoute := ""
			_, _ = s.Subscribe(func(e codingagent.AgentSessionEvent) {
				if strings.HasPrefix(string(e.AgentSessionEventType()), "summarization_") || strings.HasPrefix(string(e.AgentSessionEventType()), "compaction_") {
					events = append(events, e)
					if scenario.AbortRetry && e.AgentSessionEventType() == codingagent.AgentSessionEventTypeSummarizationRetryScheduled {
						_ = s.AbortBranchSummary()
					}
				}
			})
			messages := func(c ai.Context) []map[string]string {
				ms := []agent.AgentMessage{}
				for _, m := range c.Messages {
					ms = append(ms, m)
				}
				return tree91Messages(ms)
			}
			steps := []ai.FauxResponseStep{}
			for i := 0; i <= len(scenario.Errors); i++ {
				steps = append(steps, ai.FauxResponseFactory(func(c ai.Context, o *ai.SimpleStreamOptions, _ *ai.FauxProviderState, _ ai.Model) (ai.AssistantMessage, error) {
					system, _ := c.SystemPrompt.Value()
					requests = append(requests, map[string]any{"system": system, "messages": messages(c), "maxTokens": o.MaxTokens, "cache": o.CacheRetention, "reasoning": o.Reasoning, "apiKey": o.APIKey})
					if o.SessionID != nil {
						if firstRoute == "" {
							firstRoute = *o.SessionID
						}
						routing = routing && *o.SessionID != "" && *o.SessionID == firstRoute
					} else {
						routing = false
					}
					m, _ := ai.FauxAssistantMessage(ai.FauxAssistantText("summary result"))
					m.Usage = ai.Usage{Input: 80, Output: 20, TotalTokens: 100}
					if scenario.Abort {
						_ = s.AbortBranchSummary()
						m.StopReason = ai.StopReasonAborted
					}
					if i < len(scenario.Errors) {
						m.StopReason = ai.StopReasonError
						m.ErrorMessage = ai.Some(scenario.Errors[i])
					}
					return m, nil
				}))
			}
			core.SetResponses(steps)
			if scenario.Start != "" {
				if _, err := s.NavigateTree(ctx, scenario.Start); err != nil {
					return parity.Observation{}, err
				}
			}
			before := len(manager.GetEntries())
			r, err := s.NavigateTree(ctx, scenario.Target, codingagent.NavigateTreeOptions{Summarize: true, Label: scenario.Label, CustomInstructions: scenario.Instructions, ReplaceInstructions: scenario.Replace})
			var errorText any
			if err != nil {
				errorText = err.Error()
			}
			var summary any
			if e := r.SummaryEntry; e != nil {
				summary = map[string]any{"parent": e.ParentID, "from": e.FromID, "text": e.Summary, "details": e.Details, "usage": e.Usage, "fromHook": e.FromHook, "label": manager.GetLabel(e.ID)}
			}
			state := map[string]any{"error": errorText, "editorText": r.EditorText, "cancelled": r.Cancelled, "aborted": r.Aborted, "summary": summary, "messages": tree91Messages(s.Messages()), "added": len(manager.GetEntries()) - before, "original": manager.GetEntry("leaf") != nil}
			reopened, err := codingagent.OpenSessionManager(file, nil, nil)
			if err != nil {
				return parity.Observation{}, err
			}
			restored, err := codingagent.CreateAgentSession(ctx, codingagent.CreateAgentSessionOptions{CWD: dir, ModelRuntime: runtime, SessionManager: reopened, SettingsManager: settings, Tools: []string{}})
			if err != nil {
				return parity.Observation{}, err
			}
			persisted := tree91Messages(restored.Session.Messages())
			_ = restored.Session.Dispose()
			continuation := []map[string]string{}
			core.SetResponses([]ai.FauxResponseStep{ai.FauxResponseFactory(func(c ai.Context, _ *ai.SimpleStreamOptions, _ *ai.FauxProviderState, _ ai.Model) (ai.AssistantMessage, error) {
				continuation = messages(c)
				return ai.FauxAssistantMessage(ai.FauxAssistantText("continued"))
			})})
			if err := s.Prompt(ctx, "continue target"); err != nil {
				return parity.Observation{}, err
			}

			var roundtrip any
			if scenario.Roundtrip {
				var request []map[string]string
				core.SetResponses([]ai.FauxResponseStep{ai.FauxResponseFactory(func(c ai.Context, _ *ai.SimpleStreamOptions, _ *ai.FauxProviderState, _ ai.Model) (ai.AssistantMessage, error) {
					request = messages(c)
					m, _ := ai.FauxAssistantMessage(ai.FauxAssistantText("return summary"))
					m.Usage = ai.Usage{Input: 80, Output: 20, TotalTokens: 100}
					return m, nil
				})})
				next, err := s.NavigateTree(ctx, "leaf", codingagent.NavigateTreeOptions{Summarize: true})
				if err != nil {
					return parity.Observation{}, err
				}
				count := len(manager.GetEntries())
				if _, err := s.NavigateTree(ctx, *manager.GetLeafID(), codingagent.NavigateTreeOptions{Summarize: true}); err != nil {
					return parity.Observation{}, err
				}
				roundtrip = map[string]any{"request": request, "summary": next.SummaryEntry.Summary, "details": next.SummaryEntry.Details, "noDuplicate": len(manager.GetEntries()) == count, "original": manager.GetEntry("other") != nil}
			} else if _, err := s.NavigateTree(ctx, "leaf"); err != nil {
				return parity.Observation{}, err
			}

			runs = append(runs, map[string]any{"name": scenario.Name, "roundtrip": roundtrip, "requests": requests, "events": events, "state": state, "persisted": persisted, "continuation": continuation, "returned": tree91Messages(s.Messages()), "routing": routing})
		}
		outcome, _ := json.Marshal(runs)
		return parity.Observation{Outcome: outcome, SideEffects: &[]parity.SideEffect{}}, nil
	}})
	if err != nil || !result.Match {
		t.Fatalf("branch summary parity: %v\n%+v\nwant %s\ngot %s", err, result.Differences, fixture.Observation.Outcome, result.Pig.Outcome)
	}
}

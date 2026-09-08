package codingagent_test

import (
	"context"
	"encoding/json"
	"fmt"
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

type bash93Operations func(context.Context, string, string, codingagent.BashExecOptions) (codingagent.BashExecResult, error)

func (f bash93Operations) Exec(ctx context.Context, command, cwd string, o codingagent.BashExecOptions) (codingagent.BashExecResult, error) {
	return f(ctx, command, cwd, o)
}
func bash93History(messages []agent.AgentMessage) []map[string]any {
	out := []map[string]any{}
	for _, m := range messages {
		if m.MessageRole() == "bashExecution" {
			var b agent.BashExecutionMessage
			raw, _ := agent.MarshalAgentMessage(m)
			json.Unmarshal(raw, &b)
			var fields map[string]json.RawMessage
			json.Unmarshal(raw, &fields)
			b.ExitCodeSet = len(fields["exitCode"]) > 0 && string(fields["exitCode"]) != "null"
			var code *int
			if b.ExitCodeSet {
				v := b.ExitCode
				code = &v
			}
			out = append(out, map[string]any{"role": b.Role, "command": b.Command, "output": b.Output, "exitCode": code, "cancelled": b.Cancelled, "truncated": b.Truncated, "exclude": b.ExcludeFromContext})
		} else {
			out = append(out, map[string]any{"role": m.MessageRole(), "text": message85Text(m)})
		}
	}
	return out
}
func TestSessionBashParity(t *testing.T) {
	root, _ := filepath.Abs("..")
	lock, _, err := baseline.Load(filepath.Join(root, "parity/baseline"))
	if err != nil {
		t.Fatal(err)
	}
	pinned := parity.Baseline{ID: lock.BaselineID, Commit: lock.Upstream.Commit, Repository: lock.Upstream.Repository}
	fixture, err := parity.LoadFixture(filepath.Join(root, "parity/oracle/fixtures/session-bash.json"), pinned)
	if err != nil {
		t.Fatal(err)
	}
	oracle, err := parity.NewFixtureDriver(fixture, pinned)
	if err != nil {
		t.Fatal(err)
	}
	result, err := parity.RunCase(context.Background(), fixture.Case, oracle, parity.DriverFunc{SurfaceName: parity.SurfaceGoSDK, ObserveFunc: func(ctx context.Context, c parity.Case) (parity.Observation, error) {
		var input struct {
			Commands []struct {
				Command                  string
				Chunks                   []json.RawMessage
				Code                     int
				ID                       *string
				Exclude, Cancel, Failure bool
			}
		}
		if err := json.Unmarshal(c.Input, &input); err != nil {
			return parity.Observation{}, err
		}
		dir := t.TempDir()
		manager, err := codingagent.NewSessionManager(dir, &dir)
		if err != nil {
			return parity.Observation{}, err
		}
		settings, err := codingagent.NewInMemorySettingsManager(codingagent.Settings{})
		if err != nil {
			return parity.Observation{}, err
		}
		if err := settings.SetShellCommandPrefix("prefix"); err != nil {
			return parity.Observation{}, err
		}
		core, _ := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
		model, _ := core.GetModel()
		reply, _ := ai.FauxAssistantMessage(ai.FauxAssistantText("reply"), ai.FauxAssistantMessageOptions{Timestamp: ai.Some(int64(2))})
		core.SetResponses([]ai.FauxResponseStep{reply, reply})
		events, results, requests := []map[string]any{}, []map[string]any{}, [][]map[string]any{}
		var session *codingagent.AgentSession
		var deferred, settlement map[string]any
		created, err := codingagent.CreateAgentSession(ctx, codingagent.CreateAgentSessionOptions{CWD: dir, AgentDir: dir, SessionManager: manager, SettingsManager: settings, Model: &model, NoTools: codingagent.NoToolsAll, StreamFunction: func(ctx context.Context, m ai.Model, in ai.Context, o ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
			messages := []agent.AgentMessage{}
			for _, m := range in.Messages {
				messages = append(messages, m)
			}
			requests = append(requests, bash93History(messages))
			if len(requests) == 1 {
				code := 0
				if err := session.RecordBashResult("during", codingagent.BashResult{Output: "queued", ExitCode: &code}); err != nil {
					t.Error(err)
				}
				pending, err := session.HasPendingBashMessages()
				if err != nil {
					t.Error(err)
				}
				deferred = map[string]any{"pending": pending, "history": bash93History(session.Messages())}
			}
			return core.StreamSimple(ctx, m, in, o)
		}})
		if err != nil {
			return parity.Observation{}, err
		}
		session = created.Session
		defer session.Dispose()
		session.Subscribe(func(e codingagent.AgentSessionEvent) {
			if e, ok := e.(codingagent.AgentSessionBashExecutionUpdateEvent); ok {
				events = append(events, map[string]any{"id": e.ID, "delta": e.Delta})
			}
		})
		for _, c := range input.Commands {
			chunks, calls := []string{}, []map[string]any{}
			r, err := session.ExecuteBash(ctx, c.Command, codingagent.ExecuteBashOptions{ID: c.ID, ExcludeFromContext: c.Exclude, OnChunk: func(s string) { chunks = append(chunks, s) }, Operations: bash93Operations(func(ctx context.Context, command, cwd string, o codingagent.BashExecOptions) (codingagent.BashExecResult, error) {
				running, e := session.IsBashRunning()
				if e != nil {
					t.Error(e)
				}
				calls = append(calls, map[string]any{"command": command, "cwd": cwd == dir, "timeout": o.Timeout, "running": running})
				for _, raw := range c.Chunks {
					var s string
					if json.Unmarshal(raw, &s) == nil {
						o.OnData([]byte(s))
					} else {
						var bytes []int
						json.Unmarshal(raw, &bytes)
						data := []byte{}
						for _, b := range bytes {
							data = append(data, byte(b))
						}
						o.OnData(data)
					}
				}
				if c.Cancel {
					session.AbortBash()
				}
				if c.Failure || c.Cancel {
					return codingagent.BashExecResult{}, fmt.Errorf("failed")
				}
				return codingagent.BashExecResult{ExitCode: &c.Code}, nil
			})})
			var value any
			if err == nil {
				value = map[string]any{"output": r.Output, "exitCode": r.ExitCode, "cancelled": r.Cancelled, "truncated": r.Truncated, "fullOutputPath": r.FullOutputPath}
			}
			running, e := session.IsBashRunning()
			if e != nil {
				return parity.Observation{}, e
			}
			results = append(results, map[string]any{"chunks": chunks, "calls": calls, "failed": err != nil, "result": value, "running": running})
		}
		session.Subscribe(func(e codingagent.AgentSessionEvent) {
			if e.AgentSessionEventType() == codingagent.AgentSessionEventTypeAgentSettled && settlement == nil {
				code := 0
				if e := session.RecordBashResult("settled", codingagent.BashResult{Output: "visible", ExitCode: &code}); e != nil {
					t.Error(e)
				}
				pending, e := session.HasPendingBashMessages()
				if e != nil {
					t.Error(e)
				}
				settlement = map[string]any{"pending": pending, "history": bash93History(session.Messages())}
			}
		})
		history := bash93History(session.Messages())
		if err := session.Prompt(ctx, "first"); err != nil {
			return parity.Observation{}, err
		}
		pending, err := session.HasPendingBashMessages()
		if err != nil {
			return parity.Observation{}, err
		}
		settled := map[string]any{"pending": pending, "history": bash93History(session.Messages())}
		if err := session.Prompt(ctx, "next"); err != nil {
			return parity.Observation{}, err
		}
		reopened, err := codingagent.OpenSessionManager(*session.SessionFile(), nil, nil)
		if err != nil {
			return parity.Observation{}, err
		}
		truncations := []map[string]any{}
		for _, tc := range []struct {
			name, chunk string
			count       int
		}{{"lines", "line\n", 2501}, {"bytes", strings.Repeat("x", 32769), 5}, {"sanitized", strings.Repeat("\x1b[31m\x00\r", 10001), 1}} {
			large := newMessage85Session(t, nil)
			r, e := large.ExecuteBash(ctx, tc.name, codingagent.ExecuteBashOptions{Operations: bash93Operations(func(_ context.Context, _, _ string, o codingagent.BashExecOptions) (codingagent.BashExecResult, error) {
				for range tc.count {
					o.OnData([]byte(tc.chunk))
				}
				code := 0
				return codingagent.BashExecResult{ExitCode: &code}, nil
			})})
			if e != nil {
				return parity.Observation{}, e
			}
			large.Dispose()
			if r.FullOutputPath == nil {
				return parity.Observation{}, fmt.Errorf("missing output path: %s", tc.name)
			}
			full, e := os.ReadFile(*r.FullOutputPath)
			os.Remove(*r.FullOutputPath)
			if e != nil {
				return parity.Observation{}, e
			}
			truncations = append(truncations, map[string]any{"name": tc.name, "outputLength": len(r.Output), "outputStart": r.Output[:min(5, len(r.Output))], "outputEnd": r.Output[max(0, len(r.Output)-5):], "truncated": r.Truncated, "hasFile": true, "fullLength": len(full), "fullStart": string(full[:min(5, len(full))]), "fullEnd": string(full[max(0, len(full)-5):])})
		}
		data, err := json.Marshal(map[string]any{"truncations": truncations, "results": results, "events": events, "history": history, "requests": requests, "deferred": deferred, "settlement": settlement, "settled": settled, "reopened": bash93History(reopened.BuildSessionContext().Messages)})
		return parity.Observation{Outcome: data, SideEffects: &[]parity.SideEffect{}}, err
	}})
	if err != nil || !result.Match {
		t.Fatalf("Session Bash parity: %+v; Pig=%s; err=%v", result.Differences, result.Pig.Outcome, err)
	}
}

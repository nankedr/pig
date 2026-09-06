package codingagent_test

import (
	"context"
	"encoding/json"
	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
	"github.com/nankedr/pig/internal/baseline"
	"github.com/nankedr/pig/internal/parity"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestDefaultCodingToolsParity(t *testing.T) {
	root, _ := filepath.Abs("..")
	lock, _, err := baseline.Load(filepath.Join(root, "parity/baseline"))
	if err != nil {
		t.Fatal(err)
	}
	locked := parity.Baseline{ID: lock.BaselineID, Commit: lock.Upstream.Commit, Repository: lock.Upstream.Repository}
	fixture, err := parity.LoadFixture(filepath.Join(root, "parity/oracle/fixtures/coding-tools.json"), locked)
	if err != nil {
		t.Fatal(err)
	}
	oracle, err := parity.NewFixtureDriver(fixture, locked)
	if err != nil {
		t.Fatal(err)
	}
	result, err := parity.RunCase(context.Background(), fixture.Case, oracle, parity.DriverFunc{SurfaceName: parity.SurfaceGoSDK, ObserveFunc: func(ctx context.Context, c parity.Case) (parity.Observation, error) {
		var input struct {
			Selections []struct {
				Tools, ExcludeTools []string
				NoTools             codingagent.NoToolsMode
			}
		}
		if err := json.Unmarshal(c.Input, &input); err != nil {
			return parity.Observation{}, err
		}
		cwd := t.TempDir()
		core, err := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
		if err != nil {
			return parity.Observation{}, err
		}
		model, _ := core.GetModel()
		selections := []map[string]any{}
		for _, selection := range input.Selections {
			created, err := codingagent.CreateAgentSession(ctx, codingagent.CreateAgentSessionOptions{CWD: cwd, AgentDir: cwd, Model: &model, StreamFunction: agent.StreamFunction(core.StreamSimple), Tools: selection.Tools, ExcludeTools: selection.ExcludeTools, NoTools: selection.NoTools})
			if err != nil {
				return parity.Observation{}, err
			}
			names := append([]string{}, created.Session.GetActiveToolNames()...)
			promptTools := []string{}
			for _, name := range []string{"read", "bash", "edit", "write"} {
				if strings.Contains(created.Session.SystemPrompt(), "- "+name+":") {
					promptTools = append(promptTools, name)
				}
			}
			selections = append(selections, map[string]any{"tools": names, "promptTools": promptTools})
			created.Session.Dispose()
		}
		outcome, err := json.Marshal(map[string]any{"selections": selections})
		return parity.Observation{Outcome: outcome, SideEffects: &[]parity.SideEffect{}}, err
	}})
	if err != nil || !result.Match {
		t.Fatalf("parity differences: %+v; Pig=%s; err=%v", result.Differences, result.Pig.Outcome, err)
	}
}

func TestDefaultCodingToolsSettingsAndIdentity(t *testing.T) {
	ctx := context.Background()
	cwd := t.TempDir()
	shell := filepath.Join(cwd, "shell")
	if err := os.WriteFile(shell, []byte("#!/bin/sh\nexport SHELL_MARKER=custom\nexec /bin/sh \"$@\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	prefix := "export PREFIX_MARKER=trusted"
	timeout, delay, retries := int64(4321), int64(2345), 2
	queue, transport := agent.QueueAll, ai.TransportSSE
	settings, err := codingagent.NewInMemorySettingsManager(codingagent.Settings{SteeringMode: &queue, FollowUpMode: &queue, Transport: &transport, ThinkingBudgets: map[agent.ThinkingLevel]int64{ai.ModelThinkingLevelHigh: 1234}, ShellPath: &shell, ShellCommandPrefix: &prefix, Retry: &codingagent.RetrySettings{Provider: &codingagent.ProviderRetrySettings{TimeoutMS: &timeout, MaxRetries: &retries, MaxRetryDelayMS: &delay}}})
	if err != nil {
		t.Fatal(err)
	}
	manager := codingagent.NewInMemorySessionManager(cwd)
	core, err := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	model, _ := core.GetModel()
	call, _ := ai.FauxAssistantMessage(ai.FauxAssistantBlocks(ai.ToolCall{Type: "toolCall", ID: "env", Name: "bash", Arguments: map[string]any{"command": `printf '%s|%s|%s|%s' "$SHELL_MARKER" "$PREFIX_MARKER" "$PIG_SESSION_ID" "${PIG_SESSION_FILE-unset}"`}}), ai.FauxAssistantMessageOptions{StopReason: ai.Some(ai.StopReasonToolUse)})
	final, _ := ai.FauxAssistantMessage(ai.FauxAssistantText("done"))
	core.SetResponses([]ai.FauxResponseStep{call, final})
	created, err := codingagent.CreateAgentSession(ctx, codingagent.CreateAgentSessionOptions{SessionManager: manager, SettingsManager: settings, Model: &model, StreamFunction: func(ctx context.Context, m ai.Model, input ai.Context, options ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
		if options.Transport == nil || *options.Transport != ai.TransportSSE || options.ThinkingBudgets == nil || options.ThinkingBudgets.High == nil || *options.ThinkingBudgets.High != 1234 {
			t.Error("transport/thinking settings missing")
		}
		if options.SessionID == nil || *options.SessionID != manager.GetSessionID() {
			t.Error("Provider session identity missing")
		}
		if options.TimeoutMS == nil || *options.TimeoutMS != timeout || options.MaxRetries == nil || *options.MaxRetries != retries || options.MaxRetryDelayMS == nil || *options.MaxRetryDelayMS != delay {
			t.Errorf("Provider settings missing: %+v", options)
		}
		return core.StreamSimple(ctx, m, input, options)
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer created.Session.Dispose()
	if created.Session.Agent().SteeringMode() != queue || created.Session.Agent().FollowUpMode() != queue {
		t.Error("queue settings missing")
	}
	if err := created.Session.Prompt(ctx, "inspect session"); err != nil {
		t.Fatal(err)
	}
	results := []ai.ToolResultMessage{}
	for _, message := range created.Session.Messages() {
		if result, ok := message.(ai.ToolResultMessage); ok {
			results = append(results, result)
		}
	}
	if len(results) != 1 || results[0].IsError || readToolResultText(t, results[0]) != "custom|trusted|"+manager.GetSessionID()+"|unset" {
		t.Fatalf("bash result: %+v", results)
	}
	files, err := os.ReadDir(cwd)
	if err != nil || len(files) != 1 || manager.IsPersisted() || created.Session.SessionFile() != nil {
		t.Fatalf("memory session wrote state: %v %v", files, err)
	}
}

func TestDefaultCodingToolsUpdatesStreamSettings(t *testing.T) {
	ctx := context.Background()
	idle, retries, delay := int64(1234), 1, int64(2345)
	settings, err := codingagent.NewInMemorySettingsManager(codingagent.Settings{HTTPIdleTimeoutMS: &idle, Retry: &codingagent.RetrySettings{Provider: &codingagent.ProviderRetrySettings{MaxRetries: &retries, MaxRetryDelayMS: &delay}}})
	if err != nil {
		t.Fatal(err)
	}
	core, err := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	final, _ := ai.FauxAssistantMessage(ai.FauxAssistantText("done"))
	core.SetResponses([]ai.FauxResponseStep{final, final, final})
	model, _ := core.GetModel()
	var observed [][3]int64
	created, err := codingagent.CreateAgentSession(ctx, codingagent.CreateAgentSessionOptions{CWD: t.TempDir(), Model: &model, SettingsManager: settings, StreamFunction: func(ctx context.Context, m ai.Model, input ai.Context, o ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
		observed = append(observed, [3]int64{*o.TimeoutMS, int64(*o.MaxRetries), *o.MaxRetryDelayMS})
		return core.StreamSimple(ctx, m, input, o)
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer created.Session.Dispose()
	if err := created.Session.Prompt(ctx, "first"); err != nil {
		t.Fatal(err)
	}
	if err := settings.SetHTTPIdleTimeoutMS(0); err != nil {
		t.Fatal(err)
	}
	nextRetries, nextDelay := 0, int64(3456)
	if err := settings.ApplyOverrides(codingagent.Settings{Retry: &codingagent.RetrySettings{Provider: &codingagent.ProviderRetrySettings{MaxRetries: &nextRetries, MaxRetryDelayMS: &nextDelay}}}); err != nil {
		t.Fatal(err)
	}
	if err := created.Session.Prompt(ctx, "second"); err != nil {
		t.Fatal(err)
	}
	explicit := int64(4567)
	if err := settings.ApplyOverrides(codingagent.Settings{Retry: &codingagent.RetrySettings{Provider: &codingagent.ProviderRetrySettings{TimeoutMS: &explicit}}}); err != nil {
		t.Fatal(err)
	}
	if err := created.Session.Prompt(ctx, "third"); err != nil {
		t.Fatal(err)
	}
	want := [][3]int64{{1234, 1, 2345}, {2147483647, 0, 3456}, {4567, 0, 3456}}
	if !reflect.DeepEqual(observed, want) {
		t.Fatalf("request settings=%v want=%v", observed, want)
	}
	invalid := int64(-1)
	if err := settings.ApplyOverrides(codingagent.Settings{HTTPIdleTimeoutMS: &invalid}); err != nil {
		t.Fatal(err)
	}
	if err := created.Session.Prompt(ctx, "invalid setting"); err != nil {
		t.Fatal(err)
	}
	messages := created.Session.Messages()
	last, ok := messages[len(messages)-1].(ai.AssistantMessage)
	if !ok || last.StopReason != ai.StopReasonError || len(observed) != 3 {
		t.Fatalf("invalid setting reached Provider or lost outcome: %+v", last)
	}
}

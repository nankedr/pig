package codingagent_test

import (
	"context"
	"encoding/json"
	"errors"
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
	"time"
)

func TestBashToolSessionParity(t *testing.T) {
	root, _ := filepath.Abs("..")
	lock, _, err := baseline.Load(filepath.Join(root, "parity/baseline"))
	if err != nil {
		t.Fatal(err)
	}
	locked := parity.Baseline{ID: lock.BaselineID, Commit: lock.Upstream.Commit, Repository: lock.Upstream.Repository}
	fixture, err := parity.LoadFixture(filepath.Join(root, "parity/oracle/fixtures/bash-tool.json"), locked)
	if err != nil {
		t.Fatal(err)
	}
	oracle, err := parity.NewFixtureDriver(fixture, locked)
	if err != nil {
		t.Fatal(err)
	}
	result, err := parity.RunCase(context.Background(), fixture.Case, oracle, parity.DriverFunc{SurfaceName: parity.SurfaceGoSDK, ObserveFunc: func(ctx context.Context, c parity.Case) (parity.Observation, error) {
		var input struct{ Commands []map[string]any }
		if err := json.Unmarshal(c.Input, &input); err != nil {
			return parity.Observation{}, err
		}
		cwd := t.TempDir()
		tool, err := codingagent.CreateBashTool(cwd)
		if err != nil {
			return parity.Observation{}, err
		}
		results := []map[string]any{}
		for _, command := range input.Commands {
			messages := runBashSession(t, ctx, cwd, []agent.ErasedAgentTool{tool}, []ai.ToolCall{{Type: "toolCall", ID: "bash", Name: "bash", Arguments: command}})
			results = append(results, map[string]any{"text": readToolResultText(t, messages[0]), "isError": messages[0].IsError})
		}
		outcome, err := json.Marshal(map[string]any{"results": results})
		return parity.Observation{Outcome: outcome, SideEffects: &[]parity.SideEffect{}}, err
	}})
	if err != nil || !result.Match {
		t.Fatalf("parity differences: %+v; Pig=%s; err=%v", result.Differences, result.Pig.Outcome, err)
	}
}
func runBashSession(t *testing.T, ctx context.Context, cwd string, tools []agent.ErasedAgentTool, calls []ai.ToolCall) []ai.ToolResultMessage {
	t.Helper()
	core, err := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var responses []ai.FauxResponseStep
	var got []ai.ToolResultMessage
	for _, call := range calls {
		response, err := ai.FauxAssistantMessage(ai.FauxAssistantBlocks(call), ai.FauxAssistantMessageOptions{StopReason: ai.Some(ai.StopReasonToolUse)})
		if err != nil {
			t.Fatal(err)
		}
		responses = append(responses, response)
	}
	final, err := ai.FauxAssistantMessage(ai.FauxAssistantText("write/read complete"))
	if err != nil {
		t.Fatal(err)
	}
	responses = append(responses, final)
	core.SetResponses(responses)
	model, _ := core.GetModel()
	created, err := codingagent.CreateAgentSession(ctx, codingagent.CreateAgentSessionOptions{CWD: cwd, Model: &model, AgentTools: tools, Tools: []string{"bash", "read"}, StreamFunction: func(ctx context.Context, model ai.Model, input ai.Context, options ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
		if len(input.Messages) > 0 {
			if result, ok := input.Messages[len(input.Messages)-1].(ai.ToolResultMessage); ok {
				got = append(got, result)
			}
		}
		return core.StreamSimple(ctx, model, input, options)
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer created.Session.Dispose()
	if err := created.Session.Prompt(ctx, "write and read back"); err != nil {
		t.Fatal(err)
	}
	var persisted []ai.ToolResultMessage
	for _, message := range created.Session.Messages() {
		if result, ok := message.(ai.ToolResultMessage); ok {
			persisted = append(persisted, result)
		}
	}
	if !reflect.DeepEqual(got, persisted) {
		t.Fatalf("continuation differs from transcript: %#v / %#v", got, persisted)
	}
	if len(got) != len(calls) {
		t.Fatalf("continuation results: %d, want %d", len(got), len(calls))
	}
	return got
}

func TestBashToolRetainsFullOutputAfterSessionDispose(t *testing.T) {
	cwd := t.TempDir()
	tool, err := codingagent.CreateBashTool(cwd)
	if err != nil {
		t.Fatal(err)
	}
	messages := runBashSession(t, context.Background(), cwd, []agent.ErasedAgentTool{tool}, []ai.ToolCall{{Type: "toolCall", ID: "bash", Name: "bash", Arguments: map[string]any{"command": "i=1; while [ $i -le 4000 ]; do printf 'line-%04d\\n' $i; i=$((i+1)); done"}}})
	value, _ := messages[0].Details.Value()
	data, _ := json.Marshal(value)
	var details struct {
		FullOutputPath string
		Truncation     struct{ TotalLines, OutputLines int }
	}
	if err := json.Unmarshal(data, &details); err != nil {
		t.Fatal(err)
	}
	if details.Truncation.TotalLines != 4000 || details.Truncation.OutputLines != 2000 || !strings.HasPrefix(filepath.Base(details.FullOutputPath), "pig-bash-") {
		t.Fatalf("details: %s", data)
	}
	t.Cleanup(func() { os.Remove(details.FullOutputPath) })
	if text := readToolResultText(t, messages[0]); !strings.HasPrefix(text, "line-2001\n") || !strings.Contains(text, "[Showing lines 2001-4000 of 4000. Full output: ") {
		t.Fatal(text)
	}
	read, err := codingagent.CreateReadTool(cwd)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		offset int
		want   string
	}{{1, "line-0001\nline-0002"}, {3999, "line-3999\nline-4000"}} {
		got := runBashSession(t, context.Background(), cwd, []agent.ErasedAgentTool{read}, []ai.ToolCall{{Type: "toolCall", ID: "read", Name: "read", Arguments: map[string]any{"path": details.FullOutputPath, "offset": test.offset, "limit": 2}}})
		if got[0].IsError || !strings.HasPrefix(readToolResultText(t, got[0]), test.want) {
			t.Fatalf("read: %#v", got)
		}
	}
}

func TestBashToolSessionEnvironment(t *testing.T) {
	cwd := t.TempDir()
	t.Setenv("HOST_VALUE", "inherited")
	for _, key := range []string{"PIG_SESSION_ID", "PIG_SESSION_FILE", "PIG_PROVIDER", "PIG_MODEL", "PIG_REASONING_LEVEL"} {
		t.Setenv(key, "stale")
	}
	tool, err := codingagent.CreateBashTool(cwd, codingagent.BashToolOptions{CommandPrefix: "export PREFIX_VALUE=prefix", ShellPath: "/bin/sh"})
	if err != nil {
		t.Fatal(err)
	}
	messages := runBashSession(t, context.Background(), cwd, []agent.ErasedAgentTool{tool}, []ai.ToolCall{{Type: "toolCall", ID: "env", Name: "bash", Arguments: map[string]any{"command": `printf '%s|%s|%s|%s|%s|%s|%s' "$HOST_VALUE" "$PREFIX_VALUE" "$PIG_SESSION_ID" "$PIG_SESSION_FILE" "$PIG_PROVIDER" "$PIG_MODEL" "$PIG_REASONING_LEVEL"`}}})
	fields := strings.Split(readToolResultText(t, messages[0]), "|")
	if len(fields) != 7 || fields[0] != "inherited" || fields[1] != "prefix" || fields[2] == "" || fields[3] != "" || fields[4] == "" || fields[5] == "" || fields[6] == "" || strings.Contains(strings.Join(fields, "|"), "stale") {
		t.Fatalf("env: %v", fields)
	}
}

type bashTestOperations func(context.Context, string, string, codingagent.BashExecOptions) (codingagent.BashExecResult, error)

func (f bashTestOperations) Exec(ctx context.Context, command, cwd string, options codingagent.BashExecOptions) (codingagent.BashExecResult, error) {
	return f(ctx, command, cwd, options)
}

func TestBashToolByteTruncationAndErrorOutput(t *testing.T) {
	for _, failure := range []string{"", "aborted", "timeout:5"} {
		t.Run(failure, func(t *testing.T) {
			cwd := t.TempDir()
			ops := bashTestOperations(func(_ context.Context, _, _ string, options codingagent.BashExecOptions) (codingagent.BashExecResult, error) {
				raw := []byte(strings.Repeat("€", 20000))
				options.OnData(raw[:1])
				options.OnData(raw[1:])
				if failure != "" {
					return codingagent.BashExecResult{}, errors.New(failure)
				}
				zero := 0
				return codingagent.BashExecResult{ExitCode: &zero}, nil
			})
			tool, err := codingagent.CreateBashTool(cwd, codingagent.BashToolOptions{Operations: ops})
			if err != nil {
				t.Fatal(err)
			}
			messages := runBashSession(t, context.Background(), cwd, []agent.ErasedAgentTool{tool}, []ai.ToolCall{{Type: "toolCall", ID: "bytes", Name: "bash", Arguments: map[string]any{"command": "bytes"}}})
			text := readToolResultText(t, messages[0])
			if strings.Contains(text, "�") || !strings.Contains(text, "[Showing last 50.0KB of line 1 (line is 58.6KB). Full output: ") || messages[0].IsError != (failure != "") {
				t.Fatalf("output suffix=%q", text[max(0, len(text)-250):])
			}
			path := strings.Split(strings.Split(text, "Full output: ")[1], "]")[0]
			t.Cleanup(func() { os.Remove(path) })
			back, err := os.ReadFile(path)
			if err != nil || len(back) != 60000 || !strings.HasPrefix(string(back), "€€") {
				t.Fatalf("full bytes=%d err=%v", len(back), err)
			}
		})
	}
}

func TestBashToolDefinitionStreamingBarrierAndSpawnOptions(t *testing.T) {
	var late func([]byte)
	var updates []agent.ErasedAgentToolResult
	disabled := false
	t.Setenv("PIG_MODEL", "stale")
	tool, err := codingagent.CreateBashToolDefinition(t.TempDir(), codingagent.BashToolOptions{ExposeSessionEnvironment: &disabled, CommandPrefix: "prefix", ShellPath: "ignored", SpawnHook: func(spawn codingagent.BashSpawnContext) codingagent.BashSpawnContext {
		spawn.Command += " hook"
		spawn.Env["HOOK"] = "yes"
		return spawn
	}, Operations: bashTestOperations(func(_ context.Context, command, _ string, options codingagent.BashExecOptions) (codingagent.BashExecResult, error) {
		if command != "prefix\ncommand hook" || options.Timeout != nil || options.Env["HOOK"] != "yes" || options.Env["PIG_MODEL"] != "" {
			t.Errorf("unexpected spawn command, timeout or metadata: command=%q", command)
		}
		late = options.OnData
		for range 5000 {
			options.OnData([]byte("x"))
		}
		zero := 0
		return codingagent.BashExecResult{ExitCode: &zero}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	result, err := tool.Execute(context.Background(), "stream", map[string]any{"command": "command"}, func(value agent.ErasedAgentToolResult) { updates = append(updates, value) })
	if err != nil || len(result.Content) != 1 || len(updates) < 2 || len(updates) > 25 || len(updates[0].Content) != 0 {
		t.Fatalf("stream updates=%d result=%v err=%v", len(updates), result, err)
	}
	before := len(updates)
	late([]byte("late"))
	time.Sleep(120 * time.Millisecond)
	if len(updates) != before || !reflect.DeepEqual(updates[len(updates)-1], result) {
		t.Fatal("late update or incomplete final update")
	}
}

func TestBashToolLocalOperationsAndShellErrors(t *testing.T) {
	cwd := t.TempDir()
	ops := codingagent.CreateLocalBashOperations(codingagent.BashToolOptions{ShellPath: "/bin/sh"})
	var output strings.Builder
	result, err := ops.Exec(context.Background(), `printf '%s' "$VALUE"; printf 'stderr' >&2; exit 3`, cwd, codingagent.BashExecOptions{Env: map[string]string{"VALUE": "stdout"}, OnData: func(data []byte) { output.Write(data) }})
	if err != nil || result.ExitCode == nil || *result.ExitCode != 3 || output.String() != "stdoutstderr" {
		t.Fatalf("exec=%v output=%q err=%v", result, output.String(), err)
	}
	config, err := codingagent.GetShellConfig()
	if err != nil || config.Shell != "/bin/bash" || !reflect.DeepEqual(config.Args, []string{"-c"}) {
		t.Fatalf("shell=%+v err=%v", config, err)
	}
	for _, test := range []struct {
		options codingagent.BashToolOptions
		cwd     string
		args    map[string]any
		want    string
	}{
		{cwd: cwd, options: codingagent.BashToolOptions{ShellPath: "/nonexistent/pig-shell"}, args: map[string]any{"command": "true"}, want: "Custom shell path not found"},
		{cwd: filepath.Join(cwd, "missing"), args: map[string]any{"command": "true"}, want: "Working directory does not exist"},
		{cwd: cwd, args: map[string]any{"command": "true", "timeout": 0}, want: "Invalid timeout"},
		{cwd: cwd, args: map[string]any{"command": "true", "timeout": 2147484}, want: "Invalid timeout: maximum"},
	} {
		tool, err := codingagent.CreateBashTool(test.cwd, test.options)
		if err != nil {
			t.Fatal(err)
		}
		messages := runBashSession(t, context.Background(), cwd, []agent.ErasedAgentTool{tool}, []ai.ToolCall{{Type: "toolCall", ID: "error", Name: "bash", Arguments: test.args}})
		if !messages[0].IsError || !strings.Contains(readToolResultText(t, messages[0]), test.want) {
			t.Fatalf("result=%v", messages)
		}
	}
}

func TestBashToolDrainsActiveOutputAfterShellExit(t *testing.T) {
	cwd := t.TempDir()
	tool, err := codingagent.CreateBashTool(cwd)
	if err != nil {
		t.Fatal(err)
	}
	messages := runBashSession(t, context.Background(), cwd, []agent.ErasedAgentTool{tool}, []ai.ToolCall{{Type: "toolCall", ID: "drain", Name: "bash", Arguments: map[string]any{"command": "(i=1; while [ $i -le 10 ]; do printf 'tick-%s\\n' $i; i=$((i+1)); sleep 0.03; done) & exit 0"}}})
	if messages[0].IsError || !strings.Contains(readToolResultText(t, messages[0]), "tick-10") {
		t.Fatalf("drain=%v", messages)
	}
}

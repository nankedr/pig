package codingagent_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
	"github.com/nankedr/pig/internal/baseline"
	"github.com/nankedr/pig/internal/parity"
)

type find84Operations struct{ results []string }

func (o find84Operations) Exists(context.Context, string) (bool, error) { return true, nil }
func (o find84Operations) Glob(_ context.Context, _, _ string, options codingagent.FindGlobOptions) ([]string, error) {
	return o.results, nil
}

func TestFindLsSessionParity(t *testing.T) {
	if _, err := exec.LookPath("fd"); err != nil {
		t.Skip("real fd parity requires a prepared fd on PATH; see docs/learning/m4-find-ls.md")
	}
	root, _ := filepath.Abs("..")
	lock, _, err := baseline.Load(filepath.Join(root, "parity/baseline"))
	if err != nil {
		t.Fatal(err)
	}
	locked := parity.Baseline{ID: lock.BaselineID, Commit: lock.Upstream.Commit, Repository: lock.Upstream.Repository}
	fixture, err := parity.LoadFixture(filepath.Join(root, "parity/oracle/fixtures/find-ls.json"), locked)
	if err != nil {
		t.Fatal(err)
	}
	oracle, err := parity.NewFixtureDriver(fixture, locked)
	if err != nil {
		t.Fatal(err)
	}
	result, err := parity.RunCase(context.Background(), fixture.Case, oracle, parity.DriverFunc{SurfaceName: parity.SurfaceGoSDK, ObserveFunc: func(ctx context.Context, c parity.Case) (parity.Observation, error) {
		var input struct {
			Files map[string]string
			Cases []struct {
				Tool   string
				Args   map[string]any
				Sort   bool
				Custom []string
			}
		}
		if err := json.Unmarshal(c.Input, &input); err != nil {
			return parity.Observation{}, err
		}
		cwd := t.TempDir()
		for name, content := range input.Files {
			path := filepath.Join(cwd, name)
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(content), 0600); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.Mkdir(filepath.Join(cwd, "empty"), 0700); err != nil {
			t.Fatal(err)
		}
		for name, target := range map[string]string{"link": "a", "broken": "missing"} {
			if err := os.Symlink(target, filepath.Join(cwd, name)); err != nil {
				t.Fatal(err)
			}
		}
		results := []map[string]any{}
		for _, item := range input.Cases {
			var tool agent.ErasedAgentTool
			if item.Tool == "ls" {
				tool, err = codingagent.CreateLsTool(cwd)
			} else {
				var options codingagent.FindToolOptions
				if item.Custom != nil {
					paths := append([]string{}, item.Custom...)
					for i, p := range paths {
						paths[i] = strings.ReplaceAll(p, "{cwd}", cwd)
					}
					options.Operations = find84Operations{paths}
				}
				tool, err = codingagent.CreateFindTool(cwd, options)
			}
			if err != nil {
				return parity.Observation{}, err
			}
			if item.Tool == "find" && item.Custom == nil {
				if _, err := exec.LookPath("fd"); err != nil {
					t.Fatal("real fd required for parity; add a prepared fd binary to PATH")
				}
			}
			got := runFindLsSession(t, ctx, cwd, []agent.ErasedAgentTool{tool}, []ai.ToolCall{{Type: "toolCall", ID: "locate", Name: item.Tool, Arguments: item.Args}})[0]
			text := strings.ReplaceAll(readToolResultText(t, got), cwd, "{cwd}")
			if item.Sort {
				lines := strings.Split(text, "\n")
				sort.Strings(lines)
				text = strings.Join(lines, "\n")
			}
			details, _ := got.Details.Value()
			if got.IsError {
				details = nil
			}
			results = append(results, map[string]any{"text": text, "details": details, "isError": got.IsError})
		}
		metadata := []map[string]any{}
		for _, factory := range []func(string) (codingagent.ToolDefinition, error){func(c string) (codingagent.ToolDefinition, error) { return codingagent.CreateFindToolDefinition(c) }, func(c string) (codingagent.ToolDefinition, error) { return codingagent.CreateLsToolDefinition(c) }} {
			d, err := factory(cwd)
			if err != nil {
				return parity.Observation{}, err
			}
			metadata = append(metadata, map[string]any{"name": d.Name, "label": d.Label, "description": d.Description, "parameters": d.Parameters, "promptSnippet": d.PromptSnippet})
		}
		out, err := json.Marshal(map[string]any{"results": results, "metadata": metadata})
		return parity.Observation{Outcome: out, SideEffects: &[]parity.SideEffect{}}, err
	}})
	if err != nil || !result.Match {
		var want, got struct {
			Results  []map[string]any
			Metadata []map[string]any
		}
		json.Unmarshal(fixture.Observation.Outcome, &want)
		json.Unmarshal(result.Pig.Outcome, &got)
		for i := range got.Results {
			if !reflect.DeepEqual(got.Results[i], want.Results[i]) {
				t.Errorf("case %d: got %.1000v want %.1000v", i, got.Results[i], want.Results[i])
			}
		}
		if !reflect.DeepEqual(got.Metadata, want.Metadata) {
			t.Errorf("metadata got %v want %v", got.Metadata, want.Metadata)
		}
		t.Fatalf("parity differences: %+v; err=%v", result.Differences, err)
	}
}

func runFindLsSession(t *testing.T, ctx context.Context, cwd string, tools []agent.ErasedAgentTool, calls []ai.ToolCall) []ai.ToolResultMessage {
	t.Helper()
	core, err := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var responses []ai.FauxResponseStep
	for _, call := range calls {
		step, err := ai.FauxAssistantMessage(ai.FauxAssistantBlocks(call), ai.FauxAssistantMessageOptions{StopReason: ai.Some(ai.StopReasonToolUse)})
		if err != nil {
			t.Fatal(err)
		}
		responses = append(responses, step)
	}
	final, _ := ai.FauxAssistantMessage(ai.FauxAssistantText("located and read"))
	core.SetResponses(append(responses, final))
	model, _ := core.GetModel()
	var got []ai.ToolResultMessage
	created, err := codingagent.CreateAgentSession(ctx, codingagent.CreateAgentSessionOptions{CWD: cwd, AgentDir: t.TempDir(), Model: &model, AgentTools: tools, Tools: []string{"find", "ls", "read"}, StreamFunction: func(ctx context.Context, m ai.Model, input ai.Context, options ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
		if len(input.Messages) > 0 {
			if result, ok := input.Messages[len(input.Messages)-1].(ai.ToolResultMessage); ok {
				got = append(got, result)
			}
		}
		return core.StreamSimple(ctx, m, input, options)
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer created.Session.Dispose()
	if err := created.Session.Prompt(ctx, "locate and read"); err != nil {
		t.Fatal(err)
	}
	var persisted []ai.ToolResultMessage
	for _, message := range created.Session.Messages() {
		if result, ok := message.(ai.ToolResultMessage); ok {
			persisted = append(persisted, result)
		}
	}
	if !reflect.DeepEqual(got, persisted) || len(got) != len(calls) {
		t.Fatalf("continuation mismatch: %v / %v", got, persisted)
	}
	return got
}

func TestFindLsExplicitSDKReadContinuation(t *testing.T) {
	cwd := t.TempDir()
	if err := os.WriteFile(filepath.Join(cwd, "result.txt"), []byte("located content"), 0600); err != nil {
		t.Fatal(err)
	}
	got := runFindLsSession(t, context.Background(), cwd, nil, []ai.ToolCall{
		{Type: "toolCall", ID: "list", Name: "ls", Arguments: map[string]any{}},
		{Type: "toolCall", ID: "read", Name: "read", Arguments: map[string]any{"path": "result.txt"}},
	})
	if got[0].IsError || readToolResultText(t, got[0]) != "result.txt" || got[1].IsError || readToolResultText(t, got[1]) != "located content" {
		t.Fatalf("results: %v", got)
	}
}

type find84FailureOperations struct {
	exists func(context.Context, string) (bool, error)
	glob   func(context.Context, string, string, codingagent.FindGlobOptions) ([]string, error)
}

func (o find84FailureOperations) Exists(c context.Context, p string) (bool, error) {
	return o.exists(c, p)
}
func (o find84FailureOperations) Glob(c context.Context, p, r string, opts codingagent.FindGlobOptions) ([]string, error) {
	return o.glob(c, p, r, opts)
}

func TestFindLsDefinitionOperationsAndCancellation(t *testing.T) {
	cwd := t.TempDir()
	ctx := context.Background()
	calls := 0
	paths := []string{filepath.Join(cwd, "result.txt")}
	ops := find84FailureOperations{exists: func(context.Context, string) (bool, error) { calls++; return true, nil }, glob: func(_ context.Context, pattern, path string, opts codingagent.FindGlobOptions) ([]string, error) {
		calls++
		if pattern != "*.txt" || path != cwd || opts.Limit != 1000 || !reflect.DeepEqual(opts.Ignore, []string{"**/node_modules/**", "**/.git/**"}) {
			t.Errorf("operation args: %q %q %+v", pattern, path, opts)
		}
		return paths, nil
	}}
	definition, err := codingagent.CreateFindToolDefinition(cwd, codingagent.FindToolOptions{Operations: ops})
	if err != nil || calls != 0 {
		t.Fatalf("factory: %v calls=%d", err, calls)
	}
	result, err := definition.Execute(ctx, "find", map[string]any{"pattern": "*.txt"}, nil)
	if err != nil || result.Content[0].(ai.TextContent).Text != "result.txt" || paths[0] != filepath.Join(cwd, "result.txt") {
		t.Fatalf("definition: %+v %v paths=%v", result, err, paths)
	}
	ops.glob = func(context.Context, string, string, codingagent.FindGlobOptions) ([]string, error) {
		return nil, errors.New("remote glob failed")
	}
	tool, err := codingagent.CreateFindTool(cwd, codingagent.FindToolOptions{Operations: ops})
	if err != nil {
		t.Fatal(err)
	}
	got := runFindLsSession(t, ctx, cwd, []agent.ErasedAgentTool{tool}, []ai.ToolCall{{Type: "toolCall", ID: "find", Name: "find", Arguments: map[string]any{"pattern": "*.txt"}}})[0]
	if !got.IsError || readToolResultText(t, got) != "remote glob failed" {
		t.Fatalf("error continuation: %v", got)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	for _, factory := range []func() (codingagent.ToolDefinition, error){func() (codingagent.ToolDefinition, error) {
		return codingagent.CreateFindToolDefinition(cwd, codingagent.FindToolOptions{Operations: ops})
	}, func() (codingagent.ToolDefinition, error) { return codingagent.CreateLsToolDefinition(cwd) }} {
		def, err := factory()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := def.Execute(canceled, "cancel", map[string]any{"pattern": "*"}, nil); !errors.Is(err, context.Canceled) {
			t.Fatalf("cancel: %v", err)
		}
	}
	ctx, cancel = context.WithCancel(context.Background())
	ops.exists = func(context.Context, string) (bool, error) { cancel(); return true, nil }
	ops.glob = func(context.Context, string, string, codingagent.FindGlobOptions) ([]string, error) {
		t.Error("glob after cancellation")
		return nil, nil
	}
	definition, _ = codingagent.CreateFindToolDefinition(cwd, codingagent.FindToolOptions{Operations: ops})
	if _, err := definition.Execute(ctx, "cancel", map[string]any{"pattern": "*"}, nil); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestFindLsPathsAndByteTruncation(t *testing.T) {
	cwd := t.TempDir()
	sub := filepath.Join(cwd, "space name")
	if err := os.Mkdir(sub, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "result.txt"), []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", cwd)
	t.Setenv("USERPROFILE", cwd)
	for _, path := range []string{"@space\u202fname", sub, "~/space name", (&url.URL{Scheme: "file", Path: filepath.ToSlash(sub)}).String()} {
		def, err := codingagent.CreateLsToolDefinition(cwd)
		if err != nil {
			t.Fatal(err)
		}
		got, err := def.Execute(context.Background(), "ls", map[string]any{"path": path}, nil)
		if err != nil || got.Content[0].(ai.TextContent).Text != "result.txt" {
			t.Fatalf("path %q: %v %v", path, got, err)
		}
	}
	for i := 0; i < 250; i++ {
		name := fmt.Sprintf("%03d-%s", i, strings.Repeat("x", 220))
		if err := os.WriteFile(filepath.Join(cwd, name), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	got := runFindLsSession(t, context.Background(), cwd, nil, []ai.ToolCall{{Type: "toolCall", ID: "ls", Name: "ls", Arguments: map[string]any{}}})[0]
	details, _ := got.Details.Value()
	raw, _ := json.Marshal(details)
	var value struct {
		Truncation struct {
			Truncated                         bool
			TotalLines, OutputLines, MaxLines int
		}
	}
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	if got.IsError || !value.Truncation.Truncated || value.Truncation.TotalLines != 251 || value.Truncation.OutputLines >= 251 || !strings.HasSuffix(readToolResultText(t, got), "[50.0KB limit reached]") {
		t.Fatalf("truncation: %s", raw)
	}
}

func TestFindLsRootDirectoryResult(t *testing.T) {
	tool, err := codingagent.CreateFindTool("/", codingagent.FindToolOptions{Operations: find84Operations{results: []string{"/", "."}}})
	if err != nil {
		t.Fatal(err)
	}
	got := runFindLsSession(t, context.Background(), "/", []agent.ErasedAgentTool{tool}, []ai.ToolCall{{Type: "toolCall", ID: "find", Name: "find", Arguments: map[string]any{"pattern": "**"}}})[0]
	if got.IsError || readToolResultText(t, got) != "/\n." {
		t.Fatalf("root directory result: %q", readToolResultText(t, got))
	}
}

package codingagent_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
	"github.com/nankedr/pig/internal/baseline"
	"github.com/nankedr/pig/internal/parity"
)

func TestWriteToolSessionParity(t *testing.T) {
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	lock, _, err := baseline.Load(filepath.Join(root, "parity/baseline"))
	if err != nil {
		t.Fatal(err)
	}
	locked := parity.Baseline{ID: lock.BaselineID, Commit: lock.Upstream.Commit, Repository: lock.Upstream.Repository}
	fixture, err := parity.LoadFixture(filepath.Join(root, "parity/oracle/fixtures/write-tool.json"), locked)
	if err != nil {
		t.Fatal(err)
	}
	oracle, err := parity.NewFixtureDriver(fixture, locked)
	if err != nil {
		t.Fatal(err)
	}
	result, err := parity.RunCase(context.Background(), fixture.Case, oracle, parity.DriverFunc{SurfaceName: parity.SurfaceGoSDK, ObserveFunc: func(ctx context.Context, c parity.Case) (parity.Observation, error) {
		var input struct {
			CWDForms []string `json:"cwdForms"`
			Writes   []struct{ Path, Content string }
		}
		if err := json.Unmarshal(c.Input, &input); err != nil {
			return parity.Observation{}, err
		}
		cwd := filepath.Join(t.TempDir(), "workspace")
		t.Chdir(filepath.Dir(cwd))
		if err := os.Mkdir(cwd, 0700); err != nil {
			return parity.Observation{}, err
		}
		write, err := codingagent.CreateWriteTool(cwd)
		if err != nil {
			return parity.Observation{}, err
		}
		read, err := codingagent.CreateReadTool(cwd)
		if err != nil {
			return parity.Observation{}, err
		}
		results := []map[string]any{}
		for _, item := range input.Writes {
			messages := runFileToolSession(t, ctx, cwd, []agent.ErasedAgentTool{write, read}, []ai.ToolCall{
				{Type: "toolCall", ID: "write", Name: "write", Arguments: map[string]any{"path": item.Path, "content": item.Content}},
				{Type: "toolCall", ID: "read", Name: "read", Arguments: map[string]any{"path": item.Path}},
			})
			if len(messages) != 2 || messages[0].IsError || messages[1].IsError {
				t.Fatalf("results: %#v", messages)
			}
			results = append(results, map[string]any{"text": readToolResultText(t, messages[0]), "detailsEmpty": messages[0].Details.IsNull() || !messages[0].Details.IsSet(), "read": readToolResultText(t, messages[1])})
		}

		var cwdResults []map[string]any
		t.Setenv("HOME", filepath.Dir(cwd))
		t.Setenv("USERPROFILE", filepath.Dir(cwd))
		for _, form := range input.CWDForms {
			base := "~/workspace"
			if form == "file-url" {
				base = (&url.URL{Scheme: "file", Path: cwd}).String()
			}
			tool, err := codingagent.CreateWriteTool(base)
			if err != nil {
				return parity.Observation{}, err
			}
			messages := runFileToolSession(t, ctx, cwd, []agent.ErasedAgentTool{tool, read}, []ai.ToolCall{
				{Type: "toolCall", ID: "write", Name: "write", Arguments: map[string]any{"path": "cwd.txt", "content": form}},
				{Type: "toolCall", ID: "read", Name: "read", Arguments: map[string]any{"path": "cwd.txt"}},
			})
			if messages[0].IsError || messages[1].IsError {
				t.Fatalf("cwd results: %#v", messages)
			}
			cwdResults = append(cwdResults, map[string]any{"text": readToolResultText(t, messages[0]), "read": readToolResultText(t, messages[1])})
		}
		definition, err := codingagent.CreateWriteToolDefinition(cwd)
		if err != nil {
			return parity.Observation{}, err
		}
		metadata := map[string]any{"name": definition.Name, "label": definition.Label, "description": definition.Description, "parameters": definition.Parameters, "promptSnippet": definition.PromptSnippet, "promptGuidelines": definition.PromptGuidelines}
		outcome, err := json.Marshal(map[string]any{"results": results, "metadata": metadata, "cwdResults": cwdResults})
		return parity.Observation{Outcome: outcome, SideEffects: &[]parity.SideEffect{}}, err
	}})
	if err != nil {
		t.Fatalf("%v: %+v", err, result.Differences)
	}
	if !result.Match {
		t.Fatalf("parity: %#v", result)
	}
}

func runFileToolSession(t *testing.T, ctx context.Context, cwd string, tools []agent.ErasedAgentTool, calls []ai.ToolCall) []ai.ToolResultMessage {
	t.Helper()
	core, err := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tool := range tools {
		names = append(names, tool.Name)
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
	final, err := ai.FauxAssistantMessage(ai.FauxAssistantText("file operations complete"))
	if err != nil {
		t.Fatal(err)
	}
	responses = append(responses, final)
	core.SetResponses(responses)
	model, _ := core.GetModel()
	created, err := codingagent.CreateAgentSession(ctx, codingagent.CreateAgentSessionOptions{CWD: cwd, Model: &model, AgentTools: tools, Tools: names, StreamFunction: func(ctx context.Context, model ai.Model, input ai.Context, options ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
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
	if err := created.Session.Prompt(ctx, "modify and read back"); err != nil {
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

func TestWriteToolRejectsInvalidArgumentsAndHostErrors(t *testing.T) {
	cwd := t.TempDir()
	write, err := codingagent.CreateWriteTool(cwd)
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range []map[string]any{
		{"path": "missing-content"}, {"content": "missing-path"}, {"path": nil, "content": "bad"},
		{"path": "directory", "content": "bad"}, {"path": "block/child", "content": "bad"}, {"path": "bad\x00path", "content": "bad"},
	} {
		if err := os.MkdirAll(filepath.Join(cwd, "directory"), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(cwd, "block"), []byte("unchanged"), 0600); err != nil {
			t.Fatal(err)
		}
		results := runFileToolSession(t, context.Background(), cwd, []agent.ErasedAgentTool{write}, []ai.ToolCall{{Type: "toolCall", ID: "invalid", Name: "write", Arguments: args}})
		if !results[0].IsError || strings.Contains(readToolResultText(t, results[0]), "Successfully wrote") {
			t.Fatalf("false success: %#v", results)
		}
	}
	if _, err := os.Stat(filepath.Join(cwd, "missing-content")); !os.IsNotExist(err) {
		t.Fatalf("invalid call wrote a file: %v", err)
	}
}

func TestWriteToolDefinitionAndHostPaths(t *testing.T) {
	root := t.TempDir()
	cwd := filepath.Join(root, "workspace")
	if err := os.Mkdir(cwd, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", root)
	t.Setenv("USERPROFILE", root)
	target := filepath.Join(root, "target.txt")
	if err := os.WriteFile(target, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(cwd, "alias")); err != nil {
		t.Fatal(err)
	}
	definition, err := codingagent.CreateWriteToolDefinition(cwd)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{target, "alias", "../target.txt", "~/target.txt", (&url.URL{Scheme: "file", Path: target}).String()} {
		result, err := definition.Execute(context.Background(), "write", map[string]any{"path": path, "content": "replacement"}, nil)
		if err != nil || result.Details != nil {
			t.Fatalf("definition: %#v %v", result, err)
		}
		back := runReadTool(t, context.Background(), cwd, map[string]any{"path": target})
		if back.IsError || readToolResultText(t, back) != "replacement" {
			t.Fatalf("read: %#v", back)
		}
	}
}

type writeTestOperations struct {
	mkdir func(context.Context, string) error
	write func(context.Context, string, []byte) error
}

func (o writeTestOperations) Mkdir(ctx context.Context, path string) error {
	if o.mkdir != nil {
		return o.mkdir(ctx, path)
	}
	return nil
}
func (o writeTestOperations) WriteFile(ctx context.Context, path string, content []byte) error {
	return o.write(ctx, path, content)
}

func TestWriteToolQueueCancellationAndFailure(t *testing.T) {
	for _, stage := range []string{"write", "mkdir", "failure"} {
		t.Run(stage, func(t *testing.T) {
			cwd := t.TempDir()
			target := filepath.Join(cwd, "target")
			alias := filepath.Join(cwd, "alias")
			if err := os.WriteFile(target, []byte("old"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, alias); err != nil {
				t.Fatal(err)
			}
			started, release := make(chan struct{}), make(chan struct{})
			var released sync.Once
			defer released.Do(func() { close(release) })
			operations := writeTestOperations{write: func(_ context.Context, path string, content []byte) error { return os.WriteFile(path, content, 0600) }}
			block := func() error {
				close(started)
				<-release
				if stage == "failure" {
					return os.ErrPermission
				}
				return nil
			}
			if stage == "mkdir" {
				operations.mkdir = func(context.Context, string) error { return block() }
			} else {
				operations.write = func(_ context.Context, path string, content []byte) error {
					if err := block(); err != nil {
						return err
					}
					return os.WriteFile(path, content, 0600)
				}
			}
			first, err := codingagent.CreateWriteToolDefinition(cwd, codingagent.WriteToolOptions{Operations: operations})
			if err != nil {
				t.Fatal(err)
			}
			next, err := codingagent.CreateWriteToolDefinition(cwd)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			firstDone := make(chan error, 1)
			go func() {
				_, err := first.Execute(ctx, "first", map[string]any{"path": target, "content": "first"}, nil)
				firstDone <- err
			}()
			select {
			case <-started:
			case <-time.After(3 * time.Second):
				t.Fatal("first operation never started")
			}
			if stage != "failure" {
				cancel()
			}
			nextDone := make(chan error, 1)
			go func() {
				_, err := next.Execute(context.Background(), "next", map[string]any{"path": alias, "content": "second"}, nil)
				nextDone <- err
			}()
			independent := make(chan error, 1)
			go func() {
				_, err := next.Execute(context.Background(), "other", map[string]any{"path": "other", "content": "independent"}, nil)
				independent <- err
			}()
			select {
			case err := <-independent:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("different file blocked")
			}
			select {
			case err := <-nextDone:
				t.Fatalf("same file released early: %v", err)
			case <-time.After(30 * time.Millisecond):
			}
			released.Do(func() { close(release) })
			select {
			case err := <-firstDone:
				if stage == "failure" {
					if !errors.Is(err, os.ErrPermission) {
						t.Fatal(err)
					}
				} else if !errors.Is(err, context.Canceled) {
					t.Fatalf("cancel error: %v", err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("first did not settle")
			}
			select {
			case err := <-nextDone:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("queue did not release")
			}
			back := runReadTool(t, context.Background(), cwd, map[string]any{"path": target})
			if back.IsError || readToolResultText(t, back) != "second" {
				t.Fatalf("final read: %#v", back)
			}
		})
	}
}

func TestWriteToolCanceledBeforeExecution(t *testing.T) {
	calls := 0
	tool, err := codingagent.CreateWriteToolDefinition(t.TempDir(), codingagent.WriteToolOptions{Operations: writeTestOperations{mkdir: func(context.Context, string) error { calls++; return nil }, write: func(context.Context, string, []byte) error { calls++; return nil }}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := tool.Execute(ctx, "cancel", map[string]any{"path": "canceled", "content": "bad"}, nil)
	if !errors.Is(err, context.Canceled) || calls != 0 || len(result.Content) != 0 {
		t.Fatalf("canceled: %#v %v calls=%d", result, err, calls)
	}
}

func TestWriteToolSessionPermissionErrorContinues(t *testing.T) {
	cwd := t.TempDir()
	write, err := codingagent.CreateWriteTool(cwd, codingagent.WriteToolOptions{Operations: writeTestOperations{write: func(context.Context, string, []byte) error { return os.ErrPermission }}})
	if err != nil {
		t.Fatal(err)
	}
	results := runFileToolSession(t, context.Background(), cwd, []agent.ErasedAgentTool{write}, []ai.ToolCall{{Type: "toolCall", ID: "denied", Name: "write", Arguments: map[string]any{"path": "denied", "content": "bad"}}})
	if !results[0].IsError || !strings.Contains(readToolResultText(t, results[0]), "permission denied") {
		t.Fatalf("permission result: %#v", results)
	}
}

func TestWriteToolSessionAbortWaitsForMutation(t *testing.T) {
	cwd := t.TempDir()
	started, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(release) })
	write, err := codingagent.CreateWriteTool(cwd, codingagent.WriteToolOptions{Operations: writeTestOperations{write: func(_ context.Context, path string, content []byte) error {
		close(started)
		<-release
		return os.WriteFile(path, content, 0600)
	}}})
	if err != nil {
		t.Fatal(err)
	}
	core, err := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	call := ai.ToolCall{Type: "toolCall", ID: "abort", Name: "write", Arguments: map[string]any{"path": "result", "content": "in flight"}}
	response, err := ai.FauxAssistantMessage(ai.FauxAssistantBlocks(call), ai.FauxAssistantMessageOptions{StopReason: ai.Some(ai.StopReasonToolUse)})
	if err != nil {
		t.Fatal(err)
	}
	core.SetResponses([]ai.FauxResponseStep{response})
	model, _ := core.GetModel()
	created, err := codingagent.CreateAgentSession(context.Background(), codingagent.CreateAgentSessionOptions{CWD: cwd, Model: &model, AgentTools: []agent.ErasedAgentTool{write}, Tools: []string{"write"}, StreamFunction: agent.StreamFunction(core.StreamSimple)})
	if err != nil {
		t.Fatal(err)
	}
	defer created.Session.Dispose()
	done := make(chan error, 1)
	go func() { done <- created.Session.Prompt(context.Background(), "save") }()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("write did not start")
	}
	if err := created.Session.Abort(); err != nil {
		t.Fatal(err)
	}
	waitCtx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if err := created.Session.WaitForIdle(waitCtx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("settled before mutation finished: %v", err)
	}
	once.Do(func() { close(release) })
	select {
	case err := <-done:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("abort did not settle")
	}
	found := false
	for _, message := range created.Session.Messages() {
		if result, ok := message.(ai.ToolResultMessage); ok {
			found = true
			if !result.IsError || strings.Contains(readToolResultText(t, result), "Successfully wrote") {
				t.Fatalf("false success on cancel: %#v", result)
			}
		}
	}
	if !found {
		t.Fatal("missing canceled ToolResult")
	}
	next, err := codingagent.CreateWriteToolDefinition(cwd)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := next.Execute(context.Background(), "next", map[string]any{"path": "result", "content": "next"}, nil); err != nil {
		t.Fatal(err)
	}
	back := runReadTool(t, context.Background(), cwd, map[string]any{"path": "result"})
	if back.IsError || readToolResultText(t, back) != "next" {
		t.Fatalf("queue after abort: %#v", back)
	}
}

func TestWriteToolHeadlessSelection(t *testing.T) {
	for _, test := range []struct{ names, excluded, want []string }{
		{want: []string{"read", "bash", "edit", "write"}}, {names: []string{"write"}, want: []string{"write"}},
		{names: []string{"read", "write"}, excluded: []string{"write"}, want: []string{"read"}},
	} {
		settings, err := codingagent.NewInMemorySettingsManager(codingagent.Settings{})
		if err != nil {
			t.Fatal(err)
		}
		runtime, err := codingagent.CreateHeadlessSession(context.Background(), codingagent.CreateHeadlessSessionOptions{CWD: t.TempDir(), SettingsManager: settings, SessionManager: codingagent.NewInMemorySessionManager(""), Provider: "deepseek", Model: "deepseek-v4-flash", Tools: test.names, ExcludeTools: test.excluded})
		if err != nil {
			t.Fatal(err)
		}
		if got := runtime.Session().GetActiveToolNames(); !reflect.DeepEqual(got, test.want) {
			t.Fatalf("active=%v want=%v", got, test.want)
		}
		if strings.Contains(runtime.Session().SystemPrompt(), "Use write only") != slices.Contains(test.want, "write") {
			t.Fatal("prompt does not match selected tools")
		}
		runtime.Dispose(context.Background())
	}
}

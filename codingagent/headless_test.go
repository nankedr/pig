package codingagent_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
)

func TestHeadlessRunnerReturnsFinalAssistantText(t *testing.T) {
	response, err := ai.FauxAssistantMessage(ai.FauxAssistantBlocks(
		ai.FauxText("first"),
		ai.FauxText("second"),
	))
	if err != nil {
		t.Fatal(err)
	}
	runtime := newHeadlessFauxRuntime(t, []ai.FauxResponseStep{response}, nil, nil)
	prompt := "hello"

	outcome, err := codingagent.RunHeadless(context.Background(), runtime, codingagent.HeadlessRunOptions{
		InitialMessage: &prompt,
	})
	if err != nil {
		t.Fatalf("RunHeadless() error = %v", err)
	}
	if outcome.FinalMessage == nil || outcome.FinalMessage.StopReason != ai.StopReasonStop {
		t.Fatalf("final message = %#v, want successful Assistant message", outcome.FinalMessage)
	}
	if !reflect.DeepEqual(outcome.Text, []string{"first", "second"}) {
		t.Fatalf("text = %#v, want final text blocks only", outcome.Text)
	}
	if outcome.Canceled {
		t.Fatal("successful outcome reported cancellation")
	}
	if !runtime.Session().IsIdle() {
		t.Fatal("RunHeadless returned before the session settled")
	}
}

func TestPrintModeRejectsImageInputInsteadOfSilentlyDroppingIt(t *testing.T) {
	runtime := newHeadlessFauxRuntime(t, nil, nil, nil)
	_, err := codingagent.RunPrintMode(context.Background(), runtime, codingagent.PrintModeOptions{
		InitialImages: []ai.ImageContent{{Type: ai.ContentTypeImage, Data: "synthetic", MIMEType: "image/png"}},
	})
	var unavailable *codingagent.NotImplementedError
	if !errors.As(err, &unavailable) || unavailable.Operation != "mode.print.images" {
		t.Fatalf("RunPrintMode() error = %#v, want mode.print.images Capability Stub", err)
	}
}

func TestJSONPrintModeRequiresSessionHeader(t *testing.T) {
	runtime := codingagent.NewAgentSessionRuntime(
		codingagent.NewAgentSession(codingagent.AgentSessionConfig{}),
		codingagent.AgentSessionServices{}, nil, nil, nil,
	)
	prompt := "hello"
	code, err := codingagent.RunPrintMode(context.Background(), runtime, codingagent.PrintModeOptions{
		InitialMessage: &prompt,
		Mode:           codingagent.ModeJSON,
	})
	if code != 1 || err == nil || err.Error() != "JSON mode requires a Session header" {
		t.Fatalf("RunPrintMode() = (%d, %v), want (1, missing Session header)", code, err)
	}
}

func TestHeadlessRunnerPreservesPartialCancellationOutcome(t *testing.T) {
	rate := 100.0
	minTokenSize := 1
	response, err := ai.FauxAssistantMessage(ai.FauxAssistantText("partial response that must survive cancellation"))
	if err != nil {
		t.Fatal(err)
	}
	runtime := newHeadlessFauxRuntime(t, []ai.FauxResponseStep{response}, &rate, &minTokenSize)
	ctx, cancel := context.WithCancelCause(context.Background())
	var once sync.Once
	prompt := "start"

	outcome, err := codingagent.RunHeadless(ctx, runtime, codingagent.HeadlessRunOptions{
		InitialMessage: &prompt,
		OnEvent: func(event codingagent.AgentSessionEvent) {
			if update, ok := event.(codingagent.AgentSessionMessageUpdateEvent); ok {
				assistant, ok := update.Message.(ai.AssistantMessage)
				if ok && len(assistant.Content) > 0 {
					if text, ok := assistant.Content[0].(ai.TextContent); ok && text.Text != "" {
						once.Do(func() { cancel(context.Canceled) })
					}
				}
			}
		},
	})
	if err != nil {
		t.Fatalf("RunHeadless() error = %v, want terminal cancellation outcome", err)
	}
	if !outcome.Canceled || outcome.FinalMessage == nil || outcome.FinalMessage.StopReason != ai.StopReasonAborted {
		t.Fatalf("cancellation outcome = %#v", outcome)
	}
	if len(outcome.Text) != 1 || outcome.Text[0] == "" || outcome.Text[0] == "partial response that must survive cancellation" {
		t.Fatalf("partial text = %#v, want non-empty proper prefix", outcome.Text)
	}
	if !errors.Is(context.Cause(ctx), context.Canceled) {
		t.Fatalf("run context cause = %v, want context.Canceled", context.Cause(ctx))
	}
}

func TestPrintModeClassifiesCancellationDuringReadTool(t *testing.T) {
	call, err := ai.FauxToolCall("read", map[string]any{"path": "blocked.txt"}, ai.FauxToolCallOptions{ID: ai.Some("cancel-read")})
	if err != nil {
		t.Fatal(err)
	}
	readRequest, err := ai.FauxAssistantMessage(
		ai.FauxAssistantBlocks(call),
		ai.FauxAssistantMessageOptions{StopReason: ai.Some(ai.StopReasonToolUse)},
	)
	if err != nil {
		t.Fatal(err)
	}
	provider, err := ai.NewFauxProvider()
	if err != nil {
		t.Fatal(err)
	}
	provider.SetResponses([]ai.FauxResponseStep{readRequest})
	model, _ := provider.GetModel()
	started := make(chan struct{})
	readTool, err := codingagent.CreateReadTool(t.TempDir(), codingagent.ReadToolOptions{Operations: blockingReadOperations{started: started}})
	if err != nil {
		t.Fatal(err)
	}
	created, err := codingagent.CreateAgentSession(context.Background(), codingagent.CreateAgentSessionOptions{
		Model:      &model,
		Provider:   provider.Provider,
		AgentTools: []agent.ErasedAgentTool{readTool},
	})
	if err != nil {
		t.Fatal(err)
	}
	runtime := codingagent.NewAgentSessionRuntime(created.Session, codingagent.AgentSessionServices{}, nil, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	prompt := "read blocked.txt"
	type result struct {
		code int
		err  error
	}
	finished := make(chan result, 1)
	go func() {
		code, err := codingagent.RunPrintMode(ctx, runtime, codingagent.PrintModeOptions{InitialMessage: &prompt})
		finished <- result{code: code, err: err}
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		cancel()
		t.Fatal("read Tool did not start")
	}
	cancel()
	var got result
	select {
	case got = <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("RunPrintMode did not settle after cancellation")
	}
	var outcomeError *codingagent.HeadlessOutcomeError
	if got.code != 1 || !errors.As(got.err, &outcomeError) {
		t.Fatalf("RunPrintMode() = (%d, %#v), want Headless cancellation error", got.code, got.err)
	}
	if outcomeError.ExitCode() != 130 || !errors.Is(outcomeError, context.Canceled) {
		t.Fatalf("cancellation error = %#v, exit %d", outcomeError, outcomeError.ExitCode())
	}
	if got, want := outcomeError.Error(), "Request aborted"; got != want {
		t.Fatalf("cancellation error = %q, want %q", got, want)
	}
	if !outcomeError.Outcome.Canceled || outcomeError.Outcome.FinalMessage == nil || outcomeError.Outcome.FinalMessage.StopReason != ai.StopReasonToolUse {
		t.Fatalf("cancellation outcome = %#v, want preserved ToolCall Assistant and canceled classification", outcomeError.Outcome)
	}
}

type blockingReadOperations struct {
	started chan<- struct{}
}

func (operations blockingReadOperations) Access(ctx context.Context, _ string) error {
	close(operations.started)
	<-ctx.Done()
	return context.Cause(ctx)
}

func (blockingReadOperations) ReadFile(context.Context, string) ([]byte, error) {
	return nil, errors.New("unexpected read after canceled access")
}

func TestHeadlessRunnerStopsAfterTerminalProviderError(t *testing.T) {
	failure, err := ai.FauxAssistantMessage(ai.FauxAssistantText("partial"), ai.FauxAssistantMessageOptions{
		StopReason:   ai.Some(ai.StopReasonError),
		ErrorMessage: ai.Some("provider failed"),
	})
	if err != nil {
		t.Fatal(err)
	}
	unexpected, err := ai.FauxAssistantMessage(ai.FauxAssistantText("must not run"))
	if err != nil {
		t.Fatal(err)
	}
	runtime := newHeadlessFauxRuntime(t, []ai.FauxResponseStep{failure, unexpected}, nil, nil)
	prompt := "first"

	outcome, err := codingagent.RunHeadless(context.Background(), runtime, codingagent.HeadlessRunOptions{
		InitialMessage: &prompt,
		Messages:       []string{"second"},
	})
	if err != nil {
		t.Fatalf("RunHeadless() error = %v, want terminal Provider outcome", err)
	}
	if outcome.FinalMessage == nil || outcome.FinalMessage.StopReason != ai.StopReasonError {
		t.Fatalf("outcome = %#v, want Provider error", outcome)
	}
	if !reflect.DeepEqual(outcome.Text, []string{"partial"}) {
		t.Fatalf("text = %#v, want first terminal partial only", outcome.Text)
	}
	if got := runtime.Session().Messages(); len(got) != 2 {
		t.Fatalf("transcript length = %d, want second prompt not submitted", len(got))
	}
}

func TestCreateHeadlessSessionUsesExplicitAndAmbientCredentialsWithoutStateRuntime(t *testing.T) {
	const explicit = "explicit-key"
	cwd := t.TempDir()
	runtime, err := codingagent.CreateHeadlessSession(context.Background(), codingagent.CreateHeadlessSessionOptions{
		CWD:            cwd,
		Provider:       ai.ProviderIDDeepSeek,
		Model:          "deepseek-v4-flash",
		APIKey:         ptrString(explicit),
		Environment:    ai.ProviderEnv{"DEEPSEEK_API_KEY": "ambient-key"},
		Tools:          []string{"read"},
		SessionManager: codingagent.NewInMemorySessionManager(cwd),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Session().Dispose()
	if runtime.Session().SessionFile() != nil || runtime.Session().SettingsManager() != nil || runtime.Session().ResourceLoader() != nil {
		t.Fatalf("Headless session activated M3/M5 state: file=%v settings=%v resources=%v", runtime.Session().SessionFile(), runtime.Session().SettingsManager(), runtime.Session().ResourceLoader())
	}
	if got := runtime.Session().GetActiveToolNames(); !reflect.DeepEqual(got, []string{"read"}) {
		t.Fatalf("active tools = %v, want [read]", got)
	}
	if model := runtime.Session().Model(); model.Provider != ai.ProviderIDDeepSeek || model.ID != "deepseek-v4-flash" {
		t.Fatalf("model = %#v", model)
	}
}

func TestCreateHeadlessSessionBuildsDefaultSystemPromptAndHonorsOverride(t *testing.T) {
	tests := []struct {
		name      string
		override  *string
		tools     []string
		noTools   codingagent.NoToolsMode
		want      []string
		doNotWant []string
	}{
		{
			name:  "default prompt reflects active read tool",
			tools: []string{"read"},
			want: []string{
				"You are an expert coding assistant operating inside pig",
				"- read: Read file contents",
				"- Use read to examine files instead of cat or sed.",
			},
		},
		{
			name:      "default prompt reflects tool suppression",
			noTools:   codingagent.NoToolsAll,
			want:      []string{"Available tools:\n(none)"},
			doNotWant: []string{"- read: Read file contents", "Use read to examine files"},
		},
		{
			name:      "explicit override replaces default prompt",
			override:  ptrString("answer only with the requested value"),
			tools:     []string{"read"},
			want:      []string{"answer only with the requested value"},
			doNotWant: []string{"expert coding assistant", "Available tools:"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cwd := t.TempDir()
			runtime, err := codingagent.CreateHeadlessSession(context.Background(), codingagent.CreateHeadlessSessionOptions{
				CWD:            cwd,
				Provider:       ai.ProviderIDDeepSeek,
				Model:          "deepseek-v4-flash",
				Environment:    ai.ProviderEnv{"DEEPSEEK_API_KEY": "offline-test-key"},
				Tools:          test.tools,
				NoTools:        test.noTools,
				SystemPrompt:   test.override,
				SessionManager: codingagent.NewInMemorySessionManager(cwd),
			})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := runtime.Dispose(context.Background()); err != nil {
					t.Errorf("Dispose() error = %v", err)
				}
			})

			prompt := runtime.Session().SystemPrompt()
			for _, want := range append(test.want, "Current working directory: "+filepath.ToSlash(cwd)) {
				if !strings.Contains(prompt, want) {
					t.Errorf("system prompt = %q, want substring %q", prompt, want)
				}
			}
			for _, unwanted := range test.doNotWant {
				if strings.Contains(prompt, unwanted) {
					t.Errorf("system prompt = %q, do not want substring %q", prompt, unwanted)
				}
			}
		})
	}
}

func ptrString(value string) *string { return &value }

func TestHeadlessRunnerCompletesOfflineReadContinuation(t *testing.T) {
	cwd := t.TempDir()
	const sentinel = "issue-56-headless-read-sentinel"
	if err := os.WriteFile(filepath.Join(cwd, "sentinel.txt"), []byte(sentinel), 0o600); err != nil {
		t.Fatal(err)
	}
	readTool, err := codingagent.CreateReadTool(cwd)
	if err != nil {
		t.Fatal(err)
	}
	call, err := ai.FauxToolCall("read", map[string]any{"path": "sentinel.txt"}, ai.FauxToolCallOptions{ID: ai.Some("headless-read")})
	if err != nil {
		t.Fatal(err)
	}
	readRequest, err := ai.FauxAssistantMessage(
		ai.FauxAssistantBlocks(call),
		ai.FauxAssistantMessageOptions{StopReason: ai.Some(ai.StopReasonToolUse)},
	)
	if err != nil {
		t.Fatal(err)
	}
	answer, err := ai.FauxAssistantMessage(ai.FauxAssistantText("read: " + sentinel))
	if err != nil {
		t.Fatal(err)
	}
	provider, err := ai.NewFauxProvider()
	if err != nil {
		t.Fatal(err)
	}
	provider.SetResponses([]ai.FauxResponseStep{
		readRequest,
		ai.FauxResponseFactory(func(input ai.Context, _ *ai.SimpleStreamOptions, _ *ai.FauxProviderState, _ ai.Model) (ai.AssistantMessage, error) {
			result, ok := input.Messages[len(input.Messages)-1].(ai.ToolResultMessage)
			if !ok || result.IsError || len(result.Content) != 1 || result.Content[0].(ai.TextContent).Text != sentinel {
				t.Fatalf("read continuation input = %#v", input.Messages)
			}
			return answer, nil
		}),
	})
	model, _ := provider.GetModel()
	created, err := codingagent.CreateAgentSession(context.Background(), codingagent.CreateAgentSessionOptions{
		CWD:        cwd,
		Model:      &model,
		Provider:   provider.Provider,
		AgentTools: []agent.ErasedAgentTool{readTool},
	})
	if err != nil {
		t.Fatal(err)
	}
	runtime := codingagent.NewAgentSessionRuntime(created.Session, codingagent.AgentSessionServices{CWD: cwd}, nil, nil, nil)
	prompt := "read sentinel.txt"
	outcome, err := codingagent.RunHeadless(context.Background(), runtime, codingagent.HeadlessRunOptions{InitialMessage: &prompt})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(outcome.Text, []string{"read: " + sentinel}) {
		t.Fatalf("text = %#v", outcome.Text)
	}
}

func newHeadlessFauxRuntime(t *testing.T, responses []ai.FauxResponseStep, rate *float64, minTokenSize *int) *codingagent.AgentSessionRuntime {
	t.Helper()
	options := ai.RegisterFauxProviderOptions{TokensPerSecond: rate}
	if minTokenSize != nil {
		options.TokenSize = &ai.FauxTokenSize{Min: minTokenSize}
	}
	provider, err := ai.NewFauxProvider(options)
	if err != nil {
		t.Fatal(err)
	}
	provider.SetResponses(responses)
	model, ok := provider.GetModel()
	if !ok {
		t.Fatal("Faux provider has no model")
	}
	created, err := codingagent.CreateAgentSession(context.Background(), codingagent.CreateAgentSessionOptions{
		Model:          &model,
		Provider:       provider.Provider,
		SessionManager: codingagent.NewInMemorySessionManager(""),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := created.Session.Dispose(); err != nil {
			t.Errorf("Dispose() error = %v", err)
		}
	})
	return codingagent.NewAgentSessionRuntime(created.Session, codingagent.AgentSessionServices{}, nil, nil, nil)
}

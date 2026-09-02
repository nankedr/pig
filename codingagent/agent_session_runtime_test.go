package codingagent_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
)

func TestCreateAgentSessionRunsTextRoundAndSettlesAfterTranscriptUpdate(t *testing.T) {
	env := newSDKSentinelEnvironment(t)
	response, err := ai.FauxAssistantMessage(
		ai.FauxAssistantText("hello from the SDK"),
		ai.FauxAssistantMessageOptions{Timestamp: ai.Some(int64(2))},
	)
	if err != nil {
		t.Fatal(err)
	}
	minTokenSize := 100
	core, err := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{
		TokenSize: &ai.FauxTokenSize{Min: &minTokenSize},
	})
	if err != nil {
		t.Fatal(err)
	}
	core.SetResponses([]ai.FauxResponseStep{response})
	model, _ := core.GetModel()
	var providerSessionID *string
	var providerCacheRetention *ai.CacheRetention
	stream := func(ctx context.Context, model ai.Model, input ai.Context, options ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
		if options.SessionID != nil {
			value := *options.SessionID
			providerSessionID = &value
		}
		if options.CacheRetention != nil {
			value := *options.CacheRetention
			providerCacheRetention = &value
		}
		return core.StreamSimple(ctx, model, input, options)
	}

	result, err := codingagent.CreateAgentSession(context.Background(), codingagent.CreateAgentSessionOptions{
		CWD:            env.cwd,
		AgentDir:       env.agentDir,
		Model:          &model,
		StreamFunction: stream,
	})
	if err != nil {
		t.Fatalf("CreateAgentSession() error = %v", err)
	}
	session := result.Session
	if session == nil {
		t.Fatal("CreateAgentSession() returned a nil Session")
	}
	if session.SessionID() == "" || session.SessionManager().GetHeader().ID != session.SessionID() {
		t.Fatalf("session/header IDs = (%q, %#v), want one non-empty ID", session.SessionID(), session.SessionManager().GetHeader())
	}
	if session.SessionFile() != nil || session.SessionManager().IsPersisted() {
		t.Fatalf("in-memory SessionFile = %v, persisted = %t", session.SessionFile(), session.SessionManager().IsPersisted())
	}

	settledStarted := make(chan struct{})
	releaseSettled := make(chan struct{})
	var eventTypes []codingagent.AgentSessionEventType
	unsubscribe, err := session.Subscribe(func(event codingagent.AgentSessionEvent) {
		eventTypes = append(eventTypes, event.AgentSessionEventType())
		if event.AgentSessionEventType() != codingagent.AgentSessionEventTypeAgentSettled {
			return
		}
		entries := session.SessionManager().GetEntries()
		if len(entries) != 2 || entries[0].Message.MessageRole() != ai.MessageRoleUser || entries[1].Message.MessageRole() != ai.MessageRoleAssistant {
			t.Errorf("transcript at agent_settled = %#v, want user and assistant", entries)
		}
		close(settledStarted)
		<-releaseSettled
	})
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}

	promptDone := make(chan error, 1)
	go func() { promptDone <- session.Prompt(context.Background(), "hello") }()
	select {
	case <-settledStarted:
	case err := <-promptDone:
		t.Fatalf("Prompt() completed before agent_settled listener: %v", err)
	case <-time.After(time.Second):
		t.Fatal("Prompt() did not publish agent_settled")
	}
	if session.IsIdle() {
		t.Fatal("Session became idle before agent_settled listeners completed")
	}
	waitContext, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := session.WaitForIdle(waitContext); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("WaitForIdle() error = %v, want deadline while agent_settled is blocked", err)
	}

	close(releaseSettled)
	if err := <-promptDone; err != nil {
		t.Fatalf("Prompt() error = %v", err)
	}
	if err := session.WaitForIdle(context.Background()); err != nil {
		t.Fatalf("WaitForIdle() error = %v", err)
	}
	if !session.IsIdle() {
		t.Fatal("Session remained busy after Prompt settled")
	}
	wantEventTypes := []codingagent.AgentSessionEventType{
		codingagent.AgentSessionEventTypeAgentStart,
		codingagent.AgentSessionEventTypeTurnStart,
		codingagent.AgentSessionEventTypeMessageStart,
		codingagent.AgentSessionEventTypeMessageEnd,
		codingagent.AgentSessionEventTypeMessageStart,
		codingagent.AgentSessionEventTypeMessageUpdate,
		codingagent.AgentSessionEventTypeMessageUpdate,
		codingagent.AgentSessionEventTypeMessageUpdate,
		codingagent.AgentSessionEventTypeMessageEnd,
		codingagent.AgentSessionEventTypeTurnEnd,
		codingagent.AgentSessionEventTypeAgentEnd,
		codingagent.AgentSessionEventTypeAgentSettled,
	}
	if !reflect.DeepEqual(eventTypes, wantEventTypes) {
		t.Fatalf("event types = %v, want %v", eventTypes, wantEventTypes)
	}
	if got := session.Messages(); len(got) != 2 {
		t.Fatalf("Agent transcript = %#v, want two messages", got)
	}
	if providerSessionID == nil || *providerSessionID != session.SessionID() {
		t.Fatalf("Provider SessionID = %v, want %q", providerSessionID, session.SessionID())
	}
	if providerCacheRetention == nil || *providerCacheRetention != ai.CacheRetentionNone {
		t.Fatalf("Provider CacheRetention = %v, want %q", providerCacheRetention, ai.CacheRetentionNone)
	}

	unsubscribe()
	if err := session.Dispose(); err != nil {
		t.Fatalf("Dispose() error = %v", err)
	}
	if err := session.Dispose(); err != nil {
		t.Fatalf("second Dispose() error = %v", err)
	}
	env.assertUnchanged(t)
}

func TestCreateAgentSessionRunsReadContinuationWithInjectedProviderAndTool(t *testing.T) {
	cwd := t.TempDir()
	const sentinel = "issue-55-public-sdk-read-sentinel"
	if err := os.WriteFile(filepath.Join(cwd, "sentinel.txt"), []byte(sentinel), 0o600); err != nil {
		t.Fatal(err)
	}
	readTool, err := codingagent.CreateReadTool(cwd)
	if err != nil {
		t.Fatal(err)
	}
	call, err := ai.FauxToolCall("read", map[string]any{"path": "sentinel.txt"}, ai.FauxToolCallOptions{ID: ai.Some("read-sdk")})
	if err != nil {
		t.Fatal(err)
	}
	toolResponse, err := ai.FauxAssistantMessage(
		ai.FauxAssistantBlocks(call),
		ai.FauxAssistantMessageOptions{StopReason: ai.Some(ai.StopReasonToolUse), Timestamp: ai.Some(int64(2))},
	)
	if err != nil {
		t.Fatal(err)
	}
	finalResponse, err := ai.FauxAssistantMessage(
		ai.FauxAssistantText("The SDK read "+sentinel),
		ai.FauxAssistantMessageOptions{Timestamp: ai.Some(int64(4))},
	)
	if err != nil {
		t.Fatal(err)
	}
	fauxProvider, err := ai.NewFauxProvider()
	if err != nil {
		t.Fatal(err)
	}
	fauxProvider.SetResponses([]ai.FauxResponseStep{
		toolResponse,
		ai.FauxResponseFactory(func(input ai.Context, _ *ai.SimpleStreamOptions, _ *ai.FauxProviderState, _ ai.Model) (ai.AssistantMessage, error) {
			if len(input.Messages) != 3 {
				t.Fatalf("continuation context = %#v, want user/Assistant/ToolResult", input.Messages)
			}
			toolResult, ok := input.Messages[2].(ai.ToolResultMessage)
			if !ok || toolResult.IsError || toolResult.ToolCallID != "read-sdk" {
				t.Fatalf("continuation ToolResult = %#v", input.Messages[2])
			}
			text, ok := toolResult.Content[0].(ai.TextContent)
			if !ok || text.Text != sentinel {
				t.Fatalf("read ToolResult content = %#v, want %q", toolResult.Content, sentinel)
			}
			return finalResponse, nil
		}),
	})
	model, _ := fauxProvider.GetModel()

	result, err := codingagent.CreateAgentSession(context.Background(), codingagent.CreateAgentSessionOptions{
		CWD:        cwd,
		Model:      &model,
		Provider:   fauxProvider.Provider,
		AgentTools: []agent.ErasedAgentTool{readTool},
	})
	if err != nil {
		t.Fatalf("CreateAgentSession() error = %v", err)
	}
	session := result.Session
	if got := session.GetActiveToolNames(); !reflect.DeepEqual(got, []string{"read"}) {
		t.Fatalf("active Tool names = %v, want [read]", got)
	}
	var eventTypes []codingagent.AgentSessionEventType
	_, err = session.Subscribe(func(event codingagent.AgentSessionEvent) {
		if event.AgentSessionEventType() != codingagent.AgentSessionEventTypeMessageUpdate {
			eventTypes = append(eventTypes, event.AgentSessionEventType())
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Prompt(context.Background(), "read the sentinel"); err != nil {
		t.Fatalf("Prompt() error = %v", err)
	}
	wantEventTypes := []codingagent.AgentSessionEventType{
		codingagent.AgentSessionEventTypeAgentStart,
		codingagent.AgentSessionEventTypeTurnStart,
		codingagent.AgentSessionEventTypeMessageStart,
		codingagent.AgentSessionEventTypeMessageEnd,
		codingagent.AgentSessionEventTypeMessageStart,
		codingagent.AgentSessionEventTypeMessageEnd,
		codingagent.AgentSessionEventTypeToolExecutionStart,
		codingagent.AgentSessionEventTypeToolExecutionEnd,
		codingagent.AgentSessionEventTypeMessageStart,
		codingagent.AgentSessionEventTypeMessageEnd,
		codingagent.AgentSessionEventTypeTurnEnd,
		codingagent.AgentSessionEventTypeTurnStart,
		codingagent.AgentSessionEventTypeMessageStart,
		codingagent.AgentSessionEventTypeMessageEnd,
		codingagent.AgentSessionEventTypeTurnEnd,
		codingagent.AgentSessionEventTypeAgentEnd,
		codingagent.AgentSessionEventTypeAgentSettled,
	}
	if !reflect.DeepEqual(eventTypes, wantEventTypes) {
		t.Fatalf("non-update event types = %v, want %v", eventTypes, wantEventTypes)
	}

	messages := session.Messages()
	entries := session.SessionManager().GetEntries()
	if len(messages) != 4 || len(entries) != 4 {
		t.Fatalf("transcript lengths = Agent %d, SessionManager %d; want four", len(messages), len(entries))
	}
	resultMessage, ok := entries[2].Message.(ai.ToolResultMessage)
	if !ok || resultMessage.IsError || resultMessage.ToolCallID != "read-sdk" {
		t.Fatalf("persisted ToolResult = %#v", entries[2].Message)
	}
	final, ok := entries[3].Message.(ai.AssistantMessage)
	if !ok || len(final.Content) != 1 || final.Content[0].(ai.TextContent).Text != "The SDK read "+sentinel {
		t.Fatalf("final Assistant message = %#v", entries[3].Message)
	}
}

func TestAgentSessionAbortRequestsCancellationAndWaitForIdleObservesSettledTranscript(t *testing.T) {
	rate := 100.0
	minTokenSize := 1
	response, err := ai.FauxAssistantMessage(ai.FauxAssistantText("partial content that should be interrupted"))
	if err != nil {
		t.Fatal(err)
	}
	core, err := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{
		TokensPerSecond: &rate,
		TokenSize:       &ai.FauxTokenSize{Min: &minTokenSize},
	})
	if err != nil {
		t.Fatal(err)
	}
	core.SetResponses([]ai.FauxResponseStep{response})
	model, _ := core.GetModel()
	created, err := codingagent.CreateAgentSession(context.Background(), codingagent.CreateAgentSessionOptions{
		Model:          &model,
		StreamFunction: agent.StreamFunction(core.StreamSimple),
	})
	if err != nil {
		t.Fatal(err)
	}
	session := created.Session
	firstContent := make(chan struct{})
	settled := make(chan struct{})
	_, err = session.Subscribe(func(event codingagent.AgentSessionEvent) {
		switch event := event.(type) {
		case codingagent.AgentSessionMessageUpdateEvent:
			assistant := event.Message.(ai.AssistantMessage)
			if len(assistant.Content) != 0 {
				if text, ok := assistant.Content[0].(ai.TextContent); ok && text.Text != "" {
					select {
					case <-firstContent:
					default:
						close(firstContent)
					}
				}
			}
		case codingagent.AgentSessionAgentSettledEvent:
			close(settled)
		}
	})
	if err != nil {
		t.Fatal(err)
	}

	promptDone := make(chan error, 1)
	go func() { promptDone <- session.Prompt(context.Background(), "start") }()
	select {
	case <-firstContent:
	case <-time.After(time.Second):
		t.Fatal("Prompt did not begin streaming")
	}
	if err := session.Abort(); err != nil {
		t.Fatalf("Abort() error = %v", err)
	}
	if err := session.WaitForIdle(context.Background()); err != nil {
		t.Fatalf("WaitForIdle() error = %v", err)
	}
	select {
	case <-settled:
	default:
		t.Fatal("WaitForIdle() returned before agent_settled")
	}
	if err := <-promptDone; err != nil {
		t.Fatalf("Prompt() after Abort error = %v", err)
	}
	entries := session.SessionManager().GetEntries()
	if len(entries) != 2 {
		t.Fatalf("aborted transcript = %#v, want user and partial Assistant", entries)
	}
	assistant, ok := entries[1].Message.(ai.AssistantMessage)
	if !ok || assistant.StopReason != ai.StopReasonAborted {
		t.Fatalf("aborted Assistant = %#v", entries[1].Message)
	}
	text := assistant.Content[0].(ai.TextContent).Text
	if text == "" || len(text) >= len("partial content that should be interrupted") {
		t.Fatalf("aborted partial text = %q", text)
	}
}

func TestAgentSessionAbortIsSafeFromSynchronousListeners(t *testing.T) {
	rate := 100.0
	minTokenSize := 1
	response, err := ai.FauxAssistantMessage(ai.FauxAssistantText("listener cancellation must not deadlock"))
	if err != nil {
		t.Fatal(err)
	}
	core, err := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{
		TokensPerSecond: &rate,
		TokenSize:       &ai.FauxTokenSize{Min: &minTokenSize},
	})
	if err != nil {
		t.Fatal(err)
	}
	core.SetResponses([]ai.FauxResponseStep{response})
	model, _ := core.GetModel()
	created, err := codingagent.CreateAgentSession(context.Background(), codingagent.CreateAgentSessionOptions{
		Model:          &model,
		StreamFunction: agent.StreamFunction(core.StreamSimple),
	})
	if err != nil {
		t.Fatal(err)
	}
	session := created.Session
	abortReturned := make(chan struct{})
	settledAbortReturned := make(chan struct{})
	var abortOnce, settledOnce sync.Once
	_, err = session.Subscribe(func(event codingagent.AgentSessionEvent) {
		switch event.AgentSessionEventType() {
		case codingagent.AgentSessionEventTypeMessageUpdate:
			abortOnce.Do(func() {
				if err := session.Abort(); err != nil {
					t.Errorf("Abort() from message listener: %v", err)
				}
				close(abortReturned)
			})
		case codingagent.AgentSessionEventTypeAgentSettled:
			settledOnce.Do(func() {
				if err := session.Abort(); err != nil {
					t.Errorf("Abort() from settled listener: %v", err)
				}
				close(settledAbortReturned)
			})
		}
	})
	if err != nil {
		t.Fatal(err)
	}

	promptDone := make(chan error, 1)
	go func() { promptDone <- session.Prompt(context.Background(), "start") }()
	select {
	case <-abortReturned:
	case <-time.After(time.Second):
		t.Fatal("Abort() blocked inside message listener")
	}
	select {
	case err := <-promptDone:
		if err != nil {
			t.Fatalf("Prompt() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Prompt() did not settle after listener cancellation")
	}
	select {
	case <-settledAbortReturned:
	default:
		t.Fatal("Abort() blocked inside agent_settled listener")
	}
}

func TestAgentSessionDisposeCancelsWithoutDroppingActiveTranscript(t *testing.T) {
	rate := 100.0
	minTokenSize := 1
	response, err := ai.FauxAssistantMessage(ai.FauxAssistantText("dispose should retain this partial response"))
	if err != nil {
		t.Fatal(err)
	}
	core, err := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{
		TokensPerSecond: &rate,
		TokenSize:       &ai.FauxTokenSize{Min: &minTokenSize},
	})
	if err != nil {
		t.Fatal(err)
	}
	core.SetResponses([]ai.FauxResponseStep{response})
	model, _ := core.GetModel()
	created, err := codingagent.CreateAgentSession(context.Background(), codingagent.CreateAgentSessionOptions{
		Model:          &model,
		StreamFunction: agent.StreamFunction(core.StreamSimple),
	})
	if err != nil {
		t.Fatal(err)
	}
	session := created.Session
	firstContent := make(chan struct{})
	releaseContent := make(chan struct{})
	_, err = session.Subscribe(func(event codingagent.AgentSessionEvent) {
		switch event := event.(type) {
		case codingagent.AgentSessionMessageUpdateEvent:
			assistant := event.Message.(ai.AssistantMessage)
			if len(assistant.Content) != 0 {
				if text, ok := assistant.Content[0].(ai.TextContent); ok && text.Text != "" {
					select {
					case <-firstContent:
					default:
						close(firstContent)
					}
					<-releaseContent
				}
			}
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	promptDone := make(chan error, 1)
	go func() { promptDone <- session.Prompt(context.Background(), "start") }()
	select {
	case <-firstContent:
	case <-time.After(time.Second):
		t.Fatal("Prompt did not begin streaming")
	}
	disposeDone := make(chan error, 1)
	go func() { disposeDone <- session.Dispose() }()
	select {
	case err := <-disposeDone:
		if err != nil {
			t.Fatalf("Dispose() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Dispose() waited for an active event listener")
	}
	close(releaseContent)
	if err := session.WaitForIdle(context.Background()); err != nil {
		t.Fatalf("WaitForIdle() after Dispose error = %v", err)
	}
	if err := <-promptDone; err != nil {
		t.Fatalf("Prompt() after Dispose error = %v", err)
	}
	entries := session.SessionManager().GetEntries()
	if len(entries) != 2 {
		t.Fatalf("disposed transcript = %#v, want user and partial Assistant", entries)
	}
	assistant, ok := entries[1].Message.(ai.AssistantMessage)
	if !ok || assistant.StopReason != ai.StopReasonAborted {
		t.Fatalf("disposed Assistant = %#v", entries[1].Message)
	}
	if err := session.Dispose(); err != nil {
		t.Fatalf("second Dispose() error = %v", err)
	}
}

func TestAgentSessionUnsupportedPromptOptionsAreExactInertStubs(t *testing.T) {
	falseValue := false
	callbackCalls := 0
	tests := []struct {
		name      string
		option    codingagent.PromptOptions
		operation string
	}{
		{name: "template expansion", option: codingagent.PromptOptions{ExpandPromptTemplates: &falseValue}, operation: "AgentSession.Prompt.ExpandPromptTemplates"},
		{name: "images", option: codingagent.PromptOptions{Images: []ai.ImageContent{{Type: ai.ContentTypeImage, Data: "aGk=", MIMEType: "image/png"}}}, operation: "AgentSession.Prompt.Images"},
		{name: "preflight callback", option: codingagent.PromptOptions{PreflightResult: func(bool) { callbackCalls++ }}, operation: "AgentSession.Prompt.PreflightResult"},
		{name: "source", option: codingagent.PromptOptions{Source: "rpc"}, operation: "AgentSession.Prompt.Source"},
		{name: "streaming behavior", option: codingagent.PromptOptions{StreamingBehavior: "steer"}, operation: "AgentSession.Prompt.StreamingBehavior"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			session := codingagent.NewAgentSession(codingagent.AgentSessionConfig{})
			err := session.Prompt(context.Background(), "must not run", test.option)
			assertCodingAgentNotImplemented(t, err, test.operation)
			if !session.IsIdle() {
				t.Fatal("unsupported Prompt option activated the session")
			}
		})
	}
	if callbackCalls != 0 {
		t.Fatalf("unsupported preflight callback calls = %d, want zero", callbackCalls)
	}
}

func TestCreateAgentSessionNoToolsModesMatchPublicSelectionContract(t *testing.T) {
	read, err := codingagent.CreateReadTool(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	custom, err := agent.EraseAgentTool(agent.AgentTool[ai.JSONValue, ai.JSONValue]{
		Tool: ai.Tool{Name: "lookup", Description: "lookup", Parameters: json.RawMessage(`{"type":"object"}`)},
	})
	if err != nil {
		t.Fatal(err)
	}
	core, err := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	model, _ := core.GetModel()
	tests := []struct {
		name  string
		mode  codingagent.NoToolsMode
		allow []string
		want  []string
	}{
		{name: "builtin suppression retains custom", mode: codingagent.NoToolsBuiltin, want: []string{"lookup"}},
		{name: "all suppression", mode: codingagent.NoToolsAll},
		{name: "explicit allowlist overrides builtin suppression", mode: codingagent.NoToolsBuiltin, allow: []string{"read"}, want: []string{"read"}},
		{name: "explicit allowlist overrides all suppression", mode: codingagent.NoToolsAll, allow: []string{"lookup"}, want: []string{"lookup"}},
		{name: "explicit empty allowlist", allow: []string{}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			created, err := codingagent.CreateAgentSession(context.Background(), codingagent.CreateAgentSessionOptions{
				Model:          &model,
				StreamFunction: agent.StreamFunction(core.StreamSimple),
				AgentTools:     []agent.ErasedAgentTool{read, custom},
				Tools:          test.allow,
				NoTools:        test.mode,
			})
			if err != nil {
				t.Fatal(err)
			}
			if got := created.Session.GetActiveToolNames(); !slices.Equal(got, test.want) {
				t.Fatalf("active Tools = %v, want %v", got, test.want)
			}
		})
	}
}

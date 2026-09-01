package agent_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
)

func TestAgentPromptTextSettlesAfterOrderedAgentEndListeners(t *testing.T) {
	response, err := ai.FauxAssistantMessage(ai.FauxAssistantText("ok"), ai.FauxAssistantMessageOptions{
		Timestamp: ai.Some(int64(2)),
	})
	if err != nil {
		t.Fatal(err)
	}
	core, err := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	core.SetResponses([]ai.FauxResponseStep{response})
	model, _ := core.GetModel()
	created, err := agent.NewAgent(agent.AgentOptions{
		InitialState:   &agent.AgentInitialState{Model: model},
		StreamFunction: agent.StreamFunction(core.StreamSimple),
	})
	if err != nil {
		t.Fatal(err)
	}

	agentEndStarted := make(chan struct{})
	releaseAgentEnd := make(chan struct{})
	var listenerOrder []string
	created.Subscribe(func(_ context.Context, event agent.AgentEvent) error {
		if event.AgentEventType() == agent.AgentEventTypeAgentEnd {
			listenerOrder = append(listenerOrder, "first-start")
			close(agentEndStarted)
			<-releaseAgentEnd
			listenerOrder = append(listenerOrder, "first-end")
		}
		return nil
	})
	created.Subscribe(func(_ context.Context, event agent.AgentEvent) error {
		if event.AgentEventType() == agent.AgentEventTypeAgentEnd {
			listenerOrder = append(listenerOrder, "second")
		}
		return nil
	})

	promptDone := make(chan error, 1)
	go func() { promptDone <- created.PromptText(context.Background(), "hello") }()
	select {
	case <-agentEndStarted:
	case err := <-promptDone:
		t.Fatalf("PromptText() completed before agent_end listener: %v", err)
	case <-time.After(time.Second):
		t.Fatal("PromptText() did not reach agent_end listener")
	}
	if !created.Busy() || !created.State().IsStreaming {
		t.Fatal("Agent became idle before agent_end listeners settled")
	}
	waitContext, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := created.WaitForIdle(waitContext); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("WaitForIdle() error = %v, want deadline while listener is blocked", err)
	}
	close(releaseAgentEnd)
	if err := <-promptDone; err != nil {
		t.Fatalf("PromptText() error = %v", err)
	}
	if !reflect.DeepEqual(listenerOrder, []string{"first-start", "first-end", "second"}) {
		t.Fatalf("listener order = %v", listenerOrder)
	}
	if created.Busy() || created.State().IsStreaming {
		t.Fatal("Agent remained busy after PromptText settled")
	}
	state := created.State()
	if len(state.Messages) != 2 || state.Messages[0].MessageRole() != ai.MessageRoleUser || state.Messages[1].MessageRole() != ai.MessageRoleAssistant {
		t.Fatalf("transcript = %#v, want user and assistant", state.Messages)
	}
}

func TestAgentProviderErrorAndAbortKeepPartialAssistantContent(t *testing.T) {
	t.Run("provider error", func(t *testing.T) {
		response, err := ai.FauxAssistantMessage(ai.FauxAssistantText("partial"), ai.FauxAssistantMessageOptions{
			StopReason:   ai.Some(ai.StopReasonError),
			ErrorMessage: ai.Some("provider failed"),
		})
		if err != nil {
			t.Fatal(err)
		}
		created := newFauxAgent(t, ai.RegisterFauxProviderOptions{}, response)
		if err := created.PromptText(context.Background(), "hello"); err != nil {
			t.Fatalf("PromptText() error = %v", err)
		}
		assertAgentFailureState(t, created, ai.StopReasonError, "partial", "provider failed")
	})

	t.Run("abort", func(t *testing.T) {
		rate := 100.0
		minTokenSize := 1
		response, err := ai.FauxAssistantMessage(ai.FauxAssistantText("partial content that should stop"))
		if err != nil {
			t.Fatal(err)
		}
		created := newFauxAgent(t, ai.RegisterFauxProviderOptions{
			TokensPerSecond: &rate,
			TokenSize:       &ai.FauxTokenSize{Min: &minTokenSize},
		}, response)
		created.Subscribe(func(_ context.Context, event agent.AgentEvent) error {
			update, ok := event.(agent.MessageUpdateEvent)
			if !ok {
				return nil
			}
			assistant := update.Message.(ai.AssistantMessage)
			if len(assistant.Content) != 0 {
				if text, ok := assistant.Content[0].(ai.TextContent); ok && text.Text != "" {
					created.Abort()
				}
			}
			return nil
		})
		if err := created.PromptText(context.Background(), "hello"); err != nil {
			t.Fatalf("PromptText() error = %v", err)
		}
		state := created.State()
		assistant := state.Messages[len(state.Messages)-1].(ai.AssistantMessage)
		text := assistant.Content[0].(ai.TextContent).Text
		if text == "" || len(text) >= len("partial content that should stop") {
			t.Fatalf("aborted partial text = %q", text)
		}
		assertAgentFailureState(t, created, ai.StopReasonAborted, text, "Request was aborted")
	})
}

func TestAgentPromptAndContinueKeepM2QueuesOutOfRuntime(t *testing.T) {
	core, err := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	response := func(text string, wantMessages int) ai.FauxResponseFactory {
		return func(input ai.Context, _ *ai.SimpleStreamOptions, _ *ai.FauxProviderState, _ ai.Model) (ai.AssistantMessage, error) {
			if len(input.Messages) != wantMessages {
				t.Fatalf("model messages = %d, want %d", len(input.Messages), wantMessages)
			}
			for _, message := range input.Messages {
				if message.MessageRole() != ai.MessageRoleUser && message.MessageRole() != ai.MessageRoleAssistant {
					t.Fatalf("custom Agent message reached model context: %T", message)
				}
			}
			return ai.FauxAssistantMessage(ai.FauxAssistantText(text))
		}
	}
	core.SetResponses([]ai.FauxResponseStep{response("continued", 1), response("prompted", 3)})
	model, _ := core.GetModel()
	created, err := agent.NewAgent(agent.AgentOptions{
		InitialState: &agent.AgentInitialState{
			Model:    model,
			Messages: []agent.AgentMessage{userMessage("existing")},
		},
		StreamFunction: agent.StreamFunction(core.StreamSimple),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := created.Steer(userMessage("queued steering")); err != nil {
		t.Fatal(err)
	}
	if err := created.FollowUp(userMessage("queued follow-up")); err != nil {
		t.Fatal(err)
	}
	if err := created.Continue(context.Background()); err != nil {
		t.Fatalf("Continue() error = %v", err)
	}
	custom, err := agent.NewRawAgentMessage(json.RawMessage(`{"role":"notice","text":"private"}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := created.Prompt(context.Background(), custom, userMessage("next")); err != nil {
		t.Fatalf("Prompt() error = %v", err)
	}
	state := created.State()
	wantRoles := []ai.MessageRole{ai.MessageRoleUser, ai.MessageRoleAssistant, "notice", ai.MessageRoleUser, ai.MessageRoleAssistant}
	gotRoles := make([]ai.MessageRole, len(state.Messages))
	for i, message := range state.Messages {
		gotRoles[i] = message.MessageRole()
	}
	if !reflect.DeepEqual(gotRoles, wantRoles) {
		t.Fatalf("transcript roles = %v, want %v", gotRoles, wantRoles)
	}
	if !created.HasQueuedMessages() {
		t.Fatal("M2 steering/follow-up queues were consumed by M1 runtime")
	}
	if err := created.Continue(context.Background()); err == nil {
		t.Fatal("Continue() accepted an assistant transcript tail")
	}
	if !created.HasQueuedMessages() {
		t.Fatal("illegal Continue consumed M2 queues")
	}
}

func TestAgentPrepareNextTurnRemainsExplicitCapabilityStub(t *testing.T) {
	callbackCalled := false
	streamCalled := false
	listenerCalled := false
	created, err := agent.NewAgent(agent.AgentOptions{
		PrepareNextTurn: func(context.Context) (*agent.AgentLoopTurnUpdate, error) {
			callbackCalled = true
			return nil, nil
		},
		StreamFunction: func(context.Context, ai.Model, ai.Context, ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
			streamCalled = true
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	created.Subscribe(func(context.Context, agent.AgentEvent) error {
		listenerCalled = true
		return nil
	})
	err = created.Prompt(context.Background(), userMessage("prompt"))
	if !errors.Is(err, agent.ErrNotImplemented) {
		t.Fatalf("Prompt() error = %v, want ErrNotImplemented", err)
	}
	state := created.State()
	if callbackCalled || streamCalled || listenerCalled || created.Busy() || state.IsStreaming || len(state.Messages) != 0 {
		t.Fatalf("PrepareNextTurn stub side effects: callback=%t stream=%t listener=%t state=%#v", callbackCalled, streamCalled, listenerCalled, state)
	}
}

func newFauxAgent(t *testing.T, options ai.RegisterFauxProviderOptions, responses ...ai.FauxResponseStep) *agent.Agent {
	t.Helper()
	core, err := ai.CreateFauxCore(options)
	if err != nil {
		t.Fatal(err)
	}
	core.SetResponses(responses)
	model, _ := core.GetModel()
	created, err := agent.NewAgent(agent.AgentOptions{
		InitialState:   &agent.AgentInitialState{Model: model},
		StreamFunction: agent.StreamFunction(core.StreamSimple),
	})
	if err != nil {
		t.Fatal(err)
	}
	return created
}

func assertAgentFailureState(t *testing.T, created *agent.Agent, reason ai.StopReason, text, errorMessage string) {
	t.Helper()
	state := created.State()
	if created.Busy() || state.IsStreaming {
		t.Fatal("failed Agent run did not become idle")
	}
	if len(state.Messages) != 2 {
		t.Fatalf("transcript length = %d, want 2", len(state.Messages))
	}
	assistant := state.Messages[1].(ai.AssistantMessage)
	gotError, _ := assistant.ErrorMessage.Value()
	gotText := assistant.Content[0].(ai.TextContent).Text
	if assistant.StopReason != reason || gotText != text || gotError != errorMessage {
		t.Fatalf("assistant = %#v, text = %q, error = %q", assistant, gotText, gotError)
	}
	if state.ErrorMessage == nil || *state.ErrorMessage != errorMessage {
		t.Fatalf("State.ErrorMessage = %v", state.ErrorMessage)
	}
}

func TestAgentOptionsExposeRuntimeConfigurationCallbacks(t *testing.T) {
	t.Parallel()

	var convert agent.ConvertToLLMFunc = func(context.Context, []agent.AgentMessage) ([]ai.Message, error) {
		return nil, nil
	}
	var transform agent.TransformContextFunc = func(_ context.Context, messages []agent.AgentMessage) ([]agent.AgentMessage, error) {
		return messages, nil
	}
	delay := int64(5)
	options := agent.AgentOptions{
		ConvertToLLM:     convert,
		TransformContext: transform,
		StreamFunction: func(context.Context, ai.Model, ai.Context, ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
			return nil
		},
		GetAPIKey: func(context.Context, ai.ProviderID) (string, bool, error) { return "key", true, nil },
		OnPayload: func(context.Context, ai.JSONValue, ai.Model) (ai.PayloadHookResult, error) {
			return ai.PayloadHookResult{}, nil
		},
		OnResponse: func(context.Context, ai.ProviderResponse, ai.Model) error { return nil },
		BeforeToolCall: func(context.Context, agent.BeforeToolCallContext) (*agent.BeforeToolCallResult, error) {
			return nil, nil
		},
		AfterToolCall: func(context.Context, agent.AfterToolCallContext) (*agent.AfterToolCallResult, error) {
			return nil, nil
		},
		ShouldStopAfterTurn: func(context.Context, agent.ShouldStopAfterTurnContext) (bool, error) {
			return false, nil
		},
		PrepareNextTurn: func(context.Context) (*agent.AgentLoopTurnUpdate, error) { return nil, nil },
		PrepareNextTurnWithContext: func(context.Context, agent.PrepareNextTurnContext) (*agent.AgentLoopTurnUpdate, error) {
			return nil, nil
		},
		SteeringMode:    agent.QueueAll,
		FollowUpMode:    agent.QueueAll,
		SessionID:       "session",
		ThinkingBudgets: &ai.ThinkingBudgets{},
		Transport:       ai.TransportSSE,
		MaxRetryDelayMS: &delay,
		ToolExecution:   agent.ToolExecutionSequential,
	}
	created, err := agent.NewAgent(options)
	if err != nil {
		t.Fatalf("NewAgent(full options) error = %v", err)
	}
	if created.SteeringMode() != agent.QueueAll || created.FollowUpMode() != agent.QueueAll {
		t.Fatalf("configured queue modes = (%q, %q)", created.SteeringMode(), created.FollowUpMode())
	}
	if created.ConvertToLLM() == nil || created.TransformContext() == nil || created.StreamFunction() == nil || created.GetAPIKey() == nil || created.OnPayload() == nil || created.OnResponse() == nil || created.BeforeToolCall() == nil || created.AfterToolCall() == nil || created.ShouldStopAfterTurn() == nil || created.PrepareNextTurn() == nil || created.PrepareNextTurnWithContext() == nil {
		t.Fatal("Agent did not expose one or more configured runtime callbacks")
	}
	if created.SessionID() != "session" || created.Transport() != ai.TransportSSE || created.ToolExecution() != agent.ToolExecutionSequential {
		t.Fatalf("runtime configuration = session %q, transport %q, tools %q", created.SessionID(), created.Transport(), created.ToolExecution())
	}
	if got := created.MaxRetryDelayMS(); got == nil || *got != delay {
		t.Fatalf("MaxRetryDelayMS() = %v, want %d", got, delay)
	}
	if got := created.ThinkingBudgets(); got == nil {
		t.Fatal("ThinkingBudgets() = nil")
	}
}

func TestAgentRuntimeConfigurationCanBeRewiredAtomically(t *testing.T) {
	t.Parallel()

	created, err := agent.NewAgent(agent.AgentOptions{})
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}
	convert := agent.ConvertToLLMFunc(func(context.Context, []agent.AgentMessage) ([]ai.Message, error) { return nil, nil })
	transform := agent.TransformContextFunc(func(_ context.Context, messages []agent.AgentMessage) ([]agent.AgentMessage, error) {
		return messages, nil
	})
	stream := agent.StreamFunction(func(context.Context, ai.Model, ai.Context, ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
		return nil
	})
	getAPIKey := func(context.Context, ai.ProviderID) (string, bool, error) { return "key", true, nil }
	onPayload := ai.PayloadHook(func(context.Context, ai.JSONValue, ai.Model) (ai.PayloadHookResult, error) {
		return ai.PayloadHookResult{}, nil
	})
	onResponse := ai.ResponseHook(func(context.Context, ai.ProviderResponse, ai.Model) error { return nil })
	before := func(context.Context, agent.BeforeToolCallContext) (*agent.BeforeToolCallResult, error) {
		return nil, nil
	}
	after := func(context.Context, agent.AfterToolCallContext) (*agent.AfterToolCallResult, error) { return nil, nil }
	stop := func(context.Context, agent.ShouldStopAfterTurnContext) (bool, error) { return false, nil }
	prepare := func(context.Context) (*agent.AgentLoopTurnUpdate, error) { return nil, nil }
	prepareWithContext := func(context.Context, agent.PrepareNextTurnContext) (*agent.AgentLoopTurnUpdate, error) {
		return nil, nil
	}
	budget := int64(64)
	retry := int64(50)

	created.SetConvertToLLM(convert)
	created.SetTransformContext(transform)
	created.SetStreamFunction(stream)
	created.SetGetAPIKey(getAPIKey)
	created.SetOnPayload(onPayload)
	created.SetOnResponse(onResponse)
	created.SetBeforeToolCall(before)
	created.SetAfterToolCall(after)
	created.SetShouldStopAfterTurn(stop)
	created.SetPrepareNextTurn(prepare)
	created.SetPrepareNextTurnWithContext(prepareWithContext)
	created.SetSessionID("rewired")
	created.SetThinkingBudgets(&ai.ThinkingBudgets{High: &budget})
	if err := created.SetTransport(ai.TransportWebSocket); err != nil {
		t.Fatalf("SetTransport() error = %v", err)
	}
	if err := created.SetMaxRetryDelayMS(&retry); err != nil {
		t.Fatalf("SetMaxRetryDelayMS() error = %v", err)
	}
	if err := created.SetToolExecution(agent.ToolExecutionSequential); err != nil {
		t.Fatalf("SetToolExecution() error = %v", err)
	}

	if reflect.ValueOf(created.ConvertToLLM()).Pointer() != reflect.ValueOf(convert).Pointer() || reflect.ValueOf(created.TransformContext()).Pointer() != reflect.ValueOf(transform).Pointer() || reflect.ValueOf(created.StreamFunction()).Pointer() != reflect.ValueOf(stream).Pointer() || reflect.ValueOf(created.GetAPIKey()).Pointer() != reflect.ValueOf(getAPIKey).Pointer() || reflect.ValueOf(created.OnPayload()).Pointer() != reflect.ValueOf(onPayload).Pointer() || reflect.ValueOf(created.OnResponse()).Pointer() != reflect.ValueOf(onResponse).Pointer() || reflect.ValueOf(created.BeforeToolCall()).Pointer() != reflect.ValueOf(before).Pointer() || reflect.ValueOf(created.AfterToolCall()).Pointer() != reflect.ValueOf(after).Pointer() || reflect.ValueOf(created.ShouldStopAfterTurn()).Pointer() != reflect.ValueOf(stop).Pointer() || reflect.ValueOf(created.PrepareNextTurn()).Pointer() != reflect.ValueOf(prepare).Pointer() || reflect.ValueOf(created.PrepareNextTurnWithContext()).Pointer() != reflect.ValueOf(prepareWithContext).Pointer() {
		t.Fatal("one or more runtime callback setters did not replace the configured callback")
	}
	if created.SessionID() != "rewired" || created.Transport() != ai.TransportWebSocket || created.ToolExecution() != agent.ToolExecutionSequential {
		t.Fatalf("rewired scalar options = session %q, transport %q, tools %q", created.SessionID(), created.Transport(), created.ToolExecution())
	}

	returnedBudgets := created.ThinkingBudgets()
	returnedRetry := created.MaxRetryDelayMS()
	budget = 65
	retry = 51
	*returnedBudgets.High = 66
	*returnedRetry = 52
	if got := created.ThinkingBudgets(); got == nil || got.High == nil || *got.High != 64 {
		t.Fatalf("ThinkingBudgets retained caller storage: %#v", got)
	}
	if got := created.MaxRetryDelayMS(); got == nil || *got != 50 {
		t.Fatalf("MaxRetryDelayMS retained caller storage: %v", got)
	}

	if err := created.SetTransport(ai.Transport("invalid")); err == nil || created.Transport() != ai.TransportWebSocket {
		t.Fatalf("invalid transport changed value to %q; error %v", created.Transport(), err)
	}
	negative := int64(-1)
	if err := created.SetMaxRetryDelayMS(&negative); err == nil || *created.MaxRetryDelayMS() != 50 {
		t.Fatalf("invalid retry delay changed value to %v; error %v", created.MaxRetryDelayMS(), err)
	}
	if err := created.SetToolExecution(agent.ToolExecutionMode("invalid")); err == nil || created.ToolExecution() != agent.ToolExecutionSequential {
		t.Fatalf("invalid Tool execution changed value to %q; error %v", created.ToolExecution(), err)
	}

	created.SetConvertToLLM(nil)
	created.SetTransformContext(nil)
	created.SetStreamFunction(nil)
	created.SetGetAPIKey(nil)
	created.SetOnPayload(nil)
	created.SetOnResponse(nil)
	created.SetBeforeToolCall(nil)
	created.SetAfterToolCall(nil)
	created.SetShouldStopAfterTurn(nil)
	created.SetPrepareNextTurn(nil)
	created.SetPrepareNextTurnWithContext(nil)
	if created.ConvertToLLM() == nil || created.TransformContext() != nil || created.StreamFunction() != nil || created.GetAPIKey() != nil || created.OnPayload() != nil || created.OnResponse() != nil || created.BeforeToolCall() != nil || created.AfterToolCall() != nil || created.ShouldStopAfterTurn() != nil || created.PrepareNextTurn() != nil || created.PrepareNextTurnWithContext() != nil {
		t.Fatal("nil callback reset semantics are incorrect")
	}
}

func TestNewAgentUsesLegacyDefaultsAndCopiesInitialState(t *testing.T) {
	t.Parallel()

	messages := []agent.AgentMessage{userMessage("initial")}
	tools := []agent.ErasedAgentTool{erasedTestTool(t, "read", "Read", nil)}
	created, err := agent.NewAgent(agent.AgentOptions{InitialState: &agent.AgentInitialState{
		SystemPrompt: "system",
		Messages:     messages,
		Tools:        tools,
	}})
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}

	state := created.State()
	if state.SystemPrompt != "system" {
		t.Fatalf("SystemPrompt = %q, want system", state.SystemPrompt)
	}
	if state.Model.ID != "unknown" || state.Model.Name != "unknown" || state.Model.API != ai.API("unknown") || state.Model.Provider != ai.ProviderID("unknown") {
		t.Fatalf("default Model = %#v, want pinned Pi unknown model", state.Model)
	}
	if state.Model.Input == nil || len(state.Model.Input) != 0 {
		t.Fatalf("default Model.Input = %#v, want a non-nil empty slice", state.Model.Input)
	}
	if state.Model.BaseURL != "" || state.Model.Reasoning || state.Model.ContextWindow != 0 || state.Model.MaxTokens != 0 || state.Model.ThinkingLevelMap != nil || state.Model.SamplingParams != nil || state.Model.Headers != nil || state.Model.Compat.IsSet() {
		t.Fatalf("default Model has non-zero pinned fields: %#v", state.Model)
	}
	if !reflect.DeepEqual(state.Model.Cost, ai.ModelCost{}) {
		t.Fatalf("default Model.Cost = %#v, want zero cost", state.Model.Cost)
	}
	if state.ThinkingLevel != ai.ModelThinkingLevelOff {
		t.Fatalf("ThinkingLevel = %q, want off", state.ThinkingLevel)
	}
	if state.Tools == nil || state.Messages == nil || state.PendingToolCalls == nil {
		t.Fatalf("default collections = tools %#v, messages %#v, pending %#v; want non-nil empty values", state.Tools, state.Messages, state.PendingToolCalls)
	}
	if created.SteeringMode() != agent.QueueOneAtATime || created.FollowUpMode() != agent.QueueOneAtATime {
		t.Fatalf("queue modes = (%q, %q), want one-at-a-time", created.SteeringMode(), created.FollowUpMode())
	}
	if created.Busy() {
		t.Fatal("new Agent is busy")
	}
	if _, ok := created.ActiveContext(); ok {
		t.Fatal("new Agent has an active context")
	}

	messages[0] = userMessage("mutated")
	tools[0].Name = "mutated"
	state.Messages[0] = userMessage("snapshot-mutated")
	state.Tools[0].Name = "snapshot-mutated"
	state.Model.Input = append(state.Model.Input, ai.ModelInputText)
	state.PendingToolCalls["fake"] = struct{}{}
	next := created.State()
	if got := messageText(t, next.Messages[0]); got != "initial" {
		t.Fatalf("stored message = %q, want initial", got)
	}
	if got := next.Tools[0].Name; got != "read" {
		t.Fatalf("stored Tool name = %q, want read", got)
	}
	if len(next.Model.Input) != 0 || len(next.PendingToolCalls) != 0 {
		t.Fatalf("State exposed mutable Model or pending calls: %#v", next)
	}
}

func TestAgentStateDeepCopiesCustomMessagesAndErrorPointers(t *testing.T) {
	t.Parallel()

	funcMessage := &customStateMessage{
		Role:    "notice",
		Payload: map[string]any{"items": []any{"original"}},
		hidden:  map[string][]byte{"secret": []byte("original")},
	}
	model := ai.Model{
		ID: "initial", Input: []ai.ModelInput{ai.ModelInputText},
		Headers: map[string]string{"initial": "value"},
	}
	tool := erasedTestTool(t, "initial", "", nil)
	created, err := agent.NewAgent(agent.AgentOptions{InitialState: &agent.AgentInitialState{
		Model: model, Tools: []agent.ErasedAgentTool{tool}, Messages: []agent.AgentMessage{funcMessage},
	}})
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}
	funcMessage.Payload["items"].([]any)[0] = "input-mutated"
	funcMessage.hidden["secret"][0] = 'I'
	model.Input[0] = ai.ModelInputImage
	model.Headers["initial"] = "mutated"
	tool.Parameters[0] = '['

	snapshot := created.State()
	got := snapshot.Messages[0].(*customStateMessage)
	if value := got.Payload["items"].([]any)[0]; value != "original" {
		t.Fatalf("custom message value = %v, want original", value)
	}
	if value := string(got.hidden["secret"]); value != "original" {
		t.Fatalf("custom message hidden value = %q, want original", value)
	}
	if snapshot.Model.Input[0] != ai.ModelInputText || snapshot.Model.Headers["initial"] != "value" || string(snapshot.Tools[0].Parameters) != `{"type":"object"}` {
		t.Fatalf("constructor retained nested caller storage: %#v", snapshot)
	}
	got.Payload["items"].([]any)[0] = "snapshot-mutated"
	got.hidden["secret"][0] = 'X'
	gotAgain := created.State().Messages[0].(*customStateMessage)
	if value := gotAgain.Payload["items"].([]any)[0]; value != "original" {
		t.Fatalf("stored custom message value = %v, want original", value)
	}
	if value := string(gotAgain.hidden["secret"]); value != "original" {
		t.Fatalf("stored custom message hidden value = %q, want original", value)
	}

	// ErrorMessage is runtime-owned and cannot be installed through the public
	// M0 API, so this compile assertion protects its explicit absence shape.
	var _ *string = created.State().ErrorMessage
}

func TestAgentRejectsCustomMessagesWithoutAnOwnershipClone(t *testing.T) {
	t.Parallel()

	uncloneable := &customMessageWithoutClone{Role: "notice", Payload: map[string]string{"value": "original"}}
	if _, err := agent.NewAgent(agent.AgentOptions{InitialState: &agent.AgentInitialState{Messages: []agent.AgentMessage{uncloneable}}}); err == nil {
		t.Fatal("NewAgent accepted a custom message without CloneAgentMessage")
	}

	created, err := agent.NewAgent(agent.AgentOptions{InitialState: &agent.AgentInitialState{Messages: []agent.AgentMessage{userMessage("existing")}}})
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}
	for name, mutate := range map[string]func() error{
		"ReplaceMessages": func() error { return created.ReplaceMessages([]agent.AgentMessage{uncloneable}) },
		"AppendMessage":   func() error { return created.AppendMessage(uncloneable) },
		"Steer":           func() error { return created.Steer(uncloneable) },
		"FollowUp":        func() error { return created.FollowUp(uncloneable) },
	} {
		if err := mutate(); err == nil {
			t.Errorf("%s accepted a custom message without CloneAgentMessage", name)
		}
	}
	if got := messageText(t, created.State().Messages[0]); got != "existing" {
		t.Fatalf("failed mutations changed transcript to %q", got)
	}
	if created.HasQueuedMessages() {
		t.Fatal("failed mutations changed queues")
	}
}

func TestAgentSettersCopyMutableValuesAndRejectInvalidInputAtomically(t *testing.T) {
	t.Parallel()

	created, err := agent.NewAgent(agent.AgentOptions{})
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}
	created.SetSystemPrompt("updated")
	model := ai.Model{
		ID: "model", Name: "Model", API: ai.APIOpenAICompletions, Provider: ai.ProviderIDOpenAI,
		Input: []ai.ModelInput{ai.ModelInputText},
		ThinkingLevelMap: ai.ThinkingLevelMap{
			ai.ModelThinkingLevelHigh: ai.Some("high"),
		},
		SamplingParams: map[string]json.RawMessage{"temperature": json.RawMessage(`0.5`)},
		Headers:        map[string]string{"x-test": "original"},
	}
	created.SetModel(model)
	tools := []agent.ErasedAgentTool{erasedTestTool(t, "write", "Write", nil)}
	if err := created.SetTools(tools); err != nil {
		t.Fatalf("SetTools() error = %v", err)
	}
	messages := []agent.AgentMessage{userMessage("first")}
	if err := created.ReplaceMessages(messages); err != nil {
		t.Fatalf("ReplaceMessages() error = %v", err)
	}
	if err := created.AppendMessage(userMessage("second")); err != nil {
		t.Fatalf("AppendMessage() error = %v", err)
	}
	if err := created.SetThinkingLevel(ai.ModelThinkingLevelHigh); err != nil {
		t.Fatalf("SetThinkingLevel() error = %v", err)
	}

	model.Input[0] = ai.ModelInputImage
	model.ThinkingLevelMap[ai.ModelThinkingLevelHigh] = ai.Some("mutated")
	model.SamplingParams["temperature"][0] = '9'
	model.Headers["x-test"] = "mutated"
	tools[0].Parameters[0] = '['
	messages[0] = userMessage("mutated")

	before := created.State()
	if err := created.SetThinkingLevel(agent.ThinkingLevel("future")); err == nil {
		t.Fatal("SetThinkingLevel accepted an invalid mode")
	}
	if err := created.ReplaceMessages([]agent.AgentMessage{userMessage("replacement"), nil}); err == nil {
		t.Fatal("ReplaceMessages accepted a nil message")
	}
	var typedNil *ai.UserMessage
	if err := created.AppendMessage(typedNil); err == nil {
		t.Fatal("AppendMessage accepted a typed nil message")
	}
	after := created.State()
	beforeWithoutTools := before
	afterWithoutTools := after
	beforeWithoutTools.Tools = nil
	afterWithoutTools.Tools = nil
	if !reflect.DeepEqual(afterWithoutTools, beforeWithoutTools) {
		t.Fatalf("invalid mutations changed state: before %#v, after %#v", before, after)
	}
	if len(after.Tools) != len(before.Tools) || after.Tools[0].Name != before.Tools[0].Name || after.Tools[0].Label != before.Tools[0].Label || !reflect.DeepEqual(after.Tools[0].Parameters, before.Tools[0].Parameters) {
		t.Fatalf("invalid mutations changed Tools: before %#v, after %#v", before.Tools, after.Tools)
	}
	if after.SystemPrompt != "updated" || after.ThinkingLevel != ai.ModelThinkingLevelHigh {
		t.Fatalf("configured state = %#v", after)
	}
	if after.Model.Input[0] != ai.ModelInputText || after.Model.Headers["x-test"] != "original" || string(after.Model.SamplingParams["temperature"]) != "0.5" {
		t.Fatalf("stored Model was mutated through caller input: %#v", after.Model)
	}
	if got := string(after.Tools[0].Parameters); got != `{"type":"object"}` {
		t.Fatalf("stored Tool schema = %q", got)
	}
	if got := []string{messageText(t, after.Messages[0]), messageText(t, after.Messages[1])}; !reflect.DeepEqual(got, []string{"first", "second"}) {
		t.Fatalf("stored messages = %v", got)
	}
	after.Model.Headers["x-test"] = "snapshot-mutated"
	after.Model.SamplingParams["temperature"][0] = '8'
	after.Tools[0].Parameters[0] = '['
	after.Messages[0] = userMessage("snapshot-mutated")
	final := created.State()
	if final.Model.Headers["x-test"] != "original" || string(final.Model.SamplingParams["temperature"]) != "0.5" || string(final.Tools[0].Parameters) != `{"type":"object"}` || messageText(t, final.Messages[0]) != "first" {
		t.Fatalf("State exposed mutable storage: %#v", final)
	}
	forged := []agent.ErasedAgentTool{{Tool: ai.Tool{Name: "forged", Parameters: json.RawMessage(`{"type":"object"}`)}}}
	if err := created.SetTools(forged); err == nil {
		t.Fatal("SetTools accepted a Tool that did not come from EraseAgentTool")
	}
	if got := created.State().Tools[0].Name; got != "write" {
		t.Fatalf("invalid SetTools changed stored Tool to %q", got)
	}
}

func TestNewAgentRejectsInvalidConfiguration(t *testing.T) {
	t.Parallel()

	negative := int64(-1)
	tests := []agent.AgentOptions{
		{SteeringMode: agent.QueueMode("later")},
		{FollowUpMode: agent.QueueMode("later")},
		{ToolExecution: agent.ToolExecutionMode("sometimes")},
		{Transport: ai.Transport("carrier-pigeon")},
		{MaxRetryDelayMS: &negative},
		{InitialState: &agent.AgentInitialState{ThinkingLevel: agent.ThinkingLevel("future")}},
		{InitialState: &agent.AgentInitialState{Messages: []agent.AgentMessage{nil}}},
		{InitialState: &agent.AgentInitialState{Tools: []agent.ErasedAgentTool{{Tool: ai.Tool{Name: "forged", Parameters: json.RawMessage(`{"type":"object"}`)}}}}},
	}
	for index, options := range tests {
		if got, err := agent.NewAgent(options); err == nil || got != nil {
			t.Errorf("case %d NewAgent() = (%#v, %v), want nil and error", index, got, err)
		}
	}
}

func TestAgentQueuesModesAndResetAreLocalStateOperations(t *testing.T) {
	t.Parallel()

	created, err := agent.NewAgent(agent.AgentOptions{InitialState: &agent.AgentInitialState{
		SystemPrompt: "system",
		Model:        ai.Model{ID: "configured"},
		Tools:        []agent.ErasedAgentTool{erasedTestTool(t, "read", "", nil)},
		Messages:     []agent.AgentMessage{userMessage("existing")},
	}})
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}
	if err := created.Steer(userMessage("steer")); err != nil {
		t.Fatalf("Steer() error = %v", err)
	}
	if err := created.FollowUp(userMessage("follow")); err != nil {
		t.Fatalf("FollowUp() error = %v", err)
	}
	if !created.HasQueuedMessages() {
		t.Fatal("queued messages were not reported")
	}
	if err := created.SetSteeringMode(agent.QueueAll); err != nil {
		t.Fatalf("SetSteeringMode() error = %v", err)
	}
	if err := created.SetFollowUpMode(agent.QueueAll); err != nil {
		t.Fatalf("SetFollowUpMode() error = %v", err)
	}
	if err := created.SetSteeringMode(agent.QueueMode("invalid")); err == nil || created.SteeringMode() != agent.QueueAll {
		t.Fatalf("invalid steering mode changed mode to %q; error %v", created.SteeringMode(), err)
	}
	if err := created.SetFollowUpMode(agent.QueueMode("invalid")); err == nil || created.FollowUpMode() != agent.QueueAll {
		t.Fatalf("invalid follow-up mode changed mode to %q; error %v", created.FollowUpMode(), err)
	}
	created.ClearSteeringQueue()
	if !created.HasQueuedMessages() {
		t.Fatal("ClearSteeringQueue also cleared follow-up messages")
	}
	created.ClearFollowUpQueue()
	if created.HasQueuedMessages() {
		t.Fatal("ClearFollowUpQueue left a queued message")
	}
	if err := created.Steer(userMessage("steer-again")); err != nil {
		t.Fatalf("Steer() error = %v", err)
	}
	if err := created.FollowUp(userMessage("follow-again")); err != nil {
		t.Fatalf("FollowUp() error = %v", err)
	}
	created.ClearAllQueues()
	if created.HasQueuedMessages() {
		t.Fatal("ClearAllQueues left a queued message")
	}
	if err := created.Steer(nil); err == nil || created.HasQueuedMessages() {
		t.Fatalf("invalid Steer() = %v, queued = %t", err, created.HasQueuedMessages())
	}
	if err := created.FollowUp(nil); err == nil || created.HasQueuedMessages() {
		t.Fatalf("invalid FollowUp() = %v, queued = %t", err, created.HasQueuedMessages())
	}
	if err := created.Steer(userMessage("queued")); err != nil {
		t.Fatalf("Steer() error = %v", err)
	}
	if err := created.Reset(); err != nil {
		t.Fatalf("Reset() error = %v", err)
	}
	state := created.State()
	if len(state.Messages) != 0 || state.IsStreaming || state.StreamingMessage != nil || len(state.PendingToolCalls) != 0 || state.ErrorMessage != nil || created.HasQueuedMessages() {
		t.Fatalf("Reset left runtime state: state %#v, queued %t", state, created.HasQueuedMessages())
	}
	if state.SystemPrompt != "system" || state.Model.ID != "configured" || len(state.Tools) != 1 || created.SteeringMode() != agent.QueueAll || created.FollowUpMode() != agent.QueueAll {
		t.Fatalf("Reset changed configuration: state %#v, modes (%q, %q)", state, created.SteeringMode(), created.FollowUpMode())
	}
}

func TestAgentRuntimeValidationHasNoSideEffects(t *testing.T) {
	t.Parallel()

	var callbackCalls atomic.Int64
	markCalled := func() { callbackCalls.Add(1) }
	counterTool := erasedTestTool(t, "counter", "", func(context.Context, string, map[string]any, agent.AgentToolUpdateCallback[map[string]any]) (agent.AgentToolResult[map[string]any], error) {
		markCalled()
		return agent.AgentToolResult[map[string]any]{}, nil
	})
	created, err := agent.NewAgent(agent.AgentOptions{
		InitialState: &agent.AgentInitialState{
			Tools: []agent.ErasedAgentTool{counterTool},
		},
		ConvertToLLM: func(context.Context, []agent.AgentMessage) ([]ai.Message, error) { markCalled(); return nil, nil },
		TransformContext: func(context.Context, []agent.AgentMessage) ([]agent.AgentMessage, error) {
			markCalled()
			return nil, nil
		},
		StreamFunction: func(context.Context, ai.Model, ai.Context, ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
			markCalled()
			return nil
		},
		GetAPIKey: func(context.Context, ai.ProviderID) (string, bool, error) { markCalled(); return "", false, nil },
		OnPayload: func(context.Context, ai.JSONValue, ai.Model) (ai.PayloadHookResult, error) {
			markCalled()
			return ai.PayloadHookResult{}, nil
		},
		OnResponse: func(context.Context, ai.ProviderResponse, ai.Model) error { markCalled(); return nil },
		BeforeToolCall: func(context.Context, agent.BeforeToolCallContext) (*agent.BeforeToolCallResult, error) {
			markCalled()
			return nil, nil
		},
		AfterToolCall: func(context.Context, agent.AfterToolCallContext) (*agent.AfterToolCallResult, error) {
			markCalled()
			return nil, nil
		},
		ShouldStopAfterTurn: func(context.Context, agent.ShouldStopAfterTurnContext) (bool, error) {
			markCalled()
			return false, nil
		},
		PrepareNextTurn: func(context.Context) (*agent.AgentLoopTurnUpdate, error) { markCalled(); return nil, nil },
		PrepareNextTurnWithContext: func(context.Context, agent.PrepareNextTurnContext) (*agent.AgentLoopTurnUpdate, error) {
			markCalled()
			return nil, nil
		},
	})
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}
	if err := created.Steer(userMessage("queued")); err != nil {
		t.Fatalf("Steer() error = %v", err)
	}
	listenerCalls := atomic.Int64{}
	unsubscribe := created.Subscribe(func(context.Context, agent.AgentEvent) error {
		listenerCalls.Add(1)
		return nil
	})
	nilUnsubscribe := created.Subscribe(nil)
	nilUnsubscribe()
	nilUnsubscribe()

	before := created.State()
	for name, call := range map[string]func() error{
		"Prompt":   func() error { return created.Prompt(context.Background(), nil) },
		"Continue": func() error { return created.Continue(context.Background()) },
	} {
		err := call()
		if err == nil {
			t.Errorf("%s() error = nil", name)
		}
	}
	if got := callbackCalls.Load(); got != 0 {
		t.Fatalf("invalid runtime calls invoked callbacks %d times", got)
	}
	if got := listenerCalls.Load(); got != 0 {
		t.Fatalf("invalid runtime calls invoked listeners %d times", got)
	}
	after := created.State()
	if !reflect.DeepEqual(after.Model, before.Model) || !reflect.DeepEqual(after.Messages, before.Messages) || len(after.Tools) != len(before.Tools) || after.IsStreaming != before.IsStreaming || after.StreamingMessage != before.StreamingMessage || !reflect.DeepEqual(after.PendingToolCalls, before.PendingToolCalls) || !reflect.DeepEqual(after.ErrorMessage, before.ErrorMessage) || !created.HasQueuedMessages() {
		t.Fatalf("invalid runtime calls mutated state or drained queues: before %#v, after %#v, queued %t", before, created.State(), created.HasQueuedMessages())
	}
	if created.Busy() {
		t.Fatal("invalid runtime call left Agent busy")
	}
	if _, ok := created.ActiveContext(); ok {
		t.Fatal("invalid runtime call installed an active context")
	}
	created.Abort()
	if err := created.WaitForIdle(context.Background()); err != nil {
		t.Fatalf("WaitForIdle(idle) error = %v", err)
	}
	unsubscribe()
	unsubscribe()
}

func (m *customStateMessage) MessageRole() ai.MessageRole { return m.Role }

type customStateMessage struct {
	Role    ai.MessageRole `json:"role"`
	Payload map[string]any `json:"payload"`
	hidden  map[string][]byte
}

func (m *customStateMessage) CloneAgentMessage() agent.AgentMessage {
	if m == nil {
		return (*customStateMessage)(nil)
	}
	clone := &customStateMessage{Role: m.Role, Payload: make(map[string]any, len(m.Payload)), hidden: make(map[string][]byte, len(m.hidden))}
	for key, value := range m.Payload {
		raw, _ := json.Marshal(value)
		_ = json.Unmarshal(raw, &value)
		clone.Payload[key] = value
	}
	for key, value := range m.hidden {
		clone.hidden[key] = append([]byte(nil), value...)
	}
	return clone
}

type customMessageWithoutClone struct {
	Role    ai.MessageRole    `json:"role"`
	Payload map[string]string `json:"payload"`
}

func (m *customMessageWithoutClone) MessageRole() ai.MessageRole { return m.Role }

func userMessage(text string) ai.UserMessage {
	return ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText(text)}
}

func messageText(t *testing.T, message agent.AgentMessage) string {
	t.Helper()
	user, ok := message.(ai.UserMessage)
	if !ok {
		t.Fatalf("message type = %T, want ai.UserMessage", message)
	}
	text, ok := user.Content.Text()
	if !ok {
		t.Fatal("user message content is not text")
	}
	return text
}

func erasedTestTool(
	t *testing.T,
	name string,
	label string,
	execute func(context.Context, string, map[string]any, agent.AgentToolUpdateCallback[map[string]any]) (agent.AgentToolResult[map[string]any], error),
) agent.ErasedAgentTool {
	t.Helper()
	tool, err := agent.EraseAgentTool(agent.AgentTool[map[string]any, map[string]any]{
		Tool: ai.Tool{
			Name:        name,
			Description: name,
			Parameters:  json.RawMessage(`{"type":"object"}`),
		},
		Label: label,
		DecodeValidated: func(value ai.JSONValue) map[string]any {
			return value.(map[string]any)
		},
		Execute: execute,
	})
	if err != nil {
		t.Fatalf("EraseAgentTool(%q) error = %v", name, err)
	}
	return tool
}

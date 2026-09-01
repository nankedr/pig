package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/nankedr/pig/ai"
)

// ConvertToLLMFunc converts the open Agent transcript into model-visible
// messages. AgentOptions uses the same named function type as the low-level
// loop's inline callback.
type ConvertToLLMFunc func(context.Context, []AgentMessage) ([]ai.Message, error)

// TransformContextFunc optionally changes the open Agent transcript before it
// is converted to model-visible messages.
type TransformContextFunc func(context.Context, []AgentMessage) ([]AgentMessage, error)

// AgentInitialState contains the caller-owned configuration state copied by
// NewAgent. Runtime state is deliberately not accepted during construction.
type AgentInitialState struct {
	SystemPrompt  string
	Model         ai.Model
	ThinkingLevel ThinkingLevel
	Tools         []ErasedAgentTool
	Messages      []AgentMessage
}

// AgentState is a defensive snapshot of the Agent's configuration, transcript,
// and current runtime status. Every map and slice is copied before it crosses
// the Agent boundary.
type AgentState struct {
	SystemPrompt     string
	Model            ai.Model
	ThinkingLevel    ThinkingLevel
	Tools            []ErasedAgentTool
	Messages         []AgentMessage
	IsStreaming      bool
	StreamingMessage AgentMessage
	PendingToolCalls map[string]struct{}
	ErrorMessage     *string
}

// AgentOptions configures a stateful legacy Agent. Runtime callbacks are kept
// for the M1 loop implementation; the M0 Prompt stubs never invoke them.
type AgentOptions struct {
	InitialState               *AgentInitialState
	ConvertToLLM               ConvertToLLMFunc
	TransformContext           TransformContextFunc
	StreamFunction             StreamFunction
	GetAPIKey                  func(context.Context, ai.ProviderID) (string, bool, error)
	OnPayload                  ai.PayloadHook
	OnResponse                 ai.ResponseHook
	BeforeToolCall             func(context.Context, BeforeToolCallContext) (*BeforeToolCallResult, error)
	AfterToolCall              func(context.Context, AfterToolCallContext) (*AfterToolCallResult, error)
	ShouldStopAfterTurn        func(context.Context, ShouldStopAfterTurnContext) (bool, error)
	PrepareNextTurn            func(context.Context) (*AgentLoopTurnUpdate, error)
	PrepareNextTurnWithContext func(context.Context, PrepareNextTurnContext) (*AgentLoopTurnUpdate, error)
	SteeringMode               QueueMode
	FollowUpMode               QueueMode
	SessionID                  string
	ThinkingBudgets            *ai.ThinkingBudgets
	Transport                  ai.Transport
	MaxRetryDelayMS            *int64
	ToolExecution              ToolExecutionMode
}

// Unsubscribe removes a registered Agent listener. Repeated calls are safe.
type Unsubscribe func()

// Agent owns one transcript and the two pending-message queues used by the
// legacy loop. Network and Tool execution are deferred Capability Stubs.
type Agent struct {
	mu sync.RWMutex

	state         AgentState
	steeringQueue []AgentMessage
	followUpQueue []AgentMessage
	steeringMode  QueueMode
	followUpMode  QueueMode
	listeners     map[uint64]AgentEventListener
	listenerOrder []uint64
	nextListener  uint64
	activeContext context.Context
	activeCancel  context.CancelCauseFunc
	idle          chan struct{}

	convertToLLM               ConvertToLLMFunc
	transformContext           TransformContextFunc
	streamFunction             StreamFunction
	getAPIKey                  func(context.Context, ai.ProviderID) (string, bool, error)
	onPayload                  ai.PayloadHook
	onResponse                 ai.ResponseHook
	beforeToolCall             func(context.Context, BeforeToolCallContext) (*BeforeToolCallResult, error)
	afterToolCall              func(context.Context, AfterToolCallContext) (*AfterToolCallResult, error)
	shouldStopAfterTurn        func(context.Context, ShouldStopAfterTurnContext) (bool, error)
	prepareNextTurn            func(context.Context) (*AgentLoopTurnUpdate, error)
	prepareNextTurnWithContext func(context.Context, PrepareNextTurnContext) (*AgentLoopTurnUpdate, error)
	sessionID                  string
	thinkingBudgets            *ai.ThinkingBudgets
	transport                  ai.Transport
	maxRetryDelayMS            *int64
	toolExecution              ToolExecutionMode
}

// NewAgent validates configuration and copies all caller-owned mutable state.
func NewAgent(options AgentOptions) (*Agent, error) {
	steeringMode, err := queueModeOrDefault(options.SteeringMode)
	if err != nil {
		return nil, fmt.Errorf("steering mode: %w", err)
	}
	followUpMode, err := queueModeOrDefault(options.FollowUpMode)
	if err != nil {
		return nil, fmt.Errorf("follow-up mode: %w", err)
	}
	if err := validateToolExecutionMode(options.ToolExecution); err != nil {
		return nil, err
	}
	if err := validateTransport(options.Transport); err != nil {
		return nil, err
	}
	if options.MaxRetryDelayMS != nil && *options.MaxRetryDelayMS < 0 {
		return nil, fmt.Errorf("max retry delay must not be negative")
	}
	initial := AgentInitialState{}
	if options.InitialState != nil {
		initial = *options.InitialState
	}
	if err := validateThinkingLevelOrDefault(initial.ThinkingLevel); err != nil {
		return nil, err
	}
	if err := validateErasedAgentTools(initial.Tools); err != nil {
		return nil, err
	}
	messages, err := cloneAgentMessagesForOwnership(initial.Messages)
	if err != nil {
		return nil, err
	}

	model := initial.Model
	if reflect.ValueOf(model).IsZero() {
		model = defaultAgentModel()
	}
	thinkingLevel := initial.ThinkingLevel
	if thinkingLevel == "" {
		thinkingLevel = ai.ModelThinkingLevelOff
	}
	transport := options.Transport
	if transport == "" {
		transport = ai.TransportAuto
	}
	toolExecution := options.ToolExecution
	if toolExecution == "" {
		toolExecution = ToolExecutionParallel
	}
	idle := make(chan struct{})
	close(idle)
	tools := cloneAgentTools(initial.Tools)
	if tools == nil {
		tools = []ErasedAgentTool{}
	}
	if messages == nil {
		messages = []AgentMessage{}
	}

	a := &Agent{
		state: AgentState{
			SystemPrompt:     initial.SystemPrompt,
			Model:            cloneAgentModel(model),
			ThinkingLevel:    thinkingLevel,
			Tools:            tools,
			Messages:         messages,
			PendingToolCalls: make(map[string]struct{}),
		},
		steeringMode:               steeringMode,
		followUpMode:               followUpMode,
		listeners:                  make(map[uint64]AgentEventListener),
		idle:                       idle,
		convertToLLM:               options.ConvertToLLM,
		transformContext:           options.TransformContext,
		streamFunction:             options.StreamFunction,
		getAPIKey:                  options.GetAPIKey,
		onPayload:                  options.OnPayload,
		onResponse:                 options.OnResponse,
		beforeToolCall:             options.BeforeToolCall,
		afterToolCall:              options.AfterToolCall,
		shouldStopAfterTurn:        options.ShouldStopAfterTurn,
		prepareNextTurn:            options.PrepareNextTurn,
		prepareNextTurnWithContext: options.PrepareNextTurnWithContext,
		sessionID:                  options.SessionID,
		thinkingBudgets:            cloneThinkingBudgets(options.ThinkingBudgets),
		transport:                  transport,
		maxRetryDelayMS:            cloneInt64(options.MaxRetryDelayMS),
		toolExecution:              toolExecution,
	}
	if a.convertToLLM == nil {
		a.convertToLLM = DefaultConvertToLLM
	}
	return a, nil
}

func defaultAgentModel() ai.Model {
	return ai.Model{
		ID:       "unknown",
		Name:     "unknown",
		API:      ai.API("unknown"),
		Provider: ai.ProviderID("unknown"),
		Input:    []ai.ModelInput{},
	}
}

// State returns a defensive snapshot.
func (a *Agent) State() AgentState {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return cloneAgentState(a.state)
}

// Busy reports whether a Prompt or continuation is active.
func (a *Agent) Busy() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.activeContext != nil
}

// ConvertToLLM returns the transcript conversion callback used by the next
// runtime invocation. It is always non-nil.
func (a *Agent) ConvertToLLM() ConvertToLLMFunc {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.convertToLLM
}

// SetConvertToLLM replaces the transcript conversion callback. Passing nil
// restores DefaultConvertToLLM.
func (a *Agent) SetConvertToLLM(convert ConvertToLLMFunc) {
	if convert == nil {
		convert = DefaultConvertToLLM
	}
	a.mu.Lock()
	a.convertToLLM = convert
	a.mu.Unlock()
}

func (a *Agent) TransformContext() TransformContextFunc {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.transformContext
}

func (a *Agent) SetTransformContext(transform TransformContextFunc) {
	a.mu.Lock()
	a.transformContext = transform
	a.mu.Unlock()
}

func (a *Agent) StreamFunction() StreamFunction {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.streamFunction
}

func (a *Agent) SetStreamFunction(stream StreamFunction) {
	a.mu.Lock()
	a.streamFunction = stream
	a.mu.Unlock()
}

func (a *Agent) GetAPIKey() func(context.Context, ai.ProviderID) (string, bool, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.getAPIKey
}

func (a *Agent) SetGetAPIKey(getAPIKey func(context.Context, ai.ProviderID) (string, bool, error)) {
	a.mu.Lock()
	a.getAPIKey = getAPIKey
	a.mu.Unlock()
}

func (a *Agent) OnPayload() ai.PayloadHook {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.onPayload
}

func (a *Agent) SetOnPayload(onPayload ai.PayloadHook) {
	a.mu.Lock()
	a.onPayload = onPayload
	a.mu.Unlock()
}

func (a *Agent) OnResponse() ai.ResponseHook {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.onResponse
}

func (a *Agent) SetOnResponse(onResponse ai.ResponseHook) {
	a.mu.Lock()
	a.onResponse = onResponse
	a.mu.Unlock()
}

func (a *Agent) BeforeToolCall() func(context.Context, BeforeToolCallContext) (*BeforeToolCallResult, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.beforeToolCall
}

func (a *Agent) SetBeforeToolCall(beforeToolCall func(context.Context, BeforeToolCallContext) (*BeforeToolCallResult, error)) {
	a.mu.Lock()
	a.beforeToolCall = beforeToolCall
	a.mu.Unlock()
}

func (a *Agent) AfterToolCall() func(context.Context, AfterToolCallContext) (*AfterToolCallResult, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.afterToolCall
}

func (a *Agent) SetAfterToolCall(afterToolCall func(context.Context, AfterToolCallContext) (*AfterToolCallResult, error)) {
	a.mu.Lock()
	a.afterToolCall = afterToolCall
	a.mu.Unlock()
}

func (a *Agent) ShouldStopAfterTurn() func(context.Context, ShouldStopAfterTurnContext) (bool, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.shouldStopAfterTurn
}

func (a *Agent) SetShouldStopAfterTurn(shouldStopAfterTurn func(context.Context, ShouldStopAfterTurnContext) (bool, error)) {
	a.mu.Lock()
	a.shouldStopAfterTurn = shouldStopAfterTurn
	a.mu.Unlock()
}

func (a *Agent) PrepareNextTurn() func(context.Context) (*AgentLoopTurnUpdate, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.prepareNextTurn
}

func (a *Agent) SetPrepareNextTurn(prepareNextTurn func(context.Context) (*AgentLoopTurnUpdate, error)) {
	a.mu.Lock()
	a.prepareNextTurn = prepareNextTurn
	a.mu.Unlock()
}

func (a *Agent) PrepareNextTurnWithContext() func(context.Context, PrepareNextTurnContext) (*AgentLoopTurnUpdate, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.prepareNextTurnWithContext
}

func (a *Agent) SetPrepareNextTurnWithContext(prepareNextTurn func(context.Context, PrepareNextTurnContext) (*AgentLoopTurnUpdate, error)) {
	a.mu.Lock()
	a.prepareNextTurnWithContext = prepareNextTurn
	a.mu.Unlock()
}

func (a *Agent) SessionID() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.sessionID
}

func (a *Agent) SetSessionID(sessionID string) {
	a.mu.Lock()
	a.sessionID = sessionID
	a.mu.Unlock()
}

func (a *Agent) ThinkingBudgets() *ai.ThinkingBudgets {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return cloneThinkingBudgets(a.thinkingBudgets)
}

func (a *Agent) SetThinkingBudgets(thinkingBudgets *ai.ThinkingBudgets) {
	a.mu.Lock()
	a.thinkingBudgets = cloneThinkingBudgets(thinkingBudgets)
	a.mu.Unlock()
}

func (a *Agent) Transport() ai.Transport {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.transport
}

func (a *Agent) SetTransport(transport ai.Transport) error {
	if err := validateTransport(transport); err != nil {
		return err
	}
	if transport == "" {
		transport = ai.TransportAuto
	}
	a.mu.Lock()
	a.transport = transport
	a.mu.Unlock()
	return nil
}

func (a *Agent) MaxRetryDelayMS() *int64 {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return cloneInt64(a.maxRetryDelayMS)
}

func (a *Agent) SetMaxRetryDelayMS(maxRetryDelayMS *int64) error {
	if maxRetryDelayMS != nil && *maxRetryDelayMS < 0 {
		return fmt.Errorf("max retry delay must not be negative")
	}
	a.mu.Lock()
	a.maxRetryDelayMS = cloneInt64(maxRetryDelayMS)
	a.mu.Unlock()
	return nil
}

func (a *Agent) ToolExecution() ToolExecutionMode {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.toolExecution
}

func (a *Agent) SetToolExecution(toolExecution ToolExecutionMode) error {
	if err := validateToolExecutionMode(toolExecution); err != nil {
		return err
	}
	if toolExecution == "" {
		toolExecution = ToolExecutionParallel
	}
	a.mu.Lock()
	a.toolExecution = toolExecution
	a.mu.Unlock()
	return nil
}

func (a *Agent) SetSystemPrompt(systemPrompt string) {
	a.mu.Lock()
	a.state.SystemPrompt = systemPrompt
	a.mu.Unlock()
}

func (a *Agent) SetModel(model ai.Model) {
	a.mu.Lock()
	a.state.Model = cloneAgentModel(model)
	a.mu.Unlock()
}

func (a *Agent) SetThinkingLevel(level ThinkingLevel) error {
	if err := validateThinkingLevel(level); err != nil {
		return err
	}
	a.mu.Lock()
	a.state.ThinkingLevel = level
	a.mu.Unlock()
	return nil
}

func (a *Agent) SetTools(tools []ErasedAgentTool) error {
	if err := validateErasedAgentTools(tools); err != nil {
		return err
	}
	a.mu.Lock()
	a.state.Tools = cloneAgentTools(tools)
	a.mu.Unlock()
	return nil
}

func (a *Agent) ReplaceMessages(messages []AgentMessage) error {
	cloned, err := cloneAgentMessagesForOwnership(messages)
	if err != nil {
		return err
	}
	a.mu.Lock()
	a.state.Messages = cloned
	a.mu.Unlock()
	return nil
}

func (a *Agent) AppendMessage(message AgentMessage) error {
	cloned, err := cloneAgentMessageForOwnership(message)
	if err != nil {
		return err
	}
	a.mu.Lock()
	a.state.Messages = append(a.state.Messages, cloned)
	a.mu.Unlock()
	return nil
}

// Subscribe registers a listener for future runtime events. The returned
// unsubscribe function is idempotent. Local state operations emit no events.
func (a *Agent) Subscribe(listener AgentEventListener) Unsubscribe {
	if listener == nil {
		return func() {}
	}
	a.mu.Lock()
	id := a.nextListener
	a.nextListener++
	a.listeners[id] = listener
	a.listenerOrder = append(a.listenerOrder, id)
	a.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			a.mu.Lock()
			delete(a.listeners, id)
			a.mu.Unlock()
		})
	}
}

func (a *Agent) Steer(message AgentMessage) error {
	cloned, err := cloneAgentMessageForOwnership(message)
	if err != nil {
		return err
	}
	a.mu.Lock()
	a.steeringQueue = append(a.steeringQueue, cloned)
	a.mu.Unlock()
	return nil
}

func (a *Agent) FollowUp(message AgentMessage) error {
	cloned, err := cloneAgentMessageForOwnership(message)
	if err != nil {
		return err
	}
	a.mu.Lock()
	a.followUpQueue = append(a.followUpQueue, cloned)
	a.mu.Unlock()
	return nil
}

func (a *Agent) ClearSteeringQueue() {
	a.mu.Lock()
	a.steeringQueue = nil
	a.mu.Unlock()
}

func (a *Agent) ClearFollowUpQueue() {
	a.mu.Lock()
	a.followUpQueue = nil
	a.mu.Unlock()
}

func (a *Agent) ClearAllQueues() {
	a.mu.Lock()
	a.steeringQueue = nil
	a.followUpQueue = nil
	a.mu.Unlock()
}

func (a *Agent) HasQueuedMessages() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return len(a.steeringQueue) != 0 || len(a.followUpQueue) != 0
}

func (a *Agent) SteeringMode() QueueMode {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.steeringMode
}

func (a *Agent) SetSteeringMode(mode QueueMode) error {
	if err := validateQueueMode(mode); err != nil {
		return err
	}
	a.mu.Lock()
	a.steeringMode = mode
	a.mu.Unlock()
	return nil
}

func (a *Agent) FollowUpMode() QueueMode {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.followUpMode
}

func (a *Agent) SetFollowUpMode(mode QueueMode) error {
	if err := validateQueueMode(mode); err != nil {
		return err
	}
	a.mu.Lock()
	a.followUpMode = mode
	a.mu.Unlock()
	return nil
}

// ActiveContext reports the context of a currently executing runtime call.
func (a *Agent) ActiveContext() (context.Context, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.activeContext, a.activeContext != nil
}

// Abort cancels an active run. It is a no-op while idle.
func (a *Agent) Abort() {
	a.mu.RLock()
	cancel := a.activeCancel
	a.mu.RUnlock()
	if cancel != nil {
		cancel(context.Canceled)
	}
}

// WaitForIdle waits for the current run, if any, without starting work.
func (a *Agent) WaitForIdle(ctx context.Context) error {
	a.mu.RLock()
	idle := a.idle
	a.mu.RUnlock()
	select {
	case <-idle:
		return nil
	default:
	}
	if ctx == nil {
		return fmt.Errorf("wait for idle context must not be nil")
	}
	select {
	case <-idle:
		return nil
	case <-ctx.Done():
		return context.Cause(ctx)
	}
}

// Reset clears transcript/runtime state and pending queues while preserving
// model, prompt, tools, options, modes, and subscriptions.
func (a *Agent) Reset() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.activeContext != nil {
		return fmt.Errorf("cannot reset a busy Agent")
	}
	a.state.Messages = []AgentMessage{}
	a.state.IsStreaming = false
	a.state.StreamingMessage = nil
	a.state.PendingToolCalls = make(map[string]struct{})
	a.state.ErrorMessage = nil
	a.steeringQueue = nil
	a.followUpQueue = nil
	return nil
}

// Prompt starts one Tool-free legacy Agent run with the supplied messages.
func (a *Agent) Prompt(ctx context.Context, messages ...AgentMessage) error {
	owned, err := cloneAgentMessagesForOwnership(messages)
	if err != nil {
		return err
	}
	return a.run(ctx, func(runContext context.Context, agentContext AgentContext, config AgentLoopConfig, streamFunction StreamFunction) error {
		_, err := RunAgentLoop(runContext, owned, agentContext, config, a.processEvent, streamFunction)
		return err
	})
}

// PromptText starts one Tool-free run from text and optional images.
func (a *Agent) PromptText(ctx context.Context, text string, images ...ai.ImageContent) error {
	content := make([]ai.UserContent, 0, len(images)+1)
	content = append(content, ai.TextContent{Type: ai.ContentTypeText, Text: text})
	for _, image := range images {
		content = append(content, image)
	}
	return a.Prompt(ctx, ai.UserMessage{
		Role: ai.MessageRoleUser, Content: ai.UserBlocks(content...), Timestamp: time.Now().UnixMilli(),
	})
}

// Continue runs from an existing transcript whose last message is user or
// ToolResult. Queue consumption remains deferred to M2.
func (a *Agent) Continue(ctx context.Context) error {
	a.mu.RLock()
	messages := cloneAgentMessages(a.state.Messages)
	busy := a.activeContext != nil
	a.mu.RUnlock()
	if busy {
		return fmt.Errorf("Agent is already processing; wait for completion before continuing")
	}
	if len(messages) == 0 {
		return fmt.Errorf("no messages to continue from")
	}
	if messages[len(messages)-1].MessageRole() == ai.MessageRoleAssistant {
		return fmt.Errorf("cannot continue from message role: assistant")
	}
	return a.run(ctx, func(runContext context.Context, agentContext AgentContext, config AgentLoopConfig, streamFunction StreamFunction) error {
		_, err := RunAgentLoopContinue(runContext, agentContext, config, a.processEvent, streamFunction)
		return err
	})
}

func (a *Agent) run(ctx context.Context, execute func(context.Context, AgentContext, AgentLoopConfig, StreamFunction) error) error {
	if ctx == nil {
		return fmt.Errorf("Agent run context must not be nil")
	}
	runContext, agentContext, config, streamFunction, err := a.startRun(ctx)
	if err != nil {
		return err
	}
	defer a.finishRun(runContext)
	return execute(runContext, agentContext, config, streamFunction)
}

func (a *Agent) startRun(ctx context.Context) (context.Context, AgentContext, AgentLoopConfig, StreamFunction, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.activeContext != nil {
		return nil, AgentContext{}, AgentLoopConfig{}, nil, fmt.Errorf("Agent is already processing")
	}
	runContext, cancel := context.WithCancelCause(ctx)
	a.activeContext = runContext
	a.activeCancel = cancel
	a.idle = make(chan struct{})
	a.state.IsStreaming = true
	a.state.StreamingMessage = nil
	a.state.ErrorMessage = nil

	streamOptions := ai.SimpleStreamOptions{StreamOptions: ai.StreamOptions{
		ProviderRequestOptions: ai.ProviderRequestOptions{
			OnPayload: a.onPayload, OnResponse: a.onResponse, MaxRetryDelayMS: cloneInt64(a.maxRetryDelayMS),
		},
	}}
	if a.state.ThinkingLevel != ai.ModelThinkingLevelOff {
		reasoning := ai.ThinkingLevel(a.state.ThinkingLevel)
		streamOptions.Reasoning = &reasoning
	}
	if a.sessionID != "" {
		sessionID := a.sessionID
		streamOptions.SessionID = &sessionID
	}
	transport := a.transport
	streamOptions.Transport = &transport
	streamOptions.ThinkingBudgets = cloneThinkingBudgets(a.thinkingBudgets)
	config := AgentLoopConfig{
		SimpleStreamOptions: streamOptions,
		Model:               cloneAgentModel(a.state.Model),
		ConvertToLLM:        a.convertToLLM,
		TransformContext:    a.transformContext,
		GetAPIKey:           a.getAPIKey,
		ShouldStopAfterTurn: a.shouldStopAfterTurn,
		BeforeToolCall:      a.beforeToolCall,
		AfterToolCall:       a.afterToolCall,
		ToolExecution:       a.toolExecution,
		GetSteeringMessages: nil,
		GetFollowUpMessages: nil,
	}
	if a.prepareNextTurnWithContext != nil {
		config.PrepareNextTurn = a.prepareNextTurnWithContext
	} else if a.prepareNextTurn != nil {
		prepareNextTurn := a.prepareNextTurn
		config.PrepareNextTurn = func(ctx context.Context, _ PrepareNextTurnContext) (*AgentLoopTurnUpdate, error) {
			return prepareNextTurn(ctx)
		}
	}
	agentContext := AgentContext{
		SystemPrompt: a.state.SystemPrompt,
		Messages:     cloneAgentMessages(a.state.Messages),
		Tools:        cloneAgentTools(a.state.Tools),
	}
	return runContext, agentContext, config, a.streamFunction, nil
}

func (a *Agent) finishRun(runContext context.Context) {
	a.mu.Lock()
	if a.activeContext != runContext {
		a.mu.Unlock()
		return
	}
	cancel := a.activeCancel
	a.state.IsStreaming = false
	a.state.StreamingMessage = nil
	a.state.PendingToolCalls = make(map[string]struct{})
	a.activeContext = nil
	a.activeCancel = nil
	idle := a.idle
	close(idle)
	a.mu.Unlock()
	if cancel != nil {
		cancel(nil)
	}
}

func (a *Agent) processEvent(ctx context.Context, event AgentEvent) error {
	a.mu.Lock()
	switch event := event.(type) {
	case MessageStartEvent:
		a.state.StreamingMessage = cloneAgentMessage(event.Message)
	case MessageUpdateEvent:
		a.state.StreamingMessage = cloneAgentMessage(event.Message)
	case MessageEndEvent:
		a.state.StreamingMessage = nil
		a.state.Messages = append(a.state.Messages, cloneAgentMessage(event.Message))
	case TurnEndEvent:
		if assistant, ok := agentAssistantMessage(event.Message); ok {
			if errorMessage, ok := assistant.ErrorMessage.Value(); ok {
				a.state.ErrorMessage = &errorMessage
			}
		}
	case AgentEndEvent:
		a.state.StreamingMessage = nil
	}
	listeners := make([]AgentEventListener, 0, len(a.listeners))
	for _, id := range a.listenerOrder {
		if listener, ok := a.listeners[id]; ok {
			listeners = append(listeners, listener)
		}
	}
	a.mu.Unlock()
	for _, listener := range listeners {
		if err := listener(ctx, cloneAgentEvent(event)); err != nil {
			return err
		}
	}
	return nil
}

func agentAssistantMessage(message AgentMessage) (ai.AssistantMessage, bool) {
	switch message := message.(type) {
	case ai.AssistantMessage:
		return message, true
	case *ai.AssistantMessage:
		if message != nil {
			return *message, true
		}
	}
	return ai.AssistantMessage{}, false
}

func queueModeOrDefault(mode QueueMode) (QueueMode, error) {
	if mode == "" {
		return QueueOneAtATime, nil
	}
	if err := validateQueueMode(mode); err != nil {
		return "", err
	}
	return mode, nil
}

func validateQueueMode(mode QueueMode) error {
	switch mode {
	case QueueAll, QueueOneAtATime:
		return nil
	default:
		return fmt.Errorf("invalid queue mode %q", mode)
	}
}

func validateToolExecutionMode(mode ToolExecutionMode) error {
	switch mode {
	case "", ToolExecutionParallel, ToolExecutionSequential:
		return nil
	default:
		return fmt.Errorf("invalid Tool execution mode %q", mode)
	}
}

func validateTransport(transport ai.Transport) error {
	switch transport {
	case "", ai.TransportAuto, ai.TransportSSE, ai.TransportWebSocket, ai.TransportWebSocketCached:
		return nil
	default:
		return fmt.Errorf("invalid transport %q", transport)
	}
}

func validateThinkingLevelOrDefault(level ThinkingLevel) error {
	if level == "" {
		return nil
	}
	return validateThinkingLevel(level)
}

func validateThinkingLevel(level ThinkingLevel) error {
	switch level {
	case ai.ModelThinkingLevelOff, ai.ModelThinkingLevelMinimal, ai.ModelThinkingLevelLow, ai.ModelThinkingLevelMedium, ai.ModelThinkingLevelHigh, ai.ModelThinkingLevelXHigh, ai.ModelThinkingLevelMax:
		return nil
	default:
		return fmt.Errorf("invalid thinking level %q", level)
	}
}

func validateAgentMessages(messages []AgentMessage) error {
	for index, message := range messages {
		if err := validateAgentMessage(message); err != nil {
			return fmt.Errorf("Agent message %d: %w", index, err)
		}
	}
	return nil
}

func validateErasedAgentTools(tools []ErasedAgentTool) error {
	for index, tool := range tools {
		if err := validateErasedAgentTool(tool); err != nil {
			return fmt.Errorf("Agent Tool %d: %w", index, err)
		}
	}
	return nil
}

func validateAgentMessage(message AgentMessage) error {
	if message == nil {
		return fmt.Errorf("Agent message must not be nil")
	}
	value := reflect.ValueOf(message)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		if value.IsNil() {
			return fmt.Errorf("Agent message must not be nil")
		}
	}
	if message.MessageRole() == "" {
		return fmt.Errorf("Agent message role must not be empty")
	}
	if _, err := MarshalAgentMessage(message); err != nil {
		return fmt.Errorf("invalid Agent message: %w", err)
	}
	return nil
}

func cloneAgentState(state AgentState) AgentState {
	state.Model = cloneAgentModel(state.Model)
	state.Tools = cloneAgentTools(state.Tools)
	state.Messages = cloneAgentMessages(state.Messages)
	state.StreamingMessage = cloneAgentMessage(state.StreamingMessage)
	state.PendingToolCalls = clonePendingToolCalls(state.PendingToolCalls)
	if state.ErrorMessage != nil {
		errorMessage := *state.ErrorMessage
		state.ErrorMessage = &errorMessage
	}
	return state
}

func cloneAgentMessages(messages []AgentMessage) []AgentMessage {
	if messages == nil {
		return nil
	}
	cloned := make([]AgentMessage, len(messages))
	for index, message := range messages {
		cloned[index] = cloneAgentMessage(message)
	}
	return cloned
}

func cloneAgentMessagesForOwnership(messages []AgentMessage) ([]AgentMessage, error) {
	if messages == nil {
		return nil, nil
	}
	cloned := make([]AgentMessage, len(messages))
	for index, message := range messages {
		owned, err := cloneAgentMessageForOwnership(message)
		if err != nil {
			return nil, fmt.Errorf("Agent message %d: %w", index, err)
		}
		cloned[index] = owned
	}
	return cloned, nil
}

func cloneAgentMessageForOwnership(message AgentMessage) (AgentMessage, error) {
	if err := validateAgentMessage(message); err != nil {
		return nil, err
	}
	switch message.(type) {
	case ai.UserMessage, *ai.UserMessage, ai.AssistantMessage, *ai.AssistantMessage, ai.ToolResultMessage, *ai.ToolResultMessage, RawAgentMessage, *RawAgentMessage:
		return cloneAgentMessage(message), nil
	}
	cloner, ok := message.(agentMessageCloner)
	if !ok {
		return nil, fmt.Errorf("custom Agent message %T must implement CloneAgentMessage before Agent takes ownership", message)
	}
	cloned := cloner.CloneAgentMessage()
	if err := validateAgentMessage(cloned); err != nil {
		return nil, fmt.Errorf("custom Agent message %T returned an invalid clone: %w", message, err)
	}
	if reflect.TypeOf(cloned) != reflect.TypeOf(message) {
		return nil, fmt.Errorf("custom Agent message %T clone changed concrete type to %T", message, cloned)
	}
	if cloned.MessageRole() != message.MessageRole() {
		return nil, fmt.Errorf("custom Agent message %T clone changed role from %q to %q", message, message.MessageRole(), cloned.MessageRole())
	}
	return cloned, nil
}

func cloneAgentMessage(message AgentMessage) AgentMessage {
	switch message := message.(type) {
	case nil:
		return nil
	case ai.UserMessage:
		return cloneUserMessage(message)
	case *ai.UserMessage:
		if message == nil {
			return nil
		}
		clone := cloneUserMessage(*message)
		return &clone
	case ai.AssistantMessage:
		return ai.CloneAssistantMessage(message)
	case *ai.AssistantMessage:
		if message == nil {
			return nil
		}
		clone := ai.CloneAssistantMessage(*message)
		return &clone
	case ai.ToolResultMessage:
		return cloneToolResultMessage(message)
	case *ai.ToolResultMessage:
		if message == nil {
			return nil
		}
		clone := cloneToolResultMessage(*message)
		return &clone
	case RawAgentMessage:
		clone, _ := NewRawAgentMessage(message.RawJSON())
		return clone
	case *RawAgentMessage:
		if message == nil {
			return nil
		}
		clone, _ := NewRawAgentMessage(message.RawJSON())
		return &clone
	default:
		return message.(agentMessageCloner).CloneAgentMessage()
	}
}

func cloneUserMessage(message ai.UserMessage) ai.UserMessage {
	encoded, err := ai.MarshalMessage(message)
	if err != nil {
		return message
	}
	cloned, err := ai.UnmarshalMessage(encoded)
	if err != nil {
		return message
	}
	return cloned.(ai.UserMessage)
}

func cloneToolResultMessage(message ai.ToolResultMessage) ai.ToolResultMessage {
	encoded, err := ai.MarshalMessage(message)
	if err != nil {
		return message
	}
	cloned, err := ai.UnmarshalMessage(encoded)
	if err != nil {
		return message
	}
	return cloned.(ai.ToolResultMessage)
}

func cloneAgentTools(tools []ErasedAgentTool) []ErasedAgentTool {
	if tools == nil {
		return nil
	}
	cloned := make([]ErasedAgentTool, len(tools))
	for index, tool := range tools {
		cloned[index] = tool
		cloned[index].Tool = cloneAgentToolMetadata(tool.Tool)
	}
	return cloned
}

func cloneAgentModel(model ai.Model) ai.Model {
	if model.Input != nil {
		model.Input = append([]ai.ModelInput{}, model.Input...)
	}
	if model.Cost.Tiers != nil {
		model.Cost.Tiers = append([]ai.ModelCostTier{}, model.Cost.Tiers...)
	}
	if model.ThinkingLevelMap != nil {
		cloned := make(ai.ThinkingLevelMap, len(model.ThinkingLevelMap))
		for level, value := range model.ThinkingLevelMap {
			cloned[level] = value
		}
		model.ThinkingLevelMap = cloned
	}
	model.SamplingParams = cloneRawMessages(model.SamplingParams)
	if model.Headers != nil {
		cloned := make(map[string]string, len(model.Headers))
		for key, value := range model.Headers {
			cloned[key] = value
		}
		model.Headers = cloned
	}
	if model.Compat.IsSet() && !model.Compat.IsNull() {
		raw, _ := model.Compat.Value()
		model.Compat = ai.Some(append(json.RawMessage(nil), raw...))
	}
	return model
}

func cloneRawMessages(values map[string]json.RawMessage) map[string]json.RawMessage {
	if values == nil {
		return nil
	}
	cloned := make(map[string]json.RawMessage, len(values))
	for key, value := range values {
		cloned[key] = append(json.RawMessage(nil), value...)
	}
	return cloned
}

func clonePendingToolCalls(values map[string]struct{}) map[string]struct{} {
	cloned := make(map[string]struct{}, len(values))
	for value := range values {
		cloned[value] = struct{}{}
	}
	return cloned
}

func cloneThinkingBudgets(value *ai.ThinkingBudgets) *ai.ThinkingBudgets {
	if value == nil {
		return nil
	}
	clone := *value
	clone.Minimal = cloneInt64(value.Minimal)
	clone.Low = cloneInt64(value.Low)
	clone.Medium = cloneInt64(value.Medium)
	clone.High = cloneInt64(value.High)
	return &clone
}

func cloneInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

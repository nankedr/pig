package codingagent

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
)

// AgentSessionEventType is the wire discriminator for the production
// AgentSession event stream. It is separate from Harness-v4 events.
type AgentSessionEventType string

const (
	AgentSessionEventTypeAgentStart                     AgentSessionEventType = "agent_start"
	AgentSessionEventTypeAgentEnd                       AgentSessionEventType = "agent_end"
	AgentSessionEventTypeTurnStart                      AgentSessionEventType = "turn_start"
	AgentSessionEventTypeTurnEnd                        AgentSessionEventType = "turn_end"
	AgentSessionEventTypeMessageStart                   AgentSessionEventType = "message_start"
	AgentSessionEventTypeMessageUpdate                  AgentSessionEventType = "message_update"
	AgentSessionEventTypeMessageEnd                     AgentSessionEventType = "message_end"
	AgentSessionEventTypeToolExecutionStart             AgentSessionEventType = "tool_execution_start"
	AgentSessionEventTypeToolExecutionUpdate            AgentSessionEventType = "tool_execution_update"
	AgentSessionEventTypeToolExecutionEnd               AgentSessionEventType = "tool_execution_end"
	AgentSessionEventTypeAgentSettled                   AgentSessionEventType = "agent_settled"
	AgentSessionEventTypeQueueUpdate                    AgentSessionEventType = "queue_update"
	AgentSessionEventTypeCompactionStart                AgentSessionEventType = "compaction_start"
	AgentSessionEventTypeCompactionEnd                  AgentSessionEventType = "compaction_end"
	AgentSessionEventTypeEntryAppended                  AgentSessionEventType = "entry_appended"
	AgentSessionEventTypeSessionInfoChanged             AgentSessionEventType = "session_info_changed"
	AgentSessionEventTypeThinkingLevelChanged           AgentSessionEventType = "thinking_level_changed"
	AgentSessionEventTypeAutoRetryStart                 AgentSessionEventType = "auto_retry_start"
	AgentSessionEventTypeAutoRetryEnd                   AgentSessionEventType = "auto_retry_end"
	AgentSessionEventTypeSummarizationRetryScheduled    AgentSessionEventType = "summarization_retry_scheduled"
	AgentSessionEventTypeSummarizationRetryAttemptStart AgentSessionEventType = "summarization_retry_attempt_start"
	AgentSessionEventTypeSummarizationRetryFinished     AgentSessionEventType = "summarization_retry_finished"
	AgentSessionEventTypeBashExecutionUpdate            AgentSessionEventType = "bash_execution_update"
)

// AgentSessionEvent is the closed set emitted by the production AgentSession.
// The ten legacy Agent variants below embed their agent package carriers so
// their payload behavior and lower-level union remain authoritative.
type AgentSessionEvent interface {
	agentSessionEvent()
	AgentSessionEventType() AgentSessionEventType
}

type AgentSessionEventListener func(AgentSessionEvent)
type AgentSessionUnsubscribe func()

type AgentSessionAgentStartEvent struct{ agent.AgentStartEvent }

func (AgentSessionAgentStartEvent) agentSessionEvent() {}
func (e AgentSessionAgentStartEvent) AgentSessionEventType() AgentSessionEventType {
	return AgentSessionEventType(e.AgentEventType())
}
func (e AgentSessionAgentStartEvent) MarshalJSON() ([]byte, error) {
	return agent.MarshalAgentEvent(e.AgentStartEvent)
}
func (AgentSessionAgentStartEvent) jsonAgentSessionEvent() {}

type AgentSessionAgentEndEvent struct {
	agent.AgentEndEvent
	WillRetry bool `json:"willRetry"`
}

func (AgentSessionAgentEndEvent) agentSessionEvent() {}
func (e AgentSessionAgentEndEvent) AgentSessionEventType() AgentSessionEventType {
	return AgentSessionEventType(e.AgentEventType())
}
func (e AgentSessionAgentEndEvent) MarshalJSON() ([]byte, error) {
	encoded, err := agent.MarshalAgentEvent(e.AgentEndEvent)
	if err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		return nil, err
	}
	fields["willRetry"], _ = json.Marshal(e.WillRetry)
	return json.Marshal(fields)
}
func (AgentSessionAgentEndEvent) jsonAgentSessionEvent() {}

type AgentSessionTurnStartEvent struct{ agent.TurnStartEvent }

func (AgentSessionTurnStartEvent) agentSessionEvent() {}
func (e AgentSessionTurnStartEvent) AgentSessionEventType() AgentSessionEventType {
	return AgentSessionEventType(e.AgentEventType())
}
func (e AgentSessionTurnStartEvent) MarshalJSON() ([]byte, error) {
	return agent.MarshalAgentEvent(e.TurnStartEvent)
}
func (AgentSessionTurnStartEvent) jsonAgentSessionEvent() {}

type AgentSessionTurnEndEvent struct{ agent.TurnEndEvent }

func (AgentSessionTurnEndEvent) agentSessionEvent() {}
func (e AgentSessionTurnEndEvent) AgentSessionEventType() AgentSessionEventType {
	return AgentSessionEventType(e.AgentEventType())
}
func (e AgentSessionTurnEndEvent) MarshalJSON() ([]byte, error) {
	return agent.MarshalAgentEvent(e.TurnEndEvent)
}
func (AgentSessionTurnEndEvent) jsonAgentSessionEvent() {}

type AgentSessionMessageStartEvent struct{ agent.MessageStartEvent }

func (AgentSessionMessageStartEvent) agentSessionEvent() {}
func (e AgentSessionMessageStartEvent) AgentSessionEventType() AgentSessionEventType {
	return AgentSessionEventType(e.AgentEventType())
}
func (e AgentSessionMessageStartEvent) MarshalJSON() ([]byte, error) {
	return agent.MarshalAgentEvent(e.MessageStartEvent)
}
func (AgentSessionMessageStartEvent) jsonAgentSessionEvent() {}

// AgentSessionMessageUpdateEvent is the in-process event. Its lower event
// retains cumulative Message and Partial snapshots; JSON/RPC projection uses
// JSONAgentSessionMessageUpdateEvent instead.
type AgentSessionMessageUpdateEvent struct{ agent.MessageUpdateEvent }

func (AgentSessionMessageUpdateEvent) agentSessionEvent() {}
func (e AgentSessionMessageUpdateEvent) AgentSessionEventType() AgentSessionEventType {
	return AgentSessionEventType(e.AgentEventType())
}
func (e AgentSessionMessageUpdateEvent) MarshalJSON() ([]byte, error) {
	return agent.MarshalAgentEvent(e.MessageUpdateEvent)
}

type AgentSessionMessageEndEvent struct{ agent.MessageEndEvent }

func (AgentSessionMessageEndEvent) agentSessionEvent() {}
func (e AgentSessionMessageEndEvent) AgentSessionEventType() AgentSessionEventType {
	return AgentSessionEventType(e.AgentEventType())
}
func (e AgentSessionMessageEndEvent) MarshalJSON() ([]byte, error) {
	return agent.MarshalAgentEvent(e.MessageEndEvent)
}
func (AgentSessionMessageEndEvent) jsonAgentSessionEvent() {}

type AgentSessionToolExecutionStartEvent struct{ agent.ToolExecutionStartEvent }

func (AgentSessionToolExecutionStartEvent) agentSessionEvent() {}
func (e AgentSessionToolExecutionStartEvent) AgentSessionEventType() AgentSessionEventType {
	return AgentSessionEventType(e.AgentEventType())
}
func (e AgentSessionToolExecutionStartEvent) MarshalJSON() ([]byte, error) {
	return agent.MarshalAgentEvent(e.ToolExecutionStartEvent)
}
func (AgentSessionToolExecutionStartEvent) jsonAgentSessionEvent() {}

type AgentSessionToolExecutionUpdateEvent struct{ agent.ToolExecutionUpdateEvent }

func (AgentSessionToolExecutionUpdateEvent) agentSessionEvent() {}
func (e AgentSessionToolExecutionUpdateEvent) AgentSessionEventType() AgentSessionEventType {
	return AgentSessionEventType(e.AgentEventType())
}
func (e AgentSessionToolExecutionUpdateEvent) MarshalJSON() ([]byte, error) {
	return agent.MarshalAgentEvent(e.ToolExecutionUpdateEvent)
}
func (AgentSessionToolExecutionUpdateEvent) jsonAgentSessionEvent() {}

type AgentSessionToolExecutionEndEvent struct{ agent.ToolExecutionEndEvent }

func (AgentSessionToolExecutionEndEvent) agentSessionEvent() {}
func (e AgentSessionToolExecutionEndEvent) AgentSessionEventType() AgentSessionEventType {
	return AgentSessionEventType(e.AgentEventType())
}
func (e AgentSessionToolExecutionEndEvent) MarshalJSON() ([]byte, error) {
	return agent.MarshalAgentEvent(e.ToolExecutionEndEvent)
}
func (AgentSessionToolExecutionEndEvent) jsonAgentSessionEvent() {}

type CompactionReason string

const (
	CompactionReasonManual    CompactionReason = "manual"
	CompactionReasonThreshold CompactionReason = "threshold"
	CompactionReasonOverflow  CompactionReason = "overflow"
)

type SummarizationRetrySource string

const (
	SummarizationRetrySourceBranchSummary SummarizationRetrySource = "branchSummary"
	SummarizationRetrySourceCompaction    SummarizationRetrySource = "compaction"
)

type AgentSessionAgentSettledEvent struct {
	Type AgentSessionEventType `json:"type"`
}

type AgentSessionQueueUpdateEvent struct {
	Type     AgentSessionEventType `json:"type"`
	Steering []string              `json:"steering"`
	FollowUp []string              `json:"followUp"`
}

type AgentSessionCompactionStartEvent struct {
	Type   AgentSessionEventType `json:"type"`
	Reason CompactionReason      `json:"reason"`
}

type AgentSessionCompactionEndEvent struct {
	Type         AgentSessionEventType `json:"type"`
	Reason       CompactionReason      `json:"reason"`
	Result       *CompactionResult     `json:"result,omitempty"`
	Aborted      bool                  `json:"aborted"`
	WillRetry    bool                  `json:"willRetry"`
	ErrorMessage *string               `json:"errorMessage,omitempty"`
}

type AgentSessionEntryAppendedEvent struct {
	Type  AgentSessionEventType `json:"type"`
	Entry SessionEntry          `json:"entry"`
}

type AgentSessionInfoChangedEvent struct {
	Type AgentSessionEventType `json:"type"`
	Name *string               `json:"name,omitempty"`
}

type AgentSessionThinkingLevelChangedEvent struct {
	Type  AgentSessionEventType `json:"type"`
	Level agent.ThinkingLevel   `json:"level"`
}

type AgentSessionAutoRetryStartEvent struct {
	Type         AgentSessionEventType `json:"type"`
	Attempt      int                   `json:"attempt"`
	MaxAttempts  int                   `json:"maxAttempts"`
	DelayMS      int64                 `json:"delayMs"`
	ErrorMessage string                `json:"errorMessage"`
}

type AgentSessionAutoRetryEndEvent struct {
	Type       AgentSessionEventType `json:"type"`
	Success    bool                  `json:"success"`
	Attempt    int                   `json:"attempt"`
	FinalError *string               `json:"finalError,omitempty"`
}

type AgentSessionSummarizationRetryScheduledEvent struct {
	Type         AgentSessionEventType `json:"type"`
	Attempt      int                   `json:"attempt"`
	MaxAttempts  int                   `json:"maxAttempts"`
	DelayMS      int64                 `json:"delayMs"`
	ErrorMessage string                `json:"errorMessage"`
}

type AgentSessionBranchSummaryRetryAttemptStartEvent struct {
	Type   AgentSessionEventType    `json:"type"`
	Source SummarizationRetrySource `json:"source"`
}

type AgentSessionCompactionRetryAttemptStartEvent struct {
	Type   AgentSessionEventType    `json:"type"`
	Source SummarizationRetrySource `json:"source"`
	Reason CompactionReason         `json:"reason"`
}

type AgentSessionSummarizationRetryFinishedEvent struct {
	Type AgentSessionEventType `json:"type"`
}

type AgentSessionBashExecutionUpdateEvent struct {
	Type  AgentSessionEventType `json:"type"`
	ID    *string               `json:"id,omitempty"`
	Delta string                `json:"delta"`
}

func (AgentSessionAgentSettledEvent) agentSessionEvent()                       {}
func (AgentSessionQueueUpdateEvent) agentSessionEvent()                        {}
func (AgentSessionCompactionStartEvent) agentSessionEvent()                    {}
func (AgentSessionCompactionEndEvent) agentSessionEvent()                      {}
func (AgentSessionEntryAppendedEvent) agentSessionEvent()                      {}
func (AgentSessionInfoChangedEvent) agentSessionEvent()                        {}
func (AgentSessionThinkingLevelChangedEvent) agentSessionEvent()               {}
func (AgentSessionAutoRetryStartEvent) agentSessionEvent()                     {}
func (AgentSessionAutoRetryEndEvent) agentSessionEvent()                       {}
func (AgentSessionSummarizationRetryScheduledEvent) agentSessionEvent()        {}
func (AgentSessionBranchSummaryRetryAttemptStartEvent) agentSessionEvent()     {}
func (AgentSessionCompactionRetryAttemptStartEvent) agentSessionEvent()        {}
func (AgentSessionSummarizationRetryFinishedEvent) agentSessionEvent()         {}
func (AgentSessionBashExecutionUpdateEvent) agentSessionEvent()                {}
func (AgentSessionAgentSettledEvent) jsonAgentSessionEvent()                   {}
func (AgentSessionQueueUpdateEvent) jsonAgentSessionEvent()                    {}
func (AgentSessionCompactionStartEvent) jsonAgentSessionEvent()                {}
func (AgentSessionCompactionEndEvent) jsonAgentSessionEvent()                  {}
func (AgentSessionEntryAppendedEvent) jsonAgentSessionEvent()                  {}
func (AgentSessionInfoChangedEvent) jsonAgentSessionEvent()                    {}
func (AgentSessionThinkingLevelChangedEvent) jsonAgentSessionEvent()           {}
func (AgentSessionAutoRetryStartEvent) jsonAgentSessionEvent()                 {}
func (AgentSessionAutoRetryEndEvent) jsonAgentSessionEvent()                   {}
func (AgentSessionSummarizationRetryScheduledEvent) jsonAgentSessionEvent()    {}
func (AgentSessionBranchSummaryRetryAttemptStartEvent) jsonAgentSessionEvent() {}
func (AgentSessionCompactionRetryAttemptStartEvent) jsonAgentSessionEvent()    {}
func (AgentSessionSummarizationRetryFinishedEvent) jsonAgentSessionEvent()     {}
func (AgentSessionBashExecutionUpdateEvent) jsonAgentSessionEvent()            {}

func (e AgentSessionAgentSettledEvent) AgentSessionEventType() AgentSessionEventType {
	return e.Type
}
func (e AgentSessionQueueUpdateEvent) AgentSessionEventType() AgentSessionEventType {
	return e.Type
}
func (e AgentSessionCompactionStartEvent) AgentSessionEventType() AgentSessionEventType {
	return e.Type
}
func (e AgentSessionCompactionEndEvent) AgentSessionEventType() AgentSessionEventType {
	return e.Type
}
func (e AgentSessionEntryAppendedEvent) AgentSessionEventType() AgentSessionEventType {
	return e.Type
}
func (e AgentSessionInfoChangedEvent) AgentSessionEventType() AgentSessionEventType {
	return e.Type
}
func (e AgentSessionThinkingLevelChangedEvent) AgentSessionEventType() AgentSessionEventType {
	return e.Type
}
func (e AgentSessionAutoRetryStartEvent) AgentSessionEventType() AgentSessionEventType {
	return e.Type
}
func (e AgentSessionAutoRetryEndEvent) AgentSessionEventType() AgentSessionEventType {
	return e.Type
}
func (e AgentSessionSummarizationRetryScheduledEvent) AgentSessionEventType() AgentSessionEventType {
	return e.Type
}
func (e AgentSessionBranchSummaryRetryAttemptStartEvent) AgentSessionEventType() AgentSessionEventType {
	return e.Type
}
func (e AgentSessionCompactionRetryAttemptStartEvent) AgentSessionEventType() AgentSessionEventType {
	return e.Type
}
func (e AgentSessionSummarizationRetryFinishedEvent) AgentSessionEventType() AgentSessionEventType {
	return e.Type
}
func (e AgentSessionBashExecutionUpdateEvent) AgentSessionEventType() AgentSessionEventType {
	return e.Type
}

type ModelCycleResult struct {
	IsScoped      bool
	Model         ai.Model
	ThinkingLevel agent.ThinkingLevel
}

// UserMessageDelivery selects the queue used when a user message is sent
// while the Agent is already streaming.
type UserMessageDelivery string

const (
	UserMessageDeliverySteer    UserMessageDelivery = "steer"
	UserMessageDeliveryFollowUp UserMessageDelivery = "followUp"
)

// ModelCycleDirection selects the direction used when cycling models.
type ModelCycleDirection string

const (
	ModelCycleForward  ModelCycleDirection = "forward"
	ModelCycleBackward ModelCycleDirection = "backward"
)

type SendUserMessageOptions struct {
	DeliverAs UserMessageDelivery
}

// ExtensionBindings inventories the optional AgentSession extension
// bindings without choosing an executable Go callback ABI. ADR-0009 keeps
// every executable slot opaque until the extension-runtime milestone.
type ExtensionBindings struct {
	UIContext             *ExtensionUIContext
	Mode                  *ExtensionMode
	CommandContextActions *ExtensionCommandContextActions
	AbortHandler          ExtensionHandler
	ShutdownHandler       ExtensionHandler
	OnError               ExtensionHandler
}

type ExecuteBashOptions struct {
	OnChunk            func(string)
	ExcludeFromContext bool
	ID                 *string
	Operations         BashOperations
}

type RecordBashResultOptions struct {
	ExcludeFromContext bool
}

type ParsedSkillBlock struct{ Content, Location, Name, UserMessage string }
type PromptOptions struct {
	ExpandPromptTemplates *bool
	Images                []ai.ImageContent
	PreflightResult       func(bool)
	Source                string
	StreamingBehavior     string
}
type SessionStats struct {
	AssistantMessages, ToolCalls, ToolResults, TotalMessages, UserMessages int
	ContextUsage                                                           *ContextUsage
	Cost                                                                   float64
	SessionFile                                                            *string
	SessionID                                                              string
	Tokens                                                                 ai.Usage
}
type ScopedModel struct {
	Model         ai.Model
	ThinkingLevel agent.ThinkingLevel
}
type AgentSessionConfig struct {
	Agent                                                       *agent.Agent
	AllowedToolNames, ExcludedToolNames, InitialActiveToolNames []string
	BaseToolsOverride                                           map[string]agent.ErasedAgentTool
	CWD                                                         string
	CustomTools                                                 []ToolDefinition
	ExtensionRunnerRef                                          *ExtensionRunner
	ModelRuntime                                                *ModelRuntime
	ResourceLoader                                              ResourceLoader
	ScopedModels                                                []ScopedModel
	SessionManager                                              *SessionManager
	SessionStartEvent                                           *SessionStartEvent
	SettingsManager                                             *SettingsManager
}

type AgentSession struct {
	agent                                   *agent.Agent
	sessionManager                          *SessionManager
	settingsManager                         *SettingsManager
	resourceLoader                          ResourceLoader
	modelRuntime                            *ModelRuntime
	extensionRunner                         *ExtensionRunner
	sessionStartEvent                       *SessionStartEvent
	scopedModels                            []ScopedModel
	activeToolNames                         []string
	autoCompactionEnabled, autoRetryEnabled bool
	retryAttempt                            int
}

func NewAgentSession(config AgentSessionConfig) *AgentSession {
	return &AgentSession{agent: config.Agent, sessionManager: config.SessionManager, settingsManager: config.SettingsManager, resourceLoader: config.ResourceLoader, modelRuntime: config.ModelRuntime, extensionRunner: config.ExtensionRunnerRef, sessionStartEvent: config.SessionStartEvent, scopedModels: cloneScopedModels(config.ScopedModels), activeToolNames: append([]string(nil), config.InitialActiveToolNames...)}
}
func (s *AgentSession) Agent() *agent.Agent               { return s.agent }
func (s *AgentSession) SessionManager() *SessionManager   { return s.sessionManager }
func (s *AgentSession) SettingsManager() *SettingsManager { return s.settingsManager }
func (s *AgentSession) ResourceLoader() ResourceLoader    { return s.resourceLoader }
func (s *AgentSession) ModelRuntime() *ModelRuntime       { return s.modelRuntime }
func (s *AgentSession) ExtensionRunner() *ExtensionRunner { return s.extensionRunner }
func (s *AgentSession) ScopedModels() []ScopedModel {
	return cloneScopedModels(s.scopedModels)
}
func (s *AgentSession) State() agent.AgentState {
	if s.agent == nil {
		return agent.AgentState{}
	}
	return s.agent.State()
}
func (s *AgentSession) Messages() []agent.AgentMessage     { return s.State().Messages }
func (s *AgentSession) Model() ai.Model                    { return s.State().Model }
func (s *AgentSession) ThinkingLevel() agent.ThinkingLevel { return s.State().ThinkingLevel }
func (s *AgentSession) SystemPrompt() string               { return s.State().SystemPrompt }
func (s *AgentSession) IsStreaming() bool                  { return s.agent != nil && s.agent.Busy() }
func (s *AgentSession) IsIdle() bool                       { return !s.IsStreaming() }
func (*AgentSession) IsCompacting() (bool, error) {
	return false, notImplemented("AgentSession.IsCompacting")
}
func (*AgentSession) IsRetrying() (bool, error) {
	return false, notImplemented("AgentSession.IsRetrying")
}
func (*AgentSession) IsBashRunning() (bool, error) {
	return false, notImplemented("AgentSession.IsBashRunning")
}
func (*AgentSession) HasPendingBashMessages() (bool, error) {
	return false, notImplemented("AgentSession.HasPendingBashMessages")
}
func (s *AgentSession) PendingMessageCount() (int, error) {
	return 0, notImplemented("AgentSession.PendingMessageCount")
}
func (s *AgentSession) RetryAttempt() int { return s.retryAttempt }
func (s *AgentSession) SessionFile() *string {
	if s.sessionManager == nil {
		return nil
	}
	return s.sessionManager.GetSessionFile()
}
func (s *AgentSession) SessionID() string {
	if s.sessionManager != nil {
		return s.sessionManager.GetSessionID()
	}
	if s.agent != nil {
		return s.agent.SessionID()
	}
	return ""
}
func (s *AgentSession) SessionName() *string {
	if s.sessionManager == nil {
		return nil
	}
	return s.sessionManager.GetSessionName()
}
func (s *AgentSession) AutoCompactionEnabled() bool { return s.autoCompactionEnabled }
func (s *AgentSession) AutoRetryEnabled() bool      { return s.autoRetryEnabled }
func (s *AgentSession) SteeringMode() agent.QueueMode {
	if s.agent == nil {
		return ""
	}
	return s.agent.SteeringMode()
}
func (s *AgentSession) FollowUpMode() agent.QueueMode {
	if s.agent == nil {
		return ""
	}
	return s.agent.FollowUpMode()
}
func (s *AgentSession) GetActiveToolNames() []string {
	return append([]string(nil), s.activeToolNames...)
}
func (s *AgentSession) GetAllTools() []agent.ErasedAgentTool {
	if s.agent == nil {
		return nil
	}
	return s.agent.State().Tools
}
func (*AgentSession) GetSteeringMessages() ([]string, error) {
	return nil, notImplemented("AgentSession.GetSteeringMessages")
}
func (*AgentSession) GetFollowUpMessages() ([]string, error) {
	return nil, notImplemented("AgentSession.GetFollowUpMessages")
}
func (s *AgentSession) ClearQueue() error { return notImplemented("AgentSession.ClearQueue") }
func (s *AgentSession) WaitForIdle(ctx context.Context) error {
	return notImplemented("AgentSession.WaitForIdle")
}
func (s *AgentSession) Abort() error   { return notImplemented("AgentSession.Abort") }
func (s *AgentSession) Dispose() error { return notImplemented("AgentSession.Dispose") }
func (s *AgentSession) Subscribe(AgentSessionEventListener) (AgentSessionUnsubscribe, error) {
	return nil, notImplemented("AgentSession.Subscribe")
}
func (s *AgentSession) Prompt(context.Context, string, ...PromptOptions) error {
	return notImplemented("AgentSession.Prompt")
}
func (s *AgentSession) Steer(string) error    { return notImplemented("AgentSession.Steer") }
func (s *AgentSession) FollowUp(string) error { return notImplemented("AgentSession.FollowUp") }
func (s *AgentSession) SendCustomMessage(any, ...any) error {
	return notImplemented("AgentSession.SendCustomMessage")
}
func (s *AgentSession) SendUserMessage(ai.UserMessageContent, ...SendUserMessageOptions) error {
	return notImplemented("AgentSession.SendUserMessage")
}
func (s *AgentSession) AbortBash() error { return notImplemented("AgentSession.AbortBash") }
func (s *AgentSession) AbortBranchSummary() error {
	return notImplemented("AgentSession.AbortBranchSummary")
}
func (s *AgentSession) AbortCompaction() error { return notImplemented("AgentSession.AbortCompaction") }
func (s *AgentSession) AbortRetry() error      { return notImplemented("AgentSession.AbortRetry") }
func (s *AgentSession) BindExtensions(ExtensionBindings) error {
	return notImplemented("AgentSession.BindExtensions")
}
func (s *AgentSession) Compact(context.Context, ...string) (CompactionResult, error) {
	return CompactionResult{}, notImplemented("AgentSession.Compact")
}
func (s *AgentSession) CreateReplacedSessionContext() (ExtensionCommandContext, error) {
	return ExtensionCommandContext{}, notImplemented("AgentSession.CreateReplacedSessionContext")
}
func (s *AgentSession) CycleModel(context.Context, ...ModelCycleDirection) (*ModelCycleResult, error) {
	return nil, notImplemented("AgentSession.CycleModel")
}
func (s *AgentSession) CycleThinkingLevel() (agent.ThinkingLevel, error) {
	return "", notImplemented("AgentSession.CycleThinkingLevel")
}
func (s *AgentSession) ExecuteBash(context.Context, string, ...ExecuteBashOptions) (BashResult, error) {
	return BashResult{}, notImplemented("AgentSession.ExecuteBash")
}
func (s *AgentSession) ExportToHTML(context.Context, ...string) (string, error) {
	return "", notImplemented("AgentSession.ExportToHTML")
}
func (s *AgentSession) ExportToJSONL(...string) (string, error) {
	return "", notImplemented("AgentSession.ExportToJSONL")
}
func (*AgentSession) GetAvailableThinkingLevels() ([]agent.ThinkingLevel, error) {
	return nil, notImplemented("AgentSession.GetAvailableThinkingLevels")
}
func (*AgentSession) GetContextUsage() (*ContextUsage, error) {
	return nil, notImplemented("AgentSession.GetContextUsage")
}
func (*AgentSession) GetLastAssistantText() (*string, error) {
	return nil, notImplemented("AgentSession.GetLastAssistantText")
}
func (*AgentSession) GetSessionStats() (SessionStats, error) {
	return SessionStats{}, notImplemented("AgentSession.GetSessionStats")
}
func (*AgentSession) GetToolDefinition(string) (ToolDefinition, bool, error) {
	return ToolDefinition{}, false, notImplemented("AgentSession.GetToolDefinition")
}

type ForkMessage struct {
	EntryID string
	Text    string
}

func (*AgentSession) GetUserMessagesForForking() ([]ForkMessage, error) {
	return nil, notImplemented("AgentSession.GetUserMessagesForForking")
}
func (*AgentSession) HasExtensionHandlers(string) (bool, error) {
	return false, notImplemented("AgentSession.HasExtensionHandlers")
}
func (s *AgentSession) NavigateTree(string) error {
	return notImplemented("AgentSession.NavigateTree")
}
func (*AgentSession) PromptTemplates() ([]PromptTemplate, error) {
	return nil, notImplemented("AgentSession.PromptTemplates")
}
func (s *AgentSession) RecordBashResult(string, BashResult, ...RecordBashResultOptions) error {
	return notImplemented("AgentSession.RecordBashResult")
}
func (s *AgentSession) Reload(context.Context) error { return notImplemented("AgentSession.Reload") }
func (s *AgentSession) SetActiveToolsByName([]string) error {
	return notImplemented("AgentSession.SetActiveToolsByName")
}
func (s *AgentSession) SetAutoCompactionEnabled(bool) error {
	return notImplemented("AgentSession.SetAutoCompactionEnabled")
}
func (s *AgentSession) SetAutoRetryEnabled(bool) error {
	return notImplemented("AgentSession.SetAutoRetryEnabled")
}
func (s *AgentSession) SetFollowUpMode(agent.QueueMode) error {
	return notImplemented("AgentSession.SetFollowUpMode")
}
func (s *AgentSession) SetModel(ai.Model) error { return notImplemented("AgentSession.SetModel") }
func (s *AgentSession) SetScopedModels([]ScopedModel) error {
	return notImplemented("AgentSession.SetScopedModels")
}
func (s *AgentSession) SetSessionName(string) error {
	return notImplemented("AgentSession.SetSessionName")
}
func (s *AgentSession) SetSteeringMode(agent.QueueMode) error {
	return notImplemented("AgentSession.SetSteeringMode")
}
func (s *AgentSession) SetThinkingLevel(agent.ThinkingLevel) error {
	return notImplemented("AgentSession.SetThinkingLevel")
}
func (*AgentSession) SupportsThinking() (bool, error) {
	return false, notImplemented("AgentSession.SupportsThinking")
}

var skillBlockPattern = regexp.MustCompile(`(?s)^<skill name="([^"]+)" location="([^"]+)">\n(.*?)\n</skill>(?:\n\n(.+))?$`)

func ParseSkillBlock(input string) *ParsedSkillBlock {
	match := skillBlockPattern.FindStringSubmatch(input)
	if match == nil {
		return nil
	}
	return &ParsedSkillBlock{
		Name:        match[1],
		Location:    match[2],
		Content:     match[3],
		UserMessage: strings.TrimSpace(match[4]),
	}
}

func cloneScopedModels(models []ScopedModel) []ScopedModel {
	if models == nil {
		return nil
	}
	cloned := make([]ScopedModel, len(models))
	for index := range models {
		cloned[index] = models[index]
		model := models[index].Model
		cloned[index].Model.Input = append([]ai.ModelInput(nil), model.Input...)
		cloned[index].Model.Cost.Tiers = append([]ai.ModelCostTier(nil), model.Cost.Tiers...)
		if model.ThinkingLevelMap != nil {
			cloned[index].Model.ThinkingLevelMap = make(ai.ThinkingLevelMap, len(model.ThinkingLevelMap))
			for level, value := range model.ThinkingLevelMap {
				cloned[index].Model.ThinkingLevelMap[level] = value
			}
		}
		if model.SamplingParams != nil {
			cloned[index].Model.SamplingParams = make(map[string]json.RawMessage, len(model.SamplingParams))
			for name, raw := range model.SamplingParams {
				cloned[index].Model.SamplingParams[name] = cloneJSON(raw)
			}
		}
		if model.Headers != nil {
			cloned[index].Model.Headers = make(map[string]string, len(model.Headers))
			for name, value := range model.Headers {
				cloned[index].Model.Headers[name] = value
			}
		}
		if raw, ok := model.Compat.Value(); ok {
			cloned[index].Model.Compat = ai.Some(cloneJSON(raw))
		} else if model.Compat.IsNull() {
			cloned[index].Model.Compat = ai.Null[json.RawMessage]()
		}
	}
	return cloned
}
func cloneJSON(raw json.RawMessage) json.RawMessage { return append(json.RawMessage(nil), raw...) }

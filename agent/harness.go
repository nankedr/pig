package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/telemetry"
)

// HarnessNotImplemented is the fixed-snapshot AgentHarness capability error.
// It retains Pi's short operation name while exposing Pig's standard
// NotImplementedError through errors.Is/errors.As.
type HarnessNotImplemented struct {
	Name      string
	Message   string
	Operation string
	Cause     error
}

func (e *HarnessNotImplemented) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return fmt.Sprintf("AgentHarness.%s is not implemented yet", e.Operation)
}

func (e *HarnessNotImplemented) Unwrap() error {
	return e.Cause
}

// HarnessClosed reports an unavailable unfinished operation after Close.
type HarnessClosed struct{}

func (*HarnessClosed) Error() string {
	return "AgentHarness was closed while the operation was active"
}

type HarnessFault struct {
	Message string
	Cause   any
}

func (e *HarnessFault) Error() string { return e.Message }

func (e *HarnessFault) Unwrap() error {
	cause, _ := e.Cause.(error)
	return cause
}

type LaneBusy struct {
	Lane          string
	OperationID   string
	OperationKind OperationKind
	Message       string
}

func (e *LaneBusy) Error() string { return e.Message }
func (*LaneBusy) Tag() string     { return "LaneBusy" }
func (e *LaneBusy) ToJSON() map[string]any {
	return harnessTaggedJSON(e.Tag(), e.Message, map[string]any{"lane": e.Lane, "operationId": e.OperationID, "operationKind": e.OperationKind})
}

type MissingIdentities struct {
	Lane    string
	Tools   []string
	Models  []string
	Message string
}

func (e *MissingIdentities) Error() string { return e.Message }
func (*MissingIdentities) Tag() string     { return "MissingIdentities" }
func (e *MissingIdentities) ToJSON() map[string]any {
	return harnessTaggedJSON(e.Tag(), e.Message, map[string]any{"lane": e.Lane, "tools": append([]string(nil), e.Tools...), "models": append([]string(nil), e.Models...)})
}

type NoActiveRun struct {
	Lane    string
	Message string
}

func (e *NoActiveRun) Error() string { return e.Message }
func (*NoActiveRun) Tag() string     { return "NoActiveRun" }
func (e *NoActiveRun) ToJSON() map[string]any {
	return harnessTaggedJSON(e.Tag(), e.Message, map[string]any{"lane": e.Lane})
}

type NoActiveOperation struct {
	Lane    string
	Message string
}

func (e *NoActiveOperation) Error() string { return e.Message }
func (*NoActiveOperation) Tag() string     { return "NoActiveOperation" }
func (e *NoActiveOperation) ToJSON() map[string]any {
	return harnessTaggedJSON(e.Tag(), e.Message, map[string]any{"lane": e.Lane})
}

type NothingToResume struct {
	Lane    string
	Message string
}

func (e *NothingToResume) Error() string { return e.Message }
func (*NothingToResume) Tag() string     { return "NothingToResume" }
func (e *NothingToResume) ToJSON() map[string]any {
	return harnessTaggedJSON(e.Tag(), e.Message, map[string]any{"lane": e.Lane})
}

type InvalidMessage struct {
	Lane    string
	Reason  string
	Message string
}

func (e *InvalidMessage) Error() string { return e.Message }
func (*InvalidMessage) Tag() string     { return "InvalidMessage" }
func (e *InvalidMessage) ToJSON() map[string]any {
	return harnessTaggedJSON(e.Tag(), e.Message, map[string]any{"lane": e.Lane, "reason": e.Reason})
}

type UnknownSkill struct {
	Name    string
	Message string
}

func (e *UnknownSkill) Error() string { return e.Message }
func (*UnknownSkill) Tag() string     { return "UnknownSkill" }
func (e *UnknownSkill) ToJSON() map[string]any {
	return harnessTaggedJSON(e.Tag(), e.Message, map[string]any{"name": e.Name})
}

type UnknownTemplate struct {
	Name    string
	Message string
}

func (e *UnknownTemplate) Error() string { return e.Message }
func (*UnknownTemplate) Tag() string     { return "UnknownTemplate" }
func (e *UnknownTemplate) ToJSON() map[string]any {
	return harnessTaggedJSON(e.Tag(), e.Message, map[string]any{"name": e.Name})
}

type UnknownTarget struct {
	TargetID string
	Message  string
}

func (e *UnknownTarget) Error() string { return e.Message }
func (*UnknownTarget) Tag() string     { return "UnknownTarget" }
func (e *UnknownTarget) ToJSON() map[string]any {
	return harnessTaggedJSON(e.Tag(), e.Message, map[string]any{"targetId": e.TargetID})
}

type UnknownQueueItem struct {
	Lane    string
	EntryID string
	Message string
}

func (e *UnknownQueueItem) Error() string { return e.Message }
func (*UnknownQueueItem) Tag() string     { return "UnknownQueueItem" }
func (e *UnknownQueueItem) ToJSON() map[string]any {
	return harnessTaggedJSON(e.Tag(), e.Message, map[string]any{"lane": e.Lane, "entryId": e.EntryID})
}

type LaneExists struct {
	Lane    string
	Message string
}

func (e *LaneExists) Error() string { return e.Message }
func (*LaneExists) Tag() string     { return "LaneExists" }
func (e *LaneExists) ToJSON() map[string]any {
	return harnessTaggedJSON(e.Tag(), e.Message, map[string]any{"lane": e.Lane})
}

type InvalidLane struct {
	Lane    string
	Reason  string
	Message string
}

func (e *InvalidLane) Error() string { return e.Message }
func (*InvalidLane) Tag() string     { return "InvalidLane" }
func (e *InvalidLane) ToJSON() map[string]any {
	return harnessTaggedJSON(e.Tag(), e.Message, map[string]any{"lane": e.Lane, "reason": e.Reason})
}

type NothingToCompact struct {
	Lane    string
	Message string
}

func (e *NothingToCompact) Error() string { return e.Message }
func (*NothingToCompact) Tag() string     { return "NothingToCompact" }
func (e *NothingToCompact) ToJSON() map[string]any {
	return harnessTaggedJSON(e.Tag(), e.Message, map[string]any{"lane": e.Lane})
}

type Closed struct {
	Message string
}

func (e *Closed) Error() string { return e.Message }
func (*Closed) Tag() string     { return "Closed" }
func (e *Closed) ToJSON() map[string]any {
	return harnessTaggedJSON(e.Tag(), e.Message, nil)
}

func harnessTaggedJSON(tag, message string, properties map[string]any) map[string]any {
	payload := make(map[string]any, len(properties)+2)
	payload["_tag"] = tag
	payload["message"] = message
	for key, value := range properties {
		payload[key] = value
	}
	return payload
}

type RunRejected interface {
	taggedHarnessError
	runRejected()
}

type CompactionRejected interface {
	taggedHarnessError
	compactionRejected()
}

type NavigationRejected interface {
	taggedHarnessError
	navigationRejected()
}

type ResumeRejected interface {
	taggedHarnessError
	resumeRejected()
}

type QueueRejected interface {
	taggedHarnessError
	queueRejected()
}

type CancelQueuedRejected interface {
	taggedHarnessError
	cancelQueuedRejected()
}

type AbortRejected interface {
	taggedHarnessError
	abortRejected()
}

type taggedHarnessError interface {
	error
	Tag() string
	ToJSON() map[string]any
}

func (*LaneBusy) runRejected()                  {}
func (*LaneBusy) compactionRejected()           {}
func (*LaneBusy) navigationRejected()           {}
func (*LaneBusy) resumeRejected()               {}
func (*MissingIdentities) resumeRejected()      {}
func (*NothingToResume) resumeRejected()        {}
func (*InvalidMessage) runRejected()            {}
func (*InvalidMessage) queueRejected()          {}
func (*UnknownSkill) runRejected()              {}
func (*UnknownTemplate) runRejected()           {}
func (*UnknownTarget) navigationRejected()      {}
func (*NoActiveRun) queueRejected()             {}
func (*UnknownQueueItem) cancelQueuedRejected() {}
func (*NoActiveOperation) abortRejected()       {}
func (*NothingToCompact) compactionRejected()   {}
func (*Closed) runRejected()                    {}
func (*Closed) compactionRejected()             {}
func (*Closed) navigationRejected()             {}
func (*Closed) resumeRejected()                 {}
func (*Closed) queueRejected()                  {}
func (*Closed) cancelQueuedRejected()           {}
func (*Closed) abortRejected()                  {}

type OperationError struct {
	Code    string
	Message string
}

type RunOutcome struct {
	Kind            string
	LeafID          string
	FinalEntryID    string
	FinalEntryIDSet bool
	FinalMessage    ai.AssistantMessage
	FinalMessageSet bool
	Error           *OperationError
	Deferred        *ai.DeferredHandle
}

type CompactionOutcome struct {
	Kind   string
	LeafID string
	Entry  *CompactionEntry
	Error  *OperationError
}

type NavigationOutcome struct {
	Kind         string
	NewLeafID    string
	NewLeafIDSet bool
	LeafID       string
	LeafIDSet    bool
	SummaryEntry *BranchSummaryEntry
	Error        *OperationError
}

type ResumeOutcome struct {
	Kind       string
	Operation  OperationKind
	RunID      string
	Run        *RunOutcome
	Compaction *CompactionOutcome
	Navigation *NavigationOutcome
}

type RunResult struct {
	Result[RunOutcome]
	RunID string
}

type CompactionResult struct {
	Result[CompactionOutcome]
	RunID string
}

type NavigationResult struct {
	Result[NavigationOutcome]
	RunID string
}

type QueueOutcome struct {
	EntryID string
}

type QueueResult struct{ Result[QueueOutcome] }

type CancelQueuedOutcome struct {
	Outcome string
}

type CancelQueuedResult struct{ Result[CancelQueuedOutcome] }
type RecordUsageResult struct{ Result[struct{}] }

type AbortOutcome struct {
	RunID    string
	Steer    []AgentMessage
	FollowUp []AgentMessage
}

type AbortResult struct{ Result[AbortOutcome] }
type ResumeResult struct{ Result[ResumeOutcome] }
type CreateLaneResult struct{ Result[AgentLane] }

type NavigateOptions struct {
	Summarize          *bool
	CustomInstructions *string
	Label              *string
}

type CompactOptions struct {
	CustomInstructions *string
}

type RecordUsageOptions struct {
	EntryID    string
	EntryIDSet bool
	Details    JsonValue
	DetailsSet bool
}

type SuspendedOperation struct {
	Lane      string
	Kind      OperationKind
	ID        string
	StartedAt int64
	Reason    string
	Prompt    []AgentMessage
	Deferred  *ai.DeferredHandle
	Aborting  *SuspendedAborting
	Missing   SuspendedMissingIdentities
}

type SuspendedAborting struct {
	Steer    []AgentMessage
	FollowUp []AgentMessage
}

type SuspendedMissingIdentities struct {
	Tools  []string
	Models []string
}

type LaneOperationInfo struct {
	ID     string
	Kind   OperationKind
	Status string
}

type LaneInfo struct {
	Name      string
	LeafID    string
	LeafIDSet bool
	Operation *LaneOperationInfo
}

type QueuedItem struct {
	EntryID string
	Message AgentMessage
}

type LaneQueues struct {
	Steer    []QueuedItem
	FollowUp []QueuedItem
	NextRun  []QueuedItem
}

type PendingWrite struct {
	ID    string
	Entry ProvisionedEntry
}

type LaneSnapshot struct {
	Lane          string
	Transcript    []Entry
	LeafID        string
	LeafIDSet     bool
	Operation     *LaneOperationInfo
	Queues        LaneQueues
	PendingWrites []PendingWrite
	Faulted       bool
}

type SessionLaneSnapshot struct {
	LaneInfo
	Suspended *SuspendedOperation
}

type SessionSnapshot struct {
	Lanes   []SessionLaneSnapshot
	Faulted bool
}

type ActionInfo struct {
	Kind       string
	EntryType  EntryType
	EntryID    string
	RecordType string
	To         string
	ToSet      bool
	Fact       string
	Outcome    string
	Queue      QueueName
	Step       StepKind
	Attempt    int
	ToolCallID string
	ToolName   string
	Provider   string
	ID         string
	Hook       HookName
	DelayMS    int64
}

type HookName string

const (
	HookBeforeRun        HookName = "before_run"
	HookBeforeResume     HookName = "before_resume"
	HookBeforeRunEnd     HookName = "before_run_end"
	HookTransformContext HookName = "transform_context"
	HookBeforeRequest    HookName = "before_request"
	HookBeforePayload    HookName = "before_payload"
	HookAfterResponse    HookName = "after_response"
	HookBeforeTool       HookName = "before_tool"
	HookAfterTool        HookName = "after_tool"
	HookBeforeCompaction HookName = "before_compaction"
	HookBeforeNavigation HookName = "before_navigation"
)

type HookHandler func(context.Context, any) (any, error)

type HookOptions struct {
	ID string
}

type Hooks interface {
	On(context.Context, HookName, HookHandler, ...HookOptions) (Unsubscribe, error)
}

type EventListener func(context.Context, any) error

type Events interface {
	On(context.Context, string, EventListener) (Unsubscribe, error)
}

type WatchHandle[TSnapshot any] interface {
	Snapshot() TSnapshot
	Start(func(any)) error
	Unsubscribe()
}

type AgentLane interface {
	Name() string
	GetLeafID(context.Context) (string, bool, error)
	Prompt(context.Context, ...AgentMessage) (RunResult, error)
	PromptText(context.Context, string, ...ai.ImageContent) (RunResult, error)
	Skill(context.Context, string, ...string) (RunResult, error)
	PromptFromTemplate(context.Context, string, ...string) (RunResult, error)
	Compact(context.Context, ...CompactOptions) (CompactionResult, error)
	NavigateTree(context.Context, *string, ...NavigateOptions) (NavigationResult, error)
	Resume(context.Context) (ResumeResult, error)
	Abort(context.Context) (AbortResult, error)
	Steer(context.Context, AgentMessage) (QueueResult, error)
	SteerText(context.Context, string, ...ai.ImageContent) (QueueResult, error)
	FollowUp(context.Context, AgentMessage) (QueueResult, error)
	FollowUpText(context.Context, string, ...ai.ImageContent) (QueueResult, error)
	NextRun(context.Context, AgentMessage) (QueueResult, error)
	NextRunText(context.Context, string, ...ai.ImageContent) (QueueResult, error)
	CancelQueued(context.Context, string) (CancelQueuedResult, error)
	RecordUsage(context.Context, ai.Usage, ...RecordUsageOptions) (RecordUsageResult, error)
	WaitForIdle(context.Context) error
	RunWhenIdle(context.Context, func(context.Context) error) error
	PeekAction(context.Context) (ActionInfo, bool, error)
	ExecuteAction(context.Context) (ActionInfo, bool, error)
	RunToCompletion(context.Context) error
	GetModel(context.Context) (ai.Model, error)
	SetModel(context.Context, ai.Model) error
	GetThinkingLevel(context.Context) (ThinkingLevel, error)
	SetThinkingLevel(context.Context, ThinkingLevel) error
	GetActiveTools(context.Context) ([]string, error)
	SetActiveTools(context.Context, []string) error
	Session() SessionTree
	Watch(context.Context) (WatchHandle[LaneSnapshot], error)
}

var _ AgentLane = (*AgentHarness)(nil)

// AgentHarnessOptions contains the dependencies required to open the fixed
// snapshot Harness scaffold.
type AgentHarnessOptions struct {
	Session            *Session
	Models             ai.Models
	Model              ai.Model
	ThinkingLevel      ThinkingLevel
	ActiveToolNames    []string
	Tools              []HarnessTool
	ToolContext        any
	SystemPrompt       any
	Resources          Resources
	StreamOptions      StreamOptions
	Retry              RetryPolicy
	Compaction         CompactionSettings
	SteeringMode       QueueMode
	FollowUpMode       QueueMode
	ToolExecution      ToolExecutionMode
	Drive              string
	ToProviderMessages func(context.Context, []AgentMessage) ([]ai.Message, error)
	EntryProjectors    map[string]EntryProjector
	Context            telemetry.TelemetryContext
}

type StreamOptions = ai.SimpleStreamOptions
type StreamOptionsPatch = ai.SimpleStreamOptions
type EntryProjector func(context.Context, Entry) ([]AgentMessage, error)

// AgentHarness owns the independent v4 Harness path. It does not bridge to the
// legacy Agent or production v3 Session.
type AgentHarness struct {
	mu                 sync.RWMutex
	session            *Session
	model              ai.Model
	thinkingLevel      ThinkingLevel
	activeToolNames    []string
	tools              []HarnessTool
	resources          Resources
	streamOptions      StreamOptions
	retryPolicy        RetryPolicy
	compactionSettings CompactionSettings
	steeringMode       QueueMode
	followUpMode       QueueMode
	closed             bool
}

// NewAgentHarness opens a record-free v4 Session. Restore is intentionally the
// same explicit capability gap as the fixed Pi snapshot.
func NewAgentHarness(
	ctx context.Context,
	options AgentHarnessOptions,
) (*AgentHarness, []SuspendedOperation, error) {
	if options.Session == nil {
		return nil, nil, fmt.Errorf("AgentHarness requires a Session")
	}
	records, err := options.Session.FindRecords(ctx, RecordQuery{Limit: 1})
	if err != nil {
		return nil, nil, err
	}
	if len(records) != 0 {
		return nil, nil, newHarnessNotImplemented("create.restore")
	}
	thinkingLevel := options.ThinkingLevel
	if thinkingLevel == "" {
		thinkingLevel = ai.ModelThinkingLevelOff
	}
	activeToolNames := append([]string(nil), options.ActiveToolNames...)
	if options.ActiveToolNames == nil {
		activeToolNames = make([]string, len(options.Tools))
		for i := range options.Tools {
			activeToolNames[i] = options.Tools[i].Name
		}
	}
	retry := options.Retry
	if retry == (RetryPolicy{}) {
		retry.BaseDelayMS = 1000
	}
	compaction := options.Compaction
	if compaction == (CompactionSettings{}) {
		compaction = DefaultCompactionSettings
	}
	steeringMode := options.SteeringMode
	if steeringMode == "" {
		steeringMode = QueueOneAtATime
	}
	followUpMode := options.FollowUpMode
	if followUpMode == "" {
		followUpMode = QueueOneAtATime
	}
	harness := &AgentHarness{
		session:            options.Session,
		model:              cloneAgentModel(options.Model),
		thinkingLevel:      thinkingLevel,
		activeToolNames:    activeToolNames,
		tools:              cloneHarnessTools(options.Tools),
		resources:          cloneHarnessResources(options.Resources),
		streamOptions:      cloneHarnessStreamOptions(options.StreamOptions),
		retryPolicy:        retry,
		compactionSettings: compaction,
		steeringMode:       steeringMode,
		followUpMode:       followUpMode,
	}
	return harness, []SuspendedOperation{}, nil
}

func newHarnessNotImplemented(operation string) *HarnessNotImplemented {
	return &HarnessNotImplemented{
		Name:      "HarnessNotImplemented",
		Message:   fmt.Sprintf("AgentHarness.%s is not implemented yet", operation),
		Operation: operation,
		Cause:     newNotImplemented("AgentHarness." + operation),
	}
}

// Name returns the root lane name.
func (*AgentHarness) Name() string {
	return "main"
}

// Session returns the root v4 Session view.
func (h *AgentHarness) Session() SessionTree {
	return h.session
}

// GetLeafID returns the root lane's current leaf.
func (h *AgentHarness) GetLeafID(ctx context.Context) (string, bool, error) {
	return h.session.GetLeafID(ctx)
}

func (h *AgentHarness) GetModel(context.Context) (ai.Model, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return cloneAgentModel(h.model), nil
}

func (h *AgentHarness) SetModel(_ context.Context, model ai.Model) error {
	h.mu.Lock()
	h.model = cloneAgentModel(model)
	h.mu.Unlock()
	return nil
}

func (h *AgentHarness) GetThinkingLevel(context.Context) (ThinkingLevel, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.thinkingLevel, nil
}

func (h *AgentHarness) SetThinkingLevel(_ context.Context, level ThinkingLevel) error {
	h.mu.Lock()
	h.thinkingLevel = level
	h.mu.Unlock()
	return nil
}

func (h *AgentHarness) GetActiveTools(context.Context) ([]string, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return append([]string(nil), h.activeToolNames...), nil
}

func (h *AgentHarness) SetActiveTools(_ context.Context, names []string) error {
	h.mu.Lock()
	h.activeToolNames = append([]string(nil), names...)
	h.mu.Unlock()
	return nil
}

func (h *AgentHarness) GetTools(context.Context) ([]HarnessTool, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return cloneHarnessTools(h.tools), nil
}

func (h *AgentHarness) SetTools(_ context.Context, tools []HarnessTool, activeNames ...[]string) error {
	h.mu.Lock()
	h.tools = cloneHarnessTools(tools)
	if len(activeNames) == 0 || activeNames[0] == nil {
		h.activeToolNames = make([]string, len(tools))
		for i := range tools {
			h.activeToolNames[i] = tools[i].Name
		}
	} else {
		h.activeToolNames = append([]string(nil), activeNames[0]...)
	}
	h.mu.Unlock()
	return nil
}

func (h *AgentHarness) GetResources(context.Context) (Resources, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return cloneHarnessResources(h.resources), nil
}

func (h *AgentHarness) SetResources(_ context.Context, resources Resources) error {
	h.mu.Lock()
	h.resources = cloneHarnessResources(resources)
	h.mu.Unlock()
	return nil
}

func (h *AgentHarness) GetStreamOptions(context.Context) (StreamOptions, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return cloneHarnessStreamOptions(h.streamOptions), nil
}

func (h *AgentHarness) SetStreamOptions(_ context.Context, options StreamOptions) error {
	h.mu.Lock()
	h.streamOptions = cloneHarnessStreamOptions(options)
	h.mu.Unlock()
	return nil
}

func (h *AgentHarness) GetRetryPolicy(context.Context) (RetryPolicy, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.retryPolicy, nil
}

func (h *AgentHarness) SetRetryPolicy(_ context.Context, policy RetryPolicy) error {
	h.mu.Lock()
	h.retryPolicy = policy
	h.mu.Unlock()
	return nil
}

func (h *AgentHarness) GetCompactionSettings(context.Context) (CompactionSettings, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.compactionSettings, nil
}

func (h *AgentHarness) SetCompactionSettings(_ context.Context, settings CompactionSettings) error {
	h.mu.Lock()
	h.compactionSettings = settings
	h.mu.Unlock()
	return nil
}

func (h *AgentHarness) GetSteeringMode(context.Context) (QueueMode, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.steeringMode, nil
}

func (h *AgentHarness) SetSteeringMode(_ context.Context, mode QueueMode) error {
	h.mu.Lock()
	h.steeringMode = mode
	h.mu.Unlock()
	return nil
}

func (h *AgentHarness) GetFollowUpMode(context.Context) (QueueMode, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.followUpMode, nil
}

func (h *AgentHarness) SetFollowUpMode(_ context.Context, mode QueueMode) error {
	h.mu.Lock()
	h.followUpMode = mode
	h.mu.Unlock()
	return nil
}

func (h *AgentHarness) Prompt(context.Context, ...AgentMessage) (RunResult, error) {
	return RunResult{}, h.unavailable("prompt")
}

func (h *AgentHarness) PromptText(context.Context, string, ...ai.ImageContent) (RunResult, error) {
	return RunResult{}, h.unavailable("prompt")
}

func (h *AgentHarness) Skill(context.Context, string, ...string) (RunResult, error) {
	return RunResult{}, h.unavailable("skill")
}

func (h *AgentHarness) PromptFromTemplate(context.Context, string, ...string) (RunResult, error) {
	return RunResult{}, h.unavailable("promptFromTemplate")
}

func (h *AgentHarness) Compact(context.Context, ...CompactOptions) (CompactionResult, error) {
	return CompactionResult{}, h.unavailable("compact")
}

func (h *AgentHarness) NavigateTree(context.Context, *string, ...NavigateOptions) (NavigationResult, error) {
	return NavigationResult{}, h.unavailable("navigateTree")
}

func (h *AgentHarness) Resume(context.Context) (ResumeResult, error) {
	return ResumeResult{}, h.unavailable("resume")
}

func (h *AgentHarness) Abort(context.Context) (AbortResult, error) {
	return AbortResult{}, h.unavailable("abort")
}

func (h *AgentHarness) Steer(context.Context, AgentMessage) (QueueResult, error) {
	return QueueResult{}, h.unavailable("steer")
}

func (h *AgentHarness) SteerText(context.Context, string, ...ai.ImageContent) (QueueResult, error) {
	return QueueResult{}, h.unavailable("steer")
}

func (h *AgentHarness) FollowUp(context.Context, AgentMessage) (QueueResult, error) {
	return QueueResult{}, h.unavailable("followUp")
}

func (h *AgentHarness) FollowUpText(context.Context, string, ...ai.ImageContent) (QueueResult, error) {
	return QueueResult{}, h.unavailable("followUp")
}

func (h *AgentHarness) NextRun(context.Context, AgentMessage) (QueueResult, error) {
	return QueueResult{}, h.unavailable("nextRun")
}

func (h *AgentHarness) NextRunText(context.Context, string, ...ai.ImageContent) (QueueResult, error) {
	return QueueResult{}, h.unavailable("nextRun")
}

func (h *AgentHarness) CancelQueued(context.Context, string) (CancelQueuedResult, error) {
	return CancelQueuedResult{}, h.unavailable("cancelQueued")
}

func (h *AgentHarness) RecordUsage(context.Context, ai.Usage, ...RecordUsageOptions) (RecordUsageResult, error) {
	return RecordUsageResult{}, h.unavailable("recordUsage")
}

func (h *AgentHarness) WaitForIdle(context.Context) error {
	return h.unavailable("waitForIdle")
}

func (h *AgentHarness) RunWhenIdle(context.Context, func(context.Context) error) error {
	return h.unavailable("runWhenIdle")
}

func (h *AgentHarness) PeekAction(context.Context) (ActionInfo, bool, error) {
	return ActionInfo{}, false, h.unavailable("peekAction")
}

func (h *AgentHarness) ExecuteAction(context.Context) (ActionInfo, bool, error) {
	return ActionInfo{}, false, h.unavailable("executeAction")
}

func (h *AgentHarness) RunToCompletion(context.Context) error {
	return h.unavailable("runToCompletion")
}

func (h *AgentHarness) Watch(context.Context) (WatchHandle[LaneSnapshot], error) {
	return nil, h.unavailable("watch")
}

func (h *AgentHarness) Lane(context.Context, string) (AgentLane, bool, error) {
	return nil, false, h.unavailable("lane")
}

func (h *AgentHarness) CreateLane(context.Context, string, *string) (CreateLaneResult, error) {
	return CreateLaneResult{}, h.unavailable("createLane")
}

func (h *AgentHarness) Lanes(context.Context) ([]LaneInfo, error) {
	return nil, h.unavailable("lanes")
}

func (h *AgentHarness) WatchSession(context.Context) (WatchHandle[SessionSnapshot], error) {
	return nil, h.unavailable("watchSession")
}

func (h *AgentHarness) Hooks() Hooks {
	return unavailableHooks{harness: h}
}

func (h *AgentHarness) Events() Events {
	return unavailableEvents{harness: h}
}

func (h *AgentHarness) unavailable(operation string) error {
	h.mu.RLock()
	closed := h.closed
	h.mu.RUnlock()
	if closed {
		return &HarnessClosed{}
	}
	return newHarnessNotImplemented(operation)
}

type unavailableHooks struct{ harness *AgentHarness }

func (r unavailableHooks) On(context.Context, HookName, HookHandler, ...HookOptions) (Unsubscribe, error) {
	return nil, r.harness.unavailable("hooks.on")
}

type unavailableEvents struct{ harness *AgentHarness }

func (r unavailableEvents) On(context.Context, string, EventListener) (Unsubscribe, error) {
	return nil, r.harness.unavailable("events.on")
}

// Close marks unfinished Harness operations closed. It does not modify the
// durable Session.
func (h *AgentHarness) Close(context.Context) error {
	h.mu.Lock()
	h.closed = true
	h.mu.Unlock()
	return nil
}

func cloneHarnessTools(tools []HarnessTool) []HarnessTool {
	if tools == nil {
		return nil
	}
	cloned := append([]HarnessTool(nil), tools...)
	for i := range cloned {
		cloned[i].Tool = cloneAgentToolMetadata(cloned[i].Tool)
	}
	return cloned
}

func cloneHarnessResources(resources Resources) Resources {
	return Resources{
		Skills:          append([]Skill(nil), resources.Skills...),
		PromptTemplates: append([]PromptTemplate(nil), resources.PromptTemplates...),
	}
}

func cloneHarnessStreamOptions(options StreamOptions) StreamOptions {
	options.APIKey = cloneHarnessPointer(options.APIKey)
	if options.Env != nil {
		options.Env = cloneHarnessMap(options.Env)
	}
	if options.Headers != nil {
		headers := make(ai.ProviderHeaders, len(options.Headers))
		for key, value := range options.Headers {
			headers[key] = cloneHarnessPointer(value)
		}
		options.Headers = headers
	}
	options.TimeoutMS = cloneHarnessPointer(options.TimeoutMS)
	options.MaxRetries = cloneHarnessPointer(options.MaxRetries)
	options.MaxRetryDelayMS = cloneHarnessPointer(options.MaxRetryDelayMS)
	options.Temperature = cloneHarnessPointer(options.Temperature)
	options.SamplingParams = cloneHarnessRawMessages(options.SamplingParams)
	options.MaxTokens = cloneHarnessPointer(options.MaxTokens)
	options.Transport = cloneHarnessPointer(options.Transport)
	options.CacheRetention = cloneHarnessPointer(options.CacheRetention)
	options.SessionID = cloneHarnessPointer(options.SessionID)
	options.WebSocketConnectTimeoutMS = cloneHarnessPointer(options.WebSocketConnectTimeoutMS)
	options.Metadata = cloneHarnessRawMessages(options.Metadata)
	options.Reasoning = cloneHarnessPointer(options.Reasoning)
	options.ThinkingBudgets = cloneThinkingBudgets(options.ThinkingBudgets)
	switch deferred := options.Deferred.(type) {
	case *ai.DeferredBoolean:
		if deferred != nil {
			cloned := *deferred
			options.Deferred = &cloned
		}
	case ai.DeferredWindowOptions:
		deferred.Window = cloneHarnessPointer(deferred.Window)
		options.Deferred = deferred
	case *ai.DeferredWindowOptions:
		if deferred != nil {
			cloned := *deferred
			cloned.Window = cloneHarnessPointer(deferred.Window)
			options.Deferred = &cloned
		}
	}
	return options
}

func cloneHarnessPointer[T any](value *T) *T {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneHarnessMap[K comparable, V any](values map[K]V) map[K]V {
	cloned := make(map[K]V, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func cloneHarnessRawMessages(values map[string]json.RawMessage) map[string]json.RawMessage {
	if values == nil {
		return nil
	}
	cloned := make(map[string]json.RawMessage, len(values))
	for key, value := range values {
		cloned[key] = append(json.RawMessage(nil), value...)
	}
	return cloned
}

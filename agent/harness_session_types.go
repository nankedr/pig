package agent

import (
	"context"
	"fmt"

	"github.com/nankedr/pig/ai"
)

type JSONValue = any
type JsonValue = JSONValue

type SessionStopReason string

const (
	SessionStopReasonStop     SessionStopReason = "stop"
	SessionStopReasonLength   SessionStopReason = "length"
	SessionStopReasonToolUse  SessionStopReason = "toolUse"
	SessionStopReasonError    SessionStopReason = "error"
	SessionStopReasonAborted  SessionStopReason = "aborted"
	SessionStopReasonDeferred SessionStopReason = "deferred"
)

type IDGenerator interface {
	Next() string
}

type IdGenerator = IDGenerator

type EntryType string

const (
	EntryTypeMessage             EntryType = "message"
	EntryTypeModelChange         EntryType = "model_change"
	EntryTypeThinkingLevelChange EntryType = "thinking_level_change"
	EntryTypeActiveToolsChange   EntryType = "active_tools_change"
	EntryTypeCompaction          EntryType = "compaction"
	EntryTypeBranchSummary       EntryType = "branch_summary"
	EntryTypeCustom              EntryType = "custom"
)

type EntryBase struct {
	ID          string
	Seq         int64
	ParentID    string
	ParentIDSet bool
	Timestamp   int64
}

type Entry interface {
	EntryType() EntryType
	entryBase() EntryBase
}

type MessageEntry struct {
	EntryBase
	Message   AgentMessage
	Terminate bool
}

func (e MessageEntry) EntryType() EntryType { return EntryTypeMessage }
func (e MessageEntry) entryBase() EntryBase { return e.EntryBase }

type ModelChangeEntry struct {
	EntryBase
	Provider string
	ModelID  string
}

func (e ModelChangeEntry) EntryType() EntryType { return EntryTypeModelChange }
func (e ModelChangeEntry) entryBase() EntryBase { return e.EntryBase }

type ThinkingLevelEntry struct {
	EntryBase
	ThinkingLevel string
}

func (e ThinkingLevelEntry) EntryType() EntryType { return EntryTypeThinkingLevelChange }
func (e ThinkingLevelEntry) entryBase() EntryBase { return e.EntryBase }

type ActiveToolsEntry struct {
	EntryBase
	ActiveToolNames []string
}

func (e ActiveToolsEntry) EntryType() EntryType { return EntryTypeActiveToolsChange }
func (e ActiveToolsEntry) entryBase() EntryBase { return e.EntryBase }

type CompactionEntry struct {
	EntryBase
	Summary      string
	RetainedTail []AgentMessage
	TokensBefore int64
	Details      JSONValue
	Usage        *ai.Usage
}

func (e CompactionEntry) EntryType() EntryType { return EntryTypeCompaction }
func (e CompactionEntry) entryBase() EntryBase { return e.EntryBase }

type BranchSummaryEntry struct {
	EntryBase
	FromID  string
	Summary string
	Details JSONValue
	Usage   *ai.Usage
}

func (e BranchSummaryEntry) EntryType() EntryType { return EntryTypeBranchSummary }
func (e BranchSummaryEntry) entryBase() EntryBase { return e.EntryBase }

type CustomEntry struct {
	EntryBase
	CustomType string
	Data       JSONValue
	DataSet    bool
}

func (e CustomEntry) EntryType() EntryType { return EntryTypeCustom }
func (e CustomEntry) entryBase() EntryBase { return e.EntryBase }

type ProvisionedEntry = Entry

type RecordBase struct {
	ID        string
	Seq       int64
	Lane      string
	Timestamp int64
}

type LaneRecordType string

const (
	LaneRecordTypeOperationStarted  LaneRecordType = "operation_started"
	LaneRecordTypeAbortRequested    LaneRecordType = "abort_requested"
	LaneRecordTypeOperationFinished LaneRecordType = "operation_finished"
	LaneRecordTypeStepAttempt       LaneRecordType = "step_attempt"
	LaneRecordTypeToolStarted       LaneRecordType = "tool_started"
	LaneRecordTypeQueueEnqueued     LaneRecordType = "queue_enqueued"
	LaneRecordTypeQueueCancelled    LaneRecordType = "queue_cancelled"
	LaneRecordTypeWriteDeferred     LaneRecordType = "write_deferred"
	LaneRecordTypeUsage             LaneRecordType = "usage"
)

type LaneRecord interface {
	RecordType() LaneRecordType
	recordBase() RecordBase
}

type OperationKind string

const (
	OperationKindRun        OperationKind = "run"
	OperationKindCompaction OperationKind = "compaction"
	OperationKindNavigation OperationKind = "navigation"
)

type OperationIntent interface {
	OperationKind() OperationKind
	operationIntent()
}

type RunOperationIntent struct {
	OriginalPrompt       []AgentMessage
	InitialMessages      []ProvisionedEntry
	SystemPromptOverride *string
	ResumeData           map[string]JSONValue
}

func (RunOperationIntent) OperationKind() OperationKind { return OperationKindRun }
func (RunOperationIntent) operationIntent()             {}

type CompactionOperationIntent struct {
	CustomInstructions *string
	ResultEntryID      string
}

func (CompactionOperationIntent) OperationKind() OperationKind { return OperationKindCompaction }
func (CompactionOperationIntent) operationIntent()             {}

type NavigationOperationIntent struct {
	TargetID           string
	TargetIDSet        bool
	Summarize          bool
	CustomInstructions *string
	Label              *string
	SummaryEntryID     *string
}

func (NavigationOperationIntent) OperationKind() OperationKind { return OperationKindNavigation }
func (NavigationOperationIntent) operationIntent()             {}

type OperationStartedRecord struct {
	RecordBase
	SourceLeafID    string
	SourceLeafIDSet bool
	Intent          OperationIntent
}

func (r OperationStartedRecord) RecordType() LaneRecordType { return LaneRecordTypeOperationStarted }
func (r OperationStartedRecord) recordBase() RecordBase     { return r.RecordBase }

type AbortRequestedRecord struct {
	RecordBase
	RunID string
}

func (r AbortRequestedRecord) RecordType() LaneRecordType { return LaneRecordTypeAbortRequested }
func (r AbortRequestedRecord) recordBase() RecordBase     { return r.RecordBase }

type OperationOutcome string

const (
	OperationOutcomeCompleted OperationOutcome = "completed"
	OperationOutcomeAborted   OperationOutcome = "aborted"
	OperationOutcomeFailed    OperationOutcome = "failed"
	OperationOutcomeDeclined  OperationOutcome = "declined"
)

type OperationRecordError struct {
	Code    string
	Message string
}

type OperationFinishedRecord struct {
	RecordBase
	RunID   string
	Outcome OperationOutcome
	Error   *OperationRecordError
}

func (r OperationFinishedRecord) RecordType() LaneRecordType { return LaneRecordTypeOperationFinished }
func (r OperationFinishedRecord) recordBase() RecordBase     { return r.RecordBase }

type CompactionReason string

const (
	CompactionReasonManual    CompactionReason = "manual"
	CompactionReasonThreshold CompactionReason = "threshold"
	CompactionReasonOverflow  CompactionReason = "overflow"
)

type StepKind string

const (
	StepKindAssistant     StepKind = "assistant"
	StepKindCompaction    StepKind = "compaction"
	StepKindBranchSummary StepKind = "branch_summary"
)

type StepAttemptRecord struct {
	RecordBase
	RunID            string
	Step             StepKind
	Attempt          int
	ResultEntryID    string
	CompactionReason *CompactionReason
}

func (r StepAttemptRecord) RecordType() LaneRecordType { return LaneRecordTypeStepAttempt }
func (r StepAttemptRecord) recordBase() RecordBase     { return r.RecordBase }

type ToolReplay string

const (
	ToolReplayNever ToolReplay = "never"
	ToolReplaySafe  ToolReplay = "safe"
)

type ToolStartedRecord struct {
	RecordBase
	RunID            string
	AssistantEntryID string
	ToolIndex        int
	ToolCallID       string
	ToolName         string
	EffectiveArgs    map[string]any
	ResultEntryID    string
	Replay           ToolReplay
}

func (r ToolStartedRecord) RecordType() LaneRecordType { return LaneRecordTypeToolStarted }
func (r ToolStartedRecord) recordBase() RecordBase     { return r.RecordBase }

type QueueName string

const (
	QueueNameSteer    QueueName = "steer"
	QueueNameFollowUp QueueName = "followUp"
	QueueNameNextRun  QueueName = "nextRun"
)

type QueueEnqueuedRecord struct {
	RecordBase
	Queue    QueueName
	RunID    string
	RunIDSet bool
	Target   ProvisionedEntry
}

func (r QueueEnqueuedRecord) RecordType() LaneRecordType { return LaneRecordTypeQueueEnqueued }
func (r QueueEnqueuedRecord) recordBase() RecordBase     { return r.RecordBase }

type QueueCancelledRecord struct {
	RecordBase
	RunID    string
	RunIDSet bool
	EntryID  string
}

func (r QueueCancelledRecord) RecordType() LaneRecordType { return LaneRecordTypeQueueCancelled }
func (r QueueCancelledRecord) recordBase() RecordBase     { return r.RecordBase }

type WriteDeferredRecord struct {
	RecordBase
	RunID  string
	Target ProvisionedEntry
}

func (r WriteDeferredRecord) RecordType() LaneRecordType { return LaneRecordTypeWriteDeferred }
func (r WriteDeferredRecord) recordBase() RecordBase     { return r.RecordBase }

type UsageCause string

const (
	UsageCauseAssistant     UsageCause = "assistant"
	UsageCauseCompaction    UsageCause = "compaction"
	UsageCauseBranchSummary UsageCause = "branch_summary"
	UsageCauseDeferredFetch UsageCause = "deferred_fetch"
	UsageCauseTool          UsageCause = "tool"
	UsageCauseHook          UsageCause = "hook"
	UsageCauseAdjustment    UsageCause = "adjustment"
)

type UsageRecord struct {
	RecordBase
	Usage      ai.Usage
	Cause      UsageCause
	RunID      string
	RunIDSet   bool
	EntryID    string
	EntryIDSet bool
	Attempt    int
	AttemptSet bool
	StopReason SessionStopReason
	ToolCallID string
	Details    JSONValue
	DetailsSet bool
}

func (r UsageRecord) RecordType() LaneRecordType { return LaneRecordTypeUsage }
func (r UsageRecord) recordBase() RecordBase     { return r.RecordBase }

type NewRecord = LaneRecord

type EntryOrder string

const (
	EntryOrderNewestFirst EntryOrder = "newestFirst"
	EntryOrderOldestFirst EntryOrder = "oldestFirst"
)

type EntryCursor struct {
	AfterSeq int64
}

type EntryQuery struct {
	Type       EntryType
	CustomType string
	Order      EntryOrder
	Limit      int
	Cursor     *EntryCursor
}

type BranchBounds struct {
	Start      string
	StartSet   bool
	StopAtType EntryType
	StopAtID   string
}

type RecordQuery struct {
	Lane          string
	Type          LaneRecordType
	RunID         string
	OperationKind OperationKind
	AfterSeq      int64
	AfterSeqSet   bool
	Order         EntryOrder
	Limit         int
}

type SessionMetadata struct {
	ID                 string
	CreatedAt          int64
	ParentSessionID    string
	ParentSessionIDSet bool
}

type SessionStats struct {
	MessageCount   int64
	CachedTokens   int64
	UncachedTokens int64
	TotalTokens    int64
	CostTotal      float64
}

type LanePointer struct {
	Lane      string
	LeafID    string
	LeafIDSet bool
}

type LogItemKind string

const (
	LogItemKindEntry  LogItemKind = "entry"
	LogItemKindRecord LogItemKind = "record"
	LogItemKindLane   LogItemKind = "lane"
	LogItemKindFact   LogItemKind = "fact"
)

type LogItem interface {
	LogItemKind() LogItemKind
	LogSequence() int64
}

type EntryLogItem struct {
	Seq   int64
	Entry Entry
}

func (i EntryLogItem) LogItemKind() LogItemKind { return LogItemKindEntry }
func (i EntryLogItem) LogSequence() int64       { return i.Seq }

type RecordLogItem struct {
	Seq    int64
	Record LaneRecord
}

func (i RecordLogItem) LogItemKind() LogItemKind { return LogItemKindRecord }
func (i RecordLogItem) LogSequence() int64       { return i.Seq }

type LaneLogItem struct {
	Seq       int64
	Lane      string
	LeafID    string
	LeafIDSet bool
}

func (i LaneLogItem) LogItemKind() LogItemKind { return LogItemKindLane }
func (i LaneLogItem) LogSequence() int64       { return i.Seq }

type FactNameLogItem struct {
	Seq     int64
	Name    string
	NameSet bool
}

func (i FactNameLogItem) LogItemKind() LogItemKind { return LogItemKindFact }
func (i FactNameLogItem) LogSequence() int64       { return i.Seq }

type FactLabelLogItem struct {
	Seq      int64
	TargetID string
	Label    string
	LabelSet bool
}

func (i FactLabelLogItem) LogItemKind() LogItemKind { return LogItemKindFact }
func (i FactLabelLogItem) LogSequence() int64       { return i.Seq }

type LogOptions struct {
	AfterSeq    int64
	AfterSeqSet bool
	Limit       int
}

type SessionStorage interface {
	GetMetadata(context.Context) (SessionMetadata, error)
	GetLanes(context.Context) ([]LanePointer, error)
	CreateLane(context.Context, string, string, bool) error
	MoveLane(context.Context, string, string, bool) error
	AppendEntry(context.Context, ProvisionedEntry, string) (Entry, error)
	AppendRecord(context.Context, NewRecord) (LaneRecord, error)
	GetEntry(context.Context, string) (Entry, bool, error)
	FindEntries(context.Context, EntryQuery) ([]Entry, error)
	FindEntriesOnBranch(context.Context, EntryQuery, BranchBounds) ([]Entry, error)
	FindRecords(context.Context, RecordQuery) ([]LaneRecord, error)
	FindOpenOperations(context.Context, string, int) ([]OperationStartedRecord, error)
	GetLog(context.Context, LogOptions) ([]LogItem, error)
	GetName(context.Context) (string, bool, error)
	SetName(context.Context, string, bool) error
	GetLabel(context.Context, string) (string, bool, error)
	SetLabel(context.Context, string, string, bool) error
	GetStats(context.Context) (SessionStats, error)
}

type SessionTree interface {
	GetLeafID(context.Context) (string, bool, error)
	GetEntry(context.Context, string) (Entry, bool, error)
	GetStats(context.Context) (SessionStats, error)
	GetName(context.Context) (string, bool, error)
	SetName(context.Context, string, bool) error
	GetLabel(context.Context, string) (string, bool, error)
	SetLabel(context.Context, string, string, bool) error
	FindEntries(context.Context, EntryQuery) ([]Entry, error)
	FindEntry(context.Context, EntryQuery) (Entry, bool, error)
	FindEntriesOnBranch(context.Context, EntryQuery, BranchBounds) ([]Entry, error)
	FindEntryOnBranch(context.Context, EntryQuery, BranchBounds) (Entry, bool, error)
	AppendMessage(context.Context, AgentMessage) (string, error)
	AppendCustomEntry(context.Context, string, ...JSONValue) (string, error)
}

type SessionCreateOptions struct {
	ID                 string
	ParentSessionID    string
	ParentSessionIDSet bool
}

type ForkScope string

const (
	ForkScopeBranch ForkScope = "branch"
	ForkScopeTree   ForkScope = "tree"
)

type ForkPosition string

const (
	ForkPositionBefore ForkPosition = "before"
	ForkPositionAt     ForkPosition = "at"
)

type ForkOptions struct {
	Scope      ForkScope
	EntryID    string
	EntryIDSet bool
	Position   ForkPosition
}

type SessionRepo interface {
	Create(context.Context, SessionCreateOptions) (*Session, error)
	Open(context.Context, SessionMetadata) (*Session, error)
	List(context.Context) ([]SessionMetadata, error)
	Delete(context.Context, SessionMetadata) error
	Fork(context.Context, SessionMetadata, ForkOptions, SessionCreateOptions) (*Session, error)
}

type SessionErrorCode string

const (
	SessionErrorNotFound          SessionErrorCode = "not_found"
	SessionErrorAlreadyExists     SessionErrorCode = "already_exists"
	SessionErrorInvalidEntry      SessionErrorCode = "invalid_entry"
	SessionErrorInvalidPayload    SessionErrorCode = "invalid_payload"
	SessionErrorInvalidLane       SessionErrorCode = "invalid_lane"
	SessionErrorInvalidQuery      SessionErrorCode = "invalid_query"
	SessionErrorInvalidForkTarget SessionErrorCode = "invalid_fork_target"
	SessionErrorStorage           SessionErrorCode = "storage"
)

type SessionError struct {
	Code    SessionErrorCode
	Message string
	Cause   error
}

func (e *SessionError) Error() string {
	if e == nil {
		return "<nil>"
	}
	return e.Message
}

func (e *SessionError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func newSessionError(code SessionErrorCode, format string, args ...any) *SessionError {
	return &SessionError{Code: code, Message: fmt.Sprintf(format, args...)}
}

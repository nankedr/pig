package protocol

// SessionPhase identifies the current session lifecycle phase.
type SessionPhase string

const (
	SessionPhaseIdle          SessionPhase = "idle"
	SessionPhaseTurn          SessionPhase = "turn"
	SessionPhaseCompaction    SessionPhase = "compaction"
	SessionPhaseBranchSummary SessionPhase = "branch_summary"
	SessionPhaseRetry         SessionPhase = "retry"
)

// SessionMetadata is the compact session record used by list and server
// snapshot messages.
type SessionMetadata struct {
	ID              string           `json:"id"`
	CreatedAt       int64            `json:"createdAt"`
	UpdatedAt       Optional[int64]  `json:"updatedAt"`
	ParentSessionID Optional[string] `json:"parentSessionId"`
	SessionName     Optional[string] `json:"sessionName"`
	CWD             Optional[string] `json:"cwd"`
}

// ServerSnapshot is the authoritative snapshot advertised by a remote
// service.
type ServerSnapshot struct {
	ServerID        string            `json:"serverId"`
	ProtocolVersion int               `json:"protocolVersion"`
	Revision        int64             `json:"revision"`
	Sessions        []SessionMetadata `json:"sessions"`
	Models          []ModelMetadata   `json:"models"`
}

// SessionSnapshot is the authoritative state for one remote session.
type SessionSnapshot struct {
	ID               string               `json:"id"`
	Name             Optional[string]     `json:"name"`
	CWD              string               `json:"cwd"`
	CreatedAt        int64                `json:"createdAt"`
	UpdatedAt        int64                `json:"updatedAt"`
	Phase            SessionPhase         `json:"phase"`
	Model            ModelRef             `json:"model"`
	ThinkingLevel    ThinkingLevel        `json:"thinkingLevel"`
	Attached         bool                 `json:"attached"`
	Locked           bool                 `json:"locked"`
	Revision         int64                `json:"revision"`
	Transcript       []TranscriptItem     `json:"transcript"`
	QueuedSteer      []UserTranscriptItem `json:"queuedSteer"`
	QueuedSteerCount int64                `json:"queuedSteerCount"`
}

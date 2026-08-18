package client

import (
	"context"

	"github.com/nankedr/pig/protocol"
)

// SessionLeaseMode controls local lifecycle coordination within one Client. It
// is not authentication, authorization, or cross-client ownership.
type SessionLeaseMode string

const (
	// SessionLeaseModeShared permits other shared leases in the same Client.
	SessionLeaseModeShared SessionLeaseMode = "shared"
	// SessionLeaseModeExclusive conflicts with every lease in the same Client.
	SessionLeaseModeExclusive SessionLeaseMode = "exclusive"
)

// AcquireSessionOptions configures local session-lease acquisition.
type AcquireSessionOptions struct {
	Mode SessionLeaseMode
}

// CreateSessionOptions configures remote session creation. Optional fields
// preserve absence separately from their Go zero values.
type CreateSessionOptions struct {
	CWD           protocol.Optional[string]
	Name          protocol.Optional[string]
	Model         protocol.Optional[protocol.ModelRef]
	ThinkingLevel protocol.Optional[protocol.ThinkingLevel]
}

// SessionLease is one local lifecycle claim on an attached remote session.
// Published snapshots are immutable by contract; M9 must clone slice-backed
// data before exposing them to callers.
type SessionLease interface {
	ID() string
	Active() bool
	Attached() bool
	Snapshot() *protocol.SessionSnapshot

	Subscribe(listener func(protocol.SessionSnapshot)) (Unsubscribe, error)
	OnEvent(listener func(protocol.ServerEvent)) (Unsubscribe, error)

	Detach(ctx context.Context) error
	Dispose(ctx context.Context) error
	Prompt(ctx context.Context, text string) (protocol.SessionSnapshot, error)
	Steer(ctx context.Context, text string) (protocol.SessionSnapshot, error)
	Abort(ctx context.Context) (protocol.SessionSnapshot, error)
	SetModel(ctx context.Context, model protocol.ModelRef) (protocol.SessionSnapshot, error)
	SetThinking(ctx context.Context, level protocol.ThinkingLevel) (protocol.SessionSnapshot, error)
}

// SessionHandle maps the upstream PiSessionHandle alias to Pig terminology.
type SessionHandle = SessionLease

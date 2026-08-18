package client

import (
	"github.com/nankedr/pig/internal/capability"
	"github.com/nankedr/pig/protocol"
)

// ErrNotImplemented identifies a locally deferred Client capability.
var ErrNotImplemented = capability.ErrNotImplemented

// NotImplementedError identifies the module and operation of a deferred call.
type NotImplementedError = capability.NotImplementedError

// ServerError maps PiServerError and preserves the peer's protocol code and
// optional JSON details. It is distinct from the local ErrNotImplemented stub.
type ServerError struct {
	Code    protocol.ProtocolErrorCode
	Details protocol.Optional[protocol.JSONValue]
	Message string
}

// NewServerError converts one wire ProtocolError into a client-facing error.
func NewServerError(remote protocol.ProtocolError) *ServerError {
	return &ServerError{Code: remote.Code, Details: remote.Details, Message: remote.Message}
}

func (e *ServerError) Error() string {
	return e.Message
}

// DisconnectedError reports that an operation requires a connected Client.
type DisconnectedError struct {
	Message string
}

func (e *DisconnectedError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return "Pig client is disconnected"
}

// ClientDisposedError reports use after Client disposal.
type ClientDisposedError struct{}

func (e *ClientDisposedError) Error() string {
	return "Pig client is disposed"
}

// SessionOwnershipError reports a local shared/exclusive lease conflict. Lease
// ownership is lifecycle coordination within one Client, not authorization.
type SessionOwnershipError struct {
	SessionID string
	Message   string
}

func (e *SessionOwnershipError) Error() string {
	return e.Message
}

// SessionDetachedError reports use of an inactive session lease.
type SessionDetachedError struct {
	SessionID string
}

func (e *SessionDetachedError) Error() string {
	return "session " + e.SessionID + " is not attached"
}

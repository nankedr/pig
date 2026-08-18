package protocol

// ProtocolErrorCode classifies an error sent over the wire.
type ProtocolErrorCode string

const (
	ProtocolErrorCodeVersion        ProtocolErrorCode = "version"
	ProtocolErrorCodeBusy           ProtocolErrorCode = "busy"
	ProtocolErrorCodeSessionLocked  ProtocolErrorCode = "session_locked"
	ProtocolErrorCodeNotFound       ProtocolErrorCode = "not_found"
	ProtocolErrorCodeInvalidRequest ProtocolErrorCode = "invalid_request"
	ProtocolErrorCodeNotImplemented ProtocolErrorCode = "not_implemented"
	ProtocolErrorCodeInternalError  ProtocolErrorCode = "internal_error"
)

// ProtocolError is wire data carried by failed response messages. It is
// intentionally distinct from local Go errors.
type ProtocolError struct {
	Code    ProtocolErrorCode   `json:"code"`
	Message string              `json:"message"`
	Details Optional[JSONValue] `json:"details"`
}

// ServerEventType is a server-event wire discriminator.
type ServerEventType string

const (
	ServerEventTypeServerSnapshot  ServerEventType = "server_snapshot"
	ServerEventTypeSessionSnapshot ServerEventType = "session_snapshot"
	ServerEventTypeSessionProgress ServerEventType = "session_progress"
	ServerEventTypeSessionRemoved  ServerEventType = "session_removed"
)

// ServerEvent is a closed union of unsolicited server events.
type ServerEvent interface {
	serverEvent()
	ServerEventType() ServerEventType
}

// ServerSnapshotEvent replaces the caller's authoritative server snapshot.
type ServerSnapshotEvent struct {
	Type     ServerEventType `json:"type"`
	Snapshot ServerSnapshot  `json:"snapshot"`
}

func (ServerSnapshotEvent) serverEvent()                       {}
func (e ServerSnapshotEvent) ServerEventType() ServerEventType { return e.Type }

// SessionSnapshotEvent replaces the caller's authoritative session snapshot.
type SessionSnapshotEvent struct {
	Type     ServerEventType `json:"type"`
	Snapshot SessionSnapshot `json:"snapshot"`
}

func (SessionSnapshotEvent) serverEvent()                       {}
func (e SessionSnapshotEvent) ServerEventType() ServerEventType { return e.Type }

// SessionProgressEvent reports incremental activity for a session.
type SessionProgressEvent struct {
	Type      ServerEventType    `json:"type"`
	SessionID string             `json:"sessionId"`
	Progress  TranscriptProgress `json:"progress"`
}

func (SessionProgressEvent) serverEvent()                       {}
func (e SessionProgressEvent) ServerEventType() ServerEventType { return e.Type }

// SessionRemovedEvent reports that a session no longer exists.
type SessionRemovedEvent struct {
	Type      ServerEventType `json:"type"`
	SessionID string          `json:"sessionId"`
}

func (SessionRemovedEvent) serverEvent()                       {}
func (e SessionRemovedEvent) ServerEventType() ServerEventType { return e.Type }

// MessageType is a top-level message wire discriminator.
type MessageType string

const (
	MessageTypeHello      MessageType = "hello"
	MessageTypeHelloError MessageType = "hello_error"
	MessageTypeRequest    MessageType = "request"
	MessageTypeResponse   MessageType = "response"
	MessageTypeEvent      MessageType = "event"
)

// ClientMessage is a closed union of client-to-server messages.
type ClientMessage interface {
	clientMessage()
	ClientMessageType() MessageType
}

type ClientHello struct {
	Type    MessageType `json:"type"`
	Version int         `json:"version"`
}

func (ClientHello) clientMessage()                   {}
func (m ClientHello) ClientMessageType() MessageType { return m.Type }

type RequestEnvelope struct {
	Type    MessageType `json:"type"`
	ID      string      `json:"id"`
	Request Command     `json:"request"`
}

func (RequestEnvelope) clientMessage()                   {}
func (m RequestEnvelope) ClientMessageType() MessageType { return m.Type }

// ServerMessage is a closed union of server-to-client messages.
type ServerMessage interface {
	serverMessage()
	ServerMessageType() MessageType
}

type ServerHello struct {
	Type         MessageType    `json:"type"`
	Version      int            `json:"version"`
	ConnectionID string         `json:"connectionId"`
	Snapshot     ServerSnapshot `json:"snapshot"`
}

func (ServerHello) serverMessage()                   {}
func (m ServerHello) ServerMessageType() MessageType { return m.Type }

type ServerHelloError struct {
	Type  MessageType   `json:"type"`
	Error ProtocolError `json:"error"`
}

func (ServerHelloError) serverMessage()                   {}
func (m ServerHelloError) ServerMessageType() MessageType { return m.Type }

// ResponseEnvelope is a closed union of success and error responses.
type ResponseEnvelope interface {
	ServerMessage
	responseEnvelope()
}

type SuccessResponseEnvelope struct {
	Type   MessageType   `json:"type"`
	ID     string        `json:"id"`
	OK     bool          `json:"ok"`
	Result CommandResult `json:"result"`
}

func (SuccessResponseEnvelope) serverMessage()                   {}
func (SuccessResponseEnvelope) responseEnvelope()                {}
func (m SuccessResponseEnvelope) ServerMessageType() MessageType { return m.Type }

type ErrorResponseEnvelope struct {
	Type  MessageType   `json:"type"`
	ID    string        `json:"id"`
	OK    bool          `json:"ok"`
	Error ProtocolError `json:"error"`
}

func (ErrorResponseEnvelope) serverMessage()                   {}
func (ErrorResponseEnvelope) responseEnvelope()                {}
func (m ErrorResponseEnvelope) ServerMessageType() MessageType { return m.Type }

type EventEnvelope struct {
	Type  MessageType `json:"type"`
	Event ServerEvent `json:"event"`
}

func (EventEnvelope) serverMessage()                   {}
func (m EventEnvelope) ServerMessageType() MessageType { return m.Type }

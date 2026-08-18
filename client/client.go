package client

import (
	"context"
	"errors"

	"github.com/nankedr/pig/protocol"
)

// Client is a transport-neutral Remote Session Protocol client. It maps the
// upstream PiClient class to Pig's Go API. M0 exposes the complete contract but
// keeps remotely meaningful behavior as explicit capability stubs.
type Client struct{}

// NewClient allocates a disconnected Client. It validates no remote state and
// never invokes options.TransportFactory.
func NewClient(options ClientOptions) (*Client, error) {
	if options.TransportFactory == nil {
		return nil, errors.New("client transport factory must not be nil")
	}
	if options.MaxFrameLength != nil && *options.MaxFrameLength == 0 {
		return nil, errors.New("client max frame length must be positive")
	}
	return &Client{}, nil
}

// Dial maps the static PiClient.connect operation. M0 returns an explicit
// capability stub before inspecting ctx or invoking the transport factory.
func Dial(ctx context.Context, options ClientOptions) (*Client, error) {
	return nil, notImplemented("Dial")
}

// ConnectionState reports the truthful local connection state.
func (c *Client) ConnectionState() ConnectionState {
	return ConnectionStateDisconnected
}

// Connected reports whether the Remote Session Protocol handshake completed.
func (c *Client) Connected() bool {
	return false
}

// Disposed reports whether the Client has been disposed.
func (c *Client) Disposed() bool {
	return false
}

// Snapshot returns the latest authoritative server snapshot, if any.
func (c *Client) Snapshot() *protocol.ServerSnapshot {
	return nil
}

// Connect establishes a Remote Session Protocol connection.
func (c *Client) Connect(ctx context.Context) (protocol.ServerSnapshot, error) {
	return protocol.ServerSnapshot{}, notImplemented("Client.Connect")
}

// Reconnect starts a fresh connection attempt; automatic reconnect is not part
// of the contract.
func (c *Client) Reconnect(ctx context.Context) (protocol.ServerSnapshot, error) {
	return protocol.ServerSnapshot{}, notImplemented("Client.Reconnect")
}

// Disconnect closes the current connection with reason.
func (c *Client) Disconnect(reason string) error {
	return notImplemented("Client.Disconnect")
}

// Dispose releases the Client's resources. Its context is the Go shape for a
// blocking cleanup call and does not claim request tombstone behavior.
func (c *Client) Dispose(ctx context.Context) error {
	return notImplemented("Client.Dispose")
}

// Subscribe registers a listener for authoritative server snapshots. Published
// snapshots are immutable by contract; M9 must clone slice-backed data before
// exposing it.
func (c *Client) Subscribe(listener func(protocol.ServerSnapshot)) (Unsubscribe, error) {
	return nil, notImplemented("Client.Subscribe")
}

// OnEvent registers a listener for Remote Session Protocol server events.
func (c *Client) OnEvent(listener func(protocol.ServerEvent)) (Unsubscribe, error) {
	return nil, notImplemented("Client.OnEvent")
}

// OnConnectionStateChange registers a listener for connection transitions.
func (c *Client) OnConnectionStateChange(listener func(ConnectionStateChange)) (Unsubscribe, error) {
	return nil, notImplemented("Client.OnConnectionStateChange")
}

// ListSessions requests refreshed durable session metadata. It does not return
// fabricated empty success while the remote capability is deferred.
func (c *Client) ListSessions(ctx context.Context) ([]protocol.SessionMetadata, error) {
	return nil, notImplemented("Client.ListSessions")
}

// CreateSession creates a remote session and, once implemented, returns an
// exclusive local lease.
func (c *Client) CreateSession(ctx context.Context, options CreateSessionOptions) (SessionHandle, error) {
	return nil, notImplemented("Client.CreateSession")
}

// AttachSession is the shared-lease convenience operation.
func (c *Client) AttachSession(ctx context.Context, sessionID string) (SessionHandle, error) {
	return nil, notImplemented("Client.AttachSession")
}

// AcquireSession attaches a session under a local shared or exclusive lease.
func (c *Client) AcquireSession(ctx context.Context, sessionID string, options AcquireSessionOptions) (SessionHandle, error) {
	return nil, notImplemented("Client.AcquireSession")
}

func notImplemented(operation string) *NotImplementedError {
	return &NotImplementedError{Module: "client", Operation: operation}
}

package client

// ConnectionState is the lifecycle state of a Client connection.
type ConnectionState string

const (
	// ConnectionStateDisconnected is the initial state and follows a terminal close.
	ConnectionStateDisconnected ConnectionState = "disconnected"
	// ConnectionStateConnecting means transport establishment or handshake is in progress.
	ConnectionStateConnecting ConnectionState = "connecting"
	// ConnectionStateConnected means the Remote Session Protocol handshake completed.
	ConnectionStateConnected ConnectionState = "connected"
)

// ConnectionStateChange describes one connection lifecycle transition.
type ConnectionStateChange struct {
	State ConnectionState
	Error error
}

// Unsubscribe removes a listener. Implementations must make repeated calls harmless.
type Unsubscribe func()

// ListenerErrorHandler reports subscriber failures without corrupting client state.
type ListenerErrorHandler func(error)

// ClientOptions configures a Client. It maps PiClientOptions without carrying
// Pi product branding into Pig's Go API.
type ClientOptions struct {
	TransportFactory ByteTransportFactory
	// MaxFrameLength is absent for the protocol default. A present zero value is
	// invalid, matching the fixed Client contract.
	MaxFrameLength  *uint32
	OnListenerError ListenerErrorHandler
}

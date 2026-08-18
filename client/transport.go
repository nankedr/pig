package client

import "context"

// ByteTransport exchanges arbitrary ordered byte chunks for the Remote Session
// Protocol. It does not encode Coding Agent JSONL RPC messages.
type ByteTransport interface {
	// Send blocks until chunk has been accepted and preserves invocation order.
	Send(ctx context.Context, chunk []byte) error
	// Close closes the transport. Repeated calls must be harmless.
	Close() error
}

// ByteTransportHandlers receives inbound chunks and terminal transport events.
// A transport must report at most one of OnClose and OnError.
type ByteTransportHandlers struct {
	OnData  func(chunk []byte)
	OnClose func()
	OnError func(error)
}

// ByteTransportFactory creates a fresh connected and authenticated transport.
// The context is the Go shape for a potentially blocking factory call; it does
// not imply that Client request-cancellation tombstones are implemented.
type ByteTransportFactory func(ctx context.Context, handlers ByteTransportHandlers) (ByteTransport, error)

// Package unix provides the Unix-domain-socket ByteTransport factory contract
// for the Remote Session Protocol. It is unrelated to Coding Agent JSONL RPC.
//
// M0 validates options without dialing. Invoking a validated factory returns a
// structured capability error without opening a socket, starting a goroutine,
// retaining state, or invoking handlers.
package unix

import (
	"context"
	"errors"
	"fmt"
	"runtime"

	"github.com/nankedr/pig/client"
)

// Options maps the upstream UnixTransportOptions export.
type Options struct {
	Path string
	// MaxPendingBytes is absent for four times the protocol frame default. A
	// present value must be positive.
	MaxPendingBytes *int
}

// NewTransportFactory maps createUnixTransportFactory. M0 performs only pure
// option validation and returns a stateless deferred factory.
func NewTransportFactory(options Options) (client.ByteTransportFactory, error) {
	if options.Path == "" {
		return nil, errors.New("unix transport path must not be empty")
	}
	maxPathBytes := 103
	if runtime.GOOS == "linux" {
		maxPathBytes = 107
	}
	if len(options.Path) > maxPathBytes {
		return nil, fmt.Errorf("unix transport path is too long; maximum is %d UTF-8 bytes", maxPathBytes)
	}
	if options.MaxPendingBytes != nil && *options.MaxPendingBytes <= 0 {
		return nil, errors.New("unix transport max pending bytes must be positive")
	}
	if runtime.GOOS == "windows" {
		return nil, errors.New("unix transport is not supported on Windows")
	}
	return func(context.Context, client.ByteTransportHandlers) (client.ByteTransport, error) {
		return nil, &client.NotImplementedError{
			Module:    "client/unix",
			Operation: "NewTransportFactory.Invoke",
		}
	}, nil
}

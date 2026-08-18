package unix_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/nankedr/pig/client"
	clientunix "github.com/nankedr/pig/client/unix"
)

func TestTransportFactoryInvocationFailsWithoutDialingOrCallingHandlers(t *testing.T) {
	socketPath := fmt.Sprintf("/private/tmp/pig-client-%d-deferred.sock", os.Getpid())
	if _, err := os.Stat(socketPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("socket path exists before test or cannot be checked: %v", err)
	}
	factory, err := clientunix.NewTransportFactory(clientunix.Options{Path: socketPath})
	if err != nil {
		t.Fatalf("NewTransportFactory error = %v", err)
	}

	handlerCalled := false
	handlers := client.ByteTransportHandlers{
		OnData:  func([]byte) { handlerCalled = true },
		OnClose: func() { handlerCalled = true },
		OnError: func(error) { handlerCalled = true },
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	transport, err := factory(ctx, handlers)
	if transport != nil {
		t.Fatalf("factory transport = %#v, want nil", transport)
	}
	if !errors.Is(err, client.ErrNotImplemented) {
		t.Fatalf("factory error = %v, want ErrNotImplemented", err)
	}
	if errors.Is(err, context.Canceled) {
		t.Fatal("factory returned context cancellation instead of capability stub")
	}
	var target *client.NotImplementedError
	if !errors.As(err, &target) {
		t.Fatalf("errors.As(%v, *NotImplementedError) = false", err)
	}
	if target.Module != "client/unix" || target.Operation != "NewTransportFactory.Invoke" {
		t.Fatalf("NotImplementedError = %#v", target)
	}
	if handlerCalled {
		t.Fatal("deferred Unix factory invoked a transport handler")
	}
	if _, statErr := os.Stat(socketPath); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("socket path stat error = %v, want not-exist", statErr)
	}
}

func TestNewTransportFactoryValidatesOptionsWithoutFilesystemAccess(t *testing.T) {
	maxPathBytes := 103
	if runtime.GOOS == "linux" {
		maxPathBytes = 107
	}
	tests := []struct {
		name      string
		options   clientunix.Options
		wantValid bool
	}{
		{name: "default pending byte limit", options: clientunix.Options{Path: "missing.sock"}, wantValid: true},
		{name: "positive pending byte limit", options: clientunix.Options{Path: "missing.sock", MaxPendingBytes: intPointer(1)}, wantValid: true},
		{name: "empty path", options: clientunix.Options{}},
		{name: "path too long", options: clientunix.Options{Path: strings.Repeat("x", maxPathBytes+1)}},
		{name: "explicit zero pending byte limit", options: clientunix.Options{Path: "missing.sock", MaxPendingBytes: intPointer(0)}},
		{name: "negative pending byte limit", options: clientunix.Options{Path: "missing.sock", MaxPendingBytes: intPointer(-1)}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.wantValid && runtime.GOOS == "windows" {
				t.Skip("Unix transport is not available on Windows")
			}
			factory, err := clientunix.NewTransportFactory(test.options)
			if test.wantValid {
				if err != nil {
					t.Fatalf("NewTransportFactory(%#v) error = %v", test.options, err)
				}
				if factory == nil {
					t.Fatalf("NewTransportFactory(%#v) factory = nil", test.options)
				}
				return
			}
			if err == nil {
				t.Fatalf("NewTransportFactory(%#v) error = nil", test.options)
			}
			if factory != nil {
				t.Fatalf("NewTransportFactory(%#v) factory != nil", test.options)
			}
		})
	}
}

func TestNewTransportFactoryRejectsPendingBytesAboveJavaScriptSafeInteger(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix transport is not available on Windows")
	}
	if strconv.IntSize < 64 {
		t.Skip("values above JavaScript's maximum safe integer do not fit in int")
	}
	maxSafeInteger64 := int64(1)<<53 - 1
	maxSafeInteger := int(maxSafeInteger64)
	factory, err := clientunix.NewTransportFactory(clientunix.Options{
		Path:            "missing.sock",
		MaxPendingBytes: &maxSafeInteger,
	})
	if err != nil || factory == nil {
		t.Fatalf("NewTransportFactory(max safe integer) = (%v, %v), want non-nil factory and nil error", factory, err)
	}

	aboveSafeInteger64 := int64(1) << 53
	aboveSafeInteger := int(aboveSafeInteger64)
	factory, err = clientunix.NewTransportFactory(clientunix.Options{
		Path:            "missing.sock",
		MaxPendingBytes: &aboveSafeInteger,
	})
	if err == nil {
		t.Fatal("NewTransportFactory error = nil")
	}
	if factory != nil {
		t.Fatal("NewTransportFactory factory != nil")
	}
}

func intPointer(value int) *int {
	return &value
}

// Compile-time Unix export-subpath parity: both exports on Pi client's `./unix`
// subpath map to compile-usable declarations. Comments preserve the exact Pi
// names recorded in parity/surface/symbols.jsonl.
var (
	_                                                               = clientunix.Options{Path: "", MaxPendingBytes: new(int)} // UnixTransportOptions -> unix.Options
	_ func(clientunix.Options) (client.ByteTransportFactory, error) = clientunix.NewTransportFactory                          // createUnixTransportFactory -> NewTransportFactory
)

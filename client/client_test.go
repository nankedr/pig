package client_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/nankedr/pig/client"
	"github.com/nankedr/pig/protocol"
)

type contractTransport struct{}

func (contractTransport) Send(context.Context, []byte) error { return nil }
func (contractTransport) Close() error                       { return nil }

func TestByteTransportContractIsUsable(t *testing.T) {
	handlers := client.ByteTransportHandlers{
		OnData:  func([]byte) {},
		OnClose: func() {},
		OnError: func(error) {},
	}
	factory := client.ByteTransportFactory(func(context.Context, client.ByteTransportHandlers) (client.ByteTransport, error) {
		return contractTransport{}, nil
	})

	transport, err := factory(context.Background(), handlers)
	if err != nil {
		t.Fatalf("factory error = %v", err)
	}
	if err := transport.Send(context.Background(), []byte("chunk")); err != nil {
		t.Fatalf("Send error = %v", err)
	}
	if err := transport.Close(); err != nil {
		t.Fatalf("Close error = %v", err)
	}
}

func TestNewClientIsDisconnectedAndDoesNotCreateTransport(t *testing.T) {
	factoryCalls := 0
	factory := func(context.Context, client.ByteTransportHandlers) (client.ByteTransport, error) {
		factoryCalls++
		return contractTransport{}, nil
	}

	c, err := client.NewClient(client.ClientOptions{TransportFactory: factory})
	if err != nil {
		t.Fatalf("NewClient error = %v", err)
	}
	if factoryCalls != 0 {
		t.Fatalf("transport factory calls = %d, want 0", factoryCalls)
	}
	if got := c.ConnectionState(); got != client.ConnectionStateDisconnected {
		t.Fatalf("ConnectionState = %q, want %q", got, client.ConnectionStateDisconnected)
	}
	if c.Connected() {
		t.Fatal("Connected = true, want false")
	}
	if c.Disposed() {
		t.Fatal("Disposed = true, want false")
	}
	if snapshot := c.Snapshot(); snapshot != nil {
		t.Fatalf("Snapshot = %#v, want nil", snapshot)
	}
}

func TestNewClientRejectsMissingTransportFactory(t *testing.T) {
	c, err := client.NewClient(client.ClientOptions{})
	if err == nil {
		t.Fatal("NewClient error = nil, want missing-factory validation error")
	}
	if c != nil {
		t.Fatalf("NewClient client = %#v, want nil", c)
	}
}

func TestNewClientDistinguishesDefaultAndExplicitZeroFrameLimit(t *testing.T) {
	factory := func(context.Context, client.ByteTransportHandlers) (client.ByteTransport, error) {
		return contractTransport{}, nil
	}
	if c, err := client.NewClient(client.ClientOptions{TransportFactory: factory}); err != nil || c == nil {
		t.Fatalf("NewClient(default frame limit) = (%#v, %v), want client and nil error", c, err)
	}
	zero := uint32(0)
	if c, err := client.NewClient(client.ClientOptions{TransportFactory: factory, MaxFrameLength: &zero}); err == nil || c != nil {
		t.Fatalf("NewClient(explicit zero frame limit) = (%#v, %v), want nil client and error", c, err)
	}
}

func TestConnectionLifecycleStubsFailWithoutCreatingTransport(t *testing.T) {
	factoryCalls := 0
	options := client.ClientOptions{
		TransportFactory: func(context.Context, client.ByteTransportHandlers) (client.ByteTransport, error) {
			factoryCalls++
			return contractTransport{}, nil
		},
	}
	c, err := client.NewClient(options)
	if err != nil {
		t.Fatalf("NewClient error = %v", err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()

	dialed, err := client.Dial(canceled, options)
	assertNotImplemented(t, err, "Dial")
	if dialed != nil {
		t.Fatalf("Dial client = %#v, want nil", dialed)
	}

	snapshot, err := c.Connect(canceled)
	assertNotImplemented(t, err, "Client.Connect")
	if !reflect.DeepEqual(snapshot, protocol.ServerSnapshot{}) {
		t.Fatalf("Connect snapshot = %#v, want zero value", snapshot)
	}

	snapshot, err = c.Reconnect(canceled)
	assertNotImplemented(t, err, "Client.Reconnect")
	if !reflect.DeepEqual(snapshot, protocol.ServerSnapshot{}) {
		t.Fatalf("Reconnect snapshot = %#v, want zero value", snapshot)
	}

	assertNotImplemented(t, c.Disconnect("test disconnect"), "Client.Disconnect")
	assertNotImplemented(t, c.Dispose(canceled), "Client.Dispose")

	if factoryCalls != 0 {
		t.Fatalf("transport factory calls = %d, want 0", factoryCalls)
	}
	if got := c.ConnectionState(); got != client.ConnectionStateDisconnected {
		t.Fatalf("ConnectionState after stubs = %q, want disconnected", got)
	}
	if c.Connected() || c.Disposed() || c.Snapshot() != nil {
		t.Fatal("lifecycle stubs changed observable client state")
	}
}

func assertNotImplemented(t *testing.T, err error, operation string) {
	t.Helper()
	if !errors.Is(err, client.ErrNotImplemented) {
		t.Fatalf("%s error = %v, want ErrNotImplemented", operation, err)
	}
	if errors.Is(err, context.Canceled) {
		t.Fatalf("%s error = %v, unexpectedly matches context.Canceled", operation, err)
	}
	var target *client.NotImplementedError
	if !errors.As(err, &target) {
		t.Fatalf("errors.As(%v, *NotImplementedError) = false", err)
	}
	if target.Module != "client" || target.Operation != operation {
		t.Fatalf("NotImplementedError = %#v, want module client operation %s", target, operation)
	}
}

func TestStructuredClientErrorsPreservePublicDetails(t *testing.T) {
	remote := protocol.ProtocolError{
		Code:    protocol.ProtocolErrorCodeNotImplemented,
		Message: "remote capability is unavailable",
		Details: protocol.Some[protocol.JSONValue](map[string]any{
			"operation": "prompt",
			"retry":     map[string]any{"allowed": false},
		}),
	}
	server := client.NewServerError(remote)
	if server.Code != remote.Code {
		t.Fatalf("ServerError = %#v", server)
	}
	if !reflect.DeepEqual(server.Details, remote.Details) {
		t.Fatalf("ServerError.Details = %#v, want %#v", server.Details, remote.Details)
	}
	if server.Error() != "remote capability is unavailable" {
		t.Fatalf("ServerError.Error() = %q", server.Error())
	}

	tests := []struct {
		name      string
		err       error
		contains  string
		sessionID string
	}{
		{name: "disposed", err: &client.ClientDisposedError{}, contains: "Pig client is disposed"},
		{name: "disconnected default", err: &client.DisconnectedError{}, contains: "Pig client is disconnected"},
		{name: "disconnected reason", err: &client.DisconnectedError{Message: "transport closed"}, contains: "transport closed"},
		{name: "detached", err: &client.SessionDetachedError{SessionID: "s-1"}, contains: "not attached", sessionID: "s-1"},
		{name: "ownership", err: &client.SessionOwnershipError{SessionID: "s-2", Message: "exclusive lease exists"}, contains: "exclusive lease exists", sessionID: "s-2"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if !strings.Contains(test.err.Error(), test.contains) {
				t.Fatalf("Error() = %q, want substring %q", test.err.Error(), test.contains)
			}
			switch err := test.err.(type) {
			case *client.SessionDetachedError:
				if err.SessionID != test.sessionID {
					t.Fatalf("SessionID = %q, want %q", err.SessionID, test.sessionID)
				}
			case *client.SessionOwnershipError:
				if err.SessionID != test.sessionID {
					t.Fatalf("SessionID = %q, want %q", err.SessionID, test.sessionID)
				}
			}
		})
	}
}

func TestClientWireStrings(t *testing.T) {
	connectionStates := []struct {
		name string
		got  client.ConnectionState
		want string
	}{
		{name: "disconnected", got: client.ConnectionStateDisconnected, want: "disconnected"},
		{name: "connecting", got: client.ConnectionStateConnecting, want: "connecting"},
		{name: "connected", got: client.ConnectionStateConnected, want: "connected"},
	}
	for _, test := range connectionStates {
		t.Run("connection state/"+test.name, func(t *testing.T) {
			if string(test.got) != test.want {
				t.Fatalf("wire value = %q, want %q", test.got, test.want)
			}
		})
	}

	leaseModes := []struct {
		name string
		got  client.SessionLeaseMode
		want string
	}{
		{name: "shared", got: client.SessionLeaseModeShared, want: "shared"},
		{name: "exclusive", got: client.SessionLeaseModeExclusive, want: "exclusive"},
	}
	for _, test := range leaseModes {
		t.Run("lease mode/"+test.name, func(t *testing.T) {
			if string(test.got) != test.want {
				t.Fatalf("wire value = %q, want %q", test.got, test.want)
			}
		})
	}
}

func TestSubscriptionStubsDoNotRegisterOrInvokeListeners(t *testing.T) {
	c, err := client.NewClient(client.ClientOptions{
		TransportFactory: func(context.Context, client.ByteTransportHandlers) (client.ByteTransport, error) {
			t.Fatal("subscription stub invoked transport factory")
			return nil, nil
		},
	})
	if err != nil {
		t.Fatalf("NewClient error = %v", err)
	}

	listenerCalled := false
	unsubscribe, err := c.Subscribe(func(protocol.ServerSnapshot) { listenerCalled = true })
	assertNotImplemented(t, err, "Client.Subscribe")
	if unsubscribe != nil {
		t.Fatal("Subscribe returned a plausible unsubscribe function")
	}

	unsubscribe, err = c.OnEvent(func(protocol.ServerEvent) { listenerCalled = true })
	assertNotImplemented(t, err, "Client.OnEvent")
	if unsubscribe != nil {
		t.Fatal("OnEvent returned a plausible unsubscribe function")
	}

	unsubscribe, err = c.OnConnectionStateChange(func(client.ConnectionStateChange) { listenerCalled = true })
	assertNotImplemented(t, err, "Client.OnConnectionStateChange")
	if unsubscribe != nil {
		t.Fatal("OnConnectionStateChange returned a plausible unsubscribe function")
	}
	if listenerCalled {
		t.Fatal("subscription stub invoked a listener")
	}
}

func TestSessionRequestStubsFailWithoutTransportOrLeaseState(t *testing.T) {
	factoryCalls := 0
	c, err := client.NewClient(client.ClientOptions{
		TransportFactory: func(context.Context, client.ByteTransportHandlers) (client.ByteTransport, error) {
			factoryCalls++
			return contractTransport{}, nil
		},
	})
	if err != nil {
		t.Fatalf("NewClient error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	sessions, err := c.ListSessions(ctx)
	assertNotImplemented(t, err, "Client.ListSessions")
	if sessions != nil {
		t.Fatalf("ListSessions = %#v, want nil (not fabricated empty success)", sessions)
	}

	lease, err := c.CreateSession(ctx, client.CreateSessionOptions{
		CWD:           protocol.Some("/workspace"),
		Name:          protocol.Some("review"),
		Model:         protocol.Some(protocol.ModelRef{Provider: "test", ID: "model"}),
		ThinkingLevel: protocol.Some(protocol.ThinkingLevelMedium),
	})
	assertNotImplemented(t, err, "Client.CreateSession")
	if lease != nil {
		t.Fatalf("CreateSession lease = %#v, want nil", lease)
	}

	lease, err = c.AttachSession(ctx, "session-1")
	assertNotImplemented(t, err, "Client.AttachSession")
	if lease != nil {
		t.Fatalf("AttachSession lease = %#v, want nil", lease)
	}

	lease, err = c.AcquireSession(ctx, "session-1", client.AcquireSessionOptions{Mode: client.SessionLeaseModeShared})
	assertNotImplemented(t, err, "Client.AcquireSession")
	if lease != nil {
		t.Fatalf("AcquireSession lease = %#v, want nil", lease)
	}
	if factoryCalls != 0 {
		t.Fatalf("transport factory calls = %d, want 0", factoryCalls)
	}
	if c.Snapshot() != nil || c.Connected() || c.Disposed() {
		t.Fatal("session request stubs changed observable client state")
	}
}

// Compile-time root surface parity: every one of the 19 exports on Pi's client
// root (`.`) export subpath maps to a compile-usable Pig Go declaration. The
// trailing comments preserve navigation to the exact Pi names recorded in
// parity/surface/symbols.jsonl.
var (
	_ *client.Client // PiClient -> Client (constructed by NewClient; static connect -> Dial)

	_ = &client.ClientDisposedError{}          // PiClientDisposedError -> ClientDisposedError
	_ = &client.DisconnectedError{Message: ""} // PiDisconnectedError -> DisconnectedError
	_ = &client.ServerError{                   // PiServerError -> ServerError
		Code:    protocol.ProtocolErrorCodeNotImplemented,
		Details: protocol.None[protocol.JSONValue](),
		Message: "",
	}
	_ = &client.SessionDetachedError{SessionID: ""}               // PiSessionDetachedError -> SessionDetachedError
	_ = &client.SessionOwnershipError{SessionID: "", Message: ""} // PiSessionOwnershipError -> SessionOwnershipError

	_                         = client.AcquireSessionOptions{Mode: client.SessionLeaseModeShared} // AcquireSessionOptions
	_ client.SessionHandle    = (*compileSessionLease)(nil)                                       // PiSessionHandle -> SessionHandle
	_ client.SessionLease     = (*compileSessionLease)(nil)                                       // SessionLease
	_ client.SessionLeaseMode = client.SessionLeaseModeShared                                     // SessionLeaseMode

	_ client.ByteTransport        = contractTransport{} // ByteTransport
	_ client.ByteTransportFactory = func(context.Context, client.ByteTransportHandlers) (client.ByteTransport, error) {
		return nil, nil
	} // ByteTransportFactory
	_ = client.ByteTransportHandlers{ // ByteTransportHandlers
		OnData:  func([]byte) {},
		OnClose: func() {},
		OnError: func(error) {},
	}

	_ client.ConnectionState = client.ConnectionStateDisconnected // ConnectionState
	_                        = client.ConnectionStateChange{State: client.ConnectionStateDisconnected, Error: nil}
	_                        = client.CreateSessionOptions{ // CreateSessionOptions
		CWD:           protocol.None[string](),
		Name:          protocol.None[string](),
		Model:         protocol.None[protocol.ModelRef](),
		ThinkingLevel: protocol.None[protocol.ThinkingLevel](),
	}
	_ client.ListenerErrorHandler = func(error) {}        // ListenerErrorHandler
	_                             = client.ClientOptions{ // PiClientOptions -> ClientOptions
		TransportFactory: func(context.Context, client.ByteTransportHandlers) (client.ByteTransport, error) {
			return nil, nil
		},
		MaxFrameLength:  new(uint32),
		OnListenerError: func(error) {},
	}
	_ client.Unsubscribe = func() {} // Unsubscribe
)

// Compile-time PiClient member snapshot. NewClient maps construction and Dial
// maps the upstream static PiClient.connect operation.
var (
	_ = client.NewClient
	_ = client.Dial
	_ = (*client.Client).Connect
	_ = (*client.Client).Reconnect
	_ = (*client.Client).Disconnect
	_ = (*client.Client).Dispose
	_ = (*client.Client).Connected
	_ = (*client.Client).ConnectionState
	_ = (*client.Client).Disposed
	_ = (*client.Client).Snapshot
	_ = (*client.Client).Subscribe
	_ = (*client.Client).OnEvent
	_ = (*client.Client).OnConnectionStateChange
	_ = (*client.Client).ListSessions
	_ = (*client.Client).CreateSession
	_ = (*client.Client).AttachSession
	_ = (*client.Client).AcquireSession
)

type compileSessionLease struct{}

func (*compileSessionLease) ID() string                          { return "" }
func (*compileSessionLease) Active() bool                        { return false }
func (*compileSessionLease) Attached() bool                      { return false }
func (*compileSessionLease) Snapshot() *protocol.SessionSnapshot { return nil }
func (*compileSessionLease) Subscribe(func(protocol.SessionSnapshot)) (client.Unsubscribe, error) {
	return nil, nil
}
func (*compileSessionLease) OnEvent(func(protocol.ServerEvent)) (client.Unsubscribe, error) {
	return nil, nil
}
func (*compileSessionLease) Detach(context.Context) error  { return nil }
func (*compileSessionLease) Dispose(context.Context) error { return nil }
func (*compileSessionLease) Prompt(context.Context, string) (protocol.SessionSnapshot, error) {
	return protocol.SessionSnapshot{}, nil
}
func (*compileSessionLease) Steer(context.Context, string) (protocol.SessionSnapshot, error) {
	return protocol.SessionSnapshot{}, nil
}
func (*compileSessionLease) Abort(context.Context) (protocol.SessionSnapshot, error) {
	return protocol.SessionSnapshot{}, nil
}
func (*compileSessionLease) SetModel(context.Context, protocol.ModelRef) (protocol.SessionSnapshot, error) {
	return protocol.SessionSnapshot{}, nil
}
func (*compileSessionLease) SetThinking(context.Context, protocol.ThinkingLevel) (protocol.SessionSnapshot, error) {
	return protocol.SessionSnapshot{}, nil
}

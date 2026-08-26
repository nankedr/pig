package codingagent_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/client"
	"github.com/nankedr/pig/codingagent"
)

// These assignments pin the Go projections of the 14 static class methods in
// the Coding Agent baseline. Optional TypeScript arguments map to pointers or
// variadic option values, while asynchronous operations receive a context.
var (
	_ func(context.Context, *client.Client, string, ...codingagent.RemoteSessionOptions) (*codingagent.RemoteSession, error)                                 = codingagent.OpenRemoteSession
	_ func(context.Context, *client.Client, codingagent.CreateRemoteSessionOptions, ...codingagent.RemoteSessionOptions) (*codingagent.RemoteSession, error) = codingagent.CreateRemoteSession
	_ func(...string) (*codingagent.KeybindingsManager, error)                                                                                               = codingagent.NewKeybindingsManager
	_ func(context.Context, ...codingagent.CreateModelRuntimeOptions) (*codingagent.ModelRuntime, error)                                                     = codingagent.NewModelRuntime
	_ func(string, *string, ...codingagent.NewSessionOptions) (*codingagent.SessionManager, error)                                                           = codingagent.NewSessionManager
	_ func(string, *string, *string) (*codingagent.SessionManager, error)                                                                                    = codingagent.OpenSessionManager
	_ func(string, *string) (*codingagent.SessionManager, error)                                                                                             = codingagent.ContinueRecentSessionManager
	_ func(string, ...codingagent.NewSessionOptions) *codingagent.SessionManager                                                                             = codingagent.NewInMemorySessionManager
	_ func(string, string, *string, ...codingagent.NewSessionOptions) (*codingagent.SessionManager, error)                                                   = codingagent.ForkSessionManager
	_ func(context.Context, string, ...codingagent.SessionListOptions) ([]codingagent.SessionInfo, error)                                                    = codingagent.ListSessions
	_ func(context.Context, ...codingagent.SessionListOptions) ([]codingagent.SessionInfo, error)                                                            = codingagent.ListAllSessions
	_ func(string, *string, ...codingagent.SettingsManagerCreateOptions) (*codingagent.SettingsManager, error)                                               = codingagent.NewSettingsManager
	_ func(codingagent.SettingsStorage, ...codingagent.SettingsManagerCreateOptions) (*codingagent.SettingsManager, error)                                   = codingagent.NewSettingsManagerFromStorage
	_ func(codingagent.Settings, ...codingagent.SettingsManagerCreateOptions) (*codingagent.SettingsManager, error)                                          = codingagent.NewInMemorySettingsManager

	_ *bool                     = (codingagent.SettingsManagerCreateOptions{}).ProjectTrusted
	_ codingagent.SettingsScope = (codingagent.SettingsError{}).Scope
	_ error                     = (codingagent.SettingsError{}).Error
)

type staticFactoryPanicCredentialStore struct{ calls *int }

func (s staticFactoryPanicCredentialStore) called() {
	(*s.calls)++
	panic("credential store must not be called by a static capability stub")
}

func (s staticFactoryPanicCredentialStore) Read(context.Context, ai.ProviderID, ai.AuthOperationOptions) (ai.Credential, error) {
	s.called()
	return nil, nil
}

func (s staticFactoryPanicCredentialStore) List(context.Context, ai.AuthOperationOptions) ([]ai.CredentialInfo, error) {
	s.called()
	return nil, nil
}

func (s staticFactoryPanicCredentialStore) Modify(context.Context, ai.ProviderID, ai.CredentialModifyFunc, ai.AuthOperationOptions) (ai.Credential, error) {
	s.called()
	return nil, nil
}

func (s staticFactoryPanicCredentialStore) Delete(context.Context, ai.ProviderID, ai.AuthOperationOptions) error {
	s.called()
	return nil
}

type staticFactoryPanicModelsStore struct{ calls *int }

func (s staticFactoryPanicModelsStore) Read(context.Context, ai.ProviderID) (ai.ModelsStoreEntry, bool, error) {
	(*s.calls)++
	panic("models store must not be called by a static capability stub")
}

func (s staticFactoryPanicModelsStore) Write(context.Context, ai.ProviderID, ai.ModelsStoreEntry) error {
	(*s.calls)++
	panic("models store must not be called by a static capability stub")
}

func (s staticFactoryPanicModelsStore) Delete(context.Context, ai.ProviderID) error {
	(*s.calls)++
	panic("models store must not be called by a static capability stub")
}

type staticFactoryPanicSettingsStorage struct{ calls *int }

func (s staticFactoryPanicSettingsStorage) WithLock(codingagent.SettingsScope, func(*string) *string) {
	(*s.calls)++
	panic("settings storage must not be called by a static capability stub")
}

func TestStaticFactoryProjectionsAreInertCapabilityStubs(t *testing.T) {
	canceled, cancel := context.WithCancel(context.Background())
	cancel()

	invalidPath := "invalid\x00path"
	callbackCalls := 0
	dependencyCalls := 0
	transportCalls := 0
	remoteClient, err := client.NewClient(client.ClientOptions{
		TransportFactory: func(context.Context, client.ByteTransportHandlers) (client.ByteTransport, error) {
			transportCalls++
			panic("transport factory must not be called by a static capability stub")
		},
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	trusted := false
	refreshOnCreate := true
	tests := []struct {
		operation string
		call      func() (any, error)
	}{
		{"OpenRemoteSession", func() (any, error) {
			return codingagent.OpenRemoteSession(canceled, remoteClient, "session-1", codingagent.RemoteSessionOptions{
				OnListenerError: func(error) { callbackCalls++ },
			})
		}},
		{"CreateRemoteSession", func() (any, error) {
			return codingagent.CreateRemoteSession(canceled, remoteClient, codingagent.CreateRemoteSessionOptions{CWD: invalidPath}, codingagent.RemoteSessionOptions{
				OnListenerError: func(error) { callbackCalls++ },
			})
		}},
		{"NewKeybindingsManager", func() (any, error) {
			return codingagent.NewKeybindingsManager(invalidPath)
		}},
		{"NewModelRuntime", func() (any, error) {
			return codingagent.NewModelRuntime(canceled, codingagent.CreateModelRuntimeOptions{
				AuthPath:        invalidPath,
				ModelsPath:      ai.Some(invalidPath),
				Credentials:     staticFactoryPanicCredentialStore{calls: &dependencyCalls},
				ModelsStore:     staticFactoryPanicModelsStore{calls: &dependencyCalls},
				RefreshOnCreate: &refreshOnCreate,
			})
		}},
		{"NewSessionManager", func() (any, error) {
			return codingagent.NewSessionManager(invalidPath, &invalidPath, codingagent.NewSessionOptions{ID: "session-1"})
		}},
		{"OpenSessionManager", func() (any, error) {
			return codingagent.OpenSessionManager(invalidPath, &invalidPath, &invalidPath)
		}},
		{"ContinueRecentSessionManager", func() (any, error) {
			return codingagent.ContinueRecentSessionManager(invalidPath, &invalidPath)
		}},
		{"ForkSessionManager", func() (any, error) {
			return codingagent.ForkSessionManager(invalidPath, invalidPath, &invalidPath, codingagent.NewSessionOptions{ID: "fork-1"})
		}},
		{"ListSessions", func() (any, error) {
			return codingagent.ListSessions(canceled, invalidPath, codingagent.SessionListOptions{
				SessionDir: &invalidPath,
				OnProgress: func(int, int) { callbackCalls++ },
			})
		}},
		{"ListAllSessions", func() (any, error) {
			return codingagent.ListAllSessions(canceled, codingagent.SessionListOptions{
				SessionDir: &invalidPath,
				OnProgress: func(int, int) { callbackCalls++ },
			})
		}},
		{"NewSettingsManager", func() (any, error) {
			return codingagent.NewSettingsManager(invalidPath, &invalidPath, codingagent.SettingsManagerCreateOptions{ProjectTrusted: &trusted})
		}},
		{"NewSettingsManagerFromStorage", func() (any, error) {
			return codingagent.NewSettingsManagerFromStorage(staticFactoryPanicSettingsStorage{calls: &dependencyCalls}, codingagent.SettingsManagerCreateOptions{ProjectTrusted: &trusted})
		}},
		{"NewInMemorySettingsManager", func() (any, error) {
			return codingagent.NewInMemorySettingsManager(codingagent.Settings{QuietStartup: &trusted}, codingagent.SettingsManagerCreateOptions{ProjectTrusted: &trusted})
		}},
	}

	for _, test := range tests {
		t.Run(test.operation, func(t *testing.T) {
			result, err := test.call()
			if !isNilStaticFactoryResult(result) {
				t.Fatalf("result = %#v, want exact zero value", result)
			}
			assertStaticFactoryNotImplemented(t, err, test.operation)
		})
	}

	if callbackCalls != 0 || dependencyCalls != 0 || transportCalls != 0 {
		t.Fatalf("stub side effects = callbacks %d, dependencies %d, transports %d; want all zero", callbackCalls, dependencyCalls, transportCalls)
	}
}

func TestNewInMemorySessionManagerRemainsFunctional(t *testing.T) {
	manager := codingagent.NewInMemorySessionManager("/project", codingagent.NewSessionOptions{ID: "session-1"})
	if manager == nil {
		t.Fatal("NewInMemorySessionManager() = nil, want functional in-memory manager")
	}
	if manager.GetCWD() != "/project" || manager.GetSessionID() != "session-1" || manager.IsPersisted() {
		t.Fatalf("manager state = cwd %q id %q persisted %t", manager.GetCWD(), manager.GetSessionID(), manager.IsPersisted())
	}
}

func TestStaticFactorySupportingCarriersPreserveAbsence(t *testing.T) {
	if codingagent.SettingsScopeGlobal != "global" || codingagent.SettingsScopeProject != "project" {
		t.Fatalf("settings scopes = (%q, %q), want (global, project)", codingagent.SettingsScopeGlobal, codingagent.SettingsScopeProject)
	}
	if (codingagent.SettingsManagerCreateOptions{}).ProjectTrusted != nil {
		t.Fatal("zero SettingsManagerCreateOptions.ProjectTrusted is present")
	}
	if (codingagent.SessionListOptions{}).SessionDir != nil || (codingagent.SessionListOptions{}).OnProgress != nil {
		t.Fatalf("zero SessionListOptions = %#v, want absent directory and progress callback", codingagent.SessionListOptions{})
	}
}

func isNilStaticFactoryResult(result any) bool {
	if result == nil {
		return true
	}
	value := reflect.ValueOf(result)
	return (value.Kind() == reflect.Pointer || value.Kind() == reflect.Slice) && value.IsNil()
}

func assertStaticFactoryNotImplemented(t *testing.T, err error, operation string) {
	t.Helper()
	if !errors.Is(err, codingagent.ErrNotImplemented) {
		t.Fatalf("error = %v, want ErrNotImplemented", err)
	}
	var unavailable *codingagent.NotImplementedError
	if !errors.As(err, &unavailable) {
		t.Fatalf("error = %T, want *codingagent.NotImplementedError", err)
	}
	if unavailable.Module != "codingagent" || unavailable.Operation != operation {
		t.Fatalf("NotImplementedError = %#v, want codingagent.%s", unavailable, operation)
	}
}

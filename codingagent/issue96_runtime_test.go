package codingagent_test

import (
	"context"
	"errors"
	"github.com/nankedr/pig/codingagent"
	"testing"
)

func TestIssue96ReplacementFactoryFailureKeepsLiveSession(t *testing.T) {
	ctx := context.Background()
	manager := codingagent.NewInMemorySessionManager(t.TempDir())
	session := codingagent.NewAgentSession(codingagent.AgentSessionConfig{SessionManager: manager})
	failure := errors.New("target assembly failed")
	runtime := codingagent.NewAgentSessionRuntime(session, codingagent.AgentSessionServices{CWD: manager.GetCWD()}, func(context.Context, codingagent.CreateAgentSessionRuntimeOptions) (codingagent.CreateAgentSessionRuntimeResult, error) {
		return codingagent.CreateAgentSessionRuntimeResult{}, failure
	}, nil, nil)
	defer runtime.Dispose(ctx)
	invalidated := false
	runtime.SetBeforeSessionInvalidate(func() { invalidated = true })
	if _, err := runtime.NewSession(ctx); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	if runtime.Session() != session || invalidated {
		t.Fatal("failed factory invalidated original Session")
	}
	stop, err := session.Subscribe(func(codingagent.AgentSessionEvent) {})
	if err != nil {
		t.Fatal("old Session disposed after failed replacement", err)
	}
	stop()
}

func TestIssue96ReplacementCancellationDisposesOnlyStagedSession(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	manager := codingagent.NewInMemorySessionManager(t.TempDir())
	old := codingagent.NewAgentSession(codingagent.AgentSessionConfig{SessionManager: manager})
	var staged *codingagent.AgentSession
	runtime := codingagent.NewAgentSessionRuntime(old, codingagent.AgentSessionServices{CWD: manager.GetCWD()}, func(_ context.Context, options codingagent.CreateAgentSessionRuntimeOptions) (codingagent.CreateAgentSessionRuntimeResult, error) {
		staged = codingagent.NewAgentSession(codingagent.AgentSessionConfig{SessionManager: options.SessionManager})
		cancel()
		return codingagent.CreateAgentSessionRuntimeResult{CreateAgentSessionResult: codingagent.CreateAgentSessionResult{Session: staged}}, nil
	}, nil, nil)
	defer runtime.Dispose(context.Background())
	if _, err := runtime.NewSession(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if runtime.Session() != old {
		t.Fatal("cancel committed replacement")
	}
	stop, err := old.Subscribe(func(codingagent.AgentSessionEvent) {})
	if err != nil {
		t.Fatal(err)
	}
	stop()
	if _, err := staged.Subscribe(func(codingagent.AgentSessionEvent) {}); err == nil {
		t.Fatal("staged Session leaked")
	}
}

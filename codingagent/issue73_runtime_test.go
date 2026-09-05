package codingagent_test

import (
	"context"
	"errors"
	"os"
	"reflect"
	"testing"

	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
)

func TestSessionRuntimeReplacementLifecycle(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	manager, err := codingagent.NewSessionManager(dir, &dir)
	if err != nil {
		t.Fatal(err)
	}
	provider, err := ai.NewFauxProvider()
	if err != nil {
		t.Fatal(err)
	}
	model, _ := provider.GetModel()
	reply, _ := ai.FauxAssistantMessage(ai.FauxAssistantText("reply"))
	provider.SetResponses([]ai.FauxResponseStep{reply, reply, reply})
	phases := []string{}
	var start *codingagent.SessionStartEvent
	factory := func(ctx context.Context, o codingagent.CreateAgentSessionRuntimeOptions) (codingagent.CreateAgentSessionRuntimeResult, error) {
		phases = append(phases, "create")
		start = o.SessionStartEvent
		created, err := codingagent.CreateAgentSession(ctx, codingagent.CreateAgentSessionOptions{CWD: o.CWD, Model: &model, Provider: provider.Provider, SessionManager: o.SessionManager, SessionStartEvent: o.SessionStartEvent})
		return codingagent.CreateAgentSessionRuntimeResult{CreateAgentSessionResult: created, Services: codingagent.AgentSessionServices{CWD: o.CWD}}, err
	}
	runtime, err := codingagent.CreateAgentSessionRuntime(ctx, factory, codingagent.CreateAgentSessionRuntimeOptions{CWD: dir, SessionManager: manager})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Dispose(ctx)
	old := runtime.Session()
	if err := old.Prompt(ctx, "first"); err != nil {
		t.Fatal(err)
	}
	source := *old.SessionFile()
	before, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	users, err := old.GetUserMessagesForForking()
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 1 {
		t.Fatal(users)
	}
	phases = nil
	runtime.SetBeforeSessionInvalidate(func() { phases = append(phases, "invalidate") })
	unreg, err := ai.RegisterSessionResourceCleanup(func(ids ...string) {
		if len(ids) == 1 && ids[0] == old.SessionID() {
			phases = append(phases, "cleanup")
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	defer unreg()
	runtime.SetRebindSession(func(s *codingagent.AgentSession) error {
		phases = append(phases, "rebind")
		if runtime.Session() != s {
			t.Fatal("rebind observed stale runtime Session")
		}
		if s == old {
			t.Fatal("rebound old session")
		}
		return nil
	})
	result, err := runtime.Fork(ctx, users[0].EntryID)
	if err != nil || result.Cancelled || result.SelectedText == nil || *result.SelectedText != "first" {
		t.Fatalf("fork=%+v err=%v", result, err)
	}
	if !reflect.DeepEqual(phases, []string{"invalidate", "cleanup", "create", "rebind"}) {
		t.Fatal(phases)
	}
	if start.Reason != "fork" || start.PreviousSessionFile != source {
		t.Fatal(start)
	}
	if err := old.Prompt(ctx, "stale"); err == nil {
		t.Fatal("old session remained usable")
	}
	if err := runtime.Session().Prompt(ctx, "fork prompt"); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(source)
	if err != nil || string(before) != string(after) {
		t.Fatal("fork changed source", err)
	}
	if _, err := runtime.SwitchSession(ctx, source); err != nil {
		t.Fatal(err)
	}
	if len(runtime.Session().Messages()) != 2 {
		t.Fatal("switch lost history")
	}
	active := runtime.Session()
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := runtime.NewSession(cancelled); !errors.Is(err, context.Canceled) || runtime.Session() != active {
		t.Fatalf("cancel replaced session: %v", err)
	}
	if _, err := runtime.Fork(ctx, "missing"); err == nil || runtime.Session() != active {
		t.Fatal("invalid fork replaced session")
	}
	if _, err := runtime.NewSession(ctx); err != nil {
		t.Fatal(err)
	}
	if len(runtime.Session().Messages()) != 0 {
		t.Fatal("new session kept old history")
	}
	failure := errors.New("rebind failed")
	var failed *codingagent.AgentSession
	runtime.SetRebindSession(func(s *codingagent.AgentSession) error { failed = s; return failure })
	if _, err := runtime.NewSession(ctx); !errors.Is(err, failure) {
		t.Fatalf("rebind error=%v", err)
	}
	if failed == nil {
		t.Fatal("rebind was not called")
	}
	if err := failed.Prompt(ctx, "leak"); err == nil {
		t.Fatal("failed replacement left live session")
	}
}

func TestSessionRuntimeDisposeCleansResourcesOnce(t *testing.T) {
	manager := codingagent.NewInMemorySessionManager(t.TempDir())
	session := codingagent.NewAgentSession(codingagent.AgentSessionConfig{SessionManager: manager})
	runtime := codingagent.NewAgentSessionRuntime(session, codingagent.AgentSessionServices{}, nil, nil, nil)
	cleanups, invalidations := 0, 0
	unregister, err := ai.RegisterSessionResourceCleanup(func(ids ...string) {
		if len(ids) == 1 && ids[0] == manager.GetSessionID() {
			cleanups++
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	defer unregister()
	runtime.SetBeforeSessionInvalidate(func() { invalidations++ })
	if err := runtime.Dispose(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Dispose(context.Background()); err != nil {
		t.Fatal(err)
	}
	if cleanups != 1 || invalidations != 1 {
		t.Fatalf("cleanup=%d invalidate=%d", cleanups, invalidations)
	}
}

func TestSessionRuntimeMissingManagerReturnsError(t *testing.T) {
	runtime, err := codingagent.CreateAgentSessionRuntime(context.Background(), func(context.Context, codingagent.CreateAgentSessionRuntimeOptions) (codingagent.CreateAgentSessionRuntimeResult, error) {
		return codingagent.CreateAgentSessionRuntimeResult{CreateAgentSessionResult: codingagent.CreateAgentSessionResult{Session: codingagent.NewAgentSession(codingagent.AgentSessionConfig{})}}, nil
	}, codingagent.CreateAgentSessionRuntimeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Dispose(context.Background())
	if _, err := runtime.NewSession(context.Background()); err == nil {
		t.Fatal("new without manager succeeded")
	}
	if _, err := runtime.Fork(context.Background(), "entry"); err == nil {
		t.Fatal("fork without manager succeeded")
	}
}

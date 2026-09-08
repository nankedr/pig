package codingagent_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
)

func tree91Session(t *testing.T) (*codingagent.AgentSession, *ai.FauxCore, string) {
	t.Helper()
	dir := t.TempDir()
	manager, err := codingagent.NewSessionManager(dir, &dir)
	if err != nil {
		t.Fatal(err)
	}
	root, err := manager.AppendMessage(ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText("original"), Timestamp: 1})
	if err != nil {
		t.Fatal(err)
	}
	core, err := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	model, ok := core.GetModel()
	if !ok {
		t.Fatal("missing faux model")
	}
	reply, err := ai.FauxAssistantMessage(ai.FauxAssistantText("old answer"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = manager.AppendMessage(reply); err != nil {
		t.Fatal(err)
	}
	created, err := codingagent.CreateAgentSession(context.Background(), codingagent.CreateAgentSessionOptions{CWD: dir, SessionManager: manager, Model: &model, StreamFunction: agent.StreamFunction(core.StreamSimple), Tools: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = created.Session.Dispose() })
	return created.Session, core, root
}

func tree91Snapshot(t *testing.T, s *codingagent.AgentSession) string {
	t.Helper()
	data, err := json.Marshal([]any{s.State(), s.SessionManager().GetEntries(), s.SessionManager().GetLeafID()})
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestSessionTreeNavigationRootContinuationAndLifecycle(t *testing.T) {
	for _, label := range []string{"", "restart"} {
		t.Run("label="+label, func(t *testing.T) {
			s, core, root := tree91Session(t)
			manager := s.SessionManager()
			agentBefore, loader, runtime := s.Agent(), s.ResourceLoader(), s.ModelRuntime()
			path := *s.SessionFile()
			id := s.SessionID()
			oldLeaf := *manager.GetLeafID()
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			events := 0
			unsubscribe, err := s.Subscribe(func(e codingagent.AgentSessionEvent) {
				if e.AgentSessionEventType() == codingagent.AgentSessionEventTypeMessageEnd {
					events++
				}
			})
			if err != nil {
				t.Fatal(err)
			}
			r, err := s.NavigateTree(context.Background(), root, codingagent.NavigateTreeOptions{Label: label})
			if err != nil {
				t.Fatal(err)
			}
			if r.EditorText == nil || *r.EditorText != "original" || len(s.Messages()) != 0 || r.Cancelled || r.Aborted || r.SummaryEntry != nil {
				t.Fatalf("navigation: %+v %+v", r, s.Messages())
			}
			if label == "" {
				if manager.GetLeafID() != nil {
					t.Fatal("root must have nil leaf")
				}
				after, err := os.ReadFile(path)
				if err != nil || string(after) != string(before) {
					t.Fatal("unlabelled navigation wrote file", err)
				}
			} else {
				leaf := manager.GetLeafEntry()
				if leaf.Type != "label" || leaf.ParentID != nil || leaf.TargetID != root || *manager.GetLabel(root) != label {
					t.Fatalf("root label: %+v", leaf)
				}
			}
			if events != 0 || s.Agent() != agentBefore || s.ResourceLoader() != loader || s.ModelRuntime() != runtime || s.SessionManager() != manager || s.SessionID() != id || *s.SessionFile() != path {
				t.Fatal("navigation replaced resources, identity or emitted lifecycle events")
			}
			parent := manager.GetLeafID()
			core.SetResponses([]ai.FauxResponseStep{ai.FauxResponseFactory(func(c ai.Context, _ *ai.SimpleStreamOptions, _ *ai.FauxProviderState, _ ai.Model) (ai.AssistantMessage, error) {
				if len(c.Messages) != 1 {
					t.Errorf("abandoned context leaked: %+v", c.Messages)
				}
				return ai.FauxAssistantMessage(ai.FauxAssistantText("new answer"))
			})})
			if err = s.Prompt(context.Background(), "new question"); err != nil {
				t.Fatal(err)
			}
			if events != 2 {
				t.Fatalf("duplicate/lost listeners: %d", events)
			}
			entries := manager.GetEntries()
			user := entries[len(entries)-2]
			if !reflect.DeepEqual(user.ParentID, parent) {
				t.Fatalf("wrong continuation parent: %+v", user)
			}
			unsubscribe()
			reopened, err := codingagent.OpenSessionManager(path, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			if reopened.GetEntry(oldLeaf) == nil || reopened.GetEntry(root) == nil {
				t.Fatal("old branch lost")
			}
			model := s.Model()
			restored, err := codingagent.CreateAgentSession(context.Background(), codingagent.CreateAgentSessionOptions{CWD: manager.GetCWD(), SessionManager: reopened, Model: &model, StreamFunction: agent.StreamFunction(core.StreamSimple), Tools: []string{}})
			if err != nil {
				t.Fatal(err)
			}
			defer restored.Session.Dispose()
			if got := tree91Messages(restored.Session.Messages()); len(got) != 2 || got[0]["text"] != "new question" || got[1]["text"] != "new answer" {
				t.Fatalf("reopened branch: %+v", got)
			}
			if _, err = restored.Session.NavigateTree(context.Background(), oldLeaf); err != nil {
				t.Fatal(err)
			}
			if got := tree91Messages(restored.Session.Messages()); len(got) != 2 || got[0]["text"] != "original" {
				t.Fatalf("original branch: %+v", got)
			}
			if err := s.Prompt(context.Background(), "again"); err != nil {
				t.Fatal(err)
			}
			if events != 2 {
				t.Fatal("unsubscribe lost across navigation")
			}
		})
	}
}

func TestSessionTreeNavigationFailuresAreAtomic(t *testing.T) {
	s, _, root := tree91Session(t)
	before := tree91Snapshot(t, s)
	path := *s.SessionFile()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, tc := range []struct {
		name    string
		ctx     context.Context
		target  string
		options []codingagent.NavigateTreeOptions
		want    error
	}{
		{name: "cancelled", ctx: ctx, target: root, want: context.Canceled},
		{name: "invalid", ctx: context.Background(), target: "missing"},
		{name: "empty", ctx: context.Background()},
		{name: "nil context", target: root},
		{name: "multiple options", ctx: context.Background(), target: root, options: []codingagent.NavigateTreeOptions{{}, {}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, err := s.NavigateTree(tc.ctx, tc.target, tc.options...)
			if err == nil || (tc.want != nil && !errors.Is(err, tc.want)) || !reflect.DeepEqual(r, codingagent.NavigateTreeResult{}) {
				t.Fatalf("result: %+v %v", r, err)
			}
			if tree91Snapshot(t, s) != before {
				t.Fatal("failed navigation changed state")
			}
			after, err := os.ReadFile(path)
			if err != nil || string(after) != string(data) {
				t.Fatal("failed navigation wrote file", err)
			}
		})
	}
	if err := os.Rename(path, path+".saved"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := s.NavigateTree(context.Background(), root, codingagent.NavigateTreeOptions{Label: "must rollback"}); err == nil {
		t.Fatal("directory replacement succeeded")
	}
	if tree91Snapshot(t, s) != before {
		t.Fatal("rename failure left half switch")
	}
	staged, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".session-tree-*"))
	if err != nil || len(staged) != 0 {
		t.Fatalf("staged files leaked: %v %v", staged, err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path+".saved", path); err != nil {
		t.Fatal(err)
	}
	if _, err := s.NavigateTree(context.Background(), root, codingagent.NavigateTreeOptions{Label: "retry"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Dispose(); err != nil {
		t.Fatal(err)
	}
	before = tree91Snapshot(t, s)
	if _, err := s.NavigateTree(context.Background(), root); err == nil {
		t.Fatal("disposed navigation succeeded")
	}
	if tree91Snapshot(t, s) != before {
		t.Fatal("disposed navigation mutated state")
	}
}

func TestSessionTreeNavigationBusyAndReentrantListeners(t *testing.T) {
	s, core, root := tree91Session(t)
	entered, release := make(chan struct{}), make(chan struct{})
	core.SetResponses([]ai.FauxResponseStep{ai.FauxResponseFactory(func(ai.Context, *ai.SimpleStreamOptions, *ai.FauxProviderState, ai.Model) (ai.AssistantMessage, error) {
		close(entered)
		<-release
		return ai.FauxAssistantMessage(ai.FauxAssistantText("answer"))
	})})
	_, err := s.Subscribe(func(e codingagent.AgentSessionEvent) {
		if e.AgentSessionEventType() == codingagent.AgentSessionEventTypeMessageEnd {
			if _, err := s.NavigateTree(context.Background(), root); err == nil || !strings.Contains(err.Error(), "busy") {
				t.Errorf("reentrant navigation: %v", err)
			}
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- s.Prompt(context.Background(), "running") }()
	<-entered
	before := tree91Snapshot(t, s)
	_, err = s.NavigateTree(context.Background(), root)
	if err == nil || !strings.Contains(err.Error(), "busy") {
		t.Errorf("busy navigation: %v", err)
	}
	if tree91Snapshot(t, s) != before {
		t.Error("busy navigation mutated state")
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if _, err := s.NavigateTree(context.Background(), root); err != nil {
		t.Fatal(err)
	}
}

func TestSessionTreeNavigationCompactionIntegration(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	s, manager := compact89Session(t, func(context.Context, ai.Model, ai.Context, ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
		close(entered)
		<-release
		return compact89Response("checkpoint", ai.StopReasonStop)
	}, true)
	oldLeaf := *manager.GetLeafID()
	original := tree91Messages(s.Messages())
	done := make(chan error, 1)
	go func() { _, err := s.Compact(context.Background()); done <- err }()
	<-entered
	before := tree91Snapshot(t, s)
	_, err := s.NavigateTree(context.Background(), oldLeaf)
	if err == nil || !strings.Contains(err.Error(), "busy") {
		t.Errorf("navigation during compaction: %v", err)
	}
	if tree91Snapshot(t, s) != before {
		t.Error("busy navigation changed compaction input")
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	compactedLeaf := *manager.GetLeafID()
	compacted := tree91Messages(s.Messages())
	if _, err := s.NavigateTree(context.Background(), oldLeaf); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(tree91Messages(s.Messages()), original) {
		t.Fatal("navigation did not restore the original uncompacted path")
	}
	if _, err := s.NavigateTree(context.Background(), compactedLeaf); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(tree91Messages(s.Messages()), compacted) {
		t.Fatal("navigation did not restore the compacted path")
	}
	reopened, err := codingagent.OpenSessionManager(*s.SessionFile(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.GetEntry(oldLeaf) == nil || reopened.GetEntry(compactedLeaf) == nil {
		t.Fatal("navigation lost a persisted branch")
	}
}

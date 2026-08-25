package agent_test

import (
	"context"
	"errors"
	"testing"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
)

func TestInMemoryHarnessSessionBuildsTheMainLane(t *testing.T) {
	ctx := context.Background()
	repo := agent.NewInMemorySessionRepo()
	session, err := repo.Create(ctx, agent.SessionCreateOptions{ID: "session-1"})
	if err != nil {
		t.Fatal(err)
	}

	rootID, err := session.AppendCustomEntry(ctx, "note", map[string]any{"text": "root"})
	if err != nil {
		t.Fatal(err)
	}
	childID, err := session.AppendCustomEntry(ctx, "note", map[string]any{"text": "child"})
	if err != nil {
		t.Fatal(err)
	}

	leafID, ok, err := session.GetLeafID(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || leafID != childID {
		t.Fatalf("GetLeafID() = (%q, %t), want (%q, true)", leafID, ok, childID)
	}
	entries, err := session.FindEntriesOnBranch(ctx, agent.EntryQuery{Order: agent.EntryOrderOldestFirst}, agent.BranchBounds{})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("FindEntriesOnBranch() returned %d entries, want 2", len(entries))
	}
	root, ok := entries[0].(agent.CustomEntry)
	if !ok || root.ID != rootID || root.ParentIDSet {
		t.Fatalf("root entry = %#v, want id %q with no parent", entries[0], rootID)
	}
	child, ok := entries[1].(agent.CustomEntry)
	if !ok || !child.ParentIDSet || child.ParentID != rootID {
		t.Fatalf("child entry = %#v, want parent %q", entries[1], rootID)
	}
}

func TestJSONLHarnessSessionCreateIsASideEffectFreeCapabilityStub(t *testing.T) {
	repo := agent.NewJSONLSessionRepo(agent.JSONLSessionRepoOptions{SessionsRoot: "/must-not-be-touched"})
	_, err := repo.Create(context.Background(), agent.JSONLSessionCreateOptions{CWD: "/work"})
	if !errors.Is(err, agent.ErrNotImplemented) {
		t.Fatalf("Create() error = %v, want ErrNotImplemented", err)
	}
	var capabilityError *agent.NotImplementedError
	if !errors.As(err, &capabilityError) || capabilityError.Module != "agent" || capabilityError.Operation != "JSONLSessionRepo.Create" {
		t.Fatalf("Create() error = %#v, want agent.JSONLSessionRepo.Create", capabilityError)
	}
}

func TestBuildSessionContextUsesTheLatestCompactionWithoutLosingState(t *testing.T) {
	activeTools := []string{"read", "write"}
	retained := ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText("retained"), Timestamp: 5}
	current := ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText("current"), Timestamp: 8}
	entries := []agent.Entry{
		agent.ModelChangeEntry{Provider: "provider", ModelID: "model"},
		agent.ThinkingLevelEntry{ThinkingLevel: "high"},
		agent.ActiveToolsEntry{ActiveToolNames: activeTools},
		agent.MessageEntry{Message: ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText("old"), Timestamp: 4}},
		agent.CompactionEntry{EntryBase: agent.EntryBase{Timestamp: 6}, Summary: "summary", RetainedTail: []agent.AgentMessage{retained}, TokensBefore: 42},
		agent.MessageEntry{Message: ai.AssistantMessage{Role: ai.MessageRoleAssistant, Provider: "provider", Model: "model", StopReason: ai.StopReasonDeferred, Timestamp: 7}},
		agent.MessageEntry{Message: current},
	}

	context := agent.BuildSessionContext(entries, agent.SessionContextBuildOptions{})
	activeTools[0] = "mutated"
	if context.ThinkingLevel != "high" || context.Model == nil || context.Model.Provider != "provider" || context.Model.ModelID != "model" {
		t.Fatalf("session state = %#v, want high/provider/model", context)
	}
	if len(context.ActiveToolNames) != 2 || context.ActiveToolNames[0] != "read" {
		t.Fatalf("active tools = %#v, want defensive copy of [read write]", context.ActiveToolNames)
	}
	if len(context.Messages) != 3 {
		t.Fatalf("messages = %#v, want summary, retained tail, and current message", context.Messages)
	}
	if summary, ok := context.Messages[0].(agent.CompactionSummaryMessage); !ok || summary.Summary != "summary" || summary.TokensBefore != 42 {
		t.Fatalf("summary message = %#v", context.Messages[0])
	}
	retainedMessage, retainedOK := context.Messages[1].(ai.UserMessage)
	retainedText, _ := retainedMessage.Content.Text()
	currentMessage, currentOK := context.Messages[2].(ai.UserMessage)
	currentText, _ := currentMessage.Content.Text()
	if !retainedOK || retainedText != "retained" || !currentOK || currentText != "current" {
		t.Fatalf("messages = %#v, want retained then current", context.Messages)
	}
}

func TestScanningSessionSearchFindsMatchingEntries(t *testing.T) {
	ctx := context.Background()
	repo := agent.NewInMemorySessionRepo()
	session, err := repo.Create(ctx, agent.SessionCreateOptions{ID: "search-session"})
	if err != nil {
		t.Fatal(err)
	}
	entryID, err := session.AppendCustomEntry(ctx, "note", map[string]any{"text": "Find the Needle"})
	if err != nil {
		t.Fatal(err)
	}

	search := agent.CreateScanningSessionSearch(repo)
	hits, err := search.Search(ctx, agent.SessionSearchOptions{Text: " needle "})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].Metadata.ID != "search-session" || hits[0].EntryID != entryID || !hits[0].SnippetSet {
		t.Fatalf("Search() = %#v, want one hit for %q", hits, entryID)
	}
}

func TestScanningSessionSearchRejectsUnsupportedCWDFilter(t *testing.T) {
	search := agent.CreateScanningSessionSearch(agent.NewInMemorySessionRepo())
	hits, err := search.Search(context.Background(), agent.SessionSearchOptions{Text: "needle", CWD: "/work", CWDSet: true})
	if hits != nil || !errors.Is(err, agent.ErrNotImplemented) {
		t.Fatalf("Search(cwd) = %#v, %v, want nil and ErrNotImplemented", hits, err)
	}
	var capability *agent.NotImplementedError
	if !errors.As(err, &capability) || capability.Module != "agent" || capability.Operation != "SessionSearch.Search.cwd" {
		t.Fatalf("Search(cwd) error = %#v", capability)
	}
}

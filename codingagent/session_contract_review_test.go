package codingagent_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
)

// These assignments intentionally make the reviewed public signatures part of
// the compile contract. Optional TypeScript arguments map to Go variadics,
// nullable values map to pointers, and capability stubs add an error result.
var (
	_ func(string, ...codingagent.NewSessionOptions) *codingagent.SessionManager                   = codingagent.NewInMemorySessionManager
	_ func(string, *string, ...codingagent.NewSessionOptions) (*codingagent.SessionManager, error) = codingagent.NewSessionManager
	_ func(string, *string, *string) (*codingagent.SessionManager, error)                          = codingagent.OpenSessionManager

	_ func(*codingagent.SessionManager, string) error                                                                  = (*codingagent.SessionManager).SetSessionFile
	_ func(*codingagent.SessionManager, ...codingagent.NewSessionOptions) (*string, error)                             = (*codingagent.SessionManager).NewSession
	_ func(*codingagent.SessionManager) bool                                                                           = (*codingagent.SessionManager).IsPersisted
	_ func(*codingagent.SessionManager) string                                                                         = (*codingagent.SessionManager).GetCWD
	_ func(*codingagent.SessionManager) string                                                                         = (*codingagent.SessionManager).GetSessionDir
	_ func(*codingagent.SessionManager) bool                                                                           = (*codingagent.SessionManager).UsesDefaultSessionDir
	_ func(*codingagent.SessionManager) string                                                                         = (*codingagent.SessionManager).GetSessionID
	_ func(*codingagent.SessionManager) *string                                                                        = (*codingagent.SessionManager).GetSessionFile
	_ func(*codingagent.SessionManager, agent.AgentMessage) (string, error)                                            = (*codingagent.SessionManager).AppendMessage
	_ func(*codingagent.SessionManager, string) (string, error)                                                        = (*codingagent.SessionManager).AppendThinkingLevelChange
	_ func(*codingagent.SessionManager, string, string) (string, error)                                                = (*codingagent.SessionManager).AppendModelChange
	_ func(*codingagent.SessionManager, string, string, int64, ...codingagent.AppendCompactionOptions) (string, error) = (*codingagent.SessionManager).AppendCompaction
	_ func(*codingagent.SessionManager, string, ...any) (string, error)                                                = (*codingagent.SessionManager).AppendCustomEntry
	_ func(*codingagent.SessionManager, string) (string, error)                                                        = (*codingagent.SessionManager).AppendSessionInfo
	_ func(*codingagent.SessionManager) *string                                                                        = (*codingagent.SessionManager).GetSessionName
	_ func(*codingagent.SessionManager, string, ai.UserMessageContent, bool, ...json.RawMessage) (string, error)       = (*codingagent.SessionManager).AppendCustomMessageEntry
	_ func(*codingagent.SessionManager) *string                                                                        = (*codingagent.SessionManager).GetLeafID
	_ func(*codingagent.SessionManager) *codingagent.SessionEntry                                                      = (*codingagent.SessionManager).GetLeafEntry
	_ func(*codingagent.SessionManager, string) *codingagent.SessionEntry                                              = (*codingagent.SessionManager).GetEntry
	_ func(*codingagent.SessionManager, string) ([]codingagent.SessionEntry, error)                                    = (*codingagent.SessionManager).GetChildren
	_ func(*codingagent.SessionManager, string) *string                                                                = (*codingagent.SessionManager).GetLabel
	_ func(*codingagent.SessionManager, string, *string) (string, error)                                               = (*codingagent.SessionManager).AppendLabelChange
	_ func(*codingagent.SessionManager, ...string) []codingagent.SessionEntry                                          = (*codingagent.SessionManager).GetBranch
	_ func(*codingagent.SessionManager) []codingagent.SessionEntry                                                     = (*codingagent.SessionManager).BuildContextEntries
	_ func(*codingagent.SessionManager) codingagent.SessionContext                                                     = (*codingagent.SessionManager).BuildSessionContext
	_ func(*codingagent.SessionManager) *codingagent.SessionHeader                                                     = (*codingagent.SessionManager).GetHeader
	_ func(*codingagent.SessionManager) []codingagent.SessionEntry                                                     = (*codingagent.SessionManager).GetEntries
	_ func(*codingagent.SessionManager) ([]codingagent.SessionTreeNode, error)                                         = (*codingagent.SessionManager).GetTree
	_ func(*codingagent.SessionManager, string) error                                                                  = (*codingagent.SessionManager).Branch
	_ func(*codingagent.SessionManager) error                                                                          = (*codingagent.SessionManager).ResetLeaf
	_ func(*codingagent.SessionManager, *string, string, ...codingagent.BranchSummaryOptions) (string, error)          = (*codingagent.SessionManager).BranchWithSummary
	_ func(*codingagent.SessionManager, string) (*string, error)                                                       = (*codingagent.SessionManager).CreateBranchedSession
)

func TestAgentSessionEventDiscriminatorsAndLegacyComposition(t *testing.T) {
	legacyMessage := ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText("hello"), Timestamp: 1}
	legacyAssistant := ai.AssistantMessage{Role: ai.MessageRoleAssistant}
	toolResult := agent.ErasedAgentToolResult{Content: []ai.ToolResultContent{}, Details: map[string]any{}}
	branchSource := codingagent.SummarizationRetrySourceBranchSummary

	tests := []struct {
		name  string
		event codingagent.AgentSessionEvent
		want  codingagent.AgentSessionEventType
	}{
		{"agent start", codingagent.AgentSessionAgentStartEvent{AgentStartEvent: agent.AgentStartEvent{Type: agent.AgentEventTypeAgentStart}}, codingagent.AgentSessionEventTypeAgentStart},
		{"agent end", codingagent.AgentSessionAgentEndEvent{AgentEndEvent: agent.AgentEndEvent{Type: agent.AgentEventTypeAgentEnd}, WillRetry: true}, codingagent.AgentSessionEventTypeAgentEnd},
		{"turn start", codingagent.AgentSessionTurnStartEvent{TurnStartEvent: agent.TurnStartEvent{Type: agent.AgentEventTypeTurnStart}}, codingagent.AgentSessionEventTypeTurnStart},
		{"turn end", codingagent.AgentSessionTurnEndEvent{TurnEndEvent: agent.TurnEndEvent{Type: agent.AgentEventTypeTurnEnd, Message: legacyMessage}}, codingagent.AgentSessionEventTypeTurnEnd},
		{"message start", codingagent.AgentSessionMessageStartEvent{MessageStartEvent: agent.MessageStartEvent{Type: agent.AgentEventTypeMessageStart, Message: legacyMessage}}, codingagent.AgentSessionEventTypeMessageStart},
		{"message update", codingagent.AgentSessionMessageUpdateEvent{MessageUpdateEvent: agent.MessageUpdateEvent{Type: agent.AgentEventTypeMessageUpdate, Message: legacyAssistant, AssistantMessageEvent: ai.AssistantMessageTextDeltaEvent{Type: ai.AssistantMessageEventTypeTextDelta, Delta: "x", Partial: legacyAssistant}}}, codingagent.AgentSessionEventTypeMessageUpdate},
		{"message end", codingagent.AgentSessionMessageEndEvent{MessageEndEvent: agent.MessageEndEvent{Type: agent.AgentEventTypeMessageEnd, Message: legacyMessage}}, codingagent.AgentSessionEventTypeMessageEnd},
		{"tool start", codingagent.AgentSessionToolExecutionStartEvent{ToolExecutionStartEvent: agent.ToolExecutionStartEvent{Type: agent.AgentEventTypeToolExecutionStart}}, codingagent.AgentSessionEventTypeToolExecutionStart},
		{"tool update", codingagent.AgentSessionToolExecutionUpdateEvent{ToolExecutionUpdateEvent: agent.ToolExecutionUpdateEvent{Type: agent.AgentEventTypeToolExecutionUpdate, PartialResult: toolResult}}, codingagent.AgentSessionEventTypeToolExecutionUpdate},
		{"tool end", codingagent.AgentSessionToolExecutionEndEvent{ToolExecutionEndEvent: agent.ToolExecutionEndEvent{Type: agent.AgentEventTypeToolExecutionEnd, Result: toolResult}}, codingagent.AgentSessionEventTypeToolExecutionEnd},
		{"agent settled", codingagent.AgentSessionAgentSettledEvent{Type: codingagent.AgentSessionEventTypeAgentSettled}, codingagent.AgentSessionEventTypeAgentSettled},
		{"queue update", codingagent.AgentSessionQueueUpdateEvent{Type: codingagent.AgentSessionEventTypeQueueUpdate}, codingagent.AgentSessionEventTypeQueueUpdate},
		{"compaction start", codingagent.AgentSessionCompactionStartEvent{Type: codingagent.AgentSessionEventTypeCompactionStart}, codingagent.AgentSessionEventTypeCompactionStart},
		{"compaction end", codingagent.AgentSessionCompactionEndEvent{Type: codingagent.AgentSessionEventTypeCompactionEnd}, codingagent.AgentSessionEventTypeCompactionEnd},
		{"entry appended", codingagent.AgentSessionEntryAppendedEvent{Type: codingagent.AgentSessionEventTypeEntryAppended}, codingagent.AgentSessionEventTypeEntryAppended},
		{"session info changed", codingagent.AgentSessionInfoChangedEvent{Type: codingagent.AgentSessionEventTypeSessionInfoChanged}, codingagent.AgentSessionEventTypeSessionInfoChanged},
		{"thinking changed", codingagent.AgentSessionThinkingLevelChangedEvent{Type: codingagent.AgentSessionEventTypeThinkingLevelChanged}, codingagent.AgentSessionEventTypeThinkingLevelChanged},
		{"retry start", codingagent.AgentSessionAutoRetryStartEvent{Type: codingagent.AgentSessionEventTypeAutoRetryStart}, codingagent.AgentSessionEventTypeAutoRetryStart},
		{"retry end", codingagent.AgentSessionAutoRetryEndEvent{Type: codingagent.AgentSessionEventTypeAutoRetryEnd}, codingagent.AgentSessionEventTypeAutoRetryEnd},
		{"summarization scheduled", codingagent.AgentSessionSummarizationRetryScheduledEvent{Type: codingagent.AgentSessionEventTypeSummarizationRetryScheduled}, codingagent.AgentSessionEventTypeSummarizationRetryScheduled},
		{"summarization attempt", codingagent.AgentSessionBranchSummaryRetryAttemptStartEvent{Type: codingagent.AgentSessionEventTypeSummarizationRetryAttemptStart, Source: branchSource}, codingagent.AgentSessionEventTypeSummarizationRetryAttemptStart},
		{"summarization finished", codingagent.AgentSessionSummarizationRetryFinishedEvent{Type: codingagent.AgentSessionEventTypeSummarizationRetryFinished}, codingagent.AgentSessionEventTypeSummarizationRetryFinished},
		{"bash update", codingagent.AgentSessionBashExecutionUpdateEvent{Type: codingagent.AgentSessionEventTypeBashExecutionUpdate}, codingagent.AgentSessionEventTypeBashExecutionUpdate},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.event.AgentSessionEventType(); got != test.want {
				t.Fatalf("AgentSessionEventType() = %q, want %q", got, test.want)
			}
		})
	}

	var lower agent.AgentEvent = codingagent.AgentSessionMessageStartEvent{
		MessageStartEvent: agent.MessageStartEvent{Type: agent.AgentEventTypeMessageStart, Message: legacyMessage},
	}
	if lower.AgentEventType() != agent.AgentEventTypeMessageStart {
		t.Fatalf("composed legacy event type = %q", lower.AgentEventType())
	}
}

func TestNewInMemorySessionManagerBuildsCompleteUniqueV3Headers(t *testing.T) {
	before := time.Now().Add(-time.Second)
	first := codingagent.NewInMemorySessionManager("/project")
	second := codingagent.NewInMemorySessionManager("/project")
	after := time.Now().Add(time.Second)

	if first.GetSessionID() == "" || second.GetSessionID() == "" || first.GetSessionID() == second.GetSessionID() {
		t.Fatalf("generated IDs = (%q, %q), want distinct non-empty IDs", first.GetSessionID(), second.GetSessionID())
	}
	header := first.GetHeader()
	if header == nil || header.Version == nil || *header.Version != codingagent.CurrentSessionVersion || header.ID != first.GetSessionID() {
		t.Fatalf("header = %#v, want complete v3 header for %q", header, first.GetSessionID())
	}
	timestamp, err := time.Parse(time.RFC3339Nano, header.Timestamp)
	if err != nil || timestamp.Before(before) || timestamp.After(after) {
		t.Fatalf("header timestamp = %q (%v), want current RFC3339 timestamp", header.Timestamp, err)
	}
	if first.GetSessionFile() != nil || first.IsPersisted() || first.GetLeafID() != nil {
		t.Fatalf("in-memory persistence state = file %v persisted %t leaf %v", first.GetSessionFile(), first.IsPersisted(), first.GetLeafID())
	}
	if got := first.GetEntries(); got == nil || len(got) != 0 {
		t.Fatalf("GetEntries() = %#v, want non-nil empty slice", got)
	}
	if got, err := first.GetTree(); got != nil || !errors.Is(err, codingagent.ErrNotImplemented) {
		t.Fatalf("GetTree() = (%#v, %v), want nil and ErrNotImplemented", got, err)
	}
	if got := first.GetBranch(); got == nil || len(got) != 0 {
		t.Fatalf("GetBranch() = %#v, want non-nil empty slice", got)
	}
}

func TestSessionManagerGetSessionNameReturnsNormalizedDefensiveCopy(t *testing.T) {
	manager := codingagent.NewInMemorySessionManager("/project")
	entries := manager.GetEntries()
	if entries == nil {
		t.Fatal("GetEntries returned nil")
	}
	if name := manager.GetSessionName(); name != nil {
		t.Fatalf("initial name = %q, want nil", *name)
	}

	// The stub must not fabricate a name or mutate the in-memory session. The
	// defensive-copy case itself is exercised by a parsed manager fixture below.
	before := manager.GetHeader()
	if id, err := manager.AppendSessionInfo("  title  "); !errors.Is(err, codingagent.ErrNotImplemented) || id != "" {
		t.Fatalf("AppendSessionInfo = (%q, %v), want empty ID and ErrNotImplemented", id, err)
	}
	if !reflect.DeepEqual(manager.GetHeader(), before) || manager.GetSessionName() != nil {
		t.Fatal("AppendSessionInfo stub changed manager state")
	}
}

func TestSessionManagerMutationStubsReturnNoIDsAndHaveNoSideEffects(t *testing.T) {
	manager := codingagent.NewInMemorySessionManager("/project", codingagent.NewSessionOptions{ID: "session-1"})
	beforeHeader := manager.GetHeader()
	beforeEntries := manager.GetEntries()
	name := "label"
	root := (*string)(nil)

	tests := []struct {
		name string
		call func() (string, error)
	}{
		{"append message", func() (string, error) { return manager.AppendMessage(ai.UserMessage{}) }},
		{"append thinking", func() (string, error) { return manager.AppendThinkingLevelChange("high") }},
		{"append model", func() (string, error) { return manager.AppendModelChange("provider", "model") }},
		{"append compaction", func() (string, error) { return manager.AppendCompaction("summary", "entry", 1) }},
		{"append custom", func() (string, error) { return manager.AppendCustomEntry("kind") }},
		{"append custom message", func() (string, error) { return manager.AppendCustomMessageEntry("kind", ai.UserText("text"), true) }},
		{"append label", func() (string, error) { return manager.AppendLabelChange("entry", &name) }},
		{"append info", func() (string, error) { return manager.AppendSessionInfo("name") }},
		{"branch summary from root", func() (string, error) { return manager.BranchWithSummary(root, "summary") }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			id, err := test.call()
			if id != "" || !errors.Is(err, codingagent.ErrNotImplemented) {
				t.Fatalf("result = (%q, %v), want empty ID and ErrNotImplemented", id, err)
			}
		})
	}

	if err := manager.ResetLeaf(); !errors.Is(err, codingagent.ErrNotImplemented) {
		t.Fatalf("ResetLeaf error = %v, want ErrNotImplemented", err)
	}
	if file, err := manager.NewSession(); file != nil || !errors.Is(err, codingagent.ErrNotImplemented) {
		t.Fatalf("NewSession = (%v, %v), want nil and ErrNotImplemented", file, err)
	}
	if file, err := manager.CreateBranchedSession("entry"); file != nil || !errors.Is(err, codingagent.ErrNotImplemented) {
		t.Fatalf("CreateBranchedSession = (%v, %v), want nil and ErrNotImplemented", file, err)
	}
	if !reflect.DeepEqual(manager.GetHeader(), beforeHeader) || !reflect.DeepEqual(manager.GetEntries(), beforeEntries) || manager.GetLeafID() != nil {
		t.Fatal("unsupported SessionManager operations changed in-memory state")
	}
}

package codingagent_test

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
)

var (
	_ agent.CompactionSettings       = codingagent.CompactionSettings{}
	_ codingagent.CompactionSettings = agent.CompactionSettings{}
)

func TestCompactionPolicyUsesTheSharedAgentPrimitive(t *testing.T) {
	if got, want := codingagent.DefaultCompactionSettings, agent.DefaultCompactionSettings; got != want {
		t.Fatalf("DefaultCompactionSettings = %#v, want %#v", got, want)
	}

	usageCases := []ai.Usage{
		{},
		{Input: 3, Output: 5, CacheRead: 7, CacheWrite: 11},
		{Input: 3, Output: 5, TotalTokens: 17},
	}
	for _, usage := range usageCases {
		if got, want := codingagent.CalculateContextTokens(usage), agent.CalculateContextTokens(usage); got != want {
			t.Errorf("CalculateContextTokens(%#v) = %d, want shared result %d", usage, got, want)
		}
	}

	policyCases := []struct {
		contextTokens int64
		contextWindow int64
		settings      agent.CompactionSettings
	}{
		{contextTokens: 901, contextWindow: 1000, settings: agent.CompactionSettings{Enabled: true, ReserveTokens: 100}},
		{contextTokens: 900, contextWindow: 1000, settings: agent.CompactionSettings{Enabled: true, ReserveTokens: 100}},
		{contextTokens: 901, contextWindow: 1000, settings: agent.CompactionSettings{Enabled: false, ReserveTokens: 100}},
		{contextTokens: 1, contextWindow: 100, settings: agent.CompactionSettings{Enabled: true, ReserveTokens: 200}},
	}
	for _, test := range policyCases {
		got := codingagent.ShouldCompact(test.contextTokens, test.contextWindow, test.settings)
		want := agent.ShouldCompact(test.contextTokens, test.contextWindow, test.settings)
		if got != want {
			t.Errorf("ShouldCompact(%d, %d, %#v) = %v, want shared result %v", test.contextTokens, test.contextWindow, test.settings, got, want)
		}
	}
}

func TestFindCutPoint(t *testing.T) {
	entries := []codingagent.SessionEntry{
		compactionMessageEntry(ai.UserMessage{
			Role:    ai.MessageRoleUser,
			Content: ai.UserText("u"),
		}),
		compactionMessageEntry(ai.AssistantMessage{
			Role: ai.MessageRoleAssistant,
			Content: []ai.AssistantContent{
				ai.TextContent{Type: ai.ContentTypeText, Text: "a"},
			},
		}),
		compactionMessageEntry(ai.ToolResultMessage{
			Role:       ai.MessageRoleToolResult,
			ToolCallID: "call-1",
			ToolName:   "read",
			Content: []ai.ToolResultContent{
				ai.TextContent{Type: ai.ContentTypeText, Text: "123456789012345678901234"},
			},
		}),
		compactionMessageEntry(ai.AssistantMessage{
			Role: ai.MessageRoleAssistant,
			Content: []ai.AssistantContent{
				ai.TextContent{Type: ai.ContentTypeText, Text: "b"},
			},
		}),
	}

	result := codingagent.FindCutPoint(entries, 0, len(entries), 6)

	if result.FirstKeptEntryIndex != 3 || result.TurnStartIndex != 0 || !result.IsSplitTurn {
		t.Fatalf("FindCutPoint() = %#v, want assistant cut point 3 with turn start 0", result)
	}
	if entries[result.FirstKeptEntryIndex].Message.MessageRole() == ai.MessageRoleToolResult {
		t.Fatal("FindCutPoint() selected a tool result")
	}

	t.Run("skips application-defined roles as cut points", func(t *testing.T) {
		unknown, err := agent.NewRawAgentMessage(json.RawMessage(`{"role":"notification","text":"status"}`))
		if err != nil {
			t.Fatalf("NewRawAgentMessage() error = %v", err)
		}
		entries := []codingagent.SessionEntry{
			compactionMessageEntry(ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText("u")}),
			compactionMessageEntry(ai.ToolResultMessage{
				Role:       ai.MessageRoleToolResult,
				ToolCallID: "call-1",
				ToolName:   "read",
				Content: []ai.ToolResultContent{
					ai.TextContent{Type: ai.ContentTypeText, Text: "12345678"},
				},
			}),
			compactionMessageEntry(unknown),
			compactionMessageEntry(ai.AssistantMessage{
				Role: ai.MessageRoleAssistant,
				Content: []ai.AssistantContent{
					ai.TextContent{Type: ai.ContentTypeText, Text: "a"},
				},
			}),
		}

		result := codingagent.FindCutPoint(entries, 0, len(entries), 2)

		if result.FirstKeptEntryIndex != 3 || result.TurnStartIndex != 0 || !result.IsSplitTurn {
			t.Fatalf("FindCutPoint() = %#v, want assistant cut point 3 with turn start 0", result)
		}
	})

	t.Run("accounts for canonical custom message content", func(t *testing.T) {
		tests := []struct {
			name    string
			content any
		}{
			{name: "string", content: "abcdefgh"},
			{name: "user content slice", content: []ai.UserContent{
				ai.TextContent{Type: ai.ContentTypeText, Text: "abcdefgh"},
			}},
			{name: "raw JSON", content: json.RawMessage(`"abcdefgh"`)},
			{name: "typed user message content", content: ai.UserText("abcdefgh")},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				entries := []codingagent.SessionEntry{
					compactionMessageEntry(ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText("u")}),
					compactionMessageEntry(ai.AssistantMessage{
						Role: ai.MessageRoleAssistant,
						Content: []ai.AssistantContent{
							ai.TextContent{Type: ai.ContentTypeText, Text: "a"},
						},
					}),
					{
						SessionEntryBase: codingagent.SessionEntryBase{Type: "custom_message"},
						CustomType:       "notice",
						Content:          test.content,
					},
					compactionMessageEntry(ai.AssistantMessage{
						Role: ai.MessageRoleAssistant,
						Content: []ai.AssistantContent{
							ai.TextContent{Type: ai.ContentTypeText, Text: "b"},
						},
					}),
				}

				result := codingagent.FindCutPoint(entries, 0, len(entries), 2)

				if result.FirstKeptEntryIndex != 2 || result.TurnStartIndex != -1 || result.IsSplitTurn {
					t.Fatalf("FindCutPoint() = %#v, want custom-message cut point 2", result)
				}
			})
		}
	})
}

func TestCollectEntriesForBranchSummary(t *testing.T) {
	root := branchSummaryTestEntry("root")
	oldParent := branchSummaryTestEntry("old-parent", "root")
	oldLeaf := branchSummaryTestEntry("old-leaf", "old-parent")
	targetParent := branchSummaryTestEntry("target-parent", "root")
	targetLeaf := branchSummaryTestEntry("target-leaf", "target-parent")
	allEntries := []codingagent.SessionEntry{root, oldParent, oldLeaf, targetParent, targetLeaf}

	tests := []struct {
		name            string
		oldLeafID       string
		targetID        string
		wantEntries     []string
		wantCommon      string
		wantBranchCalls []string
		wantEntryCalls  []string
	}{
		{
			name:            "divergent branches",
			oldLeafID:       "old-leaf",
			targetID:        "target-leaf",
			wantEntries:     []string{"old-parent", "old-leaf"},
			wantCommon:      "root",
			wantBranchCalls: []string{"old-leaf", "target-leaf"},
			wantEntryCalls:  []string{"old-leaf", "old-parent"},
		},
		{
			name:            "missing target returns full old branch",
			oldLeafID:       "old-leaf",
			targetID:        "missing",
			wantEntries:     []string{"root", "old-parent", "old-leaf"},
			wantBranchCalls: []string{"old-leaf", "missing"},
			wantEntryCalls:  []string{"old-leaf", "old-parent", "root"},
		},
		{
			name:            "empty target returns full old branch",
			oldLeafID:       "old-leaf",
			wantEntries:     []string{"root", "old-parent", "old-leaf"},
			wantBranchCalls: []string{"old-leaf", ""},
			wantEntryCalls:  []string{"old-leaf", "old-parent", "root"},
		},
		{
			name:            "missing old leaf returns empty result",
			oldLeafID:       "missing",
			targetID:        "target-leaf",
			wantEntries:     []string{},
			wantBranchCalls: []string{"missing", "target-leaf"},
			wantEntryCalls:  []string{"missing"},
		},
		{
			name:        "empty old leaf returns empty result",
			targetID:    "target-leaf",
			wantEntries: []string{},
		},
		{
			name:            "same leaf returns common leaf and no entries",
			oldLeafID:       "old-leaf",
			targetID:        "old-leaf",
			wantEntries:     []string{},
			wantCommon:      "old-leaf",
			wantBranchCalls: []string{"old-leaf", "old-leaf"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			session := newBranchSummarySessionFake(allEntries, map[string][]codingagent.SessionEntry{
				"root":          {root},
				"old-parent":    {root, oldParent},
				"old-leaf":      {root, oldParent, oldLeaf},
				"target-parent": {root, targetParent},
				"target-leaf":   {root, targetParent, targetLeaf},
			})

			result, err := codingagent.CollectEntriesForBranchSummary(context.Background(), session, test.oldLeafID, test.targetID)
			if err != nil {
				t.Fatalf("CollectEntriesForBranchSummary() error = %v", err)
			}
			if result.Entries == nil {
				t.Fatal("CollectEntriesForBranchSummary() Entries = nil, want allocated slice")
			}
			if got := branchSummaryEntryIDs(result.Entries); !reflect.DeepEqual(got, test.wantEntries) {
				t.Errorf("CollectEntriesForBranchSummary() entry IDs = %v, want %v", got, test.wantEntries)
			}
			if test.wantCommon == "" {
				if result.CommonAncestorID != nil {
					t.Errorf("CollectEntriesForBranchSummary() common ancestor = %q, want nil", *result.CommonAncestorID)
				}
			} else if result.CommonAncestorID == nil || *result.CommonAncestorID != test.wantCommon {
				t.Errorf("CollectEntriesForBranchSummary() common ancestor = %v, want %q", result.CommonAncestorID, test.wantCommon)
			}
			if !reflect.DeepEqual(session.branchCalls, test.wantBranchCalls) {
				t.Errorf("GetBranch calls = %v, want explicit calls %v", session.branchCalls, test.wantBranchCalls)
			}
			if !reflect.DeepEqual(session.entryCalls, test.wantEntryCalls) {
				t.Errorf("GetEntry calls = %v, want %v", session.entryCalls, test.wantEntryCalls)
			}
		})
	}
}

func TestPrepareBranchEntries(t *testing.T) {
	const timestamp = "2025-01-04T03:04:05.6789Z"
	tests := []struct {
		name        string
		content     any
		wantContent ai.UserMessageContent
		details     json.RawMessage
		wantDetails any
		wantNull    bool
		wantSet     bool
	}{
		{
			name:        "string",
			content:     "abcd",
			wantContent: ai.UserText("abcd"),
			details:     json.RawMessage(`{"source":"string"}`),
			wantDetails: map[string]any{"source": "string"},
			wantSet:     true,
		},
		{
			name: "user content slice",
			content: []ai.UserContent{
				ai.TextContent{Type: ai.ContentTypeText, Text: "abcdefgh"},
			},
			wantContent: ai.UserBlocks(ai.TextContent{Type: ai.ContentTypeText, Text: "abcdefgh"}),
			details:     json.RawMessage(`null`),
			wantNull:    true,
			wantSet:     true,
		},
		{
			name:        "raw JSON",
			content:     json.RawMessage(`"abcdefghijkl"`),
			wantContent: ai.UserText("abcdefghijkl"),
		},
		{
			name:        "typed user message content",
			content:     ai.UserBlocks(ai.TextContent{Type: ai.ContentTypeText, Text: "abcdefghijklmnop"}),
			wantContent: ai.UserBlocks(ai.TextContent{Type: ai.ContentTypeText, Text: "abcdefghijklmnop"}),
			details:     json.RawMessage(`{"source":"typed"}`),
			wantDetails: map[string]any{"source": "typed"},
			wantSet:     true,
		},
	}

	entries := make([]codingagent.SessionEntry, len(tests))
	for index, test := range tests {
		entries[index] = codingagent.SessionEntry{
			SessionEntryBase: codingagent.SessionEntryBase{Type: "custom_message", Timestamp: timestamp},
			CustomType:       test.name,
			Content:          test.content,
			Details:          test.details,
			Display:          true,
		}
	}

	result := codingagent.PrepareBranchEntries(entries)

	if len(result.Messages) != len(tests) {
		t.Fatalf("PrepareBranchEntries() returned %d messages, want %d", len(result.Messages), len(tests))
	}
	if result.TotalTokens != 10 {
		t.Fatalf("PrepareBranchEntries() TotalTokens = %d, want 10", result.TotalTokens)
	}
	for index, test := range tests {
		message, ok := result.Messages[index].(agent.CustomMessage)
		if !ok {
			t.Fatalf("message %d type = %T, want agent.CustomMessage", index, result.Messages[index])
		}
		if message.CustomType != test.name || !message.Display {
			t.Errorf("message %d identity = (%q, %t), want (%q, true)", index, message.CustomType, message.Display, test.name)
		}
		if !reflect.DeepEqual(message.Content, test.wantContent) {
			t.Errorf("message %d content = %#v, want %#v", index, message.Content, test.wantContent)
		}
		if got, want := codingagent.EstimateTokens(message), int64(index+1); got != want {
			t.Errorf("message %d tokens = %d, want %d", index, got, want)
		}
		if message.Timestamp != 1735959845678 {
			t.Errorf("message %d timestamp = %d, want 1735959845678", index, message.Timestamp)
		}
		if message.Details.IsSet() != test.wantSet || message.Details.IsNull() != test.wantNull {
			t.Errorf("message %d details state = (set %t, null %t), want (set %t, null %t)", index, message.Details.IsSet(), message.Details.IsNull(), test.wantSet, test.wantNull)
		}
		if got, ok := message.Details.Value(); ok != (test.wantDetails != nil) || !reflect.DeepEqual(got, test.wantDetails) {
			t.Errorf("message %d details value = (%#v, %t), want (%#v, %t)", index, got, ok, test.wantDetails, test.wantDetails != nil)
		}
	}

	t.Run("preserves summary timestamps", func(t *testing.T) {
		entries := []codingagent.SessionEntry{
			{
				SessionEntryBase: codingagent.SessionEntryBase{Type: "branch_summary", Timestamp: timestamp},
				Summary:          "branch",
				FromID:           "abandoned",
			},
			{
				SessionEntryBase: codingagent.SessionEntryBase{Type: "compaction", Timestamp: timestamp},
				Summary:          "compacted",
				TokensBefore:     42,
			},
		}

		result := codingagent.PrepareBranchEntries(entries)

		if len(result.Messages) != 2 {
			t.Fatalf("PrepareBranchEntries() returned %d summary messages, want 2", len(result.Messages))
		}
		branch, ok := result.Messages[0].(agent.BranchSummaryMessage)
		if !ok || branch.Summary != "branch" || branch.FromID != "abandoned" || branch.Timestamp != 1735959845678 {
			t.Errorf("branch summary = %#v, want preserved fields and timestamp 1735959845678", result.Messages[0])
		}
		compaction, ok := result.Messages[1].(agent.CompactionSummaryMessage)
		if !ok || compaction.Summary != "compacted" || compaction.TokensBefore != 42 || compaction.Timestamp != 1735959845678 {
			t.Errorf("compaction summary = %#v, want preserved fields and timestamp 1735959845678", result.Messages[1])
		}
	})
}

func TestSerializeConversationTruncatesToolResultsByUTF16CodeUnits(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "exact boundary ending in non-BMP character",
			content: strings.Repeat("a", 1998) + "😀",
			want:    strings.Repeat("a", 1998) + "😀",
		},
		{
			name:    "truncates after complete non-BMP character",
			content: strings.Repeat("a", 1998) + "😀z",
			want:    strings.Repeat("a", 1998) + "😀\n\n[... 1 more characters truncated]",
		},
		{
			name:    "split surrogate is replaced with valid Unicode",
			content: strings.Repeat("a", 1999) + "😀z",
			want:    strings.Repeat("a", 1999) + "�\n\n[... 2 more characters truncated]",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			message := ai.ToolResultMessage{
				Role:       ai.MessageRoleToolResult,
				ToolCallID: "call-1",
				ToolName:   "read",
				Content: []ai.ToolResultContent{
					ai.TextContent{Type: ai.ContentTypeText, Text: test.content},
				},
			}

			got := codingagent.SerializeConversation([]ai.Message{message})
			want := "[Tool result]: " + test.want

			if got != want {
				t.Errorf("SerializeConversation() = %q, want %q", got, want)
			}
			if !utf8.ValidString(got) {
				t.Errorf("SerializeConversation() returned invalid UTF-8: %q", got)
			}
		})
	}
}

func compactionMessageEntry(message agent.AgentMessage) codingagent.SessionEntry {
	return codingagent.SessionEntry{
		SessionEntryBase: codingagent.SessionEntryBase{Type: "message"},
		Message:          message,
	}
}

type branchSummarySessionFake struct {
	entries     map[string]codingagent.SessionEntry
	branches    map[string][]codingagent.SessionEntry
	branchCalls []string
	entryCalls  []string
}

func newBranchSummarySessionFake(entries []codingagent.SessionEntry, branches map[string][]codingagent.SessionEntry) *branchSummarySessionFake {
	byID := make(map[string]codingagent.SessionEntry, len(entries))
	for _, entry := range entries {
		byID[entry.ID] = entry
	}
	return &branchSummarySessionFake{entries: byID, branches: branches}
}

func (session *branchSummarySessionFake) GetBranch(fromID ...string) []codingagent.SessionEntry {
	if len(fromID) != 1 {
		panic("GetBranch must receive exactly one explicit leaf ID")
	}
	session.branchCalls = append(session.branchCalls, fromID[0])
	return append([]codingagent.SessionEntry{}, session.branches[fromID[0]]...)
}

func (session *branchSummarySessionFake) GetEntry(id string) *codingagent.SessionEntry {
	session.entryCalls = append(session.entryCalls, id)
	entry, ok := session.entries[id]
	if !ok {
		return nil
	}
	return &entry
}

func branchSummaryTestEntry(id string, parentID ...string) codingagent.SessionEntry {
	entry := codingagent.SessionEntry{SessionEntryBase: codingagent.SessionEntryBase{ID: id}}
	if len(parentID) != 0 {
		parent := parentID[0]
		entry.ParentID = &parent
	}
	return entry
}

func branchSummaryEntryIDs(entries []codingagent.SessionEntry) []string {
	ids := make([]string, len(entries))
	for index := range entries {
		ids[index] = entries[index].ID
	}
	return ids
}

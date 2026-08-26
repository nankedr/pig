package codingagent_test

import (
	"reflect"
	"testing"

	"github.com/nankedr/pig/codingagent"
	"github.com/nankedr/pig/protocol"
)

func TestTranscriptProjectionReducer(t *testing.T) {
	t.Run("orders overlays and queued steering by stable ID", func(t *testing.T) {
		snapshot := protocol.SessionSnapshot{
			ID:       "session-1",
			Revision: 1,
			Transcript: []protocol.TranscriptItem{
				protocol.UserTranscriptItem{ID: "user-1", Role: protocol.TranscriptRoleUser, Content: []protocol.UserContent{protocol.TextContent{Type: protocol.ContentTypeText, Text: "original"}}},
				protocol.StreamingAssistantTranscriptItem{ID: "assistant-1", Role: protocol.TranscriptRoleAssistant, Status: protocol.AssistantTranscriptStatusStreaming},
			},
			QueuedSteer: []protocol.UserTranscriptItem{
				{ID: "user-1", Role: protocol.TranscriptRoleUser},
				{ID: "queued-1", Role: protocol.TranscriptRoleUser},
			},
		}
		state := codingagent.CreateTranscriptState(snapshot)
		state = codingagent.ApplyTranscriptProgress(state, protocol.ItemStartedProgress{
			Type: protocol.TranscriptProgressTypeItemStarted,
			Item: protocol.UserTranscriptItem{ID: "user-1", Role: protocol.TranscriptRoleUser, Content: []protocol.UserContent{protocol.TextContent{Type: protocol.ContentTypeText, Text: "overlaid"}}},
		})
		state = codingagent.ApplyTranscriptProgress(state, protocol.ItemStartedProgress{
			Type: protocol.TranscriptProgressTypeItemStarted,
			Item: protocol.RunningToolTranscriptItem{ID: "tool-1", Role: protocol.TranscriptRoleTool, Status: protocol.ToolTranscriptStatusRunning},
		})
		state = codingagent.ApplyTranscriptProgress(state, protocol.ItemUpdatedProgress{
			Type: protocol.TranscriptProgressTypeItemUpdated,
			Item: protocol.CompleteToolTranscriptItem{ID: "tool-1", Role: protocol.TranscriptRoleTool, Status: protocol.ToolTranscriptStatusComplete},
		})

		transcript := codingagent.SelectTranscript(state)
		if got, want := transcriptIDs(transcript), []string{"user-1", "assistant-1", "tool-1", "queued-1"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("SelectTranscript IDs = %v, want %v", got, want)
		}
		user, ok := transcript[0].(protocol.UserTranscriptItem)
		if !ok || len(user.Content) != 1 {
			t.Fatalf("overlaid user item = %#v", transcript[0])
		}
		text, ok := user.Content[0].(protocol.TextContent)
		if !ok || text.Text != "overlaid" {
			t.Fatalf("overlaid user content = %#v, want overlaid text", user.Content[0])
		}
		tool, ok := transcript[2].(protocol.CompleteToolTranscriptItem)
		if !ok || tool.Status != protocol.ToolTranscriptStatusComplete {
			t.Fatalf("updated tool item = %#v, want complete tool item", transcript[2])
		}
	})

	t.Run("applies assistant deltas and buffers partial tool JSON", func(t *testing.T) {
		snapshot := protocol.SessionSnapshot{
			ID:       "session-1",
			Revision: 1,
			Transcript: []protocol.TranscriptItem{protocol.StreamingAssistantTranscriptItem{
				ID:     "assistant-1",
				Role:   protocol.TranscriptRoleAssistant,
				Status: protocol.AssistantTranscriptStatusStreaming,
				Content: []protocol.AssistantContent{
					protocol.TextContent{Type: protocol.ContentTypeText, Text: "text"},
					protocol.ThinkingContent{Type: protocol.ContentTypeThinking, Thinking: "thought"},
					protocol.ToolCallContent{Type: protocol.ContentTypeToolCall, ToolCallID: "call-1", ToolName: "read", Input: ""},
				},
			}},
		}
		original := codingagent.CreateTranscriptState(snapshot)
		state := codingagent.ApplyTranscriptProgress(original, protocol.AssistantDeltaProgress{Type: protocol.TranscriptProgressTypeAssistantDelta, MessageID: "assistant-1", ContentIndex: 0, Kind: protocol.AssistantDeltaKindText, Delta: " delta"})
		state = codingagent.ApplyTranscriptProgress(state, protocol.AssistantDeltaProgress{Type: protocol.TranscriptProgressTypeAssistantDelta, MessageID: "assistant-1", ContentIndex: 1, Kind: protocol.AssistantDeltaKindThinking, Delta: " delta"})
		state = codingagent.ApplyTranscriptProgress(state, protocol.AssistantDeltaProgress{Type: protocol.TranscriptProgressTypeAssistantDelta, MessageID: "assistant-1", ContentIndex: 2, Kind: protocol.AssistantDeltaKindToolCall, Delta: `{"path":`})

		assistant := transcriptAssistant(t, codingagent.SelectTranscript(state))
		if got := assistant.Content[0].(protocol.TextContent).Text; got != "text delta" {
			t.Errorf("text delta result = %q, want text delta", got)
		}
		if got := assistant.Content[1].(protocol.ThinkingContent).Thinking; got != "thought delta" {
			t.Errorf("thinking delta result = %q, want thought delta", got)
		}
		if got := assistant.Content[2].(protocol.ToolCallContent).Input; got != `{"path":` {
			t.Errorf("partial tool input = %#v, want raw prefix", got)
		}

		state = codingagent.ApplyTranscriptProgress(state, protocol.AssistantDeltaProgress{Type: protocol.TranscriptProgressTypeAssistantDelta, MessageID: "assistant-1", ContentIndex: 2, Kind: protocol.AssistantDeltaKindToolCall, Delta: `"README.md"}`})
		assistant = transcriptAssistant(t, codingagent.SelectTranscript(state))
		if got, want := assistant.Content[2].(protocol.ToolCallContent).Input, map[string]any{"path": "README.md"}; !reflect.DeepEqual(got, want) {
			t.Errorf("complete tool input = %#v, want %#v", got, want)
		}
		if got := transcriptAssistant(t, codingagent.SelectTranscript(original)).Content[0].(protocol.TextContent).Text; got != "text" {
			t.Errorf("ApplyTranscriptProgress mutated prior state: original text = %q", got)
		}

		finished := protocol.CompleteAssistantTranscriptItem{ID: "assistant-1", Role: protocol.TranscriptRoleAssistant, Status: protocol.AssistantTranscriptStatusComplete, StopReason: protocol.AssistantStopReasonStop}
		state = codingagent.ApplyTranscriptProgress(state, protocol.ItemFinishedProgress{Type: protocol.TranscriptProgressTypeItemFinished, Item: finished})
		if len(state.ToolCallBuffers) != 0 {
			t.Fatalf("finished item retained tool-call buffers: %v", state.ToolCallBuffers)
		}
	})

	t.Run("applies snapshot revisions within each session", func(t *testing.T) {
		state := codingagent.CreateTranscriptState(protocol.SessionSnapshot{
			ID:         "session-1",
			Revision:   5,
			Transcript: []protocol.TranscriptItem{protocol.UserTranscriptItem{ID: "current", Role: protocol.TranscriptRoleUser}},
		})
		state = codingagent.ApplyTranscriptProgress(state, protocol.ItemStartedProgress{
			Type: protocol.TranscriptProgressTypeItemStarted,
			Item: protocol.UserTranscriptItem{ID: "progress", Role: protocol.TranscriptRoleUser},
		})

		stale := codingagent.ApplyTranscriptSnapshot(state, protocol.SessionSnapshot{ID: "session-1", Revision: 4})
		if stale.Snapshot.Revision != 5 || len(stale.ProgressItems) != 1 {
			t.Fatalf("stale same-session snapshot changed state: %#v", stale)
		}

		equal := codingagent.ApplyTranscriptSnapshot(state, protocol.SessionSnapshot{
			ID:         "session-1",
			Revision:   5,
			Transcript: []protocol.TranscriptItem{protocol.UserTranscriptItem{ID: "replacement", Role: protocol.TranscriptRoleUser}},
		})
		if got := transcriptIDs(codingagent.SelectTranscript(equal)); !reflect.DeepEqual(got, []string{"replacement"}) || len(equal.ProgressItems) != 0 || len(equal.ToolCallBuffers) != 0 {
			t.Fatalf("equal revision did not replace snapshot and clear progress: IDs=%v state=%#v", got, equal)
		}

		different := codingagent.ApplyTranscriptSnapshot(state, protocol.SessionSnapshot{ID: "session-2", Revision: 1})
		if different.Snapshot.ID != "session-2" || different.Snapshot.Revision != 1 || len(different.ProgressItems) != 0 {
			t.Fatalf("different-session snapshot was rejected or retained progress: %#v", different)
		}
	})
}

func transcriptIDs(items []protocol.TranscriptItem) []string {
	ids := make([]string, len(items))
	for index, item := range items {
		switch item := item.(type) {
		case protocol.UserTranscriptItem:
			ids[index] = item.ID
		case protocol.StreamingAssistantTranscriptItem:
			ids[index] = item.ID
		case protocol.CompleteAssistantTranscriptItem:
			ids[index] = item.ID
		case protocol.RunningToolTranscriptItem:
			ids[index] = item.ID
		case protocol.CompleteToolTranscriptItem:
			ids[index] = item.ID
		}
	}
	return ids
}

func transcriptAssistant(t *testing.T, items []protocol.TranscriptItem) protocol.StreamingAssistantTranscriptItem {
	t.Helper()
	if len(items) != 1 {
		t.Fatalf("transcript length = %d, want 1", len(items))
	}
	assistant, ok := items[0].(protocol.StreamingAssistantTranscriptItem)
	if !ok {
		t.Fatalf("transcript item = %T, want protocol.StreamingAssistantTranscriptItem", items[0])
	}
	return assistant
}

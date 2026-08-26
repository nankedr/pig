package codingagent

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/nankedr/pig/ai"
)

func TestParseSessionEntriesPreservesEverySyntacticallyValidRecord(t *testing.T) {
	validLines := []string{
		`{"type":"session","version":"future","id":"session-1","timestamp":"2025-01-01T00:00:00Z","cwd":"/project"}`,
		`{"type":"message","id":"message-1","parentId":null,"timestamp":"2025-01-01T00:00:01Z","message":{"role":"user","content":{"future":true},"timestamp":1}}`,
		`{"type":"custom_message","id":"custom-1","parentId":"message-1","timestamp":"2025-01-01T00:00:02Z","customType":"future","content":{"future":true},"display":true}`,
		`{"type":"compaction","id":"compaction-1","parentId":"custom-1","timestamp":"2025-01-01T00:00:03Z","summary":"kept","usage":"future"}`,
		`{"type":7,"id":"wrongly-typed-discriminator","timestamp":"2025-01-01T00:00:04Z"}`,
		`["future","record"]`,
		`"future-record"`,
		`42`,
		`true`,
		`null`,
	}
	content := strings.Join(append(append([]string{}, validLines...), `{"type":`), "\n")

	entries := ParseSessionEntries(content)

	if len(entries) != len(validLines) {
		t.Fatalf("ParseSessionEntries() returned %d entries, want %d valid records", len(entries), len(validLines))
	}
	for index, want := range validLines {
		if !json.Valid(entries[index].Raw) {
			t.Errorf("entry %d Raw = %q, want valid JSON", index, entries[index].Raw)
		}
		if !bytes.Equal(entries[index].Raw, []byte(want)) {
			t.Errorf("entry %d Raw = %s, want %s", index, entries[index].Raw, want)
		}
	}

	header := entries[0].Header
	if header == nil || header.ID != "session-1" || header.Version != nil {
		t.Errorf("partially decoded header = %#v, want retained ID and unsupported version left unset", header)
	}
	message := entries[1].Entry
	if message == nil || message.Type != "message" || message.ID != "message-1" || message.Message != nil {
		t.Errorf("partially decoded message entry = %#v, want base fields retained and invalid typed message unset", message)
	}
	custom := entries[2].Entry
	if custom == nil {
		t.Fatal("custom message did not retain its typed entry projection")
	}
	rawContent, ok := custom.Content.(json.RawMessage)
	if !ok || !bytes.Equal(rawContent, []byte(`{"future":true}`)) {
		t.Errorf("custom content = %#v, want raw forward-compatible JSON", custom)
	}
	compaction := entries[3].Entry
	if compaction == nil || compaction.Summary != "kept" || compaction.Usage != nil {
		t.Errorf("partially decoded compaction = %#v, want independent valid fields retained", compaction)
	}
}

func TestParseSessionEntriesNormalizesMissingAndNullBuiltInMessageContent(t *testing.T) {
	tests := []struct {
		name    string
		message string
		assert  func(*testing.T, any)
	}{
		{
			name:    "user missing content",
			message: `{"role":"user","timestamp":1}`,
			assert: func(t *testing.T, value any) {
				message, ok := value.(ai.UserMessage)
				if !ok {
					t.Fatalf("Message = %T, want ai.UserMessage", value)
				}
				blocks, ok := message.Content.Blocks()
				if !ok || len(blocks) != 0 {
					t.Fatalf("user Content.Blocks() = (%#v, %t), want empty blocks", blocks, ok)
				}
			},
		},
		{
			name:    "user null content",
			message: `{"role":"user","content":null,"timestamp":1}`,
			assert: func(t *testing.T, value any) {
				message, ok := value.(ai.UserMessage)
				if !ok {
					t.Fatalf("Message = %T, want ai.UserMessage", value)
				}
				blocks, ok := message.Content.Blocks()
				if !ok || len(blocks) != 0 {
					t.Fatalf("user Content.Blocks() = (%#v, %t), want empty blocks", blocks, ok)
				}
			},
		},
		{
			name:    "assistant missing content",
			message: `{"role":"assistant","api":"test-api","provider":"test-provider","model":"model-1","usage":{},"stopReason":"stop","timestamp":2}`,
			assert: func(t *testing.T, value any) {
				message, ok := value.(ai.AssistantMessage)
				if !ok || len(message.Content) != 0 {
					t.Fatalf("Message = %#v, want ai.AssistantMessage with empty content", value)
				}
			},
		},
		{
			name:    "assistant null content",
			message: `{"role":"assistant","content":null,"api":"test-api","provider":"test-provider","model":"model-1","usage":{},"stopReason":"stop","timestamp":2}`,
			assert: func(t *testing.T, value any) {
				message, ok := value.(ai.AssistantMessage)
				if !ok || len(message.Content) != 0 {
					t.Fatalf("Message = %#v, want ai.AssistantMessage with empty content", value)
				}
			},
		},
		{
			name:    "tool result missing content",
			message: `{"role":"toolResult","toolCallId":"call-1","toolName":"read","isError":false,"timestamp":3}`,
			assert: func(t *testing.T, value any) {
				message, ok := value.(ai.ToolResultMessage)
				if !ok || len(message.Content) != 0 {
					t.Fatalf("Message = %#v, want ai.ToolResultMessage with empty content", value)
				}
			},
		},
		{
			name:    "tool result null content",
			message: `{"role":"toolResult","toolCallId":"call-1","toolName":"read","content":null,"isError":false,"timestamp":3}`,
			assert: func(t *testing.T, value any) {
				message, ok := value.(ai.ToolResultMessage)
				if !ok || len(message.Content) != 0 {
					t.Fatalf("Message = %#v, want ai.ToolResultMessage with empty content", value)
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			line := `{"type":"message","id":"message-1","parentId":null,"timestamp":"2025-01-01T00:00:01Z","message":` + test.message + `}`
			entries := ParseSessionEntries(line)
			if len(entries) != 1 || entries[0].Entry == nil {
				t.Fatalf("ParseSessionEntries() = %#v, want one typed entry", entries)
			}
			test.assert(t, entries[0].Entry.Message)
			if !bytes.Equal(entries[0].Raw, []byte(line)) || !bytes.Equal(entries[0].Entry.Raw, []byte(line)) {
				t.Fatalf("session parser changed original JSONL record: file Raw %s, entry Raw %s", entries[0].Raw, entries[0].Entry.Raw)
			}
		})
	}
}

func TestSessionManagerGetBranchDistinguishesOmittedAndInvalidLeafIDs(t *testing.T) {
	parent := func(id string) *string { return &id }
	entries := []SessionEntry{
		{SessionEntryBase: SessionEntryBase{Type: "message", ID: "root"}},
		{SessionEntryBase: SessionEntryBase{Type: "message", ID: "current", ParentID: parent("root")}},
		{SessionEntryBase: SessionEntryBase{Type: "message", ID: "final", ParentID: parent("root")}},
	}
	currentLeaf := "current"
	manager := &SessionManager{entries: entries, leafID: &currentLeaf}

	tests := []struct {
		name string
		get  func() []SessionEntry
		want []string
	}{
		{name: "omitted leaf uses current leaf", get: func() []SessionEntry { return manager.GetBranch() }, want: []string{"root", "current"}},
		{name: "explicit existing leaf uses selected branch", get: func() []SessionEntry { return manager.GetBranch("final") }, want: []string{"root", "final"}},
		{name: "explicit empty leaf is empty", get: func() []SessionEntry { return manager.GetBranch("") }, want: []string{}},
		{name: "explicit missing leaf is empty", get: func() []SessionEntry { return manager.GetBranch("missing") }, want: []string{}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := test.get()
			if got == nil {
				t.Fatal("GetBranch() returned nil, want an allocated branch")
			}
			if ids := sessionEntryIDsForReview(got); !equalStrings(ids, test.want) {
				t.Fatalf("GetBranch() IDs = %v, want %v", ids, test.want)
			}
		})
	}

	for _, test := range []struct {
		name   string
		leafID []*string
	}{
		{name: "omitted"},
		{name: "empty", leafID: []*string{parent("")}},
		{name: "missing", leafID: []*string{parent("missing")}},
	} {
		t.Run("standalone context "+test.name, func(t *testing.T) {
			got := sessionEntryIDsForReview(BuildContextEntries(entries, test.leafID...))
			if !equalStrings(got, []string{"root", "final"}) {
				t.Fatalf("BuildContextEntries() IDs = %v, want final-entry branch", got)
			}
		})
	}
}

func sessionEntryIDsForReview(entries []SessionEntry) []string {
	ids := make([]string, len(entries))
	for index := range entries {
		ids[index] = entries[index].ID
	}
	return ids
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func TestSessionManagerTreeOperationsAreExplicitCapabilityStubs(t *testing.T) {
	manager := NewInMemorySessionManager("/project")

	tree, treeErr := manager.GetTree()
	if tree != nil || !errors.Is(treeErr, ErrNotImplemented) {
		t.Fatalf("GetTree() = (%#v, %v), want (nil, ErrNotImplemented)", tree, treeErr)
	}
	var treeCapability *NotImplementedError
	if !errors.As(treeErr, &treeCapability) || treeCapability.Operation != "SessionManager.GetTree" {
		t.Fatalf("GetTree() error = %#v, want structured SessionManager.GetTree capability error", treeErr)
	}

	children, childrenErr := manager.GetChildren("entry-1")
	if children != nil || !errors.Is(childrenErr, ErrNotImplemented) {
		t.Fatalf("GetChildren() = (%#v, %v), want (nil, ErrNotImplemented)", children, childrenErr)
	}
	var childrenCapability *NotImplementedError
	if !errors.As(childrenErr, &childrenCapability) || childrenCapability.Operation != "SessionManager.GetChildren" {
		t.Fatalf("GetChildren() error = %#v, want structured SessionManager.GetChildren capability error", childrenErr)
	}
}

package codingagent_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
)

func TestParseSessionEntriesDecodesV3MessageDiscriminators(t *testing.T) {
	content := `{"type":"session","version":3,"id":"session-1","timestamp":"2025-01-01T00:00:00Z","cwd":"/project"}
{"type":"message","id":"message-1","parentId":null,"timestamp":"2025-01-01T00:00:01Z","message":{"role":"user","content":"hello","timestamp":1}}
not-json`

	entries := codingagent.ParseSessionEntries(content)
	if len(entries) != 2 {
		t.Fatalf("ParseSessionEntries() returned %d entries, want 2", len(entries))
	}
	if entries[0].Header == nil || entries[0].Header.Version == nil || *entries[0].Header.Version != 3 {
		t.Fatalf("header = %#v, want v3 session header", entries[0].Header)
	}
	if entries[1].Entry == nil {
		t.Fatal("message row was not decoded")
	}
	message, ok := entries[1].Entry.Message.(ai.UserMessage)
	if !ok {
		t.Fatalf("message type = %T, want ai.UserMessage", entries[1].Entry.Message)
	}
	if text, ok := message.Content.Text(); !ok || text != "hello" {
		t.Fatalf("user content = (%q, %t), want (hello, true)", text, ok)
	}
}

func TestBuildSessionContextUsesLatestV3CompactionAndFullPathSettings(t *testing.T) {
	parent := func(id string) *string { return &id }
	user := func(id string, parentID *string, text string) codingagent.SessionEntry {
		return codingagent.SessionEntry{
			SessionEntryBase: codingagent.SessionEntryBase{Type: "message", ID: id, ParentID: parentID, Timestamp: "2025-01-01T00:00:00Z"},
			Message:          ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText(text), Timestamp: 1},
		}
	}
	assistant := func(id string, parentID *string, provider, model string) codingagent.SessionEntry {
		return codingagent.SessionEntry{
			SessionEntryBase: codingagent.SessionEntryBase{Type: "message", ID: id, ParentID: parentID, Timestamp: "2025-01-01T00:00:00Z"},
			Message: ai.AssistantMessage{
				Role: ai.MessageRoleAssistant, Content: []ai.AssistantContent{ai.TextContent{Type: ai.ContentTypeText, Text: id}},
				Provider: ai.ProviderID(provider), Model: model,
			},
		}
	}

	entries := []codingagent.SessionEntry{
		user("1", nil, "old"),
		{SessionEntryBase: codingagent.SessionEntryBase{Type: "thinking_level_change", ID: "2", ParentID: parent("1")}, ThinkingLevel: "high"},
		assistant("3", parent("2"), "old-provider", "old-model"),
		user("4", parent("3"), "kept"),
		{SessionEntryBase: codingagent.SessionEntryBase{Type: "compaction", ID: "5", ParentID: parent("4"), Timestamp: "2025-01-02T03:04:05Z"}, Summary: "first", FirstKeptEntryID: "4", TokensBefore: 10},
		assistant("6", parent("5"), "new-provider", "new-model"),
		{SessionEntryBase: codingagent.SessionEntryBase{Type: "compaction", ID: "7", ParentID: parent("6"), Timestamp: "2025-01-03T03:04:05Z"}, Summary: "latest", FirstKeptEntryID: "6", TokensBefore: 20},
		{SessionEntryBase: codingagent.SessionEntryBase{Type: "custom_message", ID: "8", ParentID: parent("7"), Timestamp: "2025-01-04T03:04:05Z"}, CustomType: "notice", Content: ai.UserText("custom"), Display: true},
		{SessionEntryBase: codingagent.SessionEntryBase{Type: "branch_summary", ID: "9", ParentID: parent("8"), Timestamp: "2025-01-05T03:04:05Z"}, FromID: "abandoned", Summary: "branch"},
	}

	contextEntries := codingagent.BuildContextEntries(entries)
	if got, want := sessionEntryIDs(contextEntries), []string{"7", "6", "8", "9"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("BuildContextEntries IDs = %v, want %v", got, want)
	}

	context := codingagent.BuildSessionContext(entries)
	if context.ThinkingLevel != "high" {
		t.Fatalf("ThinkingLevel = %q, want high", context.ThinkingLevel)
	}
	if context.Model == nil || context.Model.Provider != "new-provider" || context.Model.ModelID != "new-model" {
		t.Fatalf("Model = %#v, want latest assistant provider/model", context.Model)
	}
	if len(context.Messages) != 4 {
		t.Fatalf("Messages length = %d, want 4", len(context.Messages))
	}
	if message, ok := context.Messages[0].(agent.CompactionSummaryMessage); !ok || message.Summary != "latest" || message.TokensBefore != 20 || message.Timestamp != 1735873445000 {
		t.Fatalf("compaction message = %#v, want latest summary with parsed timestamp", context.Messages[0])
	}
	if message, ok := context.Messages[2].(agent.CustomMessage); !ok || message.CustomType != "notice" || !message.Display || message.Timestamp != 1735959845000 {
		t.Fatalf("custom message = %#v, want projected custom_message", context.Messages[2])
	}
	if message, ok := context.Messages[3].(agent.BranchSummaryMessage); !ok || message.Summary != "branch" || message.FromID != "abandoned" || message.Timestamp != 1736046245000 {
		t.Fatalf("branch message = %#v, want projected branch_summary", context.Messages[3])
	}
}

func sessionEntryIDs(entries []codingagent.SessionEntry) []string {
	ids := make([]string, len(entries))
	for i := range entries {
		ids[i] = entries[i].ID
	}
	return ids
}

func TestMigrateSessionEntriesMigratesV1ToV3InPlace(t *testing.T) {
	entries := codingagent.ParseSessionEntries(`{"type":"session","id":"session-1","timestamp":"2025-01-01T00:00:00Z","cwd":"/project"}
{"type":"message","timestamp":"2025-01-01T00:00:01Z","message":{"role":"user","content":"hello","timestamp":1}}
{"type":"compaction","timestamp":"2025-01-01T00:00:02Z","summary":"summary","firstKeptEntryIndex":1,"tokensBefore":2}`)

	codingagent.MigrateSessionEntries(entries)

	if entries[0].Header == nil || entries[0].Header.Version == nil || *entries[0].Header.Version != codingagent.CurrentSessionVersion {
		t.Fatalf("migrated header = %#v, want version %d", entries[0].Header, codingagent.CurrentSessionVersion)
	}
	first, second := entries[1].Entry, entries[2].Entry
	if first == nil || len(first.ID) != 8 || first.ParentID != nil {
		t.Fatalf("first migrated entry = %#v, want 8-char ID and nil parent", first)
	}
	if second == nil || len(second.ID) != 8 || second.ParentID == nil || *second.ParentID != first.ID {
		t.Fatalf("second migrated entry = %#v, want chained 8-char ID", second)
	}
	if second.FirstKeptEntryID != first.ID {
		t.Fatalf("firstKeptEntryId = %q, want %q", second.FirstKeptEntryID, first.ID)
	}
}

func TestMigrateSessionEntriesRenamesV2HookMessageRole(t *testing.T) {
	headerVersion := 2
	parent := "entry-1"
	hookMessage, err := agent.NewRawAgentMessage([]byte(`{"role":"hookMessage","content":"notice","timestamp":1}`))
	if err != nil {
		t.Fatal(err)
	}
	entries := []codingagent.FileEntry{
		{Type: "session", Header: &codingagent.SessionHeader{Type: "session", Version: &headerVersion}},
		{Type: "message", Entry: &codingagent.SessionEntry{SessionEntryBase: codingagent.SessionEntryBase{Type: "message", ID: "entry-1"}, Message: hookMessage}},
		{Type: "message", Entry: &codingagent.SessionEntry{SessionEntryBase: codingagent.SessionEntryBase{Type: "message", ID: "entry-2", ParentID: &parent}, Message: ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText("kept"), Timestamp: 2}}},
	}

	codingagent.MigrateSessionEntries(entries)

	if got := *entries[0].Header.Version; got != 3 {
		t.Fatalf("version = %d, want 3", got)
	}
	if got := entries[1].Entry.Message.MessageRole(); got != "custom" {
		t.Fatalf("migrated role = %q, want custom", got)
	}
	if entries[1].Entry.ID != "entry-1" || entries[2].Entry.ID != "entry-2" || entries[2].Entry.ParentID == nil || *entries[2].Entry.ParentID != "entry-1" {
		t.Fatalf("v2 migration rewrote existing tree: %#v", entries)
	}
}

func TestParseSkillBlockMatchesPinnedV3Envelope(t *testing.T) {
	input := "<skill name=\"review\" location=\"/tmp/SKILL.md\">\n  keep content whitespace  \n</skill>\n\n  inspect this change  \n"
	got := codingagent.ParseSkillBlock(input)
	want := &codingagent.ParsedSkillBlock{
		Name:        "review",
		Location:    "/tmp/SKILL.md",
		Content:     "  keep content whitespace  ",
		UserMessage: "inspect this change",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseSkillBlock() = %#v, want %#v", got, want)
	}

	withoutUserMessage := codingagent.ParseSkillBlock("<skill name=\"empty\" location=\"relative/SKILL.md\">\n\n</skill>")
	if withoutUserMessage == nil || withoutUserMessage.Content != "" || withoutUserMessage.UserMessage != "" {
		t.Fatalf("empty skill block = %#v, want empty content and absent Go user-message value", withoutUserMessage)
	}
}

func TestParseSkillBlockRejectsNonCanonicalEnvelopes(t *testing.T) {
	invalid := []string{
		"prefix <skill name=\"a\" location=\"b\">\nx\n</skill>",
		"<skill location=\"b\" name=\"a\">\nx\n</skill>",
		"<skill name=\"a\" location=\"b\">\r\nx\r\n</skill>",
		"<skill name=\"a\" location=\"b\">\nx\n</skill>\n",
		"<skill name=\"a\" location=\"b\">\nx\n</skill>\n\n",
	}
	for _, input := range invalid {
		if got := codingagent.ParseSkillBlock(input); got != nil {
			t.Errorf("ParseSkillBlock(%q) = %#v, want nil", input, got)
		}
	}
}

func TestAgentSessionComposesLegacyAgentAndV3Session(t *testing.T) {
	legacy, err := agent.NewAgent(agent.AgentOptions{InitialState: &agent.AgentInitialState{
		SystemPrompt: "system",
		Model: ai.Model{
			ID:       "model-1",
			Name:     "Model One",
			API:      ai.API("test-api"),
			Provider: ai.ProviderID("test-provider"),
		},
	}})
	if err != nil {
		t.Fatalf("NewAgent: %v", err)
	}
	v3 := codingagent.NewInMemorySessionManager("/project", codingagent.NewSessionOptions{ID: "session-1"})
	session := codingagent.NewAgentSession(codingagent.AgentSessionConfig{
		Agent:                  legacy,
		SessionManager:         v3,
		InitialActiveToolNames: []string{"read"},
		ScopedModels:           []codingagent.ScopedModel{{Model: legacy.State().Model}},
	})

	if session.Agent() != legacy {
		t.Fatal("AgentSession did not retain the legacy Agent")
	}
	if session.SessionManager() != v3 {
		t.Fatal("AgentSession did not retain the production v3 SessionManager")
	}
	if got := session.SessionID(); got != "session-1" {
		t.Fatalf("SessionID = %q, want session-1", got)
	}
	if got := session.Model().ID; got != "model-1" {
		t.Fatalf("Model().ID = %q, want model-1", got)
	}

	active := session.GetActiveToolNames()
	active[0] = "mutated"
	if got := session.GetActiveToolNames(); !reflect.DeepEqual(got, []string{"read"}) {
		t.Fatalf("active Tool names alias caller memory: %v", got)
	}
	scoped := session.ScopedModels()
	scoped[0].Model.ID = "mutated"
	if got := session.ScopedModels()[0].Model.ID; got != "model-1" {
		t.Fatalf("scoped models alias caller memory: %q", got)
	}
}

func TestAgentSessionPromptPropagatesUnconfiguredAgentErrorAndSettles(t *testing.T) {
	legacy, err := agent.NewAgent(agent.AgentOptions{})
	if err != nil {
		t.Fatalf("NewAgent: %v", err)
	}
	v3 := codingagent.NewInMemorySessionManager("/project", codingagent.NewSessionOptions{ID: "session-1"})
	session := codingagent.NewAgentSession(codingagent.AgentSessionConfig{
		Agent:                  legacy,
		SessionManager:         v3,
		InitialActiveToolNames: []string{"read", "bash"},
	})
	var events []codingagent.AgentSessionEventType
	_, err = session.Subscribe(func(event codingagent.AgentSessionEvent) {
		events = append(events, event.AgentSessionEventType())
	})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	err = session.Prompt(context.Background(), "must not run")
	if !errors.Is(err, agent.ErrNotImplemented) {
		t.Fatalf("Prompt error = %v, want Agent ErrNotImplemented", err)
	}
	if !reflect.DeepEqual(events, []codingagent.AgentSessionEventType{codingagent.AgentSessionEventTypeAgentSettled}) {
		t.Fatalf("events = %v, want agent_settled after failed preflight", events)
	}
	if !session.IsIdle() {
		t.Fatal("session remained active after Prompt failure")
	}
	if got := v3.GetEntries(); len(got) != 0 {
		t.Fatalf("failed Prompt appended v3 entries: %#v", got)
	}
}

func TestAgentSessionLifecycleAndUnsupportedQueueMethodsHaveNoQueueSideEffects(t *testing.T) {
	legacy, err := agent.NewAgent(agent.AgentOptions{})
	if err != nil {
		t.Fatalf("NewAgent: %v", err)
	}
	queued := ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText("queued"), Timestamp: 1}
	if err := legacy.Steer(queued); err != nil {
		t.Fatalf("legacy Steer: %v", err)
	}
	session := codingagent.NewAgentSession(codingagent.AgentSessionConfig{Agent: legacy})
	before := legacy.State()

	if err := session.ClearQueue(); !errors.Is(err, codingagent.ErrNotImplemented) {
		t.Errorf("ClearQueue() error = %v, want ErrNotImplemented", err)
	}
	if err := session.Abort(); err != nil {
		t.Errorf("Abort() error = %v", err)
	}
	if err := session.WaitForIdle(context.Background()); err != nil {
		t.Errorf("WaitForIdle() error = %v", err)
	}
	if count, err := session.PendingMessageCount(); !errors.Is(err, codingagent.ErrNotImplemented) || count != 0 {
		t.Errorf("PendingMessageCount() = (%d, %v), want (0, ErrNotImplemented)", count, err)
	}

	if got := legacy.State(); !reflect.DeepEqual(got, before) {
		t.Fatalf("unsupported session methods mutated legacy state: got %#v, want %#v", got, before)
	}
	if !legacy.HasQueuedMessages() {
		t.Fatal("session lifecycle methods drained the legacy queue")
	}
	if err := session.Dispose(); err != nil {
		t.Errorf("Dispose() error = %v", err)
	}
}

func TestSessionCarriersDefensivelyCopyNestedMutableValues(t *testing.T) {
	manager := codingagent.NewInMemorySessionManager("/project", codingagent.NewSessionOptions{ID: "session-1", ParentSession: "parent.jsonl"})
	header := manager.GetHeader()
	*header.ParentSession = "mutated"
	if got := manager.GetHeader().ParentSession; got == nil || *got != "parent.jsonl" {
		t.Fatalf("GetHeader retained returned ParentSession pointer: %v", got)
	}

	parentID := "parent"
	fromHook := true
	usage := ai.Usage{Input: 7}
	compactionInput := []codingagent.SessionEntry{{
		SessionEntryBase: codingagent.SessionEntryBase{Type: "compaction", ID: "compact", ParentID: &parentID},
		Details:          json.RawMessage(`{"nested":true}`),
		Usage:            &usage,
		FromHook:         &fromHook,
	}}
	compaction := codingagent.GetLatestCompactionEntry(compactionInput)
	*compaction.ParentID = "mutated"
	compaction.Details[0] = '['
	compaction.Usage.Input = 99
	*compaction.FromHook = false
	if *compactionInput[0].ParentID != "parent" || string(compactionInput[0].Details) != `{"nested":true}` || compactionInput[0].Usage.Input != 7 || !*compactionInput[0].FromHook {
		t.Fatalf("GetLatestCompactionEntry result aliases input: %#v", compactionInput[0])
	}

	models := []codingagent.ScopedModel{{Model: ai.Model{
		ID:               "model-1",
		Input:            []ai.ModelInput{ai.ModelInputText},
		Headers:          map[string]string{"x-test": "original"},
		SamplingParams:   map[string]json.RawMessage{"temperature": json.RawMessage(`0.5`)},
		ThinkingLevelMap: ai.ThinkingLevelMap{"high": ai.Some("high")},
	}}}
	session := codingagent.NewAgentSession(codingagent.AgentSessionConfig{ScopedModels: models})
	models[0].Model.Headers["x-test"] = "caller-mutated"
	returned := session.ScopedModels()
	returned[0].Model.Input[0] = ai.ModelInputImage
	returned[0].Model.SamplingParams["temperature"][0] = '9'
	returned[0].Model.ThinkingLevelMap["high"] = ai.Some("low")
	stored := session.ScopedModels()[0].Model
	if stored.Headers["x-test"] != "original" || stored.Input[0] != ai.ModelInputText || string(stored.SamplingParams["temperature"]) != "0.5" {
		t.Fatalf("ScopedModels aliases caller or returned memory: %#v", stored)
	}
	if level, ok := stored.ThinkingLevelMap["high"].Value(); !ok || level != "high" {
		t.Fatalf("ScopedModels thinking-level map aliases returned memory: (%q, %t)", level, ok)
	}
}

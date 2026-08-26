package codingagent

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
)

const CurrentSessionVersion = 3

type SessionHeader struct {
	Type          string  `json:"type"`
	Version       *int    `json:"version,omitempty"`
	ID            string  `json:"id"`
	Timestamp     string  `json:"timestamp"`
	CWD           string  `json:"cwd"`
	ParentSession *string `json:"parentSession,omitempty"`
}
type NewSessionOptions struct{ ID, ParentSession string }

type AppendCompactionOptions struct {
	Details  json.RawMessage
	FromHook *bool
	Usage    *ai.Usage
}

type BranchSummaryOptions struct {
	Details  json.RawMessage
	FromHook *bool
	Usage    *ai.Usage
}
type SessionEntryBase struct {
	Type, ID, Timestamp string
	ParentID            *string
}
type SessionMessageEntry struct {
	SessionEntryBase
	Message agent.AgentMessage
}
type ThinkingLevelChangeEntry struct {
	SessionEntryBase
	ThinkingLevel string
}
type ModelChangeEntry struct {
	SessionEntryBase
	Provider, ModelID string
}
type CompactionEntry struct {
	SessionEntryBase
	Summary, FirstKeptEntryID string
	TokensBefore              int64
	Details                   json.RawMessage
	Usage                     *ai.Usage
	FromHook                  *bool
}
type BranchSummaryEntry struct {
	SessionEntryBase
	FromID, Summary string
	Details         json.RawMessage
	Usage           *ai.Usage
	FromHook        *bool
}
type CustomEntry struct {
	SessionEntryBase
	CustomType string
	Data       json.RawMessage
}
type CustomMessageEntry struct {
	SessionEntryBase
	CustomType string
	Content    any
	Details    json.RawMessage
	Display    bool
}
type LabelEntry struct {
	SessionEntryBase
	TargetID string
	Label    *string
}
type SessionInfoEntry struct {
	SessionEntryBase
	Name *string
}

// SessionEntry is the v3 production-session carrier. Typed variant fields are
// populated when understood; Raw preserves the complete record independently.
// It is intentionally unrelated to agent's v4 Entry.
type SessionEntry struct {
	SessionEntryBase
	// Raw preserves the complete JSONL record when its typed projection is
	// partial (for example, when a newer message/content variant is read).
	Raw              json.RawMessage `json:"-"`
	Message          agent.AgentMessage
	ThinkingLevel    string
	Provider         string
	ModelID          string
	Summary          string
	FirstKeptEntryID string
	TokensBefore     int64
	FromID           string
	CustomType       string
	Data             json.RawMessage
	Content          any
	Details          json.RawMessage
	Display          bool
	TargetID         string
	Label            *string
	Name             *string
	Usage            *ai.Usage
	FromHook         *bool
}
type FileEntry struct {
	Type, ID, Timestamp string
	Header              *SessionHeader
	Entry               *SessionEntry
	// Raw is non-nil for every syntactically valid JSONL record returned by
	// ParseSessionEntries, including values that have no typed Go projection.
	Raw json.RawMessage `json:"-"`
}
type SessionModel struct{ Provider, ModelID string }
type SessionContext struct {
	Messages      []agent.AgentMessage
	ThinkingLevel string
	Model         *SessionModel
}
type SessionTreeNode struct {
	Entry          SessionEntry
	Children       []SessionTreeNode
	Label          *string
	LabelTimestamp *string
}
type SessionInfo struct {
	Path, ID, CWD                 string
	Name, ParentSessionPath       *string
	Created, Modified             time.Time
	MessageCount                  int
	FirstMessage, AllMessagesText string
}

type SessionListProgress func(loaded, total int)

type SessionListOptions struct {
	SessionDir *string
	OnProgress SessionListProgress
}

type SessionManager struct {
	cwd, sessionDir, sessionID, sessionFile string
	header                                  SessionHeader
	entries                                 []SessionEntry
	leafID                                  *string
}

// NewInMemorySessionManager creates only in-memory state and performs no I/O.
func NewInMemorySessionManager(cwd string, options ...NewSessionOptions) *SessionManager {
	m := &SessionManager{
		cwd:     cwd,
		entries: []SessionEntry{},
		header: SessionHeader{
			Type:      "session",
			ID:        newSessionID(),
			Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
			CWD:       cwd,
		},
	}
	v := CurrentSessionVersion
	m.header.Version = &v
	if len(options) > 0 {
		if options[0].ID != "" {
			m.header.ID = options[0].ID
		}
		if options[0].ParentSession != "" {
			p := options[0].ParentSession
			m.header.ParentSession = &p
		}
	}
	m.sessionID = m.header.ID
	return m
}
func NewSessionManager(string, *string, ...NewSessionOptions) (*SessionManager, error) {
	return nil, notImplemented("NewSessionManager")
}
func OpenSessionManager(string, *string, *string) (*SessionManager, error) {
	return nil, notImplemented("OpenSessionManager")
}
func ContinueRecentSessionManager(string, *string) (*SessionManager, error) {
	return nil, notImplemented("ContinueRecentSessionManager")
}
func ForkSessionManager(string, string, *string, ...NewSessionOptions) (*SessionManager, error) {
	return nil, notImplemented("ForkSessionManager")
}
func ListSessions(context.Context, string, ...SessionListOptions) ([]SessionInfo, error) {
	return nil, notImplemented("ListSessions")
}
func ListAllSessions(context.Context, ...SessionListOptions) ([]SessionInfo, error) {
	return nil, notImplemented("ListAllSessions")
}
func (m *SessionManager) GetCWD() string        { return m.cwd }
func (m *SessionManager) GetSessionDir() string { return m.sessionDir }
func (m *SessionManager) GetSessionID() string  { return m.sessionID }
func (m *SessionManager) GetSessionFile() *string {
	if m.sessionFile == "" {
		return nil
	}
	file := m.sessionFile
	return &file
}
func (m *SessionManager) GetLeafID() *string {
	if m.leafID == nil {
		return nil
	}
	v := *m.leafID
	return &v
}
func (m *SessionManager) GetLeafEntry() *SessionEntry {
	if m.leafID == nil {
		return nil
	}
	return m.GetEntry(*m.leafID)
}
func (m *SessionManager) GetEntry(id string) *SessionEntry {
	for i := range m.entries {
		if m.entries[i].ID == id {
			v := cloneSessionEntry(m.entries[i])
			return &v
		}
	}
	return nil
}
func (m *SessionManager) GetLabel(id string) *string {
	e := m.GetEntry(id)
	if e == nil || e.Label == nil {
		return nil
	}
	v := *e.Label
	return &v
}
func (m *SessionManager) GetBranch(fromID ...string) []SessionEntry {
	startID := m.leafID
	if len(fromID) > 0 {
		startID = &fromID[0]
	}
	if startID == nil || *startID == "" || m.GetEntry(*startID) == nil {
		return []SessionEntry{}
	}
	return buildContextPath(m.entries, startID, true)
}
func (m *SessionManager) BuildContextEntries() []SessionEntry {
	return BuildContextEntries(m.entries, m.leafID)
}
func (m *SessionManager) BuildSessionContext() SessionContext {
	return BuildSessionContext(m.entries, m.leafID)
}
func (m *SessionManager) GetHeader() *SessionHeader {
	header := cloneSessionHeader(m.header)
	return &header
}
func (m *SessionManager) GetEntries() []SessionEntry { return cloneSessionEntries(m.entries) }
func (m *SessionManager) GetTree() ([]SessionTreeNode, error) {
	return nil, notImplemented("SessionManager.GetTree")
}
func (m *SessionManager) GetSessionName() *string {
	for i := len(m.entries) - 1; i >= 0; i-- {
		if m.entries[i].Type == "session_info" {
			if m.entries[i].Name == nil {
				return nil
			}
			name := strings.TrimSpace(*m.entries[i].Name)
			if name == "" {
				return nil
			}
			return &name
		}
	}
	return nil
}
func (m *SessionManager) IsPersisted() bool           { return m.sessionFile != "" }
func (m *SessionManager) UsesDefaultSessionDir() bool { return false }
func (m *SessionManager) SetSessionFile(string) error {
	return notImplemented("SessionManager.SetSessionFile")
}
func (m *SessionManager) NewSession(...NewSessionOptions) (*string, error) {
	return nil, notImplemented("SessionManager.NewSession")
}
func (m *SessionManager) ResetLeaf() error { return notImplemented("SessionManager.ResetLeaf") }
func (m *SessionManager) AppendMessage(agent.AgentMessage) (string, error) {
	return "", notImplemented("SessionManager.AppendMessage")
}
func (m *SessionManager) AppendThinkingLevelChange(string) (string, error) {
	return "", notImplemented("SessionManager.AppendThinkingLevelChange")
}
func (m *SessionManager) AppendModelChange(string, string) (string, error) {
	return "", notImplemented("SessionManager.AppendModelChange")
}
func (m *SessionManager) AppendCompaction(string, string, int64, ...AppendCompactionOptions) (string, error) {
	return "", notImplemented("SessionManager.AppendCompaction")
}
func (m *SessionManager) AppendBranchSummary(BranchSummaryEntry) error {
	return notImplemented("SessionManager.AppendBranchSummary")
}
func (m *SessionManager) AppendCustomEntry(string, ...any) (string, error) {
	return "", notImplemented("SessionManager.AppendCustomEntry")
}
func (m *SessionManager) AppendCustomMessageEntry(string, ai.UserMessageContent, bool, ...json.RawMessage) (string, error) {
	return "", notImplemented("SessionManager.AppendCustomMessageEntry")
}
func (m *SessionManager) AppendLabelChange(string, *string) (string, error) {
	return "", notImplemented("SessionManager.AppendLabelChange")
}
func (m *SessionManager) AppendSessionInfo(string) (string, error) {
	return "", notImplemented("SessionManager.AppendSessionInfo")
}
func (m *SessionManager) Branch(string) error { return notImplemented("SessionManager.Branch") }
func (m *SessionManager) BranchWithSummary(*string, string, ...BranchSummaryOptions) (string, error) {
	return "", notImplemented("SessionManager.BranchWithSummary")
}
func (m *SessionManager) CreateBranchedSession(string) (*string, error) {
	return nil, notImplemented("SessionManager.CreateBranchedSession")
}
func (m *SessionManager) GetChildren(string) ([]SessionEntry, error) {
	return nil, notImplemented("SessionManager.GetChildren")
}

var fallbackSessionID uint64

func newSessionID() string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err == nil {
		bytes[6] = (bytes[6] & 0x0f) | 0x40
		bytes[8] = (bytes[8] & 0x3f) | 0x80
		return fmt.Sprintf("%x-%x-%x-%x-%x", bytes[0:4], bytes[4:6], bytes[6:8], bytes[8:10], bytes[10:16])
	}
	return fmt.Sprintf("session-%d-%d", time.Now().UTC().UnixNano(), atomic.AddUint64(&fallbackSessionID, 1))
}

func ParseSessionEntries(content string) []FileEntry {
	var out []FileEntry
	for _, line := range splitLines(content) {
		record := []byte(line)
		if !json.Valid(record) {
			continue
		}

		fileEntry := FileEntry{Raw: cloneRawMessage(record)}
		fields, ok := decodeJSONObject(record)
		if !ok {
			out = append(out, fileEntry)
			continue
		}

		fileEntry.Type, _ = decodeJSONField[string](fields, "type")
		fileEntry.ID, _ = decodeJSONField[string](fields, "id")
		fileEntry.Timestamp, _ = decodeJSONField[string](fields, "timestamp")
		if fileEntry.Type == "session" {
			header := decodeSessionHeader(fields)
			fileEntry.Header = &header
		} else if entry, err := decodeSessionEntry(record); err == nil {
			fileEntry.Entry = &entry
		}
		out = append(out, fileEntry)
	}
	return out
}

func decodeSessionEntry(data []byte) (SessionEntry, error) {
	fields, ok := decodeJSONObject(data)
	if !ok {
		return SessionEntry{}, fmt.Errorf("session entry must be a JSON object")
	}

	entry := SessionEntry{Raw: cloneRawMessage(data)}
	entry.Type, _ = decodeJSONField[string](fields, "type")
	entry.ID, _ = decodeJSONField[string](fields, "id")
	entry.ParentID, _ = decodeJSONField[*string](fields, "parentId")
	entry.Timestamp, _ = decodeJSONField[string](fields, "timestamp")
	entry.ThinkingLevel, _ = decodeJSONField[string](fields, "thinkingLevel")
	entry.Provider, _ = decodeJSONField[string](fields, "provider")
	entry.ModelID, _ = decodeJSONField[string](fields, "modelId")
	entry.Summary, _ = decodeJSONField[string](fields, "summary")
	entry.FirstKeptEntryID, _ = decodeJSONField[string](fields, "firstKeptEntryId")
	entry.TokensBefore, _ = decodeJSONField[int64](fields, "tokensBefore")
	entry.FromID, _ = decodeJSONField[string](fields, "fromId")
	entry.CustomType, _ = decodeJSONField[string](fields, "customType")
	entry.Data = cloneRawMessage(fields["data"])
	entry.Details = cloneRawMessage(fields["details"])
	entry.Display, _ = decodeJSONField[bool](fields, "display")
	entry.TargetID, _ = decodeJSONField[string](fields, "targetId")
	entry.Label, _ = decodeJSONField[*string](fields, "label")
	entry.Name, _ = decodeJSONField[*string](fields, "name")
	entry.Usage, _ = decodeJSONField[*ai.Usage](fields, "usage")
	entry.FromHook, _ = decodeJSONField[*bool](fields, "fromHook")

	if entry.Type == "compaction" {
		if firstKeptIndex, ok := decodeJSONField[int](fields, "firstKeptEntryIndex"); ok {
			entry.Data, _ = json.Marshal(firstKeptIndex)
		}
	}

	switch entry.Type {
	case "message":
		if message, err := unmarshalSessionAgentMessage(fields["message"]); err == nil {
			entry.Message = message
		}
	case "custom_message":
		content := ai.UserBlocks()
		rawContent := fields["content"]
		if len(rawContent) != 0 && string(rawContent) != "null" {
			if err := json.Unmarshal(rawContent, &content); err != nil {
				entry.Content = cloneRawMessage(rawContent)
				break
			}
		}
		entry.Content = content
	}

	return entry, nil
}

func unmarshalSessionAgentMessage(data json.RawMessage) (agent.AgentMessage, error) {
	messageFields, ok := decodeJSONObject(data)
	if !ok {
		return agent.UnmarshalAgentMessage(data)
	}

	role, ok := decodeJSONField[string](messageFields, "role")
	if ok && isBuiltInSessionMessageRole(role) {
		content, present := messageFields["content"]
		if !present || bytes.Equal(bytes.TrimSpace(content), []byte("null")) {
			messageFields["content"] = json.RawMessage("[]")
			normalized, err := json.Marshal(messageFields)
			if err != nil {
				return nil, err
			}
			data = normalized
		}
	}

	return agent.UnmarshalAgentMessage(data)
}

func isBuiltInSessionMessageRole(role string) bool {
	switch ai.MessageRole(role) {
	case ai.MessageRoleUser, ai.MessageRoleAssistant, ai.MessageRoleToolResult:
		return true
	default:
		return false
	}
}

func decodeSessionHeader(fields map[string]json.RawMessage) SessionHeader {
	header := SessionHeader{}
	header.Type, _ = decodeJSONField[string](fields, "type")
	header.Version, _ = decodeJSONField[*int](fields, "version")
	header.ID, _ = decodeJSONField[string](fields, "id")
	header.Timestamp, _ = decodeJSONField[string](fields, "timestamp")
	header.CWD, _ = decodeJSONField[string](fields, "cwd")
	header.ParentSession, _ = decodeJSONField[*string](fields, "parentSession")
	return header
}

func decodeJSONObject(data []byte) (map[string]json.RawMessage, bool) {
	trimmed := strings.TrimSpace(string(data))
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return nil, false
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, false
	}
	return fields, fields != nil
}

func decodeJSONField[T any](fields map[string]json.RawMessage, name string) (T, bool) {
	var zero T
	var value T
	raw, ok := fields[name]
	if !ok || json.Unmarshal(raw, &value) != nil {
		return zero, false
	}
	return value, true
}

func cloneRawMessage(raw json.RawMessage) json.RawMessage {
	if raw == nil {
		return nil
	}
	return append(json.RawMessage(nil), raw...)
}
func MigrateSessionEntries(entries []FileEntry) {
	version := 1
	for index := range entries {
		if entries[index].Header != nil && entries[index].Header.Type == "session" {
			if entries[index].Header.Version != nil {
				version = *entries[index].Header.Version
			}
			break
		}
	}
	if version >= CurrentSessionVersion {
		return
	}
	if version < 2 {
		migrateSessionV1ToV2(entries)
	}
	if version < 3 {
		migrateSessionV2ToV3(entries)
	}
}

func migrateSessionV1ToV2(entries []FileEntry) {
	used := make(map[string]struct{}, len(entries))
	var previousID *string
	for index := range entries {
		fileEntry := &entries[index]
		if fileEntry.Header != nil && fileEntry.Header.Type == "session" {
			version := 2
			fileEntry.Header.Version = &version
			continue
		}
		if fileEntry.Entry == nil {
			continue
		}

		id := generateMigratedSessionEntryID(used)
		used[id] = struct{}{}
		fileEntry.Entry.ID = id
		fileEntry.Entry.ParentID = cloneStringPointer(previousID)
		fileEntry.ID = id
		previousID = &id

		if fileEntry.Entry.Type != "compaction" || len(fileEntry.Entry.Data) == 0 {
			continue
		}
		var firstKeptIndex int
		if err := json.Unmarshal(fileEntry.Entry.Data, &firstKeptIndex); err == nil && firstKeptIndex >= 0 && firstKeptIndex < len(entries) {
			target := entries[firstKeptIndex]
			if target.Entry != nil && target.Entry.Type != "session" {
				fileEntry.Entry.FirstKeptEntryID = target.Entry.ID
			}
		}
		fileEntry.Entry.Data = nil
	}
}

func migrateSessionV2ToV3(entries []FileEntry) {
	for index := range entries {
		fileEntry := &entries[index]
		if fileEntry.Header != nil && fileEntry.Header.Type == "session" {
			version := 3
			fileEntry.Header.Version = &version
			continue
		}
		if fileEntry.Entry == nil || fileEntry.Entry.Type != "message" || fileEntry.Entry.Message == nil || fileEntry.Entry.Message.MessageRole() != "hookMessage" {
			continue
		}
		if message, ok := migrateSessionMessageRole(fileEntry.Entry.Message, "custom"); ok {
			fileEntry.Entry.Message = message
		}
	}
}

func generateMigratedSessionEntryID(used map[string]struct{}) string {
	for attempt := 0; attempt < 100; attempt++ {
		random := make([]byte, 4)
		if _, err := rand.Read(random); err != nil {
			break
		}
		id := fmt.Sprintf("%x", random)
		if _, exists := used[id]; !exists {
			return id
		}
	}
	for candidate := 0; ; candidate++ {
		id := fmt.Sprintf("%08x", candidate)
		if _, exists := used[id]; !exists {
			return id
		}
	}
}

func migrateSessionMessageRole(message agent.AgentMessage, role string) (agent.AgentMessage, bool) {
	encoded, err := agent.MarshalAgentMessage(message)
	if err != nil {
		return nil, false
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &object); err != nil {
		return nil, false
	}
	object["role"], _ = json.Marshal(role)
	encoded, err = json.Marshal(object)
	if err != nil {
		return nil, false
	}
	migrated, err := agent.UnmarshalAgentMessage(encoded)
	return migrated, err == nil
}

func cloneStringPointer(value *string) *string {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}
func GetLatestCompactionEntry(entries []SessionEntry) *CompactionEntry {
	for i := len(entries) - 1; i >= 0; i-- {
		if entries[i].Type == "compaction" {
			e := cloneSessionEntry(entries[i])
			return &CompactionEntry{SessionEntryBase: e.SessionEntryBase, Summary: e.Summary, FirstKeptEntryID: e.FirstKeptEntryID, TokensBefore: e.TokensBefore, Details: e.Details, Usage: e.Usage, FromHook: e.FromHook}
		}
	}
	return nil
}
func SessionEntryToContextMessages(entry SessionEntry) []agent.AgentMessage {
	switch entry.Type {
	case "message":
		if entry.Message != nil {
			return []agent.AgentMessage{cloneSessionAgentMessage(entry.Message)}
		}
	case "custom_message":
		content, ok := sessionCustomMessageContent(entry.Content)
		if !ok {
			content = ai.UserBlocks()
		}
		return []agent.AgentMessage{agent.CreateCustomMessage(
			entry.CustomType, content, entry.Display, sessionMessageDetails(entry.Details), sessionTimestampMillis(entry.Timestamp),
		)}
	case "branch_summary":
		if entry.Summary != "" {
			return []agent.AgentMessage{agent.CreateBranchSummaryMessage(entry.Summary, entry.FromID, sessionTimestampMillis(entry.Timestamp))}
		}
	case "compaction":
		return []agent.AgentMessage{agent.CreateCompactionSummaryMessage(entry.Summary, entry.TokensBefore, sessionTimestampMillis(entry.Timestamp))}
	}
	return nil
}
func buildContextPath(entries []SessionEntry, leafID *string, leafSpecified bool) []SessionEntry {
	if len(entries) == 0 {
		return []SessionEntry{}
	}
	if leafSpecified && leafID == nil {
		return []SessionEntry{}
	}
	byID := map[string]SessionEntry{}
	for _, e := range entries {
		byID[e.ID] = e
	}
	leaf := entries[len(entries)-1]
	if leafID != nil && *leafID != "" {
		if e, ok := byID[*leafID]; ok {
			leaf = e
		}
	}
	var rev []SessionEntry
	seen := map[string]bool{}
	for {
		if seen[leaf.ID] {
			break
		}
		seen[leaf.ID] = true
		rev = append(rev, cloneSessionEntry(leaf))
		if leaf.ParentID == nil {
			break
		}
		n, ok := byID[*leaf.ParentID]
		if !ok {
			break
		}
		leaf = n
	}
	for i, j := 0, len(rev)-1; i < j; i, j = i+1, j-1 {
		rev[i], rev[j] = rev[j], rev[i]
	}
	return rev
}
func BuildContextEntries(entries []SessionEntry, leafID ...*string) []SessionEntry {
	var leaf *string
	specified := false
	if len(leafID) > 0 {
		leaf = leafID[0]
		specified = true
	}
	path := buildContextPath(entries, leaf, specified)
	compactionIndex := -1
	for index := range path {
		if path[index].Type == "compaction" {
			compactionIndex = index
		}
	}
	if compactionIndex < 0 {
		return path
	}

	compaction := path[compactionIndex]
	contextEntries := make([]SessionEntry, 0, len(path)-compactionIndex+1)
	contextEntries = append(contextEntries, cloneSessionEntry(compaction))
	foundFirstKept := false
	for index := 0; index < compactionIndex; index++ {
		if path[index].ID == compaction.FirstKeptEntryID {
			foundFirstKept = true
		}
		if foundFirstKept {
			contextEntries = append(contextEntries, cloneSessionEntry(path[index]))
		}
	}
	contextEntries = append(contextEntries, cloneSessionEntries(path[compactionIndex+1:])...)
	return contextEntries
}
func BuildSessionContext(entries []SessionEntry, leafID ...*string) SessionContext {
	var leaf *string
	specified := false
	if len(leafID) > 0 {
		leaf = leafID[0]
		specified = true
	}
	path := buildContextPath(entries, leaf, specified)
	out := SessionContext{ThinkingLevel: "off", Messages: []agent.AgentMessage{}}
	for _, e := range path {
		switch e.Type {
		case "thinking_level_change":
			out.ThinkingLevel = e.ThinkingLevel
		case "model_change":
			out.Model = &SessionModel{Provider: e.Provider, ModelID: e.ModelID}
		case "message":
			if assistant, ok := sessionAssistantMessage(e.Message); ok {
				out.Model = &SessionModel{Provider: string(assistant.Provider), ModelID: assistant.Model}
			}
		}
	}
	for _, entry := range BuildContextEntries(entries, leafID...) {
		out.Messages = append(out.Messages, SessionEntryToContextMessages(entry)...)
	}
	return out
}

func sessionAssistantMessage(message agent.AgentMessage) (ai.AssistantMessage, bool) {
	switch message := message.(type) {
	case ai.AssistantMessage:
		return message, true
	case *ai.AssistantMessage:
		if message != nil {
			return *message, true
		}
	}
	return ai.AssistantMessage{}, false
}

func sessionCustomMessageContent(content any) (ai.UserMessageContent, bool) {
	switch content := content.(type) {
	case ai.UserMessageContent:
		encoded, err := json.Marshal(content)
		if err != nil {
			return ai.UserMessageContent{}, false
		}
		var cloned ai.UserMessageContent
		if err := json.Unmarshal(encoded, &cloned); err != nil {
			return ai.UserMessageContent{}, false
		}
		return cloned, true
	case string:
		return ai.UserText(content), true
	case []ai.UserContent:
		return ai.UserBlocks(content...), true
	case json.RawMessage:
		var decoded ai.UserMessageContent
		if err := json.Unmarshal(content, &decoded); err == nil {
			return decoded, true
		}
	}
	return ai.UserMessageContent{}, false
}

func sessionMessageDetails(raw json.RawMessage) ai.Optional[ai.JSONValue] {
	if len(raw) == 0 {
		return ai.Absent[ai.JSONValue]()
	}
	var details ai.JSONValue
	if err := json.Unmarshal(raw, &details); err != nil {
		return ai.Absent[ai.JSONValue]()
	}
	if details == nil {
		return ai.Null[ai.JSONValue]()
	}
	return ai.Some(details)
}

func sessionTimestampMillis(timestamp string) int64 {
	parsed, err := time.Parse(time.RFC3339Nano, timestamp)
	if err != nil {
		return 0
	}
	return parsed.UnixMilli()
}

func cloneSessionAgentMessage(message agent.AgentMessage) agent.AgentMessage {
	if message == nil {
		return nil
	}
	encoded, err := agent.MarshalAgentMessage(message)
	if err != nil {
		return message
	}
	cloned, err := agent.UnmarshalAgentMessage(encoded)
	if err != nil {
		return message
	}
	return cloned
}

func cloneSessionHeader(header SessionHeader) SessionHeader {
	clone := header
	if header.Version != nil {
		value := *header.Version
		clone.Version = &value
	}
	clone.ParentSession = cloneStringPointer(header.ParentSession)
	return clone
}
func splitLines(s string) []string {
	var out []string
	start := 0
	for i, c := range s {
		if c == '\n' {
			if i > start {
				out = append(out, s[start:i])
			}
			start = i + 1
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}
func cloneSessionEntry(e SessionEntry) SessionEntry {
	e.Raw = cloneRawMessage(e.Raw)
	if e.ParentID != nil {
		value := *e.ParentID
		e.ParentID = &value
	}
	e.Message = cloneSessionAgentMessage(e.Message)
	e.Data = append(json.RawMessage(nil), e.Data...)
	e.Details = append(json.RawMessage(nil), e.Details...)
	if raw, ok := e.Content.(json.RawMessage); ok {
		e.Content = cloneRawMessage(raw)
	} else if content, ok := sessionCustomMessageContent(e.Content); ok {
		e.Content = content
	}
	if e.Label != nil {
		value := *e.Label
		e.Label = &value
	}
	if e.Name != nil {
		value := *e.Name
		e.Name = &value
	}
	if e.Usage != nil {
		value := *e.Usage
		e.Usage = &value
	}
	if e.FromHook != nil {
		value := *e.FromHook
		e.FromHook = &value
	}
	return e
}
func cloneSessionEntries(in []SessionEntry) []SessionEntry {
	out := make([]SessionEntry, len(in))
	for i := range in {
		out[i] = cloneSessionEntry(in[i])
	}
	return out
}

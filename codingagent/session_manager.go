package codingagent

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
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
	mu                                      sync.RWMutex
	cwd, sessionDir, sessionID, sessionFile string
	header                                  SessionHeader
	entries                                 []SessionEntry
	leafID                                  *string
	flushed                                 bool
}

// NewInMemorySessionManager creates only in-memory state and performs no I/O.
func NewInMemorySessionManager(cwd string, options ...NewSessionOptions) *SessionManager {
	m := &SessionManager{
		cwd:     cwd,
		entries: []SessionEntry{},
		header: SessionHeader{
			Type:      "session",
			ID:        newSessionID(),
			Timestamp: sessionTimestamp(),
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
func NewSessionManager(cwd string, sessionDir *string, options ...NewSessionOptions) (*SessionManager, error) {
	option := NewSessionOptions{}
	if len(options) != 0 {
		option = options[0]
	}
	if option.ID != "" && !customSessionID.MatchString(option.ID) {
		return nil, fmt.Errorf("Session id must be non-empty, contain only alphanumeric characters, '-', '_', and '.', and start and end with an alphanumeric character")
	}
	resolvedCWD, err := resolveSessionPath(cwd)
	if err != nil {
		return nil, fmt.Errorf("resolve session cwd: %w", err)
	}
	dir, _, err := resolveSessionDir(resolvedCWD, sessionDir)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create session directory %q: %w", dir, err)
	}
	manager := NewInMemorySessionManager(resolvedCWD, option)
	manager.sessionDir = dir
	manager.sessionFile = filepath.Join(dir, sessionFileName(manager.header.Timestamp, manager.sessionID))
	return manager, nil
}
func OpenSessionManager(path string, sessionDir, cwdOverride *string) (*SessionManager, error) {
	resolvedPath, err := resolveSessionPath(path)
	if err != nil {
		return nil, fmt.Errorf("resolve session file: %w", err)
	}
	parsed, size, readErr := loadSessionFile(resolvedPath)
	if readErr != nil && !os.IsNotExist(readErr) {
		return nil, fmt.Errorf("read session file %q: %w", resolvedPath, readErr)
	}

	var header SessionHeader
	var entries []SessionEntry
	migrated := false
	if readErr == nil && size != 0 {
		if len(parsed) == 0 {
			return nil, fmt.Errorf("Session file is not a valid pi session: %s", resolvedPath)
		}
		headerFields, headerObject := decodeJSONObject(parsed[0].Raw)
		_, hasStringID := decodeJSONField[string](headerFields, "id")
		hasStringID = hasStringID && !bytes.Equal(bytes.TrimSpace(headerFields["id"]), []byte("null"))
		if !headerObject || !hasStringID || parsed[0].Header == nil || parsed[0].Header.Type != "session" {
			return nil, fmt.Errorf("Session file is not a valid pi session: %s", resolvedPath)
		}
		migrated = parsed[0].Header.Version == nil || *parsed[0].Header.Version < CurrentSessionVersion
		MigrateSessionEntries(parsed)
		header = cloneSessionHeader(*parsed[0].Header)
		for _, item := range parsed[1:] {
			if item.Entry != nil {
				entries = append(entries, cloneSessionEntry(*item.Entry))
			}
		}
	}

	cwd := ""
	if cwdOverride != nil {
		cwd = *cwdOverride
	} else if header.CWD != "" {
		cwd = header.CWD
	}
	if cwd == "" {
		cwd, err = os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("resolve session cwd: %w", err)
		}
	}
	resolvedCWD, err := resolveSessionPath(cwd)
	if err != nil {
		return nil, fmt.Errorf("resolve session cwd: %w", err)
	}
	dir := filepath.Dir(resolvedPath)
	if sessionDir != nil {
		dir, err = resolveSessionPath(*sessionDir)
		if err != nil {
			return nil, fmt.Errorf("resolve session directory: %w", err)
		}
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create session directory %q: %w", dir, err)
	}

	if readErr != nil || size == 0 {
		manager := NewInMemorySessionManager(resolvedCWD)
		manager.sessionDir = dir
		manager.sessionFile = resolvedPath
		if readErr == nil {
			manager.flushed = true
			if err := manager.rewriteFile(); err != nil {
				return nil, err
			}
		}
		return manager, nil
	}

	manager := &SessionManager{
		cwd: resolvedCWD, sessionDir: dir, sessionID: header.ID, sessionFile: resolvedPath,
		header: header, entries: entries, flushed: true,
	}
	if len(entries) != 0 {
		leaf := entries[len(entries)-1].ID
		manager.leafID = &leaf
	}
	if migrated {
		var data bytes.Buffer
		for _, item := range parsed {
			data.Write(item.Raw)
			data.WriteByte('\n')
		}
		if err := os.WriteFile(resolvedPath, data.Bytes(), 0o600); err != nil {
			return nil, fmt.Errorf("persist session %q: %w", resolvedPath, err)
		}
	}
	return manager, nil
}

func loadSessionFile(path string) ([]FileEntry, int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, 0, err
	}
	var entries []FileEntry
	reader := bufio.NewReader(file)
	for {
		line, err := reader.ReadString('\n')
		for _, entry := range ParseSessionEntries(line) {
			if entry.Header == nil && entry.Entry == nil {
				var value any
				if json.Unmarshal(entry.Raw, &value) == nil && (value == nil || value == false || value == float64(0) || value == "") {
					continue
				}
			}
			entries = append(entries, entry)
		}
		if err == io.EOF {
			return entries, info.Size(), nil
		}
		if err != nil {
			return nil, 0, err
		}
	}
}

func (m *SessionManager) GetCWD() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.cwd
}
func (m *SessionManager) GetSessionDir() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sessionDir
}
func (m *SessionManager) GetSessionID() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sessionID
}
func (m *SessionManager) GetSessionFile() *string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.sessionFile == "" {
		return nil
	}
	file := m.sessionFile
	return &file
}
func (m *SessionManager) GetLeafID() *string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.leafID == nil {
		return nil
	}
	v := *m.leafID
	return &v
}
func (m *SessionManager) GetLeafEntry() *SessionEntry {
	leafID := m.GetLeafID()
	if leafID == nil {
		return nil
	}
	return m.GetEntry(*leafID)
}
func (m *SessionManager) GetEntry(id string) *SessionEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for i := range m.entries {
		if m.entries[i].ID == id {
			v := cloneSessionEntry(m.entries[i])
			return &v
		}
	}
	return nil
}
func (m *SessionManager) GetBranch(fromID ...string) []SessionEntry {
	m.mu.RLock()
	entries := cloneSessionEntries(m.entries)
	startID := cloneStringPointer(m.leafID)
	m.mu.RUnlock()
	if len(fromID) > 0 {
		startID = &fromID[0]
	}
	if startID == nil || *startID == "" {
		return []SessionEntry{}
	}
	found := false
	for i := range entries {
		if entries[i].ID == *startID {
			found = true
			break
		}
	}
	if !found {
		return []SessionEntry{}
	}
	return buildContextPath(entries, startID, true)
}
func (m *SessionManager) BuildContextEntries() []SessionEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return BuildContextEntries(m.entries, m.leafID)
}
func (m *SessionManager) BuildSessionContext() SessionContext {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return BuildSessionContext(m.entries, m.leafID)
}
func (m *SessionManager) GetHeader() *SessionHeader {
	m.mu.RLock()
	defer m.mu.RUnlock()
	header := cloneSessionHeader(m.header)
	return &header
}
func (m *SessionManager) GetEntries() []SessionEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return cloneSessionEntries(m.entries)
}
func (m *SessionManager) GetSessionName() *string {
	m.mu.RLock()
	defer m.mu.RUnlock()
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
func (m *SessionManager) IsPersisted() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sessionFile != ""
}
func (m *SessionManager) UsesDefaultSessionDir() bool {
	m.mu.RLock()
	cwd, dir := m.cwd, m.sessionDir
	m.mu.RUnlock()
	defaultDir, _, err := resolveSessionDir(cwd, nil)
	return err == nil && dir == defaultDir
}

func (m *SessionManager) AppendMessage(message agent.AgentMessage) (string, error) {
	encoded, err := agent.MarshalAgentMessage(message)
	if err != nil {
		return "", fmt.Errorf("append session message: %w", err)
	}
	owned, err := agent.UnmarshalAgentMessage(encoded)
	if err != nil {
		return "", fmt.Errorf("append session message: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	entry := m.newEntryLocked("message")
	entry.Message = owned
	return entry.ID, m.appendEntryLocked(entry)
}
func (m *SessionManager) AppendThinkingLevelChange(thinkingLevel string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	entry := m.newEntryLocked("thinking_level_change")
	entry.ThinkingLevel = thinkingLevel
	return entry.ID, m.appendEntryLocked(entry)
}
func (m *SessionManager) AppendModelChange(provider, modelID string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	entry := m.newEntryLocked("model_change")
	entry.Provider, entry.ModelID = provider, modelID
	return entry.ID, m.appendEntryLocked(entry)
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
func (m *SessionManager) AppendSessionInfo(name string) (string, error) {
	name = strings.TrimSpace(regexp.MustCompile(`[\r\n]+`).ReplaceAllString(name, " "))
	m.mu.Lock()
	defer m.mu.Unlock()
	entry := m.newEntryLocked("session_info")
	entry.Name = &name
	return entry.ID, m.appendEntryLocked(entry)
}
func (m *SessionManager) newEntryLocked(entryType string) SessionEntry {
	used := make(map[string]struct{}, len(m.entries))
	for i := range m.entries {
		used[m.entries[i].ID] = struct{}{}
	}
	return SessionEntry{SessionEntryBase: SessionEntryBase{
		Type: entryType, ID: generateMigratedSessionEntryID(used), ParentID: cloneStringPointer(m.leafID), Timestamp: sessionTimestamp(),
	}}
}

func (m *SessionManager) appendEntryLocked(entry SessionEntry) error {
	m.entries = append(m.entries, entry)
	leaf := entry.ID
	m.leafID = &leaf
	return m.persistEntryLocked(entry)
}

func (m *SessionManager) persistEntryLocked(entry SessionEntry) error {
	if m.sessionFile == "" {
		return nil
	}
	hasAssistant := false
	for i := range m.entries {
		if m.entries[i].Type == "message" {
			if _, ok := sessionAssistantMessage(m.entries[i].Message); ok {
				hasAssistant = true
				break
			}
		}
	}
	if !hasAssistant {
		if m.flushed {
			return appendSessionRecord(m.sessionFile, entry)
		}
		return nil
	}
	if m.flushed {
		return appendSessionRecord(m.sessionFile, entry)
	}
	data, err := encodeSessionFile(m.header, m.entries)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(m.sessionFile, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("persist session %q: %w", m.sessionFile, err)
	}
	_, writeErr := file.Write(data)
	closeErr := file.Close()
	if writeErr != nil {
		return fmt.Errorf("persist session %q: %w", m.sessionFile, writeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("persist session %q: %w", m.sessionFile, closeErr)
	}
	m.flushed = true
	return nil
}

func (m *SessionManager) rewriteFile() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	data, err := encodeSessionFile(m.header, m.entries)
	if err != nil {
		return err
	}
	if err := os.WriteFile(m.sessionFile, data, 0o600); err != nil {
		return fmt.Errorf("persist session %q: %w", m.sessionFile, err)
	}
	return nil
}

func appendSessionRecord(path string, entry SessionEntry) error {
	data, err := marshalSessionEntry(entry)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("persist session %q: %w", path, err)
	}
	_, writeErr := file.Write(data)
	closeErr := file.Close()
	if writeErr != nil {
		return fmt.Errorf("persist session %q: %w", path, writeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("persist session %q: %w", path, closeErr)
	}
	return nil
}

func encodeSessionFile(header SessionHeader, entries []SessionEntry) ([]byte, error) {
	var out bytes.Buffer
	headerData, err := json.Marshal(header)
	if err != nil {
		return nil, fmt.Errorf("encode session header: %w", err)
	}
	out.Write(headerData)
	out.WriteByte('\n')
	for _, entry := range entries {
		data, err := marshalSessionEntry(entry)
		if err != nil {
			return nil, err
		}
		out.Write(data)
		out.WriteByte('\n')
	}
	return out.Bytes(), nil
}

func marshalSessionEntry(entry SessionEntry) ([]byte, error) {
	if len(entry.Raw) != 0 {
		return cloneRawMessage(entry.Raw), nil
	}
	base := struct {
		Type      string  `json:"type"`
		ID        string  `json:"id"`
		ParentID  *string `json:"parentId"`
		Timestamp string  `json:"timestamp"`
	}{entry.Type, entry.ID, entry.ParentID, entry.Timestamp}
	switch entry.Type {
	case "label", "branch_summary":
		fields := map[string]any{"type": base.Type, "id": base.ID, "parentId": base.ParentID, "timestamp": base.Timestamp}
		if entry.Type == "label" {
			fields["targetId"] = entry.TargetID
			if entry.Label != nil {
				fields["label"] = *entry.Label
			}
		} else {
			fields["fromId"] = entry.FromID
			fields["summary"] = entry.Summary
			if len(entry.Details) > 0 {
				fields["details"] = entry.Details
			}
			if entry.Usage != nil {
				fields["usage"] = entry.Usage
			}
			if entry.FromHook != nil {
				fields["fromHook"] = entry.FromHook
			}
		}
		return json.Marshal(fields)
	case "session_info":
		return json.Marshal(struct {
			Type      string  `json:"type"`
			ID        string  `json:"id"`
			ParentID  *string `json:"parentId"`
			Timestamp string  `json:"timestamp"`
			Name      *string `json:"name,omitempty"`
		}{base.Type, base.ID, base.ParentID, base.Timestamp, entry.Name})
	case "message":
		message, err := agent.MarshalAgentMessage(entry.Message)
		if err != nil {
			return nil, fmt.Errorf("encode session message: %w", err)
		}
		return json.Marshal(struct {
			Type      string          `json:"type"`
			ID        string          `json:"id"`
			ParentID  *string         `json:"parentId"`
			Timestamp string          `json:"timestamp"`
			Message   json.RawMessage `json:"message"`
		}{base.Type, base.ID, base.ParentID, base.Timestamp, message})
	case "model_change":
		return json.Marshal(struct {
			Type      string  `json:"type"`
			ID        string  `json:"id"`
			ParentID  *string `json:"parentId"`
			Timestamp string  `json:"timestamp"`
			Provider  string  `json:"provider"`
			ModelID   string  `json:"modelId"`
		}{base.Type, base.ID, base.ParentID, base.Timestamp, entry.Provider, entry.ModelID})
	case "thinking_level_change":
		return json.Marshal(struct {
			Type          string  `json:"type"`
			ID            string  `json:"id"`
			ParentID      *string `json:"parentId"`
			Timestamp     string  `json:"timestamp"`
			ThinkingLevel string  `json:"thinkingLevel"`
		}{base.Type, base.ID, base.ParentID, base.Timestamp, entry.ThinkingLevel})
	default:
		if len(entry.Raw) != 0 {
			return cloneRawMessage(entry.Raw), nil
		}
		return nil, fmt.Errorf("encode session entry type %q: unsupported", entry.Type)
	}
}

func resolveSessionDir(cwd string, explicit *string) (string, bool, error) {
	if explicit != nil {
		dir, err := resolveSessionPath(*explicit)
		if err != nil {
			return "", false, fmt.Errorf("resolve session directory: %w", err)
		}
		return dir, false, nil
	}
	agentDir, err := GetAgentDir()
	if err != nil {
		return "", false, err
	}
	safe := strings.NewReplacer("/", "-", `\`, "-", ":", "-").Replace(strings.TrimLeft(cwd, `/\`))
	return filepath.Join(agentDir, "sessions", "--"+safe+"--"), true, nil
}

func resolveSessionPath(path string) (string, error) {
	if path == "~" || strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		path = filepath.Join(home, strings.TrimLeft(path[1:], `/\`))
	}
	return filepath.Abs(path)
}

func sessionTimestamp() string {
	return time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
}

func sessionFileName(timestamp, id string) string {
	return strings.NewReplacer(":", "-", ".", "-").Replace(timestamp) + "_" + id + ".jsonl"
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
	for index := range entries {
		item := &entries[index]
		fields, ok := decodeJSONObject(item.Raw)
		if !ok {
			continue
		}
		if item.Header != nil {
			fields["version"] = json.RawMessage("3")
		} else if item.Entry != nil {
			if version < 2 {
				fields["id"], _ = json.Marshal(item.Entry.ID)
				fields["parentId"], _ = json.Marshal(item.Entry.ParentID)
				if item.Type == "compaction" {
					if _, ok := decodeJSONField[float64](fields, "firstKeptEntryIndex"); ok {
						delete(fields, "firstKeptEntryIndex")
						if item.Entry.FirstKeptEntryID != "" {
							fields["firstKeptEntryId"], _ = json.Marshal(item.Entry.FirstKeptEntryID)
						}
					}
				}
			}
			if item.Type == "message" {
				if message, ok := decodeJSONObject(fields["message"]); ok {
					if role, _ := decodeJSONField[string](message, "role"); role == "hookMessage" {
						message["role"] = json.RawMessage(`"custom"`)
						fields["message"], _ = json.Marshal(message)
					}
				}
			}
		}
		item.Raw, _ = json.Marshal(fields)
		if item.Entry != nil {
			item.Entry.Raw = cloneRawMessage(item.Raw)
		}
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

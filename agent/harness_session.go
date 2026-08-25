package agent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/nankedr/pig/ai"
)

type SessionContextModel struct {
	Provider string
	ModelID  string
}

type SessionContext struct {
	Messages        []AgentMessage
	ThinkingLevel   string
	Model           *SessionContextModel
	ActiveToolNames []string
}

type ContextEntryTransform func([]Entry) []Entry

type CustomEntryContextMessageProjector func(CustomEntry, int, []Entry) []AgentMessage

type SessionContextBuildOptions struct {
	EntryTransforms []ContextEntryTransform
	EntryProjectors map[string]CustomEntryContextMessageProjector
}

func DefaultContextEntryTransform(pathEntries []Entry) []Entry {
	for index := len(pathEntries) - 1; index >= 0; index-- {
		if pathEntries[index].EntryType() == EntryTypeCompaction {
			entries := make([]Entry, 1, len(pathEntries)-index)
			entries[0] = pathEntries[index]
			return append(entries, pathEntries[index+1:]...)
		}
	}
	return append([]Entry(nil), pathEntries...)
}

func BuildContextEntries(pathEntries []Entry, options ...SessionContextBuildOptions) []Entry {
	entries := DefaultContextEntryTransform(pathEntries)
	if len(options) == 0 {
		return entries
	}
	for _, transform := range options[0].EntryTransforms {
		if transform != nil {
			entries = append([]Entry(nil), transform(append([]Entry(nil), entries...))...)
		}
	}
	return entries
}

func SessionEntryToContextMessages(entry Entry, index int, entries []Entry, options ...SessionContextBuildOptions) []AgentMessage {
	switch entry := entry.(type) {
	case MessageEntry:
		if assistant, ok := assistantMessage(entry.Message); ok && assistant.StopReason == ai.StopReasonDeferred {
			return nil
		}
		return []AgentMessage{entry.Message}
	case CompactionEntry:
		messages := make([]AgentMessage, 1, len(entry.RetainedTail)+1)
		messages[0] = CreateCompactionSummaryMessage(entry.Summary, entry.TokensBefore, entry.Timestamp)
		return append(messages, entry.RetainedTail...)
	case BranchSummaryEntry:
		if entry.Summary != "" {
			return []AgentMessage{CreateBranchSummaryMessage(entry.Summary, entry.FromID, entry.Timestamp)}
		}
	case CustomEntry:
		if len(options) != 0 {
			if projector := options[0].EntryProjectors[entry.CustomType]; projector != nil {
				return append([]AgentMessage(nil), projector(entry, index, append([]Entry(nil), entries...))...)
			}
		}
	}
	return nil
}

func BuildSessionContext(pathEntries []Entry, options ...SessionContextBuildOptions) SessionContext {
	result := SessionContext{ThinkingLevel: "off"}
	for _, entry := range pathEntries {
		switch entry := entry.(type) {
		case ThinkingLevelEntry:
			result.ThinkingLevel = entry.ThinkingLevel
		case ModelChangeEntry:
			result.Model = &SessionContextModel{Provider: entry.Provider, ModelID: entry.ModelID}
		case MessageEntry:
			if assistant, ok := assistantMessage(entry.Message); ok {
				result.Model = &SessionContextModel{Provider: string(assistant.Provider), ModelID: assistant.Model}
			}
		case ActiveToolsEntry:
			result.ActiveToolNames = append([]string(nil), entry.ActiveToolNames...)
		}
	}
	entries := BuildContextEntries(pathEntries, options...)
	for index, entry := range entries {
		result.Messages = append(result.Messages, SessionEntryToContextMessages(entry, index, entries, options...)...)
	}
	return result
}

func assistantMessage(message AgentMessage) (ai.AssistantMessage, bool) {
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

type SessionSearchOptions struct {
	Text   string
	CWD    string
	CWDSet bool
}

type SessionSearchHit struct {
	Metadata   SessionMetadata
	EntryID    string
	Timestamp  string
	Snippet    string
	SnippetSet bool
	Score      float64
	ScoreSet   bool
}

type SessionSearch interface {
	Search(context.Context, SessionSearchOptions) ([]SessionSearchHit, error)
}

type scanningSessionSearch struct {
	source SessionRepo
}

func CreateScanningSessionSearch(source SessionRepo) SessionSearch {
	return &scanningSessionSearch{source: source}
}

func (s *scanningSessionSearch) Search(ctx context.Context, options SessionSearchOptions) ([]SessionSearchHit, error) {
	if err := requireContext(ctx); err != nil {
		return nil, err
	}
	text := strings.ToLower(strings.TrimSpace(options.Text))
	if text == "" {
		return []SessionSearchHit{}, nil
	}
	if options.CWDSet || options.CWD != "" {
		return nil, newNotImplemented("SessionSearch.Search.cwd")
	}
	metadata, err := s.source.List(ctx)
	if err != nil {
		return nil, err
	}
	hits := make([]SessionSearchHit, 0)
	for _, item := range metadata {
		session, err := s.source.Open(ctx, item)
		if err != nil {
			return nil, err
		}
		entries, err := session.FindEntries(ctx, EntryQuery{Order: EntryOrderOldestFirst})
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			payload, err := json.Marshal(entry)
			if err != nil {
				return nil, err
			}
			if !strings.Contains(strings.ToLower(string(payload)), text) {
				continue
			}
			base := entry.entryBase()
			hits = append(hits, SessionSearchHit{
				Metadata:   item,
				EntryID:    base.ID,
				Timestamp:  time.UnixMilli(base.Timestamp).UTC().Format("2006-01-02T15:04:05.000Z07:00"),
				Snippet:    string(payload),
				SnippetSet: true,
			})
		}
	}
	return hits, nil
}

func GetFileSystemResultOrThrow[T any](result Result[T], message string) (T, error) {
	if result.OK {
		return result.Value, nil
	}
	cause := result.Error
	if cause == nil {
		cause = errors.New("file operation failed")
	}
	code := SessionErrorStorage
	var fileError *FileError
	if errors.As(cause, &fileError) && fileError.Code == FileErrorNotFound {
		code = SessionErrorNotFound
	}
	var zero T
	return zero, &SessionError{Code: code, Message: fmt.Sprintf("%s: %s", message, cause), Cause: cause}
}

type randomIDGenerator struct{}

func (randomIDGenerator) Next() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(value[:])
}

type Session struct {
	storage     SessionStorage
	IDGenerator IDGenerator
	lane        string
}

func NewSession(storage SessionStorage, generators ...IDGenerator) *Session {
	generator := IDGenerator(randomIDGenerator{})
	if len(generators) > 0 && generators[0] != nil {
		generator = generators[0]
	}
	return &Session{storage: storage, IDGenerator: generator, lane: "main"}
}

func AssertJSONSerializable(value any) error {
	if _, err := json.Marshal(value); err != nil {
		return &SessionError{Code: SessionErrorInvalidPayload, Message: "Durable payload is not JSON serializable", Cause: err}
	}
	return nil
}

func (s *Session) GetMetadata(ctx context.Context) (SessionMetadata, error) {
	return s.storage.GetMetadata(ctx)
}

func (s *Session) View(lane string) SessionTree {
	if lane == "main" {
		return s
	}
	return &sessionView{session: s, lane: lane}
}

func (s *Session) GetLeafID(ctx context.Context) (string, bool, error) {
	return s.getLeafIDForLane(ctx, s.lane)
}

func (s *Session) GetEntry(ctx context.Context, id string) (Entry, bool, error) {
	return s.storage.GetEntry(ctx, id)
}

func (s *Session) GetStats(ctx context.Context) (SessionStats, error) {
	return s.storage.GetStats(ctx)
}

func (s *Session) GetName(ctx context.Context) (string, bool, error) {
	return s.storage.GetName(ctx)
}

func (s *Session) SetName(ctx context.Context, name string, set bool) error {
	return s.storage.SetName(ctx, name, set)
}

func (s *Session) GetLabel(ctx context.Context, targetID string) (string, bool, error) {
	return s.storage.GetLabel(ctx, targetID)
}

func (s *Session) SetLabel(ctx context.Context, targetID, label string, set bool) error {
	return s.storage.SetLabel(ctx, targetID, label, set)
}

func (s *Session) FindEntries(ctx context.Context, query EntryQuery) ([]Entry, error) {
	if err := validateEntryQuery(query); err != nil {
		return nil, err
	}
	return s.storage.FindEntries(ctx, query)
}

func (s *Session) FindEntry(ctx context.Context, query EntryQuery) (Entry, bool, error) {
	query.Limit = 1
	entries, err := s.FindEntries(ctx, query)
	if err != nil || len(entries) == 0 {
		return nil, false, err
	}
	return entries[0], true, nil
}

func (s *Session) FindEntriesOnBranch(ctx context.Context, query EntryQuery, bounds BranchBounds) ([]Entry, error) {
	if err := validateEntryQuery(query); err != nil {
		return nil, err
	}
	if !bounds.StartSet {
		leafID, ok, err := s.getLeafIDForLane(ctx, s.lane)
		if err != nil || !ok {
			return nil, err
		}
		bounds.Start, bounds.StartSet = leafID, true
	}
	return s.storage.FindEntriesOnBranch(ctx, query, bounds)
}

func (s *Session) FindEntryOnBranch(ctx context.Context, query EntryQuery, bounds BranchBounds) (Entry, bool, error) {
	query.Limit = 1
	entries, err := s.FindEntriesOnBranch(ctx, query, bounds)
	if err != nil || len(entries) == 0 {
		return nil, false, err
	}
	return entries[0], true, nil
}

func (s *Session) AppendMessage(ctx context.Context, message AgentMessage) (string, error) {
	entry, err := s.AppendEntry(ctx, MessageEntry{EntryBase: EntryBase{ID: s.IDGenerator.Next()}, Message: message}, s.lane)
	if err != nil {
		return "", err
	}
	return entry.entryBase().ID, nil
}

func (s *Session) AppendCustomEntry(ctx context.Context, customType string, data ...JSONValue) (string, error) {
	entry := CustomEntry{EntryBase: EntryBase{ID: s.IDGenerator.Next()}, CustomType: customType}
	if len(data) > 0 {
		entry.Data, entry.DataSet = data[0], true
	}
	committed, err := s.AppendEntry(ctx, entry, s.lane)
	if err != nil {
		return "", err
	}
	return committed.entryBase().ID, nil
}

func (s *Session) GetLanes(ctx context.Context) ([]LanePointer, error) {
	return s.storage.GetLanes(ctx)
}

func (s *Session) CreateLane(ctx context.Context, lane, at string, atSet bool) error {
	return s.storage.CreateLane(ctx, lane, at, atSet)
}

func (s *Session) MoveLane(ctx context.Context, lane, to string, toSet bool) error {
	return s.storage.MoveLane(ctx, lane, to, toSet)
}

func (s *Session) AppendEntry(ctx context.Context, entry ProvisionedEntry, lane string) (Entry, error) {
	if err := AssertJSONSerializable(entry); err != nil {
		return nil, err
	}
	return s.storage.AppendEntry(ctx, entry, lane)
}

func (s *Session) AppendRecord(ctx context.Context, record NewRecord) (LaneRecord, error) {
	if err := AssertJSONSerializable(record); err != nil {
		return nil, err
	}
	return s.storage.AppendRecord(ctx, record)
}

func (s *Session) FindRecords(ctx context.Context, query RecordQuery) ([]LaneRecord, error) {
	if err := validateRecordQuery(query); err != nil {
		return nil, err
	}
	return s.storage.FindRecords(ctx, query)
}

func (s *Session) FindOpenOperations(ctx context.Context, lane string, limit int) ([]OperationStartedRecord, error) {
	if err := validateLimit(limit); err != nil {
		return nil, err
	}
	return s.storage.FindOpenOperations(ctx, lane, limit)
}

func (s *Session) GetLog(ctx context.Context, options LogOptions) ([]LogItem, error) {
	if err := validateLimit(options.Limit); err != nil {
		return nil, err
	}
	if options.AfterSeqSet && options.AfterSeq < 0 {
		return nil, newSessionError(SessionErrorInvalidQuery, "cursor sequence must be a non-negative integer")
	}
	return s.storage.GetLog(ctx, options)
}

func (s *Session) getLeafIDForLane(ctx context.Context, lane string) (string, bool, error) {
	lanes, err := s.storage.GetLanes(ctx)
	if err != nil {
		return "", false, err
	}
	for _, candidate := range lanes {
		if candidate.Lane == lane {
			return candidate.LeafID, candidate.LeafIDSet, nil
		}
	}
	return "", false, newSessionError(SessionErrorInvalidLane, "Lane not found: %s", lane)
}

type sessionView struct {
	session *Session
	lane    string
}

func (v *sessionView) GetLeafID(ctx context.Context) (string, bool, error) {
	return v.session.getLeafIDForLane(ctx, v.lane)
}

func (v *sessionView) GetEntry(ctx context.Context, id string) (Entry, bool, error) {
	return v.session.GetEntry(ctx, id)
}

func (v *sessionView) GetStats(ctx context.Context) (SessionStats, error) {
	return v.session.GetStats(ctx)
}

func (v *sessionView) GetName(ctx context.Context) (string, bool, error) {
	return v.session.GetName(ctx)
}

func (v *sessionView) SetName(ctx context.Context, name string, set bool) error {
	return v.session.SetName(ctx, name, set)
}

func (v *sessionView) GetLabel(ctx context.Context, id string) (string, bool, error) {
	return v.session.GetLabel(ctx, id)
}

func (v *sessionView) SetLabel(ctx context.Context, id, label string, set bool) error {
	return v.session.SetLabel(ctx, id, label, set)
}

func (v *sessionView) FindEntries(ctx context.Context, query EntryQuery) ([]Entry, error) {
	return v.session.FindEntries(ctx, query)
}

func (v *sessionView) FindEntry(ctx context.Context, query EntryQuery) (Entry, bool, error) {
	return v.session.FindEntry(ctx, query)
}

func (v *sessionView) FindEntriesOnBranch(ctx context.Context, query EntryQuery, bounds BranchBounds) ([]Entry, error) {
	if !bounds.StartSet {
		leafID, ok, err := v.session.getLeafIDForLane(ctx, v.lane)
		if err != nil || !ok {
			return nil, err
		}
		bounds.Start, bounds.StartSet = leafID, true
	}
	if err := validateEntryQuery(query); err != nil {
		return nil, err
	}
	return v.session.storage.FindEntriesOnBranch(ctx, query, bounds)
}

func (v *sessionView) FindEntryOnBranch(ctx context.Context, query EntryQuery, bounds BranchBounds) (Entry, bool, error) {
	query.Limit = 1
	entries, err := v.FindEntriesOnBranch(ctx, query, bounds)
	if err != nil || len(entries) == 0 {
		return nil, false, err
	}
	return entries[0], true, nil
}

func (v *sessionView) AppendMessage(ctx context.Context, message AgentMessage) (string, error) {
	entry, err := v.session.AppendEntry(ctx, MessageEntry{EntryBase: EntryBase{ID: v.session.IDGenerator.Next()}, Message: message}, v.lane)
	if err != nil {
		return "", err
	}
	return entry.entryBase().ID, nil
}

func (v *sessionView) AppendCustomEntry(ctx context.Context, customType string, data ...JSONValue) (string, error) {
	entry := CustomEntry{EntryBase: EntryBase{ID: v.session.IDGenerator.Next()}, CustomType: customType}
	if len(data) > 0 {
		entry.Data, entry.DataSet = data[0], true
	}
	committed, err := v.session.AppendEntry(ctx, entry, v.lane)
	if err != nil {
		return "", err
	}
	return committed.entryBase().ID, nil
}

func validateEntryQuery(query EntryQuery) error {
	if err := validateLimit(query.Limit); err != nil {
		return err
	}
	if query.Cursor != nil && query.Cursor.AfterSeq < 0 {
		return newSessionError(SessionErrorInvalidQuery, "cursor sequence must be a non-negative integer")
	}
	return nil
}

func validateRecordQuery(query RecordQuery) error {
	if err := validateLimit(query.Limit); err != nil {
		return err
	}
	if query.AfterSeqSet && query.AfterSeq < 0 {
		return newSessionError(SessionErrorInvalidQuery, "cursor sequence must be a non-negative integer")
	}
	if query.OperationKind != "" && query.Type != LaneRecordTypeOperationStarted {
		return newSessionError(SessionErrorInvalidQuery, "operationKind requires type operation_started")
	}
	return nil
}

func validateLimit(limit int) error {
	if limit < 0 {
		return newSessionError(SessionErrorInvalidQuery, "limit must be a positive integer")
	}
	return nil
}

func requireContext(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("nil context")
	}
	return ctx.Err()
}

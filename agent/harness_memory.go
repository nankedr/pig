package agent

import (
	"context"
	"encoding/json"
	"sync"
	"time"
)

type laneLeaf struct {
	id  string
	set bool
}

type InMemorySessionStorage struct {
	mu             sync.RWMutex
	metadata       SessionMetadata
	sequence       int64
	usedIDs        map[string]struct{}
	entries        []Entry
	entriesByID    map[string]Entry
	records        []LaneRecord
	openOperations map[string]map[string]OperationStartedRecord
	lanes          map[string]laneLeaf
	laneOrder      []string
	log            []LogItem
	stats          SessionStats
	name           string
	nameSet        bool
	labels         map[string]string
}

func NewInMemorySessionStorage(metadata SessionMetadata) *InMemorySessionStorage {
	return &InMemorySessionStorage{
		metadata:       metadata,
		usedIDs:        make(map[string]struct{}),
		entriesByID:    make(map[string]Entry),
		openOperations: make(map[string]map[string]OperationStartedRecord),
		lanes:          map[string]laneLeaf{"main": {}},
		laneOrder:      []string{"main"},
		labels:         make(map[string]string),
	}
}

func (s *InMemorySessionStorage) GetMetadata(ctx context.Context) (SessionMetadata, error) {
	if err := requireContext(ctx); err != nil {
		return SessionMetadata{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.metadata, nil
}

func (s *InMemorySessionStorage) GetLanes(ctx context.Context) ([]LanePointer, error) {
	if err := requireContext(ctx); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	lanes := make([]LanePointer, 0, len(s.laneOrder))
	for _, name := range s.laneOrder {
		leaf := s.lanes[name]
		lanes = append(lanes, LanePointer{Lane: name, LeafID: leaf.id, LeafIDSet: leaf.set})
	}
	return lanes, nil
}

func (s *InMemorySessionStorage) CreateLane(ctx context.Context, lane, at string, atSet bool) error {
	if err := requireContext(ctx); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.lanes[lane]; exists {
		return newSessionError(SessionErrorAlreadyExists, "Lane already exists: %s", lane)
	}
	if atSet {
		if _, exists := s.entriesByID[at]; !exists {
			return newSessionError(SessionErrorNotFound, "Entry not found: %s", at)
		}
	}
	s.sequence++
	s.lanes[lane] = laneLeaf{id: at, set: atSet}
	s.laneOrder = append(s.laneOrder, lane)
	s.log = append(s.log, LaneLogItem{Seq: s.sequence, Lane: lane, LeafID: at, LeafIDSet: atSet})
	return nil
}

func (s *InMemorySessionStorage) MoveLane(ctx context.Context, lane, to string, toSet bool) error {
	if err := requireContext(ctx); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.lanes[lane]; !exists {
		return newSessionError(SessionErrorInvalidLane, "Lane not found: %s", lane)
	}
	if toSet {
		if _, exists := s.entriesByID[to]; !exists {
			return newSessionError(SessionErrorNotFound, "Entry not found: %s", to)
		}
	}
	s.sequence++
	s.lanes[lane] = laneLeaf{id: to, set: toSet}
	s.log = append(s.log, LaneLogItem{Seq: s.sequence, Lane: lane, LeafID: to, LeafIDSet: toSet})
	return nil
}

func (s *InMemorySessionStorage) AppendEntry(ctx context.Context, provisioned ProvisionedEntry, lane string) (Entry, error) {
	if err := requireContext(ctx); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	leaf, exists := s.lanes[lane]
	if !exists {
		return nil, newSessionError(SessionErrorInvalidLane, "Lane not found: %s", lane)
	}
	base := provisioned.entryBase()
	if _, exists := s.usedIDs[base.ID]; exists {
		return nil, newSessionError(SessionErrorAlreadyExists, "Session id already exists: %s", base.ID)
	}
	base.Seq = s.sequence + 1
	base.ParentID, base.ParentIDSet = leaf.id, leaf.set
	base.Timestamp = time.Now().UnixMilli()
	entry := cloneEntry(withEntryBase(provisioned, base))
	s.sequence++
	s.usedIDs[base.ID] = struct{}{}
	s.entries = append(s.entries, entry)
	s.entriesByID[base.ID] = entry
	s.lanes[lane] = laneLeaf{id: base.ID, set: true}
	s.log = append(s.log, EntryLogItem{Seq: base.Seq, Entry: cloneEntry(entry)})
	if entry.EntryType() == EntryTypeMessage {
		s.stats.MessageCount++
	}
	return cloneEntry(entry), nil
}

func (s *InMemorySessionStorage) AppendRecord(ctx context.Context, provisioned NewRecord) (LaneRecord, error) {
	if err := requireContext(ctx); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	base := provisioned.recordBase()
	if _, exists := s.lanes[base.Lane]; !exists {
		return nil, newSessionError(SessionErrorInvalidLane, "Lane not found: %s", base.Lane)
	}
	if _, exists := s.usedIDs[base.ID]; exists {
		return nil, newSessionError(SessionErrorAlreadyExists, "Session id already exists: %s", base.ID)
	}
	if provisioned.RecordType() == LaneRecordTypeOperationStarted && len(s.openOperations[base.Lane]) != 0 {
		for id := range s.openOperations[base.Lane] {
			return nil, newSessionError(SessionErrorStorage, "Lane %s already has an open operation %s", base.Lane, id)
		}
	}
	base.Seq = s.sequence + 1
	base.Timestamp = time.Now().UnixMilli()
	record := cloneRecord(withRecordBase(provisioned, base))
	s.sequence++
	s.usedIDs[base.ID] = struct{}{}
	s.records = append(s.records, record)
	if started, ok := record.(OperationStartedRecord); ok {
		if s.openOperations[base.Lane] == nil {
			s.openOperations[base.Lane] = make(map[string]OperationStartedRecord)
		}
		s.openOperations[base.Lane][started.ID] = started
	}
	if finished, ok := record.(OperationFinishedRecord); ok {
		delete(s.openOperations[base.Lane], finished.RunID)
	}
	s.log = append(s.log, RecordLogItem{Seq: base.Seq, Record: cloneRecord(record)})
	if usage, ok := record.(UsageRecord); ok {
		s.stats.CachedTokens += usage.Usage.CacheRead
		s.stats.UncachedTokens += usage.Usage.Input + usage.Usage.CacheWrite
		s.stats.TotalTokens += usage.Usage.TotalTokens
		s.stats.CostTotal += usage.Usage.Cost.Total
	}
	return cloneRecord(record), nil
}

func (s *InMemorySessionStorage) GetEntry(ctx context.Context, id string) (Entry, bool, error) {
	if err := requireContext(ctx); err != nil {
		return nil, false, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	entry, ok := s.entriesByID[id]
	if !ok {
		return nil, false, nil
	}
	return cloneEntry(entry), true, nil
}

func (s *InMemorySessionStorage) FindEntries(ctx context.Context, query EntryQuery) ([]Entry, error) {
	if err := requireContext(ctx); err != nil {
		return nil, err
	}
	if err := validateEntryQuery(query); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return findEntries(s.entries, query), nil
}

func (s *InMemorySessionStorage) FindEntriesOnBranch(ctx context.Context, query EntryQuery, bounds BranchBounds) ([]Entry, error) {
	if err := requireContext(ctx); err != nil {
		return nil, err
	}
	if err := validateEntryQuery(query); err != nil {
		return nil, err
	}
	if !bounds.StartSet {
		return nil, newSessionError(SessionErrorInvalidQuery, "branch start is required")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	path, err := s.walkToRootLocked(bounds.Start, bounds)
	if err != nil {
		return nil, err
	}
	if query.Order == EntryOrderOldestFirst {
		reverseEntries(path)
	}
	results := make([]Entry, 0, len(path))
	for _, entry := range path {
		if matchesEntryQuery(entry, query) {
			results = append(results, cloneEntry(entry))
			if query.Limit > 0 && len(results) == query.Limit {
				break
			}
		}
	}
	return results, nil
}

func (s *InMemorySessionStorage) FindRecords(ctx context.Context, query RecordQuery) ([]LaneRecord, error) {
	if err := requireContext(ctx); err != nil {
		return nil, err
	}
	if err := validateRecordQuery(query); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	results := make([]LaneRecord, 0)
	visit := func(record LaneRecord) bool {
		if !matchesRecordQuery(record, query) {
			return false
		}
		results = append(results, cloneRecord(record))
		return query.Limit > 0 && len(results) == query.Limit
	}
	if query.Order == EntryOrderOldestFirst {
		for _, record := range s.records {
			if visit(record) {
				break
			}
		}
	} else {
		for index := len(s.records) - 1; index >= 0; index-- {
			if visit(s.records[index]) {
				break
			}
		}
	}
	return results, nil
}

func (s *InMemorySessionStorage) FindOpenOperations(ctx context.Context, lane string, limit int) ([]OperationStartedRecord, error) {
	if err := requireContext(ctx); err != nil {
		return nil, err
	}
	if err := validateLimit(limit); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	open := s.openOperations[lane]
	results := make([]OperationStartedRecord, 0, len(open))
	for index := len(s.records) - 1; index >= 0; index-- {
		started, ok := s.records[index].(OperationStartedRecord)
		if !ok {
			continue
		}
		if _, exists := open[started.ID]; !exists {
			continue
		}
		results = append(results, cloneRecord(started).(OperationStartedRecord))
		if limit > 0 && len(results) == limit {
			break
		}
	}
	return results, nil
}

func (s *InMemorySessionStorage) GetLog(ctx context.Context, options LogOptions) ([]LogItem, error) {
	if err := requireContext(ctx); err != nil {
		return nil, err
	}
	if err := validateLimit(options.Limit); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := make([]LogItem, 0)
	for _, item := range s.log {
		if options.AfterSeqSet && item.LogSequence() <= options.AfterSeq {
			continue
		}
		items = append(items, cloneLogItem(item))
		if options.Limit > 0 && len(items) == options.Limit {
			break
		}
	}
	return items, nil
}

func (s *InMemorySessionStorage) GetName(ctx context.Context) (string, bool, error) {
	if err := requireContext(ctx); err != nil {
		return "", false, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.name, s.nameSet, nil
}

func (s *InMemorySessionStorage) SetName(ctx context.Context, name string, set bool) error {
	if err := requireContext(ctx); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sequence++
	s.name, s.nameSet = name, set
	s.log = append(s.log, FactNameLogItem{Seq: s.sequence, Name: name, NameSet: set})
	return nil
}

func (s *InMemorySessionStorage) GetLabel(ctx context.Context, id string) (string, bool, error) {
	if err := requireContext(ctx); err != nil {
		return "", false, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	label, ok := s.labels[id]
	return label, ok, nil
}

func (s *InMemorySessionStorage) SetLabel(ctx context.Context, id, label string, set bool) error {
	if err := requireContext(ctx); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.entriesByID[id]; !exists {
		return newSessionError(SessionErrorNotFound, "Entry not found: %s", id)
	}
	s.sequence++
	if set {
		s.labels[id] = label
	} else {
		delete(s.labels, id)
	}
	s.log = append(s.log, FactLabelLogItem{Seq: s.sequence, TargetID: id, Label: label, LabelSet: set})
	return nil
}

func (s *InMemorySessionStorage) GetStats(ctx context.Context) (SessionStats, error) {
	if err := requireContext(ctx); err != nil {
		return SessionStats{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.stats, nil
}

func (s *InMemorySessionStorage) walkToRootLocked(start string, bounds BranchBounds) ([]Entry, error) {
	current, exists := s.entriesByID[start]
	if !exists {
		return nil, newSessionError(SessionErrorNotFound, "Entry not found: %s", start)
	}
	visited := make(map[string]struct{})
	path := make([]Entry, 0)
	for {
		base := current.entryBase()
		if _, duplicate := visited[base.ID]; duplicate {
			return nil, newSessionError(SessionErrorInvalidEntry, "Session branch contains a cycle at %s", base.ID)
		}
		visited[base.ID] = struct{}{}
		path = append(path, current)
		if base.ID == bounds.StopAtID || current.EntryType() == bounds.StopAtType || !base.ParentIDSet {
			return path, nil
		}
		current, exists = s.entriesByID[base.ParentID]
		if !exists {
			return nil, newSessionError(SessionErrorInvalidEntry, "Entry not found: %s", base.ParentID)
		}
	}
}

type InMemorySessionRepo struct {
	mu       sync.RWMutex
	sessions map[string]*InMemorySessionStorage
}

func NewInMemorySessionRepo() *InMemorySessionRepo {
	return &InMemorySessionRepo{sessions: make(map[string]*InMemorySessionStorage)}
}

func (r *InMemorySessionRepo) Create(ctx context.Context, options SessionCreateOptions) (*Session, error) {
	if err := requireContext(ctx); err != nil {
		return nil, err
	}
	id := options.ID
	if id == "" {
		id = randomIDGenerator{}.Next()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.sessions[id]; exists {
		return nil, newSessionError(SessionErrorAlreadyExists, "Session already exists: %s", id)
	}
	storage := NewInMemorySessionStorage(SessionMetadata{
		ID:                 id,
		CreatedAt:          time.Now().UnixMilli(),
		ParentSessionID:    options.ParentSessionID,
		ParentSessionIDSet: options.ParentSessionIDSet,
	})
	r.sessions[id] = storage
	return NewSession(storage), nil
}

func (r *InMemorySessionRepo) Open(ctx context.Context, metadata SessionMetadata) (*Session, error) {
	if err := requireContext(ctx); err != nil {
		return nil, err
	}
	r.mu.RLock()
	storage := r.sessions[metadata.ID]
	r.mu.RUnlock()
	if storage == nil {
		return nil, newSessionError(SessionErrorNotFound, "Session not found: %s", metadata.ID)
	}
	return NewSession(storage), nil
}

func (r *InMemorySessionRepo) List(ctx context.Context) ([]SessionMetadata, error) {
	if err := requireContext(ctx); err != nil {
		return nil, err
	}
	r.mu.RLock()
	storages := make([]*InMemorySessionStorage, 0, len(r.sessions))
	for _, storage := range r.sessions {
		storages = append(storages, storage)
	}
	r.mu.RUnlock()
	metadata := make([]SessionMetadata, 0, len(storages))
	for _, storage := range storages {
		value, err := storage.GetMetadata(ctx)
		if err != nil {
			return nil, err
		}
		metadata = append(metadata, value)
	}
	return metadata, nil
}

func (r *InMemorySessionRepo) Delete(ctx context.Context, metadata SessionMetadata) error {
	if err := requireContext(ctx); err != nil {
		return err
	}
	r.mu.Lock()
	delete(r.sessions, metadata.ID)
	r.mu.Unlock()
	return nil
}

func (r *InMemorySessionRepo) Fork(ctx context.Context, source SessionMetadata, options ForkOptions, create SessionCreateOptions) (*Session, error) {
	if err := requireContext(ctx); err != nil {
		return nil, err
	}
	r.mu.RLock()
	sourceStorage := r.sessions[source.ID]
	r.mu.RUnlock()
	if sourceStorage == nil {
		return nil, newSessionError(SessionErrorNotFound, "Session not found: %s", source.ID)
	}
	id := create.ID
	if id == "" {
		id = randomIDGenerator{}.Next()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.sessions[id]; exists {
		return nil, newSessionError(SessionErrorAlreadyExists, "Session already exists: %s", id)
	}
	metadata := SessionMetadata{ID: id, CreatedAt: time.Now().UnixMilli(), ParentSessionID: source.ID, ParentSessionIDSet: true}
	if create.ParentSessionIDSet {
		metadata.ParentSessionID = create.ParentSessionID
	}
	storage, err := sourceStorage.Fork(ctx, metadata, options)
	if err != nil {
		return nil, err
	}
	r.sessions[id] = storage
	return NewSession(storage), nil
}

func (s *InMemorySessionStorage) Fork(ctx context.Context, metadata SessionMetadata, options ForkOptions) (*InMemorySessionStorage, error) {
	if err := requireContext(ctx); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	target := NewInMemorySessionStorage(metadata)
	var copied []Entry
	if options.Scope == ForkScopeTree {
		for _, entry := range s.entries {
			copied = append(copied, cloneEntry(entry))
		}
	} else {
		leaf := s.lanes["main"]
		targetID, targetSet := leaf.id, leaf.set
		if options.EntryIDSet {
			entry, exists := s.entriesByID[options.EntryID]
			if !exists || entry.EntryType() != EntryTypeMessage {
				return nil, newSessionError(SessionErrorInvalidForkTarget, "Fork target is not a message entry: %s", options.EntryID)
			}
			targetID, targetSet = options.EntryID, true
			position := options.Position
			if position == "" {
				position = ForkPositionBefore
			}
			if position == ForkPositionBefore {
				base := entry.entryBase()
				targetID, targetSet = base.ParentID, base.ParentIDSet
			}
		}
		if targetSet {
			path, err := s.walkToRootLocked(targetID, BranchBounds{Start: targetID, StartSet: true})
			if err != nil {
				return nil, err
			}
			reverseEntries(path)
			for _, entry := range path {
				copied = append(copied, cloneEntry(entry))
			}
		}
	}
	for _, entry := range copied {
		base := entry.entryBase()
		target.sequence++
		base.Seq = target.sequence
		entry = withEntryBase(entry, base)
		target.entries = append(target.entries, entry)
		target.entriesByID[base.ID] = entry
		target.usedIDs[base.ID] = struct{}{}
		target.log = append(target.log, EntryLogItem{Seq: base.Seq, Entry: cloneEntry(entry)})
		if entry.EntryType() == EntryTypeMessage {
			target.stats.MessageCount++
		}
	}
	if options.Scope == ForkScopeTree {
		target.lanes = make(map[string]laneLeaf, len(s.lanes))
		target.laneOrder = append([]string(nil), s.laneOrder...)
		for lane, leaf := range s.lanes {
			target.sequence++
			target.lanes[lane] = leaf
			target.log = append(target.log, LaneLogItem{Seq: target.sequence, Lane: lane, LeafID: leaf.id, LeafIDSet: leaf.set})
		}
	} else {
		leaf := laneLeaf{}
		if len(copied) > 0 {
			leaf = laneLeaf{id: copied[len(copied)-1].entryBase().ID, set: true}
		}
		target.lanes["main"] = leaf
		target.sequence++
		target.log = append(target.log, LaneLogItem{Seq: target.sequence, Lane: "main", LeafID: leaf.id, LeafIDSet: leaf.set})
	}
	if s.nameSet {
		target.sequence++
		target.name, target.nameSet = s.name, true
		target.log = append(target.log, FactNameLogItem{Seq: target.sequence, Name: s.name, NameSet: true})
	}
	for _, entry := range copied {
		id := entry.entryBase().ID
		if label, ok := s.labels[id]; ok {
			target.sequence++
			target.labels[id] = label
			target.log = append(target.log, FactLabelLogItem{Seq: target.sequence, TargetID: id, Label: label, LabelSet: true})
		}
	}
	return target, nil
}

func findEntries(entries []Entry, query EntryQuery) []Entry {
	results := make([]Entry, 0)
	visit := func(entry Entry) bool {
		if !matchesEntryQuery(entry, query) {
			return false
		}
		results = append(results, cloneEntry(entry))
		return query.Limit > 0 && len(results) == query.Limit
	}
	if query.Order == EntryOrderOldestFirst {
		for _, entry := range entries {
			if visit(entry) {
				break
			}
		}
	} else {
		for index := len(entries) - 1; index >= 0; index-- {
			if visit(entries[index]) {
				break
			}
		}
	}
	return results
}

func matchesEntryQuery(entry Entry, query EntryQuery) bool {
	base := entry.entryBase()
	if query.Type != "" && entry.EntryType() != query.Type {
		return false
	}
	if query.CustomType != "" {
		custom, ok := entry.(CustomEntry)
		if !ok || custom.CustomType != query.CustomType {
			return false
		}
	}
	if query.Cursor != nil {
		if query.Order == EntryOrderOldestFirst {
			return base.Seq > query.Cursor.AfterSeq
		}
		return base.Seq < query.Cursor.AfterSeq
	}
	return true
}

func matchesRecordQuery(record LaneRecord, query RecordQuery) bool {
	base := record.recordBase()
	if query.Lane != "" && base.Lane != query.Lane {
		return false
	}
	if query.Type != "" && record.RecordType() != query.Type {
		return false
	}
	if query.RunID != "" {
		if started, ok := record.(OperationStartedRecord); ok {
			if started.ID != query.RunID {
				return false
			}
		} else if runID, ok := recordRunID(record); !ok || runID != query.RunID {
			return false
		}
	}
	if query.OperationKind != "" {
		started, ok := record.(OperationStartedRecord)
		if !ok || started.Intent == nil || started.Intent.OperationKind() != query.OperationKind {
			return false
		}
	}
	return !query.AfterSeqSet || base.Seq > query.AfterSeq
}

func recordRunID(record LaneRecord) (string, bool) {
	switch record := record.(type) {
	case AbortRequestedRecord:
		return record.RunID, true
	case OperationFinishedRecord:
		return record.RunID, true
	case StepAttemptRecord:
		return record.RunID, true
	case ToolStartedRecord:
		return record.RunID, true
	case QueueEnqueuedRecord:
		return record.RunID, record.RunIDSet
	case QueueCancelledRecord:
		return record.RunID, record.RunIDSet
	case WriteDeferredRecord:
		return record.RunID, true
	case UsageRecord:
		return record.RunID, record.RunIDSet
	default:
		return "", false
	}
}

func withEntryBase(entry Entry, base EntryBase) Entry {
	switch entry := entry.(type) {
	case MessageEntry:
		entry.EntryBase = base
		return entry
	case ModelChangeEntry:
		entry.EntryBase = base
		return entry
	case ThinkingLevelEntry:
		entry.EntryBase = base
		return entry
	case ActiveToolsEntry:
		entry.EntryBase = base
		return entry
	case CompactionEntry:
		entry.EntryBase = base
		return entry
	case BranchSummaryEntry:
		entry.EntryBase = base
		return entry
	case CustomEntry:
		entry.EntryBase = base
		return entry
	default:
		panic("unsupported Entry implementation")
	}
}

func cloneEntry(entry Entry) Entry {
	switch entry := entry.(type) {
	case MessageEntry:
		entry.Message = cloneAgentMessage(entry.Message)
		return entry
	case ModelChangeEntry:
		return entry
	case ThinkingLevelEntry:
		return entry
	case ActiveToolsEntry:
		entry.ActiveToolNames = append([]string(nil), entry.ActiveToolNames...)
		return entry
	case CompactionEntry:
		entry.RetainedTail = cloneAgentMessages(entry.RetainedTail)
		entry.Details = cloneJSONValue(entry.Details)
		if entry.Usage != nil {
			usage := *entry.Usage
			entry.Usage = &usage
		}
		return entry
	case BranchSummaryEntry:
		entry.Details = cloneJSONValue(entry.Details)
		if entry.Usage != nil {
			usage := *entry.Usage
			entry.Usage = &usage
		}
		return entry
	case CustomEntry:
		entry.Data = cloneJSONValue(entry.Data)
		return entry
	default:
		panic("unsupported Entry implementation")
	}
}

func withRecordBase(record LaneRecord, base RecordBase) LaneRecord {
	switch record := record.(type) {
	case OperationStartedRecord:
		record.RecordBase = base
		return record
	case AbortRequestedRecord:
		record.RecordBase = base
		return record
	case OperationFinishedRecord:
		record.RecordBase = base
		return record
	case StepAttemptRecord:
		record.RecordBase = base
		return record
	case ToolStartedRecord:
		record.RecordBase = base
		return record
	case QueueEnqueuedRecord:
		record.RecordBase = base
		return record
	case QueueCancelledRecord:
		record.RecordBase = base
		return record
	case WriteDeferredRecord:
		record.RecordBase = base
		return record
	case UsageRecord:
		record.RecordBase = base
		return record
	default:
		panic("unsupported LaneRecord implementation")
	}
}

func cloneRecord(record LaneRecord) LaneRecord {
	switch record := record.(type) {
	case OperationStartedRecord:
		record.Intent = cloneOperationIntent(record.Intent)
		return record
	case AbortRequestedRecord:
		return record
	case OperationFinishedRecord:
		if record.Error != nil {
			value := *record.Error
			record.Error = &value
		}
		return record
	case StepAttemptRecord:
		if record.CompactionReason != nil {
			value := *record.CompactionReason
			record.CompactionReason = &value
		}
		return record
	case ToolStartedRecord:
		record.EffectiveArgs = cloneStringAnyMap(record.EffectiveArgs)
		return record
	case QueueEnqueuedRecord:
		record.Target = cloneEntry(record.Target)
		return record
	case QueueCancelledRecord:
		return record
	case WriteDeferredRecord:
		record.Target = cloneEntry(record.Target)
		return record
	case UsageRecord:
		record.Details = cloneJSONValue(record.Details)
		return record
	default:
		panic("unsupported LaneRecord implementation")
	}
}

func cloneOperationIntent(intent OperationIntent) OperationIntent {
	switch intent := intent.(type) {
	case RunOperationIntent:
		intent.OriginalPrompt = cloneAgentMessages(intent.OriginalPrompt)
		intent.InitialMessages = cloneEntries(intent.InitialMessages)
		if intent.SystemPromptOverride != nil {
			value := *intent.SystemPromptOverride
			intent.SystemPromptOverride = &value
		}
		intent.ResumeData = cloneStringAnyMap(intent.ResumeData)
		return intent
	case CompactionOperationIntent:
		if intent.CustomInstructions != nil {
			value := *intent.CustomInstructions
			intent.CustomInstructions = &value
		}
		return intent
	case NavigationOperationIntent:
		if intent.CustomInstructions != nil {
			value := *intent.CustomInstructions
			intent.CustomInstructions = &value
		}
		if intent.Label != nil {
			value := *intent.Label
			intent.Label = &value
		}
		if intent.SummaryEntryID != nil {
			value := *intent.SummaryEntryID
			intent.SummaryEntryID = &value
		}
		return intent
	case nil:
		return nil
	default:
		panic("unsupported OperationIntent implementation")
	}
}

func cloneEntries(entries []Entry) []Entry {
	cloned := make([]Entry, len(entries))
	for index, entry := range entries {
		cloned[index] = cloneEntry(entry)
	}
	return cloned
}

func cloneStringAnyMap[T ~map[string]V, V any](value T) T {
	if value == nil {
		return nil
	}
	cloned := make(T, len(value))
	for key, item := range value {
		cloned[key] = any(cloneJSONValue(item)).(V)
	}
	return cloned
}

func cloneJSONValue(value any) any {
	if value == nil {
		return nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return value
	}
	var cloned any
	if err := json.Unmarshal(data, &cloned); err != nil {
		return value
	}
	return cloned
}

func cloneLogItem(item LogItem) LogItem {
	switch item := item.(type) {
	case EntryLogItem:
		item.Entry = cloneEntry(item.Entry)
		return item
	case RecordLogItem:
		item.Record = cloneRecord(item.Record)
		return item
	case LaneLogItem:
		return item
	case FactNameLogItem:
		return item
	case FactLabelLogItem:
		return item
	default:
		panic("unsupported LogItem implementation")
	}
}

func reverseEntries(entries []Entry) {
	for left, right := 0, len(entries)-1; left < right; left, right = left+1, right-1 {
		entries[left], entries[right] = entries[right], entries[left]
	}
}

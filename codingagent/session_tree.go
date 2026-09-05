package codingagent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"sort"
)

func (m *SessionManager) GetLabel(id string) *string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	label, _ := sessionLabel(m.entries, id)
	return label
}

func sessionLabel(entries []SessionEntry, id string) (*string, *string) {
	for i := len(entries) - 1; i >= 0; i-- {
		e := entries[i]
		if e.Type == "label" && e.TargetID == id {
			if e.Label == nil || *e.Label == "" {
				return nil, nil
			}
			return cloneStringPointer(e.Label), cloneStringPointer(&e.Timestamp)
		}
	}
	return nil, nil
}

func (m *SessionManager) GetChildren(id string) ([]SessionEntry, error) {
	children := []SessionEntry{}
	for _, e := range m.GetEntries() {
		if e.ParentID != nil && *e.ParentID == id {
			children = append(children, e)
		}
	}
	return children, nil
}

func (m *SessionManager) GetTree() ([]SessionTreeNode, error) {
	entries := m.GetEntries()
	nodes := make(map[string]*SessionTreeNode, len(entries))
	children := map[string][]string{}
	roots := []string{}
	labels := map[string]SessionEntry{}
	for _, e := range entries {
		if e.Type == "label" {
			labels[e.TargetID] = e
		}
	}
	for _, e := range entries {
		node := &SessionTreeNode{Entry: e, Children: []SessionTreeNode{}}
		if l, ok := labels[e.ID]; ok && l.Label != nil && *l.Label != "" {
			node.Label = cloneStringPointer(l.Label)
			node.LabelTimestamp = cloneStringPointer(&l.Timestamp)
		}
		nodes[e.ID] = node
	}
	for _, e := range entries {
		if e.ParentID == nil || *e.ParentID == e.ID || nodes[*e.ParentID] == nil {
			roots = append(roots, e.ID)
		} else {
			children[*e.ParentID] = append(children[*e.ParentID], e.ID)
		}
	}
	// Assemble bottom-up so a long transcript does not recurse on the Go stack.
	order := append([]string{}, roots...)
	seen := map[string]bool{}
	for i := 0; i < len(order); i++ {
		id := order[i]
		if seen[id] {
			continue
		}
		seen[id] = true
		sort.SliceStable(children[id], func(i, j int) bool {
			return sessionTimestampMillis(nodes[children[id][i]].Entry.Timestamp) < sessionTimestampMillis(nodes[children[id][j]].Entry.Timestamp)
		})
		order = append(order, children[id]...)
	}
	for i := len(order) - 1; i >= 0; i-- {
		id := order[i]
		for _, child := range children[id] {
			nodes[id].Children = append(nodes[id].Children, *nodes[child])
		}
	}
	result := make([]SessionTreeNode, 0, len(roots))
	for _, id := range roots {
		result = append(result, *nodes[id])
	}
	return result, nil
}

func (m *SessionManager) Branch(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.hasEntryLocked(id) {
		return fmt.Errorf("Entry %s not found", id)
	}
	m.leafID = &id
	return nil
}
func (m *SessionManager) ResetLeaf() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.leafID = nil
	return nil
}
func (m *SessionManager) hasEntryLocked(id string) bool {
	for _, e := range m.entries {
		if e.ID == id {
			return true
		}
	}
	return false
}

func (m *SessionManager) AppendLabelChange(id string, label *string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.hasEntryLocked(id) {
		return "", fmt.Errorf("Entry %s not found", id)
	}
	entry := m.newEntryLocked("label")
	entry.TargetID = id
	entry.Label = cloneStringPointer(label)
	return entry.ID, m.appendEntryLocked(entry)
}

func (m *SessionManager) BranchWithSummary(id *string, summary string, options ...BranchSummaryOptions) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if id != nil && !m.hasEntryLocked(*id) {
		return "", fmt.Errorf("Entry %s not found", *id)
	}
	m.leafID = cloneStringPointer(id)
	entry := m.newEntryLocked("branch_summary")
	entry.Summary = summary
	entry.FromID = "root"
	if id != nil {
		entry.FromID = *id
	}
	if len(options) > 0 {
		o := options[0]
		entry.Details = cloneRawMessage(o.Details)
		entry.Usage = o.Usage
		entry.FromHook = o.FromHook
	}
	entry = cloneSessionEntry(entry)
	return entry.ID, m.appendEntryLocked(entry)
}

func (m *SessionManager) replaceLocked(next *SessionManager) {
	m.cwd, m.sessionDir, m.sessionID, m.sessionFile = next.cwd, next.sessionDir, next.sessionID, next.sessionFile
	m.header, m.entries, m.leafID = next.header, next.entries, next.leafID
	m.flushed = next.flushed
}

func (m *SessionManager) SetSessionFile(path string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	next, err := OpenSessionManager(path, &m.sessionDir, &m.cwd)
	if err != nil {
		return err
	}
	m.replaceLocked(next)
	return nil
}
func (m *SessionManager) NewSession(options ...NewSessionOptions) (*string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	next := NewInMemorySessionManager(m.cwd, options...)
	if next.sessionID == "" || !customSessionID.MatchString(next.sessionID) {
		return nil, fmt.Errorf("Invalid session ID")
	}
	if m.sessionFile != "" {
		var err error
		next, err = NewSessionManager(m.cwd, &m.sessionDir, options...)
		if err != nil {
			return nil, err
		}
	}
	m.replaceLocked(next)
	if m.sessionFile == "" {
		return nil, nil
	}
	return cloneStringPointer(&m.sessionFile), nil
}

func (m *SessionManager) CreateBranchedSession(id string) (*string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.hasEntryLocked(id) {
		return nil, fmt.Errorf("Entry %s not found", id)
	}
	next := NewInMemorySessionManager(m.cwd)
	if m.sessionFile != "" {
		var err error
		next, err = NewSessionManager(m.cwd, &m.sessionDir, NewSessionOptions{ParentSession: m.sessionFile})
		if err != nil {
			return nil, err
		}
	}
	for _, entry := range buildContextPath(m.entries, &id, true) {
		if entry.Type == "label" {
			continue
		}
		entry.ParentID = cloneStringPointer(next.leafID)
		if len(entry.Raw) > 0 {
			var raw map[string]json.RawMessage
			if err := json.Unmarshal(entry.Raw, &raw); err != nil {
				return nil, err
			}
			raw["parentId"], _ = json.Marshal(entry.ParentID)
			entry.Raw, _ = json.Marshal(raw)
		}
		next.entries = append(next.entries, entry)
		next.leafID = cloneStringPointer(&entry.ID)
	}
	retained := map[string]bool{}
	for _, e := range next.entries {
		retained[e.ID] = true
	}
	// Keep resolved-label insertion order, including clear followed by re-add.
	labelOrder := []string{}
	active := map[string]SessionEntry{}
	for _, e := range m.entries {
		if e.Type != "label" {
			continue
		}
		if e.Label == nil || *e.Label == "" {
			delete(active, e.TargetID)
			for i, id := range labelOrder {
				if id == e.TargetID {
					labelOrder = append(labelOrder[:i], labelOrder[i+1:]...)
					break
				}
			}
		} else {
			if _, ok := active[e.TargetID]; !ok {
				labelOrder = append(labelOrder, e.TargetID)
			}
			active[e.TargetID] = e
		}
	}
	for _, target := range labelOrder {
		if !retained[target] {
			continue
		}
		old := active[target]
		e := next.newEntryLocked("label")
		e.TargetID = target
		e.Label = cloneStringPointer(old.Label)
		e.Timestamp = old.Timestamp
		next.entries = append(next.entries, e)
		next.leafID = cloneStringPointer(&e.ID)
	}
	if len(next.entries) > 0 {
		if err := next.persistEntryLocked(next.entries[len(next.entries)-1]); err != nil {
			return nil, err
		}
	}
	m.replaceLocked(next)
	if m.sessionFile == "" {
		return nil, nil
	}
	return cloneStringPointer(&m.sessionFile), nil
}

func ForkSessionManager(source, cwd string, dir *string, options ...NewSessionOptions) (*SessionManager, error) {
	path, err := resolveSessionPath(source)
	if err != nil {
		return nil, err
	}
	records, _, err := loadSessionFile(path)
	if err != nil || len(records) == 0 || records[0].Header == nil {
		return nil, fmt.Errorf("Cannot fork: source session file is empty or invalid: %s", path)
	}
	fields, _ := decodeJSONObject(records[0].Raw)
	_, validID := decodeJSONField[string](fields, "id")
	if !validID || bytes.Equal(bytes.TrimSpace(fields["id"]), []byte("null")) {
		return nil, fmt.Errorf("Cannot fork: source session file is empty or invalid: %s", path)
	}
	MigrateSessionEntries(records)
	option := NewSessionOptions{}
	if len(options) > 0 {
		option = options[0]
	}
	option.ParentSession = path
	next, err := NewSessionManager(cwd, dir, option)
	if err != nil {
		return nil, err
	}
	header, err := json.Marshal(next.header)
	if err != nil {
		return nil, err
	}
	var encoded bytes.Buffer
	encoded.Write(header)
	encoded.WriteByte('\n')
	for _, record := range records[1:] {
		if record.Header != nil {
			continue
		}
		encoded.Write(record.Raw)
		encoded.WriteByte('\n')
		if record.Entry != nil {
			entry := cloneSessionEntry(*record.Entry)
			next.entries = append(next.entries, entry)
			next.leafID = cloneStringPointer(&entry.ID)
		}
	}
	file, err := os.OpenFile(next.sessionFile, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	_, writeErr := file.Write(encoded.Bytes())
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		_ = os.Remove(next.sessionFile)
		if writeErr != nil {
			return nil, writeErr
		}
		return nil, closeErr
	}
	next.flushed = true
	return next, nil
}

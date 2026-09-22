package codingagent

import (
	"context"
	"fmt"
	"github.com/nankedr/pig/tui"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

type SessionsLoader func(context.Context) ([]SessionInfo, error)
type SessionSelectorOptions struct {
	Keybindings            *KeybindingsManager
	CurrentSessionFilePath string
	RenameSession          func(path, name string) error
}
type SessionSelectorComponent struct {
	tui.Container
	Focused                           bool
	ctx                               context.Context
	current, all                      SessionsLoader
	sessions, filtered                []SessionInfo
	search, rename                    *tui.Input
	list                              *tui.SelectList
	options                           SessionSelectorOptions
	onSelect                          func(string)
	onCancel                          func()
	allScope, named, showPath, closed bool
	sortMode                          int
	target, status                    string
}

func NewSessionSelectorComponent(ctx context.Context, current, all SessionsLoader, onSelect func(string), onCancel func(), options ...SessionSelectorOptions) (*SessionSelectorComponent, error) {
	if ctx == nil || current == nil || all == nil {
		return nil, fmt.Errorf("Session selector requires a context and loaders")
	}
	s := &SessionSelectorComponent{ctx: ctx, current: current, all: all, onSelect: onSelect, onCancel: onCancel, search: tui.NewInput(), Focused: true}
	if len(options) > 0 {
		s.options = options[0]
	}
	if s.options.Keybindings == nil {
		var err error
		s.options.Keybindings, err = NewKeybindingsManager()
		if err != nil {
			return nil, err
		}
	}
	s.search.SetKeybindings(&s.options.Keybindings.KeybindingsManager)
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}
func (s *SessionSelectorComponent) GetSessionList() (*tui.SelectList, error) {
	if s.list == nil {
		return nil, notImplemented("SessionSelectorComponent.GetSessionList")
	}
	return s.list, nil
}
func (s *SessionSelectorComponent) load() error {
	loader := s.current
	if s.allScope {
		loader = s.all
	}
	entries, err := loader(s.ctx)
	if err != nil {
		return err
	}
	s.sessions = append([]SessionInfo(nil), entries...)
	for i := range s.sessions {
		s.sessions[i].Name = cloneStringPointer(s.sessions[i].Name)
		s.sessions[i].ParentSessionPath = cloneStringPointer(s.sessions[i].ParentSessionPath)
	}
	s.filter()
	return nil
}
func (s *SessionSelectorComponent) index() int {
	if s.list != nil {
		if item, _ := s.list.GetSelectedItem(); item != nil {
			for i, entry := range s.filtered {
				if entry.Path == item.Value {
					return i
				}
			}
		}
	}
	return 0
}
func (s *SessionSelectorComponent) filter() {
	index := s.index()
	query := strings.TrimSpace(s.search.GetValue())
	type ranked struct {
		info  SessionInfo
		score float64
	}
	matches := []ranked{}
	for _, entry := range s.sessions {
		if s.named && (entry.Name == nil || strings.TrimSpace(*entry.Name) == "") {
			continue
		}
		if score, ok := matchSessionSearch(entry, query); ok {
			matches = append(matches, ranked{entry, score})
		}
	}
	if query != "" && s.sortMode != 1 {
		sort.SliceStable(matches, func(i, j int) bool {
			if matches[i].score != matches[j].score {
				return matches[i].score < matches[j].score
			}
			return matches[i].info.Modified.After(matches[j].info.Modified)
		})
	}
	s.filtered = nil
	for _, match := range matches {
		s.filtered = append(s.filtered, match.info)
	}
	depths := map[string]int{}
	if query == "" && s.sortMode == 0 {
		s.filtered, depths = threadedSessions(s.filtered)
	}
	items := make([]tui.SelectItem, len(s.filtered))
	for i, entry := range s.filtered {
		label := entry.FirstMessage
		if entry.Name != nil {
			label = *entry.Name
		}
		label = strings.Join(strings.Fields(tui.SafeTerminalText(label)), " ")
		if depth := depths[entry.Path]; depth > 0 {
			label = strings.Repeat("  ", depth) + "└─ " + label
		}
		if sameSessionPath(entry.Path, s.options.CurrentSessionFilePath) {
			label += " ✓"
		}
		description := fmt.Sprintf("%d · %s", entry.MessageCount, entry.Modified.Format("2006-01-02"))
		if s.allScope {
			description = entry.CWD + " · " + description
		}
		if s.showPath {
			description = entry.Path + " · " + description
		}
		description = tui.SafeTerminalText(description)
		items[i] = tui.SelectItem{Value: entry.Path, Label: label, Description: &description}
	}
	list := tui.NewSelectList(items, 10, tui.SelectListTheme{})
	if s.list == nil {
		s.list = list
	} else {
		*s.list = *list
	}
	s.list.SetSelectedIndex(index)
}
func sameSessionPath(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	canonical := func(p string) string {
		p, _ = filepath.Abs(p)
		if real, err := filepath.EvalSymlinks(p); err == nil {
			return real
		}
		return p
	}
	return canonical(a) == canonical(b)
}
func threadedSessions(entries []SessionInfo) ([]SessionInfo, map[string]int) {
	byPath := map[string]int{}
	for i, e := range entries {
		p, _ := filepath.Abs(e.Path)
		if real, err := filepath.EvalSymlinks(p); err == nil {
			p = real
		}
		byPath[p] = i
	}
	children := map[int][]int{}
	roots := []int{}
	for i, e := range entries {
		parent := -1
		if e.ParentSessionPath != nil {
			p, _ := filepath.Abs(*e.ParentSessionPath)
			if real, err := filepath.EvalSymlinks(p); err == nil {
				p = real
			}
			if n, ok := byPath[p]; ok && n != i {
				parent = n
			}
		}
		if parent < 0 {
			roots = append(roots, i)
		} else {
			children[parent] = append(children[parent], i)
		}
	}
	latest := make([]int64, len(entries))
	visiting := map[int]bool{}
	var activity func(int) int64
	activity = func(i int) int64 {
		if visiting[i] {
			return latest[i]
		}
		visiting[i] = true
		latest[i] = entries[i].Modified.UnixMilli()
		for _, c := range children[i] {
			latest[i] = max(latest[i], activity(c))
		}
		return latest[i]
	}
	for i := range entries {
		activity(i)
	}
	sortNodes := func(nodes []int) {
		sort.SliceStable(nodes, func(i, j int) bool { return latest[nodes[i]] > latest[nodes[j]] })
	}
	sortNodes(roots)
	for _, nodes := range children {
		sortNodes(nodes)
	}
	out := []SessionInfo{}
	depths := map[string]int{}
	seen := map[int]bool{}
	var walk func(int, int)
	walk = func(i, depth int) {
		if seen[i] {
			return
		}
		seen[i] = true
		depths[entries[i].Path] = depth
		out = append(out, entries[i])
		for _, c := range children[i] {
			walk(c, depth+1)
		}
	}
	for _, i := range roots {
		walk(i, 0)
	}
	for i := range entries {
		walk(i, 0)
	}
	return out, depths
}
func matchSessionSearch(entry SessionInfo, query string) (float64, bool) {
	text := entry.ID + " " + optionalHeadlessString(entry.Name) + " " + entry.AllMessagesText + " " + entry.CWD
	if strings.HasPrefix(query, "re:") {
		pattern := strings.TrimSpace(query[3:])
		if pattern == "" {
			return 0, false
		}
		re, err := regexp.Compile("(?i)" + pattern)
		if err != nil {
			return 0, false
		}
		loc := re.FindStringIndex(text)
		if loc == nil {
			return 0, false
		}
		return float64(len([]rune(text[:loc[0]]))) * 0.1, true
	}
	type token struct {
		value  string
		phrase bool
	}
	tokens := []token{}
	var buf strings.Builder
	quoted := false
	flush := func() {
		if v := strings.TrimSpace(buf.String()); v != "" {
			tokens = append(tokens, token{v, quoted})
		}
		buf.Reset()
	}
	for _, c := range query {
		if c == '"' {
			flush()
			quoted = !quoted
		} else if !quoted && unicode.IsSpace(c) {
			flush()
		} else {
			buf.WriteRune(c)
		}
	}
	flush()
	if quoted {
		tokens = nil
		for _, v := range strings.Fields(query) {
			tokens = append(tokens, token{v, false})
		}
	}
	score := 0.0
	for _, t := range tokens {
		if t.phrase {
			normalized := strings.Join(strings.Fields(strings.ToLower(text)), " ")
			phrase := strings.Join(strings.Fields(strings.ToLower(t.value)), " ")
			i := strings.Index(normalized, phrase)
			if i < 0 {
				return 0, false
			}
			score += float64(len([]rune(normalized[:i]))) * 0.1
		} else {
			m, err := tui.FuzzyMatch(t.value, text)
			if err != nil || !m.Matches {
				return 0, false
			}
			score += m.Score
		}
	}
	return score, true
}
func (s *SessionSelectorComponent) HandleInput(data string) error {
	if s.list == nil {
		return notImplemented("SessionSelectorComponent.HandleInput")
	}
	if s.closed {
		return nil
	}
	match := func(action tui.Keybinding) bool { v, _ := s.options.Keybindings.Matches(data, action); return v }
	if s.rename != nil {
		if match("tui.select.cancel") {
			s.rename = nil
			s.target = ""
			return nil
		}
		if match("tui.select.confirm") {
			name := strings.TrimSpace(s.rename.GetValue())
			if name == "" {
				return nil
			}
			err := s.options.RenameSession(s.target, name)
			s.rename = nil
			s.target = ""
			if err == nil {
				err = s.load()
			}
			if err != nil {
				s.status = "Failed to rename: " + err.Error()
			} else {
				s.status = "Session renamed"
			}
			return nil
		}
		return s.rename.HandleInput(data)
	}
	if s.target != "" {
		if match("tui.select.cancel") {
			s.target = ""
			s.status = ""
		} else if match("tui.select.confirm") {
			path := s.target
			s.target = ""
			err := os.Remove(path)
			if err == nil {
				err = s.load()
			}
			if err != nil {
				s.status = "Failed to delete: " + err.Error()
			} else {
				s.status = "Session deleted"
			}
		}
		return nil
	}
	switch {
	case match("tui.input.tab"):
		s.allScope = !s.allScope
		if err := s.load(); err != nil {
			s.allScope = !s.allScope
			s.status = "Failed to load sessions: " + err.Error()
		}
	case match("app.session.toggleSort"):
		s.sortMode = (s.sortMode + 1) % 3
		s.filter()
	case match("app.session.toggleNamedFilter"):
		s.named = !s.named
		s.filter()
	case match("app.session.togglePath"):
		s.showPath = !s.showPath
		s.filter()
	case match("app.session.rename"):
		if s.options.RenameSession != nil && len(s.filtered) > 0 {
			entry := s.filtered[s.index()]
			s.target = entry.Path
			s.rename = tui.NewInput()
			s.rename.SetKeybindings(&s.options.Keybindings.KeybindingsManager)
			s.rename.SetValue(optionalHeadlessString(entry.Name))
		}
	case match("app.session.delete"), match("app.session.deleteNoninvasive") && s.search.GetValue() == "":
		if len(s.filtered) > 0 {
			path := s.filtered[s.index()].Path
			if sameSessionPath(path, s.options.CurrentSessionFilePath) {
				s.status = "Cannot delete the currently active session"
			} else {
				s.target = path
				s.status = "Delete session? Enter confirm · Esc cancel"
			}
		}
	case match("tui.select.up"):
		s.list.SetSelectedIndex(s.index() - 1)
	case match("tui.select.down"):
		s.list.SetSelectedIndex(s.index() + 1)
	case match("tui.select.pageUp"):
		s.list.SetSelectedIndex(s.index() - 10)
	case match("tui.select.pageDown"):
		s.list.SetSelectedIndex(s.index() + 10)
	case match("tui.select.confirm"):
		if item, _ := s.list.GetSelectedItem(); item != nil {
			s.closed = true
			if s.onSelect != nil {
				s.onSelect(item.Value)
			}
		}
	case match("tui.select.cancel"):
		s.closed = true
		if s.onCancel != nil {
			s.onCancel()
		}
	default:
		if err := s.search.HandleInput(data); err != nil {
			return err
		}
		s.filter()
	}
	return nil
}
func (s *SessionSelectorComponent) Render(width int) ([]string, error) {
	if s.list == nil {
		return s.Container.Render(width)
	}
	input := s.search
	title := "Resume Session (Current Folder)"
	if s.allScope {
		title = "Resume Session (All)"
	}
	if s.rename != nil {
		title = "Rename Session"
		input = s.rename
	}
	input.Focused = s.Focused
	lines, err := input.Render(width)
	if err != nil {
		return nil, err
	}
	lines = append([]string{title}, lines...)
	if s.rename == nil {
		filter := "All"
		if s.named {
			filter = "Named"
		}
		lines = append(lines, "Sort: "+[]string{"Threaded", "Recent", "Fuzzy"}[s.sortMode]+" · Name: "+filter)
		if len(s.filtered) == 0 {
			lines = append(lines, "No sessions found · Tab to view all")
		} else {
			rows, err := s.list.Render(width)
			if err != nil {
				return nil, err
			}
			lines = append(lines, rows...)
		}
		lines = append(lines, "Tab scope · Ctrl+S sort · Ctrl+N named · Ctrl+P path", "Ctrl+D delete · Ctrl+R rename · Enter select · Esc cancel")
	}
	if s.status != "" {
		lines = append(lines, s.status)
	}
	return selectorLines(lines, width), nil
}

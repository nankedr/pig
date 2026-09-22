package codingagent

import (
	"encoding/json"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/tui"
)

type TreeSelectorOptions struct {
	InitialSelectedID string
	InitialFilterMode string
	OnLabelChange     func(string, *string) error
	Keybindings       *KeybindingsManager
}
type treeSelectorNode struct {
	entry                        SessionEntry
	label, labelTimestamp        *string
	text, searchable, role, stop string
}
type TreeSelectorComponent struct {
	tui.Container
	Focused                               bool
	nodes                                 []treeSelectorNode
	byID                                  map[string]int
	active, folded                        map[string]bool
	visible                               []int
	parents                               map[int]int
	children                              map[int][]int
	selected, maxVisible                  int
	leaf, lastSelected, filterMode, query string
	showTimestamps                        bool
	labelInput                            *tui.Input
	labelNode                             int
	status                                string
	list                                  *tui.SelectList
	options                               TreeSelectorOptions
	onSelect                              func(string)
	onCancel                              func()
	onCopy                                func(string)
}

func NewTreeSelectorComponent(tree []SessionTreeNode, currentLeafID *string, terminalHeight int, onSelect func(string), onCancel func(), options ...TreeSelectorOptions) *TreeSelectorComponent {
	s := &TreeSelectorComponent{Focused: true, byID: map[string]int{}, active: map[string]bool{}, folded: map[string]bool{}, maxVisible: max(5, terminalHeight/2), onSelect: onSelect, onCancel: onCancel}
	if len(options) > 0 {
		s.options = options[0]
	}
	if s.options.Keybindings == nil {
		s.options.Keybindings = &KeybindingsManager{KeybindingsManager: *tui.NewKeybindingsManager(appKeybindings())}
	}
	s.leaf = optionalHeadlessString(currentLeafID)
	stack := append([]SessionTreeNode{}, tree...)
	for len(stack) > 0 {
		node := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		stack = append(stack, node.Children...)
		entry := cloneSessionEntry(node.Entry)
		n := treeSelectorNode{entry: entry, label: cloneStringPointer(node.Label), labelTimestamp: cloneStringPointer(node.LabelTimestamp)}
		if entry.Message != nil {
			raw, _ := agent.MarshalAgentMessage(entry.Message)
			var m struct {
				Content                           json.RawMessage
				Command, ErrorMessage, StopReason string
			}
			_ = json.Unmarshal(raw, &m)
			n.role = string(entry.Message.MessageRole())
			n.stop = m.StopReason
			n.text = sessionContentText(m.Content, "")
			if n.role == "bashExecution" {
				n.text = m.Command
			}
			n.searchable = n.role + " " + string([]rune(n.text)[:min(200, len([]rune(n.text)))])
			if n.text == "" && n.role == "assistant" {
				n.text = m.ErrorMessage
			}
		} else {
			switch entry.Type {
			case "custom_message":
				raw, _ := json.Marshal(entry.Content)
				n.text = sessionContentText(raw, "")
				n.searchable = entry.CustomType + " " + n.text
			case "branch_summary":
				n.text = entry.Summary
				n.searchable = "branch summary " + n.text
			case "compaction":
				n.text = entry.Summary
				n.searchable = "compaction"
			case "model_change":
				n.searchable = "model " + entry.ModelID
			case "thinking_level_change":
				n.searchable = "thinking " + entry.ThinkingLevel
			case "session_info":
				n.searchable = "title " + optionalHeadlessString(entry.Name)
			case "custom":
				n.searchable = "custom " + entry.CustomType
			case "label":
				n.searchable = "label " + optionalHeadlessString(entry.Label)
			}
		}
		s.byID[entry.ID] = len(s.nodes)
		s.nodes = append(s.nodes, n)
	}
	for id := s.leaf; id != "" && !s.active[id]; {
		s.active[id] = true
		i, ok := s.byID[id]
		if !ok {
			break
		}
		id = optionalHeadlessString(s.nodes[i].entry.ParentID)
	}
	// Keep each subtree together, with the active branch first.
	ordered := []treeSelectorNode{}
	stack = append([]SessionTreeNode{}, tree...)
	order := func(nodes []SessionTreeNode) {
		slices.SortStableFunc(nodes, func(a, b SessionTreeNode) int {
			if s.active[a.Entry.ID] == s.active[b.Entry.ID] {
				return 0
			}
			if s.active[a.Entry.ID] {
				return -1
			}
			return 1
		})
	}
	order(stack)
	slices.Reverse(stack)
	for len(stack) > 0 {
		node := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		ordered = append(ordered, s.nodes[s.byID[node.Entry.ID]])
		children := append([]SessionTreeNode{}, node.Children...)
		order(children)
		slices.Reverse(children)
		stack = append(stack, children...)
	}
	s.nodes = ordered
	for i, n := range s.nodes {
		s.byID[n.entry.ID] = i
	}
	s.filterMode = s.options.InitialFilterMode
	if s.filterMode == "" {
		s.filterMode = "default"
	}
	s.lastSelected = s.options.InitialSelectedID
	if s.lastSelected == "" {
		s.lastSelected = s.leaf
	}
	s.filter()
	return s
}
func (s *TreeSelectorComponent) GetTreeList() (*tui.SelectList, error) {
	if s.list == nil {
		return nil, notImplemented("TreeSelectorComponent.GetTreeList")
	}
	return s.list, nil
}
func (s *TreeSelectorComponent) OnCopy(fn func(string)) error {
	if s.list == nil {
		return notImplemented("TreeSelectorComponent.OnCopy")
	}
	s.onCopy = fn
	return nil
}
func (s *TreeSelectorComponent) remember() {
	if len(s.visible) > 0 {
		s.lastSelected = s.nodes[s.visible[s.selected]].entry.ID
	}
}
func (s *TreeSelectorComponent) filter() {
	s.remember()
	s.visible = nil
	hidden := map[string]bool{}
	for i, n := range s.nodes {
		e := n.entry
		parent := optionalHeadlessString(e.ParentID)
		if s.folded[parent] || hidden[parent] {
			hidden[e.ID] = true
			continue
		}
		if n.role == "assistant" && e.ID != s.leaf && strings.TrimSpace(n.text) == "" && (n.stop == "" || n.stop == "stop" || n.stop == "toolUse") {
			continue
		}
		settings := e.Type == "label" || e.Type == "custom" || e.Type == "model_change" || e.Type == "thinking_level_change" || e.Type == "session_info"
		pass := !settings
		switch s.filterMode {
		case "all":
			pass = true
		case "user-only":
			pass = n.role == "user"
		case "no-tools":
			pass = !settings && n.role != "toolResult"
		case "labeled-only":
			pass = n.label != nil
		}
		for _, token := range strings.Fields(strings.ToLower(s.query)) {
			pass = pass && strings.Contains(strings.ToLower(optionalHeadlessString(n.label)+" "+n.searchable), token)
		}
		if pass {
			s.visible = append(s.visible, i)
		}
	}
	s.selected = max(0, len(s.visible)-1)
	for id := s.lastSelected; id != ""; {
		i, ok := s.byID[id]
		if !ok {
			break
		}
		if v := slices.Index(s.visible, i); v >= 0 {
			s.selected = v
			break
		}
		id = optionalHeadlessString(s.nodes[i].entry.ParentID)
	}
	s.parents = map[int]int{}
	s.children = map[int][]int{}
	visible := map[string]int{}
	items := []tui.SelectItem{}
	for v, i := range s.visible {
		n := s.nodes[i]
		parent := -1
		for id := optionalHeadlessString(n.entry.ParentID); id != ""; {
			if p, ok := visible[id]; ok {
				parent = p
				break
			}
			j, ok := s.byID[id]
			if !ok {
				break
			}
			id = optionalHeadlessString(s.nodes[j].entry.ParentID)
		}
		s.parents[v] = parent
		s.children[parent] = append(s.children[parent], v)
		visible[n.entry.ID] = v
		items = append(items, tui.SelectItem{Value: n.entry.ID, Label: tui.SafeTerminalText(n.searchable)})
	}
	s.list = tui.NewSelectList(items, s.maxVisible, tui.SelectListTheme{})
	s.list.SetKeybindings(&s.options.Keybindings.KeybindingsManager)
	s.list.SetSelectedIndex(s.selected)
	s.list.OnSelect = func(item tui.SelectItem) {
		if s.onSelect != nil {
			s.onSelect(item.Value)
		}
	}
	s.list.OnCancel = s.onCancel
	s.list.OnSelectionChange = func(item tui.SelectItem) { s.selected = slices.Index(s.visible, s.byID[item.Value]); s.remember() }
	s.remember()
}
func (s *TreeSelectorComponent) HandleInput(data string) error {
	if s.list == nil {
		return notImplemented("TreeSelectorComponent.HandleInput")
	}
	match := func(action tui.Keybinding) bool { v, _ := s.options.Keybindings.Matches(data, action); return v }
	if s.labelInput != nil {
		switch {
		case match("tui.select.cancel"):
			s.labelInput = nil
		case match("tui.select.confirm"):
			var label *string
			if text := strings.TrimSpace(s.labelInput.GetValue()); text != "" {
				label = &text
			}
			n := &s.nodes[s.labelNode]
			if s.options.OnLabelChange != nil {
				if err := s.options.OnLabelChange(n.entry.ID, label); err != nil {
					s.status = err.Error()
					return nil
				}
			}
			n.label = label
			n.labelTimestamp = nil
			if label != nil {
				now := time.Now().UTC().Format(time.RFC3339)
				n.labelTimestamp = &now
			}
			s.labelInput = nil
			s.status = ""
		default:
			return s.labelInput.HandleInput(data)
		}
		return nil
	}
	switch {
	case match("tui.select.up"), match("tui.select.down"):
		return s.list.HandleInput(data)
	case match("tui.select.confirm"):
		return s.list.HandleInput(data)
	case match("tui.select.cancel"):
		if s.query != "" {
			s.query = ""
			s.folded = map[string]bool{}
			s.filter()
		} else if s.onCancel != nil {
			s.onCancel()
		}
	case match("app.message.copy"):
		text := ""
		if len(s.visible) > 0 {
			text = s.nodes[s.visible[s.selected]].text
		}
		if s.onCopy != nil {
			s.onCopy(text)
		}
	case match("app.tree.editLabel"):
		if len(s.visible) > 0 {
			s.labelNode = s.visible[s.selected]
			s.labelInput = tui.NewInput()
			s.labelInput.SetKeybindings(&s.options.Keybindings.KeybindingsManager)
			s.labelInput.SetValue(optionalHeadlessString(s.nodes[s.labelNode].label))
			s.labelInput.HandleInput("\x01")
		}
	case match("app.tree.toggleLabelTimestamp"):
		s.showTimestamps = !s.showTimestamps
	case match("app.tree.foldOrUp"), match("app.tree.unfoldOrDown"):
		if len(s.visible) == 0 {
			return nil
		}
		id := s.nodes[s.visible[s.selected]].entry.ID
		up := match("app.tree.foldOrUp")
		parent := s.parents[s.selected]
		foldable := len(s.children[s.selected]) > 0 && (parent < 0 || len(s.children[parent]) > 1)
		if up && foldable && !s.folded[id] {
			s.folded[id] = true
			s.filter()
		} else if !up && s.folded[id] {
			delete(s.folded, id)
			s.filter()
		} else {
			current := s.selected
			for {
				if up {
					p := s.parents[current]
					if p < 0 {
						break
					}
					if len(s.children[p]) > 1 && current < s.selected {
						break
					}
					current = p
				} else {
					children := s.children[current]
					if len(children) == 0 {
						break
					}
					current = children[0]
					if len(children) > 1 {
						break
					}
				}
			}
			s.selected = current
		}
	case match("tui.editor.cursorLeft"), match("tui.select.pageUp"):
		s.selected = max(0, s.selected-s.maxVisible)
	case match("tui.editor.cursorRight"), match("tui.select.pageDown"):
		s.selected = max(0, min(len(s.visible)-1, s.selected+s.maxVisible))
	default:
		modes := []string{"default", "no-tools", "user-only", "labeled-only", "all"}
		changed := false
		for i, action := range []tui.Keybinding{"app.tree.filter.default", "app.tree.filter.noTools", "app.tree.filter.userOnly", "app.tree.filter.labeledOnly", "app.tree.filter.all"} {
			if match(action) {
				mode := modes[i]
				if s.filterMode == mode {
					mode = "default"
				}
				s.filterMode = mode
				changed = true
				break
			}
		}
		if match("app.tree.filter.cycleForward") || match("app.tree.filter.cycleBackward") {
			delta := 1
			if match("app.tree.filter.cycleBackward") {
				delta = -1
			}
			s.filterMode = modes[(max(0, slices.Index(modes, s.filterMode))+delta+len(modes))%len(modes)]
			changed = true
		}
		if match("tui.editor.deleteCharBackward") {
			r := []rune(s.query)
			if len(r) > 0 {
				s.query = string(r[:len(r)-1])
				changed = true
			}
		} else if !changed && data != "" && strings.IndexFunc(data, unicode.IsControl) < 0 {
			s.query += data
			changed = true
		}
		if changed {
			s.folded = map[string]bool{}
			s.filter()
		}
	}
	s.list.SetSelectedIndex(s.selected)
	s.remember()
	return nil
}
func (s *TreeSelectorComponent) Render(width int) ([]string, error) {
	if s.list == nil {
		return s.Container.Render(width)
	}
	if s.labelInput != nil {
		lines, err := s.labelInput.Render(max(1, width-2))
		return selectorLines(append([]string{"Label (empty to remove):", s.status}, lines...), width), err
	}
	lines := []string{"Session Tree · " + s.filterMode, "Search: " + s.query, "↑/↓ select · ←/→ page · ctrl+←/→ fold · L label · ctrl+O filter · esc cancel"}
	if s.status != "" {
		lines = append(lines, s.status)
	}
	if len(s.visible) == 0 {
		lines = append(lines, "No matching entries")
	}
	depths := map[int]int{}
	for v := range s.visible {
		p := s.parents[v]
		if p >= 0 {
			depths[v] = depths[p]
			if len(s.children[p]) > 1 {
				depths[v]++
			}
		}
	}
	start := max(0, min(s.selected-s.maxVisible/2, len(s.visible)-s.maxVisible))
	for v := start; v < min(len(s.visible), start+s.maxVisible); v++ {
		n := s.nodes[s.visible[v]]
		cursor := "  "
		if v == s.selected {
			cursor = "→ "
		}
		marker := ""
		if s.active[n.entry.ID] {
			marker = "• "
		}
		if n.entry.ID == s.leaf {
			marker = "● "
		}
		if s.folded[n.entry.ID] {
			marker += "▸ "
		}
		label := ""
		if n.label != nil {
			label = " [" + *n.label + "]"
			if s.showTimestamps && n.labelTimestamp != nil {
				label += " " + *n.labelTimestamp
			}
		}
		text := n.searchable
		if n.role != "" {
			text = n.role + ": " + n.text
		}
		indent := strings.Repeat("  ", min(depths[v], max(0, (width-24)/2)))
		if depths[v] > 0 {
			indent += "└─ "
		}
		lines = append(lines, cursor+indent+marker+tui.SafeTerminalText(text+label))
	}
	return selectorLines(lines, width), nil
}

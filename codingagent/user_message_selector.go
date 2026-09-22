package codingagent

import "github.com/nankedr/pig/tui"

type UserMessageSelectorComponent struct {
	tui.Container
	list *tui.SelectList
}

func NewUserMessageSelectorComponent(messages []ForkMessage, onSelect func(string), onCancel func(), initialSelectedID ...string) *UserMessageSelectorComponent {
	items := make([]tui.SelectItem, 0, len(messages))
	selected := len(messages) - 1
	for i, message := range messages {
		items = append(items, tui.SelectItem{Value: message.EntryID, Label: tui.SafeTerminalText(message.Text)})
		if len(initialSelectedID) > 0 && message.EntryID == initialSelectedID[0] {
			selected = i
		}
	}
	s := &UserMessageSelectorComponent{list: tui.NewSelectList(items, 10, tui.SelectListTheme{})}
	s.list.SetSelectedIndex(selected)
	s.list.OnSelect = func(item tui.SelectItem) {
		if onSelect != nil {
			onSelect(item.Value)
		}
	}
	s.list.OnCancel = onCancel
	return s
}
func (s *UserMessageSelectorComponent) GetMessageList() (*tui.SelectList, error) {
	if s.list == nil {
		return nil, notImplemented("UserMessageSelectorComponent.GetMessageList")
	}
	return s.list, nil
}
func (s *UserMessageSelectorComponent) HandleInput(data string) error {
	if s.list == nil {
		return notImplemented("UserMessageSelectorComponent.HandleInput")
	}
	return s.list.HandleInput(data)
}
func (s *UserMessageSelectorComponent) Render(width int) ([]string, error) {
	if s.list == nil {
		return s.Container.Render(width)
	}
	lines, err := s.list.Render(width)
	return selectorLines(append([]string{"Fork from Message", "Select a user message to copy the active path into a new session", ""}, lines...), width), err
}

package codingagent

import (
	"context"
	"errors"
	"github.com/nankedr/pig/tui"
	"strings"
)

func (m *InteractiveMode) selectFork(ctx context.Context) error {
	messages, err := m.runtime.Session().GetUserMessagesForForking()
	if err != nil {
		return err
	}
	if len(messages) == 0 {
		return m.appendNotice("\nNo messages to fork from\n")
	}
	selected := ""
	done, finish := selectorSignal()
	selector := NewUserMessageSelectorComponent(messages, func(id string) { selected = id; finish() }, finish)
	selector.list.SetKeybindings(&m.options.Keybindings.KeybindingsManager)
	if err = m.waitSelector(ctx, selector, done); err != nil || selected == "" {
		return err
	}
	if m.settleSession != nil {
		if err = m.settleSession(); err != nil {
			return err
		}
	}
	result, err := m.runtime.Fork(ctx, selected)
	if err != nil || result.Cancelled {
		return err
	}
	if err = m.sessionChanged("Forked to new session"); err != nil {
		return err
	}
	return m.ui.SetEditorText(optionalHeadlessString(result.SelectedText), false)
}
func (m *InteractiveMode) selectTree(ctx context.Context) error {
	s := m.runtime.Session()
	manager := s.SessionManager()
	initial := ""
	for {
		tree, err := manager.GetTree()
		if err != nil {
			return err
		}
		if len(tree) == 0 {
			return m.appendNotice("\nNo entries in session\n")
		}
		mode, err := s.SettingsManager().GetTreeFilterMode()
		if err != nil {
			return err
		}
		height, _ := m.ui.Terminal().Rows()
		selected := ""
		done, finish := selectorSignal()
		selector := NewTreeSelectorComponent(tree, manager.GetLeafID(), height, func(id string) { selected = id; finish() }, finish, TreeSelectorOptions{InitialSelectedID: initial, InitialFilterMode: mode, Keybindings: m.options.Keybindings, OnLabelChange: func(id string, label *string) error {
			s.mu.Lock()
			defer s.mu.Unlock()
			if err := s.configurationReady(); err != nil {
				return err
			}
			_, err := manager.AppendLabelChange(id, label)
			return err
		}})
		selector.OnCopy(func(text string) {
			if text == "" {
				selector.status = "Selected entry has no text to copy"
				return
			}
			if err := CopyToClipboard(text); err != nil {
				selector.status = err.Error()
			} else {
				selector.status = "Copied selected message to clipboard"
			}
		})
		if err = m.waitSelector(ctx, selector, done); err != nil || selected == "" {
			return err
		}
		if selected == optionalHeadlessString(manager.GetLeafID()) {
			return m.appendNotice("\nAlready at this point\n")
		}
		initial = selected
		options, back, err := m.branchSummaryOptions(ctx)
		if err != nil {
			return err
		}
		if back {
			continue
		}
		if err = m.restoreQueued(); err != nil {
			return err
		}
		if m.settleSession != nil {
			if err = m.settleSession(); err != nil {
				return err
			}
		}
		result, err := m.navigateTree(ctx, selected, options)
		if result.Aborted {
			_ = m.appendNotice("\nBranch summarization cancelled\n")
			if ctx.Err() != nil {
				return ctx.Err()
			}
			continue
		}
		if err != nil {
			return err
		}
		if result.Cancelled {
			return m.appendNotice("\nNavigation cancelled\n")
		}
		m.sessionRevision++
		if err = m.sessionChanged("Navigated to selected point"); err != nil {
			return err
		}
		if result.EditorText != nil {
			return m.ui.SetEditorText(*result.EditorText, true)
		}
		return nil
	}
}
func (m *InteractiveMode) branchSummaryOptions(ctx context.Context) (NavigateTreeOptions, bool, error) {
	options := NavigateTreeOptions{}
	skip, err := m.runtime.Session().SettingsManager().GetBranchSummarySkipPrompt()
	if err != nil || skip {
		return options, false, err
	}
	for {
		choice := ""
		done, finish := selectorSignal()
		dialog := tui.NewSelectDialog("Summarize branch?", []tui.SelectItem{{Value: "none", Label: "No summary"}, {Value: "summary", Label: "Summarize"}, {Value: "custom", Label: "Summarize with custom prompt"}}, func(item tui.SelectItem) { choice = item.Value; finish() }, finish)
		dialog.List.SetKeybindings(&m.options.Keybindings.KeybindingsManager)
		if err = m.waitSelector(ctx, dialog, done); err != nil || choice == "" {
			return options, true, err
		}
		options.Summarize = choice != "none"
		if choice == "custom" {
			done, finish = selectorSignal()
			accepted := false
			prompt := &branchPrompt{title: "Custom summarization instructions", editor: tui.NewEditor(nil, tui.EditorTheme{}, tui.EditorOptions{Keybindings: &m.options.Keybindings.KeybindingsManager}), bindings: m.options.Keybindings, cancel: finish}
			prompt.editor.SetOnSubmit(func(text string) { options.CustomInstructions = text; accepted = true; finish() })
			if err = m.waitSelector(ctx, prompt, done); err != nil {
				return options, false, err
			}
			if !accepted {
				continue
			}
		}
		return options, false, nil
	}
}
func (m *InteractiveMode) navigateTree(ctx context.Context, id string, options NavigateTreeOptions) (NavigateTreeResult, error) {
	if !options.Summarize {
		return m.runtime.Session().NavigateTree(ctx, id, options)
	}
	run, cancel := context.WithCancel(ctx)
	defer cancel()
	prompt := &branchPrompt{title: "Summarizing branch… (Esc to cancel)", editor: tui.NewEditor(nil, tui.EditorTheme{}, tui.EditorOptions{Keybindings: &m.options.Keybindings.KeybindingsManager}), bindings: m.options.Keybindings, cancel: cancel}
	prompt.editor.DisableSubmit = true
	if err := m.ui.SetDialog(prompt); err != nil {
		return NavigateTreeResult{}, err
	}
	// Terminal shutdown must also cancel the Provider request while Run is in navigation.
	done := make(chan struct{})
	go func() {
		select {
		case <-m.ui.Done():
			cancel()
		case <-done:
		}
	}()
	result, err := m.runtime.Session().NavigateTree(run, id, options)
	close(done)
	err = errors.Join(err, m.ui.SetDialog(nil))
	if draft, _ := prompt.editor.GetExpandedText(); strings.TrimSpace(draft) != "" {
		err = errors.Join(err, m.ui.PrependEditor(draft))
	}
	return result, err
}

type branchPrompt struct {
	title    string
	editor   *tui.Editor
	bindings *KeybindingsManager
	cancel   func()
}

func (p *branchPrompt) Invalidate() error { return nil }
func (p *branchPrompt) Render(width int) ([]string, error) {
	lines, err := p.editor.Render(width)
	return append([]string{p.title}, lines...), err
}
func (p *branchPrompt) HandleInput(data string) error {
	if cancel, _ := p.bindings.Matches(data, "tui.select.cancel"); cancel {
		p.cancel()
		return nil
	}
	return p.editor.HandleInput(data)
}

package tui

import (
	"context"
	"errors"
	"io"
)

// SelectDialog provides clamped navigation for a modal choice; nil means cancel.
type SelectDialog struct {
	List  *SelectList
	title string
}

func NewSelectDialog(title string, items []SelectItem, onSelect func(SelectItem), onCancel func()) *SelectDialog {
	list := NewSelectList(items, max(1, len(items)), SelectListTheme{})
	list.OnSelect, list.OnCancel = onSelect, onCancel
	return &SelectDialog{List: list, title: title}
}
func (*SelectDialog) Invalidate() error { return nil }
func (d *SelectDialog) Render(width int) ([]string, error) {
	lines, err := WrapTextWithANSI(d.title, max(1, width))
	if err != nil {
		return nil, err
	}
	choices, err := d.List.Render(width)
	if err != nil {
		return nil, err
	}
	lines = append(append(lines, ""), choices...)
	hint, err := WrapTextWithANSI("↑↓ navigate · Enter select · Esc cancel", max(1, width))
	return append(append(lines, ""), hint...), err
}
func (d *SelectDialog) HandleInput(data string) error {
	kb, _ := GetKeybindings()
	up, _ := kb.Matches(data, "tui.select.up")
	down, _ := kb.Matches(data, "tui.select.down")
	switch {
	case up || data == "k":
		return d.List.SetSelectedIndex(d.List.selected - 1)
	case down || data == "j":
		return d.List.SetSelectedIndex(d.List.selected + 1)
	case data == "\n":
		data = "\r"
	}
	return d.List.HandleInput(data)
}

// ShowSelectDialog owns the terminal until a selection, cancellation, or error.
func ShowSelectDialog(ctx context.Context, terminal Terminal, title string, items []SelectItem) (selected *SelectItem, err error) {
	if ctx == nil {
		return nil, errors.New("dialog context must not be nil")
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	result := make(chan *SelectItem, 1)
	finish := func(item *SelectItem) {
		select {
		case result <- item:
		default:
		}
	}
	dialog := NewSelectDialog(title, items, func(item SelectItem) { finish(&item) }, func() { finish(nil) })
	ui := NewTUIMainScreen(terminal)
	ui.AddChild(dialog)
	if err = ui.SetFocus(dialog); err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, ui.Stop()) }()
	if err = ui.Start(); err != nil {
		return nil, err
	}
	var done <-chan struct{}
	if terminal, ok := terminal.(interface{ Done() <-chan struct{} }); ok {
		done = terminal.Done()
	}
	select {
	case selected = <-result:
		return selected, nil
	case err = <-ui.renderErrors:
		return nil, err
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-done:
		if terminal, ok := terminal.(interface{ Err() error }); ok && terminal.Err() != nil {
			return nil, terminal.Err()
		}
		return nil, io.EOF
	}
}

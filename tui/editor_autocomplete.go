package tui

import (
	"context"
	"strings"
)

type editorAutocomplete struct {
	provider AutocompleteProvider
	cancel   context.CancelFunc
	pending  chan autocompleteResponse
	list     *SelectList
	prefix   string
	force    bool
}
type autocompleteResponse struct {
	suggestions     AutocompleteSuggestions
	ok              bool
	err             error
	text            string
	cursor          int
	force, explicit bool
}

func (e *Editor) SetAutocompleteProvider(provider AutocompleteProvider) error {
	e.cancelAutocomplete()
	e.completion.provider = provider
	return nil
}
func (e *Editor) SetAutocompleteMaxVisible(value int) error {
	e.autocompleteMaxVisible = max(3, min(20, value))
	if e.completion.list != nil {
		e.completion.list.maxVisible = e.autocompleteMaxVisible
	}
	return nil
}
func (e *Editor) IsShowingAutocomplete() bool {
	_ = e.pollAutocomplete()
	return e.completion.list != nil
}
func (e *Editor) cancelAutocomplete() {
	if e.completion.cancel != nil {
		e.completion.cancel()
	}
	e.completion.cancel = nil
	e.completion.pending = nil
	e.completion.list = nil
	e.completion.prefix = ""
}
func (e *Editor) requestAutocomplete(force, explicit bool) error {
	provider := e.completion.provider
	if provider == nil {
		return nil
	}
	cursor := e.GetCursor()
	lines := e.GetLines()
	if force {
		ok, err := provider.ShouldTriggerFileCompletion(lines, cursor.Line, cursor.Col)
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}
	}
	e.cancelAutocomplete()
	ctx, cancel := context.WithCancel(context.Background())
	pending := make(chan autocompleteResponse, 1)
	e.completion.cancel = cancel
	e.completion.pending = pending
	e.completion.force = force
	text, pos, ui := e.text, e.cursor, e.ui
	go func() {
		suggestions, ok, err := provider.GetSuggestions(ctx, lines, cursor.Line, cursor.Col, AutocompleteOptions{Force: force})
		if ctx.Err() != nil {
			return
		}
		pending <- autocompleteResponse{suggestions, ok, err, text, pos, force, explicit}
		if ui != nil {
			_ = ui.RequestRender()
		}
	}()
	return nil
}
func (e *Editor) pollAutocomplete() error {
	select {
	case response := <-e.completion.pending:
		e.completion.pending = nil
		if response.text != e.text || response.cursor != e.cursor {
			return nil
		}
		if response.err != nil {
			e.cancelAutocomplete()
			return response.err
		}
		if !response.ok || len(response.suggestions.Items) == 0 {
			e.cancelAutocomplete()
			return nil
		}
		e.completion.prefix = response.suggestions.Prefix
		items := make([]SelectItem, len(response.suggestions.Items))
		for i, item := range response.suggestions.Items {
			label := item.Label
			if label == "" {
				label = item.Value
			}
			var description *string
			if item.Description != nil {
				safe := terminalText(*item.Description)
				description = &safe
			}
			items[i] = SelectItem{Value: item.Value, Label: terminalText(label), Description: description}
		}
		e.completion.list = NewSelectList(items, e.autocompleteMaxVisible, e.theme.SelectList)
		best := -1
		for i, item := range items {
			if item.Value == e.completion.prefix {
				best = i
				break
			}
			if best < 0 && e.completion.prefix != "" && strings.HasPrefix(item.Value, e.completion.prefix) {
				best = i
			}
		}
		if best >= 0 {
			_ = e.completion.list.SetSelectedIndex(best)
		}
		if response.force && response.explicit && len(items) == 1 {
			return e.acceptAutocomplete(true)
		}
	default:
	}
	return nil
}
func (e *Editor) acceptAutocomplete(notify bool) error {
	item, err := e.completion.list.GetSelectedItem()
	if err != nil || item == nil {
		return err
	}
	cursor := e.GetCursor()
	result, err := e.completion.provider.ApplyCompletion(e.GetLines(), cursor.Line, cursor.Col, AutocompleteItem{Value: item.Value, Label: item.Label, Description: item.Description}, e.completion.prefix)
	if err != nil {
		return err
	}
	if _, err = completionBefore(result.Lines, result.CursorLine, result.CursorCol); err != nil {
		return err
	}
	e.snapshot()
	e.exitHistory()
	e.lastAction = ""
	e.text = strings.Join(result.Lines, "\n")
	e.cursor = result.CursorCol
	for _, line := range result.Lines[:result.CursorLine] {
		e.cursor += len(line) + 1
	}
	e.cancelAutocomplete()
	if notify {
		e.changed()
	}
	return nil
}
func (e *Editor) naturalAutocomplete(data string, deletion bool) bool {
	cursor := e.GetCursor()
	before := e.GetLines()[cursor.Line][:cursor.Col]
	slash := cursor.Line == 0 && strings.HasPrefix(strings.TrimLeft(before, " \t"), "/")
	if data == "/" && slash && strings.TrimSpace(before) == "/" {
		return true
	}
	prefix := completionPrefix(before)
	triggers := append([]string{"@"}, e.completion.provider.TriggerCharacters()...)
	symbol := false
	for _, trigger := range triggers {
		if trigger != "" && strings.HasPrefix(prefix, trigger) {
			symbol = true
			if data == trigger {
				return true
			}
		}
	}
	typed := strings.ContainsAny(data, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789.-_")
	return (typed || deletion) && (slash || symbol)
}

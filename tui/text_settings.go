package tui

import "fmt"

// TextUIInteractionOptions changes rendering and input behavior without replacing the editor.
type TextUIInteractionOptions struct {
	HideThinking, ShowHardwareCursor, ClearOnShrink, ShowTerminalProgress bool
	EditorPaddingX, AutocompleteMaxVisible                                int
	DoubleEscapeAction                                                    string
	Scrollbar                                                             ScrollViewScrollbar
	ExitTranscript                                                        bool
}

func (u *TextUI) HideThinking() bool { u.mu.Lock(); defer u.mu.Unlock(); return u.hideThinking }
func (u *TextUI) ApplyInteractionOptions(options TextUIInteractionOptions) error {
	if options.EditorPaddingX < 0 || options.EditorPaddingX > 3 || options.AutocompleteMaxVisible < 3 || options.AutocompleteMaxVisible > 20 {
		return fmt.Errorf("invalid editor settings")
	}
	switch options.DoubleEscapeAction {
	case "tree", "fork", "none":
	default:
		return fmt.Errorf("invalid double-escape action")
	}
	switch options.Scrollbar {
	case ScrollViewScrollbarAuto, ScrollViewScrollbarAlways, ScrollViewScrollbarHidden:
	default:
		return fmt.Errorf("invalid scrollbar")
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.stopped {
		return fmt.Errorf("text UI is stopped")
	}
	oldProgress := u.interaction.ShowTerminalProgress
	u.interaction = options
	u.hideThinking = options.HideThinking
	u.exitTranscript = options.ExitTranscript
	u.scrollbar = options.Scrollbar
	_ = u.scroll.SetScrollbar(options.Scrollbar)
	_ = u.editor.SetPaddingX(options.EditorPaddingX)
	_ = u.editor.SetAutocompleteMaxVisible(options.AutocompleteMaxVisible)
	if u.started && !u.stopped && !u.paused {
		if oldProgress && !options.ShowTerminalProgress {
			if err := u.terminal.Write("\x1b]9;4;0;\x07"); err != nil {
				return err
			}
		}
		if options.ShowTerminalProgress {
			if err := u.terminal.Write(u.progressSequence()); err != nil {
				return err
			}
		}
		return u.render()
	}
	return nil
}
func (u *TextUI) progressSequence() string {
	if u.turn != 0 {
		return "\x1b]9;4;3;\x07"
	}
	return "\x1b]9;4;0;\x07"
}

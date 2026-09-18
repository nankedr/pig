package main

import (
	"fmt"
	"github.com/nankedr/pig/tui"
)

func main() {
	keys := tui.NewKeybindingsManager(tui.NewTUIKeybindings(), tui.KeybindingsConfig{tui.KeybindingInputSubmit: {"ctrl+s"}, tui.KeybindingEditorUndo: {"ctrl+s"}})
	conflicts, _ := keys.GetConflicts()
	fmt.Printf("conflicts: %v\n", conflicts)
	editor := tui.NewEditor(nil, tui.EditorTheme{}, tui.EditorOptions{Keybindings: keys})
	editor.OnSubmit = func(text string) { fmt.Printf("submitted: %q\n", text) }
	buffer := tui.NewStdinBuffer()
	defer buffer.Destroy()
	buffer.SetHandlers(tui.StdinBufferEventMap{Data: func(data string) { editor.HandleInput(data) }})
	// Remove the conflict before submitting.
	keys.SetUserBindings(tui.KeybindingsConfig{tui.KeybindingInputSubmit: {"ctrl+s"}})
	for _, chunk := range []string{"你好", "\x1b[97", "u", "a", "\x13"} {
		buffer.Process([]byte(chunk))
	}
}

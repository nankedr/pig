package codingagent

import (
	"encoding/json"
	"github.com/nankedr/pig/tui"
	"os"
	"path/filepath"
	"runtime"
)

type KeybindingsManager struct {
	tui.KeybindingsManager
	configPath string
}

func NewKeybindingsManager(agentDir ...string) (*KeybindingsManager, error) {
	dir := ""
	if len(agentDir) > 0 {
		dir = agentDir[0]
	}
	if dir == "" {
		var err error
		dir, err = GetAgentDir()
		if err != nil {
			return nil, err
		}
	}
	m := &KeybindingsManager{KeybindingsManager: *tui.NewKeybindingsManager(appKeybindings()), configPath: filepath.Join(dir, "keybindings.json")}
	return m, m.Reload()
}
func (m *KeybindingsManager) GetEffectiveConfig() (tui.KeybindingsConfig, error) {
	return m.GetResolvedBindings()
}
func (m *KeybindingsManager) Reload() error {
	if m.configPath == "" {
		return nil
	}
	config := tui.KeybindingsConfig{}
	data, err := os.ReadFile(m.configPath)
	var raw map[string]json.RawMessage
	if err == nil && json.Unmarshal(data, &raw) == nil {
		for key, value := range raw {
			action := key
			if next, ok := keybindingMigrations[key]; ok {
				action = next
				if _, present := raw[next]; present {
					continue
				}
			}
			var single string
			var keys []tui.KeyID
			if json.Unmarshal(value, &single) == nil {
				keys = []tui.KeyID{tui.KeyID(single)}
			} else if string(value) == "null" || json.Unmarshal(value, &keys) != nil {
				continue
			}
			config[tui.Keybinding(action)] = keys
		}
	}
	return m.SetUserBindings(config)
}
func appKeybindings() tui.KeybindingDefinitions {
	definitions := tui.NewTUIKeybindings()
	add := func(action tui.Keybinding, description string, keys ...tui.KeyID) {
		definitions[action] = tui.KeybindingDefinition{DefaultKeys: keys, Description: &description}
	}
	add("app.interrupt", "Cancel or abort", "escape")
	add("app.clear", "Clear editor", "ctrl+c")
	add("app.exit", "Exit when editor is empty", "ctrl+d")
	add("app.suspend", "Suspend to background", "ctrl+z")
	if runtime.GOOS == "windows" {
		d := definitions["app.suspend"]
		d.DefaultKeys = []tui.KeyID{}
		definitions["app.suspend"] = d
	}
	add("app.thinking.cycle", "Cycle thinking level", "shift+tab")
	add("app.model.cycleForward", "Cycle to next model", "ctrl+p")
	add("app.model.cycleBackward", "Cycle to previous model", "shift+ctrl+p")
	add("app.model.select", "Open model selector", "ctrl+l")
	add("app.tools.expand", "Toggle tool output", "ctrl+o")
	add("app.thinking.toggle", "Toggle thinking blocks", "ctrl+t")
	add("app.session.toggleNamedFilter", "Toggle named session filter", "ctrl+n")
	add("app.editor.external", "Open external editor", "ctrl+g")
	add("app.message.copy", "Copy message to clipboard", "ctrl+x")
	add("app.message.followUp", "Queue follow-up message", "alt+enter")
	add("app.message.dequeue", "Restore queued messages", "alt+up")
	add("app.clipboard.pasteImage", "Paste image from clipboard (text fallback)", "ctrl+v")
	if runtime.GOOS == "windows" {
		d := definitions["app.clipboard.pasteImage"]
		d.DefaultKeys = []tui.KeyID{"alt+v"}
		definitions["app.clipboard.pasteImage"] = d
	}
	add("app.session.new", "Start a new session")
	add("app.session.tree", "Open session tree")
	add("app.session.fork", "Fork current session")
	add("app.session.resume", "Resume a session")
	add("app.tree.foldOrUp", "Fold tree branch or move up", "ctrl+left", "alt+left")
	if runtime.GOOS == "darwin" {
		d := definitions["app.tree.foldOrUp"]
		d.DefaultKeys = []tui.KeyID{"alt+left", "ctrl+left"}
		definitions["app.tree.foldOrUp"] = d
	}
	add("app.tree.unfoldOrDown", "Unfold tree branch or move down", "ctrl+right", "alt+right")
	if runtime.GOOS == "darwin" {
		d := definitions["app.tree.unfoldOrDown"]
		d.DefaultKeys = []tui.KeyID{"alt+right", "ctrl+right"}
		definitions["app.tree.unfoldOrDown"] = d
	}
	add("app.tree.editLabel", "Edit tree label", "shift+l")
	add("app.tree.toggleLabelTimestamp", "Toggle tree label timestamps", "shift+t")
	add("app.session.togglePath", "Toggle session path display", "ctrl+p")
	add("app.session.toggleSort", "Toggle session sort mode", "ctrl+s")
	add("app.session.rename", "Rename session", "ctrl+r")
	add("app.session.delete", "Delete session", "ctrl+d")
	add("app.session.deleteNoninvasive", "Delete session when query is empty", "ctrl+backspace")
	add("app.models.save", "Save model selection", "ctrl+s")
	add("app.models.enableAll", "Enable all models", "ctrl+a")
	add("app.models.clearAll", "Clear all models", "ctrl+x")
	add("app.models.toggleProvider", "Toggle all models for provider", "ctrl+p")
	add("app.models.reorderUp", "Move model up in order", "alt+up")
	add("app.models.reorderDown", "Move model down in order", "alt+down")
	add("app.tree.filter.default", "Tree filter: default view", "ctrl+d")
	add("app.tree.filter.noTools", "Tree filter: hide tool results", "ctrl+t")
	add("app.tree.filter.userOnly", "Tree filter: user messages only", "ctrl+u")
	add("app.tree.filter.labeledOnly", "Tree filter: labeled entries only", "ctrl+l")
	add("app.tree.filter.all", "Tree filter: show all entries", "ctrl+a")
	add("app.tree.filter.cycleForward", "Tree filter: cycle forward", "ctrl+o")
	add("app.tree.filter.cycleBackward", "Tree filter: cycle backward", "shift+ctrl+o")
	return definitions
}

var keybindingMigrations = map[string]string{
	"cursorUp":                 "tui.editor.cursorUp",
	"cursorDown":               "tui.editor.cursorDown",
	"cursorLeft":               "tui.editor.cursorLeft",
	"cursorRight":              "tui.editor.cursorRight",
	"cursorWordLeft":           "tui.editor.cursorWordLeft",
	"cursorWordRight":          "tui.editor.cursorWordRight",
	"cursorLineStart":          "tui.editor.cursorLineStart",
	"cursorLineEnd":            "tui.editor.cursorLineEnd",
	"jumpForward":              "tui.editor.jumpForward",
	"jumpBackward":             "tui.editor.jumpBackward",
	"pageUp":                   "tui.editor.pageUp",
	"pageDown":                 "tui.editor.pageDown",
	"deleteCharBackward":       "tui.editor.deleteCharBackward",
	"deleteCharForward":        "tui.editor.deleteCharForward",
	"deleteWordBackward":       "tui.editor.deleteWordBackward",
	"deleteWordForward":        "tui.editor.deleteWordForward",
	"deleteToLineStart":        "tui.editor.deleteToLineStart",
	"deleteToLineEnd":          "tui.editor.deleteToLineEnd",
	"yank":                     "tui.editor.yank",
	"yankPop":                  "tui.editor.yankPop",
	"undo":                     "tui.editor.undo",
	"newLine":                  "tui.input.newLine",
	"submit":                   "tui.input.submit",
	"tab":                      "tui.input.tab",
	"copy":                     "tui.input.copy",
	"selectUp":                 "tui.select.up",
	"selectDown":               "tui.select.down",
	"selectPageUp":             "tui.select.pageUp",
	"selectPageDown":           "tui.select.pageDown",
	"selectConfirm":            "tui.select.confirm",
	"selectCancel":             "tui.select.cancel",
	"interrupt":                "app.interrupt",
	"clear":                    "app.clear",
	"exit":                     "app.exit",
	"suspend":                  "app.suspend",
	"cycleThinkingLevel":       "app.thinking.cycle",
	"cycleModelForward":        "app.model.cycleForward",
	"cycleModelBackward":       "app.model.cycleBackward",
	"selectModel":              "app.model.select",
	"expandTools":              "app.tools.expand",
	"toggleThinking":           "app.thinking.toggle",
	"toggleSessionNamedFilter": "app.session.toggleNamedFilter",
	"externalEditor":           "app.editor.external",
	"followUp":                 "app.message.followUp",
	"dequeue":                  "app.message.dequeue",
	"pasteImage":               "app.clipboard.pasteImage",
	"newSession":               "app.session.new",
	"tree":                     "app.session.tree",
	"fork":                     "app.session.fork",
	"resume":                   "app.session.resume",
	"treeFoldOrUp":             "app.tree.foldOrUp",
	"treeUnfoldOrDown":         "app.tree.unfoldOrDown",
	"treeEditLabel":            "app.tree.editLabel",
	"treeToggleLabelTimestamp": "app.tree.toggleLabelTimestamp",
	"toggleSessionPath":        "app.session.togglePath",
	"toggleSessionSort":        "app.session.toggleSort",
	"renameSession":            "app.session.rename",
	"deleteSession":            "app.session.delete",
	"deleteSessionNoninvasive": "app.session.deleteNoninvasive",
}

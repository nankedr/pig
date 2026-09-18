package codingagent_test

import (
	"github.com/nankedr/pig/codingagent"
	"github.com/nankedr/pig/tui"
	"os"
	"path/filepath"
	"testing"
)

func TestKeybindingsFile111(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "keybindings.json")
	if err := os.WriteFile(path, []byte(`{"submit":"ctrl+s","tui.input.submit":"ctrl+enter","app.session.new":"ctrl+n","app.exit":[],"app.clear":42}`), 0600); err != nil {
		t.Fatal(err)
	}
	m, err := codingagent.NewKeybindingsManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		data   string
		action tui.Keybinding
		want   bool
	}{{"\x1b[13;5u", tui.KeybindingInputSubmit, true}, {"\x13", tui.KeybindingInputSubmit, false}, {"\x0e", "app.session.new", true}, {"\x04", "app.exit", false}, {"\x03", "app.clear", true}} {
		got, err := m.Matches(c.data, c.action)
		if err != nil || got != c.want {
			t.Fatalf("%s: %t %v", c.action, got, err)
		}
	}
	if err = os.WriteFile(path, []byte(`{"submit":"ctrl+s"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err = m.Reload(); err != nil {
		t.Fatal(err)
	}
	if got, _ := m.Matches("\x13", tui.KeybindingInputSubmit); !got {
		t.Fatal("reload did not apply")
	}
	if err = os.WriteFile(path, []byte(`{broken`), 0600); err != nil {
		t.Fatal(err)
	}
	if err = m.Reload(); err != nil {
		t.Fatal(err)
	}
	if got, _ := m.Matches("\r", tui.KeybindingInputSubmit); !got {
		t.Fatal("malformed config must fall back to defaults")
	}
}

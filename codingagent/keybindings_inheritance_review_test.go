package codingagent_test

import (
	"reflect"
	"testing"

	"github.com/nankedr/pig/codingagent"
	"github.com/nankedr/pig/tui"
)

var (
	_ func(*codingagent.KeybindingsManager, string, tui.Keybinding) (bool, error)                   = (*codingagent.KeybindingsManager).Matches
	_ func(*codingagent.KeybindingsManager, tui.Keybinding) ([]tui.KeyID, error)                    = (*codingagent.KeybindingsManager).GetKeys
	_ func(*codingagent.KeybindingsManager, tui.Keybinding) (tui.KeybindingDefinition, bool, error) = (*codingagent.KeybindingsManager).GetDefinition
	_ func(*codingagent.KeybindingsManager) ([]tui.KeybindingConflict, error)                       = (*codingagent.KeybindingsManager).GetConflicts
	_ func(*codingagent.KeybindingsManager, tui.KeybindingsConfig) error                            = (*codingagent.KeybindingsManager).SetUserBindings
	_ func(*codingagent.KeybindingsManager) (tui.KeybindingsConfig, error)                          = (*codingagent.KeybindingsManager).GetUserBindings
	_ func(*codingagent.KeybindingsManager) (tui.KeybindingsConfig, error)                          = (*codingagent.KeybindingsManager).GetResolvedBindings

	_ func(*codingagent.KeybindingsManager) (tui.KeybindingsConfig, error) = (*codingagent.KeybindingsManager).GetEffectiveConfig
	_ func(*codingagent.KeybindingsManager) error                          = (*codingagent.KeybindingsManager).Reload
	_ func(...string) (*codingagent.KeybindingsManager, error)             = codingagent.NewKeybindingsManager
)

func TestKeybindingsManagerInheritance(t *testing.T) {
	t.Run("embeds the canonical TUI manager by value", func(t *testing.T) {
		managerType := reflect.TypeOf(codingagent.KeybindingsManager{})
		field, ok := managerType.FieldByName("KeybindingsManager")
		if !ok {
			t.Fatal("KeybindingsManager does not embed tui.KeybindingsManager")
		}
		if !field.Anonymous || field.Type != reflect.TypeOf(tui.KeybindingsManager{}) {
			t.Fatalf("embedded field = anonymous %t type %v, want anonymous value tui.KeybindingsManager", field.Anonymous, field.Type)
		}
	})

	t.Run("exposes the complete inherited and local method set", func(t *testing.T) {
		wantMethods := []string{
			"GetConflicts",
			"GetDefinition",
			"GetEffectiveConfig",
			"GetKeys",
			"GetResolvedBindings",
			"GetUserBindings",
			"Matches",
			"Reload",
			"SetUserBindings",
		}
		managerType := reflect.TypeOf((*codingagent.KeybindingsManager)(nil))
		if managerType.NumMethod() != len(wantMethods) {
			t.Fatalf("method count = %d, want %d", managerType.NumMethod(), len(wantMethods))
		}
		for _, methodName := range wantMethods {
			if _, ok := managerType.MethodByName(methodName); !ok {
				t.Errorf("method %s is missing", methodName)
			}
		}
	})

}

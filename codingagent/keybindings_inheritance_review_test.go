package codingagent_test

import (
	"errors"
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

	t.Run("promoted methods retain TUI ownership on the zero value", func(t *testing.T) {
		manager := &codingagent.KeybindingsManager{}
		tests := []struct {
			operation string
			call      func() error
		}{
			{"KeybindingsManager.matches", func() error {
				matched, err := manager.Matches("enter", tui.KeybindingInputSubmit)
				if matched {
					t.Error("Matches result = true, want false")
				}
				return err
			}},
			{"KeybindingsManager.getKeys", func() error {
				keys, err := manager.GetKeys(tui.KeybindingInputSubmit)
				if keys != nil {
					t.Errorf("GetKeys result = %#v, want nil", keys)
				}
				return err
			}},
			{"KeybindingsManager.getDefinition", func() error {
				definition, ok, err := manager.GetDefinition(tui.KeybindingInputSubmit)
				if ok || !reflect.DeepEqual(definition, tui.KeybindingDefinition{}) {
					t.Errorf("GetDefinition result = (%#v, %t), want zero definition and false", definition, ok)
				}
				return err
			}},
			{"KeybindingsManager.getConflicts", func() error {
				conflicts, err := manager.GetConflicts()
				if conflicts != nil {
					t.Errorf("GetConflicts result = %#v, want nil", conflicts)
				}
				return err
			}},
			{"KeybindingsManager.setUserBindings", func() error {
				return manager.SetUserBindings(tui.KeybindingsConfig{tui.KeybindingInputSubmit: {"ctrl+enter"}})
			}},
			{"KeybindingsManager.getUserBindings", func() error {
				bindings, err := manager.GetUserBindings()
				if bindings != nil {
					t.Errorf("GetUserBindings result = %#v, want nil", bindings)
				}
				return err
			}},
			{"KeybindingsManager.getResolvedBindings", func() error {
				bindings, err := manager.GetResolvedBindings()
				if bindings != nil {
					t.Errorf("GetResolvedBindings result = %#v, want nil", bindings)
				}
				return err
			}},
		}

		for _, test := range tests {
			t.Run(test.operation, func(t *testing.T) {
				assertKeybindingsNotImplemented(t, test.call(), "tui", test.operation)
			})
		}
	})

	t.Run("local methods retain Coding Agent ownership", func(t *testing.T) {
		manager := &codingagent.KeybindingsManager{}
		config, err := manager.GetEffectiveConfig()
		if config != nil {
			t.Fatalf("GetEffectiveConfig result = %#v, want nil", config)
		}
		assertKeybindingsNotImplemented(t, err, "codingagent", "KeybindingsManager.GetEffectiveConfig")
		assertKeybindingsNotImplemented(t, manager.Reload(), "codingagent", "KeybindingsManager.Reload")
	})

	t.Run("constructor remains an inert capability stub", func(t *testing.T) {
		manager, err := codingagent.NewKeybindingsManager("invalid\x00path")
		if manager != nil {
			t.Fatalf("NewKeybindingsManager result = %#v, want nil", manager)
		}
		assertKeybindingsNotImplemented(t, err, "codingagent", "NewKeybindingsManager")
	})
}

func assertKeybindingsNotImplemented(t *testing.T, err error, module, operation string) {
	t.Helper()
	if !errors.Is(err, codingagent.ErrNotImplemented) {
		t.Fatalf("error = %v, want ErrNotImplemented", err)
	}
	var unavailable *codingagent.NotImplementedError
	if !errors.As(err, &unavailable) {
		t.Fatalf("error = %T, want *codingagent.NotImplementedError", err)
	}
	if unavailable.Module != module || unavailable.Operation != operation {
		t.Fatalf("NotImplementedError = %#v, want module %q operation %q", unavailable, module, operation)
	}
}

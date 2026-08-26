package codingagent_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
)

var (
	_ func(*codingagent.ModelRuntime) ([]ai.Model, error)                                       = (*codingagent.ModelRuntime).GetAvailableSnapshot
	_ func(*codingagent.ModelRuntime, ai.Model) (codingagent.CompatibilityRequestConfig, error) = (*codingagent.ModelRuntime).GetCompatibilityRequestConfig
	_ func(*codingagent.ModelRuntime) (string, error)                                           = (*codingagent.ModelRuntime).GetError
	_ func(*codingagent.ModelRuntime, string, string) (ai.Model, bool, error)                   = (*codingagent.ModelRuntime).GetModel
	_ func(*codingagent.ModelRuntime, ...string) ([]ai.Model, error)                            = (*codingagent.ModelRuntime).GetModels
	_ func(*codingagent.ModelRuntime, string) (ai.Provider, bool, error)                        = (*codingagent.ModelRuntime).GetProvider
	_ func(*codingagent.ModelRuntime, string) (codingagent.AuthStatus, error)                   = (*codingagent.ModelRuntime).GetProviderAuthStatus
	_ func(*codingagent.ModelRuntime) ([]ai.Provider, error)                                    = (*codingagent.ModelRuntime).GetProviders
	_ func(*codingagent.ModelRuntime, string) (ai.Provider, bool, error)                        = (*codingagent.ModelRuntime).GetRegisteredNativeProvider
	_ func(*codingagent.ModelRuntime, string) (codingagent.ProviderConfigInput, bool, error)    = (*codingagent.ModelRuntime).GetRegisteredProviderConfig
	_ func(*codingagent.ModelRuntime) ([]string, error)                                         = (*codingagent.ModelRuntime).GetRegisteredProviderIDs
	_ func(*codingagent.ModelRuntime, string) (bool, error)                                     = (*codingagent.ModelRuntime).HasConfiguredAuth
	_ func(*codingagent.ModelRuntime, string) (bool, error)                                     = (*codingagent.ModelRuntime).IsUsingOAuth
	_ func(*codingagent.ModelRuntime, string) (bool, error)                                     = (*codingagent.ModelRuntime).IsUsingSubscription

	_ func(*codingagent.ModelRegistry, string, string) (ai.Model, bool, error)                = (*codingagent.ModelRegistry).Find
	_ func(*codingagent.ModelRegistry) ([]ai.Model, error)                                    = (*codingagent.ModelRegistry).GetAll
	_ func(*codingagent.ModelRegistry) ([]ai.Model, error)                                    = (*codingagent.ModelRegistry).GetAvailable
	_ func(*codingagent.ModelRegistry) (string, error)                                        = (*codingagent.ModelRegistry).GetError
	_ func(*codingagent.ModelRegistry, string) (ai.Provider, bool, error)                     = (*codingagent.ModelRegistry).GetProvider
	_ func(*codingagent.ModelRegistry, string) (codingagent.AuthStatus, error)                = (*codingagent.ModelRegistry).GetProviderAuthStatus
	_ func(*codingagent.ModelRegistry, string) (string, error)                                = (*codingagent.ModelRegistry).GetProviderDisplayName
	_ func(*codingagent.ModelRegistry, string) (ai.Provider, bool, error)                     = (*codingagent.ModelRegistry).GetRegisteredNativeProvider
	_ func(*codingagent.ModelRegistry, string) (codingagent.ProviderConfigInput, bool, error) = (*codingagent.ModelRegistry).GetRegisteredProviderConfig
	_ func(*codingagent.ModelRegistry) ([]string, error)                                      = (*codingagent.ModelRegistry).GetRegisteredProviderIDs
	_ func(*codingagent.ModelRegistry, ai.Model) (bool, error)                                = (*codingagent.ModelRegistry).HasConfiguredAuth
	_ func(*codingagent.ModelRegistry, ai.Model) (bool, error)                                = (*codingagent.ModelRegistry).IsUsingOAuth
)

func TestCreateModelRuntimeOptionsModelsPathPreservesTriState(t *testing.T) {
	requireOptionalString := func(ai.Optional[string]) {}

	absent := codingagent.CreateModelRuntimeOptions{}
	requireOptionalString(absent.ModelsPath)
	if absent.ModelsPath.IsSet() {
		t.Fatal("zero ModelsPath is set, want absent")
	}

	null := codingagent.CreateModelRuntimeOptions{ModelsPath: ai.Null[string]()}
	if !null.ModelsPath.IsSet() || !null.ModelsPath.IsNull() {
		t.Fatalf("null ModelsPath state = (set %t, null %t), want (true, true)", null.ModelsPath.IsSet(), null.ModelsPath.IsNull())
	}

	present := codingagent.CreateModelRuntimeOptions{ModelsPath: ai.Some("models.json")}
	value, ok := present.ModelsPath.Value()
	if !present.ModelsPath.IsSet() || present.ModelsPath.IsNull() || !ok || value != "models.json" {
		t.Fatalf("present ModelsPath = (%q, %t, set %t, null %t), want models.json/true/true/false", value, ok, present.ModelsPath.IsSet(), present.ModelsPath.IsNull())
	}
}

func TestUnsupportedModelRuntimeGettersReturnStructuredErrors(t *testing.T) {
	var runtime codingagent.ModelRuntime
	tests := []struct {
		operation string
		call      func(*testing.T) error
	}{
		{"ModelRuntime.GetAvailableSnapshot", func(t *testing.T) error {
			got, err := runtime.GetAvailableSnapshot()
			requireModelGetterZero(t, got)
			return err
		}},
		{"ModelRuntime.GetCompatibilityRequestConfig", func(t *testing.T) error {
			got, err := runtime.GetCompatibilityRequestConfig(ai.Model{})
			requireModelGetterZero(t, got)
			return err
		}},
		{"ModelRuntime.GetError", func(t *testing.T) error {
			got, err := runtime.GetError()
			requireModelGetterZero(t, got)
			return err
		}},
		{"ModelRuntime.GetModel", func(t *testing.T) error {
			got, found, err := runtime.GetModel("provider", "model")
			requireModelGetterZero(t, got)
			requireModelGetterZero(t, found)
			return err
		}},
		{"ModelRuntime.GetModels", func(t *testing.T) error {
			got, err := runtime.GetModels("provider")
			requireModelGetterZero(t, got)
			return err
		}},
		{"ModelRuntime.GetProvider", func(t *testing.T) error {
			got, found, err := runtime.GetProvider("provider")
			requireModelGetterZero(t, got)
			requireModelGetterZero(t, found)
			return err
		}},
		{"ModelRuntime.GetProviderAuthStatus", func(t *testing.T) error {
			got, err := runtime.GetProviderAuthStatus("provider")
			requireModelGetterZero(t, got)
			return err
		}},
		{"ModelRuntime.GetProviders", func(t *testing.T) error {
			got, err := runtime.GetProviders()
			requireModelGetterZero(t, got)
			return err
		}},
		{"ModelRuntime.GetRegisteredNativeProvider", func(t *testing.T) error {
			got, found, err := runtime.GetRegisteredNativeProvider("provider")
			requireModelGetterZero(t, got)
			requireModelGetterZero(t, found)
			return err
		}},
		{"ModelRuntime.GetRegisteredProviderConfig", func(t *testing.T) error {
			got, found, err := runtime.GetRegisteredProviderConfig("provider")
			requireModelGetterZero(t, got)
			requireModelGetterZero(t, found)
			return err
		}},
		{"ModelRuntime.GetRegisteredProviderIDs", func(t *testing.T) error {
			got, err := runtime.GetRegisteredProviderIDs()
			requireModelGetterZero(t, got)
			return err
		}},
		{"ModelRuntime.HasConfiguredAuth", func(t *testing.T) error {
			got, err := runtime.HasConfiguredAuth("provider")
			requireModelGetterZero(t, got)
			return err
		}},
		{"ModelRuntime.IsUsingOAuth", func(t *testing.T) error {
			got, err := runtime.IsUsingOAuth("provider")
			requireModelGetterZero(t, got)
			return err
		}},
		{"ModelRuntime.IsUsingSubscription", func(t *testing.T) error {
			got, err := runtime.IsUsingSubscription("provider")
			requireModelGetterZero(t, got)
			return err
		}},
	}

	runUnsupportedModelGetterTests(t, tests)
}

func TestUnsupportedModelRegistryGettersReturnStructuredErrors(t *testing.T) {
	registry := codingagent.NewModelRegistry(&codingagent.ModelRuntime{})
	tests := []struct {
		operation string
		call      func(*testing.T) error
	}{
		{"ModelRegistry.Find", func(t *testing.T) error {
			got, found, err := registry.Find("provider", "model")
			requireModelGetterZero(t, got)
			requireModelGetterZero(t, found)
			return err
		}},
		{"ModelRegistry.GetAll", func(t *testing.T) error {
			got, err := registry.GetAll()
			requireModelGetterZero(t, got)
			return err
		}},
		{"ModelRegistry.GetAvailable", func(t *testing.T) error {
			got, err := registry.GetAvailable()
			requireModelGetterZero(t, got)
			return err
		}},
		{"ModelRegistry.GetError", func(t *testing.T) error {
			got, err := registry.GetError()
			requireModelGetterZero(t, got)
			return err
		}},
		{"ModelRegistry.GetProvider", func(t *testing.T) error {
			got, found, err := registry.GetProvider("provider")
			requireModelGetterZero(t, got)
			requireModelGetterZero(t, found)
			return err
		}},
		{"ModelRegistry.GetProviderAuthStatus", func(t *testing.T) error {
			got, err := registry.GetProviderAuthStatus("provider")
			requireModelGetterZero(t, got)
			return err
		}},
		{"ModelRegistry.GetProviderDisplayName", func(t *testing.T) error {
			got, err := registry.GetProviderDisplayName("provider")
			requireModelGetterZero(t, got)
			return err
		}},
		{"ModelRegistry.GetRegisteredNativeProvider", func(t *testing.T) error {
			got, found, err := registry.GetRegisteredNativeProvider("provider")
			requireModelGetterZero(t, got)
			requireModelGetterZero(t, found)
			return err
		}},
		{"ModelRegistry.GetRegisteredProviderConfig", func(t *testing.T) error {
			got, found, err := registry.GetRegisteredProviderConfig("provider")
			requireModelGetterZero(t, got)
			requireModelGetterZero(t, found)
			return err
		}},
		{"ModelRegistry.GetRegisteredProviderIDs", func(t *testing.T) error {
			got, err := registry.GetRegisteredProviderIDs()
			requireModelGetterZero(t, got)
			return err
		}},
		{"ModelRegistry.HasConfiguredAuth", func(t *testing.T) error {
			got, err := registry.HasConfiguredAuth(ai.Model{})
			requireModelGetterZero(t, got)
			return err
		}},
		{"ModelRegistry.IsUsingOAuth", func(t *testing.T) error {
			got, err := registry.IsUsingOAuth(ai.Model{})
			requireModelGetterZero(t, got)
			return err
		}},
	}

	runUnsupportedModelGetterTests(t, tests)
}

func runUnsupportedModelGetterTests(t *testing.T, tests []struct {
	operation string
	call      func(*testing.T) error
}) {
	t.Helper()
	for _, test := range tests {
		t.Run(test.operation, func(t *testing.T) {
			err := test.call(t)
			if !errors.Is(err, codingagent.ErrNotImplemented) {
				t.Fatalf("error = %v, want ErrNotImplemented", err)
			}
			var target *codingagent.NotImplementedError
			if !errors.As(err, &target) {
				t.Fatalf("errors.As(%v, *NotImplementedError) = false", err)
			}
			if target.Module != "codingagent" || target.Operation != test.operation {
				t.Fatalf("NotImplementedError = %#v, want module codingagent operation %s", target, test.operation)
			}
		})
	}
}

func requireModelGetterZero[T any](t *testing.T, got T) {
	t.Helper()
	var zero T
	if !reflect.DeepEqual(got, zero) {
		t.Fatalf("result = %#v, want zero %#v", got, zero)
	}
}

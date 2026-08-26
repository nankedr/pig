package codingagent_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
)

var (
	_ func(*codingagent.ModelRuntime, context.Context, string, ...codingagent.ModelRuntimeAuthOverrides) (ai.Optional[ai.AuthResult], error)   = (*codingagent.ModelRuntime).GetAuth
	_ func(*codingagent.ModelRuntime, context.Context, ai.Model, ...codingagent.ModelRuntimeAuthOverrides) (ai.Optional[ai.AuthResult], error) = (*codingagent.ModelRuntime).GetModelAuth
	_ func(*codingagent.ModelRegistry, string, codingagent.ProviderConfigInput) error                                                          = (*codingagent.ModelRegistry).RegisterProvider
	_ func(*codingagent.ModelRegistry, ai.Provider) error                                                                                      = (*codingagent.ModelRegistry).RegisterNativeProvider
)

func TestModelRuntimeAuthOverloadProjections(t *testing.T) {
	runtime := &codingagent.ModelRuntime{}
	overrides := codingagent.ModelRuntimeAuthOverrides{
		APIKey:             "runtime-key",
		Env:                ai.ProviderEnv{"MODEL_TOKEN": "token"},
		MinOAuthValidityMS: 30_000,
	}
	wantOverrides := codingagent.ModelRuntimeAuthOverrides{
		APIKey:             "runtime-key",
		Env:                ai.ProviderEnv{"MODEL_TOKEN": "token"},
		MinOAuthValidityMS: 30_000,
	}

	t.Run("provider ID overload", func(t *testing.T) {
		got, err := runtime.GetAuth(context.Background(), "provider", overrides)
		if want := ai.Absent[ai.AuthResult](); !reflect.DeepEqual(got, want) {
			t.Fatalf("GetAuth result = %#v, want zero %#v", got, want)
		}
		requireModelOverloadStubError(t, err, "ModelRuntime.GetAuth")
		if !reflect.DeepEqual(overrides, wantOverrides) {
			t.Fatalf("GetAuth overrides = %#v, want unchanged %#v", overrides, wantOverrides)
		}
	})

	t.Run("model overload", func(t *testing.T) {
		model := ai.Model{
			ID:       "model",
			Provider: "provider",
			Headers:  map[string]string{"X-Model": "value"},
		}
		wantModel := ai.Model{
			ID:       "model",
			Provider: "provider",
			Headers:  map[string]string{"X-Model": "value"},
		}

		got, err := runtime.GetModelAuth(context.Background(), model, overrides)
		if want := ai.Absent[ai.AuthResult](); !reflect.DeepEqual(got, want) {
			t.Fatalf("GetModelAuth result = %#v, want zero %#v", got, want)
		}
		requireModelOverloadStubError(t, err, "ModelRuntime.GetModelAuth")
		if !reflect.DeepEqual(model, wantModel) {
			t.Fatalf("GetModelAuth model = %#v, want unchanged %#v", model, wantModel)
		}
		if !reflect.DeepEqual(overrides, wantOverrides) {
			t.Fatalf("GetModelAuth overrides = %#v, want unchanged %#v", overrides, wantOverrides)
		}
	})
}

func TestModelRegistryProviderOverloadProjections(t *testing.T) {
	registry := codingagent.NewModelRegistry(&codingagent.ModelRuntime{})
	header := "value"
	wantHeader := "value"
	config := codingagent.ProviderConfigInput{
		Name:    "Provider",
		Headers: ai.ProviderHeaders{"X-Provider": &header},
		Models:  []ai.Model{{ID: "model", Provider: "provider"}},
	}
	wantConfig := codingagent.ProviderConfigInput{
		Name:    "Provider",
		Headers: ai.ProviderHeaders{"X-Provider": &wantHeader},
		Models:  []ai.Model{{ID: "model", Provider: "provider"}},
	}

	t.Run("config overload", func(t *testing.T) {
		err := registry.RegisterProvider("provider", config)
		requireModelOverloadStubError(t, err, "ModelRegistry.RegisterProvider")
		if !reflect.DeepEqual(config, wantConfig) {
			t.Fatalf("RegisterProvider config = %#v, want unchanged %#v", config, wantConfig)
		}
	})

	t.Run("native provider overload", func(t *testing.T) {
		err := registry.RegisterNativeProvider(&callbackTrapProvider{})
		requireModelOverloadStubError(t, err, "ModelRegistry.RegisterNativeProvider")
	})
}

type callbackTrapProvider struct {
	ai.Provider
}

func requireModelOverloadStubError(t *testing.T, err error, operation string) {
	t.Helper()
	if !errors.Is(err, codingagent.ErrNotImplemented) {
		t.Fatalf("error = %v, want ErrNotImplemented", err)
	}
	var target *codingagent.NotImplementedError
	if !errors.As(err, &target) {
		t.Fatalf("errors.As(%v, *NotImplementedError) = false", err)
	}
	if target.Module != "codingagent" || target.Operation != operation {
		t.Fatalf("NotImplementedError = %#v, want module codingagent operation %s", target, operation)
	}
}

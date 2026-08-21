package ai_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/nankedr/pig/ai"
)

var (
	_ = ai.AnthropicAuthTokenEnv    // upstream: ANTHROPIC_AUTH_TOKEN_ENV
	_ = ai.AnthropicOAuthTokenEnv   // upstream: ANTHROPIC_OAUTH_TOKEN_ENV
	_ = ai.AnthropicAPIKeyEnv       // upstream: ANTHROPIC_API_KEY_ENV
	_ = ai.FindEnvKeys              // upstream: findEnvKeys
	_ = ai.GetEnvAPIKey             // upstream: getEnvApiKey
	_ = ai.OAuthPrompt{}            // upstream: OAuthPrompt
	_ = ai.OAuthAuthInfo{}          // upstream: OAuthAuthInfo
	_ = ai.OAuthDeviceCodeInfo{}    // upstream: OAuthDeviceCodeInfo
	_ = ai.OAuthSelectOption{}      // upstream: OAuthSelectOption
	_ = ai.OAuthSelectPrompt{}      // upstream: OAuthSelectPrompt
	_ = ai.OAuthLoginCallbacks{}    // upstream: OAuthLoginCallbacks
	_ = ai.RegisterBunOAuthFlows    // upstream: registerBunOAuthFlows
	_ = ai.CloudflareWorkersAIAuth  // upstream: cloudflareWorkersAIAuth
	_ = ai.CloudflareAIGatewayAuth  // upstream: cloudflareAIGatewayAuth
	_ = ai.SetBedrockProviderModule // upstream: setBedrockProviderModule
)

func TestAmbientEnvHelpersRequireExplicitEnvironment(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "process-secret")

	if keys, err := ai.FindEnvKeys(ai.ProviderIDOpenAI); keys != nil || !errors.Is(err, ai.ErrNotImplemented) {
		t.Fatalf("FindEnvKeys without explicit env = (%v, %v), want nil and ErrNotImplemented", keys, err)
	}
	if key, ok, err := ai.GetEnvAPIKey(ai.ProviderIDOpenAI); key != "" || ok || !errors.Is(err, ai.ErrNotImplemented) {
		t.Fatalf("GetEnvAPIKey without explicit env = (%q, %t, %v), want empty/false/ErrNotImplemented", key, ok, err)
	}

	env := ai.ProviderEnv{
		"ANTHROPIC_AUTH_TOKEN":  "bearer-token",
		"ANTHROPIC_OAUTH_TOKEN": "oauth-token",
		"ANTHROPIC_API_KEY":     "api-key",
		"OPENAI_API_KEY":        "explicit-secret",
	}
	keys, err := ai.FindEnvKeys(ai.ProviderIDAnthropic, env)
	if err != nil || !reflect.DeepEqual(keys, []string{
		ai.AnthropicAuthTokenEnv,
		ai.AnthropicOAuthTokenEnv,
		ai.AnthropicAPIKeyEnv,
	}) {
		t.Fatalf("FindEnvKeys(anthropic) = (%v, %v)", keys, err)
	}
	key, ok, err := ai.GetEnvAPIKey(ai.ProviderIDAnthropic, env)
	if err != nil || !ok || key != "oauth-token" {
		t.Fatalf("GetEnvAPIKey(anthropic) = (%q, %t, %v)", key, ok, err)
	}
	key, ok, err = ai.GetEnvAPIKey(ai.ProviderIDOpenAI, env)
	if err != nil || !ok || key != "explicit-secret" {
		t.Fatalf("GetEnvAPIKey(openai) = (%q, %t, %v)", key, ok, err)
	}
	key, ok, err = ai.GetEnvAPIKey(ai.ProviderIDGoogleVertex, ai.ProviderEnv{
		"GOOGLE_APPLICATION_CREDENTIALS": "/not-read/adc.json",
		"GOOGLE_CLOUD_PROJECT":           "project",
		"GOOGLE_CLOUD_LOCATION":          "location",
	})
	if key != "" || ok || !errors.Is(err, ai.ErrNotImplemented) {
		t.Fatalf("GetEnvAPIKey(vertex ADC) = (%q, %t, %v), want explicit stub", key, ok, err)
	}
}

func TestLegacyOAuthCompatibilityTypesRemainCompileUsable(t *testing.T) {
	t.Parallel()

	prompt := ai.OAuthPrompt{Message: "Paste code", Placeholder: ptr("code"), AllowEmpty: ptr(false)}
	selectPrompt := ai.OAuthSelectPrompt{
		Message: "Choose account",
		Options: []ai.OAuthSelectOption{{ID: "one", Label: "One"}},
	}
	callbacks := ai.OAuthLoginCallbacks{
		Signal:       context.Background(),
		OnAuth:       func(ai.OAuthAuthInfo) {},
		OnDeviceCode: func(ai.OAuthDeviceCodeInfo) {},
		OnPrompt: func(context.Context, ai.OAuthPrompt) (string, error) {
			return "code", nil
		},
		OnSelect: func(context.Context, ai.OAuthSelectPrompt) (ai.Optional[string], error) {
			return ai.Some("one"), nil
		},
	}
	if prompt.Message == "" || len(selectPrompt.Options) != 1 || callbacks.Signal == nil || callbacks.OnPrompt == nil || callbacks.OnSelect == nil {
		t.Fatalf("legacy OAuth contract is incomplete: %#v %#v %#v", prompt, selectPrompt, callbacks)
	}
	if err := ai.RegisterBunOAuthFlows(); !errors.Is(err, ai.ErrNotImplemented) {
		t.Fatalf("RegisterBunOAuthFlows error = %v, want ErrNotImplemented", err)
	}
}

func TestBedrockModuleOverrideIsAnExplicitStub(t *testing.T) {
	t.Parallel()

	module := ai.NewStubProviderStreams()
	if err := ai.SetBedrockProviderModule(module); !errors.Is(err, ai.ErrNotImplemented) {
		t.Fatalf("SetBedrockProviderModule error = %v, want ErrNotImplemented", err)
	}
	stream := ai.BedrockProviderModule.Stream(context.Background(), ai.Model{}, ai.Context{}, ai.StreamOptions{})
	if _, err := stream.Result(context.Background()); !errors.Is(err, ai.ErrNotImplemented) {
		t.Fatalf("BedrockProviderModule stream error = %v, want ErrNotImplemented", err)
	}
}

func TestCloudflareAmbientAuthFactoriesAreSideEffectFreeStubs(t *testing.T) {
	t.Parallel()

	for _, factory := range []func() ai.APIKeyAuth{
		ai.CloudflareWorkersAIAuth,
		ai.CloudflareAIGatewayAuth,
	} {
		auth := factory()
		if auth.Check != nil {
			t.Fatal("Cloudflare Check must be absent")
		}
		ambient := &fakeAuthContext{env: map[string]string{"CLOUDFLARE_API_KEY": "secret"}}
		result, err := auth.Resolve(context.Background(), ai.APIKeyResolveInput{
			Context: ambient,
		})
		if result.IsSet() || !errors.Is(err, ai.ErrNotImplemented) {
			t.Fatalf("Cloudflare Resolve = (%#v, %v)", result, err)
		}
		if ambient.calls != 0 {
			t.Fatalf("Cloudflare Resolve read environment %d times", ambient.calls)
		}
	}
}

package ai_test

import (
	"context"
	"errors"
	"testing"

	"github.com/nankedr/pig/ai"
)

var _ = ai.RadiusProviderOptions{} // upstream: RadiusProviderOptions

func TestProviderFactoriesExposeEveryBuiltinProvider(t *testing.T) {
	t.Parallel()

	tests := []struct {
		id      ai.ProviderID
		factory func() ai.Provider
	}{
		{ai.ProviderIDAmazonBedrock, ai.AmazonBedrockProvider},
		{ai.ProviderIDAntLing, ai.AntLingProvider},
		{ai.ProviderIDAnthropic, ai.AnthropicProvider},
		{ai.ProviderIDAzureOpenAIResponses, ai.AzureOpenAIResponsesProvider},
		{ai.ProviderIDBaseten, ai.BasetenProvider},
		{ai.ProviderIDCerebras, ai.CerebrasProvider},
		{ai.ProviderIDCloudflareAIGateway, ai.CloudflareAIGatewayProvider},
		{ai.ProviderIDCloudflareWorkersAI, ai.CloudflareWorkersAIProvider},
		{ai.ProviderIDDeepSeek, ai.DeepSeekProvider},
		{ai.ProviderIDFireworks, ai.FireworksProvider},
		{ai.ProviderIDGitHubCopilot, ai.GitHubCopilotProvider},
		{ai.ProviderIDGoogle, ai.GoogleProvider},
		{ai.ProviderIDGoogleVertex, ai.GoogleVertexProvider},
		{ai.ProviderIDGroq, ai.GroqProvider},
		{ai.ProviderIDHuggingFace, ai.HuggingFaceProvider},
		{ai.ProviderIDKimiCoding, ai.KimiCodingProvider},
		{ai.ProviderIDMiniMax, ai.MiniMaxProvider},
		{ai.ProviderIDMiniMaxCN, ai.MiniMaxCNProvider},
		{ai.ProviderIDMistral, ai.MistralProvider},
		{ai.ProviderIDMoonshotAI, ai.MoonshotAIProvider},
		{ai.ProviderIDMoonshotAICN, ai.MoonshotAICNProvider},
		{ai.ProviderIDNVIDIA, ai.NVIDIAProvider},
		{ai.ProviderIDOpenAI, ai.OpenAIProvider},
		{ai.ProviderIDOpenAICodex, ai.OpenAICodexProvider},
		{ai.ProviderIDOpenCode, ai.OpenCodeProvider},
		{ai.ProviderIDOpenCodeGo, ai.OpenCodeGoProvider},
		{ai.ProviderIDOpenRouter, ai.OpenRouterProvider},
		{ai.ProviderIDQwenTokenPlan, ai.QwenTokenPlanProvider},
		{ai.ProviderIDQwenTokenPlanCN, ai.QwenTokenPlanCNProvider},
		{ai.ProviderIDQwenTokenPlanIndividual, ai.QwenTokenPlanIndividualProvider},
		{ai.ProviderIDRadius, func() ai.Provider { return ai.RadiusProvider() }},
		{ai.ProviderIDTogether, ai.TogetherProvider},
		{ai.ProviderIDVercelAIGateway, ai.VercelAIGatewayProvider},
		{ai.ProviderIDXAI, ai.XAIProvider},
		{ai.ProviderIDXiaomi, ai.XiaomiProvider},
		{ai.ProviderIDXiaomiTokenPlanAMS, ai.XiaomiTokenPlanAMSProvider},
		{ai.ProviderIDXiaomiTokenPlanCN, ai.XiaomiTokenPlanCNProvider},
		{ai.ProviderIDXiaomiTokenPlanSGP, ai.XiaomiTokenPlanSGPProvider},
		{ai.ProviderIDZAI, ai.ZAIProvider},
		{ai.ProviderIDZAICodingCN, ai.ZAICodingCNProvider},
	}

	if len(tests) != 40 {
		t.Fatalf("provider factory count = %d, want 40", len(tests))
	}
	for _, test := range tests {
		test := test
		t.Run(string(test.id), func(t *testing.T) {
			t.Parallel()

			first := test.factory()
			second := test.factory()
			if first == nil || second == nil {
				t.Fatal("factory returned nil")
			}
			if first.ID() != test.id || second.ID() != test.id {
				t.Fatalf("factory ids = (%q, %q), want %q", first.ID(), second.ID(), test.id)
			}
			if first == second {
				t.Fatal("factory reused a provider instance")
			}
		})
	}
}

func TestDeferredProviderFactoryDoesNotInvokeRequestHooks(t *testing.T) {
	t.Parallel()

	provider := ai.GroqProvider()
	model := ai.Model{ID: "stub", Provider: provider.ID(), API: ai.APIOpenAICompletions}
	invoked := 0
	stream := provider.Stream(context.Background(), model, ai.Context{}, ai.OpenAICompletionsOptions{
		StreamOptions: ai.StreamOptions{ProviderRequestOptions: ai.ProviderRequestOptions{
			Fetch: func(context.Context, ai.FetchRequest) (ai.FetchResponse, error) {
				invoked++
				return ai.FetchResponse{}, nil
			},
			OnPayload: func(context.Context, ai.JSONValue, ai.Model) (ai.PayloadHookResult, error) {
				invoked++
				return ai.PayloadHookResult{}, nil
			},
			OnResponse: func(context.Context, ai.ProviderResponse, ai.Model) error {
				invoked++
				return nil
			},
		}},
	})
	if stream == nil {
		t.Fatal("stub stream is nil")
	}
	if _, err := stream.Result(context.Background()); !errors.Is(err, ai.ErrNotImplemented) {
		t.Fatalf("stub stream error = %v, want ErrNotImplemented", err)
	}
	if invoked != 0 {
		t.Fatalf("stub invoked request hooks %d times, want zero", invoked)
	}
}

func TestRadiusProviderOptionsMapIdentityWithoutLoadingGateway(t *testing.T) {
	t.Parallel()

	provider := ai.RadiusProvider(ai.RadiusProviderOptions{
		ID:      "private-radius",
		Name:    "Private Radius",
		Gateway: "https://radius.example.test",
	})
	if provider.ID() != "private-radius" || provider.Name() != "Private Radius" {
		t.Fatalf("provider identity = (%q, %q)", provider.ID(), provider.Name())
	}
	if _, ok := provider.BaseURL().Value(); ok {
		t.Fatal("Radius gateway leaked into model API base URL")
	}
	if auth := provider.Auth(); auth.OAuth == nil || auth.OAuth.Name != "Private Radius" {
		t.Fatalf("Radius OAuth metadata = %#v, want custom provider name", auth.OAuth)
	}
	if models := provider.GetModels(); len(models) != 0 {
		t.Fatalf("provider loaded %d dynamic models, want zero", len(models))
	}
	stream := provider.Stream(context.Background(), ai.Model{
		ID:       "wire-probe",
		Provider: "private-radius",
		API:      ai.APIPiMessages,
	}, ai.Context{}, ai.PiMessagesOptions{})
	if _, err := stream.Result(context.Background()); !errors.Is(err, ai.ErrNotImplemented) {
		t.Fatalf("Radius pi-messages stream error = %v, want ErrNotImplemented", err)
	}

	published := 0
	err := provider.RefreshModels(ai.RefreshModelsContext{
		Context:      context.Background(),
		AllowNetwork: true,
		Publish: func(ai.ModelsPublication) (bool, error) {
			published++
			return true, nil
		},
	})
	if !errors.Is(err, ai.ErrNotImplemented) {
		t.Fatalf("Radius RefreshModels error = %v, want ErrNotImplemented", err)
	}
	if published != 0 {
		t.Fatalf("Radius RefreshModels published %d times, want zero", published)
	}
}

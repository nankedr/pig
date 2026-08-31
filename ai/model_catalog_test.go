package ai_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/nankedr/pig/ai"
)

var (
	_ ai.ModelGroup   = map[string]ai.Model{}
	_ ai.ModelGroups  = []ai.ModelGroup{}
	_ ai.ModelCatalog = map[string]ai.Model{}

	_ func(ai.ProviderID, ai.ModelGroups) ai.ModelCatalog = ai.FlattenModelCatalog
	_ func() []ai.BuiltinProvider                         = ai.GetBuiltinProviders
	_ func(ai.BuiltinProvider) []ai.Model                 = ai.GetBuiltinModels
	_ func(ai.BuiltinProvider, string) (ai.Model, bool)   = ai.GetBuiltinModel
	_ func() (int64, bool)                                = ai.GetBuiltinModelDataGeneratedAt
	_ func() []ai.Provider                                = ai.BuiltinProviders
	_ func(...ai.CreateModelsOptions) ai.MutableModels    = ai.BuiltinModels
)

func TestFlattenModelCatalogPreservesOrderedLastGroupWinsSemantics(t *testing.T) {
	t.Parallel()

	groups := ai.ModelGroups{
		ai.ModelGroup{
			"shared":     {ID: "shared", Name: "first", Provider: ai.ProviderIDAnthropic},
			"only-first": {ID: "only-first", Name: "only first", Provider: ai.ProviderIDAnthropic},
		},
		ai.ModelGroup{
			"shared": {ID: "shared", Name: "second", Provider: ai.ProviderIDAnthropic},
		},
	}

	catalog := ai.FlattenModelCatalog(ai.ProviderIDOpenAI, groups)

	if len(catalog) != 2 {
		t.Fatalf("len(catalog) = %d, want 2", len(catalog))
	}
	if got := catalog["shared"]; got.Name != "second" {
		t.Fatalf("catalog[shared].Name = %q, want later group to win", got.Name)
	}
	if got := catalog["only-first"]; got.Provider != ai.ProviderIDAnthropic {
		t.Fatalf("catalog[only-first].Provider = %q, want input identity preserved", got.Provider)
	}
}

func TestBuiltinCatalogReportsStaticProvidersAndStagedSnapshot(t *testing.T) {
	t.Parallel()

	wantProviders := []ai.BuiltinProvider{
		ai.ProviderIDAmazonBedrock,
		ai.ProviderIDAntLing,
		ai.ProviderIDAnthropic,
		ai.ProviderIDAzureOpenAIResponses,
		ai.ProviderIDBaseten,
		ai.ProviderIDCerebras,
		ai.ProviderIDCloudflareAIGateway,
		ai.ProviderIDCloudflareWorkersAI,
		ai.ProviderIDDeepSeek,
		ai.ProviderIDFireworks,
		ai.ProviderIDGitHubCopilot,
		ai.ProviderIDGoogle,
		ai.ProviderIDGoogleVertex,
		ai.ProviderIDGroq,
		ai.ProviderIDHuggingFace,
		ai.ProviderIDKimiCoding,
		ai.ProviderIDMiniMax,
		ai.ProviderIDMiniMaxCN,
		ai.ProviderIDMistral,
		ai.ProviderIDMoonshotAI,
		ai.ProviderIDMoonshotAICN,
		ai.ProviderIDNVIDIA,
		ai.ProviderIDOpenAI,
		ai.ProviderIDOpenAICodex,
		ai.ProviderIDOpenCode,
		ai.ProviderIDOpenCodeGo,
		ai.ProviderIDOpenRouter,
		ai.ProviderIDQwenTokenPlan,
		ai.ProviderIDQwenTokenPlanCN,
		ai.ProviderIDQwenTokenPlanIndividual,
		ai.ProviderIDTogether,
		ai.ProviderIDVercelAIGateway,
		ai.ProviderIDXAI,
		ai.ProviderIDXiaomi,
		ai.ProviderIDXiaomiTokenPlanAMS,
		ai.ProviderIDXiaomiTokenPlanCN,
		ai.ProviderIDXiaomiTokenPlanSGP,
		ai.ProviderIDZAI,
		ai.ProviderIDZAICodingCN,
	}

	gotProviders := ai.GetBuiltinProviders()
	if !reflect.DeepEqual(gotProviders, wantProviders) {
		t.Fatalf("GetBuiltinProviders() = %#v, want fixed generated order %#v", gotProviders, wantProviders)
	}
	gotProviders[0] = ai.ProviderIDRadius
	if gotAgain := ai.GetBuiltinProviders(); !reflect.DeepEqual(gotAgain, wantProviders) {
		t.Fatalf("GetBuiltinProviders() shares its result: %#v", gotAgain)
	}

	for _, provider := range wantProviders {
		models := ai.GetBuiltinModels(provider)
		if provider == ai.ProviderIDDeepSeek {
			if len(models) != 2 {
				t.Fatalf("GetBuiltinModels(deepseek) = %#v, want two M1 models", models)
			}
		} else if len(models) != 0 {
			t.Fatalf("GetBuiltinModels(%q) = %#v, want deferred catalog", provider, models)
		}
	}
	if models := ai.GetBuiltinModels(ai.BuiltinProvider("unknown")); len(models) != 0 {
		t.Fatalf("GetBuiltinModels(unknown) = %#v, want empty", models)
	}
	if model, ok := ai.GetBuiltinModel(ai.ProviderIDOpenAI, "missing"); ok || !reflect.DeepEqual(model, ai.Model{}) {
		t.Fatalf("GetBuiltinModel() = (%#v, %t), want zero, false", model, ok)
	}
	if generatedAt, ok := ai.GetBuiltinModelDataGeneratedAt(); !ok || generatedAt != 1786081866002 {
		t.Fatalf("GetBuiltinModelDataGeneratedAt() = (%d, %t), want locked snapshot time", generatedAt, ok)
	}
}

func TestBuiltinProvidersAndModelsAreFreshOrderedOfflineAssemblies(t *testing.T) {
	t.Parallel()

	wantProviderIDs := []ai.ProviderID{
		ai.ProviderIDAmazonBedrock,
		ai.ProviderIDAntLing,
		ai.ProviderIDAnthropic,
		ai.ProviderIDAzureOpenAIResponses,
		ai.ProviderIDBaseten,
		ai.ProviderIDCerebras,
		ai.ProviderIDCloudflareAIGateway,
		ai.ProviderIDCloudflareWorkersAI,
		ai.ProviderIDDeepSeek,
		ai.ProviderIDFireworks,
		ai.ProviderIDGitHubCopilot,
		ai.ProviderIDGoogle,
		ai.ProviderIDGoogleVertex,
		ai.ProviderIDGroq,
		ai.ProviderIDHuggingFace,
		ai.ProviderIDKimiCoding,
		ai.ProviderIDMiniMax,
		ai.ProviderIDMiniMaxCN,
		ai.ProviderIDMistral,
		ai.ProviderIDMoonshotAI,
		ai.ProviderIDMoonshotAICN,
		ai.ProviderIDNVIDIA,
		ai.ProviderIDOpenAI,
		ai.ProviderIDOpenAICodex,
		ai.ProviderIDOpenCode,
		ai.ProviderIDOpenCodeGo,
		ai.ProviderIDOpenRouter,
		ai.ProviderIDQwenTokenPlan,
		ai.ProviderIDQwenTokenPlanCN,
		ai.ProviderIDQwenTokenPlanIndividual,
		ai.ProviderIDRadius,
		ai.ProviderIDTogether,
		ai.ProviderIDVercelAIGateway,
		ai.ProviderIDXAI,
		ai.ProviderIDXiaomi,
		ai.ProviderIDXiaomiTokenPlanAMS,
		ai.ProviderIDXiaomiTokenPlanCN,
		ai.ProviderIDXiaomiTokenPlanSGP,
		ai.ProviderIDZAI,
		ai.ProviderIDZAICodingCN,
	}
	type providerMetadata struct {
		name    string
		baseURL string
	}
	wantMetadata := map[ai.ProviderID]providerMetadata{
		ai.ProviderIDAmazonBedrock:           {name: "Amazon Bedrock"},
		ai.ProviderIDAntLing:                 {name: "Ant Ling", baseURL: "https://api.ant-ling.com/v1"},
		ai.ProviderIDAnthropic:               {name: "Anthropic", baseURL: "https://api.anthropic.com"},
		ai.ProviderIDAzureOpenAIResponses:    {name: "Azure OpenAI"},
		ai.ProviderIDBaseten:                 {name: "Baseten", baseURL: "https://inference.baseten.co/v1"},
		ai.ProviderIDCerebras:                {name: "Cerebras", baseURL: "https://api.cerebras.ai/v1"},
		ai.ProviderIDCloudflareAIGateway:     {name: "Cloudflare AI Gateway"},
		ai.ProviderIDCloudflareWorkersAI:     {name: "Cloudflare Workers AI"},
		ai.ProviderIDDeepSeek:                {name: "DeepSeek", baseURL: "https://api.deepseek.com"},
		ai.ProviderIDFireworks:               {name: "Fireworks", baseURL: "https://api.fireworks.ai/inference"},
		ai.ProviderIDGitHubCopilot:           {name: "GitHub Copilot", baseURL: "https://api.individual.githubcopilot.com"},
		ai.ProviderIDGoogle:                  {name: "Google", baseURL: "https://generativelanguage.googleapis.com/v1beta"},
		ai.ProviderIDGoogleVertex:            {name: "Google Vertex AI"},
		ai.ProviderIDGroq:                    {name: "Groq", baseURL: "https://api.groq.com/openai/v1"},
		ai.ProviderIDHuggingFace:             {name: "Hugging Face", baseURL: "https://router.huggingface.co/v1"},
		ai.ProviderIDKimiCoding:              {name: "Kimi For Coding", baseURL: "https://api.kimi.com/coding"},
		ai.ProviderIDMiniMax:                 {name: "MiniMax", baseURL: "https://api.minimax.io/anthropic"},
		ai.ProviderIDMiniMaxCN:               {name: "MiniMax CN", baseURL: "https://api.minimaxi.com/anthropic"},
		ai.ProviderIDMistral:                 {name: "Mistral", baseURL: "https://api.mistral.ai"},
		ai.ProviderIDMoonshotAI:              {name: "Moonshot AI", baseURL: "https://api.moonshot.ai/v1"},
		ai.ProviderIDMoonshotAICN:            {name: "Moonshot AI CN", baseURL: "https://api.moonshot.cn/v1"},
		ai.ProviderIDNVIDIA:                  {name: "NVIDIA", baseURL: "https://integrate.api.nvidia.com/v1"},
		ai.ProviderIDOpenAI:                  {name: "OpenAI", baseURL: "https://api.openai.com/v1"},
		ai.ProviderIDOpenAICodex:             {name: "OpenAI Codex", baseURL: "https://chatgpt.com/backend-api"},
		ai.ProviderIDOpenCode:                {name: "OpenCode Zen"},
		ai.ProviderIDOpenCodeGo:              {name: "OpenCode Go"},
		ai.ProviderIDOpenRouter:              {name: "OpenRouter", baseURL: "https://openrouter.ai/api/v1"},
		ai.ProviderIDQwenTokenPlan:           {name: "Qwen Token Plan", baseURL: "https://token-plan.ap-southeast-1.maas.aliyuncs.com/compatible-mode/v1"},
		ai.ProviderIDQwenTokenPlanCN:         {name: "Qwen Token Plan CN", baseURL: "https://token-plan.cn-beijing.maas.aliyuncs.com/compatible-mode/v1"},
		ai.ProviderIDQwenTokenPlanIndividual: {name: "Qwen Token Plan Individual", baseURL: "https://token-plan.ap-southeast-1.maas.aliyuncs.com/compatible-mode/v1"},
		ai.ProviderIDRadius:                  {name: "Radius"},
		ai.ProviderIDTogether:                {name: "Together", baseURL: "https://api.together.ai/v1"},
		ai.ProviderIDVercelAIGateway:         {name: "Vercel AI Gateway", baseURL: "https://ai-gateway.vercel.sh"},
		ai.ProviderIDXAI:                     {name: "xAI", baseURL: "https://api.x.ai/v1"},
		ai.ProviderIDXiaomi:                  {name: "Xiaomi", baseURL: "https://api.xiaomimimo.com/v1"},
		ai.ProviderIDXiaomiTokenPlanAMS:      {name: "Xiaomi Token Plan AMS", baseURL: "https://token-plan-ams.xiaomimimo.com/v1"},
		ai.ProviderIDXiaomiTokenPlanCN:       {name: "Xiaomi Token Plan CN", baseURL: "https://token-plan-cn.xiaomimimo.com/v1"},
		ai.ProviderIDXiaomiTokenPlanSGP:      {name: "Xiaomi Token Plan SGP", baseURL: "https://token-plan-sgp.xiaomimimo.com/v1"},
		ai.ProviderIDZAI:                     {name: "Z.AI", baseURL: "https://api.z.ai/api/coding/paas/v4"},
		ai.ProviderIDZAICodingCN:             {name: "Z.AI Coding CN", baseURL: "https://open.bigmodel.cn/api/coding/paas/v4"},
	}

	first := ai.BuiltinProviders()
	second := ai.BuiltinProviders()
	if got := providerIDsForCatalogTest(first); !reflect.DeepEqual(got, wantProviderIDs) {
		t.Fatalf("BuiltinProviders() IDs = %v, want %v", got, wantProviderIDs)
	}
	if got := providerIDsForCatalogTest(second); !reflect.DeepEqual(got, wantProviderIDs) {
		t.Fatalf("second BuiltinProviders() IDs = %v, want %v", got, wantProviderIDs)
	}
	for index := range first {
		if reflect.ValueOf(first[index]).Pointer() == reflect.ValueOf(second[index]).Pointer() {
			t.Fatalf("BuiltinProviders()[%d] reused provider instance %q", index, first[index].ID())
		}
		want := wantMetadata[first[index].ID()]
		if first[index].Name() != want.name {
			t.Fatalf("provider %q name = %q, want %q", first[index].ID(), first[index].Name(), want.name)
		}
		baseURL, present := first[index].BaseURL().Value()
		if want.baseURL == "" {
			if present {
				t.Fatalf("provider %q base URL = %q, want absent", first[index].ID(), baseURL)
			}
		} else if !present || baseURL != want.baseURL {
			t.Fatalf("provider %q base URL = (%q, %t), want %q", first[index].ID(), baseURL, present, want.baseURL)
		}
		models := first[index].GetModels()
		if first[index].ID() == ai.ProviderIDDeepSeek {
			if len(models) != 2 {
				t.Fatalf("provider deepseek models = %#v, want two M1 models", models)
			}
		} else if len(models) != 0 {
			t.Fatalf("provider %q models = %#v, want deferred catalog", first[index].ID(), models)
		}
	}

	models := ai.BuiltinModels()
	if got := providerIDsForCatalogTest(models.GetProviders()); !reflect.DeepEqual(got, wantProviderIDs) {
		t.Fatalf("BuiltinModels().GetProviders() IDs = %v, want %v", got, wantProviderIDs)
	}
}

func TestBuiltinGitHubCopilotFiltersModelsFromOAuthCredentialMetadata(t *testing.T) {
	t.Parallel()

	var copilot ai.Provider
	for _, provider := range ai.BuiltinProviders() {
		if provider.ID() == ai.ProviderIDGitHubCopilot {
			copilot = provider
			break
		}
	}
	if copilot == nil {
		t.Fatal("BuiltinProviders() has no GitHub Copilot provider")
	}

	candidates := []ai.Model{
		{ID: "first", Provider: ai.ProviderIDGitHubCopilot},
		{ID: "second", Provider: ai.ProviderIDGitHubCopilot},
		{ID: "third", Provider: ai.ProviderIDGitHubCopilot},
	}
	oauth := ai.OAuthCredential{
		OAuthCredentials: ai.OAuthCredentials{
			Extra: map[string]json.RawMessage{"availableModelIds": json.RawMessage(`["third","first"]`)},
		},
		Type: ai.AuthTypeOAuth,
	}

	got := copilot.FilterModels(candidates, oauth)
	if gotIDs := modelIDsForCatalogTest(got); !reflect.DeepEqual(gotIDs, []string{"first", "third"}) {
		t.Fatalf("FilterModels() IDs = %v, want candidate order restricted to OAuth availability", gotIDs)
	}
	got[0].ID = "mutated"
	if candidates[0].ID != "first" {
		t.Fatalf("FilterModels() mutated caller candidates: %#v", candidates)
	}

	empty := oauth
	empty.Extra = map[string]json.RawMessage{"availableModelIds": json.RawMessage(`[]`)}
	if got := copilot.FilterModels(candidates, &empty); len(got) != 0 || got == nil {
		t.Fatalf("FilterModels(valid empty availability) = %#v, want present empty result", got)
	}
}

func TestBuiltinGitHubCopilotIgnoresNonAuthoritativeAvailabilityMetadata(t *testing.T) {
	t.Parallel()

	var copilot ai.Provider
	for _, provider := range ai.BuiltinProviders() {
		if provider.ID() == ai.ProviderIDGitHubCopilot {
			copilot = provider
			break
		}
	}
	if copilot == nil {
		t.Fatal("BuiltinProviders() has no GitHub Copilot provider")
	}
	candidates := []ai.Model{
		{ID: "first", Provider: ai.ProviderIDGitHubCopilot},
		{ID: "second", Provider: ai.ProviderIDGitHubCopilot},
	}

	tests := []struct {
		name       string
		credential ai.Credential
	}{
		{name: "no credential"},
		{name: "api key", credential: ai.APIKeyCredential{Type: ai.AuthTypeAPIKey, Key: ai.Some("token")}},
		{name: "oauth metadata absent", credential: copilotOAuthCredentialForCatalogTest(nil)},
		{name: "oauth metadata null", credential: copilotOAuthCredentialForCatalogTest(json.RawMessage(`null`))},
		{name: "oauth metadata object", credential: copilotOAuthCredentialForCatalogTest(json.RawMessage(`{"first":true}`))},
		{name: "oauth metadata mixed array", credential: copilotOAuthCredentialForCatalogTest(json.RawMessage(`["first",2]`))},
		{name: "oauth metadata null element", credential: copilotOAuthCredentialForCatalogTest(json.RawMessage(`["first",null]`))},
		{name: "oauth metadata invalid json", credential: copilotOAuthCredentialForCatalogTest(json.RawMessage(`["first"`))},
		{name: "forged oauth discriminator", credential: ai.OAuthCredential{
			OAuthCredentials: ai.OAuthCredentials{Extra: map[string]json.RawMessage{"availableModelIds": json.RawMessage(`[]`)}},
			Type:             ai.AuthTypeAPIKey,
		}},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got := copilot.FilterModels(candidates, test.credential)
			if gotIDs := modelIDsForCatalogTest(got); !reflect.DeepEqual(gotIDs, []string{"first", "second"}) {
				t.Fatalf("FilterModels() IDs = %v, want unchanged candidates", gotIDs)
			}
		})
	}
}

func TestBuiltinProviderAuthMetadataMatchesFixedPiBaseline(t *testing.T) {
	t.Parallel()

	type authMetadata struct {
		apiKeyName        string
		apiKeyCheck       bool
		oauthName         string
		oauthSubscription bool
		oauthLoginLabel   ai.Optional[string]
	}
	want := map[ai.ProviderID]authMetadata{
		ai.ProviderIDAmazonBedrock:           {apiKeyName: "AWS credentials or bearer token"},
		ai.ProviderIDAntLing:                 {apiKeyName: "Ant Ling API key"},
		ai.ProviderIDAnthropic:               {apiKeyName: "Anthropic API key", oauthName: "Anthropic (Claude Pro/Max)", oauthSubscription: true},
		ai.ProviderIDAzureOpenAIResponses:    {apiKeyName: "Azure OpenAI API key"},
		ai.ProviderIDBaseten:                 {apiKeyName: "Baseten API key"},
		ai.ProviderIDCerebras:                {apiKeyName: "Cerebras API key"},
		ai.ProviderIDCloudflareAIGateway:     {apiKeyName: "Cloudflare API key"},
		ai.ProviderIDCloudflareWorkersAI:     {apiKeyName: "Cloudflare API key"},
		ai.ProviderIDDeepSeek:                {apiKeyName: "DeepSeek API key"},
		ai.ProviderIDFireworks:               {apiKeyName: "Fireworks API key"},
		ai.ProviderIDGitHubCopilot:           {apiKeyName: "GitHub Copilot token", oauthName: "GitHub Copilot", oauthSubscription: true},
		ai.ProviderIDGoogle:                  {apiKeyName: "Gemini API key"},
		ai.ProviderIDGoogleVertex:            {apiKeyName: "Google Cloud credentials"},
		ai.ProviderIDGroq:                    {apiKeyName: "Groq API key"},
		ai.ProviderIDHuggingFace:             {apiKeyName: "Hugging Face token"},
		ai.ProviderIDKimiCoding:              {apiKeyName: "Kimi API key", oauthName: "Kimi Code (subscription)", oauthSubscription: true, oauthLoginLabel: ai.Some("Sign in with Kimi Code")},
		ai.ProviderIDMiniMax:                 {apiKeyName: "MiniMax API key"},
		ai.ProviderIDMiniMaxCN:               {apiKeyName: "MiniMax CN API key"},
		ai.ProviderIDMistral:                 {apiKeyName: "Mistral API key"},
		ai.ProviderIDMoonshotAI:              {apiKeyName: "Moonshot AI API key"},
		ai.ProviderIDMoonshotAICN:            {apiKeyName: "Moonshot AI API key"},
		ai.ProviderIDNVIDIA:                  {apiKeyName: "NVIDIA API key"},
		ai.ProviderIDOpenAI:                  {apiKeyName: "OpenAI API key"},
		ai.ProviderIDOpenAICodex:             {oauthName: "OpenAI (ChatGPT Plus/Pro)", oauthSubscription: true},
		ai.ProviderIDOpenCode:                {apiKeyName: "OpenCode API key"},
		ai.ProviderIDOpenCodeGo:              {apiKeyName: "OpenCode API key"},
		ai.ProviderIDOpenRouter:              {apiKeyName: "OpenRouter API key", oauthName: "OpenRouter OAuth", oauthLoginLabel: ai.Some("Sign in with OpenRouter")},
		ai.ProviderIDQwenTokenPlan:           {apiKeyName: "Qwen Token Plan API key"},
		ai.ProviderIDQwenTokenPlanCN:         {apiKeyName: "Qwen Token Plan CN API key"},
		ai.ProviderIDQwenTokenPlanIndividual: {apiKeyName: "Qwen Token Plan Individual API key"},
		ai.ProviderIDRadius:                  {apiKeyName: "Radius API key", oauthName: "Radius"},
		ai.ProviderIDTogether:                {apiKeyName: "Together API key"},
		ai.ProviderIDVercelAIGateway:         {apiKeyName: "Vercel AI Gateway API key"},
		ai.ProviderIDXAI:                     {apiKeyName: "xAI API key", oauthName: "xAI (Grok/X subscription)", oauthSubscription: true, oauthLoginLabel: ai.Some("Sign in with SuperGrok or X Premium")},
		ai.ProviderIDXiaomi:                  {apiKeyName: "Xiaomi API key"},
		ai.ProviderIDXiaomiTokenPlanAMS:      {apiKeyName: "Xiaomi Token Plan AMS API key"},
		ai.ProviderIDXiaomiTokenPlanCN:       {apiKeyName: "Xiaomi Token Plan CN API key"},
		ai.ProviderIDXiaomiTokenPlanSGP:      {apiKeyName: "Xiaomi Token Plan SGP API key"},
		ai.ProviderIDZAI:                     {apiKeyName: "Z.AI API key"},
		ai.ProviderIDZAICodingCN:             {apiKeyName: "Z.AI Coding CN API key"},
	}

	providers := ai.BuiltinProviders()
	if len(providers) != len(want) {
		t.Fatalf("len(BuiltinProviders()) = %d, want exact auth metadata rows for %d providers", len(providers), len(want))
	}
	for _, provider := range providers {
		provider := provider
		wantMetadata, ok := want[provider.ID()]
		if !ok {
			t.Errorf("BuiltinProviders() contains %q without an auth metadata row", provider.ID())
			continue
		}
		t.Run(string(provider.ID()), func(t *testing.T) {
			auth := provider.Auth()
			wantAPIKey := wantMetadata.apiKeyName != ""
			if got := auth.APIKey != nil; got != wantAPIKey {
				t.Fatalf("API-key auth present = %t, want %t", got, wantAPIKey)
			}
			if auth.APIKey != nil {
				if auth.APIKey.Name != wantMetadata.apiKeyName {
					t.Errorf("API-key name = %q, want %q", auth.APIKey.Name, wantMetadata.apiKeyName)
				}
				if auth.APIKey.Login == nil {
					t.Error("API-key Login is nil")
				} else if provider.ID() != ai.ProviderIDDeepSeek {
					if _, err := auth.APIKey.Login(context.Background(), nil); !errors.Is(err, ai.ErrNotImplemented) {
						t.Errorf("API-key Login error = %v, want ErrNotImplemented", err)
					}
				}
				if got := auth.APIKey.Check != nil; got != wantMetadata.apiKeyCheck {
					t.Errorf("API-key Check present = %t, want %t", got, wantMetadata.apiKeyCheck)
				}
				if auth.APIKey.Resolve == nil {
					t.Error("API-key Resolve is nil")
				} else if provider.ID() == ai.ProviderIDDeepSeek {
					resolved, err := auth.APIKey.Resolve(context.Background(), ai.APIKeyResolveInput{Context: &fakeAuthContext{env: map[string]string{"DEEPSEEK_API_KEY": "test-key"}}})
					result, ok := resolved.Value()
					key, hasKey := result.Auth.APIKey.Value()
					if err != nil || !ok || !hasKey || key != "test-key" {
						t.Errorf("DeepSeek Resolve() = (%#v, %v)", resolved, err)
					}
				} else if _, err := auth.APIKey.Resolve(context.Background(), ai.APIKeyResolveInput{}); !errors.Is(err, ai.ErrNotImplemented) {
					t.Errorf("API-key Resolve error = %v, want ErrNotImplemented", err)
				}
			}

			wantOAuth := wantMetadata.oauthName != ""
			if got := auth.OAuth != nil; got != wantOAuth {
				t.Fatalf("OAuth auth present = %t, want %t", got, wantOAuth)
			}
			if auth.OAuth != nil {
				if auth.OAuth.Name != wantMetadata.oauthName {
					t.Errorf("OAuth name = %q, want %q", auth.OAuth.Name, wantMetadata.oauthName)
				}
				if auth.OAuth.IsSubscription != wantMetadata.oauthSubscription {
					t.Errorf("OAuth IsSubscription = %t, want %t", auth.OAuth.IsSubscription, wantMetadata.oauthSubscription)
				}
				if !reflect.DeepEqual(auth.OAuth.LoginLabel, wantMetadata.oauthLoginLabel) {
					t.Errorf("OAuth LoginLabel = %#v, want %#v", auth.OAuth.LoginLabel, wantMetadata.oauthLoginLabel)
				}
				if auth.OAuth.Login == nil || auth.OAuth.Refresh == nil || auth.OAuth.ToAuth == nil {
					t.Errorf("OAuth operations = %#v, want side-effect-free Capability Stubs", auth.OAuth)
				} else {
					if _, err := auth.OAuth.Login(context.Background(), nil); !errors.Is(err, ai.ErrNotImplemented) {
						t.Errorf("OAuth Login error = %v, want ErrNotImplemented", err)
					}
					if _, err := auth.OAuth.Refresh(context.Background(), ai.OAuthCredential{}); !errors.Is(err, ai.ErrNotImplemented) {
						t.Errorf("OAuth Refresh error = %v, want ErrNotImplemented", err)
					}
					if _, err := auth.OAuth.ToAuth(context.Background(), ai.OAuthCredential{}); !errors.Is(err, ai.ErrNotImplemented) {
						t.Errorf("OAuth ToAuth error = %v, want ErrNotImplemented", err)
					}
				}
			}
		})
	}
}

func TestBuiltinProviderCapabilityMatrixMatchesFixedPiBaseline(t *testing.T) {
	t.Parallel()

	type capability struct {
		apis    []ai.API
		apiKey  bool
		oauth   bool
		refresh bool
	}
	want := map[ai.ProviderID]capability{
		ai.ProviderIDAmazonBedrock:           {apis: []ai.API{ai.APIBedrockConverseStream}, apiKey: true},
		ai.ProviderIDAntLing:                 {apis: []ai.API{ai.APIOpenAICompletions}, apiKey: true},
		ai.ProviderIDAnthropic:               {apis: []ai.API{ai.APIAnthropicMessages}, apiKey: true, oauth: true},
		ai.ProviderIDAzureOpenAIResponses:    {apis: []ai.API{ai.APIAzureOpenAIResponses}, apiKey: true},
		ai.ProviderIDBaseten:                 {apis: []ai.API{ai.APIOpenAICompletions}, apiKey: true},
		ai.ProviderIDCerebras:                {apis: []ai.API{ai.APIOpenAICompletions}, apiKey: true},
		ai.ProviderIDCloudflareAIGateway:     {apis: []ai.API{ai.APIAnthropicMessages, ai.APIOpenAICompletions, ai.APIOpenAIResponses}, apiKey: true},
		ai.ProviderIDCloudflareWorkersAI:     {apis: []ai.API{ai.APIOpenAICompletions}, apiKey: true},
		ai.ProviderIDDeepSeek:                {apis: []ai.API{ai.APIOpenAICompletions}, apiKey: true},
		ai.ProviderIDFireworks:               {apis: []ai.API{ai.APIAnthropicMessages, ai.APIOpenAICompletions}, apiKey: true},
		ai.ProviderIDGitHubCopilot:           {apis: []ai.API{ai.APIAnthropicMessages, ai.APIOpenAICompletions, ai.APIOpenAIResponses}, apiKey: true, oauth: true},
		ai.ProviderIDGoogle:                  {apis: []ai.API{ai.APIGoogleGenerativeAI}, apiKey: true},
		ai.ProviderIDGoogleVertex:            {apis: []ai.API{ai.APIGoogleVertex}, apiKey: true},
		ai.ProviderIDGroq:                    {apis: []ai.API{ai.APIOpenAICompletions}, apiKey: true},
		ai.ProviderIDHuggingFace:             {apis: []ai.API{ai.APIOpenAICompletions}, apiKey: true},
		ai.ProviderIDKimiCoding:              {apis: []ai.API{ai.APIAnthropicMessages}, apiKey: true, oauth: true},
		ai.ProviderIDMiniMax:                 {apis: []ai.API{ai.APIAnthropicMessages}, apiKey: true},
		ai.ProviderIDMiniMaxCN:               {apis: []ai.API{ai.APIAnthropicMessages}, apiKey: true},
		ai.ProviderIDMistral:                 {apis: []ai.API{ai.APIMistralConversations}, apiKey: true},
		ai.ProviderIDMoonshotAI:              {apis: []ai.API{ai.APIOpenAICompletions}, apiKey: true},
		ai.ProviderIDMoonshotAICN:            {apis: []ai.API{ai.APIOpenAICompletions}, apiKey: true},
		ai.ProviderIDNVIDIA:                  {apis: []ai.API{ai.APIOpenAICompletions}, apiKey: true},
		ai.ProviderIDOpenAI:                  {apis: []ai.API{ai.APIOpenAIResponses}, apiKey: true},
		ai.ProviderIDOpenAICodex:             {apis: []ai.API{ai.APIOpenAICodexResponses}, oauth: true},
		ai.ProviderIDOpenCode:                {apis: []ai.API{ai.APIAnthropicMessages, ai.APIGoogleGenerativeAI, ai.APIOpenAICompletions, ai.APIOpenAIResponses}, apiKey: true},
		ai.ProviderIDOpenCodeGo:              {apis: []ai.API{ai.APIAnthropicMessages, ai.APIOpenAICompletions, ai.APIOpenAIResponses}, apiKey: true},
		ai.ProviderIDOpenRouter:              {apis: []ai.API{ai.APIOpenAICompletions}, apiKey: true, oauth: true},
		ai.ProviderIDQwenTokenPlan:           {apis: []ai.API{ai.APIOpenAICompletions}, apiKey: true},
		ai.ProviderIDQwenTokenPlanCN:         {apis: []ai.API{ai.APIOpenAICompletions}, apiKey: true},
		ai.ProviderIDQwenTokenPlanIndividual: {apis: []ai.API{ai.APIOpenAICompletions}, apiKey: true},
		ai.ProviderIDRadius:                  {apis: []ai.API{ai.APIPiMessages}, apiKey: true, oauth: true, refresh: true},
		ai.ProviderIDTogether:                {apis: []ai.API{ai.APIOpenAICompletions}, apiKey: true},
		ai.ProviderIDVercelAIGateway:         {apis: []ai.API{ai.APIAnthropicMessages}, apiKey: true},
		ai.ProviderIDXAI:                     {apis: []ai.API{ai.APIOpenAICompletions, ai.APIOpenAIResponses}, apiKey: true, oauth: true},
		ai.ProviderIDXiaomi:                  {apis: []ai.API{ai.APIOpenAICompletions}, apiKey: true},
		ai.ProviderIDXiaomiTokenPlanAMS:      {apis: []ai.API{ai.APIOpenAICompletions}, apiKey: true},
		ai.ProviderIDXiaomiTokenPlanCN:       {apis: []ai.API{ai.APIOpenAICompletions}, apiKey: true},
		ai.ProviderIDXiaomiTokenPlanSGP:      {apis: []ai.API{ai.APIOpenAICompletions}, apiKey: true},
		ai.ProviderIDZAI:                     {apis: []ai.API{ai.APIOpenAICompletions}, apiKey: true},
		ai.ProviderIDZAICodingCN:             {apis: []ai.API{ai.APIOpenAICompletions}, apiKey: true},
	}
	allAPIs := []ai.API{
		ai.APIOpenAICompletions,
		ai.APIMistralConversations,
		ai.APIOpenAIResponses,
		ai.APIAzureOpenAIResponses,
		ai.APIOpenAICodexResponses,
		ai.APIAnthropicMessages,
		ai.APIBedrockConverseStream,
		ai.APIGoogleGenerativeAI,
		ai.APIGoogleVertex,
		ai.APIPiMessages,
		ai.API("extension-api-not-in-fixed-pi"),
	}

	providers := ai.BuiltinProviders()
	if len(providers) != len(want) {
		t.Fatalf("len(BuiltinProviders()) = %d, want capability rows for %d providers", len(providers), len(want))
	}
	for _, provider := range providers {
		provider := provider
		wantCapability, ok := want[provider.ID()]
		if !ok {
			t.Errorf("BuiltinProviders() contains %q without a fixed capability row", provider.ID())
			continue
		}
		t.Run(string(provider.ID()), func(t *testing.T) {
			auth := provider.Auth()
			if got := auth.APIKey != nil; got != wantCapability.apiKey {
				t.Errorf("API-key auth present = %t, want %t", got, wantCapability.apiKey)
			} else if got && (auth.APIKey.Login == nil || auth.APIKey.Resolve == nil) {
				t.Errorf("API-key auth = %#v, want required Capability Stubs", auth.APIKey)
			}
			if got := auth.OAuth != nil; got != wantCapability.oauth {
				t.Errorf("OAuth auth present = %t, want %t", got, wantCapability.oauth)
			} else if got && (auth.OAuth.Login == nil || auth.OAuth.Refresh == nil || auth.OAuth.ToAuth == nil) {
				t.Errorf("OAuth auth = %#v, want complete Capability Stub", auth.OAuth)
			}
			if got := provider.SupportsRefreshModels(); got != wantCapability.refresh {
				t.Errorf("SupportsRefreshModels() = %t, want %t", got, wantCapability.refresh)
			}
			if provider.SupportsFetchDeferred() || provider.SupportsCancelDeferred() {
				t.Errorf("deferred capabilities = (fetch=%t, cancel=%t), want both false", provider.SupportsFetchDeferred(), provider.SupportsCancelDeferred())
			}

			wantAPIs := make(map[ai.API]bool, len(wantCapability.apis))
			for _, apiID := range wantCapability.apis {
				wantAPIs[apiID] = true
			}
			for _, apiID := range allAPIs {
				model := ai.Model{ID: "capability-probe", Provider: provider.ID(), API: apiID}
				result, err := provider.Stream(context.Background(), model, ai.Context{}, ai.StreamOptions{}).Result(context.Background())
				if provider.ID() == ai.ProviderIDDeepSeek && apiID == ai.APIOpenAICompletions {
					if err != nil || result.StopReason != ai.StopReasonError {
						t.Errorf("DeepSeek live Stream() = (%#v, %v)", result, err)
					}
					continue
				}
				if wantAPIs[apiID] {
					if !errors.Is(err, ai.ErrNotImplemented) {
						t.Errorf("Stream(api=%q) error = %v, want declared API Capability Stub", apiID, err)
					}
					continue
				}
				errorMessage, present := result.ErrorMessage.Value()
				if err != nil || result.StopReason != ai.StopReasonError || !present || !strings.Contains(errorMessage, "no API implementation") {
					t.Errorf("Stream(api=%q) = (%#v, %v), want undeclared-API terminal outcome", apiID, result, err)
				}
			}
		})
	}
}

func TestBuiltinRadiusOfflineRefreshRestoresStoredModels(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := ai.NewInMemoryModelsStore()
	cached := ai.Model{
		ID:       "radius-cached",
		Name:     "Radius Cached",
		Provider: ai.ProviderIDRadius,
		API:      ai.APIPiMessages,
	}
	wantEntry := ai.ModelsStoreEntry{
		Models:    []ai.Model{cached},
		CheckedAt: ai.Some[int64](1_700_000_000_000),
		ETag:      ai.Some(`"radius-cache"`),
	}
	if err := store.Write(ctx, ai.ProviderIDRadius, wantEntry); err != nil {
		t.Fatalf("ModelsStore.Write() error = %v", err)
	}

	models := ai.BuiltinModels(ai.CreateModelsOptions{ModelsStore: store})
	result := models.Refresh(ctx, ai.ModelsRefreshOptions{
		AllowNetwork: ai.Some(false),
		Providers:    []ai.ProviderID{ai.ProviderIDRadius},
	})
	if result.Aborted || len(result.Errors) != 0 {
		t.Fatalf("Radius offline Refresh() = %+v, want non-aborted success", result)
	}
	if got := models.GetModels(ai.ProviderIDRadius); !reflect.DeepEqual(got, []ai.Model{cached}) {
		t.Fatalf("Radius models after offline refresh = %#v, want cached snapshot %#v", got, []ai.Model{cached})
	}
	if got, ok, err := store.Read(ctx, ai.ProviderIDRadius); err != nil || !ok || !reflect.DeepEqual(got, wantEntry) {
		t.Fatalf("ModelsStore after offline refresh = (%#v, %t, %v), want unchanged %#v", got, ok, err, wantEntry)
	}
}

func TestBuiltinRadiusNetworkRefreshIsSideEffectFreeCapabilityStub(t *testing.T) {
	t.Parallel()

	models := ai.BuiltinModels()
	radius, ok := models.GetProvider(ai.ProviderIDRadius)
	if !ok {
		t.Fatal("BuiltinModels() has no Radius provider")
	}
	publications := 0
	err := radius.RefreshModels(ai.RefreshModelsContext{
		Context:      context.Background(),
		AllowNetwork: true,
		Publish: func(ai.ModelsPublication) (bool, error) {
			publications++
			return true, nil
		},
	})
	if !errors.Is(err, ai.ErrNotImplemented) {
		t.Fatalf("Radius network RefreshModels() error = %v, want ErrNotImplemented", err)
	}
	var notImplemented *ai.NotImplementedError
	if !errors.As(err, &notImplemented) || notImplemented.Module != "ai" || notImplemented.Operation != "Radius.FetchModels" {
		t.Fatalf("Radius network RefreshModels() error = %#v, want ai/Radius.FetchModels NotImplementedError", err)
	}
	if publications != 0 {
		t.Fatalf("Radius network RefreshModels() publications = %d, want zero", publications)
	}
	if got := radius.GetModels(); len(got) != 0 {
		t.Fatalf("Radius models after failed network refresh = %#v, want honest empty snapshot", got)
	}
}

func providerIDsForCatalogTest(providers []ai.Provider) []ai.ProviderID {
	ids := make([]ai.ProviderID, len(providers))
	for index, provider := range providers {
		ids[index] = provider.ID()
	}
	return ids
}

func modelIDsForCatalogTest(models []ai.Model) []string {
	ids := make([]string, len(models))
	for index, model := range models {
		ids[index] = model.ID
	}
	return ids
}

func copilotOAuthCredentialForCatalogTest(availableModelIDs json.RawMessage) ai.OAuthCredential {
	var extra map[string]json.RawMessage
	if availableModelIDs != nil {
		extra = map[string]json.RawMessage{"availableModelIds": availableModelIDs}
	}
	return ai.OAuthCredential{OAuthCredentials: ai.OAuthCredentials{Extra: extra}, Type: ai.AuthTypeOAuth}
}

func TestFlattenModelCatalogReturnsDeeplyIsolatedModels(t *testing.T) {
	t.Parallel()

	input := ai.Model{
		ID:               "isolated",
		Input:            []ai.ModelInput{ai.ModelInputText},
		ThinkingLevelMap: ai.ThinkingLevelMap{ai.ModelThinkingLevelHigh: ai.Some("high")},
		Cost:             ai.ModelCost{Tiers: []ai.ModelCostTier{{InputTokensAbove: 100}}},
		SamplingParams:   map[string]json.RawMessage{"temperature": json.RawMessage(`0.2`)},
		Headers:          map[string]string{"x-source": "input"},
		Compat:           ai.Some(json.RawMessage(`{"mode":"input"}`)),
	}
	group := ai.ModelGroup{input.ID: input}
	catalog := ai.FlattenModelCatalog(ai.ProviderIDOpenAI, ai.ModelGroups{group})

	input.Input[0] = ai.ModelInputImage
	input.ThinkingLevelMap[ai.ModelThinkingLevelHigh] = ai.Some("mutated")
	input.Cost.Tiers[0].InputTokensAbove = 999
	input.SamplingParams["temperature"][0] = '9'
	input.Headers["x-source"] = "mutated"
	inputCompat, _ := input.Compat.Value()
	inputCompat[2] = 'X'
	delete(group, input.ID)

	got := catalog["isolated"]
	if got.Input[0] != ai.ModelInputText || got.ThinkingLevelMap[ai.ModelThinkingLevelHigh] != ai.Some("high") {
		t.Fatalf("catalog model shares input slice or thinking map: %#v", got)
	}
	if got.Cost.Tiers[0].InputTokensAbove != 100 || string(got.SamplingParams["temperature"]) != "0.2" {
		t.Fatalf("catalog model shares cost tiers or sampling data: %#v", got)
	}
	if got.Headers["x-source"] != "input" {
		t.Fatalf("catalog model shares headers: %#v", got.Headers)
	}
	gotCompat, ok := got.Compat.Value()
	if !ok || string(gotCompat) != `{"mode":"input"}` {
		t.Fatalf("catalog model shares compat data: %q, present=%t", gotCompat, ok)
	}
}

func TestFlattenModelCatalogPreservesPresentEmptyContainers(t *testing.T) {
	t.Parallel()

	catalog := ai.FlattenModelCatalog(ai.ProviderIDOpenAI, ai.ModelGroups{{
		"empty": {
			ID:    "empty",
			Input: []ai.ModelInput{},
			Cost:  ai.ModelCost{Tiers: []ai.ModelCostTier{}},
		},
	}})
	got := catalog["empty"]
	if got.Input == nil {
		t.Fatal("catalog model Input = nil, want present empty slice")
	}
	if got.Cost.Tiers == nil {
		t.Fatal("catalog model Cost.Tiers = nil, want present empty slice")
	}
}

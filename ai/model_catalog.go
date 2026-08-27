package ai

import (
	"bytes"
	"encoding/json"
)

// ModelGroup is one ordered source group in a generated model catalog. Model
// IDs are the merge key; the map's iteration order is not observable.
type ModelGroup map[string]Model

// ModelGroups preserves source group order so a later group can replace an
// earlier model with the same ID.
type ModelGroups []ModelGroup

// ModelCatalog is a model registry keyed by model ID.
type ModelCatalog map[string]Model

// BuiltinProvider is a provider with a static entry in the fixed generated
// catalog. ProviderID remains open to extension and purely dynamic providers.
type BuiltinProvider = ProviderID

var builtinProviderIDs = [39]BuiltinProvider{
	ProviderIDAmazonBedrock,
	ProviderIDAntLing,
	ProviderIDAnthropic,
	ProviderIDAzureOpenAIResponses,
	ProviderIDBaseten,
	ProviderIDCerebras,
	ProviderIDCloudflareAIGateway,
	ProviderIDCloudflareWorkersAI,
	ProviderIDDeepSeek,
	ProviderIDFireworks,
	ProviderIDGitHubCopilot,
	ProviderIDGoogle,
	ProviderIDGoogleVertex,
	ProviderIDGroq,
	ProviderIDHuggingFace,
	ProviderIDKimiCoding,
	ProviderIDMiniMax,
	ProviderIDMiniMaxCN,
	ProviderIDMistral,
	ProviderIDMoonshotAI,
	ProviderIDMoonshotAICN,
	ProviderIDNVIDIA,
	ProviderIDOpenAI,
	ProviderIDOpenAICodex,
	ProviderIDOpenCode,
	ProviderIDOpenCodeGo,
	ProviderIDOpenRouter,
	ProviderIDQwenTokenPlan,
	ProviderIDQwenTokenPlanCN,
	ProviderIDQwenTokenPlanIndividual,
	ProviderIDTogether,
	ProviderIDVercelAIGateway,
	ProviderIDXAI,
	ProviderIDXiaomi,
	ProviderIDXiaomiTokenPlanAMS,
	ProviderIDXiaomiTokenPlanCN,
	ProviderIDXiaomiTokenPlanSGP,
	ProviderIDZAI,
	ProviderIDZAICodingCN,
}

type builtinProviderMetadata struct {
	name    string
	baseURL string
	apis    []API
}

var builtinProviderMetadataByID = map[ProviderID]builtinProviderMetadata{
	ProviderIDAmazonBedrock:           {name: "Amazon Bedrock", apis: []API{APIBedrockConverseStream}},
	ProviderIDAntLing:                 {name: "Ant Ling", baseURL: "https://api.ant-ling.com/v1", apis: []API{APIOpenAICompletions}},
	ProviderIDAnthropic:               {name: "Anthropic", baseURL: "https://api.anthropic.com", apis: []API{APIAnthropicMessages}},
	ProviderIDAzureOpenAIResponses:    {name: "Azure OpenAI", apis: []API{APIAzureOpenAIResponses}},
	ProviderIDBaseten:                 {name: "Baseten", baseURL: "https://inference.baseten.co/v1", apis: []API{APIOpenAICompletions}},
	ProviderIDCerebras:                {name: "Cerebras", baseURL: "https://api.cerebras.ai/v1", apis: []API{APIOpenAICompletions}},
	ProviderIDCloudflareAIGateway:     {name: "Cloudflare AI Gateway", apis: []API{APIAnthropicMessages, APIOpenAICompletions, APIOpenAIResponses}},
	ProviderIDCloudflareWorkersAI:     {name: "Cloudflare Workers AI", apis: []API{APIOpenAICompletions}},
	ProviderIDDeepSeek:                {name: "DeepSeek", baseURL: "https://api.deepseek.com", apis: []API{APIOpenAICompletions}},
	ProviderIDFireworks:               {name: "Fireworks", baseURL: "https://api.fireworks.ai/inference", apis: []API{APIAnthropicMessages, APIOpenAICompletions}},
	ProviderIDGitHubCopilot:           {name: "GitHub Copilot", baseURL: "https://api.individual.githubcopilot.com", apis: []API{APIAnthropicMessages, APIOpenAICompletions, APIOpenAIResponses}},
	ProviderIDGoogle:                  {name: "Google", baseURL: "https://generativelanguage.googleapis.com/v1beta", apis: []API{APIGoogleGenerativeAI}},
	ProviderIDGoogleVertex:            {name: "Google Vertex AI", apis: []API{APIGoogleVertex}},
	ProviderIDGroq:                    {name: "Groq", baseURL: "https://api.groq.com/openai/v1", apis: []API{APIOpenAICompletions}},
	ProviderIDHuggingFace:             {name: "Hugging Face", baseURL: "https://router.huggingface.co/v1", apis: []API{APIOpenAICompletions}},
	ProviderIDKimiCoding:              {name: "Kimi For Coding", baseURL: "https://api.kimi.com/coding", apis: []API{APIAnthropicMessages}},
	ProviderIDMiniMax:                 {name: "MiniMax", baseURL: "https://api.minimax.io/anthropic", apis: []API{APIAnthropicMessages}},
	ProviderIDMiniMaxCN:               {name: "MiniMax CN", baseURL: "https://api.minimaxi.com/anthropic", apis: []API{APIAnthropicMessages}},
	ProviderIDMistral:                 {name: "Mistral", baseURL: "https://api.mistral.ai", apis: []API{APIMistralConversations}},
	ProviderIDMoonshotAI:              {name: "Moonshot AI", baseURL: "https://api.moonshot.ai/v1", apis: []API{APIOpenAICompletions}},
	ProviderIDMoonshotAICN:            {name: "Moonshot AI CN", baseURL: "https://api.moonshot.cn/v1", apis: []API{APIOpenAICompletions}},
	ProviderIDNVIDIA:                  {name: "NVIDIA", baseURL: "https://integrate.api.nvidia.com/v1", apis: []API{APIOpenAICompletions}},
	ProviderIDOpenAI:                  {name: "OpenAI", baseURL: "https://api.openai.com/v1", apis: []API{APIOpenAIResponses}},
	ProviderIDOpenAICodex:             {name: "OpenAI Codex", baseURL: "https://chatgpt.com/backend-api", apis: []API{APIOpenAICodexResponses}},
	ProviderIDOpenCode:                {name: "OpenCode Zen", apis: []API{APIAnthropicMessages, APIGoogleGenerativeAI, APIOpenAICompletions, APIOpenAIResponses}},
	ProviderIDOpenCodeGo:              {name: "OpenCode Go", apis: []API{APIAnthropicMessages, APIOpenAICompletions, APIOpenAIResponses}},
	ProviderIDOpenRouter:              {name: "OpenRouter", baseURL: "https://openrouter.ai/api/v1", apis: []API{APIOpenAICompletions}},
	ProviderIDQwenTokenPlan:           {name: "Qwen Token Plan", baseURL: "https://token-plan.ap-southeast-1.maas.aliyuncs.com/compatible-mode/v1", apis: []API{APIOpenAICompletions}},
	ProviderIDQwenTokenPlanCN:         {name: "Qwen Token Plan CN", baseURL: "https://token-plan.cn-beijing.maas.aliyuncs.com/compatible-mode/v1", apis: []API{APIOpenAICompletions}},
	ProviderIDQwenTokenPlanIndividual: {name: "Qwen Token Plan Individual", baseURL: "https://token-plan.ap-southeast-1.maas.aliyuncs.com/compatible-mode/v1", apis: []API{APIOpenAICompletions}},
	ProviderIDRadius:                  {name: "Radius", apis: []API{APIPiMessages}},
	ProviderIDTogether:                {name: "Together", baseURL: "https://api.together.ai/v1", apis: []API{APIOpenAICompletions}},
	ProviderIDVercelAIGateway:         {name: "Vercel AI Gateway", baseURL: "https://ai-gateway.vercel.sh", apis: []API{APIAnthropicMessages}},
	ProviderIDXAI:                     {name: "xAI", baseURL: "https://api.x.ai/v1", apis: []API{APIOpenAICompletions, APIOpenAIResponses}},
	ProviderIDXiaomi:                  {name: "Xiaomi", baseURL: "https://api.xiaomimimo.com/v1", apis: []API{APIOpenAICompletions}},
	ProviderIDXiaomiTokenPlanAMS:      {name: "Xiaomi Token Plan AMS", baseURL: "https://token-plan-ams.xiaomimimo.com/v1", apis: []API{APIOpenAICompletions}},
	ProviderIDXiaomiTokenPlanCN:       {name: "Xiaomi Token Plan CN", baseURL: "https://token-plan-cn.xiaomimimo.com/v1", apis: []API{APIOpenAICompletions}},
	ProviderIDXiaomiTokenPlanSGP:      {name: "Xiaomi Token Plan SGP", baseURL: "https://token-plan-sgp.xiaomimimo.com/v1", apis: []API{APIOpenAICompletions}},
	ProviderIDZAI:                     {name: "Z.AI", baseURL: "https://api.z.ai/api/coding/paas/v4", apis: []API{APIOpenAICompletions}},
	ProviderIDZAICodingCN:             {name: "Z.AI Coding CN", baseURL: "https://open.bigmodel.cn/api/coding/paas/v4", apis: []API{APIOpenAICompletions}},
}

type builtinProviderAuthMetadata struct {
	apiKeyName        string
	apiKeyCheck       bool
	oauthName         string
	oauthSubscription bool
	oauthLoginLabel   Optional[string]
}

// builtinProviderAuthMetadataByID mirrors the observable auth metadata in the
// locked Pi provider definitions. Operation bodies remain M0 Capability Stubs.
var builtinProviderAuthMetadataByID = map[ProviderID]builtinProviderAuthMetadata{
	ProviderIDAmazonBedrock:           {apiKeyName: "AWS credentials or bearer token"},
	ProviderIDAntLing:                 {apiKeyName: "Ant Ling API key"},
	ProviderIDAnthropic:               {apiKeyName: "Anthropic API key", oauthName: "Anthropic (Claude Pro/Max)", oauthSubscription: true},
	ProviderIDAzureOpenAIResponses:    {apiKeyName: "Azure OpenAI API key"},
	ProviderIDBaseten:                 {apiKeyName: "Baseten API key"},
	ProviderIDCerebras:                {apiKeyName: "Cerebras API key"},
	ProviderIDCloudflareAIGateway:     {apiKeyName: "Cloudflare API key"},
	ProviderIDCloudflareWorkersAI:     {apiKeyName: "Cloudflare API key"},
	ProviderIDDeepSeek:                {apiKeyName: "DeepSeek API key"},
	ProviderIDFireworks:               {apiKeyName: "Fireworks API key"},
	ProviderIDGitHubCopilot:           {apiKeyName: "GitHub Copilot token", oauthName: "GitHub Copilot", oauthSubscription: true},
	ProviderIDGoogle:                  {apiKeyName: "Gemini API key"},
	ProviderIDGoogleVertex:            {apiKeyName: "Google Cloud credentials"},
	ProviderIDGroq:                    {apiKeyName: "Groq API key"},
	ProviderIDHuggingFace:             {apiKeyName: "Hugging Face token"},
	ProviderIDKimiCoding:              {apiKeyName: "Kimi API key", oauthName: "Kimi Code (subscription)", oauthSubscription: true, oauthLoginLabel: Some("Sign in with Kimi Code")},
	ProviderIDMiniMax:                 {apiKeyName: "MiniMax API key"},
	ProviderIDMiniMaxCN:               {apiKeyName: "MiniMax CN API key"},
	ProviderIDMistral:                 {apiKeyName: "Mistral API key"},
	ProviderIDMoonshotAI:              {apiKeyName: "Moonshot AI API key"},
	ProviderIDMoonshotAICN:            {apiKeyName: "Moonshot AI API key"},
	ProviderIDNVIDIA:                  {apiKeyName: "NVIDIA API key"},
	ProviderIDOpenAI:                  {apiKeyName: "OpenAI API key"},
	ProviderIDOpenAICodex:             {oauthName: "OpenAI (ChatGPT Plus/Pro)", oauthSubscription: true},
	ProviderIDOpenCode:                {apiKeyName: "OpenCode API key"},
	ProviderIDOpenCodeGo:              {apiKeyName: "OpenCode API key"},
	ProviderIDOpenRouter:              {apiKeyName: "OpenRouter API key", oauthName: "OpenRouter OAuth", oauthLoginLabel: Some("Sign in with OpenRouter")},
	ProviderIDQwenTokenPlan:           {apiKeyName: "Qwen Token Plan API key"},
	ProviderIDQwenTokenPlanCN:         {apiKeyName: "Qwen Token Plan CN API key"},
	ProviderIDQwenTokenPlanIndividual: {apiKeyName: "Qwen Token Plan Individual API key"},
	ProviderIDRadius:                  {apiKeyName: "Radius API key", oauthName: "Radius"},
	ProviderIDTogether:                {apiKeyName: "Together API key"},
	ProviderIDVercelAIGateway:         {apiKeyName: "Vercel AI Gateway API key"},
	ProviderIDXAI:                     {apiKeyName: "xAI API key", oauthName: "xAI (Grok/X subscription)", oauthSubscription: true, oauthLoginLabel: Some("Sign in with SuperGrok or X Premium")},
	ProviderIDXiaomi:                  {apiKeyName: "Xiaomi API key"},
	ProviderIDXiaomiTokenPlanAMS:      {apiKeyName: "Xiaomi Token Plan AMS API key"},
	ProviderIDXiaomiTokenPlanCN:       {apiKeyName: "Xiaomi Token Plan CN API key"},
	ProviderIDXiaomiTokenPlanSGP:      {apiKeyName: "Xiaomi Token Plan SGP API key"},
	ProviderIDZAI:                     {apiKeyName: "Z.AI API key"},
	ProviderIDZAICodingCN:             {apiKeyName: "Z.AI Coding CN API key"},
}

// M0 audits the frozen Catalog artifact without loading it into the runtime.
// Runtime model data is wired from M1 onward; it must never come from the source
// checkout or a live service during ordinary build and test.
var builtinModelsByProvider = map[BuiltinProvider][]Model{}

// FlattenModelCatalog merges ordered groups into one catalog. As in the fixed
// Pi baseline, provider is a build-time typing aid and does not rewrite or
// reject the provider identity already present in a model.
func FlattenModelCatalog(_ ProviderID, groups ModelGroups) ModelCatalog {
	catalog := make(ModelCatalog)
	for _, group := range groups {
		for id, model := range group {
			catalog[id] = cloneModel(model)
		}
	}
	return catalog
}

// GetBuiltinProviders returns the static generated-catalog provider IDs in
// their fixed source order. Purely dynamic providers such as radius are not
// catalog entries.
func GetBuiltinProviders() []BuiltinProvider {
	return append([]BuiltinProvider(nil), builtinProviderIDs[:]...)
}

// GetBuiltinModels returns a fresh snapshot of one provider's built-in models.
// It remains empty in the M0 runtime skeleton.
func GetBuiltinModels(provider BuiltinProvider) []Model {
	builtinModels := builtinModelsByProvider[provider]
	models := make([]Model, len(builtinModels))
	for index, model := range builtinModels {
		models[index] = cloneModel(model)
	}
	return models
}

// GetBuiltinModel looks up one model in the fixed built-in catalog.
func GetBuiltinModel(provider BuiltinProvider, modelID string) (Model, bool) {
	for _, model := range builtinModelsByProvider[provider] {
		if model.ID == modelID {
			return cloneModel(model), true
		}
	}
	return Model{}, false
}

// GetBuiltinModelDataGeneratedAt returns the catalog generation time as Unix
// milliseconds. M0 keeps it absent until runtime catalog loading starts.
func GetBuiltinModelDataGeneratedAt() (int64, bool) {
	return 0, false
}

// BuiltinProviders constructs every built-in runtime provider in fixed source
// order. Radius is included as a purely dynamic provider even though it has no
// static catalog entry. Each call returns independent provider instances whose
// auth and stream operations are side-effect-free Capability Stubs.
func BuiltinProviders() []Provider {
	providers := make([]Provider, 0, len(builtinProviderIDs)+1)
	for _, providerID := range builtinProviderIDs {
		if providerID == ProviderIDTogether {
			providers = append(providers, newBuiltinProvider(ProviderIDRadius))
		}
		providers = append(providers, newBuiltinProvider(providerID))
	}
	return providers
}

// BuiltinModels constructs a new model collection and registers all built-in
// providers. It accepts zero or one options value, matching CreateModels.
func BuiltinModels(options ...CreateModelsOptions) MutableModels {
	models := CreateModels(options...)
	for _, provider := range BuiltinProviders() {
		models.SetProvider(provider)
	}
	return models
}

func newBuiltinProvider(providerID ProviderID) Provider {
	metadata := builtinProviderMetadataByID[providerID]
	options := CreateProviderOptions{
		ID:     providerID,
		Name:   metadata.name,
		Auth:   newBuiltinProviderAuth(providerID),
		Models: GetBuiltinModels(providerID),
		API:    newBuiltinProviderAPIs(metadata.apis),
	}
	if metadata.baseURL != "" {
		options.BaseURL = Some(metadata.baseURL)
	}
	if providerID == ProviderIDRadius {
		options.FetchModels = func(RefreshModelsContext) ([]Model, error) {
			return nil, newNotImplemented("Radius.FetchModels")
		}
	}
	if providerID == ProviderIDGitHubCopilot {
		options.FilterModels = filterGitHubCopilotModels
	}
	return CreateProvider(options)
}

// filterGitHubCopilotModels applies the account-specific model picker catalog
// captured on an OAuth credential. API-key credentials and OAuth credentials
// without a well-formed availableModelIds string array retain the complete
// catalog. A present empty array is authoritative and filters every model.
func filterGitHubCopilotModels(models []Model, credential Credential) []Model {
	oauth, ok := credential.(OAuthCredential)
	if !ok {
		if pointer, pointerOK := credential.(*OAuthCredential); pointerOK && pointer != nil {
			oauth, ok = *pointer, true
		}
	}
	if !ok || oauth.Type != AuthTypeOAuth {
		return models
	}

	raw, ok := oauth.Extra["availableModelIds"]
	if !ok || !rawJSONArray(raw) {
		return models
	}
	var encodedIDs []json.RawMessage
	if err := json.Unmarshal(raw, &encodedIDs); err != nil {
		return models
	}
	available := make(map[string]struct{}, len(encodedIDs))
	for _, encodedID := range encodedIDs {
		if trimmed := bytes.TrimSpace(encodedID); len(trimmed) == 0 || trimmed[0] != '"' {
			return models
		}
		var id string
		if err := json.Unmarshal(encodedID, &id); err != nil {
			return models
		}
		available[id] = struct{}{}
	}

	filtered := make([]Model, 0, len(models))
	for _, model := range models {
		if _, ok := available[model.ID]; ok {
			filtered = append(filtered, model)
		}
	}
	return filtered
}

func rawJSONArray(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) > 0 && trimmed[0] == '['
}

func newBuiltinProviderAuth(providerID ProviderID) ProviderAuth {
	metadata := builtinProviderAuthMetadataByID[providerID]
	auth := ProviderAuth{}
	if metadata.apiKeyName != "" {
		apiKey := NewStubAPIKeyAuth(metadata.apiKeyName)
		if !metadata.apiKeyCheck {
			apiKey.Check = nil
		}
		auth.APIKey = &apiKey
	}
	if metadata.oauthName != "" {
		oauth := NewStubOAuthAuth(metadata.oauthName)
		oauth.IsSubscription = metadata.oauthSubscription
		oauth.LoginLabel = metadata.oauthLoginLabel
		auth.OAuth = &oauth
	}
	return auth
}

func newBuiltinProviderAPIs(apiIDs []API) ProviderAPIConfig {
	streams := make(map[API]ProviderStreams, len(apiIDs))
	for _, apiID := range apiIDs {
		streams[apiID] = NewStubProviderStreams()
	}
	return ProviderAPIs(streams)
}

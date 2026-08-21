package ai

import "context"

const (
	AnthropicAuthTokenEnv  = "ANTHROPIC_AUTH_TOKEN"
	AnthropicOAuthTokenEnv = "ANTHROPIC_OAUTH_TOKEN"
	AnthropicAPIKeyEnv     = "ANTHROPIC_API_KEY"
)

var providerAPIKeyEnv = map[ProviderID][]string{
	ProviderIDGitHubCopilot:           {"COPILOT_GITHUB_TOKEN"},
	ProviderIDAnthropic:               {AnthropicAuthTokenEnv, AnthropicOAuthTokenEnv, AnthropicAPIKeyEnv},
	ProviderIDAntLing:                 {"ANT_LING_API_KEY"},
	ProviderIDQwenTokenPlan:           {"QWEN_TOKEN_PLAN_API_KEY"},
	ProviderIDQwenTokenPlanCN:         {"QWEN_TOKEN_PLAN_CN_API_KEY"},
	ProviderIDQwenTokenPlanIndividual: {"QWEN_TOKEN_PLAN_API_KEY"},
	ProviderIDOpenAI:                  {"OPENAI_API_KEY"},
	ProviderIDAzureOpenAIResponses:    {"AZURE_OPENAI_API_KEY"},
	ProviderIDNVIDIA:                  {"NVIDIA_API_KEY"},
	ProviderIDDeepSeek:                {"DEEPSEEK_API_KEY"},
	ProviderIDGoogle:                  {"GEMINI_API_KEY"},
	ProviderIDGoogleVertex:            {"GOOGLE_CLOUD_API_KEY"},
	ProviderIDGroq:                    {"GROQ_API_KEY"},
	ProviderIDCerebras:                {"CEREBRAS_API_KEY"},
	ProviderIDXAI:                     {"XAI_API_KEY"},
	ProviderIDRadius:                  {"RADIUS_API_KEY"},
	ProviderIDOpenRouter:              {"OPENROUTER_API_KEY"},
	ProviderIDVercelAIGateway:         {"AI_GATEWAY_API_KEY"},
	ProviderIDZAI:                     {"ZAI_API_KEY"},
	ProviderIDZAICodingCN:             {"ZAI_CODING_CN_API_KEY"},
	ProviderIDMistral:                 {"MISTRAL_API_KEY"},
	ProviderIDMiniMax:                 {"MINIMAX_API_KEY"},
	ProviderIDMiniMaxCN:               {"MINIMAX_CN_API_KEY"},
	ProviderIDMoonshotAI:              {"MOONSHOT_API_KEY"},
	ProviderIDMoonshotAICN:            {"MOONSHOT_API_KEY"},
	ProviderIDHuggingFace:             {"HF_TOKEN"},
	ProviderIDFireworks:               {"FIREWORKS_API_KEY"},
	ProviderIDTogether:                {"TOGETHER_API_KEY"},
	ProviderIDBaseten:                 {"BASETEN_API_KEY"},
	ProviderIDOpenCode:                {"OPENCODE_API_KEY"},
	ProviderIDOpenCodeGo:              {"OPENCODE_API_KEY"},
	ProviderIDKimiCoding:              {"KIMI_API_KEY"},
	ProviderIDCloudflareWorkersAI:     {"CLOUDFLARE_API_KEY"},
	ProviderIDCloudflareAIGateway:     {"CLOUDFLARE_API_KEY"},
	ProviderIDXiaomi:                  {"XIAOMI_API_KEY"},
	ProviderIDXiaomiTokenPlanCN:       {"XIAOMI_TOKEN_PLAN_CN_API_KEY"},
	ProviderIDXiaomiTokenPlanAMS:      {"XIAOMI_TOKEN_PLAN_AMS_API_KEY"},
	ProviderIDXiaomiTokenPlanSGP:      {"XIAOMI_TOKEN_PLAN_SGP_API_KEY"},
}

func FindEnvKeys(provider ProviderID, environment ...ProviderEnv) ([]string, error) {
	if len(environment) == 0 {
		return nil, newNotImplemented("FindEnvKeys")
	}
	env := environment[0]
	var found []string
	for _, name := range providerAPIKeyEnv[provider] {
		if env[name] != "" {
			found = append(found, name)
		}
	}
	return found, nil
}

func GetEnvAPIKey(provider ProviderID, environment ...ProviderEnv) (string, bool, error) {
	if len(environment) == 0 {
		return "", false, newNotImplemented("GetEnvAPIKey")
	}
	env := environment[0]
	keys, err := FindEnvKeys(provider, env)
	if err != nil {
		return "", false, err
	}
	for _, name := range keys {
		if provider == ProviderIDAnthropic && name == AnthropicAuthTokenEnv {
			continue
		}
		return env[name], true, nil
	}
	if provider == ProviderIDGoogleVertex &&
		(env["GOOGLE_CLOUD_PROJECT"] != "" || env["GCLOUD_PROJECT"] != "") &&
		env["GOOGLE_CLOUD_LOCATION"] != "" {
		return "", false, newNotImplemented("GetEnvAPIKey.GoogleVertexADC")
	}
	if provider == ProviderIDAmazonBedrock && hasInjectedBedrockAuth(env) {
		return "<authenticated>", true, nil
	}
	return "", false, nil
}

func CloudflareWorkersAIAuth() APIKeyAuth {
	auth := NewStubAPIKeyAuth("Cloudflare API key")
	auth.Check = nil
	return auth
}

func CloudflareAIGatewayAuth() APIKeyAuth {
	auth := NewStubAPIKeyAuth("Cloudflare API key")
	auth.Check = nil
	return auth
}

func hasInjectedBedrockAuth(env ProviderEnv) bool {
	return env["AWS_PROFILE"] != "" ||
		(env["AWS_ACCESS_KEY_ID"] != "" && env["AWS_SECRET_ACCESS_KEY"] != "") ||
		env["AWS_BEARER_TOKEN_BEDROCK"] != "" ||
		env["AWS_CONTAINER_CREDENTIALS_RELATIVE_URI"] != "" ||
		env["AWS_CONTAINER_CREDENTIALS_FULL_URI"] != "" ||
		env["AWS_WEB_IDENTITY_TOKEN_FILE"] != ""
}

type OAuthPrompt struct {
	Message     string
	Placeholder *string
	AllowEmpty  *bool
}

type OAuthAuthInfo struct {
	URL          string
	Instructions *string
}

type OAuthDeviceCodeInfo struct {
	UserCode         string
	VerificationURI  string
	IntervalSeconds  *int
	ExpiresInSeconds *int
}

type OAuthSelectOption struct {
	ID    string
	Label string
}

type OAuthSelectPrompt struct {
	Message string
	Options []OAuthSelectOption
}

type OAuthLoginCallbacks struct {
	Signal            context.Context
	OnAuth            func(OAuthAuthInfo)
	OnDeviceCode      func(OAuthDeviceCodeInfo)
	OnPrompt          func(context.Context, OAuthPrompt) (string, error)
	OnProgress        func(string)
	OnManualCodeInput func(context.Context) (string, error)
	OnSelect          func(context.Context, OAuthSelectPrompt) (Optional[string], error)
}

func RegisterBunOAuthFlows() error {
	return newNotImplemented("RegisterBunOAuthFlows")
}

func SetBedrockProviderModule(ProviderStreams) error {
	return newNotImplemented("SetBedrockProviderModule")
}

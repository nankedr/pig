package ai

func AmazonBedrockProvider() Provider { return newBuiltinProvider(ProviderIDAmazonBedrock) }

func AntLingProvider() Provider { return newBuiltinProvider(ProviderIDAntLing) }

func AnthropicProvider() Provider { return newBuiltinProvider(ProviderIDAnthropic) }

func AzureOpenAIResponsesProvider() Provider {
	return newBuiltinProvider(ProviderIDAzureOpenAIResponses)
}

func BasetenProvider() Provider { return newBuiltinProvider(ProviderIDBaseten) }

func CerebrasProvider() Provider { return newBuiltinProvider(ProviderIDCerebras) }

func CloudflareAIGatewayProvider() Provider {
	return newBuiltinProvider(ProviderIDCloudflareAIGateway)
}

func CloudflareWorkersAIProvider() Provider {
	return newBuiltinProvider(ProviderIDCloudflareWorkersAI)
}

func DeepSeekProvider() Provider { return newBuiltinProvider(ProviderIDDeepSeek) }

func FireworksProvider() Provider { return newBuiltinProvider(ProviderIDFireworks) }

func GitHubCopilotProvider() Provider { return newBuiltinProvider(ProviderIDGitHubCopilot) }

func GoogleProvider() Provider { return newBuiltinProvider(ProviderIDGoogle) }

func GoogleVertexProvider() Provider { return newBuiltinProvider(ProviderIDGoogleVertex) }

func GroqProvider() Provider { return newBuiltinProvider(ProviderIDGroq) }

func HuggingFaceProvider() Provider { return newBuiltinProvider(ProviderIDHuggingFace) }

func KimiCodingProvider() Provider { return newBuiltinProvider(ProviderIDKimiCoding) }

func MiniMaxProvider() Provider { return newBuiltinProvider(ProviderIDMiniMax) }

func MiniMaxCNProvider() Provider { return newBuiltinProvider(ProviderIDMiniMaxCN) }

func MistralProvider() Provider { return newBuiltinProvider(ProviderIDMistral) }

func MoonshotAIProvider() Provider { return newBuiltinProvider(ProviderIDMoonshotAI) }

func MoonshotAICNProvider() Provider { return newBuiltinProvider(ProviderIDMoonshotAICN) }

func NVIDIAProvider() Provider { return newBuiltinProvider(ProviderIDNVIDIA) }

func OpenAIProvider() Provider { return newBuiltinProvider(ProviderIDOpenAI) }

func OpenAICodexProvider() Provider { return newBuiltinProvider(ProviderIDOpenAICodex) }

func OpenCodeProvider() Provider { return newBuiltinProvider(ProviderIDOpenCode) }

func OpenCodeGoProvider() Provider { return newBuiltinProvider(ProviderIDOpenCodeGo) }

func OpenRouterProvider() Provider { return newBuiltinProvider(ProviderIDOpenRouter) }

func QwenTokenPlanProvider() Provider { return newBuiltinProvider(ProviderIDQwenTokenPlan) }

func QwenTokenPlanCNProvider() Provider { return newBuiltinProvider(ProviderIDQwenTokenPlanCN) }

func QwenTokenPlanIndividualProvider() Provider {
	return newBuiltinProvider(ProviderIDQwenTokenPlanIndividual)
}

type RadiusProviderOptions struct {
	ID      ProviderID
	Name    string
	Gateway string
}

func RadiusProvider(options ...RadiusProviderOptions) Provider {
	configured := RadiusProviderOptions{ID: ProviderIDRadius, Name: "Radius"}
	if len(options) != 0 {
		configured = options[0]
		if configured.ID == "" {
			configured.ID = ProviderIDRadius
		}
		if configured.Name == "" {
			configured.Name = "Radius"
		}
	}
	auth := newBuiltinProviderAuth(ProviderIDRadius)
	if auth.OAuth != nil {
		auth.OAuth.Name = configured.Name
	}
	return CreateProvider(CreateProviderOptions{
		ID:     configured.ID,
		Name:   configured.Name,
		Auth:   auth,
		API:    newBuiltinProviderAPIs(configured.ID, []API{APIPiMessages}),
		Models: nil,
		FetchModels: func(RefreshModelsContext) ([]Model, error) {
			return nil, newNotImplemented("Radius.FetchModels")
		},
	})
}

func TogetherProvider() Provider { return newBuiltinProvider(ProviderIDTogether) }

func VercelAIGatewayProvider() Provider { return newBuiltinProvider(ProviderIDVercelAIGateway) }

func XAIProvider() Provider { return newBuiltinProvider(ProviderIDXAI) }

func XiaomiProvider() Provider { return newBuiltinProvider(ProviderIDXiaomi) }

func XiaomiTokenPlanAMSProvider() Provider {
	return newBuiltinProvider(ProviderIDXiaomiTokenPlanAMS)
}

func XiaomiTokenPlanCNProvider() Provider {
	return newBuiltinProvider(ProviderIDXiaomiTokenPlanCN)
}

func XiaomiTokenPlanSGPProvider() Provider {
	return newBuiltinProvider(ProviderIDXiaomiTokenPlanSGP)
}

func ZAIProvider() Provider { return newBuiltinProvider(ProviderIDZAI) }

func ZAICodingCNProvider() Provider { return newBuiltinProvider(ProviderIDZAICodingCN) }

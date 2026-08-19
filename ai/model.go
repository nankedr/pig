package ai

import "encoding/json"

// API identifies a reusable model-service wire protocol. The known constants
// below mirror Pi's fixed baseline, while the string type intentionally remains
// open so callers can describe extension APIs.
type API string

// KnownAPI is the fixed-baseline subset of API. API itself remains open.
type KnownAPI = API

const (
	APIOpenAICompletions     API = "openai-completions"
	APIMistralConversations  API = "mistral-conversations"
	APIOpenAIResponses       API = "openai-responses"
	APIAzureOpenAIResponses  API = "azure-openai-responses"
	APIOpenAICodexResponses  API = "openai-codex-responses"
	APIAnthropicMessages     API = "anthropic-messages"
	APIBedrockConverseStream API = "bedrock-converse-stream"
	APIGoogleGenerativeAI    API = "google-generative-ai"
	APIGoogleVertex          API = "google-vertex"
	APIPiMessages            API = "pi-messages"
)

// ProviderID identifies a named model source. Like API, it is open to custom
// provider IDs in addition to the constants from the fixed Pi baseline.
type ProviderID string

// KnownProvider is the fixed-baseline subset represented by the constants
// below. ProviderID itself remains open to extension providers.
type KnownProvider = ProviderID

const (
	ProviderIDAmazonBedrock           ProviderID = "amazon-bedrock"
	ProviderIDAntLing                 ProviderID = "ant-ling"
	ProviderIDAnthropic               ProviderID = "anthropic"
	ProviderIDGoogle                  ProviderID = "google"
	ProviderIDGoogleVertex            ProviderID = "google-vertex"
	ProviderIDOpenAI                  ProviderID = "openai"
	ProviderIDAzureOpenAIResponses    ProviderID = "azure-openai-responses"
	ProviderIDOpenAICodex             ProviderID = "openai-codex"
	ProviderIDRadius                  ProviderID = "radius"
	ProviderIDNVIDIA                  ProviderID = "nvidia"
	ProviderIDDeepSeek                ProviderID = "deepseek"
	ProviderIDGitHubCopilot           ProviderID = "github-copilot"
	ProviderIDXAI                     ProviderID = "xai"
	ProviderIDGroq                    ProviderID = "groq"
	ProviderIDCerebras                ProviderID = "cerebras"
	ProviderIDOpenRouter              ProviderID = "openrouter"
	ProviderIDVercelAIGateway         ProviderID = "vercel-ai-gateway"
	ProviderIDZAI                     ProviderID = "zai"
	ProviderIDZAICodingCN             ProviderID = "zai-coding-cn"
	ProviderIDMistral                 ProviderID = "mistral"
	ProviderIDMiniMax                 ProviderID = "minimax"
	ProviderIDMiniMaxCN               ProviderID = "minimax-cn"
	ProviderIDMoonshotAI              ProviderID = "moonshotai"
	ProviderIDMoonshotAICN            ProviderID = "moonshotai-cn"
	ProviderIDHuggingFace             ProviderID = "huggingface"
	ProviderIDFireworks               ProviderID = "fireworks"
	ProviderIDTogether                ProviderID = "together"
	ProviderIDBaseten                 ProviderID = "baseten"
	ProviderIDOpenCode                ProviderID = "opencode"
	ProviderIDOpenCodeGo              ProviderID = "opencode-go"
	ProviderIDKimiCoding              ProviderID = "kimi-coding"
	ProviderIDCloudflareWorkersAI     ProviderID = "cloudflare-workers-ai"
	ProviderIDCloudflareAIGateway     ProviderID = "cloudflare-ai-gateway"
	ProviderIDQwenTokenPlan           ProviderID = "qwen-token-plan"
	ProviderIDQwenTokenPlanCN         ProviderID = "qwen-token-plan-cn"
	ProviderIDQwenTokenPlanIndividual ProviderID = "qwen-token-plan-individual"
	ProviderIDXiaomi                  ProviderID = "xiaomi"
	ProviderIDXiaomiTokenPlanCN       ProviderID = "xiaomi-token-plan-cn"
	ProviderIDXiaomiTokenPlanAMS      ProviderID = "xiaomi-token-plan-ams"
	ProviderIDXiaomiTokenPlanSGP      ProviderID = "xiaomi-token-plan-sgp"
)

// ThinkingLevel is a reasoning-effort level accepted by provider APIs.
type ThinkingLevel string

const (
	ThinkingLevelMinimal ThinkingLevel = "minimal"
	ThinkingLevelLow     ThinkingLevel = "low"
	ThinkingLevelMedium  ThinkingLevel = "medium"
	ThinkingLevelHigh    ThinkingLevel = "high"
	ThinkingLevelXHigh   ThinkingLevel = "xhigh"
	ThinkingLevelMax     ThinkingLevel = "max"
)

// ModelThinkingLevel adds the disabled state used by model metadata.
type ModelThinkingLevel string

const (
	ModelThinkingLevelOff     ModelThinkingLevel = "off"
	ModelThinkingLevelMinimal ModelThinkingLevel = "minimal"
	ModelThinkingLevelLow     ModelThinkingLevel = "low"
	ModelThinkingLevelMedium  ModelThinkingLevel = "medium"
	ModelThinkingLevelHigh    ModelThinkingLevel = "high"
	ModelThinkingLevelXHigh   ModelThinkingLevel = "xhigh"
	ModelThinkingLevelMax     ModelThinkingLevel = "max"
)

// StopReason classifies the current or terminal state of an assistant turn.
type StopReason string

const (
	StopReasonPending  StopReason = "pending"
	StopReasonStop     StopReason = "stop"
	StopReasonLength   StopReason = "length"
	StopReasonToolUse  StopReason = "toolUse"
	StopReasonError    StopReason = "error"
	StopReasonAborted  StopReason = "aborted"
	StopReasonDeferred StopReason = "deferred"
)

// Transport selects the preferred provider streaming transport.
type Transport string

const (
	TransportSSE             Transport = "sse"
	TransportWebSocket       Transport = "websocket"
	TransportWebSocketCached Transport = "websocket-cached"
	TransportAuto            Transport = "auto"
)

// CacheRetention selects the requested prompt-cache lifetime.
type CacheRetention string

const (
	CacheRetentionNone  CacheRetention = "none"
	CacheRetentionShort CacheRetention = "short"
	CacheRetentionLong  CacheRetention = "long"
)

// UsageCost contains the monetary cost attributed to one response.
type UsageCost struct {
	Input      float64 `json:"input"`
	Output     float64 `json:"output"`
	CacheRead  float64 `json:"cacheRead"`
	CacheWrite float64 `json:"cacheWrite"`
	Total      float64 `json:"total"`
}

// Usage contains provider token accounting. Optional breakdowns preserve a
// provider-reported zero separately from an omitted or explicit null value.
type Usage struct {
	Input        int64           `json:"input"`
	Output       int64           `json:"output"`
	CacheRead    int64           `json:"cacheRead"`
	CacheWrite   int64           `json:"cacheWrite"`
	CacheWrite1H Optional[int64] `json:"cacheWrite1h,omitzero"`
	Reasoning    Optional[int64] `json:"reasoning,omitzero"`
	TotalTokens  int64           `json:"totalTokens"`
	Cost         UsageCost       `json:"cost"`
}

// DeferredHandle identifies a provider request that is still running outside
// the current stream. Data remains an open JSON value because each API owns
// its reconstruction metadata.
type DeferredHandle struct {
	Provider    ProviderID          `json:"provider"`
	ModelID     string              `json:"modelId"`
	API         API                 `json:"api"`
	ID          string              `json:"id"`
	ExpiresAt   Optional[int64]     `json:"expiresAt,omitzero"`
	PollAfterMS Optional[int64]     `json:"pollAfterMs,omitzero"`
	Data        Optional[JSONValue] `json:"data,omitzero"`
}

// ModelInput identifies an input modality accepted by a model.
type ModelInput string

const (
	ModelInputText  ModelInput = "text"
	ModelInputImage ModelInput = "image"
)

// ThinkingLevelMap maps Pig thinking levels to provider-specific values. A
// missing key uses the provider default; an explicit null marks it unsupported.
type ThinkingLevelMap map[ModelThinkingLevel]Optional[string]

// ModelCostRates contains prices in dollars per million tokens.
type ModelCostRates struct {
	Input      float64 `json:"input"`
	Output     float64 `json:"output"`
	CacheRead  float64 `json:"cacheRead"`
	CacheWrite float64 `json:"cacheWrite"`
}

// ModelCostTier replaces the request-wide rates above an input-token threshold.
type ModelCostTier struct {
	ModelCostRates
	InputTokensAbove int64 `json:"inputTokensAbove"`
}

// ModelCost describes default and optional tiered pricing for a model.
type ModelCost struct {
	ModelCostRates
	Tiers []ModelCostTier `json:"tiers,omitempty"`
}

// Model describes one provider model. Compat and sampling parameters stay as
// raw JSON at this erased boundary so extension APIs retain unknown options.
// Built-in API authors can use the named compatibility structs declared by
// this package before marshaling them into Compat.
type Model struct {
	ID               string                     `json:"id"`
	Name             string                     `json:"name"`
	API              API                        `json:"api"`
	Provider         ProviderID                 `json:"provider"`
	BaseURL          string                     `json:"baseUrl"`
	Reasoning        bool                       `json:"reasoning"`
	ThinkingLevelMap ThinkingLevelMap           `json:"thinkingLevelMap,omitempty"`
	Input            []ModelInput               `json:"input"`
	Cost             ModelCost                  `json:"cost"`
	ContextWindow    int64                      `json:"contextWindow"`
	MaxTokens        int64                      `json:"maxTokens"`
	SamplingParams   map[string]json.RawMessage `json:"samplingParams,omitempty"`
	Headers          map[string]string          `json:"headers,omitempty"`
	Compat           Optional[json.RawMessage]  `json:"compat,omitzero"`
}

// Compatibility metadata is declared as data only in M0. Model.Compat stays
// raw so custom APIs are lossless; these named records serve built-in authors.
type MaxTokensField string
type ThinkingFormat string
type CacheControlFormat string
type DeferredToolsMode string
type SessionAffinityFormat string
type ChatTemplateKwargValue = json.RawMessage

const (
	MaxTokensFieldCompletion       MaxTokensField        = "max_completion_tokens"
	MaxTokensFieldLegacy           MaxTokensField        = "max_tokens"
	SessionAffinityOpenAI          SessionAffinityFormat = "openai"
	SessionAffinityOpenAINoSession SessionAffinityFormat = "openai-nosession"
	SessionAffinityOpenRouter      SessionAffinityFormat = "openrouter"
)

type OpenAICompletionsCompat struct {
	SupportsStore                               Optional[bool]                    `json:"supportsStore,omitzero"`
	SupportsDeveloperRole                       Optional[bool]                    `json:"supportsDeveloperRole,omitzero"`
	SupportsReasoningEffort                     Optional[bool]                    `json:"supportsReasoningEffort,omitzero"`
	SupportsUsageInStreaming                    Optional[bool]                    `json:"supportsUsageInStreaming,omitzero"`
	SupportsFinishReason                        Optional[bool]                    `json:"supportsFinishReason,omitzero"`
	MaxTokensField                              Optional[MaxTokensField]          `json:"maxTokensField,omitzero"`
	RequiresToolResultName                      Optional[bool]                    `json:"requiresToolResultName,omitzero"`
	RequiresAssistantAfterToolResult            Optional[bool]                    `json:"requiresAssistantAfterToolResult,omitzero"`
	RequiresThinkingAsText                      Optional[bool]                    `json:"requiresThinkingAsText,omitzero"`
	RequiresReasoningContentOnAssistantMessages Optional[bool]                    `json:"requiresReasoningContentOnAssistantMessages,omitzero"`
	ThinkingFormat                              Optional[ThinkingFormat]          `json:"thinkingFormat,omitzero"`
	ChatTemplateKwargs                          map[string]ChatTemplateKwargValue `json:"chatTemplateKwargs,omitempty"`
	ChatTemplateArgs                            map[string]ChatTemplateKwargValue `json:"chatTemplateArgs,omitempty"`
	OpenRouterRouting                           Optional[OpenRouterRouting]       `json:"openRouterRouting,omitzero"`
	VercelGatewayRouting                        Optional[VercelGatewayRouting]    `json:"vercelGatewayRouting,omitzero"`
	ZAIToolStream                               Optional[bool]                    `json:"zaiToolStream,omitzero"`
	SupportsThinkingTokenBudget                 Optional[bool]                    `json:"supportsThinkingTokenBudget,omitzero"`
	SupportsOpenAIGrammarTools                  Optional[bool]                    `json:"supportsOpenAIGrammarTools,omitzero"`
	SupportsStrictMode                          Optional[bool]                    `json:"supportsStrictMode,omitzero"`
	CacheControlFormat                          Optional[CacheControlFormat]      `json:"cacheControlFormat,omitzero"`
	SendSessionAffinityHeaders                  Optional[bool]                    `json:"sendSessionAffinityHeaders,omitzero"`
	DeferredToolsMode                           Optional[DeferredToolsMode]       `json:"deferredToolsMode,omitzero"`
	SessionAffinityFormat                       Optional[SessionAffinityFormat]   `json:"sessionAffinityFormat,omitzero"`
	SupportsLongCacheRetention                  Optional[bool]                    `json:"supportsLongCacheRetention,omitzero"`
}

type OpenAIResponsesCompat struct {
	SupportsDeveloperRole           Optional[bool]                  `json:"supportsDeveloperRole,omitzero"`
	SessionAffinityFormat           Optional[SessionAffinityFormat] `json:"sessionAffinityFormat,omitzero"`
	SupportsLongCacheRetention      Optional[bool]                  `json:"supportsLongCacheRetention,omitzero"`
	SupportsStrictMode              Optional[bool]                  `json:"supportsStrictMode,omitzero"`
	SupportsOpenAIGrammarTools      Optional[bool]                  `json:"supportsOpenAIGrammarTools,omitzero"`
	SupportsAdditionalTools         Optional[bool]                  `json:"supportsAdditionalTools,omitzero"`
	SupportsToolSearch              Optional[bool]                  `json:"supportsToolSearch,omitzero"`
	SupportsExplicitPromptCacheMode Optional[bool]                  `json:"supportsExplicitPromptCacheMode,omitzero"`
}

type AnthropicMessagesCompat struct {
	SupportsEagerToolInputStreaming Optional[bool] `json:"supportsEagerToolInputStreaming,omitzero"`
	SupportsLongCacheRetention      Optional[bool] `json:"supportsLongCacheRetention,omitzero"`
	SendSessionAffinityHeaders      Optional[bool] `json:"sendSessionAffinityHeaders,omitzero"`
	SupportsCacheControlOnTools     Optional[bool] `json:"supportsCacheControlOnTools,omitzero"`
	SupportsTemperature             Optional[bool] `json:"supportsTemperature,omitzero"`
	ForceAdaptiveThinking           Optional[bool] `json:"forceAdaptiveThinking,omitzero"`
	AllowEmptySignature             Optional[bool] `json:"allowEmptySignature,omitzero"`
	SupportsStrictTools             Optional[bool] `json:"supportsStrictTools,omitzero"`
	SupportsToolReferences          Optional[bool] `json:"supportsToolReferences,omitzero"`
}

type BedrockCompat struct {
	SupportsStrictMode Optional[bool] `json:"supportsStrictMode,omitzero"`
}

// Complex OpenRouter union-valued preferences stay raw at their individual
// fields so number/string/object alternatives are retained without SDK types.
type OpenRouterRouting struct {
	AllowFallbacks         Optional[bool]            `json:"allow_fallbacks,omitzero"`
	RequireParameters      Optional[bool]            `json:"require_parameters,omitzero"`
	DataCollection         Optional[string]          `json:"data_collection,omitzero"`
	ZDR                    Optional[bool]            `json:"zdr,omitzero"`
	EnforceDistillableText Optional[bool]            `json:"enforce_distillable_text,omitzero"`
	Order                  Optional[[]string]        `json:"order,omitzero"`
	Only                   Optional[[]string]        `json:"only,omitzero"`
	Ignore                 Optional[[]string]        `json:"ignore,omitzero"`
	Quantizations          Optional[[]string]        `json:"quantizations,omitzero"`
	Sort                   Optional[json.RawMessage] `json:"sort,omitzero"`
	MaxPrice               Optional[json.RawMessage] `json:"max_price,omitzero"`
	PreferredMinThroughput Optional[json.RawMessage] `json:"preferred_min_throughput,omitzero"`
	PreferredMaxLatency    Optional[json.RawMessage] `json:"preferred_max_latency,omitzero"`
}

type VercelGatewayRouting struct {
	Only  Optional[[]string] `json:"only,omitzero"`
	Order Optional[[]string] `json:"order,omitzero"`
}

type TextSignaturePhase string

const (
	TextSignaturePhaseCommentary  TextSignaturePhase = "commentary"
	TextSignaturePhaseFinalAnswer TextSignaturePhase = "final_answer"
)

type TextSignatureV1 struct {
	V     int                          `json:"v"`
	ID    string                       `json:"id"`
	Phase Optional[TextSignaturePhase] `json:"phase,omitzero"`
}

// Image-generation value contracts are data-only at M0.
type ImagesAPI string
type ImagesProviderID string
type ImagesStopReason string
type KnownImagesAPI = ImagesAPI
type KnownImagesProvider = ImagesProviderID

const (
	ImagesAPIOpenRouter        ImagesAPI        = "openrouter-images"
	ImagesProviderIDOpenRouter ImagesProviderID = "openrouter"
	ImagesStopReasonStop       ImagesStopReason = "stop"
	ImagesStopReasonError      ImagesStopReason = "error"
	ImagesStopReasonAborted    ImagesStopReason = "aborted"
)

type ImagesInputContent interface {
	Content
	imagesInputContent()
}
type ImagesOutputContent interface {
	Content
	imagesOutputContent()
}

func (TextContent) imagesInputContent()   {}
func (TextContent) imagesOutputContent()  {}
func (ImageContent) imagesInputContent()  {}
func (ImageContent) imagesOutputContent() {}

type ImagesContext struct {
	Input []ImagesInputContent `json:"input"`
}

type AssistantImages struct {
	API          ImagesAPI             `json:"api"`
	Provider     ImagesProviderID      `json:"provider"`
	Model        string                `json:"model"`
	Output       []ImagesOutputContent `json:"output"`
	ResponseID   Optional[string]      `json:"responseId,omitzero"`
	Usage        Optional[Usage]       `json:"usage,omitzero"`
	StopReason   ImagesStopReason      `json:"stopReason"`
	ErrorMessage Optional[string]      `json:"errorMessage,omitzero"`
	Timestamp    int64                 `json:"timestamp"`
}

type ImagesModel struct {
	ID               string                     `json:"id"`
	Name             string                     `json:"name"`
	API              ImagesAPI                  `json:"api"`
	Provider         ImagesProviderID           `json:"provider"`
	BaseURL          string                     `json:"baseUrl"`
	ThinkingLevelMap ThinkingLevelMap           `json:"thinkingLevelMap,omitempty"`
	Input            []ModelInput               `json:"input"`
	Output           []ModelInput               `json:"output"`
	Cost             ModelCost                  `json:"cost"`
	SamplingParams   map[string]json.RawMessage `json:"samplingParams,omitempty"`
	Headers          map[string]string          `json:"headers,omitempty"`
}

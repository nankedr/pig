package protocol

// ThinkingLevel is the requested reasoning effort for a session.
type ThinkingLevel string

const (
	ThinkingLevelOff     ThinkingLevel = "off"
	ThinkingLevelMinimal ThinkingLevel = "minimal"
	ThinkingLevelLow     ThinkingLevel = "low"
	ThinkingLevelMedium  ThinkingLevel = "medium"
	ThinkingLevelHigh    ThinkingLevel = "high"
	ThinkingLevelXHigh   ThinkingLevel = "xhigh"
	ThinkingLevelMax     ThinkingLevel = "max"
)

// ModelInputKind identifies an input modality accepted by a model.
type ModelInputKind string

const (
	ModelInputText  ModelInputKind = "text"
	ModelInputImage ModelInputKind = "image"
)

// ModelCost contains per-token model prices.
type ModelCost struct {
	Input      float64 `json:"input"`
	Output     float64 `json:"output"`
	CacheRead  float64 `json:"cacheRead"`
	CacheWrite float64 `json:"cacheWrite"`
}

// ModelMetadata describes a model advertised by the remote service.
type ModelMetadata struct {
	Provider                string           `json:"provider"`
	ID                      string           `json:"id"`
	Name                    string           `json:"name"`
	API                     string           `json:"api"`
	Reasoning               bool             `json:"reasoning"`
	Input                   []ModelInputKind `json:"input"`
	ContextWindow           int64            `json:"contextWindow"`
	MaxTokens               int64            `json:"maxTokens"`
	Cost                    ModelCost        `json:"cost"`
	SupportedThinkingLevels []ThinkingLevel  `json:"supportedThinkingLevels"`
	Authenticated           bool             `json:"authenticated"`
}

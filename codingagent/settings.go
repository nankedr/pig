package codingagent

import (
	"encoding/json"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/tui"
)

// CompactionSettings reuses the representation-neutral Agent compaction
// policy without coupling the v3 session model to the Harness v4 entry model.
type CompactionSettings = agent.CompactionSettings

// BranchSummarySettings contains the token budget and prompt policy used when
// summarizing a branch.
type BranchSummarySettings struct {
	ReserveTokens int64 `json:"reserveTokens,omitempty"`
	SkipPrompt    bool  `json:"skipPrompt,omitempty"`
}

type DefaultProjectTrust string

const (
	DefaultProjectTrustAsk    DefaultProjectTrust = "ask"
	DefaultProjectTrustAlways DefaultProjectTrust = "always"
	DefaultProjectTrustNever  DefaultProjectTrust = "never"
)

type FullscreenExitOutput string

const (
	FullscreenExitOutputTranscript FullscreenExitOutput = "transcript"
	FullscreenExitOutputResumeHint FullscreenExitOutput = "resume-hint"
)

type TUIMode = tui.TUIMode

type ImageSettings struct {
	AutoResize  *bool `json:"autoResize,omitempty"`
	BlockImages *bool `json:"blockImages,omitempty"`
}

type ProviderRetrySettings struct {
	TimeoutMS       *int64 `json:"timeoutMs,omitempty"`
	MaxRetries      *int   `json:"maxRetries,omitempty"`
	MaxRetryDelayMS *int64 `json:"maxRetryDelayMs,omitempty"`
}

type RetrySettings struct {
	Enabled     *bool                  `json:"enabled,omitempty"`
	MaxRetries  *int                   `json:"maxRetries,omitempty"`
	BaseDelayMS *int64                 `json:"baseDelayMs,omitempty"`
	Provider    *ProviderRetrySettings `json:"provider,omitempty"`
}

// PackageSource preserves the string and filtered-object branches of the
// pinned TypeScript union.
type PackageSource interface {
	ToString() string
	ValueOf() any
	packageSource()
}

// StringPackageSource is the string branch of PackageSource.
type StringPackageSource string

func (p StringPackageSource) ToString() string { return string(p) }
func (p StringPackageSource) ValueOf() any     { return string(p) }
func (StringPackageSource) packageSource()     {}

// FilteredPackageSource is the object branch of PackageSource.
type FilteredPackageSource struct {
	Source     string   `json:"source,omitempty"`
	Autoload   *bool    `json:"autoload,omitempty"`
	Extensions []string `json:"extensions,omitzero"`
	Skills     []string `json:"skills,omitzero"`
	Prompts    []string `json:"prompts,omitzero"`
	Themes     []string `json:"themes,omitzero"`
}

func (*FilteredPackageSource) ToString() string { return "[object Object]" }
func (p *FilteredPackageSource) ValueOf() any   { return p }
func (*FilteredPackageSource) packageSource()   {}

type Settings struct {
	LastChangelogVersion      *string                       `json:"lastChangelogVersion,omitempty"`
	DefaultProvider           *string                       `json:"defaultProvider,omitempty"`
	DefaultModel              *string                       `json:"defaultModel,omitempty"`
	DefaultThinkingLevel      *agent.ThinkingLevel          `json:"defaultThinkingLevel,omitempty"`
	Transport                 *ai.Transport                 `json:"transport,omitempty"`
	SteeringMode              *agent.QueueMode              `json:"steeringMode,omitempty"`
	FollowUpMode              *agent.QueueMode              `json:"followUpMode,omitempty"`
	Theme                     *string                       `json:"theme,omitempty"`
	Compaction                *CompactionSettings           `json:"compaction,omitempty"`
	Retry                     *RetrySettings                `json:"retry,omitempty"`
	HideThinkingBlock         *bool                         `json:"hideThinkingBlock,omitempty"`
	ShowCacheMissNotices      *bool                         `json:"showCacheMissNotices,omitempty"`
	ExternalEditor            *string                       `json:"externalEditor,omitempty"`
	ShellPath                 *string                       `json:"shellPath,omitempty"`
	QuietStartup              *bool                         `json:"quietStartup,omitempty"`
	DefaultProjectTrust       *DefaultProjectTrust          `json:"defaultProjectTrust,omitempty"`
	ShellCommandPrefix        *string                       `json:"shellCommandPrefix,omitempty"`
	NpmCommand                []string                      `json:"npmCommand,omitzero"`
	CollapseChangelog         *bool                         `json:"collapseChangelog,omitempty"`
	EnableInstallTelemetry    *bool                         `json:"enableInstallTelemetry,omitempty"`
	EnableAnalytics           *bool                         `json:"enableAnalytics,omitempty"`
	TrackingID                *string                       `json:"trackingId,omitempty"`
	Packages                  []PackageSource               `json:"packages,omitzero"`
	Extensions                []string                      `json:"extensions,omitzero"`
	Skills                    []string                      `json:"skills,omitzero"`
	Prompts                   []string                      `json:"prompts,omitzero"`
	Themes                    []string                      `json:"themes,omitzero"`
	EnableSkillCommands       *bool                         `json:"enableSkillCommands,omitempty"`
	Images                    *ImageSettings                `json:"images,omitempty"`
	EnabledModels             []string                      `json:"enabledModels,omitzero"`
	SessionDir                *string                       `json:"sessionDir,omitempty"`
	HTTPIdleTimeoutMS         *int64                        `json:"httpIdleTimeoutMs,omitempty"`
	WebSocketConnectTimeoutMS *int64                        `json:"websocketConnectTimeoutMs,omitempty"`
	TuiMode                   *TUIMode                      `json:"tuiMode,omitempty"`
	FullscreenExitOutput      *FullscreenExitOutput         `json:"fullscreenExitOutput,omitempty"`
	FullscreenScrollbar       *tui.ScrollViewScrollbar      `json:"fullscreenScrollbar,omitempty"`
	BranchSummary             *BranchSummarySettings        `json:"branchSummary,omitempty"`
	Terminal                  map[string]any                `json:"terminal,omitzero"`
	ThinkingBudgets           map[agent.ThinkingLevel]int64 `json:"thinkingBudgets,omitzero"`
	Markdown                  map[string]any                `json:"markdown,omitzero"`
	Warnings                  map[string]bool               `json:"warnings,omitzero"`
	DoubleEscapeAction        *string                       `json:"doubleEscapeAction,omitempty"`
	TreeFilterMode            *string                       `json:"treeFilterMode,omitempty"`
	EditorPaddingX            *int                          `json:"editorPaddingX,omitempty"`
	OutputPad                 *int                          `json:"outputPad,omitempty"`
	AutocompleteMaxVisible    *int                          `json:"autocompleteMaxVisible,omitempty"`
	ShowHardwareCursor        *bool                         `json:"showHardwareCursor,omitempty"`
	HTTPProxy                 *string                       `json:"httpProxy,omitempty"`
	Extra                     map[string]json.RawMessage    `json:"-"`
	original                  map[string]json.RawMessage
	decoded                   map[string]json.RawMessage
}

type SettingsManagerCreateOptions struct {
	ProjectTrusted *bool
}

type SettingsScope string

const (
	SettingsScopeGlobal  SettingsScope = "global"
	SettingsScopeProject SettingsScope = "project"
)

type SettingsStorage interface {
	WithLock(SettingsScope, func(*string) *string)
}

type SettingsError struct {
	Scope SettingsScope
	Error error
}

package codingagent

import (
	"context"

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
	ReserveTokens int64
	SkipPrompt    bool
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
	AutoResize  *bool
	BlockImages *bool
}

type ProviderRetrySettings struct {
	TimeoutMS       *int64
	MaxRetries      *int
	MaxRetryDelayMS *int64
}

type RetrySettings struct {
	Enabled     *bool
	MaxRetries  *int
	BaseDelayMS *int64
	Provider    *ProviderRetrySettings
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
	Source     string
	Autoload   *bool
	Extensions []string
	Skills     []string
	Prompts    []string
	Themes     []string
}

func (*FilteredPackageSource) ToString() string { return "[object Object]" }
func (p *FilteredPackageSource) ValueOf() any   { return p }
func (*FilteredPackageSource) packageSource()   {}

type Settings struct {
	LastChangelogVersion      *string
	DefaultProvider           *string
	DefaultModel              *string
	DefaultThinkingLevel      *agent.ThinkingLevel
	Transport                 *ai.Transport
	SteeringMode              *agent.QueueMode
	FollowUpMode              *agent.QueueMode
	Theme                     *string
	Compaction                *CompactionSettings
	Retry                     *RetrySettings
	HideThinkingBlock         *bool
	ShowCacheMissNotices      *bool
	ExternalEditor            *string
	ShellPath                 *string
	QuietStartup              *bool
	DefaultProjectTrust       *DefaultProjectTrust
	ShellCommandPrefix        *string
	NpmCommand                []string
	CollapseChangelog         *bool
	EnableInstallTelemetry    *bool
	EnableAnalytics           *bool
	TrackingID                *string
	Packages                  []PackageSource
	Extensions                []string
	Skills                    []string
	Prompts                   []string
	Themes                    []string
	EnableSkillCommands       *bool
	Images                    *ImageSettings
	EnabledModels             []string
	SessionDir                *string
	HTTPIdleTimeoutMS         *int64
	WebSocketConnectTimeoutMS *int64
	TuiMode                   *TUIMode
	FullscreenExitOutput      *FullscreenExitOutput
	FullscreenScrollbar       *tui.ScrollViewScrollbar
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

// SettingsManager is intentionally inert in this scaffold. Constructors and
// every operation that could observe or persist settings fail before I/O.
type SettingsManager struct{}

func NewSettingsManager(string, *string, ...SettingsManagerCreateOptions) (*SettingsManager, error) {
	return nil, notImplemented("NewSettingsManager")
}

func NewSettingsManagerFromStorage(SettingsStorage, ...SettingsManagerCreateOptions) (*SettingsManager, error) {
	return nil, notImplemented("NewSettingsManagerFromStorage")
}

func NewInMemorySettingsManager(Settings, ...SettingsManagerCreateOptions) (*SettingsManager, error) {
	return nil, notImplemented("NewInMemorySettingsManager")
}

func (SettingsManager) ApplyOverrides(Settings) error {
	return notImplemented("SettingsManager.ApplyOverrides")
}
func (SettingsManager) DrainErrors() ([]error, error) {
	return nil, notImplemented("SettingsManager.DrainErrors")
}
func (SettingsManager) Flush(context.Context) error  { return notImplemented("SettingsManager.Flush") }
func (SettingsManager) Reload(context.Context) error { return notImplemented("SettingsManager.Reload") }
func (SettingsManager) GetGlobalSettings() (Settings, error) {
	return Settings{}, notImplemented("SettingsManager.GetGlobalSettings")
}
func (SettingsManager) GetProjectSettings() (Settings, error) {
	return Settings{}, notImplemented("SettingsManager.GetProjectSettings")
}
func (SettingsManager) IsProjectTrusted() (bool, error) {
	return false, notImplemented("SettingsManager.IsProjectTrusted")
}
func (SettingsManager) SetProjectTrusted(bool) error {
	return notImplemented("SettingsManager.SetProjectTrusted")
}

func (SettingsManager) GetAutocompleteMaxVisible() (int, error) {
	return 0, notImplemented("SettingsManager.GetAutocompleteMaxVisible")
}
func (SettingsManager) GetBlockImages() (bool, error) {
	return false, notImplemented("SettingsManager.GetBlockImages")
}
func (SettingsManager) GetBranchSummarySettings() (BranchSummarySettings, error) {
	return BranchSummarySettings{}, notImplemented("SettingsManager.GetBranchSummarySettings")
}
func (SettingsManager) GetBranchSummarySkipPrompt() (bool, error) {
	return false, notImplemented("SettingsManager.GetBranchSummarySkipPrompt")
}
func (SettingsManager) GetClearOnShrink() (bool, error) {
	return false, notImplemented("SettingsManager.GetClearOnShrink")
}
func (SettingsManager) GetCodeBlockIndent() (string, error) {
	return "", notImplemented("SettingsManager.GetCodeBlockIndent")
}
func (SettingsManager) GetCollapseChangelog() (bool, error) {
	return false, notImplemented("SettingsManager.GetCollapseChangelog")
}
func (SettingsManager) GetCompactionEnabled() (bool, error) {
	return false, notImplemented("SettingsManager.GetCompactionEnabled")
}
func (SettingsManager) GetCompactionKeepRecentTokens() (int64, error) {
	return 0, notImplemented("SettingsManager.GetCompactionKeepRecentTokens")
}
func (SettingsManager) GetCompactionReserveTokens() (int64, error) {
	return 0, notImplemented("SettingsManager.GetCompactionReserveTokens")
}
func (SettingsManager) GetCompactionSettings() (CompactionSettings, error) {
	return CompactionSettings{}, notImplemented("SettingsManager.GetCompactionSettings")
}
func (SettingsManager) GetDefaultModel() (string, error) {
	return "", notImplemented("SettingsManager.GetDefaultModel")
}
func (SettingsManager) GetDefaultProjectTrust() (DefaultProjectTrust, error) {
	return "", notImplemented("SettingsManager.GetDefaultProjectTrust")
}
func (SettingsManager) GetDefaultProvider() (string, error) {
	return "", notImplemented("SettingsManager.GetDefaultProvider")
}
func (SettingsManager) GetDefaultThinkingLevel() (agent.ThinkingLevel, error) {
	return "", notImplemented("SettingsManager.GetDefaultThinkingLevel")
}
func (SettingsManager) GetDoubleEscapeAction() (string, error) {
	return "", notImplemented("SettingsManager.GetDoubleEscapeAction")
}
func (SettingsManager) GetEditorPaddingX() (int, error) {
	return 0, notImplemented("SettingsManager.GetEditorPaddingX")
}
func (SettingsManager) GetEnableAnalytics() (bool, error) {
	return false, notImplemented("SettingsManager.GetEnableAnalytics")
}
func (SettingsManager) GetEnableInstallTelemetry() (bool, error) {
	return false, notImplemented("SettingsManager.GetEnableInstallTelemetry")
}
func (SettingsManager) GetEnableSkillCommands() (bool, error) {
	return false, notImplemented("SettingsManager.GetEnableSkillCommands")
}
func (SettingsManager) GetEnabledModels() ([]string, error) {
	return nil, notImplemented("SettingsManager.GetEnabledModels")
}
func (SettingsManager) GetExtensionPaths() ([]string, error) {
	return nil, notImplemented("SettingsManager.GetExtensionPaths")
}
func (SettingsManager) GetExternalEditorCommand() (string, error) {
	return "", notImplemented("SettingsManager.GetExternalEditorCommand")
}
func (SettingsManager) GetFollowUpMode() (agent.QueueMode, error) {
	return "", notImplemented("SettingsManager.GetFollowUpMode")
}
func (SettingsManager) GetFullscreenExitOutput() (FullscreenExitOutput, error) {
	return "", notImplemented("SettingsManager.GetFullscreenExitOutput")
}
func (SettingsManager) GetFullscreenScrollbar() (tui.ScrollViewScrollbar, error) {
	return "", notImplemented("SettingsManager.GetFullscreenScrollbar")
}
func (SettingsManager) GetHideThinkingBlock() (bool, error) {
	return false, notImplemented("SettingsManager.GetHideThinkingBlock")
}
func (SettingsManager) GetHTTPIdleTimeoutMS() (int64, error) {
	return 0, notImplemented("SettingsManager.GetHTTPIdleTimeoutMS")
}
func (SettingsManager) GetImageAutoResize() (bool, error) {
	return false, notImplemented("SettingsManager.GetImageAutoResize")
}
func (SettingsManager) GetImageWidthCells() (int, error) {
	return 0, notImplemented("SettingsManager.GetImageWidthCells")
}
func (SettingsManager) GetLastChangelogVersion() (string, error) {
	return "", notImplemented("SettingsManager.GetLastChangelogVersion")
}
func (SettingsManager) GetMermaidRenderingMode() (string, error) {
	return "", notImplemented("SettingsManager.GetMermaidRenderingMode")
}
func (SettingsManager) GetNpmCommand() ([]string, error) {
	return nil, notImplemented("SettingsManager.GetNpmCommand")
}
func (SettingsManager) GetOutputPad() (int, error) {
	return 0, notImplemented("SettingsManager.GetOutputPad")
}
func (SettingsManager) GetPackages() ([]PackageSource, error) {
	return nil, notImplemented("SettingsManager.GetPackages")
}
func (SettingsManager) GetPromptTemplatePaths() ([]string, error) {
	return nil, notImplemented("SettingsManager.GetPromptTemplatePaths")
}
func (SettingsManager) GetProviderRetrySettings() (ProviderRetrySettings, error) {
	return ProviderRetrySettings{}, notImplemented("SettingsManager.GetProviderRetrySettings")
}
func (SettingsManager) GetQuietStartup() (bool, error) {
	return false, notImplemented("SettingsManager.GetQuietStartup")
}
func (SettingsManager) GetRetryEnabled() (bool, error) {
	return false, notImplemented("SettingsManager.GetRetryEnabled")
}
func (SettingsManager) GetRetrySettings() (RetrySettings, error) {
	return RetrySettings{}, notImplemented("SettingsManager.GetRetrySettings")
}
func (SettingsManager) GetSessionDir() (string, error) {
	return "", notImplemented("SettingsManager.GetSessionDir")
}
func (SettingsManager) GetShellCommandPrefix() (string, error) {
	return "", notImplemented("SettingsManager.GetShellCommandPrefix")
}
func (SettingsManager) GetShellPath() (string, error) {
	return "", notImplemented("SettingsManager.GetShellPath")
}
func (SettingsManager) GetShowCacheMissNotices() (bool, error) {
	return false, notImplemented("SettingsManager.GetShowCacheMissNotices")
}
func (SettingsManager) GetShowHardwareCursor() (bool, error) {
	return false, notImplemented("SettingsManager.GetShowHardwareCursor")
}
func (SettingsManager) GetShowImages() (bool, error) {
	return false, notImplemented("SettingsManager.GetShowImages")
}
func (SettingsManager) GetShowTerminalProgress() (bool, error) {
	return false, notImplemented("SettingsManager.GetShowTerminalProgress")
}
func (SettingsManager) GetSkillPaths() ([]string, error) {
	return nil, notImplemented("SettingsManager.GetSkillPaths")
}
func (SettingsManager) GetSteeringMode() (agent.QueueMode, error) {
	return "", notImplemented("SettingsManager.GetSteeringMode")
}
func (SettingsManager) GetTheme() (string, error) {
	return "", notImplemented("SettingsManager.GetTheme")
}
func (SettingsManager) GetThemePaths() ([]string, error) {
	return nil, notImplemented("SettingsManager.GetThemePaths")
}
func (SettingsManager) GetThemeSetting() (string, error) {
	return "", notImplemented("SettingsManager.GetThemeSetting")
}
func (SettingsManager) GetThinkingBudgets() (map[agent.ThinkingLevel]int64, error) {
	return nil, notImplemented("SettingsManager.GetThinkingBudgets")
}
func (SettingsManager) GetTrackingID() (string, error) {
	return "", notImplemented("SettingsManager.GetTrackingID")
}
func (SettingsManager) GetTransport() (ai.Transport, error) {
	return "", notImplemented("SettingsManager.GetTransport")
}
func (SettingsManager) GetTreeFilterMode() (string, error) {
	return "", notImplemented("SettingsManager.GetTreeFilterMode")
}
func (SettingsManager) GetTUIMode() (TUIMode, error) {
	return "", notImplemented("SettingsManager.GetTUIMode")
}
func (SettingsManager) GetWarnings() (map[string]bool, error) {
	return nil, notImplemented("SettingsManager.GetWarnings")
}
func (SettingsManager) GetWebSocketConnectTimeoutMS() (int64, error) {
	return 0, notImplemented("SettingsManager.GetWebSocketConnectTimeoutMS")
}

func (SettingsManager) SetAutocompleteMaxVisible(int) error {
	return notImplemented("SettingsManager.SetAutocompleteMaxVisible")
}
func (SettingsManager) SetBlockImages(bool) error {
	return notImplemented("SettingsManager.SetBlockImages")
}
func (SettingsManager) SetClearOnShrink(bool) error {
	return notImplemented("SettingsManager.SetClearOnShrink")
}
func (SettingsManager) SetCollapseChangelog(bool) error {
	return notImplemented("SettingsManager.SetCollapseChangelog")
}
func (SettingsManager) SetCompactionEnabled(bool) error {
	return notImplemented("SettingsManager.SetCompactionEnabled")
}
func (SettingsManager) SetDefaultModel(string) error {
	return notImplemented("SettingsManager.SetDefaultModel")
}
func (SettingsManager) SetDefaultModelAndProvider(string, string) error {
	return notImplemented("SettingsManager.SetDefaultModelAndProvider")
}
func (SettingsManager) SetDefaultProjectTrust(DefaultProjectTrust) error {
	return notImplemented("SettingsManager.SetDefaultProjectTrust")
}
func (SettingsManager) SetDefaultProvider(string) error {
	return notImplemented("SettingsManager.SetDefaultProvider")
}
func (SettingsManager) SetDefaultThinkingLevel(agent.ThinkingLevel) error {
	return notImplemented("SettingsManager.SetDefaultThinkingLevel")
}
func (SettingsManager) SetDoubleEscapeAction(string) error {
	return notImplemented("SettingsManager.SetDoubleEscapeAction")
}
func (SettingsManager) SetEditorPaddingX(int) error {
	return notImplemented("SettingsManager.SetEditorPaddingX")
}
func (SettingsManager) SetEnableAnalytics(bool) error {
	return notImplemented("SettingsManager.SetEnableAnalytics")
}
func (SettingsManager) SetEnableInstallTelemetry(bool) error {
	return notImplemented("SettingsManager.SetEnableInstallTelemetry")
}
func (SettingsManager) SetEnableSkillCommands(bool) error {
	return notImplemented("SettingsManager.SetEnableSkillCommands")
}
func (SettingsManager) SetEnabledModels([]string) error {
	return notImplemented("SettingsManager.SetEnabledModels")
}
func (SettingsManager) SetExtensionPaths([]string) error {
	return notImplemented("SettingsManager.SetExtensionPaths")
}
func (SettingsManager) SetFollowUpMode(agent.QueueMode) error {
	return notImplemented("SettingsManager.SetFollowUpMode")
}
func (SettingsManager) SetFullscreenExitOutput(FullscreenExitOutput) error {
	return notImplemented("SettingsManager.SetFullscreenExitOutput")
}
func (SettingsManager) SetFullscreenScrollbar(tui.ScrollViewScrollbar) error {
	return notImplemented("SettingsManager.SetFullscreenScrollbar")
}
func (SettingsManager) SetHideThinkingBlock(bool) error {
	return notImplemented("SettingsManager.SetHideThinkingBlock")
}
func (SettingsManager) SetHTTPIdleTimeoutMS(int64) error {
	return notImplemented("SettingsManager.SetHTTPIdleTimeoutMS")
}
func (SettingsManager) SetImageAutoResize(bool) error {
	return notImplemented("SettingsManager.SetImageAutoResize")
}
func (SettingsManager) SetImageWidthCells(int) error {
	return notImplemented("SettingsManager.SetImageWidthCells")
}
func (SettingsManager) SetLastChangelogVersion(string) error {
	return notImplemented("SettingsManager.SetLastChangelogVersion")
}
func (SettingsManager) SetMermaidRenderingMode(string) error {
	return notImplemented("SettingsManager.SetMermaidRenderingMode")
}
func (SettingsManager) SetNpmCommand([]string) error {
	return notImplemented("SettingsManager.SetNpmCommand")
}
func (SettingsManager) SetOutputPad(int) error { return notImplemented("SettingsManager.SetOutputPad") }
func (SettingsManager) SetPackages([]PackageSource) error {
	return notImplemented("SettingsManager.SetPackages")
}
func (SettingsManager) SetProjectExtensionPaths([]string) error {
	return notImplemented("SettingsManager.SetProjectExtensionPaths")
}
func (SettingsManager) SetProjectPackages([]PackageSource) error {
	return notImplemented("SettingsManager.SetProjectPackages")
}
func (SettingsManager) SetProjectPromptTemplatePaths([]string) error {
	return notImplemented("SettingsManager.SetProjectPromptTemplatePaths")
}
func (SettingsManager) SetProjectSkillPaths([]string) error {
	return notImplemented("SettingsManager.SetProjectSkillPaths")
}
func (SettingsManager) SetProjectThemePaths([]string) error {
	return notImplemented("SettingsManager.SetProjectThemePaths")
}
func (SettingsManager) SetPromptTemplatePaths([]string) error {
	return notImplemented("SettingsManager.SetPromptTemplatePaths")
}
func (SettingsManager) SetQuietStartup(bool) error {
	return notImplemented("SettingsManager.SetQuietStartup")
}
func (SettingsManager) SetRetryEnabled(bool) error {
	return notImplemented("SettingsManager.SetRetryEnabled")
}
func (SettingsManager) SetShellCommandPrefix(string) error {
	return notImplemented("SettingsManager.SetShellCommandPrefix")
}
func (SettingsManager) SetShellPath(string) error {
	return notImplemented("SettingsManager.SetShellPath")
}
func (SettingsManager) SetShowCacheMissNotices(bool) error {
	return notImplemented("SettingsManager.SetShowCacheMissNotices")
}
func (SettingsManager) SetShowHardwareCursor(bool) error {
	return notImplemented("SettingsManager.SetShowHardwareCursor")
}
func (SettingsManager) SetShowImages(bool) error {
	return notImplemented("SettingsManager.SetShowImages")
}
func (SettingsManager) SetShowTerminalProgress(bool) error {
	return notImplemented("SettingsManager.SetShowTerminalProgress")
}
func (SettingsManager) SetSkillPaths([]string) error {
	return notImplemented("SettingsManager.SetSkillPaths")
}
func (SettingsManager) SetSteeringMode(agent.QueueMode) error {
	return notImplemented("SettingsManager.SetSteeringMode")
}
func (SettingsManager) SetTheme(string) error { return notImplemented("SettingsManager.SetTheme") }
func (SettingsManager) SetThemePaths([]string) error {
	return notImplemented("SettingsManager.SetThemePaths")
}
func (SettingsManager) SetTransport(ai.Transport) error {
	return notImplemented("SettingsManager.SetTransport")
}
func (SettingsManager) SetTreeFilterMode(string) error {
	return notImplemented("SettingsManager.SetTreeFilterMode")
}
func (SettingsManager) SetTUIMode(TUIMode) error { return notImplemented("SettingsManager.SetTUIMode") }
func (SettingsManager) SetWarnings(map[string]bool) error {
	return notImplemented("SettingsManager.SetWarnings")
}

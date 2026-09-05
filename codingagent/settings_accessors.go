package codingagent

import (
	"encoding/json"
	"fmt"
	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/tui"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func (m SettingsManager) GetAutocompleteMaxVisible() (int, error) {
	return settingsValue[int](m, "autocompleteMaxVisible", "", 5)
}
func (m SettingsManager) GetBlockImages() (bool, error) {
	return settingsValue[bool](m, "images", "blockImages", false)
}
func (m SettingsManager) GetBranchSummarySkipPrompt() (bool, error) {
	return settingsValue[bool](m, "branchSummary", "skipPrompt", false)
}
func (m SettingsManager) GetClearOnShrink() (bool, error) {
	return settingsValue[bool](m, "terminal", "clearOnShrink", os.Getenv("PIG_CLEAR_ON_SHRINK") == "1")
}
func (m SettingsManager) GetCodeBlockIndent() (string, error) {
	return settingsValue[string](m, "markdown", "codeBlockIndent", "  ")
}
func (m SettingsManager) GetCollapseChangelog() (bool, error) {
	return settingsValue[bool](m, "collapseChangelog", "", false)
}
func (m SettingsManager) GetCompactionEnabled() (bool, error) {
	return settingsValue[bool](m, "compaction", "enabled", true)
}
func (m SettingsManager) GetCompactionKeepRecentTokens() (int64, error) {
	return settingsValue[int64](m, "compaction", "keepRecentTokens", 20000)
}
func (m SettingsManager) GetCompactionReserveTokens() (int64, error) {
	return settingsValue[int64](m, "compaction", "reserveTokens", 16384)
}
func (m SettingsManager) GetDefaultModel() (string, error) {
	return settingsValue[string](m, "defaultModel", "", "")
}
func (m SettingsManager) GetDefaultProvider() (string, error) {
	return settingsValue[string](m, "defaultProvider", "", "")
}
func (m SettingsManager) GetDefaultThinkingLevel() (agent.ThinkingLevel, error) {
	return settingsValue[agent.ThinkingLevel](m, "defaultThinkingLevel", "", "")
}
func (m SettingsManager) GetDoubleEscapeAction() (string, error) {
	return settingsValue[string](m, "doubleEscapeAction", "", "tree")
}
func (m SettingsManager) GetEditorPaddingX() (int, error) {
	return settingsValue[int](m, "editorPaddingX", "", 0)
}
func (m SettingsManager) GetEnableAnalytics() (bool, error) {
	return settingsValue[bool](m, "enableAnalytics", "", false)
}
func (m SettingsManager) GetEnableInstallTelemetry() (bool, error) {
	return settingsValue[bool](m, "enableInstallTelemetry", "", true)
}
func (m SettingsManager) GetEnableSkillCommands() (bool, error) {
	return settingsValue[bool](m, "enableSkillCommands", "", true)
}
func (m SettingsManager) GetEnabledModels() ([]string, error) {
	return settingsValue[[]string](m, "enabledModels", "", nil)
}
func (m SettingsManager) GetExtensionPaths() ([]string, error) {
	return settingsValue[[]string](m, "extensions", "", []string{})
}
func (m SettingsManager) GetFollowUpMode() (agent.QueueMode, error) {
	return settingsValue[agent.QueueMode](m, "followUpMode", "", agent.QueueMode("one-at-a-time"))
}
func (m SettingsManager) GetFullscreenExitOutput() (FullscreenExitOutput, error) {
	value, err := settingsValue[FullscreenExitOutput](m, "fullscreenExitOutput", "", FullscreenExitOutputTranscript)
	if value != "resume-hint" {
		value = "transcript"
	}
	return value, err
}
func (m SettingsManager) GetFullscreenScrollbar() (tui.ScrollViewScrollbar, error) {
	value, err := settingsValue[tui.ScrollViewScrollbar](m, "fullscreenScrollbar", "", tui.ScrollViewScrollbar("auto"))
	if value != "always" && value != "hidden" {
		value = "auto"
	}
	return value, err
}
func (m SettingsManager) GetHideThinkingBlock() (bool, error) {
	return settingsValue[bool](m, "hideThinkingBlock", "", false)
}
func (m SettingsManager) GetHTTPIdleTimeoutMS() (int64, error) {
	value, err := settingsValue[int64](m, "httpIdleTimeoutMs", "", 300000)
	if err == nil && value < 0 {
		err = fmt.Errorf("invalid httpIdleTimeoutMs setting: %d", value)
	}
	return value, err
}
func (m SettingsManager) GetImageAutoResize() (bool, error) {
	return settingsValue[bool](m, "images", "autoResize", true)
}
func (m SettingsManager) GetImageWidthCells() (int, error) {
	value, err := settingsValue[int](m, "terminal", "imageWidthCells", 60)
	return max(1, value), err
}
func (m SettingsManager) GetLastChangelogVersion() (string, error) {
	return settingsValue[string](m, "lastChangelogVersion", "", "")
}
func (m SettingsManager) GetMermaidRenderingMode() (string, error) {
	value, err := settingsValue[string](m, "markdown", "mermaid", "streaming")
	if value != "off" && value != "final" {
		value = "streaming"
	}
	return value, err
}
func (m SettingsManager) GetNpmCommand() ([]string, error) {
	return settingsValue[[]string](m, "npmCommand", "", nil)
}
func (m SettingsManager) GetOutputPad() (int, error) {
	value, err := settingsValue[int](m, "outputPad", "", 1)
	if value != 0 {
		value = 1
	}
	return value, err
}
func (m SettingsManager) GetPromptTemplatePaths() ([]string, error) {
	return settingsValue[[]string](m, "prompts", "", []string{})
}
func (m SettingsManager) GetQuietStartup() (bool, error) {
	return settingsValue[bool](m, "quietStartup", "", false)
}
func (m SettingsManager) GetRetryEnabled() (bool, error) {
	return settingsValue[bool](m, "retry", "enabled", true)
}
func (m SettingsManager) GetShellCommandPrefix() (string, error) {
	return settingsValue[string](m, "shellCommandPrefix", "", "")
}
func (m SettingsManager) GetShowCacheMissNotices() (bool, error) {
	return settingsValue[bool](m, "showCacheMissNotices", "", false)
}
func (m SettingsManager) GetShowHardwareCursor() (bool, error) {
	return settingsValue[bool](m, "showHardwareCursor", "", os.Getenv("PIG_HARDWARE_CURSOR") == "1")
}
func (m SettingsManager) GetShowImages() (bool, error) {
	return settingsValue[bool](m, "terminal", "showImages", true)
}
func (m SettingsManager) GetShowTerminalProgress() (bool, error) {
	return settingsValue[bool](m, "terminal", "showTerminalProgress", false)
}
func (m SettingsManager) GetSkillPaths() ([]string, error) {
	return settingsValue[[]string](m, "skills", "", []string{})
}
func (m SettingsManager) GetSteeringMode() (agent.QueueMode, error) {
	return settingsValue[agent.QueueMode](m, "steeringMode", "", agent.QueueMode("one-at-a-time"))
}
func (m SettingsManager) GetThemePaths() ([]string, error) {
	return settingsValue[[]string](m, "themes", "", []string{})
}
func (m SettingsManager) GetThemeSetting() (string, error) {
	value, err := settingsValue[string](m, "theme", "", "")
	if err != nil && m.ready() == nil {
		return "", nil
	}
	return value, err
}
func (m SettingsManager) GetThinkingBudgets() (map[agent.ThinkingLevel]int64, error) {
	return settingsValue[map[agent.ThinkingLevel]int64](m, "thinkingBudgets", "", nil)
}
func (m SettingsManager) GetTrackingID() (string, error) {
	return settingsValue[string](m, "trackingId", "", "")
}
func (m SettingsManager) GetTransport() (ai.Transport, error) {
	return settingsValue[ai.Transport](m, "transport", "", ai.Transport("auto"))
}
func (m SettingsManager) GetTreeFilterMode() (string, error) {
	value, err := settingsValue[string](m, "treeFilterMode", "", "default")
	switch value {
	case "default", "no-tools", "user-only", "labeled-only", "all":
	default:
		value = "default"
	}
	return value, err
}
func (m SettingsManager) GetTUIMode() (TUIMode, error) {
	value, err := settingsValue[TUIMode](m, "tuiMode", "", TUIMode("regular"))
	if value != "fullscreen" {
		value = "regular"
	}
	return value, err
}
func (m SettingsManager) GetWarnings() (map[string]bool, error) {
	return settingsValue[map[string]bool](m, "warnings", "", map[string]bool{})
}
func (m SettingsManager) GetWebSocketConnectTimeoutMS() (int64, error) {
	value, err := settingsValue[int64](m, "websocketConnectTimeoutMs", "", 0)
	if err == nil && value < 0 {
		err = fmt.Errorf("invalid websocketConnectTimeoutMs setting: %d", value)
	}
	return value, err
}
func (m SettingsManager) SetAutocompleteMaxVisible(value int) error {
	value = max(3, min(20, value))
	return m.setValue("autocompleteMaxVisible", value)
}
func (m SettingsManager) SetBlockImages(value bool) error {
	return m.setNested("images", "blockImages", value)
}
func (m SettingsManager) SetClearOnShrink(value bool) error {
	return m.setNested("terminal", "clearOnShrink", value)
}
func (m SettingsManager) SetCollapseChangelog(value bool) error {
	return m.setValue("collapseChangelog", value)
}
func (m SettingsManager) SetCompactionEnabled(value bool) error {
	return m.setNested("compaction", "enabled", value)
}
func (m SettingsManager) SetDefaultModel(value string) error {
	return m.setValue("defaultModel", value)
}
func (m SettingsManager) SetDefaultProjectTrust(value DefaultProjectTrust) error {
	return m.setValue("defaultProjectTrust", value)
}
func (m SettingsManager) SetDefaultProvider(value string) error {
	return m.setValue("defaultProvider", value)
}
func (m SettingsManager) SetDefaultThinkingLevel(value agent.ThinkingLevel) error {
	return m.setValue("defaultThinkingLevel", value)
}
func (m SettingsManager) SetDoubleEscapeAction(value string) error {
	return m.setValue("doubleEscapeAction", value)
}
func (m SettingsManager) SetEditorPaddingX(value int) error {
	value = max(0, min(3, value))
	return m.setValue("editorPaddingX", value)
}
func (m SettingsManager) SetEnableInstallTelemetry(value bool) error {
	return m.setValue("enableInstallTelemetry", value)
}
func (m SettingsManager) SetEnableSkillCommands(value bool) error {
	return m.setValue("enableSkillCommands", value)
}
func (m SettingsManager) SetEnabledModels(value []string) error {
	return m.setValue("enabledModels", value)
}
func (m SettingsManager) SetExtensionPaths(value []string) error {
	return m.setValue("extensions", value)
}
func (m SettingsManager) SetFollowUpMode(value agent.QueueMode) error {
	return m.setValue("followUpMode", value)
}
func (m SettingsManager) SetFullscreenExitOutput(value FullscreenExitOutput) error {
	return m.setValue("fullscreenExitOutput", value)
}
func (m SettingsManager) SetFullscreenScrollbar(value tui.ScrollViewScrollbar) error {
	return m.setValue("fullscreenScrollbar", value)
}
func (m SettingsManager) SetHideThinkingBlock(value bool) error {
	return m.setValue("hideThinkingBlock", value)
}
func (m SettingsManager) SetHTTPIdleTimeoutMS(value int64) error {
	if value < 0 {
		return fmt.Errorf("invalid httpIdleTimeoutMs setting: %d", value)
	}
	return m.setValue("httpIdleTimeoutMs", value)
}
func (m SettingsManager) SetImageAutoResize(value bool) error {
	return m.setNested("images", "autoResize", value)
}
func (m SettingsManager) SetImageWidthCells(value int) error {
	value = max(1, value)
	return m.setNested("terminal", "imageWidthCells", value)
}
func (m SettingsManager) SetLastChangelogVersion(value string) error {
	return m.setValue("lastChangelogVersion", value)
}
func (m SettingsManager) SetMermaidRenderingMode(value string) error {
	return m.setNested("markdown", "mermaid", value)
}
func (m SettingsManager) SetNpmCommand(value []string) error { return m.setValue("npmCommand", value) }
func (m SettingsManager) SetOutputPad(value int) error       { return m.setValue("outputPad", value) }
func (m SettingsManager) SetPackages(value []PackageSource) error {
	return m.setValue("packages", value)
}
func (m SettingsManager) SetProjectExtensionPaths(value []string) error {
	return notImplemented("SettingsManager.SetProjectExtensionPaths")
}
func (m SettingsManager) SetProjectPackages(value []PackageSource) error {
	return notImplemented("SettingsManager.SetProjectPackages")
}
func (m SettingsManager) SetProjectPromptTemplatePaths(value []string) error {
	return notImplemented("SettingsManager.SetProjectPromptTemplatePaths")
}
func (m SettingsManager) SetProjectSkillPaths(value []string) error {
	return notImplemented("SettingsManager.SetProjectSkillPaths")
}
func (m SettingsManager) SetProjectThemePaths(value []string) error {
	return notImplemented("SettingsManager.SetProjectThemePaths")
}
func (m SettingsManager) SetPromptTemplatePaths(value []string) error {
	return m.setValue("prompts", value)
}
func (m SettingsManager) SetQuietStartup(value bool) error { return m.setValue("quietStartup", value) }
func (m SettingsManager) SetRetryEnabled(value bool) error {
	return m.setNested("retry", "enabled", value)
}
func (m SettingsManager) SetShellCommandPrefix(value string) error {
	return m.setValue("shellCommandPrefix", value)
}
func (m SettingsManager) SetShellPath(value string) error { return m.setValue("shellPath", value) }
func (m SettingsManager) SetShowCacheMissNotices(value bool) error {
	return m.setValue("showCacheMissNotices", value)
}
func (m SettingsManager) SetShowHardwareCursor(value bool) error {
	return m.setValue("showHardwareCursor", value)
}
func (m SettingsManager) SetShowImages(value bool) error {
	return m.setNested("terminal", "showImages", value)
}
func (m SettingsManager) SetShowTerminalProgress(value bool) error {
	return m.setNested("terminal", "showTerminalProgress", value)
}
func (m SettingsManager) SetSkillPaths(value []string) error { return m.setValue("skills", value) }
func (m SettingsManager) SetSteeringMode(value agent.QueueMode) error {
	return m.setValue("steeringMode", value)
}
func (m SettingsManager) SetTheme(value string) error        { return m.setValue("theme", value) }
func (m SettingsManager) SetThemePaths(value []string) error { return m.setValue("themes", value) }
func (m SettingsManager) SetTransport(value ai.Transport) error {
	return m.setValue("transport", value)
}
func (m SettingsManager) SetTreeFilterMode(value string) error {
	return m.setValue("treeFilterMode", value)
}
func (m SettingsManager) SetTUIMode(value TUIMode) error { return m.setValue("tuiMode", value) }
func (m SettingsManager) SetWarnings(value map[string]bool) error {
	return m.setValue("warnings", value)
}

func (m SettingsManager) SetDefaultModelAndProvider(provider, model string) error {
	p, _ := json.Marshal(provider)
	v, _ := json.Marshal(model)
	return m.set(map[string]json.RawMessage{"defaultProvider": p, "defaultModel": v}, false)
}
func (m SettingsManager) SetEnableAnalytics(enabled bool) error {
	tracking, err := m.GetTrackingID()
	if err != nil {
		return err
	}
	fields := map[string]json.RawMessage{}
	fields["enableAnalytics"], _ = json.Marshal(enabled)
	if enabled && tracking == "" {
		fields["trackingId"], _ = json.Marshal(newSessionID())
	}
	return m.set(fields, false)
}
func (m SettingsManager) GetCompactionSettings() (CompactionSettings, error) {
	enabled, err := m.GetCompactionEnabled()
	if err != nil {
		return CompactionSettings{}, err
	}
	reserve, err := m.GetCompactionReserveTokens()
	if err != nil {
		return CompactionSettings{}, err
	}
	recent, err := m.GetCompactionKeepRecentTokens()
	return CompactionSettings{Enabled: enabled, ReserveTokens: reserve, KeepRecentTokens: recent}, err
}
func (m SettingsManager) GetBranchSummarySettings() (BranchSummarySettings, error) {
	reserve, err := settingsValue[int64](m, "branchSummary", "reserveTokens", 16384)
	if err != nil {
		return BranchSummarySettings{}, err
	}
	skip, err := m.GetBranchSummarySkipPrompt()
	return BranchSummarySettings{ReserveTokens: reserve, SkipPrompt: skip}, err
}
func (m SettingsManager) GetRetrySettings() (RetrySettings, error) {
	enabled, err := m.GetRetryEnabled()
	if err != nil {
		return RetrySettings{}, err
	}
	retries, err := settingsValue[int](m, "retry", "maxRetries", 3)
	if err != nil {
		return RetrySettings{}, err
	}
	delay, err := settingsValue[int64](m, "retry", "baseDelayMs", 2000)
	return RetrySettings{Enabled: &enabled, MaxRetries: &retries, BaseDelayMS: &delay}, err
}
func (m SettingsManager) GetProviderRetrySettings() (ProviderRetrySettings, error) {
	value, err := settingsValue[ProviderRetrySettings](m, "retry", "provider", ProviderRetrySettings{})
	if value.MaxRetryDelayMS == nil {
		delay := int64(60000)
		value.MaxRetryDelayMS = &delay
	}
	return value, err
}
func (m SettingsManager) GetPackages() ([]PackageSource, error) {
	raw, err := settingsValue[[]json.RawMessage](m, "packages", "", []json.RawMessage{})
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(map[string]any{"packages": raw})
	if err != nil {
		return nil, err
	}
	var settings Settings
	err = json.Unmarshal(data, &settings)
	return settings.Packages, err
}
func (m SettingsManager) GetDefaultProjectTrust() (DefaultProjectTrust, error) {
	s, err := m.GetGlobalSettings()
	if err != nil {
		return "", err
	}
	if s.DefaultProjectTrust != nil && (*s.DefaultProjectTrust == DefaultProjectTrustAlways || *s.DefaultProjectTrust == DefaultProjectTrustNever) {
		return *s.DefaultProjectTrust, nil
	}
	return DefaultProjectTrustAsk, nil
}
func (m SettingsManager) GetTheme() (string, error) {
	value, err := m.GetThemeSetting()
	if strings.Contains(value, "/") {
		value = ""
	}
	return value, err
}
func (m SettingsManager) GetExternalEditorCommand() (string, error) {
	value, err := settingsValue[string](m, "externalEditor", "", "")
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(value) != "" {
		return value, nil
	}
	for _, name := range []string{"VISUAL", "EDITOR"} {
		if value := os.Getenv(name); value != "" {
			return value, nil
		}
	}
	if runtime.GOOS == "windows" {
		return "notepad", nil
	}
	return "nano", nil
}
func (m SettingsManager) GetSessionDir() (string, error) {
	value, err := settingsValue[string](m, "sessionDir", "", "")
	if err != nil {
		return "", err
	}
	return normalizeSettingsPath(value)
}
func (m SettingsManager) GetShellPath() (string, error) {
	value, err := settingsValue[string](m, "shellPath", "", "")
	if err != nil {
		return "", err
	}
	return normalizeSettingsPath(value)
}
func normalizeSettingsPath(value string) (string, error) {
	if value == "~" || strings.HasPrefix(value, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if value == "~" {
			return home, nil
		}
		return filepath.Join(home, strings.TrimPrefix(value, "~/")), nil
	}
	return value, nil
}

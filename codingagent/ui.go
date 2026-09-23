package codingagent

// The declarations in this file are product-level UI composition points. They
// reuse tui contracts and deliberately do not render, read terminal input, or
// start timers while the interactive runtime is deferred.

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/tui"
)

type inertComponent struct{}

func (*inertComponent) Invalidate() error {
	return notImplemented("Component.Invalidate")
}
func (*inertComponent) Render(int) ([]string, error) { return nil, notImplemented("Component.Render") }

type ArminComponent struct{ inertComponent }

func (*ArminComponent) Dispose() error { return notImplemented("ArminComponent.Dispose") }

type BorderedLoader struct{ tui.Container }

func (*BorderedLoader) Dispose() error           { return notImplemented("BorderedLoader.Dispose") }
func (*BorderedLoader) HandleInput(string) error { return notImplemented("BorderedLoader.HandleInput") }
func (*BorderedLoader) OnAbort(func()) error     { return notImplemented("BorderedLoader.OnAbort") }
func (*BorderedLoader) Signal() (context.Context, error) {
	return nil, notImplemented("BorderedLoader.Signal")
}

type BranchSummaryMessageComponent struct{ tui.Box }

func (*BranchSummaryMessageComponent) Invalidate() error {
	return notImplemented("BranchSummaryMessageComponent.Invalidate")
}

func (*BranchSummaryMessageComponent) SetExpanded(bool) error {
	return notImplemented("BranchSummaryMessageComponent.SetExpanded")
}

type CompactionSummaryMessageComponent struct{ tui.Box }

func (*CompactionSummaryMessageComponent) Invalidate() error {
	return notImplemented("CompactionSummaryMessageComponent.Invalidate")
}

func (*CompactionSummaryMessageComponent) SetExpanded(bool) error {
	return notImplemented("CompactionSummaryMessageComponent.SetExpanded")
}

type SkillInvocationMessageComponent struct{ tui.Box }

func (*SkillInvocationMessageComponent) Invalidate() error {
	return notImplemented("SkillInvocationMessageComponent.Invalidate")
}

func (*SkillInvocationMessageComponent) SetExpanded(bool) error {
	return notImplemented("SkillInvocationMessageComponent.SetExpanded")
}

type CustomEditor struct {
	tui.Editor
	ActionHandlers map[AppKeybinding]ExtensionHandler
}

func (*CustomEditor) HandleInput(string) error { return notImplemented("CustomEditor.HandleInput") }
func (*CustomEditor) OnAction(AppKeybinding, ExtensionHandler) error {
	return notImplemented("CustomEditor.OnAction")
}
func (*CustomEditor) OnCtrlD(ExtensionHandler) error  { return notImplemented("CustomEditor.OnCtrlD") }
func (*CustomEditor) OnEscape(ExtensionHandler) error { return notImplemented("CustomEditor.OnEscape") }
func (*CustomEditor) OnExtensionShortcut(ExtensionHandler) error {
	return notImplemented("CustomEditor.OnExtensionShortcut")
}
func (*CustomEditor) OnPasteImage(ExtensionHandler) error {
	return notImplemented("CustomEditor.OnPasteImage")
}

type CustomMessageComponent struct{ tui.Container }

func (*CustomMessageComponent) Invalidate() error {
	return notImplemented("CustomMessageComponent.Invalidate")
}

func (*CustomMessageComponent) SetExpanded(bool) error {
	return notImplemented("CustomMessageComponent.SetExpanded")
}
func (*CustomMessageComponent) SetOutputPad(bool) error {
	return notImplemented("CustomMessageComponent.SetOutputPad")
}

type RenderDiffOptions struct{ FilePath string }

type DynamicBorder struct{ inertComponent }

type ExtensionEditorComponent struct {
	tui.Container
	Focused bool
}

func (*ExtensionEditorComponent) HandleInput(string) error {
	return notImplemented("ExtensionEditorComponent.HandleInput")
}

type ExtensionInputComponent struct {
	tui.Container
	Focused bool
}

func (*ExtensionInputComponent) Dispose() error {
	return notImplemented("ExtensionInputComponent.Dispose")
}
func (*ExtensionInputComponent) HandleInput(string) error {
	return notImplemented("ExtensionInputComponent.HandleInput")
}

type ExtensionSelectorComponent struct{ tui.Container }

func (*ExtensionSelectorComponent) Dispose() error {
	return notImplemented("ExtensionSelectorComponent.Dispose")
}
func (*ExtensionSelectorComponent) HandleInput(string) error {
	return notImplemented("ExtensionSelectorComponent.HandleInput")
}

type FooterComponent struct{ inertComponent }

func (*FooterComponent) Dispose() error { return notImplemented("FooterComponent.Dispose") }
func (*FooterComponent) SetAutoCompactEnabled(bool) error {
	return notImplemented("FooterComponent.SetAutoCompactEnabled")
}
func (*FooterComponent) SetSession(*AgentSession) error {
	return notImplemented("FooterComponent.SetSession")
}

func KeyHint(AppKeybinding, string) (string, error) { return "", notImplemented("KeyHint") }
func KeyText(AppKeybinding) (string, error)         { return "", notImplemented("KeyText") }
func RawKeyHint(string, string) (string, error)     { return "", notImplemented("RawKeyHint") }

type LoginDialogComponent struct {
	tui.Container
	Focused bool
}

func (*LoginDialogComponent) HandleInput(string) error {
	return notImplemented("LoginDialogComponent.HandleInput")
}
func (*LoginDialogComponent) ShowAuth(string, ...string) error {
	return notImplemented("LoginDialogComponent.ShowAuth")
}
func (*LoginDialogComponent) ShowDetails([]string) error {
	return notImplemented("LoginDialogComponent.ShowDetails")
}
func (*LoginDialogComponent) ShowDeviceCode(ai.OAuthDeviceCodeInfo) error {
	return notImplemented("LoginDialogComponent.ShowDeviceCode")
}
func (*LoginDialogComponent) ShowInfo(string, []ai.AuthInfoLink, ...bool) error {
	return notImplemented("LoginDialogComponent.ShowInfo")
}
func (*LoginDialogComponent) ShowManualInput(string) (string, error) {
	return "", notImplemented("LoginDialogComponent.ShowManualInput")
}
func (*LoginDialogComponent) ShowProgress(string) error {
	return notImplemented("LoginDialogComponent.ShowProgress")
}
func (*LoginDialogComponent) ShowPrompt(string, ...string) (string, error) {
	return "", notImplemented("LoginDialogComponent.ShowPrompt")
}
func (*LoginDialogComponent) ShowWaiting(string) error {
	return notImplemented("LoginDialogComponent.ShowWaiting")
}
func (*LoginDialogComponent) Signal() (context.Context, error) {
	return nil, notImplemented("LoginDialogComponent.Signal")
}

type OAuthSelectorComponent struct {
	tui.Container
	Focused bool
}

func (*OAuthSelectorComponent) HandleInput(string) error {
	return notImplemented("OAuthSelectorComponent.HandleInput")
}

type MermaidRenderingMode string

const (
	MermaidRenderingModeOff       MermaidRenderingMode = "off"
	MermaidRenderingModeFinal     MermaidRenderingMode = "final"
	MermaidRenderingModeStreaming MermaidRenderingMode = "streaming"
)

type DoubleEscapeAction string

const (
	DoubleEscapeActionFork DoubleEscapeAction = "fork"
	DoubleEscapeActionTree DoubleEscapeAction = "tree"
	DoubleEscapeActionNone DoubleEscapeAction = "none"
)

type TreeFilterMode string

const (
	TreeFilterModeDefault     TreeFilterMode = "default"
	TreeFilterModeNoTools     TreeFilterMode = "no-tools"
	TreeFilterModeUserOnly    TreeFilterMode = "user-only"
	TreeFilterModeLabeledOnly TreeFilterMode = "labeled-only"
	TreeFilterModeAll         TreeFilterMode = "all"
)

type WarningSettings struct {
	AnthropicExtraUsage *bool
}

type SettingsCallbacks struct {
	OnAutoCompactChange            func(bool)
	OnAutoResizeImagesChange       func(bool)
	OnAutocompleteMaxVisibleChange func(int)
	OnBlockImagesChange            func(bool)
	OnCancel                       func()
	OnClearOnShrinkChange          func(bool)
	OnCollapseChangelogChange      func(bool)
	OnDefaultProjectTrustChange    func(DefaultProjectTrust)
	OnDoubleEscapeActionChange     func(DoubleEscapeAction)
	OnEditorPaddingXChange         func(int)
	OnEnableInstallTelemetryChange func(bool)
	OnEnableSkillCommandsChange    func(bool)
	OnFollowUpModeChange           func(agent.QueueMode)
	OnFullscreenExitOutputChange   func(FullscreenExitOutput)
	OnFullscreenScrollbarChange    func(tui.ScrollViewScrollbar)
	OnHideThinkingBlockChange      func(bool)
	OnHTTPIdleTimeoutMSChange      func(int64)
	OnImageWidthCellsChange        func(int)
	OnMermaidRenderingModeChange   func(MermaidRenderingMode)
	OnOutputPadChange              func(int)
	OnQuietStartupChange           func(bool)
	OnShowCacheMissNoticesChange   func(bool)
	OnShowHardwareCursorChange     func(bool)
	OnShowImagesChange             func(bool)
	OnShowTerminalProgressChange   func(bool)
	OnSteeringModeChange           func(agent.QueueMode)
	OnThemeChange                  func(string)
	OnThemePreview                 func(string)
	OnThinkingLevelChange          func(agent.ThinkingLevel)
	OnTransportChange              func(ai.Transport)
	OnTreeFilterModeChange         func(TreeFilterMode)
	OnTUIModeChange                func(tui.TUIMode)
	OnWarningsChange               func(WarningSettings)
}

type SettingsConfig struct {
	AutoCompact, AutoResizeImages                                                            bool
	AutocompleteMaxVisible                                                                   int
	AvailableThemes                                                                          []string
	AvailableThinkingLevels                                                                  []agent.ThinkingLevel
	BlockImages, ClearOnShrink, CollapseChangelog                                            bool
	CurrentTheme                                                                             string
	DefaultProjectTrust                                                                      DefaultProjectTrust
	DoubleEscapeAction                                                                       DoubleEscapeAction
	EditorPaddingX                                                                           int
	EnableInstallTelemetry, EnableSkillCommands                                              bool
	FollowUpMode                                                                             agent.QueueMode
	FullscreenExitOutput                                                                     FullscreenExitOutput
	FullscreenScrollbar                                                                      tui.ScrollViewScrollbar
	HideThinkingBlock                                                                        bool
	HTTPIdleTimeoutMS                                                                        int64
	ImageWidthCells                                                                          int
	MermaidRenderingMode                                                                     MermaidRenderingMode
	OutputPad                                                                                int
	QuietStartup, ShowCacheMissNotices, ShowHardwareCursor, ShowImages, ShowTerminalProgress bool
	SteeringMode                                                                             agent.QueueMode
	TerminalTheme                                                                            tui.TerminalColorScheme
	ThinkingLevel                                                                            agent.ThinkingLevel
	Transport                                                                                ai.Transport
	TreeFilterMode                                                                           TreeFilterMode
	TUIMode                                                                                  tui.TUIMode
	Warnings                                                                                 WarningSettings
}

type SettingsSelectorComponent struct {
	tui.Container
	list        *tui.SettingsList
	keybindings *tui.KeybindingsManager
}

func (s *SettingsSelectorComponent) GetSettingsList() (*tui.SettingsList, error) {
	if s.list != nil {
		return s.list, nil
	}
	return nil, notImplemented("SettingsSelectorComponent.GetSettingsList")
}

type ShowImagesSelectorComponent struct{ tui.Container }

func (*ShowImagesSelectorComponent) GetSelectList() (*tui.SelectList, error) {
	return nil, notImplemented("ShowImagesSelectorComponent.GetSelectList")
}

type ToolExecutionOptions struct {
	ImageWidthCells int
	ShowImages      bool
}

type ToolExecutionResult struct {
	Content []ai.ToolResultContent
	Details ai.Optional[ai.JSONValue]
	IsError bool
}

type VisualTruncateResult struct {
	SkippedCount int
	VisualLines  []string
}

func TruncateToVisualLines(text string, maxVisualLines, width int, paddingX ...int) (VisualTruncateResult, error) {
	if maxVisualLines <= 0 {
		return VisualTruncateResult{}, fmt.Errorf("maxVisualLines must be positive")
	}
	if width <= 0 {
		return VisualTruncateResult{}, fmt.Errorf("width must be positive")
	}
	if len(paddingX) > 1 {
		return VisualTruncateResult{}, fmt.Errorf("paddingX accepts at most one value")
	}
	padding := 0
	if len(paddingX) != 0 {
		padding = paddingX[0]
	}
	if padding < 0 {
		return VisualTruncateResult{}, fmt.Errorf("paddingX must be non-negative")
	}
	if padding > (width-1)/2 {
		return VisualTruncateResult{}, fmt.Errorf("paddingX %d leaves no content width within width %d", padding, width)
	}
	if strings.TrimSpace(text) == "" {
		return VisualTruncateResult{}, nil
	}

	contentWidth := width - 2*padding
	lines, err := tui.WrapTextWithANSI(strings.ReplaceAll(text, "\t", "   "), contentWidth)
	if err != nil {
		return VisualTruncateResult{}, fmt.Errorf("wrap text for visual truncation: %w", err)
	}
	if len(lines) == 0 {
		return VisualTruncateResult{}, fmt.Errorf("wrap text for visual truncation returned no lines")
	}

	leftPadding := strings.Repeat(" ", padding)
	for index, line := range lines {
		lineWidth, err := tui.VisibleWidth(line)
		if err != nil {
			return VisualTruncateResult{}, fmt.Errorf("measure visual line %d: %w", index, err)
		}
		if lineWidth > contentWidth {
			return VisualTruncateResult{}, fmt.Errorf("wrapped visual line %d has width %d, exceeds content width %d", index, lineWidth, contentWidth)
		}
		lines[index] = leftPadding + line + strings.Repeat(" ", width-padding-lineWidth)
	}

	if len(lines) <= maxVisualLines {
		return VisualTruncateResult{VisualLines: lines}, nil
	}
	return VisualTruncateResult{
		VisualLines:  append([]string(nil), lines[len(lines)-maxVisualLines:]...),
		SkippedCount: len(lines) - maxVisualLines,
	}, nil
}

type ThemeColor string
type ThemeBG string
type ColorMode string

const (
	ThemeColorAccent             ThemeColor = "accent"
	ThemeColorBorder             ThemeColor = "border"
	ThemeColorBorderAccent       ThemeColor = "borderAccent"
	ThemeColorBorderMuted        ThemeColor = "borderMuted"
	ThemeColorSuccess            ThemeColor = "success"
	ThemeColorError              ThemeColor = "error"
	ThemeColorWarning            ThemeColor = "warning"
	ThemeColorMuted              ThemeColor = "muted"
	ThemeColorDim                ThemeColor = "dim"
	ThemeColorText               ThemeColor = "text"
	ThemeColorThinkingText       ThemeColor = "thinkingText"
	ThemeColorUserMessageText    ThemeColor = "userMessageText"
	ThemeColorCustomMessageText  ThemeColor = "customMessageText"
	ThemeColorCustomMessageLabel ThemeColor = "customMessageLabel"
	ThemeColorToolTitle          ThemeColor = "toolTitle"
	ThemeColorToolOutput         ThemeColor = "toolOutput"
	ThemeColorMDHeading          ThemeColor = "mdHeading"
	ThemeColorMDLink             ThemeColor = "mdLink"
	ThemeColorMDLinkURL          ThemeColor = "mdLinkUrl"
	ThemeColorMDCode             ThemeColor = "mdCode"
	ThemeColorMDCodeBlock        ThemeColor = "mdCodeBlock"
	ThemeColorMDCodeBlockBorder  ThemeColor = "mdCodeBlockBorder"
	ThemeColorMDQuote            ThemeColor = "mdQuote"
	ThemeColorMDQuoteBorder      ThemeColor = "mdQuoteBorder"
	ThemeColorMDHR               ThemeColor = "mdHr"
	ThemeColorMDListBullet       ThemeColor = "mdListBullet"
	ThemeColorToolDiffAdded      ThemeColor = "toolDiffAdded"
	ThemeColorToolDiffRemoved    ThemeColor = "toolDiffRemoved"
	ThemeColorToolDiffContext    ThemeColor = "toolDiffContext"
	ThemeColorSyntaxComment      ThemeColor = "syntaxComment"
	ThemeColorSyntaxKeyword      ThemeColor = "syntaxKeyword"
	ThemeColorSyntaxFunction     ThemeColor = "syntaxFunction"
	ThemeColorSyntaxVariable     ThemeColor = "syntaxVariable"
	ThemeColorSyntaxString       ThemeColor = "syntaxString"
	ThemeColorSyntaxNumber       ThemeColor = "syntaxNumber"
	ThemeColorSyntaxType         ThemeColor = "syntaxType"
	ThemeColorSyntaxOperator     ThemeColor = "syntaxOperator"
	ThemeColorSyntaxPunctuation  ThemeColor = "syntaxPunctuation"
	ThemeColorThinkingOff        ThemeColor = "thinkingOff"
	ThemeColorThinkingMinimal    ThemeColor = "thinkingMinimal"
	ThemeColorThinkingLow        ThemeColor = "thinkingLow"
	ThemeColorThinkingMedium     ThemeColor = "thinkingMedium"
	ThemeColorThinkingHigh       ThemeColor = "thinkingHigh"
	ThemeColorThinkingXHigh      ThemeColor = "thinkingXhigh"
	ThemeColorThinkingMax        ThemeColor = "thinkingMax"
	ThemeColorBashMode           ThemeColor = "bashMode"

	ThemeBGSelected       ThemeBG = "selectedBg"
	ThemeBGScrollbarThumb ThemeBG = "scrollbarThumb"
	ThemeBGUserMessage    ThemeBG = "userMessageBg"
	ThemeBGCustomMessage  ThemeBG = "customMessageBg"
	ThemeBGToolPending    ThemeBG = "toolPendingBg"
	ThemeBGToolSuccess    ThemeBG = "toolSuccessBg"
	ThemeBGToolError      ThemeBG = "toolErrorBg"

	ColorModeTrueColor ColorMode = "truecolor"
	ColorMode256       ColorMode = "256color"
)

type Theme struct {
	Name         string
	SourceInfo   *SourceInfo
	SourcePath   string
	mode         ColorMode
	foreground   map[ThemeColor]string
	background   map[ThemeBG]string
	colors       map[string]string
	exportColors map[string]string
}

func (t *Theme) FG(color ThemeColor, text string) string {
	if t == nil {
		return text
	}
	return t.foreground[color] + text + "\x1b[39m"
}
func (t *Theme) Fg(color ThemeColor, text string) string { return t.FG(color, text) }
func (t *Theme) BG(color ThemeBG, text string) string {
	if t == nil {
		return text
	}
	return t.background[color] + text + "\x1b[49m"
}
func (t *Theme) Bg(color ThemeBG, text string) string { return t.BG(color, text) }
func (*Theme) Bold(text string) string {
	return "\x1b[1m" + strings.ReplaceAll(text, "\x1b[22m", "\x1b[22m\x1b[1m") + "\x1b[22m"
}
func (*Theme) Italic(text string) string {
	return "\x1b[3m" + strings.ReplaceAll(text, "\x1b[23m", "\x1b[23m\x1b[3m") + "\x1b[23m"
}
func (*Theme) Underline(text string) string {
	return "\x1b[4m" + strings.ReplaceAll(text, "\x1b[24m", "\x1b[24m\x1b[4m") + "\x1b[24m"
}
func (*Theme) Inverse(text string) string { return "\x1b[7m" + text + "\x1b[27m" }
func (*Theme) Strikethrough(text string) string {
	return "\x1b[9m" + strings.ReplaceAll(text, "\x1b[29m", "\x1b[29m\x1b[9m") + "\x1b[29m"
}
func (t *Theme) GetFGANSI(color ThemeColor) string {
	if t == nil {
		return ""
	}
	return t.foreground[color]
}
func (t *Theme) GetBGANSI(color ThemeBG) string {
	if t == nil {
		return ""
	}
	return t.background[color]
}
func (t *Theme) GetFgANSI(color ThemeColor) string { return t.GetFGANSI(color) }
func (t *Theme) GetBgANSI(color ThemeBG) string    { return t.GetBGANSI(color) }
func (t *Theme) GetColorMode() ColorMode {
	if t == nil {
		return ""
	}
	return t.mode
}
func (t *Theme) GetThinkingBorderColor(level agent.ThinkingLevel) (tui.TextStyleFunc, error) {
	if t == nil || t.foreground == nil {
		return nil, notImplemented("Theme.GetThinkingBorderColor")
	}
	colors := map[agent.ThinkingLevel]ThemeColor{"off": "thinkingOff", "minimal": "thinkingMinimal", "low": "thinkingLow", "medium": "thinkingMedium", "high": "thinkingHigh", "xhigh": "thinkingXhigh", "max": "thinkingMax"}
	color, ok := colors[level]
	if !ok {
		color = "thinkingOff"
	}
	return func(text string) string { return t.FG(color, text) }, nil
}
func (t *Theme) GetBashModeBorderColor() tui.TextStyleFunc {
	return func(s string) string { return t.FG("bashMode", s) }
}

func GetLanguageFromPath(path string) string {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	switch ext {
	case "go", "js", "ts", "tsx", "jsx", "py", "rs", "java", "json", "yaml", "yml", "md", "sh", "bash":
		return ext
	default:
		return ""
	}
}
func GetMarkdownTheme() (tui.MarkdownTheme, error) {
	theme, err := LoadBuiltinTheme("dark")
	if err != nil {
		return tui.MarkdownTheme{}, err
	}
	return theme.MarkdownTheme(), nil
}
func GetSelectListTheme() (tui.SelectListTheme, error) {
	theme, err := LoadBuiltinTheme("dark")
	if err != nil {
		return tui.SelectListTheme{}, err
	}
	accent := func(s string) string { return theme.FG("accent", s) }
	muted := func(s string) string { return theme.FG("muted", s) }
	return tui.SelectListTheme{SelectedPrefix: accent, SelectedText: accent, Description: muted, ScrollInfo: muted, NoMatch: muted}, nil
}
func GetSettingsListTheme() (tui.SettingsListTheme, error) {
	theme, err := LoadBuiltinTheme("dark")
	if err != nil {
		return tui.SettingsListTheme{}, err
	}
	return settingsListTheme(func() *Theme { return theme }), nil
}
func InitTheme(...any) error { return notImplemented("InitTheme") }

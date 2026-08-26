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

type AssistantMessageComponent struct{ tui.Container }

func (*AssistantMessageComponent) Invalidate() error {
	return notImplemented("AssistantMessageComponent.Invalidate")
}
func (*AssistantMessageComponent) Render(int) ([]string, error) {
	return nil, notImplemented("AssistantMessageComponent.Render")
}

func (*AssistantMessageComponent) SetHiddenThinkingLabel(string) error {
	return notImplemented("AssistantMessageComponent.SetHiddenThinkingLabel")
}
func (*AssistantMessageComponent) SetHideThinkingBlock(bool) error {
	return notImplemented("AssistantMessageComponent.SetHideThinkingBlock")
}
func (*AssistantMessageComponent) SetOutputPad(bool) error {
	return notImplemented("AssistantMessageComponent.SetOutputPad")
}
func (*AssistantMessageComponent) UpdateContent(ai.AssistantMessage, ...bool) error {
	return notImplemented("AssistantMessageComponent.UpdateContent")
}

type BashExecutionComponent struct{ tui.Container }

func (*BashExecutionComponent) Invalidate() error {
	return notImplemented("BashExecutionComponent.Invalidate")
}

func (*BashExecutionComponent) AppendOutput(string) error {
	return notImplemented("BashExecutionComponent.AppendOutput")
}
func (*BashExecutionComponent) GetCommand() (string, error) {
	return "", notImplemented("BashExecutionComponent.GetCommand")
}
func (*BashExecutionComponent) GetOutput() (string, error) {
	return "", notImplemented("BashExecutionComponent.GetOutput")
}
func (*BashExecutionComponent) SetComplete(*int, bool, *TruncationResult, *string) error {
	return notImplemented("BashExecutionComponent.SetComplete")
}
func (*BashExecutionComponent) SetExpanded(bool) error {
	return notImplemented("BashExecutionComponent.SetExpanded")
}

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

func RenderDiff(string, ...RenderDiffOptions) (string, error) {
	return "", notImplemented("RenderDiff")
}

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

type ModelSelectorComponent struct {
	tui.Container
	Focused bool
}

func (*ModelSelectorComponent) Dispose() error {
	return notImplemented("ModelSelectorComponent.Dispose")
}
func (*ModelSelectorComponent) GetSearchInput() (*tui.Input, error) {
	return nil, notImplemented("ModelSelectorComponent.GetSearchInput")
}
func (*ModelSelectorComponent) HandleInput(string) error {
	return notImplemented("ModelSelectorComponent.HandleInput")
}

type OAuthSelectorComponent struct {
	tui.Container
	Focused bool
}

func (*OAuthSelectorComponent) HandleInput(string) error {
	return notImplemented("OAuthSelectorComponent.HandleInput")
}

type SessionSelectorComponent struct {
	tui.Container
	Focused bool
}

func (*SessionSelectorComponent) GetSessionList() (*tui.SelectList, error) {
	return nil, notImplemented("SessionSelectorComponent.GetSessionList")
}
func (*SessionSelectorComponent) HandleInput(string) error {
	return notImplemented("SessionSelectorComponent.HandleInput")
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

type SettingsSelectorComponent struct{ tui.Container }

func (*SettingsSelectorComponent) GetSettingsList() (*tui.SettingsList, error) {
	return nil, notImplemented("SettingsSelectorComponent.GetSettingsList")
}

type ShowImagesSelectorComponent struct{ tui.Container }

func (*ShowImagesSelectorComponent) GetSelectList() (*tui.SelectList, error) {
	return nil, notImplemented("ShowImagesSelectorComponent.GetSelectList")
}

type ThemeSelectorComponent struct{ tui.Container }

func (*ThemeSelectorComponent) GetSelectList() (*tui.SelectList, error) {
	return nil, notImplemented("ThemeSelectorComponent.GetSelectList")
}

type ThinkingSelectorComponent struct{ tui.Container }

func (*ThinkingSelectorComponent) GetSelectList() (*tui.SelectList, error) {
	return nil, notImplemented("ThinkingSelectorComponent.GetSelectList")
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

type ToolExecutionComponent struct{ tui.Container }

func (*ToolExecutionComponent) Invalidate() error {
	return notImplemented("ToolExecutionComponent.Invalidate")
}
func (*ToolExecutionComponent) Render(int) ([]string, error) {
	return nil, notImplemented("ToolExecutionComponent.Render")
}

func (*ToolExecutionComponent) MarkExecutionStarted() error {
	return notImplemented("ToolExecutionComponent.MarkExecutionStarted")
}
func (*ToolExecutionComponent) SetArgsComplete(bool) error {
	return notImplemented("ToolExecutionComponent.SetArgsComplete")
}
func (*ToolExecutionComponent) SetExpanded(bool) error {
	return notImplemented("ToolExecutionComponent.SetExpanded")
}
func (*ToolExecutionComponent) SetImageWidthCells(int) error {
	return notImplemented("ToolExecutionComponent.SetImageWidthCells")
}
func (*ToolExecutionComponent) SetShowImages(bool) error {
	return notImplemented("ToolExecutionComponent.SetShowImages")
}
func (*ToolExecutionComponent) UpdateArgs(any) error {
	return notImplemented("ToolExecutionComponent.UpdateArgs")
}
func (*ToolExecutionComponent) UpdateResult(ToolExecutionResult, ...bool) error {
	return notImplemented("ToolExecutionComponent.UpdateResult")
}

type TreeSelectorComponent struct {
	tui.Container
	Focused bool
}

func (*TreeSelectorComponent) GetTreeList() (*tui.SelectList, error) {
	return nil, notImplemented("TreeSelectorComponent.GetTreeList")
}
func (*TreeSelectorComponent) HandleInput(string) error {
	return notImplemented("TreeSelectorComponent.HandleInput")
}
func (*TreeSelectorComponent) OnCopy(func(string)) error {
	return notImplemented("TreeSelectorComponent.OnCopy")
}

type UserMessageSelectorComponent struct{ tui.Container }

func (*UserMessageSelectorComponent) GetMessageList() (*tui.SelectList, error) {
	return nil, notImplemented("UserMessageSelectorComponent.GetMessageList")
}

type UserMessageComponent struct{ tui.Container }

func (*UserMessageComponent) Render(int) ([]string, error) {
	return nil, notImplemented("UserMessageComponent.Render")
}

func (*UserMessageComponent) SetOutputPad(bool) error {
	return notImplemented("UserMessageComponent.SetOutputPad")
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
	Name       string
	SourceInfo *SourceInfo
	SourcePath string
	mode       ColorMode
	foreground map[ThemeColor]string
	background map[ThemeBG]string
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
func (*Theme) Bold(text string) string                { return "\x1b[1m" + text + "\x1b[22m" }
func (*Theme) Italic(text string) string              { return "\x1b[3m" + text + "\x1b[23m" }
func (*Theme) Underline(text string) string           { return "\x1b[4m" + text + "\x1b[24m" }
func (*Theme) Inverse(text string) string             { return "\x1b[7m" + text + "\x1b[27m" }
func (*Theme) Strikethrough(text string) string       { return "\x1b[9m" + text + "\x1b[29m" }
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
func (*Theme) GetThinkingBorderColor(agent.ThinkingLevel) (tui.TextStyleFunc, error) {
	return nil, notImplemented("Theme.GetThinkingBorderColor")
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
	return tui.MarkdownTheme{}, notImplemented("GetMarkdownTheme")
}
func GetSelectListTheme() (tui.SelectListTheme, error) {
	return tui.SelectListTheme{}, notImplemented("GetSelectListTheme")
}
func GetSettingsListTheme() (tui.SettingsListTheme, error) {
	return tui.SettingsListTheme{}, notImplemented("GetSettingsListTheme")
}
func HighlightCode(string, ...string) ([]string, error) {
	return nil, notImplemented("HighlightCode")
}
func InitTheme(...any) error { return notImplemented("InitTheme") }

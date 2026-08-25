package tui

import (
	"context"
	"reflect"
)

// CursorMarker is the zero-width marker emitted by a focused component at its
// logical cursor position.
const CursorMarker = "\x1b_pi:c\x07"

// ViewportTUIMarker preserves the registry key used by Pi's viewport TUI
// capability. Go callers use the ViewportTUI interface for type checks.
const ViewportTUIMarker = "@earendil-works/pi-tui/viewport"

// Component is the smallest renderable TUI seam. Optional input and key-release
// behavior is represented by ComponentInputHandler and KeyReleaseRequester.
type Component interface {
	Render(width int) ([]string, error)
	Invalidate() error
}

// ComponentInputHandler is implemented by components that accept keyboard
// input while focused.
type ComponentInputHandler interface {
	HandleInput(data string) error
}

// KeyReleaseRequester is implemented by components that want Kitty keyboard
// release events.
type KeyReleaseRequester interface {
	WantsKeyRelease() bool
}

// Focusable is the mutable focus-state capability used by TUI renderers.
type Focusable interface {
	SetFocusState(bool)
}

// IsFocusable performs the structural capability check represented by Pi's
// isFocusable type guard.
func IsFocusable(component Component) (Focusable, bool) {
	focusable, ok := component.(Focusable)
	return focusable, ok
}

// Container renders its children in insertion order. Its operations are local
// and do not access a terminal.
type Container struct {
	children []Component
}

func NewContainer() *Container { return &Container{} }

func (c *Container) Children() []Component {
	return append([]Component(nil), c.children...)
}

func (c *Container) AddChild(component Component) {
	c.children = append(c.children, component)
}

func (c *Container) RemoveChild(component Component) {
	for index, child := range c.children {
		if sameComponent(child, component) {
			c.children = append(c.children[:index], c.children[index+1:]...)
			return
		}
	}
}

func (c *Container) Clear() { c.children = nil }

func (c *Container) Invalidate() error {
	for _, child := range c.children {
		if err := child.Invalidate(); err != nil {
			return err
		}
	}
	return nil
}

func (c *Container) Render(width int) ([]string, error) {
	var lines []string
	for _, child := range c.children {
		childLines, err := child.Render(width)
		if err != nil {
			return nil, err
		}
		lines = append(lines, childLines...)
	}
	return lines, nil
}

func sameComponent(left, right Component) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	leftValue, rightValue := reflect.ValueOf(left), reflect.ValueOf(right)
	if leftValue.Type() != rightValue.Type() {
		return false
	}
	if leftValue.Type().Comparable() {
		return leftValue.Interface() == rightValue.Interface()
	}
	return false
}

// CompositeTUILine will provide ANSI- and grapheme-aware overlay composition
// at the interactive TUI milestone.
func CompositeTUILine(string, string, int, int, int) (string, error) {
	return "", newNotImplemented("compositeTuiLine")
}

type OverlayAnchor string

const (
	OverlayAnchorCenter       OverlayAnchor = "center"
	OverlayAnchorTopLeft      OverlayAnchor = "top-left"
	OverlayAnchorTopRight     OverlayAnchor = "top-right"
	OverlayAnchorBottomLeft   OverlayAnchor = "bottom-left"
	OverlayAnchorBottomRight  OverlayAnchor = "bottom-right"
	OverlayAnchorTopCenter    OverlayAnchor = "top-center"
	OverlayAnchorBottomCenter OverlayAnchor = "bottom-center"
	OverlayAnchorLeftCenter   OverlayAnchor = "left-center"
	OverlayAnchorRightCenter  OverlayAnchor = "right-center"
)

type OverlayMargin struct {
	Top    *int
	Right  *int
	Bottom *int
	Left   *int
}

// SizeValue represents either an absolute terminal-cell count or a percentage.
// Exactly one field should be set.
type SizeValue struct {
	Absolute   *int
	Percentage *float64
}

// OverlayMarginValue represents either one margin for every edge or individual
// edge margins. Exactly one field should be set.
type OverlayMarginValue struct {
	All   *int
	Edges *OverlayMargin
}

type OverlayOptions struct {
	Width        *SizeValue
	MinWidth     *int
	MaxHeight    *SizeValue
	Anchor       *OverlayAnchor
	OffsetX      *int
	OffsetY      *int
	Row          *SizeValue
	Col          *SizeValue
	Margin       *OverlayMarginValue
	Visible      func(termWidth, termHeight int) bool
	NonCapturing *bool
}

type OverlayUnfocusOptions struct {
	Target Component
}

type OverlayHandle interface {
	Hide() error
	SetHidden(bool) error
	IsHidden() bool
	Focus() error
	Unfocus(...OverlayUnfocusOptions) error
	IsFocused() bool
}

type TUIInputListenerResult struct {
	Consume *bool
	Data    *string
}

type TUIInputListener func(string) TUIInputListenerResult
type TUIUnsubscribe func() error
type TerminalColorSchemeListener func(TerminalColorScheme)

type TUIMode string

const (
	TUIModeRegular    TUIMode = "regular"
	TUIModeFullscreen TUIMode = "fullscreen"
)

type TUIStopOptions struct {
	PreserveScreen *bool
}

type TerminalColorQueryOptions struct {
	TimeoutMS int64
}

// TUI is the application-facing renderer boundary. Methods that would start
// I/O, schedule work, or mutate interactive renderer state return errors.
type TUI interface {
	Component
	ComponentInputHandler
	KeyReleaseRequester
	Mode() TUIMode
	Children() []Component
	Terminal() Terminal
	OnDebug() func()
	SetOnDebug(func())
	FullRedraws() int
	AddChild(Component)
	RemoveChild(Component)
	Clear()
	GetShowHardwareCursor() bool
	SetShowHardwareCursor(bool) error
	GetClearOnShrink() bool
	SetClearOnShrink(bool) error
	SetFocus(Component) error
	ShowOverlay(Component, ...OverlayOptions) (OverlayHandle, error)
	HideOverlay() error
	HasOverlay() bool
	Start() error
	Stop(...TUIStopOptions) error
	RenderNow(...bool) error
	RequestRender(...bool) error
	AddInputListener(TUIInputListener) (TUIUnsubscribe, error)
	RemoveInputListener(TUIInputListener) error
	OnTerminalColorSchemeChange(TerminalColorSchemeListener) (TUIUnsubscribe, error)
	SetTerminalColorSchemeNotifications(bool) error
	QueryTerminalBackgroundColor(context.Context, TerminalColorQueryOptions) (RGBColor, bool, error)
	QueryTerminalColorScheme(context.Context, TerminalColorQueryOptions) (TerminalColorScheme, bool, error)
}

type ViewportTUI interface {
	TUI
	IsViewportTUI() bool
	SetLayoutRoot(Component) error
}

func IsViewportTUI(ui TUI) (ViewportTUI, bool) {
	viewport, ok := ui.(ViewportTUI)
	return viewport, ok && viewport.IsViewportTUI()
}

type TUIBaseOptions struct {
	ShowHardwareCursor *bool
	LogDirectory       string
}

// TUIBase records renderer dependencies without starting terminal input,
// writing escape sequences, reading environment state, or creating timers.
type TUIBase struct {
	Container
	terminal           Terminal
	onDebug            func()
	showHardwareCursor bool
	clearOnShrink      bool
	logDirectory       string
	fullRedraws        int
	focusedComponent   Component
	overlays           []Component
}

func NewTUIBase(terminal Terminal, options ...TUIBaseOptions) *TUIBase {
	base := &TUIBase{terminal: terminal}
	if len(options) != 0 {
		base.logDirectory = options[0].LogDirectory
		if options[0].ShowHardwareCursor != nil {
			base.showHardwareCursor = *options[0].ShowHardwareCursor
		}
	}
	return base
}

func (*TUIBase) Mode() TUIMode                    { return TUIModeRegular }
func (t *TUIBase) Terminal() Terminal             { return t.terminal }
func (t *TUIBase) OnDebug() func()                { return t.onDebug }
func (t *TUIBase) SetOnDebug(callback func())     { t.onDebug = callback }
func (t *TUIBase) FullRedraws() int               { return t.fullRedraws }
func (t *TUIBase) HasOverlayEntries() bool        { return len(t.overlays) != 0 }
func (t *TUIBase) GetShowHardwareCursor() bool    { return t.showHardwareCursor }
func (t *TUIBase) GetClearOnShrink() bool         { return t.clearOnShrink }
func (t *TUIBase) GetFocusedComponent() Component { return t.focusedComponent }
func (t *TUIBase) HasOverlay() bool               { return len(t.overlays) != 0 }
func (*TUIBase) WantsKeyRelease() bool            { return false }
func (*TUIBase) HandleInput(string) error         { return newNotImplemented("TUIBase.handleInput") }
func (*TUIBase) Render(int) ([]string, error)     { return nil, newNotImplemented("TUIBase.render") }
func (*TUIBase) Start() error                     { return newNotImplemented("TUIBase.start") }
func (*TUIBase) Stop(...TUIStopOptions) error     { return newNotImplemented("TUIBase.stop") }
func (*TUIBase) RenderNow(...bool) error          { return newNotImplemented("TUIBase.renderNow") }
func (*TUIBase) RequestRender(...bool) error      { return newNotImplemented("TUIBase.requestRender") }
func (*TUIBase) SetFocus(Component) error         { return newNotImplemented("TUIBase.setFocus") }
func (*TUIBase) HideOverlay() error               { return newNotImplemented("TUIBase.hideOverlay") }
func (*TUIBase) SetShowHardwareCursor(bool) error {
	return newNotImplemented("TUIBase.setShowHardwareCursor")
}
func (*TUIBase) SetClearOnShrink(bool) error { return newNotImplemented("TUIBase.setClearOnShrink") }
func (*TUIBase) ShowOverlay(Component, ...OverlayOptions) (OverlayHandle, error) {
	return nil, newNotImplemented("TUIBase.showOverlay")
}
func (*TUIBase) AddInputListener(TUIInputListener) (TUIUnsubscribe, error) {
	return nil, newNotImplemented("TUIBase.addInputListener")
}
func (*TUIBase) RemoveInputListener(TUIInputListener) error {
	return newNotImplemented("TUIBase.removeInputListener")
}
func (*TUIBase) OnTerminalColorSchemeChange(TerminalColorSchemeListener) (TUIUnsubscribe, error) {
	return nil, newNotImplemented("TUIBase.onTerminalColorSchemeChange")
}
func (*TUIBase) SetTerminalColorSchemeNotifications(bool) error {
	return newNotImplemented("TUIBase.setTerminalColorSchemeNotifications")
}
func (*TUIBase) QueryTerminalBackgroundColor(context.Context, TerminalColorQueryOptions) (RGBColor, bool, error) {
	return RGBColor{}, false, newNotImplemented("TUIBase.queryTerminalBackgroundColor")
}
func (*TUIBase) QueryTerminalColorScheme(context.Context, TerminalColorQueryOptions) (TerminalColorScheme, bool, error) {
	return "", false, newNotImplemented("TUIBase.queryTerminalColorScheme")
}

type TUIAltScreenOptions struct {
	ShowHardwareCursor *bool
	LogDirectory       string
	WheelScrollLines   *int
	Mouse              *bool
	OpenURL            func(string)
	OnRightClickPaste  func()
}

type TUIAltScreen struct {
	*TUIBase
	wheelScrollLines  int
	mouseEnabled      bool
	layoutRoot        Component
	viewportTop       int
	followingOutput   bool
	openURL           func(string)
	onRightClickPaste func()
}

func NewTUIAltScreen(terminal Terminal, options ...TUIAltScreenOptions) *TUIAltScreen {
	alt := &TUIAltScreen{TUIBase: NewTUIBase(terminal), wheelScrollLines: 1, mouseEnabled: true, followingOutput: true}
	if len(options) == 0 {
		return alt
	}
	option := options[0]
	alt.TUIBase = NewTUIBase(terminal, TUIBaseOptions{
		ShowHardwareCursor: option.ShowHardwareCursor,
		LogDirectory:       option.LogDirectory,
	})
	if option.WheelScrollLines != nil {
		alt.wheelScrollLines = *option.WheelScrollLines
	}
	if option.Mouse != nil {
		alt.mouseEnabled = *option.Mouse
	}
	alt.openURL = option.OpenURL
	alt.onRightClickPaste = option.OnRightClickPaste
	return alt
}

func (*TUIAltScreen) Mode() TUIMode             { return TUIModeFullscreen }
func (*TUIAltScreen) IsViewportTUI() bool       { return true }
func (t *TUIAltScreen) ViewportTop() int        { return t.viewportTop }
func (t *TUIAltScreen) IsFollowingOutput() bool { return t.followingOutput }
func (*TUIAltScreen) Render(int) ([]string, error) {
	return nil, newNotImplemented("TUIAltScreen.render")
}
func (*TUIAltScreen) SetLayoutRoot(Component) error {
	return newNotImplemented("TUIAltScreen.setLayoutRoot")
}
func (*TUIAltScreen) ScrollBy(int) error {
	return newNotImplemented("TUIAltScreen.scrollBy")
}
func (*TUIAltScreen) ScrollToTop() error {
	return newNotImplemented("TUIAltScreen.scrollToTop")
}
func (*TUIAltScreen) ScrollToBottom() error {
	return newNotImplemented("TUIAltScreen.scrollToBottom")
}
func (*TUIAltScreen) Flash(string, ...int64) error {
	return newNotImplemented("TUIAltScreen.flash")
}

type TUIMainScreenRenderState struct {
	PreviousLines       []string
	PreviousWidth       int
	PreviousHeight      int
	CursorRow           int
	HardwareCursorRow   int
	MaxLinesRendered    int
	PreviousViewportTop int
}

type TUIMainScreen struct {
	*TUIBase
	renderState TUIMainScreenRenderState
}

func NewTUIMainScreen(terminal Terminal, options ...TUIBaseOptions) *TUIMainScreen {
	return &TUIMainScreen{TUIBase: NewTUIBase(terminal, options...)}
}

func (*TUIMainScreen) Mode() TUIMode { return TUIModeRegular }
func (*TUIMainScreen) Render(int) ([]string, error) {
	return nil, newNotImplemented("TUIMainScreen.render")
}
func (t *TUIMainScreen) CaptureRenderState() TUIMainScreenRenderState {
	state := t.renderState
	state.PreviousLines = append([]string(nil), state.PreviousLines...)
	return state
}
func (*TUIMainScreen) RestoreRenderState(TUIMainScreenRenderState) error {
	return newNotImplemented("TUIMainScreen.restoreRenderState")
}

var (
	_ TUI         = (*TUIBase)(nil)
	_ TUI         = (*TUIAltScreen)(nil)
	_ ViewportTUI = (*TUIAltScreen)(nil)
	_ TUI         = (*TUIMainScreen)(nil)
)

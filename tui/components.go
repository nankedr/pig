package tui

import "context"

// TextStyleFunc applies terminal styling to text. Themes use functions rather
// than encoded color values so callers retain control of their ANSI library.
type TextStyleFunc func(string) string

// TextStyleStateFunc applies terminal styling whose appearance depends on a
// selected or focused state.
type TextStyleStateFunc func(string, bool) string

// RenderRequester is the narrow renderer seam used by components that only
// need to schedule a future render.
type RenderRequester interface {
	RequestRender(...bool) error
}

// AltScreenFlashContainer is the transient-message component used by the
// alternate-screen renderer. Flash is deliberately inert until timers and
// interactive rendering are implemented.
type AltScreenFlashContainer struct {
	requestRender func()
}

func NewAltScreenFlashContainer(requestRender func()) *AltScreenFlashContainer {
	return &AltScreenFlashContainer{requestRender: requestRender}
}

func (*AltScreenFlashContainer) Flash(string, ...int64) error {
	return newNotImplemented("AltScreenFlashContainer.flash")
}

func (*AltScreenFlashContainer) Dispose() error {
	return newNotImplemented("AltScreenFlashContainer.dispose")
}
func (*AltScreenFlashContainer) Invalidate() error { return nil }
func (*AltScreenFlashContainer) Render(int) ([]string, error) {
	return nil, newNotImplemented("AltScreenFlashContainer.render")
}

// Box is a padded component container. Its child list is constructor state;
// layout and rendering remain explicit capability stubs.
type Box struct {
	Children []Component
	paddingX int
	paddingY int
	bgFunc   TextStyleFunc
}

func NewBox(paddingX, paddingY int, bgFn ...TextStyleFunc) *Box {
	b := &Box{paddingX: paddingX, paddingY: paddingY}
	if len(bgFn) != 0 {
		b.bgFunc = bgFn[0]
	}
	return b
}

func (*Box) AddChild(Component) error      { return newNotImplemented("Box.addChild") }
func (*Box) RemoveChild(Component) error   { return newNotImplemented("Box.removeChild") }
func (*Box) Clear() error                  { return newNotImplemented("Box.clear") }
func (*Box) SetBGFunc(TextStyleFunc) error { return newNotImplemented("Box.setBgFn") }
func (*Box) Invalidate() error             { return nil }
func (*Box) Render(int) ([]string, error)  { return nil, newNotImplemented("Box.render") }

// LoaderIndicatorOptions configures a Loader animation. An empty Frames slice
// means that the indicator is hidden.
type LoaderIndicatorOptions struct {
	Frames     []string
	IntervalMS *int64
}

// Text is a multi-line text component. Mutators only update local state; no
// rendering, callback, terminal, or timer work is performed.
type Text struct {
	text       string
	paddingX   int
	paddingY   int
	customBGFn TextStyleFunc
}

func NewText(text string, paddingX, paddingY int, customBGFn ...TextStyleFunc) *Text {
	t := &Text{text: text, paddingX: paddingX, paddingY: paddingY}
	if len(customBGFn) != 0 {
		t.customBGFn = customBGFn[0]
	}
	return t
}

func (t *Text) SetText(text string)            { t.text = text }
func (t *Text) SetCustomBGFn(fn TextStyleFunc) { t.customBGFn = fn }
func (*Text) Invalidate() error                { return nil }
func (*Text) Render(int) ([]string, error)     { return nil, newNotImplemented("Text.render") }

// Loader is the animated status-text component. Construction never starts an
// animation; Start and SetIndicator fail before creating a timer.
type Loader struct {
	Text
	ui             RenderRequester
	spinnerColorFn TextStyleFunc
	messageColorFn TextStyleFunc
	message        string
	indicator      *LoaderIndicatorOptions
}

func NewLoader(ui RenderRequester, spinnerColorFn, messageColorFn TextStyleFunc, message string, indicator ...LoaderIndicatorOptions) *Loader {
	l := &Loader{
		Text: Text{paddingX: 1}, ui: ui, spinnerColorFn: spinnerColorFn, messageColorFn: messageColorFn, message: message,
	}
	if len(indicator) != 0 {
		copy := indicator[0]
		copy.Frames = append([]string(nil), copy.Frames...)
		l.indicator = &copy
	}
	return l
}

func (*Loader) Start() error { return newNotImplemented("Loader.start") }
func (*Loader) Stop() error  { return newNotImplemented("Loader.stop") }
func (*Loader) SetIndicator(LoaderIndicatorOptions) error {
	return newNotImplemented("Loader.setIndicator")
}
func (*Loader) SetMessage(string) error { return newNotImplemented("Loader.setMessage") }
func (*Loader) SetText(string) error    { return newNotImplemented("Loader.setText") }
func (*Loader) SetCustomBGFn(TextStyleFunc) error {
	return newNotImplemented("Loader.setCustomBgFn")
}
func (*Loader) Invalidate() error            { return nil }
func (*Loader) Render(int) ([]string, error) { return nil, newNotImplemented("Loader.render") }

// CancellableLoader adds cancellation state and input handling to Loader. A Go
// context carries the upstream AbortSignal contract.
type CancellableLoader struct {
	Loader
	OnAbort func()
	Signal  context.Context
	Aborted bool
}

func NewCancellableLoader(ui RenderRequester, spinnerColorFn, messageColorFn TextStyleFunc, message string, indicator ...LoaderIndicatorOptions) *CancellableLoader {
	return &CancellableLoader{Loader: *NewLoader(ui, spinnerColorFn, messageColorFn, message, indicator...), Signal: context.Background()}
}

func (*CancellableLoader) HandleInput(string) error {
	return newNotImplemented("CancellableLoader.handleInput")
}
func (*CancellableLoader) Dispose() error {
	return newNotImplemented("CancellableLoader.dispose")
}
func (*CancellableLoader) Render(int) ([]string, error) {
	return nil, newNotImplemented("CancellableLoader.render")
}
func (*CancellableLoader) SetCustomBGFn(TextStyleFunc) error {
	return newNotImplemented("CancellableLoader.setCustomBgFn")
}
func (*CancellableLoader) SetIndicator(LoaderIndicatorOptions) error {
	return newNotImplemented("CancellableLoader.setIndicator")
}
func (*CancellableLoader) SetMessage(string) error {
	return newNotImplemented("CancellableLoader.setMessage")
}
func (*CancellableLoader) SetText(string) error {
	return newNotImplemented("CancellableLoader.setText")
}
func (*CancellableLoader) Start() error {
	return newNotImplemented("CancellableLoader.start")
}
func (*CancellableLoader) Stop() error {
	return newNotImplemented("CancellableLoader.stop")
}

// ImageTheme controls the textual fallback used when an image cannot be shown.
type ImageTheme struct {
	FallbackColor TextStyleFunc
}

// ImageOptions configures terminal-cell limits and an optional reusable Kitty
// image identifier.
type ImageOptions struct {
	MaxWidthCells  *int
	MaxHeightCells *int
	Filename       *string
	ImageID        *uint32
}

// Image is the component form of terminal image data.
type Image struct {
	base64Data string
	mimeType   string
	theme      ImageTheme
	options    ImageOptions
	dimensions *ImageDimensions
	imageID    *uint32
}

func NewImage(base64Data, mimeType string, theme ImageTheme, options ImageOptions, dimensions ...ImageDimensions) *Image {
	i := &Image{base64Data: base64Data, mimeType: mimeType, theme: theme, options: options, imageID: options.ImageID}
	if len(dimensions) != 0 {
		d := dimensions[0]
		i.dimensions = &d
	}
	return i
}

func (i *Image) GetImageID() *uint32        { return i.imageID }
func (*Image) Invalidate() error            { return nil }
func (*Image) Render(int) ([]string, error) { return nil, newNotImplemented("Image.render") }

// Input is the single-line input component. Value access is state-only; input
// processing and rendering are deferred.
type Input struct {
	Focused  bool
	OnSubmit func(string)
	OnEscape func()
	value    string
}

func NewInput() *Input                      { return &Input{} }
func (i *Input) GetValue() string           { return i.value }
func (i *Input) SetValue(value string)      { i.value = value }
func (i *Input) SetFocusState(focused bool) { i.Focused = focused }
func (*Input) HandleInput(string) error     { return newNotImplemented("Input.handleInput") }
func (*Input) Invalidate() error            { return nil }
func (*Input) Render(int) ([]string, error) { return nil, newNotImplemented("Input.render") }

// DefaultTextStyle is the base style applied to unannotated Markdown text.
type DefaultTextStyle struct {
	Color         TextStyleFunc
	BGColor       TextStyleFunc
	Bold          *bool
	Italic        *bool
	Strikethrough *bool
	Underline     *bool
}

// MarkdownTheme contains styling functions for each Markdown construct.
type MarkdownTheme struct {
	Heading         TextStyleFunc
	Link            TextStyleFunc
	LinkURL         TextStyleFunc
	Code            TextStyleFunc
	CodeBlock       TextStyleFunc
	CodeBlockBorder TextStyleFunc
	Quote           TextStyleFunc
	QuoteBorder     TextStyleFunc
	HR              TextStyleFunc
	ListBullet      TextStyleFunc
	Bold            TextStyleFunc
	Italic          TextStyleFunc
	Strikethrough   TextStyleFunc
	Underline       TextStyleFunc
	HighlightCode   func(code string, language *string) []string
	CodeBlockIndent *string
}

// MarkdownOptions controls source transformation and optional render features.
type MarkdownOptions struct {
	PreserveOrderedListMarkers *bool
	PreserveBackslashEscapes   *bool
	Transform                  func(markdown string, availableWidth int) string
	RenderLatex                *bool
}

type Markdown struct {
	text             string
	paddingX         int
	paddingY         int
	theme            MarkdownTheme
	defaultTextStyle *DefaultTextStyle
	options          MarkdownOptions
}

func NewMarkdown(text string, paddingX, paddingY int, theme MarkdownTheme, defaultStyle *DefaultTextStyle, options ...MarkdownOptions) *Markdown {
	m := &Markdown{text: text, paddingX: paddingX, paddingY: paddingY, theme: theme, defaultTextStyle: defaultStyle}
	if len(options) != 0 {
		m.options = options[0]
	}
	return m
}

func (m *Markdown) SetText(text string)        { m.text = text }
func (*Markdown) Invalidate() error            { return nil }
func (*Markdown) Render(int) ([]string, error) { return nil, newNotImplemented("Markdown.render") }

type ScrollViewScrollbar string

const (
	ScrollViewScrollbarHidden ScrollViewScrollbar = "hidden"
	ScrollViewScrollbarAuto   ScrollViewScrollbar = "auto"
	ScrollViewScrollbarAlways ScrollViewScrollbar = "always"
)

type ScrollViewAxis string

const ScrollViewAxisVertical ScrollViewAxis = "vertical"

type ScrollViewFollow string

const (
	ScrollViewFollowNone ScrollViewFollow = "none"
	ScrollViewFollowEnd  ScrollViewFollow = "end"
)

type ScrollViewOverscroll string

const (
	ScrollViewOverscrollChain   ScrollViewOverscroll = "chain"
	ScrollViewOverscrollContain ScrollViewOverscroll = "contain"
)

type ScrollViewOptions struct {
	Axis                 *ScrollViewAxis
	Follow               *ScrollViewFollow
	Primary              *bool
	Overscroll           *ScrollViewOverscroll
	Scrollbar            *ScrollViewScrollbar
	ScrollbarStyle       TextStyleFunc
	ScrollbarHideDelayMS *int64
}

// ScrollView is a single-child viewport contract. It records immutable options
// but does not install hide timers or request renders in the scaffold.
type ScrollView struct {
	Primary              bool
	Overscroll           ScrollViewOverscroll
	ScrollbarStyle       TextStyleFunc
	child                Component
	axis                 ScrollViewAxis
	follow               ScrollViewFollow
	followingEnd         bool
	scrollbar            ScrollViewScrollbar
	scrollTop            int
	viewportHeight       int
	contentHeight        int
	scrollbarHideDelayMS int64
}

func NewScrollView(component Component, options ...ScrollViewOptions) *ScrollView {
	s := &ScrollView{
		child: component, axis: ScrollViewAxisVertical, follow: ScrollViewFollowNone,
		Overscroll: ScrollViewOverscrollChain, scrollbar: ScrollViewScrollbarHidden, scrollbarHideDelayMS: 1000,
	}
	if len(options) != 0 {
		o := options[0]
		if o.Axis != nil {
			s.axis = *o.Axis
		}
		if o.Follow != nil {
			s.follow = *o.Follow
			s.followingEnd = *o.Follow == ScrollViewFollowEnd
		}
		if o.Primary != nil {
			s.Primary = *o.Primary
		}
		if o.Overscroll != nil {
			s.Overscroll = *o.Overscroll
		}
		if o.Scrollbar != nil {
			s.scrollbar = *o.Scrollbar
		}
		if o.ScrollbarHideDelayMS != nil {
			s.scrollbarHideDelayMS = *o.ScrollbarHideDelayMS
		}
		s.ScrollbarStyle = o.ScrollbarStyle
	}
	return s
}

func (s *ScrollView) Children() []Component {
	if s.child == nil {
		return nil
	}
	return []Component{s.child}
}

func (*ScrollView) AddChild(Component) error    { return newNotImplemented("ScrollView.addChild") }
func (*ScrollView) RemoveChild(Component) error { return newNotImplemented("ScrollView.removeChild") }
func (*ScrollView) Clear() error                { return newNotImplemented("ScrollView.clear") }
func (*ScrollView) GetContentWidth(int) (int, error) {
	return 0, newNotImplemented("ScrollView.getContentWidth")
}
func (*ScrollView) Invalidate() error      { return nil }
func (s *ScrollView) IsFollowingEnd() bool { return s.followingEnd }
func (s *ScrollView) IsScrollbarVisible() bool {
	return s.scrollbar == ScrollViewScrollbarAlways && s.viewportHeight > 0
}
func (s *ScrollView) IsPrimary() bool                          { return s.Primary }
func (s *ScrollView) OverscrollBehavior() ScrollViewOverscroll { return s.Overscroll }
func (*ScrollView) Render(int) ([]string, error)               { return nil, newNotImplemented("ScrollView.render") }
func (*ScrollView) ScrollBy(int) (int, error)                  { return 0, newNotImplemented("ScrollView.scrollBy") }
func (*ScrollView) ScrollTo(int) error                         { return newNotImplemented("ScrollView.scrollTo") }
func (*ScrollView) ScrollToEnd() error                         { return newNotImplemented("ScrollView.scrollToEnd") }
func (*ScrollView) ScrollToStart() error                       { return newNotImplemented("ScrollView.scrollToStart") }
func (s *ScrollView) ScrollTop() int                           { return s.scrollTop }
func (s *ScrollView) Scrollbar() ScrollViewScrollbar           { return s.scrollbar }
func (*ScrollView) SetScrollbar(ScrollViewScrollbar) error {
	return newNotImplemented("ScrollView.setScrollbar")
}
func (*ScrollView) SetScrollbarActive(bool) error {
	return newNotImplemented("ScrollView.setScrollbarActive")
}
func (*ScrollView) UpdateLayout(int, int, func()) error {
	return newNotImplemented("ScrollView.updateLayout")
}
func (s *ScrollView) ViewportHeight() int { return s.viewportHeight }

type SelectItem struct {
	Value       string
	Label       string
	Description *string
}

type SelectListTheme struct {
	SelectedPrefix TextStyleFunc
	SelectedText   TextStyleFunc
	Description    TextStyleFunc
	ScrollInfo     TextStyleFunc
	NoMatch        TextStyleFunc
}

type SelectListTruncatePrimaryContext struct {
	Text        string
	MaxWidth    int
	ColumnWidth int
	Item        SelectItem
	IsSelected  bool
}

type SelectListLayoutOptions struct {
	MinPrimaryColumnWidth *int
	MaxPrimaryColumnWidth *int
	TruncatePrimary       func(SelectListTruncatePrimaryContext) string
}

type SelectList struct {
	OnSelect          func(SelectItem)
	OnCancel          func()
	OnSelectionChange func(SelectItem)
	items             []SelectItem
	maxVisible        int
	theme             SelectListTheme
	layout            SelectListLayoutOptions
}

func NewSelectList(items []SelectItem, maxVisible int, theme SelectListTheme, layout ...SelectListLayoutOptions) *SelectList {
	s := &SelectList{items: append([]SelectItem(nil), items...), maxVisible: maxVisible, theme: theme}
	if len(layout) != 0 {
		s.layout = layout[0]
	}
	return s
}

func (*SelectList) GetSelectedItem() (*SelectItem, error) {
	return nil, newNotImplemented("SelectList.getSelectedItem")
}
func (*SelectList) HandleInput(string) error     { return newNotImplemented("SelectList.handleInput") }
func (*SelectList) Invalidate() error            { return nil }
func (*SelectList) Render(int) ([]string, error) { return nil, newNotImplemented("SelectList.render") }
func (*SelectList) SetFilter(string) error       { return newNotImplemented("SelectList.setFilter") }
func (*SelectList) SetSelectedIndex(int) error {
	return newNotImplemented("SelectList.setSelectedIndex")
}

type SettingItem struct {
	ID           string
	Label        string
	Description  *string
	CurrentValue string
	Values       []string
	Submenu      func(currentValue string, done func(*string)) Component
}

type SettingsListTheme struct {
	Label       TextStyleStateFunc
	Value       TextStyleStateFunc
	Description TextStyleFunc
	Cursor      string
	Hint        TextStyleFunc
}

type SettingsListOptions struct {
	EnableSearch *bool
}

type SettingsList struct {
	items      []SettingItem
	maxVisible int
	theme      SettingsListTheme
	onChange   func(id, newValue string)
	onCancel   func()
	options    SettingsListOptions
}

func NewSettingsList(items []SettingItem, maxVisible int, theme SettingsListTheme, onChange func(string, string), onCancel func(), options ...SettingsListOptions) *SettingsList {
	s := &SettingsList{items: append([]SettingItem(nil), items...), maxVisible: maxVisible, theme: theme, onChange: onChange, onCancel: onCancel}
	if len(options) != 0 {
		s.options = options[0]
	}
	return s
}

func (*SettingsList) HandleInput(string) error { return newNotImplemented("SettingsList.handleInput") }
func (*SettingsList) Invalidate() error        { return nil }
func (*SettingsList) Render(int) ([]string, error) {
	return nil, newNotImplemented("SettingsList.render")
}
func (*SettingsList) UpdateValue(string, string) error {
	return newNotImplemented("SettingsList.updateValue")
}

type Spacer struct{ lines int }

func NewSpacer(lines ...int) *Spacer {
	value := 1
	if len(lines) != 0 {
		value = lines[0]
	}
	return &Spacer{lines: value}
}
func (s *Spacer) SetLines(lines int)         { s.lines = lines }
func (*Spacer) Invalidate() error            { return nil }
func (*Spacer) Render(int) ([]string, error) { return nil, newNotImplemented("Spacer.render") }

// StackBasis represents either a fixed terminal-cell size or the intrinsic
// "auto" size used by stack layout.
type StackBasis struct {
	Size int
	Auto bool
}

type StackEntryOptions struct {
	Basis   *StackBasis
	Grow    *int
	Shrink  *int
	MinSize *int
	MaxSize *int
	Visible func(LayoutViewport) bool
}

type StackEntry struct {
	Component Component
	StackEntryOptions
}

// StackChild maps the upstream Component | StackEntry union. Exactly one field
// should be populated by callers.
type StackChild struct {
	Component Component
	Entry     *StackEntry
}

type StackAlign string

const (
	StackAlignStretch StackAlign = "stretch"
	StackAlignStart   StackAlign = "start"
	StackAlignCenter  StackAlign = "center"
	StackAlignEnd     StackAlign = "end"
)

type StackOptions struct {
	Gap   *int
	Align *StackAlign
}

type Stack struct {
	entries []StackEntry
	gap     int
	align   StackAlign
}

func NewStack(children []StackChild, options ...StackOptions) *Stack {
	s := &Stack{align: StackAlignStretch}
	if len(options) != 0 {
		if options[0].Gap != nil {
			s.gap = *options[0].Gap
		}
		if options[0].Align != nil {
			s.align = *options[0].Align
		}
	}
	for _, child := range children {
		if child.Entry != nil {
			s.entries = append(s.entries, *child.Entry)
		} else if child.Component != nil {
			s.entries = append(s.entries, StackEntry{Component: child.Component})
		}
	}
	return s
}

func (s *Stack) Children() []Component {
	children := make([]Component, len(s.entries))
	for index := range s.entries {
		children[index] = s.entries[index].Component
	}
	return children
}

func (*Stack) AddChild(Component, ...StackEntryOptions) error {
	return newNotImplemented("Stack.addChild")
}
func (*Stack) RemoveChild(Component) error  { return newNotImplemented("Stack.removeChild") }
func (*Stack) Clear() error                 { return newNotImplemented("Stack.clear") }
func (*Stack) Invalidate() error            { return nil }
func (*Stack) Render(int) ([]string, error) { return nil, newNotImplemented("Stack.render") }

func VisibleStackEntries([]StackEntry, LayoutViewport) ([]StackEntry, error) {
	return nil, newNotImplemented("visibleStackEntries")
}

func AllocateStackSizes([]StackEntry, []int, *int, int) ([]int, error) {
	return nil, newNotImplemented("allocateStackSizes")
}

type VStack struct{ Stack }

func NewVStack(children []StackChild, options ...StackOptions) *VStack {
	return &VStack{Stack: *NewStack(children, options...)}
}
func (*VStack) AddChild(Component, ...StackEntryOptions) error {
	return newNotImplemented("VStack.addChild")
}
func (*VStack) RemoveChild(Component) error {
	return newNotImplemented("VStack.removeChild")
}
func (*VStack) Clear() error                 { return newNotImplemented("VStack.clear") }
func (*VStack) Render(int) ([]string, error) { return nil, newNotImplemented("VStack.render") }

type HStack struct{ Stack }

func NewHStack(children []StackChild, options ...StackOptions) *HStack {
	return &HStack{Stack: *NewStack(children, options...)}
}
func (*HStack) AddChild(Component, ...StackEntryOptions) error {
	return newNotImplemented("HStack.addChild")
}
func (*HStack) RemoveChild(Component) error {
	return newNotImplemented("HStack.removeChild")
}
func (*HStack) Clear() error                 { return newNotImplemented("HStack.clear") }
func (*HStack) Render(int) ([]string, error) { return nil, newNotImplemented("HStack.render") }

type TruncatedText struct {
	text     string
	paddingX int
	paddingY int
}

func NewTruncatedText(text string, paddingX, paddingY int) *TruncatedText {
	return &TruncatedText{text: text, paddingX: paddingX, paddingY: paddingY}
}
func (*TruncatedText) Invalidate() error { return nil }
func (*TruncatedText) Render(int) ([]string, error) {
	return nil, newNotImplemented("TruncatedText.render")
}

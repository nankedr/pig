package tui

// LayoutNodeMarker preserves the registry key behind Pi's symbol-keyed layout
// node capability. Go uses the LayoutComponent interface instead.
const LayoutNodeMarker = "@earendil-works/pi-tui/layout-node"

type LayoutViewport struct {
	Width  int
	Height int
}

type StackLayoutEntry struct {
	Component Component
	StackEntryOptions
}

type LayoutNodeType string

const (
	LayoutNodeTypeVStack LayoutNodeType = "vstack"
	LayoutNodeTypeHStack LayoutNodeType = "hstack"
	LayoutNodeTypeScroll LayoutNodeType = "scroll"
)

type LayoutNode interface {
	NodeType() LayoutNodeType
	isLayoutNode()
}

type StackLayoutNode struct {
	Type    LayoutNodeType
	Entries []StackLayoutEntry
	Gap     int
	Align   StackAlign
}

func (node StackLayoutNode) NodeType() LayoutNodeType { return node.Type }
func (StackLayoutNode) isLayoutNode()                 {}

type ScrollLayoutState interface {
	ScrollTop() int
	IsPrimary() bool
	OverscrollBehavior() ScrollViewOverscroll
	ViewportHeight() int
	GetContentWidth(width int) (int, error)
	UpdateLayout(contentHeight, viewportHeight int, requestRender func()) error
}

type ScrollLayoutNode struct {
	Type      LayoutNodeType
	Component Component
	State     ScrollLayoutState
}

func (node ScrollLayoutNode) NodeType() LayoutNodeType { return node.Type }
func (ScrollLayoutNode) isLayoutNode()                 {}

type LayoutComponent interface {
	Component
	LayoutNode() LayoutNode
}

func GetLayoutNode(component Component) (LayoutNode, bool) {
	layoutComponent, ok := component.(LayoutComponent)
	if !ok {
		return nil, false
	}
	return layoutComponent.LayoutNode(), true
}

type LayoutRect struct {
	X      int
	Y      int
	Width  int
	Height int
}

type LayoutBox struct {
	Component          Component
	Rect               LayoutRect
	Clip               LayoutRect
	Children           []*LayoutBox
	Parent             *LayoutBox
	Lines              []string
	LineOffset         *int
	ScrollView         *ScrollView
	ScrollContentLines []string
	Layer              int
}

type LayoutFrame struct {
	Root              *LayoutBox
	Width             int
	Height            int
	Lines             []string
	PrimaryScrollView *ScrollView
}

type ScrollbarGeometry struct {
	Column       int
	TrackTop     int
	TrackHeight  int
	ThumbTop     int
	ThumbHeight  int
	MaxScrollTop int
}

func GetScrollbarGeometry(*LayoutBox) (ScrollbarGeometry, bool, error) {
	return ScrollbarGeometry{}, false, newNotImplemented("getScrollbarGeometry")
}

func RenderLayoutFrame(Component, int, int, func()) (LayoutFrame, error) {
	return LayoutFrame{}, newNotImplemented("renderLayoutFrame")
}

func GetScrollViewBox(LayoutFrame, *ScrollView) (*LayoutBox, bool, error) {
	return nil, false, newNotImplemented("getScrollViewBox")
}

func GetScrollViewsAt(LayoutFrame, int, int) ([]*ScrollView, error) {
	return nil, newNotImplemented("getScrollViewsAt")
}

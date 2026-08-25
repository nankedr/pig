package tui

// ImageProtocol identifies a terminal inline-image protocol. The empty value
// represents a terminal with no supported image protocol.
type ImageProtocol string

const (
	ImageProtocolNone   ImageProtocol = ""
	ImageProtocolKitty  ImageProtocol = "kitty"
	ImageProtocolITerm2 ImageProtocol = "iterm2"
)

// TerminalCapabilities describes terminal features relevant to rich output.
type TerminalCapabilities struct {
	Images     ImageProtocol
	TrueColor  bool
	Hyperlinks bool
}

// CellDimensions is the size of one terminal cell in pixels.
type CellDimensions struct {
	WidthPX  int
	HeightPX int
}

// ImageDimensions is an image's intrinsic size in pixels.
type ImageDimensions struct {
	WidthPX  int
	HeightPX int
}

// ImageRenderOptions constrains inline-image placement. Pointer fields preserve
// the distinction between an omitted option and an explicit zero or false.
type ImageRenderOptions struct {
	MaxWidthCells       *int
	MaxHeightCells      *int
	PreserveAspectRatio *bool
	ImageID             *uint32
	MoveCursor          *bool
}

// ImageCellSize is the terminal-cell footprint of a rendered image.
type ImageCellSize struct {
	Columns int
	Rows    int
}

// KittyImageMetadata records the dimensions associated with a Kitty image ID.
type KittyImageMetadata struct {
	Columns  int
	Rows     int
	ImageID  uint32
	WidthPX  int
	HeightPX int
}

// KittyImagePlacement describes a placement-only replacement for an encoded
// Kitty image transmission.
type KittyImagePlacement struct {
	ImageID                uint32
	TransmissionGeneration uint64
	TransmissionBytes      int
	EstimatedDecodedBytes  int64
	Sequence               string
	ReplacementLine        string
}

// KittyEncodeOptions controls Kitty image transmission and placement.
type KittyEncodeOptions struct {
	Columns    *int
	Rows       *int
	ImageID    *uint32
	MoveCursor *bool
}

// ITerm2Dimension is an iTerm2 inline-image dimension such as "auto", a
// cell count, a pixel count, or a percentage as accepted by the protocol.
type ITerm2Dimension string

// ITerm2EncodeOptions controls iTerm2 inline-image transmission.
type ITerm2EncodeOptions struct {
	Width               *ITerm2Dimension
	Height              *ITerm2Dimension
	Name                string
	PreserveAspectRatio *bool
	Inline              *bool
}

// RenderedImage contains a terminal sequence and its cell footprint. ImageID
// is set only for protocols that expose an image identifier.
type RenderedImage struct {
	Sequence string
	Columns  int
	Rows     int
	ImageID  *uint32
}

// TmuxHyperlinkProbe reports whether the attached tmux client forwards OSC 8
// hyperlinks. DetectCapabilities does not invoke it while detection is a stub.
type TmuxHyperlinkProbe func() bool

func GetCellDimensions() (CellDimensions, error) {
	return CellDimensions{}, newNotImplemented("getCellDimensions")
}

func SetCellDimensions(CellDimensions) error {
	return newNotImplemented("setCellDimensions")
}

func DetectCapabilities(...TmuxHyperlinkProbe) (TerminalCapabilities, error) {
	return TerminalCapabilities{}, newNotImplemented("detectCapabilities")
}

func GetCapabilities() (TerminalCapabilities, error) {
	return TerminalCapabilities{}, newNotImplemented("getCapabilities")
}

func ResetCapabilitiesCache() error {
	return newNotImplemented("resetCapabilitiesCache")
}

func SetCapabilities(TerminalCapabilities) error {
	return newNotImplemented("setCapabilities")
}

func IsImageLine(string) (bool, error) {
	return false, newNotImplemented("isImageLine")
}

func AllocateImageID() (uint32, error) {
	return 0, newNotImplemented("allocateImageId")
}

func EncodeKitty(string, ...KittyEncodeOptions) (string, error) {
	return "", newNotImplemented("encodeKitty")
}

func DeleteKittyImage(uint32) (string, error) {
	return "", newNotImplemented("deleteKittyImage")
}

func DeleteAllKittyImages() (string, error) {
	return "", newNotImplemented("deleteAllKittyImages")
}

func DeleteAllKittyPlacements() (string, error) {
	return "", newNotImplemented("deleteAllKittyPlacements")
}

func EncodeITerm2(string, ...ITerm2EncodeOptions) (string, error) {
	return "", newNotImplemented("encodeITerm2")
}

func RegisterKittyImageMetadata(KittyImageMetadata) error {
	return newNotImplemented("registerKittyImageMetadata")
}

func GetKittyImageMetadata(string) (KittyImageMetadata, bool, error) {
	return KittyImageMetadata{}, false, newNotImplemented("getKittyImageMetadata")
}

func GetKittyImagePlacement(string) (KittyImagePlacement, bool, error) {
	return KittyImagePlacement{}, false, newNotImplemented("getKittyImagePlacement")
}

func CropKittyImageLine(string, int, int) (string, error) {
	return "", newNotImplemented("cropKittyImageLine")
}

func CalculateImageCellSize(ImageDimensions, int, *int, ...CellDimensions) (ImageCellSize, error) {
	return ImageCellSize{}, newNotImplemented("calculateImageCellSize")
}

func CalculateImageRows(ImageDimensions, int, ...CellDimensions) (int, error) {
	return 0, newNotImplemented("calculateImageRows")
}

func GetPNGDimensions(string) (ImageDimensions, bool, error) {
	return ImageDimensions{}, false, newNotImplemented("getPngDimensions")
}

func GetJPEGDimensions(string) (ImageDimensions, bool, error) {
	return ImageDimensions{}, false, newNotImplemented("getJpegDimensions")
}

func GetGIFDimensions(string) (ImageDimensions, bool, error) {
	return ImageDimensions{}, false, newNotImplemented("getGifDimensions")
}

func GetWebPDimensions(string) (ImageDimensions, bool, error) {
	return ImageDimensions{}, false, newNotImplemented("getWebpDimensions")
}

func GetImageDimensions(string, string) (ImageDimensions, bool, error) {
	return ImageDimensions{}, false, newNotImplemented("getImageDimensions")
}

func RenderImage(string, ImageDimensions, ...ImageRenderOptions) (RenderedImage, bool, error) {
	return RenderedImage{}, false, newNotImplemented("renderImage")
}

func Hyperlink(string, string) (string, error) {
	return "", newNotImplemented("hyperlink")
}

func ImageFallback(string, *ImageDimensions, ...string) (string, error) {
	return "", newNotImplemented("imageFallback")
}

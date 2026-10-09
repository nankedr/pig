package tui

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"strings"
)

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
	var bytes [4]byte
	for {
		if _, err := rand.Read(bytes[:]); err != nil {
			return 0, err
		}
		if id := binary.LittleEndian.Uint32(bytes[:]); id != 0 {
			return id, nil
		}
	}
}

func EncodeKitty(data string, options ...KittyEncodeOptions) (string, error) {
	if data == "" || len(data) > base64.StdEncoding.EncodedLen(8<<20) {
		return "", fmt.Errorf("invalid Kitty image size")
	}
	if _, err := base64.StdEncoding.Strict().DecodeString(data); err != nil {
		return "", err
	}
	if strings.ContainsAny(data, "\r\n") {
		return "", fmt.Errorf("Kitty base64 must not contain newlines")
	}
	params := "a=T,f=100,q=2"
	if len(options) > 0 {
		o := options[0]
		if o.MoveCursor != nil && !*o.MoveCursor {
			params += ",C=1"
		}
		if o.Columns != nil {
			if *o.Columns < 1 || *o.Columns > 10000 {
				return "", fmt.Errorf("invalid image columns")
			}
			params += fmt.Sprintf(",c=%d", *o.Columns)
		}
		if o.Rows != nil {
			if *o.Rows < 1 || *o.Rows > 10000 {
				return "", fmt.Errorf("invalid image rows")
			}
			params += fmt.Sprintf(",r=%d", *o.Rows)
		}
		if o.ImageID != nil {
			params += fmt.Sprintf(",i=%d", *o.ImageID)
		}

	}
	var output strings.Builder
	for offset := 0; offset < len(data); offset += 4096 {
		end := min(len(data), offset+4096)
		more := 0
		if end < len(data) {
			more = 1
		}
		control := fmt.Sprintf("m=%d", more)
		if offset == 0 {
			control = params
			if more == 1 {
				control += ",m=1"
			}
		}
		fmt.Fprintf(&output, "\x1b_G%s;%s\x1b\\", control, data[offset:end])
	}
	return output.String(), nil
}

func DeleteKittyImage(id uint32) (string, error) {
	if id == 0 {
		return "", fmt.Errorf("image ID must not be zero")
	}
	return fmt.Sprintf("\x1b_Ga=d,d=I,i=%d,q=2\x1b\\", id), nil
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

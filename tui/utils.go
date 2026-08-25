package tui

// RegexPattern preserves a JavaScript regular-expression source pattern until
// the Unicode-aware terminal text implementation is introduced.
type RegexPattern string

const (
	CJKBreakRegex    RegexPattern = `[\p{Script_Extensions=Han}\p{Script_Extensions=Hiragana}\p{Script_Extensions=Katakana}\p{Script_Extensions=Hangul}\p{Script_Extensions=Bopomofo}]`
	PunctuationRegex RegexPattern = "[(){}[\\]<>.,;:'\"!?+\\-=*/\\\\|&%^$#@~`]"
)

// SegmentGranularity selects the kind of Unicode text boundary produced by a
// TextSegmenter.
type SegmentGranularity string

const (
	SegmentGranularityGrapheme SegmentGranularity = "grapheme"
	SegmentGranularityWord     SegmentGranularity = "word"
)

// UnicodeSegment describes one Unicode segment. ByteOffset is an offset into the
// original UTF-8 string. WordLike is meaningful for word segmentation.
type UnicodeSegment struct {
	Text       string
	ByteOffset int
	WordLike   bool
}

// TextSegmenter is the Unicode segmentation seam returned by the shared
// segmenter accessors.
type TextSegmenter interface {
	Segment(string) ([]UnicodeSegment, error)
}

// GraphemeCellRange is the half-open terminal-cell range occupied by one
// grapheme.
type GraphemeCellRange struct {
	Start int
	End   int
}

// ANSICode is a terminal control sequence found at a byte offset. Length is
// measured in bytes.
type ANSICode struct {
	Code   string
	Length int
}

// TextSlice is a terminal-column slice and its actual visible width.
type TextSlice struct {
	Text  string
	Width int
}

// ExtractedSegments contains the portions before and after an overlay range.
type ExtractedSegments struct {
	Before      string
	BeforeWidth int
	After       string
	AfterWidth  int
}

// BackgroundFunc applies terminal background styling to text. Utility stubs do
// not invoke callbacks until terminal text rendering is implemented.
type BackgroundFunc func(string) string

// TruncateOptions controls ellipsis and padding behavior. A nil Ellipsis uses
// the upstream default; a non-nil pointer may explicitly select an empty value.
type TruncateOptions struct {
	Ellipsis *string
	Pad      bool
}

// FuzzyMatchResult reports whether a fuzzy match succeeded and its score. A
// lower score represents a better match.
type FuzzyMatchResult struct {
	Matches bool
	Score   float64
}

// RenderLaTeXOptions controls terminal layout of a LaTeX expression.
type RenderLaTeXOptions struct {
	Display bool
}

func GetGraphemeSegmenter() (TextSegmenter, error) {
	return nil, newNotImplemented("getGraphemeSegmenter")
}

func GetWordSegmenter() (TextSegmenter, error) {
	return nil, newNotImplemented("getWordSegmenter")
}

func VisibleWidth(string) (int, error) {
	return 0, newNotImplemented("visibleWidth")
}

func StripTerminalSequences(string) (string, error) {
	return "", newNotImplemented("stripTerminalSequences")
}

func GetGraphemeCellRange(string, int) (GraphemeCellRange, bool, error) {
	return GraphemeCellRange{}, false, newNotImplemented("getGraphemeCellRange")
}

func GetOSC8LinkAtColumn(string, int) (string, bool, error) {
	return "", false, newNotImplemented("getOsc8LinkAtColumn")
}

func NormalizeTerminalOutput(string) (string, error) {
	return "", newNotImplemented("normalizeTerminalOutput")
}

func ExtractANSICode(string, int) (ANSICode, bool, error) {
	return ANSICode{}, false, newNotImplemented("extractAnsiCode")
}

func WrapTextWithANSI(string, int) ([]string, error) {
	return nil, newNotImplemented("wrapTextWithAnsi")
}

func IsWhitespaceChar(string) (bool, error) {
	return false, newNotImplemented("isWhitespaceChar")
}

func IsPunctuationChar(string) (bool, error) {
	return false, newNotImplemented("isPunctuationChar")
}

func ApplyBackgroundToLine(string, int, BackgroundFunc) (string, error) {
	return "", newNotImplemented("applyBackgroundToLine")
}

func TruncateToWidth(string, int, ...TruncateOptions) (string, error) {
	return "", newNotImplemented("truncateToWidth")
}

func SliceByColumn(string, int, int, ...bool) (string, error) {
	return "", newNotImplemented("sliceByColumn")
}

func SliceWithWidth(string, int, int, ...bool) (TextSlice, error) {
	return TextSlice{}, newNotImplemented("sliceWithWidth")
}

func ExtractSegments(string, int, int, int, ...bool) (ExtractedSegments, error) {
	return ExtractedSegments{}, newNotImplemented("extractSegments")
}

func FuzzyMatch(string, string) (FuzzyMatchResult, error) {
	return FuzzyMatchResult{}, newNotImplemented("fuzzyMatch")
}

func FuzzyFilter[T any]([]T, string, func(T) string) ([]T, error) {
	return nil, newNotImplemented("fuzzyFilter")
}

func RenderLaTeX(string, ...RenderLaTeXOptions) (string, bool, error) {
	return "", false, newNotImplemented("renderLatex")
}

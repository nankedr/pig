package tui

// RGBColor is an 8-bit terminal color. Component values are represented as
// ints to match Pi's number-valued public record.
type RGBColor struct {
	R int
	G int
	B int
}

type TerminalColorScheme string

const (
	TerminalColorSchemeDark  TerminalColorScheme = "dark"
	TerminalColorSchemeLight TerminalColorScheme = "light"
)

// IsOSC11BackgroundColorResponse is kept inert with the other terminal-input
// parsers during the M0 capability scaffold.
func IsOSC11BackgroundColorResponse(string) (bool, error) {
	return false, newNotImplemented("isOsc11BackgroundColorResponse")
}

func ParseOSC11BackgroundColor(string) (RGBColor, bool, error) {
	return RGBColor{}, false, newNotImplemented("parseOsc11BackgroundColor")
}

func ParseTerminalColorSchemeReport(string) (TerminalColorScheme, bool, error) {
	return "", false, newNotImplemented("parseTerminalColorSchemeReport")
}

type ModifierKey string

const (
	ModifierKeyShift   ModifierKey = "shift"
	ModifierKeyCommand ModifierKey = "command"
	ModifierKeyControl ModifierKey = "control"
	ModifierKeyOption  ModifierKey = "option"
)

// IsNativeModifierPressed never loads a native helper or inspects platform
// state in the CGO-free capability scaffold.
func IsNativeModifierPressed(ModifierKey) (bool, error) {
	return false, newNotImplemented("isNativeModifierPressed")
}

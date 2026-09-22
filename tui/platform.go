package tui

import (
	"math"
	"regexp"
	"strconv"
	"strings"
)

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

var oscBackground = regexp.MustCompile(`(?i)^\x1b\]11;([^\x07\x1b]*)(?:\x07|\x1b\\)$`)
var schemeReport = regexp.MustCompile(`^(?:\x1b\[\?997;[12]n)+$`)
var hexChannel = regexp.MustCompile(`(?i)^[0-9a-f]+$`)

func IsOSC11BackgroundColorResponse(data string) (bool, error) {
	return oscBackground.MatchString(data), nil
}
func ParseOSC11BackgroundColor(data string) (RGBColor, bool, error) {
	m := oscBackground.FindStringSubmatch(data)
	if m == nil {
		return RGBColor{}, false, nil
	}
	value := strings.TrimSpace(m[1])
	var parts []string
	if strings.HasPrefix(value, "#") {
		value = value[1:]
		if len(value) != 6 && len(value) != 12 {
			return RGBColor{}, false, nil
		}
		n := len(value) / 3
		parts = []string{value[:n], value[n : 2*n], value[2*n:]}
	} else {
		lower := strings.ToLower(value)
		if strings.HasPrefix(lower, "rgb:") {
			value = value[4:]
		} else if strings.HasPrefix(lower, "rgba:") {
			value = value[5:]
		}
		parts = strings.Split(value, "/")
	}
	if len(parts) < 3 {
		return RGBColor{}, false, nil
	}
	values := [3]int{}
	for i, p := range parts[:3] {
		if !hexChannel.MatchString(p) {
			return RGBColor{}, false, nil
		}
		n, err := strconv.ParseUint(p, 16, 64)
		if err != nil {
			return RGBColor{}, false, nil
		}
		values[i] = int(math.Round(float64(n) / (math.Pow(16, float64(len(p))) - 1) * 255))
	}
	return RGBColor{R: values[0], G: values[1], B: values[2]}, true, nil
}
func ParseTerminalColorSchemeReport(data string) (TerminalColorScheme, bool, error) {
	if !schemeReport.MatchString(data) {
		return "", false, nil
	}
	if data[len(data)-2] == '2' {
		return TerminalColorSchemeLight, true, nil
	}
	return TerminalColorSchemeDark, true, nil
}

type ModifierKey string

const (
	ModifierKeyShift   ModifierKey = "shift"
	ModifierKeyCommand ModifierKey = "command"
	ModifierKeyControl ModifierKey = "control"
	ModifierKeyOption  ModifierKey = "option"
)

// IsNativeModifierPressed queries supported platform state, falling back to false when unavailable.
func IsNativeModifierPressed(key ModifierKey) (bool, error) { return nativeModifierPressed(key), nil }

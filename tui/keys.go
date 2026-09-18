package tui

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"unicode/utf8"
)

var kittyActive atomic.Bool

func SetKittyProtocolActive(active bool) error { kittyActive.Store(active); return nil }
func IsKittyProtocolActive() (bool, error)     { return kittyActive.Load(), nil }
func IsKeyRelease(data string) (bool, error)   { return keyEvent(data, "3"), nil }
func IsKeyRepeat(data string) (bool, error)    { return keyEvent(data, "2"), nil }
func keyEvent(data, event string) bool {
	if strings.Contains(data, "\x1b[200~") {
		return false
	}
	for _, end := range "u~ABCDHF" {
		if strings.Contains(data, ":"+event+string(end)) {
			return true
		}
	}
	return false
}

var csiKey = regexp.MustCompile(`^\x1b\[(\d+)(?::(\d*))?(?::(\d+))?(?:;(\d+))?(?::(\d+))?u$`)
var csiFunctional = regexp.MustCompile(`^\x1b\[(\d+)(?:;(\d+))?(?::(\d+))?([~ABCDHF])$`)
var modifyKey = regexp.MustCompile(`^\x1b\[27;(\d+);(\d+)~$`)

type terminalKey struct {
	cp, shifted, base, mod int
	kitty                  bool
}

func number(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return -1
	}
	return n
}
func parseTerminalKey(data string) (terminalKey, bool) {
	if m := csiKey.FindStringSubmatch(data); m != nil {
		mod := 0
		if m[4] != "" {
			mod = number(m[4]) - 1
		}
		return terminalKey{number(m[1]), number(m[2]), number(m[3]), mod, true}, true
	}
	if m := modifyKey.FindStringSubmatch(data); m != nil {
		return terminalKey{number(m[2]), -1, -1, number(m[1]) - 1, false}, true
	}
	if m := csiFunctional.FindStringSubmatch(data); m != nil {
		cp := 0
		if m[4] == "~" {
			cp = map[int]int{2: -11, 3: -10, 5: -12, 6: -13, 7: -14, 8: -15}[number(m[1])]
		} else if m[1] == "1" && m[2] != "" {
			cp = map[string]int{"A": -1, "B": -2, "C": -3, "D": -4, "H": -14, "F": -15}[m[4]]
		}
		if cp != 0 {
			mod := 0
			if m[2] != "" {
				mod = number(m[2]) - 1
			}
			return terminalKey{cp, -1, -1, mod, true}, true
		}
	}
	return terminalKey{}, false
}
func keypad(cp int) int {
	if cp >= 57399 && cp <= 57408 {
		return cp - 57399 + 48
	}
	if cp >= 57409 && cp <= 57426 {
		return []int{46, 47, 42, 45, 43, 57414, 61, 44, -4, -3, -1, -2, -12, -13, -14, -15, -11, -10}[cp-57409]
	}
	return cp
}
func identity(cp, mod int) int { return shiftIdentity(keypad(cp), mod) }
func shiftIdentity(cp, mod int) int {
	if mod&1 != 0 && cp >= 65 && cp <= 90 {
		return cp + 32
	}
	return cp
}
func knownSymbol(cp int) bool {
	return cp >= 33 && cp <= 126 && strings.ContainsRune("`-=[]\\;',./!@#$%^&*()_+|~{}:<>?", rune(cp))
}
func knownLetter(cp int) bool { return cp >= 97 && cp <= 122 }
func formatKey(cp, mod, base int) (KeyID, bool) {
	cp = identity(cp, mod)
	if !knownLetter(cp) && !(cp >= 48 && cp <= 57) && !knownSymbol(cp) && base >= 0 {
		cp = base
	}
	key := map[int]string{27: "escape", 9: "tab", 13: "enter", 57414: "enter", 32: "space", 127: "backspace", -1: "up", -2: "down", -3: "right", -4: "left", -10: "delete", -11: "insert", -12: "pageUp", -13: "pageDown", -14: "home", -15: "end"}[cp]
	if key == "" && (knownLetter(cp) || cp >= 48 && cp <= 57 || knownSymbol(cp)) {
		key = string(rune(cp))
	}
	mod &= ^192
	if key == "" || mod & ^15 != 0 {
		return "", false
	}
	prefix := ""
	for _, m := range []struct {
		bit  int
		name string
	}{{1, "shift"}, {4, "ctrl"}, {2, "alt"}, {8, "super"}} {
		if mod&m.bit != 0 {
			prefix += m.name + "+"
		}
	}
	return KeyID(prefix + key), true
}
func windowsTerminal() bool {
	return os.Getenv("WT_SESSION") != "" && os.Getenv("SSH_CONNECTION") == "" && os.Getenv("SSH_CLIENT") == "" && os.Getenv("SSH_TTY") == ""
}
func ParseKey(data string) (KeyID, bool, error) {
	key, ok := parseKey(data, kittyActive.Load())
	return key, ok, nil
}
func parseKey(data string, active bool) (KeyID, bool) {
	if k, ok := parseTerminalKey(data); ok {
		return formatKey(k.cp, k.mod, k.base)
	}
	if active && (data == "\x1b\r" || data == "\n") {
		return "shift+enter", true
	}
	if key, ok := legacyParsed[data]; ok {
		return KeyID(key), true
	}
	raw := map[string]KeyID{"\x1b": "escape", "\x1c": "ctrl+\\", "\x1d": "ctrl+]", "\x1f": "ctrl+-", "\x1b\x1b": "ctrl+alt+[", "\x1b\x1c": "ctrl+alt+\\", "\x1b\x1d": "ctrl+alt+]", "\x1b\x1f": "ctrl+alt+-", "\t": "tab", "\r": "enter", "\n": "enter", "\x1bOM": "enter", "\x00": "ctrl+space", " ": "space", "\x7f": "backspace", "\x08": "backspace", "\x1b[Z": "shift+tab", "\x1b\x7f": "alt+backspace", "\x1b\b": "alt+backspace", "\x1b[A": "up", "\x1b[B": "down", "\x1b[C": "right", "\x1b[D": "left", "\x1b[H": "home", "\x1b[F": "end"}
	if data == "\b" && windowsTerminal() {
		return "ctrl+backspace", true
	}
	if key, ok := raw[data]; ok {
		return key, true
	}
	if !active {
		if key, ok := map[string]KeyID{"\x1b\r": "alt+enter", "\x1b ": "alt+space", "\x1bB": "alt+left", "\x1bF": "alt+right"}[data]; ok {
			return key, true
		}
		if len(data) == 2 && data[0] == 27 {
			cp := int(data[1])
			if cp >= 1 && cp <= 26 {
				return KeyID("ctrl+alt+" + string(rune(cp+96))), true
			}
			if knownLetter(cp) || cp >= 48 && cp <= 57 || knownSymbol(cp) {
				return KeyID("alt+" + data[1:]), true
			}
		}
	}
	if len(data) == 1 {
		cp := data[0]
		if cp >= 1 && cp <= 26 {
			return KeyID("ctrl+" + string(rune(cp+96))), true
		}
		if cp >= 32 && cp <= 126 {
			return KeyID(data), true
		}
	}
	return "", false
}
func MatchesKey(data string, key KeyID) (bool, error) {
	return matchesKey(data, key, kittyActive.Load()), nil
}
func matchesKey(data string, id KeyID, active bool) bool {
	parts := strings.Split(strings.ToLower(string(id)), "+")
	name := parts[len(parts)-1]
	if name == "" {
		return false
	}
	mod := 0
	for _, p := range parts[:len(parts)-1] {
		mod |= map[string]int{"shift": 1, "alt": 2, "ctrl": 4, "super": 8}[p]
	}
	if name == "esc" {
		name = "escape"
	}
	if name == "return" {
		name = "enter"
	}
	cp, special := map[string]int{"escape": 27, "enter": 13, "tab": 9, "space": 32, "backspace": 127, "up": -1, "down": -2, "right": -3, "left": -4, "delete": -10, "insert": -11, "pageup": -12, "pagedown": -13, "home": -14, "end": -15}[name]
	printable := len(name) == 1 && (knownLetter(int(name[0])) || name[0] >= '0' && name[0] <= '9' || knownSymbol(int(name[0])))
	if printable {
		cp = int(name[0])
	}
	if name == "escape" && mod != 0 {
		return false
	}
	if k, ok := parseTerminalKey(data); ok && (special || printable) {
		expected := cp
		if name == "enter" && k.kitty && k.cp == 57414 {
			expected = 57414
		}
		if k.kitty {
			if k.mod & ^192 == mod {
				actual := identity(k.cp, k.mod)
				if actual == identity(expected, mod) {
					return true
				}
				if k.base >= 0 && k.base == expected && !knownLetter(actual) && !knownSymbol(actual) {
					return true
				}
			}
		} else {
			if printable {
				if mod != 0 && k.mod == mod && shiftIdentity(k.cp, k.mod) == shiftIdentity(expected, mod) {
					return true
				}
			} else if cp >= 0 {
				allowed := name != "tab" && name != "enter" || mod != 0
				if allowed && k.cp == expected && k.mod == mod {
					return true
				}
			}
		}
	}
	if mod == 0 {
		for _, s := range legacyKeys[name] {
			if data == s {
				return true
			}
		}
	}
	if mod == 1 {
		for _, s := range legacyShift[name] {
			if data == s {
				return true
			}
		}
	}
	if mod == 4 {
		for _, s := range legacyCtrl[name] {
			if data == s {
				return true
			}
		}
	}
	switch name {
	case "escape":
		return mod == 0 && data == "\x1b"
	case "space":
		return mod == 0 && data == " " || !active && (mod == 4 && data == "\x00" || mod == 2 && data == "\x1b ")
	case "tab":
		return mod == 0 && data == "\t" || mod == 1 && data == "\x1b[Z"
	case "enter":
		return mod == 0 && (data == "\r" || data == "\x1bOM" || !active && data == "\n") || mod == 1 && active && (data == "\n" || data == "\x1b\r") || mod == 2 && !active && data == "\x1b\r"
	case "backspace":
		return mod == 0 && (data == "\x7f" || !windowsTerminal() && data == "\b") || mod == 4 && windowsTerminal() && data == "\b" || mod == 2 && (data == "\x1b\x7f" || data == "\x1b\b")
	case "up", "down", "left", "right":
		if mod == 2 {
			s := map[string]string{"up": "p", "down": "n", "left": "b", "right": "f"}[name]
			return data == "\x1b"+s || !active && (name == "left" || name == "right") && data == "\x1b"+strings.ToUpper(s)
		}
	}
	if !printable {
		return false
	}
	ctrl := ""
	if knownLetter(cp) || strings.ContainsRune("[\\]_", rune(cp)) {
		ctrl = string(rune(cp & 31))
	}
	if name == "-" {
		ctrl = "\x1f"
	}
	return mod == 0 && data == name || mod == 4 && ctrl != "" && data == ctrl || mod == 1 && knownLetter(cp) && data == strings.ToUpper(name) || !active && (mod == 2 && data == "\x1b"+name || mod == 6 && ctrl != "" && data == "\x1b"+ctrl)
}
func DecodeKittyPrintable(data string) (string, bool, error) { return decodePrintable(data, true) }
func DecodePrintableKey(data string) (string, bool, error)   { return decodePrintable(data, false) }
func decodePrintable(data string, kittyOnly bool) (string, bool, error) {
	k, ok := parseTerminalKey(data)
	if !ok || kittyOnly && !csiKey.MatchString(data) {
		return "", false, nil
	}
	if k.mod & ^193 != 0 {
		return "", false, nil
	}
	cp := k.cp
	if k.kitty {
		if k.mod&1 != 0 && k.shifted >= 0 {
			cp = k.shifted
		}
		cp = keypad(cp)
	}
	if cp < 32 || cp > utf8.MaxRune || cp >= 0xd800 && cp <= 0xdfff {
		return "", false, nil
	}
	return string(rune(cp)), true, nil
}

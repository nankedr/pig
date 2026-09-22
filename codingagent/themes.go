package codingagent

import (
	"embed"
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"os"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

//go:embed themes/*.json
var builtinThemes embed.FS

var themeHex = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
var themeForeground = strings.Fields("accent border borderAccent borderMuted success error warning muted dim text thinkingText userMessageText customMessageText customMessageLabel toolTitle toolOutput mdHeading mdLink mdLinkUrl mdCode mdCodeBlock mdCodeBlockBorder mdQuote mdQuoteBorder mdHr mdListBullet toolDiffAdded toolDiffRemoved toolDiffContext syntaxComment syntaxKeyword syntaxFunction syntaxVariable syntaxString syntaxNumber syntaxType syntaxOperator syntaxPunctuation thinkingOff thinkingMinimal thinkingLow thinkingMedium thinkingHigh thinkingXhigh thinkingMax bashMode")
var themeBackground = strings.Fields("selectedBg scrollbarThumb userMessageBg customMessageBg toolPendingBg toolSuccessBg toolErrorBg")

// LoadThemeFromPath reads a regular local JSON file without executing theme content.
func LoadThemeFromPath(path string, modes ...ColorMode) (*Theme, error) {
	path, err := resolveSessionPath(path)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("theme path is not a regular file: %s", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	theme, err := parseTheme(data, modes)
	if err != nil {
		return nil, fmt.Errorf("theme %s: %w", path, err)
	}
	theme.SourcePath = path
	return theme, nil
}

func LoadBuiltinTheme(name string, modes ...ColorMode) (*Theme, error) {
	if name != "dark" && name != "light" {
		return nil, fmt.Errorf("Theme not found: %s", name)
	}
	data, err := builtinThemes.ReadFile("themes/" + name + ".json")
	if err != nil {
		return nil, err
	}
	return parseTheme(data, modes)
}

func parseTheme(data []byte, modes []ColorMode) (*Theme, error) {
	mode := detectThemeColorMode()
	if len(modes) > 1 {
		return nil, fmt.Errorf("expected at most one color mode")
	}
	if len(modes) == 1 {
		mode = modes[0]
	}
	if mode != ColorModeTrueColor && mode != ColorMode256 {
		return nil, fmt.Errorf("invalid color mode: %s", mode)
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	var name string
	if raw, ok := doc["name"]; !ok || string(raw) == "null" || json.Unmarshal(raw, &name) != nil || strings.Contains(name, "/") {
		return nil, fmt.Errorf("invalid theme name")
	}
	var colors, vars, exports map[string]any
	for key, target := range map[string]*map[string]any{"colors": &colors, "vars": &vars, "export": &exports} {
		if raw, ok := doc[key]; ok {
			if string(raw) == "null" || json.Unmarshal(raw, target) != nil {
				return nil, fmt.Errorf("invalid theme %s", key)
			}
		}
	}
	if raw, ok := doc["$schema"]; ok {
		var schema string
		if string(raw) == "null" || json.Unmarshal(raw, &schema) != nil {
			return nil, fmt.Errorf("invalid theme schema")
		}
	}
	valid := func(v any) bool {
		switch v := v.(type) {
		case string:
			return true
		case float64:
			return v >= 0 && v <= 255 && math.Trunc(v) == v
		}
		return false
	}
	for key, value := range vars {
		if !valid(value) {
			return nil, fmt.Errorf("invalid variable %s", key)
		}
	}
	for _, key := range []string{"pageBg", "cardBg", "infoBg"} {
		if value, ok := exports[key]; ok && !valid(value) {
			return nil, fmt.Errorf("invalid export color %s", key)
		}
	}
	if colors == nil {
		return nil, fmt.Errorf("missing theme colors")
	}
	if _, ok := colors["thinkingMax"]; !ok {
		colors["thinkingMax"] = colors["thinkingXhigh"]
	}
	if _, ok := colors["scrollbarThumb"]; !ok {
		colors["scrollbarThumb"] = colors["selectedBg"]
	}
	resolve := func(value any) (any, error) {
		seen := map[string]bool{}
		for {
			if !valid(value) {
				return nil, fmt.Errorf("invalid color value: %v", value)
			}
			s, ok := value.(string)
			if !ok || s == "" {
				return value, nil
			}
			if strings.HasPrefix(s, "#") {
				if !themeHex.MatchString(s) {
					return nil, fmt.Errorf("invalid hex color: %s", s)
				}
				return s, nil
			}
			if seen[s] {
				return nil, fmt.Errorf("circular variable reference: %s", s)
			}
			seen[s] = true
			value, ok = vars[s]
			if !ok {
				return nil, fmt.Errorf("variable reference not found: %s", s)
			}
		}
	}
	t := &Theme{Name: name, mode: mode, foreground: map[ThemeColor]string{}, background: map[ThemeBG]string{}, colors: map[string]string{}, exportColors: map[string]string{}}
	for _, group := range []struct {
		keys []string
		bg   bool
	}{{themeForeground, false}, {themeBackground, true}} {
		for _, key := range group.keys {
			value, err := resolve(colors[key])
			if err != nil {
				return nil, fmt.Errorf("%s: %w", key, err)
			}
			css := themeCSSColor(value)
			if css == "" {
				css = "#e5e5e7"
				if name == "light" {
					css = "#000000"
				}
			}
			t.colors[key] = css
			ansi := themeANSI(value, mode, group.bg)
			if group.bg {
				t.background[ThemeBG(key)] = ansi
			} else {
				t.foreground[ThemeColor(key)] = ansi
			}
		}
	}
	for _, key := range []string{"pageBg", "cardBg", "infoBg"} {
		if value, ok := exports[key]; ok {
			resolved, err := resolve(value)
			if err != nil {
				return nil, fmt.Errorf("export %s: %w", key, err)
			}
			if css := themeCSSColor(resolved); css != "" {
				t.exportColors[key] = css
			}
		}
	}
	return t, nil
}

func (t *Theme) ResolvedColors() map[string]string {
	if t == nil {
		return nil
	}
	return maps.Clone(t.colors)
}
func (t *Theme) ExportColors() map[string]string {
	if t == nil {
		return nil
	}
	return maps.Clone(t.exportColors)
}

func themeCSSColor(value any) string {
	if s, ok := value.(string); ok {
		return s
	}
	n := int(value.(float64))
	if n < 16 {
		return strings.Fields("#000000 #800000 #008000 #808000 #000080 #800080 #008080 #c0c0c0 #808080 #ff0000 #00ff00 #ffff00 #0000ff #ff00ff #00ffff #ffffff")[n]
	}
	if n >= 232 {
		v := 8 + (n-232)*10
		return fmt.Sprintf("#%02x%02x%02x", v, v, v)
	}
	n -= 16
	cube := []int{0, 95, 135, 175, 215, 255}
	return fmt.Sprintf("#%02x%02x%02x", cube[n/36], cube[n/6%6], cube[n%6])
}

func themeRGB(hex string) (int, int, int) {
	n, _ := strconv.ParseUint(hex[1:], 16, 24)
	return int(n >> 16), int(n >> 8 & 255), int(n & 255)
}

func themeANSI(value any, mode ColorMode, bg bool) string {
	prefix, reset := 38, 39
	if bg {
		prefix, reset = 48, 49
	}
	if value == "" {
		return fmt.Sprintf("\x1b[%dm", reset)
	}
	if n, ok := value.(float64); ok {
		return fmt.Sprintf("\x1b[%d;5;%dm", prefix, int(n))
	}
	r, g, b := themeRGB(value.(string))
	if mode == ColorModeTrueColor {
		return fmt.Sprintf("\x1b[%d;2;%d;%d;%dm", prefix, r, g, b)
	}
	cube := []int{0, 95, 135, 175, 215, 255}
	nearest := func(v int, values []int) int {
		best := 0
		for i, n := range values {
			if math.Abs(float64(v-n)) < math.Abs(float64(v-values[best])) {
				best = i
			}
		}
		return best
	}
	ri, gi, bi := nearest(r, cube), nearest(g, cube), nearest(b, cube)
	index := 16 + 36*ri + 6*gi + bi
	grays := make([]int, 24)
	for i := range grays {
		grays[i] = 8 + 10*i
	}
	gray := nearest(int(math.Round(.299*float64(r)+.587*float64(g)+.114*float64(b))), grays)
	distance := func(rr, gg, bb int) float64 {
		return .299*float64((r-rr)*(r-rr)) + .587*float64((g-gg)*(g-gg)) + .114*float64((b-bb)*(b-bb))
	}
	if max(r, g, b)-min(r, g, b) < 10 && distance(grays[gray], grays[gray], grays[gray]) < distance(cube[ri], cube[gi], cube[bi]) {
		index = 232 + gray
	}
	return fmt.Sprintf("\x1b[%d;5;%dm", prefix, index)
}

// SelectTheme selects a loaded theme before the built-in fallback. An empty name uses dark.
func SelectTheme(name string, loaded ThemeLoadResult) (*Theme, error) {
	if name == "" {
		name = "dark"
	}
	for _, theme := range loaded.Themes {
		if theme != nil && theme.Name == name {
			return theme, nil
		}
	}
	return LoadBuiltinTheme(name)
}

func detectThemeColorMode() ColorMode {
	color, term, program := strings.ToLower(os.Getenv("COLORTERM")), strings.ToLower(os.Getenv("TERM")), strings.ToLower(os.Getenv("TERM_PROGRAM"))
	hinted := color == "truecolor" || color == "24bit"
	if os.Getenv("TMUX") != "" || strings.HasPrefix(term, "tmux") || strings.HasPrefix(term, "screen") {
		if hinted {
			return ColorModeTrueColor
		}
		return ColorMode256
	}
	for _, key := range []string{"KITTY_WINDOW_ID", "GHOSTTY_RESOURCES_DIR", "WEZTERM_PANE", "WARP_SESSION_ID", "WARP_TERMINAL_SESSION_UUID", "ITERM_SESSION_ID", "WT_SESSION"} {
		hinted = hinted || os.Getenv(key) != ""
	}
	for _, name := range []string{"kitty", "ghostty", "wezterm", "warpterminal", "iterm.app", "vscode", "alacritty"} {
		hinted = hinted || program == name
	}
	if hinted || strings.Contains(term, "ghostty") || strings.EqualFold(os.Getenv("TERMINAL_EMULATOR"), "jetbrains-jediterm") || runtime.GOOS == "windows" {
		return ColorModeTrueColor
	}
	return ColorMode256
}

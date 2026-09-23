package codingagent

import (
	"context"
	"crypto/sha256"
	"fmt"
	"github.com/fsnotify/fsnotify"
	"github.com/nankedr/pig/tui"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ThemeController owns one session's theme; Current returns an immutable snapshot.
// Watch blocks until cancellation and owns no resources after returning.
type ThemeController struct {
	mu            sync.Mutex
	settings      *SettingsManager
	loaded        ThemeLoadResult
	current       *Theme
	terminal      tui.TerminalColorScheme
	auto, preview bool
	fingerprint   string
	watchChanged  chan struct{}
}

func NewThemeController(settings *SettingsManager, loaded ThemeLoadResult) *ThemeController {
	scheme, _ := DetectTerminalTheme(os.Getenv("COLORFGBG"))
	theme, _ := LoadBuiltinTheme(string(scheme))
	return &ThemeController{settings: settings, loaded: ThemeLoadResult{Themes: slices.Clone(loaded.Themes)}, current: theme, terminal: scheme, watchChanged: make(chan struct{})}
}
func (c *ThemeController) Current() *Theme { c.mu.Lock(); defer c.mu.Unlock(); return c.current }
func (c *ThemeController) ReplaceResources(settings *SettingsManager, loaded ThemeLoadResult) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.settings, c.loaded = settings, ThemeLoadResult{Themes: slices.Clone(loaded.Themes)}
	c.preview = false
	return c.applySettings()
}
func resolveThemeSetting(setting string, scheme tui.TerminalColorScheme) (string, bool) {
	parts := strings.Split(setting, "/")
	if len(parts) == 2 && strings.TrimSpace(parts[0]) != "" && strings.TrimSpace(parts[1]) != "" {
		if scheme == tui.TerminalColorSchemeLight {
			return strings.TrimSpace(parts[0]), true
		}
		return strings.TrimSpace(parts[1]), true
	}
	return setting, false
}
func (c *ThemeController) ApplySettings() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.preview = false
	return c.applySettings()
}
func (c *ThemeController) applySettings() error {
	setting, err := c.settings.GetThemeSetting()
	if err != nil {
		return err
	}
	name, auto := resolveThemeSetting(setting, c.terminal)
	c.auto = auto
	if name == "" {
		name = string(c.terminal)
	}
	return c.load(name)
}
func (c *ThemeController) load(name string) error {
	oldPath := c.current.SourcePath
	defer func() {
		if c.current.SourcePath != oldPath {
			close(c.watchChanged)
			c.watchChanged = make(chan struct{})
		}
	}()
	theme, err := SelectTheme(name, c.loaded)
	fingerprint := ""
	if err == nil && theme.SourcePath != "" {
		var data []byte
		data, err = readThemeFile(theme.SourcePath)
		if err == nil {
			var loaded *Theme
			loaded, err = parseTheme(data, []ColorMode{theme.GetColorMode()})
			if err == nil {
				loaded.SourcePath, loaded.SourceInfo = theme.SourcePath, theme.SourceInfo
				theme = loaded
				fingerprint = fmt.Sprintf("%x", sha256.Sum256(data))
			}
		}
	}
	if err != nil {
		c.current, _ = LoadBuiltinTheme("dark")
		c.fingerprint = ""
		return fmt.Errorf("Failed to load theme %q: %w; fell back to dark theme", name, err)
	}
	c.current, c.fingerprint = theme, fingerprint
	return nil
}
func (c *ThemeController) Preview(setting string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.preview = true
	name, _ := resolveThemeSetting(setting, c.terminal)
	return c.load(name)
}
func (c *ThemeController) SetTheme(setting string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	name, auto := resolveThemeSetting(setting, c.terminal)
	if err := c.load(name); err != nil {
		return err
	}
	if err := c.settings.SetTheme(setting); err != nil {
		_ = c.applySettings()
		return err
	}
	c.auto, c.preview = auto, false
	return nil
}
func (c *ThemeController) SetTerminalTheme(scheme tui.TerminalColorScheme) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if scheme != tui.TerminalColorSchemeLight && scheme != tui.TerminalColorSchemeDark {
		return fmt.Errorf("invalid terminal color scheme: %s", scheme)
	}
	c.terminal = scheme
	if c.auto && !c.preview {
		return c.applySettings()
	}
	return nil
}
func (c *ThemeController) Refresh() (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	path := c.current.SourcePath
	if path == "" {
		return false, nil
	}
	data, err := readThemeFile(path)
	fingerprint := fmt.Sprintf("%x", sha256.Sum256(data))
	if err != nil {
		fingerprint = err.Error()
	}
	if fingerprint == c.fingerprint {
		return false, nil
	}
	c.fingerprint = fingerprint
	var theme *Theme
	if err == nil {
		theme, err = parseTheme(data, []ColorMode{c.current.GetColorMode()})
	}
	if err != nil {
		return false, fmt.Errorf("Theme reload failed; keeping last valid theme: %w", err)
	}
	theme.SourcePath = path
	theme.SourceInfo = c.current.SourceInfo
	for i, t := range c.loaded.Themes {
		if t != nil && t.SourcePath == path {
			c.loaded.Themes[i] = theme
		}
	}
	c.current = theme
	return true, nil
}
func (c *ThemeController) Watch(ctx context.Context, changed func(error)) {
	notify := func(err error) {
		if changed != nil {
			changed(err)
		}
	}
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		notify(err)
		return
	}
	defer watcher.Close()
	timer := time.NewTimer(time.Hour)
	timer.Stop()
	defer timer.Stop()
	var pending <-chan time.Time
	var path, directory string
	var rebound <-chan struct{}
	bind := func() {
		timer.Stop()
		pending = nil
		c.mu.Lock()
		path, rebound = c.current.SourcePath, c.watchChanged
		c.mu.Unlock()
		next := ""
		if path != "" {
			next = filepath.Dir(path)
		}
		if next != directory {
			if directory != "" {
				_ = watcher.Remove(directory)
			}
			directory = next
			if directory != "" {
				if err := watcher.Add(directory); err != nil {
					notify(err)
				}
			}
		}
		if path != "" {
			timer.Reset(100 * time.Millisecond)
			pending = timer.C
		}
	}
	bind()
	for {
		select {
		case <-ctx.Done():
			return
		case <-rebound:
			bind()
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if event.Name == path {
				timer.Reset(100 * time.Millisecond)
				pending = timer.C
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			notify(err)
		case <-pending:
			pending = nil
			if ok, err := c.Refresh(); ok || err != nil {
				notify(err)
			}
		}
	}
}
func ThemeForRGB(rgb tui.RGBColor) tui.TerminalColorScheme {
	linear := func(n int) float64 {
		v := float64(n) / 255
		if v <= .03928 {
			return v / 12.92
		}
		return math.Pow((v+.055)/1.055, 2.4)
	}
	if .2126*linear(rgb.R)+.7152*linear(rgb.G)+.0722*linear(rgb.B) >= .5 {
		return tui.TerminalColorSchemeLight
	}
	return tui.TerminalColorSchemeDark
}

// DetectTerminalTheme returns the COLORFGBG fallback and whether it is a reliable hint.
func DetectTerminalTheme(value string) (tui.TerminalColorScheme, bool) {
	parts := strings.Split(value, ";")
	for i := len(parts) - 1; i >= 0; i-- {
		s := strings.TrimSpace(parts[i])
		end := 0
		if strings.HasPrefix(s, "+") || strings.HasPrefix(s, "-") {
			end++
		}
		for end < len(s) && s[end] >= '0' && s[end] <= '9' {
			end++
		}
		n, err := strconv.Atoi(s[:end])
		if err == nil && n >= 0 && n <= 255 {
			r, g, b := themeRGB(themeCSSColor(float64(n)))
			return ThemeForRGB(tui.RGBColor{R: r, G: g, B: b}), true
		}
	}
	return tui.TerminalColorSchemeDark, false
}

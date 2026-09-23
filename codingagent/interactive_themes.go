package codingagent

import (
	"context"
	"github.com/nankedr/pig/tui"
	"os"
	"time"
)

func (t *Theme) MarkdownTheme() tui.MarkdownTheme {
	fg := func(color ThemeColor) tui.TextStyleFunc { return func(s string) string { return t.FG(color, s) } }
	return tui.MarkdownTheme{Heading: fg("mdHeading"), Link: fg("mdLink"), LinkURL: fg("mdLinkUrl"), Code: fg("mdCode"), CodeBlock: fg("mdCodeBlock"), CodeBlockBorder: fg("mdCodeBlockBorder"), Quote: fg("mdQuote"), QuoteBorder: fg("mdQuoteBorder"), HR: fg("mdHr"), ListBullet: fg("mdListBullet"), Bold: t.Bold, Italic: t.Italic, Strikethrough: t.Strikethrough, Underline: t.Underline, HighlightCode: t.highlightCode}
}
func (m *InteractiveMode) initThemes(ctx context.Context) error {
	s := m.runtime.Session()
	loaded, err := s.ResourceLoader().GetThemes()
	if err != nil {
		return err
	}
	m.themes = NewThemeController(s.SettingsManager(), loaded)
	_ = m.themes.ApplySettings()
	m.colorReports = make(chan string, 32)
	m.themeQueries = make(chan string, 32)
	m.themeContext = ctx
	m.ui.SetTerminalColorHandler(func(data string) {
		select {
		case m.themeQueries <- data:
		default:
		}
		select {
		case m.colorReports <- data:
		default:
		}
	})
	m.ui.SetStyles(func(s string) string { return m.themes.Current().FG("text", s) }, func(s string) string { return m.themes.Current().FG("accent", s) })
	return nil
}
func (m *InteractiveMode) reloadThemes() error {
	s := m.runtime.Session()
	loaded, err := s.ResourceLoader().GetThemes()
	if err != nil {
		return err
	}
	if err = m.themes.ReplaceResources(s.SettingsManager(), loaded); err != nil {
		_ = m.ShowWarning(err.Error())
	}
	return m.queryTheme(m.themeContext)
}
func (m *InteractiveMode) themeNotifications() error {
	m.themes.mu.Lock()
	auto := m.themes.auto
	m.themes.mu.Unlock()
	seq := "\x1b[?2031l"
	if auto {
		seq = "\x1b[?2031h"
	}
	return m.ui.Terminal().Write(seq)
}
func (m *InteractiveMode) queryTheme(ctx context.Context) error {
	setting, err := m.runtime.Session().SettingsManager().GetThemeSetting()
	if err != nil {
		return err
	}
	_, auto := resolveThemeSetting(setting, tui.TerminalColorSchemeDark)
	if setting == "" || auto {
		query := "\x1b]11;?\x07"
		if auto {
			query += "\x1b[?996n"
		}
		for len(m.themeQueries) > 0 {
			<-m.themeQueries
		}
		if err = m.ui.Terminal().Write(query); err != nil {
			return err
		}
		scheme, high := DetectTerminalTheme(os.Getenv("COLORFGBG"))
		var report tui.TerminalColorScheme
		timer := time.NewTimer(100 * time.Millisecond)
	detect:
		for {
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
				break detect
			case data := <-m.themeQueries:
				if rgb, ok, _ := tui.ParseOSC11BackgroundColor(data); ok {
					scheme, high = ThemeForRGB(rgb), true
				}
				if v, ok, _ := tui.ParseTerminalColorSchemeReport(data); ok {
					report = v
				}
			}
		}
		if auto && report != "" {
			scheme = report
		}
		_ = m.themes.SetTerminalTheme(scheme)
		if setting == "" && high {
			err = m.themes.SetTheme(string(scheme))
		} else {
			err = m.themes.ApplySettings()
		}
	} else {
		err = m.themes.ApplySettings()
	}
	if err != nil {
		_ = m.ShowError(err.Error())
	}
	if err = m.themeNotifications(); err != nil {
		return err
	}
	return m.ui.Refresh()
}
func (m *InteractiveMode) detectTheme(ctx context.Context) error {
	if err := m.queryTheme(ctx); err != nil {
		return err
	}
	watchCtx, cancel := context.WithCancel(ctx)
	m.themeCancel = cancel
	m.themeDone = make(chan struct{})
	go func() {
		defer close(m.themeDone)
		watchDone := make(chan struct{})
		go func() {
			defer close(watchDone)
			m.themes.Watch(watchCtx, func(err error) {
				if err != nil {
					_ = m.ShowWarning(err.Error())
				}
				_ = m.ui.RequestRender()
			})
		}()
		defer func() { cancel(); <-watchDone }()
		for {
			select {
			case <-watchCtx.Done():
				return
			case data := <-m.colorReports:
				if scheme, ok, _ := tui.ParseTerminalColorSchemeReport(data); ok {
					if err := m.themes.SetTerminalTheme(scheme); err != nil {
						_ = m.ShowError(err.Error())
					}
					_ = m.ui.RequestRender()
				}
			}
		}
	}()
	return m.ui.Refresh()
}
func (m *InteractiveMode) selectTheme(ctx context.Context) error {
	loaded, err := m.runtime.Session().ResourceLoader().GetThemes()
	if err != nil {
		return err
	}
	setting, err := m.runtime.Session().SettingsManager().GetThemeSetting()
	if err != nil {
		return err
	}
	m.themes.mu.Lock()
	scheme := m.themes.terminal
	m.themes.mu.Unlock()
	done, finish := selectorSignal()
	selected := ""
	var previewErr error
	menu := NewThemeSettingsComponent(setting, scheme, loaded, SettingsCallbacks{
		OnThemePreview: func(value string) { previewErr = m.themes.Preview(value) },
		OnThemeChange:  func(value string) { selected = value; finish() }, OnCancel: finish,
	}, m.themes.Current)
	menu.SetKeybindings(&m.options.Keybindings.KeybindingsManager)
	err = m.waitSelector(ctx, &themeSettingsFrame{body: menu, theme: m.themes.Current, diagnostic: func() error { return previewErr }}, done)
	if err == nil && selected != "" {
		err = m.themes.SetTheme(selected)
	}
	if restoreErr := m.queryTheme(ctx); err == nil {
		err = restoreErr
	}
	return err
}

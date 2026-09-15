package codingagent

import (
	"context"
	"fmt"
	"math"
	"strings"
)

type HTMLExportOptions struct {
	OutputPath string
	Theme      *Theme
}

func loadExportTheme(ctx context.Context, settings *SettingsManager, loader ResourceLoader, options DefaultResourceLoaderOptions, trust ProjectTrustDecision) (*Theme, []ResourceDiagnostic, error) {
	if settings == nil {
		cwd, err := resolveSessionPath(options.CWD)
		if err != nil {
			return nil, nil, err
		}
		dir := options.AgentDir
		if dir == "" {
			dir, err = GetAgentDir()
			if err != nil {
				return nil, nil, err
			}
		}
		settings, err = NewSettingsManager(cwd, &dir)
		if err != nil {
			return nil, nil, err
		}
		if err = prepareHeadlessProjectSettings(ctx, cwd, dir, settings, trust); err != nil {
			return nil, nil, err
		}
		options.CWD, options.AgentDir = cwd, dir
	}
	if loader == nil {
		options.SettingsManager = settings
		options.NoContextFiles = true
		options.NoSkills = true
		options.NoPromptTemplates = true
		options.NoExtensions = true
		local, err := NewDefaultResourceLoader(options)
		if err != nil {
			return nil, nil, err
		}
		if err = local.Reload(ctx); err != nil {
			return nil, nil, err
		}
		loader = local
	}
	loaded, err := loader.GetThemes()
	if err != nil {
		return nil, nil, err
	}
	name, err := settings.GetTheme()
	if err != nil {
		return nil, loaded.Diagnostics, err
	}
	theme, err := SelectTheme(name, loaded)
	return theme, loaded.Diagnostics, err
}

func themeExportCSS(t *Theme) (string, error) {
	if t == nil {
		var err error
		t, err = LoadBuiltinTheme("dark")
		if err != nil {
			return "", err
		}
	}
	if len(t.colors) == 0 {
		return "", fmt.Errorf("theme has no resolved colors")
	}
	var css strings.Builder
	css.WriteString(":root {\n")
	for _, keys := range [][]string{themeForeground, themeBackground} {
		for _, key := range keys {
			fmt.Fprintf(&css, "--%s: %s;\n", key, t.colors[key])
		}
	}
	r, g, b := themeRGB(t.colors["userMessageBg"])
	linear := func(c int) float64 {
		v := float64(c) / 255
		if v <= .03928 {
			return v / 12.92
		}
		return math.Pow((v+.055)/1.055, 2.4)
	}
	light := .2126*linear(r)+.7152*linear(g)+.0722*linear(b) > .5
	adjust := func(f float64) string {
		return fmt.Sprintf("rgb(%d, %d, %d)", min(255, int(math.Round(float64(r)*f))), min(255, int(math.Round(float64(g)*f))), min(255, int(math.Round(float64(b)*f))))
	}
	page, card, info := adjust(.7), adjust(.85), fmt.Sprintf("rgb(%d, %d, %d)", min(255, r+20), min(255, g+15), b)
	if light {
		page, card, info = adjust(.96), t.colors["userMessageBg"], fmt.Sprintf("rgb(%d, %d, %d)", min(255, r+10), min(255, g+5), max(0, b-20))
	}
	for _, value := range []struct {
		key, alias string
		color      *string
	}{{"pageBg", "body-bg", &page}, {"cardBg", "container-bg", &card}, {"infoBg", "info-bg", &info}} {
		if custom := t.exportColors[value.key]; custom != "" {
			*value.color = custom
		}
		fmt.Fprintf(&css, "--export%s: %s;\n--%s: %s;\n", strings.ToUpper(value.key[:1])+value.key[1:], *value.color, value.alias, *value.color)
	}
	css.WriteString("}\n")
	return css.String(), nil
}

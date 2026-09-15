package codingagent_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nankedr/pig/codingagent"
)

func TestThemesTrustReloadAndOwnership(t *testing.T) {
	data, err := os.ReadFile("themes/dark.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name, settings string
		trusted        bool
	}{{"ask", "{}", false}, {"never", `{"defaultProjectTrust":"never"}`, false}, {"always", `{"defaultProjectTrust":"always"}`, true}} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			cwd, dir := t.TempDir(), t.TempDir()
			t.Setenv("HOME", t.TempDir())
			global := strings.Replace(string(data), `"name": "dark"`, `"name": "same"`, 1)
			project := strings.Replace(global, `"accent": "#8abeb7"`, `"accent": "#123456"`, 1)
			context100Write(t, filepath.Join(dir, "settings.json"), test.settings)
			context100Write(t, filepath.Join(dir, "themes/theme.json"), global)
			context100Write(t, filepath.Join(cwd, ".pig/themes/theme.json"), project)
			loader, err := codingagent.NewDefaultResourceLoader(codingagent.DefaultResourceLoaderOptions{CWD: cwd, AgentDir: dir})
			if err != nil {
				t.Fatal(err)
			}
			if err = loader.Reload(ctx); err != nil {
				t.Fatal(err)
			}
			loaded, err := loader.GetThemes()
			if err != nil || len(loaded.Themes) != 1 {
				t.Fatal(loaded, err)
			}
			want := "#8abeb7"
			if test.trusted {
				want = "#123456"
			}
			if loaded.Themes[0].ResolvedColors()["accent"] != want {
				t.Fatal("first-load trust")
			}
			old := loaded.Themes[0]
			old.Name = "MUTATED"
			old.SourceInfo.Path = "MUTATED"
			old.ResolvedColors()["accent"] = "MUTATED"
			if len(loaded.Diagnostics) > 0 {
				loaded.Diagnostics[0].Collision.WinnerPath = "MUTATED"
			}
			fresh, _ := loader.GetThemes()
			if fresh.Themes[0].Name != "same" || fresh.Themes[0].SourceInfo.Path == "MUTATED" || fresh.Themes[0].ResolvedColors()["accent"] != want {
				t.Fatal("aliased snapshot")
			}
			if test.trusted && fresh.Diagnostics[0].Collision.WinnerPath == "MUTATED" {
				t.Fatal("aliased diagnostics")
			}
			context100Write(t, filepath.Join(dir, "themes/theme.json"), "{}")
			context100Write(t, filepath.Join(cwd, ".pig/themes/theme.json"), "{}")
			canceled, cancel := context.WithCancel(ctx)
			cancel()
			if err = loader.Reload(canceled); !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
			fresh, _ = loader.GetThemes()
			if len(fresh.Themes) != 1 {
				t.Fatal("canceled reload published")
			}
			if err = loader.Reload(ctx); err != nil {
				t.Fatal(err)
			}
			fresh, _ = loader.GetThemes()
			if len(fresh.Themes) != 0 || len(fresh.Diagnostics) == 0 {
				t.Fatal("invalid reload not diagnosed")
			}
		})
	}
}

func TestThemesRejectCSSInjection(t *testing.T) {
	data, err := os.ReadFile("themes/dark.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"accent", "pageBg"} {
		for _, value := range []any{"#ffffff;</style><script>alert(1)</script>", "#zzffff", -1, 1.5, nil} {
			var doc map[string]any
			json.Unmarshal(data, &doc)
			section := "colors"
			if field == "pageBg" {
				section = "export"
			}
			doc[section].(map[string]any)[field] = value
			b, _ := json.Marshal(doc)
			path := filepath.Join(t.TempDir(), "bad.json")
			context100Write(t, path, string(b))
			if _, err = codingagent.LoadThemeFromPath(path); err == nil {
				t.Fatalf("accepted %s %v", field, value)
			}
		}
	}
	if _, err = codingagent.LoadThemeFromPath(t.TempDir()); err == nil {
		t.Fatal("accepted directory")
	}
	if _, err = codingagent.LoadBuiltinTheme("../dark"); err == nil {
		t.Fatal("accepted builtin path")
	}
}

func TestThemesSessionExport(t *testing.T) {
	dir := t.TempDir()
	data, err := os.ReadFile("../parity/export-html/session.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(dir, "session.jsonl")
	context100Write(t, source, string(data))
	manager, err := codingagent.OpenSessionManager(source, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	settings, err := codingagent.NewInMemorySettingsManager(codingagent.Settings{})
	if err != nil {
		t.Fatal(err)
	}
	if err = settings.SetTheme("light"); err != nil {
		t.Fatal(err)
	}
	loader, err := codingagent.NewDefaultResourceLoader(codingagent.DefaultResourceLoaderOptions{CWD: dir, AgentDir: dir, SettingsManager: settings})
	if err != nil {
		t.Fatal(err)
	}
	if err = loader.Reload(context.Background()); err != nil {
		t.Fatal(err)
	}
	session := codingagent.NewAgentSession(codingagent.AgentSessionConfig{SessionManager: manager, SettingsManager: settings, ResourceLoader: loader})
	defer session.Dispose()
	output, err := session.ExportToHTML(context.Background(), filepath.Join(dir, "light.html"))
	if err != nil {
		t.Fatal(err)
	}
	html, _ := os.ReadFile(output)
	theme, _ := codingagent.LoadBuiltinTheme("light")
	if !strings.Contains(string(html), "--accent: "+theme.ResolvedColors()["accent"]+";") {
		t.Fatal("session ignored theme setting")
	}
}

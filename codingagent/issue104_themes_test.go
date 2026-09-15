package codingagent_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/codingagent"
	"github.com/nankedr/pig/internal/baseline"
	"github.com/nankedr/pig/internal/parity"
)

func TestThemesColorsSDKParity(t *testing.T) {
	lock, _, err := baseline.Load("../parity/baseline")
	if err != nil {
		t.Fatal(err)
	}
	pinned := parity.Baseline{ID: lock.BaselineID, Commit: lock.Upstream.Commit, Repository: lock.Upstream.Repository}
	fixture, err := parity.LoadFixture("../parity/oracle/fixtures/themes.json", pinned)
	if err != nil {
		t.Fatal(err)
	}
	var input struct {
		Documents []json.RawMessage
		Invalid   []struct{ Doc json.RawMessage }
	}
	if err = json.Unmarshal(fixture.Case.Input, &input); err != nil {
		t.Fatal(err)
	}
	var expected struct {
		Colors []struct {
			Name  string
			Modes []struct {
				Mode             codingagent.ColorMode
				FG, BG           map[string]string
				Thinking, Styles []string
			}
			CSS, Export map[string]string
		}
		Errors []bool
	}
	b, _ := json.Marshal(fixture.Observation.Outcome)
	if err = json.Unmarshal(b, &expected); err != nil {
		t.Fatal(err)
	}
	for i, doc := range input.Documents {
		t.Run(expected.Colors[i].Name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "theme.json")
			context100Write(t, path, string(doc))
			for _, want := range expected.Colors[i].Modes {
				theme, err := codingagent.LoadThemeFromPath(path, want.Mode)
				if err != nil {
					t.Fatal(err)
				}
				if theme.GetColorMode() != want.Mode {
					t.Fatal("color mode")
				}
				for k, v := range want.FG {
					if got := theme.FG(codingagent.ThemeColor(k), "X"); got != v {
						t.Fatalf("fg %s: %q != %q", k, got, v)
					}
				}
				for k, v := range want.BG {
					if got := theme.BG(codingagent.ThemeBG(k), "X"); got != v {
						t.Fatalf("bg %s: %q != %q", k, got, v)
					}
				}
				for j, level := range []agent.ThinkingLevel{"off", "minimal", "low", "medium", "high", "xhigh", "max", "other"} {
					style, err := theme.GetThinkingBorderColor(level)
					if err != nil || style("X") != want.Thinking[j] {
						t.Fatalf("thinking %s: %v", level, err)
					}
				}
				got := []string{theme.Bold("X"), theme.Italic("X"), theme.Underline("X"), theme.Inverse("X"), theme.Strikethrough("X")}
				a, _ := json.Marshal(got)
				b, _ := json.Marshal(want.Styles)
				if string(a) != string(b) {
					t.Fatalf("styles %s != %s", a, b)
				}
				for k, v := range expected.Colors[i].CSS {
					if theme.ResolvedColors()[k] != v {
						t.Fatalf("css %s: %s != %s", k, theme.ResolvedColors()[k], v)
					}
				}
				a, _ = json.Marshal(theme.ExportColors())
				b, _ = json.Marshal(expected.Colors[i].Export)
				if string(a) != string(b) {
					t.Fatalf("export %s != %s", a, b)
				}
			}
		})
	}
	for i, c := range input.Invalid {
		path := filepath.Join(t.TempDir(), "bad.json")
		context100Write(t, path, string(c.Doc))
		_, err := codingagent.LoadThemeFromPath(path)
		if (err != nil) != expected.Errors[i] {
			t.Fatalf("invalid %d: %v", i, err)
		}
	}
}

func TestThemesResourceSDKParity(t *testing.T) {
	lock, _, err := baseline.Load("../parity/baseline")
	if err != nil {
		t.Fatal(err)
	}
	pinned := parity.Baseline{ID: lock.BaselineID, Commit: lock.Upstream.Commit, Repository: lock.Upstream.Repository}
	fixture, err := parity.LoadFixture("../parity/oracle/fixtures/themes.json", pinned)
	if err != nil {
		t.Fatal(err)
	}
	var input struct {
		Scenarios []struct {
			Name                   string
			Files                  map[string]string
			Trusted, Disabled      bool
			Paths, Global, Project []string
		}
	}
	json.Unmarshal(fixture.Case.Input, &input)
	var expected struct{ Resources []map[string]any }
	b, _ := json.Marshal(fixture.Observation.Outcome)
	json.Unmarshal(b, &expected)
	for i, scenario := range input.Scenarios {
		t.Run(scenario.Name, func(t *testing.T) {
			dir := t.TempDir()
			cwd, agentDir := filepath.Join(dir, "repo"), filepath.Join(dir, "agent")
			for p, data := range scenario.Files {
				context100Write(t, strings.ReplaceAll(filepath.Join(dir, p), "/.pi/", "/.pig/"), data)
			}
			global, _ := json.Marshal(map[string]any{"themes": scenario.Global})
			project, _ := json.Marshal(map[string]any{"themes": scenario.Project})
			context100Write(t, filepath.Join(agentDir, "settings.json"), string(global))
			context100Write(t, filepath.Join(cwd, ".pig/settings.json"), string(project))
			settings, err := codingagent.NewSettingsManager(cwd, &agentDir)
			if err != nil {
				t.Fatal(err)
			}
			if err = settings.SetProjectTrusted(scenario.Trusted); err != nil {
				t.Fatal(err)
			}
			paths := []string{}
			for _, p := range scenario.Paths {
				paths = append(paths, strings.ReplaceAll(p, ".pi/", ".pig/"))
			}
			loader, err := codingagent.NewDefaultResourceLoader(codingagent.DefaultResourceLoaderOptions{CWD: cwd, AgentDir: agentDir, SettingsManager: settings, NoThemes: scenario.Disabled, AdditionalThemePaths: paths})
			if err != nil {
				t.Fatal(err)
			}
			if err = loader.Reload(context.Background()); err != nil {
				t.Fatal(err)
			}
			loaded, err := loader.GetThemes()
			if err != nil {
				t.Fatal(err)
			}
			themes, diagnostics := []map[string]any{}, []map[string]any{}
			for _, theme := range loaded.Themes {
				s := theme.SourceInfo
				source := map[string]any{"path": s.Path, "source": s.Source, "scope": s.Scope, "origin": s.Origin}
				if s.BaseDir != "" {
					source["baseDir"] = s.BaseDir
				}
				themes = append(themes, map[string]any{"name": theme.Name, "path": theme.SourcePath, "source": source})
			}
			for _, d := range loaded.Diagnostics {
				v := map[string]any{"type": d.Type, "message": d.Message, "path": d.Path}
				if c := d.Collision; c != nil {
					v["collision"] = map[string]any{"resourceType": c.ResourceType, "name": c.Name, "winnerPath": c.WinnerPath, "loserPath": c.LoserPath}
				}
				diagnostics = append(diagnostics, v)
			}
			got, _ := json.Marshal(map[string]any{"name": scenario.Name, "themes": themes, "diagnostics": diagnostics})
			normalized := strings.ReplaceAll(strings.ReplaceAll(string(got), dir, "$ROOT"), "/.pig", "/.pi")
			want, _ := json.Marshal(expected.Resources[i])
			if normalized != string(want) {
				t.Fatalf("got %s\nwant %s", normalized, want)
			}
		})
	}
}

func TestThemesSelectionAndHTML(t *testing.T) {
	root := issue32RepoRoot(t)
	ctx := context.Background()
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("HOME", dir)
	t.Setenv("PIG_CODING_AGENT_DIR", filepath.Join(dir, "agent"))
	fixture, err := os.ReadFile(filepath.Join(root, "parity/oracle/fixtures/themes.json"))
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Case struct {
			Input struct{ Documents []json.RawMessage }
		}
	}
	json.Unmarshal(fixture, &f)
	path := filepath.Join(dir, "custom.json")
	context100Write(t, path, string(f.Case.Input.Documents[2]))
	context100Write(t, filepath.Join(dir, "agent/settings.json"), `{"theme":"custom"}`)
	session, err := os.ReadFile(filepath.Join(root, "parity/export-html/session.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	source, output := filepath.Join(dir, "session.jsonl"), filepath.Join(dir, "out.html")
	context100Write(t, source, string(session))
	result, err := codingagent.RunCLI(ctx, codingagent.CLIInvocation{Arguments: []string{"--export", source, output, "--no-themes", "--theme", path}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Stdout != "Exported to: "+output+"\n" {
		t.Fatal(result)
	}
	html, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"--accent: #8abeb7;", "--body-bg: #123456;", "--container-bg: #262626;"} {
		if !strings.Contains(string(html), value) {
			t.Fatalf("missing %s", value)
		}
	}
	context100Write(t, filepath.Join(dir, "agent/settings.json"), `{"theme":"missing"}`)
	if _, err = codingagent.RunCLI(ctx, codingagent.CLIInvocation{Arguments: []string{"--export", source, output}}); err == nil {
		t.Fatal("missing selected theme accepted")
	}
}

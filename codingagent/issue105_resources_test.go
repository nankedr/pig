package codingagent_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
	"github.com/nankedr/pig/internal/baseline"
	"github.com/nankedr/pig/internal/parity"
)

func TestLocalResourcesSDKParity(t *testing.T) {
	lock, _, err := baseline.Load("../parity/baseline")
	if err != nil {
		t.Fatal(err)
	}
	fixture, err := parity.LoadFixture("../parity/oracle/fixtures/local-resources.json", parity.Baseline{ID: lock.BaselineID, Commit: lock.Upstream.Commit, Repository: lock.Upstream.Repository})
	if err != nil {
		t.Fatal(err)
	}
	var input struct {
		Scenarios []struct {
			Name, CWD              string
			Trusted, Disabled      bool
			Files, Links           map[string]string
			Global, Project, Paths map[string][]string
		}
	}
	if err = json.Unmarshal(fixture.Case.Input, &input); err != nil {
		t.Fatal(err)
	}
	var expected []map[string]any
	if err = json.Unmarshal(fixture.Observation.Outcome, &expected); err != nil {
		t.Fatal(err)
	}
	for i, scenario := range input.Scenarios {
		t.Run(scenario.Name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("HOME", filepath.Join(dir, "home"))
			t.Setenv("COLORTERM", "truecolor")
			expand := func(s string) string { return strings.ReplaceAll(s, ".pi/", ".pig/") }
			for p, content := range scenario.Files {
				context100Write(t, expand(filepath.Join(dir, p)), content)
			}
			for p, target := range scenario.Links {
				if err := os.Symlink(expand(target), expand(filepath.Join(dir, p))); err != nil {
					t.Fatal(err)
				}
			}
			cwd := scenario.CWD
			if cwd == "" {
				cwd = "repo"
			}
			cwd, agentDir := filepath.Join(dir, cwd), filepath.Join(dir, "agent")
			global, _ := json.Marshal(scenario.Global)
			project, _ := json.Marshal(scenario.Project)
			context100Write(t, filepath.Join(agentDir, "settings.json"), string(global))
			context100Write(t, filepath.Join(cwd, ".pig/settings.json"), string(project))
			settings, err := codingagent.NewSettingsManager(cwd, &agentDir)
			if err != nil {
				t.Fatal(err)
			}
			if err = settings.SetProjectTrusted(scenario.Trusted); err != nil {
				t.Fatal(err)
			}
			paths := func(kind string) []string {
				out := []string{}
				for _, p := range scenario.Paths[kind] {
					out = append(out, expand(p))
				}
				return out
			}
			loader, err := codingagent.NewDefaultResourceLoader(codingagent.DefaultResourceLoaderOptions{CWD: cwd, AgentDir: agentDir, SettingsManager: settings, NoContextFiles: true, NoSkills: scenario.Disabled, NoPromptTemplates: scenario.Disabled, NoThemes: scenario.Disabled, AdditionalSkillPaths: paths("skills"), AdditionalPromptTemplatePaths: paths("prompts"), AdditionalThemePaths: paths("themes")})
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			if err = loader.Reload(ctx); err != nil {
				t.Fatal(err)
			}
			prompts, err := loader.GetPrompts()
			if err != nil {
				t.Fatal(err)
			}
			skills, err := loader.GetSkills()
			if err != nil {
				t.Fatal(err)
			}
			themes, err := loader.GetThemes()
			if err != nil {
				t.Fatal(err)
			}
			core, _ := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
			model, _ := core.GetModel()
			received := ""
			steps := []ai.FauxResponseStep{}
			for range prompts.Prompts {
				steps = append(steps, ai.FauxResponseFactory(func(input ai.Context, _ *ai.SimpleStreamOptions, _ *ai.FauxProviderState, _ ai.Model) (ai.AssistantMessage, error) {
					received = message85Text(input.Messages[len(input.Messages)-1])
					return ai.FauxAssistantMessage(ai.FauxAssistantText("RESOURCE_OK"))
				}))
			}
			core.SetResponses(steps)
			created, err := codingagent.CreateAgentSession(ctx, codingagent.CreateAgentSessionOptions{CWD: cwd, AgentDir: agentDir, Model: &model, StreamFunction: agent.StreamFunction(core.StreamSimple), SettingsManager: settings, ResourceLoader: loader, Tools: []string{"read"}})
			if err != nil {
				t.Fatal(err)
			}
			defer created.Session.Dispose()
			sessionPrompts, err := created.Session.PromptTemplates()
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(sessionPrompts, prompts.Prompts) {
				t.Fatal("session templates differ from resource query")
			}
			entries := func() []map[string]any { return []map[string]any{} }
			pp, ss, tt := entries(), entries(), entries()
			entry := func(name, path string, s codingagent.SourceInfo, effect string) map[string]any {
				source := map[string]any{"path": s.Path, "source": s.Source, "scope": s.Scope, "origin": s.Origin}
				if s.BaseDir != "" {
					source["baseDir"] = s.BaseDir
				}
				return map[string]any{"name": name, "path": path, "source": source, "effect": effect}
			}
			for _, p := range prompts.Prompts {
				if err := created.Session.Prompt(ctx, "/"+p.Name+" ARG"); err != nil {
					t.Fatal(err)
				}
				pp = append(pp, entry(p.Name, p.FilePath, p.SourceInfo, received))
			}
			for _, s := range skills.Skills {
				ss = append(ss, entry(s.Name, s.FilePath, s.SourceInfo, s.Description))
			}
			skillList := codingagent.FormatSkillsForPrompt(skills.Skills)
			if !strings.Contains(created.Session.SystemPrompt(), skillList) {
				t.Fatal("session omitted queried skills")
			}
			for _, theme := range themes.Themes {
				selected, err := codingagent.SelectTheme(theme.Name, themes)
				if err != nil {
					t.Fatal(err)
				}
				tt = append(tt, entry(theme.Name, theme.SourcePath, *theme.SourceInfo, selected.FG("accent", "X")))
			}
			diagnostics := entries()
			for _, d := range append(append(prompts.Diagnostics, skills.Diagnostics...), themes.Diagnostics...) {
				v := map[string]any{"type": d.Type, "message": d.Message, "path": d.Path}
				if c := d.Collision; c != nil {
					v["collision"] = map[string]any{"resourceType": c.ResourceType, "name": c.Name, "winnerPath": c.WinnerPath, "loserPath": c.LoserPath}
				}
				diagnostics = append(diagnostics, v)
			}
			raw, _ := json.Marshal(map[string]any{"name": scenario.Name, "prompts": pp, "skills": ss, "themes": tt, "skillList": skillList, "diagnostics": diagnostics})
			var got map[string]any
			if err := json.Unmarshal([]byte(strings.ReplaceAll(strings.ReplaceAll(string(raw), dir, "$ROOT"), "/.pig", "/.pi")), &got); err != nil {
				t.Fatal(err)
			}
			for key, want := range expected[i] {
				if !reflect.DeepEqual(got[key], want) {
					g, _ := json.Marshal(got[key])
					w, _ := json.Marshal(want)
					t.Errorf("%s:\nPig %s\n Pi %s", key, g, w)
				}
			}
		})
	}
}

func TestLocalResourceCollisionSources(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	t.Setenv("HOME", filepath.Join(dir, "home"))
	cwd, agentDir := filepath.Join(dir, "repo"), filepath.Join(dir, "agent")
	dark, err := os.ReadFile("themes/dark.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, base := range []string{agentDir, filepath.Join(cwd, ".pig"), filepath.Join(dir, "extra")} {
		context100Write(t, filepath.Join(base, "prompts/same.md"), "TEMPLATE")
		context100Write(t, filepath.Join(base, "skills/same/SKILL.md"), "---\nname: same\ndescription: Skill\n---\nBODY")
		context100Write(t, filepath.Join(base, "themes/same.json"), strings.Replace(string(dark), `"name": "dark"`, `"name": "same"`, 1))
	}
	settings, err := codingagent.NewSettingsManager(cwd, &agentDir)
	if err != nil {
		t.Fatal(err)
	}
	if err = settings.SetProjectTrusted(true); err != nil {
		t.Fatal(err)
	}
	loader, err := codingagent.NewDefaultResourceLoader(codingagent.DefaultResourceLoaderOptions{CWD: cwd, AgentDir: agentDir, SettingsManager: settings, AdditionalPromptTemplatePaths: []string{"../extra/prompts"}, AdditionalSkillPaths: []string{"../extra/skills"}, AdditionalThemePaths: []string{"../extra/themes"}})
	if err != nil {
		t.Fatal(err)
	}
	if err = loader.Reload(ctx); err != nil {
		t.Fatal(err)
	}
	queries := map[string]func() ([]codingagent.ResourceDiagnostic, error){
		"prompt": func() ([]codingagent.ResourceDiagnostic, error) { r, e := loader.GetPrompts(); return r.Diagnostics, e },
		"skill":  func() ([]codingagent.ResourceDiagnostic, error) { r, e := loader.GetSkills(); return r.Diagnostics, e },
		"theme":  func() ([]codingagent.ResourceDiagnostic, error) { r, e := loader.GetThemes(); return r.Diagnostics, e },
	}
	for kind, query := range queries {
		t.Run(kind, func(t *testing.T) {
			diagnostics, err := query()
			if err != nil || len(diagnostics) != 2 {
				t.Fatalf("%v %v", diagnostics, err)
			}
			for i, loser := range []string{"user:auto", "temporary:local"} {
				c := diagnostics[i].Collision
				if c == nil || c.WinnerSource == nil || c.LoserSource == nil {
					t.Fatalf("missing collision sources: %+v", c)
				}
				if *c.WinnerSource != "project:auto" || *c.LoserSource != loser {
					t.Fatalf("sources: %s / %s", *c.WinnerSource, *c.LoserSource)
				}
				*c.WinnerSource = "MUTATED"
				*c.LoserSource = "MUTATED"
			}
			fresh, err := query()
			if err != nil {
				t.Fatal(err)
			}
			if *fresh[0].Collision.WinnerSource != "project:auto" || *fresh[0].Collision.LoserSource != "user:auto" {
				t.Fatal("aliased diagnostic sources")
			}
		})
	}
	if err = settings.SetProjectTrusted(false); err != nil {
		t.Fatal(err)
	}
	if err = loader.Reload(ctx); err != nil {
		t.Fatal(err)
	}
	for kind, query := range queries {
		d, e := query()
		if e != nil || len(d) != 1 || d[0].Collision.WinnerSource == nil || *d[0].Collision.WinnerSource != "user:auto" {
			t.Fatalf("%s untrusted reload: %v %v", kind, d, e)
		}
	}
}

package codingagent_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/nankedr/pig/codingagent"
)

func TestSkillsTrustReloadAndOwnership(t *testing.T) {
	for _, test := range []struct {
		name, global string
		saved        *bool
		trusted      bool
	}{
		{name: "ask"}, {name: "never", global: `{"defaultProjectTrust":"never"}`}, {name: "always", global: `{"defaultProjectTrust":"always"}`, trusted: true}, {name: "saved-allow", saved: codingagent.ProjectTrustDecisionTrusted(), trusted: true}, {name: "saved-deny", global: `{"defaultProjectTrust":"always"}`, saved: codingagent.ProjectTrustDecisionUntrusted()},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			ctx := context.Background()
			cwd, dir := t.TempDir(), t.TempDir()
			context100Write(t, filepath.Join(cwd, ".pig/skills/review/SKILL.md"), "---\nname: review\ndescription: PROJECT\n---\nBODY")
			context100Write(t, filepath.Join(dir, "skills/review/SKILL.md"), "---\nname: review\ndescription: GLOBAL\n---\nBODY")
			if test.global != "" {
				context100Write(t, filepath.Join(dir, "settings.json"), test.global)
			}
			if test.saved != nil {
				if err := codingagent.NewProjectTrustStore(dir).Set(ctx, cwd, test.saved); err != nil {
					t.Fatal(err)
				}
			}
			models, _, _ := config87Runtime(t)
			model, _, _ := models.GetModel("deepseek", "deepseek-v4-flash")
			created, err := codingagent.CreateAgentSession(ctx, codingagent.CreateAgentSessionOptions{CWD: cwd, AgentDir: dir, Model: &model, ModelRuntime: models, NoTools: codingagent.NoToolsAll})
			if err != nil {
				t.Fatal(err)
			}
			s := created.Session
			defer s.Dispose()
			loader := s.ResourceLoader()
			loaded, err := loader.GetSkills()
			skills := loaded.Skills
			if err != nil || len(skills) != 1 {
				t.Fatalf("query: %v %v", skills, err)
			}
			want := "GLOBAL"
			if test.trusted {
				want = "PROJECT"
			}
			if skills[0].Description != want {
				t.Fatalf("first-load trust: %+v", skills)
			}
			skills[0].Description = "MUTATED"
			skills[0].SourceInfo.Path = "MUTATED"
			snapshot, _ := loader.GetSkills()
			if len(snapshot.Diagnostics) > 0 {
				snapshot.Diagnostics[0].Collision.WinnerPath = "MUTATED"
			}
			snapshot, _ = loader.GetSkills()
			if snapshot.Skills[0].Description != want || snapshot.Skills[0].SourceInfo.Path == "MUTATED" || len(snapshot.Diagnostics) > 0 && snapshot.Diagnostics[0].Collision.WinnerPath == "MUTATED" {
				t.Fatal("aliased snapshot")
			}
			context100Write(t, filepath.Join(dir, "skills/review/SKILL.md"), "---\nname: review\ndescription: RELOADED\n---\nBODY")
			settings := s.SettingsManager()
			if err := settings.SetProjectTrusted(false); err != nil {
				t.Fatal(err)
			}
			canceled, cancel := context.WithCancel(ctx)
			cancel()
			if err := loader.Reload(canceled); !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
			loaded, _ = loader.GetSkills()
			skills = loaded.Skills
			if skills[0].Description != want {
				t.Fatal("canceled reload changed skills")
			}
			if err := loader.Reload(ctx); err != nil {
				t.Fatal(err)
			}
			loaded, _ = loader.GetSkills()
			skills = loaded.Skills
			if len(skills) != 1 || skills[0].Description != "RELOADED" {
				t.Fatal(skills)
			}
		})
	}
}

func TestSkillsMissingHomeFailsBeforeRelativeDiscovery(t *testing.T) {
	t.Setenv("HOME", "")
	dir := t.TempDir()
	settings, err := codingagent.NewSettingsManager(dir, &dir)
	if err != nil {
		t.Fatal(err)
	}
	if err = settings.SetProjectTrusted(false); err != nil {
		t.Fatal(err)
	}
	loader, err := codingagent.NewDefaultResourceLoader(codingagent.DefaultResourceLoaderOptions{CWD: dir, AgentDir: dir, SettingsManager: settings, NoContextFiles: true})
	if err != nil {
		t.Fatal(err)
	}
	if err = loader.Reload(context.Background()); err == nil {
		t.Fatal("missing home must not resolve user skills relative to the process directory")
	}
}

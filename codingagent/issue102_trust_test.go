package codingagent_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/nankedr/pig/codingagent"
)

func TestPromptTemplatesTrustReloadAndOwnership(t *testing.T) {
	for _, test := range []struct {
		name, global string
		saved        *bool
		trusted      bool
	}{
		{name: "ask"}, {name: "never", global: `{"defaultProjectTrust":"never"}`}, {name: "always", global: `{"defaultProjectTrust":"always"}`, trusted: true}, {name: "saved-allow", saved: codingagent.ProjectTrustDecisionTrusted(), trusted: true}, {name: "saved-deny", global: `{"defaultProjectTrust":"always"}`, saved: codingagent.ProjectTrustDecisionUntrusted()},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			cwd, dir := t.TempDir(), t.TempDir()
			context100Write(t, filepath.Join(cwd, ".pig/prompts/review.md"), "PROJECT $1")
			context100Write(t, filepath.Join(dir, "prompts/review.md"), "GLOBAL $1")
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
			templates, err := s.PromptTemplates()
			if err != nil || len(templates) != 1 {
				t.Fatalf("query: %v %v", templates, err)
			}
			want := "GLOBAL $1"
			if test.trusted {
				want = "PROJECT $1"
			}
			if templates[0].Content != want {
				t.Fatalf("first-load trust: %+v", templates)
			}
			templates[0].Content = "MUTATED"
			templates[0].SourceInfo.Path = "MUTATED"
			snapshot, _ := loader.GetPrompts()
			if len(snapshot.Diagnostics) > 0 {
				snapshot.Diagnostics[0].Collision.WinnerPath = "MUTATED"
			}
			snapshot, _ = loader.GetPrompts()
			if snapshot.Prompts[0].Content != want || snapshot.Prompts[0].SourceInfo.Path == "MUTATED" || len(snapshot.Diagnostics) > 0 && snapshot.Diagnostics[0].Collision.WinnerPath == "MUTATED" {
				t.Fatal("aliased snapshot")
			}
			context100Write(t, filepath.Join(dir, "prompts/review.md"), "RELOADED $1")
			settings := s.SettingsManager()
			if err := settings.SetProjectTrusted(false); err != nil {
				t.Fatal(err)
			}
			canceled, cancel := context.WithCancel(ctx)
			cancel()
			if err := loader.Reload(canceled); !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
			templates, _ = s.PromptTemplates()
			if templates[0].Content != want {
				t.Fatal("canceled reload changed templates")
			}
			if err := loader.Reload(ctx); err != nil {
				t.Fatal(err)
			}
			templates, _ = s.PromptTemplates()
			if len(templates) != 1 || templates[0].Content != "RELOADED $1" {
				t.Fatal(templates)
			}
		})
	}
}

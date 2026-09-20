package codingagent_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
	"github.com/nankedr/pig/tui"
)

func TestSessionAutocompleteResources115(t *testing.T) {
	for _, scenario := range []struct {
		name                          string
		trusted, disabled, hideSkills bool
	}{{name: "untrusted"}, {name: "trusted", trusted: true}, {name: "disabled", trusted: true, disabled: true}, {name: "skill-commands-disabled", trusted: true, hideSkills: true}} {
		t.Run(scenario.name, func(t *testing.T) {
			ctx := context.Background()
			root := t.TempDir()
			cwd, dir := filepath.Join(root, "repo"), filepath.Join(root, "agent")
			context100Write(t, filepath.Join(dir, "prompts/review.md"), "---\ndescription: global\nargument-hint: <topic>\n---\nGLOBAL $1")
			context100Write(t, filepath.Join(cwd, ".pig/prompts/review.md"), "---\ndescription: project\n---\nPROJECT $1")
			context100Write(t, filepath.Join(cwd, ".pig/prompts/private.md"), "PRIVATE")
			context100Write(t, filepath.Join(dir, "skills/guide/SKILL.md"), "---\nname: guide\ndescription: guide\ndisable-model-invocation: true\n---\nGUIDE BODY")
			context100Write(t, filepath.Join(cwd, ".pig/skills/secret/SKILL.md"), "---\nname: secret\ndescription: secret\n---\nSECRET")
			settings, err := codingagent.NewSettingsManager(cwd, &dir)
			if err != nil {
				t.Fatal(err)
			}
			if err = settings.SetProjectTrusted(scenario.trusted); err != nil {
				t.Fatal(err)
			}
			if err = settings.SetEnableSkillCommands(!scenario.hideSkills); err != nil {
				t.Fatal(err)
			}
			loader, err := codingagent.NewDefaultResourceLoader(codingagent.DefaultResourceLoaderOptions{CWD: cwd, AgentDir: dir, SettingsManager: settings, NoPromptTemplates: scenario.disabled, NoSkills: scenario.disabled, NoExtensions: true})
			if err != nil {
				t.Fatal(err)
			}
			if err = loader.Reload(ctx); err != nil {
				t.Fatal(err)
			}
			core, _ := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
			model, _ := core.GetModel()
			received := []string{}
			response := ai.FauxResponseFactory(func(input ai.Context, _ *ai.SimpleStreamOptions, _ *ai.FauxProviderState, _ ai.Model) (ai.AssistantMessage, error) {
				m := input.Messages[len(input.Messages)-1].(ai.UserMessage)
				text, _ := m.Content.Text()
				if blocks, ok := m.Content.Blocks(); ok {
					for _, b := range blocks {
						if v, ok := b.(ai.TextContent); ok {
							text += v.Text
						}
					}
				}
				received = append(received, text)
				return ai.FauxAssistantMessage(ai.FauxAssistantText("DONE"))
			})
			core.SetResponses([]ai.FauxResponseStep{response, response})
			created, err := codingagent.CreateAgentSession(ctx, codingagent.CreateAgentSessionOptions{CWD: cwd, AgentDir: dir, Model: &model, StreamFunction: agent.StreamFunction(core.StreamSimple), SettingsManager: settings, ResourceLoader: loader, NoTools: codingagent.NoToolsAll})
			if err != nil {
				t.Fatal(err)
			}
			defer created.Session.Dispose()
			provider, err := codingagent.NewSessionAutocompleteProvider(created.Session, nil)
			if err != nil {
				t.Fatal(err)
			}
			all, ok, err := provider.GetSuggestions(ctx, []string{"/"}, 0, 1, tui.AutocompleteOptions{})
			if err != nil || !ok {
				t.Fatalf("menu: %+v %v", all, err)
			}
			names := map[string]tui.AutocompleteItem{}
			for _, item := range all.Items {
				names[item.Value] = item
			}
			if _, ok := names["private"]; ok != (scenario.trusted && !scenario.disabled) {
				t.Fatalf("project resource visible=%v", ok)
			}
			if _, ok := names["skill:secret"]; ok != (scenario.trusted && !scenario.disabled && !scenario.hideSkills) {
				t.Fatalf("project skill visible=%v", ok)
			}
			if _, ok := names["skill:guide"]; ok != (!scenario.disabled && !scenario.hideSkills) {
				t.Fatalf("skill visible=%v", ok)
			}
			if scenario.disabled {
				if _, ok := names["review"]; ok {
					t.Fatal("disabled template discovered")
				}
				return
			}
			loaded, _ := loader.GetPrompts()
			if scenario.trusted && len(loaded.Diagnostics) == 0 {
				t.Fatal("missing conflict diagnostic")
			}
			description, wantText := "global", "GLOBAL 主题"
			if scenario.trusted {
				description, wantText = "project", "PROJECT 主题"
			}
			item := names["review"]
			if item.Description == nil || !strings.Contains(*item.Description, description) {
				t.Fatalf("wrong winning resource: %+v", item)
			}
			result, err := provider.ApplyCompletion([]string{"/rev"}, 0, 4, item, "/rev")
			if err != nil {
				t.Fatal(err)
			}
			if err = created.Session.Prompt(ctx, result.Lines[0]+"主题"); err != nil {
				t.Fatal(err)
			}
			if len(received) != 1 || received[0] != wantText {
				t.Fatalf("Provider received %q", received)
			}
			if !scenario.hideSkills {
				result, err = provider.ApplyCompletion([]string{"/skill:g"}, 0, 8, names["skill:guide"], "/skill:g")
				if err != nil {
					t.Fatal(err)
				}
				if err = created.Session.Prompt(ctx, result.Lines[0]+"question"); err != nil {
					t.Fatal(err)
				}
				if len(received) != 2 || !strings.Contains(received[1], "GUIDE BODY\n</skill>\n\nquestion") {
					t.Fatalf("Provider skill=%q", received)
				}
			}
		})
	}
}

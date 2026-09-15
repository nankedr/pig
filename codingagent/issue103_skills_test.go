package codingagent_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
	"github.com/nankedr/pig/internal/baseline"
	"github.com/nankedr/pig/internal/parity"
)

func TestSkillsSDKParity(t *testing.T) {
	lock, _, err := baseline.Load("../parity/baseline")
	if err != nil {
		t.Fatal(err)
	}
	pinned := parity.Baseline{ID: lock.BaselineID, Commit: lock.Upstream.Commit, Repository: lock.Upstream.Repository}
	fixture, err := parity.LoadFixture("../parity/oracle/fixtures/skills.json", pinned)
	if err != nil {
		t.Fatal(err)
	}
	oracle, err := parity.NewFixtureDriver(fixture, pinned)
	if err != nil {
		t.Fatal(err)
	}
	result, err := parity.RunCase(context.Background(), fixture.Case, oracle, parity.DriverFunc{SurfaceName: parity.SurfaceGoSDK, ObserveFunc: func(ctx context.Context, c parity.Case) (parity.Observation, error) {
		var input struct {
			Scenarios []struct {
				Name, CWD                     string
				Links, ExtraFiles             map[string]string
				Files                         map[string]string
				Trusted, Disabled             bool
				Paths, Calls, Global, Project []string
			}
		}
		if err := json.Unmarshal(c.Input, &input); err != nil {
			return parity.Observation{}, err
		}
		outcomes := []map[string]any{}
		for _, scenario := range input.Scenarios {
			dir := t.TempDir()
			t.Setenv("HOME", filepath.Join(dir, "home"))
			cwdPath := scenario.CWD
			if cwdPath == "" {
				cwdPath = "repo"
			}
			cwd, agentDir := filepath.Join(dir, cwdPath), filepath.Join(dir, "agent")
			expand := func(s string) string { return strings.ReplaceAll(s, "/.pi/", "/.pig/") }
			normalize := func(s string) string {
				return strings.ReplaceAll(strings.ReplaceAll(s, dir, "$ROOT"), "/.pig", "/.pi")
			}
			for p, content := range scenario.Files {
				context100Write(t, expand(filepath.Join(dir, p)), content)
			}
			for p, content := range scenario.ExtraFiles {
				context100Write(t, filepath.Join(dir, p), content)
			}
			for p, target := range scenario.Links {
				if err := os.Symlink(target, expand(filepath.Join(dir, p))); err != nil {
					t.Fatal(err)
				}
			}
			global, _ := json.Marshal(map[string]any{"skills": scenario.Global})
			project, _ := json.Marshal(map[string]any{"skills": scenario.Project})
			context100Write(t, filepath.Join(agentDir, "settings.json"), string(global))
			context100Write(t, filepath.Join(cwd, ".pig/settings.json"), string(project))
			settings, err := codingagent.NewSettingsManager(cwd, &agentDir)
			if err != nil {
				return parity.Observation{}, err
			}
			if err = settings.SetProjectTrusted(scenario.Trusted); err != nil {
				return parity.Observation{}, err
			}
			paths := []string{}
			for _, p := range scenario.Paths {
				paths = append(paths, strings.ReplaceAll(strings.ReplaceAll(p, ".pi/", ".pig/"), "$ROOT", dir))
			}
			system := "SYSTEM"
			loader, err := codingagent.NewDefaultResourceLoader(codingagent.DefaultResourceLoaderOptions{CWD: cwd, AgentDir: agentDir, SettingsManager: settings, NoContextFiles: true, NoSkills: scenario.Disabled, AdditionalSkillPaths: paths, SystemPrompt: &system})
			if err != nil {
				return parity.Observation{}, fmt.Errorf("%s: %w", scenario.Name, err)
			}
			if err = loader.Reload(ctx); err != nil {
				return parity.Observation{}, err
			}
			loaded, err := loader.GetSkills()
			if err != nil {
				return parity.Observation{}, err
			}
			core, _ := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
			model, _ := core.GetModel()
			received := []string{}
			systems := []string{}
			steps := []ai.FauxResponseStep{}
			for range scenario.Calls {
				steps = append(steps, ai.FauxResponseFactory(func(input ai.Context, _ *ai.SimpleStreamOptions, _ *ai.FauxProviderState, _ ai.Model) (ai.AssistantMessage, error) {
					m := input.Messages[len(input.Messages)-1].(ai.UserMessage)
					text, compact := m.Content.Text()
					if !compact {
						blocks, _ := m.Content.Blocks()
						for _, b := range blocks {
							if v, ok := b.(ai.TextContent); ok {
								text += v.Text
							}
						}
					}
					received = append(received, normalize(text))
					return ai.FauxAssistantMessage(ai.FauxAssistantText("TEMPLATE_OK"))
				}))
			}
			core.SetResponses(steps)
			created, err := codingagent.CreateAgentSession(ctx, codingagent.CreateAgentSessionOptions{CWD: cwd, AgentDir: agentDir, Model: &model, StreamFunction: agent.StreamFunction(core.StreamSimple), SettingsManager: settings, ResourceLoader: loader, Tools: []string{"read"}})
			if err != nil {
				return parity.Observation{}, err
			}
			s := created.Session
			systems = append(systems, normalize(s.SystemPrompt()))
			if err := s.SetActiveToolsByName([]string{}); err != nil {
				t.Fatal(err)
			}
			systems = append(systems, normalize(s.SystemPrompt()))
			skills, err := s.ResourceLoader().GetSkills()
			if err != nil {
				s.Dispose()
				return parity.Observation{}, err
			}
			for _, call := range scenario.Calls {
				if err = s.Prompt(ctx, call); err != nil {
					s.Dispose()
					return parity.Observation{}, err
				}
			}
			messages := s.Messages()
			s.Dispose()
			if len(messages) != len(scenario.Calls)*2 {
				return parity.Observation{}, fmt.Errorf("%s missing replies: %d", scenario.Name, len(messages))
			}
			entries := []map[string]any{}
			for _, p := range skills.Skills {
				source := map[string]any{"path": normalize(p.SourceInfo.Path), "source": p.SourceInfo.Source, "scope": p.SourceInfo.Scope, "origin": p.SourceInfo.Origin}
				if p.SourceInfo.BaseDir != "" {
					source["baseDir"] = normalize(p.SourceInfo.BaseDir)
				}
				entries = append(entries, map[string]any{"name": p.Name, "description": p.Description, "filePath": normalize(p.FilePath), "baseDir": normalize(p.BaseDir), "sourceInfo": source, "disableModelInvocation": p.DisableModelInvocation})
			}
			diagnostics := []map[string]any{}
			for _, d := range loaded.Diagnostics {
				v := map[string]any{"type": d.Type, "message": d.Message, "path": normalize(d.Path)}
				if d.Collision != nil {
					c := d.Collision
					v["collision"] = map[string]any{"resourceType": c.ResourceType, "name": c.Name, "winnerPath": normalize(c.WinnerPath), "loserPath": normalize(c.LoserPath)}
				}
				diagnostics = append(diagnostics, v)
			}
			outcomes = append(outcomes, map[string]any{"name": scenario.Name, "skills": entries, "systems": systems, "diagnostics": diagnostics, "expanded": received})
		}
		outcome, err := json.Marshal(outcomes)
		effects := []parity.SideEffect{}
		return parity.Observation{Outcome: outcome, SideEffects: &effects}, err
	}})
	if err != nil || !result.Match {
		t.Fatalf("skill parity: %v %v\nPig=%s\nPi=%s", err, result.Differences, result.Pig.Outcome, result.Oracle.Outcome)
	}
}

func TestSkillsStreamingQueuesAndSendUserMessage(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "skills/review/SKILL.md")
	context100Write(t, path, "---\nname: review\ndescription: Review changes\ndisable-model-invocation: true\n---\nBODY")
	core, _ := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
	model, _ := core.GetModel()
	var session *codingagent.AgentSession
	received := []string{}
	response := ai.FauxResponseFactory(func(input ai.Context, _ *ai.SimpleStreamOptions, _ *ai.FauxProviderState, _ ai.Model) (ai.AssistantMessage, error) {
		text := message85Text(input.Messages[len(input.Messages)-1])
		if block := codingagent.ParseSkillBlock(text); block != nil {
			if !strings.Contains(block.Content, "References are relative to "+filepath.Dir(path)+".\n\nBODY") {
				return ai.AssistantMessage{}, fmt.Errorf("skill body: %s", block.Content)
			}
			received = append(received, block.UserMessage)
		} else {
			received = append(received, text)
		}
		if len(received) == 1 {
			if err := session.Steer("/skill:review steer"); err != nil {
				return ai.AssistantMessage{}, err
			}
			if err := session.FollowUp("/skill:review follow"); err != nil {
				return ai.AssistantMessage{}, err
			}
			disabled := false
			if err := session.Prompt(ctx, "/skill:review raw", codingagent.PromptOptions{StreamingBehavior: "followUp", ExpandPromptTemplates: &disabled}); err != nil {
				return ai.AssistantMessage{}, err
			}
		}
		return ai.FauxAssistantMessage(ai.FauxAssistantText("SKILL_OK"))
	})
	core.SetResponses([]ai.FauxResponseStep{response, response, response, response, response, response})
	created, err := codingagent.CreateAgentSession(ctx, codingagent.CreateAgentSessionOptions{CWD: dir, AgentDir: dir, Model: &model, StreamFunction: agent.StreamFunction(core.StreamSimple), NoTools: codingagent.NoToolsAll})
	if err != nil {
		t.Fatal(err)
	}
	session = created.Session
	defer session.Dispose()
	if err = session.Prompt(ctx, "/skill:review start"); err != nil {
		t.Fatal(err)
	}
	if err = session.SendUserMessage(ai.UserText("/skill:review sdk")); err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err = session.Prompt(ctx, "/skill:review removed"); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(received, "|"); got != "start|steer|follow|/skill:review raw|/skill:review sdk|/skill:review removed" {
		t.Fatal(got)
	}
}

func TestSkillsInvalidMetadataAndSymlinkCycle(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	ctx := context.Background()
	dir := t.TempDir()
	for name, content := range map[string]string{
		"syntax":    "---\ndescription: [broken\n---\nBAD",
		"duplicate": "---\nname: x\nname: y\ndescription: duplicate\n---\nBAD",
		"type":      "---\nname: 42\ndescription: invalid\n---\nBAD",
		"valid":     "---\nname: valid\ndescription: Valid skill\n---\nBODY",
	} {
		context100Write(t, filepath.Join(dir, "skills", name, "SKILL.md"), content)
	}
	if err := os.Symlink(".", filepath.Join(dir, "skills/cycle")); err != nil {
		t.Fatal(err)
	}
	core, _ := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
	model, _ := core.GetModel()
	created, err := codingagent.CreateAgentSession(ctx, codingagent.CreateAgentSessionOptions{CWD: dir, AgentDir: dir, Model: &model, StreamFunction: agent.StreamFunction(core.StreamSimple), Tools: []string{"read"}})
	if err != nil {
		t.Fatal(err)
	}
	defer created.Session.Dispose()
	loaded, err := created.Session.ResourceLoader().GetSkills()
	if err != nil || len(loaded.Skills) != 1 || loaded.Skills[0].Name != "valid" || len(loaded.Diagnostics) != 3 {
		t.Fatalf("%+v %v", loaded, err)
	}
	for _, d := range loaded.Diagnostics {
		if d.Type != "warning" || d.Message == "" || d.Path == "" {
			t.Fatal(d)
		}
	}
	if !strings.Contains(created.Session.SystemPrompt(), "<name>valid</name>") {
		t.Fatal("missing model-visible skill")
	}
	direct, err := codingagent.LoadSkillsFromDir(ctx, codingagent.LoadSkillsFromDirOptions{Dir: filepath.Join(dir, "skills"), Source: "user"})
	if err != nil || len(direct.Skills) != 1 || len(direct.Diagnostics) != 3 {
		t.Fatalf("%+v %v", direct, err)
	}
}

package codingagent_test

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
	"github.com/nankedr/pig/internal/baseline"
	"github.com/nankedr/pig/internal/parity"
)

func TestPromptTemplatesSDKParity(t *testing.T) {
	lock, _, err := baseline.Load("../parity/baseline")
	if err != nil {
		t.Fatal(err)
	}
	pinned := parity.Baseline{ID: lock.BaselineID, Commit: lock.Upstream.Commit, Repository: lock.Upstream.Repository}
	fixture, err := parity.LoadFixture("../parity/oracle/fixtures/prompt-templates.json", pinned)
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
				Name                          string
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
			cwd, agentDir := filepath.Join(dir, "repo"), filepath.Join(dir, "agent")
			expand := func(s string) string { return strings.ReplaceAll(s, "/.pi/", "/.pig/") }
			normalize := func(s string) string {
				return strings.ReplaceAll(strings.ReplaceAll(s, dir, "$ROOT"), "/.pig", "/.pi")
			}
			for p, content := range scenario.Files {
				context100Write(t, expand(filepath.Join(dir, p)), content)
			}
			global, _ := json.Marshal(map[string]any{"prompts": scenario.Global})
			project, _ := json.Marshal(map[string]any{"prompts": scenario.Project})
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
			loader, err := codingagent.NewDefaultResourceLoader(codingagent.DefaultResourceLoaderOptions{CWD: cwd, AgentDir: agentDir, SettingsManager: settings, NoContextFiles: true, NoPromptTemplates: scenario.Disabled, AdditionalPromptTemplatePaths: paths})
			if err != nil {
				return parity.Observation{}, fmt.Errorf("%s: %w", scenario.Name, err)
			}
			if err = loader.Reload(ctx); err != nil {
				return parity.Observation{}, err
			}
			loaded, err := loader.GetPrompts()
			if err != nil {
				return parity.Observation{}, err
			}
			core, _ := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
			model, _ := core.GetModel()
			received := []string{}
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
					received = append(received, text)
					return ai.FauxAssistantMessage(ai.FauxAssistantText("TEMPLATE_OK"))
				}))
			}
			core.SetResponses(steps)
			created, err := codingagent.CreateAgentSession(ctx, codingagent.CreateAgentSessionOptions{CWD: cwd, AgentDir: agentDir, Model: &model, StreamFunction: agent.StreamFunction(core.StreamSimple), SettingsManager: settings, ResourceLoader: loader, NoTools: codingagent.NoToolsAll})
			if err != nil {
				return parity.Observation{}, err
			}
			s := created.Session
			templates, err := s.PromptTemplates()
			if err != nil {
				s.Dispose()
				return parity.Observation{}, err
			}
			for i, call := range scenario.Calls {
				enabled := !(scenario.Name == "arguments" && i == 6)
				if err = s.Prompt(ctx, call, codingagent.PromptOptions{ExpandPromptTemplates: &enabled}); err != nil {
					s.Dispose()
					return parity.Observation{}, err
				}
			}
			messages := s.Messages()
			s.Dispose()
			if len(messages) != len(scenario.Calls)*2 {
				return parity.Observation{}, fmt.Errorf("%s missing replies: %d", scenario.Name, len(messages))
			}
			prompts := []map[string]any{}
			for _, p := range templates {
				source := map[string]any{"path": normalize(p.SourceInfo.Path), "source": p.SourceInfo.Source, "scope": p.SourceInfo.Scope, "origin": p.SourceInfo.Origin}
				if p.SourceInfo.BaseDir != "" {
					source["baseDir"] = normalize(p.SourceInfo.BaseDir)
				}
				v := map[string]any{"name": p.Name, "description": p.Description, "content": p.Content, "filePath": normalize(p.FilePath), "sourceInfo": source}
				if p.ArgumentHint != "" {
					v["argumentHint"] = p.ArgumentHint
				}
				prompts = append(prompts, v)
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
			outcomes = append(outcomes, map[string]any{"name": scenario.Name, "prompts": prompts, "diagnostics": diagnostics, "expanded": received})
		}
		outcome, err := json.Marshal(outcomes)
		effects := []parity.SideEffect{}
		return parity.Observation{Outcome: outcome, SideEffects: &effects}, err
	}})
	if err != nil || !result.Match {
		t.Fatalf("template parity: %v\nPig=%s\nPi=%s", result.Differences, result.Pig.Outcome, result.Oracle.Outcome)
	}
}

func TestPromptTemplatesStreamingQueuesAndSendUserMessage(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	context100Write(t, filepath.Join(dir, "prompts/review.md"), "Review $1")
	core, _ := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
	model, _ := core.GetModel()
	var session *codingagent.AgentSession
	var received []string
	response := ai.FauxResponseFactory(func(input ai.Context, _ *ai.SimpleStreamOptions, _ *ai.FauxProviderState, _ ai.Model) (ai.AssistantMessage, error) {
		m := input.Messages[len(input.Messages)-1].(ai.UserMessage)
		blocks, _ := m.Content.Blocks()
		text := ""
		for _, b := range blocks {
			if b, ok := b.(ai.TextContent); ok {
				text += b.Text
			}
		}
		received = append(received, text)
		if len(received) == 1 {
			if err := session.Steer("/review now"); err != nil {
				return ai.AssistantMessage{}, err
			}
			if err := session.FollowUp("/review next"); err != nil {
				return ai.AssistantMessage{}, err
			}
			disabled := false
			if err := session.Prompt(ctx, "/review raw", codingagent.PromptOptions{StreamingBehavior: "followUp", ExpandPromptTemplates: &disabled}); err != nil {
				return ai.AssistantMessage{}, err
			}
		}
		return ai.FauxAssistantMessage(ai.FauxAssistantText("QUEUE_OK"))
	})
	core.SetResponses([]ai.FauxResponseStep{response, response, response, response, response})
	created, err := codingagent.CreateAgentSession(ctx, codingagent.CreateAgentSessionOptions{CWD: dir, AgentDir: dir, Model: &model, StreamFunction: agent.StreamFunction(core.StreamSimple), NoTools: codingagent.NoToolsAll})
	if err != nil {
		t.Fatal(err)
	}
	session = created.Session
	defer session.Dispose()
	if err = session.Prompt(ctx, "/review start"); err != nil {
		t.Fatal(err)
	}
	if err = session.SendUserMessage(ai.UserText("/review sdk")); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(received, "|"); got != "Review start|Review now|Review next|/review raw|/review sdk" {
		t.Fatalf("model inputs: %s", got)
	}
	reply, err := session.GetLastAssistantText()
	if err != nil || reply == nil || *reply != "QUEUE_OK" {
		t.Fatalf("reply: %v %v", reply, err)
	}
}

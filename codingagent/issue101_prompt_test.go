package codingagent_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
	"github.com/nankedr/pig/internal/baseline"
	"github.com/nankedr/pig/internal/parity"
)

func TestSystemPromptsSDKParity(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("fixture requires POSIX non-root permissions")
	}
	lock, _, err := baseline.Load("../parity/baseline")
	if err != nil {
		t.Fatal(err)
	}
	pinned := parity.Baseline{ID: lock.BaselineID, Commit: lock.Upstream.Commit, Repository: lock.Upstream.Repository}
	fixture, err := parity.LoadFixture("../parity/oracle/fixtures/system-prompts.json", pinned)
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
				Name              string
				Files             map[string]string
				Trusted, Disabled bool
				System            *string
				Append            []string
				Unreadable        string
			}
		}
		if err := json.Unmarshal(c.Input, &input); err != nil {
			return parity.Observation{}, err
		}
		outcomes := []map[string]any{}
		for _, scenario := range input.Scenarios {
			dir := t.TempDir()
			cwd, agentDir := filepath.Join(dir, "repo"), filepath.Join(dir, "agent")
			for path, content := range scenario.Files {
				context100Write(t, filepath.Join(dir, strings.ReplaceAll(path, "/.pi/", "/.pig/")), content)
			}
			for _, p := range []string{cwd, agentDir} {
				if err := os.MkdirAll(p, 0700); err != nil {
					t.Fatal(err)
				}
			}
			if scenario.Unreadable != "" {
				p := filepath.Join(dir, scenario.Unreadable)
				if err := os.Chmod(p, 0); err != nil {
					t.Fatal(err)
				}
				defer os.Chmod(p, 0600)
			}
			// Pi resolves explicit relative inputs against the process working directory.
			t.Chdir(cwd)
			expand := func(s string) string {
				return strings.ReplaceAll(strings.ReplaceAll(s, "$ROOT", dir), "/.pi/", "/.pig/")
			}
			normalize := func(s string) string {
				return strings.ReplaceAll(strings.ReplaceAll(s, dir, "$ROOT"), "/.pig/", "/.pi/")
			}
			settings, _ := codingagent.NewInMemorySettingsManager(codingagent.Settings{}, codingagent.SettingsManagerCreateOptions{ProjectTrusted: &scenario.Trusted})
			opts := codingagent.DefaultResourceLoaderOptions{CWD: cwd, AgentDir: agentDir, SettingsManager: settings, NoContextFiles: scenario.Disabled, NoExtensions: true, NoSkills: true, NoThemes: true, NoPromptTemplates: true}
			if scenario.System != nil {
				v := expand(*scenario.System)
				opts.SystemPrompt = &v
			}
			if scenario.Append != nil {
				opts.AppendSystemPrompt = []string{}
				for _, s := range scenario.Append {
					opts.AppendSystemPrompt = append(opts.AppendSystemPrompt, expand(s))
				}
			}
			loader, err := codingagent.NewDefaultResourceLoader(opts)
			if err != nil {
				return parity.Observation{}, fmt.Errorf("%s: %w", scenario.Name, err)
			}
			if err = loader.Reload(ctx); err != nil {
				return parity.Observation{}, err
			}
			system, err := loader.GetSystemPrompt()
			if err != nil {
				return parity.Observation{}, err
			}
			appended, err := loader.GetAppendSystemPrompt()
			if err != nil {
				return parity.Observation{}, err
			}
			source, err := loader.GetSystemPromptSource()
			if err != nil {
				return parity.Observation{}, err
			}
			sources, err := loader.GetAppendSystemPromptSources()
			if err != nil {
				return parity.Observation{}, err
			}
			core, _ := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
			model, _ := core.GetModel()
			received := []string{}
			response := ai.FauxResponseFactory(func(input ai.Context, _ *ai.SimpleStreamOptions, _ *ai.FauxProviderState, _ ai.Model) (ai.AssistantMessage, error) {
				s, _ := input.SystemPrompt.Value()
				received = append(received, s)
				return ai.FauxAssistantMessage(ai.FauxAssistantText("PROMPT_OK"))
			})
			core.SetResponses([]ai.FauxResponseStep{response, response})
			read, err := codingagent.CreateReadTool(cwd)
			if err != nil {
				return parity.Observation{}, err
			}
			created, err := codingagent.CreateAgentSession(ctx, codingagent.CreateAgentSessionOptions{CWD: cwd, AgentDir: agentDir, Model: &model, StreamFunction: agent.StreamFunction(core.StreamSimple), SettingsManager: settings, ResourceLoader: loader, AgentTools: []agent.ErasedAgentTool{read}, NoTools: codingagent.NoToolsBuiltin})
			if err != nil {
				return parity.Observation{}, err
			}
			s := created.Session
			if err = s.Prompt(ctx, "first"); err != nil {
				s.Dispose()
				return parity.Observation{}, err
			}
			if err = s.SetActiveToolsByName([]string{"read"}); err != nil {
				s.Dispose()
				return parity.Observation{}, err
			}
			if err = s.Prompt(ctx, "second"); err != nil {
				s.Dispose()
				return parity.Observation{}, err
			}
			s.Dispose()
			if len(received) != 2 {
				t.Fatalf("%s missing model inputs: %v", scenario.Name, received)
			}
			custom := system != nil && *system != ""
			for _, prompt := range received {
				if custom && !strings.HasPrefix(prompt, *system) {
					t.Fatalf("%s lost replacement: %s", scenario.Name, prompt)
				}
				for _, a := range appended {
					if !strings.Contains(prompt, a) {
						t.Fatalf("%s lost append after tool change: %s", scenario.Name, prompt)
					}
				}
			}
			if !custom && (!strings.Contains(received[0], "Available tools:\n(none)") || !strings.Contains(received[1], "- read: Read file contents")) {
				t.Fatalf("tool prompt did not rebuild: %v", received)
			}
			var projectedSystem, projectedSource any
			if system != nil {
				projectedSystem = normalize(*system)
			}
			if source != nil {
				projectedSource = normalize(source.Path)
			}
			projectedAppend, projectedSources := []string{}, []string{}
			for _, a := range appended {
				projectedAppend = append(projectedAppend, normalize(a))
			}
			for _, a := range sources {
				projectedSources = append(projectedSources, normalize(a.Path))
			}
			warning := false
			if d, ok := any(loader).(interface {
				GetSystemPromptDiagnostics() []codingagent.ResourceDiagnostic
			}); ok {
				warning = len(d.GetSystemPromptDiagnostics()) > 0
			}
			outcomes = append(outcomes, map[string]any{"name": scenario.Name, "system": projectedSystem, "append": projectedAppend, "source": projectedSource, "appendSources": projectedSources, "warning": warning, "custom": custom, "context": strings.Contains(received[0], "CONTEXT"), "appended": true})
		}
		outcome, err := json.Marshal(outcomes)
		effects := []parity.SideEffect{}
		return parity.Observation{Outcome: outcome, SideEffects: &effects}, err
	}})
	if err != nil || !result.Match {
		t.Fatalf("system prompt parity: %v %v\nPig=%s\nPi=%s", err, result.Differences, result.Pig.Outcome, result.Oracle.Outcome)
	}
}

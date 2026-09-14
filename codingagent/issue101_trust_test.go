package codingagent_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
)

func TestSystemPromptsTrustAndReload(t *testing.T) {
	ctx := context.Background()
	for _, test := range []struct {
		name, global string
		saved        *bool
		trusted      bool
	}{
		{name: "ask"}, {name: "never", global: `{"defaultProjectTrust":"never"}`},
		{name: "always", global: `{"defaultProjectTrust":"always"}`, trusted: true},
		{name: "saved-allow", saved: codingagent.ProjectTrustDecisionTrusted(), trusted: true},
		{name: "saved-deny", global: `{"defaultProjectTrust":"always"}`, saved: codingagent.ProjectTrustDecisionUntrusted()},
	} {
		t.Run(test.name, func(t *testing.T) {
			cwd, dir := t.TempDir(), t.TempDir()
			context100Write(t, filepath.Join(cwd, ".pig", "SYSTEM.md"), "PROJECT_SYSTEM")
			context100Write(t, filepath.Join(cwd, ".pig", "APPEND_SYSTEM.md"), "PROJECT_APPEND")
			context100Write(t, filepath.Join(cwd, "AGENTS.md"), "CONTEXT")
			context100Write(t, filepath.Join(dir, "SYSTEM.md"), "GLOBAL_SYSTEM")
			context100Write(t, filepath.Join(dir, "APPEND_SYSTEM.md"), "GLOBAL_APPEND")
			if test.global != "" {
				context100Write(t, filepath.Join(dir, "settings.json"), test.global)
			}
			if test.saved != nil {
				if err := codingagent.NewProjectTrustStore(dir).Set(ctx, cwd, test.saved); err != nil {
					t.Fatal(err)
				}
			}
			loader, err := codingagent.NewDefaultResourceLoader(codingagent.DefaultResourceLoaderOptions{CWD: cwd, AgentDir: dir})
			if err != nil {
				t.Fatal(err)
			}
			if s, err := loader.GetSystemPrompt(); err != nil || s != nil {
				t.Fatalf("constructor loaded resources: %v %v", s, err)
			}
			if err = loader.Reload(ctx); err != nil {
				t.Fatal(err)
			}
			prefix := "GLOBAL"
			if test.trusted {
				prefix = "PROJECT"
			}
			system, _ := loader.GetSystemPrompt()
			appended, _ := loader.GetAppendSystemPrompt()
			source, _ := loader.GetSystemPromptSource()
			sources, _ := loader.GetAppendSystemPromptSources()
			if system == nil || *system != prefix+"_SYSTEM" || len(appended) != 1 || appended[0] != prefix+"_APPEND" {
				t.Fatalf("trust=%v: %v %v", test.trusted, system, appended)
			}
			*system = "MUTATED"
			appended[0] = "MUTATED"
			source.Path = "MUTATED"
			sources[0].Path = "MUTATED"
			system, _ = loader.GetSystemPrompt()
			source, _ = loader.GetSystemPromptSource()
			sources, _ = loader.GetAppendSystemPromptSources()
			if *system != prefix+"_SYSTEM" || source.Path == "MUTATED" || sources[0].Path == "MUTATED" {
				t.Fatal("resource snapshot aliases caller")
			}
			context100Write(t, filepath.Join(dir, "SYSTEM.md"), "RELOADED")
			if err = codingagent.NewProjectTrustStore(dir).Set(ctx, cwd, codingagent.ProjectTrustDecisionUntrusted()); err != nil {
				t.Fatal(err)
			}
			canceled, cancel := context.WithCancel(ctx)
			cancel()
			if err = loader.Reload(canceled); !errors.Is(err, context.Canceled) {
				t.Fatalf("cancel: %v", err)
			}
			system, _ = loader.GetSystemPrompt()
			if *system != prefix+"_SYSTEM" {
				t.Fatal("canceled reload replaced snapshot")
			}
			if err = loader.Reload(ctx); err != nil {
				t.Fatal(err)
			}
			core, _ := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
			model, _ := core.GetModel()
			core.SetResponses([]ai.FauxResponseStep{ai.FauxResponseFactory(func(input ai.Context, _ *ai.SimpleStreamOptions, _ *ai.FauxProviderState, _ ai.Model) (ai.AssistantMessage, error) {
				prompt, _ := input.SystemPrompt.Value()
				if !strings.HasPrefix(prompt, "RELOADED\n\nGLOBAL_APPEND") || !strings.Contains(prompt, "CONTEXT") || strings.Contains(prompt, "PROJECT_") {
					t.Errorf("reloaded session: %s", prompt)
				}
				return ai.FauxAssistantMessage(ai.FauxAssistantText("OK"))
			})})
			created, err := codingagent.CreateAgentSession(ctx, codingagent.CreateAgentSessionOptions{CWD: cwd, AgentDir: dir, Model: &model, StreamFunction: agent.StreamFunction(core.StreamSimple), ResourceLoader: loader, NoTools: codingagent.NoToolsAll})
			if err != nil {
				t.Fatal(err)
			}
			defer created.Session.Dispose()
			if err = created.Session.Prompt(ctx, "hello"); err != nil {
				t.Fatal(err)
			}
			if _, err = loader.LoadProjectTrustExtensions(ctx); !errors.Is(err, codingagent.ErrNotImplemented) {
				t.Fatalf("M7 boundary: %v", err)
			}
		})
	}
}

type prompt101FailureLoader struct {
	codingagent.ResourceLoader
	appendFailure bool
}

func (l prompt101FailureLoader) GetSystemPrompt() (*string, error) {
	if !l.appendFailure {
		return nil, errors.New("system load failed")
	}
	return nil, nil
}
func (l prompt101FailureLoader) GetAppendSystemPrompt() ([]string, error) {
	return nil, errors.New("append load failed")
}

func TestSystemPromptsInjectedFailuresAndOwnership(t *testing.T) {
	ctx := context.Background()
	core, _ := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
	model, _ := core.GetModel()
	for _, appendFailure := range []bool{false, true} {
		_, err := codingagent.CreateAgentSession(ctx, codingagent.CreateAgentSessionOptions{CWD: t.TempDir(), AgentDir: t.TempDir(), Model: &model, StreamFunction: agent.StreamFunction(core.StreamSimple), ResourceLoader: prompt101FailureLoader{appendFailure: appendFailure}})
		want := "system load failed"
		if appendFailure {
			want = "append load failed"
		}
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("injected failure: %v", err)
		}
	}
	cwd, dir := t.TempDir(), t.TempDir()
	system := "ORIGINAL"
	appended := []string{"APPEND"}
	loader, err := codingagent.NewDefaultResourceLoader(codingagent.DefaultResourceLoaderOptions{CWD: cwd, AgentDir: dir, SystemPrompt: &system, AppendSystemPrompt: appended})
	if err != nil {
		t.Fatal(err)
	}
	system = "MUTATED"
	appended[0] = "MUTATED"
	if err = loader.Reload(ctx); err != nil {
		t.Fatal(err)
	}
	prompt, _ := loader.GetSystemPrompt()
	appendPrompt, _ := loader.GetAppendSystemPrompt()
	if prompt == nil || *prompt != "ORIGINAL" || len(appendPrompt) != 1 || appendPrompt[0] != "APPEND" {
		t.Fatalf("input ownership: %v %v", prompt, appendPrompt)
	}
}

func TestSystemPromptsSDKSettingsTrust(t *testing.T) {
	for _, trusted := range []bool{false, true} {
		t.Run(map[bool]string{false: "untrusted", true: "trusted"}[trusted], func(t *testing.T) {
			cwd, dir := t.TempDir(), t.TempDir()
			ctx := context.Background()
			context100Write(t, filepath.Join(cwd, ".pig", "SYSTEM.md"), "PROJECT")
			context100Write(t, filepath.Join(cwd, ".pig", "APPEND_SYSTEM.md"), "PROJECT_APPEND")
			context100Write(t, filepath.Join(dir, "SYSTEM.md"), "GLOBAL")
			context100Write(t, filepath.Join(dir, "APPEND_SYSTEM.md"), "GLOBAL_APPEND")
			settings, err := codingagent.NewInMemorySettingsManager(codingagent.Settings{}, codingagent.SettingsManagerCreateOptions{ProjectTrusted: &trusted})
			if err != nil {
				t.Fatal(err)
			}
			core, _ := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
			model, _ := core.GetModel()
			core.SetResponses([]ai.FauxResponseStep{ai.FauxResponseFactory(func(input ai.Context, _ *ai.SimpleStreamOptions, _ *ai.FauxProviderState, _ ai.Model) (ai.AssistantMessage, error) {
				prompt, _ := input.SystemPrompt.Value()
				prefix := "GLOBAL\n\nGLOBAL_APPEND"
				if trusted {
					prefix = "PROJECT\n\nPROJECT_APPEND"
				}
				if !strings.HasPrefix(prompt, prefix) {
					t.Errorf("default SDK trust: %s", prompt)
				}
				return ai.FauxAssistantMessage(ai.FauxAssistantText("OK"))
			})})
			created, err := codingagent.CreateAgentSession(ctx, codingagent.CreateAgentSessionOptions{CWD: cwd, AgentDir: dir, Model: &model, StreamFunction: agent.StreamFunction(core.StreamSimple), NoTools: codingagent.NoToolsAll, SettingsManager: settings})
			if err != nil {
				t.Fatal(err)
			}
			defer created.Session.Dispose()
			if err = created.Session.Prompt(ctx, "hello"); err != nil {
				t.Fatal(err)
			}
			if got, err := created.Session.SettingsManager().IsProjectTrusted(); err != nil || got != trusted {
				t.Fatalf("settings trust differs: %v %v", got, err)
			}
		})
	}
}

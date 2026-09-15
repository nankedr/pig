package codingagent_test

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
)

func reload106Session(t *testing.T) (*codingagent.AgentSession, *reload106Loader, *ai.FauxCore, string, string) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("HOME", filepath.Join(root, "home"))
	cwd, dir := filepath.Join(root, "repo"), filepath.Join(root, "agent")
	context100Write(t, filepath.Join(cwd, "AGENTS.md"), "CONTEXT")
	context100Write(t, filepath.Join(dir, "SYSTEM.md"), "OLD")
	context100Write(t, filepath.Join(dir, "prompts/task.md"), "OLD $1")
	context100Write(t, filepath.Join(dir, "settings.json"), `{"compaction":{"enabled":false},"retry":{"enabled":false}}`)
	settings, err := codingagent.NewSettingsManager(cwd, &dir)
	if err != nil {
		t.Fatal(err)
	}
	base, err := codingagent.NewDefaultResourceLoader(codingagent.DefaultResourceLoaderOptions{CWD: cwd, AgentDir: dir, SettingsManager: settings})
	if err != nil {
		t.Fatal(err)
	}
	if err = base.Reload(context.Background()); err != nil {
		t.Fatal(err)
	}
	loader := &reload106Loader{ResourceLoader: base}
	core, err := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	model, _ := core.GetModel()
	sessionDir := filepath.Join(root, "sessions")
	manager, err := codingagent.NewSessionManager(cwd, &sessionDir)
	if err != nil {
		t.Fatal(err)
	}
	created, err := codingagent.CreateAgentSession(context.Background(), codingagent.CreateAgentSessionOptions{CWD: cwd, AgentDir: dir, Model: &model, StreamFunction: agent.StreamFunction(core.StreamSimple), ResourceLoader: loader, SessionManager: manager, SettingsManager: settings, Tools: []string{"read", "write"}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { created.Session.Dispose() })
	return created.Session, loader, core, cwd, dir
}

func TestSessionReloadCancelFailureAndRecovery(t *testing.T) {
	s, loader, core, _, dir := reload106Session(t)
	ctx := context.Background()
	original := s.SystemPrompt()
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err := s.Reload(canceled); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err := s.Reload(nil); err == nil {
		t.Fatal("nil context accepted")
	}
	for _, cooperative := range []bool{true, false} {
		entered := make(chan struct{})
		reloadCtx, cancel := context.WithCancel(ctx)
		loader.reload = func(ctx context.Context) error {
			close(entered)
			<-ctx.Done()
			if cooperative {
				return ctx.Err()
			}
			return nil
		}
		done := make(chan error, 1)
		go func() { done <- s.Reload(reloadCtx) }()
		<-entered
		if err := s.Reload(ctx); err == nil {
			t.Fatal("concurrent reload succeeded")
		}
		if err := s.Prompt(ctx, "/task x"); err == nil {
			t.Fatal("prompt entered during reload")
		}
		if err := s.Steer("/task x"); err == nil {
			t.Fatal("steer entered during reload")
		}
		if err := s.SetActiveToolsByName([]string{"read"}); err == nil {
			t.Fatal("configuration entered during reload")
		}
		cancel()
		if err := <-done; !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
		if s.SystemPrompt() != original {
			t.Fatal("canceled reload published a prompt")
		}
	}
	loader.reload = nil
	loader.skillsError = errors.New("QUERY_FAILURE")
	if err := s.Reload(ctx); !errors.Is(err, loader.skillsError) {
		t.Fatal(err)
	}
	if s.SystemPrompt() != original {
		t.Fatal("failed prompt rebuild published state")
	}
	loader.skillsError = nil
	context100Write(t, filepath.Join(dir, "SYSTEM.md"), "RECOVERED")
	loader.reload = nil
	if err := s.Reload(ctx); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(s.SystemPrompt(), "RECOVERED") {
		t.Fatal(s.SystemPrompt())
	}
	if err := s.SetActiveToolsByName([]string{"read"}); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(s.SystemPrompt(), "RECOVERED") {
		t.Fatal("tool selection restored stale prompt options")
	}
	core.SetResponses([]ai.FauxResponseStep{ai.FauxResponseFactory(func(c ai.Context, _ *ai.SimpleStreamOptions, _ *ai.FauxProviderState, _ ai.Model) (ai.AssistantMessage, error) {
		prompt, _ := c.SystemPrompt.Value()
		if !strings.HasPrefix(prompt, "RECOVERED") {
			t.Error(c.SystemPrompt)
		}
		return ai.FauxAssistantMessage(ai.FauxAssistantText("CONTINUED"))
	})})
	if err := s.Prompt(ctx, "continue"); err != nil {
		t.Fatal(err)
	}
	if err := s.Dispose(); err != nil {
		t.Fatal(err)
	}
	if err := s.Reload(ctx); err == nil {
		t.Fatal("disposed reload succeeded")
	}
}

func TestSessionReloadSettingsTrustAndBranch(t *testing.T) {
	s, _, core, cwd, dir := reload106Session(t)
	ctx := context.Background()
	context100Write(t, filepath.Join(cwd, ".pig/SYSTEM.md"), "PROJECT")
	context100Write(t, filepath.Join(cwd, ".pig/prompts/project.md"), "PROJECT $1")
	context100Write(t, filepath.Join(cwd, ".pig/settings.json"), `{"prompts":["-prompts/project.md"]}`)
	context100Write(t, filepath.Join(dir, "settings.json"), `{"defaultProjectTrust":"always","prompts":["-prompts/task.md"],"compaction":{"enabled":false},"retry":{"enabled":false}}`)
	if err := s.Reload(ctx); err != nil {
		t.Fatal(err)
	}
	if trusted, _ := s.SettingsManager().IsProjectTrusted(); trusted {
		t.Fatal("reload changed established trust")
	}
	if prompts, err := s.PromptTemplates(); err != nil || len(prompts) != 0 {
		t.Fatalf("updated settings ignored: %v %v", prompts, err)
	}
	if strings.Contains(s.SystemPrompt(), "PROJECT") {
		t.Fatal("untrusted system prompt loaded")
	}
	if err := s.SettingsManager().SetProjectTrusted(true); err != nil {
		t.Fatal(err)
	}
	if err := s.Reload(ctx); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(s.SystemPrompt(), "PROJECT") {
		t.Fatal("trusted project not loaded")
	}
	context100Write(t, filepath.Join(cwd, ".pig/settings.json"), `{}`)
	if err := s.Reload(ctx); err != nil {
		t.Fatal(err)
	}
	if prompts, err := s.PromptTemplates(); err != nil || len(prompts) != 1 || prompts[0].Name != "project" {
		t.Fatalf("project settings stale: %v %v", prompts, err)
	}
	if err := s.SettingsManager().SetProjectTrusted(false); err != nil {
		t.Fatal(err)
	}
	if err := s.Reload(ctx); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(s.SystemPrompt(), "PROJECT") {
		t.Fatal("revoked project still in prompt")
	}
	firstReply, _ := ai.FauxAssistantMessage(ai.FauxAssistantText("first"))
	secondReply, _ := ai.FauxAssistantMessage(ai.FauxAssistantText("second"))
	branchReply, _ := ai.FauxAssistantMessage(ai.FauxAssistantText("branch"))
	core.SetResponses([]ai.FauxResponseStep{firstReply, secondReply, branchReply})
	if err := s.Prompt(ctx, "one"); err != nil {
		t.Fatal(err)
	}
	first := s.SessionManager().GetLeafID()
	if err := s.Prompt(ctx, "two"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.NavigateTree(ctx, *first); err != nil {
		t.Fatal(err)
	}
	tree, err := s.SessionManager().GetTree()
	if err != nil {
		t.Fatal(err)
	}
	messages := s.Messages()
	leaf := s.SessionManager().GetLeafID()
	if err := s.Reload(ctx); err != nil {
		t.Fatal(err)
	}
	after, err := s.SessionManager().GetTree()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(tree, after) || !reflect.DeepEqual(messages, s.Messages()) || !reflect.DeepEqual(leaf, s.SessionManager().GetLeafID()) {
		t.Fatal("reload changed branch")
	}
	if err := s.Prompt(ctx, "branch"); err != nil {
		t.Fatal(err)
	}
	reopened, err := codingagent.OpenSessionManager(*s.SessionFile(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(reopened.BuildSessionContext().Messages, s.Messages()) {
		t.Fatal("continued branch did not persist")
	}
}

func TestSessionReloadConcurrentQueries(t *testing.T) {
	s, loader, _, _, dir := reload106Session(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var readers sync.WaitGroup
	for range 4 {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for ctx.Err() == nil {
				s.State()
				s.PromptTemplates()
				loader.GetSkills()
				loader.GetThemes()
				s.GetActiveToolNames()
			}
		}()
	}
	for i := 0; i < 10; i++ {
		context100Write(t, filepath.Join(dir, "SYSTEM.md"), strings.Repeat("NEW", i+1))
		if err := s.Reload(ctx); err != nil {
			t.Fatal(err)
		}
	}
	cancel()
	readers.Wait()
}

func TestSessionReloadAddedSkillsThemesAndSettings(t *testing.T) {
	s, _, core, _, dir := reload106Session(t)
	ctx := context.Background()
	context100Write(t, filepath.Join(dir, "skills/added/SKILL.md"), "---\nname: added\ndescription: ADDED_SKILL\n---\nADDED_BODY")
	theme, err := codingagent.LoadBuiltinTheme("dark")
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(map[string]any{"name": "added", "colors": theme.ResolvedColors()})
	if err != nil {
		t.Fatal(err)
	}
	context100Write(t, filepath.Join(dir, "themes/added.json"), string(data))
	if err := s.Reload(ctx); err != nil {
		t.Fatal(err)
	}
	skills, err := s.ResourceLoader().GetSkills()
	if err != nil || len(skills.Skills) != 1 || skills.Skills[0].Name != "added" {
		t.Fatalf("added Skill missing: %+v %v", skills, err)
	}
	themes, err := s.ResourceLoader().GetThemes()
	if err != nil || len(themes.Themes) != 1 {
		t.Fatalf("added Theme missing: %+v %v", themes, err)
	}
	if _, err := codingagent.SelectTheme("added", themes); err != nil {
		t.Fatal(err)
	}
	response := func(want string) ai.FauxResponseFactory {
		return func(c ai.Context, _ *ai.SimpleStreamOptions, _ *ai.FauxProviderState, _ ai.Model) (ai.AssistantMessage, error) {
			if text := message85Text(c.Messages[len(c.Messages)-1]); !strings.Contains(text, want) {
				t.Errorf("request %q missing %q", text, want)
			}
			return ai.FauxAssistantMessage(ai.FauxAssistantText("OK"))
		}
	}
	core.SetResponses([]ai.FauxResponseStep{response("ADDED_BODY"), response("/skill:added")})
	if err := s.Prompt(ctx, "/skill:added"); err != nil {
		t.Fatal(err)
	}
	context100Write(t, filepath.Join(dir, "settings.json"), `{"skills":["-skills/added"],"themes":["-themes/added.json"],"compaction":{"enabled":false},"retry":{"enabled":false}}`)
	if err := s.Reload(ctx); err != nil {
		t.Fatal(err)
	}
	skills, err = s.ResourceLoader().GetSkills()
	if err != nil || len(skills.Skills) != 0 {
		t.Fatalf("disabled Skill remains: %+v %v", skills, err)
	}
	themes, err = s.ResourceLoader().GetThemes()
	if err != nil || len(themes.Themes) != 0 {
		t.Fatalf("disabled Theme remains: %+v %v", themes, err)
	}
	if strings.Contains(s.SystemPrompt(), "ADDED_SKILL") {
		t.Fatal("disabled Skill remains in prompt")
	}
	if err := s.Prompt(ctx, "/skill:added"); err != nil {
		t.Fatal(err)
	}
}

func TestSessionReloadRejectsUnassembledConstructor(t *testing.T) {
	s, loader, _, cwd, _ := reload106Session(t)
	direct := codingagent.NewAgentSession(codingagent.AgentSessionConfig{Agent: s.Agent(), CWD: cwd, ResourceLoader: loader, SettingsManager: s.SettingsManager(), InitialActiveToolNames: s.GetActiveToolNames()})
	defer direct.Dispose()
	called := false
	loader.reload = func(context.Context) error { called = true; return nil }
	before := s.SystemPrompt()
	assertCodingAgentNotImplemented(t, direct.Reload(context.Background()), "AgentSession.Reload")
	if called || before != s.SystemPrompt() {
		t.Fatal("unassembled reload published a partial runtime")
	}
}

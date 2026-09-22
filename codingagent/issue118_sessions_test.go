package codingagent_test

import (
	"context"
	"errors"
	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func sessionRuntime118(t *testing.T) (*codingagent.AgentSessionRuntime, string) {
	t.Helper()
	ctx := context.Background()
	cwd, dir, agentDir := t.TempDir(), t.TempDir(), t.TempDir()
	models, catalog, _ := config87Runtime(t)
	factory := func(ctx context.Context, o codingagent.CreateAgentSessionRuntimeOptions) (codingagent.CreateAgentSessionRuntimeResult, error) {
		model := catalog[0]
		thinking := agent.ThinkingLevel("off")
		if o.SessionManager != nil {
			state := o.SessionManager.BuildSessionContext()
			if state.Model != nil {
				m, ok, _ := models.GetModel(state.Model.Provider, state.Model.ModelID)
				if ok {
					model = m
				}
			}
			thinking = agent.ThinkingLevel(state.ThinkingLevel)
		}
		settings, _ := codingagent.NewInMemorySettingsManager(codingagent.Settings{})
		created, err := codingagent.CreateAgentSession(ctx, codingagent.CreateAgentSessionOptions{CWD: o.CWD, AgentDir: agentDir, SessionManager: o.SessionManager, Model: &model, ThinkingLevel: thinking, ModelRuntime: models, SettingsManager: settings, StreamFunction: func(ctx context.Context, m ai.Model, input ai.Context, opts ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
			core, _ := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{Provider: m.Provider, API: m.API})
			reply, _ := ai.FauxAssistantMessage(ai.FauxAssistantText("SESSION_DONE"))
			reply.Model = m.ID
			reply.Provider = m.Provider
			reply.API = m.API
			core.SetResponses([]ai.FauxResponseStep{reply})
			return core.StreamSimple(ctx, m, input, opts)
		}, NoTools: codingagent.NoToolsAll})
		return codingagent.CreateAgentSessionRuntimeResult{CreateAgentSessionResult: created, Services: codingagent.AgentSessionServices{CWD: o.CWD, AgentDir: agentDir}}, err
	}
	manager, err := codingagent.NewSessionManager(cwd, &dir)
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := codingagent.CreateAgentSessionRuntime(ctx, factory, codingagent.CreateAgentSessionRuntimeOptions{CWD: cwd, SessionManager: manager})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Dispose(ctx) })
	return runtime, dir
}
func TestSessionSelectionPersistence118(t *testing.T) {
	ctx := context.Background()
	runtime, dir := sessionRuntime118(t)
	old := runtime.Session()
	if err := old.Prompt(ctx, "source question"); err != nil {
		t.Fatal(err)
	}
	source := *old.SessionFile()
	if _, err := runtime.NewSession(ctx); err != nil {
		t.Fatal(err)
	}
	target := runtime.Session()
	models, _, _ := config87Runtime(t)
	model, _, _ := models.GetModel("deepseek", "deepseek-v4-pro")
	if err := target.SetModel(model); err != nil {
		t.Fatal(err)
	}
	if err := target.SetThinkingLevel("high"); err != nil {
		t.Fatal(err)
	}
	if err := target.SetSessionName("target named"); err != nil {
		t.Fatal(err)
	}
	if err := target.Prompt(ctx, "target question"); err != nil {
		t.Fatal(err)
	}
	targetPath := *target.SessionFile()
	if _, err := runtime.SwitchSession(ctx, source); err != nil {
		t.Fatal(err)
	}
	current := func(ctx context.Context) ([]codingagent.SessionInfo, error) {
		return codingagent.ListSessions(ctx, runtime.CWD(), codingagent.SessionListOptions{SessionDir: &dir})
	}
	var failure error
	selector, err := codingagent.NewSessionSelectorComponent(ctx, current, current, func(path string) { _, failure = runtime.SwitchSession(ctx, path) }, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"target named", "\r"} {
		if err = selector.HandleInput(key); err != nil {
			t.Fatal(err)
		}
	}
	if failure != nil {
		t.Fatal(failure)
	}
	selected := runtime.Session()
	if selected.Model().ID != "deepseek-v4-pro" || selected.ThinkingLevel() != "high" || selected.SessionName() == nil || *selected.SessionName() != "target named" {
		t.Fatalf("target configuration not restored: %s %s %v path %v want %s", selected.Model().ID, selected.ThinkingLevel(), selected.SessionName(), selected.SessionFile(), targetPath)
	}
	if err = selected.Prompt(ctx, "continued question"); err != nil {
		t.Fatal(err)
	}
	reopened, err := codingagent.OpenSessionManager(targetPath, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	var users []string
	for _, m := range reopened.BuildSessionContext().Messages {
		if m.MessageRole() == ai.MessageRoleUser {
			users = append(users, message85Text(m))
		}
	}
	if strings.Join(users, "|") != "target question|continued question" {
		t.Fatal(users)
	}
	if err = old.Prompt(ctx, "stale"); err == nil {
		t.Fatal("old Session still usable")
	}
}
func TestSessionSelectionFailureAndCancel118(t *testing.T) {
	ctx := context.Background()
	runtime, dir := sessionRuntime118(t)
	old := runtime.Session()
	if err := old.Prompt(ctx, "before failure"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"missing", "empty", "corrupt", "directory"} {
		path := filepath.Join(dir, name+".jsonl")
		switch name {
		case "empty":
			os.WriteFile(path, nil, 0600)
		case "corrupt":
			os.WriteFile(path, []byte("not JSON"), 0600)
		case "directory":
			os.Mkdir(path, 0700)
		}
		if _, err := runtime.SwitchSession(ctx, path); err == nil {
			t.Fatalf("accepted %s", name)
		}
		if runtime.Session() != old {
			t.Fatal("failure replaced Session")
		}
	}
	loader := func(ctx context.Context) ([]codingagent.SessionInfo, error) {
		return codingagent.ListSessions(ctx, runtime.CWD(), codingagent.SessionListOptions{SessionDir: &dir})
	}
	selected := false
	selector, err := codingagent.NewSessionSelectorComponent(ctx, loader, loader, func(string) { selected = true }, nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = selector.HandleInput("\x1b")
	_ = selector.HandleInput("\r")
	if selected || runtime.Session() != old {
		t.Fatal("cancel selected a Session")
	}
	if err = old.Prompt(ctx, "after failure"); err != nil {
		t.Fatal(err)
	}
	source := *old.SessionFile()
	replacement := codingagent.NewAgentSessionRuntime(old, runtime.Services(), func(context.Context, codingagent.CreateAgentSessionRuntimeOptions) (codingagent.CreateAgentSessionRuntimeResult, error) {
		return codingagent.CreateAgentSessionRuntimeResult{}, errors.New("factory failed")
	}, nil, nil)
	if _, err = replacement.SwitchSession(ctx, source); err == nil {
		t.Fatal("factory succeeded")
	}
	if err = old.Prompt(ctx, "after factory failure"); err != nil {
		t.Fatal(err)
	}
}
func TestSessionSelectorManagement118(t *testing.T) {
	ctx := context.Background()
	runtime, dir := sessionRuntime118(t)
	if err := runtime.Session().Prompt(ctx, "original"); err != nil {
		t.Fatal(err)
	}
	path := *runtime.Session().SessionFile()
	loader := func(ctx context.Context) ([]codingagent.SessionInfo, error) {
		return codingagent.ListSessions(ctx, runtime.CWD(), codingagent.SessionListOptions{SessionDir: &dir})
	}
	options := codingagent.SessionSelectorOptions{CurrentSessionFilePath: path, RenameSession: func(path, name string) error { return runtime.Session().SetSessionName(name) }}
	s, err := codingagent.NewSessionSelectorComponent(ctx, loader, loader, nil, nil, options)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"\x12", "renamed", "\r", "\x04", "\r"} {
		if err = s.HandleInput(key); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := s.Render(100)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(rows, "\n"), "Cannot delete the currently active session") {
		t.Fatal(rows)
	}
	reopened, err := codingagent.OpenSessionManager(path, nil, nil)
	if err != nil || reopened.GetSessionName() == nil || *reopened.GetSessionName() != "renamed" {
		t.Fatal("rename not persisted", err)
	}
	if _, err = runtime.NewSession(ctx); err != nil {
		t.Fatal(err)
	}
	s, err = codingagent.NewSessionSelectorComponent(ctx, loader, loader, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"\x04", "\x1b"} {
		_ = s.HandleInput(key)
	}
	if _, err = os.Stat(path); err != nil {
		t.Fatal("cancel deleted file", err)
	}
	for _, key := range []string{"\x04", "\r"} {
		_ = s.HandleInput(key)
	}
	if _, err = os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("delete did not remove selected file", err)
	}
}

package codingagent_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
	"github.com/nankedr/pig/internal/baseline"
	"github.com/nankedr/pig/internal/parity"
)

type reload106Loader struct {
	codingagent.ResourceLoader
	reload      func(context.Context) error
	skillsError error
}

func (l *reload106Loader) GetSkills() (codingagent.SkillLoadResult, error) {
	if l.skillsError != nil {
		return codingagent.SkillLoadResult{}, l.skillsError
	}
	return l.ResourceLoader.GetSkills()
}

func (l *reload106Loader) Reload(ctx context.Context, _ ...codingagent.ResourceLoaderReloadOptions) error {
	if l.reload != nil {
		return l.reload(ctx)
	}
	return l.ResourceLoader.Reload(ctx)
}

func TestSessionReloadSDKParity(t *testing.T) {
	lock, _, err := baseline.Load("../parity/baseline")
	if err != nil {
		t.Fatal(err)
	}
	fixture, err := parity.LoadFixture("../parity/oracle/fixtures/session-reload.json", parity.Baseline{ID: lock.BaselineID, Commit: lock.Upstream.Commit, Repository: lock.Upstream.Repository})
	if err != nil {
		t.Fatal(err)
	}
	var want map[string]any
	if err = json.Unmarshal(fixture.Observation.Outcome, &want); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	t.Setenv("HOME", filepath.Join(dir, "home"))
	t.Setenv("COLORTERM", "truecolor")
	cwd, agentDir := filepath.Join(dir, "repo"), filepath.Join(dir, "agent")
	write := func(p, text string) { context100Write(t, filepath.Join(dir, p), text) }
	dark, err := os.ReadFile("themes/dark.json")
	if err != nil {
		t.Fatal(err)
	}
	resources := func(version string) {
		write("agent/SYSTEM.md", version)
		write("repo/AGENTS.md", "CONTEXT_"+version)
		write("agent/APPEND_SYSTEM.md", "APPEND_"+version)
		write("agent/prompts/task.md", version+" $1")
		write("agent/skills/work/SKILL.md", "---\nname: work\ndescription: SKILL_"+version+"\n---\nBODY_"+version)
		var theme map[string]any
		if err := json.Unmarshal(dark, &theme); err != nil {
			t.Fatal(err)
		}
		theme["name"] = "local"
		accent := "#111111"
		if version == "TWO" {
			accent = "#222222"
		}
		theme["colors"].(map[string]any)["accent"] = accent
		data, _ := json.Marshal(theme)
		write("agent/themes/local.json", string(data))
	}
	resources("ONE")
	write("agent/settings.json", `{"compaction":{"enabled":false},"retry":{"enabled":false}}`)
	settings, err := codingagent.NewSettingsManager(cwd, &agentDir)
	if err != nil {
		t.Fatal(err)
	}
	base, err := codingagent.NewDefaultResourceLoader(codingagent.DefaultResourceLoaderOptions{CWD: cwd, AgentDir: agentDir, SettingsManager: settings})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err = base.Reload(ctx); err != nil {
		t.Fatal(err)
	}
	loader := &reload106Loader{ResourceLoader: base}
	core, _ := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
	model, _ := core.GetModel()
	model.Reasoning = true
	sessionDir := filepath.Join(dir, "sessions")
	manager, err := codingagent.NewSessionManager(cwd, &sessionDir)
	if err != nil {
		t.Fatal(err)
	}
	created, err := codingagent.CreateAgentSession(ctx, codingagent.CreateAgentSessionOptions{CWD: cwd, AgentDir: agentDir, Model: &model, StreamFunction: agent.StreamFunction(core.StreamSimple), SessionManager: manager, SettingsManager: settings, ResourceLoader: loader, Tools: []string{"read"}, ThinkingLevel: "high"})
	if err != nil {
		t.Fatal(err)
	}
	s := created.Session
	defer s.Dispose()
	states, requests := []map[string]any{}, []map[string]any{}
	capture := func(label string) {
		prompts, err := s.PromptTemplates()
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
		pp, ss, tt := []string{}, []string{}, []string{}
		for _, p := range prompts {
			pp = append(pp, p.Name)
		}
		for _, skill := range skills.Skills {
			ss = append(ss, skill.Description)
		}
		for _, theme := range themes.Themes {
			tt = append(tt, theme.FG("accent", "X"))
		}
		states = append(states, map[string]any{"label": label, "prompts": pp, "skills": ss, "themes": tt, "system": s.SystemPrompt(), "steering": s.SteeringMode(), "followUp": s.FollowUpMode()})
	}
	request := func(text string) {
		core.SetResponses([]ai.FauxResponseStep{ai.FauxResponseFactory(func(c ai.Context, _ *ai.SimpleStreamOptions, _ *ai.FauxProviderState, _ ai.Model) (ai.AssistantMessage, error) {
			m := c.Messages[len(c.Messages)-1].(ai.UserMessage)
			requests = append(requests, map[string]any{"system": c.SystemPrompt, "text": m.Content, "history": len(c.Messages)})
			return ai.FauxAssistantMessage(ai.FauxAssistantText("OK"))
		})})
		if err := s.Prompt(ctx, text); err != nil {
			t.Fatal(err)
		}
	}
	snapshot := func() map[string]any {
		return map[string]any{"messages": s.Messages(), "entries": manager.GetEntries(), "leaf": manager.GetLeafID(), "file": manager.GetSessionFile(), "id": s.SessionID(), "model": s.Model(), "thinking": s.ThinkingLevel(), "tools": s.GetActiveToolNames()}
	}
	capture("initial")
	request("/task ARG")
	before := snapshot()
	resources("TWO")
	write("agent/prompts/added.md", "ADDED $1")
	write("agent/settings.json", `{"compaction":{"enabled":false},"retry":{"enabled":false},"steeringMode":"all","followUpMode":"all"}`)
	if err = s.Reload(ctx); err != nil {
		t.Fatal(err)
	}
	capture("updated")
	preserved := map[string]bool{}
	after := snapshot()
	for k, v := range before {
		preserved[k] = reflect.DeepEqual(v, after[k])
	}
	request("/task ARG")
	request("/skill:work ARG")
	request("/added ARG")
	for _, p := range []string{"agent/SYSTEM.md", "agent/APPEND_SYSTEM.md", "repo/AGENTS.md", "agent/prompts/task.md", "agent/skills/work/SKILL.md", "agent/themes/local.json"} {
		if err := os.Remove(filepath.Join(dir, p)); err != nil {
			t.Fatal(err)
		}
	}
	if err = s.Reload(ctx); err != nil {
		t.Fatal(err)
	}
	capture("deleted")
	request("/task ARG")
	request("/skill:work ARG")
	write("agent/settings.json", `{"steeringMode":"one-at-a-time","followUpMode":"one-at-a-time","compaction":{"enabled":false},"retry":{"enabled":false}}`)
	loader.reload = func(context.Context) error { return errors.New("RESOURCE_FAILURE") }
	failure := ""
	if err = s.Reload(ctx); err != nil {
		failure = err.Error()
	}
	capture("failed")
	write("agent/SYSTEM.md", "ACTIVE")
	entered, release := make(chan struct{}), make(chan struct{})
	var activeInput string
	core.SetResponses([]ai.FauxResponseStep{ai.FauxResponseFactory(func(c ai.Context, _ *ai.SimpleStreamOptions, _ *ai.FauxProviderState, _ ai.Model) (ai.AssistantMessage, error) {
		activeInput, _ = c.SystemPrompt.Value()
		close(entered)
		<-release
		return ai.FauxAssistantMessage(ai.FauxAssistantText("ACTIVE_OK"))
	})})
	turn := make(chan error, 1)
	go func() { turn <- s.Prompt(ctx, "active") }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("turn never started")
	}
	loadEntered, loadRelease := make(chan struct{}), make(chan struct{})
	loader.reload = func(ctx context.Context) error { close(loadEntered); <-loadRelease; return base.Reload(ctx) }
	reloaded := make(chan error, 1)
	go func() { reloaded <- s.Reload(ctx) }()
	select {
	case <-loadEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("reload never started")
	}
	prompts, err := s.PromptTemplates()
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, p := range prompts {
		names = append(names, p.Name)
	}
	during := map[string]any{"system": s.SystemPrompt(), "prompts": names, "streaming": s.IsStreaming()}
	close(loadRelease)
	if err := <-reloaded; err != nil {
		t.Fatal(err)
	}
	active := map[string]any{"during": during, "after": s.SystemPrompt(), "streaming": s.IsStreaming(), "input": activeInput}
	close(release)
	if err := <-turn; err != nil {
		t.Fatal(err)
	}
	active["reply"], err = s.GetLastAssistantText()
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]any{"states": states, "requests": requests, "preserved": preserved, "failure": failure, "active": active})
	var got map[string]any
	if err = json.Unmarshal([]byte(strings.ReplaceAll(string(raw), dir, "$ROOT")), &got); err != nil {
		t.Fatal(err)
	}
	for k, v := range want {
		if !reflect.DeepEqual(got[k], v) {
			g, _ := json.MarshalIndent(got[k], "", "  ")
			w, _ := json.MarshalIndent(v, "", "  ")
			t.Errorf("%s:\nPig %s\nPi %s", k, g, w)
		}
	}
}

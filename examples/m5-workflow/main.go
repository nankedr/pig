package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
)

func main() {
	for _, mode := range []string{"trusted", "denied", "explicit"} {
		run(mode)
	}
	fmt.Println("PASS: local resources, template, skill, reload, persistence, theme and HTML export")
}

func run(mode string) {
	ctx := context.Background()
	root, err := os.MkdirTemp("", "pig-m5-"+mode+"-")
	must(err)
	defer os.RemoveAll(root)
	cwd, dir := filepath.Join(root, "repo"), filepath.Join(root, "agent")
	write := func(path, content string) {
		must(os.MkdirAll(filepath.Dir(path), 0700))
		must(os.WriteFile(path, []byte(content), 0600))
	}
	write(filepath.Join(cwd, "AGENTS.md"), "CONTEXT_ALWAYS_LOADED")
	write(filepath.Join(cwd, ".pig/SYSTEM.md"), "TRUSTED_PROJECT_SYSTEM")
	write(filepath.Join(dir, "SYSTEM.md"), "GLOBAL_SYSTEM")
	write(filepath.Join(dir, "settings.json"), `{"theme":"local","compaction":{"enabled":false},"retry":{"enabled":false}}`)
	theme, err := codingagent.LoadBuiltinTheme("light")
	must(err)
	resourceDir := dir
	if mode == "trusted" {
		resourceDir = filepath.Join(cwd, ".pig")
	}
	if mode == "explicit" {
		resourceDir = filepath.Join(root, "explicit")
	}
	resources := func(version, color string) {
		write(filepath.Join(resourceDir, "prompts/review.md"), version+" review $1")
		write(filepath.Join(resourceDir, "skills/check/SKILL.md"), "---\nname: check\ndescription: "+version+" checks\n---\n"+version+" SKILL_BODY")
		colors := theme.ResolvedColors()
		colors["accent"] = color
		data, err := json.Marshal(map[string]any{"name": "local", "colors": colors})
		must(err)
		write(filepath.Join(resourceDir, "themes/local.json"), string(data))
	}
	resources("ONE", "#112233")
	settings, err := codingagent.NewSettingsManager(cwd, &dir)
	must(err)
	must(settings.SetProjectTrusted(mode == "trusted"))
	options := codingagent.DefaultResourceLoaderOptions{CWD: cwd, AgentDir: dir, SettingsManager: settings}
	if mode == "explicit" {
		options.NoSkills, options.NoThemes, options.NoPromptTemplates = true, true, true
		options.AdditionalSkillPaths = []string{filepath.Join(resourceDir, "skills")}
		options.AdditionalPromptTemplatePaths = []string{filepath.Join(resourceDir, "prompts")}
		options.AdditionalThemePaths = []string{filepath.Join(resourceDir, "themes")}
	}
	loader, err := codingagent.NewDefaultResourceLoader(options)
	must(err)
	must(loader.Reload(ctx))
	core, err := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
	must(err)
	model, ok := core.GetModel()
	check(ok, "missing Faux model")
	sessions := filepath.Join(root, "sessions")
	manager, err := codingagent.NewSessionManager(cwd, &sessions)
	must(err)
	config := codingagent.CreateAgentSessionOptions{CWD: cwd, AgentDir: dir, Model: &model, Tools: []string{"read"}, StreamFunction: agent.StreamFunction(core.StreamSimple), SessionManager: manager, SettingsManager: settings, ResourceLoader: loader}
	created, err := codingagent.CreateAgentSession(ctx, config)
	must(err)
	session := created.Session
	defer session.Dispose()
	request := func(prompt, want string, history int) {
		core.SetResponses([]ai.FauxResponseStep{ai.FauxResponseFactory(func(c ai.Context, _ *ai.SimpleStreamOptions, _ *ai.FauxProviderState, _ ai.Model) (ai.AssistantMessage, error) {
			system, _ := c.SystemPrompt.Value()
			if !strings.Contains(system, "CONTEXT_ALWAYS_LOADED") || strings.Contains(system, "TRUSTED_PROJECT_SYSTEM") != (mode == "trusted") {
				return ai.AssistantMessage{}, fmt.Errorf("project trust/context mismatch: %s", system)
			}
			data, err := json.Marshal(c.Messages[len(c.Messages)-1])
			if err != nil {
				return ai.AssistantMessage{}, err
			}
			if len(c.Messages) != history || !strings.Contains(string(data), want) {
				return ai.AssistantMessage{}, fmt.Errorf("request history=%d, want=%d; input=%s", len(c.Messages), history, data)
			}
			return ai.FauxAssistantMessage(ai.FauxAssistantText("M5_OK"))
		})})
		must(session.Prompt(ctx, prompt))
	}
	request("/review main.go", "ONE review main.go", 1)
	request("/skill:check main.go", "ONE SKILL_BODY", 3)
	before, id, leaf, file := session.Messages(), session.SessionID(), *manager.GetLeafID(), *session.SessionFile()
	resources("TWO", "#445566")
	must(session.Reload(ctx))
	check(reflect.DeepEqual(before, session.Messages()) && id == session.SessionID() && leaf == *manager.GetLeafID() && file == *session.SessionFile(), "reload changed history or persistence")
	prompts, err := session.PromptTemplates()
	must(err)
	check(len(prompts) == 1 && prompts[0].Content == "TWO review $1", "stale template query")
	skills, err := loader.GetSkills()
	must(err)
	found := false
	for _, skill := range skills.Skills {
		if skill.Name == "check" && skill.Description == "TWO checks" {
			found = true
		}
	}
	check(found, "stale skill query")
	themes, err := loader.GetThemes()
	must(err)
	selected, err := codingagent.SelectTheme("local", themes)
	must(err)
	check(selected.ResolvedColors()["accent"] == "#445566", "stale theme query")
	request("/review main.go", "TWO review main.go", 5)
	request("/skill:check main.go", "TWO SKILL_BODY", 7)
	before = session.Messages()
	must(session.Dispose())
	manager, err = codingagent.OpenSessionManager(file, nil, nil)
	must(err)
	config.SessionManager = manager
	created, err = codingagent.CreateAgentSession(ctx, config)
	must(err)
	session = created.Session
	defer session.Dispose()
	check(id == session.SessionID() && reflect.DeepEqual(before, session.Messages()), "reopen changed persisted context")
	request("/review resumed.go", "TWO review resumed.go", 9)
	output, err := session.ExportToHTML(ctx, filepath.Join(root, "session.html"))
	must(err)
	html, err := os.ReadFile(output)
	must(err)
	encoded := strings.SplitN(string(html), `<script id="session-data" type="application/json">`, 2)
	check(len(encoded) == 2, "missing embedded session")
	transcript, err := base64.StdEncoding.DecodeString(strings.SplitN(encoded[1], "</script>", 2)[0])
	must(err)
	check(strings.Contains(string(html), "--accent: #445566;") && strings.Contains(string(transcript), "M5_OK"), "HTML lost theme or transcript")
	fmt.Printf("%s: template → skill → reload → reopen → HTML: PASS\n", mode)
}

func check(ok bool, message string) {
	if !ok {
		panic(message)
	}
}
func must(err error) {
	if err != nil {
		panic(err)
	}
}

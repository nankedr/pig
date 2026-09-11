package codingagent_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
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

func TestContextFilesSDKParity(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("fixture requires POSIX permissions and non-root user")
	}
	lock, _, err := baseline.Load("../parity/baseline")
	if err != nil {
		t.Fatal(err)
	}
	pinned := parity.Baseline{ID: lock.BaselineID, Commit: lock.Upstream.Commit, Repository: lock.Upstream.Repository}
	fixture, err := parity.LoadFixture("../parity/oracle/fixtures/context-files.json", pinned)
	if err != nil {
		t.Fatal(err)
	}
	oracle, err := parity.NewFixtureDriver(fixture, pinned)
	if err != nil {
		t.Fatal(err)
	}
	for _, injected := range []bool{false, true} {
		t.Run(fmt.Sprintf("injected=%v", injected), func(t *testing.T) {
			result, err := parity.RunCase(context.Background(), fixture.Case, oracle, parity.DriverFunc{SurfaceName: parity.SurfaceGoSDK, ObserveFunc: func(ctx context.Context, c parity.Case) (parity.Observation, error) {
				var input struct {
					Scenarios []struct {
						Name                 string
						Files, Links         map[string]string
						Unreadable, AgentDir string
						Disabled, FileURL    bool
					}
				}
				if err := json.Unmarshal(c.Input, &input); err != nil {
					return parity.Observation{}, err
				}
				outcomes := []map[string]any{}
				for _, scenario := range input.Scenarios {
					dir := t.TempDir()
					for path, content := range scenario.Files {
						context100Write(t, filepath.Join(dir, path), content)
					}
					for path, target := range scenario.Links {
						full := filepath.Join(dir, path)
						if err := os.MkdirAll(filepath.Dir(full), 0700); err != nil {
							t.Fatal(err)
						}
						if err := os.Symlink(target, full); err != nil {
							t.Fatal(err)
						}
					}
					cwd, agentDir := filepath.Join(dir, "repo/child"), filepath.Join(dir, "agent")
					if scenario.AgentDir != "" {
						agentDir = filepath.Join(dir, scenario.AgentDir)
					}
					for _, path := range []string{cwd, agentDir} {
						if err := os.MkdirAll(path, 0700); err != nil {
							t.Fatal(err)
						}
					}
					if scenario.Unreadable != "" {
						path := filepath.Join(dir, scenario.Unreadable)
						if err := os.Chmod(path, 0); err != nil {
							t.Fatal(err)
						}
						t.Cleanup(func() { _ = os.Chmod(path, 0600) })
					}
					if scenario.FileURL {
						cwd = (&url.URL{Scheme: "file", Path: cwd}).String()
					}
					core, err := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
					if err != nil {
						return parity.Observation{}, err
					}
					received := []string{}
					response := ai.FauxResponseFactory(func(input ai.Context, _ *ai.SimpleStreamOptions, _ *ai.FauxProviderState, _ ai.Model) (ai.AssistantMessage, error) {
						prompt, _ := input.SystemPrompt.Value()
						received = append(received, prompt)
						return ai.FauxAssistantMessage(ai.FauxAssistantText("CONTEXT_OK"))
					})
					core.SetResponses([]ai.FauxResponseStep{response, response})
					model, _ := core.GetModel()
					options := codingagent.CreateAgentSessionOptions{CWD: cwd, AgentDir: agentDir, Model: &model, StreamFunction: agent.StreamFunction(core.StreamSimple), NoTools: codingagent.NoToolsAll, NoContextFiles: scenario.Disabled}
					if injected {
						loader, err := codingagent.NewDefaultResourceLoader(codingagent.DefaultResourceLoaderOptions{CWD: cwd, AgentDir: agentDir, NoContextFiles: scenario.Disabled})
						if err != nil {
							return parity.Observation{}, err
						}
						if err = loader.Reload(ctx); err != nil {
							return parity.Observation{}, err
						}
						options.ResourceLoader = loader
					}
					created, err := codingagent.CreateAgentSession(ctx, options)
					if err != nil {
						return parity.Observation{}, err
					}
					s := created.Session
					if s.ResourceLoader() == nil {
						s.Dispose()
						return parity.Observation{}, fmt.Errorf("%s: session has no default resource loader", scenario.Name)
					}
					files, err := s.ResourceLoader().GetAgentsFiles()
					if err != nil {
						s.Dispose()
						return parity.Observation{}, err
					}
					if err = s.Prompt(ctx, "first"); err != nil {
						s.Dispose()
						return parity.Observation{}, err
					}
					if err = s.Prompt(ctx, "next"); err != nil {
						s.Dispose()
						return parity.Observation{}, err
					}
					last, lastErr := s.GetLastAssistantText()
					if len(received) != 2 || received[0] != received[1] || lastErr != nil || last == nil || *last != "CONTEXT_OK" {
						t.Fatalf("generation did not retain context: %v", received)
					}
					projected := []map[string]string{}
					inPrompt := true
					position := -1
					for _, f := range files {
						if !strings.HasPrefix(f.Path, dir+string(filepath.Separator)) {
							continue
						}
						path := filepath.ToSlash(strings.TrimPrefix(f.Path, dir+string(filepath.Separator)))
						if scenario.Name == "uppercase" {
							path = strings.ToLower(path)
						}
						projected = append(projected, map[string]string{"path": path, "content": f.Content})
						block := fmt.Sprintf("<project_instructions path=\"%s\">\n%s\n</project_instructions>", f.Path, f.Content)
						index := strings.Index(received[0], block)
						inPrompt = inPrompt && index > position
						position = index
					}
					if scenario.Disabled && strings.Contains(received[0], "<project_context>") {
						t.Fatal("disabled context entered generation")
					}
					warning := false
					if source, ok := s.ResourceLoader().(interface {
						GetContextFileDiagnostics() []codingagent.ResourceDiagnostic
					}); ok {
						warning = len(source.GetContextFileDiagnostics()) > 0
					}
					outcomes = append(outcomes, map[string]any{"name": scenario.Name, "files": projected, "inPrompt": inPrompt, "warning": warning})
					s.Dispose()
				}
				outcome, err := json.Marshal(outcomes)
				effects := []parity.SideEffect{}
				return parity.Observation{Outcome: outcome, SideEffects: &effects}, err
			}})
			if err != nil || !result.Match {
				t.Fatalf("context parity: %v %v\nPig=%s\nPi=%s", err, result.Differences, result.Pig.Outcome, result.Oracle.Outcome)
			}
		})
	}
}

func context100Write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

type context100Loader struct {
	codingagent.ResourceLoader
	files []codingagent.AgentsFile
	err   error
}

func (l context100Loader) GetAgentsFiles() ([]codingagent.AgentsFile, error) { return l.files, l.err }

func TestContextFilesInjectedLoaderAndDisable(t *testing.T) {
	for _, disabled := range []bool{false, true} {
		core, _ := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
		response := ai.FauxResponseFactory(func(input ai.Context, _ *ai.SimpleStreamOptions, _ *ai.FauxProviderState, _ ai.Model) (ai.AssistantMessage, error) {
			prompt, _ := input.SystemPrompt.Value()
			if strings.Contains(prompt, "INJECTED") == disabled {
				t.Errorf("disabled=%v prompt=%s", disabled, prompt)
			}
			return ai.FauxAssistantMessage(ai.FauxAssistantText("reply"))
		})
		core.SetResponses([]ai.FauxResponseStep{response})
		model, _ := core.GetModel()
		loader := context100Loader{files: []codingagent.AgentsFile{{Path: "virtual/AGENTS.md", Content: "INJECTED"}}}
		s, err := codingagent.CreateAgentSession(context.Background(), codingagent.CreateAgentSessionOptions{CWD: t.TempDir(), Model: &model, StreamFunction: agent.StreamFunction(core.StreamSimple), NoTools: codingagent.NoToolsAll, ResourceLoader: loader, NoContextFiles: disabled})
		if err != nil {
			t.Fatal(err)
		}
		if err = s.Session.Prompt(context.Background(), "hello"); err != nil {
			t.Fatal(err)
		}
		s.Session.Dispose()
	}
	core, _ := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
	model, _ := core.GetModel()
	_, err := codingagent.CreateAgentSession(context.Background(), codingagent.CreateAgentSessionOptions{CWD: t.TempDir(), Model: &model, StreamFunction: agent.StreamFunction(core.StreamSimple), ResourceLoader: context100Loader{err: fmt.Errorf("resource failure")}})
	if err == nil || !strings.Contains(err.Error(), "resource failure") {
		t.Fatalf("loader failure: %v", err)
	}
}

func TestContextFilesReloadAndOwnership(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "AGENTS.md")
	context100Write(t, path, "BEFORE")
	loader, err := codingagent.NewDefaultResourceLoader(codingagent.DefaultResourceLoaderOptions{CWD: dir, AgentDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if files, err := loader.GetAgentsFiles(); err != nil || len(files) != 0 {
		t.Fatalf("constructor should not load: %v %v", files, err)
	}
	if err = loader.Reload(ctx); err != nil {
		t.Fatal(err)
	}
	files, _ := loader.GetAgentsFiles()
	files[0].Content = "MUTATED"
	context100Write(t, path, "AFTER")
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err = loader.Reload(canceled); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
	before, _ := loader.GetAgentsFiles()
	if before[0].Content != "BEFORE" {
		t.Fatalf("snapshot mutated: %v", before)
	}
	if err = loader.Reload(ctx); err != nil {
		t.Fatal(err)
	}
	after, _ := loader.GetAgentsFiles()
	if after[0].Content != "AFTER" {
		t.Fatalf("reload: %v", after)
	}
	for _, fn := range []func() error{
		func() error { _, err := loader.GetSkills(); return err }, func() error { _, err := loader.GetPrompts(); return err }, func() error { _, err := loader.GetThemes(); return err }, func() error { _, err := loader.GetExtensions(); return err }, func() error { _, err := loader.GetSystemPrompt(); return err },
	} {
		if err := fn(); !errors.Is(err, codingagent.ErrNotImplemented) {
			t.Fatalf("unsupported query: %v", err)
		}
	}
}

func TestContextFilesSessionServices(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	agentDir := t.TempDir()
	context100Write(t, filepath.Join(dir, "AGENTS.md"), "SERVICES_CONTEXT")
	models, _, _ := config87Runtime(t)
	received := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []struct{ Role, Content string }
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		for _, m := range body.Messages {
			if m.Role == "system" {
				received <- m.Content
			}
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"services reply\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	model, _, _ := models.GetModel("deepseek", "deepseek-v4-flash")
	model.BaseURL = server.URL
	settings, _ := codingagent.NewInMemorySettingsManager(codingagent.Settings{})
	services, err := codingagent.CreateAgentSessionServices(ctx, codingagent.CreateAgentSessionServicesOptions{CWD: dir, AgentDir: agentDir, ModelRuntime: models, SettingsManager: settings, ResourceLoaderOptions: codingagent.DefaultResourceLoaderOptions{NoExtensions: true}})
	if err != nil {
		t.Fatal(err)
	}
	created, err := codingagent.CreateAgentSessionFromServices(ctx, codingagent.CreateAgentSessionFromServicesOptions{Services: services, Model: &model, NoTools: codingagent.NoToolsAll})
	if err != nil {
		t.Fatal(err)
	}
	defer created.Session.Dispose()
	if created.Session.ResourceLoader() != services.ResourceLoader {
		t.Fatal("services loader replaced")
	}
	if err = created.Session.Prompt(ctx, "hello"); err != nil {
		t.Fatal(err)
	}
	select {
	case prompt := <-received:
		if !strings.Contains(prompt, "SERVICES_CONTEXT") {
			t.Fatalf("missing services context: %s", prompt)
		}
	default:
		t.Fatal("no request")
	}
}

package codingagent_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
)

func TestSessionConfigurationRuntimeAndRestore(t *testing.T) {
	runtime, _, store := config87Runtime(t)
	models := []ai.Model{}
	for _, id := range []string{"deepseek-v4-flash", "deepseek-v4-pro"} {
		m, _, _ := runtime.GetModel("deepseek", id)
		models = append(models, m)
	}
	type request struct {
		Model, Key, Thinking string
		Tools                []string
		Prompt               string
	}
	requests := []request{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model     string `json:"model"`
			Reasoning string `json:"reasoning_effort"`
			Tools     []struct{ Function struct{ Name string } }
			Messages  []struct {
				Role    string
				Content json.RawMessage
			}
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		got := request{Model: body.Model, Key: r.Header.Get("Authorization"), Thinking: body.Reasoning, Tools: []string{}}
		for _, tool := range body.Tools {
			got.Tools = append(got.Tools, tool.Function.Name)
		}
		for _, m := range body.Messages {
			if m.Role == "system" {
				_ = json.Unmarshal(m.Content, &got.Prompt)
			}
		}
		requests = append(requests, got)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	for i := range models {
		models[i].BaseURL = server.URL
	}
	dir := t.TempDir()
	sessionDir := t.TempDir()
	manager, err := codingagent.NewSessionManager(dir, &sessionDir)
	if err != nil {
		t.Fatal(err)
	}
	settings, _ := codingagent.NewInMemorySettingsManager(codingagent.Settings{})
	created, err := codingagent.CreateAgentSession(context.Background(), codingagent.CreateAgentSessionOptions{CWD: dir, AgentDir: t.TempDir(), Model: &models[0], ModelRuntime: runtime, SessionManager: manager, SettingsManager: settings, ThinkingLevel: "off"})
	if err != nil {
		t.Fatal(err)
	}
	s := created.Session
	defer s.Dispose()
	if err = s.Prompt(context.Background(), "before"); err != nil {
		t.Fatal(err)
	}
	if err = s.SetModel(models[1]); err != nil {
		t.Fatal(err)
	}
	if err = s.SetThinkingLevel("max"); err != nil {
		t.Fatal(err)
	}
	if err = s.SetActiveToolsByName([]string{"edit", "write"}); err != nil {
		t.Fatal(err)
	}
	credential76Set(t, store, "deepseek", "rotated")
	if err = s.Prompt(context.Background(), "after"); err != nil {
		t.Fatal(err)
	}
	if len(requests) != 2 || requests[0].Model != "deepseek-v4-flash" || requests[1].Model != "deepseek-v4-pro" || requests[1].Key != "Bearer rotated" || requests[1].Thinking != "max" || !reflect.DeepEqual(requests[1].Tools, []string{"edit", "write"}) || strings.Contains(requests[1].Prompt, "- read:") || !strings.Contains(requests[1].Prompt, "- edit:") {
		t.Fatalf("requests: %+v", requests)
	}
	saved, err := codingagent.OpenSessionManager(*s.SessionFile(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := codingagent.CreateAgentSession(context.Background(), codingagent.CreateAgentSessionOptions{CWD: dir, AgentDir: t.TempDir(), ModelRuntime: runtime, SessionManager: saved, SettingsManager: settings})
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Session.Dispose()
	if restored.Session.Model().ID != "deepseek-v4-pro" || restored.Session.ThinkingLevel() != "max" || !reflect.DeepEqual(restored.Session.GetActiveToolNames(), []string{"read", "bash", "edit", "write"}) {
		t.Fatalf("restored: %+v", restored.Session.State())
	}
	unsupported, _, _ := runtime.GetModel("openai", "gpt-4o")
	before := config87State(s)
	count := len(manager.GetEntries())
	if err = s.SetModel(unsupported); !errors.Is(err, codingagent.ErrNotImplemented) {
		t.Fatalf("adapter: %v", err)
	}
	if !reflect.DeepEqual(before, config87State(s)) || len(manager.GetEntries()) != count {
		t.Fatal("unsupported adapter partially switched")
	}
}

func TestSessionConfigurationFailuresAndBusy(t *testing.T) {
	runtime, models, store := config87Runtime(t)
	entered, release := make(chan struct{}), make(chan struct{})
	core, _ := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
	message, _ := ai.FauxAssistantMessage(ai.FauxAssistantText("ok"))
	core.SetResponses([]ai.FauxResponseStep{ai.FauxResponseFactory(func(ai.Context, *ai.SimpleStreamOptions, *ai.FauxProviderState, ai.Model) (ai.AssistantMessage, error) {
		close(entered)
		<-release
		return message, nil
	})})
	dir := t.TempDir()
	manager := codingagent.NewInMemorySessionManager(dir)
	created, err := codingagent.CreateAgentSession(context.Background(), codingagent.CreateAgentSessionOptions{CWD: dir, Model: &models[0], ModelRuntime: runtime, StreamFunction: agent.StreamFunction(core.StreamSimple), SessionManager: manager})
	if err != nil {
		t.Fatal(err)
	}
	s := created.Session
	defer s.Dispose()
	invalid := models[0]
	invalid.ID = "missing"
	checks := []func() error{
		func() error { return s.SetModel(invalid) }, func() error { return s.SetThinkingLevel("nonsense") }, func() error { return s.SetActiveToolsByName([]string{"read", "unknown"}) },
		func() error {
			return s.SetScopedModels([]codingagent.ScopedModel{{Model: models[0], ThinkingLevel: "invalid"}})
		},
		func() error { _, e := s.CycleModel(context.Background(), "sideways"); return e },
	}
	before := config87State(s)
	count := len(manager.GetEntries())
	for _, check := range checks {
		if check() == nil {
			t.Fatal("invalid configuration succeeded")
		}
		if !reflect.DeepEqual(before, config87State(s)) || len(manager.GetEntries()) != count {
			t.Fatal("invalid configuration mutated state")
		}
	}
	// Auth is resolved again when switching, even with a previously available snapshot.
	err = store.Delete(context.Background(), "deepseek", ai.AuthOperationOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("DEEPSEEK_API_KEY", "")
	if err = s.SetModel(models[1]); err == nil {
		t.Fatal("switch without auth succeeded")
	}
	credential76Set(t, store, "deepseek", "fixture")
	done := make(chan error, 1)
	go func() { done <- s.Prompt(context.Background(), "busy") }()
	<-entered
	checks = []func() error{func() error { return s.SetModel(models[1]) }, func() error { return s.SetThinkingLevel("high") }, func() error { return s.SetActiveToolsByName([]string{}) }, func() error { return s.SetScopedModels(nil) }, func() error { _, e := s.CycleThinkingLevel(); return e }, func() error { _, e := s.CycleModel(context.Background()); return e }}
	for _, check := range checks {
		if e := check(); e == nil || !strings.Contains(e.Error(), "busy") {
			t.Errorf("busy configuration: %v", e)
		}
	}
	close(release)
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	if err = s.SetModel(models[1]); err != nil {
		t.Fatal(err)
	}
}

func TestSessionConfigurationToolRulesAndSnapshots(t *testing.T) {
	for _, tc := range []struct {
		name           string
		tools, exclude []string
		mode           codingagent.NoToolsMode
		allowed        bool
	}{
		{name: "all", allowed: true}, {name: "allowed", tools: []string{"read"}}, {name: "excluded", exclude: []string{"write"}}, {name: "none", mode: codingagent.NoToolsAll}, {name: "builtin-disabled", mode: codingagent.NoToolsBuiltin, allowed: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			core, _ := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
			m, _ := core.GetModel()
			created, err := codingagent.CreateAgentSession(context.Background(), codingagent.CreateAgentSessionOptions{CWD: t.TempDir(), Model: &m, StreamFunction: agent.StreamFunction(core.StreamSimple), Tools: tc.tools, ExcludeTools: tc.exclude, NoTools: tc.mode})
			if err != nil {
				t.Fatal(err)
			}
			s := created.Session
			defer s.Dispose()
			err = s.SetActiveToolsByName([]string{"write"})
			if (err == nil) != tc.allowed {
				t.Fatalf("write: %v", err)
			}
			if tc.name == "all" {
				names := []string{}
				for _, tool := range s.GetAllTools() {
					names = append(names, tool.Name)
				}
				if !reflect.DeepEqual(names, []string{"read", "bash", "edit", "write"}) {
					t.Fatalf("registry order: %v", names)
				}
			}
			before := s.GetAllTools()
			if len(before) > 0 {
				before[0].Name = "changed"
				if s.GetAllTools()[0].Name == "changed" {
					t.Fatal("registry aliases caller")
				}
			}
		})
	}
}

func TestSessionConfigurationConcurrentSnapshots(t *testing.T) {
	runtime, models, _ := config87Runtime(t)
	core, _ := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
	created, err := codingagent.CreateAgentSession(context.Background(), codingagent.CreateAgentSessionOptions{CWD: t.TempDir(), Model: &models[0], ModelRuntime: runtime, StreamFunction: agent.StreamFunction(core.StreamSimple)})
	if err != nil {
		t.Fatal(err)
	}
	s := created.Session
	defer s.Dispose()
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				switch i {
				case 0:
					_ = s.SetModel(models[j%2])
				case 1:
					_ = s.SetThinkingLevel("high")
				case 2:
					_ = s.SetActiveToolsByName([]string{"read"})
					_ = s.SetScopedModels([]codingagent.ScopedModel{{Model: models[0]}})
				case 3:
					state := s.State()
					if !state.Model.Reasoning && state.ThinkingLevel != "off" {
						t.Error("half model/thinking snapshot")
					}
					_ = s.GetActiveToolNames()
					_ = s.GetAllTools()
					scope := s.ScopedModels()
					if len(scope) > 0 {
						scope[0].Model.ID = "mutated"
					}
				}
			}
		}(i)
	}
	wg.Wait()
}

type config87Storage struct {
	value string
	fail  bool
}

func (s *config87Storage) WithLock(_ codingagent.SettingsScope, fn func(*string) *string) {
	if s.fail {
		panic("settings failed")
	}
	if next := fn(&s.value); next != nil {
		s.value = *next
	}
}

func TestSessionConfigurationPersistenceFailure(t *testing.T) {
	runtime, models, _ := config87Runtime(t)
	core, _ := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
	dir := t.TempDir()
	sessionDir := t.TempDir()
	manager, err := codingagent.NewSessionManager(dir, &sessionDir)
	if err != nil {
		t.Fatal(err)
	}
	storage := &config87Storage{value: "{}"}
	settings, _ := codingagent.NewSettingsManagerFromStorage(storage)
	created, err := codingagent.CreateAgentSession(context.Background(), codingagent.CreateAgentSessionOptions{CWD: dir, Model: &models[0], ModelRuntime: runtime, StreamFunction: agent.StreamFunction(core.StreamSimple), SettingsManager: settings, SessionManager: manager})
	if err != nil {
		t.Fatal(err)
	}
	s := created.Session
	defer s.Dispose()
	message, _ := ai.FauxAssistantMessage(ai.FauxAssistantText("persist"))
	core.SetResponses([]ai.FauxResponseStep{ai.FauxResponseFactory(func(ai.Context, *ai.SimpleStreamOptions, *ai.FauxProviderState, ai.Model) (ai.AssistantMessage, error) {
		return message, nil
	})})
	if err = s.Prompt(context.Background(), "save"); err != nil {
		t.Fatal(err)
	}
	before := config87State(s)
	entries := manager.GetEntries()
	data, err := os.ReadFile(*s.SessionFile())
	if err != nil {
		t.Fatal(err)
	}
	storage.fail = true
	if err = s.SetModel(models[1]); err == nil {
		t.Fatal("settings failure hidden")
	}
	after, _ := os.ReadFile(*s.SessionFile())
	if !reflect.DeepEqual(before, config87State(s)) || !reflect.DeepEqual(entries, manager.GetEntries()) || string(data) != string(after) {
		t.Fatal("failed settings write partially switched")
	}
	storage.fail = false
	// Replacing the file with a directory forces the final rename to fail.
	path := *s.SessionFile()
	if err = os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	oldSettings := storage.value
	if err = s.SetModel(models[1]); err == nil {
		t.Fatal("session failure hidden")
	}
	if !reflect.DeepEqual(before, config87State(s)) || !reflect.DeepEqual(entries, manager.GetEntries()) {
		t.Fatal("failed session write partially switched")
	}
	var a, b any
	_ = json.Unmarshal([]byte(oldSettings), &a)
	_ = json.Unmarshal([]byte(storage.value), &b)
	if !reflect.DeepEqual(a, b) {
		t.Fatal("settings rollback failed")
	}
	matches, _ := filepath.Glob(filepath.Join(sessionDir, ".session-config-*"))
	if len(matches) > 0 {
		t.Fatal("staging files leaked")
	}
}

func config87State(s *codingagent.AgentSession) string {
	data, _ := json.Marshal(s.State())
	return string(data)
}

func TestSessionConfigurationEventsAndAvailableCycle(t *testing.T) {
	runtime, models, _ := config87Runtime(t)
	core, _ := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
	created, err := codingagent.CreateAgentSession(context.Background(), codingagent.CreateAgentSessionOptions{CWD: t.TempDir(), Model: &models[0], ModelRuntime: runtime, StreamFunction: agent.StreamFunction(core.StreamSimple), ThinkingLevel: "off"})
	if err != nil {
		t.Fatal(err)
	}
	s := created.Session
	defer s.Dispose()
	entered, release := make(chan struct{}), make(chan struct{})
	unsubscribe, err := s.Subscribe(func(event codingagent.AgentSessionEvent) {
		if e, ok := event.(codingagent.AgentSessionThinkingLevelChangedEvent); ok {
			if s.ThinkingLevel() != e.Level {
				t.Error("event precedes configuration")
			}
			close(entered)
			<-release
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- s.SetThinkingLevel("high") }()
	<-entered
	if e := s.SetThinkingLevel("off"); e == nil {
		t.Error("overlapping event delivery accepted")
	}
	if e := s.Prompt(context.Background(), "overlap"); e == nil {
		t.Error("generation overtook configuration event")
	}
	close(release)
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	unsubscribe()
	before := len(s.SessionManager().GetEntries())
	if err = s.SetThinkingLevel("high"); err != nil || len(s.SessionManager().GetEntries()) != before {
		t.Fatal("unchanged thinking appended entry")
	}
	available, err := runtime.GetAvailableSnapshot()
	if err != nil || len(available) != 2 {
		t.Fatalf("available models: %v %v", available, err)
	}
	next, err := s.CycleModel(context.Background())
	if err != nil || next == nil || next.IsScoped || next.Model.ID != models[1].ID {
		t.Fatalf("available cycle: %+v %v", next, err)
	}
	previous, err := s.CycleModel(context.Background(), codingagent.ModelCycleBackward)
	if err != nil || previous == nil || previous.Model.ID != models[0].ID {
		t.Fatalf("backward: %+v %v", previous, err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	snapshot := config87State(s)
	if _, err = s.CycleModel(canceled); !errors.Is(err, context.Canceled) || config87State(s) != snapshot {
		t.Fatal("canceled cycle mutated state")
	}
	if err = s.SetModel(models[1]); err != nil {
		t.Fatal(err)
	}
	if level, e := s.CycleThinkingLevel(); e != nil || level != "" {
		t.Fatalf("nonreasoning cycle: %q %v", level, e)
	}
	s.Dispose()
	if err = s.SetActiveToolsByName([]string{}); err == nil {
		t.Fatal("disposed session changed")
	}
}

func TestSessionConfigurationMissingRuntime(t *testing.T) {
	core, _ := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
	model, _ := core.GetModel()
	created, err := codingagent.CreateAgentSession(context.Background(), codingagent.CreateAgentSessionOptions{CWD: t.TempDir(), Model: &model, StreamFunction: agent.StreamFunction(core.StreamSimple)})
	if err != nil {
		t.Fatal(err)
	}
	s := created.Session
	defer s.Dispose()
	before := config87State(s)
	count := len(s.SessionManager().GetEntries())
	if result, e := s.CycleModel(context.Background()); e == nil || result != nil {
		t.Fatalf("missing runtime: %v %v", result, e)
	}
	if e := s.SetModel(model); e == nil {
		t.Fatal("model change without runtime succeeded")
	}
	if config87State(s) != before || len(s.SessionManager().GetEntries()) != count {
		t.Fatal("missing runtime failure mutated session")
	}
}

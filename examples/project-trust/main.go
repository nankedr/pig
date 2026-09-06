package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/nankedr/pig/codingagent"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	dir, err := os.MkdirTemp("", "pig-project-trust-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	cwd, agentDir := filepath.Join(dir, "project"), filepath.Join(dir, "agent")
	for _, path := range []string{agentDir, filepath.Join(cwd, ".pig")} {
		if err := os.MkdirAll(path, 0700); err != nil {
			return err
		}
	}
	for path, content := range map[string]string{filepath.Join(agentDir, "settings.json"): `{"defaultProvider":"deepseek","defaultModel":"deepseek-v4-flash"}`, filepath.Join(cwd, ".pig", "settings.json"): `{"defaultModel":"deepseek-v4-pro"}`, filepath.Join(cwd, "AGENTS.override.md"): "EXAMPLE_CONTEXT"} {
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			return err
		}
	}
	requests := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		prompt, _ := json.Marshal(body["messages"])
		requests <- fmt.Sprintf("model=%v context=%t", body["model"], strings.Contains(string(prompt), "EXAMPLE_CONTEXT"))
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"reply\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	ctx := context.Background()
	key := "fixture"
	for _, trusted := range []bool{false, true} {
		runtime, err := codingagent.CreateHeadlessSession(ctx, codingagent.CreateHeadlessSessionOptions{CWD: cwd, AgentDir: agentDir, APIKey: &key, BaseURL: &server.URL, ProjectTrustOverride: &trusted, SessionManager: codingagent.NewInMemorySessionManager(cwd)})
		if err != nil {
			return err
		}
		outcome, err := codingagent.RunHeadless(ctx, runtime, codingagent.HeadlessRunOptions{Messages: []string{"hello"}})
		runtime.Session().Dispose()
		if err != nil {
			return err
		}
		if outcome.FinalMessage == nil || outcome.FinalMessage.StopReason == "error" {
			return fmt.Errorf("headless request failed")
		}
		fmt.Printf("trusted=%t %s\n", trusted, <-requests)
	}
	store := codingagent.NewProjectTrustStore(agentDir)
	if err := store.Set(ctx, cwd, codingagent.ProjectTrustDecisionTrusted()); err != nil {
		return err
	}
	decision, err := store.Get(ctx, cwd)
	if err != nil {
		return err
	}
	fmt.Printf("persisted=%t\n", decision != nil && *decision)
	return nil
}

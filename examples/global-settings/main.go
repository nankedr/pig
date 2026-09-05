package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"

	"github.com/nankedr/pig/codingagent"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	dir, err := os.MkdirTemp("", "pig-global-settings-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	sessionDir := filepath.Join(dir, "sessions")
	data, err := json.Marshal(codingagent.Settings{SessionDir: &sessionDir})
	if err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(dir, "settings.json"), data, 0o600); err != nil {
		return err
	}
	settings, err := codingagent.NewSettingsManager(dir, &dir)
	if err != nil {
		return err
	}
	if err = settings.SetDefaultModelAndProvider("deepseek", "deepseek-v4-flash"); err != nil {
		return err
	}
	if err = settings.SetDefaultThinkingLevel("high"); err != nil {
		return err
	}
	ctx := context.Background()
	if err = settings.Flush(ctx); err != nil {
		return err
	}
	if diagnostics, err := settings.DrainErrors(); err != nil {
		return err
	} else if len(diagnostics) > 0 {
		return diagnostics[0]
	}
	reopened, err := codingagent.NewSettingsManager(dir, &dir)
	if err != nil {
		return err
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"id\":\"example\",\"choices\":[{\"delta\":{\"content\":\"Settings restored\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	key := "local-fixture"
	runtime, err := codingagent.CreateHeadlessSession(ctx, codingagent.CreateHeadlessSessionOptions{CWD: dir, SettingsManager: reopened, APIKey: &key, BaseURL: &server.URL})
	if err != nil {
		return err
	}
	defer runtime.Session().Dispose()
	outcome, err := codingagent.RunHeadless(ctx, runtime, codingagent.HeadlessRunOptions{Messages: []string{"hello"}})
	if err != nil {
		return err
	}
	if outcome.FinalMessage == nil || outcome.FinalMessage.StopReason == "error" {
		return &codingagent.HeadlessOutcomeError{Outcome: outcome}
	}
	fmt.Println(outcome.Text[0])
	fmt.Println("Session stored:", runtime.Session().SessionFile() != nil)
	return nil
}

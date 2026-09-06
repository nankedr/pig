package codingagent_test

import (
	"context"
	"encoding/json"
	"github.com/nankedr/pig/codingagent"
	"os"
	"path/filepath"
	"testing"
)

func TestHeadlessProjectTrustPriority(t *testing.T) {
	for _, test := range []struct {
		name            string
		resource        bool
		policy          string
		saved, override *bool
		want            bool
	}{
		{name: "ask closed", resource: true},
		{name: "always", resource: true, policy: "always", want: true},
		{name: "never", resource: true, policy: "never"},
		{name: "empty ignores never", policy: "never", want: true},
		{name: "saved deny beats always", resource: true, policy: "always", saved: codingagent.ProjectTrustDecisionUntrusted()},
		{name: "saved trust beats never", resource: true, policy: "never", saved: codingagent.ProjectTrustDecisionTrusted(), want: true},
		{name: "approve beats saved", resource: true, saved: codingagent.ProjectTrustDecisionUntrusted(), override: codingagent.ProjectTrustDecisionTrusted(), want: true},
		{name: "deny empty", override: codingagent.ProjectTrustDecisionUntrusted()},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			cwd := filepath.Join(dir, "project")
			agentDir := filepath.Join(dir, "agent")
			os.MkdirAll(cwd, 0700)
			os.MkdirAll(agentDir, 0700)
			t.Setenv("HOME", dir)
			data, _ := json.Marshal(map[string]any{"defaultProjectTrust": test.policy, "defaultProvider": "deepseek", "defaultModel": "deepseek-v4-flash"})
			if err := os.WriteFile(filepath.Join(agentDir, "settings.json"), data, 0600); err != nil {
				t.Fatal(err)
			}
			if test.resource {
				os.MkdirAll(filepath.Join(cwd, ".pig"), 0700)
				os.WriteFile(filepath.Join(cwd, ".pig", "settings.json"), []byte(`{"defaultModel":"deepseek-v4-pro"}`), 0600)
			}
			store := codingagent.NewProjectTrustStore(agentDir)
			if test.saved != nil {
				if err := store.Set(context.Background(), cwd, test.saved); err != nil {
					t.Fatal(err)
				}
			}
			key := "fixture"
			runtime, err := codingagent.CreateHeadlessSession(context.Background(), codingagent.CreateHeadlessSessionOptions{CWD: cwd, AgentDir: agentDir, APIKey: &key, ProjectTrustOverride: test.override, SessionManager: codingagent.NewInMemorySessionManager(cwd)})
			if err != nil {
				t.Fatal(err)
			}
			defer runtime.Session().Dispose()
			trusted, err := runtime.Services().SettingsManager.IsProjectTrusted()
			if err != nil || trusted != test.want {
				t.Fatalf("trusted %v want %v: %v", trusted, test.want, err)
			}
			model, _ := runtime.Services().SettingsManager.GetDefaultModel()
			want := "deepseek-v4-flash"
			if test.resource && test.want {
				want = "deepseek-v4-pro"
			}
			if model != want {
				t.Fatalf("model %s want %s", model, want)
			}
			saved, err := store.Get(context.Background(), cwd)
			if err != nil {
				t.Fatal(err)
			}
			if (saved == nil) != (test.saved == nil) || saved != nil && *saved != *test.saved {
				t.Fatal("run-only override persisted")
			}
		})
	}
}

package codingagent_test

import (
	"context"
	"testing"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
)

func TestHeadlessSessionReplacementKeepsSettingsAndRestoresThinking(t *testing.T) {
	for _, explicit := range []agent.ThinkingLevel{"", "off"} {
		t.Run("explicit="+string(explicit), func(t *testing.T) {
			ctx := context.Background()
			dir := t.TempDir()
			settings, err := codingagent.NewSettingsManager(dir, &dir)
			if err != nil {
				t.Fatal(err)
			}
			if err := settings.SetDefaultModelAndProvider("deepseek", "deepseek-v4-flash"); err != nil {
				t.Fatal(err)
			}
			if err := settings.SetDefaultThinkingLevel("off"); err != nil {
				t.Fatal(err)
			}
			saved, err := codingagent.NewSessionManager(dir, &dir)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := saved.AppendModelChange("deepseek", "deepseek-v4-pro"); err != nil {
				t.Fatal(err)
			}
			if _, err := saved.AppendThinkingLevelChange("high"); err != nil {
				t.Fatal(err)
			}
			reply, _ := ai.FauxAssistantMessage(ai.FauxAssistantText("previous reply"))
			if _, err := saved.AppendMessage(reply); err != nil {
				t.Fatal(err)
			}
			key := "fixture"
			runtime, err := codingagent.CreateHeadlessSession(ctx, codingagent.CreateHeadlessSessionOptions{CWD: dir, AgentDir: dir, APIKey: &key, Thinking: explicit})
			if err != nil {
				t.Fatal(err)
			}
			defer runtime.Dispose(ctx)
			if runtime.Session().ThinkingLevel() != "off" {
				t.Fatal("initial session ignored global thinking")
			}
			effective := runtime.Services().SettingsManager
			model := "deepseek-v4-pro"
			if err := effective.ApplyOverrides(codingagent.Settings{DefaultModel: &model}); err != nil {
				t.Fatal(err)
			}
			if _, err := runtime.SwitchSession(ctx, *saved.GetSessionFile()); err != nil {
				t.Fatal(err)
			}
			want := agent.ThinkingLevel("high")
			if explicit != "" {
				want = explicit
			}
			if runtime.Session().ThinkingLevel() != want || runtime.Session().Model().ID != model {
				t.Fatalf("restored model/thinking = %s/%s", runtime.Session().Model().ID, runtime.Session().ThinkingLevel())
			}
			if _, err := runtime.NewSession(ctx); err != nil {
				t.Fatal(err)
			}
			if runtime.Services().SettingsManager != effective || runtime.Session().ThinkingLevel() != "off" || runtime.Session().Model().ID != model {
				t.Fatal("new session lost effective settings")
			}
		})
	}
}

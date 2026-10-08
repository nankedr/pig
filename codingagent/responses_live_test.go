package codingagent_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
)

func TestResponsesCodingAgentLiveRestore(t *testing.T) {
	if os.Getenv("PIG_REQUIRE_RESPONSES_SESSION_LIVE") != "1" {
		t.Skip("protected smoke requires PIG_REQUIRE_RESPONSES_SESSION_LIVE=1")
	}
	key := strings.TrimSpace(os.Getenv("DEEPSEEK_API_KEY"))
	if key == "" {
		t.Fatal("protected smoke requires DEEPSEEK_API_KEY")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	root, dir := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "sentinel.txt"), []byte("local value: 133\n"), 0600); err != nil {
		t.Fatal(err)
	}
	id := os.Getenv("PIG_RESPONSES_SMOKE_MODEL")
	if id == "" {
		id = "deepseek-v4-pro"
	}
	options := codingagent.CreateHeadlessSessionOptions{CWD: root, AgentDir: dir, Provider: ai.ProviderIDDeepSeek, Model: id, API: ai.APIOpenAIResponses, Thinking: "low", Environment: ai.ProviderEnv{"DEEPSEEK_API_KEY": key}, Tools: []string{"read"}, NoContextFiles: true}
	runtime, err := codingagent.CreateHeadlessSession(ctx, options)
	if err != nil {
		t.Fatal("protected runtime creation failed")
	}
	if err := runtime.Session().Prompt(ctx, "Read sentinel.txt once using read, then briefly report the returned value."); err != nil {
		runtime.Dispose(context.Background())
		t.Fatal("protected tool run failed")
	}
	stats, err := runtime.Session().GetSessionStats()
	if err != nil || stats.ToolCalls != 1 || stats.ToolResults != 1 {
		runtime.Dispose(context.Background())
		t.Fatal("protected tool continuation did not execute once")
	}
	path := *runtime.Session().SessionFile()
	if err := runtime.Dispose(context.Background()); err != nil {
		t.Fatal(err)
	}
	manager, err := codingagent.OpenSessionManager(path, nil, nil)
	if err != nil {
		t.Fatal("protected v3 restore failed")
	}
	options.API, options.Model, options.SessionManager = "", "", manager
	runtime, err = codingagent.CreateHeadlessSession(ctx, options)
	if err != nil {
		t.Fatal("protected runtime restore failed")
	}
	defer runtime.Dispose(context.Background())
	if runtime.Session().Model().API != ai.APIOpenAIResponses {
		t.Fatal("protected restore changed API")
	}
	outcome, err := codingagent.RunHeadless(ctx, runtime, codingagent.HeadlessRunOptions{Messages: []string{"Continue from the value already read. Do not call tools. Reply briefly."}})
	if err != nil || outcome.FinalMessage == nil || outcome.FinalMessage.StopReason != ai.StopReasonStop || len(outcome.Text) == 0 {
		t.Fatal("protected restored continuation failed")
	}
	stats, err = runtime.Session().GetSessionStats()
	if err != nil || stats.ToolCalls != 1 || stats.ToolResults != 1 {
		t.Fatal("protected restored continuation repeated tools")
	}
	t.Logf("PASS DeepSeek %s: production-v3 read once, restore API/history and continue", id)
}

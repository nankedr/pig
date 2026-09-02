package codingagent_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
)

func TestDecideLiveSmoke(t *testing.T) {
	cases := []struct {
		name    string
		require bool
		key     string
		want    codingagent.LiveSmokeDecision
	}{
		{"no key skips ordinary run", false, "", codingagent.LiveSmokeSkip},
		{"no key fails required run", true, "", codingagent.LiveSmokeFail},
		{"blank key fails required run", true, "   ", codingagent.LiveSmokeFail},
		{"key runs ordinary", false, "sk-live", codingagent.LiveSmokeRun},
		{"key runs required", true, "sk-live", codingagent.LiveSmokeRun},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := codingagent.DecideLiveSmoke(tc.require, tc.key); got != tc.want {
				t.Fatalf("DecideLiveSmoke(%t, %q) = %q, want %q", tc.require, tc.key, got, tc.want)
			}
		})
	}
}

// TestDeepSeekLiveHeadlessReadContinuation runs the protected DeepSeek live
// smoke through the public Headless product path: a low-token text stream, then
// a real two-request read Tool continuation. It skips on an ordinary PR without
// a key and fails a Freeze/release run (PIG_REQUIRE_LIVE=1) that lacks one. It
// never prints the key, request/response bodies, or file contents.
func TestDeepSeekLiveHeadlessReadContinuation(t *testing.T) {
	key := os.Getenv("DEEPSEEK_API_KEY")
	switch codingagent.DecideLiveSmoke(os.Getenv("PIG_REQUIRE_LIVE") == "1", key) {
	case codingagent.LiveSmokeSkip:
		t.Skip("set DEEPSEEK_API_KEY to run the DeepSeek live smoke; PIG_REQUIRE_LIVE=1 forces it")
	case codingagent.LiveSmokeFail:
		t.Fatal("PIG_REQUIRE_LIVE=1 requires DEEPSEEK_API_KEY for the Freeze/release live smoke")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	environment := ai.ProviderEnv{"DEEPSEEK_API_KEY": key}

	textOutcome := runLiveHeadless(t, ctx, codingagent.CreateHeadlessSessionOptions{
		CWD: t.TempDir(), Provider: ai.ProviderIDDeepSeek, Model: "deepseek-v4-flash", Environment: environment,
	}, "Reply with the single word ok.")
	if textOutcome.FinalMessage == nil || textOutcome.FinalMessage.StopReason != ai.StopReasonStop ||
		strings.TrimSpace(strings.Join(textOutcome.Text, "")) == "" {
		t.Fatalf("phase 1 live text stream did not complete with assistant text (stopReason=%v)", liveStopReason(textOutcome))
	}

	cwd := t.TempDir()
	sentinel := liveSentinel(t)
	if err := os.WriteFile(filepath.Join(cwd, "sentinel.txt"), []byte(sentinel), 0o600); err != nil {
		t.Fatal(err)
	}
	runtime, err := codingagent.CreateHeadlessSession(ctx, codingagent.CreateHeadlessSessionOptions{
		CWD: cwd, Provider: ai.ProviderIDDeepSeek, Model: "deepseek-v4-flash", Environment: environment, Tools: []string{"read"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Dispose(context.WithoutCancel(ctx))
	prompt := "Use the read tool to read sentinel.txt in the working directory, then reply with its exact contents."
	outcome, err := codingagent.RunHeadless(ctx, runtime, codingagent.HeadlessRunOptions{InitialMessage: &prompt})
	if err != nil {
		t.Fatalf("phase 2 live read continuation failed: %v", err)
	}
	assertLiveReadContinuation(t, runtime.Session().Messages(), sentinel, outcome)
}

func runLiveHeadless(t *testing.T, ctx context.Context, options codingagent.CreateHeadlessSessionOptions, prompt string) codingagent.HeadlessOutcome {
	t.Helper()
	runtime, err := codingagent.CreateHeadlessSession(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Dispose(context.WithoutCancel(ctx))
	outcome, err := codingagent.RunHeadless(ctx, runtime, codingagent.HeadlessRunOptions{InitialMessage: &prompt})
	if err != nil {
		t.Fatalf("live headless run failed: %v", err)
	}
	return outcome
}

// assertLiveReadContinuation checks the real transcript carries a read ToolCall,
// a matching ToolResult, and final text echoing the sentinel, without printing
// the sentinel or any model output on failure.
func assertLiveReadContinuation(t *testing.T, messages []agent.AgentMessage, sentinel string, outcome codingagent.HeadlessOutcome) {
	t.Helper()
	var toolCallID string
	for _, message := range messages {
		assistant, ok := message.(ai.AssistantMessage)
		if !ok {
			continue
		}
		for _, content := range assistant.Content {
			if call, ok := content.(ai.ToolCall); ok && call.Name == "read" {
				toolCallID = call.ID
			}
		}
	}
	if toolCallID == "" {
		t.Fatal("live continuation produced no read ToolCall")
	}
	foundResult := false
	for _, message := range messages {
		result, ok := message.(ai.ToolResultMessage)
		if !ok || result.ToolName != "read" || result.IsError || result.ToolCallID != toolCallID {
			continue
		}
		for _, content := range result.Content {
			if text, ok := content.(ai.TextContent); ok && strings.Contains(text.Text, sentinel) {
				foundResult = true
			}
		}
	}
	if !foundResult {
		t.Fatal("live read ToolResult did not carry the sentinel file content for the read ToolCall")
	}
	if !strings.Contains(strings.Join(outcome.Text, "\n"), sentinel) {
		t.Fatal("final live assistant text did not echo the sentinel")
	}
}

func liveSentinel(t *testing.T) string {
	t.Helper()
	buffer := make([]byte, 8)
	if _, err := rand.Read(buffer); err != nil {
		t.Fatal(err)
	}
	return "issue58-live-" + hex.EncodeToString(buffer)
}

func liveStopReason(outcome codingagent.HeadlessOutcome) ai.StopReason {
	if outcome.FinalMessage == nil {
		return ""
	}
	return outcome.FinalMessage.StopReason
}

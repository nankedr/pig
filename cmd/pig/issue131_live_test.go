package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/nankedr/pig/ai"
)

func TestIssue131DeepSeekResponsesLiveText(t *testing.T) {
	if os.Getenv("PIG_REQUIRE_RESPONSES_LIVE") != "1" {
		t.Skip("protected smoke requires PIG_REQUIRE_RESPONSES_LIVE=1")
	}
	key := strings.TrimSpace(os.Getenv("DEEPSEEK_API_KEY"))
	if key == "" {
		t.Fatal("protected Responses smoke requires DEEPSEEK_API_KEY")
	}
	id := os.Getenv("PIG_RESPONSES_SMOKE_MODEL")
	if id == "" {
		id = "deepseek-v4-pro"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	tokens := int64(64)
	model := ai.Model{ID: id, Provider: ai.ProviderIDDeepSeek, API: ai.APIOpenAIResponses, BaseURL: "https://api.deepseek.com", Input: []ai.ModelInput{ai.ModelInputText}, Reasoning: true, MaxTokens: 64}
	input := ai.Context{Messages: []ai.Message{ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText("Reply briefly with a greeting.")}}}
	out, err := ai.BuiltinModels().Complete(ctx, model, input, ai.ModelsStreamOptions{StreamOptions: ai.StreamOptions{ProviderRequestOptions: ai.ProviderRequestOptions{APIKey: &key}, MaxTokens: &tokens}})
	if err != nil || out.StopReason != ai.StopReasonStop || len(out.Content) == 0 {
		t.Fatal("protected SDK Responses text smoke did not complete")
	}
	text := ""
	for _, block := range out.Content {
		if c, ok := block.(ai.TextContent); ok {
			text += c.Text
		}
	}
	if strings.TrimSpace(text) == "" {
		t.Fatal("protected SDK Responses smoke produced no text")
	}
	dir := t.TempDir()
	command := exec.CommandContext(ctx, buildPigBinary(t), "--api", "openai-responses", "--provider", "deepseek", "--model", id, "--thinking", "off", "--no-tools", "--no-session", "--no-context-files", "--offline", "-p", "Reply briefly with a greeting.")
	command.Dir = dir
	command.Env = append(filteredEnvironment(os.Environ(), "PIG_DEEPSEEK_BASE_URL", "PIG_CODING_AGENT_DIR"), "PIG_CODING_AGENT_DIR="+dir)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil || strings.TrimSpace(stdout.String()) == "" {
		t.Fatal("protected CLI Responses text smoke did not complete")
	}
}

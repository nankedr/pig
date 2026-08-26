package agent_test

import (
	"errors"
	"testing"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
)

func TestEstimateTokensSupportsAllMessageRepresentations(t *testing.T) {
	user := ai.UserMessage{
		Role: ai.MessageRoleUser,
		Content: ai.UserBlocks(
			ai.TextContent{Type: ai.ContentTypeText, Text: "界ab"},
			ai.ImageContent{Type: ai.ContentTypeImage},
		),
	}
	assistant := ai.AssistantMessage{
		Role: ai.MessageRoleAssistant,
		Content: []ai.AssistantContent{
			ai.TextContent{Type: ai.ContentTypeText, Text: "界a"},
			ai.ThinkingContent{Type: ai.ContentTypeThinking, Thinking: "想b"},
			ai.ToolCall{
				Type:      ai.ContentTypeToolCall,
				Name:      "工具",
				Arguments: map[string]any{"x": "界"},
			},
		},
	}
	toolResult := ai.ToolResultMessage{
		Role: ai.MessageRoleToolResult,
		Content: []ai.ToolResultContent{
			ai.TextContent{Type: ai.ContentTypeText, Text: "界ab"},
			ai.ImageContent{Type: ai.ContentTypeImage},
		},
	}

	tests := []struct {
		name    string
		message agent.AgentMessage
		want    int64
	}{
		{name: "user value", message: user, want: 1201},
		{name: "user pointer", message: &user, want: 1201},
		{name: "assistant value", message: assistant, want: 4},
		{name: "assistant pointer", message: &assistant, want: 4},
		{name: "tool result value", message: toolResult, want: 1201},
		{name: "tool result pointer", message: &toolResult, want: 1201},
		{name: "bash execution", message: agent.BashExecutionMessage{Command: "界ab", Output: "cde"}, want: 2},
		{name: "custom", message: agent.CustomMessage{Content: ai.UserText("界abc")}, want: 1},
		{name: "branch summary", message: agent.BranchSummaryMessage{Summary: "界abcd"}, want: 2},
		{name: "compaction summary", message: agent.CompactionSummaryMessage{Summary: "界abcdefg"}, want: 2},
		{name: "nil user pointer", message: (*ai.UserMessage)(nil), want: 0},
		{name: "nil assistant pointer", message: (*ai.AssistantMessage)(nil), want: 0},
		{name: "nil tool result pointer", message: (*ai.ToolResultMessage)(nil), want: 0},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := agent.EstimateTokens(test.message); got != test.want {
				t.Errorf("EstimateTokens(%T) = %d, want %d", test.message, got, test.want)
			}
		})
	}
}

func TestEstimateTokensUsesUTF16CodeUnits(t *testing.T) {
	tests := []struct {
		name    string
		message agent.AgentMessage
		want    int64
	}{
		{name: "user text", message: ai.UserMessage{Content: ai.UserText("abc😀")}, want: 2},
		{name: "user block", message: ai.UserMessage{Content: ai.UserBlocks(ai.TextContent{Text: "abc😀"})}, want: 2},
		{name: "assistant text", message: ai.AssistantMessage{Content: []ai.AssistantContent{ai.TextContent{Text: "abc😀"}}}, want: 2},
		{name: "assistant thinking", message: ai.AssistantMessage{Content: []ai.AssistantContent{ai.ThinkingContent{Thinking: "abc😀"}}}, want: 2},
		{name: "tool call arguments", message: ai.AssistantMessage{Content: []ai.AssistantContent{ai.ToolCall{Arguments: map[string]any{"": "😀"}}}}, want: 3},
		{name: "tool result", message: ai.ToolResultMessage{Content: []ai.ToolResultContent{ai.TextContent{Text: "abc😀"}}}, want: 2},
		{name: "bash execution", message: agent.BashExecutionMessage{Command: "abc", Output: "😀"}, want: 2},
		{name: "custom", message: agent.CustomMessage{Content: ai.UserText("abc😀")}, want: 2},
		{name: "branch summary", message: agent.BranchSummaryMessage{Summary: "abc😀"}, want: 2},
		{name: "compaction summary", message: agent.CompactionSummaryMessage{Summary: "abc😀"}, want: 2},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := agent.EstimateTokens(test.message); got != test.want {
				t.Errorf("EstimateTokens(%T) = %d, want %d", test.message, got, test.want)
			}
		})
	}
}

func TestCompactionThresholdAndDeferredRuntime(t *testing.T) {
	settings := agent.CompactionSettings{Enabled: true, ReserveTokens: 100}
	if agent.ShouldCompact(900, 1000, settings) {
		t.Fatal("ShouldCompact triggered at the threshold")
	}
	if !agent.ShouldCompact(901, 1000, settings) {
		t.Fatal("ShouldCompact did not trigger above the threshold")
	}

	result, err := agent.PrepareCompaction(nil, settings)
	if result.OK || !errors.Is(err, agent.ErrNotImplemented) {
		t.Fatalf("PrepareCompaction() = %#v, %v", result, err)
	}
}

func TestFindCutPointHonorsRecentTokenBudget(t *testing.T) {
	message := func(role ai.MessageRole, text string) agent.Entry {
		if role == ai.MessageRoleUser {
			return agent.MessageEntry{Message: ai.UserMessage{Role: role, Content: ai.UserText(text)}}
		}
		return agent.MessageEntry{Message: ai.AssistantMessage{
			Role: role,
			Content: []ai.AssistantContent{
				ai.TextContent{Type: ai.ContentTypeText, Text: text},
			},
		}}
	}
	entries := []agent.Entry{
		message(ai.MessageRoleUser, "turn"),
		message(ai.MessageRoleAssistant, "0123456789012345678901234567890123456789"),
		message(ai.MessageRoleUser, "next"),
		message(ai.MessageRoleAssistant, "0123456789012345678901234567890123456789"),
	}

	split := agent.FindCutPoint(entries, 0, len(entries), 1)
	if split.FirstKeptEntryIndex != 3 || split.TurnStartIndex != 2 || !split.IsSplitTurn {
		t.Fatalf("FindCutPoint(1) = %#v, want split at assistant entry 3", split)
	}
	wholeTurn := agent.FindCutPoint(entries, 0, len(entries), 11)
	if wholeTurn.FirstKeptEntryIndex != 2 || wholeTurn.TurnStartIndex != -1 || wholeTurn.IsSplitTurn {
		t.Fatalf("FindCutPoint(11) = %#v, want whole turn at user entry 2", wholeTurn)
	}
}

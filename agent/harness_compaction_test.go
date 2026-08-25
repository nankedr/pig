package agent_test

import (
	"errors"
	"testing"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
)

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

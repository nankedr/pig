package codingagent_test

import (
	"encoding/json"
	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
	"github.com/nankedr/pig/tui"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestAssistantMessageParity112(t *testing.T) {
	data, err := os.ReadFile("../parity/oracle/fixtures/text-rendering.json")
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Case struct {
			Input struct {
				Assistant []struct {
					Content      []struct{ Type, Text, Thinking string }
					StopReason   ai.StopReason
					ErrorMessage string
				}
			}
		}
		Observation struct {
			Outcome struct{ Assistant [][][]string }
		}
	}
	if err = json.Unmarshal(data, &f); err != nil {
		t.Fatal(err)
	}
	for i, c := range f.Case.Input.Assistant {
		message := ai.AssistantMessage{Role: ai.MessageRoleAssistant, StopReason: c.StopReason}
		if c.ErrorMessage != "" {
			message.ErrorMessage = ai.Some(c.ErrorMessage)
		}
		for _, p := range c.Content {
			if p.Type == "text" {
				message.Content = append(message.Content, ai.TextContent{Type: ai.ContentTypeText, Text: p.Text})
			} else {
				message.Content = append(message.Content, ai.ThinkingContent{Type: ai.ContentTypeThinking, Thinking: p.Thinking})
			}
		}
		for j, hide := range []bool{false, true} {
			view := new(codingagent.AssistantMessageComponent)
			if err = view.UpdateContent(message); err != nil {
				t.Fatal(err)
			}
			view.SetHideThinkingBlock(hide)
			lines, err := view.Render(50)
			if err != nil {
				t.Fatal(err)
			}
			for n, s := range lines {
				s, _ = tui.StripTerminalSequences(s)
				lines[n] = strings.TrimRight(s, " ")
			}
			if !reflect.DeepEqual(lines, f.Observation.Outcome.Assistant[i][j]) {
				t.Errorf("%d hide=%t: got %#v want %#v", i, hide, lines, f.Observation.Outcome.Assistant[i][j])
			}
		}
	}
}

func TestToolFinalResultWins112(t *testing.T) {
	view := codingagent.NewToolExecutionComponent("probe", "call-1", map[string]any{"path": "a"})
	if err := view.UpdateResult(codingagent.ToolExecutionResult{Content: []ai.ToolResultContent{ai.TextContent{Type: ai.ContentTypeText, Text: "FINAL"}}}); err != nil {
		t.Fatal(err)
	}
	if err := view.UpdateResult(codingagent.ToolExecutionResult{Content: []ai.ToolResultContent{ai.TextContent{Type: ai.ContentTypeText, Text: "LATE"}}}, true); err != nil {
		t.Fatal(err)
	}
	lines, err := view.Render(30)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Join(lines, "\n")
	if !strings.Contains(text, "FINAL") || strings.Contains(text, "LATE") {
		t.Fatalf("late update overwrote final: %s", text)
	}
	view = codingagent.NewToolExecutionComponent("probe", "call-2", nil)
	view.UpdateResult(codingagent.ToolExecutionResult{Content: []ai.ToolResultContent{ai.TextContent{Type: ai.ContentTypeText, Text: strings.Repeat("line\n", 12) + "LAST"}}})
	lines, _ = view.Render(40)
	if strings.Contains(strings.Join(lines, "\n"), "LAST") {
		t.Fatal("not collapsed")
	}
	view.SetExpanded(true)
	lines, _ = view.Render(40)
	if !strings.Contains(strings.Join(lines, "\n"), "LAST") {
		t.Fatal("not expanded")
	}
}

func TestTranscriptStreamingEqualsHistory112(t *testing.T) {
	user := ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText("question")}
	assistant := ai.AssistantMessage{Role: ai.MessageRoleAssistant, Content: []ai.AssistantContent{ai.ThinkingContent{Type: ai.ContentTypeThinking, Thinking: "reason"}, ai.TextContent{Type: ai.ContentTypeText, Text: "**answer**"}, ai.ToolCall{Type: ai.ContentTypeToolCall, ID: "a", Name: "probe", Arguments: map[string]any{"path": "a"}}, ai.ToolCall{Type: ai.ContentTypeToolCall, ID: "b", Name: "probe", Arguments: map[string]any{"path": "b"}}}}
	a := ai.ToolResultMessage{Role: ai.MessageRoleToolResult, ToolCallID: "a", ToolName: "probe", Content: []ai.ToolResultContent{ai.TextContent{Type: ai.ContentTypeText, Text: "RESULT_A"}}}
	b := ai.ToolResultMessage{Role: ai.MessageRoleToolResult, ToolCallID: "b", ToolName: "probe", Content: []ai.ToolResultContent{ai.TextContent{Type: ai.ContentTypeText, Text: "RESULT_B"}}, IsError: true}
	live := codingagent.NewTranscript()
	for _, m := range []agent.AgentMessage{user, assistant} {
		live.Update(codingagent.AgentSessionMessageStartEvent{MessageStartEvent: agent.MessageStartEvent{Message: m}})
		live.Update(codingagent.AgentSessionMessageEndEvent{MessageEndEvent: agent.MessageEndEvent{Message: m}})
	}
	for _, m := range []ai.ToolResultMessage{b, a} {
		live.Update(codingagent.AgentSessionToolExecutionEndEvent{ToolExecutionEndEvent: agent.ToolExecutionEndEvent{ToolCallID: m.ToolCallID, ToolName: m.ToolName, Result: agent.ErasedAgentToolResult{Content: m.Content}, IsError: m.IsError}})
	}
	live.Update(codingagent.AgentSessionToolExecutionUpdateEvent{ToolExecutionUpdateEvent: agent.ToolExecutionUpdateEvent{ToolCallID: "a", PartialResult: agent.ErasedAgentToolResult{Content: []ai.ToolResultContent{ai.TextContent{Type: ai.ContentTypeText, Text: "LATE"}}}}})
	for _, m := range []ai.ToolResultMessage{a, b} {
		live.Update(codingagent.AgentSessionMessageStartEvent{MessageStartEvent: agent.MessageStartEvent{Message: m}})
		live.Update(codingagent.AgentSessionMessageEndEvent{MessageEndEvent: agent.MessageEndEvent{Message: m}})
	}
	history := codingagent.NewTranscript()
	history.SetMessages([]agent.AgentMessage{user, assistant, a, b, agent.CreateCompactionSummaryMessage("summary", 123, 1)})
	live.Update(codingagent.AgentSessionMessageStartEvent{MessageStartEvent: agent.MessageStartEvent{Message: agent.CreateCompactionSummaryMessage("summary", 123, 1)}})
	for _, width := range []int{16, 80} {
		got, err := live.Render(width)
		if err != nil {
			t.Fatal(err)
		}
		want, _ := history.Render(width)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("live != history: %#v / %#v", got, want)
		}
		s := strings.Join(got, "\n")
		if strings.Contains(s, "LATE") || strings.Index(s, "RESULT_A") > strings.Index(s, "RESULT_B") {
			t.Fatal(s)
		}
	}
}

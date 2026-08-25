package agent_test

import (
	"testing"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
)

func TestConvertToLLMProjectsHarnessMessages(t *testing.T) {
	messages := []agent.AgentMessage{
		agent.BashExecutionMessage{Role: "bashExecution", Command: "pwd", Output: "/tmp", ExitCodeSet: true, Timestamp: 1},
		agent.CreateCompactionSummaryMessage("kept context", 42, 2),
		agent.BashExecutionMessage{Role: "bashExecution", Command: "secret", ExcludeFromContext: true},
	}
	converted := agent.ConvertToLLM(messages)
	if len(converted) != 2 {
		t.Fatalf("ConvertToLLM() returned %d messages", len(converted))
	}
	first, ok := converted[0].(ai.UserMessage)
	if !ok {
		t.Fatalf("first converted message is %T", converted[0])
	}
	text, ok := first.Content.Text()
	if !ok || text != "Ran `pwd`\n```\n/tmp\n```" {
		t.Fatalf("bash projection = %q, %v", text, ok)
	}
}

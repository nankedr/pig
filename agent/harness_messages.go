package agent

import (
	"fmt"
	"strings"

	"github.com/nankedr/pig/ai"
)

const (
	CompactionSummaryPrefix = "The conversation history before this point was compacted into the following summary:\n\n<summary>\n"
	CompactionSummarySuffix = "\n</summary>"
	BranchSummaryPrefix     = "The following is a summary of a branch that this conversation came back from:\n\n<summary>\n"
	BranchSummarySuffix     = "</summary>"
)

type BashExecutionMessage struct {
	Role               ai.MessageRole
	Command            string
	Output             string
	ExitCode           int
	ExitCodeSet        bool
	Cancelled          bool
	Truncated          bool
	FullOutputPath     string
	Timestamp          int64
	ExcludeFromContext bool
}

func (m BashExecutionMessage) MessageRole() ai.MessageRole { return m.Role }

type CustomMessage struct {
	Role       ai.MessageRole
	CustomType string
	Content    ai.UserMessageContent
	Display    bool
	Details    ai.Optional[ai.JSONValue]
	Timestamp  int64
}

func (m CustomMessage) MessageRole() ai.MessageRole { return m.Role }

type BranchSummaryMessage struct {
	Role      ai.MessageRole
	Summary   string
	FromID    string
	Timestamp int64
}

func (m BranchSummaryMessage) MessageRole() ai.MessageRole { return m.Role }

type CompactionSummaryMessage struct {
	Role         ai.MessageRole
	Summary      string
	TokensBefore int64
	Timestamp    int64
}

func (m CompactionSummaryMessage) MessageRole() ai.MessageRole { return m.Role }

func BashExecutionToText(message BashExecutionMessage) string {
	var result strings.Builder
	fmt.Fprintf(&result, "Ran `%s`\n", message.Command)
	if message.Output == "" {
		result.WriteString("(no output)")
	} else {
		fmt.Fprintf(&result, "```\n%s\n```", message.Output)
	}
	if message.Cancelled {
		result.WriteString("\n\n(command cancelled)")
	} else if message.ExitCodeSet && message.ExitCode != 0 {
		fmt.Fprintf(&result, "\n\nCommand exited with code %d", message.ExitCode)
	}
	if message.Truncated && message.FullOutputPath != "" {
		fmt.Fprintf(&result, "\n\n[Output truncated. Full output: %s]", message.FullOutputPath)
	}
	return result.String()
}

func CreateBranchSummaryMessage(summary, fromID string, timestamp int64) AgentMessage {
	return BranchSummaryMessage{Role: "branchSummary", Summary: summary, FromID: fromID, Timestamp: timestamp}
}

func CreateCompactionSummaryMessage(summary string, tokensBefore, timestamp int64) AgentMessage {
	return CompactionSummaryMessage{Role: "compactionSummary", Summary: summary, TokensBefore: tokensBefore, Timestamp: timestamp}
}

func CreateCustomMessage(customType string, content ai.UserMessageContent, display bool, details ai.Optional[ai.JSONValue], timestamp int64) AgentMessage {
	return CustomMessage{Role: "custom", CustomType: customType, Content: content, Display: display, Details: details, Timestamp: timestamp}
}

func ConvertToLLM(messages []AgentMessage) []ai.Message {
	converted := make([]ai.Message, 0, len(messages))
	for _, message := range messages {
		switch message := message.(type) {
		case ai.Message:
			converted = append(converted, message)
		case BashExecutionMessage:
			if !message.ExcludeFromContext {
				converted = append(converted, ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText(BashExecutionToText(message)), Timestamp: message.Timestamp})
			}
		case CustomMessage:
			converted = append(converted, ai.UserMessage{Role: ai.MessageRoleUser, Content: message.Content, Timestamp: message.Timestamp})
		case BranchSummaryMessage:
			converted = append(converted, ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText(BranchSummaryPrefix + message.Summary + BranchSummarySuffix), Timestamp: message.Timestamp})
		case CompactionSummaryMessage:
			converted = append(converted, ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText(CompactionSummaryPrefix + message.Summary + CompactionSummarySuffix), Timestamp: message.Timestamp})
		}
	}
	return converted
}

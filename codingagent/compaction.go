package codingagent

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode/utf16"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
)

var DefaultCompactionSettings = agent.DefaultCompactionSettings

type CompactionResult struct {
	Details              ai.JSONValue
	EstimatedTokensAfter *int64
	FirstKeptEntryID     string
	Summary              string
	TokensBefore         int64
	Usage                *ai.Usage
}

type CutPointResult struct {
	FirstKeptEntryIndex int
	IsSplitTurn         bool
	TurnStartIndex      int
}

type FileOperations struct {
	Edited  map[string]struct{}
	Read    map[string]struct{}
	Written map[string]struct{}
}

func newFileOperations() FileOperations {
	return FileOperations{Edited: map[string]struct{}{}, Read: map[string]struct{}{}, Written: map[string]struct{}{}}
}

type BranchPreparation struct {
	FileOps     FileOperations
	Messages    []agent.AgentMessage
	TotalTokens int64
}

type BranchSummaryResult struct {
	Aborted       bool
	Error         string
	ModifiedFiles []string
	ReadFiles     []string
	Summary       string
	Usage         *ai.Usage
}

type CollectEntriesResult struct {
	CommonAncestorID *string
	Entries          []SessionEntry
}

type branchSummarySession interface {
	GetBranch(...string) []SessionEntry
	GetEntry(string) *SessionEntry
}

type GenerateBranchSummaryOptions struct {
	APIKey              *string
	Callbacks           *agent.RetryCallbacks
	CustomInstructions  string
	Env                 map[string]string
	Headers             map[string]string
	Model               ai.Model
	ReplaceInstructions bool
	ReserveTokens       int64
	Retry               *agent.RetryPolicy
	StreamFn            agent.StreamFunction
}

func CalculateContextTokens(usage ai.Usage) int64 {
	return agent.CalculateContextTokens(usage)
}

func ShouldCompact(contextTokens, contextWindow int64, settings CompactionSettings) bool {
	return agent.ShouldCompact(contextTokens, contextWindow, settings)
}

func EstimateTokens(message agent.AgentMessage) int64 {
	return agent.EstimateTokens(message)
}

func GetLastAssistantUsage(entries []SessionEntry) (*ai.Usage, bool) {
	for index := len(entries) - 1; index >= 0; index-- {
		if entries[index].Type != "message" {
			continue
		}
		var assistant ai.AssistantMessage
		switch value := entries[index].Message.(type) {
		case ai.AssistantMessage:
			assistant = value
		case *ai.AssistantMessage:
			if value == nil {
				continue
			}
			assistant = *value
		default:
			continue
		}
		if assistant.StopReason == ai.StopReasonAborted || assistant.StopReason == ai.StopReasonError || CalculateContextTokens(assistant.Usage) == 0 {
			continue
		}
		usage := assistant.Usage
		return &usage, true
	}
	return nil, false
}

func FindTurnStartIndex(entries []SessionEntry, entryIndex, startIndex int) int {
	if entryIndex >= len(entries) {
		entryIndex = len(entries) - 1
	}
	if startIndex < 0 {
		startIndex = 0
	}
	for index := entryIndex; index >= startIndex; index-- {
		if entryStartsTurn(entries[index]) {
			return index
		}
	}
	return -1
}

func FindCutPoint(entries []SessionEntry, startIndex, endIndex int, keepRecentTokens int64) CutPointResult {
	startIndex = max(0, startIndex)
	endIndex = min(len(entries), endIndex)
	if startIndex >= endIndex {
		return CutPointResult{FirstKeptEntryIndex: startIndex, TurnStartIndex: -1}
	}
	valid := make([]int, 0, endIndex-startIndex)
	for index := startIndex; index < endIndex; index++ {
		if entryHasCutPoint(entries[index]) {
			valid = append(valid, index)
		}
	}
	if len(valid) == 0 {
		return CutPointResult{FirstKeptEntryIndex: startIndex, TurnStartIndex: -1}
	}
	cutIndex := valid[0]
	var tokens int64
	for index := endIndex - 1; index >= startIndex; index-- {
		var messageTokens int64
		for _, message := range SessionEntryToContextMessages(entries[index]) {
			messageTokens += EstimateTokens(message)
		}
		if messageTokens == 0 {
			continue
		}
		tokens += messageTokens
		if tokens < keepRecentTokens {
			continue
		}
		for _, point := range valid {
			if point >= index {
				cutIndex = point
				break
			}
		}
		break
	}
	for cutIndex > startIndex && entries[cutIndex-1].Type != "compaction" && len(SessionEntryToContextMessages(entries[cutIndex-1])) == 0 {
		cutIndex--
	}
	startsTurn := entryStartsTurn(entries[cutIndex])
	turnStart := -1
	if !startsTurn {
		turnStart = FindTurnStartIndex(entries, cutIndex, startIndex)
	}
	return CutPointResult{FirstKeptEntryIndex: cutIndex, TurnStartIndex: turnStart, IsSplitTurn: !startsTurn && turnStart != -1}
}

func entryHasCutPoint(entry SessionEntry) bool {
	if entry.Type == "compaction" {
		return false
	}
	message := branchEntryMessage(entry)
	if message == nil {
		return false
	}
	switch message.MessageRole() {
	case ai.MessageRoleUser, ai.MessageRoleAssistant, "bashExecution", "custom", "branchSummary", "compactionSummary":
		return true
	}
	return false
}

func entryStartsTurn(entry SessionEntry) bool {
	if entry.Type == "compaction" {
		return false
	}
	message := branchEntryMessage(entry)
	if message == nil {
		return false
	}
	switch message.MessageRole() {
	case ai.MessageRoleUser, "bashExecution", "custom", "branchSummary", "compactionSummary":
		return true
	}
	return false
}

func CollectEntriesForBranchSummary(_ context.Context, session branchSummarySession, oldLeafID, targetID string) (CollectEntriesResult, error) {
	if session == nil || oldLeafID == "" {
		return CollectEntriesResult{Entries: []SessionEntry{}}, nil
	}
	oldBranch := session.GetBranch(oldLeafID)
	targetBranch := session.GetBranch(targetID)
	oldIDs := make(map[string]struct{}, len(oldBranch))
	for _, entry := range oldBranch {
		oldIDs[entry.ID] = struct{}{}
	}
	var common *string
	for index := len(targetBranch) - 1; index >= 0; index-- {
		if _, ok := oldIDs[targetBranch[index].ID]; ok {
			id := targetBranch[index].ID
			common = &id
			break
		}
	}
	entries := make([]SessionEntry, 0, len(oldBranch))
	for current := oldLeafID; current != "" && (common == nil || current != *common); {
		entry := session.GetEntry(current)
		if entry == nil {
			break
		}
		entries = append(entries, cloneSessionEntry(*entry))
		if entry.ParentID == nil {
			break
		}
		current = *entry.ParentID
	}
	for left, right := 0, len(entries)-1; left < right; left, right = left+1, right-1 {
		entries[left], entries[right] = entries[right], entries[left]
	}
	return CollectEntriesResult{Entries: entries, CommonAncestorID: common}, nil
}

func PrepareBranchEntries(entries []SessionEntry, tokenBudget ...int64) BranchPreparation {
	budget := int64(0)
	if len(tokenBudget) != 0 {
		budget = tokenBudget[0]
	}
	result := BranchPreparation{FileOps: newFileOperations()}
	for _, entry := range entries {
		if entry.Type != "branch_summary" || entry.FromHook != nil && *entry.FromHook || len(entry.Details) == 0 {
			continue
		}
		var details struct {
			ReadFiles     []string `json:"readFiles"`
			ModifiedFiles []string `json:"modifiedFiles"`
		}
		if json.Unmarshal(entry.Details, &details) == nil {
			for _, path := range details.ReadFiles {
				result.FileOps.Read[path] = struct{}{}
			}
			for _, path := range details.ModifiedFiles {
				result.FileOps.Edited[path] = struct{}{}
			}
		}
	}
	for index := len(entries) - 1; index >= 0; index-- {
		message := branchEntryMessage(entries[index])
		if message == nil {
			continue
		}
		extractFileOperations(message, &result.FileOps)
		tokens := EstimateTokens(message)
		if budget > 0 && result.TotalTokens+tokens > budget {
			if (entries[index].Type == "compaction" || entries[index].Type == "branch_summary") && result.TotalTokens < budget*9/10 {
				result.Messages = append([]agent.AgentMessage{message}, result.Messages...)
				result.TotalTokens += tokens
			}
			break
		}
		result.Messages = append([]agent.AgentMessage{message}, result.Messages...)
		result.TotalTokens += tokens
	}
	return result
}

func branchEntryMessage(entry SessionEntry) agent.AgentMessage {
	switch entry.Type {
	case "message":
		if entry.Message != nil && entry.Message.MessageRole() != ai.MessageRoleToolResult {
			return entry.Message
		}
	case "custom_message":
		if messages := SessionEntryToContextMessages(entry); len(messages) != 0 {
			return messages[0]
		}
	case "branch_summary":
		return agent.CreateBranchSummaryMessage(entry.Summary, entry.FromID, sessionTimestampMillis(entry.Timestamp))
	case "compaction":
		return agent.CreateCompactionSummaryMessage(entry.Summary, entry.TokensBefore, sessionTimestampMillis(entry.Timestamp))
	}
	return nil
}

func extractFileOperations(message agent.AgentMessage, operations *FileOperations) {
	var content []ai.AssistantContent
	switch value := message.(type) {
	case ai.AssistantMessage:
		content = value.Content
	case *ai.AssistantMessage:
		if value != nil {
			content = value.Content
		}
	}
	for _, block := range content {
		call, ok := block.(ai.ToolCall)
		if !ok {
			continue
		}
		path, _ := call.Arguments["path"].(string)
		if path == "" {
			continue
		}
		switch call.Name {
		case "read":
			operations.Read[path] = struct{}{}
		case "write":
			operations.Written[path] = struct{}{}
		case "edit":
			operations.Edited[path] = struct{}{}
		}
	}
}

func SerializeConversation(messages []ai.Message) string {
	parts := make([]string, 0, len(messages))
	for _, message := range messages {
		switch value := message.(type) {
		case ai.UserMessage:
			text := userContentText(value.Content)
			if text != "" {
				parts = append(parts, "[User]: "+text)
			}
		case ai.AssistantMessage:
			parts = appendSerializedAssistant(parts, value)
		case *ai.AssistantMessage:
			if value != nil {
				parts = appendSerializedAssistant(parts, *value)
			}
		case *ai.UserMessage:
			if value != nil {
				if text := userContentText(value.Content); text != "" {
					parts = append(parts, "[User]: "+text)
				}
			}
		case ai.ToolResultMessage:
			if text := toolResultText(value.Content); text != "" {
				parts = append(parts, "[Tool result]: "+truncateSummaryText(text, 2000))
			}
		case *ai.ToolResultMessage:
			if value != nil {
				if text := toolResultText(value.Content); text != "" {
					parts = append(parts, "[Tool result]: "+truncateSummaryText(text, 2000))
				}
			}
		}
	}
	return strings.Join(parts, "\n\n")
}

func appendSerializedAssistant(parts []string, value ai.AssistantMessage) []string {
	var thinking, text, calls []string
	for _, block := range value.Content {
		switch content := block.(type) {
		case ai.ThinkingContent:
			thinking = append(thinking, content.Thinking)
		case ai.TextContent:
			text = append(text, content.Text)
		case ai.ToolCall:
			keys := make([]string, 0, len(content.Arguments))
			for key := range content.Arguments {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			args := make([]string, 0, len(keys))
			for _, key := range keys {
				encoded, _ := json.Marshal(content.Arguments[key])
				args = append(args, key+"="+string(encoded))
			}
			calls = append(calls, content.Name+"("+strings.Join(args, ", ")+")")
		}
	}
	if len(thinking) != 0 {
		parts = append(parts, "[Assistant thinking]: "+strings.Join(thinking, "\n"))
	}
	if len(text) != 0 {
		parts = append(parts, "[Assistant]: "+strings.Join(text, ""))
	}
	if len(calls) != 0 {
		parts = append(parts, "[Assistant tool calls]: "+strings.Join(calls, "; "))
	}
	return parts
}

func userContentText(content ai.UserMessageContent) string {
	if text, ok := content.Text(); ok {
		return text
	}
	blocks, _ := content.Blocks()
	var parts []string
	for _, block := range blocks {
		if text, ok := block.(ai.TextContent); ok {
			parts = append(parts, text.Text)
		}
	}
	return strings.Join(parts, "")
}

func toolResultText(content []ai.ToolResultContent) string {
	var parts []string
	for _, block := range content {
		if text, ok := block.(ai.TextContent); ok {
			parts = append(parts, text.Text)
		}
	}
	return strings.Join(parts, "")
}

func truncateSummaryText(text string, limit int) string {
	codeUnits := utf16.Encode([]rune(text))
	if len(codeUnits) <= limit {
		return text
	}
	return string(utf16.Decode(codeUnits[:limit])) + fmt.Sprintf("\n\n[... %d more characters truncated]", len(codeUnits)-limit)
}

func GenerateBranchSummary(context.Context, []SessionEntry, GenerateBranchSummaryOptions) (BranchSummaryResult, error) {
	return BranchSummaryResult{}, notImplemented("GenerateBranchSummary")
}

func GenerateSummary(context.Context, []agent.AgentMessage, ai.Model, int64, ...any) (string, error) {
	return "", notImplemented("GenerateSummary")
}

type SummaryWithUsage struct {
	Text  string
	Usage ai.Usage
}

func GenerateSummaryWithUsage(context.Context, []agent.AgentMessage, ai.Model, int64, ...any) (SummaryWithUsage, error) {
	return SummaryWithUsage{}, notImplemented("GenerateSummaryWithUsage")
}

func Compact(context.Context, any, ai.Model, ...any) (CompactionResult, error) {
	return CompactionResult{}, notImplemented("Compact")
}

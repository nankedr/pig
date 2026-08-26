package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/nankedr/pig/ai"
)

type CompactionSettings struct {
	Enabled          bool
	ReserveTokens    int64
	KeepRecentTokens int64
}

var DefaultCompactionSettings = CompactionSettings{
	Enabled:          true,
	ReserveTokens:    16_384,
	KeepRecentTokens: 20_000,
}

func CalculateContextTokens(usage ai.Usage) int64 {
	if usage.TotalTokens != 0 {
		return usage.TotalTokens
	}
	return usage.Input + usage.Output + usage.CacheRead + usage.CacheWrite
}

func ShouldCompact(contextTokens, contextWindow int64, settings CompactionSettings) bool {
	return settings.Enabled && contextTokens > contextWindow-settings.ReserveTokens
}

type CompactResult struct {
	Summary      string
	TokensBefore int64
	Usage        *ai.Usage
	RetainedTail []AgentMessage
	Details      ai.JSONValue
}

type FileOperations struct {
	Read    map[string]struct{}
	Written map[string]struct{}
	Edited  map[string]struct{}
}

func NewFileOperations() FileOperations {
	return FileOperations{Read: map[string]struct{}{}, Written: map[string]struct{}{}, Edited: map[string]struct{}{}}
}

type CompactionPreparation struct {
	MessagesToSummarize []AgentMessage
	TurnPrefixMessages  []AgentMessage
	RetainedTail        []AgentMessage
	IsSplitTurn         bool
	TokensBefore        int64
	PreviousSummary     string
	FileOps             FileOperations
	Settings            CompactionSettings
}

type ContextUsageEstimate struct {
	Tokens         int64
	UsageTokens    int64
	TrailingTokens int64
	LastUsageIndex int
}

func EstimateContextTokens(messages []AgentMessage) ContextUsageEstimate {
	result := ContextUsageEstimate{LastUsageIndex: -1}
	for index := len(messages) - 1; index >= 0; index-- {
		assistant, ok := messages[index].(ai.AssistantMessage)
		if !ok || assistant.StopReason == ai.StopReasonAborted || assistant.StopReason == ai.StopReasonError {
			continue
		}
		if tokens := CalculateContextTokens(assistant.Usage); tokens > 0 {
			result.UsageTokens = tokens
			result.LastUsageIndex = index
			break
		}
	}
	start := 0
	if result.LastUsageIndex >= 0 {
		start = result.LastUsageIndex + 1
	}
	for _, message := range messages[start:] {
		result.TrailingTokens += EstimateTokens(message)
	}
	result.Tokens = result.UsageTokens + result.TrailingTokens
	return result
}

func EstimateTokens(message AgentMessage) int64 {
	utf16Units := 0
	switch message := message.(type) {
	case ai.UserMessage:
		utf16Units = userContentUTF16Units(message.Content)
	case *ai.UserMessage:
		if message != nil {
			utf16Units = userContentUTF16Units(message.Content)
		}
	case ai.AssistantMessage:
		utf16Units = assistantContentUTF16Units(message.Content)
	case *ai.AssistantMessage:
		if message != nil {
			utf16Units = assistantContentUTF16Units(message.Content)
		}
	case ai.ToolResultMessage:
		utf16Units = toolResultContentUTF16Units(message.Content)
	case *ai.ToolResultMessage:
		if message != nil {
			utf16Units = toolResultContentUTF16Units(message.Content)
		}
	case BashExecutionMessage:
		utf16Units = utf16CodeUnits(message.Command) + utf16CodeUnits(message.Output)
	case CustomMessage:
		utf16Units = userContentUTF16Units(message.Content)
	case BranchSummaryMessage:
		utf16Units = utf16CodeUnits(message.Summary)
	case CompactionSummaryMessage:
		utf16Units = utf16CodeUnits(message.Summary)
	}
	return int64((utf16Units + 3) / 4)
}

func utf16CodeUnits(text string) int {
	units := 0
	for _, character := range text {
		units++
		if character > 0xffff {
			units++
		}
	}
	return units
}

func userContentUTF16Units(content ai.UserMessageContent) int {
	if text, ok := content.Text(); ok {
		return utf16CodeUnits(text)
	}
	blocks, _ := content.Blocks()
	utf16Units := 0
	for _, block := range blocks {
		switch block := block.(type) {
		case ai.TextContent:
			utf16Units += utf16CodeUnits(block.Text)
		case ai.ImageContent:
			utf16Units += 4800
		}
	}
	return utf16Units
}

func assistantContentUTF16Units(content []ai.AssistantContent) int {
	utf16Units := 0
	for _, block := range content {
		switch block := block.(type) {
		case ai.TextContent:
			utf16Units += utf16CodeUnits(block.Text)
		case ai.ThinkingContent:
			utf16Units += utf16CodeUnits(block.Thinking)
		case ai.ToolCall:
			encoded, _ := json.Marshal(block.Arguments)
			utf16Units += utf16CodeUnits(block.Name) + utf16CodeUnits(string(encoded))
		}
	}
	return utf16Units
}

func toolResultContentUTF16Units(content []ai.ToolResultContent) int {
	utf16Units := 0
	for _, block := range content {
		switch block := block.(type) {
		case ai.TextContent:
			utf16Units += utf16CodeUnits(block.Text)
		case ai.ImageContent:
			utf16Units += 4800
		}
	}
	return utf16Units
}

func GetLastAssistantUsage(entries []Entry) (*ai.Usage, bool) {
	for index := len(entries) - 1; index >= 0; index-- {
		message, ok := entries[index].(MessageEntry)
		if !ok {
			continue
		}
		assistant, ok := message.Message.(ai.AssistantMessage)
		if !ok || assistant.StopReason == ai.StopReasonAborted || assistant.StopReason == ai.StopReasonError || CalculateContextTokens(assistant.Usage) == 0 {
			continue
		}
		usage := assistant.Usage
		return &usage, true
	}
	return nil, false
}

type CutPointResult struct {
	FirstKeptEntryIndex int
	TurnStartIndex      int
	IsSplitTurn         bool
}

func FindTurnStartIndex(entries []Entry, entryIndex, startIndex int) int {
	for index := entryIndex; index >= startIndex; index-- {
		switch entry := entries[index].(type) {
		case BranchSummaryEntry:
			return index
		case MessageEntry:
			role := entry.Message.MessageRole()
			if role == ai.MessageRoleUser || role == "bashExecution" {
				return index
			}
		}
	}
	return -1
}

func findValidCutPoints(entries []Entry, startIndex, endIndex int) []int {
	cutPoints := make([]int, 0, endIndex-startIndex)
	for index := startIndex; index < endIndex; index++ {
		switch entry := entries[index].(type) {
		case MessageEntry:
			switch entry.Message.MessageRole() {
			case "bashExecution", "custom", "branchSummary", "compactionSummary", ai.MessageRoleUser, ai.MessageRoleAssistant:
				cutPoints = append(cutPoints, index)
			}
		case BranchSummaryEntry:
			cutPoints = append(cutPoints, index)
		}
	}
	return cutPoints
}

func FindCutPoint(entries []Entry, startIndex, endIndex int, keepRecentTokens int64) CutPointResult {
	if startIndex < 0 {
		startIndex = 0
	}
	if endIndex > len(entries) {
		endIndex = len(entries)
	}
	if startIndex >= endIndex {
		return CutPointResult{FirstKeptEntryIndex: startIndex, TurnStartIndex: -1}
	}
	cutPoints := findValidCutPoints(entries, startIndex, endIndex)
	if len(cutPoints) == 0 {
		return CutPointResult{FirstKeptEntryIndex: startIndex, TurnStartIndex: -1}
	}
	cutIndex := cutPoints[0]
	var accumulatedTokens int64
	for index := endIndex - 1; index >= startIndex; index-- {
		entry, ok := entries[index].(MessageEntry)
		if !ok {
			continue
		}
		accumulatedTokens += EstimateTokens(entry.Message)
		if accumulatedTokens < keepRecentTokens {
			continue
		}
		for _, point := range cutPoints {
			if point >= index {
				cutIndex = point
				break
			}
		}
		break
	}
adjustCut:
	for cutIndex > startIndex {
		switch entries[cutIndex-1].(type) {
		case CompactionEntry, MessageEntry:
			break adjustCut
		default:
			cutIndex--
		}
	}
	message, isMessage := entries[cutIndex].(MessageEntry)
	isUserMessage := isMessage && message.Message.MessageRole() == ai.MessageRoleUser
	turnStart := -1
	if !isUserMessage {
		turnStart = FindTurnStartIndex(entries, cutIndex, startIndex)
	}
	return CutPointResult{
		FirstKeptEntryIndex: cutIndex,
		TurnStartIndex:      turnStart,
		IsSplitTurn:         !isUserMessage && turnStart != -1,
	}
}

func PrepareCompaction([]Entry, CompactionSettings) (Result[*CompactionPreparation], error) {
	return Result[*CompactionPreparation]{}, newNotImplemented("PrepareCompaction")
}

func GenerateSummary(context.Context, []AgentMessage, ai.Models, ai.Model, int64) (Result[string], error) {
	return Result[string]{}, newNotImplemented("GenerateSummary")
}

type SummaryWithUsage struct {
	Text  string
	Usage ai.Usage
}

func GenerateSummaryWithUsage(context.Context, []AgentMessage, ai.Models, ai.Model, int64) (Result[SummaryWithUsage], error) {
	return Result[SummaryWithUsage]{}, newNotImplemented("GenerateSummaryWithUsage")
}

func Compact(context.Context, CompactionPreparation, ai.Models, ai.Model) (Result[CompactResult], error) {
	return Result[CompactResult]{}, newNotImplemented("Compact")
}

type BranchSummaryResult struct {
	Summary       string
	Usage         *ai.Usage
	ReadFiles     []string
	ModifiedFiles []string
}

type BranchSummaryDetails struct {
	ReadFiles     []string
	ModifiedFiles []string
}

type BranchPreparation struct {
	Messages    []AgentMessage
	FileOps     FileOperations
	TotalTokens int64
}

type CollectEntriesResult struct {
	Entries          []Entry
	CommonAncestorID string
}

type GenerateBranchSummaryOptions struct {
	Models              ai.Models
	Model               ai.Model
	CustomInstructions  string
	ReplaceInstructions bool
	ReserveTokens       int64
	Retry               RetryPolicy
	Callbacks           RetryCallbacks
}

func CollectEntriesForBranchSummary(context.Context, *Session, string, string) (CollectEntriesResult, error) {
	return CollectEntriesResult{}, newNotImplemented("CollectEntriesForBranchSummary")
}

func PrepareBranchEntries(entries []Entry, tokenBudget int64) BranchPreparation {
	result := BranchPreparation{FileOps: NewFileOperations()}
	for index := len(entries) - 1; index >= 0; index-- {
		var message AgentMessage
		switch entry := entries[index].(type) {
		case MessageEntry:
			message = entry.Message
		case BranchSummaryEntry:
			message = CreateBranchSummaryMessage(entry.Summary, entry.FromID, entry.Timestamp)
		case CompactionEntry:
			message = CreateCompactionSummaryMessage(entry.Summary, entry.TokensBefore, entry.Timestamp)
		}
		if message == nil {
			continue
		}
		tokens := EstimateTokens(message)
		if tokenBudget > 0 && result.TotalTokens+tokens > tokenBudget {
			break
		}
		result.Messages = append([]AgentMessage{message}, result.Messages...)
		result.TotalTokens += tokens
	}
	return result
}

func GenerateBranchSummary(context.Context, []Entry, GenerateBranchSummaryOptions) (Result[BranchSummaryResult], error) {
	return Result[BranchSummaryResult]{}, newNotImplemented("GenerateBranchSummary")
}

func SerializeConversation(messages []ai.Message) string {
	parts := make([]string, 0, len(messages))
	for _, message := range messages {
		encoded, err := ai.MarshalMessage(message)
		if err != nil {
			continue
		}
		parts = append(parts, fmt.Sprintf("[%s]: %s", message.MessageRole(), encoded))
	}
	return strings.Join(parts, "\n\n")
}

func ComputeFileLists(operations FileOperations) (readFiles, modifiedFiles []string) {
	modified := make(map[string]struct{}, len(operations.Written)+len(operations.Edited))
	for path := range operations.Written {
		modified[path] = struct{}{}
	}
	for path := range operations.Edited {
		modified[path] = struct{}{}
	}
	for path := range operations.Read {
		if _, changed := modified[path]; !changed {
			readFiles = append(readFiles, path)
		}
	}
	for path := range modified {
		modifiedFiles = append(modifiedFiles, path)
	}
	sort.Strings(readFiles)
	sort.Strings(modifiedFiles)
	return readFiles, modifiedFiles
}

func FormatFileOperations(readFiles, modifiedFiles []string) string {
	var sections []string
	if len(readFiles) > 0 {
		sections = append(sections, "<read-files>\n"+strings.Join(readFiles, "\n")+"\n</read-files>")
	}
	if len(modifiedFiles) > 0 {
		sections = append(sections, "<modified-files>\n"+strings.Join(modifiedFiles, "\n")+"\n</modified-files>")
	}
	if len(sections) == 0 {
		return ""
	}
	return "\n\n" + strings.Join(sections, "\n\n")
}

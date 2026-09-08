package codingagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
)

type SummaryOptions struct {
	APIKey             *string
	Headers            map[string]string
	Env                map[string]string
	CustomInstructions string
	PreviousSummary    string
	ThinkingLevel      agent.ThinkingLevel
	StreamFn           agent.StreamFunction
	Retry              *agent.RetryPolicy
	Callbacks          *agent.RetryCallbacks
}

type CompactionPreparation struct {
	FirstKeptEntryID    string
	MessagesToSummarize []agent.AgentMessage
	TurnPrefixMessages  []agent.AgentMessage
	IsSplitTurn         bool
	TokensBefore        int64
	PreviousSummary     string
	FileOps             FileOperations
	Settings            CompactionSettings
}

func PrepareCompaction(entries []SessionEntry, settings CompactionSettings) *CompactionPreparation {
	if len(entries) == 0 || entries[len(entries)-1].Type == "compaction" {
		return nil
	}
	start, prev := 0, -1
	for i := len(entries) - 1; i >= 0; i-- {
		if entries[i].Type == "compaction" {
			prev = i
			start = i + 1
			for j, e := range entries {
				if e.ID == entries[i].FirstKeptEntryID {
					start = j
					break
				}
			}
			break
		}
	}
	cut := FindCutPoint(entries, start, len(entries), settings.KeepRecentTokens)
	if cut.FirstKeptEntryIndex >= len(entries) || entries[cut.FirstKeptEntryIndex].ID == "" {
		return nil
	}
	result := &CompactionPreparation{FirstKeptEntryID: entries[cut.FirstKeptEntryIndex].ID, IsSplitTurn: cut.IsSplitTurn, TokensBefore: compactionContextTokens(BuildSessionContext(entries).Messages), FileOps: newFileOperations(), Settings: settings}
	if prev >= 0 {
		result.PreviousSummary = entries[prev].Summary
		if entries[prev].FromHook == nil || !*entries[prev].FromHook {
			var details struct{ ReadFiles, ModifiedFiles []string }
			if json.Unmarshal(entries[prev].Details, &details) == nil {
				for _, f := range details.ReadFiles {
					result.FileOps.Read[f] = struct{}{}
				}
				for _, f := range details.ModifiedFiles {
					result.FileOps.Edited[f] = struct{}{}
				}
			}
		}
	}
	end := cut.FirstKeptEntryIndex
	if cut.IsSplitTurn {
		end = cut.TurnStartIndex
	}
	for i := start; i < cut.FirstKeptEntryIndex; i++ {
		if entries[i].Type == "compaction" {
			continue
		}
		messages := SessionEntryToContextMessages(entries[i])
		for _, m := range messages {
			extractFileOperations(m, &result.FileOps)
		}
		if i < end {
			result.MessagesToSummarize = append(result.MessagesToSummarize, messages...)
		} else {
			result.TurnPrefixMessages = append(result.TurnPrefixMessages, messages...)
		}
	}
	if len(result.MessagesToSummarize) == 0 && len(result.TurnPrefixMessages) == 0 {
		return nil
	}
	return result
}

func summaryOptions(options []any) (SummaryOptions, error) {
	if len(options) == 0 {
		return SummaryOptions{}, nil
	}
	if len(options) == 1 {
		if o, ok := options[0].(SummaryOptions); ok {
			return o, nil
		}
	}
	return SummaryOptions{}, fmt.Errorf("expected one SummaryOptions value")
}

func GenerateSummary(ctx context.Context, messages []agent.AgentMessage, model ai.Model, reserve int64, options ...any) (string, error) {
	result, err := GenerateSummaryWithUsage(ctx, messages, model, reserve, options...)
	return result.Text, err
}
func GenerateSummaryWithUsage(ctx context.Context, messages []agent.AgentMessage, model ai.Model, reserve int64, options ...any) (SummaryWithUsage, error) {
	o, err := summaryOptions(options)
	if err != nil {
		return SummaryWithUsage{}, err
	}
	prompt := "<conversation>\n" + SerializeConversation(ConvertToLLM(messages)) + "\n</conversation>\n\n"
	base := summaryPrompt
	if o.PreviousSummary != "" {
		prompt += "<previous-summary>\n" + o.PreviousSummary + "\n</previous-summary>\n\n"
		base = updateSummaryPrompt
	}
	if o.CustomInstructions != "" {
		base += "\n\nAdditional focus: " + o.CustomInstructions
	}
	return completeSummary(ctx, model, prompt+base, reserve*8/10, o)
}
func requestSummary(ctx context.Context, model ai.Model, prompt string, maxTokens int64, o SummaryOptions) (ai.AssistantMessage, error) {
	if ctx == nil {
		return ai.AssistantMessage{}, fmt.Errorf("summary context must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return ai.AssistantMessage{}, context.Cause(ctx)
	}
	if maxTokens <= 0 {
		return ai.AssistantMessage{}, fmt.Errorf("summary token budget must be positive")
	}
	cache, id := ai.CacheRetentionNone, newSessionID()
	headers := ai.ProviderHeaders{}
	for k, v := range o.Headers {
		value := v
		headers[k] = &value
	}
	options := ai.SimpleStreamOptions{StreamOptions: ai.StreamOptions{ProviderRequestOptions: ai.ProviderRequestOptions{APIKey: o.APIKey, Headers: headers, Env: o.Env}, MaxTokens: &maxTokens, CacheRetention: &cache, SessionID: &id}}
	if model.Reasoning && o.ThinkingLevel != "" && o.ThinkingLevel != "off" {
		level := ai.ThinkingLevel(o.ThinkingLevel)
		options.Reasoning = &level
	}
	input := ai.Context{SystemPrompt: ai.Some(summarySystemPrompt), Messages: []ai.Message{ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserBlocks(ai.TextContent{Type: ai.ContentTypeText, Text: prompt}), Timestamp: time.Now().UnixMilli()}}}
	stream := o.StreamFn
	if stream == nil {
		stream = func(ctx context.Context, m ai.Model, c ai.Context, o ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
			return ai.StreamSimple(ctx, m, c, o)
		}
	}
	return retrySummary(ctx, func() (ai.AssistantMessage, error) { return summaryResponse(ctx, stream(ctx, model, input, options)) }, o)
}
func completeSummary(ctx context.Context, model ai.Model, prompt string, maxTokens int64, o SummaryOptions) (SummaryWithUsage, error) {
	if model.MaxTokens > 0 {
		maxTokens = min(maxTokens, model.MaxTokens)
	}
	result, err := requestSummary(ctx, model, prompt, maxTokens, o)
	if err != nil {
		return SummaryWithUsage{}, err
	}
	if ctx.Err() != nil {
		return SummaryWithUsage{}, context.Cause(ctx)
	}
	if result.StopReason == ai.StopReasonAborted {
		return SummaryWithUsage{}, context.Canceled
	}
	if result.StopReason == ai.StopReasonError {
		text, _ := result.ErrorMessage.Value()
		if text == "" {
			text = "Unknown error"
		}
		return SummaryWithUsage{}, fmt.Errorf("Summarization failed: %s", text)
	}
	parts := []string{}
	for _, block := range result.Content {
		if b, ok := block.(ai.TextContent); ok {
			parts = append(parts, b.Text)
		}
	}
	return SummaryWithUsage{Text: strings.Join(parts, "\n"), Usage: result.Usage}, nil
}
func Compact(ctx context.Context, value any, model ai.Model, options ...any) (CompactionResult, error) {
	p, ok := value.(*CompactionPreparation)
	if !ok || p == nil {
		return CompactionResult{}, fmt.Errorf("expected non-nil *CompactionPreparation")
	}
	o, err := summaryOptions(options)
	if err != nil {
		return CompactionResult{}, err
	}
	o.PreviousSummary = p.PreviousSummary
	var summary SummaryWithUsage
	if p.IsSplitTurn && len(p.TurnPrefixMessages) > 0 {
		summary.Text = "No prior history."
		if len(p.MessagesToSummarize) > 0 {
			summary, err = GenerateSummaryWithUsage(ctx, p.MessagesToSummarize, model, p.Settings.ReserveTokens, o)
			if err != nil {
				return CompactionResult{}, err
			}
		}
		prompt := "<conversation>\n" + SerializeConversation(ConvertToLLM(p.TurnPrefixMessages)) + "\n</conversation>\n\n" + turnPrefixPrompt
		prefix, e := completeSummary(ctx, model, prompt, p.Settings.ReserveTokens/2, o)
		if e != nil {
			if strings.HasPrefix(e.Error(), "Summarization failed:") {
				e = fmt.Errorf("Turn prefix summarization failed:%s", strings.TrimPrefix(e.Error(), "Summarization failed:"))
			}
			return CompactionResult{}, e
		}
		summary.Text += "\n\n---\n\n**Turn Context (split turn):**\n\n" + prefix.Text
		summary.Usage = combineSummaryUsage(summary.Usage, prefix.Usage)
	} else {
		summary, err = GenerateSummaryWithUsage(ctx, p.MessagesToSummarize, model, p.Settings.ReserveTokens, o)
		if err != nil {
			return CompactionResult{}, err
		}
	}
	modified := map[string]struct{}{}
	for f := range p.FileOps.Edited {
		modified[f] = struct{}{}
	}
	for f := range p.FileOps.Written {
		modified[f] = struct{}{}
	}
	reads, writes := []string{}, []string{}
	for f := range p.FileOps.Read {
		if _, ok := modified[f]; !ok {
			reads = append(reads, f)
		}
	}
	for f := range modified {
		writes = append(writes, f)
	}
	sort.Strings(reads)
	sort.Strings(writes)
	if len(reads) > 0 {
		summary.Text += "\n\n<read-files>\n" + strings.Join(reads, "\n") + "\n</read-files>"
	}
	if len(writes) > 0 {
		summary.Text += "\n\n<modified-files>\n" + strings.Join(writes, "\n") + "\n</modified-files>"
	}
	if p.FirstKeptEntryID == "" {
		return CompactionResult{}, fmt.Errorf("First kept entry has no UUID - session may need migration")
	}
	return CompactionResult{Summary: summary.Text, FirstKeptEntryID: p.FirstKeptEntryID, TokensBefore: p.TokensBefore, Usage: &summary.Usage, Details: map[string]any{"readFiles": reads, "modifiedFiles": writes}}, nil
}

func combineSummaryUsage(a, b ai.Usage) ai.Usage {
	a.Input += b.Input
	a.Output += b.Output
	a.CacheRead += b.CacheRead
	a.CacheWrite += b.CacheWrite
	a.TotalTokens += b.TotalTokens
	for _, p := range []struct {
		dst *ai.Optional[int64]
		src ai.Optional[int64]
	}{{&a.CacheWrite1H, b.CacheWrite1H}, {&a.Reasoning, b.Reasoning}} {
		x, ok := p.dst.Value()
		y, other := p.src.Value()
		if ok || other {
			*p.dst = ai.Some(x + y)
		}
	}
	a.Cost.Input += b.Cost.Input
	a.Cost.Output += b.Cost.Output
	a.Cost.CacheRead += b.Cost.CacheRead
	a.Cost.CacheWrite += b.Cost.CacheWrite
	a.Cost.Total += b.Cost.Total
	return a
}

func retrySummary(ctx context.Context, produce func() (ai.AssistantMessage, error), o SummaryOptions) (response ai.AssistantMessage, err error) {
	maxAttempts := 0
	if o.Retry != nil && o.Retry.Enabled {
		maxAttempts = o.Retry.MaxRetries
	}
	callbacks := agent.RetryCallbacks{}
	if o.Callbacks != nil {
		callbacks = *o.Callbacks
	}
	attempt := 0
	defer func() {
		if attempt > 0 && callbacks.OnRetryFinished != nil {
			success := err == nil && response.StopReason != ai.StopReasonError && response.StopReason != ai.StopReasonAborted
			var final *string
			if text, ok := response.ErrorMessage.Value(); ok {
				final = &text
			}
			err = errors.Join(err, callbacks.OnRetryFinished(success, attempt, final))
		}
	}()
	for {
		response, err = produce()
		if err != nil {
			return response, err
		}
		if ctx.Err() != nil {
			return response, context.Cause(ctx)
		}
		text, _ := response.ErrorMessage.Value()
		if response.StopReason != ai.StopReasonError || attempt >= maxAttempts || sessionProviderLimitError.MatchString(text) || !sessionTransientError.MatchString(text) {
			return response, nil
		}
		attempt++
		base := o.Retry.BaseDelayMS
		if base != 0 && (attempt > 64 || base > math.MaxInt64>>(attempt-1) || base < math.MinInt64>>(attempt-1)) {
			return response, fmt.Errorf("retry delay exceeds int64 milliseconds")
		}
		delay := base << (attempt - 1)
		if text == "" {
			text = "Unknown error"
		}
		if callbacks.OnRetryScheduled != nil {
			if err = callbacks.OnRetryScheduled(attempt, maxAttempts, delay, text); err != nil {
				return response, err
			}
		}
		wait := delay
		if wait < 1 || wait > math.MaxInt32 {
			wait = 1
		}
		timer := time.NewTimer(time.Duration(wait) * time.Millisecond)
		select {
		case <-timer.C:
		case <-ctx.Done():
		}
		timer.Stop()
		if ctx.Err() != nil {
			return response, context.Cause(ctx)
		}
		if callbacks.OnRetryAttemptStart != nil {
			if err = callbacks.OnRetryAttemptStart(); err != nil {
				return response, err
			}
		}
	}
}

func summaryResponse(ctx context.Context, stream *ai.AssistantMessageEventStream) (ai.AssistantMessage, error) {
	if stream == nil {
		return ai.AssistantMessage{}, fmt.Errorf("summary stream is nil")
	}
	started := false
	for {
		event, ok, err := stream.Next(ctx)
		if err != nil || !ok {
			break
		}
		if event.AssistantMessageEventType() == ai.AssistantMessageEventTypeStart {
			started = true
		}
	}
	message, err := stream.Result(ctx)
	if started && errors.Is(err, ai.ErrOpenAISSETruncated) && strings.HasSuffix(err.Error(), "\nstream ended without finish_reason") {
		message.StopReason = ai.StopReasonError
		message.ErrorMessage = ai.Some("Stream ended without finish_reason")
		return message, nil
	}
	return message, err
}

func compactionContextTokens(messages []agent.AgentMessage) int64 {
	var tokens int64
	for i := len(messages) - 1; i >= 0; i-- {
		if message, ok := sessionAssistantMessage(messages[i]); ok && message.StopReason != ai.StopReasonError && message.StopReason != ai.StopReasonAborted {
			if usage := CalculateContextTokens(message.Usage); usage > 0 {
				return usage + tokens
			}
		}
		tokens += EstimateTokens(messages[i])
	}
	return tokens
}

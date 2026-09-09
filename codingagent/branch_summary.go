package codingagent

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/nankedr/pig/ai"
)

const branchSummaryPrompt = `Create a structured summary of this conversation branch for context when returning later.

Use this EXACT format:

## Goal
[What was the user trying to accomplish in this branch?]

## Constraints & Preferences
- [Any constraints, preferences, or requirements mentioned]
- [Or "(none)" if none were mentioned]

## Progress
### Done
- [x] [Completed tasks/changes]

### In Progress
- [ ] [Work that was started but not finished]

### Blocked
- [Issues preventing progress, if any]

## Key Decisions
- **[Decision]**: [Brief rationale]

## Next Steps
1. [What should happen next to continue this work]

Keep each section concise. Preserve exact file paths, function names, and error messages.`

func GenerateBranchSummary(ctx context.Context, entries []SessionEntry, o GenerateBranchSummaryOptions) (BranchSummaryResult, error) {
	reserve := o.ReserveTokens
	if reserve == 0 {
		reserve = 16384
	}
	return generateBranchSummary(ctx, entries, o, reserve)
}

func generateBranchSummary(ctx context.Context, entries []SessionEntry, o GenerateBranchSummaryOptions, reserve int64) (BranchSummaryResult, error) {
	if ctx == nil {
		return BranchSummaryResult{}, fmt.Errorf("branch summary context must not be nil")
	}
	window := o.Model.ContextWindow
	if window == 0 {
		window = 128000
	}
	p := PrepareBranchEntries(entries, window-reserve)
	if len(p.Messages) == 0 {
		return BranchSummaryResult{Summary: "No content to summarize"}, nil
	}
	instructions := branchSummaryPrompt
	if o.CustomInstructions != "" {
		if o.ReplaceInstructions {
			instructions = o.CustomInstructions
		} else {
			instructions += "\n\nAdditional focus: " + o.CustomInstructions
		}
	}
	prompt := "<conversation>\n" + SerializeConversation(ConvertToLLM(p.Messages)) + "\n</conversation>\n\n" + instructions
	response, err := requestSummary(ctx, o.Model, prompt, 2048, SummaryOptions{APIKey: o.APIKey, Headers: o.Headers, Env: o.Env, StreamFn: o.StreamFn, Retry: o.Retry, Callbacks: o.Callbacks})
	if err != nil {
		return BranchSummaryResult{}, err
	}
	if response.StopReason == ai.StopReasonAborted {
		return BranchSummaryResult{Aborted: true}, nil
	}
	if response.StopReason == ai.StopReasonError {
		text, _ := response.ErrorMessage.Value()
		if text == "" {
			text = "Summarization failed"
		}
		return BranchSummaryResult{Error: text}, nil
	}
	parts := []string{}
	for _, block := range response.Content {
		if text, ok := block.(ai.TextContent); ok {
			parts = append(parts, text.Text)
		}
	}
	result := BranchSummaryResult{Summary: "The user explored a different conversation branch before returning here.\nSummary of that exploration:\n\n" + strings.Join(parts, "\n"), Usage: &response.Usage, ReadFiles: []string{}, ModifiedFiles: []string{}}
	for f := range p.FileOps.Written {
		p.FileOps.Edited[f] = struct{}{}
	}
	for f := range p.FileOps.Read {
		if _, ok := p.FileOps.Edited[f]; !ok {
			result.ReadFiles = append(result.ReadFiles, f)
		}
	}
	for f := range p.FileOps.Edited {
		result.ModifiedFiles = append(result.ModifiedFiles, f)
	}
	sort.Strings(result.ReadFiles)
	sort.Strings(result.ModifiedFiles)
	if len(result.ReadFiles) > 0 {
		result.Summary += "\n\n<read-files>\n" + strings.Join(result.ReadFiles, "\n") + "\n</read-files>"
	}
	if len(result.ModifiedFiles) > 0 {
		result.Summary += "\n\n<modified-files>\n" + strings.Join(result.ModifiedFiles, "\n") + "\n</modified-files>"
	}
	return result, nil
}

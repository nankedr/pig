package ai

import "regexp"

var overflowPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)prompt is too long`),
	regexp.MustCompile(`(?i)request_too_large`),
	regexp.MustCompile(`(?i)input is too long for requested model`),
	regexp.MustCompile(`(?i)exceeds the context window`),
	regexp.MustCompile(`(?i)exceeds (?:the )?(?:model'?s )?maximum context length(?: of [\d,]+ tokens?|\s*\([\d,]+\))`),
	regexp.MustCompile(`(?i)input token count.*exceeds the maximum`),
	regexp.MustCompile(`(?i)maximum prompt length is \d+`),
	regexp.MustCompile(`(?i)reduce the length of the messages`),
	regexp.MustCompile(`(?i)maximum context length is \d+ tokens`),
	regexp.MustCompile(`(?i)exceeds (?:the )?maximum allowed input length of [\d,]+ tokens?`),
	regexp.MustCompile(`(?i)input \(\d+ tokens\) is longer than the model'?s context length \(\d+ tokens\)`),
	regexp.MustCompile(`(?i)exceeds the limit of \d+`),
	regexp.MustCompile(`(?i)exceeds the available context size`),
	regexp.MustCompile(`(?i)greater than the context length`),
	regexp.MustCompile(`(?i)context window exceeds limit`),
	regexp.MustCompile(`(?i)exceeded model token limit`),
	regexp.MustCompile(`(?i)too large for model with \d+ maximum context length`),
	regexp.MustCompile(`(?i)prompt has [\d,]+ tokens?, but the configured context size is [\d,]+ tokens?`),
	regexp.MustCompile(`(?i)model_context_window_exceeded`),
	regexp.MustCompile(`(?i)prompt too long; exceeded (?:max )?context length`),
	regexp.MustCompile(`(?i)range of input length should be`),
	regexp.MustCompile(`(?i)context[_ ]length[_ ]exceeded`),
	regexp.MustCompile(`(?i)too many tokens`),
	regexp.MustCompile(`(?i)token limit exceeded`),
	regexp.MustCompile(`(?i)^4(?:00|13)\s*(?:status code)?\s*\(no body\)`),
}

var nonOverflowPattern = regexp.MustCompile(`(?i)^(Throttling error|Service unavailable):|rate limit|too many requests`)

// IsContextOverflow recognizes fixed-baseline Provider errors and, when a
// context window is supplied, silent overflow and zero-output length stops.
func IsContextOverflow(message AssistantMessage, contextWindow ...int64) bool {
	if text, ok := message.ErrorMessage.Value(); ok && message.StopReason == StopReasonError && !nonOverflowPattern.MatchString(text) {
		for _, pattern := range overflowPatterns {
			if pattern.MatchString(text) {
				return true
			}
		}
	}
	if len(contextWindow) == 0 || contextWindow[0] == 0 {
		return false
	}
	input := message.Usage.Input + message.Usage.CacheRead
	if message.StopReason == StopReasonStop {
		return input > contextWindow[0]
	}
	return message.StopReason == StopReasonLength && message.Usage.Output == 0 && float64(input) >= float64(contextWindow[0])*0.99
}

// IsRecoverableLength compares output with the original desired limit, before
// context-based clamping. It does not perform compaction or retry.
func IsRecoverableLength(message AssistantMessage, desiredMaxOutput int64) bool {
	return message.StopReason == StopReasonLength && desiredMaxOutput > 0 && message.Usage.Output < desiredMaxOutput
}

func GetOverflowPatterns() []*regexp.Regexp {
	patterns := make([]*regexp.Regexp, len(overflowPatterns))
	for i, pattern := range overflowPatterns {
		patterns[i] = regexp.MustCompile(pattern.String())
	}
	return patterns
}

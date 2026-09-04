package ai

import (
	"bytes"
	"encoding/json"
	"unicode/utf16"
)

type ContextUsageEstimate struct {
	Tokens         int64 `json:"tokens"`
	UsageTokens    int64 `json:"usageTokens"`
	TrailingTokens int64 `json:"trailingTokens"`
	LastUsageIndex *int  `json:"lastUsageIndex"`
}

// EstimateContextTokens reuses the latest usage that still describes the
// transcript prefix. LastUsageIndex is nil when all tokens are estimated.
func EstimateContextTokens(input Context) ContextUsageEstimate {
	index, tokens := lastAssistantUsage(input.Messages)
	estimate := ContextUsageEstimate{UsageTokens: tokens}
	if index >= 0 {
		estimate.LastUsageIndex = &index
	} else if system, ok := input.SystemPrompt.Value(); ok {
		estimate.TrailingTokens = estimateCharacters(jsStringLength(system))
	}
	addedNames := map[string]bool{}
	for _, message := range input.Messages[index+1:] {
		estimate.TrailingTokens += estimateMessageTokens(message)
		var names Optional[[]string]
		switch value := message.(type) {
		case ToolResultMessage:
			names = value.AddedToolNames
		case *ToolResultMessage:
			if value != nil {
				names = value.AddedToolNames
			}
		}
		if values, ok := names.Value(); ok {
			for _, name := range values {
				addedNames[name] = true
			}
		}
	}
	tools := input.Tools
	if index >= 0 {
		tools = nil
		for _, tool := range input.Tools {
			if addedNames[tool.Name] {
				tools = append(tools, tool)
			}
		}
	}
	if len(tools) > 0 {
		estimate.TrailingTokens += estimateCharacters(estimateJSONCharacters(tools))
	}
	estimate.Tokens = estimate.UsageTokens + estimate.TrailingTokens
	return estimate
}

func estimateMessageTokens(message Message) int64 {
	chars := 0
	switch value := message.(type) {
	case UserMessage:
		if value.Content.isText {
			chars = jsStringLength(value.Content.text)
		} else {
			for _, block := range value.Content.blocks {
				chars += estimateContentCharacters(block)
			}
		}
	case AssistantMessage:
		for _, block := range value.Content {
			chars += estimateContentCharacters(block)
		}
	case ToolResultMessage:
		for _, block := range value.Content {
			chars += estimateContentCharacters(block)
		}
	case *UserMessage:
		if value != nil {
			return estimateMessageTokens(*value)
		}
	case *AssistantMessage:
		if value != nil {
			return estimateMessageTokens(*value)
		}
	case *ToolResultMessage:
		if value != nil {
			return estimateMessageTokens(*value)
		}
	}
	return estimateCharacters(chars)
}

func estimateContentCharacters(content Content) int {
	switch value := content.(type) {
	case TextContent:
		return jsStringLength(value.Text)
	case ThinkingContent:
		return jsStringLength(value.Thinking)
	case ImageContent:
		return 4800
	case ToolCall:
		return jsStringLength(value.Name) + estimateJSONCharacters(value.Arguments)
	case *TextContent:
		if value != nil {
			return estimateContentCharacters(*value)
		}
	case *ThinkingContent:
		if value != nil {
			return estimateContentCharacters(*value)
		}
	case *ImageContent:
		if value != nil {
			return estimateContentCharacters(*value)
		}
	case *ToolCall:
		if value != nil {
			return estimateContentCharacters(*value)
		}
	}
	return 0
}

func estimateJSONCharacters(value any) int {
	encoded, err := json.Marshal(value)
	if err != nil {
		return len("[unserializable]")
	}
	var decoded any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		return len("[unserializable]")
	}
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(decoded); err != nil {
		return len("[unserializable]")
	}
	text := buffer.String()
	chars := jsStringLength(text) - 1
	// JSON.stringify leaves Unicode separators unescaped, unlike encoding/json.
	for i := 0; i < len(text)-1; i++ {
		if text[i] == '\\' {
			if i+6 <= len(text) && (text[i:i+6] == `\u2028` || text[i:i+6] == `\u2029`) {
				chars -= 5
			}
			i++
		}
	}
	return chars
}

func lastAssistantUsage(messages []Message) (int, int64) {
	latestTimestamp := int64(-1 << 63)
	index, tokens := -1, int64(0)
	for i, message := range messages {
		var assistant *AssistantMessage
		switch value := message.(type) {
		case AssistantMessage:
			assistant = &value
		case *AssistantMessage:
			assistant = value
		}
		if assistant != nil && assistant.Timestamp >= latestTimestamp && assistant.StopReason != StopReasonAborted && assistant.StopReason != StopReasonError {
			usageTokens := assistant.Usage.TotalTokens
			if usageTokens == 0 {
				usageTokens = assistant.Usage.Input + assistant.Usage.Output + assistant.Usage.CacheRead + assistant.Usage.CacheWrite
			}
			if usageTokens > 0 {
				index, tokens = i, usageTokens
			}
		}
		switch value := message.(type) {
		case UserMessage:
			latestTimestamp = max(latestTimestamp, value.Timestamp)
		case *UserMessage:
			if value != nil {
				latestTimestamp = max(latestTimestamp, value.Timestamp)
			}
		case AssistantMessage:
			latestTimestamp = max(latestTimestamp, value.Timestamp)
		case *AssistantMessage:
			if value != nil {
				latestTimestamp = max(latestTimestamp, value.Timestamp)
			}
		case ToolResultMessage:
			latestTimestamp = max(latestTimestamp, value.Timestamp)
		case *ToolResultMessage:
			if value != nil {
				latestTimestamp = max(latestTimestamp, value.Timestamp)
			}
		}
	}
	return index, tokens
}

func estimateCharacters(characters int) int64 {
	return int64((characters + 3) / 4)
}

func jsStringLength(value string) int {
	return len(utf16.Encode([]rune(value)))
}

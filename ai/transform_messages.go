package ai

import (
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode"
)

// ToolCallIDNormalizer applies the target API's ID constraints during handoff.
type ToolCallIDNormalizer func(id string, model Model, source AssistantMessage) string

// TransformMessages prepares history for replay without changing the original messages.
func TransformMessages(messages []Message, model Model, normalizers ...ToolCallIDNormalizer) ([]Message, error) {
	var normalize ToolCallIDNormalizer
	if len(normalizers) > 0 {
		normalize = normalizers[0]
	}
	ids := map[string]string{}
	result := make([]Message, 0, len(messages))
	for _, message := range messages {
		if isNilClosedUnion(message) {
			return nil, fmt.Errorf("nil replay message")
		}
		switch value := message.(type) {
		case *UserMessage:
			message = *value
		case *AssistantMessage:
			message = *value
		case *ToolResultMessage:
			message = *value
		}
		switch value := message.(type) {
		case UserMessage:
			if blocks, ok := value.Content.Blocks(); ok {
				if err := checkReplayImages(blocks, model); err != nil {
					return nil, newNotImplemented("TransformMessages.UserImage")
				}
				value.Content = UserBlocks(blocks...)
			}
			result = append(result, value)
		case ToolResultMessage:
			if err := checkReplayImages(value.Content, model); err != nil {
				return nil, newNotImplemented("TransformMessages.ToolResultImage")
			}
			if id := ids[value.ToolCallID]; id != "" {
				value.ToolCallID = id
			}
			content := make([]ToolResultContent, len(value.Content))
			for i, block := range value.Content {
				if normalized := replayContentValue(block); normalized != nil {
					content[i] = normalized.(ToolResultContent)
				}
			}
			value.Content = content
			value.Details = cloneOptional(value.Details, cloneJSONValue)
			value.AddedToolNames = cloneOptional(value.AddedToolNames, func(names []string) []string { return append([]string{}, names...) })
			result = append(result, value)
		case AssistantMessage:
			value = CloneAssistantMessage(value)
			content := make([]AssistantContent, 0, len(value.Content))
			sameModel := value.Provider == model.Provider && value.API == model.API && value.Model == model.ID
			for _, block := range value.Content {
				switch block := replayContentValue(block).(type) {
				case ThinkingContent:
					redacted, _ := block.Redacted.Value()
					signature, _ := block.ThinkingSignature.Value()
					if redacted {
						if sameModel {
							content = append(content, block)
						}
					} else if sameModel && signature != "" || hasReplayText(block.Thinking) {
						if sameModel {
							content = append(content, block)
						} else {
							content = append(content, TextContent{Type: ContentTypeText, Text: block.Thinking})
						}
					}
				case TextContent:
					if !sameModel {
						block.TextSignature = Absent[string]()
					}
					content = append(content, block)
				case ToolCall:
					if !sameModel {
						if signature, _ := block.ThoughtSignature.Value(); signature != "" {
							block.ThoughtSignature = Absent[string]()
						}
						if normalize != nil {
							id := normalize(block.ID, cloneModel(model), CloneAssistantMessage(value))
							if id != block.ID {
								ids[block.ID] = id
								block.ID = id
							}
						}
					}
					content = append(content, block)
				}
			}
			value.Content = content
			result = append(result, value)
		}
	}
	replay := make([]Message, 0, len(result))
	var pending []ToolCall
	existing := map[string]bool{}
	flush := func() {
		for _, call := range pending {
			if !existing[call.ID] {
				replay = append(replay, ToolResultMessage{
					Role: MessageRoleToolResult, ToolCallID: call.ID, ToolName: call.Name,
					Content: []ToolResultContent{TextContent{Type: ContentTypeText, Text: "No result provided"}},
					IsError: true, Timestamp: time.Now().UnixMilli(),
				})
			}
		}
		pending = nil
		clear(existing)
	}
	for _, message := range result {
		switch value := message.(type) {
		case AssistantMessage:
			flush()
			if value.StopReason == StopReasonError || value.StopReason == StopReasonAborted {
				continue
			}
			for _, block := range value.Content {
				if call, ok := block.(ToolCall); ok {
					pending = append(pending, call)
				}
			}
		case UserMessage:
			flush()
		case ToolResultMessage:
			existing[value.ToolCallID] = true
		}
		replay = append(replay, message)
	}
	flush()
	return replay, nil
}

func hasReplayText(text string) bool {
	// Match ECMAScript trim: BOM is whitespace, U+0085 is not.
	return strings.TrimFunc(text, func(r rune) bool {
		return r == '\ufeff' || r != '\u0085' && unicode.IsSpace(r)
	}) != ""
}

func checkReplayImages[T Content](blocks []T, model Model) error {
	if !slices.Contains(model.Input, ModelInputImage) {
		for _, block := range blocks {
			if _, ok := replayContentValue(block).(ImageContent); ok {
				return newNotImplemented("TransformMessages.ImageDowngrade")
			}
		}
	}
	return nil
}

func replayContentValue(content Content) Content {
	if isNilClosedUnion(content) {
		return nil
	}
	if value, ok := content.(*TextContent); ok {
		return *value
	}
	if value, ok := content.(*ThinkingContent); ok {
		return *value
	}
	if value, ok := content.(*ToolCall); ok {
		return *value
	}
	if value, ok := content.(*ImageContent); ok {
		return *value
	}
	return content
}

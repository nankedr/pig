package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/nankedr/pig/ai"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	ctx := context.Background()
	faux, err := ai.NewFauxProvider(ai.RegisterFauxProviderOptions{})
	if err != nil {
		return err
	}
	for _, name := range []string{"discover", "read", ""} {
		faux.AppendResponses([]ai.FauxResponseStep{ai.FauxResponseFactory(func(input ai.Context, _ *ai.SimpleStreamOptions, _ *ai.FauxProviderState, _ ai.Model) (ai.AssistantMessage, error) {
			immediate, deferred := ai.SplitDeferredTools(input, true, nil)
			fmt.Printf("immediate=%s deferred=%s\n", names(immediate), names(deferred))
			if name == "" {
				return ai.FauxAssistantMessage(ai.FauxAssistantText("done"))
			}
			call, err := ai.FauxToolCall(name, map[string]any{})
			if err != nil {
				return ai.AssistantMessage{}, err
			}
			return ai.FauxAssistantMessage(ai.FauxAssistantBlocks(call), ai.FauxAssistantMessageOptions{StopReason: ai.Some(ai.StopReasonToolUse)})
		})})
	}
	models := ai.CreateModels()
	models.SetProvider(faux.Provider)
	model, _ := faux.GetModel()
	tool := func(name string) ai.Tool {
		return ai.Tool{Name: name, Description: name, Parameters: json.RawMessage(`{"type":"object","properties":{}}`)}
	}
	input := ai.Context{
		Tools:    []ai.Tool{tool("discover")},
		Messages: []ai.Message{ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText("discover and use a tool"), Timestamp: 1}},
	}
	for {
		message, err := models.CompleteSimple(ctx, model, input)
		if err != nil {
			return err
		}
		input.Messages = append(input.Messages, message)
		if message.StopReason == ai.StopReasonStop {
			fmt.Println(message.Content[0].(ai.TextContent).Text)
			return nil
		}
		if message.StopReason != ai.StopReasonToolUse {
			return fmt.Errorf("unexpected stop reason: %s", message.StopReason)
		}
		call := message.Content[0].(ai.ToolCall)
		result := ai.ToolResultMessage{Role: ai.MessageRoleToolResult, ToolCallID: call.ID, ToolName: call.Name, Content: []ai.ToolResultContent{ai.FauxText("file contents")}, Timestamp: 2}
		if call.Name == "discover" {
			input.Tools = append(input.Tools, tool("read"), tool("unused"))
			result.Content = []ai.ToolResultContent{ai.FauxText("read and unused discovered")}
			result.AddedToolNames = ai.Some([]string{"read", "read", "unused"})
		}
		input.Messages = append(input.Messages, result)
	}
}

func names(tools []ai.Tool) string {
	result := make([]string, len(tools))
	for i, tool := range tools {
		result[i] = tool.Name
	}
	return strings.Join(result, ",")
}

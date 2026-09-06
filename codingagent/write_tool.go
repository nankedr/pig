package codingagent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"unicode/utf16"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
)

// ToolExecuteFunc executes built-in tools; it does not define an extension host ABI.
type ToolExecuteFunc func(context.Context, string, ai.JSONValue, agent.AgentToolUpdateCallback[ai.JSONValue]) (agent.ErasedAgentToolResult, error)

func CreateWriteToolDefinition(cwd string, options ...WriteToolOptions) (ToolDefinition, error) {
	var operations WriteOperations = localWriteOperations{}
	if len(options) > 0 && options[0].Operations != nil {
		operations = options[0].Operations
	}
	return ToolDefinition{
		Name: "write", Label: "write",
		Description:      "Write content to a file. Creates the file if it doesn't exist, overwrites if it does. Automatically creates parent directories.",
		Parameters:       json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"Path to the file to write (relative or absolute)"},"content":{"type":"string","description":"Content to write to the file"}},"required":["path","content"]}`),
		PromptSnippet:    "Create or overwrite files",
		PromptGuidelines: []string{"Use write only for new files or complete rewrites."},
		Execute: func(ctx context.Context, _ string, args ai.JSONValue, _ agent.AgentToolUpdateCallback[ai.JSONValue]) (agent.ErasedAgentToolResult, error) {
			object, _ := args.(map[string]any)
			path, pathOK := object["path"].(string)
			content, contentOK := object["content"].(string)
			if !pathOK || !contentOK {
				return agent.ErasedAgentToolResult{}, fmt.Errorf("write requires string path and content")
			}
			absolutePath, err := resolveToolPath(path, cwd)
			if err != nil {
				return agent.ErasedAgentToolResult{}, err
			}
			return WithFileMutationQueue(ctx, absolutePath, func(ctx context.Context) (agent.ErasedAgentToolResult, error) {
				if err := operations.Mkdir(ctx, filepath.Dir(absolutePath)); err != nil {
					return agent.ErasedAgentToolResult{}, err
				}
				if err := context.Cause(ctx); err != nil {
					return agent.ErasedAgentToolResult{}, err
				}
				if err := operations.WriteFile(ctx, absolutePath, []byte(content)); err != nil {
					return agent.ErasedAgentToolResult{}, err
				}
				if err := context.Cause(ctx); err != nil {
					return agent.ErasedAgentToolResult{}, err
				}
				return agent.ErasedAgentToolResult{Content: []ai.ToolResultContent{ai.TextContent{Type: "text", Text: fmt.Sprintf("Successfully wrote %d bytes to %s", len(utf16.Encode([]rune(content))), path)}}}, nil
			})
		},
	}, nil
}

func CreateWriteTool(cwd string, options ...WriteToolOptions) (agent.ErasedAgentTool, error) {
	definition, err := CreateWriteToolDefinition(cwd, options...)
	if err != nil {
		return agent.ErasedAgentTool{}, err
	}
	return agent.EraseAgentTool(agent.AgentTool[ai.JSONValue, ai.JSONValue]{
		Tool:            ai.Tool{Name: definition.Name, Description: definition.Description, Parameters: definition.Parameters},
		Label:           definition.Label,
		DecodeValidated: func(value ai.JSONValue) ai.JSONValue { return value },
		Execute:         definition.Execute,
	})
}

type localWriteOperations struct{}

func (localWriteOperations) Mkdir(ctx context.Context, path string) error {
	if err := context.Cause(ctx); err != nil {
		return err
	}
	return os.MkdirAll(path, 0777)
}
func (localWriteOperations) WriteFile(ctx context.Context, path string, content []byte) error {
	if err := context.Cause(ctx); err != nil {
		return err
	}
	return os.WriteFile(path, content, 0666)
}

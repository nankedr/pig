package codingagent

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
	"golang.org/x/text/collate"
	"golang.org/x/text/language"
)

func CreateLsToolDefinition(cwd string, options ...LsToolOptions) (ToolDefinition, error) {
	var operations LsOperations = localLsOperations{}
	if len(options) > 0 && options[0].Operations != nil {
		operations = options[0].Operations
	}
	return ToolDefinition{
		Name: "ls", Label: "ls",
		Description:   fmt.Sprintf("List directory contents. Returns entries sorted alphabetically, with '/' suffix for directories. Includes dotfiles. Output is truncated to 500 entries or %dKB (whichever is hit first).", DefaultMaxBytes/1024),
		PromptSnippet: "List directory contents",
		Parameters:    json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"Directory to list (default: current directory)"},"limit":{"type":"number","description":"Maximum number of entries to return (default: 500)"}}}`),
		Execute: func(ctx context.Context, _ string, args ai.JSONValue, _ agent.AgentToolUpdateCallback[ai.JSONValue]) (agent.ErasedAgentToolResult, error) {
			if err := context.Cause(ctx); err != nil {
				return agent.ErasedAgentToolResult{}, err
			}
			path, limit, err := directoryToolArguments(cwd, args, 500)
			if err != nil {
				return agent.ErasedAgentToolResult{}, err
			}
			exists, err := operations.Exists(ctx, path)
			if err != nil {
				return agent.ErasedAgentToolResult{}, err
			}
			if err := context.Cause(ctx); err != nil {
				return agent.ErasedAgentToolResult{}, err
			}
			if !exists {
				return agent.ErasedAgentToolResult{}, fmt.Errorf("Path not found: %s", path)
			}
			info, err := operations.Stat(ctx, path)
			if err != nil {
				return agent.ErasedAgentToolResult{}, err
			}
			if err := context.Cause(ctx); err != nil {
				return agent.ErasedAgentToolResult{}, err
			}
			if !info.IsDirectory() {
				return agent.ErasedAgentToolResult{}, fmt.Errorf("Not a directory: %s", path)
			}
			entries, err := operations.Readdir(ctx, path)
			if err != nil {
				return agent.ErasedAgentToolResult{}, fmt.Errorf("Cannot read directory: %w", err)
			}
			if err := context.Cause(ctx); err != nil {
				return agent.ErasedAgentToolResult{}, err
			}
			entries = append([]string{}, entries...)
			ordering := collate.New(language.English)
			sort.SliceStable(entries, func(i, j int) bool {
				return ordering.CompareString(strings.ToLower(entries[i]), strings.ToLower(entries[j])) < 0
			})
			results := []string{}
			notice := ""
			details := map[string]any{}
			for _, entry := range entries {
				if err := context.Cause(ctx); err != nil {
					return agent.ErasedAgentToolResult{}, err
				}
				if float64(len(results)) >= limit {
					notice = fmt.Sprintf("%s entries limit reached. Use limit=%s for more", directoryLimitString(limit), directoryLimitString(limit*2))
					details["entryLimitReached"] = limit
					break
				}
				info, err := operations.Stat(ctx, filepath.Join(path, entry))
				if err := context.Cause(ctx); err != nil {
					return agent.ErasedAgentToolResult{}, err
				}
				if err != nil {
					continue
				}
				if info.IsDirectory() {
					entry += "/"
				}
				results = append(results, entry)
			}
			if len(results) == 0 {
				return directoryToolResult("(empty directory)", nil), nil
			}
			return formatDirectoryResults(results, notice, details), nil
		},
	}, nil
}

func CreateLsTool(cwd string, options ...LsToolOptions) (agent.ErasedAgentTool, error) {
	definition, err := CreateLsToolDefinition(cwd, options...)
	if err != nil {
		return agent.ErasedAgentTool{}, err
	}
	return eraseDirectoryTool(definition)
}

func eraseDirectoryTool(definition ToolDefinition) (agent.ErasedAgentTool, error) {
	return agent.EraseAgentTool(agent.AgentTool[ai.JSONValue, ai.JSONValue]{Tool: ai.Tool{Name: definition.Name, Description: definition.Description, Parameters: definition.Parameters}, Label: definition.Label, DecodeValidated: func(value ai.JSONValue) ai.JSONValue { return value }, Execute: definition.Execute})
}

func directoryToolArguments(cwd string, args ai.JSONValue, limit float64) (string, float64, error) {
	object, ok := args.(map[string]any)
	if !ok {
		return "", 0, fmt.Errorf("directory tool requires an object")
	}
	path := "."
	if value, ok := object["path"]; ok {
		var valid bool
		path, valid = value.(string)
		if !valid {
			return "", 0, fmt.Errorf("path must be a string")
		}
		if path == "" {
			path = "."
		}
	}
	if value, ok := object["limit"]; ok {
		var valid bool
		limit, valid = readToolNumber(value)
		if !valid || math.IsNaN(limit) || math.IsInf(limit, 0) {
			return "", 0, fmt.Errorf("limit must be a finite number")
		}
	}
	absolute, err := resolveToolPath(path, cwd)
	return absolute, limit, err
}

func directoryLimitString(limit float64) string { return strconv.FormatFloat(limit, 'f', -1, 64) }

func directoryToolResult(text string, details ai.JSONValue) agent.ErasedAgentToolResult {
	return agent.ErasedAgentToolResult{Content: []ai.ToolResultContent{ai.TextContent{Type: "text", Text: text}}, Details: details}
}

func formatDirectoryResults(paths []string, notice string, details map[string]any) agent.ErasedAgentToolResult {
	truncation := TruncateHead(strings.Join(paths, "\n"), TruncationOptions{MaxLines: 9007199254740991})
	if truncation.Truncated {
		if notice != "" {
			notice += ". "
		}
		notice += FormatSize(DefaultMaxBytes) + " limit reached"
		details["truncation"] = readToolTruncationJSON(truncation)
	}
	text := truncation.Content
	if notice != "" {
		text += "\n\n[" + notice + "]"
	}
	if len(details) == 0 {
		return directoryToolResult(text, nil)
	}
	return directoryToolResult(text, details)
}

type localLsOperations struct{}
type localLsFileInfo struct{ os.FileInfo }

func (i localLsFileInfo) IsDirectory() bool { return i.IsDir() }
func (localLsOperations) Exists(ctx context.Context, path string) (bool, error) {
	if err := context.Cause(ctx); err != nil {
		return false, err
	}
	return readToolPathExists(path), nil
}
func (localLsOperations) Stat(ctx context.Context, path string) (LsFileInfo, error) {
	if err := context.Cause(ctx); err != nil {
		return nil, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	return localLsFileInfo{info}, nil
}
func (localLsOperations) Readdir(ctx context.Context, path string) ([]string, error) {
	if err := context.Cause(ctx); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	names := make([]string, len(entries))
	for i, entry := range entries {
		names[i] = entry.Name()
	}
	return names, nil
}

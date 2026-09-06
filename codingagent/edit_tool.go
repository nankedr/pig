package codingagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"syscall"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
	"golang.org/x/text/encoding/unicode"
)

const editPromptSnippet = "Make precise file edits with exact text replacement, including multiple disjoint edits in one call"

var editPromptGuidelines = []string{"Use edit for precise changes (edits[].oldText must match exactly)", "When changing multiple separate locations in one file, use one edit call with multiple entries in edits[] instead of multiple edit calls", "Each edits[].oldText is matched against the original file, not after earlier edits are applied. Do not emit overlapping or nested edits. Merge nearby changes into one edit.", "Keep edits[].oldText as small as possible while still being unique in the file. Do not pad with large unchanged regions."}

func CreateEditToolDefinition(cwd string, options ...EditToolOptions) (ToolDefinition, error) {
	var operations EditOperations = localEditOperations{}
	if len(options) > 0 && options[0].Operations != nil {
		operations = options[0].Operations
	}
	return ToolDefinition{
		Name: "edit", Label: "edit",
		Description:      "Edit a single file using exact text replacement. Every edits[].oldText must match a unique, non-overlapping region of the original file. If two changes affect the same block or nearby lines, merge them into one edit instead of emitting overlapping edits. Do not include large unchanged regions just to connect distant changes.",
		Parameters:       json.RawMessage(`{"type":"object","required":["path","edits"],"properties":{"path":{"type":"string","description":"Path to the file to edit (relative or absolute)"},"edits":{"type":"array","items":{"type":"object","required":["oldText","newText"],"properties":{"oldText":{"type":"string","description":"Exact text for one targeted replacement. It must be unique in the original file and must not overlap with any other edits[].oldText in the same call."},"newText":{"type":"string","description":"Replacement text for this targeted edit."}}},"description":"One or more targeted replacements. Each edit is matched against the original file, not incrementally. Do not include overlapping or nested edits. If two changes touch the same block or nearby lines, merge them into one edit instead."}}}`),
		PrepareArguments: prepareEditArguments,
		PromptSnippet:    editPromptSnippet, PromptGuidelines: append([]string(nil), editPromptGuidelines...),
		Execute: func(ctx context.Context, _ string, args ai.JSONValue, _ agent.AgentToolUpdateCallback[ai.JSONValue]) (agent.ErasedAgentToolResult, error) {
			var zero agent.ErasedAgentToolResult
			object, _ := args.(map[string]any)
			path, ok := object["path"].(string)
			if !ok {
				return zero, fmt.Errorf("edit requires string path")
			}
			rawEdits, ok := object["edits"].([]any)
			if !ok || len(rawEdits) == 0 {
				return zero, fmt.Errorf("Edit tool input is invalid. edits must contain at least one replacement.")
			}
			edits := make([]Edit, len(rawEdits))
			for i, raw := range rawEdits {
				edit, _ := raw.(map[string]any)
				old, oldOK := edit["oldText"].(string)
				newText, newOK := edit["newText"].(string)
				if !oldOK || !newOK {
					return zero, fmt.Errorf("edits[%d] requires string oldText and newText", i)
				}
				edits[i] = Edit{OldText: old, NewText: newText}
			}
			absolutePath, err := resolveToolPath(path, cwd)
			if err != nil {
				return zero, err
			}
			return WithFileMutationQueue(ctx, absolutePath, func(ctx context.Context) (agent.ErasedAgentToolResult, error) {
				if err := operations.Access(ctx, absolutePath); err != nil {
					if cause := context.Cause(ctx); cause != nil {
						return zero, cause
					}
					message := "Error: " + err.Error()
					var errno syscall.Errno
					if errors.As(err, &errno) {
						message = "Error code: " + editErrorCode(errno)
					}
					return zero, fmt.Errorf("Could not edit file: %s. %s.", path, message)
				}
				if err := context.Cause(ctx); err != nil {
					return zero, err
				}
				data, err := operations.ReadFile(ctx, absolutePath)
				if err != nil {
					return zero, err
				}
				if err := context.Cause(ctx); err != nil {
					return zero, err
				}
				raw, _ := unicode.UTF8.NewDecoder().String(string(data))
				bom := ""
				if strings.HasPrefix(raw, "\ufeff") {
					bom = "\ufeff"
					raw = strings.TrimPrefix(raw, bom)
				}
				crlf, lf := strings.Index(raw, "\r\n"), strings.Index(raw, "\n")
				base := normalizeEditLF(raw)
				content, err := applyFileEdits(base, edits, path)
				if err != nil {
					return zero, err
				}
				if err := context.Cause(ctx); err != nil {
					return zero, err
				}
				final := content
				if crlf >= 0 && crlf < lf {
					final = strings.ReplaceAll(final, "\n", "\r\n")
				}
				if err := operations.WriteFile(ctx, absolutePath, []byte(bom+final)); err != nil {
					return zero, err
				}
				if err := context.Cause(ctx); err != nil {
					return zero, err
				}
				details := editDiffDetails(path, base, content)
				return agent.ErasedAgentToolResult{Content: []ai.ToolResultContent{ai.TextContent{Type: "text", Text: fmt.Sprintf("Successfully replaced %d block(s) in %s.", len(edits), path)}}, Details: map[string]any{"diff": details.Diff, "patch": details.Patch, "firstChangedLine": details.FirstChangedLine}}, nil
			})
		},
	}, nil
}

func CreateEditTool(cwd string, options ...EditToolOptions) (agent.ErasedAgentTool, error) {
	definition, err := CreateEditToolDefinition(cwd, options...)
	if err != nil {
		return agent.ErasedAgentTool{}, err
	}
	return agent.EraseAgentTool(agent.AgentTool[ai.JSONValue, ai.JSONValue]{
		Tool: ai.Tool{Name: definition.Name, Description: definition.Description, Parameters: definition.Parameters}, Label: definition.Label,
		PrepareArguments: definition.PrepareArguments,
		DecodeValidated:  func(value ai.JSONValue) ai.JSONValue { return value }, Execute: definition.Execute,
	})
}

func normalizeEditLF(text string) string {
	return strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
}

type localEditOperations struct {
	localReadOperations
	localWriteOperations
}

func (localEditOperations) Access(ctx context.Context, path string) error {
	if err := context.Cause(ctx); err != nil {
		return err
	}
	return editFileAccess(path)
}

func editErrorCode(err syscall.Errno) string {
	switch err {
	case syscall.ENOENT:
		return "ENOENT"
	case syscall.EACCES:
		return "EACCES"
	case syscall.EPERM:
		return "EPERM"
	case syscall.ENOTDIR:
		return "ENOTDIR"
	case syscall.EISDIR:
		return "EISDIR"
	case syscall.ELOOP:
		return "ELOOP"
	case syscall.ENAMETOOLONG:
		return "ENAMETOOLONG"
	}
	return fmt.Sprintf("%d", err)
}

func prepareEditArguments(input ai.JSONValue) (ai.JSONValue, error) {
	object, ok := input.(map[string]any)
	if !ok {
		return input, nil
	}
	if text, ok := object["edits"].(string); ok {
		var parsed any
		if json.Unmarshal([]byte(text), &parsed) == nil {
			if edits, ok := parsed.([]any); ok {
				object["edits"] = edits
			}
		}
	}
	old, oldOK := object["oldText"].(string)
	newText, newOK := object["newText"].(string)
	if !oldOK || !newOK {
		return object, nil
	}
	result := make(map[string]any, len(object))
	for key, value := range object {
		if key != "oldText" && key != "newText" {
			result[key] = value
		}
	}
	edits, _ := object["edits"].([]any)
	result["edits"] = append(append([]any{}, edits...), map[string]any{"oldText": old, "newText": newText})
	return result, nil
}

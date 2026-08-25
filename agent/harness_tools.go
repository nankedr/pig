package agent

import (
	"context"
	"encoding/json"

	"github.com/nankedr/pig/ai"
)

type AgentHarnessToolContextSource func(context.Context) (any, error)

type AgentHarnessTool[TContext, TParameters, TDetails any] struct {
	ai.Tool
	Label            string
	PrepareArguments PrepareArgumentsFunc
	Execute          func(context.Context, string, TParameters, AgentToolUpdateCallback[TDetails], TContext) (AgentToolResult[TDetails], error)
	ExecutionMode    ToolExecutionMode
}

type HarnessToolExecute func(context.Context, string, ai.JSONValue, AgentToolUpdateCallback[ai.JSONValue], ExecutionToolContext) (ErasedAgentToolResult, error)

type HarnessTool struct {
	ai.Tool
	Label            string
	PrepareArguments PrepareArgumentsFunc
	Execute          HarnessToolExecute
	ExecutionMode    ToolExecutionMode
	Replay           ToolReplay
}

type ExecutionToolContext struct {
	Env ExecutionEnv
}

type BashToolInput struct {
	Command string
	Timeout *float64
}

type BashToolDetails struct {
	Truncation     *TruncationResult
	FullOutputPath string
}

type BashExecution struct {
	Command    string
	CWD        string
	Env        map[string]string
	InheritEnv bool
}

type BashPrepare func(context.Context, *BashExecution, ExecutionToolContext) error

type BashToolOptions struct {
	CommandPrefix string
	Prepare       BashPrepare
}

type Edit struct {
	OldText string
	NewText string
}

type EditToolInput struct {
	Path  string
	Edits []Edit
}

type EditToolDetails struct {
	Diff             string
	Patch            string
	FirstChangedLine int
}

type ReadToolInput struct {
	Path   string
	Offset *int
	Limit  *int
}

type ReadToolDetails struct {
	Truncation *TruncationResult
}

type ReadImageProcessorResult struct {
	OK       bool
	Data     string
	MIMEType string
	Hints    []string
	Message  string
}

type ReadImageProcessor func(context.Context, []byte, string, bool) (ReadImageProcessorResult, error)

type ReadToolOptions struct {
	AutoResizeImages *bool
	ImageProcessor   ReadImageProcessor
}

type WriteToolInput struct {
	Path    string
	Content string
}

func CreateBashTool(options BashToolOptions) HarnessTool {
	return newDeferredHarnessTool("bash", "Execute a bash command in the current working directory.", bashToolSchema, options.Prepare)
}

func CreateEditTool() HarnessTool {
	return newDeferredHarnessTool("edit", "Edit a single file using exact text replacement.", editToolSchema, nil)
}

func CreateReadTool(options ReadToolOptions) HarnessTool {
	return newDeferredHarnessTool("read", "Read the contents of a file.", readToolSchema, options.ImageProcessor)
}

func CreateWriteTool() HarnessTool {
	return newDeferredHarnessTool("write", "Write content to a file.", writeToolSchema, nil)
}

func newDeferredHarnessTool(name, description string, parameters json.RawMessage, retained any) HarnessTool {
	_ = retained
	return HarnessTool{
		Tool:   ai.Tool{Name: name, Description: description, Parameters: append(json.RawMessage(nil), parameters...)},
		Label:  name,
		Replay: ToolReplayNever,
		Execute: func(context.Context, string, ai.JSONValue, AgentToolUpdateCallback[ai.JSONValue], ExecutionToolContext) (ErasedAgentToolResult, error) {
			return ErasedAgentToolResult{}, newNotImplemented("HarnessTool." + name + ".Execute")
		},
	}
}

var (
	bashToolSchema  = json.RawMessage(`{"type":"object","properties":{"command":{"type":"string"},"timeout":{"type":"number"}},"required":["command"]}`)
	editToolSchema  = json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"edits":{"type":"array"}},"required":["path","edits"]}`)
	readToolSchema  = json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"offset":{"type":"number"},"limit":{"type":"number"}},"required":["path"]}`)
	writeToolSchema = json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"content":{"type":"string"}},"required":["path","content"]}`)
)

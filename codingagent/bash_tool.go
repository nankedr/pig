package codingagent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
)

type bashSessionKey struct{}
type bashAbortError struct {
	text  string
	cause error
}

func (e bashAbortError) Error() string { return e.text }
func (e bashAbortError) Unwrap() error { return e.cause }

func CreateBashToolDefinition(cwd string, options ...BashToolOptions) (ToolDefinition, error) {
	var config BashToolOptions
	if len(options) > 0 {
		config = options[0]
	}
	ops := config.Operations
	if ops == nil {
		ops = CreateLocalBashOperations(config)
	}
	definition := ToolDefinition{Name: "bash", Label: "bash", Description: fmt.Sprintf("Execute a bash command in the current working directory. Returns stdout and stderr. Output is truncated to last %d lines or %dKB (whichever is hit first). If truncated, full output is saved to a temp file. Optionally provide a timeout in seconds.", DefaultMaxLines, DefaultMaxBytes/1024), Parameters: json.RawMessage(`{"type":"object","properties":{"command":{"type":"string","description":"Bash command to execute"},"timeout":{"type":"number","description":"Timeout in seconds (optional, no default timeout)"}},"required":["command"]}`), PromptSnippet: "Execute bash commands (ls, grep, find, etc.)"}
	if config.ExposeSessionEnvironment == nil || *config.ExposeSessionEnvironment {
		definition.PromptGuidelines = []string{"You can inspect PIG_* environment variables for current model and session details."}
	}
	definition.Execute = func(ctx context.Context, _ string, args ai.JSONValue, update agent.AgentToolUpdateCallback[ai.JSONValue]) (agent.ErasedAgentToolResult, error) {
		object, _ := args.(map[string]any)
		command, ok := object["command"].(string)
		if !ok {
			return agent.ErasedAgentToolResult{}, fmt.Errorf("bash requires string command")
		}
		var timeout *float64
		if value, exists := object["timeout"]; exists {
			number, ok := readToolNumber(value)
			if !ok {
				return agent.ErasedAgentToolResult{}, fmt.Errorf("bash timeout must be a number")
			}
			timeout = &number
		}
		if config.CommandPrefix != "" {
			command = config.CommandPrefix + "\n" + command
		}
		spawn := BashSpawnContext{Command: command, CWD: cwd, Env: shellEnvironment()}
		for _, key := range []string{"PIG_SESSION_ID", "PIG_SESSION_FILE", "PIG_PROVIDER", "PIG_MODEL", "PIG_REASONING_LEVEL"} {
			delete(spawn.Env, key)
		}
		if config.ExposeSessionEnvironment == nil || *config.ExposeSessionEnvironment {
			if session, ok := ctx.Value(bashSessionKey{}).(*AgentSession); ok {
				state := session.State()
				spawn.Env["PIG_SESSION_ID"] = session.SessionID()
				if file := session.SessionFile(); file != nil {
					spawn.Env["PIG_SESSION_FILE"] = *file
				}
				spawn.Env["PIG_PROVIDER"] = string(state.Model.Provider)
				spawn.Env["PIG_MODEL"] = state.Model.ID
				if state.ThinkingLevel != "" {
					spawn.Env["PIG_REASONING_LEVEL"] = string(state.ThinkingLevel)
				}
			}
		}
		if config.SpawnHook != nil {
			spawn = config.SpawnHook(spawn)
		}
		var mu sync.Mutex
		output := bashOutput{tailBoundary: true}
		var timer *time.Timer
		dirty := false
		var last time.Time
		emit := func() {
			if update == nil || !dirty {
				return
			}
			dirty = false
			last = time.Now()
			trunc, details := output.snapshot()
			update(bashTextResult(trunc.Content, details))
		}
		accepting := true
		if update != nil {
			update(agent.ErasedAgentToolResult{Content: []ai.ToolResultContent{}})
		}
		result, err := ops.Exec(ctx, spawn.Command, spawn.CWD, BashExecOptions{Timeout: timeout, Env: spawn.Env, OnData: func(data []byte) {
			mu.Lock()
			defer mu.Unlock()
			if !accepting {
				return
			}
			output.append(data)
			if update == nil {
				return
			}
			dirty = true
			delay := 100*time.Millisecond - time.Since(last)
			if delay <= 0 {
				if timer != nil {
					timer.Stop()
					timer = nil
				}
				emit()
			} else if timer == nil {
				timer = time.AfterFunc(delay, func() {
					mu.Lock()
					defer mu.Unlock()
					timer = nil
					if accepting {
						emit()
					}
				})
			}
		}})
		mu.Lock()
		accepting = false
		if timer != nil {
			timer.Stop()
		}
		output.decode(nil, true)
		emit()
		text, details, outputErr := output.finish()
		mu.Unlock()
		if outputErr != nil {
			return agent.ErasedAgentToolResult{}, outputErr
		}

		if err != nil {
			status := err.Error()
			if status == "aborted" {
				status = "Command aborted"
			} else if strings.HasPrefix(status, "timeout:") {
				status = "Command timed out after " + strings.TrimPrefix(status, "timeout:") + " seconds"
			} else {
				return agent.ErasedAgentToolResult{}, err
			}
			if text != "" {
				status = text + "\n\n" + status
			}
			if err.Error() == "aborted" && context.Cause(ctx) != nil {
				return agent.ErasedAgentToolResult{}, bashAbortError{text: status, cause: context.Cause(ctx)}
			}
			return agent.ErasedAgentToolResult{}, fmt.Errorf("%s", status)
		}
		if text == "" {
			text = "(no output)"
		}
		if result.ExitCode != nil && *result.ExitCode != 0 {
			return agent.ErasedAgentToolResult{}, fmt.Errorf("%s\n\nCommand exited with code %d", text, *result.ExitCode)
		}
		return bashTextResult(text, details), nil
	}
	return definition, nil
}

func bashTextResult(text string, details ai.JSONValue) agent.ErasedAgentToolResult {
	return agent.ErasedAgentToolResult{Content: []ai.ToolResultContent{ai.TextContent{Type: "text", Text: text}}, Details: details}
}

func CreateBashTool(cwd string, options ...BashToolOptions) (agent.ErasedAgentTool, error) {
	definition, err := CreateBashToolDefinition(cwd, options...)
	if err != nil {
		return agent.ErasedAgentTool{}, err
	}
	return agent.EraseAgentTool(agent.AgentTool[ai.JSONValue, ai.JSONValue]{Tool: ai.Tool{Name: definition.Name, Description: definition.Description, Parameters: definition.Parameters}, Label: definition.Label, DecodeValidated: func(value ai.JSONValue) ai.JSONValue { return value }, Execute: definition.Execute})
}

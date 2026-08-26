package codingagent

import (
	"context"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
)

type NoToolsMode string

const (
	NoToolsAll     NoToolsMode = "all"
	NoToolsBuiltin NoToolsMode = "builtin"
)

type CreateAgentSessionOptions struct {
	CWD, AgentDir       string
	ModelRuntime        *ModelRuntime
	Model               *ai.Model
	ThinkingLevel       agent.ThinkingLevel
	ScopedModels        []ScopedModel
	NoTools             NoToolsMode
	Tools, ExcludeTools []string
	CustomTools         []ToolDefinition
	ResourceLoader      ResourceLoader
	SessionManager      *SessionManager
	SettingsManager     *SettingsManager
	SessionStartEvent   *SessionStartEvent
}
type CreateAgentSessionResult struct {
	Session              *AgentSession
	ExtensionsResult     LoadExtensionsResult
	ModelFallbackMessage *string
}

// CreateAgentSession is an ambient-runtime Capability Stub. It deliberately
// returns before inspecting paths or invoking any supplied service.
func CreateAgentSession(context.Context, ...CreateAgentSessionOptions) (CreateAgentSessionResult, error) {
	return CreateAgentSessionResult{}, notImplemented("CreateAgentSession")
}

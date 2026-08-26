package codingagent

import (
	"context"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
)

type AgentSessionRuntimeDiagnostic struct{ Type, Message string }
type AgentSessionServices struct {
	CWD, AgentDir   string
	Diagnostics     []AgentSessionRuntimeDiagnostic
	ModelRuntime    *ModelRuntime
	ResourceLoader  ResourceLoader
	SettingsManager *SettingsManager
}
type CreateAgentSessionServicesOptions struct {
	CWD, AgentDir               string
	ExtensionFlagValues         map[string]any
	ModelRuntime                *ModelRuntime
	ResourceLoaderOptions       DefaultResourceLoaderOptions
	ResourceLoaderReloadOptions ResourceLoaderReloadOptions
	SettingsManager             *SettingsManager
}
type CreateAgentSessionFromServicesOptions struct {
	Services            AgentSessionServices
	SessionManager      *SessionManager
	SessionStartEvent   *SessionStartEvent
	Model               *ai.Model
	ThinkingLevel       agent.ThinkingLevel
	ScopedModels        []ScopedModel
	Tools, ExcludeTools []string
	NoTools             NoToolsMode
	CustomTools         []ToolDefinition
}

func CreateAgentSessionServices(context.Context, CreateAgentSessionServicesOptions) (AgentSessionServices, error) {
	return AgentSessionServices{}, notImplemented("CreateAgentSessionServices")
}
func CreateAgentSessionFromServices(context.Context, CreateAgentSessionFromServicesOptions) (CreateAgentSessionResult, error) {
	return CreateAgentSessionResult{}, notImplemented("CreateAgentSessionFromServices")
}

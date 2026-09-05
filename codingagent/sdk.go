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

// AgentSessionProvider is the narrow Provider capability needed by the M1
// in-memory session. Full Provider discovery, auth, and refresh remain owned
// by later runtime milestones.
type AgentSessionProvider interface {
	StreamSimple(context.Context, ai.Model, ai.Context, ai.SimpleStreamOptions) *ai.AssistantMessageEventStream
}

type CreateAgentSessionOptions struct {
	CWD, AgentDir       string
	ModelRuntime        *ModelRuntime
	Model               *ai.Model
	Provider            AgentSessionProvider
	StreamFunction      agent.StreamFunction
	ThinkingLevel       agent.ThinkingLevel
	ScopedModels        []ScopedModel
	NoTools             NoToolsMode
	Tools, ExcludeTools []string
	AgentTools          []agent.ErasedAgentTool
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

// CreateAgentSession constructs the SDK path when the caller supplies both a
// model and a Provider or StreamFunction.
func CreateAgentSession(_ context.Context, options ...CreateAgentSessionOptions) (CreateAgentSessionResult, error) {
	if len(options) != 1 || options[0].Model == nil {
		return CreateAgentSessionResult{}, notImplemented("CreateAgentSession")
	}
	config := options[0]
	if len(config.CustomTools) != 0 {
		return CreateAgentSessionResult{}, notImplemented("CreateAgentSession.CustomTools")
	}
	stream := config.StreamFunction
	if stream == nil && config.Provider != nil {
		stream = agent.StreamFunction(config.Provider.StreamSimple)
	}
	if stream == nil {
		return CreateAgentSessionResult{}, notImplemented("CreateAgentSession")
	}
	baseStream := stream
	stream = func(ctx context.Context, model ai.Model, input ai.Context, options ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
		// M1 propagates the session identity but does not activate Provider
		// prompt-cache retention, which belongs to a later option surface.
		cacheRetention := ai.CacheRetentionNone
		options.CacheRetention = &cacheRetention
		return baseStream(ctx, model, input, options)
	}

	manager := config.SessionManager
	if manager == nil {
		manager = NewInMemorySessionManager(config.CWD)
	}
	sessionContext := manager.BuildSessionContext()
	hasMessages := len(sessionContext.Messages) != 0
	thinkingLevel := config.ThinkingLevel
	if hasMessages && thinkingLevel == "" {
		thinkingLevel = agent.ThinkingLevel(sessionContext.ThinkingLevel)
	}

	tools := selectAgentTools(config.AgentTools, config.Tools, config.ExcludeTools, config.NoTools)
	created, err := agent.NewAgent(agent.AgentOptions{
		InitialState: &agent.AgentInitialState{
			Model:         *config.Model,
			ThinkingLevel: thinkingLevel,
			Tools:         tools,
			Messages:      sessionContext.Messages,
		},
		StreamFunction: stream,
		SessionID:      manager.GetSessionID(),
	})
	if err != nil {
		return CreateAgentSessionResult{}, err
	}
	if !hasMessages {
		if _, err := manager.AppendModelChange(string(config.Model.Provider), config.Model.ID); err != nil {
			return CreateAgentSessionResult{}, err
		}
		if _, err := manager.AppendThinkingLevelChange(string(created.State().ThinkingLevel)); err != nil {
			return CreateAgentSessionResult{}, err
		}
	} else {
		hasThinkingEntry := false
		for _, entry := range manager.GetBranch() {
			if entry.Type == "thinking_level_change" {
				hasThinkingEntry = true
				break
			}
		}
		if !hasThinkingEntry {
			if _, err := manager.AppendThinkingLevelChange(string(created.State().ThinkingLevel)); err != nil {
				return CreateAgentSessionResult{}, err
			}
		}
	}

	activeToolNames := make([]string, len(tools))
	for i := range tools {
		activeToolNames[i] = tools[i].Name
	}
	session := NewAgentSession(AgentSessionConfig{
		Agent:                  created,
		CWD:                    config.CWD,
		InitialActiveToolNames: activeToolNames,
		ModelRuntime:           config.ModelRuntime,
		ResourceLoader:         config.ResourceLoader,
		ScopedModels:           config.ScopedModels,
		SessionManager:         manager,
		SessionStartEvent:      config.SessionStartEvent,
		SettingsManager:        config.SettingsManager,
	})
	return CreateAgentSessionResult{Session: session}, nil
}

func selectAgentTools(tools []agent.ErasedAgentTool, included, excluded []string, mode NoToolsMode) []agent.ErasedAgentTool {
	include := make(map[string]struct{}, len(included))
	for _, name := range included {
		include[name] = struct{}{}
	}
	exclude := make(map[string]struct{}, len(excluded))
	for _, name := range excluded {
		exclude[name] = struct{}{}
	}
	selected := make([]agent.ErasedAgentTool, 0, len(tools))
	for _, tool := range tools {
		if included != nil {
			if _, ok := include[tool.Name]; !ok {
				continue
			}
		} else if mode == NoToolsAll {
			continue
		} else if mode == NoToolsBuiltin && isBuiltinAgentTool(tool.Name) {
			continue
		}
		if _, ok := exclude[tool.Name]; ok {
			continue
		}
		selected = append(selected, tool)
	}
	return selected
}

func isBuiltinAgentTool(name string) bool {
	switch name {
	case "read", "bash", "edit", "write":
		return true
	default:
		return false
	}
}

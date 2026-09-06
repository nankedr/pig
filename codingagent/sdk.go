package codingagent

import (
	"context"
	"fmt"

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

// CreateAgentSession uses the shared model runtime unless a stream is injected.
func CreateAgentSession(ctx context.Context, options ...CreateAgentSessionOptions) (CreateAgentSessionResult, error) {
	if len(options) > 1 {
		return CreateAgentSessionResult{}, notImplemented("CreateAgentSession")
	}
	config := CreateAgentSessionOptions{}
	if len(options) == 1 {
		config = options[0]
	}
	if ctx == nil {
		return CreateAgentSessionResult{}, fmt.Errorf("CreateAgentSession context must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return CreateAgentSessionResult{}, err
	}
	if len(config.CustomTools) != 0 {
		return CreateAgentSessionResult{}, notImplemented("CreateAgentSession.CustomTools")
	}
	if config.CWD == "" && config.SessionManager != nil {
		config.CWD = config.SessionManager.GetCWD()
	}
	cwd, err := resolveSessionPath(config.CWD)
	if err != nil {
		return CreateAgentSessionResult{}, err
	}
	config.CWD = cwd
	stream := config.StreamFunction
	if stream == nil && config.Provider != nil {
		stream = agent.StreamFunction(config.Provider.StreamSimple)
	}
	runtimePath := stream == nil
	var fallback *string
	if stream == nil {
		services, err := CreateAgentSessionServices(ctx, CreateAgentSessionServicesOptions{CWD: config.CWD, AgentDir: config.AgentDir, ModelRuntime: config.ModelRuntime, SettingsManager: config.SettingsManager})
		if err != nil {
			return CreateAgentSessionResult{}, err
		}
		config.ModelRuntime = services.ModelRuntime
		config.SettingsManager = services.SettingsManager
		if config.Model == nil {
			model, thinking, err := resolveHeadlessModel(ctx, config.ModelRuntime, config.SettingsManager, CreateHeadlessSessionOptions{Thinking: config.ThinkingLevel, SessionManager: config.SessionManager})
			if err != nil {
				return CreateAgentSessionResult{}, err
			}
			config.Model = &model
			config.ThinkingLevel = thinking
			fallback = restoredModelFallback(config.SessionManager, model, false)
		} else {
			_, thinking, err := resolveHeadlessModel(ctx, config.ModelRuntime, config.SettingsManager, CreateHeadlessSessionOptions{Provider: config.Model.Provider, Model: config.Model.ID, Thinking: config.ThinkingLevel, SessionManager: config.SessionManager})
			if err != nil {
				return CreateAgentSessionResult{}, err
			}
			config.ThinkingLevel = ai.ClampThinkingLevel(*config.Model, thinking)
		}
		if err := checkRuntimeAdapter(*config.Model); err != nil {
			return CreateAgentSessionResult{}, err
		}
		stream = func(ctx context.Context, model ai.Model, input ai.Context, options ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
			result, _ := config.ModelRuntime.StreamSimple(ctx, model, input, ai.ModelsSimpleStreamOptions{SimpleStreamOptions: options})
			return result
		}
	}
	if config.Model == nil {
		return CreateAgentSessionResult{}, fmt.Errorf("an injected stream requires a model")
	}
	if config.SettingsManager == nil {
		config.SettingsManager, err = NewInMemorySettingsManager(Settings{})
		if err != nil {
			return CreateAgentSessionResult{}, err
		}
	}
	if config.AgentTools == nil {
		config.AgentTools, err = sessionServiceTools(config.CWD, config.SettingsManager, config.Tools)
		if err != nil {
			return CreateAgentSessionResult{}, err
		}
	}
	retry, err := config.SettingsManager.GetProviderRetrySettings()
	if err != nil {
		return CreateAgentSessionResult{}, err
	}
	timeout, err := config.SettingsManager.GetHTTPIdleTimeoutMS()
	if err != nil {
		return CreateAgentSessionResult{}, err
	}
	if timeout == 0 {
		timeout = 2147483647
	}
	if retry.TimeoutMS != nil {
		timeout = *retry.TimeoutMS
	}
	baseStream := stream
	stream = func(ctx context.Context, model ai.Model, input ai.Context, options ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
		cacheRetention := ai.CacheRetentionNone
		options.CacheRetention = &cacheRetention
		if options.TimeoutMS == nil {
			options.TimeoutMS = &timeout
		}
		if options.MaxRetries == nil {
			options.MaxRetries = retry.MaxRetries
		}
		if options.MaxRetryDelayMS == nil {
			options.MaxRetryDelayMS = retry.MaxRetryDelayMS
		}
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
	steering, err := config.SettingsManager.GetSteeringMode()
	if err != nil {
		return CreateAgentSessionResult{}, err
	}
	followUp, err := config.SettingsManager.GetFollowUpMode()
	if err != nil {
		return CreateAgentSessionResult{}, err
	}
	transport, err := config.SettingsManager.GetTransport()
	if err != nil {
		return CreateAgentSessionResult{}, err
	}
	budgets, err := config.SettingsManager.GetThinkingBudgets()
	if err != nil {
		return CreateAgentSessionResult{}, err
	}
	var thinkingBudgets *ai.ThinkingBudgets
	if budgets != nil {
		thinkingBudgets = &ai.ThinkingBudgets{}
		for level, budget := range budgets {
			switch level {
			case ai.ModelThinkingLevelMinimal:
				thinkingBudgets.Minimal = &budget
			case ai.ModelThinkingLevelLow:
				thinkingBudgets.Low = &budget
			case ai.ModelThinkingLevelMedium:
				thinkingBudgets.Medium = &budget
			case ai.ModelThinkingLevelHigh:
				thinkingBudgets.High = &budget
			}
		}
	}
	created, err := agent.NewAgent(agent.AgentOptions{
		InitialState: &agent.AgentInitialState{
			Model:         *config.Model,
			ThinkingLevel: thinkingLevel,
			Tools:         tools,
			Messages:      sessionContext.Messages,
		},
		ConvertToLLM: func(_ context.Context, messages []agent.AgentMessage) ([]ai.Message, error) {
			return ConvertToLLM(messages), nil
		},
		StreamFunction:  stream,
		SessionID:       manager.GetSessionID(),
		SteeringMode:    steering,
		FollowUpMode:    followUp,
		Transport:       transport,
		ThinkingBudgets: thinkingBudgets,
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
	if err := configureSessionPrompt(ctx, session, CreateHeadlessSessionOptions{CWD: config.CWD, AgentDir: config.AgentDir, NoContextFiles: !runtimePath}); err != nil {
		session.Dispose()
		return CreateAgentSessionResult{}, err
	}
	return CreateAgentSessionResult{Session: session, ModelFallbackMessage: fallback}, nil
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

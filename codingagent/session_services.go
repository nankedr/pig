package codingagent

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"

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

func CreateAgentSessionServices(ctx context.Context, options CreateAgentSessionServicesOptions) (AgentSessionServices, error) {
	if len(options.ExtensionFlagValues) > 0 || !reflect.ValueOf(options.ResourceLoaderOptions).IsZero() || !reflect.ValueOf(options.ResourceLoaderReloadOptions).IsZero() {
		return AgentSessionServices{}, notImplemented("CreateAgentSessionServices.Resources")
	}
	if ctx == nil {
		return AgentSessionServices{}, fmt.Errorf("services context must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return AgentSessionServices{}, err
	}
	cwd, err := resolveSessionPath(options.CWD)
	if err != nil {
		return AgentSessionServices{}, err
	}
	dir := options.AgentDir
	if dir == "" {
		dir, err = GetAgentDir()
	} else {
		dir, err = resolveSessionPath(dir)
	}
	if err != nil {
		return AgentSessionServices{}, err
	}
	settings := options.SettingsManager
	if settings == nil {
		settings, err = NewSettingsManager(cwd, &dir)
		if err != nil {
			return AgentSessionServices{}, err
		}
		if err = prepareHeadlessProjectSettings(ctx, cwd, dir, settings, nil); err != nil {
			return AgentSessionServices{}, err
		}
	}
	if err = checkHeadlessSettings(settings); err != nil {
		return AgentSessionServices{}, err
	}
	runtime := options.ModelRuntime
	if runtime == nil {
		runtime, err = NewModelRuntime(ctx, CreateModelRuntimeOptions{AuthPath: filepath.Join(dir, "auth.json")})
		if err != nil {
			return AgentSessionServices{}, err
		}
	}
	result := AgentSessionServices{CWD: cwd, AgentDir: dir, ModelRuntime: runtime, SettingsManager: settings}
	diagnostics, err := settings.DrainErrors()
	if err != nil {
		return AgentSessionServices{}, err
	}
	for _, d := range diagnostics {
		result.Diagnostics = append(result.Diagnostics, AgentSessionRuntimeDiagnostic{Type: "warning", Message: d.Error()})
	}
	return result, nil
}
func CreateAgentSessionFromServices(ctx context.Context, options CreateAgentSessionFromServicesOptions) (CreateAgentSessionResult, error) {
	s := options.Services
	if s.ModelRuntime == nil || s.SettingsManager == nil {
		return CreateAgentSessionResult{}, notImplemented("CreateAgentSessionFromServices")
	}
	tools, err := sessionServiceTools(s.CWD, s.SettingsManager, options.Tools)
	if err != nil {
		return CreateAgentSessionResult{}, err
	}
	return CreateAgentSession(ctx, CreateAgentSessionOptions{CWD: s.CWD, AgentDir: s.AgentDir, ModelRuntime: s.ModelRuntime, SettingsManager: s.SettingsManager, ResourceLoader: s.ResourceLoader, SessionManager: options.SessionManager, SessionStartEvent: options.SessionStartEvent, Model: options.Model, ThinkingLevel: options.ThinkingLevel, ScopedModels: options.ScopedModels, Tools: options.Tools, ExcludeTools: options.ExcludeTools, NoTools: options.NoTools, CustomTools: options.CustomTools, AgentTools: tools})
}

func sessionServiceTools(cwd string, settings *SettingsManager, names []string) ([]agent.ErasedAgentTool, error) {
	if names == nil {
		names = []string{"read"}
	}
	tools := []agent.ErasedAgentTool{}
	for _, name := range names {
		var tool agent.ErasedAgentTool
		var err error
		switch name {
		case "read":
			tool, err = CreateReadTool(cwd)
		case "write":
			tool, err = CreateWriteTool(cwd)
		case "bash":
			shell, e := settings.GetShellPath()
			if e != nil {
				return nil, e
			}
			prefix, e := settings.GetShellCommandPrefix()
			if e != nil {
				return nil, e
			}
			tool, err = CreateBashTool(cwd, BashToolOptions{ShellPath: shell, CommandPrefix: prefix})
		default:
			return nil, notImplemented("tool." + name)
		}
		if err != nil {
			return nil, err
		}
		tools = append(tools, tool)
	}
	return tools, nil
}

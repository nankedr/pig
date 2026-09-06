package codingagent

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
)

// HeadlessRunOptions contains the prompts and optional event observer shared
// by the text and JSON Headless Coding Agent modes.
type HeadlessRunOptions struct {
	InitialMessage *string
	Messages       []string
	OnEvent        AgentSessionEventListener
}

// HeadlessOutcome is the final observable state of a Headless run. Text holds
// only Assistant text blocks; cancellation and Provider failures retain the
// same partial/final Assistant outcome stored in the in-memory session.
type HeadlessOutcome struct {
	FinalMessage *ai.AssistantMessage
	Text         []string
	Canceled     bool
}

// CreateHeadlessSessionOptions contains the explicit Headless product inputs.
type CreateHeadlessSessionOptions struct {
	ModelRuntime         *ModelRuntime
	Models               []string
	Offline              bool
	ProjectTrustOverride ProjectTrustDecision
	NoContextFiles       bool
	CWD                  string
	AgentDir             string
	AuthPath             string
	Credentials          ai.CredentialStore
	SettingsManager      *SettingsManager
	SessionDir           *string
	Provider             ai.ProviderID
	Model                string
	APIKey               *string
	Environment          ai.ProviderEnv
	BaseURL              *string
	Thinking             agent.ThinkingLevel
	Tools                []string
	ExcludeTools         []string
	NoTools              NoToolsMode
	SystemPrompt         *string
	SessionManager       *SessionManager
}

// CreateHeadlessSession assembles an AgentSession from explicit inputs and the
// fixed built-in Provider catalog.
func CreateHeadlessSession(ctx context.Context, options CreateHeadlessSessionOptions) (*AgentSessionRuntime, error) {
	if ctx == nil {
		return nil, fmt.Errorf("CreateHeadlessSession context must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	settings := options.SettingsManager
	if settings == nil {
		var dir *string
		if options.AgentDir != "" {
			dir = &options.AgentDir
		}
		var err error
		settings, err = NewSettingsManager(options.CWD, dir)
		if err != nil {
			return nil, err
		}
	}
	if options.SettingsManager == nil || options.ProjectTrustOverride != nil {
		if err := prepareHeadlessProjectSettings(ctx, options.CWD, options.AgentDir, settings, options.ProjectTrustOverride); err != nil {
			return nil, err
		}
	}
	if err := checkHeadlessSettings(settings); err != nil {
		return nil, err
	}
	models := options.ModelRuntime
	var err error
	if models == nil {
		credentials := options.Credentials
		if credentials == nil {
			path := options.AuthPath
			if path == "" && options.AgentDir != "" {
				path = filepath.Join(options.AgentDir, "auth.json")
			}
			var err error
			credentials, err = NewAuthStorage(path)
			if err != nil {
				return nil, err
			}
		}
		environment := options.Environment
		if environment == nil {
			environment = ai.ProviderEnv{}
		}
		models, err = newModelRuntime(ctx, CreateModelRuntimeOptions{Offline: ResolveOffline(options.Offline), Credentials: headlessCredentials{CredentialStore: credentials, apiKey: options.APIKey}}, environment)
		if err != nil {
			return nil, err
		}
	}
	model, thinking, err := resolveHeadlessModel(ctx, models, settings, options)
	if err != nil {
		return nil, err
	}
	scoped := []ScopedModel{}
	diagnostics := []AgentSessionRuntimeDiagnostic{}
	if len(options.Models) > 0 {
		scope, e := ResolveModelScopeWithDiagnostics(ctx, options.Models, models)
		if e != nil {
			return nil, e
		}
		scoped = scope.ScopedModels
		for _, d := range scope.Diagnostics {
			diagnostics = append(diagnostics, AgentSessionRuntimeDiagnostic{Type: d.Type, Message: d.Message})
		}
		if options.Model == "" && len(scoped) > 0 && (options.SessionManager == nil || len(options.SessionManager.BuildSessionContext().Messages) == 0) {
			model = scoped[0].Model
			level := options.Thinking
			if level == "" {
				level = scoped[0].ThinkingLevel
			}
			if level == "" {
				level, _ = settings.GetDefaultThinkingLevel()
			}
			if level == "" {
				level = "medium"
			}
			thinking = ai.ClampThinkingLevel(model, level)
		}
	}
	if options.Model != "" {
		resolved, e := ResolveCLIModel(ResolveCliModelOptions{CLIProvider: string(options.Provider), CLIModel: options.Model, CLIThinking: options.Thinking, ModelRuntime: models})
		if e != nil {
			return nil, e
		}
		if resolved.Warning != nil {
			diagnostics = append(diagnostics, AgentSessionRuntimeDiagnostic{Type: "warning", Message: *resolved.Warning})
		}
	}
	if options.BaseURL != nil {
		baseURL := strings.TrimSpace(*options.BaseURL)
		if baseURL == "" {
			return nil, &CLIArgumentError{Message: "Provider base URL must not be empty"}
		}
		model.BaseURL = baseURL
	}

	if err = checkRuntimeAdapter(model); err != nil {
		return nil, err
	}
	availableTools, err := sessionServiceTools(options.CWD, settings, options.Tools)
	if err != nil {
		return nil, err
	}
	stream := func(runContext context.Context, requestModel ai.Model, input ai.Context, streamOptions ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
		// AgentSession supplies its identity and the explicit no-cache marker to
		// every Provider. Chat Completions does not consume either M1 hint.
		streamOptions.SessionID = nil
		streamOptions.CacheRetention = nil
		if options.APIKey != nil && *options.APIKey != "" {
			key := *options.APIKey
			streamOptions.APIKey = &key
		}
		result, _ := models.StreamSimple(runContext, requestModel, input, ai.ModelsSimpleStreamOptions{SimpleStreamOptions: streamOptions})
		return result
	}
	manager := options.SessionManager
	if manager == nil {
		dir := options.SessionDir
		if dir == nil {
			value, err := settings.GetSessionDir()
			if err != nil {
				return nil, err
			}
			if value != "" {
				dir = &value
			}
		}
		if dir == nil && options.AgentDir != "" {
			agentDir, err := resolveSessionPath(options.AgentDir)
			if err != nil {
				return nil, err
			}
			cwd, err := resolveSessionPath(options.CWD)
			if err != nil {
				return nil, err
			}
			value := defaultSessionDir(cwd, agentDir)
			dir = &value
		}
		manager, err = NewSessionManager(options.CWD, dir)
		if err != nil {
			return nil, err
		}
	}
	created, err := CreateAgentSession(ctx, CreateAgentSessionOptions{
		CWD:             options.CWD,
		ModelRuntime:    models,
		ScopedModels:    scoped,
		Model:           &model,
		StreamFunction:  agent.StreamFunction(stream),
		ThinkingLevel:   thinking,
		Tools:           options.Tools,
		ExcludeTools:    options.ExcludeTools,
		NoTools:         options.NoTools,
		AgentTools:      availableTools,
		SessionManager:  manager,
		SettingsManager: settings,
	})
	if err != nil {
		return nil, err
	}
	if err = configureSessionPrompt(ctx, created.Session, options); err != nil {
		created.Session.Dispose()
		return nil, err
	}
	factory := func(ctx context.Context, next CreateAgentSessionRuntimeOptions) (CreateAgentSessionRuntimeResult, error) {
		config := options
		config.CWD = next.CWD
		config.SessionManager = next.SessionManager
		config.SettingsManager = settings
		if next.CWD != options.CWD {
			config.SettingsManager = nil
		}
		runtime, err := CreateHeadlessSession(ctx, config)
		if err != nil {
			return CreateAgentSessionRuntimeResult{}, err
		}
		runtime.session.sessionStartEvent = next.SessionStartEvent
		return CreateAgentSessionRuntimeResult{CreateAgentSessionResult: CreateAgentSessionResult{Session: runtime.Session()}, Services: runtime.Services()}, nil
	}
	return NewAgentSessionRuntime(created.Session, AgentSessionServices{CWD: options.CWD, AgentDir: options.AgentDir, ModelRuntime: models, SettingsManager: settings, Diagnostics: diagnostics}, factory, nil, nil), nil
}

func configureSessionPrompt(ctx context.Context, session *AgentSession, options CreateHeadlessSessionOptions) error {
	var err error
	activeTools := append([]string{}, session.GetActiveToolNames()...)
	promptOptions := BuildSystemPromptOptions{
		CWD:           options.CWD,
		SelectedTools: activeTools,
		ToolSnippets: map[string]string{
			"read":  "Read file contents",
			"bash":  "Execute bash commands (ls, grep, find, etc.)",
			"write": "Create or overwrite files",
		},
	}
	if containsTool(activeTools, "read", false) {
		promptOptions.PromptGuidelines = []string{"Use read to examine files instead of cat or sed."}
	}
	if containsTool(activeTools, "write", false) {
		promptOptions.PromptGuidelines = append(promptOptions.PromptGuidelines, "Use write only for new files or complete rewrites.")
	}
	if containsTool(activeTools, "bash", false) {
		promptOptions.PromptGuidelines = append(promptOptions.PromptGuidelines, "You can inspect PIG_* environment variables for current model and session details.")
	}

	if options.SystemPrompt != nil {
		promptOptions.CustomPrompt = *options.SystemPrompt
	}
	if !options.NoContextFiles {
		dir := options.AgentDir
		if dir == "" {
			dir, err = GetAgentDir()
			if err != nil {
				return err
			}
		}
		promptOptions.ContextFiles, err = LoadProjectContextFiles(ctx, options.CWD, dir)
		if err != nil {
			session.Dispose()
			return err
		}
	}
	session.Agent().SetSystemPrompt(buildSystemPrompt(promptOptions))
	return nil
}

// HeadlessOutcomeError presents a terminal Provider failure or cancellation
// while retaining the complete Headless outcome for programmatic inspection.
type HeadlessOutcomeError struct {
	Outcome HeadlessOutcome
}

func (e *HeadlessOutcomeError) Error() string {
	if e == nil {
		return "Headless run did not produce an Assistant message"
	}
	if e.Outcome.FinalMessage != nil {
		if message, ok := e.Outcome.FinalMessage.ErrorMessage.Value(); ok && message != "" {
			return message
		}
	}
	if e.Outcome.Canceled {
		return "Request aborted"
	}
	if e.Outcome.FinalMessage == nil {
		return "Headless run did not produce an Assistant message"
	}
	return fmt.Sprintf("Request %s", e.Outcome.FinalMessage.StopReason)
}

func (e *HeadlessOutcomeError) Unwrap() error {
	if e != nil && e.Outcome.Canceled {
		return context.Canceled
	}
	return nil
}

// ExitCode distinguishes an interrupted Headless run from ordinary argument,
// Provider, and Capability Stub failures at the process boundary.
func (e *HeadlessOutcomeError) ExitCode() int {
	if e != nil && e.Outcome.Canceled {
		return 130
	}
	return 1
}

type headlessAuthContext ai.ProviderEnv

func (environment headlessAuthContext) Env(ctx context.Context, name string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return environment[name], nil
}

func (headlessAuthContext) FileExists(context.Context, string) (bool, error) {
	return false, nil
}

// RunHeadless sends each configured prompt through the runtime's current
// in-memory AgentSession and returns its last Assistant outcome. It does not
// dispose the runtime, allowing callers to present or inspect the outcome
// before taking ownership of lifecycle cleanup.
func RunHeadless(ctx context.Context, runtime *AgentSessionRuntime, options HeadlessRunOptions) (HeadlessOutcome, error) {
	if ctx == nil {
		return HeadlessOutcome{}, fmt.Errorf("Headless run context must not be nil")
	}
	if runtime == nil || runtime.Session() == nil {
		return HeadlessOutcome{}, fmt.Errorf("Headless run requires an AgentSession runtime")
	}

	session := runtime.Session()
	var unsubscribe AgentSessionUnsubscribe
	var err error
	if options.OnEvent != nil {
		unsubscribe, err = session.Subscribe(options.OnEvent)
		if err != nil {
			return HeadlessOutcome{}, err
		}
		defer unsubscribe()
	}

	prompts := make([]string, 0, len(options.Messages)+1)
	if options.InitialMessage != nil {
		prompts = append(prompts, *options.InitialMessage)
	}
	prompts = append(prompts, options.Messages...)

	for _, prompt := range prompts {
		if err := session.Prompt(ctx, prompt); err != nil {
			outcome := headlessOutcome(session.Messages())
			if cause := context.Cause(ctx); (cause != nil && errors.Is(err, cause)) || errors.Is(err, context.Canceled) {
				outcome.Canceled = true
				return outcome, nil
			}
			return outcome, err
		}
		if outcome := headlessOutcome(session.Messages()); outcome.FinalMessage != nil &&
			(outcome.FinalMessage.StopReason == ai.StopReasonError || outcome.Canceled) {
			return outcome, nil
		}
	}
	if err := session.WaitForIdle(context.WithoutCancel(ctx)); err != nil {
		return headlessOutcome(session.Messages()), err
	}
	return headlessOutcome(session.Messages()), nil
}

func headlessOutcome(messages []agent.AgentMessage) HeadlessOutcome {
	for index := len(messages) - 1; index >= 0; index-- {
		message, ok := messages[index].(ai.AssistantMessage)
		if !ok {
			continue
		}
		message = ai.CloneAssistantMessage(message)
		outcome := HeadlessOutcome{
			FinalMessage: &message,
			Canceled:     message.StopReason == ai.StopReasonAborted,
		}
		for _, content := range message.Content {
			switch text := content.(type) {
			case ai.TextContent:
				outcome.Text = append(outcome.Text, text.Text)
			case *ai.TextContent:
				if text != nil {
					outcome.Text = append(outcome.Text, text.Text)
				}
			}
		}
		return outcome
	}
	return HeadlessOutcome{}
}

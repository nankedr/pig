package codingagent

import (
	"context"
	"errors"
	"fmt"
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
	CWD            string
	Provider       ai.ProviderID
	Model          string
	APIKey         *string
	Environment    ai.ProviderEnv
	BaseURL        *string
	Thinking       agent.ThinkingLevel
	Tools          []string
	ExcludeTools   []string
	NoTools        NoToolsMode
	SystemPrompt   *string
	SessionManager *SessionManager
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
	if options.Provider == "" {
		return nil, &CLIArgumentError{Message: "Headless mode requires --provider <provider>"}
	}
	if options.Model == "" {
		return nil, &CLIArgumentError{Message: "Headless mode requires --model <model>"}
	}

	models := ai.BuiltinModels(ai.CreateModelsOptions{AuthContext: headlessAuthContext(options.Environment)})
	if _, ok := models.GetProvider(options.Provider); !ok {
		return nil, &CLIArgumentError{Message: fmt.Sprintf("Unknown provider %q", options.Provider)}
	}
	model, ok := models.GetModel(options.Provider, options.Model)
	if !ok {
		return nil, &CLIArgumentError{Message: fmt.Sprintf("Unknown model %q for provider %q", options.Model, options.Provider)}
	}
	if options.BaseURL != nil {
		baseURL := strings.TrimSpace(*options.BaseURL)
		if baseURL == "" {
			return nil, &CLIArgumentError{Message: "Provider base URL must not be empty"}
		}
		model.BaseURL = baseURL
	}

	availableTools := make([]agent.ErasedAgentTool, 0, 1)
	readTool, err := CreateReadTool(options.CWD)
	if err != nil {
		return nil, err
	}
	availableTools = append(availableTools, readTool)
	for _, name := range options.Tools {
		if name != "read" {
			return nil, notImplemented("tool." + name)
		}
	}

	stream := func(runContext context.Context, requestModel ai.Model, input ai.Context, streamOptions ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
		// AgentSession supplies its identity and the explicit no-cache marker to
		// every Provider. Chat Completions does not consume either M1 hint.
		streamOptions.SessionID = nil
		streamOptions.CacheRetention = nil
		if options.APIKey != nil {
			key := *options.APIKey
			streamOptions.APIKey = &key
		}
		return models.StreamSimple(runContext, requestModel, input, ai.ModelsSimpleStreamOptions{SimpleStreamOptions: streamOptions})
	}
	manager := options.SessionManager
	if manager == nil {
		manager, err = NewSessionManager(options.CWD, nil)
		if err != nil {
			return nil, err
		}
	}
	created, err := CreateAgentSession(ctx, CreateAgentSessionOptions{
		CWD:            options.CWD,
		Model:          &model,
		StreamFunction: agent.StreamFunction(stream),
		ThinkingLevel:  options.Thinking,
		Tools:          options.Tools,
		ExcludeTools:   options.ExcludeTools,
		NoTools:        options.NoTools,
		AgentTools:     availableTools,
		SessionManager: manager,
	})
	if err != nil {
		return nil, err
	}
	// Preserve an explicit empty selection: nil means the prompt builder's
	// default tool set, while Headless has already resolved its actual tools.
	activeTools := append([]string{}, created.Session.GetActiveToolNames()...)
	promptOptions := BuildSystemPromptOptions{
		CWD:           options.CWD,
		SelectedTools: activeTools,
		ToolSnippets: map[string]string{
			"read": "Read file contents",
		},
	}
	if containsTool(activeTools, "read", false) {
		promptOptions.PromptGuidelines = []string{"Use read to examine files instead of cat or sed."}
	}
	if options.SystemPrompt != nil {
		promptOptions.CustomPrompt = *options.SystemPrompt
	}
	created.Session.Agent().SetSystemPrompt(buildSystemPrompt(promptOptions))
	return NewAgentSessionRuntime(created.Session, AgentSessionServices{CWD: options.CWD}, nil, nil, nil), nil
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

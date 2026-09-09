package codingagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/tui"
)

// InteractiveModeOptions contains product-level composition options. Terminal
// behavior itself remains owned by package tui.
type InteractiveModeOptions struct {
	AutoTrustOnReloadCWD *string
	InitialImages        []ai.ImageContent
	InitialMessage       *string
	InitialMessages      []string
	MigratedProviders    []string
	ModelFallbackMessage *string
	TUIMode              tui.TUIMode
	Verbose              bool
}

// LatestRelease describes an available Pi package release. PackageName and
// Note preserve the distinction between an omitted value and an empty string.
type LatestRelease struct {
	Version     string
	PackageName *string
	Note        *string
}

// InteractiveMode is the product-level interactive composition boundary. It
// intentionally contains no alternate terminal or rendering implementation.
type InteractiveMode struct {
	runtime *AgentSessionRuntime
	options InteractiveModeOptions
}

// NewInteractiveMode records inert dependencies without starting a terminal.
func NewInteractiveMode(runtime *AgentSessionRuntime, options ...InteractiveModeOptions) *InteractiveMode {
	mode := &InteractiveMode{runtime: runtime}
	if len(options) != 0 {
		mode.options = options[0]
	}
	return mode
}

func (*InteractiveMode) ClearEditor() error {
	return notImplemented("InteractiveMode.ClearEditor")
}

func (*InteractiveMode) GetUserInput(context.Context) (string, error) {
	return "", notImplemented("InteractiveMode.GetUserInput")
}

func (*InteractiveMode) Init(context.Context) error {
	return notImplemented("InteractiveMode.Init")
}

func (*InteractiveMode) RenderInitialMessages() error {
	return notImplemented("InteractiveMode.RenderInitialMessages")
}

func (*InteractiveMode) Run(context.Context) error {
	return notImplemented("InteractiveMode.Run")
}

func (*InteractiveMode) ShowError(string) error {
	return notImplemented("InteractiveMode.ShowError")
}

func (*InteractiveMode) ShowNewVersionNotification(LatestRelease) error {
	return notImplemented("InteractiveMode.ShowNewVersionNotification")
}

func (*InteractiveMode) ShowPackageUpdateNotification([]string) error {
	return notImplemented("InteractiveMode.ShowPackageUpdateNotification")
}

func (*InteractiveMode) ShowWarning(string) error {
	return notImplemented("InteractiveMode.ShowWarning")
}

func (*InteractiveMode) Stop(...bool) error {
	return notImplemented("InteractiveMode.Stop")
}

// JSONAgentSessionEvent is a JSON-mode projection of a production-v3 session
// event. It is deliberately unrelated to protocol.ServerEvent and Harness-v4
// events.
type JSONAgentSessionEvent interface {
	AgentSessionEvent
	jsonAgentSessionEvent()
}

// JSONAgentSessionMessageUpdateEvent is the wire-only message update shape.
// It omits both cumulative snapshots retained by the in-process event.
type JSONAgentSessionMessageUpdateEvent struct {
	Type                  AgentSessionEventType `json:"type"`
	AssistantMessageEvent json.RawMessage       `json:"assistantMessageEvent"`
}

func (JSONAgentSessionMessageUpdateEvent) agentSessionEvent()     {}
func (JSONAgentSessionMessageUpdateEvent) jsonAgentSessionEvent() {}
func (e JSONAgentSessionMessageUpdateEvent) AgentSessionEventType() AgentSessionEventType {
	return e.Type
}

// projectJSONAgentSessionEvent converts an in-process session event to the
// JSON/RPC event union. Non-update variants already have their wire shape and
// are returned unchanged.
func projectJSONAgentSessionEvent(event AgentSessionEvent) (JSONAgentSessionEvent, error) {
	switch event := event.(type) {
	case AgentSessionMessageUpdateEvent:
		return projectJSONAgentSessionMessageUpdateEvent(event)
	case *AgentSessionMessageUpdateEvent:
		if event == nil {
			return nil, fmt.Errorf("project JSON agent session event: nil message_update event")
		}
		return projectJSONAgentSessionMessageUpdateEvent(*event)
	default:
		projected, ok := event.(JSONAgentSessionEvent)
		if !ok || projected == nil {
			return nil, fmt.Errorf("project JSON agent session event: unsupported concrete type %T", event)
		}
		return projected, nil
	}
}

func projectJSONAgentSessionMessageUpdateEvent(event AgentSessionMessageUpdateEvent) (JSONAgentSessionEvent, error) {
	if event.AgentEventType() != agent.AgentEventTypeMessageUpdate {
		return nil, fmt.Errorf("project JSON agent session event: message_update has discriminator %q", event.AgentEventType())
	}
	encoded, err := ai.MarshalAssistantMessageEvent(event.AssistantMessageEvent)
	if err != nil {
		return nil, fmt.Errorf("project JSON agent session event: assistantMessageEvent: %w", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		return nil, fmt.Errorf("project JSON agent session event: assistantMessageEvent: %w", err)
	}
	delete(fields, "partial")
	projected, err := json.Marshal(fields)
	if err != nil {
		return nil, fmt.Errorf("project JSON agent session event: assistantMessageEvent: %w", err)
	}
	return JSONAgentSessionMessageUpdateEvent{
		Type:                  AgentSessionEventTypeMessageUpdate,
		AssistantMessageEvent: projected,
	}, nil
}

type PrintModeOptions struct {
	InitialImages  []ai.ImageContent
	InitialMessage *string
	Messages       []string
	Mode           Mode
}

// RunPrintMode runs the shared Headless lifecycle and presents either final
// Assistant text or a session-first stream of JSON events.
func RunPrintMode(ctx context.Context, runtime *AgentSessionRuntime, options PrintModeOptions) (int, error) {
	if options.Mode != "" && options.Mode != ModeText && options.Mode != ModeJSON {
		return 1, &CLIArgumentError{Message: fmt.Sprintf("Invalid print mode %q", options.Mode)}
	}
	if runtime == nil || runtime.Session() == nil {
		return 1, fmt.Errorf("Print mode requires an AgentSession runtime")
	}
	if len(options.InitialImages) != 0 {
		return 1, notImplemented("mode.print.images")
	}
	defer runtime.Dispose(context.WithoutCancel(ctx))

	if options.Mode == ModeJSON {
		return runJSONPrintMode(ctx, runtime, options)
	}

	outcome, err := RunHeadless(ctx, runtime, HeadlessRunOptions{
		InitialMessage: options.InitialMessage,
		Messages:       options.Messages,
	})
	if err != nil {
		return 1, err
	}
	if outcome.FinalMessage == nil {
		return 1, &HeadlessOutcomeError{Outcome: outcome}
	}
	if outcome.FinalMessage.StopReason == ai.StopReasonError || outcome.Canceled {
		return 1, &HeadlessOutcomeError{Outcome: outcome}
	}
	for _, text := range outcome.Text {
		if _, err := fmt.Fprintln(os.Stdout, text); err != nil {
			return 1, errors.New("Error: Failed to write stdout.")
		}
	}
	return 0, nil
}

type jsonLineWriter struct {
	mu     sync.Mutex
	output io.Writer
	err    error
}

func (w *jsonLineWriter) Write(value any) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.err != nil {
		return
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		w.err = fmt.Errorf("encode JSONL record: %w", err)
		return
	}
	encoded = append(encoded, '\n')
	written, err := w.output.Write(encoded)
	if err == nil && written != len(encoded) {
		err = io.ErrShortWrite
	}
	if err != nil {
		w.err = errors.New("Error: Failed to write stdout.")
	}
}

func (w *jsonLineWriter) Err() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.err
}

func runJSONPrintMode(ctx context.Context, runtime *AgentSessionRuntime, options PrintModeOptions) (int, error) {
	writer := &jsonLineWriter{output: os.Stdout}
	manager := runtime.Session().SessionManager()
	if manager == nil || manager.GetHeader() == nil {
		return 1, errors.New("JSON mode requires a Session header")
	}
	writer.Write(manager.GetHeader())
	if err := writer.Err(); err != nil {
		return 1, err
	}

	outcome, err := RunHeadless(ctx, runtime, HeadlessRunOptions{
		InitialMessage: options.InitialMessage,
		Messages:       options.Messages,
		OnEvent: func(event AgentSessionEvent) {
			projected, projectionErr := projectJSONAgentSessionEvent(event)
			if projectionErr != nil {
				writer.mu.Lock()
				if writer.err == nil {
					writer.err = projectionErr
				}
				writer.mu.Unlock()
				return
			}
			writer.Write(projected)
		},
	})
	if err != nil {
		return 1, err
	}
	if err := writer.Err(); err != nil {
		return 1, err
	}
	if outcome.Canceled {
		return 1, &HeadlessOutcomeError{Outcome: outcome}
	}
	return 0, nil
}

// RPCCommand is the open JSONL command envelope used by the trusted local
// subprocess interface. It is not a Remote Session Protocol command.
type RPCCommand struct {
	ID   *string         `json:"id,omitempty"`
	Type string          `json:"type"`
	Data json.RawMessage `json:"data,omitempty"`
}

type RPCExtensionUIRequest struct {
	ID     string          `json:"id"`
	Method string          `json:"method"`
	Type   string          `json:"type"`
	Data   json.RawMessage `json:"data,omitempty"`
}

type RPCExtensionUIResponse struct {
	ID   string          `json:"id"`
	Type string          `json:"type"`
	Data json.RawMessage `json:"data,omitempty"`
}

type RPCResponse struct {
	Command string          `json:"command"`
	ID      *string         `json:"id,omitempty"`
	Success bool            `json:"success"`
	Type    string          `json:"type"`
	Data    json.RawMessage `json:"data,omitempty"`
	Error   *string         `json:"error,omitempty"`
}

type RPCSessionState struct {
	AutoCompactionEnabled bool                `json:"autoCompactionEnabled"`
	FollowUpMode          agent.QueueMode     `json:"followUpMode"`
	IsCompacting          bool                `json:"isCompacting"`
	IsStreaming           bool                `json:"isStreaming"`
	MessageCount          int                 `json:"messageCount"`
	Model                 *ai.Model           `json:"model,omitempty"`
	PendingMessageCount   int                 `json:"pendingMessageCount"`
	SessionFile           *string             `json:"sessionFile,omitempty"`
	SessionID             string              `json:"sessionId"`
	SessionName           *string             `json:"sessionName,omitempty"`
	SteeringMode          agent.QueueMode     `json:"steeringMode"`
	ThinkingLevel         agent.ThinkingLevel `json:"thinkingLevel"`
}

type ModelInfo struct {
	ContextWindow int64
	ID            string
	Provider      string
	Reasoning     bool
}

// BashResult contains the observable outcome of an RPC bash command. Pointer
// fields preserve the upstream distinction between an omitted value and its
// zero value.
type BashResult struct {
	Output         string
	ExitCode       *int
	Cancelled      bool
	Truncated      bool
	FullOutputPath *string
}

type RPCClientOptions struct {
	Args     []string
	CLIPath  *string
	CWD      *string
	Env      map[string]string
	Model    *string
	Provider *string
}

type RPCEventListener func(JSONAgentSessionEvent)

func (*RPCClient) GetCommands(context.Context) ([]ResolvedCommand, error) {
	return nil, notImplemented("RPCClient.GetCommands")
}

package codingagent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/tui"
)

// InteractiveMode composes the production AgentSession with tui-owned input and rendering.
// Run and Stop dispose the supplied runtime; construction performs no terminal I/O.
type InteractiveMode struct {
	runtime                       *AgentSessionRuntime
	options                       InteractiveModeOptions
	ui                            *tui.TextUI
	mu                            sync.Mutex
	initialized, running, stopped bool
	cancel                        context.CancelFunc
	stopOnce                      sync.Once
	stopErr                       error
	turnMu                        sync.Mutex
	turnCancel                    context.CancelFunc
	setupErr                      error
}

func NewInteractiveMode(runtime *AgentSessionRuntime, options ...InteractiveModeOptions) *InteractiveMode {
	mode := &InteractiveMode{runtime: runtime}
	if len(options) > 0 {
		mode.options = options[0]
	}
	terminal := mode.options.Terminal
	if terminal == nil {
		terminal = tui.NewProcessTerminal(os.Stdin, os.Stdout)
	}
	bindings := mode.options.Keybindings
	if bindings == nil {
		dir := ""
		if runtime != nil {
			dir = runtime.Services().AgentDir
		}
		bindings, mode.setupErr = NewKeybindingsManager(dir)
	}
	mode.options.Keybindings = bindings
	var manager *tui.KeybindingsManager
	if bindings != nil {
		manager = &bindings.KeybindingsManager
	}
	mode.ui = tui.NewTextUI(terminal, tui.TextUIOptions{Keybindings: manager, OnInterrupt: func() {
		mode.turnMu.Lock()
		defer mode.turnMu.Unlock()
		if mode.turnCancel != nil {
			mode.turnCancel()
		}
	}})
	return mode
}
func (m *InteractiveMode) Init(ctx context.Context) (err error) {
	defer func() {
		if err != nil {
			if stopErr := m.Stop(); stopErr != nil {
				err = errors.Join(err, stopErr)
			}
		}
	}()
	m.mu.Lock()
	defer m.mu.Unlock()
	if ctx == nil {
		return errors.New("Interactive context must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if m.stopped {
		return errors.New("Interactive mode is stopped")
	}
	if m.initialized {
		return nil
	}
	if m.setupErr != nil {
		return m.setupErr
	}
	if m.runtime == nil || m.runtime.Session() == nil {
		return errors.New("Interactive mode requires an AgentSession runtime")
	}
	if len(m.options.InitialImages) > 0 {
		return notImplemented("InteractiveMode.images")
	}
	if m.options.TUIMode != "" && m.options.TUIMode != tui.TUIModeRegular {
		return notImplemented("InteractiveMode.fullscreen")
	}
	if err := m.ui.Start(); err != nil {
		return err
	}
	if m.options.Keybindings != nil {
		conflicts, _ := m.options.Keybindings.GetConflicts()
		for _, conflict := range conflicts {
			if err := m.ShowWarning(fmt.Sprintf("Keybinding conflict %s: %s", conflict.Key, strings.Join(conflict.Keybindings, ", "))); err != nil {
				return err
			}
		}
	}
	m.initialized = true
	return nil
}
func (m *InteractiveMode) ClearEditor() error { return m.ui.ClearEditor() }
func (m *InteractiveMode) GetUserInput(ctx context.Context) (string, error) {
	for {
		input, err := m.ui.ReadInput(ctx)
		if err != nil {
			return "", err
		}
		switch input.Action {
		case "app.session.new":
			_, err = m.runtime.NewSession(ctx)
			if err != nil {
				_ = m.ShowError(err.Error())
			} else {
				_ = m.ui.Append("\nNew session\n")
			}
		case "app.suspend":
			if err = m.ui.Suspend(); err != nil {
				return "", err
			}
		default:
			return input.Text, nil
		}
	}
}
func (m *InteractiveMode) ShowError(message string) error {
	return m.ui.Append("\nError: " + message + "\n")
}
func (m *InteractiveMode) ShowWarning(message string) error {
	return m.ui.Append("\nWarning: " + message + "\n")
}
func (*InteractiveMode) ShowNewVersionNotification(LatestRelease) error {
	return notImplemented("InteractiveMode.ShowNewVersionNotification")
}
func (*InteractiveMode) ShowPackageUpdateNotification([]string) error {
	return notImplemented("InteractiveMode.ShowPackageUpdateNotification")
}
func (m *InteractiveMode) RenderInitialMessages() error {
	if m.runtime == nil || m.runtime.Session() == nil {
		return errors.New("Interactive mode requires an AgentSession runtime")
	}
	for _, message := range m.runtime.Session().Messages() {
		var text string
		switch message := message.(type) {
		case ai.UserMessage:
			text, _ = message.Content.Text()
			blocks, _ := message.Content.Blocks()
			for _, content := range blocks {
				if part, ok := content.(ai.TextContent); ok {
					text += part.Text
				}
			}
		case ai.AssistantMessage:
			for _, content := range message.Content {
				if part, ok := content.(ai.TextContent); ok {
					text += part.Text
				}
			}
		}
		if text != "" {
			if message.MessageRole() == "user" {
				if err := m.ui.AddToHistory(text); err != nil {
					return err
				}
			}
			if err := m.ui.Append(fmt.Sprintf("%s: %s\n", message.MessageRole(), text)); err != nil {
				return err
			}
		}
	}
	return nil
}
func (m *InteractiveMode) Run(ctx context.Context) (err error) {
	if ctx == nil {
		return errors.Join(errors.New("Interactive context must not be nil"), m.Stop())
	}
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return errors.New("Interactive mode is already running")
	}
	m.running = true
	runCtx, cancel := context.WithCancel(ctx)
	m.cancel = cancel
	m.mu.Unlock()
	defer func() {
		cancel()
		if stopErr := m.Stop(); stopErr != nil {
			err = errors.Join(err, stopErr)
		}
	}()
	if err = m.Init(runCtx); err != nil {
		return err
	}
	if err = m.RenderInitialMessages(); err != nil {
		return err
	}
	go func() {
		select {
		case <-runCtx.Done():
		case <-m.ui.Done():
			cancel()
		}
	}()
	prompts := append([]string(nil), m.options.InitialMessages...)
	if m.options.InitialMessage != nil {
		prompts = append([]string{*m.options.InitialMessage}, prompts...)
	}
	if m.options.ModelFallbackMessage != nil {
		if err = m.ShowWarning(*m.options.ModelFallbackMessage); err != nil {
			return err
		}
	}
	for {
		var prompt string
		if len(prompts) > 0 {
			prompt = prompts[0]
			prompts = prompts[1:]
		} else {
			prompt, err = m.GetUserInput(runCtx)
		}
		if err != nil {
			break
		}
		if strings.TrimSpace(prompt) == "/quit" {
			break
		}
		if err = m.ui.Append("\nuser: " + prompt + "\nassistant: "); err != nil {
			break
		}
		turnCtx, turnCancel := context.WithCancel(runCtx)
		m.turnMu.Lock()
		m.turnCancel = turnCancel
		m.turnMu.Unlock()
		outcome, promptErr := RunHeadless(turnCtx, m.runtime, HeadlessRunOptions{InitialMessage: &prompt, OnEvent: func(event AgentSessionEvent) {
			if update, ok := event.(AgentSessionMessageUpdateEvent); ok {
				if delta, ok := update.AssistantMessageEvent.(ai.AssistantMessageTextDeltaEvent); ok {
					_ = m.ui.Append(delta.Delta)
				}
			}
		}})
		m.turnMu.Lock()
		m.turnCancel = nil
		m.turnMu.Unlock()
		turnCancel()
		if runCtx.Err() != nil {
			err = runCtx.Err()
			break
		}
		if promptErr != nil {
			err = m.ShowError(promptErr.Error())
		} else if outcome.FinalMessage != nil && (outcome.FinalMessage.StopReason == ai.StopReasonError || outcome.Canceled) {
			err = m.ShowError((&HeadlessOutcomeError{Outcome: outcome}).Error())
		} else {
			err = m.ui.Append("\n")
		}
		if err != nil {
			break
		}
	}
	if failure := m.ui.Err(); failure != nil {
		return failure
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if errors.Is(err, io.EOF) || errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}
func (m *InteractiveMode) Stop(_ ...bool) error {
	m.stopOnce.Do(func() {
		m.mu.Lock()
		m.stopped = true
		cancel := m.cancel
		m.mu.Unlock()
		if cancel != nil {
			cancel()
		}
		m.stopErr = m.ui.Stop()
		if m.runtime != nil {
			m.stopErr = errors.Join(m.stopErr, m.runtime.Dispose(context.Background()))
		}
	})
	return m.stopErr
}

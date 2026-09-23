package codingagent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/tui"
)

// InteractiveMode composes the production AgentSession with tui-owned input and rendering.
// Run and Stop dispose the supplied runtime; construction performs no terminal I/O.
type InteractiveMode struct {
	themes                        *ThemeController
	themeCancel                   context.CancelFunc
	themeDone                     chan struct{}
	colorReports                  chan string
	themeQueries                  chan string
	themeContext                  context.Context
	runtime                       *AgentSessionRuntime
	options                       InteractiveModeOptions
	ui                            *tui.TextUI
	mu                            sync.Mutex
	initialized, running, stopped bool
	cancel                        context.CancelFunc
	stopOnce                      sync.Once
	stopErr                       error
	progressMu                    sync.Mutex
	retry                         *AgentSessionAutoRetryStartEvent
	retryUntil                    time.Time
	setupErr                      error
	transcript                    *Transcript
	renderSession                 atomic.Pointer[AgentSession]
	settleSession                 func() error
	sessionRevision               uint64
}

func NewInteractiveMode(runtime *AgentSessionRuntime, options ...InteractiveModeOptions) *InteractiveMode {
	mode := &InteractiveMode{runtime: runtime, transcript: NewTranscript()}
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
	if runtime != nil {
		mode.renderSession.Store(runtime.Session())
	}
	var manager *tui.KeybindingsManager
	if bindings != nil {
		manager = &bindings.KeybindingsManager
	}
	hide := false
	scrollbar := tui.ScrollViewScrollbarAuto
	preserve := false
	if runtime != nil && runtime.Session() != nil {
		hide, _ = runtime.Session().SettingsManager().GetHideThinkingBlock()
		scrollbar, _ = runtime.Session().SettingsManager().GetFullscreenScrollbar()
		exit, _ := runtime.Session().SettingsManager().GetFullscreenExitOutput()
		preserve = exit != FullscreenExitOutputTranscript
	}
	mode.ui = tui.NewTextUI(terminal, tui.TextUIOptions{Mode: mode.options.TUIMode, Scrollbar: scrollbar, PreserveScreen: preserve, Keybindings: manager, HideThinking: hide, RenderTranscript: func(width int, expanded, hide bool) ([]string, error) {
		if mode.themes != nil {
			mode.transcript.SetTheme(mode.themes.Current())
		}
		mode.transcript.SetExpanded(expanded)
		mode.transcript.SetHideThinkingBlock(hide)
		lines, err := mode.transcript.Render(width)
		if err != nil {
			return nil, err
		}
		if session := mode.renderSession.Load(); session != nil {
			steering, _ := session.GetSteeringMessages()
			followUp, _ := session.GetFollowUpMessages()
			for _, text := range steering {
				wrapped, _ := tui.WrapTextWithANSI(tui.SafeTerminalText("Steering: "+text), width)
				lines = append(lines, wrapped...)
			}
			for _, text := range followUp {
				wrapped, _ := tui.WrapTextWithANSI(tui.SafeTerminalText("Follow-up: "+text), width)
				lines = append(lines, wrapped...)
			}
		}
		mode.progressMu.Lock()
		if retry := mode.retry; retry != nil {
			seconds := max(0, int(time.Until(mode.retryUntil).Seconds()+0.999))
			lines = append(lines, fmt.Sprintf("Retrying (%d/%d) in %ds... (%s to cancel)", retry.Attempt, retry.MaxAttempts, seconds, mode.interruptHint()))
		}
		mode.progressMu.Unlock()
		return lines, nil
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

	fd, _ := findBinary(ctx)
	var fdPath *string
	if fd != "" {
		fdPath = &fd
	}
	provider, err := NewSessionAutocompleteProvider(m.runtime.Session(), fdPath)
	if err != nil {
		return err
	}
	if err = m.ui.SetAutocompleteProvider(provider); err != nil {
		return err
	}
	visible, err := m.runtime.Session().SettingsManager().GetAutocompleteMaxVisible()
	if err != nil {
		return err
	}
	_ = m.ui.SetAutocompleteMaxVisible(visible)
	if err := m.initThemes(ctx); err != nil {
		return err
	}
	if err := m.applyInteractionSettings("init"); err != nil {
		return err
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
	if err := m.detectTheme(ctx); err != nil {
		return err
	}
	quiet, _ := m.runtime.Session().SettingsManager().GetQuietStartup()
	if !quiet {
		if err := m.ui.Append("pig · /settings · /hotkeys\n"); err != nil {
			return err
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
		if handled, actionErr := m.modelAction(ctx, input.Action); handled {
			if actionErr != nil {
				_ = m.ShowError(actionErr.Error())
			}
			continue
		}
		switch input.Action {
		case "app.session.fork":
			if err = m.selectFork(ctx); err != nil {
				_ = m.ShowError(err.Error())
			}
		case "app.session.tree":
			if err = m.selectTree(ctx); err != nil {
				_ = m.ShowError(err.Error())
			}
		case "app.session.new", "app.session.resume":
			if input.Action == "app.session.new" {
				err = m.newSession(ctx)
			} else {
				err = m.selectSession(ctx)
			}
			if err != nil {
				_ = m.ShowError(err.Error())
			}
		case "app.message.dequeue":
			if err = m.restoreQueued(); err != nil {
				return "", err
			}
		case "app.interrupt":
			if retrying, _ := m.runtime.Session().IsRetrying(); retrying {
				_ = m.runtime.Session().AbortRetry()
			} else if m.runtime.Session().IsStreaming() {
				if err = m.restoreQueued(); err != nil {
					return "", err
				}
				if err = m.runtime.Session().Abort(); err != nil {
					return "", err
				}
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
	messages := m.transcriptMessages()
	for _, message := range messages {
		if message.MessageRole() == "user" {
			if err := m.ui.AddToHistory(sessionUserText(message)); err != nil {
				return err
			}
		}
	}
	if err := m.transcript.SetMessages(messages); err != nil {
		return err
	}
	return m.ui.Refresh()
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
	inputs := make(chan tui.TextInput)
	inputDone := make(chan struct{})
	go func() {
		defer close(inputDone)
		for {
			input, readErr := m.ui.ReadInput(runCtx)
			if readErr != nil {
				return
			}
			select {
			case inputs <- input:
			case <-runCtx.Done():
				return
			}
		}
	}()
	var turnDone chan error
	var turnCancel context.CancelFunc
	var turnContext context.Context
	var settling, pendingPrompts []string
	var turnID uint64
	defer func() {
		cancel()
		if turnCancel != nil {
			turnCancel()
		}
		if turnDone != nil {
			<-turnDone
		}
		<-inputDone
	}()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	finishTurn := func(promptErr error) error {
		if turnContext.Err() != nil {
			if messages := append(pendingPrompts, settling...); len(messages) > 0 {
				_ = m.ui.PrependEditor(strings.Join(messages, "\n\n"))
			}
			pendingPrompts = nil
		} else {
			pendingPrompts = append(pendingPrompts, settling...)
		}
		settling = nil
		turnDone = nil
		turnCancel()
		turnCancel = nil
		m.ui.SetTurn(0)
		if err := m.transcript.SetMessages(m.transcriptMessages()); err != nil {
			return err
		}
		if promptErr != nil {
			return m.ShowError(promptErr.Error())
		}
		return m.ui.Refresh()
	}
	m.settleSession = func() error {
		if turnDone == nil {
			return nil
		}
		turnCancel()
		promptErr := <-turnDone
		pendingPrompts = nil
		settling = nil
		if errors.Is(promptErr, context.Canceled) {
			promptErr = nil
		}
		return finishTurn(promptErr)
	}
	defer func() { m.settleSession = nil }()
loop:
	for {
		var input tui.TextInput
		literalPrompt := false
		if turnDone == nil && len(pendingPrompts) > 0 {
			input.Text, pendingPrompts = pendingPrompts[0], pendingPrompts[1:]
			literalPrompt = true
		} else if turnDone == nil && len(prompts) > 0 {
			input.Text, prompts = prompts[0], prompts[1:]
		} else {
			select {
			case <-runCtx.Done():
				err = runCtx.Err()
				break loop
			case promptErr := <-turnDone:
				if err = finishTurn(promptErr); err != nil {
					break loop
				}
				continue
			case <-ticker.C:
				if err = m.ui.Refresh(); err != nil {
					break loop
				}
				continue
			case input = <-inputs:
			}
		}
		// Settle a completed worker before deciding whether idle input starts a new turn.
		if turnDone != nil {
			select {
			case promptErr := <-turnDone:
				if err = finishTurn(promptErr); err != nil {
					break loop
				}
			default:
			}
		}

		if input.Turn != 0 && input.Turn != turnID {
			if input.Text != "" {
				_ = m.ui.PrependEditor(input.Text)
				_ = m.ShowWarning("Turn already settled; message restored to editor")
			}
			continue
		}
		literalPrompt = literalPrompt || turnDone != nil && input.Action == "app.message.followUp"
		if !literalPrompt && strings.TrimSpace(input.Text) == "/quit" {
			break
		}
		session := m.runtime.Session()
		if handled, actionErr := m.modelAction(runCtx, input.Action); handled {
			if actionErr != nil {
				_ = m.ShowError(actionErr.Error())
			}
			continue
		}
		switch input.Action {
		case "app.suspend":
			if err = m.ui.Suspend(); err != nil {
				break loop
			}
			continue
		case "app.message.dequeue":
			if err = m.restoreQueued(); err != nil {
				_ = m.ShowError(err.Error())
			}
			continue
		case "app.interrupt":
			if input.Turn != 0 && input.Turn == turnID && turnDone != nil {
				if retrying, _ := session.IsRetrying(); retrying {
					session.AbortRetry()
				} else {
					turnCancel()
					if err = m.restoreQueued(); err != nil {
						_ = m.ShowError(err.Error())
					}
				}
			}
			continue
		case "app.session.fork":
			input.Text = "/fork"
		case "app.session.tree":
			input.Text = "/tree"
		case "app.session.new":
			input.Text = "/new"
		case "app.session.resume":
			input.Text = "/resume"
		}

		if !literalPrompt {
			revision := m.sessionRevision
			if handled, commandErr := m.handleCommand(runCtx, input.Text); handled {
				if m.runtime.Session() != session || m.sessionRevision != revision {
					turnID++
					prompts = nil
					pendingPrompts = nil
					settling = nil
				}
				if commandErr != nil {
					_ = m.ShowError(commandErr.Error())
				}
				continue
			}
		}
		if turnDone != nil {
			var queueErr error
			if input.Action == "app.message.followUp" {
				queueErr = session.FollowUp(input.Text)
			} else {
				queueErr = session.Steer(input.Text)
			}
			if queueErr != nil && turnContext.Err() == nil && (errors.Is(queueErr, agent.ErrAgentIdle) || errors.Is(queueErr, agent.ErrAgentSettling)) {
				if retrying, _ := session.IsRetrying(); !retrying {
					settling = append(settling, input.Text)
					_ = m.ShowWarning("Waiting for current turn to settle")
					continue
				}
			}
			if queueErr != nil {
				_ = m.ui.PrependEditor(input.Text)
				_ = m.ShowError(queueErr.Error())
			}
			continue
		}
		turnID++
		m.ui.SetTurn(turnID)
		turnCtx, stop := context.WithCancel(runCtx)
		turnContext = turnCtx
		turnCancel = stop
		turnDone = make(chan error, 1)
		started := make(chan struct{}, 1)
		go func(prompt string, done chan<- error) {
			_, promptErr := RunHeadless(turnCtx, m.runtime, HeadlessRunOptions{InitialMessage: &prompt, OnEvent: func(event AgentSessionEvent) {
				if event.AgentSessionEventType() == AgentSessionEventTypeAgentStart {
					select {
					case started <- struct{}{}:
					default:
					}
				}
				if event.AgentSessionEventType() == AgentSessionEventTypeAgentSettled {
					m.ui.SetTurn(0)
				}
				m.progressMu.Lock()
				switch e := event.(type) {
				case AgentSessionAutoRetryStartEvent:
					m.retry = &e
					m.retryUntil = time.Now().Add(time.Duration(e.DelayMS) * time.Millisecond)
				case AgentSessionAutoRetryEndEvent:
					m.retry = nil
				}
				m.progressMu.Unlock()
				if updateErr := m.transcript.Update(event); updateErr != nil {
					_ = m.ShowError(updateErr.Error())
					stop()
					return
				}
				if e, ok := event.(AgentSessionAutoRetryEndEvent); ok && !e.Success {
					message := "Unknown error"
					if e.FinalError != nil {
						message = *e.FinalError
					}
					_ = m.ShowError(fmt.Sprintf("Retry failed after %d attempts: %s", e.Attempt, message))
				}
				if renderErr := m.ui.Refresh(); renderErr != nil {
					stop()
				}
			}})
			done <- promptErr
		}(input.Text, turnDone)
		select {
		case <-started:
		case promptErr := <-turnDone:
			if err = finishTurn(promptErr); err != nil {
				break loop
			}
		case <-runCtx.Done():
			err = runCtx.Err()
			break loop
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
		cancel, initialized := m.cancel, m.initialized
		m.mu.Unlock()
		if cancel != nil {
			cancel()
		}
		if m.themeCancel != nil {
			m.themeCancel()
			<-m.themeDone
		}
		m.ui.SetTerminalColorHandler(nil)
		if initialized {
			_ = m.ui.Terminal().Write("\x1b[?2031l")
		}
		m.stopErr = m.ui.Stop()
		if initialized && m.ui.Mode() == tui.TUIModeFullscreen {
			if output, _ := m.runtime.Session().SettingsManager().GetFullscreenExitOutput(); output == FullscreenExitOutputResumeHint {
				if path := m.runtime.Session().SessionManager().GetSessionFile(); path != nil {
					if info, err := os.Stat(*path); err == nil && info.Mode().IsRegular() {
						m.stopErr = errors.Join(m.stopErr, m.ui.Terminal().Write(fmt.Sprintf("\r\nResume this session with: pig --session %q\r\n", *path)))
					}
				}
			}
		}
		if initialized {
			if terminal, ok := m.ui.Terminal().(interface{ Done() <-chan struct{} }); ok {
				<-terminal.Done()
			}
		}
		if m.runtime != nil {
			m.stopErr = errors.Join(m.stopErr, m.runtime.Dispose(context.Background()))
		}
	})
	return m.stopErr
}

func (m *InteractiveMode) transcriptMessages() []agent.AgentMessage {
	session := m.runtime.Session()
	if manager := session.SessionManager(); manager != nil {
		return manager.BuildSessionContext().Messages
	}
	return session.Messages()
}

func (m *InteractiveMode) restoreQueued() error {
	steering, followUp, err := m.runtime.Session().TakeQueuedMessages()
	if err != nil {
		return err
	}
	messages := append(steering, followUp...)
	if len(messages) == 0 {
		return m.ui.Refresh()
	}
	return m.ui.PrependEditor(strings.Join(messages, "\n\n"))
}

func (m *InteractiveMode) interruptHint() string {
	if m.options.Keybindings != nil {
		keys, _ := m.options.Keybindings.GetKeys("app.interrupt")
		if len(keys) > 0 {
			return string(keys[0])
		}
	}
	return "interrupt"
}

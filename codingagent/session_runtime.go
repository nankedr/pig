package codingagent

import (
	"context"
	"errors"
	"fmt"
	"github.com/nankedr/pig/ai"
	"os"
	"sync/atomic"
)

type CreateAgentSessionRuntimeOptions struct {
	CWD, AgentDir       string
	SessionManager      *SessionManager
	SessionStartEvent   *SessionStartEvent
	ProjectTrustContext *ProjectTrustContext
}
type CreateAgentSessionRuntimeResult struct {
	CreateAgentSessionResult
	Services    AgentSessionServices
	Diagnostics []AgentSessionRuntimeDiagnostic
}
type CreateAgentSessionRuntimeFactory func(context.Context, CreateAgentSessionRuntimeOptions) (CreateAgentSessionRuntimeResult, error)
type SessionReplacementResult struct {
	Cancelled    bool
	SelectedText *string
}
type SwitchSessionOptions struct {
	CWDOverride                *string
	WithSession                ExtensionHandler
	ProjectTrustContextFactory func(string) ProjectTrustContext
}
type NewRuntimeSessionOptions struct {
	ParentSession *string
	Setup         ExtensionHandler
	WithSession   ExtensionHandler
}
type ForkSessionOptions struct {
	Position    *string
	WithSession ExtensionHandler
}

type AgentSessionRuntime struct {
	invalidated             *AgentSession
	replacing               atomic.Bool
	session                 *AgentSession
	services                AgentSessionServices
	createRuntime           CreateAgentSessionRuntimeFactory
	diagnostics             []AgentSessionRuntimeDiagnostic
	modelFallbackMessage    *string
	rebindSession           func(*AgentSession) error
	beforeSessionInvalidate func()
}

func NewAgentSessionRuntime(session *AgentSession, services AgentSessionServices, factory CreateAgentSessionRuntimeFactory, diagnostics []AgentSessionRuntimeDiagnostic, fallback *string) *AgentSessionRuntime {
	return &AgentSessionRuntime{
		session:              session,
		services:             cloneServices(services),
		createRuntime:        factory,
		diagnostics:          cloneRuntimeDiagnostics(diagnostics),
		modelFallbackMessage: cloneStringPointer(fallback),
	}
}
func (r *AgentSessionRuntime) Session() *AgentSession         { return r.session }
func (r *AgentSessionRuntime) Services() AgentSessionServices { return cloneServices(r.services) }
func (r *AgentSessionRuntime) CWD() string                    { return r.services.CWD }
func (r *AgentSessionRuntime) Diagnostics() []AgentSessionRuntimeDiagnostic {
	return cloneRuntimeDiagnostics(r.diagnostics)
}
func (r *AgentSessionRuntime) ModelFallbackMessage() *string {
	if r.modelFallbackMessage == nil {
		return nil
	}
	v := *r.modelFallbackMessage
	return &v
}
func (r *AgentSessionRuntime) SetRebindSession(fn func(*AgentSession) error) { r.rebindSession = fn }
func (r *AgentSessionRuntime) SetBeforeSessionInvalidate(fn func())          { r.beforeSessionInvalidate = fn }
func (r *AgentSessionRuntime) beginReplacement(ctx context.Context) (func(), error) {
	if ctx == nil {
		return nil, fmt.Errorf("session replacement context must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !r.replacing.CompareAndSwap(false, true) {
		return nil, fmt.Errorf("session replacement is already in progress")
	}
	if r.session == nil || r.session.SessionManager() == nil || r.createRuntime == nil {
		r.replacing.Store(false)
		return nil, fmt.Errorf("session replacement requires a Session and runtime factory")
	}
	return func() { r.replacing.Store(false) }, nil
}
func (r *AgentSessionRuntime) SwitchSession(ctx context.Context, path string, options ...SwitchSessionOptions) (SessionReplacementResult, error) {
	done, err := r.beginReplacement(ctx)
	if err != nil {
		return SessionReplacementResult{}, err
	}
	defer done()
	option := SwitchSessionOptions{}
	if len(options) > 0 {
		option = options[0]
	}
	if option.WithSession != nil {
		return SessionReplacementResult{}, notImplemented("AgentSessionRuntime.SwitchSession.WithSession")
	}
	manager, err := OpenSessionManager(path, nil, option.CWDOverride)
	if err != nil {
		return SessionReplacementResult{}, err
	}
	if stat, err := os.Stat(manager.GetCWD()); err != nil || !stat.IsDir() {
		return SessionReplacementResult{}, fmt.Errorf("Session working directory does not exist: %s", manager.GetCWD())
	}
	var trust *ProjectTrustContext
	if option.ProjectTrustContextFactory != nil {
		v := option.ProjectTrustContextFactory(manager.GetCWD())
		trust = &v
	}
	return SessionReplacementResult{}, r.replaceSession(ctx, manager, "resume", trust)
}
func (r *AgentSessionRuntime) NewSession(ctx context.Context, options ...NewRuntimeSessionOptions) (SessionReplacementResult, error) {
	done, err := r.beginReplacement(ctx)
	if err != nil {
		return SessionReplacementResult{}, err
	}
	defer done()
	option := NewRuntimeSessionOptions{}
	if len(options) > 0 {
		option = options[0]
	}
	if option.WithSession != nil || option.Setup != nil {
		return SessionReplacementResult{}, notImplemented("AgentSessionRuntime.NewSession.ExtensionCallbacks")
	}
	manager, err := r.newSessionManager(option.ParentSession)
	if err != nil {
		return SessionReplacementResult{}, err
	}
	return SessionReplacementResult{}, r.replaceSession(ctx, manager, "new", nil)
}
func (r *AgentSessionRuntime) newSessionManager(parent *string) (*SessionManager, error) {
	option := NewSessionOptions{}
	if parent != nil {
		option.ParentSession = *parent
	}
	if r.session.SessionManager().IsPersisted() {
		dir := r.session.SessionManager().GetSessionDir()
		return NewSessionManager(r.CWD(), &dir, option)
	}
	return NewInMemorySessionManager(r.CWD(), option), nil
}
func (r *AgentSessionRuntime) Fork(ctx context.Context, id string, options ...ForkSessionOptions) (SessionReplacementResult, error) {
	done, err := r.beginReplacement(ctx)
	if err != nil {
		return SessionReplacementResult{}, err
	}
	defer done()
	option := ForkSessionOptions{}
	if len(options) > 0 {
		option = options[0]
	}
	if option.WithSession != nil {
		return SessionReplacementResult{}, notImplemented("AgentSessionRuntime.Fork.WithSession")
	}
	position := "before"
	if option.Position != nil {
		position = *option.Position
	}
	if position != "before" && position != "at" {
		return SessionReplacementResult{}, fmt.Errorf("Invalid fork position: %s", position)
	}
	source := r.session.SessionManager()
	entry := source.GetEntry(id)
	if entry == nil {
		return SessionReplacementResult{}, fmt.Errorf("Invalid entry ID for forking")
	}
	leaf := &entry.ID
	result := SessionReplacementResult{}
	if position == "before" {
		if entry.Type != "message" || entry.Message == nil || entry.Message.MessageRole() != ai.MessageRoleUser {
			return result, fmt.Errorf("Invalid entry ID for forking")
		}
		leaf = entry.ParentID
		text := sessionUserText(entry.Message)
		result.SelectedText = &text
	}
	var manager *SessionManager
	if leaf == nil {
		manager, err = r.newSessionManager(source.GetSessionFile())
	} else {
		if source.IsPersisted() {
			file := *source.GetSessionFile()
			if _, err := os.Stat(file); err != nil {
				return result, fmt.Errorf("This session has not been saved yet. Wait for the first assistant response before cloning or forking it.")
			}
			dir := source.GetSessionDir()
			manager, err = OpenSessionManager(file, &dir, nil)
		} else {
			manager = NewInMemorySessionManager(source.GetCWD())
			manager.entries = source.GetEntries()
			manager.leafID = source.GetLeafID()
		}
		if err == nil {
			_, err = manager.CreateBranchedSession(*leaf)
		}
	}
	if err != nil {
		return SessionReplacementResult{}, err
	}
	if err = r.replaceSession(ctx, manager, "fork", nil); err != nil {
		return SessionReplacementResult{}, err
	}
	return result, nil
}
func (r *AgentSessionRuntime) replaceSession(ctx context.Context, manager *SessionManager, reason string, trust *ProjectTrustContext) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	old := r.session
	previous := ""
	if file := old.SessionFile(); file != nil {
		previous = *file
	}
	if err := r.teardownCurrent(ctx); err != nil {
		return err
	}
	result, err := r.createRuntime(ctx, CreateAgentSessionRuntimeOptions{CWD: manager.GetCWD(), AgentDir: r.services.AgentDir, SessionManager: manager, SessionStartEvent: &SessionStartEvent{Type: "session_start", Reason: reason, PreviousSessionFile: previous}, ProjectTrustContext: trust})
	if err != nil {
		if result.Session != nil {
			err = errors.Join(err, result.Session.Dispose(), ai.CleanupSessionResources(result.Session.SessionID()))
		}
		return err
	}
	if result.Session == nil {
		return fmt.Errorf("runtime factory returned a nil Session")
	}
	r.session = result.Session
	r.services = cloneServices(result.Services)
	r.diagnostics = cloneRuntimeDiagnostics(result.Diagnostics)
	r.modelFallbackMessage = cloneStringPointer(result.ModelFallbackMessage)
	if r.rebindSession != nil {
		err = r.rebindSession(result.Session)
	}
	if err == nil {
		err = ctx.Err()
	}
	if err != nil {
		r.invalidated = result.Session
		return errors.Join(err, result.Session.Dispose(), ai.CleanupSessionResources(result.Session.SessionID()))
	}

	return nil
}
func (r *AgentSessionRuntime) ImportFromJSONL(context.Context, string, ...string) (SessionReplacementResult, error) {
	return SessionReplacementResult{}, notImplemented("AgentSessionRuntime.ImportFromJSONL")
}
func (r *AgentSessionRuntime) teardownCurrent(ctx context.Context) error {
	s := r.session
	if s == nil || s == r.invalidated {
		return nil
	}
	if err := s.Abort(); err != nil {
		return err
	}
	if err := s.WaitForIdle(ctx); err != nil {
		return err
	}
	if r.beforeSessionInvalidate != nil {
		r.beforeSessionInvalidate()
	}
	r.invalidated = s
	return errors.Join(s.Dispose(), ai.CleanupSessionResources(s.SessionID()))
}
func (r *AgentSessionRuntime) Dispose(ctx context.Context) error {
	if r == nil {
		return nil
	}
	if !r.replacing.CompareAndSwap(false, true) {
		return fmt.Errorf("session replacement is already in progress")
	}
	defer r.replacing.Store(false)
	return r.teardownCurrent(ctx)
}
func CreateAgentSessionRuntime(ctx context.Context, factory CreateAgentSessionRuntimeFactory, options CreateAgentSessionRuntimeOptions) (*AgentSessionRuntime, error) {
	if ctx == nil {
		return nil, fmt.Errorf("CreateAgentSessionRuntime context must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if factory == nil {
		return nil, fmt.Errorf("CreateAgentSessionRuntime factory must not be nil")
	}
	result, err := factory(ctx, options)
	if err != nil {
		return nil, err
	}
	if result.Session == nil {
		return nil, fmt.Errorf("CreateAgentSessionRuntime factory returned a nil Session")
	}
	return NewAgentSessionRuntime(
		result.Session,
		result.Services,
		factory,
		result.Diagnostics,
		result.ModelFallbackMessage,
	), nil
}
func cloneServices(s AgentSessionServices) AgentSessionServices {
	s.Diagnostics = cloneRuntimeDiagnostics(s.Diagnostics)
	return s
}

func cloneRuntimeDiagnostics(diagnostics []AgentSessionRuntimeDiagnostic) []AgentSessionRuntimeDiagnostic {
	if diagnostics == nil {
		return nil
	}
	return append(make([]AgentSessionRuntimeDiagnostic, 0, len(diagnostics)), diagnostics...)
}

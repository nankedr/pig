package codingagent

import "context"

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
func (r *AgentSessionRuntime) SwitchSession(context.Context, string, ...SwitchSessionOptions) (SessionReplacementResult, error) {
	return SessionReplacementResult{}, notImplemented("AgentSessionRuntime.SwitchSession")
}
func (r *AgentSessionRuntime) NewSession(context.Context, ...NewRuntimeSessionOptions) (SessionReplacementResult, error) {
	return SessionReplacementResult{}, notImplemented("AgentSessionRuntime.NewSession")
}
func (r *AgentSessionRuntime) Fork(context.Context, string, ...ForkSessionOptions) (SessionReplacementResult, error) {
	return SessionReplacementResult{}, notImplemented("AgentSessionRuntime.Fork")
}
func (r *AgentSessionRuntime) ImportFromJSONL(context.Context, string, ...string) (SessionReplacementResult, error) {
	return SessionReplacementResult{}, notImplemented("AgentSessionRuntime.ImportFromJSONL")
}
func (r *AgentSessionRuntime) Dispose(context.Context) error {
	return notImplemented("AgentSessionRuntime.Dispose")
}
func CreateAgentSessionRuntime(context.Context, CreateAgentSessionRuntimeFactory, CreateAgentSessionRuntimeOptions) (*AgentSessionRuntime, error) {
	return nil, notImplemented("CreateAgentSessionRuntime")
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

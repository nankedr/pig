package codingagent

import (
	"context"
	"fmt"
	"sync"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
)

type ModelScopeDiagnostic struct {
	Type    string
	Code    string
	Message string
	Pattern string
}

type ResolveCLIModelResult struct {
	Model         *ai.Model
	ThinkingLevel *agent.ThinkingLevel
	Warning       *string
	Error         *string
}

type ResolveModelScopeResult struct {
	ScopedModels []ScopedModel
	Diagnostics  []ModelScopeDiagnostic
}

type ResolveCliModelOptions struct {
	CLIProvider  string
	CLIModel     string
	CLIThinking  agent.ThinkingLevel
	ModelRuntime *ModelRuntime
}

type CreateModelRuntimeOptions struct {
	Offline               bool
	AllowModelNetwork     bool
	AuthPath              string
	CatalogBaseURL        string
	Credentials           ai.CredentialStore
	ModelRefreshTimeoutMS int64
	ModelsPath            ai.Optional[string]
	ModelsStore           ai.ModelsStore
	ModelsStorePath       string
	RefreshOnCreate       *bool
}

type ModelRuntimeAuthOverrides struct {
	APIKey             string
	Env                ai.ProviderEnv
	MinOAuthValidityMS int64
}

type CredentialSynchronizationOperation string

const (
	CredentialSynchronizationLogin               CredentialSynchronizationOperation = "login"
	CredentialSynchronizationLogout              CredentialSynchronizationOperation = "logout"
	CredentialSynchronizationSetRuntimeAPIKey    CredentialSynchronizationOperation = "setRuntimeApiKey"
	CredentialSynchronizationRemoveRuntimeAPIKey CredentialSynchronizationOperation = "removeRuntimeApiKey"
)

type CredentialSynchronizationError struct {
	Cause      error
	Credential ai.Credential
	Message    string
	Name       string
	Operation  CredentialSynchronizationOperation
	ProviderID string
	Stack      string
}

func (e *CredentialSynchronizationError) Error() string {
	if e == nil {
		return "credential synchronization failed"
	}
	if e.Message != "" {
		return e.Message
	}
	return fmt.Sprintf("credential %s committed for %s, but local synchronization failed", e.Operation, e.ProviderID)
}
func (e *CredentialSynchronizationError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

type AuthStatus struct {
	Configured bool
	Source     string
	Label      string
}

type ProviderConfigInput struct {
	Name       string
	BaseURL    string
	APIKey     string
	API        ai.API
	Headers    ai.ProviderHeaders
	AuthHeader *bool
	Models     []ai.Model
}

type CompatibilityRequestConfig struct {
	Headers    ai.ProviderHeaders
	AuthHeader bool
}

type ModelRuntime struct {
	models            ai.MutableModels
	offline           bool
	credentials       ai.CredentialStore
	mu                sync.RWMutex
	refreshMu         sync.Mutex
	available         []ai.Model
	auth              map[string]AuthStatus
	availabilityError string
}

func NewModelRuntime(ctx context.Context, options ...CreateModelRuntimeOptions) (*ModelRuntime, error) {
	config := CreateModelRuntimeOptions{}
	if len(options) > 1 {
		return nil, fmt.Errorf("NewModelRuntime expects at most one options value")
	}
	if len(options) == 1 {
		config = options[0]
	}
	return newModelRuntime(ctx, config, nil)
}

func (r *ModelRuntime) CancelDeferred(ctx context.Context, model ai.Model, handle ai.DeferredHandle, options ...ai.ModelsDeferredCancelOptions) error {
	if r == nil || r.models == nil {
		return notImplemented("ModelRuntime.CancelDeferred")
	}
	return r.models.CancelDeferred(ctx, model, handle, options...)
}
func (r *ModelRuntime) CheckAuth(ctx context.Context, provider string, options ...ai.AuthOperationOptions) (ai.Optional[ai.AuthCheck], error) {
	if r == nil || r.models == nil {
		return ai.Absent[ai.AuthCheck](), notImplemented("ModelRuntime.CheckAuth")
	}
	return r.models.CheckAuth(ctx, ai.ProviderID(provider), options...)
}
func (r *ModelRuntime) Complete(ctx context.Context, model ai.Model, input ai.Context, options ...ai.ModelsStreamOption) (ai.AssistantMessage, error) {
	if r == nil || r.models == nil {
		return ai.AssistantMessage{}, notImplemented("ModelRuntime.Complete")
	}
	stream, err := r.Stream(ctx, model, input, options...)
	if err != nil {
		return ai.AssistantMessage{}, err
	}
	return stream.Result(context.WithoutCancel(ctx))
}
func (r *ModelRuntime) CompleteSimple(ctx context.Context, model ai.Model, input ai.Context, options ...ai.ModelsSimpleStreamOptions) (ai.AssistantMessage, error) {
	if r == nil || r.models == nil {
		return ai.AssistantMessage{}, notImplemented("ModelRuntime.CompleteSimple")
	}
	stream, err := r.StreamSimple(ctx, model, input, options...)
	if err != nil {
		return ai.AssistantMessage{}, err
	}
	return stream.Result(context.WithoutCancel(ctx))
}
func (r *ModelRuntime) FetchDeferred(ctx context.Context, model ai.Model, handle ai.DeferredHandle, options ...ai.ModelsDeferredFetchOptions) (ai.AssistantMessage, error) {
	if r == nil || r.models == nil {
		return ai.AssistantMessage{}, notImplemented("ModelRuntime.FetchDeferred")
	}
	return r.models.FetchDeferred(ctx, model, handle, options...)
}
func (r *ModelRuntime) GetAuth(ctx context.Context, provider string, overrides ...ModelRuntimeAuthOverrides) (ai.Optional[ai.AuthResult], error) {
	if r == nil || r.models == nil {
		return ai.Absent[ai.AuthResult](), notImplemented("ModelRuntime.GetAuth")
	}
	return r.models.GetProviderAuth(ctx, ai.ProviderID(provider), runtimeAuthOverrides(overrides))
}
func (r *ModelRuntime) GetModelAuth(ctx context.Context, model ai.Model, overrides ...ModelRuntimeAuthOverrides) (ai.Optional[ai.AuthResult], error) {
	if r == nil || r.models == nil {
		return ai.Absent[ai.AuthResult](), notImplemented("ModelRuntime.GetModelAuth")
	}
	return r.models.GetModelAuth(ctx, model, runtimeAuthOverrides(overrides))
}
func (r *ModelRuntime) GetAvailable(ctx context.Context, providers ...string) ([]ai.Model, error) {
	if r == nil || r.models == nil {
		return nil, notImplemented("ModelRuntime.GetAvailable")
	}
	return r.refreshAvailability(ctx, providers)
}
func (r *ModelRuntime) GetAvailableSnapshot() ([]ai.Model, error) {
	if r == nil || r.models == nil {
		return nil, notImplemented("ModelRuntime.GetAvailableSnapshot")
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return cloneRuntimeModels(r.available), nil
}
func (r *ModelRuntime) GetCompatibilityRequestConfig(model ai.Model) (CompatibilityRequestConfig, error) {
	if r == nil || r.models == nil {
		return CompatibilityRequestConfig{}, notImplemented("ModelRuntime.GetCompatibilityRequestConfig")
	}
	headers := ai.ProviderHeaders{}
	for k, v := range model.Headers {
		value := v
		headers[k] = &value
	}
	return CompatibilityRequestConfig{Headers: headers, AuthHeader: true}, nil
}
func (r *ModelRuntime) GetError() (string, error) {
	if r == nil || r.models == nil {
		return "", notImplemented("ModelRuntime.GetError")
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.availabilityError, nil
}
func (r *ModelRuntime) GetModel(provider, id string) (ai.Model, bool, error) {
	if r == nil || r.models == nil {
		return ai.Model{}, false, notImplemented("ModelRuntime.GetModel")
	}
	model, ok := r.models.GetModel(ai.ProviderID(provider), id)
	return model, ok, nil
}
func (r *ModelRuntime) GetModels(providers ...string) ([]ai.Model, error) {
	if r == nil || r.models == nil {
		return nil, notImplemented("ModelRuntime.GetModels")
	}
	if len(providers) > 0 {
		return r.models.GetModels(ai.ProviderID(providers[0])), nil
	}
	return r.models.GetModels(), nil
}
func (r *ModelRuntime) GetProvider(provider string) (ai.Provider, bool, error) {
	if r == nil || r.models == nil {
		return nil, false, notImplemented("ModelRuntime.GetProvider")
	}
	p, ok := r.models.GetProvider(ai.ProviderID(provider))
	return p, ok, nil
}
func (r *ModelRuntime) GetProviderAuthStatus(provider string) (AuthStatus, error) {
	if r == nil || r.models == nil {
		return AuthStatus{}, notImplemented("ModelRuntime.GetProviderAuthStatus")
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.auth[provider], nil
}
func (r *ModelRuntime) GetProviders() ([]ai.Provider, error) {
	if r == nil || r.models == nil {
		return nil, notImplemented("ModelRuntime.GetProviders")
	}
	return r.models.GetProviders(), nil
}
func (*ModelRuntime) GetRegisteredNativeProvider(string) (ai.Provider, bool, error) {
	return nil, false, notImplemented("ModelRuntime.GetRegisteredNativeProvider")
}
func (*ModelRuntime) GetRegisteredProviderConfig(string) (ProviderConfigInput, bool, error) {
	return ProviderConfigInput{}, false, notImplemented("ModelRuntime.GetRegisteredProviderConfig")
}
func (*ModelRuntime) GetRegisteredProviderIDs() ([]string, error) {
	return nil, notImplemented("ModelRuntime.GetRegisteredProviderIDs")
}
func (r *ModelRuntime) HasConfiguredAuth(provider string) (bool, error) {
	if r == nil || r.models == nil {
		return false, notImplemented("ModelRuntime.HasConfiguredAuth")
	}
	status, err := r.GetProviderAuthStatus(provider)
	return status.Configured, err
}
func (*ModelRuntime) IsUsingOAuth(string) (bool, error) {
	return false, notImplemented("ModelRuntime.IsUsingOAuth")
}
func (*ModelRuntime) IsUsingSubscription(string) (bool, error) {
	return false, notImplemented("ModelRuntime.IsUsingSubscription")
}
func (r *ModelRuntime) ListCredentials(ctx context.Context, options ...ai.AuthOperationOptions) ([]ai.CredentialInfo, error) {
	if r == nil || r.models == nil {
		return nil, notImplemented("ModelRuntime.ListCredentials")
	}
	option := ai.AuthOperationOptions{}
	if len(options) > 0 {
		option = options[0]
	}
	return r.credentials.List(ctx, option)
}
func (*ModelRuntime) Login(context.Context, string, ai.AuthType, ai.AuthInteraction) (ai.Credential, error) {
	return nil, notImplemented("ModelRuntime.Login")
}
func (*ModelRuntime) Logout(context.Context, string, ...ai.AuthOperationOptions) error {
	return notImplemented("ModelRuntime.Logout")
}
func (r *ModelRuntime) Refresh(ctx context.Context, options ...ai.ModelsRefreshOptions) error {
	if r == nil || r.models == nil {
		return notImplemented("ModelRuntime.Refresh")
	}
	if len(options) > 0 {
		if allowed, ok := options[0].AllowNetwork.Value(); ok && allowed && !r.offline && !ResolveOffline(false) {
			return notImplemented("ModelRuntime.Refresh.Network")
		}
	}
	if len(options) > 0 && options[0].Providers != nil {
		for _, provider := range options[0].Providers {
			if _, err := r.GetAvailable(ctx, string(provider)); err != nil {
				return err
			}
		}
		return nil
	}
	_, err := r.GetAvailable(ctx)
	return err
}
func (*ModelRuntime) RegisterNativeProvider(ai.Provider) error {
	return notImplemented("ModelRuntime.RegisterNativeProvider")
}
func (*ModelRuntime) RegisterProvider(string, ProviderConfigInput) error {
	return notImplemented("ModelRuntime.RegisterProvider")
}
func (*ModelRuntime) RemoveRuntimeAPIKey(context.Context, string, ...ai.AuthOperationOptions) error {
	return notImplemented("ModelRuntime.RemoveRuntimeAPIKey")
}
func (*ModelRuntime) SetRuntimeAPIKey(context.Context, string, string, ...ai.AuthOperationOptions) error {
	return notImplemented("ModelRuntime.SetRuntimeAPIKey")
}
func (r *ModelRuntime) Stream(ctx context.Context, model ai.Model, input ai.Context, options ...ai.ModelsStreamOption) (*ai.AssistantMessageEventStream, error) {
	if r == nil || r.models == nil {
		return nil, notImplemented("ModelRuntime.Stream")
	}
	if err := checkRuntimeAdapter(model); err != nil {
		return nil, err
	}
	return r.models.Stream(ctx, model, input, options...), nil
}
func (r *ModelRuntime) StreamSimple(ctx context.Context, model ai.Model, input ai.Context, options ...ai.ModelsSimpleStreamOptions) (*ai.AssistantMessageEventStream, error) {
	if r == nil || r.models == nil {
		return nil, notImplemented("ModelRuntime.StreamSimple")
	}
	if err := checkRuntimeAdapter(model); err != nil {
		return nil, err
	}
	return r.models.StreamSimple(ctx, model, input, options...), nil
}
func (*ModelRuntime) UnregisterProvider(string) error {
	return notImplemented("ModelRuntime.UnregisterProvider")
}

type ResolvedRequestAuth struct {
	OK      bool
	APIKey  string
	Headers ai.ProviderHeaders
	BaseURL string
	Env     ai.ProviderEnv
	Error   string
}

type ModelRegistry struct{ runtime *ModelRuntime }

func NewModelRegistry(runtime *ModelRuntime) *ModelRegistry { return &ModelRegistry{runtime: runtime} }
func (r *ModelRegistry) Complete(ctx context.Context, model ai.Model, input ai.Context, options ...ai.ModelsStreamOption) (ai.AssistantMessage, error) {
	if r == nil || r.runtime == nil || r.runtime.models == nil {
		return ai.AssistantMessage{}, notImplemented("ModelRegistry.Complete")
	}
	return r.runtime.Complete(ctx, model, input, options...)
}
func (r *ModelRegistry) Find(provider, id string) (ai.Model, bool, error) {
	if r == nil || r.runtime == nil || r.runtime.models == nil {
		return ai.Model{}, false, notImplemented("ModelRegistry.Find")
	}
	return r.runtime.GetModel(provider, id)
}
func (r *ModelRegistry) GetAll() ([]ai.Model, error) {
	if r == nil || r.runtime == nil || r.runtime.models == nil {
		return nil, notImplemented("ModelRegistry.GetAll")
	}
	return r.runtime.GetModels()
}
func (r *ModelRegistry) GetAPIKeyAndHeaders(ctx context.Context, model ai.Model) (ResolvedRequestAuth, error) {
	if r == nil || r.runtime == nil || r.runtime.models == nil {
		return ResolvedRequestAuth{}, notImplemented("ModelRegistry.GetAPIKeyAndHeaders")
	}
	auth, err := r.runtime.GetModelAuth(ctx, model)
	if err != nil {
		return ResolvedRequestAuth{Error: err.Error()}, nil
	}
	value, ok := auth.Value()
	if !ok {
		return ResolvedRequestAuth{Error: fmt.Sprintf("No API key found for %q", model.Provider)}, nil
	}
	key, _ := value.Auth.APIKey.Value()
	base, _ := value.Auth.BaseURL.Value()
	return ResolvedRequestAuth{OK: true, APIKey: key, Headers: value.Auth.Headers, BaseURL: base, Env: value.Env}, nil
}
func (r *ModelRegistry) GetAPIKeyForProvider(ctx context.Context, provider string) (string, error) {
	if r == nil || r.runtime == nil || r.runtime.models == nil {
		return "", notImplemented("ModelRegistry.GetAPIKeyForProvider")
	}
	auth, err := r.runtime.GetAuth(ctx, provider)
	if err != nil {
		return "", err
	}
	v, _ := auth.Value()
	key, _ := v.Auth.APIKey.Value()
	return key, nil
}
func (r *ModelRegistry) GetAvailable() ([]ai.Model, error) {
	if r == nil || r.runtime == nil || r.runtime.models == nil {
		return nil, notImplemented("ModelRegistry.GetAvailable")
	}
	return r.runtime.GetAvailableSnapshot()
}
func (r *ModelRegistry) GetError() (string, error) {
	if r == nil || r.runtime == nil || r.runtime.models == nil {
		return "", notImplemented("ModelRegistry.GetError")
	}
	return r.runtime.GetError()
}
func (r *ModelRegistry) GetProvider(provider string) (ai.Provider, bool, error) {
	if r == nil || r.runtime == nil || r.runtime.models == nil {
		return nil, false, notImplemented("ModelRegistry.GetProvider")
	}
	return r.runtime.GetProvider(provider)
}
func (r *ModelRegistry) GetProviderAuth(ctx context.Context, provider string) (ai.Optional[ai.AuthResult], error) {
	if r == nil || r.runtime == nil || r.runtime.models == nil {
		return ai.Absent[ai.AuthResult](), notImplemented("ModelRegistry.GetProviderAuth")
	}
	return r.runtime.GetAuth(ctx, provider)
}
func (r *ModelRegistry) GetProviderAuthStatus(provider string) (AuthStatus, error) {
	if r == nil || r.runtime == nil || r.runtime.models == nil {
		return AuthStatus{}, notImplemented("ModelRegistry.GetProviderAuthStatus")
	}
	return r.runtime.GetProviderAuthStatus(provider)
}
func (r *ModelRegistry) GetProviderDisplayName(provider string) (string, error) {
	if r == nil || r.runtime == nil || r.runtime.models == nil {
		return "", notImplemented("ModelRegistry.GetProviderDisplayName")
	}
	p, ok, err := r.runtime.GetProvider(provider)
	if err != nil || !ok {
		return provider, err
	}
	return p.Name(), nil
}
func (*ModelRegistry) GetRegisteredNativeProvider(string) (ai.Provider, bool, error) {
	return nil, false, notImplemented("ModelRegistry.GetRegisteredNativeProvider")
}
func (*ModelRegistry) GetRegisteredProviderConfig(string) (ProviderConfigInput, bool, error) {
	return ProviderConfigInput{}, false, notImplemented("ModelRegistry.GetRegisteredProviderConfig")
}
func (*ModelRegistry) GetRegisteredProviderIDs() ([]string, error) {
	return nil, notImplemented("ModelRegistry.GetRegisteredProviderIDs")
}
func (r *ModelRegistry) HasConfiguredAuth(model ai.Model) (bool, error) {
	if r == nil || r.runtime == nil || r.runtime.models == nil {
		return false, notImplemented("ModelRegistry.HasConfiguredAuth")
	}
	return r.runtime.HasConfiguredAuth(string(model.Provider))
}
func (*ModelRegistry) IsUsingOAuth(ai.Model) (bool, error) {
	return false, notImplemented("ModelRegistry.IsUsingOAuth")
}
func (r *ModelRegistry) Refresh(ctx context.Context, options ...ai.ModelsRefreshOptions) error {
	if r == nil || r.runtime == nil || r.runtime.models == nil {
		return notImplemented("ModelRegistry.Refresh")
	}
	return r.runtime.Refresh(ctx, options...)
}
func (*ModelRegistry) RegisterProvider(string, ProviderConfigInput) error {
	return notImplemented("ModelRegistry.RegisterProvider")
}
func (*ModelRegistry) RegisterNativeProvider(ai.Provider) error {
	return notImplemented("ModelRegistry.RegisterNativeProvider")
}
func (*ModelRegistry) UnregisterProvider(string) error {
	return notImplemented("ModelRegistry.UnregisterProvider")
}

func ReadStoredCredential(ctx context.Context, provider string, path ...string) (ai.Credential, error) {
	store, err := NewAuthStorage(path...)
	if err != nil {
		return nil, err
	}
	return store.readRaw(ctx, ai.ProviderID(provider))
}

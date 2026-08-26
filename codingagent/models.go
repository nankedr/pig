package codingagent

import (
	"context"
	"fmt"

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

func ResolveCLIModel(ResolveCliModelOptions) (ResolveCLIModelResult, error) {
	return ResolveCLIModelResult{}, notImplemented("ResolveCLIModel")
}

func ResolveModelScopeWithDiagnostics(context.Context, []string, *ModelRuntime) (ResolveModelScopeResult, error) {
	return ResolveModelScopeResult{}, notImplemented("ResolveModelScopeWithDiagnostics")
}

type CreateModelRuntimeOptions struct {
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

type ModelRuntime struct{}

func NewModelRuntime(context.Context, ...CreateModelRuntimeOptions) (*ModelRuntime, error) {
	return nil, notImplemented("NewModelRuntime")
}

func (*ModelRuntime) CancelDeferred(context.Context, ai.Model, ai.DeferredHandle, ...ai.ModelsDeferredCancelOptions) error {
	return notImplemented("ModelRuntime.CancelDeferred")
}
func (*ModelRuntime) CheckAuth(context.Context, string, ...ai.AuthOperationOptions) (ai.Optional[ai.AuthCheck], error) {
	return ai.Absent[ai.AuthCheck](), notImplemented("ModelRuntime.CheckAuth")
}
func (*ModelRuntime) Complete(context.Context, ai.Model, ai.Context, ...ai.ModelsStreamOption) (ai.AssistantMessage, error) {
	return ai.AssistantMessage{}, notImplemented("ModelRuntime.Complete")
}
func (*ModelRuntime) CompleteSimple(context.Context, ai.Model, ai.Context, ...ai.ModelsSimpleStreamOptions) (ai.AssistantMessage, error) {
	return ai.AssistantMessage{}, notImplemented("ModelRuntime.CompleteSimple")
}
func (*ModelRuntime) FetchDeferred(context.Context, ai.Model, ai.DeferredHandle, ...ai.ModelsDeferredFetchOptions) (ai.AssistantMessage, error) {
	return ai.AssistantMessage{}, notImplemented("ModelRuntime.FetchDeferred")
}
func (*ModelRuntime) GetAuth(context.Context, string, ...ModelRuntimeAuthOverrides) (ai.Optional[ai.AuthResult], error) {
	return ai.Absent[ai.AuthResult](), notImplemented("ModelRuntime.GetAuth")
}
func (*ModelRuntime) GetModelAuth(context.Context, ai.Model, ...ModelRuntimeAuthOverrides) (ai.Optional[ai.AuthResult], error) {
	return ai.Absent[ai.AuthResult](), notImplemented("ModelRuntime.GetModelAuth")
}
func (*ModelRuntime) GetAvailable(context.Context, ...string) ([]ai.Model, error) {
	return nil, notImplemented("ModelRuntime.GetAvailable")
}
func (*ModelRuntime) GetAvailableSnapshot() ([]ai.Model, error) {
	return nil, notImplemented("ModelRuntime.GetAvailableSnapshot")
}
func (*ModelRuntime) GetCompatibilityRequestConfig(ai.Model) (CompatibilityRequestConfig, error) {
	return CompatibilityRequestConfig{}, notImplemented("ModelRuntime.GetCompatibilityRequestConfig")
}
func (*ModelRuntime) GetError() (string, error) {
	return "", notImplemented("ModelRuntime.GetError")
}
func (*ModelRuntime) GetModel(string, string) (ai.Model, bool, error) {
	return ai.Model{}, false, notImplemented("ModelRuntime.GetModel")
}
func (*ModelRuntime) GetModels(...string) ([]ai.Model, error) {
	return nil, notImplemented("ModelRuntime.GetModels")
}
func (*ModelRuntime) GetProvider(string) (ai.Provider, bool, error) {
	return nil, false, notImplemented("ModelRuntime.GetProvider")
}
func (*ModelRuntime) GetProviderAuthStatus(string) (AuthStatus, error) {
	return AuthStatus{}, notImplemented("ModelRuntime.GetProviderAuthStatus")
}
func (*ModelRuntime) GetProviders() ([]ai.Provider, error) {
	return nil, notImplemented("ModelRuntime.GetProviders")
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
func (*ModelRuntime) HasConfiguredAuth(string) (bool, error) {
	return false, notImplemented("ModelRuntime.HasConfiguredAuth")
}
func (*ModelRuntime) IsUsingOAuth(string) (bool, error) {
	return false, notImplemented("ModelRuntime.IsUsingOAuth")
}
func (*ModelRuntime) IsUsingSubscription(string) (bool, error) {
	return false, notImplemented("ModelRuntime.IsUsingSubscription")
}
func (*ModelRuntime) ListCredentials(context.Context, ...ai.AuthOperationOptions) ([]ai.CredentialInfo, error) {
	return nil, notImplemented("ModelRuntime.ListCredentials")
}
func (*ModelRuntime) Login(context.Context, string, ai.AuthType, ai.AuthInteraction) (ai.Credential, error) {
	return nil, notImplemented("ModelRuntime.Login")
}
func (*ModelRuntime) Logout(context.Context, string, ...ai.AuthOperationOptions) error {
	return notImplemented("ModelRuntime.Logout")
}
func (*ModelRuntime) Refresh(context.Context, ...ai.ModelsRefreshOptions) error {
	return notImplemented("ModelRuntime.Refresh")
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
func (*ModelRuntime) Stream(context.Context, ai.Model, ai.Context, ...ai.ModelsStreamOption) (*ai.AssistantMessageEventStream, error) {
	return nil, notImplemented("ModelRuntime.Stream")
}
func (*ModelRuntime) StreamSimple(context.Context, ai.Model, ai.Context, ...ai.ModelsSimpleStreamOptions) (*ai.AssistantMessageEventStream, error) {
	return nil, notImplemented("ModelRuntime.StreamSimple")
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
func (*ModelRegistry) Complete(context.Context, ai.Model, ai.Context, ...ai.ModelsStreamOption) (ai.AssistantMessage, error) {
	return ai.AssistantMessage{}, notImplemented("ModelRegistry.Complete")
}
func (*ModelRegistry) Find(string, string) (ai.Model, bool, error) {
	return ai.Model{}, false, notImplemented("ModelRegistry.Find")
}
func (*ModelRegistry) GetAll() ([]ai.Model, error) {
	return nil, notImplemented("ModelRegistry.GetAll")
}
func (*ModelRegistry) GetAPIKeyAndHeaders(context.Context, ai.Model) (ResolvedRequestAuth, error) {
	return ResolvedRequestAuth{}, notImplemented("ModelRegistry.GetAPIKeyAndHeaders")
}
func (*ModelRegistry) GetAPIKeyForProvider(context.Context, string) (string, error) {
	return "", notImplemented("ModelRegistry.GetAPIKeyForProvider")
}
func (*ModelRegistry) GetAvailable() ([]ai.Model, error) {
	return nil, notImplemented("ModelRegistry.GetAvailable")
}
func (*ModelRegistry) GetError() (string, error) {
	return "", notImplemented("ModelRegistry.GetError")
}
func (*ModelRegistry) GetProvider(string) (ai.Provider, bool, error) {
	return nil, false, notImplemented("ModelRegistry.GetProvider")
}
func (*ModelRegistry) GetProviderAuth(context.Context, string) (ai.Optional[ai.AuthResult], error) {
	return ai.Absent[ai.AuthResult](), notImplemented("ModelRegistry.GetProviderAuth")
}
func (*ModelRegistry) GetProviderAuthStatus(string) (AuthStatus, error) {
	return AuthStatus{}, notImplemented("ModelRegistry.GetProviderAuthStatus")
}
func (*ModelRegistry) GetProviderDisplayName(string) (string, error) {
	return "", notImplemented("ModelRegistry.GetProviderDisplayName")
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
func (*ModelRegistry) HasConfiguredAuth(ai.Model) (bool, error) {
	return false, notImplemented("ModelRegistry.HasConfiguredAuth")
}
func (*ModelRegistry) IsUsingOAuth(ai.Model) (bool, error) {
	return false, notImplemented("ModelRegistry.IsUsingOAuth")
}
func (*ModelRegistry) Refresh(context.Context, ...ai.ModelsRefreshOptions) error {
	return notImplemented("ModelRegistry.Refresh")
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

func ReadStoredCredential(context.Context, string, ...string) (ai.Credential, error) {
	return nil, notImplemented("ReadStoredCredential")
}

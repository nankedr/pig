package ai

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// ModelAuth is the request auth for a single model request. If a value cannot
// be expressed as APIKey, Headers, or BaseURL, it is provider config, not auth.
// Its three fields are separate from provider configuration by design.
type ModelAuth struct {
	APIKey  Optional[string] `json:"apiKey,omitzero"`
	Headers ProviderHeaders  `json:"headers,omitempty"`
	BaseURL Optional[string] `json:"baseUrl,omitzero"`
}

// AuthContext provides environment and filesystem access for auth resolution.
// It is injectable for tests and browsers. The default context returned by
// DefaultProviderAuthContext is an M0 stub: its methods perform no env or file
// access and report ErrNotImplemented, so live resolution requires an injected
// context (tests supply a fake, later milestones supply the real one).
type AuthContext interface {
	// Env reports a named environment value. An empty string with a nil error
	// means the variable is unset (matching the upstream undefined result).
	Env(ctx context.Context, name string) (string, error)
	// FileExists reports whether path exists. Implementations may expand a
	// leading "~"; browsers always report false.
	FileExists(ctx context.Context, path string) (bool, error)
}

type stubAuthContext struct{}

// Env is a side-effect-free M0 stub: it never reads process environment.
func (stubAuthContext) Env(context.Context, string) (string, error) {
	return "", newNotImplemented("AuthContext.Env")
}

// FileExists is a side-effect-free M0 stub: it never touches the filesystem.
func (stubAuthContext) FileExists(context.Context, string) (bool, error) {
	return false, newNotImplemented("AuthContext.FileExists")
}

// DefaultProviderAuthContext returns the default auth context. In this M0
// scaffold it is a side-effect-free stub whose Env and FileExists report
// ErrNotImplemented instead of reading the environment or filesystem. Inject a
// concrete AuthContext to resolve against real ambient sources.
func DefaultProviderAuthContext() AuthContext {
	return stubAuthContext{}
}

// AuthResult is the result of resolving auth for a model.
type AuthResult struct {
	Auth ModelAuth `json:"auth"`
	// Env carries provider-scoped environment/config values resolved from
	// credentials and ambient context.
	Env ProviderEnv `json:"env,omitempty"`
	// Source is a human-readable label for status UI, e.g. "ANTHROPIC_API_KEY",
	// "OAuth", or "~/.aws/credentials".
	Source Optional[string] `json:"source,omitzero"`
}

// AuthCheck is the outcome of a side-effect-free availability check.
type AuthCheck struct {
	Source Optional[string] `json:"source,omitzero"`
	Type   AuthType         `json:"type"`
}

// AuthPromptType is the published discriminator for a login prompt.
type AuthPromptType string

const (
	AuthPromptTypeText       AuthPromptType = "text"
	AuthPromptTypeSecret     AuthPromptType = "secret"
	AuthPromptTypeSelect     AuthPromptType = "select"
	AuthPromptTypeManualCode AuthPromptType = "manual_code"
)

// AuthPrompt is the closed set of prompts shown to the user during login.
// Upstream carries an optional per-prompt AbortSignal; in Pig per-prompt
// cancellation is the context.Context passed to AuthInteraction.Prompt.
type AuthPrompt interface {
	authPrompt()
	// AuthPromptType reports the concrete variant's discriminator.
	AuthPromptType() AuthPromptType
}

// AuthPromptOption is one selectable option in a select prompt.
type AuthPromptOption struct {
	ID          string           `json:"id"`
	Label       string           `json:"label"`
	Description Optional[string] `json:"description,omitzero"`
}

// TextAuthPrompt requests a single line of free text.
type TextAuthPrompt struct {
	Message     string           `json:"message"`
	Placeholder Optional[string] `json:"placeholder,omitzero"`
}

func (TextAuthPrompt) authPrompt()                    {}
func (TextAuthPrompt) AuthPromptType() AuthPromptType { return AuthPromptTypeText }

// SecretAuthPrompt requests a secret value such as an API key.
type SecretAuthPrompt struct {
	Message     string           `json:"message"`
	Placeholder Optional[string] `json:"placeholder,omitzero"`
}

func (SecretAuthPrompt) authPrompt()                    {}
func (SecretAuthPrompt) AuthPromptType() AuthPromptType { return AuthPromptTypeSecret }

// SelectAuthPrompt asks the user to choose one option; Prompt returns the id.
type SelectAuthPrompt struct {
	Message string             `json:"message"`
	Options []AuthPromptOption `json:"options"`
}

func (SelectAuthPrompt) authPrompt()                    {}
func (SelectAuthPrompt) AuthPromptType() AuthPromptType { return AuthPromptTypeSelect }

// ManualCodeAuthPrompt requests a code the user pastes back from a browser flow.
type ManualCodeAuthPrompt struct {
	Message     string           `json:"message"`
	Placeholder Optional[string] `json:"placeholder,omitzero"`
}

func (ManualCodeAuthPrompt) authPrompt()                    {}
func (ManualCodeAuthPrompt) AuthPromptType() AuthPromptType { return AuthPromptTypeManualCode }

// AuthInfoLink is a labelled link surfaced during login.
type AuthInfoLink struct {
	URL   string           `json:"url"`
	Label Optional[string] `json:"label,omitzero"`
}

// AuthEventType is the published discriminator for a login event.
type AuthEventType string

const (
	AuthEventTypeInfo       AuthEventType = "info"
	AuthEventTypeAuthURL    AuthEventType = "auth_url"
	AuthEventTypeDeviceCode AuthEventType = "device_code"
	AuthEventTypeProgress   AuthEventType = "progress"
)

// AuthEvent is the closed set of out-of-band events emitted during login.
type AuthEvent interface {
	authEvent()
	// AuthEventType reports the concrete variant's discriminator.
	AuthEventType() AuthEventType
}

// InfoAuthEvent surfaces an informational message and optional links.
type InfoAuthEvent struct {
	Message string         `json:"message"`
	Links   []AuthInfoLink `json:"links,omitempty"`
}

func (InfoAuthEvent) authEvent()                   {}
func (InfoAuthEvent) AuthEventType() AuthEventType { return AuthEventTypeInfo }

// AuthURLAuthEvent surfaces a URL the user should open.
type AuthURLAuthEvent struct {
	URL          string           `json:"url"`
	Instructions Optional[string] `json:"instructions,omitzero"`
}

func (AuthURLAuthEvent) authEvent()                   {}
func (AuthURLAuthEvent) AuthEventType() AuthEventType { return AuthEventTypeAuthURL }

// DeviceCodeAuthEvent surfaces a device-code login step.
type DeviceCodeAuthEvent struct {
	UserCode         string        `json:"userCode"`
	VerificationURI  string        `json:"verificationUri"`
	IntervalSeconds  Optional[int] `json:"intervalSeconds,omitzero"`
	ExpiresInSeconds Optional[int] `json:"expiresInSeconds,omitzero"`
}

func (DeviceCodeAuthEvent) authEvent()                   {}
func (DeviceCodeAuthEvent) AuthEventType() AuthEventType { return AuthEventTypeDeviceCode }

// ProgressAuthEvent surfaces incremental progress during a login flow.
type ProgressAuthEvent struct {
	Message string `json:"message"`
}

func (ProgressAuthEvent) authEvent()                   {}
func (ProgressAuthEvent) AuthEventType() AuthEventType { return AuthEventTypeProgress }

// AuthInteraction carries login interaction callbacks serving both api-key and
// OAuth flows. Prompt returns the entered/selected string (a select prompt
// returns the option id) and reports an error on cancel or abort. The whole
// login flow's AbortSignal is the context.Context passed to the provider login
// method; per-prompt cancellation is the ctx passed to Prompt.
type AuthInteraction interface {
	Prompt(ctx context.Context, prompt AuthPrompt) (string, error)
	Notify(event AuthEvent)
}

// ProviderAuthInteraction is the normalized interaction passed to provider
// login implementations. Upstream guarantees a non-optional signal; in Pig that
// guarantee is the non-nil context.Context passed to the login method, so the
// Go shape is identical to AuthInteraction.
type ProviderAuthInteraction = AuthInteraction

// APIKeyResolveInput is the input to APIKeyAuth.Resolve. Credential is nil when
// no credential is stored, in which case resolution consults ambient sources.
type APIKeyResolveInput struct {
	Context    AuthContext
	Credential *APIKeyCredential
}

// APIKeyCheckInput is the input to APIKeyAuth.Check.
type APIKeyCheckInput struct {
	Context    AuthContext
	Credential *APIKeyCredential
}

// APIKeyAuth is api-key auth ownership: a stored key/provider env plus ambient
// sources (env vars, AWS profiles, ADC files). It is modelled as a bundle of
// operations, mirroring ProviderStreams: optional operations are nil function
// fields, so an ambient-only provider leaves Login nil while still owning
// Resolve. At least Resolve is present; even keyless local servers report their
// configured state through it.
type APIKeyAuth struct {
	// Name is the display name, e.g. "Anthropic API key".
	Name string

	// Login runs interactive setup (prompt for key/provider env). A nil Login
	// marks an ambient-only provider.
	Login func(ctx context.Context, interaction ProviderAuthInteraction) (APIKeyCredential, error)

	// Check is an optional side-effect-free availability check. A nil Check
	// means availability is determined by resolving auth.
	Check func(ctx context.Context, input APIKeyCheckInput) (Optional[AuthCheck], error)

	// Resolve resolves auth from the stored credential and/or ambient sources.
	// An absent result means the provider is not configured.
	Resolve func(ctx context.Context, input APIKeyResolveInput) (Optional[AuthResult], error)
}

// OAuthAuth is OAuth auth ownership. The Refresh/ToAuth split lets the resolver
// own the locked refresh pattern: Refresh produces a credential and ToAuth
// derives request auth from whatever credential ends up stored.
type OAuthAuth struct {
	// Name is the display name, e.g. "Anthropic (Claude Pro/Max)".
	Name string

	// IsSubscription reports whether access is backed by a provider subscription.
	IsSubscription bool

	// LoginLabel is the selector label for the OAuth login option.
	LoginLabel Optional[string]

	// Login runs the interactive OAuth login flow.
	Login func(ctx context.Context, interaction ProviderAuthInteraction) (OAuthCredential, error)

	// Refresh exchanges the refresh token. It is a network call that fails on
	// invalid_grant and similar; the resolver runs it under the store lock.
	Refresh func(ctx context.Context, credential OAuthCredential) (OAuthCredential, error)

	// ToAuth derives request auth from a valid credential. It is side-effect
	// free and covers per-credential baseUrl (e.g. GitHub Copilot).
	ToAuth func(ctx context.Context, credential OAuthCredential) (ModelAuth, error)
}

// ProviderAuth is provider auth ownership. At least one of APIKey/OAuth must be
// present: even ambient-credential providers and keyless local servers provide
// APIKey auth whose Resolve reports whether the provider is configured.
type ProviderAuth struct {
	APIKey *APIKeyAuth
	OAuth  *OAuthAuth
}

// NewStubAPIKeyAuth returns an APIKeyAuth whose operations are side-effect-free
// M0 Capability Stubs: Login, Check, and Resolve report ErrNotImplemented
// without touching the environment, filesystem, or network.
func NewStubAPIKeyAuth(name string) APIKeyAuth {
	return APIKeyAuth{
		Name: name,
		Login: func(context.Context, ProviderAuthInteraction) (APIKeyCredential, error) {
			return APIKeyCredential{}, newNotImplemented("APIKeyAuth.Login")
		},
		Check: func(context.Context, APIKeyCheckInput) (Optional[AuthCheck], error) {
			return Absent[AuthCheck](), newNotImplemented("APIKeyAuth.Check")
		},
		Resolve: func(context.Context, APIKeyResolveInput) (Optional[AuthResult], error) {
			return Absent[AuthResult](), newNotImplemented("APIKeyAuth.Resolve")
		},
	}
}

// NewStubOAuthAuth returns an OAuthAuth whose operations are side-effect-free M0
// Capability Stubs: Login, Refresh, and ToAuth report ErrNotImplemented without
// performing any network exchange.
func NewStubOAuthAuth(name string) OAuthAuth {
	return OAuthAuth{
		Name: name,
		Login: func(context.Context, ProviderAuthInteraction) (OAuthCredential, error) {
			return OAuthCredential{}, newNotImplemented("OAuthAuth.Login")
		},
		Refresh: func(context.Context, OAuthCredential) (OAuthCredential, error) {
			return OAuthCredential{}, newNotImplemented("OAuthAuth.Refresh")
		},
		ToAuth: func(context.Context, OAuthCredential) (ModelAuth, error) {
			return ModelAuth{}, newNotImplemented("OAuthAuth.ToAuth")
		},
	}
}

// EnvAPIKeyAuth builds the standard api-key auth: a stored credential key wins,
// otherwise the first set environment variable resolves. It includes a Login
// that prompts for the key. Providers with non-standard resolution write their
// own APIKeyAuth. The resolution is live pure orchestration over the injected
// AuthContext, so it performs no direct environment access itself.
func EnvAPIKeyAuth(name string, envVars ...string) APIKeyAuth {
	return APIKeyAuth{
		Name: name,
		Login: func(ctx context.Context, interaction ProviderAuthInteraction) (APIKeyCredential, error) {
			if err := ctx.Err(); err != nil {
				return APIKeyCredential{}, err
			}
			key, err := interaction.Prompt(ctx, SecretAuthPrompt{Message: "Enter " + name})
			if err != nil {
				return APIKeyCredential{}, err
			}
			if err := ctx.Err(); err != nil {
				return APIKeyCredential{}, err
			}
			return APIKeyCredential{Type: AuthTypeAPIKey, Key: Some(key)}, nil
		},
		Resolve: func(ctx context.Context, input APIKeyResolveInput) (Optional[AuthResult], error) {
			if err := ctx.Err(); err != nil {
				return Absent[AuthResult](), err
			}
			if input.Credential != nil {
				if key, ok := input.Credential.Key.Value(); ok && key != "" {
					return Some(AuthResult{
						Auth:   ModelAuth{APIKey: Some(key)},
						Env:    input.Credential.Env,
						Source: Some("stored credential"),
					}), nil
				}
			}
			if input.Context == nil {
				return Absent[AuthResult](), nil
			}
			for _, envVar := range envVars {
				value, err := input.Context.Env(ctx, envVar)
				if err != nil {
					return Absent[AuthResult](), err
				}
				if err := ctx.Err(); err != nil {
					return Absent[AuthResult](), err
				}
				if value != "" {
					return Some(AuthResult{
						Auth:   ModelAuth{APIKey: Some(value)},
						Source: Some(envVar),
					}), nil
				}
			}
			return Absent[AuthResult](), nil
		},
	}
}

// LazyOAuthInput configures LazyOAuth.
type LazyOAuthInput struct {
	Name           string
	IsSubscription bool
	LoginLabel     Optional[string]
	// Load resolves the concrete OAuthAuth on first use. It is invoked at most
	// once; the result (or error) is cached.
	Load func() (OAuthAuth, error)
}

// LazyOAuth wraps a lazily loaded OAuthAuth so provider definitions can
// advertise OAuth without importing the implementation. The flow loads on the
// first Login/Refresh/ToAuth call.
func LazyOAuth(input LazyOAuthInput) OAuthAuth {
	var once sync.Once
	var loaded OAuthAuth
	var loadErr error
	get := func() (OAuthAuth, error) {
		once.Do(func() {
			if input.Load == nil {
				loadErr = newNotImplemented("OAuthAuth.Load")
				return
			}
			loaded, loadErr = input.Load()
		})
		return loaded, loadErr
	}
	return OAuthAuth{
		Name:           input.Name,
		IsSubscription: input.IsSubscription,
		LoginLabel:     input.LoginLabel,
		Login: func(ctx context.Context, interaction ProviderAuthInteraction) (OAuthCredential, error) {
			auth, err := get()
			if err != nil {
				return OAuthCredential{}, err
			}
			if auth.Login == nil {
				return OAuthCredential{}, newNotImplemented("OAuthAuth.Login")
			}
			return auth.Login(ctx, interaction)
		},
		Refresh: func(ctx context.Context, credential OAuthCredential) (OAuthCredential, error) {
			auth, err := get()
			if err != nil {
				return OAuthCredential{}, err
			}
			if auth.Refresh == nil {
				return OAuthCredential{}, newNotImplemented("OAuthAuth.Refresh")
			}
			return auth.Refresh(ctx, credential)
		},
		ToAuth: func(ctx context.Context, credential OAuthCredential) (ModelAuth, error) {
			auth, err := get()
			if err != nil {
				return ModelAuth{}, err
			}
			if auth.ToAuth == nil {
				return ModelAuth{}, newNotImplemented("OAuthAuth.ToAuth")
			}
			return auth.ToAuth(ctx, credential)
		},
	}
}

// ModelsErrorCode classifies a ModelsError.
type ModelsErrorCode string

const (
	ModelsErrorCodeModelSource     ModelsErrorCode = "model_source"
	ModelsErrorCodeModelValidation ModelsErrorCode = "model_validation"
	ModelsErrorCodeProvider        ModelsErrorCode = "provider"
	ModelsErrorCodeStream          ModelsErrorCode = "stream"
	ModelsErrorCodeAuth            ModelsErrorCode = "auth"
	ModelsErrorCodeOAuth           ModelsErrorCode = "oauth"
)

// ModelsError is the classified error surfaced by model-collection operations
// such as auth resolution. Code identifies the failure category; Cause retains
// the underlying reason so errors.Is and errors.As reach it.
type ModelsError struct {
	Code    ModelsErrorCode
	Message string
	Cause   error
}

func (e *ModelsError) Error() string {
	if e.Cause != nil {
		return string(e.Code) + ": " + e.Message + ": " + e.Cause.Error()
	}
	return string(e.Code) + ": " + e.Message
}

// Unwrap exposes the underlying cause for errors.Is/errors.As.
func (e *ModelsError) Unwrap() error { return e.Cause }

func newModelsError(code ModelsErrorCode, message string, cause error) *ModelsError {
	return &ModelsError{Code: code, Message: message, Cause: cause}
}

// Default OAuth resolution windows, mirroring the upstream contract.
const (
	// DefaultOAuthMinimumValidity is the minimum remaining OAuth-token validity
	// before a stored token is refreshed during resolution.
	DefaultOAuthMinimumValidity = 5 * time.Minute
	// DefaultOAuthRefreshTimeout bounds a single refresh exchange.
	DefaultOAuthRefreshTimeout = 15 * time.Second
)

// ProviderAuthTarget is the provider identity and auth ownership passed to
// ResolveProviderAuth. It mirrors the upstream inline { id, auth } shape.
type ProviderAuthTarget struct {
	ID   ProviderID
	Auth ProviderAuth
}

// AuthResolutionOverrides carries per-request resolution overrides. Upstream's
// AbortSignal maps to the context.Context passed to ResolveProviderAuth.
type AuthResolutionOverrides struct {
	// APIKey forces an explicit request-level api key, bypassing stored and
	// ambient credentials.
	APIKey Optional[string]
	// Env overlays provider-scoped environment/config values.
	Env ProviderEnv
	// MinOAuthValidity requires this much remaining OAuth-token validity; when
	// absent, DefaultOAuthMinimumValidity applies.
	MinOAuthValidity Optional[time.Duration]
}

// ResolveProviderAuth resolves auth for a provider. A stored credential owns
// the provider: ambient/env is consulted only when nothing is stored. There is
// no silent env fallback after a failed refresh or for a credential type
// without a matching handler. OAuth resolution uses double-checked locking so a
// rotated token is refreshed once globally.
//
// This orchestration is live. Its only side effects are those of the injected
// seams (CredentialStore, AuthContext, and the APIKeyAuth/OAuthAuth operations);
// with the M0 stub auth bundles those seams report ErrNotImplemented, which is
// surfaced as a ModelsError with the matching code.
func ResolveProviderAuth(
	ctx context.Context,
	provider ProviderAuthTarget,
	credentials CredentialStore,
	authContext AuthContext,
	overrides AuthResolutionOverrides,
) (Optional[AuthResult], error) {
	if err := ctx.Err(); err != nil {
		return Absent[AuthResult](), err
	}

	requestAuthContext := authContext
	if len(overrides.Env) > 0 {
		requestAuthContext = overlayEnvAuthContext(authContext, overrides.Env)
	}

	if apiKey, ok := overrides.APIKey.Value(); ok && provider.Auth.APIKey != nil {
		credential := APIKeyCredential{Type: AuthTypeAPIKey, Key: Some(apiKey), Env: overrides.Env}
		return resolveAPIKey(ctx, requestAuthContext, provider.Auth.APIKey, provider.ID, &credential)
	}

	stored, err := readStoredCredential(ctx, credentials, provider.ID)
	if err != nil {
		return Absent[AuthResult](), err
	}
	if stored != nil {
		switch credential := stored.(type) {
		case OAuthCredential:
			if provider.Auth.OAuth != nil {
				return resolveStoredOAuth(ctx, credentials, provider.ID, provider.Auth.OAuth, credential, overrides.MinOAuthValidity)
			}
		case APIKeyCredential:
			if provider.Auth.APIKey != nil {
				merged := credential
				if len(overrides.Env) > 0 {
					merged.Env = mergeProviderEnv(credential.Env, overrides.Env)
				}
				return resolveAPIKey(ctx, requestAuthContext, provider.Auth.APIKey, provider.ID, &merged)
			}
		}
		// A stored credential without a matching handler owns the provider: no
		// silent ambient fallback.
		return Absent[AuthResult](), nil
	}

	// Ambient sources (env vars, AWS profiles, ADC files).
	if provider.Auth.APIKey != nil {
		return resolveAPIKey(ctx, requestAuthContext, provider.Auth.APIKey, provider.ID, nil)
	}
	return Absent[AuthResult](), nil
}

func overlayEnvAuthContext(base AuthContext, env ProviderEnv) AuthContext {
	return overlayAuthContext{base: base, env: env}
}

type overlayAuthContext struct {
	base AuthContext
	env  ProviderEnv
}

func (c overlayAuthContext) Env(ctx context.Context, name string) (string, error) {
	if value := c.env[name]; value != "" {
		return value, nil
	}
	if c.base == nil {
		return "", nil
	}
	return c.base.Env(ctx, name)
}

func (c overlayAuthContext) FileExists(ctx context.Context, path string) (bool, error) {
	if c.base == nil {
		return false, nil
	}
	return c.base.FileExists(ctx, path)
}

func mergeProviderEnv(base, overlay ProviderEnv) ProviderEnv {
	if len(base) == 0 && len(overlay) == 0 {
		return nil
	}
	merged := make(ProviderEnv, len(base)+len(overlay))
	for key, value := range base {
		merged[key] = value
	}
	for key, value := range overlay {
		merged[key] = value
	}
	return merged
}

func readStoredCredential(ctx context.Context, credentials CredentialStore, providerID ProviderID) (Credential, error) {
	stored, err := credentials.Read(ctx, providerID, AuthOperationOptions{})
	if err != nil {
		return nil, newModelsError(ModelsErrorCodeAuth, fmt.Sprintf("credential store read failed for %s", providerID), err)
	}
	return stored, nil
}

func resolveAPIKey(
	ctx context.Context,
	authContext AuthContext,
	apiKey *APIKeyAuth,
	providerID ProviderID,
	credential *APIKeyCredential,
) (Optional[AuthResult], error) {
	if apiKey.Resolve == nil {
		return Absent[AuthResult](), newModelsError(ModelsErrorCodeAuth, fmt.Sprintf("api key auth for %s has no resolve", providerID), nil)
	}
	result, err := apiKey.Resolve(ctx, APIKeyResolveInput{Context: authContext, Credential: credential})
	if err != nil {
		return Absent[AuthResult](), newModelsError(ModelsErrorCodeAuth, fmt.Sprintf("API key auth failed for provider %s", providerID), err)
	}
	return result, nil
}

// resolveStoredOAuth resolves an OAuth credential with double-checked locking:
// a token with less than the minimum validity remaining takes the per-provider
// store lock, re-checks expiry under the lock, refreshes once, and persists the
// rotated credential before release.
func resolveStoredOAuth(
	ctx context.Context,
	credentials CredentialStore,
	providerID ProviderID,
	oauth *OAuthAuth,
	stored OAuthCredential,
	minOAuthValidity Optional[time.Duration],
) (Optional[AuthResult], error) {
	minimumValidity := DefaultOAuthMinimumValidity
	if requested, ok := minOAuthValidity.Value(); ok && requested > minimumValidity {
		minimumValidity = requested
	}
	expiresSoon := func(credential OAuthCredential) bool {
		return time.Now().Add(minimumValidity).UnixMilli() >= credential.Expires
	}

	credential := stored
	if expiresSoon(credential) {
		post, err := credentials.Modify(ctx, providerID, func(modifyCtx context.Context, current Credential) (Credential, error) {
			currentOAuth, ok := current.(OAuthCredential)
			if !ok {
				return nil, nil // logged out meanwhile
			}
			if !expiresSoon(currentOAuth) {
				return nil, nil // another request refreshed it
			}
			if oauth.Refresh == nil {
				return nil, newModelsError(ModelsErrorCodeOAuth, fmt.Sprintf("OAuth refresh failed for %s", providerID), newNotImplemented("OAuthAuth.Refresh"))
			}
			refreshCtx, cancel := context.WithTimeout(modifyCtx, DefaultOAuthRefreshTimeout)
			defer cancel()
			refreshed, refreshErr := oauth.Refresh(refreshCtx, currentOAuth)
			if refreshErr != nil {
				return nil, newModelsError(ModelsErrorCodeOAuth, fmt.Sprintf("OAuth refresh failed for %s", providerID), refreshErr)
			}
			return refreshed, nil
		}, AuthOperationOptions{})
		if err != nil {
			var modelsErr *ModelsError
			if errors.As(err, &modelsErr) {
				return Absent[AuthResult](), modelsErr
			}
			return Absent[AuthResult](), newModelsError(ModelsErrorCodeAuth, fmt.Sprintf("credential store modify failed for %s", providerID), err)
		}
		refreshedOAuth, ok := post.(OAuthCredential)
		if !ok {
			return Absent[AuthResult](), nil // logged out meanwhile
		}
		credential = refreshedOAuth
		if _, requested := minOAuthValidity.Value(); requested && expiresSoon(credential) {
			return Absent[AuthResult](), newModelsError(ModelsErrorCodeOAuth, fmt.Sprintf("OAuth refresh returned a token that expires too soon for %s", providerID), nil)
		}
	}

	if oauth.ToAuth == nil {
		return Absent[AuthResult](), newModelsError(ModelsErrorCodeOAuth, fmt.Sprintf("OAuth auth derivation failed for %s", providerID), newNotImplemented("OAuthAuth.ToAuth"))
	}
	auth, err := oauth.ToAuth(ctx, credential)
	if err != nil {
		return Absent[AuthResult](), newModelsError(ModelsErrorCodeOAuth, fmt.Sprintf("OAuth auth derivation failed for %s", providerID), err)
	}
	return Some(AuthResult{Auth: auth, Source: Some("OAuth")}), nil
}

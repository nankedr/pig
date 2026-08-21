package ai_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nankedr/pig/ai"
)

// Compile-time surface parity for the auth ownership contract. Each closed
// union, interface, and value type below must expose exactly the shape the
// upstream auth module promises; a dropped variant, method, or field fails to
// build here rather than silently narrowing the public surface.
var (
	_ ai.AuthContext             = ai.DefaultProviderAuthContext()
	_ ai.AuthInteraction         = (*recordingInteraction)(nil)
	_ ai.ProviderAuthInteraction = (*recordingInteraction)(nil)

	_ ai.AuthPrompt = ai.TextAuthPrompt{}
	_ ai.AuthPrompt = ai.SecretAuthPrompt{}
	_ ai.AuthPrompt = ai.SelectAuthPrompt{}
	_ ai.AuthPrompt = ai.ManualCodeAuthPrompt{}

	_ ai.AuthEvent = ai.InfoAuthEvent{}
	_ ai.AuthEvent = ai.AuthURLAuthEvent{}
	_ ai.AuthEvent = ai.DeviceCodeAuthEvent{}
	_ ai.AuthEvent = ai.ProgressAuthEvent{}
)

// TestAuthOwnershipTypesExposeEveryDocumentedField touches every field and
// method of the auth ownership value types so a removed member breaks the
// build. It is a structural parity guard, not a behavior test.
func TestAuthOwnershipTypesExposeEveryDocumentedField(t *testing.T) {
	t.Parallel()

	modelAuth := ai.ModelAuth{APIKey: ai.Some("k"), Headers: ai.ProviderHeaders{}, BaseURL: ai.Some("https://x")}
	_ = modelAuth.APIKey
	_ = modelAuth.Headers
	_ = modelAuth.BaseURL

	result := ai.AuthResult{Auth: modelAuth, Env: ai.ProviderEnv{}, Source: ai.Some("s")}
	_ = result.Auth
	_ = result.Env
	_ = result.Source

	check := ai.AuthCheck{Source: ai.Some("s"), Type: ai.AuthTypeAPIKey}
	_ = check.Source
	_ = check.Type

	info := ai.CredentialInfo{ProviderID: "p", Type: ai.AuthTypeOAuth}
	_ = info.ProviderID
	_ = info.Type

	apiKey := ai.APIKeyAuth{
		Name: "n",
		Login: func(context.Context, ai.ProviderAuthInteraction) (ai.APIKeyCredential, error) {
			return ai.APIKeyCredential{}, nil
		},
		Check: func(context.Context, ai.APIKeyCheckInput) (ai.Optional[ai.AuthCheck], error) {
			return ai.Absent[ai.AuthCheck](), nil
		},
		Resolve: func(context.Context, ai.APIKeyResolveInput) (ai.Optional[ai.AuthResult], error) {
			return ai.Absent[ai.AuthResult](), nil
		},
	}
	_ = apiKey.Name
	_ = apiKey.Login
	_ = apiKey.Check
	_ = apiKey.Resolve

	oauth := ai.OAuthAuth{
		Name:           "n",
		IsSubscription: true,
		LoginLabel:     ai.Some("l"),
		Login: func(context.Context, ai.ProviderAuthInteraction) (ai.OAuthCredential, error) {
			return ai.OAuthCredential{}, nil
		},
		Refresh: func(context.Context, ai.OAuthCredential) (ai.OAuthCredential, error) {
			return ai.OAuthCredential{}, nil
		},
		ToAuth: func(context.Context, ai.OAuthCredential) (ai.ModelAuth, error) { return ai.ModelAuth{}, nil },
	}
	_ = oauth.Name
	_ = oauth.IsSubscription
	_ = oauth.LoginLabel
	_ = oauth.Login
	_ = oauth.Refresh
	_ = oauth.ToAuth

	providerAuth := ai.ProviderAuth{APIKey: &apiKey, OAuth: &oauth}
	_ = providerAuth.APIKey
	_ = providerAuth.OAuth

	// Resolution inputs and overrides.
	_ = ai.APIKeyResolveInput{Context: ai.DefaultProviderAuthContext(), Credential: &ai.APIKeyCredential{}}
	_ = ai.APIKeyCheckInput{Context: ai.DefaultProviderAuthContext(), Credential: &ai.APIKeyCredential{}}
	_ = ai.ProviderAuthTarget{ID: "p", Auth: providerAuth}
	_ = ai.AuthResolutionOverrides{APIKey: ai.Some("k"), Env: ai.ProviderEnv{}, MinOAuthValidity: ai.Some(time.Minute)}
}

func TestModelAuthCodecSeparatesAPIKeyHeadersBaseURL(t *testing.T) {
	t.Parallel()

	empty := ""
	auth := ai.ModelAuth{
		APIKey:  ai.Some("sk-live"),
		Headers: ai.ProviderHeaders{"x-clear": nil, "x-empty": &empty},
		BaseURL: ai.Some("https://example.invalid/v1"),
	}
	encoded, err := json.Marshal(auth)
	if err != nil {
		t.Fatalf("json.Marshal(ModelAuth) error = %v", err)
	}
	var decoded ai.ModelAuth
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("json.Unmarshal(ModelAuth) error = %v", err)
	}
	if key, ok := decoded.APIKey.Value(); !ok || key != "sk-live" {
		t.Fatalf("decoded APIKey = (%q, %t), want sk-live", key, ok)
	}
	if base, ok := decoded.BaseURL.Value(); !ok || base != "https://example.invalid/v1" {
		t.Fatalf("decoded BaseURL = (%q, %t)", base, ok)
	}
	cleared, ok := decoded.Headers["x-clear"]
	if !ok || cleared != nil {
		t.Fatalf("cleared header = (%v, %t), want (nil, true)", cleared, ok)
	}
	explicitEmpty, ok := decoded.Headers["x-empty"]
	if !ok || explicitEmpty == nil || *explicitEmpty != "" {
		t.Fatalf("empty header = (%v, %t), want pointer to empty string", explicitEmpty, ok)
	}

	// An absent ModelAuth encodes without any of the three keys.
	zero, err := json.Marshal(ai.ModelAuth{})
	if err != nil {
		t.Fatalf("json.Marshal(zero ModelAuth) error = %v", err)
	}
	if string(zero) != "{}" {
		t.Fatalf("zero ModelAuth = %s, want {}", zero)
	}
}

func TestDefaultProviderAuthContextIsSideEffectFreeStub(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	authContext := ai.DefaultProviderAuthContext()

	if _, err := authContext.Env(ctx, "HOME"); err == nil {
		t.Fatal("stub Env() error = nil, want ErrNotImplemented")
	} else {
		assertNotImplementedOperation(t, err, "AuthContext.Env")
	}
	if _, err := authContext.FileExists(ctx, "~/.aws/credentials"); err == nil {
		t.Fatal("stub FileExists() error = nil, want ErrNotImplemented")
	} else {
		assertNotImplementedOperation(t, err, "AuthContext.FileExists")
	}
}

func TestStubAPIKeyAuthReportsNotImplementedWithoutSideEffects(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	auth := ai.NewStubAPIKeyAuth("Anthropic API key")
	if auth.Name != "Anthropic API key" || auth.Login == nil || auth.Check == nil || auth.Resolve == nil {
		t.Fatalf("stub APIKeyAuth = %#v, want a named bundle with callable stubs", auth)
	}

	interaction := &recordingInteraction{}
	if _, err := auth.Login(ctx, interaction); err == nil {
		t.Fatal("stub Login() error = nil")
	} else {
		assertNotImplementedOperation(t, err, "APIKeyAuth.Login")
	}
	if _, err := auth.Check(ctx, ai.APIKeyCheckInput{}); err == nil {
		t.Fatal("stub Check() error = nil")
	} else {
		assertNotImplementedOperation(t, err, "APIKeyAuth.Check")
	}
	if _, err := auth.Resolve(ctx, ai.APIKeyResolveInput{}); err == nil {
		t.Fatal("stub Resolve() error = nil")
	} else {
		assertNotImplementedOperation(t, err, "APIKeyAuth.Resolve")
	}
	if interaction.prompts != 0 || interaction.notices != 0 {
		t.Fatalf("stub APIKeyAuth touched the interaction: prompts=%d notices=%d", interaction.prompts, interaction.notices)
	}
}

func TestStubOAuthAuthReportsNotImplementedWithoutSideEffects(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	auth := ai.NewStubOAuthAuth("Anthropic (Claude Pro/Max)")
	if auth.Name != "Anthropic (Claude Pro/Max)" || auth.Login == nil || auth.Refresh == nil || auth.ToAuth == nil {
		t.Fatalf("stub OAuthAuth = %#v, want a named bundle with callable stubs", auth)
	}

	if _, err := auth.Login(ctx, &recordingInteraction{}); err == nil {
		t.Fatal("stub Login() error = nil")
	} else {
		assertNotImplementedOperation(t, err, "OAuthAuth.Login")
	}
	if _, err := auth.Refresh(ctx, ai.OAuthCredential{}); err == nil {
		t.Fatal("stub Refresh() error = nil")
	} else {
		assertNotImplementedOperation(t, err, "OAuthAuth.Refresh")
	}
	if _, err := auth.ToAuth(ctx, ai.OAuthCredential{}); err == nil {
		t.Fatal("stub ToAuth() error = nil")
	} else {
		assertNotImplementedOperation(t, err, "OAuthAuth.ToAuth")
	}
}

func TestEnvAPIKeyAuthPrefersStoredKeyThenEnvThenAbsent(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	auth := ai.EnvAPIKeyAuth("Anthropic API key", "ANTHROPIC_API_KEY", "ANTHROPIC_KEY")

	// Stored key wins over ambient env, and preserves provider env.
	stored := &ai.APIKeyCredential{Type: ai.AuthTypeAPIKey, Key: ai.Some("stored-key"), Env: ai.ProviderEnv{"REGION": "us"}}
	fakeCtx := &fakeAuthContext{env: map[string]string{"ANTHROPIC_API_KEY": "env-key"}}
	result, err := auth.Resolve(ctx, ai.APIKeyResolveInput{Context: fakeCtx, Credential: stored})
	if err != nil {
		t.Fatalf("Resolve(stored) error = %v", err)
	}
	got, ok := result.Value()
	if !ok {
		t.Fatal("Resolve(stored) returned absent, want the stored credential")
	}
	if key, _ := got.Auth.APIKey.Value(); key != "stored-key" {
		t.Fatalf("resolved apiKey = %q, want stored-key", key)
	}
	if source, _ := got.Source.Value(); source != "stored credential" {
		t.Fatalf("resolved source = %q, want stored credential", source)
	}
	if got.Env["REGION"] != "us" {
		t.Fatalf("resolved env = %v, want the credential env preserved", got.Env)
	}
	if fakeCtx.calls != 0 {
		t.Fatalf("stored key resolution consulted env %d times, want zero", fakeCtx.calls)
	}

	// No stored key: the first set env var wins, in order.
	fakeCtx = &fakeAuthContext{env: map[string]string{"ANTHROPIC_KEY": "second-env"}}
	result, err = auth.Resolve(ctx, ai.APIKeyResolveInput{Context: fakeCtx})
	if err != nil {
		t.Fatalf("Resolve(env) error = %v", err)
	}
	got, _ = result.Value()
	if key, _ := got.Auth.APIKey.Value(); key != "second-env" {
		t.Fatalf("resolved apiKey = %q, want second-env", key)
	}
	if source, _ := got.Source.Value(); source != "ANTHROPIC_KEY" {
		t.Fatalf("resolved source = %q, want ANTHROPIC_KEY", source)
	}

	// Nothing stored and nothing in the environment: not configured.
	fakeCtx = &fakeAuthContext{}
	result, err = auth.Resolve(ctx, ai.APIKeyResolveInput{Context: fakeCtx})
	if err != nil {
		t.Fatalf("Resolve(absent) error = %v", err)
	}
	if _, ok := result.Value(); ok {
		t.Fatalf("Resolve(absent) = %#v, want absent", result)
	}

	// A keyless credential with a nil context reports the not-configured state
	// without dereferencing a missing ambient source.
	result, err = auth.Resolve(ctx, ai.APIKeyResolveInput{Credential: &ai.APIKeyCredential{Type: ai.AuthTypeAPIKey}})
	if err != nil {
		t.Fatalf("Resolve(keyless, nil ctx) error = %v", err)
	}
	if _, ok := result.Value(); ok {
		t.Fatal("Resolve(keyless, nil ctx) = present, want absent")
	}
}

func TestEnvAPIKeyAuthLoginPromptsForSecret(t *testing.T) {
	t.Parallel()

	auth := ai.EnvAPIKeyAuth("Anthropic API key", "ANTHROPIC_API_KEY")
	interaction := &recordingInteraction{response: "typed-key"}
	credential, err := auth.Login(context.Background(), interaction)
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if credential.Type != ai.AuthTypeAPIKey {
		t.Fatalf("login credential type = %q, want api_key", credential.Type)
	}
	if key, _ := credential.Key.Value(); key != "typed-key" {
		t.Fatalf("login credential key = %q, want typed-key", key)
	}
	if interaction.lastPromptType != ai.AuthPromptTypeSecret {
		t.Fatalf("login prompt type = %q, want secret", interaction.lastPromptType)
	}
}

func TestLazyOAuthLoadsOnceOnFirstUse(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	var loads int32
	inner := ai.OAuthAuth{
		Name: "inner",
		ToAuth: func(context.Context, ai.OAuthCredential) (ai.ModelAuth, error) {
			return ai.ModelAuth{APIKey: ai.Some("derived")}, nil
		},
	}
	lazy := ai.LazyOAuth(ai.LazyOAuthInput{
		Name:           "Lazy",
		IsSubscription: true,
		LoginLabel:     ai.Some("Sign in"),
		Load: func() (ai.OAuthAuth, error) {
			atomic.AddInt32(&loads, 1)
			return inner, nil
		},
	})
	if lazy.Name != "Lazy" || !lazy.IsSubscription {
		t.Fatalf("lazy wrapper dropped advertised metadata: %#v", lazy)
	}

	for i := 0; i < 3; i++ {
		auth, err := lazy.ToAuth(ctx, ai.OAuthCredential{})
		if err != nil {
			t.Fatalf("ToAuth() error = %v", err)
		}
		if key, _ := auth.APIKey.Value(); key != "derived" {
			t.Fatalf("ToAuth() apiKey = %q, want derived", key)
		}
	}
	if got := atomic.LoadInt32(&loads); got != 1 {
		t.Fatalf("Load called %d times, want exactly once", got)
	}
}

func TestLazyOAuthReportsNotImplementedWithoutLoader(t *testing.T) {
	t.Parallel()

	lazy := ai.LazyOAuth(ai.LazyOAuthInput{Name: "Lazy"})
	if _, err := lazy.ToAuth(context.Background(), ai.OAuthCredential{}); err == nil {
		t.Fatal("ToAuth() error = nil, want ErrNotImplemented")
	} else {
		assertNotImplementedOperation(t, err, "OAuthAuth.Load")
	}
}

func TestModelsErrorWrapsCauseAndCarriesCode(t *testing.T) {
	t.Parallel()

	const secret = "secret-provider-response"
	cause := errors.New(secret)
	err := &ai.ModelsError{Code: ai.ModelsErrorCodeAuth, Message: secret, Cause: cause}
	if !errors.Is(err, cause) {
		t.Fatal("errors.Is(ModelsError, cause) = false, want the cause reachable")
	}
	var target *ai.ModelsError
	if !errors.As(err, &target) || target.Code != ai.ModelsErrorCodeAuth {
		t.Fatalf("errors.As(*ModelsError) = %#v, want code auth", target)
	}
	if got := err.Error(); got != "auth: request failed" {
		t.Fatalf("ModelsError.Error() = %q, want redacted fallback", got)
	} else if strings.Contains(got, secret) {
		t.Fatalf("ModelsError.Error() leaked externally supplied fields: %q", got)
	}
}

func TestModelsErrorRenderingRejectsExternalCodeText(t *testing.T) {
	t.Parallel()

	const secret = "sk-secret-error-code"
	err := &ai.ModelsError{Code: ai.ModelsErrorCode(secret), Message: secret, Cause: errors.New(secret)}
	if got := err.Error(); got != "provider: request failed" {
		t.Fatalf("ModelsError.Error() = %q, want safe fallback for unknown code", got)
	}
}

func TestResolveProviderAuthOverrideAPIKeyBypassesStore(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := ai.NewInMemoryCredentialStore()
	apiKeyAuth := ai.EnvAPIKeyAuth("Anthropic API key", "ANTHROPIC_API_KEY")
	target := ai.ProviderAuthTarget{ID: "anthropic", Auth: ai.ProviderAuth{APIKey: &apiKeyAuth}}

	result, err := ai.ResolveProviderAuth(ctx, target, store, &fakeAuthContext{}, ai.AuthResolutionOverrides{APIKey: ai.Some("override-key")})
	if err != nil {
		t.Fatalf("ResolveProviderAuth(override) error = %v", err)
	}
	got, ok := result.Value()
	if !ok {
		t.Fatal("ResolveProviderAuth(override) = absent, want the override key")
	}
	if key, _ := got.Auth.APIKey.Value(); key != "override-key" {
		t.Fatalf("resolved apiKey = %q, want override-key", key)
	}
}

func TestResolveProviderAuthStoredCredentialOwnsProvider(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := ai.NewInMemoryCredentialStore()
	if _, err := store.Modify(ctx, "anthropic", func(context.Context, ai.Credential) (ai.Credential, error) {
		return ai.APIKeyCredential{Type: ai.AuthTypeAPIKey, Key: ai.Some("stored")}, nil
	}, ai.AuthOperationOptions{}); err != nil {
		t.Fatalf("seed store error = %v", err)
	}

	apiKeyAuth := ai.EnvAPIKeyAuth("Anthropic API key", "ANTHROPIC_API_KEY")
	target := ai.ProviderAuthTarget{ID: "anthropic", Auth: ai.ProviderAuth{APIKey: &apiKeyAuth}}
	// The environment has a key, but the stored credential owns the provider.
	fakeCtx := &fakeAuthContext{env: map[string]string{"ANTHROPIC_API_KEY": "ambient"}}

	result, err := ai.ResolveProviderAuth(ctx, target, store, fakeCtx, ai.AuthResolutionOverrides{})
	if err != nil {
		t.Fatalf("ResolveProviderAuth(stored) error = %v", err)
	}
	got, _ := result.Value()
	if key, _ := got.Auth.APIKey.Value(); key != "stored" {
		t.Fatalf("resolved apiKey = %q, want stored (no ambient fallback)", key)
	}
}

func TestResolveProviderAuthRejectsForgedStoredCredentialDiscriminators(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		credential ai.Credential
	}{
		{
			name:       "api key concrete type tagged oauth",
			credential: ai.APIKeyCredential{Type: ai.AuthTypeOAuth, Key: ai.Some("must-not-resolve")},
		},
		{
			name:       "api key concrete type with empty discriminator",
			credential: ai.APIKeyCredential{Key: ai.Some("must-not-resolve")},
		},
		{
			name: "oauth concrete type tagged api key",
			credential: ai.OAuthCredential{
				OAuthCredentials: ai.OAuthCredentials{Refresh: "must-not-refresh", Access: "must-not-resolve", Expires: farFuture()},
				Type:             ai.AuthTypeAPIKey,
			},
		},
		{
			name: "oauth concrete type with empty discriminator",
			credential: ai.OAuthCredential{
				OAuthCredentials: ai.OAuthCredentials{Refresh: "must-not-refresh", Access: "must-not-resolve", Expires: farFuture()},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			store := ai.NewInMemoryCredentialStore()
			if _, err := store.Modify(ctx, "forged", func(context.Context, ai.Credential) (ai.Credential, error) {
				return test.credential, nil
			}, ai.AuthOperationOptions{}); err != nil {
				t.Fatalf("seed store error = %v", err)
			}

			apiKey := ai.APIKeyAuth{Resolve: func(context.Context, ai.APIKeyResolveInput) (ai.Optional[ai.AuthResult], error) {
				t.Fatal("APIKeyAuth.Resolve called for forged stored credential")
				return ai.Absent[ai.AuthResult](), nil
			}}
			oauth := ai.OAuthAuth{
				Refresh: func(context.Context, ai.OAuthCredential) (ai.OAuthCredential, error) {
					t.Fatal("OAuthAuth.Refresh called for forged stored credential")
					return ai.OAuthCredential{}, nil
				},
				ToAuth: func(context.Context, ai.OAuthCredential) (ai.ModelAuth, error) {
					t.Fatal("OAuthAuth.ToAuth called for forged stored credential")
					return ai.ModelAuth{}, nil
				},
			}
			target := ai.ProviderAuthTarget{ID: "forged", Auth: ai.ProviderAuth{APIKey: &apiKey, OAuth: &oauth}}

			_, err := ai.ResolveProviderAuth(ctx, target, store, ai.DefaultProviderAuthContext(), ai.AuthResolutionOverrides{})
			assertModelsErrorCode(t, err, ai.ModelsErrorCodeAuth)
		})
	}
}

func TestResolveProviderAuthDoesNotExposeStoredAPIKeyCredentialToResolve(t *testing.T) {
	ctx := context.Background()
	store := ai.NewInMemoryCredentialStore()
	if _, err := store.Modify(ctx, "isolated-resolve", func(context.Context, ai.Credential) (ai.Credential, error) {
		return ai.APIKeyCredential{
			Type: ai.AuthTypeAPIKey,
			Key:  ai.Some("stored-key"),
			Env:  ai.ProviderEnv{"account": "stored"},
		}, nil
	}, ai.AuthOperationOptions{}); err != nil {
		t.Fatalf("seed store error = %v", err)
	}

	callbackErr := errors.New("resolve failed")
	auth := ai.APIKeyAuth{
		Name: "Mutating resolver",
		Resolve: func(_ context.Context, input ai.APIKeyResolveInput) (ai.Optional[ai.AuthResult], error) {
			input.Credential.Env["account"] = "mutated"
			return ai.Absent[ai.AuthResult](), callbackErr
		},
	}
	target := ai.ProviderAuthTarget{ID: "isolated-resolve", Auth: ai.ProviderAuth{APIKey: &auth}}

	if _, err := ai.ResolveProviderAuth(ctx, target, store, ai.DefaultProviderAuthContext(), ai.AuthResolutionOverrides{}); !errors.Is(err, callbackErr) {
		t.Fatalf("ResolveProviderAuth() error = %v, want wrapped callback error", err)
	}
	stored, err := store.Read(ctx, "isolated-resolve", ai.AuthOperationOptions{})
	if err != nil {
		t.Fatal(err)
	}
	credential, ok := stored.(ai.APIKeyCredential)
	if !ok || credential.Env["account"] != "stored" {
		t.Fatalf("stored credential after failed Resolve = %#v, want original env", stored)
	}
}

func TestResolveProviderAuthStoredCredentialWithoutHandlerReturnsAbsent(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := ai.NewInMemoryCredentialStore()
	// Store an OAuth credential but advertise only api-key auth: no handler, and
	// no silent ambient fallback.
	if _, err := store.Modify(ctx, "anthropic", func(context.Context, ai.Credential) (ai.Credential, error) {
		return ai.OAuthCredential{OAuthCredentials: ai.OAuthCredentials{Refresh: "r", Access: "a", Expires: farFuture()}, Type: ai.AuthTypeOAuth}, nil
	}, ai.AuthOperationOptions{}); err != nil {
		t.Fatalf("seed store error = %v", err)
	}

	apiKeyAuth := ai.EnvAPIKeyAuth("Anthropic API key", "ANTHROPIC_API_KEY")
	target := ai.ProviderAuthTarget{ID: "anthropic", Auth: ai.ProviderAuth{APIKey: &apiKeyAuth}}
	fakeCtx := &fakeAuthContext{env: map[string]string{"ANTHROPIC_API_KEY": "ambient"}}

	result, err := ai.ResolveProviderAuth(ctx, target, store, fakeCtx, ai.AuthResolutionOverrides{})
	if err != nil {
		t.Fatalf("ResolveProviderAuth(mismatch) error = %v", err)
	}
	if _, ok := result.Value(); ok {
		t.Fatalf("ResolveProviderAuth(mismatch) = %#v, want absent", result)
	}
}

func TestResolveProviderAuthFallsBackToAmbientWhenNothingStored(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := ai.NewInMemoryCredentialStore()
	apiKeyAuth := ai.EnvAPIKeyAuth("Anthropic API key", "ANTHROPIC_API_KEY")
	target := ai.ProviderAuthTarget{ID: "anthropic", Auth: ai.ProviderAuth{APIKey: &apiKeyAuth}}
	fakeCtx := &fakeAuthContext{env: map[string]string{"ANTHROPIC_API_KEY": "ambient"}}

	result, err := ai.ResolveProviderAuth(ctx, target, store, fakeCtx, ai.AuthResolutionOverrides{})
	if err != nil {
		t.Fatalf("ResolveProviderAuth(ambient) error = %v", err)
	}
	got, _ := result.Value()
	if key, _ := got.Auth.APIKey.Value(); key != "ambient" {
		t.Fatalf("resolved apiKey = %q, want ambient", key)
	}
}

func TestResolveProviderAuthRefreshesExpiringOAuthUnderLock(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := ai.NewInMemoryCredentialStore()
	expiring := ai.OAuthCredential{
		OAuthCredentials: ai.OAuthCredentials{Refresh: "r0", Access: "expiring", Expires: time.Now().Add(time.Minute).UnixMilli()},
		Type:             ai.AuthTypeOAuth,
	}
	if _, err := store.Modify(ctx, "anthropic", func(context.Context, ai.Credential) (ai.Credential, error) {
		return expiring, nil
	}, ai.AuthOperationOptions{}); err != nil {
		t.Fatalf("seed store error = %v", err)
	}

	var refreshes int32
	refreshed := ai.OAuthCredential{
		OAuthCredentials: ai.OAuthCredentials{Refresh: "r1", Access: "fresh", Expires: farFuture()},
		Type:             ai.AuthTypeOAuth,
	}
	oauthAuth := ai.OAuthAuth{
		Name: "Anthropic OAuth",
		Refresh: func(_ context.Context, current ai.OAuthCredential) (ai.OAuthCredential, error) {
			atomic.AddInt32(&refreshes, 1)
			if current.Access != "expiring" {
				t.Fatalf("refresh saw credential %#v, want the expiring one", current)
			}
			return refreshed, nil
		},
		ToAuth: func(_ context.Context, current ai.OAuthCredential) (ai.ModelAuth, error) {
			return ai.ModelAuth{APIKey: ai.Some(current.Access)}, nil
		},
	}
	target := ai.ProviderAuthTarget{ID: "anthropic", Auth: ai.ProviderAuth{OAuth: &oauthAuth}}

	result, err := ai.ResolveProviderAuth(ctx, target, store, ai.DefaultProviderAuthContext(), ai.AuthResolutionOverrides{})
	if err != nil {
		t.Fatalf("ResolveProviderAuth(oauth) error = %v", err)
	}
	got, _ := result.Value()
	if key, _ := got.Auth.APIKey.Value(); key != "fresh" {
		t.Fatalf("derived apiKey = %q, want fresh (post-refresh)", key)
	}
	if source, _ := got.Source.Value(); source != "OAuth" {
		t.Fatalf("resolved source = %q, want OAuth", source)
	}
	if got := atomic.LoadInt32(&refreshes); got != 1 {
		t.Fatalf("refresh called %d times, want exactly once", got)
	}
	// The rotated credential must be persisted so a later request sees it fresh.
	stored, _ := store.Read(ctx, "anthropic", ai.AuthOperationOptions{})
	if stored.(ai.OAuthCredential).Access != "fresh" {
		t.Fatalf("stored credential = %#v, want the refreshed token persisted", stored)
	}
}

func TestResolveProviderAuthDoesNotExposeStoredOAuthCredentialToRefresh(t *testing.T) {
	ctx := context.Background()
	store := ai.NewInMemoryCredentialStore()
	if _, err := store.Modify(ctx, "isolated-oauth-refresh", func(context.Context, ai.Credential) (ai.Credential, error) {
		return ai.OAuthCredential{
			OAuthCredentials: ai.OAuthCredentials{
				Refresh: "refresh-token",
				Access:  "expired",
				Expires: 0,
				Extra:   map[string]json.RawMessage{"account": json.RawMessage(`"stored"`)},
			},
			Type: ai.AuthTypeOAuth,
		}, nil
	}, ai.AuthOperationOptions{}); err != nil {
		t.Fatalf("seed store error = %v", err)
	}

	callbackErr := errors.New("refresh failed")
	oauth := ai.OAuthAuth{
		Name: "Mutating refresh",
		Refresh: func(_ context.Context, credential ai.OAuthCredential) (ai.OAuthCredential, error) {
			credential.Extra["account"] = json.RawMessage(`"mutated"`)
			return ai.OAuthCredential{}, callbackErr
		},
		ToAuth: func(context.Context, ai.OAuthCredential) (ai.ModelAuth, error) {
			return ai.ModelAuth{}, nil
		},
	}
	target := ai.ProviderAuthTarget{ID: "isolated-oauth-refresh", Auth: ai.ProviderAuth{OAuth: &oauth}}

	if _, err := ai.ResolveProviderAuth(ctx, target, store, ai.DefaultProviderAuthContext(), ai.AuthResolutionOverrides{}); !errors.Is(err, callbackErr) {
		t.Fatalf("ResolveProviderAuth() error = %v, want wrapped callback error", err)
	}
	stored, err := store.Read(ctx, "isolated-oauth-refresh", ai.AuthOperationOptions{})
	if err != nil {
		t.Fatal(err)
	}
	credential, ok := stored.(ai.OAuthCredential)
	if !ok || string(credential.Extra["account"]) != `"stored"` {
		t.Fatalf("stored credential after failed Refresh = %#v, want original extra", stored)
	}
}

func TestResolveProviderAuthRejectsMismatchedRefreshDiscriminator(t *testing.T) {
	ctx := context.Background()
	store := ai.NewInMemoryCredentialStore()
	if _, err := store.Modify(ctx, "mismatched-refresh", func(context.Context, ai.Credential) (ai.Credential, error) {
		return ai.OAuthCredential{
			OAuthCredentials: ai.OAuthCredentials{Refresh: "refresh-token", Access: "old", Expires: 0},
			Type:             ai.AuthTypeOAuth,
		}, nil
	}, ai.AuthOperationOptions{}); err != nil {
		t.Fatalf("seed store error = %v", err)
	}

	oauth := ai.OAuthAuth{
		Name: "Mismatched refresh",
		Refresh: func(context.Context, ai.OAuthCredential) (ai.OAuthCredential, error) {
			return ai.OAuthCredential{
				OAuthCredentials: ai.OAuthCredentials{Refresh: "rotated", Access: "new", Expires: farFuture()},
				Type:             ai.AuthTypeAPIKey,
			}, nil
		},
		ToAuth: func(context.Context, ai.OAuthCredential) (ai.ModelAuth, error) {
			return ai.ModelAuth{}, nil
		},
	}
	target := ai.ProviderAuthTarget{ID: "mismatched-refresh", Auth: ai.ProviderAuth{OAuth: &oauth}}

	_, err := ai.ResolveProviderAuth(ctx, target, store, ai.DefaultProviderAuthContext(), ai.AuthResolutionOverrides{})
	var modelsErr *ai.ModelsError
	if !errors.As(err, &modelsErr) || modelsErr.Code != ai.ModelsErrorCodeOAuth {
		t.Fatalf("ResolveProviderAuth() error = %v, want oauth ModelsError", err)
	}
	stored, readErr := store.Read(ctx, "mismatched-refresh", ai.AuthOperationOptions{})
	if readErr != nil {
		t.Fatal(readErr)
	}
	credential, ok := stored.(ai.OAuthCredential)
	if !ok || credential.Access != "old" || credential.Type != ai.AuthTypeOAuth {
		t.Fatalf("stored credential = %#v, want original OAuth credential", stored)
	}
}

func TestResolveProviderAuthSkipsRefreshForValidOAuth(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := ai.NewInMemoryCredentialStore()
	valid := ai.OAuthCredential{
		OAuthCredentials: ai.OAuthCredentials{Refresh: "r", Access: "valid", Expires: farFuture()},
		Type:             ai.AuthTypeOAuth,
	}
	if _, err := store.Modify(ctx, "anthropic", func(context.Context, ai.Credential) (ai.Credential, error) {
		return valid, nil
	}, ai.AuthOperationOptions{}); err != nil {
		t.Fatalf("seed store error = %v", err)
	}

	oauthAuth := ai.OAuthAuth{
		Name: "Anthropic OAuth",
		Refresh: func(context.Context, ai.OAuthCredential) (ai.OAuthCredential, error) {
			t.Fatal("Refresh ran for a token that is still valid")
			return ai.OAuthCredential{}, nil
		},
		ToAuth: func(_ context.Context, current ai.OAuthCredential) (ai.ModelAuth, error) {
			return ai.ModelAuth{APIKey: ai.Some(current.Access)}, nil
		},
	}
	target := ai.ProviderAuthTarget{ID: "anthropic", Auth: ai.ProviderAuth{OAuth: &oauthAuth}}

	result, err := ai.ResolveProviderAuth(ctx, target, store, ai.DefaultProviderAuthContext(), ai.AuthResolutionOverrides{})
	if err != nil {
		t.Fatalf("ResolveProviderAuth(valid oauth) error = %v", err)
	}
	got, _ := result.Value()
	if key, _ := got.Auth.APIKey.Value(); key != "valid" {
		t.Fatalf("derived apiKey = %q, want valid", key)
	}
}

func TestResolveProviderAuthDoesNotExposeStoredOAuthCredentialToToAuth(t *testing.T) {
	ctx := context.Background()
	store := ai.NewInMemoryCredentialStore()
	if _, err := store.Modify(ctx, "isolated-oauth-to-auth", func(context.Context, ai.Credential) (ai.Credential, error) {
		return ai.OAuthCredential{
			OAuthCredentials: ai.OAuthCredentials{
				Refresh: "refresh-token",
				Access:  "valid",
				Expires: farFuture(),
				Extra:   map[string]json.RawMessage{"account": json.RawMessage(`"stored"`)},
			},
			Type: ai.AuthTypeOAuth,
		}, nil
	}, ai.AuthOperationOptions{}); err != nil {
		t.Fatalf("seed store error = %v", err)
	}

	callbackErr := errors.New("auth derivation failed")
	oauth := ai.OAuthAuth{
		Name: "Mutating auth derivation",
		ToAuth: func(_ context.Context, credential ai.OAuthCredential) (ai.ModelAuth, error) {
			credential.Extra["account"] = json.RawMessage(`"mutated"`)
			return ai.ModelAuth{}, callbackErr
		},
	}
	target := ai.ProviderAuthTarget{ID: "isolated-oauth-to-auth", Auth: ai.ProviderAuth{OAuth: &oauth}}

	if _, err := ai.ResolveProviderAuth(ctx, target, store, ai.DefaultProviderAuthContext(), ai.AuthResolutionOverrides{}); !errors.Is(err, callbackErr) {
		t.Fatalf("ResolveProviderAuth() error = %v, want wrapped callback error", err)
	}
	stored, err := store.Read(ctx, "isolated-oauth-to-auth", ai.AuthOperationOptions{})
	if err != nil {
		t.Fatal(err)
	}
	credential, ok := stored.(ai.OAuthCredential)
	if !ok || string(credential.Extra["account"]) != `"stored"` {
		t.Fatalf("stored credential after failed ToAuth = %#v, want original extra", stored)
	}
}

func TestResolveProviderAuthClassifiesFailuresAsModelsError(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// A failing api-key resolve is classified as an auth error.
	t.Run("api key resolve failure", func(t *testing.T) {
		t.Parallel()
		store := ai.NewInMemoryCredentialStore()
		apiKeyAuth := ai.APIKeyAuth{
			Name: "failing",
			Resolve: func(context.Context, ai.APIKeyResolveInput) (ai.Optional[ai.AuthResult], error) {
				return ai.Absent[ai.AuthResult](), errors.New("provider command exploded")
			},
		}
		target := ai.ProviderAuthTarget{ID: "anthropic", Auth: ai.ProviderAuth{APIKey: &apiKeyAuth}}
		_, err := ai.ResolveProviderAuth(ctx, target, store, &fakeAuthContext{}, ai.AuthResolutionOverrides{})
		assertModelsErrorCode(t, err, ai.ModelsErrorCodeAuth)
	})

	// A failing OAuth refresh is classified as an oauth error and preserves the
	// underlying cause.
	t.Run("oauth refresh failure", func(t *testing.T) {
		t.Parallel()
		store := ai.NewInMemoryCredentialStore()
		if _, err := store.Modify(ctx, "anthropic", func(context.Context, ai.Credential) (ai.Credential, error) {
			return ai.OAuthCredential{OAuthCredentials: ai.OAuthCredentials{Refresh: "r", Access: "old", Expires: time.Now().Add(time.Minute).UnixMilli()}, Type: ai.AuthTypeOAuth}, nil
		}, ai.AuthOperationOptions{}); err != nil {
			t.Fatalf("seed store error = %v", err)
		}
		cause := errors.New("invalid_grant")
		oauthAuth := ai.OAuthAuth{
			Name: "failing",
			Refresh: func(context.Context, ai.OAuthCredential) (ai.OAuthCredential, error) {
				return ai.OAuthCredential{}, cause
			},
			ToAuth: func(context.Context, ai.OAuthCredential) (ai.ModelAuth, error) { return ai.ModelAuth{}, nil },
		}
		target := ai.ProviderAuthTarget{ID: "anthropic", Auth: ai.ProviderAuth{OAuth: &oauthAuth}}
		_, err := ai.ResolveProviderAuth(ctx, target, store, ai.DefaultProviderAuthContext(), ai.AuthResolutionOverrides{})
		assertModelsErrorCode(t, err, ai.ModelsErrorCodeOAuth)
		if !errors.Is(err, cause) {
			t.Fatalf("refresh failure error = %v, want the underlying cause reachable", err)
		}
	})

	// A stub OAuthAuth surfaces ErrNotImplemented classified as an oauth error.
	t.Run("stub oauth refresh", func(t *testing.T) {
		t.Parallel()
		store := ai.NewInMemoryCredentialStore()
		if _, err := store.Modify(ctx, "anthropic", func(context.Context, ai.Credential) (ai.Credential, error) {
			return ai.OAuthCredential{OAuthCredentials: ai.OAuthCredentials{Refresh: "r", Access: "old", Expires: time.Now().Add(time.Minute).UnixMilli()}, Type: ai.AuthTypeOAuth}, nil
		}, ai.AuthOperationOptions{}); err != nil {
			t.Fatalf("seed store error = %v", err)
		}
		stub := ai.NewStubOAuthAuth("Anthropic OAuth")
		target := ai.ProviderAuthTarget{ID: "anthropic", Auth: ai.ProviderAuth{OAuth: &stub}}
		_, err := ai.ResolveProviderAuth(ctx, target, store, ai.DefaultProviderAuthContext(), ai.AuthResolutionOverrides{})
		assertModelsErrorCode(t, err, ai.ModelsErrorCodeOAuth)
		if !errors.Is(err, ai.ErrNotImplemented) {
			t.Fatalf("stub refresh error = %v, want ErrNotImplemented reachable", err)
		}
	})
}

func TestResolveProviderAuthHonorsContextCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	store := ai.NewInMemoryCredentialStore()
	apiKeyAuth := ai.EnvAPIKeyAuth("Anthropic API key", "ANTHROPIC_API_KEY")
	target := ai.ProviderAuthTarget{ID: "anthropic", Auth: ai.ProviderAuth{APIKey: &apiKeyAuth}}

	if _, err := ai.ResolveProviderAuth(ctx, target, store, &fakeAuthContext{}, ai.AuthResolutionOverrides{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("ResolveProviderAuth(cancelled) error = %v, want context.Canceled", err)
	}
}

func assertModelsErrorCode(t *testing.T, err error, want ai.ModelsErrorCode) {
	t.Helper()
	if err == nil {
		t.Fatalf("error = nil, want ModelsError with code %q", want)
	}
	var target *ai.ModelsError
	if !errors.As(err, &target) {
		t.Fatalf("error = %v, want *ModelsError", err)
	}
	if target.Code != want {
		t.Fatalf("ModelsError code = %q, want %q", target.Code, want)
	}
}

func farFuture() int64 {
	return time.Now().Add(24 * time.Hour).UnixMilli()
}

// fakeAuthContext is a map-backed AuthContext for tests: no process env or
// filesystem access, and it counts env lookups so tests can assert that a
// stored credential short-circuits ambient resolution.
type fakeAuthContext struct {
	env   map[string]string
	files map[string]bool
	calls int
}

func (c *fakeAuthContext) Env(_ context.Context, name string) (string, error) {
	c.calls++
	return c.env[name], nil
}

func (c *fakeAuthContext) FileExists(_ context.Context, path string) (bool, error) {
	return c.files[path], nil
}

// recordingInteraction is an AuthInteraction test double that returns a canned
// prompt response and counts callbacks.
type recordingInteraction struct {
	response       string
	prompts        int
	notices        int
	lastPromptType ai.AuthPromptType
}

func (r *recordingInteraction) Prompt(_ context.Context, prompt ai.AuthPrompt) (string, error) {
	r.prompts++
	r.lastPromptType = prompt.AuthPromptType()
	return r.response, nil
}

func (r *recordingInteraction) Notify(ai.AuthEvent) {
	r.notices++
}

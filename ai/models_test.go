package ai_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/telemetry"
)

type modelsTestProvider struct {
	ai.Provider
	id     ai.ProviderID
	models []ai.Model
	auth   ai.ProviderAuth
	filter func([]ai.Model, ai.Credential) []ai.Model
	panic  bool
}

type modelsRefreshTestProvider struct {
	ai.Provider
	id       ai.ProviderID
	auth     ai.ProviderAuth
	supports func() bool
	refresh  func(ai.RefreshModelsContext) error
}

func (p *modelsRefreshTestProvider) ID() ai.ProviderID     { return p.id }
func (p *modelsRefreshTestProvider) Auth() ai.ProviderAuth { return p.auth }
func (p *modelsRefreshTestProvider) SupportsRefreshModels() bool {
	if p.supports != nil {
		return p.supports()
	}
	return p.refresh != nil
}
func (p *modelsRefreshTestProvider) RefreshModels(ctx ai.RefreshModelsContext) error {
	if p.refresh == nil {
		return errors.New("unexpected refresh")
	}
	return p.refresh(ctx)
}

func (p *modelsTestProvider) ID() ai.ProviderID { return p.id }
func (p *modelsTestProvider) Name() string      { return string(p.id) }
func (p *modelsTestProvider) Auth() ai.ProviderAuth {
	return p.auth
}

func (p *modelsTestProvider) GetModels() []ai.Model {
	if p.panic {
		panic("provider model source failed")
	}
	return p.models
}

func (p *modelsTestProvider) FilterModels(models []ai.Model, credential ai.Credential) []ai.Model {
	if p.filter != nil {
		return p.filter(models, credential)
	}
	return models
}

func TestMutableModelsRegistryQueriesAreOrderedAndIsolated(t *testing.T) {
	models := ai.CreateModels()
	first := &modelsTestProvider{id: "first", models: []ai.Model{{
		ID:       "first-model",
		Provider: "first",
		Headers:  map[string]string{"X-Model": "original"},
	}}}
	broken := &modelsTestProvider{id: "broken", panic: true}
	third := &modelsTestProvider{id: "third", models: []ai.Model{{ID: "third-model", Provider: "third"}}}

	models.SetProvider(first)
	models.SetProvider(broken)
	models.SetProvider(third)
	models.SetProvider(&modelsTestProvider{id: "first", models: []ai.Model{{ID: "replacement", Provider: "first"}}})

	providers := models.GetProviders()
	gotProviderIDs := make([]ai.ProviderID, len(providers))
	for index, provider := range providers {
		gotProviderIDs[index] = provider.ID()
	}
	if want := []ai.ProviderID{"first", "broken", "third"}; !reflect.DeepEqual(gotProviderIDs, want) {
		t.Fatalf("GetProviders() IDs = %v, want %v", gotProviderIDs, want)
	}
	providers[0] = nil
	if provider, ok := models.GetProvider("first"); !ok || provider == nil || provider.GetModels()[0].ID != "replacement" {
		t.Fatalf("GetProvider(first) = (%v, %v), want replacement provider", provider, ok)
	}

	got := models.GetModels()
	if want := []string{"replacement", "third-model"}; !reflect.DeepEqual(modelIDs(got), want) {
		t.Fatalf("GetModels() IDs = %v, want %v", modelIDs(got), want)
	}
	if brokenModels := models.GetModels("broken"); len(brokenModels) != 0 {
		t.Fatalf("GetModels(broken) = %v, want empty best-effort result", brokenModels)
	}

	got[0].ID = "mutated"
	got[0].Headers = map[string]string{"X-Model": "mutated"}
	if model, ok := models.GetModel("first", "replacement"); !ok || model.ID != "replacement" {
		t.Fatalf("GetModel(first, replacement) = (%+v, %v), want isolated replacement snapshot", model, ok)
	}

	models.DeleteProvider("broken")
	models.SetProvider(broken)
	if got := providerIDs(models.GetProviders()); !reflect.DeepEqual(got, []ai.ProviderID{"first", "third", "broken"}) {
		t.Fatalf("provider order after delete/re-add = %v, want [first third broken]", got)
	}
	models.ClearProviders()
	if got := models.GetProviders(); len(got) != 0 {
		t.Fatalf("GetProviders() after ClearProviders = %v, want empty", got)
	}
}

func TestModelsCheckAuthDoesNotRefreshOAuthAndGetAvailableFilters(t *testing.T) {
	ctx := context.Background()
	credentials := ai.NewInMemoryCredentialStore()
	refreshes := 0
	oauth := ai.OAuthAuth{
		Name: "Test OAuth",
		Refresh: func(context.Context, ai.OAuthCredential) (ai.OAuthCredential, error) {
			refreshes++
			return ai.OAuthCredential{}, nil
		},
	}
	_, err := credentials.Modify(ctx, "oauth", func(context.Context, ai.Credential) (ai.Credential, error) {
		return ai.OAuthCredential{
			OAuthCredentials: ai.OAuthCredentials{Access: "expired", Refresh: "refresh", Expires: 0},
			Type:             ai.AuthTypeOAuth,
		}, nil
	}, ai.AuthOperationOptions{})
	if err != nil {
		t.Fatal(err)
	}

	ambient := ai.APIKeyAuth{
		Name: "Ambient",
		Resolve: func(context.Context, ai.APIKeyResolveInput) (ai.Optional[ai.AuthResult], error) {
			return ai.Some(ai.AuthResult{Auth: ai.ModelAuth{APIKey: ai.Some("ambient-key")}, Source: ai.Some("ambient")}), nil
		},
	}
	unconfigured := ai.APIKeyAuth{
		Name: "Missing",
		Resolve: func(context.Context, ai.APIKeyResolveInput) (ai.Optional[ai.AuthResult], error) {
			return ai.Absent[ai.AuthResult](), nil
		},
	}
	models := ai.CreateModels(ai.CreateModelsOptions{Credentials: credentials})
	models.SetProvider(ai.CreateProvider(ai.CreateProviderOptions{
		ID:     "ambient",
		Auth:   ai.ProviderAuth{APIKey: &ambient},
		Models: []ai.Model{{ID: "visible", Provider: "ambient"}, {ID: "hidden", Provider: "ambient"}},
		FilterModels: func(candidates []ai.Model, credential ai.Credential) []ai.Model {
			if credential != nil {
				t.Fatalf("ambient filter credential = %#v, want nil stored credential", credential)
			}
			return candidates[:1]
		},
		API: ai.SingleProviderAPI(ai.NewStubProviderStreams()),
	}))
	models.SetProvider(ai.CreateProvider(ai.CreateProviderOptions{
		ID:     "missing",
		Auth:   ai.ProviderAuth{APIKey: &unconfigured},
		Models: []ai.Model{{ID: "missing-model", Provider: "missing"}},
		API:    ai.SingleProviderAPI(ai.NewStubProviderStreams()),
	}))
	models.SetProvider(ai.CreateProvider(ai.CreateProviderOptions{
		ID:     "oauth",
		Auth:   ai.ProviderAuth{OAuth: &oauth},
		Models: []ai.Model{{ID: "oauth-model", Provider: "oauth"}},
		API:    ai.SingleProviderAPI(ai.NewStubProviderStreams()),
	}))

	check, err := models.CheckAuth(ctx, "oauth")
	if err != nil {
		t.Fatalf("CheckAuth(oauth) error = %v", err)
	}
	gotCheck, ok := check.Value()
	if !ok || gotCheck.Type != ai.AuthTypeOAuth || refreshes != 0 {
		t.Fatalf("CheckAuth(oauth) = (%+v, %v), refreshes = %d; want configured OAuth without refresh", gotCheck, ok, refreshes)
	}

	available, err := models.GetAvailable(ctx)
	if err != nil {
		t.Fatalf("GetAvailable() error = %v", err)
	}
	if want := []string{"visible", "oauth-model"}; !reflect.DeepEqual(modelIDs(available), want) {
		t.Fatalf("GetAvailable() IDs = %v, want %v", modelIDs(available), want)
	}
}

func TestModelsGetAvailableTreatsGetModelsPanicAsEmpty(t *testing.T) {
	configured := ai.APIKeyAuth{
		Name: "Configured",
		Check: func(context.Context, ai.APIKeyCheckInput) (ai.Optional[ai.AuthCheck], error) {
			return ai.Some(ai.AuthCheck{Type: ai.AuthTypeAPIKey}), nil
		},
	}
	models := ai.CreateModels()
	models.SetProvider(&modelsTestProvider{id: "broken", auth: ai.ProviderAuth{APIKey: &configured}, panic: true})
	models.SetProvider(&modelsTestProvider{
		id:     "healthy",
		auth:   ai.ProviderAuth{APIKey: &configured},
		models: []ai.Model{{ID: "healthy-model", Provider: "healthy"}},
	})

	available, err := models.GetAvailable(context.Background())
	if err != nil {
		t.Fatalf("GetAvailable() error = %v, want best-effort success", err)
	}
	if want := []string{"healthy-model"}; !reflect.DeepEqual(modelIDs(available), want) {
		t.Fatalf("GetAvailable() IDs = %v, want %v", modelIDs(available), want)
	}
}

func TestModelsGetAvailableReturnsModelsErrorForFilterPanicWithoutMutatingCredential(t *testing.T) {
	ctx := context.Background()
	credentials := ai.NewInMemoryCredentialStore()
	if _, err := credentials.Modify(ctx, "broken-filter", func(context.Context, ai.Credential) (ai.Credential, error) {
		return ai.APIKeyCredential{
			Type: ai.AuthTypeAPIKey,
			Key:  ai.Some("secret"),
			Env:  ai.ProviderEnv{"account": "stored"},
		}, nil
	}, ai.AuthOperationOptions{}); err != nil {
		t.Fatalf("seed store error = %v", err)
	}
	configured := ai.APIKeyAuth{
		Name: "Configured",
		Check: func(context.Context, ai.APIKeyCheckInput) (ai.Optional[ai.AuthCheck], error) {
			return ai.Some(ai.AuthCheck{Type: ai.AuthTypeAPIKey}), nil
		},
	}
	models := ai.CreateModels(ai.CreateModelsOptions{Credentials: credentials})
	models.SetProvider(&modelsTestProvider{
		id:     "broken-filter",
		auth:   ai.ProviderAuth{APIKey: &configured},
		models: []ai.Model{{ID: "model", Provider: "broken-filter"}},
		filter: func(_ []ai.Model, credential ai.Credential) []ai.Model {
			credential.(ai.APIKeyCredential).Env["account"] = "mutated"
			panic("filter failed")
		},
	})

	available, err := models.GetAvailable(ctx)
	var modelsErr *ai.ModelsError
	if !errors.As(err, &modelsErr) || modelsErr.Code != ai.ModelsErrorCodeModelSource {
		t.Fatalf("GetAvailable() error = %v, want model_source ModelsError", err)
	}
	if available != nil {
		t.Fatalf("GetAvailable() models = %v, want nil on filter panic", available)
	}
	stored, readErr := credentials.Read(ctx, "broken-filter", ai.AuthOperationOptions{})
	if readErr != nil {
		t.Fatal(readErr)
	}
	credential, ok := stored.(ai.APIKeyCredential)
	if !ok || credential.Env["account"] != "stored" {
		t.Fatalf("stored credential after FilterModels panic = %#v, want original env", stored)
	}
}

func TestModelsCheckAuthDoesNotExposeStoredCredentialToProviderCallback(t *testing.T) {
	ctx := context.Background()
	credentials := ai.NewInMemoryCredentialStore()
	_, err := credentials.Modify(ctx, "isolated-check", func(context.Context, ai.Credential) (ai.Credential, error) {
		return ai.APIKeyCredential{
			Type: ai.AuthTypeAPIKey,
			Key:  ai.Some("secret"),
			Env:  ai.ProviderEnv{"account": "stored"},
		}, nil
	}, ai.AuthOperationOptions{})
	if err != nil {
		t.Fatal(err)
	}

	callbackErr := errors.New("check failed")
	auth := ai.APIKeyAuth{
		Name: "Isolated check",
		Check: func(_ context.Context, input ai.APIKeyCheckInput) (ai.Optional[ai.AuthCheck], error) {
			input.Credential.Env["account"] = "mutated"
			return ai.Absent[ai.AuthCheck](), callbackErr
		},
	}
	models := ai.CreateModels(ai.CreateModelsOptions{Credentials: credentials})
	models.SetProvider(ai.CreateProvider(ai.CreateProviderOptions{
		ID:   "isolated-check",
		Auth: ai.ProviderAuth{APIKey: &auth},
		API:  ai.SingleProviderAPI(ai.NewStubProviderStreams()),
	}))

	if _, err := models.CheckAuth(ctx, "isolated-check"); !errors.Is(err, callbackErr) {
		t.Fatalf("CheckAuth() error = %v, want wrapped callback error", err)
	}
	stored, err := credentials.Read(ctx, "isolated-check", ai.AuthOperationOptions{})
	if err != nil {
		t.Fatal(err)
	}
	credential, ok := stored.(ai.APIKeyCredential)
	if !ok || credential.Env["account"] != "stored" {
		t.Fatalf("stored credential after failed Check = %#v, want original env", stored)
	}
}

type modelsTestInteraction struct{}

func (modelsTestInteraction) Prompt(context.Context, ai.AuthPrompt) (string, error) {
	return "unused", nil
}

func (modelsTestInteraction) Notify(ai.AuthEvent) {}

func TestModelsResolveLoginAndLogoutDelegateToAuthOwnership(t *testing.T) {
	ctx := context.Background()
	credentials := ai.NewInMemoryCredentialStore()
	apiKey := ai.APIKeyAuth{
		Name: "Test key",
		Login: func(context.Context, ai.ProviderAuthInteraction) (ai.APIKeyCredential, error) {
			return ai.APIKeyCredential{Type: ai.AuthTypeAPIKey, Key: ai.Some("logged-in")}, nil
		},
		Resolve: func(_ context.Context, input ai.APIKeyResolveInput) (ai.Optional[ai.AuthResult], error) {
			if input.Credential == nil {
				return ai.Absent[ai.AuthResult](), nil
			}
			key, _ := input.Credential.Key.Value()
			return ai.Some(ai.AuthResult{
				Auth: ai.ModelAuth{
					APIKey:  ai.Some(key),
					Headers: ai.ProviderHeaders{"Authorization": stringPointer("Bearer " + key), "x-shared": stringPointer("auth")},
				},
				Source: ai.Some("stored"),
			}), nil
		},
	}
	models := ai.CreateModels(ai.CreateModelsOptions{Credentials: credentials})
	models.SetProvider(ai.CreateProvider(ai.CreateProviderOptions{
		ID:   "owned",
		Auth: ai.ProviderAuth{APIKey: &apiKey},
		API:  ai.SingleProviderAPI(ai.NewStubProviderStreams()),
	}))

	credential, err := models.Login(ctx, "owned", ai.AuthTypeAPIKey, modelsTestInteraction{})
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if keyCredential, ok := credential.(ai.APIKeyCredential); !ok {
		t.Fatalf("Login() credential = %T, want APIKeyCredential", credential)
	} else if key, _ := keyCredential.Key.Value(); key != "logged-in" {
		t.Fatalf("Login() key = %q, want logged-in", key)
	}

	providerAuth, err := models.GetProviderAuth(ctx, "owned")
	if err != nil {
		t.Fatalf("GetProviderAuth() error = %v", err)
	}
	resolved, ok := providerAuth.Value()
	if !ok || resolved.Auth.Headers["x-shared"] == nil || *resolved.Auth.Headers["x-shared"] != "auth" {
		t.Fatalf("GetProviderAuth() = (%+v, %v), want resolved provider auth", resolved, ok)
	}
	model := ai.Model{Provider: "owned", Headers: map[string]string{"X-Shared": "model", "X-Model": "yes"}}
	modelAuth, err := models.GetModelAuth(ctx, model)
	if err != nil {
		t.Fatalf("GetModelAuth() error = %v", err)
	}
	resolvedModel, ok := modelAuth.Value()
	if !ok || resolvedModel.Auth.Headers["X-Shared"] == nil || *resolvedModel.Auth.Headers["X-Shared"] != "model" || resolvedModel.Auth.Headers["X-Model"] == nil {
		t.Fatalf("GetModelAuth() headers = %#v, want case-insensitive model override", resolvedModel.Auth.Headers)
	}
	if _, duplicate := resolvedModel.Auth.Headers["x-shared"]; duplicate {
		t.Fatalf("GetModelAuth() kept case-insensitive duplicate: %#v", resolvedModel.Auth.Headers)
	}

	if err := models.Logout(ctx, "owned"); err != nil {
		t.Fatalf("Logout() error = %v", err)
	}
	if stored, err := credentials.Read(ctx, "owned", ai.AuthOperationOptions{}); err != nil || stored != nil {
		t.Fatalf("credential after Logout = (%#v, %v), want nil", stored, err)
	}

	_, err = models.Login(ctx, "unknown", ai.AuthTypeAPIKey, modelsTestInteraction{})
	var modelsErr *ai.ModelsError
	if !errors.As(err, &modelsErr) || modelsErr.Code != ai.ModelsErrorCodeProvider {
		t.Fatalf("Login(unknown) error = %v, want provider ModelsError", err)
	}
}

func TestModelsLoginRejectsMismatchedCredentialDiscriminator(t *testing.T) {
	tests := []struct {
		name     string
		authType ai.AuthType
		auth     ai.ProviderAuth
	}{
		{
			name:     "api key",
			authType: ai.AuthTypeAPIKey,
			auth: ai.ProviderAuth{APIKey: &ai.APIKeyAuth{
				Name: "API key",
				Login: func(context.Context, ai.ProviderAuthInteraction) (ai.APIKeyCredential, error) {
					return ai.APIKeyCredential{Type: ai.AuthTypeOAuth, Key: ai.Some("secret")}, nil
				},
			}},
		},
		{
			name:     "oauth",
			authType: ai.AuthTypeOAuth,
			auth: ai.ProviderAuth{OAuth: &ai.OAuthAuth{
				Name: "OAuth",
				Login: func(context.Context, ai.ProviderAuthInteraction) (ai.OAuthCredential, error) {
					return ai.OAuthCredential{
						OAuthCredentials: ai.OAuthCredentials{Access: "secret", Expires: time.Now().Add(time.Hour).UnixMilli()},
						Type:             ai.AuthTypeAPIKey,
					}, nil
				},
			}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			credentials := ai.NewInMemoryCredentialStore()
			models := ai.CreateModels(ai.CreateModelsOptions{Credentials: credentials})
			models.SetProvider(ai.CreateProvider(ai.CreateProviderOptions{
				ID:   "mismatched-login",
				Auth: test.auth,
				API:  ai.SingleProviderAPI(ai.NewStubProviderStreams()),
			}))

			_, err := models.Login(ctx, "mismatched-login", test.authType, modelsTestInteraction{})
			var modelsErr *ai.ModelsError
			if !errors.As(err, &modelsErr) || modelsErr.Code != ai.ModelsErrorCodeAuth {
				t.Fatalf("Login() error = %v, want auth ModelsError", err)
			}
			if stored, readErr := credentials.Read(ctx, "mismatched-login", ai.AuthOperationOptions{}); readErr != nil || stored != nil {
				t.Fatalf("stored credential = (%#v, %v), want nil after rejected login", stored, readErr)
			}
		})
	}
}

func TestModelsStreamSimpleLazilyAppliesAuthWithoutErasingRequestSeams(t *testing.T) {
	resolveStarted := make(chan struct{})
	continueResolve := make(chan struct{})
	apiKey := ai.APIKeyAuth{
		Name: "Scoped key",
		Resolve: func(_ context.Context, input ai.APIKeyResolveInput) (ai.Optional[ai.AuthResult], error) {
			close(resolveStarted)
			<-continueResolve
			if input.Credential == nil {
				return ai.Absent[ai.AuthResult](), nil
			}
			key, _ := input.Credential.Key.Value()
			if key != "request-key" || input.Credential.Env["SHARED"] != "request" {
				return ai.Absent[ai.AuthResult](), errors.New("request auth overrides were not forwarded")
			}
			return ai.Some(ai.AuthResult{
				Auth: ai.ModelAuth{
					APIKey:  ai.Some("resolved-key"),
					BaseURL: ai.Some("https://auth.test/v1"),
					Headers: ai.ProviderHeaders{"Authorization": stringPointer("Bearer resolved"), "X-Shared": stringPointer("auth")},
				},
				Env: ai.ProviderEnv{"AUTH": "yes", "SHARED": "auth"},
			}), nil
		},
	}

	var capturedModel ai.Model
	var capturedOptions ai.SimpleStreamOptions
	providerCalled := make(chan struct{})
	streams := ai.ProviderStreams{
		Stream: func(context.Context, ai.Model, ai.Context, ai.StreamOptions) *ai.AssistantMessageEventStream {
			t.Fatal("unexpected Stream call")
			return nil
		},
		StreamSimple: func(_ context.Context, model ai.Model, _ ai.Context, options ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
			capturedModel = model
			capturedOptions = options
			close(providerCalled)
			return completedModelsTestStream(model)
		},
	}
	models := ai.CreateModels()
	models.SetProvider(ai.CreateProvider(ai.CreateProviderOptions{
		ID:   "request-provider",
		Auth: ai.ProviderAuth{APIKey: &apiKey},
		API:  ai.SingleProviderAPI(streams),
	}))
	model := ai.Model{
		ID:       "request-model",
		API:      ai.API("request-api"),
		Provider: "request-provider",
		BaseURL:  "https://catalog.test/v1",
		Headers: map[string]string{
			"x-model":  "model",
			"x-shared": "model",
		},
	}

	fetch := func(context.Context, ai.FetchRequest) (ai.FetchResponse, error) { return ai.FetchResponse{}, nil }
	payloadHook := func(context.Context, ai.JSONValue, ai.Model) (ai.PayloadHookResult, error) {
		return ai.PayloadHookResult{}, nil
	}
	responseHook := func(context.Context, ai.ProviderResponse, ai.Model) error { return nil }
	requestKey := "request-key"
	transforms := 0
	options := ai.ModelsSimpleStreamOptions{
		SimpleStreamOptions: ai.SimpleStreamOptions{StreamOptions: ai.StreamOptions{ProviderRequestOptions: ai.ProviderRequestOptions{
			TelemetryContext: telemetry.NOOPTelemetryContext,
			APIKey:           &requestKey,
			Fetch:            fetch,
			Env:              ai.ProviderEnv{"SHARED": "request", "REQUEST": "yes"},
			OnPayload:        payloadHook,
			OnResponse:       responseHook,
			Headers: ai.ProviderHeaders{
				"authorization": stringPointer("Explicit token"),
				"X-Model":       nil,
				"X-Request":     stringPointer("yes"),
			},
		}}},
		ModelsRequestTransforms: ai.ModelsRequestTransforms{TransformHeaders: func(_ context.Context, headers ai.ProviderHeaders) (ai.ProviderHeaders, error) {
			transforms++
			if headers["authorization"] == nil || *headers["authorization"] != "Explicit token" {
				return nil, errors.New("request header did not override auth case-insensitively")
			}
			if value, exists := headers["X-Model"]; !exists || value != nil {
				return nil, errors.New("request header deletion was not preserved")
			}
			if _, duplicate := headers["Authorization"]; duplicate {
				return nil, errors.New("case-insensitive auth header duplicate remains")
			}
			if headers["x-shared"] == nil || *headers["x-shared"] != "model" {
				return nil, errors.New("model header did not override auth header")
			}
			headers["X-Transformed"] = stringPointer("yes")
			return headers, nil
		}},
	}

	stream := models.StreamSimple(context.Background(), model, ai.Context{}, options)
	select {
	case <-resolveStarted:
	case <-time.After(time.Second):
		t.Fatal("auth resolution did not start asynchronously")
	}
	select {
	case <-providerCalled:
		t.Fatal("provider called before auth resolution completed")
	default:
	}
	close(continueResolve)
	result, err := stream.Result(context.Background())
	if err != nil || result.StopReason != ai.StopReasonStop {
		t.Fatalf("stream.Result() = (%+v, %v), want successful outcome", result, err)
	}

	if transforms != 1 {
		t.Fatalf("TransformHeaders calls = %d, want 1", transforms)
	}
	if capturedModel.BaseURL != "https://auth.test/v1" || model.BaseURL != "https://catalog.test/v1" {
		t.Fatalf("request/catalog base URLs = (%q, %q), want temporary auth override", capturedModel.BaseURL, model.BaseURL)
	}
	if capturedOptions.APIKey == nil || *capturedOptions.APIKey != "request-key" {
		t.Fatalf("provider APIKey = %#v, want explicit request key", capturedOptions.APIKey)
	}
	if capturedOptions.Env["AUTH"] != "yes" || capturedOptions.Env["SHARED"] != "request" || capturedOptions.Env["REQUEST"] != "yes" {
		t.Fatalf("provider Env = %#v, want resolved plus request override", capturedOptions.Env)
	}
	if capturedOptions.Headers["X-Transformed"] == nil || *capturedOptions.Headers["X-Transformed"] != "yes" {
		t.Fatalf("provider Headers = %#v, want transformed headers", capturedOptions.Headers)
	}
	if reflect.ValueOf(capturedOptions.Fetch).Pointer() != reflect.ValueOf(fetch).Pointer() ||
		reflect.ValueOf(capturedOptions.OnPayload).Pointer() != reflect.ValueOf(payloadHook).Pointer() ||
		reflect.ValueOf(capturedOptions.OnResponse).Pointer() != reflect.ValueOf(responseHook).Pointer() ||
		capturedOptions.TelemetryContext != telemetry.NOOPTelemetryContext {
		t.Fatal("provider request lost Fetch/hooks/telemetry function values")
	}
}

func TestModelsUnknownProviderBecomesTerminalStreamOutcome(t *testing.T) {
	models := ai.CreateModels()
	model := ai.Model{ID: "missing-model", Provider: "missing-provider"}
	stream := models.StreamSimple(context.Background(), model, ai.Context{})
	result, err := stream.Result(context.Background())
	if err != nil {
		t.Fatalf("Result() Go error = %v, want terminal provider outcome", err)
	}
	message, ok := result.ErrorMessage.Value()
	if result.StopReason != ai.StopReasonError || !ok || message == "" {
		t.Fatalf("Result() = %+v, want terminal error message", result)
	}
}

func TestModelsDeferredDelegatesAndPreservesCapabilityFailures(t *testing.T) {
	ctx := context.Background()
	auth := ai.APIKeyAuth{
		Name: "Configured",
		Resolve: func(context.Context, ai.APIKeyResolveInput) (ai.Optional[ai.AuthResult], error) {
			return ai.Some(ai.AuthResult{Auth: ai.ModelAuth{APIKey: ai.Some("key")}}), nil
		},
	}
	model := ai.Model{ID: "deferred-model", API: "deferred-api", Provider: "deferred"}
	handle := ai.DeferredHandle{Provider: "deferred", ModelID: model.ID, API: model.API, ID: "job-1"}
	fetchCalls := 0
	cancelCalls := 0
	streams := ai.ProviderStreams{
		Stream: func(context.Context, ai.Model, ai.Context, ai.StreamOptions) *ai.AssistantMessageEventStream {
			return completedModelsTestStream(model)
		},
		StreamSimple: func(context.Context, ai.Model, ai.Context, ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
			return completedModelsTestStream(model)
		},
		FetchDeferred: func(_ context.Context, gotModel ai.Model, gotHandle ai.DeferredHandle, options ai.DeferredFetchOptions) (*ai.AssistantMessageEventStream, error) {
			fetchCalls++
			if gotModel.ID != model.ID || gotHandle.ID != handle.ID || options.APIKey == nil || *options.APIKey != "key" {
				return nil, errors.New("deferred fetch inputs were not authenticated and forwarded")
			}
			return completedModelsTestStream(gotModel), nil
		},
		CancelDeferred: func(_ context.Context, gotModel ai.Model, gotHandle ai.DeferredHandle, options ai.DeferredCancelOptions) error {
			cancelCalls++
			if gotModel.ID != model.ID || gotHandle.ID != handle.ID || options.APIKey == nil || *options.APIKey != "key" {
				return errors.New("deferred cancel inputs were not authenticated and forwarded")
			}
			return nil
		},
	}
	models := ai.CreateModels()
	models.SetProvider(ai.CreateProvider(ai.CreateProviderOptions{ID: "deferred", Auth: ai.ProviderAuth{APIKey: &auth}, API: ai.SingleProviderAPI(streams)}))

	result, err := models.FetchDeferred(ctx, model, handle)
	if err != nil || result.Model != model.ID || result.StopReason != ai.StopReasonStop || fetchCalls != 1 {
		t.Fatalf("FetchDeferred() = (%+v, %v), calls=%d; want final assistant result", result, err, fetchCalls)
	}
	if err := models.CancelDeferred(ctx, model, handle); err != nil || cancelCalls != 1 {
		t.Fatalf("CancelDeferred() error = %v, calls=%d; want one successful delegation", err, cancelCalls)
	}

	unsupported := ai.CreateModels()
	unsupported.SetProvider(ai.CreateProvider(ai.CreateProviderOptions{
		ID:   "deferred",
		Auth: ai.ProviderAuth{APIKey: &auth},
		API: ai.SingleProviderAPI(ai.ProviderStreams{
			Stream:       streams.Stream,
			StreamSimple: streams.StreamSimple,
		}),
	}))
	_, err = unsupported.FetchDeferred(ctx, model, handle)
	var modelsErr *ai.ModelsError
	if !errors.As(err, &modelsErr) || modelsErr.Code != ai.ModelsErrorCodeProvider {
		t.Fatalf("unsupported FetchDeferred error = %v, want provider ModelsError", err)
	}

	stubbed := ai.CreateModels()
	stubbed.SetProvider(ai.CreateProvider(ai.CreateProviderOptions{
		ID:   "deferred",
		Auth: ai.ProviderAuth{APIKey: &auth},
		API:  ai.SingleProviderAPI(ai.NewStubProviderStreams()),
	}))
	if _, err := stubbed.FetchDeferred(ctx, model, handle); !errors.Is(err, ai.ErrNotImplemented) {
		t.Fatalf("stub FetchDeferred error = %v, want ErrNotImplemented", err)
	}
}

func TestModelsRefreshRunsCacheThenNetworkWithOptionalForce(t *testing.T) {
	ctx := context.Background()
	credentials := ai.NewInMemoryCredentialStore()
	_, err := credentials.Modify(ctx, "dynamic", func(context.Context, ai.Credential) (ai.Credential, error) {
		return ai.APIKeyCredential{Type: ai.AuthTypeAPIKey, Key: ai.Some("stored-key")}, nil
	}, ai.AuthOperationOptions{})
	if err != nil {
		t.Fatal(err)
	}
	store := ai.NewInMemoryModelsStore()
	if err := store.Write(ctx, "dynamic", ai.ModelsStoreEntry{Models: []ai.Model{{ID: "cached", Provider: "dynamic"}}}); err != nil {
		t.Fatal(err)
	}

	apiKey := ai.APIKeyAuth{
		Name: "Stored key",
		Resolve: func(context.Context, ai.APIKeyResolveInput) (ai.Optional[ai.AuthResult], error) {
			return ai.Some(ai.AuthResult{Auth: ai.ModelAuth{APIKey: ai.Some("stored-key")}}), nil
		},
	}
	phases := make([]string, 0, 2)
	provider := &modelsRefreshTestProvider{id: "dynamic", auth: ai.ProviderAuth{APIKey: &apiKey}}
	provider.refresh = func(refresh ai.RefreshModelsContext) error {
		if refresh.Context == nil {
			return errors.New("refresh context is nil")
		}
		if refresh.Stored == nil || len(refresh.Stored.Models) != 1 || refresh.Stored.Models[0].ID != "cached" {
			return errors.New("stored catalog snapshot missing")
		}
		refresh.Stored.Models[0].ID = "mutated-by-provider"
		if refresh.AllowNetwork {
			phases = append(phases, "network")
			force, ok := refresh.Force.Value()
			if !ok || force {
				return errors.New("network force did not preserve explicit false")
			}
			credential, ok := refresh.Credential.(ai.APIKeyCredential)
			if !ok {
				return errors.New("network credential is not APIKeyCredential")
			}
			key, _ := credential.Key.Value()
			if key != "stored-key" {
				return errors.New("network credential key mismatch")
			}
			return nil
		}
		phases = append(phases, "cache")
		if refresh.Force.IsSet() {
			return errors.New("cache force must be absent")
		}
		return nil
	}

	models := ai.CreateModels(ai.CreateModelsOptions{Credentials: credentials, ModelsStore: store})
	models.SetProvider(provider)
	models.SetProvider(&modelsRefreshTestProvider{id: "static"})
	result := models.Refresh(ctx, ai.ModelsRefreshOptions{Providers: []ai.ProviderID{"dynamic", "static", "unknown"}, Force: ai.Some(false)})
	if result.Aborted || len(result.Errors) != 0 {
		t.Fatalf("Refresh() = %+v, want non-aborted success", result)
	}
	if want := []string{"cache", "network"}; !reflect.DeepEqual(phases, want) {
		t.Fatalf("refresh phases = %v, want %v", phases, want)
	}
	stored, ok, err := store.Read(ctx, "dynamic")
	if err != nil || !ok || stored.Models[0].ID != "cached" {
		t.Fatalf("stored catalog after provider mutation = (%+v, %v, %v), want isolated cached snapshot", stored, ok, err)
	}
}

func TestModelsRefreshDoesNotExposeStoredOAuthCredentialToRefreshCallback(t *testing.T) {
	ctx := context.Background()
	credentials := ai.NewInMemoryCredentialStore()
	if _, err := credentials.Modify(ctx, "isolated-model-refresh", func(context.Context, ai.Credential) (ai.Credential, error) {
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
	}
	provider := &modelsRefreshTestProvider{id: "isolated-model-refresh", auth: ai.ProviderAuth{OAuth: &oauth}}
	provider.refresh = func(ai.RefreshModelsContext) error { return nil }
	models := ai.CreateModels(ai.CreateModelsOptions{Credentials: credentials})
	models.SetProvider(provider)

	result := models.Refresh(ctx, ai.ModelsRefreshOptions{Providers: []ai.ProviderID{"isolated-model-refresh"}})
	if !errors.Is(result.Errors["isolated-model-refresh"], callbackErr) {
		t.Fatalf("Refresh() error = %v, want wrapped callback error", result.Errors["isolated-model-refresh"])
	}
	stored, err := credentials.Read(ctx, "isolated-model-refresh", ai.AuthOperationOptions{})
	if err != nil {
		t.Fatal(err)
	}
	credential, ok := stored.(ai.OAuthCredential)
	if !ok || string(credential.Extra["account"]) != `"stored"` {
		t.Fatalf("stored credential after failed Refresh callback = %#v, want original extra", stored)
	}
}

func TestModelsRefreshRejectsMismatchedOAuthCredentialDiscriminator(t *testing.T) {
	ctx := context.Background()
	credentials := ai.NewInMemoryCredentialStore()
	if _, err := credentials.Modify(ctx, "mismatched-model-refresh", func(context.Context, ai.Credential) (ai.Credential, error) {
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
				OAuthCredentials: ai.OAuthCredentials{Refresh: "rotated", Access: "new", Expires: time.Now().Add(time.Hour).UnixMilli()},
				Type:             ai.AuthTypeAPIKey,
			}, nil
		},
	}
	provider := &modelsRefreshTestProvider{id: "mismatched-model-refresh", auth: ai.ProviderAuth{OAuth: &oauth}}
	provider.refresh = func(ai.RefreshModelsContext) error { return nil }
	models := ai.CreateModels(ai.CreateModelsOptions{Credentials: credentials})
	models.SetProvider(provider)

	result := models.Refresh(ctx, ai.ModelsRefreshOptions{Providers: []ai.ProviderID{"mismatched-model-refresh"}})
	var modelsErr *ai.ModelsError
	if !errors.As(result.Errors["mismatched-model-refresh"], &modelsErr) || modelsErr.Code != ai.ModelsErrorCodeOAuth {
		t.Fatalf("Refresh() error = %v, want oauth ModelsError", result.Errors["mismatched-model-refresh"])
	}
	stored, err := credentials.Read(ctx, "mismatched-model-refresh", ai.AuthOperationOptions{})
	if err != nil {
		t.Fatal(err)
	}
	credential, ok := stored.(ai.OAuthCredential)
	if !ok || credential.Access != "old" || credential.Type != ai.AuthTypeOAuth {
		t.Fatalf("stored credential = %#v, want original OAuth credential", stored)
	}
}

func TestModelsRefreshLatestGenerationRejectsLatePublication(t *testing.T) {
	ctx := context.Background()
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	var networkCalls int
	var state string
	var published []bool
	var stateMu sync.Mutex
	auth := ai.APIKeyAuth{
		Name: "Configured",
		Resolve: func(context.Context, ai.APIKeyResolveInput) (ai.Optional[ai.AuthResult], error) {
			return ai.Some(ai.AuthResult{Auth: ai.ModelAuth{APIKey: ai.Some("key")}}), nil
		},
	}
	provider := &modelsRefreshTestProvider{id: "latest", auth: ai.ProviderAuth{APIKey: &auth}}
	provider.refresh = func(refresh ai.RefreshModelsContext) error {
		if !refresh.AllowNetwork {
			return nil
		}
		stateMu.Lock()
		networkCalls++
		call := networkCalls
		stateMu.Unlock()
		if call == 1 {
			close(firstStarted)
			<-releaseFirst
		}
		value := fmt.Sprintf("generation-%d", call)
		ok, err := refresh.Publish(ai.ModelsPublication{Update: func() {
			stateMu.Lock()
			state = value
			stateMu.Unlock()
		}})
		stateMu.Lock()
		published = append(published, ok)
		stateMu.Unlock()
		return err
	}
	models := ai.CreateModels()
	models.SetProvider(provider)

	firstDone := make(chan ai.ModelsRefreshResult, 1)
	go func() {
		firstDone <- models.Refresh(ctx, ai.ModelsRefreshOptions{Providers: []ai.ProviderID{"latest"}})
	}()
	select {
	case <-firstStarted:
	case <-time.After(time.Second):
		t.Fatal("first network refresh did not start")
	}
	second := models.Refresh(ctx, ai.ModelsRefreshOptions{Providers: []ai.ProviderID{"latest"}})
	if second.Aborted || len(second.Errors) != 0 {
		t.Fatalf("second Refresh() = %+v, want success", second)
	}
	select {
	case first := <-firstDone:
		if first.Aborted || len(first.Errors) != 0 {
			t.Fatalf("superseded Refresh() = %+v, want non-caller-aborted result", first)
		}
	case <-time.After(time.Second):
		t.Fatal("superseded refresh kept waiting for a non-cooperative provider")
	}
	close(releaseFirst)
	deadline := time.Now().Add(time.Second)
	for {
		stateMu.Lock()
		done := len(published) == 2
		stateMu.Unlock()
		if done {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("late stale publication did not complete")
		}
		time.Sleep(time.Millisecond)
	}

	stateMu.Lock()
	defer stateMu.Unlock()
	if state != "generation-2" {
		t.Fatalf("published state = %q, want generation-2", state)
	}
	if want := []bool{true, false}; !reflect.DeepEqual(published, want) {
		t.Fatalf("publication results = %v, want %v (new then stale)", published, want)
	}
}

func TestModelsRegistryMutationsSupersedeRefreshPublication(t *testing.T) {
	mutations := []struct {
		name   string
		mutate func(ai.MutableModels)
	}{
		{name: "set", mutate: func(models ai.MutableModels) { models.SetProvider(&modelsRefreshTestProvider{id: "dynamic"}) }},
		{name: "delete", mutate: func(models ai.MutableModels) { models.DeleteProvider("dynamic") }},
		{name: "clear", mutate: func(models ai.MutableModels) { models.ClearProviders() }},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			started := make(chan struct{})
			release := make(chan struct{})
			publicationDone := make(chan struct{})
			updated := false
			published := true
			auth := ai.APIKeyAuth{
				Name: "Configured",
				Resolve: func(context.Context, ai.APIKeyResolveInput) (ai.Optional[ai.AuthResult], error) {
					return ai.Some(ai.AuthResult{Auth: ai.ModelAuth{APIKey: ai.Some("key")}}), nil
				},
			}
			provider := &modelsRefreshTestProvider{id: "dynamic", auth: ai.ProviderAuth{APIKey: &auth}}
			provider.refresh = func(refresh ai.RefreshModelsContext) error {
				if !refresh.AllowNetwork {
					return nil
				}
				close(started)
				<-release
				var err error
				published, err = refresh.Publish(ai.ModelsPublication{Update: func() { updated = true }})
				close(publicationDone)
				return err
			}
			models := ai.CreateModels()
			models.SetProvider(provider)
			done := make(chan ai.ModelsRefreshResult, 1)
			go func() { done <- models.Refresh(context.Background()) }()
			select {
			case <-started:
			case <-time.After(time.Second):
				t.Fatal("network refresh did not start")
			}
			mutation.mutate(models)
			close(release)
			select {
			case result := <-done:
				if result.Aborted || len(result.Errors) != 0 {
					t.Fatalf("Refresh() = %+v, want clean provider-local supersession", result)
				}
			case <-time.After(time.Second):
				t.Fatal("superseded refresh did not finish")
			}
			select {
			case <-publicationDone:
			case <-time.After(time.Second):
				t.Fatal("late stale publication did not return")
			}
			if published || updated {
				t.Fatalf("stale publication = %v, update = %v; want rejected/no update", published, updated)
			}
		})
	}
}

func TestModelsRegistryMutationsSupersedeSnapshottedProviderBeforeRefreshStarts(t *testing.T) {
	mutations := []struct {
		name   string
		mutate func(ai.MutableModels)
	}{
		{name: "set", mutate: func(models ai.MutableModels) { models.SetProvider(&modelsRefreshTestProvider{id: "dynamic"}) }},
		{name: "delete", mutate: func(models ai.MutableModels) { models.DeleteProvider("dynamic") }},
		{name: "clear", mutate: func(models ai.MutableModels) { models.ClearProviders() }},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			snapshotReached := make(chan struct{})
			releaseSnapshot := make(chan struct{})
			refreshStarted := make(chan struct{}, 1)
			provider := &modelsRefreshTestProvider{id: "dynamic"}
			provider.supports = func() bool {
				close(snapshotReached)
				<-releaseSnapshot
				return true
			}
			provider.refresh = func(ai.RefreshModelsContext) error {
				refreshStarted <- struct{}{}
				return nil
			}

			models := ai.CreateModels()
			models.SetProvider(provider)
			refreshDone := make(chan ai.ModelsRefreshResult, 1)
			go func() { refreshDone <- models.Refresh(context.Background()) }()
			select {
			case <-snapshotReached:
			case <-time.After(time.Second):
				t.Fatal("Refresh did not snapshot the provider")
			}

			mutation.mutate(models)
			close(releaseSnapshot)
			select {
			case result := <-refreshDone:
				if result.Aborted || len(result.Errors) != 0 {
					t.Fatalf("Refresh() = %+v, want clean supersession", result)
				}
			case <-time.After(time.Second):
				t.Fatal("Refresh did not finish after registry mutation")
			}
			select {
			case <-refreshStarted:
				t.Fatal("snapshotted stale provider started after registry mutation")
			default:
			}
		})
	}
}

func TestModelsRegistryMutationsDoNotWaitForInFlightPublicationUpdate(t *testing.T) {
	mutations := []struct {
		name   string
		mutate func(ai.MutableModels)
	}{
		{name: "set", mutate: func(models ai.MutableModels) { models.SetProvider(&modelsRefreshTestProvider{id: "dynamic"}) }},
		{name: "delete", mutate: func(models ai.MutableModels) { models.DeleteProvider("dynamic") }},
		{name: "clear", mutate: func(models ai.MutableModels) { models.ClearProviders() }},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			updateStarted := make(chan struct{})
			releaseUpdate := make(chan struct{})
			var releaseOnce sync.Once
			t.Cleanup(func() { releaseOnce.Do(func() { close(releaseUpdate) }) })

			auth := ai.APIKeyAuth{
				Name: "Configured",
				Resolve: func(context.Context, ai.APIKeyResolveInput) (ai.Optional[ai.AuthResult], error) {
					return ai.Some(ai.AuthResult{Auth: ai.ModelAuth{APIKey: ai.Some("key")}}), nil
				},
			}
			provider := &modelsRefreshTestProvider{id: "dynamic", auth: ai.ProviderAuth{APIKey: &auth}}
			provider.refresh = func(refresh ai.RefreshModelsContext) error {
				if !refresh.AllowNetwork {
					return nil
				}
				published, err := refresh.Publish(ai.ModelsPublication{Update: func() {
					close(updateStarted)
					<-releaseUpdate
				}})
				if err != nil {
					return err
				}
				if !published {
					return errors.New("in-flight publication was rejected")
				}
				return nil
			}

			models := ai.CreateModels()
			models.SetProvider(provider)
			refreshDone := make(chan ai.ModelsRefreshResult, 1)
			go func() { refreshDone <- models.Refresh(context.Background()) }()
			select {
			case <-updateStarted:
			case <-time.After(time.Second):
				t.Fatal("publication update did not start")
			}

			mutationStarted := make(chan struct{})
			mutationDone := make(chan struct{})
			go func() {
				close(mutationStarted)
				mutation.mutate(models)
				close(mutationDone)
			}()
			<-mutationStarted
			select {
			case <-mutationDone:
			case <-time.After(time.Second):
				t.Fatal("registry mutation waited for an in-flight publication update")
			}

			releaseOnce.Do(func() { close(releaseUpdate) })
			select {
			case result := <-refreshDone:
				if result.Aborted || len(result.Errors) != 0 {
					t.Fatalf("Refresh() = %+v, want committed publication before mutation", result)
				}
			case <-time.After(time.Second):
				t.Fatal("Refresh did not finish after publication commit")
			}
		})
	}
}

func TestModelsRefreshCallerCancellationReturnsWithoutProviderError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	release := make(chan struct{})
	provider := &modelsRefreshTestProvider{id: "non-cooperative"}
	provider.refresh = func(ai.RefreshModelsContext) error {
		close(started)
		<-release
		return errors.New("late provider failure")
	}
	models := ai.CreateModels()
	models.SetProvider(provider)
	done := make(chan ai.ModelsRefreshResult, 1)
	go func() { done <- models.Refresh(ctx, ai.ModelsRefreshOptions{AllowNetwork: ai.Some(false)}) }()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("provider refresh did not start")
	}
	cancel()
	select {
	case result := <-done:
		if !result.Aborted || len(result.Errors) != 0 {
			t.Fatalf("Refresh(canceled) = %+v, want Aborted with no provider errors", result)
		}
	case <-time.After(time.Second):
		t.Fatal("Refresh did not stop waiting for non-cooperative provider")
	}
	close(release)
}

func TestModelsRefreshReportsAlreadyCanceledCallerWithNoRefreshableProviders(t *testing.T) {
	models := ai.CreateModels()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result := models.Refresh(ctx)
	if !result.Aborted || len(result.Errors) != 0 {
		t.Fatalf("Refresh(canceled, no providers) = %+v, want Aborted with no errors", result)
	}
}

func TestModelsRejectForgedStoredCredentialBeforeCollectionCallbacks(t *testing.T) {
	store := ai.NewInMemoryCredentialStore()
	if _, err := store.Modify(context.Background(), "forged-models", func(context.Context, ai.Credential) (ai.Credential, error) {
		return ai.APIKeyCredential{Type: ai.AuthTypeOAuth, Key: ai.Some("must-not-resolve")}, nil
	}, ai.AuthOperationOptions{}); err != nil {
		t.Fatalf("seed credential store: %v", err)
	}
	apiKey := ai.APIKeyAuth{
		Name: "Configured",
		Resolve: func(context.Context, ai.APIKeyResolveInput) (ai.Optional[ai.AuthResult], error) {
			t.Fatal("APIKeyAuth.Resolve called for forged stored credential")
			return ai.Absent[ai.AuthResult](), nil
		},
		Check: func(context.Context, ai.APIKeyCheckInput) (ai.Optional[ai.AuthCheck], error) {
			t.Fatal("APIKeyAuth.Check called for forged stored credential")
			return ai.Absent[ai.AuthCheck](), nil
		},
	}
	provider := &modelsRefreshTestProvider{id: "forged-models", auth: ai.ProviderAuth{APIKey: &apiKey}}
	provider.refresh = func(refresh ai.RefreshModelsContext) error {
		if refresh.AllowNetwork {
			t.Fatal("network RefreshModels called for forged stored credential")
		}
		return nil
	}
	models := ai.CreateModels(ai.CreateModelsOptions{Credentials: store})
	models.SetProvider(provider)

	if _, err := models.CheckAuth(context.Background(), provider.id); err == nil {
		t.Fatal("CheckAuth() error = nil, want forged discriminator rejection")
	}
	result := models.Refresh(context.Background())
	if err := result.Errors[provider.id]; err == nil {
		t.Fatalf("Refresh() = %+v, want forged discriminator rejection", result)
	}
}

func TestModelsRefreshSupersessionWinsOverSimultaneousProviderError(t *testing.T) {
	providerErr := errors.New("provider failed while being superseded")
	for iteration := 0; iteration < 1000; iteration++ {
		models := ai.CreateModels()
		provider := &modelsRefreshTestProvider{id: "dynamic"}
		provider.refresh = func(ai.RefreshModelsContext) error {
			models.SetProvider(&modelsRefreshTestProvider{id: "dynamic"})
			return providerErr
		}
		models.SetProvider(provider)

		result := models.Refresh(context.Background(), ai.ModelsRefreshOptions{AllowNetwork: ai.Some(false)})
		if result.Aborted || len(result.Errors) != 0 {
			t.Fatalf("iteration %d: Refresh() = %+v, want clean provider-local supersession", iteration, result)
		}
	}
}

func completedModelsTestStream(model ai.Model) *ai.AssistantMessageEventStream {
	message := ai.AssistantMessage{
		Role:       ai.MessageRoleAssistant,
		Content:    []ai.AssistantContent{},
		API:        model.API,
		Provider:   model.Provider,
		Model:      model.ID,
		StopReason: ai.StopReasonStop,
	}
	stream := ai.NewAssistantMessageEventStream()
	stream.Push(ai.AssistantMessageDoneEvent{Type: ai.AssistantMessageEventTypeDone, Reason: ai.StopReasonStop, Message: message})
	return stream
}

func providerIDs(providers []ai.Provider) []ai.ProviderID {
	ids := make([]ai.ProviderID, len(providers))
	for index, provider := range providers {
		ids[index] = provider.ID()
	}
	return ids
}

func modelIDs(models []ai.Model) []string {
	ids := make([]string, len(models))
	for index, model := range models {
		ids[index] = model.ID
	}
	return ids
}

func stringPointer(value string) *string { return &value }

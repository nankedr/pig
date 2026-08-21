package ai_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nankedr/pig/ai"
)

// Compile-time acceptance guard for Issue #39. The runtime returned by
// CreateModels must keep exposing the complete public surface: registry,
// auth/credential delegation, request dispatch, and refresh orchestration.
//
// If a method or refresh type is still missing, this file must fail to build
// loudly so the gap is visible before behavior quietly diverges.
type acceptanceMutableModelsSurface interface {
	GetProviders() []ai.Provider
	GetProvider(id ai.ProviderID) (ai.Provider, bool)
	GetModels(provider ...ai.ProviderID) []ai.Model
	GetModel(provider ai.ProviderID, id string) (ai.Model, bool)

	CheckAuth(ctx context.Context, provider ai.ProviderID, options ...ai.AuthOperationOptions) (ai.Optional[ai.AuthCheck], error)
	GetAvailable(ctx context.Context, provider ...ai.ProviderID) ([]ai.Model, error)
	GetProviderAuth(ctx context.Context, provider ai.ProviderID, overrides ...ai.AuthResolutionOverrides) (ai.Optional[ai.AuthResult], error)
	GetModelAuth(ctx context.Context, model ai.Model, overrides ...ai.AuthResolutionOverrides) (ai.Optional[ai.AuthResult], error)
	Login(ctx context.Context, provider ai.ProviderID, authType ai.AuthType, interaction ai.ProviderAuthInteraction) (ai.Credential, error)
	Logout(ctx context.Context, provider ai.ProviderID, options ...ai.AuthOperationOptions) error

	Stream(ctx context.Context, model ai.Model, input ai.Context, options ...ai.ModelsStreamOption) *ai.AssistantMessageEventStream
	Complete(ctx context.Context, model ai.Model, input ai.Context, options ...ai.ModelsStreamOption) (ai.AssistantMessage, error)
	StreamSimple(ctx context.Context, model ai.Model, input ai.Context, options ...ai.ModelsSimpleStreamOptions) *ai.AssistantMessageEventStream
	CompleteSimple(ctx context.Context, model ai.Model, input ai.Context, options ...ai.ModelsSimpleStreamOptions) (ai.AssistantMessage, error)
	FetchDeferred(ctx context.Context, model ai.Model, handle ai.DeferredHandle, options ...ai.ModelsDeferredFetchOptions) (ai.AssistantMessage, error)
	CancelDeferred(ctx context.Context, model ai.Model, handle ai.DeferredHandle, options ...ai.ModelsDeferredCancelOptions) error

	Refresh(ctx context.Context, options ...ai.ModelsRefreshOptions) ai.ModelsRefreshResult

	SetProvider(provider ai.Provider)
	DeleteProvider(id ai.ProviderID)
	ClearProviders()
}

var (
	_                                = ai.ModelsRefreshOptions{}
	_                                = ai.ModelsRefreshResult{}
	_ acceptanceMutableModelsSurface = ai.CreateModels()
)

func TestModelsContractStreamAndCompleteDispatchPreserveRequestHooksAndAuthOverlay(t *testing.T) {
	var models acceptanceMutableModelsSurface = ai.CreateModels()

	const providerID = ai.ProviderID("acceptance-stream")
	model := acceptanceModel(providerID, "stream-model")
	timeoutMS := int64(111)
	retries := 3
	sessionID := "session-acceptance"
	transport := ai.TransportAuto
	requestMetadata := map[string]json.RawMessage{"trace": json.RawMessage(`{"v":1}`)}

	fetchCalls := 0
	payloadCalls := 0
	responseCalls := 0
	streamCalls := 0

	options := ai.ModelsStreamOptions{
		StreamOptions: ai.StreamOptions{
			ProviderRequestOptions: ai.ProviderRequestOptions{
				Fetch: func(context.Context, ai.FetchRequest) (ai.FetchResponse, error) {
					fetchCalls++
					return ai.FetchResponse{Status: 204}, nil
				},
				OnPayload: func(context.Context, ai.JSONValue, ai.Model) (ai.PayloadHookResult, error) {
					payloadCalls++
					return ai.PayloadHookResult{}, nil
				},
				OnResponse: func(context.Context, ai.ProviderResponse, ai.Model) error {
					responseCalls++
					return nil
				},
				Env:        ai.ProviderEnv{"REQUEST_ENV": "request"},
				Headers:    ai.ProviderHeaders{"X-Shared": acceptanceStringPointer("request"), "X-Request": acceptanceStringPointer("request-only")},
				TimeoutMS:  &timeoutMS,
				MaxRetries: &retries,
			},
			SessionID: &sessionID,
			Transport: &transport,
			Metadata:  requestMetadata,
		},
	}

	provider := ai.CreateProvider(ai.CreateProviderOptions{
		ID: providerID,
		Auth: ai.ProviderAuth{
			APIKey: &ai.APIKeyAuth{
				Name: "Acceptance key",
				Resolve: func(context.Context, ai.APIKeyResolveInput) (ai.Optional[ai.AuthResult], error) {
					return ai.Some(ai.AuthResult{
						Auth: ai.ModelAuth{
							APIKey:  ai.Some("resolved-key"),
							BaseURL: ai.Some("https://resolved.example.test/v1"),
							Headers: ai.ProviderHeaders{"x-shared": acceptanceStringPointer("auth"), "x-auth": acceptanceStringPointer("auth-only")},
						},
						Env:    ai.ProviderEnv{"AUTH_ENV": "auth"},
						Source: ai.Some("ambient"),
					}), nil
				},
			},
		},
		Models: []ai.Model{model},
		API: ai.SingleProviderAPI(ai.ProviderStreams{
			Stream: func(ctx context.Context, got ai.Model, _ ai.Context, gotOptions ai.StreamOptions) *ai.AssistantMessageEventStream {
				streamCalls++
				acceptanceAssertResolvedDispatchState(t, got, model)
				acceptanceAssertRequestOptionsPreserved(t, ctx, got, gotOptions.ProviderRequestOptions, timeoutMS, retries)
				if gotOptions.SessionID != &sessionID {
					t.Fatalf("SessionID pointer = %p, want %p", gotOptions.SessionID, &sessionID)
				}
				if gotOptions.Transport != &transport {
					t.Fatalf("Transport pointer = %p, want %p", gotOptions.Transport, &transport)
				}
				if !reflect.DeepEqual(gotOptions.Metadata, requestMetadata) {
					t.Fatalf("Metadata = %#v, want %#v", gotOptions.Metadata, requestMetadata)
				}
				return acceptanceDoneStream(got, "stream-complete")
			},
		}),
	})
	models.SetProvider(provider)

	stream := models.Stream(context.Background(), model, ai.Context{}, options)
	result := acceptanceAwaitMessage(t, stream)
	if result.StopReason != ai.StopReasonStop || result.Model != model.ID {
		t.Fatalf("Stream().Result() = %#v, want terminal assistant message for %q", result, model.ID)
	}
	if streamCalls != 1 || fetchCalls != 1 || payloadCalls != 1 || responseCalls != 1 {
		t.Fatalf("stream path calls = stream %d/fetch %d/payload %d/response %d, want 1/1/1/1", streamCalls, fetchCalls, payloadCalls, responseCalls)
	}

	complete, err := models.Complete(context.Background(), model, ai.Context{}, options)
	if err != nil {
		t.Fatalf("Complete() error = %v, want nil structured outcome", err)
	}
	if complete.StopReason != ai.StopReasonStop || complete.Model != model.ID {
		t.Fatalf("Complete() = %#v, want terminal assistant message for %q", complete, model.ID)
	}
	if streamCalls != 2 || fetchCalls != 2 || payloadCalls != 2 || responseCalls != 2 {
		t.Fatalf("complete path calls = stream %d/fetch %d/payload %d/response %d, want 2/2/2/2", streamCalls, fetchCalls, payloadCalls, responseCalls)
	}
}

func TestModelsContractPreservesConcreteAPIOptionsThroughAuthOverlay(t *testing.T) {
	var models acceptanceMutableModelsSurface = ai.CreateModels()

	const providerID = ai.ProviderID("acceptance-typed-options")
	model := acceptanceModel(providerID, "typed-options-model")
	model.API = ai.APIOpenAIResponses
	sessionID := "typed-options-session"
	fetchCalls := 0
	streamCalls := 0

	provider := ai.CreateProvider(ai.CreateProviderOptions{
		ID: providerID,
		Auth: ai.ProviderAuth{APIKey: &ai.APIKeyAuth{
			Name: "Typed options key",
			Resolve: func(context.Context, ai.APIKeyResolveInput) (ai.Optional[ai.AuthResult], error) {
				return ai.Some(ai.AuthResult{Auth: ai.ModelAuth{APIKey: ai.Some("resolved-key")}}), nil
			},
		}},
		Models: []ai.Model{model},
		API: ai.SingleProviderAPI(ai.NewTypedProviderStreams(
			func(ctx context.Context, got ai.Model, _ ai.Context, options ai.OpenAIResponsesOptions) *ai.AssistantMessageEventStream {
				streamCalls++
				tier, ok := options.ServiceTier.Value()
				if !ok || tier != ai.OpenAIServiceTierPriority {
					t.Fatalf("ServiceTier = (%q, %t), want priority", tier, ok)
				}
				if options.SessionID != &sessionID {
					t.Fatalf("SessionID pointer = %p, want %p", options.SessionID, &sessionID)
				}
				if options.APIKey == nil || *options.APIKey != "resolved-key" {
					t.Fatalf("APIKey = %#v, want resolved key", options.APIKey)
				}
				if response, err := options.Fetch(ctx, ai.FetchRequest{}); err != nil || response.Status != 206 {
					t.Fatalf("Fetch() = (%#v, %v), want status 206", response, err)
				}
				return acceptanceDoneStream(got, "typed-options")
			},
		)),
	})
	models.SetProvider(provider)

	options := ai.ModelsAPIStreamOptions[ai.OpenAIResponsesOptions]{
		Options: ai.OpenAIResponsesOptions{
			StreamOptions: ai.StreamOptions{
				ProviderRequestOptions: ai.ProviderRequestOptions{Fetch: func(context.Context, ai.FetchRequest) (ai.FetchResponse, error) {
					fetchCalls++
					return ai.FetchResponse{Status: 206}, nil
				}},
				SessionID: &sessionID,
			},
			ServiceTier: ai.Some(ai.OpenAIServiceTierPriority),
		},
	}

	if result := acceptanceAwaitMessage(t, models.Stream(context.Background(), model, ai.Context{}, options)); result.StopReason != ai.StopReasonStop {
		t.Fatalf("Stream().Result() = %#v, want stop outcome", result)
	}
	if result, err := models.Complete(context.Background(), model, ai.Context{}, options); err != nil || result.StopReason != ai.StopReasonStop {
		t.Fatalf("Complete() = (%#v, %v), want stop outcome", result, err)
	}
	if streamCalls != 2 || fetchCalls != 2 {
		t.Fatalf("typed option calls = stream %d/fetch %d, want 2/2", streamCalls, fetchCalls)
	}
}

func TestModelsContractPromotesGenericOptionsForTypedProvider(t *testing.T) {
	for _, test := range []struct {
		name    string
		options []ai.ModelsStreamOption
	}{
		{name: "omitted"},
		{name: "generic", options: []ai.ModelsStreamOption{ai.ModelsStreamOptions{}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var models acceptanceMutableModelsSurface = ai.CreateModels()

			const providerID = ai.ProviderID("acceptance-typed-defaults")
			model := acceptanceModel(providerID, "typed-defaults-model")
			model.API = ai.APIOpenAIResponses
			received := make(chan ai.OpenAIResponsesOptions, 1)
			provider := ai.CreateProvider(ai.CreateProviderOptions{
				ID: providerID,
				Auth: ai.ProviderAuth{APIKey: &ai.APIKeyAuth{
					Name: "Typed defaults key",
					Resolve: func(context.Context, ai.APIKeyResolveInput) (ai.Optional[ai.AuthResult], error) {
						return ai.Some(ai.AuthResult{Auth: ai.ModelAuth{APIKey: ai.Some("resolved-key")}}), nil
					},
				}},
				Models: []ai.Model{model},
				API: ai.SingleProviderAPI(ai.NewTypedProviderStreams(
					func(_ context.Context, got ai.Model, _ ai.Context, options ai.OpenAIResponsesOptions) *ai.AssistantMessageEventStream {
						received <- options
						return acceptanceDoneStream(got, "typed-defaults")
					},
				)),
			})
			models.SetProvider(provider)

			result := acceptanceAwaitMessage(t, models.Stream(context.Background(), model, ai.Context{}, test.options...))
			if result.StopReason != ai.StopReasonStop {
				t.Fatalf("Stream().Result() = %#v, want stop outcome", result)
			}
			options := <-received
			if options.ServiceTier.IsSet() {
				t.Fatalf("default ServiceTier = %#v, want absent", options.ServiceTier)
			}
			if options.APIKey == nil || *options.APIKey != "resolved-key" {
				t.Fatalf("default typed APIKey = %#v, want resolved key", options.APIKey)
			}
		})
	}
}

func TestModelsContractSimpleStreamAndCompleteDispatchPreserveSimpleOptions(t *testing.T) {
	var models acceptanceMutableModelsSurface = ai.CreateModels()

	const providerID = ai.ProviderID("acceptance-simple")
	model := acceptanceModel(providerID, "simple-model")
	reasoning := ai.ThinkingLevelHigh
	budget := int64(512)
	timeoutMS := int64(222)
	simpleCalls := 0
	fetchCalls := 0

	options := ai.ModelsSimpleStreamOptions{
		SimpleStreamOptions: ai.SimpleStreamOptions{
			StreamOptions: ai.StreamOptions{
				ProviderRequestOptions: ai.ProviderRequestOptions{
					Fetch: func(context.Context, ai.FetchRequest) (ai.FetchResponse, error) {
						fetchCalls++
						return ai.FetchResponse{Status: 205}, nil
					},
					TimeoutMS: &timeoutMS,
				},
			},
			Reasoning:       &reasoning,
			Deferred:        ai.DeferredBoolean{Enabled: true},
			ThinkingBudgets: &ai.ThinkingBudgets{High: &budget},
		},
	}

	provider := ai.CreateProvider(ai.CreateProviderOptions{
		ID: providerID,
		Auth: ai.ProviderAuth{
			APIKey: &ai.APIKeyAuth{
				Name: "Acceptance key",
				Resolve: func(context.Context, ai.APIKeyResolveInput) (ai.Optional[ai.AuthResult], error) {
					return ai.Some(ai.AuthResult{Auth: ai.ModelAuth{APIKey: ai.Some("resolved-key")}}), nil
				},
			},
		},
		Models: []ai.Model{model},
		API: ai.SingleProviderAPI(ai.ProviderStreams{
			StreamSimple: func(ctx context.Context, got ai.Model, _ ai.Context, gotOptions ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
				simpleCalls++
				if got.ID != model.ID || got.Provider != providerID {
					t.Fatalf("StreamSimple() model = %#v, want provider %q model %q", got, providerID, model.ID)
				}
				if gotOptions.Reasoning != &reasoning {
					t.Fatalf("Reasoning pointer = %p, want %p", gotOptions.Reasoning, &reasoning)
				}
				if gotOptions.ThinkingBudgets == nil || gotOptions.ThinkingBudgets.High != &budget {
					t.Fatalf("ThinkingBudgets = %#v, want High pointer %p", gotOptions.ThinkingBudgets, &budget)
				}
				deferred, ok := gotOptions.Deferred.(ai.DeferredBoolean)
				if !ok || !deferred.Enabled {
					t.Fatalf("Deferred = %#v, want DeferredBoolean{Enabled:true}", gotOptions.Deferred)
				}
				if gotOptions.TimeoutMS != &timeoutMS {
					t.Fatalf("TimeoutMS pointer = %p, want %p", gotOptions.TimeoutMS, &timeoutMS)
				}
				response, err := gotOptions.Fetch(ctx, ai.FetchRequest{})
				if err != nil || response.Status != 205 {
					t.Fatalf("Fetch() = (%#v, %v), want status 205", response, err)
				}
				return acceptanceDoneStream(got, "simple-complete")
			},
		}),
	})
	models.SetProvider(provider)

	stream := models.StreamSimple(context.Background(), model, ai.Context{}, options)
	result := acceptanceAwaitMessage(t, stream)
	if result.StopReason != ai.StopReasonStop || result.Model != model.ID {
		t.Fatalf("StreamSimple().Result() = %#v, want terminal assistant message for %q", result, model.ID)
	}
	if simpleCalls != 1 || fetchCalls != 1 {
		t.Fatalf("simple stream calls = %d/%d, want 1/1", simpleCalls, fetchCalls)
	}

	complete, err := models.CompleteSimple(context.Background(), model, ai.Context{}, options)
	if err != nil {
		t.Fatalf("CompleteSimple() error = %v, want nil structured outcome", err)
	}
	if complete.StopReason != ai.StopReasonStop || complete.Model != model.ID {
		t.Fatalf("CompleteSimple() = %#v, want terminal assistant message for %q", complete, model.ID)
	}
	if simpleCalls != 2 || fetchCalls != 2 {
		t.Fatalf("simple complete calls = %d/%d, want 2/2", simpleCalls, fetchCalls)
	}
}

func TestModelsContractUnknownProviderAndMissingAuthReturnStructuredOutcomes(t *testing.T) {
	var models acceptanceMutableModelsSurface = ai.CreateModels()

	unknown := acceptanceModel("missing-provider", "ghost-model")
	complete, err := models.Complete(context.Background(), unknown, ai.Context{}, ai.ModelsStreamOptions{})
	if err != nil {
		t.Fatalf("Complete(unknown provider) error = %v, want nil structured outcome", err)
	}
	acceptanceAssertTerminalModelsOutcome(t, complete, unknown, ai.ModelsErrorCodeProvider, "Unknown provider: missing-provider")

	const providerID = ai.ProviderID("unconfigured")
	unconfigured := acceptanceModel(providerID, "unconfigured-model")
	provider := ai.CreateProvider(ai.CreateProviderOptions{
		ID: providerID,
		Auth: ai.ProviderAuth{
			APIKey: &ai.APIKeyAuth{
				Name: "Missing key",
				Resolve: func(context.Context, ai.APIKeyResolveInput) (ai.Optional[ai.AuthResult], error) {
					return ai.Absent[ai.AuthResult](), nil
				},
			},
		},
		Models: []ai.Model{unconfigured},
		API: ai.SingleProviderAPI(ai.ProviderStreams{
			Stream: func(context.Context, ai.Model, ai.Context, ai.StreamOptions) *ai.AssistantMessageEventStream {
				t.Fatal("Stream() reached provider despite absent auth")
				return nil
			},
			StreamSimple: func(context.Context, ai.Model, ai.Context, ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
				t.Fatal("StreamSimple() reached provider despite absent auth")
				return nil
			},
		}),
	})
	models.SetProvider(provider)

	simple, err := models.CompleteSimple(context.Background(), unconfigured, ai.Context{}, ai.ModelsSimpleStreamOptions{})
	if err != nil {
		t.Fatalf("CompleteSimple(unconfigured) error = %v, want nil structured outcome", err)
	}
	acceptanceAssertTerminalModelsOutcome(t, simple, unconfigured, ai.ModelsErrorCodeAuth, "configured")
}

type acceptanceRefreshCall struct {
	result ai.ModelsRefreshResult
}

func TestModelsRefreshRestoresCacheBeforeNetworkAndPersistsBeforeUpdate(t *testing.T) {
	store := newAcceptanceModelsStore(map[ai.ProviderID]ai.ModelsStoreEntry{
		"refresh-cache": {Models: []ai.Model{acceptanceModel("refresh-cache", "cached-model")}},
	})
	var models acceptanceMutableModelsSurface = ai.CreateModels(ai.CreateModelsOptions{ModelsStore: store})

	restored := make(chan struct{})
	releaseNetwork := make(chan struct{})
	var persistBeforeUpdate atomic.Bool
	var restoredOnce sync.Once
	var provider *acceptanceRefreshProvider
	provider = newAcceptanceRefreshProvider("refresh-cache", acceptanceAmbientAuth(), func(refresh ai.RefreshModelsContext) error {
		if refresh.Stored != nil {
			cached := acceptanceCloneModels(refresh.Stored.Models)
			ok, err := refresh.Publish(ai.ModelsPublication{
				Update: func() { provider.setModels(cached) },
			})
			if err != nil || !ok {
				return err
			}
			restoredOnce.Do(func() { close(restored) })
		}
		if !refresh.AllowNetwork {
			return nil
		}
		<-releaseNetwork

		fresh := []ai.Model{acceptanceModel("refresh-cache", "fresh-model")}
		ok, err := refresh.Publish(ai.ModelsPublication{
			Persist: ai.Some(&ai.ModelsStoreEntry{Models: acceptanceCloneModels(fresh)}),
			Update: func() {
				entry, found := store.Entry("refresh-cache")
				if found && len(entry.Models) == 1 && entry.Models[0].ID == "fresh-model" {
					persistBeforeUpdate.Store(true)
				}
				provider.setModels(fresh)
			},
		})
		if err != nil || !ok {
			return err
		}
		return nil
	})
	models.SetProvider(provider)

	done := make(chan acceptanceRefreshCall, 1)
	go func() {
		result := models.Refresh(context.Background(), ai.ModelsRefreshOptions{AllowNetwork: ai.Some(true), Force: ai.Some(true)})
		done <- acceptanceRefreshCall{result: result}
	}()

	acceptanceWaitSignal(t, restored, "cached restore publication")
	if _, ok := models.GetModel("refresh-cache", "cached-model"); !ok {
		t.Fatal("GetModel(refresh-cache, cached-model) = missing, want cache-first restore before network publication")
	}

	close(releaseNetwork)
	call := acceptanceWaitRefreshCall(t, done)
	if call.result.Aborted {
		t.Fatalf("Refresh() = %#v, want non-aborted successful refresh", call.result)
	}
	if len(call.result.Errors) != 0 {
		t.Fatalf("Refresh().Errors = %#v, want empty success map", call.result.Errors)
	}
	if !persistBeforeUpdate.Load() {
		t.Fatal("refresh publication updated provider state before persistence was visible in ModelsStore")
	}
	if _, ok := models.GetModel("refresh-cache", "fresh-model"); !ok {
		t.Fatal("GetModel(refresh-cache, fresh-model) = missing, want network publication to win after refresh")
	}
	if entry, ok := store.Entry("refresh-cache"); !ok || len(entry.Models) != 1 || entry.Models[0].ID != "fresh-model" {
		t.Fatalf("ModelsStore entry = %#v, want persisted fresh-model snapshot", entry)
	}
}

func TestModelsRefreshRejectsStaleGenerationPublication(t *testing.T) {
	var models acceptanceMutableModelsSurface = ai.CreateModels()

	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	secondPublished := make(chan struct{})
	var networkCalls atomic.Int32

	var provider *acceptanceRefreshProvider
	provider = newAcceptanceRefreshProvider("refresh-generation", acceptanceAmbientAuth(), func(refresh ai.RefreshModelsContext) error {
		if !refresh.AllowNetwork {
			return nil
		}
		switch networkCalls.Add(1) {
		case 1:
			close(firstStarted)
			<-releaseFirst
			published, err := refresh.Publish(ai.ModelsPublication{
				Update: func() { provider.setModels([]ai.Model{acceptanceModel("refresh-generation", "stale-model")}) },
			})
			if err != nil {
				return err
			}
			if published {
				t.Fatal("stale first refresh publication unexpectedly succeeded")
			}
		case 2:
			published, err := refresh.Publish(ai.ModelsPublication{
				Update: func() { provider.setModels([]ai.Model{acceptanceModel("refresh-generation", "fresh-model")}) },
			})
			if err != nil {
				return err
			}
			if !published {
				t.Fatal("second refresh publication was rejected")
			}
			close(secondPublished)
		default:
			t.Fatalf("unexpected network refresh call count %d", networkCalls.Load())
		}
		return nil
	})
	models.SetProvider(provider)

	doneFirst := make(chan error, 1)
	go func() {
		result := models.Refresh(context.Background(), ai.ModelsRefreshOptions{AllowNetwork: ai.Some(true), Force: ai.Some(true)})
		if err := result.Errors["refresh-generation"]; err != nil {
			doneFirst <- err
			return
		}
		doneFirst <- nil
	}()
	acceptanceWaitSignal(t, firstStarted, "first refresh start")

	doneSecond := make(chan error, 1)
	go func() {
		result := models.Refresh(context.Background(), ai.ModelsRefreshOptions{AllowNetwork: ai.Some(true), Force: ai.Some(true)})
		if err := result.Errors["refresh-generation"]; err != nil {
			doneSecond <- err
			return
		}
		doneSecond <- nil
	}()
	acceptanceWaitSignal(t, secondPublished, "second refresh publication")
	close(releaseFirst)

	if err := acceptanceWaitError(t, doneFirst); err != nil {
		t.Fatalf("first Refresh() error = %v", err)
	}
	if err := acceptanceWaitError(t, doneSecond); err != nil {
		t.Fatalf("second Refresh() error = %v", err)
	}
	if _, ok := models.GetModel("refresh-generation", "fresh-model"); !ok {
		t.Fatal("GetModel(refresh-generation, fresh-model) = missing, want newer generation to remain published")
	}
	if _, ok := models.GetModel("refresh-generation", "stale-model"); ok {
		t.Fatal("GetModel(refresh-generation, stale-model) = present, want stale publication rejected")
	}
}

func TestModelsRefreshCancellationPreventsLatePublication(t *testing.T) {
	var models acceptanceMutableModelsSurface = ai.CreateModels()

	started := make(chan struct{})
	var provider *acceptanceRefreshProvider
	provider = newAcceptanceRefreshProvider("refresh-cancel", acceptanceAmbientAuth(), func(refresh ai.RefreshModelsContext) error {
		close(started)
		<-refresh.Context.Done()
		published, err := refresh.Publish(ai.ModelsPublication{
			Update: func() { provider.setModels([]ai.Model{acceptanceModel("refresh-cancel", "late-model")}) },
		})
		if err != nil {
			return err
		}
		if published {
			t.Fatal("canceled refresh publication unexpectedly succeeded")
		}
		return nil
	})
	models.SetProvider(provider)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan acceptanceRefreshCall, 1)
	go func() {
		result := models.Refresh(ctx, ai.ModelsRefreshOptions{AllowNetwork: ai.Some(true), Force: ai.Some(true)})
		done <- acceptanceRefreshCall{result: result}
	}()

	acceptanceWaitSignal(t, started, "cancelable refresh start")
	cancel()
	call := acceptanceWaitCanceledRefresh(t, done)
	if !call.result.Aborted {
		t.Fatalf("Refresh(canceled) = %#v, want Aborted=true", call.result)
	}
	if len(call.result.Errors) != 0 {
		t.Fatalf("Refresh(canceled).Errors = %#v, want empty map", call.result.Errors)
	}
	if _, ok := models.GetModel("refresh-cancel", "late-model"); ok {
		t.Fatal("GetModel(refresh-cancel, late-model) = present, want canceled publication rejected")
	}
}

type acceptanceRefreshProvider struct {
	id        ai.ProviderID
	auth      ai.ProviderAuth
	mu        sync.Mutex
	models    []ai.Model
	onRefresh func(ai.RefreshModelsContext) error
}

func newAcceptanceRefreshProvider(
	id ai.ProviderID,
	auth ai.ProviderAuth,
	onRefresh func(ai.RefreshModelsContext) error,
) *acceptanceRefreshProvider {
	return &acceptanceRefreshProvider{id: id, auth: auth, onRefresh: onRefresh}
}

func (p *acceptanceRefreshProvider) ID() ai.ProviderID            { return p.id }
func (p *acceptanceRefreshProvider) Name() string                 { return string(p.id) }
func (p *acceptanceRefreshProvider) BaseURL() ai.Optional[string] { return ai.Absent[string]() }
func (p *acceptanceRefreshProvider) Headers() ai.ProviderHeaders  { return nil }
func (p *acceptanceRefreshProvider) Auth() ai.ProviderAuth        { return p.auth }

func (p *acceptanceRefreshProvider) GetModels() []ai.Model {
	p.mu.Lock()
	defer p.mu.Unlock()
	return acceptanceCloneModels(p.models)
}

func (p *acceptanceRefreshProvider) FilterModels(models []ai.Model, _ ai.Credential) []ai.Model {
	return acceptanceCloneModels(models)
}

func (p *acceptanceRefreshProvider) Stream(context.Context, ai.Model, ai.Context, ai.ProviderStreamOptions) *ai.AssistantMessageEventStream {
	return acceptanceDoneStream(acceptanceModel(p.id, "unused-stream-model"), "unused")
}

func (p *acceptanceRefreshProvider) StreamSimple(context.Context, ai.Model, ai.Context, ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
	return acceptanceDoneStream(acceptanceModel(p.id, "unused-simple-model"), "unused")
}

func (p *acceptanceRefreshProvider) SupportsRefreshModels() bool { return p.onRefresh != nil }

func (p *acceptanceRefreshProvider) RefreshModels(refresh ai.RefreshModelsContext) error {
	if p.onRefresh == nil {
		return errors.New("refresh not configured for acceptance provider")
	}
	return p.onRefresh(refresh)
}

func (p *acceptanceRefreshProvider) SupportsFetchDeferred() bool { return false }

func (p *acceptanceRefreshProvider) FetchDeferred(context.Context, ai.Model, ai.DeferredHandle, ai.DeferredFetchOptions) (*ai.AssistantMessageEventStream, error) {
	return nil, ai.ErrNotImplemented
}

func (p *acceptanceRefreshProvider) SupportsCancelDeferred() bool { return false }

func (p *acceptanceRefreshProvider) CancelDeferred(context.Context, ai.Model, ai.DeferredHandle, ai.DeferredCancelOptions) error {
	return ai.ErrNotImplemented
}

func (p *acceptanceRefreshProvider) setModels(models []ai.Model) {
	p.mu.Lock()
	p.models = acceptanceCloneModels(models)
	p.mu.Unlock()
}

type acceptanceModelsStore struct {
	mu      sync.Mutex
	entries map[ai.ProviderID]ai.ModelsStoreEntry
}

func newAcceptanceModelsStore(initial map[ai.ProviderID]ai.ModelsStoreEntry) *acceptanceModelsStore {
	entries := make(map[ai.ProviderID]ai.ModelsStoreEntry, len(initial))
	for providerID, entry := range initial {
		entries[providerID] = acceptanceCloneStoreEntry(entry)
	}
	return &acceptanceModelsStore{entries: entries}
}

func (s *acceptanceModelsStore) Read(ctx context.Context, providerID ai.ProviderID) (ai.ModelsStoreEntry, bool, error) {
	if err := ctx.Err(); err != nil {
		return ai.ModelsStoreEntry{}, false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.entries[providerID]
	if !ok {
		return ai.ModelsStoreEntry{}, false, nil
	}
	return acceptanceCloneStoreEntry(entry), true, nil
}

func (s *acceptanceModelsStore) Write(ctx context.Context, providerID ai.ProviderID, entry ai.ModelsStoreEntry) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	s.entries[providerID] = acceptanceCloneStoreEntry(entry)
	s.mu.Unlock()
	return nil
}

func (s *acceptanceModelsStore) Delete(ctx context.Context, providerID ai.ProviderID) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	delete(s.entries, providerID)
	s.mu.Unlock()
	return nil
}

func (s *acceptanceModelsStore) Entry(providerID ai.ProviderID) (ai.ModelsStoreEntry, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.entries[providerID]
	return acceptanceCloneStoreEntry(entry), ok
}

func acceptanceAmbientAuth() ai.ProviderAuth {
	return ai.ProviderAuth{
		APIKey: &ai.APIKeyAuth{
			Name: "Acceptance ambient key",
			Resolve: func(context.Context, ai.APIKeyResolveInput) (ai.Optional[ai.AuthResult], error) {
				return ai.Some(ai.AuthResult{Auth: ai.ModelAuth{APIKey: ai.Some("ambient-key")}, Source: ai.Some("ambient")}), nil
			},
		},
	}
}

func acceptanceAssertResolvedDispatchState(t *testing.T, got, original ai.Model) {
	t.Helper()
	if got.ID != original.ID || got.Provider != original.Provider || got.API != original.API {
		t.Fatalf("resolved request model = %#v, want identity-preserving provider/model/api", got)
	}
	if got.BaseURL != "https://resolved.example.test/v1" {
		t.Fatalf("resolved BaseURL = %q, want auth override", got.BaseURL)
	}
}

func acceptanceAssertRequestOptionsPreserved(
	t *testing.T,
	ctx context.Context,
	model ai.Model,
	got ai.ProviderRequestOptions,
	timeoutMS int64,
	retries int,
) {
	t.Helper()
	if got.TimeoutMS == nil || *got.TimeoutMS != timeoutMS {
		t.Fatalf("TimeoutMS = %#v, want %d", got.TimeoutMS, timeoutMS)
	}
	if got.MaxRetries == nil || *got.MaxRetries != retries {
		t.Fatalf("MaxRetries = %#v, want %d", got.MaxRetries, retries)
	}
	if got.APIKey == nil || *got.APIKey != "resolved-key" {
		t.Fatalf("APIKey = %#v, want resolved-key", got.APIKey)
	}
	if response, err := got.Fetch(ctx, ai.FetchRequest{}); err != nil || response.Status != 204 {
		t.Fatalf("Fetch() = (%#v, %v), want status 204", response, err)
	}
	if _, err := got.OnPayload(ctx, nil, model); err != nil {
		t.Fatalf("OnPayload() error = %v", err)
	}
	if err := got.OnResponse(ctx, ai.ProviderResponse{}, model); err != nil {
		t.Fatalf("OnResponse() error = %v", err)
	}
	if got.Env["AUTH_ENV"] != "auth" || got.Env["REQUEST_ENV"] != "request" {
		t.Fatalf("Env = %#v, want merged auth and request env", got.Env)
	}
	if got.Headers["X-Model"] == nil || *got.Headers["X-Model"] != "model-only" {
		t.Fatalf("Headers = %#v, want model header preserved", got.Headers)
	}
	if got.Headers["X-Shared"] == nil || *got.Headers["X-Shared"] != "request" {
		t.Fatalf("Headers = %#v, want request header overriding auth header case-insensitively", got.Headers)
	}
	if got.Headers["X-Request"] == nil || *got.Headers["X-Request"] != "request-only" {
		t.Fatalf("Headers = %#v, want request-only header preserved", got.Headers)
	}
	if _, duplicated := got.Headers["x-shared"]; duplicated {
		t.Fatalf("Headers kept case-insensitive duplicate: %#v", got.Headers)
	}
}

func acceptanceAwaitMessage(t *testing.T, stream *ai.AssistantMessageEventStream) ai.AssistantMessage {
	t.Helper()
	wait, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	result, err := stream.Result(wait)
	if err != nil {
		t.Fatalf("Result() error = %v", err)
	}
	return result
}

func acceptanceAssertTerminalModelsOutcome(
	t *testing.T,
	message ai.AssistantMessage,
	model ai.Model,
	code ai.ModelsErrorCode,
	wantSubstring string,
) {
	t.Helper()
	errorMessage, ok := message.ErrorMessage.Value()
	if !ok {
		t.Fatalf("ErrorMessage = absent, want structured %q failure", code)
	}
	if message.Role != ai.MessageRoleAssistant || message.StopReason != ai.StopReasonError ||
		message.Provider != model.Provider || message.Model != model.ID || message.API != model.API {
		t.Fatalf("terminal outcome = %#v, want structured assistant error for %#v", message, model)
	}
	if !strings.Contains(errorMessage, string(code)) || !strings.Contains(errorMessage, wantSubstring) {
		t.Fatalf("ErrorMessage = %q, want code %q and substring %q", errorMessage, code, wantSubstring)
	}
}

func acceptanceWaitSignal(t *testing.T, signal <-chan struct{}, label string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for %s", label)
	}
}

func acceptanceWaitError(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for refresh completion")
		return nil
	}
}

func acceptanceWaitRefreshCall(t *testing.T, done <-chan acceptanceRefreshCall) acceptanceRefreshCall {
	t.Helper()
	select {
	case call := <-done:
		return call
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for refresh result")
		return acceptanceRefreshCall{}
	}
}

func acceptanceWaitCanceledRefresh(t *testing.T, done <-chan acceptanceRefreshCall) acceptanceRefreshCall {
	t.Helper()
	return acceptanceWaitRefreshCall(t, done)
}

func acceptanceDoneStream(model ai.Model, text string) *ai.AssistantMessageEventStream {
	stream := ai.NewAssistantMessageEventStream()
	stream.Push(ai.AssistantMessageDoneEvent{
		Type:   ai.AssistantMessageEventTypeDone,
		Reason: ai.StopReasonStop,
		Message: ai.AssistantMessage{
			Role:       ai.MessageRoleAssistant,
			Content:    []ai.AssistantContent{ai.TextContent{Type: ai.ContentTypeText, Text: text}},
			API:        model.API,
			Provider:   model.Provider,
			Model:      model.ID,
			StopReason: ai.StopReasonStop,
			Timestamp:  time.Now().UnixMilli(),
		},
	})
	return stream
}

func acceptanceModel(provider ai.ProviderID, id string) ai.Model {
	return ai.Model{
		ID:       id,
		Name:     id,
		API:      ai.API("acceptance-api"),
		Provider: provider,
		BaseURL:  "https://model.example.test/v1",
		Headers:  map[string]string{"X-Model": "model-only"},
	}
}

func acceptanceCloneModels(models []ai.Model) []ai.Model {
	if models == nil {
		return nil
	}
	cloned := make([]ai.Model, len(models))
	for index, model := range models {
		cloned[index] = model
		if model.Headers != nil {
			cloned[index].Headers = make(map[string]string, len(model.Headers))
			for key, value := range model.Headers {
				cloned[index].Headers[key] = value
			}
		}
		if model.Input != nil {
			cloned[index].Input = append([]ai.ModelInput(nil), model.Input...)
		}
		if model.Cost.Tiers != nil {
			cloned[index].Cost.Tiers = append([]ai.ModelCostTier(nil), model.Cost.Tiers...)
		}
	}
	return cloned
}

func acceptanceCloneStoreEntry(entry ai.ModelsStoreEntry) ai.ModelsStoreEntry {
	clone := entry
	clone.Models = acceptanceCloneModels(entry.Models)
	return clone
}

func acceptanceStringPointer(value string) *string { return &value }

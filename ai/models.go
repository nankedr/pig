package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
)

// ModelsRequestTransforms are collection-only transformations applied after
// provider auth, model headers, and explicit request headers are assembled.
type ModelsRequestTransforms struct {
	TransformHeaders func(context.Context, ProviderHeaders) (ProviderHeaders, error) `json:"-"`
}

// ModelsStreamOptions adds Models-owned request transforms without erasing
// any function-valued StreamOptions fields.
type ModelsStreamOptions struct {
	StreamOptions
	ModelsRequestTransforms
}

// ModelsAPIStreamOptions carries one concrete API-specific option value plus
// Models-owned transforms through the heterogeneous runtime collection.
type ModelsAPIStreamOptions[T ProviderStreamOptions] struct {
	Options T
	ModelsRequestTransforms
}

// ModelsStreamOption is the closed runtime carrier accepted by Stream and
// Complete. Use ModelsStreamOptions for generic APIs or
// ModelsAPIStreamOptions[T] for a known API's concrete option type.
type ModelsStreamOption interface {
	modelsProviderStreamOptions() ProviderStreamOptions
	modelsRequestTransforms() ModelsRequestTransforms
}

func (o ModelsStreamOptions) modelsProviderStreamOptions() ProviderStreamOptions {
	return o.StreamOptions
}
func (o ModelsStreamOptions) modelsRequestTransforms() ModelsRequestTransforms {
	return o.ModelsRequestTransforms
}
func (o ModelsAPIStreamOptions[T]) modelsProviderStreamOptions() ProviderStreamOptions {
	return o.Options
}
func (o ModelsAPIStreamOptions[T]) modelsRequestTransforms() ModelsRequestTransforms {
	return o.ModelsRequestTransforms
}

type ModelsSimpleStreamOptions struct {
	SimpleStreamOptions
	ModelsRequestTransforms
}

type ModelsDeferredFetchOptions struct {
	DeferredFetchOptions
	ModelsRequestTransforms
}

type ModelsDeferredCancelOptions struct {
	DeferredCancelOptions
	ModelsRequestTransforms
}

// ModelsRefreshOptions selects dynamic providers and controls their network
// phase. Absent AllowNetwork defaults to true; Force remains three-state.
type ModelsRefreshOptions struct {
	AllowNetwork Optional[bool]
	Providers    []ProviderID
	Force        Optional[bool]
}

// ModelsRefreshResult reports caller cancellation separately from per-provider
// failures. Provider-local supersession is not caller cancellation.
type ModelsRefreshResult struct {
	Aborted bool
	Errors  map[ProviderID]error
}

// CreateModelsOptions supplies the application-owned seams used by a Models
// collection. Zero values select side-effect-free in-memory defaults.
type CreateModelsOptions struct {
	Credentials CredentialStore
	ModelsStore ModelsStore
	AuthContext AuthContext
}

// Models is the runtime collection of providers and their last-known models.
type Models interface {
	GetProviders() []Provider
	GetProvider(id ProviderID) (Provider, bool)
	GetModels(provider ...ProviderID) []Model
	GetModel(provider ProviderID, id string) (Model, bool)
	Refresh(ctx context.Context, options ...ModelsRefreshOptions) ModelsRefreshResult
	CheckAuth(ctx context.Context, provider ProviderID, options ...AuthOperationOptions) (Optional[AuthCheck], error)
	GetAvailable(ctx context.Context, provider ...ProviderID) ([]Model, error)
	GetProviderAuth(ctx context.Context, provider ProviderID, overrides ...AuthResolutionOverrides) (Optional[AuthResult], error)
	GetModelAuth(ctx context.Context, model Model, overrides ...AuthResolutionOverrides) (Optional[AuthResult], error)
	Login(ctx context.Context, provider ProviderID, authType AuthType, interaction ProviderAuthInteraction) (Credential, error)
	Logout(ctx context.Context, provider ProviderID, options ...AuthOperationOptions) error
	Stream(ctx context.Context, model Model, input Context, options ...ModelsStreamOption) *AssistantMessageEventStream
	Complete(ctx context.Context, model Model, input Context, options ...ModelsStreamOption) (AssistantMessage, error)
	StreamSimple(ctx context.Context, model Model, input Context, options ...ModelsSimpleStreamOptions) *AssistantMessageEventStream
	CompleteSimple(ctx context.Context, model Model, input Context, options ...ModelsSimpleStreamOptions) (AssistantMessage, error)
	FetchDeferred(ctx context.Context, model Model, handle DeferredHandle, options ...ModelsDeferredFetchOptions) (AssistantMessage, error)
	CancelDeferred(ctx context.Context, model Model, handle DeferredHandle, options ...ModelsDeferredCancelOptions) error
}

// MutableModels adds ordered provider-registry mutation to Models.
type MutableModels interface {
	Models
	SetProvider(provider Provider)
	DeleteProvider(id ProviderID)
	ClearProviders()
}

type modelsImpl struct {
	mu         sync.RWMutex
	registryMu sync.Mutex
	providers  map[ProviderID]Provider
	order      []ProviderID
	refresh    map[ProviderID]*providerRefreshState

	credentials CredentialStore
	modelsStore ModelsStore
	authContext AuthContext
}

type providerRefreshState struct {
	generation       uint64
	registryRevision uint64
	current          *providerRefreshRun
	outstanding      int
	updating         bool
}

type providerRefreshRun struct {
	generation   uint64
	ctx          context.Context
	cancel       context.CancelFunc
	done         chan error
	publications chan struct{}
}

type providerRefreshCandidate struct {
	provider         Provider
	state            *providerRefreshState
	registryRevision uint64
}

// CreateModels returns an independent mutable provider collection. It accepts
// zero or one option value so callers can use CreateModels() for M0 defaults.
func CreateModels(options ...CreateModelsOptions) MutableModels {
	configured := CreateModelsOptions{}
	if len(options) != 0 {
		configured = options[0]
	}
	if configured.Credentials == nil {
		configured.Credentials = NewInMemoryCredentialStore()
	}
	if configured.ModelsStore == nil {
		configured.ModelsStore = NewInMemoryModelsStore()
	}
	if configured.AuthContext == nil {
		configured.AuthContext = DefaultProviderAuthContext()
	}
	return &modelsImpl{
		providers:   make(map[ProviderID]Provider),
		refresh:     make(map[ProviderID]*providerRefreshState),
		credentials: configured.Credentials,
		modelsStore: configured.ModelsStore,
		authContext: configured.AuthContext,
	}
}

func (m *modelsImpl) SetProvider(provider Provider) {
	if provider == nil {
		return
	}
	id := provider.ID()
	m.registryMu.Lock()
	m.mu.Lock()
	cancel := m.invalidateProviderRefreshLocked(id)
	if _, exists := m.providers[id]; !exists {
		m.order = append(m.order, id)
	}
	m.providers[id] = provider
	m.mu.Unlock()
	m.registryMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (m *modelsImpl) DeleteProvider(id ProviderID) {
	m.registryMu.Lock()
	m.mu.Lock()
	var cancel context.CancelFunc
	if _, exists := m.providers[id]; exists || m.refresh[id] != nil {
		cancel = m.invalidateProviderRefreshLocked(id)
	}
	if _, exists := m.providers[id]; exists {
		delete(m.providers, id)
		for index, candidate := range m.order {
			if candidate == id {
				m.order = append(m.order[:index], m.order[index+1:]...)
				break
			}
		}
	}
	m.deleteIdleRefreshStateLocked(id)
	m.mu.Unlock()
	m.registryMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (m *modelsImpl) ClearProviders() {
	m.registryMu.Lock()
	m.mu.Lock()
	ids := make(map[ProviderID]struct{}, len(m.providers)+len(m.refresh))
	for id := range m.providers {
		ids[id] = struct{}{}
	}
	for id := range m.refresh {
		ids[id] = struct{}{}
	}
	cancels := make([]context.CancelFunc, 0, len(ids))
	for id := range ids {
		if cancel := m.invalidateProviderRefreshLocked(id); cancel != nil {
			cancels = append(cancels, cancel)
		}
	}
	clear(m.providers)
	m.order = nil
	for id := range ids {
		m.deleteIdleRefreshStateLocked(id)
	}
	m.mu.Unlock()
	m.registryMu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
}

func (m *modelsImpl) ensureProviderRefreshState(providerID ProviderID) *providerRefreshState {
	m.mu.Lock()
	state := m.ensureProviderRefreshStateLocked(providerID)
	m.mu.Unlock()
	return state
}

func (m *modelsImpl) ensureProviderRefreshStateLocked(providerID ProviderID) *providerRefreshState {
	state := m.refresh[providerID]
	if state == nil {
		state = &providerRefreshState{}
		m.refresh[providerID] = state
	}
	return state
}

func (m *modelsImpl) supersedeProviderRefreshLocked(providerID ProviderID) context.CancelFunc {
	state := m.ensureProviderRefreshStateLocked(providerID)
	state.generation++
	var cancel context.CancelFunc
	if state.current != nil {
		cancel = state.current.cancel
		state.current = nil
	}
	return cancel
}

func (m *modelsImpl) invalidateProviderRefreshLocked(providerID ProviderID) context.CancelFunc {
	state := m.ensureProviderRefreshStateLocked(providerID)
	state.registryRevision++
	return m.supersedeProviderRefreshLocked(providerID)
}

func (m *modelsImpl) deleteIdleRefreshStateLocked(providerID ProviderID) {
	state := m.refresh[providerID]
	if state != nil && state.outstanding == 0 {
		if _, registered := m.providers[providerID]; !registered {
			delete(m.refresh, providerID)
		}
	}
}

func (m *modelsImpl) GetProviders() []Provider {
	m.mu.RLock()
	providers := make([]Provider, 0, len(m.order))
	for _, id := range m.order {
		providers = append(providers, m.providers[id])
	}
	m.mu.RUnlock()
	return providers
}

func (m *modelsImpl) GetProvider(id ProviderID) (Provider, bool) {
	m.mu.RLock()
	provider, ok := m.providers[id]
	m.mu.RUnlock()
	return provider, ok
}

func (m *modelsImpl) GetModels(provider ...ProviderID) []Model {
	if len(provider) != 0 {
		entry, ok := m.GetProvider(provider[0])
		if !ok {
			return []Model{}
		}
		return bestEffortProviderModels(entry)
	}

	providers := m.GetProviders()
	models := make([]Model, 0)
	for _, entry := range providers {
		models = append(models, bestEffortProviderModels(entry)...)
	}
	return models
}

func (m *modelsImpl) GetModel(provider ProviderID, id string) (Model, bool) {
	for _, model := range m.GetModels(provider) {
		if model.ID == id {
			return model, true
		}
	}
	return Model{}, false
}

// Refresh runs cache restoration before auth/network work for every selected
// dynamic provider. Providers refresh concurrently; failures are collected.
func (m *modelsImpl) Refresh(ctx context.Context, options ...ModelsRefreshOptions) ModelsRefreshResult {
	ctx = nonNilContext(ctx)
	result := ModelsRefreshResult{Errors: make(map[ProviderID]error)}
	if ctx.Err() != nil {
		result.Aborted = true
		return result
	}
	configured := firstModelsRefreshOptions(options)
	allowNetwork := true
	if value, ok := configured.AllowNetwork.Value(); ok {
		allowNetwork = value
	}
	var selected map[ProviderID]struct{}
	if configured.Providers != nil {
		selected = make(map[ProviderID]struct{}, len(configured.Providers))
		for _, id := range configured.Providers {
			selected[id] = struct{}{}
		}
	}

	candidates := m.snapshotProviderRefreshCandidates()
	refreshable := make([]providerRefreshCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		provider := candidate.provider
		if selected != nil {
			if _, ok := selected[provider.ID()]; !ok {
				continue
			}
		}
		if provider.SupportsRefreshModels() {
			refreshable = append(refreshable, candidate)
		}
	}

	type refreshOutcome struct {
		providerID ProviderID
		err        error
	}
	outcomes := make(chan refreshOutcome, len(refreshable))
	for _, candidate := range refreshable {
		candidate := candidate
		go func() {
			provider := candidate.provider
			run, state, started, beginErr := m.beginProviderRefresh(ctx, provider.ID(), candidate.state, candidate.registryRevision)
			if beginErr != nil {
				outcomes <- refreshOutcome{providerID: provider.ID(), err: beginErr}
				return
			}
			if !started {
				outcomes <- refreshOutcome{providerID: provider.ID()}
				return
			}
			defer run.cancel()
			go func() {
				err := m.runProviderRefresh(run.ctx, provider, run, state, allowNetwork, configured.Force)
				m.finishProviderRefresh(provider.ID(), state, run)
				run.done <- err
			}()
			var err error
			select {
			case err = <-run.done:
				if run.ctx.Err() != nil {
					err = nil
				}
			case <-run.ctx.Done():
				err = nil
			}
			outcomes <- refreshOutcome{providerID: provider.ID(), err: err}
		}()
	}
	for range refreshable {
		select {
		case outcome := <-outcomes:
			if ctx.Err() != nil {
				result.Aborted = true
				return result
			}
			if outcome.err != nil {
				result.Errors[outcome.providerID] = outcome.err
			}
		case <-ctx.Done():
			result.Aborted = true
			return result
		}
	}
	result.Aborted = ctx.Err() != nil
	return result
}

func (m *modelsImpl) snapshotProviderRefreshCandidates() []providerRefreshCandidate {
	m.mu.RLock()
	candidates := make([]providerRefreshCandidate, 0, len(m.order))
	for _, id := range m.order {
		state := m.refresh[id]
		revision := uint64(0)
		if state != nil {
			revision = state.registryRevision
		}
		candidates = append(candidates, providerRefreshCandidate{
			provider:         m.providers[id],
			state:            state,
			registryRevision: revision,
		})
	}
	m.mu.RUnlock()
	return candidates
}

func (m *modelsImpl) beginProviderRefresh(parent context.Context, providerID ProviderID, expectedState *providerRefreshState, registryRevision uint64) (*providerRefreshRun, *providerRefreshState, bool, error) {
	ctx, cancel := context.WithCancel(parent)
	run := &providerRefreshRun{
		ctx:          ctx,
		cancel:       cancel,
		done:         make(chan error, 1),
		publications: make(chan struct{}, 1),
	}
	m.registryMu.Lock()
	m.mu.Lock()
	state := m.ensureProviderRefreshStateLocked(providerID)
	if state != expectedState || state.registryRevision != registryRevision {
		m.mu.Unlock()
		m.registryMu.Unlock()
		cancel()
		return run, state, false, nil
	}
	if state.updating {
		m.mu.Unlock()
		m.registryMu.Unlock()
		cancel()
		return run, state, false, newModelsError(
			ModelsErrorCodeModelSource,
			fmt.Sprintf("provider %s refresh cannot start during its publication update", providerID),
			ErrEventStreamInvariant,
		)
	}
	previous := m.supersedeProviderRefreshLocked(providerID)
	run.generation = state.generation
	state.outstanding++
	state.current = run
	m.mu.Unlock()
	m.registryMu.Unlock()
	if previous != nil {
		previous()
	}
	return run, state, true, nil
}

func (m *modelsImpl) finishProviderRefresh(providerID ProviderID, state *providerRefreshState, run *providerRefreshRun) {
	m.mu.Lock()
	if current := m.refresh[providerID]; current == state {
		if state.current == run {
			state.current = nil
		}
		if state.outstanding > 0 {
			state.outstanding--
		}
		m.deleteIdleRefreshStateLocked(providerID)
	}
	m.mu.Unlock()
}

func (m *modelsImpl) runProviderRefresh(
	ctx context.Context,
	provider Provider,
	run *providerRefreshRun,
	state *providerRefreshState,
	allowNetwork bool,
	force Optional[bool],
) error {
	credential, credentialErr := m.readCredential(ctx, provider.ID(), AuthOperationOptions{})
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err := m.runProviderRefreshPhase(ctx, provider, cloneCredential(credential), false, Absent[bool](), run, state); err != nil {
		return err
	}
	if credentialErr != nil {
		return credentialErr
	}
	if !allowNetwork || ctx.Err() != nil {
		return ctx.Err()
	}
	effective, err := m.resolveRefreshCredential(ctx, provider, credential)
	if err != nil {
		return err
	}
	if effective == nil || ctx.Err() != nil {
		return ctx.Err()
	}
	return m.runProviderRefreshPhase(ctx, provider, effective, true, force, run, state)
}

func (m *modelsImpl) runProviderRefreshPhase(
	ctx context.Context,
	provider Provider,
	credential Credential,
	allowNetwork bool,
	force Optional[bool],
	run *providerRefreshRun,
	state *providerRefreshState,
) error {
	stored, ok, err := m.modelsStore.Read(ctx, provider.ID())
	if err != nil {
		return err
	}
	var storedSnapshot *ModelsStoreEntry
	if ok {
		clone := cloneModelsStoreEntry(stored)
		storedSnapshot = &clone
	}
	return provider.RefreshModels(RefreshModelsContext{
		Context:      ctx,
		Credential:   cloneCredential(credential),
		Stored:       storedSnapshot,
		AllowNetwork: allowNetwork,
		Force:        force,
		Publish: func(publication ModelsPublication) (bool, error) {
			return m.publishProviderModels(ctx, provider.ID(), run, state, publication)
		},
	})
}

func (m *modelsImpl) publishProviderModels(
	ctx context.Context,
	providerID ProviderID,
	run *providerRefreshRun,
	state *providerRefreshState,
	publication ModelsPublication,
) (bool, error) {
	if !m.providerRefreshCurrent(ctx, providerID, run.generation, state) {
		return false, nil
	}
	select {
	case run.publications <- struct{}{}:
		defer func() { <-run.publications }()
	case <-ctx.Done():
		return false, nil
	}
	if !m.providerRefreshCurrent(ctx, providerID, run.generation, state) {
		return false, nil
	}

	if publication.Persist.IsSet() {
		entry, hasEntry := publication.Persist.Value()
		if !hasEntry || entry == nil {
			if err := m.modelsStore.Delete(ctx, providerID); err != nil {
				return false, err
			}
		} else if err := m.modelsStore.Write(ctx, providerID, cloneModelsStoreEntry(*entry)); err != nil {
			return false, err
		}
	}

	if !m.providerRefreshCurrent(ctx, providerID, run.generation, state) {
		return false, nil
	}
	if publication.Update != nil {
		if !m.beginProviderPublicationUpdate(ctx, providerID, run.generation, state) {
			return false, nil
		}
		defer m.endProviderPublicationUpdate(state)
		publication.Update()
	}
	return true, nil
}

// beginProviderPublicationUpdate gives a synchronous update a linearization
// point without holding an internal mutex across extension code. A same-
// provider Refresh that overlaps this window is rejected by
// beginProviderRefresh; callers that need another refresh must start it after
// Update returns.
func (m *modelsImpl) beginProviderPublicationUpdate(
	ctx context.Context,
	providerID ProviderID,
	generation uint64,
	state *providerRefreshState,
) bool {
	if ctx.Err() != nil {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	current := m.refresh[providerID]
	if current != state || state.generation != generation || state.updating || ctx.Err() != nil {
		return false
	}
	state.updating = true
	return true
}

func (m *modelsImpl) endProviderPublicationUpdate(state *providerRefreshState) {
	m.mu.Lock()
	state.updating = false
	m.mu.Unlock()
}

func (m *modelsImpl) providerRefreshCurrent(ctx context.Context, providerID ProviderID, generation uint64, state *providerRefreshState) bool {
	if ctx.Err() != nil {
		return false
	}
	m.mu.RLock()
	current := m.refresh[providerID]
	valid := current == state && state.generation == generation
	m.mu.RUnlock()
	return valid && ctx.Err() == nil
}

func (m *modelsImpl) resolveRefreshCredential(ctx context.Context, provider Provider, stored Credential) (Credential, error) {
	auth := provider.Auth()
	if oauthCredential, ok := oauthCredentialValue(stored); ok {
		if auth.OAuth == nil {
			return nil, nil
		}
		if time.Now().UnixMilli() < oauthCredential.Expires {
			return cloneCredential(oauthCredential), nil
		}
		post, err := m.credentials.Modify(ctx, provider.ID(), func(modifyCtx context.Context, current Credential) (Credential, error) {
			currentOAuth, ok := oauthCredentialValue(current)
			if !ok || time.Now().UnixMilli() < currentOAuth.Expires {
				return nil, nil
			}
			if auth.OAuth.Refresh == nil {
				return nil, newModelsError(ModelsErrorCodeOAuth, fmt.Sprintf("OAuth refresh failed for %s", provider.ID()), newNotImplemented("OAuthAuth.Refresh"))
			}
			callbackCredential := currentOAuth
			callbackCredential.Extra = cloneRawMessageMap(callbackCredential.Extra)
			refreshed, refreshErr := auth.OAuth.Refresh(modifyCtx, callbackCredential)
			if refreshErr != nil {
				return nil, newModelsError(ModelsErrorCodeOAuth, fmt.Sprintf("OAuth refresh failed for %s", provider.ID()), refreshErr)
			}
			if refreshed.Type == "" {
				refreshed.Type = AuthTypeOAuth
			} else if refreshed.Type != AuthTypeOAuth {
				return nil, newModelsError(
					ModelsErrorCodeOAuth,
					fmt.Sprintf("OAuth refresh returned credential discriminator %q for %s, want %q", refreshed.Type, provider.ID(), AuthTypeOAuth),
					nil,
				)
			}
			return refreshed, nil
		}, AuthOperationOptions{})
		if err != nil {
			var modelsErr *ModelsError
			if errors.As(err, &modelsErr) {
				return nil, err
			}
			return nil, newModelsError(ModelsErrorCodeAuth, fmt.Sprintf("credential store modify failed for %s", provider.ID()), err)
		}
		if refreshed, ok := oauthCredentialValue(post); ok {
			return cloneCredential(refreshed), nil
		}
		return nil, nil
	}

	apiKey := auth.APIKey
	if apiKey == nil {
		return nil, nil
	}
	var credential *APIKeyCredential
	if storedAPIKey, ok := apiKeyCredentialValue(stored); ok {
		copy := storedAPIKey
		copy.Env = cloneProviderEnv(copy.Env)
		credential = &copy
	} else if stored != nil {
		return nil, nil
	}
	if apiKey.Resolve == nil {
		return nil, newModelsError(ModelsErrorCodeAuth, fmt.Sprintf("api key auth for %s has no resolve", provider.ID()), nil)
	}
	resolved, err := apiKey.Resolve(ctx, APIKeyResolveInput{Context: m.authContext, Credential: credential})
	if err != nil {
		return nil, newModelsError(ModelsErrorCodeAuth, fmt.Sprintf("API key auth failed for provider %s", provider.ID()), err)
	}
	result, ok := resolved.Value()
	if !ok {
		return nil, nil
	}
	return APIKeyCredential{Type: AuthTypeAPIKey, Key: result.Auth.APIKey, Env: cloneProviderEnv(result.Env)}, nil
}

func firstModelsRefreshOptions(options []ModelsRefreshOptions) ModelsRefreshOptions {
	if len(options) == 0 {
		return ModelsRefreshOptions{}
	}
	return options[0]
}

// CheckAuth reports whether provider is configured without refreshing a
// stored OAuth credential.
func (m *modelsImpl) CheckAuth(ctx context.Context, providerID ProviderID, options ...AuthOperationOptions) (Optional[AuthCheck], error) {
	ctx = nonNilContext(ctx)
	provider, ok := m.GetProvider(providerID)
	if !ok {
		return Absent[AuthCheck](), nil
	}
	credential, err := m.readCredential(ctx, providerID, firstAuthOperationOptions(options))
	if err != nil {
		return Absent[AuthCheck](), err
	}
	return m.checkProviderAuth(ctx, provider, credential)
}

// GetAvailable returns models only from configured providers and applies each
// provider's credential-specific availability policy.
func (m *modelsImpl) GetAvailable(ctx context.Context, providerID ...ProviderID) ([]Model, error) {
	ctx = nonNilContext(ctx)
	providers := m.GetProviders()
	if len(providerID) != 0 {
		provider, ok := m.GetProvider(providerID[0])
		if !ok {
			return []Model{}, nil
		}
		providers = []Provider{provider}
	}

	available := make([]Model, 0)
	for _, provider := range providers {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		credential, err := m.readCredential(ctx, provider.ID(), AuthOperationOptions{})
		if err != nil {
			return nil, err
		}
		check, err := m.checkProviderAuth(ctx, provider, credential)
		if err != nil {
			return nil, err
		}
		if _, configured := check.Value(); !configured {
			continue
		}
		models := bestEffortProviderModels(provider)
		if len(models) == 0 {
			continue
		}
		filtered, err := bestEffortFilterModels(provider, models, cloneCredential(credential))
		if err != nil {
			return nil, err
		}
		available = append(available, filtered...)
	}
	return available, nil
}

// GetProviderAuth resolves request auth for provider. Unknown and
// unconfigured providers return an absent result.
func (m *modelsImpl) GetProviderAuth(ctx context.Context, providerID ProviderID, overrides ...AuthResolutionOverrides) (Optional[AuthResult], error) {
	ctx = nonNilContext(ctx)
	provider, ok := m.GetProvider(providerID)
	if !ok {
		return Absent[AuthResult](), nil
	}
	return ResolveProviderAuth(
		ctx,
		ProviderAuthTarget{ID: providerID, Auth: provider.Auth()},
		m.credentials,
		m.authContext,
		firstAuthResolutionOverrides(overrides),
	)
}

// GetModelAuth resolves provider auth and overlays the model's static headers.
func (m *modelsImpl) GetModelAuth(ctx context.Context, model Model, overrides ...AuthResolutionOverrides) (Optional[AuthResult], error) {
	resolved, err := m.GetProviderAuth(ctx, model.Provider, overrides...)
	if err != nil {
		return Absent[AuthResult](), err
	}
	result, ok := resolved.Value()
	if !ok {
		return Absent[AuthResult](), nil
	}
	result.Auth.Headers = mergeProviderHeaders(result.Auth.Headers, modelHeaders(model.Headers))
	result.Auth = cloneModelAuth(result.Auth)
	result.Env = cloneProviderEnv(result.Env)
	return Some(result), nil
}

// Login runs the provider-owned flow and persists the returned credential via
// the store's serialized mutation seam.
func (m *modelsImpl) Login(ctx context.Context, providerID ProviderID, authType AuthType, interaction ProviderAuthInteraction) (Credential, error) {
	ctx = nonNilContext(ctx)
	provider, ok := m.GetProvider(providerID)
	if !ok {
		return nil, newModelsError(ModelsErrorCodeProvider, fmt.Sprintf("Unknown provider: %s", providerID), nil)
	}
	if interaction == nil {
		return nil, newModelsError(ModelsErrorCodeAuth, fmt.Sprintf("%s login requires an interaction", provider.Name()), nil)
	}

	auth := provider.Auth()
	var credential Credential
	var err error
	switch authType {
	case AuthTypeAPIKey:
		if auth.APIKey == nil || auth.APIKey.Login == nil {
			return nil, newModelsError(ModelsErrorCodeAuth, fmt.Sprintf("%s does not support %s login", provider.Name(), authType), nil)
		}
		value, loginErr := auth.APIKey.Login(ctx, interaction)
		if value.Type == "" {
			value.Type = AuthTypeAPIKey
		}
		credential, err = value, loginErr
	case AuthTypeOAuth:
		if auth.OAuth == nil || auth.OAuth.Login == nil {
			return nil, newModelsError(ModelsErrorCodeAuth, fmt.Sprintf("%s does not support %s login", provider.Name(), authType), nil)
		}
		value, loginErr := auth.OAuth.Login(ctx, interaction)
		if value.Type == "" {
			value.Type = AuthTypeOAuth
		}
		credential, err = value, loginErr
	default:
		return nil, newModelsError(ModelsErrorCodeAuth, fmt.Sprintf("unsupported auth type %q for %s", authType, providerID), nil)
	}
	if err != nil {
		return nil, err
	}
	if credential.CredentialType() != authType {
		return nil, newModelsError(
			ModelsErrorCodeAuth,
			fmt.Sprintf("%s login returned credential discriminator %q, want %q", provider.Name(), credential.CredentialType(), authType),
			nil,
		)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	persisted, err := m.credentials.Modify(ctx, providerID, func(context.Context, Credential) (Credential, error) {
		return cloneCredential(credential), nil
	}, AuthOperationOptions{})
	if err != nil {
		return nil, newModelsError(ModelsErrorCodeAuth, fmt.Sprintf("credential store modify failed for %s", providerID), err)
	}
	return cloneCredential(persisted), nil
}

// Logout removes provider credentials even if the provider is not registered.
func (m *modelsImpl) Logout(ctx context.Context, providerID ProviderID, options ...AuthOperationOptions) error {
	ctx = nonNilContext(ctx)
	if err := m.credentials.Delete(ctx, providerID, firstAuthOperationOptions(options)); err != nil {
		return newModelsError(ModelsErrorCodeAuth, fmt.Sprintf("credential store delete failed for %s", providerID), err)
	}
	return nil
}

// Stream resolves auth asynchronously and forwards the selected Provider
// stream into an immediately returned outer stream.
func (m *modelsImpl) Stream(ctx context.Context, model Model, input Context, options ...ModelsStreamOption) *AssistantMessageEventStream {
	ctx = nonNilContext(ctx)
	configured := firstModelsStreamOptions(options)
	return lazyModelsStream(ctx, model, func() (*AssistantMessageEventStream, error) {
		if isNilRuntimeValue(configured) {
			return nil, newModelsError(ModelsErrorCodeStream, "Models stream options must not be nil", ErrEventStreamInvariant)
		}
		providerOptions := configured.modelsProviderStreamOptions()
		if isNilRuntimeValue(providerOptions) {
			return nil, newModelsError(ModelsErrorCodeStream, "Provider stream options must not be nil", ErrEventStreamInvariant)
		}
		provider, requestModel, requestOptions, err := m.applyAuth(ctx, model, providerOptions.providerRequestOptions(), configured.modelsRequestTransforms())
		if err != nil {
			return nil, err
		}
		return provider.Stream(ctx, requestModel, input, providerOptions.withProviderRequestOptions(requestOptions)), nil
	})
}

func (m *modelsImpl) Complete(ctx context.Context, model Model, input Context, options ...ModelsStreamOption) (AssistantMessage, error) {
	ctx = nonNilContext(ctx)
	return m.Stream(ctx, model, input, options...).Result(context.WithoutCancel(ctx))
}

// StreamSimple is the simplified typed-options counterpart of Stream.
func (m *modelsImpl) StreamSimple(ctx context.Context, model Model, input Context, options ...ModelsSimpleStreamOptions) *AssistantMessageEventStream {
	ctx = nonNilContext(ctx)
	configured := firstModelsSimpleStreamOptions(options)
	return lazyModelsStream(ctx, model, func() (*AssistantMessageEventStream, error) {
		provider, requestModel, requestOptions, err := m.applyAuth(ctx, model, configured.ProviderRequestOptions, configured.ModelsRequestTransforms)
		if err != nil {
			return nil, err
		}
		configured.SimpleStreamOptions.StreamOptions.ProviderRequestOptions = requestOptions
		return provider.StreamSimple(ctx, requestModel, input, configured.SimpleStreamOptions), nil
	})
}

func (m *modelsImpl) CompleteSimple(ctx context.Context, model Model, input Context, options ...ModelsSimpleStreamOptions) (AssistantMessage, error) {
	ctx = nonNilContext(ctx)
	return m.StreamSimple(ctx, model, input, options...).Result(context.WithoutCancel(ctx))
}

func (m *modelsImpl) FetchDeferred(ctx context.Context, model Model, handle DeferredHandle, options ...ModelsDeferredFetchOptions) (AssistantMessage, error) {
	ctx = nonNilContext(ctx)
	configured := firstModelsDeferredFetchOptions(options)
	provider, ok := m.GetProvider(model.Provider)
	if !ok {
		return modelsTerminalMessage(model, newModelsError(ModelsErrorCodeProvider, fmt.Sprintf("Unknown provider: %s", model.Provider), nil)), nil
	}
	if !provider.SupportsFetchDeferred() {
		return AssistantMessage{}, newModelsError(
			ModelsErrorCodeProvider,
			fmt.Sprintf("Provider %s does not support deferred responses", model.Provider),
			newNotImplemented("Provider.FetchDeferred"),
		)
	}
	stream := lazyModelsStream(ctx, model, func() (*AssistantMessageEventStream, error) {
		provider, requestModel, requestOptions, err := m.applyAuth(ctx, model, configured.ProviderRequestOptions, configured.ModelsRequestTransforms)
		if err != nil {
			return nil, err
		}
		configured.DeferredFetchOptions.ProviderRequestOptions = requestOptions
		return provider.FetchDeferred(ctx, requestModel, handle, configured.DeferredFetchOptions)
	})
	return stream.Result(context.WithoutCancel(ctx))
}

func (m *modelsImpl) CancelDeferred(ctx context.Context, model Model, handle DeferredHandle, options ...ModelsDeferredCancelOptions) error {
	ctx = nonNilContext(ctx)
	provider, ok := m.GetProvider(model.Provider)
	if !ok {
		return newModelsError(ModelsErrorCodeProvider, fmt.Sprintf("Unknown provider: %s", model.Provider), nil)
	}
	if !provider.SupportsCancelDeferred() {
		return newModelsError(
			ModelsErrorCodeProvider,
			fmt.Sprintf("Provider %s does not support deferred responses", model.Provider),
			newNotImplemented("Provider.CancelDeferred"),
		)
	}
	configured := firstModelsDeferredCancelOptions(options)
	provider, requestModel, requestOptions, err := m.applyAuth(ctx, model, configured.DeferredCancelOptions, configured.ModelsRequestTransforms)
	if err != nil {
		return err
	}
	configured.DeferredCancelOptions = requestOptions
	return provider.CancelDeferred(ctx, requestModel, handle, configured.DeferredCancelOptions)
}

func (m *modelsImpl) applyAuth(
	ctx context.Context,
	model Model,
	request ProviderRequestOptions,
	transforms ModelsRequestTransforms,
) (Provider, Model, ProviderRequestOptions, error) {
	provider, ok := m.GetProvider(model.Provider)
	if !ok {
		return nil, Model{}, ProviderRequestOptions{}, newModelsError(ModelsErrorCodeProvider, fmt.Sprintf("Unknown provider: %s", model.Provider), nil)
	}

	overrides := AuthResolutionOverrides{Env: cloneProviderEnv(request.Env)}
	if request.APIKey != nil {
		overrides.APIKey = Some(*request.APIKey)
	}
	resolved, err := m.GetModelAuth(ctx, model, overrides)
	if err != nil {
		return nil, Model{}, ProviderRequestOptions{}, err
	}
	result, configured := resolved.Value()
	if !configured {
		return nil, Model{}, ProviderRequestOptions{}, newModelsError(ModelsErrorCodeAuth, fmt.Sprintf("Provider is not configured: %s", model.Provider), nil)
	}

	assembled := request
	if request.APIKey != nil {
		copy := *request.APIKey
		assembled.APIKey = &copy
	} else if apiKey, ok := result.Auth.APIKey.Value(); ok {
		assembled.APIKey = stringPointerValue(apiKey)
	} else {
		assembled.APIKey = nil
	}
	assembled.Headers = mergeProviderHeaders(result.Auth.Headers, request.Headers)
	if transforms.TransformHeaders != nil {
		headers := assembled.Headers
		if headers == nil {
			headers = ProviderHeaders{}
		}
		headers, err = transforms.TransformHeaders(ctx, cloneProviderHeaders(headers))
		if err != nil {
			return nil, Model{}, ProviderRequestOptions{}, err
		}
		assembled.Headers = cloneProviderHeaders(headers)
	}
	assembled.Env = mergeProviderEnv(result.Env, request.Env)

	requestModel := cloneModel(model)
	if baseURL, ok := result.Auth.BaseURL.Value(); ok && baseURL != "" {
		requestModel.BaseURL = baseURL
	}
	return provider, requestModel, assembled, nil
}

func lazyModelsStream(
	ctx context.Context,
	model Model,
	setup func() (*AssistantMessageEventStream, error),
) *AssistantMessageEventStream {
	outer := NewAssistantMessageEventStream()
	go func() {
		var partial AssistantMessage
		hasPartial := false
		inner, err := setup()
		if err != nil {
			if errors.Is(err, ErrNotImplemented) {
				outer.stream.endWithError(err)
				return
			}
			pushModelsTerminalError(outer, model, AssistantMessage{}, false, err)
			return
		}
		if inner == nil {
			pushModelsTerminalError(outer, model, AssistantMessage{}, false, newModelsError(ModelsErrorCodeStream, "Provider returned a nil stream", ErrEventStreamInvariant))
			return
		}
		for {
			event, ok, nextErr := inner.Next(ctx)
			if nextErr != nil {
				if errors.Is(nextErr, ErrNotImplemented) {
					outer.stream.endWithError(nextErr)
					return
				}
				pushModelsTerminalError(outer, model, partial, hasPartial, nextErr)
				return
			}
			if !ok {
				break
			}
			if snapshot, ok := assistantMessageEventPartial(event); ok {
				partial = snapshot
				hasPartial = true
			}
			outer.Push(event)
		}
		result, resultErr := inner.Result(ctx)
		if resultErr != nil {
			outer.stream.endWithError(resultErr)
			return
		}
		outer.End(result)
	}()
	return outer
}

func pushModelsTerminalError(stream *AssistantMessageEventStream, model Model, partial AssistantMessage, hasPartial bool, err error) {
	message := modelsTerminalMessage(model, err)
	if hasPartial {
		message = CloneAssistantMessage(partial)
		message.Role = MessageRoleAssistant
		message.API = model.API
		message.Provider = model.Provider
		message.Model = model.ID
		message.StopReason = StopReasonError
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			message.StopReason = StopReasonAborted
		}
		message.ErrorMessage = Some(terminalErrorMessage(err))
		message.Timestamp = time.Now().UnixMilli()
	}
	stream.Push(AssistantMessageErrorEvent{Type: AssistantMessageEventTypeError, Reason: message.StopReason, Error: message})
}

func modelsTerminalMessage(model Model, err error) AssistantMessage {
	reason := StopReasonError
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		reason = StopReasonAborted
	}
	return AssistantMessage{
		Role:         MessageRoleAssistant,
		Content:      []AssistantContent{},
		API:          model.API,
		Provider:     model.Provider,
		Model:        model.ID,
		StopReason:   reason,
		ErrorMessage: Some(terminalErrorMessage(err)),
		Timestamp:    time.Now().UnixMilli(),
	}
}

func assistantMessageEventPartial(event AssistantMessageEvent) (AssistantMessage, bool) {
	switch event := event.(type) {
	case AssistantMessageStartEvent:
		return CloneAssistantMessage(event.Partial), true
	case *AssistantMessageStartEvent:
		return CloneAssistantMessage(event.Partial), event != nil
	case AssistantMessageTextStartEvent:
		return CloneAssistantMessage(event.Partial), true
	case *AssistantMessageTextStartEvent:
		return CloneAssistantMessage(event.Partial), event != nil
	case AssistantMessageTextDeltaEvent:
		return CloneAssistantMessage(event.Partial), true
	case *AssistantMessageTextDeltaEvent:
		return CloneAssistantMessage(event.Partial), event != nil
	case AssistantMessageTextEndEvent:
		return CloneAssistantMessage(event.Partial), true
	case *AssistantMessageTextEndEvent:
		return CloneAssistantMessage(event.Partial), event != nil
	case AssistantMessageThinkingStartEvent:
		return CloneAssistantMessage(event.Partial), true
	case *AssistantMessageThinkingStartEvent:
		return CloneAssistantMessage(event.Partial), event != nil
	case AssistantMessageThinkingDeltaEvent:
		return CloneAssistantMessage(event.Partial), true
	case *AssistantMessageThinkingDeltaEvent:
		return CloneAssistantMessage(event.Partial), event != nil
	case AssistantMessageThinkingEndEvent:
		return CloneAssistantMessage(event.Partial), true
	case *AssistantMessageThinkingEndEvent:
		return CloneAssistantMessage(event.Partial), event != nil
	case AssistantMessageToolCallStartEvent:
		return CloneAssistantMessage(event.Partial), true
	case *AssistantMessageToolCallStartEvent:
		return CloneAssistantMessage(event.Partial), event != nil
	case AssistantMessageToolCallDeltaEvent:
		return CloneAssistantMessage(event.Partial), true
	case *AssistantMessageToolCallDeltaEvent:
		return CloneAssistantMessage(event.Partial), event != nil
	case AssistantMessageToolCallEndEvent:
		return CloneAssistantMessage(event.Partial), true
	case *AssistantMessageToolCallEndEvent:
		return CloneAssistantMessage(event.Partial), event != nil
	default:
		return AssistantMessage{}, false
	}
}

func firstModelsStreamOptions(options []ModelsStreamOption) ModelsStreamOption {
	if len(options) == 0 {
		return ModelsStreamOptions{}
	}
	return options[0]
}

func firstModelsSimpleStreamOptions(options []ModelsSimpleStreamOptions) ModelsSimpleStreamOptions {
	if len(options) == 0 {
		return ModelsSimpleStreamOptions{}
	}
	return options[0]
}

func firstModelsDeferredFetchOptions(options []ModelsDeferredFetchOptions) ModelsDeferredFetchOptions {
	if len(options) == 0 {
		return ModelsDeferredFetchOptions{}
	}
	return options[0]
}

func firstModelsDeferredCancelOptions(options []ModelsDeferredCancelOptions) ModelsDeferredCancelOptions {
	if len(options) == 0 {
		return ModelsDeferredCancelOptions{}
	}
	return options[0]
}

func stringPointerValue(value string) *string { return &value }

func (m *modelsImpl) checkProviderAuth(ctx context.Context, provider Provider, credential Credential) (Optional[AuthCheck], error) {
	auth := provider.Auth()
	if _, ok := oauthCredentialValue(credential); ok {
		if auth.OAuth == nil {
			return Absent[AuthCheck](), nil
		}
		return Some(AuthCheck{Source: Some("OAuth"), Type: AuthTypeOAuth}), nil
	}

	apiKeyCredential, hasAPIKeyCredential := apiKeyCredentialValue(credential)
	if credential != nil && !hasAPIKeyCredential {
		return Absent[AuthCheck](), nil
	}
	if auth.APIKey == nil {
		return Absent[AuthCheck](), nil
	}
	if auth.APIKey.Check != nil {
		var stored *APIKeyCredential
		if hasAPIKeyCredential {
			copy := apiKeyCredential
			copy.Env = cloneProviderEnv(copy.Env)
			stored = &copy
		}
		check, err := auth.APIKey.Check(ctx, APIKeyCheckInput{Context: m.authContext, Credential: stored})
		if err != nil {
			return Absent[AuthCheck](), newModelsError(ModelsErrorCodeAuth, fmt.Sprintf("API key auth check failed for provider %s", provider.ID()), err)
		}
		return check, nil
	}

	resolved, err := ResolveProviderAuth(
		ctx,
		ProviderAuthTarget{ID: provider.ID(), Auth: auth},
		m.credentials,
		m.authContext,
		AuthResolutionOverrides{},
	)
	if err != nil {
		return Absent[AuthCheck](), err
	}
	result, ok := resolved.Value()
	if !ok {
		return Absent[AuthCheck](), nil
	}
	return Some(AuthCheck{Source: result.Source, Type: AuthTypeAPIKey}), nil
}

func (m *modelsImpl) readCredential(ctx context.Context, providerID ProviderID, options AuthOperationOptions) (Credential, error) {
	credential, err := m.credentials.Read(ctx, providerID, options)
	if err != nil {
		return nil, newModelsError(ModelsErrorCodeAuth, fmt.Sprintf("credential store read failed for %s", providerID), err)
	}
	if err := validateStoredCredential(providerID, credential); err != nil {
		return nil, err
	}
	return credential, nil
}

func nonNilContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func firstAuthOperationOptions(options []AuthOperationOptions) AuthOperationOptions {
	if len(options) == 0 {
		return AuthOperationOptions{}
	}
	return options[0]
}

func firstAuthResolutionOverrides(overrides []AuthResolutionOverrides) AuthResolutionOverrides {
	if len(overrides) == 0 {
		return AuthResolutionOverrides{}
	}
	return overrides[0]
}

func modelHeaders(headers map[string]string) ProviderHeaders {
	if headers == nil {
		return nil
	}
	converted := make(ProviderHeaders, len(headers))
	for name, value := range headers {
		copy := value
		converted[name] = &copy
	}
	return converted
}

func mergeProviderHeaders(base, override ProviderHeaders) ProviderHeaders {
	if base == nil && override == nil {
		return nil
	}
	merged := cloneProviderHeaders(base)
	if merged == nil {
		merged = make(ProviderHeaders, len(override))
	}
	for name, value := range override {
		for existing := range merged {
			if equalFoldASCII(existing, name) {
				delete(merged, existing)
			}
		}
		if value == nil {
			merged[name] = nil
			continue
		}
		copy := *value
		merged[name] = &copy
	}
	return merged
}

func equalFoldASCII(left, right string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		a, b := left[index], right[index]
		if 'A' <= a && a <= 'Z' {
			a += 'a' - 'A'
		}
		if 'A' <= b && b <= 'Z' {
			b += 'a' - 'A'
		}
		if a != b {
			return false
		}
	}
	return true
}

func cloneModelAuth(auth ModelAuth) ModelAuth {
	auth.Headers = cloneProviderHeaders(auth.Headers)
	return auth
}

func apiKeyCredentialValue(credential Credential) (APIKeyCredential, bool) {
	switch value := credential.(type) {
	case APIKeyCredential:
		return value, true
	case *APIKeyCredential:
		if value != nil {
			return *value, true
		}
	}
	return APIKeyCredential{}, false
}

func oauthCredentialValue(credential Credential) (OAuthCredential, bool) {
	switch value := credential.(type) {
	case OAuthCredential:
		return value, true
	case *OAuthCredential:
		if value != nil {
			return *value, true
		}
	}
	return OAuthCredential{}, false
}

func cloneCredential(credential Credential) Credential {
	if value, ok := apiKeyCredentialValue(credential); ok {
		value.Env = cloneProviderEnv(value.Env)
		return value
	}
	if value, ok := oauthCredentialValue(credential); ok {
		if value.Extra != nil {
			value.Extra = cloneRawMessageMap(value.Extra)
		}
		return value
	}
	return nil
}

func cloneProviderEnv(source ProviderEnv) ProviderEnv {
	if source == nil {
		return nil
	}
	clone := make(ProviderEnv, len(source))
	for key, value := range source {
		clone[key] = value
	}
	return clone
}

func bestEffortProviderModels(provider Provider) (models []Model) {
	defer func() {
		if recover() != nil {
			models = []Model{}
		}
	}()
	return cloneModels(provider.GetModels())
}

func bestEffortFilterModels(provider Provider, models []Model, credential Credential) (filtered []Model, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			filtered = nil
			err = newModelsError(
				ModelsErrorCodeModelSource,
				fmt.Sprintf("provider %s model filter panicked", provider.ID()),
				fmt.Errorf("%v", recovered),
			)
		}
	}()
	return cloneModels(provider.FilterModels(cloneModels(models), cloneCredential(credential))), nil
}

func cloneRawMessageMap(source map[string]json.RawMessage) map[string]json.RawMessage {
	if source == nil {
		return nil
	}
	clone := make(map[string]json.RawMessage, len(source))
	for key, value := range source {
		clone[key] = append(json.RawMessage(nil), value...)
	}
	return clone
}

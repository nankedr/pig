package ai_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nankedr/pig/ai"
)

const (
	modelsPublicationReentrancyProviderID = ai.ProviderID("publication-reentrancy")
	modelsPublicationReentrancyModelID    = "baseline"
	modelsPublicationReentrancyTimeout    = time.Second
	modelsPublicationReentrancyCaseEnv    = "PIG_MODELS_PUBLICATION_REENTRANCY_CASE"
)

type modelsPublicationReentrancyProvider struct {
	ai.Provider
	id      ai.ProviderID
	modelID string
	refresh func(ai.RefreshModelsContext) error
}

func (p *modelsPublicationReentrancyProvider) ID() ai.ProviderID { return p.id }

func (p *modelsPublicationReentrancyProvider) GetModels() []ai.Model {
	if p.modelID == "" {
		return []ai.Model{}
	}
	return []ai.Model{{ID: p.modelID, Provider: p.id}}
}

func (p *modelsPublicationReentrancyProvider) SupportsRefreshModels() bool {
	return p.refresh != nil
}

func (p *modelsPublicationReentrancyProvider) RefreshModels(refresh ai.RefreshModelsContext) error {
	return p.refresh(refresh)
}

type modelsPublicationReentrancyStore struct {
	backing  *ai.InMemoryModelsStore
	onWrite  func()
	onDelete func()
}

func newModelsPublicationReentrancyStore() *modelsPublicationReentrancyStore {
	return &modelsPublicationReentrancyStore{backing: ai.NewInMemoryModelsStore()}
}

func (s *modelsPublicationReentrancyStore) Read(
	ctx context.Context,
	providerID ai.ProviderID,
) (ai.ModelsStoreEntry, bool, error) {
	return s.backing.Read(ctx, providerID)
}

func (s *modelsPublicationReentrancyStore) Write(
	ctx context.Context,
	providerID ai.ProviderID,
	entry ai.ModelsStoreEntry,
) error {
	if err := s.backing.Write(ctx, providerID, entry); err != nil {
		return err
	}
	if s.onWrite != nil {
		s.onWrite()
	}
	return nil
}

func (s *modelsPublicationReentrancyStore) Delete(ctx context.Context, providerID ai.ProviderID) error {
	if err := s.backing.Delete(ctx, providerID); err != nil {
		return err
	}
	if s.onDelete != nil {
		s.onDelete()
	}
	return nil
}

type modelsPublicationReentrancyMutation struct {
	name   string
	mutate func(ai.MutableModels, ai.Provider)
	assert func(*testing.T, ai.MutableModels, ai.Provider)
}

func modelsPublicationReentrancyMutations() []modelsPublicationReentrancyMutation {
	return []modelsPublicationReentrancyMutation{
		{
			name: "set-provider",
			mutate: func(models ai.MutableModels, replacement ai.Provider) {
				models.SetProvider(replacement)
			},
			assert: func(t *testing.T, models ai.MutableModels, replacement ai.Provider) {
				t.Helper()
				got, ok := models.GetProvider(modelsPublicationReentrancyProviderID)
				if !ok || got != replacement {
					t.Fatalf("GetProvider() = (%T, %v), want replacement provider", got, ok)
				}
			},
		},
		{
			name: "delete-provider",
			mutate: func(models ai.MutableModels, _ ai.Provider) {
				models.DeleteProvider(modelsPublicationReentrancyProviderID)
			},
			assert: func(t *testing.T, models ai.MutableModels, _ ai.Provider) {
				t.Helper()
				if _, ok := models.GetProvider(modelsPublicationReentrancyProviderID); ok {
					t.Fatal("GetProvider() found provider after DeleteProvider callback")
				}
			},
		},
		{
			name: "clear-providers",
			mutate: func(models ai.MutableModels, _ ai.Provider) {
				models.ClearProviders()
			},
			assert: func(t *testing.T, models ai.MutableModels, _ ai.Provider) {
				t.Helper()
				if got := models.GetProviders(); len(got) != 0 {
					t.Fatalf("GetProviders() = %v, want empty registry after ClearProviders callback", got)
				}
			},
		},
	}
}

type modelsPublicationReentrancyResult struct {
	published bool
	err       error
}

func TestModelsPublicationStoreWriteCallbackCanMutateRegistry(t *testing.T) {
	for _, mutation := range modelsPublicationReentrancyMutations() {
		t.Run(mutation.name, func(t *testing.T) {
			runModelsPublicationReentrancyChild(t, "write", mutation.name)
		})
	}
}

func TestModelsPublicationStoreDeleteCallbackCanMutateRegistry(t *testing.T) {
	for _, mutation := range modelsPublicationReentrancyMutations() {
		t.Run(mutation.name, func(t *testing.T) {
			runModelsPublicationReentrancyChild(t, "delete", mutation.name)
		})
	}
}

func TestModelsPublicationUpdateCallbackCanMutateRegistry(t *testing.T) {
	for _, mutation := range modelsPublicationReentrancyMutations() {
		t.Run(mutation.name, func(t *testing.T) {
			runModelsPublicationReentrancyChild(t, "update", mutation.name)
		})
	}
}

func TestModelsPublicationUpdateCallbackRejectsSynchronousRefreshSameProvider(t *testing.T) {
	runModelsPublicationReentrancyChild(t, "refresh", "same-provider")
}

func TestModelsPublicationReentrancyChild(t *testing.T) {
	callback, mutationName, ok := strings.Cut(os.Getenv(modelsPublicationReentrancyCaseEnv), "/")
	if !ok {
		t.Skip("publication reentrancy child process only")
	}
	if callback == "refresh" {
		testModelsPublicationUpdateCallbackRefreshesSameProvider(t)
		return
	}
	mutation := modelsPublicationReentrancyMutationNamed(t, mutationName)
	switch callback {
	case "write":
		testModelsPublicationStoreWriteCallback(t, mutation)
	case "delete":
		testModelsPublicationStoreDeleteCallback(t, mutation)
	case "update":
		testModelsPublicationUpdateCallback(t, mutation)
	default:
		t.Fatalf("unknown publication callback %q", callback)
	}
}

func testModelsPublicationUpdateCallbackRefreshesSameProvider(t *testing.T) {
	t.Helper()
	models, provider, _ := modelsPublicationReentrancyRegistry(nil)
	var refreshCalls atomic.Int32
	var updateCalls atomic.Int32
	var nestedResult ai.ModelsRefreshResult
	provider.refresh = func(refresh ai.RefreshModelsContext) error {
		call := refreshCalls.Add(1)
		published, err := refresh.Publish(ai.ModelsPublication{Update: func() {
			updateCalls.Add(1)
			if call == 1 {
				nestedResult = models.Refresh(context.Background(), ai.ModelsRefreshOptions{
					Providers:    []ai.ProviderID{modelsPublicationReentrancyProviderID},
					AllowNetwork: ai.Some(false),
				})
			}
		}})
		if err != nil {
			return err
		}
		if !published {
			return fmt.Errorf("publication %d was rejected", call)
		}
		return nil
	}

	outerResult := models.Refresh(context.Background(), ai.ModelsRefreshOptions{
		Providers:    []ai.ProviderID{modelsPublicationReentrancyProviderID},
		AllowNetwork: ai.Some(false),
	})
	if outerResult.Aborted || len(outerResult.Errors) != 0 {
		t.Fatalf("outer Refresh() = %+v, want clean provider-local supersession", outerResult)
	}
	if nestedResult.Aborted || len(nestedResult.Errors) != 1 {
		t.Fatalf("nested Refresh() = %+v, want one structured reentrancy error", nestedResult)
	}
	nestedErr := nestedResult.Errors[modelsPublicationReentrancyProviderID]
	if nestedErr == nil {
		t.Fatalf("nested Refresh() errors = %+v, want same-provider reentrancy error", nestedResult.Errors)
	}
	var modelsErr *ai.ModelsError
	if !errors.As(nestedErr, &modelsErr) || modelsErr.Code != ai.ModelsErrorCodeModelSource || !errors.Is(nestedErr, ai.ErrEventStreamInvariant) {
		t.Fatalf("nested Refresh() error = %v, want model_source ErrEventStreamInvariant", nestedErr)
	}
	if got := refreshCalls.Load(); got != 1 {
		t.Fatalf("provider refresh calls = %d, want only the outer call", got)
	}
	if got := updateCalls.Load(); got != 1 {
		t.Fatalf("publication update calls = %d, want only the outer call", got)
	}
}

func runModelsPublicationReentrancyChild(t *testing.T, callback, mutation string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*modelsPublicationReentrancyTimeout)
	defer cancel()
	cmd := exec.CommandContext(
		ctx,
		os.Args[0],
		"-test.run=^TestModelsPublicationReentrancyChild$",
		"-test.timeout=5s",
		"-test.v",
	)
	cmd.Env = append(os.Environ(), modelsPublicationReentrancyCaseEnv+"="+callback+"/"+mutation)
	output, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("%s/%s child process timed out; callback may have deadlocked\n%s", callback, mutation, output)
	}
	if err != nil {
		t.Fatalf("%s/%s child process failed: %v\n%s", callback, mutation, err, output)
	}
}

func modelsPublicationReentrancyMutationNamed(
	t *testing.T,
	name string,
) modelsPublicationReentrancyMutation {
	t.Helper()
	for _, mutation := range modelsPublicationReentrancyMutations() {
		if mutation.name == name {
			return mutation
		}
	}
	t.Fatalf("unknown registry mutation %q", name)
	return modelsPublicationReentrancyMutation{}
}

func testModelsPublicationStoreWriteCallback(
	t *testing.T,
	mutation modelsPublicationReentrancyMutation,
) {
	t.Helper()
	store := newModelsPublicationReentrancyStore()
	models, original, replacement := modelsPublicationReentrancyRegistry(store)
	var callbackReadErr error
	store.onWrite = func() {
		callbackReadErr = modelsPublicationReentrancyObserveRegistry(models, original)
		mutation.mutate(models, replacement)
	}

	var updated atomic.Bool
	publicationDone, refreshDone := modelsPublicationReentrancyRefresh(models, original, ai.ModelsPublication{
		Persist: ai.Some(&ai.ModelsStoreEntry{
			Models: []ai.Model{{ID: "persisted-write", Provider: modelsPublicationReentrancyProviderID}},
		}),
		Update: func() { updated.Store(true) },
	})

	publication := awaitModelsPublicationReentrancyResult(t, publicationDone)
	assertModelsPublicationReentrancyRefresh(t, refreshDone)
	if callbackReadErr != nil {
		t.Fatalf("read-only registry access from ModelsStore.Write callback: %v", callbackReadErr)
	}
	if publication.err != nil || publication.published {
		t.Fatalf("Publish() = (%v, %v), want (false, nil) after Write callback invalidates generation", publication.published, publication.err)
	}
	if updated.Load() {
		t.Fatal("ModelsPublication.Update ran after Write callback invalidated generation")
	}
	entry, ok, err := store.backing.Read(context.Background(), modelsPublicationReentrancyProviderID)
	if err != nil || !ok || len(entry.Models) != 1 || entry.Models[0].ID != "persisted-write" {
		t.Fatalf("persisted entry = (%+v, %v, %v), want completed write to remain observable", entry, ok, err)
	}
	mutation.assert(t, models, replacement)
}

func testModelsPublicationStoreDeleteCallback(
	t *testing.T,
	mutation modelsPublicationReentrancyMutation,
) {
	t.Helper()
	store := newModelsPublicationReentrancyStore()
	if err := store.backing.Write(context.Background(), modelsPublicationReentrancyProviderID, ai.ModelsStoreEntry{
		Models: []ai.Model{{ID: "seeded-delete", Provider: modelsPublicationReentrancyProviderID}},
	}); err != nil {
		t.Fatalf("seed ModelsStore: %v", err)
	}

	models, original, replacement := modelsPublicationReentrancyRegistry(store)
	var callbackReadErr error
	store.onDelete = func() {
		callbackReadErr = modelsPublicationReentrancyObserveRegistry(models, original)
		mutation.mutate(models, replacement)
	}

	var updated atomic.Bool
	publicationDone, refreshDone := modelsPublicationReentrancyRefresh(models, original, ai.ModelsPublication{
		Persist: ai.Null[*ai.ModelsStoreEntry](),
		Update:  func() { updated.Store(true) },
	})

	publication := awaitModelsPublicationReentrancyResult(t, publicationDone)
	assertModelsPublicationReentrancyRefresh(t, refreshDone)
	if callbackReadErr != nil {
		t.Fatalf("read-only registry access from ModelsStore.Delete callback: %v", callbackReadErr)
	}
	if publication.err != nil || publication.published {
		t.Fatalf("Publish() = (%v, %v), want (false, nil) after Delete callback invalidates generation", publication.published, publication.err)
	}
	if updated.Load() {
		t.Fatal("ModelsPublication.Update ran after Delete callback invalidated generation")
	}
	if entry, ok, err := store.backing.Read(context.Background(), modelsPublicationReentrancyProviderID); err != nil || ok {
		t.Fatalf("persisted entry after delete = (%+v, %v, %v), want completed delete to remain observable", entry, ok, err)
	}
	mutation.assert(t, models, replacement)
}

func testModelsPublicationUpdateCallback(
	t *testing.T,
	mutation modelsPublicationReentrancyMutation,
) {
	t.Helper()
	models, original, replacement := modelsPublicationReentrancyRegistry(nil)
	var callbackReadErr error
	var updated atomic.Bool
	publicationDone, refreshDone := modelsPublicationReentrancyRefresh(models, original, ai.ModelsPublication{
		Update: func() {
			callbackReadErr = modelsPublicationReentrancyObserveRegistry(models, original)
			updated.Store(true)
			mutation.mutate(models, replacement)
		},
	})

	publication := awaitModelsPublicationReentrancyResult(t, publicationDone)
	assertModelsPublicationReentrancyRefresh(t, refreshDone)
	if callbackReadErr != nil {
		t.Fatalf("read-only registry access from ModelsPublication.Update callback: %v", callbackReadErr)
	}
	if publication.err != nil || !publication.published {
		t.Fatalf("Publish() = (%v, %v), want (true, nil) after Update callback mutates registry", publication.published, publication.err)
	}
	if !updated.Load() {
		t.Fatal("ModelsPublication.Update did not run")
	}
	mutation.assert(t, models, replacement)
}

func modelsPublicationReentrancyRegistry(
	store ai.ModelsStore,
) (ai.MutableModels, *modelsPublicationReentrancyProvider, *modelsPublicationReentrancyProvider) {
	options := ai.CreateModelsOptions{}
	if store != nil {
		options.ModelsStore = store
	}
	models := ai.CreateModels(options)
	original := &modelsPublicationReentrancyProvider{
		id:      modelsPublicationReentrancyProviderID,
		modelID: modelsPublicationReentrancyModelID,
	}
	replacement := &modelsPublicationReentrancyProvider{
		id:      modelsPublicationReentrancyProviderID,
		modelID: "replacement",
	}
	models.SetProvider(original)
	return models, original, replacement
}

func modelsPublicationReentrancyRefresh(
	models ai.MutableModels,
	provider *modelsPublicationReentrancyProvider,
	publication ai.ModelsPublication,
) (<-chan modelsPublicationReentrancyResult, <-chan ai.ModelsRefreshResult) {
	publicationDone := make(chan modelsPublicationReentrancyResult, 1)
	provider.refresh = func(refresh ai.RefreshModelsContext) error {
		published, err := refresh.Publish(publication)
		publicationDone <- modelsPublicationReentrancyResult{published: published, err: err}
		return err
	}
	refreshDone := make(chan ai.ModelsRefreshResult, 1)
	go func() {
		refreshDone <- models.Refresh(context.Background(), ai.ModelsRefreshOptions{
			Providers:    []ai.ProviderID{modelsPublicationReentrancyProviderID},
			AllowNetwork: ai.Some(false),
		})
	}()
	return publicationDone, refreshDone
}

func modelsPublicationReentrancyObserveRegistry(
	models ai.MutableModels,
	wantProvider ai.Provider,
) error {
	providers := models.GetProviders()
	if len(providers) != 1 || providers[0] != wantProvider {
		return fmt.Errorf("GetProviders() = %v, want original provider", providers)
	}
	provider, ok := models.GetProvider(modelsPublicationReentrancyProviderID)
	if !ok || provider != wantProvider {
		return fmt.Errorf("GetProvider() = (%T, %v), want original provider", provider, ok)
	}
	modelsForProvider := models.GetModels(modelsPublicationReentrancyProviderID)
	if len(modelsForProvider) != 1 || modelsForProvider[0].ID != modelsPublicationReentrancyModelID {
		return fmt.Errorf("GetModels() = %+v, want baseline model", modelsForProvider)
	}
	if model, found := models.GetModel(modelsPublicationReentrancyProviderID, modelsPublicationReentrancyModelID); !found || model.ID != modelsPublicationReentrancyModelID {
		return fmt.Errorf("GetModel() = (%+v, %v), want baseline model", model, found)
	}
	return nil
}

func awaitModelsPublicationReentrancyResult(
	t *testing.T,
	done <-chan modelsPublicationReentrancyResult,
) modelsPublicationReentrancyResult {
	t.Helper()
	select {
	case result := <-done:
		return result
	case <-time.After(modelsPublicationReentrancyTimeout):
		t.Fatal("timed out waiting for ModelsPublication.Publish; callback may have deadlocked")
		return modelsPublicationReentrancyResult{}
	}
}

func assertModelsPublicationReentrancyRefresh(t *testing.T, done <-chan ai.ModelsRefreshResult) {
	t.Helper()
	select {
	case result := <-done:
		if result.Aborted || len(result.Errors) != 0 {
			t.Fatalf("Refresh() = %+v, want clean provider-local supersession", result)
		}
	case <-time.After(modelsPublicationReentrancyTimeout):
		t.Fatal("timed out waiting for Refresh after publication callback")
	}
}

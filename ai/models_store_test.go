package ai

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
)

var (
	_ ModelsStore                 = (*InMemoryModelsStore)(nil)
	_ func() *InMemoryModelsStore = NewInMemoryModelsStore
)

func TestModelsStoreEntryExposesOptionalUnixMillisecondMetadata(t *testing.T) {
	t.Parallel()

	entry := ModelsStoreEntry{
		Models:       []Model{{ID: "model-1"}},
		LastModified: Some[int64](1_700_000_000_001),
		CheckedAt:    Some[int64](1_700_000_000_002),
		ETag:         Some(`"opaque-validator"`),
	}
	if len(entry.Models) != 1 || entry.Models[0].ID != "model-1" {
		t.Fatalf("ModelsStoreEntry.Models = %#v", entry.Models)
	}
	if got, ok := entry.LastModified.Value(); !ok || got != 1_700_000_000_001 {
		t.Fatalf("LastModified.Value() = (%d, %t)", got, ok)
	}
	if got, ok := entry.CheckedAt.Value(); !ok || got != 1_700_000_000_002 {
		t.Fatalf("CheckedAt.Value() = (%d, %t)", got, ok)
	}
	if got, ok := entry.ETag.Value(); !ok || got != `"opaque-validator"` {
		t.Fatalf("ETag.Value() = (%q, %t)", got, ok)
	}

	var _ ModelsStoreOperationOptions
}

func TestInMemoryModelsStoreReadsWritesAndDeletesEntries(t *testing.T) {
	t.Parallel()

	store := NewInMemoryModelsStore()
	ctx := context.Background()
	providerID := ProviderID("acme-cloud")

	if entry, ok, err := store.Read(ctx, providerID); err != nil || ok || len(entry.Models) != 0 {
		t.Fatalf("Read(missing) = (%#v, %t, %v), want zero entry, false, nil", entry, ok, err)
	}

	want := ModelsStoreEntry{Models: []Model{{ID: "model-1"}}, CheckedAt: Some[int64](123)}
	if err := store.Write(ctx, providerID, want); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if got, ok, err := store.Read(ctx, providerID); err != nil || !ok || len(got.Models) != 1 || got.Models[0].ID != "model-1" {
		t.Fatalf("Read(written) = (%#v, %t, %v)", got, ok, err)
	}

	if err := store.Delete(ctx, providerID); err != nil {
		t.Fatalf("Delete(existing) error = %v", err)
	}
	if err := store.Delete(ctx, providerID); err != nil {
		t.Fatalf("Delete(missing) error = %v, want idempotent success", err)
	}
	if _, ok, err := store.Read(ctx, providerID); err != nil || ok {
		t.Fatalf("Read(deleted) = (_, %t, %v), want false, nil", ok, err)
	}
}

func TestInMemoryModelsStoreHonorsCanceledContextBeforeEveryOperation(t *testing.T) {
	t.Parallel()

	store := NewInMemoryModelsStore()
	providerID := ProviderID("acme-cloud")
	original := ModelsStoreEntry{Models: []Model{{ID: "original"}}}
	if err := store.Write(context.Background(), providerID, original); err != nil {
		t.Fatalf("setup Write() error = %v", err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()

	if entry, ok, err := store.Read(canceled, providerID); !errors.Is(err, context.Canceled) || ok || len(entry.Models) != 0 {
		t.Fatalf("Read(canceled) = (%#v, %t, %v), want zero entry, false, context.Canceled", entry, ok, err)
	}
	if err := store.Write(canceled, providerID, ModelsStoreEntry{Models: []Model{{ID: "replacement"}}}); !errors.Is(err, context.Canceled) {
		t.Fatalf("Write(canceled) error = %v, want context.Canceled", err)
	}
	if err := store.Delete(canceled, providerID); !errors.Is(err, context.Canceled) {
		t.Fatalf("Delete(canceled) error = %v, want context.Canceled", err)
	}

	got, ok, err := store.Read(context.Background(), providerID)
	if err != nil || !ok || len(got.Models) != 1 || got.Models[0].ID != "original" {
		t.Fatalf("entry after canceled mutations = (%#v, %t, %v), want original entry", got, ok, err)
	}
}

func TestInMemoryModelsStoreReadRechecksCanceledContextAfterWaitingForLock(t *testing.T) {
	store := NewInMemoryModelsStore()
	providerID := ProviderID("acme-cloud")
	if err := store.Write(context.Background(), providerID, ModelsStoreEntry{Models: []Model{{ID: "original"}}}); err != nil {
		t.Fatalf("setup Write() error = %v", err)
	}

	store.mu.Lock()
	ctx, cancel := context.WithCancel(context.Background())
	firstErrObserved := make(chan struct{})
	ctxWithObservedErr := &observedErrContext{Context: ctx, firstErrObserved: firstErrObserved}
	type readResult struct {
		entry ModelsStoreEntry
		ok    bool
		err   error
	}
	result := make(chan readResult, 1)
	go func() {
		entry, ok, err := store.Read(ctxWithObservedErr, providerID)
		result <- readResult{entry: entry, ok: ok, err: err}
	}()

	<-firstErrObserved
	cancel()
	store.mu.Unlock()
	got := <-result

	if !errors.Is(got.err, context.Canceled) || got.ok || len(got.entry.Models) != 0 {
		t.Fatalf("Read(canceled while waiting for lock) = (%#v, %t, %v), want zero entry, false, context.Canceled", got.entry, got.ok, got.err)
	}
}

func TestInMemoryModelsStoreWriteRechecksCanceledContextAfterWaitingForLock(t *testing.T) {
	store := NewInMemoryModelsStore()
	providerID := ProviderID("acme-cloud")
	if err := store.Write(context.Background(), providerID, ModelsStoreEntry{Models: []Model{{ID: "original"}}}); err != nil {
		t.Fatalf("setup Write() error = %v", err)
	}

	store.mu.Lock()
	ctx, cancel := context.WithCancel(context.Background())
	firstErrObserved := make(chan struct{})
	ctxWithObservedErr := &observedErrContext{Context: ctx, firstErrObserved: firstErrObserved}
	result := make(chan error, 1)
	go func() {
		result <- store.Write(ctxWithObservedErr, providerID, ModelsStoreEntry{Models: []Model{{ID: "replacement"}}})
	}()

	<-firstErrObserved
	cancel()
	store.mu.Unlock()
	err := <-result
	got := readStoredEntry(t, store, providerID)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Write(canceled while waiting for lock) error = %v, want context.Canceled", err)
	}
	if len(got.Models) != 1 || got.Models[0].ID != "original" {
		t.Fatalf("entry after canceled Write() = %#v, want original entry", got)
	}
}

func TestInMemoryModelsStoreDeleteRechecksCanceledContextAfterWaitingForLock(t *testing.T) {
	store := NewInMemoryModelsStore()
	providerID := ProviderID("acme-cloud")
	if err := store.Write(context.Background(), providerID, ModelsStoreEntry{Models: []Model{{ID: "original"}}}); err != nil {
		t.Fatalf("setup Write() error = %v", err)
	}

	store.mu.Lock()
	ctx, cancel := context.WithCancel(context.Background())
	firstErrObserved := make(chan struct{})
	ctxWithObservedErr := &observedErrContext{Context: ctx, firstErrObserved: firstErrObserved}
	result := make(chan error, 1)
	go func() {
		result <- store.Delete(ctxWithObservedErr, providerID)
	}()

	<-firstErrObserved
	cancel()
	store.mu.Unlock()
	err := <-result
	got := readStoredEntry(t, store, providerID)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Delete(canceled while waiting for lock) error = %v, want context.Canceled", err)
	}
	if len(got.Models) != 1 || got.Models[0].ID != "original" {
		t.Fatalf("entry after canceled Delete() = %#v, want original entry", got)
	}
}

func TestInMemoryModelsStoreClonesEveryMutableModelFieldOnWriteAndRead(t *testing.T) {
	t.Parallel()

	store := NewInMemoryModelsStore()
	providerID := ProviderID("acme-cloud")
	entry := ModelsStoreEntry{
		Models: []Model{{
			ID:    "model-1",
			Input: []ModelInput{ModelInputText, ModelInputImage},
			Cost: ModelCost{Tiers: []ModelCostTier{{
				InputTokensAbove: 200_000,
				ModelCostRates:   ModelCostRates{Input: 3},
			}}},
			ThinkingLevelMap: ThinkingLevelMap{ModelThinkingLevelHigh: Some("high")},
			SamplingParams:   map[string]json.RawMessage{"top_p": json.RawMessage(`0.8`)},
			Headers:          map[string]string{"x-model": "original"},
			Compat:           Some(json.RawMessage(`{"nested":true}`)),
		}},
		LastModified: Some[int64](100),
		CheckedAt:    Null[int64](),
		ETag:         Some(`"etag"`),
	}
	if err := store.Write(context.Background(), providerID, entry); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	mutateStoreEntry(&entry)
	first := readStoredEntry(t, store, providerID)
	assertOriginalStoreEntry(t, first)

	mutateStoreEntry(&first)
	second := readStoredEntry(t, store, providerID)
	assertOriginalStoreEntry(t, second)
}

func TestInMemoryModelsStoreSupportsConcurrentAccess(t *testing.T) {
	t.Parallel()

	store := NewInMemoryModelsStore()
	ctx := context.Background()
	const workers = 16
	const operations = 100
	start := make(chan struct{})
	errs := make(chan error, workers*operations)
	var wait sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		wait.Add(1)
		go func(worker int) {
			defer wait.Done()
			<-start
			providerID := ProviderID("shared-provider")
			for operation := 0; operation < operations; operation++ {
				switch (worker + operation) % 3 {
				case 0:
					errs <- store.Write(ctx, providerID, ModelsStoreEntry{Models: []Model{{
						ID:             "model",
						SamplingParams: map[string]json.RawMessage{"worker": json.RawMessage(`1`)},
					}}})
				case 1:
					_, _, err := store.Read(ctx, providerID)
					errs <- err
				case 2:
					errs <- store.Delete(ctx, providerID)
				}
			}
		}(worker)
	}
	close(start)
	wait.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent store operation error = %v", err)
		}
	}
}

func mutateStoreEntry(entry *ModelsStoreEntry) {
	entry.Models[0].ID = "mutated"
	entry.Models[0].Input[0] = ModelInputImage
	entry.Models[0].Cost.Tiers[0].Input = 99
	entry.Models[0].ThinkingLevelMap[ModelThinkingLevelHigh] = Some("mutated")
	entry.Models[0].SamplingParams["top_p"][0] = '9'
	entry.Models[0].Headers["x-model"] = "mutated"
	compat, _ := entry.Models[0].Compat.Value()
	compat[0] = '['
	entry.Models = append(entry.Models, Model{ID: "added"})
}

func readStoredEntry(t *testing.T, store ModelsStore, providerID ProviderID) ModelsStoreEntry {
	t.Helper()
	entry, ok, err := store.Read(context.Background(), providerID)
	if err != nil || !ok {
		t.Fatalf("Read() = (%#v, %t, %v), want entry", entry, ok, err)
	}
	return entry
}

func assertOriginalStoreEntry(t *testing.T, entry ModelsStoreEntry) {
	t.Helper()
	if len(entry.Models) != 1 {
		t.Fatalf("len(Models) = %d, want 1", len(entry.Models))
	}
	model := entry.Models[0]
	compat, compatOK := model.Compat.Value()
	if model.ID != "model-1" ||
		len(model.Input) != 2 || model.Input[0] != ModelInputText ||
		len(model.Cost.Tiers) != 1 || model.Cost.Tiers[0].Input != 3 ||
		model.ThinkingLevelMap[ModelThinkingLevelHigh] != Some("high") ||
		string(model.SamplingParams["top_p"]) != `0.8` ||
		model.Headers["x-model"] != "original" ||
		!compatOK || string(compat) != `{"nested":true}` {
		t.Fatalf("stored model was mutated through an alias: %#v, compat=%s", model, compat)
	}
	if lastModified, ok := entry.LastModified.Value(); !ok || lastModified != 100 {
		t.Fatalf("LastModified.Value() = (%d, %t), want (100, true)", lastModified, ok)
	}
	if !entry.CheckedAt.IsNull() {
		t.Fatal("CheckedAt lost explicit null state")
	}
	if etag, ok := entry.ETag.Value(); !ok || etag != `"etag"` {
		t.Fatalf("ETag.Value() = (%q, %t)", etag, ok)
	}
}

type observedErrContext struct {
	context.Context
	firstErrObserved chan struct{}
	once             sync.Once
}

func (c *observedErrContext) Err() error {
	err := c.Context.Err()
	c.once.Do(func() { close(c.firstErrObserved) })
	return err
}

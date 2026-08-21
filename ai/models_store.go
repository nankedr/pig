package ai

import (
	"context"
	"sync"
)

// ModelsStoreEntry is one provider's persisted model-catalog snapshot. Time
// values use Unix milliseconds, matching the fixed Pi baseline's Date.now().
type ModelsStoreEntry struct {
	Models       []Model          `json:"models"`
	LastModified Optional[int64]  `json:"lastModified,omitzero"`
	CheckedAt    Optional[int64]  `json:"checkedAt,omitzero"`
	ETag         Optional[string] `json:"etag,omitzero"`
}

// ModelsStoreOperationOptions retains the fixed-baseline public symbol. Go
// operations receive cancellation through context.Context instead.
type ModelsStoreOperationOptions struct{}

// ModelsStore persists model-catalog snapshots keyed by provider. Methods are
// called without a Models collection lock and may synchronously use its
// registry mutators. Implementations must honor a canceled context before
// mutating storage.
type ModelsStore interface {
	Read(ctx context.Context, providerID ProviderID) (ModelsStoreEntry, bool, error)
	Write(ctx context.Context, providerID ProviderID, entry ModelsStoreEntry) error
	Delete(ctx context.Context, providerID ProviderID) error
}

// InMemoryModelsStore is a concurrent, process-local ModelsStore. Stored and
// returned entries are independent snapshots.
type InMemoryModelsStore struct {
	mu      sync.RWMutex
	entries map[ProviderID]ModelsStoreEntry
}

// NewInMemoryModelsStore returns an empty in-memory model store.
func NewInMemoryModelsStore() *InMemoryModelsStore {
	return &InMemoryModelsStore{entries: make(map[ProviderID]ModelsStoreEntry)}
}

// Read returns an independent snapshot for providerID.
func (s *InMemoryModelsStore) Read(ctx context.Context, providerID ProviderID) (ModelsStoreEntry, bool, error) {
	if err := ctx.Err(); err != nil {
		return ModelsStoreEntry{}, false, err
	}

	s.mu.RLock()
	if err := ctx.Err(); err != nil {
		s.mu.RUnlock()
		return ModelsStoreEntry{}, false, err
	}
	entry, ok := s.entries[providerID]
	if ok {
		entry = cloneModelsStoreEntry(entry)
	}
	s.mu.RUnlock()
	return entry, ok, nil
}

// Write replaces providerID's entry with an independent snapshot.
func (s *InMemoryModelsStore) Write(ctx context.Context, providerID ProviderID, entry ModelsStoreEntry) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	entry = cloneModelsStoreEntry(entry)
	s.mu.Lock()
	if err := ctx.Err(); err != nil {
		s.mu.Unlock()
		return err
	}
	s.entries[providerID] = entry
	s.mu.Unlock()
	return nil
}

// Delete removes providerID. Deleting a missing entry succeeds.
func (s *InMemoryModelsStore) Delete(ctx context.Context, providerID ProviderID) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	s.mu.Lock()
	if err := ctx.Err(); err != nil {
		s.mu.Unlock()
		return err
	}
	delete(s.entries, providerID)
	s.mu.Unlock()
	return nil
}

func cloneModelsStoreEntry(entry ModelsStoreEntry) ModelsStoreEntry {
	clone := entry
	if entry.Models != nil {
		clone.Models = make([]Model, len(entry.Models))
		for index, model := range entry.Models {
			clone.Models[index] = cloneModel(model)
		}
	}
	return clone
}

var _ ModelsStore = (*InMemoryModelsStore)(nil)

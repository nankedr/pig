package codingagent

import (
	_ "embed"
	"encoding/json"
	"sort"

	"github.com/nankedr/pig/ai"
)

//go:embed data/models.json
var catalogSnapshot string

type snapshotProvider struct {
	ai.Provider
	models []ai.Model
}

func (p snapshotProvider) GetModels() []ai.Model { return cloneRuntimeModels(p.models) }
func (p snapshotProvider) GetModel(id string) (ai.Model, bool) {
	for _, m := range p.models {
		if m.ID == id {
			return cloneRuntimeModels([]ai.Model{m})[0], true
		}
	}
	return ai.Model{}, false
}

func installCatalogSnapshot(models ai.MutableModels) error {
	var catalog map[string]map[string]ai.Model
	if err := json.Unmarshal([]byte(catalogSnapshot), &catalog); err != nil {
		return err
	}
	for _, provider := range models.GetProviders() {
		group := catalog[string(provider.ID())]
		if len(group) == 0 {
			continue
		}
		ids := make([]string, 0, len(group))
		for id := range group {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		entries := make([]ai.Model, 0, len(ids))
		for _, id := range ids {
			entries = append(entries, group[id])
		}
		models.SetProvider(snapshotProvider{Provider: provider, models: entries})
	}
	return nil
}
